package persona.feature.machines

import android.graphics.Bitmap
import androidx.activity.ComponentActivity
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.navigation3.runtime.NavKey
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonElement
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import persona.core.contracts.*
import persona.core.contracts.api.*
import persona.core.designsystem.*
import persona.core.navigation.AppTab
import persona.core.navigation.Navigator
import java.io.File
import java.time.ZoneOffset
import java.util.Locale

/**
 * Machine monitor and a machine's page in each of the app's languages, Glass and Brief, light and dark (docs/protocol/i18n.md). Written
 * as PNGs to `build/i18n-shots/` (or `-Dpersona.shots=<dir>`) to look at.
 */
@RunWith(AndroidJUnit4::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MachinesShotsTest {
    @get:Rule val rule = createAndroidComposeRule<ComponentActivity>()

    private fun out(): File = File(System.getProperty("persona.shots") ?: "build/i18n-shots").also { it.mkdirs() }

    @Config(sdk = [35], qualifiers = "zh-w400dp-h5200dp-xhdpi")
    @Test fun simplifiedChinese() {
        assertEquals("2026年11月2日 08:30", MetricFormat.boot(1_793_579_400.0, Locale.SIMPLIFIED_CHINESE, ZoneOffset.ofHours(8)))
        shots("zh", listTitle = "机器监控", detail = "引擎与连接", botmux = "同时显示 botmux 会话",
            offline = "连不上这台电脑。确认它开着、网络通、node 在运行，然后重新连接")
    }

    @Config(sdk = [35], qualifiers = "en-w400dp-h5200dp-xhdpi")
    @Test fun english() {
        assertTrue(MetricFormat.boot(1_793_579_400.0, Locale.ENGLISH, ZoneOffset.ofHours(8))!!.startsWith("Nov 2, 2026"))
        shots("en", listTitle = "Machines", detail = "Engine and connection", botmux = "Also show botmux sessions",
            offline = "Can't reach this computer. Make sure it's on, the network is up and the node is running, then reconnect")
    }

    @Config(sdk = [35], qualifiers = "ja-w400dp-h5200dp-xhdpi")
    @Test fun japanese() = shots("ja", listTitle = "マシン", detail = "エンジンと接続", botmux = "botmux のセッションも表示",
        offline = "このコンピューターに接続できません。電源が入っていて、ネットワークがつながり、ノードが動作していることを確認してから、再接続してください")

    private val now = 1_793_600_000L
    private val lab = Machine("lab", name = "Lab", kind = "ssh", sshHost = "lab.example.com", online = true, hostname = "lab.local",
        herdrVersion = "0.9.1", claudeVersion = "2.1.0", nodeVersion = "abc1234", nodeMismatch = true,
        system = MachineSystem(os = "linux", name = "Ubuntu 24.04", version = "24.04", kernel = "6.8.0", arch = "x86_64",
            model = "Workstation", cpu = "AMD Ryzen 9", cores = 4, memory = 64L shl 30, bootTime = (now - 3 * 86_400 - 5 * 3600).toDouble()))
    private val mac = Machine("mac", name = "MacBook", kind = "local", online = false, lastSeen = "2026-11-02T08:30:00Z",
        error = "ssh: connection refused", system = MachineSystem(os = "darwin", distro = "macos", name = "macOS 26", cores = 10, memory = 32L shl 30))
    private val sample = MetricSample(now - 5, cpu = 93.5, cores = listOf(91.0, 88.5, 97.0, 12.0), load = 6.2, memUsed = 54L shl 30,
        memTotal = 64L shl 30, swapUsed = 0, swapTotal = 8L shl 30, netRx = 1.5 * 1024 * 1024, netTx = 240.0 * 1024,
        diskUsed = 410L shl 30, diskTotal = 500L shl 30)
    private val points = (0 until 360).map { i ->
        val t = now - 3600 + i * 10
        MetricPoint(t, cpu = 40.0 + 30 * kotlin.math.sin(i / 20.0), load = 3.0 + kotlin.math.cos(i / 30.0), mem = (50L shl 30).toDouble(),
            rx = 1024.0 * (300 + i), tx = 1024.0 * 100, disk = (400L shl 30).toDouble())
    }
    private val labMetrics = MachineMetrics("lab", true, lab.system, sample, points)
    private val macMetrics = MachineMetrics("mac", false, mac.system)

    private val api = object : MachineMetricsApi {
        override suspend fun dashboard() = MetricsDashboard(listOf(labMetrics, macMetrics), step = 10)
        override suspend fun history(machineId: String, range: MetricRange) = MetricsHistory(labMetrics, range.wire, range.step)
    }
    private val repository = object : SessionsRepository {
        override val machines = MutableStateFlow(listOf(lab, mac))
        override val sessions = MutableStateFlow(listOf(
            Session("lab", "s1", title = "Fix the login flow", cwd = "/home/me/app", status = "working"),
            Session("lab", "s2", title = "Write release notes", cwd = "/home/me/docs", status = "blocked"),
        ))
        override suspend fun refresh() = Unit
    }
    private val management = object : MachineManagement {
        override suspend fun launchers(machineId: String) = listOf(AgentLauncher("claude", model = "anthropic/claude-opus-5"), AgentLauncher("codex"))
        override suspend fun launcherQuota(machineId: String, name: String, refresh: Boolean) = LauncherQuotaAnswer()
        override suspend fun reconnect(machineId: String) = Unit
        override suspend fun deploy(machineId: String) = Unit
        override suspend fun herdr(machineId: String): HerdrState = error("unused")
        override suspend fun startHerdr(machineId: String): HerdrState = error("unused")
        override suspend fun setEngine(machineId: String, engine: String) = Machine(machineId, engine = engine)
        override suspend fun claudeSettings(machineId: String) = emptyList<ClaudeSetting>()
        override suspend fun setClaudeSetting(machineId: String, key: String, value: JsonElement) = emptyList<ClaudeSetting>()
        override suspend fun codexSettings(machineId: String) = emptyList<ClaudeSetting>()
        override suspend fun setCodexSetting(machineId: String, key: String, value: JsonElement) = emptyList<ClaudeSetting>()
        override suspend fun botmux(machineId: String): Boolean? = true
    }
    private val navigator = object : Navigator {
        override fun navigate(key: NavKey) = Unit
        override fun back() = Unit
        override fun selectTab(tab: AppTab) = Unit
    }

    /** [offline]: what the offline MacBook's card says first instead of the hub's raw error (#4463). */
    private fun shots(language: String, listTitle: String, detail: String, botmux: String, offline: String) {
        val list = MachinesViewModel(api, repository, null) { now }
        val page = MachinesViewModel(api, repository, "lab") { now }
        runBlocking { list.load(MetricRange.Hour); page.load(MetricRange.Hour) }
        val actions = MachineActionsViewModel(management, repository)
        actions.loadBotmux("lab")
        var style by mutableStateOf(AppStyle.Glass)
        var dark by mutableStateOf(false)
        var detailPage by mutableStateOf(false)
        rule.setContent {
            PersonaTheme(darkTheme = dark, reduceMotion = true, style = style) {
                Box(Modifier.testTag("shot").width(400.dp).height(if (detailPage) 5200.dp else 1000.dp).background(PersonaTheme.palette.background)) {
                    if (detailPage) MachinesScreen("lab", page, navigator, actions) else MachinesScreen(null, list, navigator)
                }
            }
        }
        for (d in listOf(true, false)) for (s in listOf(AppStyle.Glass, AppStyle.entries.last())) for (dk in listOf(false, true)) {
            if (s != AppStyle.Glass && dk) continue
            detailPage = d; style = s; dark = dk
            rule.waitForIdle()
            val bitmap = rule.onNodeWithTag("shot").captureToImage().asAndroidBitmap()
            val name = "machines-${if (d) "detail" else "list"}-$language-${s.name.lowercase()}-${if (dk) "dark" else "light"}.png"
            File(out(), name).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
            rule.onNodeWithText(if (d) detail else listTitle).assertExists()
            if (d) rule.onNodeWithText(botmux).assertExists()
            else {
                rule.onNodeWithText(offline).assertExists()
                rule.onNodeWithText(mac.error!!).assertExists() // the hub's own words stay, after the plain ones
            }
        }
    }

    @Config(sdk = [35], qualifiers = "zh-w400dp-h5200dp-xhdpi", fontScale = 2f)
    @Test fun simplifiedChineseAgentsAtMaximumText() = agentShowAll("zh", "显示全部 6 个", "收起")

    @Config(sdk = [35], qualifiers = "en-w400dp-h5200dp-xhdpi", fontScale = 2f)
    @Test fun englishAgentsAtMaximumText() = agentShowAll("en", "Show all 6", "Show less")

    @Config(sdk = [35], qualifiers = "ja-w400dp-h5200dp-xhdpi", fontScale = 2f)
    @Test fun japaneseAgentsAtMaximumText() = agentShowAll("ja", "すべて表示（6件）", "折りたたむ")

    @Config(sdk = [35], qualifiers = "en-w400dp-h2400dp-xhdpi")
    @Test fun linkSwitchDetail() = linkSwitchDetail(1f)

    @Config(sdk = [35], qualifiers = "en-w400dp-h2400dp-xhdpi", fontScale = 2f)
    @Test fun linkSwitchDetailAtLargestText() = linkSwitchDetail(2f)

    /** The real detail screen: its expected disconnect is not a new SSH error and offers no ordinary Reconnect. */
    private fun linkSwitchDetail(fontScale: Float) {
        val switching = lab.copy(online = false, error = "ssh: connection refused",
            linkSwitch = MachineLinkSwitch(available = false, phase = "installing", at = "2026-11-20T10:00:00Z"))
        val metrics = MachineMetrics("lab", false, lab.system, sample, points)
        val switchApi = object : MachineMetricsApi {
            override suspend fun dashboard() = MetricsDashboard(listOf(metrics), step = 10)
            override suspend fun history(machineId: String, range: MetricRange) = MetricsHistory(metrics, range.wire, range.step)
        }
        val switchRepository = object : SessionsRepository {
            override val machines = MutableStateFlow(listOf(switching))
            override val sessions = MutableStateFlow(emptyList<Session>())
            override suspend fun refresh() = Unit
        }
        val page = MachinesViewModel(switchApi, switchRepository, "lab") { now }
        runBlocking { page.load(MetricRange.Hour) }
        val actions = MachineActionsViewModel(management, switchRepository)
        var dark by mutableStateOf(false)
        rule.setContent {
            CompositionLocalProvider(LocalDensity provides Density(LocalDensity.current.density, fontScale)) {
                PersonaTheme(darkTheme = dark, reduceMotion = true, style = AppStyle.Glass) {
                    Box(Modifier.testTag("link-switch-detail-shot").width(400.dp)
                        .height(if (fontScale > 1f) 1800.dp else 1200.dp).background(PersonaTheme.palette.background)) {
                        MachinesScreen("lab", page, navigator, actions)
                    }
                }
            }
        }
        for (d in listOf(false, true)) {
            rule.runOnIdle { dark = d }
            rule.waitForIdle()
            rule.onNodeWithText(rule.activity.getString(R.string.machines_link_switch_switching))
                .performScrollTo().assertIsDisplayed()
            rule.onNodeWithText("ssh: connection refused").assertDoesNotExist()
            rule.onNodeWithText(rule.activity.getString(R.string.machines_reconnect)).assertDoesNotExist()
            val bitmap = rule.onNodeWithTag("link-switch-detail-shot").captureToImage().asAndroidBitmap()
            val size = if (fontScale > 1f) "large" else "default"
            File(out(), "machines-detail-link-switch-glass-${if (d) "dark" else "light"}-$size.png")
                .outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
        }
    }

    private fun agentShowAll(language: String, showAll: String, showLess: String) {
        val repository = object : SessionsRepository {
            override val machines = MutableStateFlow(listOf(lab))
            override val sessions = MutableStateFlow((1..6).map { index ->
                Session("lab", "agent-$index", title = "Agent $index", cwd = "/work/example", status = "working")
            })
            override suspend fun refresh() = Unit
        }
        val model = MachinesViewModel(api, repository, "lab") { now }
        runBlocking { model.load(MetricRange.Hour) }
        var style by mutableStateOf(AppStyle.Glass)
        var dark by mutableStateOf(false)
        rule.setContent {
            PersonaTheme(darkTheme = dark, reduceMotion = true, style = style) {
                Box(Modifier.testTag("agents-shot").width(400.dp).height(5200.dp).background(PersonaTheme.palette.background)) {
                    MachinesScreen("lab", model, navigator)
                }
            }
        }
        for (s in AppStyle.entries) for (dk in listOf(false, true)) {
            rule.runOnIdle { style = s; dark = dk }
            rule.waitForIdle()
            rule.onNodeWithText(showAll).performScrollTo().assertIsDisplayed()
            var bitmap = rule.onNodeWithTag("agents-shot").captureToImage().asAndroidBitmap()
            File(out(), "machines-agents-$language-${s.name.lowercase()}-${if (dk) "dark" else "light"}-collapsed.png")
                .outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
            rule.onNodeWithText(showAll).performClick()
            rule.onNodeWithText(showLess).performScrollTo().assertIsDisplayed()
            bitmap = rule.onNodeWithTag("agents-shot").captureToImage().asAndroidBitmap()
            File(out(), "machines-agents-$language-${s.name.lowercase()}-${if (dk) "dark" else "light"}-expanded.png")
                .outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
            rule.onNodeWithText(showLess).performClick()
        }
    }
}
