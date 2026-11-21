package persona.feature.machines

import androidx.activity.ComponentActivity
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner
import androidx.lifecycle.viewmodel.compose.LocalViewModelStoreOwner
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNotSame
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.annotation.Config
import persona.core.contracts.Machine
import persona.core.contracts.MachineLinkSwitch
import persona.core.contracts.Session
import persona.core.contracts.api.AgentLauncher
import persona.core.contracts.api.ClaudeSetting
import persona.core.contracts.api.HerdrState
import persona.core.contracts.api.LauncherQuotaAnswer
import persona.core.contracts.api.MachineManagement
import persona.core.contracts.api.SessionsRepository
import kotlinx.serialization.json.JsonElement

/** A computer detail retained in two navigation/tab stores still owns only one confirmed switch. */
@RunWith(AndroidJUnit4::class)
class MachineActionsRetentionTest {
    @get:Rule val rule = createAndroidComposeRule<ComponentActivity>()

    private class Owner : ViewModelStoreOwner {
        override val viewModelStore = ViewModelStore()
    }

    private class Sessions : SessionsRepository {
        override val machines = MutableStateFlow(listOf(Machine("lab", linkSwitch = MachineLinkSwitch(available = true))))
        override val sessions = MutableStateFlow(emptyList<Session>())
        override suspend fun refresh() = Unit
    }

    private class Management : MachineManagement {
        val gate = CompletableDeferred<Unit>()
        val calls = mutableListOf<String>()
        override suspend fun startLinkSwitch(machineId: String): MachineLinkSwitch {
            calls += machineId
            gate.await()
            return MachineLinkSwitch(available = false, phase = "checking")
        }
        override suspend fun launchers(machineId: String) = emptyList<AgentLauncher>()
        override suspend fun launcherQuota(machineId: String, name: String, refresh: Boolean) = LauncherQuotaAnswer()
        override suspend fun reconnect(machineId: String) = Unit
        override suspend fun deploy(machineId: String) = Unit
        override suspend fun herdr(machineId: String) = HerdrState(false, false, false)
        override suspend fun startHerdr(machineId: String) = HerdrState(false, false, false)
        override suspend fun setEngine(machineId: String, engine: String) = Machine(machineId)
        override suspend fun claudeSettings(machineId: String) = emptyList<ClaudeSetting>()
        override suspend fun setClaudeSetting(machineId: String, key: String, value: JsonElement) = emptyList<ClaudeSetting>()
        override suspend fun codexSettings(machineId: String) = emptyList<ClaudeSetting>()
        override suspend fun setCodexSetting(machineId: String, key: String, value: JsonElement) = emptyList<ClaudeSetting>()
    }

    @Config(sdk = [35])
    @Test fun retainedDetailsShareOneOperationAcrossNavigationStores() {
        val management = Management()
        val sessions = Sessions()
        val firstOwner = Owner()
        val secondOwner = Owner()
        var first: MachineActionsViewModel? = null
        var second: MachineActionsViewModel? = null
        try {
            rule.setContent {
                CompositionLocalProvider(LocalViewModelStoreOwner provides firstOwner) {
                    first = rememberMachineActions(management, sessions, "lab")
                }
                CompositionLocalProvider(LocalViewModelStoreOwner provides secondOwner) {
                    second = rememberMachineActions(management, sessions, "lab")
                }
            }
            rule.runOnIdle {
                assertNotNull(first)
                assertSame(first, second)
                first!!.startLinkSwitch("lab")
                second!!.startLinkSwitch("lab")
            }
            rule.waitUntil(timeoutMillis = 5_000) { management.calls == listOf("lab") }
            assertEquals(listOf("lab"), management.calls)
        } finally {
            management.gate.complete(Unit)
            firstOwner.viewModelStore.clear()
            secondOwner.viewModelStore.clear()
        }
    }

    @Config(sdk = [35])
    @Test fun replacementPairingDoesNotInheritAnAcceptedAttempt() {
        val management = Management()
        val sessions = Sessions()
        var scope by mutableStateOf("pairing-a")
        var previous: MachineActionsViewModel? = null
        var current: MachineActionsViewModel? = null
        try {
            rule.setContent {
                current = rememberMachineActions(management, sessions, "lab", operationScope = scope)
            }
            rule.runOnIdle { current!!.startLinkSwitch("lab") }
            rule.waitUntil(timeoutMillis = 5_000) { management.calls == listOf("lab") }
            management.gate.complete(Unit)
            rule.waitUntil(timeoutMillis = 5_000) { current!!.state.value.linkSwitches["lab"]?.accepted != null }
            rule.runOnIdle {
                // The replacement hub reuses the machine id but has no Link-switch field at all.
                sessions.machines.value = listOf(Machine("lab"))
                previous = current
                scope = "pairing-b"
            }
            rule.waitForIdle()
            rule.runOnIdle {
                assertNotSame(previous, current)
                assertTrue(current!!.state.value.linkSwitches.isEmpty())
                assertNull(shownLinkSwitch(null, current!!.state.value.linkSwitches["lab"]))
                current!!.startLinkSwitch("lab")
            }
            rule.waitForIdle()
            // No inherited Starting state, card or POST appears on the replacement hub.
            assertEquals(listOf("lab"), management.calls)
            assertTrue(current!!.state.value.linkSwitches.isEmpty())
        } finally {
            management.gate.complete(Unit)
            rule.activity.viewModelStore.clear()
        }
    }
}
