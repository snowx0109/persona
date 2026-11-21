package persona.feature.machines

import androidx.lifecycle.viewModelScope
import persona.core.contracts.*
import persona.core.contracts.api.*
import persona.core.designsystem.UiText
import persona.core.hub.HubError
import persona.core.hub.HubFailure
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.*
import kotlinx.serialization.json.JsonElement
import org.junit.Assert.*
import org.junit.Test
import java.io.IOException

@OptIn(ExperimentalCoroutinesApi::class)
class MachineActionsTest {
    private class Sessions(machines: List<Machine>) : SessionsRepository {
        override val machines = MutableStateFlow(machines)
        override val sessions = MutableStateFlow(emptyList<Session>())
        var refreshes = 0
        val refreshedMachines = mutableListOf<String>()
        var refreshFailure: Exception? = null
        var refreshGate: CompletableDeferred<Unit>? = null
        var onRefresh: () -> Unit = {}
        override suspend fun refresh() { refreshGate?.await(); refreshes++; refreshFailure?.let { throw it }; onRefresh() }
        override suspend fun refreshMachine(machineId: String) { refreshedMachines += machineId; refresh() }
    }

    private open class Management : MachineManagement {
        val calls = mutableListOf<String>()
        var failure: Exception? = null
        var onReconnect: () -> Unit = {}
        override suspend fun launchers(machineId: String): List<AgentLauncher> {
            calls += "launchers:$machineId"; failure?.let { throw it }
            return listOf(AgentLauncher("claude", model = "anthropic/claude-opus-5"), AgentLauncher("plain"))
        }
        override suspend fun launcherQuota(machineId: String, name: String, refresh: Boolean) = persona.core.contracts.api.LauncherQuotaAnswer()
        override suspend fun reconnect(machineId: String) { calls += "reconnect:$machineId"; failure?.let { throw it }; onReconnect() }
        override suspend fun deploy(machineId: String) { calls += "deploy:$machineId"; failure?.let { throw it } }
        override suspend fun rename(machineId: String, name: String): String {
            calls += "rename:$machineId:$name"; failure?.let { throw it }; return name.ifBlank { machineId }
        }
        override suspend fun herdr(machineId: String): persona.core.contracts.api.HerdrState = error("unused")
        override suspend fun startHerdr(machineId: String): persona.core.contracts.api.HerdrState = error("unused")
        override suspend fun setEngine(machineId: String, engine: String) = Machine(machineId, engine = engine)
        override suspend fun claudeSettings(machineId: String) = emptyList<ClaudeSetting>()
        override suspend fun setClaudeSetting(machineId: String, key: String, value: JsonElement) = emptyList<ClaudeSetting>()
        override suspend fun codexSettings(machineId: String) = emptyList<ClaudeSetting>()
        override suspend fun setCodexSetting(machineId: String, key: String, value: JsonElement) = emptyList<ClaudeSetting>()
        /** Null: an older hub without the route. */
        var botmuxOn: Boolean? = true
        var botmuxFailure: Exception? = null
        var linkAnswer = MachineLinkSwitch(available = false, phase = "checking", at = "2026-11-20T10:00:00Z")
        var linkFailure: Exception? = null
        var linkGate: CompletableDeferred<Unit>? = null
        override suspend fun startLinkSwitch(machineId: String): MachineLinkSwitch {
            calls += "link-switch:$machineId"
            linkGate?.await()
            linkFailure?.let { throw it }
            return linkAnswer
        }
        override suspend fun botmux(machineId: String): Boolean? { calls += "botmux:$machineId"; return botmuxOn }
        override suspend fun setBotmux(machineId: String, on: Boolean): Boolean {
            calls += "set-botmux:$machineId:$on"; botmuxFailure?.let { throw it }; botmuxOn = on; return on
        }
    }

    private fun <T> test(block: suspend TestScope.() -> T) = runTest {
        Dispatchers.setMain(StandardTestDispatcher(testScheduler))
        try { block() } finally { Dispatchers.resetMain() }
    }

    @Test fun reconnectWaitsForTheMachineToComeOnlineAndSaysSo() = test {
        val sessions = Sessions(listOf(Machine("lab", online = false, error = "ssh: connection refused")))
        val management = Management().apply { onReconnect = { sessions.machines.value = listOf(Machine("lab", online = true)) } }
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.reconnect("lab")
            model.reconnect("lab")
            runCurrent()
            assertEquals(listOf("reconnect:lab"), management.calls)
            assertEquals(MachineNotice.Connected, model.notices.first())
            assertFalse(model.state.value.reconnecting)
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun reconnectReadsTheListWhenNoPushSaysTheMachineIsBack() = test {
        val sessions = Sessions(listOf(Machine("lab", online = false)))
        // The hub has it back, but no machines push arrives: the next read of the list shows it.
        sessions.onRefresh = { sessions.machines.value = listOf(Machine("lab", online = true)) }
        val model = MachineActionsViewModel(Management(), sessions)
        try {
            model.reconnect("lab")
            runCurrent()
            assertTrue(model.state.value.reconnecting)
            advanceTimeBy(3_001)
            runCurrent()
            assertEquals(MachineNotice.Connected, model.notices.first())
            assertEquals(1, sessions.refreshes)
            advanceTimeBy(30_000)
            runCurrent()
            assertEquals(1, sessions.refreshes)
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun reconnectGivesUpAfterTheWaitWithTheMachinesError() = test {
        val sessions = Sessions(listOf(Machine("lab", online = false, error = "ssh: connection refused")))
        val model = MachineActionsViewModel(Management(), sessions, reconnectWaitMillis = 15_000)
        try {
            model.reconnect("lab")
            runCurrent()
            assertTrue(model.state.value.reconnecting)
            advanceTimeBy(15_001)
            runCurrent()
            // The hub's error is read as the offline card reads it: why and what to do, not the raw English (#4537).
            assertEquals(MachineNotice.NotYet(UiText.Res(R.string.machines_offline_unreachable_ssh)), model.notices.first())
            assertFalse(model.state.value.reconnecting)
            val refused = Management().apply { failure = HubFailure(HubError.Server, "machine is offline", 503) }
            val other = MachineActionsViewModel(refused, sessions)
            try {
                other.reconnect("lab")
                runCurrent()
                assertEquals(MachineNotice.ReconnectFailed(UiText.Plain("machine is offline")), other.notices.first())
            } finally { other.viewModelScope.cancel() }
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun notYetSaysWhyInPlainWordsAndKeepsAnErrorItCannotRead() {
        assertEquals(UiText.Res(R.string.machines_offline_auth_link),
            notYetReason(Machine("mac", kind = "link", online = false, error = "pairing: 401 unauthorized")))
        assertEquals(UiText.Res(R.string.machines_offline_timeout),
            notYetReason(Machine("lab", online = false, error = "ssh: connect to host port 22: Operation timed out")))
        assertEquals(UiText.Res(R.string.machines_offline_version_local),
            notYetReason(Machine("lab", online = false, error = "protocol version mismatch")))
        // A known error_code wins, as on the card.
        assertEquals(UiText.Res(R.string.machines_onb_code_link_offline),
            notYetReason(Machine("mac", kind = "link", online = false, error = "dial tcp: connection refused", errorCode = "link_offline")))
        assertEquals(UiText.Plain("node said something new"), notYetReason(Machine("lab", online = false, error = "node said something new")))
        assertNull(notYetReason(Machine("lab", online = false, error = " ")))
        assertNull(notYetReason(Machine("lab", online = false)))
    }

    @Test fun deployRefreshesTheListAndReportsTheHubsReason() = test {
        val sessions = Sessions(listOf(Machine("mac", online = true, nodeMismatch = true)))
        val management = Management()
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.deploy("mac")
            assertEquals(setOf("mac"), model.state.value.deployStarting)
            runCurrent()
            assertEquals(listOf("deploy:mac"), management.calls)
            assertEquals(1, sessions.refreshes)
            assertTrue(model.state.value.deployStarting.isEmpty())
            management.failure = HubFailure(HubError.Server, "这台电脑已经是 hub 这一版", 409)
            model.deploy("mac")
            runCurrent()
            assertEquals(MachineNotice.DeployFailed(UiText.Plain("这台电脑已经是 hub 这一版")), model.notices.first())
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun launchersAreReadOnlyOnlineAndFailuresSayWhy() = test {
        val management = Management()
        val model = MachineActionsViewModel(management, Sessions(emptyList()))
        try {
            model.loadLaunchers("mac", online = false)
            runCurrent()
            assertTrue(management.calls.isEmpty())
            assertNull(model.state.value.launchers)
            model.loadLaunchers("mac", online = true)
            runCurrent()
            assertEquals(listOf("claude", "plain"), model.state.value.launchers?.map { it.name })
            assertEquals(listOf("claude-opus-5", null), model.state.value.launchers?.map(::launcherModel))
            management.failure = HubFailure(HubError.Server, "mac: machine is offline", 503)
            model.loadLaunchers("mac", online = true)
            runCurrent()
            assertEquals(UiText.Plain("mac: machine is offline"), model.state.value.launchersError)
            // What was read before stays listed.
            assertEquals(2, model.state.value.launchers?.size)
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun renameSaysTheHubsNameAndRefreshesOrSaysWhyNot() = test {
        val sessions = Sessions(listOf(Machine("mac", online = true)))
        val management = Management()
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.rename("mac", "  Studio  ")
            assertTrue(model.state.value.renaming)
            model.rename("mac", "again") // one at a time
            runCurrent()
            assertEquals(listOf("rename:mac:Studio"), management.calls)
            assertEquals(MachineNotice.Renamed("Studio"), model.notices.first())
            assertEquals(1, sessions.refreshes)
            assertFalse(model.state.value.renaming)
            model.rename("mac", "")
            runCurrent()
            assertEquals(MachineNotice.Renamed("mac"), model.notices.first())
            management.failure = HubFailure(HubError.Server, "The name can be at most 40 characters, on one line", 400)
            model.rename("mac", "x".repeat(41))
            runCurrent()
            assertEquals(MachineNotice.RenameFailed(UiText.Plain("The name can be at most 40 characters, on one line")), model.notices.first())
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun botmuxSwitchSavesAtOnceAndAFailedSaveFlipsBack() = test {
        val management = Management()
        val model = MachineActionsViewModel(management, Sessions(emptyList()))
        try {
            assertNull(model.state.value.botmux)
            model.loadBotmux("mac")
            runCurrent()
            assertEquals(true, model.state.value.botmux)
            model.setBotmux("mac", false)
            // Shown at once, before the hub answers; a second flip while saving is ignored.
            assertEquals(false, model.state.value.botmux)
            assertTrue(model.state.value.botmuxSaving)
            model.setBotmux("mac", true)
            runCurrent()
            assertEquals(listOf("botmux:mac", "set-botmux:mac:false"), management.calls)
            assertEquals(false, model.state.value.botmux)
            assertFalse(model.state.value.botmuxSaving)
            management.botmuxFailure = HubFailure(HubError.Server, "database is down", 500)
            model.setBotmux("mac", true)
            runCurrent()
            assertEquals(false, model.state.value.botmux)
            assertEquals(MachineNotice.BotmuxFailed(UiText.Plain("database is down")), model.notices.first())
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun anOlderHubOrAFailedReadShowsNoBotmuxSwitch() = test {
        val older = Management().apply { botmuxOn = null }
        val model = MachineActionsViewModel(older, Sessions(emptyList()))
        val broken = object : Management() {
            override suspend fun botmux(machineId: String): Boolean? = throw HubFailure(HubError.Server, "machine is offline", 503)
        }
        val other = MachineActionsViewModel(broken, Sessions(emptyList()))
        try {
            model.loadBotmux("mac")
            other.loadBotmux("mac")
            runCurrent()
            assertNull(model.state.value.botmux)
            assertNull(other.state.value.botmux)
            // Nothing to flip: no switch, no call.
            model.setBotmux("mac", false)
            runCurrent()
            assertEquals(listOf("botmux:mac"), older.calls)
        } finally { model.viewModelScope.cancel(); other.viewModelScope.cancel() }
    }

    @Test fun deployLineDiskShareAndBootTime() {
        assertTrue(showsDeploy(Machine("a", nodeMismatch = true)))
        assertTrue(showsDeploy(Machine("a", deploying = true)))
        assertFalse(showsDeploy(Machine("a")))
        assertFalse(showsDeploy(null))
        val points = listOf(MetricPoint(1, disk = 50.0), MetricPoint(2, gap = true), MetricPoint(3))
        assertEquals(listOf(25.0, null, null), diskPercentPoints(points, 200).map { it.disk })
        assertTrue(diskPercentPoints(points, null).all { it.disk == null })
        // The boot time's words are the language's (MachinesShotsTest reads them in each language).
        assertNull(MetricFormat.boot(0.0, java.util.Locale.ENGLISH))
        assertTrue(MetricKind.Disk.percent && MetricKind.Cpu.percent && !MetricKind.Memory.percent)
    }

    @Test fun linkSwitchEligibilityComesOnlyFromTheHub() {
        assertFalse(showsLinkSwitch(null, null))
        assertFalse(canStartLinkSwitch(null, null, 0))
        assertTrue(canStartLinkSwitch(MachineLinkSwitch(available = true), null, 0))
        assertTrue(canStartLinkSwitch(MachineLinkSwitch(available = true, phase = "failed"), null, 0))
        assertTrue(canStartLinkSwitch(MachineLinkSwitch(available = true, phase = "succeeded"), null, 0))
        for (phase in listOf("checking", "installing", "connecting", "rolling_back", "future_phase")) {
            assertFalse(phase, canStartLinkSwitch(MachineLinkSwitch(available = true, phase = phase), null, 0))
        }
        assertFalse(canStartLinkSwitch(MachineLinkSwitch(available = false, reason = "unsupported_os"), null, 0))
        // A quiet ineligibility reason is not a new card; an operation or useful reason is.
        assertFalse(showsLinkSwitch(MachineLinkSwitch(available = false, reason = "offline"), null))
        assertFalse(showsLinkSwitch(MachineLinkSwitch(available = false, reason = "busy"), null))
        assertTrue(showsLinkSwitch(MachineLinkSwitch(available = false, reason = "offline", phase = "failed"), null))
        assertTrue(showsLinkSwitch(MachineLinkSwitch(available = false, reason = "unsupported_os"), null))
    }

    @Test fun linkSwitchConfirmIsImmediateSingleFlightAndUsesTheAcceptedState() = test {
        val eligible = MachineLinkSwitch(available = true, at = "2026-11-20T09:59:59Z")
        val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible)))
        val gate = CompletableDeferred<Unit>()
        val management = Management().apply { linkGate = gate }
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.observeLinkSwitch("lab")
            runCurrent()
            assertTrue(management.calls.isEmpty()) // observation and opening the page never start it
            model.startLinkSwitch("lab")
            // Local Starting is stored before the coroutine can make the request.
            assertTrue(model.state.value.linkSwitches.getValue("lab").starting)
            model.startLinkSwitch("lab")
            runCurrent()
            assertEquals(listOf("link-switch:lab"), management.calls)
            gate.complete(Unit)
            runCurrent()
            assertEquals(listOf("link-switch:lab"), management.calls)
            assertEquals(0, sessions.refreshes) // the 202 is enough; do not manufacture another read
            val accepted = management.linkAnswer
            assertEquals(accepted, shownLinkSwitch(eligible, model.state.value.linkSwitches["lab"], 0))
            assertFalse(canStartLinkSwitch(eligible, model.state.value.linkSwitches["lab"], 0))
            model.startLinkSwitch("lab")
            runCurrent()
            assertEquals(1, management.calls.size)
            // A newer authoritative snapshot takes over and retires the local accepted overlay.
            val done = accepted.copy(phase = "succeeded", at = "2026-11-20T10:00:01Z")
            sessions.machines.value = listOf(Machine("lab", linkSwitch = done))
            runCurrent()
            assertNull(model.state.value.linkSwitches["lab"])
            assertEquals(done, shownLinkSwitch(done, model.state.value.linkSwitches["lab"], 0))
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun linkSwitchProgressRollbackAndOutcomeStayAuthoritative() {
        for (phase in listOf("checking", "installing", "connecting", "rolling_back")) {
            assertTrue(phase, linkSwitchRunning(MachineLinkSwitch(available = false, phase = phase)))
        }
        for (phase in listOf("failed", "succeeded", "future_phase", null)) {
            assertFalse(phase ?: "null", linkSwitchRunning(MachineLinkSwitch(available = false, phase = phase)))
        }
        val failed = MachineLinkSwitch(available = true, phase = "failed", error = "install_failed")
        assertEquals(R.string.machines_link_switch_why_install_failed, linkSwitchReasonText(failed.error))
        assertFalse(failed.rollbackUnconfirmed)
        assertTrue(failed.copy(rollbackUnconfirmed = true).rollbackUnconfirmed)
        assertEquals(R.string.machines_link_switch_why_unavailable, linkSwitchReasonText("brand_new_code"))
        assertEquals("owner", machineLogin("/home/owner/"))
        assertNull(machineLogin("/"))
        val eligibleForBusy = MachineLinkSwitch(available = true)
        assertTrue(linkSwitchConnectionBusy(eligibleForBusy, LinkSwitchAttempt(linkSwitchKey(eligibleForBusy)), 0))
        val unresolved = LinkSwitchAttempt(linkSwitchKey(eligibleForBusy), starting = false,
            notice = LinkSwitchNotice.Uncertain, refreshFailed = true)
        assertTrue(linkSwitchConnectionBusy(eligibleForBusy, unresolved, 0))
        assertFalse(linkSwitchConnectionBusy(eligibleForBusy, unresolved.copy(refreshFailed = false), 0))
        val waiting = LinkSwitchAttempt(linkSwitchKey(eligibleForBusy))
        assertTrue(linkSwitchWaiting(eligibleForBusy, waiting))
        assertFalse(linkSwitchWaiting(eligibleForBusy.copy(phase = "checking"), waiting))
        assertFalse(linkSwitchWaiting(eligibleForBusy, waiting.copy(starting = false)))
    }

    @Test fun linkSwitchTimestampsAndCompleteStateDecideWhichSnapshotWins() {
        val before = MachineLinkSwitch(available = true, at = "2026-11-20T09:59:59Z")
        val accepted = MachineLinkSwitch(available = false, phase = "checking", at = "2026-11-20T10:00:00Z")
        val attempt = LinkSwitchAttempt(linkSwitchKey(before), starting = false, accepted = accepted, settledAt = 0)
        assertEquals(accepted, shownLinkSwitch(before, attempt, 1_000))
        assertEquals(accepted, shownLinkSwitch(before.copy(at = "2026-11-20T09:59:59.500Z"), attempt, 1_000))
        val sameTime = accepted.copy(phase = "installing")
        assertEquals(sameTime, shownLinkSwitch(sameTime, attempt, 1_000))
        val later = accepted.copy(phase = "succeeded", at = "2026-11-20T10:00:00.001Z")
        assertEquals(later, shownLinkSwitch(later, attempt, 1_000))
        // An accepted operation's timestamp cannot be caught up by an eligibility-only snapshot with no operation time.
        val delayedEligibility = MachineLinkSwitch(available = false, reason = "offline")
        assertEquals(accepted, shownLinkSwitch(delayedEligibility, attempt, 1_000))
        // Without two usable timestamps, only a changed complete state is newer than the pre-start list.
        val timelessBefore = before.copy(at = null)
        val timelessAccepted = accepted.copy(at = null)
        val timeless = LinkSwitchAttempt(linkSwitchKey(timelessBefore), starting = false,
            accepted = timelessAccepted, settledAt = 0)
        assertEquals(timelessAccepted, shownLinkSwitch(timelessBefore, timeless, 1_000))
        val malformed = timelessBefore.copy(at = "not-a-time")
        assertEquals(malformed, shownLinkSwitch(malformed, timeless, 1_000))
        assertEquals(later.copy(at = null), shownLinkSwitch(later.copy(at = null), timeless, 1_000))
    }

    @Test fun activeAcceptedLinkSwitchNeverExpiresBackIntoAStart() {
        val eligible = MachineLinkSwitch(available = true)
        val active = MachineLinkSwitch(available = false, phase = "connecting")
        val attempt = LinkSwitchAttempt(linkSwitchKey(eligible), starting = false, accepted = active, settledAt = 0)
        val beyondOrdinaryNoticeLinger = LINK_SWITCH_ATTEMPT_TTL_MILLIS * 10
        assertTrue(linkSwitchAttemptLive(attempt, beyondOrdinaryNoticeLinger))
        assertEquals(active, shownLinkSwitch(eligible, attempt, beyondOrdinaryNoticeLinger))
        assertFalse(canStartLinkSwitch(eligible, attempt, beyondOrdinaryNoticeLinger))
        val settled = attempt.copy(accepted = active.copy(phase = "succeeded"))
        assertFalse(linkSwitchAttemptLive(settled, beyondOrdinaryNoticeLinger))
        assertEquals(eligible, shownLinkSwitch(eligible, settled, beyondOrdinaryNoticeLinger))
    }

    @Test fun staleAcceptedLinkSwitchNeedsAReadAndNeverReposts() = test {
        for (outcome in listOf<MachineLinkSwitch?>(null, MachineLinkSwitch(available = true))) {
            var now = 0L
            val eligible = MachineLinkSwitch(available = true)
            val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible)))
            val management = Management()
            val model = MachineActionsViewModel(management, sessions, linkSwitchClock = { now })
            try {
                model.startLinkSwitch("lab")
                runCurrent()
                val accepted = model.state.value.linkSwitches.getValue("lab")
                assertTrue(linkSwitchRunning(accepted.accepted))
                now = LINK_SWITCH_STALE_AFTER_MILLIS + 1
                assertTrue(linkSwitchNeedsStatus(accepted, now))
                assertEquals(LinkSwitchNotice.Stale, linkSwitchNotice(eligible, accepted, now))
                assertEquals(eligible, shownLinkSwitch(eligible, accepted, now))
                assertFalse(canStartLinkSwitch(eligible, accepted, now))
                assertTrue(linkSwitchConnectionBusy(eligible, accepted, now))

                // A missing/absent terminal record cannot clear the attempt by itself, and a failed read leaves Check status.
                sessions.machines.value = listOf(Machine("lab", linkSwitch = outcome))
                sessions.refreshFailure = java.io.IOException("still unreachable")
                model.checkLinkSwitch("lab")
                runCurrent()
                assertEquals(1, sessions.refreshes)
                assertEquals(1, management.calls.size)
                val failed = model.state.value.linkSwitches.getValue("lab")
                assertTrue(failed.refreshFailed)
                assertEquals(accepted.settledAt, failed.settledAt)
                assertEquals(LinkSwitchNotice.Stale, linkSwitchNotice(outcome, failed, now))
                assertFalse(canStartLinkSwitch(outcome, failed, now))
                assertTrue(linkSwitchConnectionBusy(outcome, failed, now))

                // Only the successful owning-hub read retires the overlay; its own current state is authoritative.
                sessions.refreshFailure = null
                model.checkLinkSwitch("lab")
                runCurrent()
                assertEquals(2, sessions.refreshes)
                assertEquals(listOf("lab", "lab"), sessions.refreshedMachines)
                assertEquals(1, management.calls.size)
                assertNull(model.state.value.linkSwitches["lab"])
                assertEquals(outcome, shownLinkSwitch(outcome, null, now))
            } finally { model.viewModelScope.cancel() }
        }
    }

    @Test fun forbiddenLinkSwitchDoesNotReadBackOrStartAgain() = test {
        val eligible = MachineLinkSwitch(available = true)
        val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible)))
        val management = Management().apply {
            linkFailure = HubFailure(HubError.Server, "read only", 403, "forbidden")
        }
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.startLinkSwitch("lab")
            runCurrent()
            assertEquals(listOf("link-switch:lab"), management.calls)
            assertEquals(0, sessions.refreshes)
            val attempt = model.state.value.linkSwitches.getValue("lab")
            val now = attempt.settledAt!!
            assertEquals(LinkSwitchNotice.Forbidden, linkSwitchNotice(eligible, attempt, now))
            assertFalse(canStartLinkSwitch(eligible, attempt, now))
            model.startLinkSwitch("lab")
            runCurrent()
            assertEquals(1, management.calls.size)
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun definiteLinkSwitchRefusalsReadBackOnceWithoutRetrying() = test {
        for (failure in listOf(
            HubFailure(HubError.Server, "already switching", 409, "in_progress"),
            HubFailure(HubError.Server, "computer is updating", 400, "busy"),
        )) {
            val eligible = MachineLinkSwitch(available = true)
            val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible)))
            val management = Management().apply { linkFailure = failure }
            val model = MachineActionsViewModel(management, sessions)
            try {
                model.startLinkSwitch("lab")
                runCurrent()
                val attempt = model.state.value.linkSwitches.getValue("lab")
                val now = attempt.settledAt!!
                assertEquals(LinkSwitchNotice.Refused(failure.code), linkSwitchNotice(eligible, attempt, now))
                assertFalse(attempt.refreshFailed)
                advanceTimeBy(LINK_SWITCH_ATTEMPT_TTL_MILLIS + 1)
                runCurrent()
                assertEquals(listOf("link-switch:lab"), management.calls)
                assertEquals(1, sessions.refreshes)
                assertEquals(listOf("lab"), sessions.refreshedMachines)
            } finally { model.viewModelScope.cancel() }
        }
    }

    @Test fun uncertainLinkSwitchAnswersReadBackAndNeverRepostAutomatically() = test {
        val failures = listOf(
            IOException("connection dropped"),
            HubFailure(HubError.Server, "bad gateway", 502),
            HubFailure(HubError.Timeout, "timed out", 408),
            HubFailure(HubError.RateLimited, "slow down", 429),
            HubFailure(HubError.Protocol, "accepted without link_switch", retryable = false),
        )
        for (failure in failures) {
            val eligible = MachineLinkSwitch(available = true)
            val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible)))
            val management = Management().apply { linkFailure = failure }
            val model = MachineActionsViewModel(management, sessions)
            try {
                model.startLinkSwitch("lab")
                runCurrent()
                val attempt = model.state.value.linkSwitches.getValue("lab")
                val now = attempt.settledAt!!
                assertEquals(failure.message, LinkSwitchNotice.Uncertain, linkSwitchNotice(eligible, attempt, now))
                assertFalse(attempt.refreshFailed)
                // A successful read allows a new, separately confirmed start; it does not start one itself.
                assertTrue(canStartLinkSwitch(eligible, attempt, now))
                advanceTimeBy(LINK_SWITCH_ATTEMPT_TTL_MILLIS + 1)
                runCurrent()
                assertEquals(failure.message, listOf("link-switch:lab"), management.calls)
                assertEquals(failure.message, 1, sessions.refreshes)
                assertEquals(failure.message, listOf("lab"), sessions.refreshedMachines)
            } finally { model.viewModelScope.cancel() }
        }
    }

    @Test fun failedUncertainReadLeavesOnlyCheckStatusUntilAReadSucceeds() = test {
        val eligible = MachineLinkSwitch(available = true)
        val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible))).apply {
            refreshFailure = IOException("list unavailable")
        }
        val management = Management().apply { linkFailure = IOException("answer lost") }
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.startLinkSwitch("lab")
            runCurrent()
            assertEquals(1, management.calls.size)
            assertEquals(1, sessions.refreshes)
            val uncertain = model.state.value.linkSwitches.getValue("lab")
            assertTrue(uncertain.refreshFailed)
            assertFalse(canStartLinkSwitch(eligible, uncertain, 0))
            model.startLinkSwitch("lab")
            model.checkLinkSwitch("lab")
            model.checkLinkSwitch("lab") // one read in flight, and still no POST
            runCurrent()
            assertEquals(1, management.calls.size)
            assertEquals(2, sessions.refreshes)
            assertTrue(model.state.value.linkSwitches.getValue("lab").refreshFailed)
            sessions.refreshFailure = null
            model.checkLinkSwitch("lab")
            assertTrue(model.state.value.linkSwitches.getValue("lab").starting)
            runCurrent()
            assertEquals(1, management.calls.size)
            assertEquals(3, sessions.refreshes)
            val checked = model.state.value.linkSwitches.getValue("lab")
            assertFalse(checked.refreshFailed)
            assertTrue(canStartLinkSwitch(eligible, checked, 0))
            // Only this second explicit confirmation can make another request.
            management.linkFailure = null
            model.startLinkSwitch("lab")
            runCurrent()
            assertEquals(2, management.calls.size)
            assertEquals(3, sessions.refreshes)
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun failedUncertainReadCannotBeReconciledByAnUnprovenListChange() = test {
        val eligible = MachineLinkSwitch(available = true)
        val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible))).apply {
            refreshFailure = IOException("list unavailable")
        }
        val management = Management().apply { linkFailure = IOException("answer lost") }
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.startLinkSwitch("lab")
            runCurrent()
            assertTrue(model.state.value.linkSwitches.getValue("lab").refreshFailed)
            // A push/cache change is not proof that the failed owning-hub read succeeded.
            sessions.machines.value = listOf(Machine("lab"))
            runCurrent()
            val unresolved = model.state.value.linkSwitches.getValue("lab")
            assertTrue(unresolved.refreshFailed)
            assertEquals(LinkSwitchNotice.Uncertain, linkSwitchNotice(null, unresolved, 0))
            assertFalse(canStartLinkSwitch(null, unresolved, 0))
            assertTrue(linkSwitchConnectionBusy(null, unresolved, 0))
            sessions.refreshFailure = null
            model.checkLinkSwitch("lab")
            runCurrent()
            assertEquals(1, management.calls.size)
            assertEquals(2, sessions.refreshes)
            assertNull(model.state.value.linkSwitches["lab"])
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun cancelledLinkSwitchCallsBecomeRecoverableUncertaintyNotAStuckStart() = test {
        val eligible = MachineLinkSwitch(available = true)
        val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible)))
        val post = CompletableDeferred<Unit>()
        val management = Management().apply { linkGate = post }
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.startLinkSwitch("lab")
            runCurrent()
            assertTrue(model.state.value.linkSwitches.getValue("lab").starting)
            // Cancelling the request does not destroy this ViewModel; another POST would not be safe.
            post.cancel()
            runCurrent()
            val cancelled = model.state.value.linkSwitches.getValue("lab")
            assertFalse(cancelled.starting)
            assertEquals(LinkSwitchNotice.Uncertain, cancelled.notice)
            assertTrue(cancelled.refreshFailed)
            assertFalse(canStartLinkSwitch(eligible, cancelled, 0))
            assertTrue(linkSwitchConnectionBusy(eligible, cancelled, 0))
            assertEquals(listOf("link-switch:lab"), management.calls)

            // The explicit read-only recovery can itself be cancelled and still offers Check status again.
            val read = CompletableDeferred<Unit>()
            sessions.refreshGate = read
            model.checkLinkSwitch("lab")
            runCurrent()
            assertTrue(model.state.value.linkSwitches.getValue("lab").starting)
            read.cancel()
            runCurrent()
            val checked = model.state.value.linkSwitches.getValue("lab")
            assertFalse(checked.starting)
            assertTrue(checked.refreshFailed)
            assertFalse(canStartLinkSwitch(eligible, checked, 0))
            assertEquals(listOf("link-switch:lab"), management.calls)
        } finally { model.viewModelScope.cancel() }
    }

    @Test fun successfulReadbackUsesTheChangedMachineState() = test {
        val eligible = MachineLinkSwitch(available = true)
        val running = MachineLinkSwitch(available = false, phase = "installing", at = "2026-11-20T10:00:00Z")
        val sessions = Sessions(listOf(Machine("lab", linkSwitch = eligible))).apply {
            onRefresh = { sessionsSet(this, running) }
        }
        val management = Management().apply { linkFailure = IOException("answer lost") }
        val model = MachineActionsViewModel(management, sessions)
        try {
            model.startLinkSwitch("lab")
            runCurrent()
            assertEquals(1, management.calls.size)
            assertEquals(1, sessions.refreshes)
            assertEquals(listOf("lab"), sessions.refreshedMachines)
            // The changed list is authoritative, so the warning is gone and no new start is possible.
            assertNull(model.state.value.linkSwitches["lab"])
            assertFalse(canStartLinkSwitch(running, model.state.value.linkSwitches["lab"], 0))
        } finally { model.viewModelScope.cancel() }
    }

    private fun sessionsSet(sessions: Sessions, state: MachineLinkSwitch) {
        sessions.machines.value = listOf(Machine("lab", linkSwitch = state))
    }
}
