package persona.feature.machines

import android.content.res.Configuration
import android.graphics.Bitmap
import androidx.activity.ComponentActivity
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.isDialog
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import persona.core.contracts.Machine
import persona.core.contracts.MachineLinkSwitch
import persona.core.designsystem.AppStyle
import persona.core.designsystem.PersonaTheme
import persona.core.designsystem.Tokens
import java.io.File

/**
 * The advanced computer detail's SSH-to-Link action, confirmation and authoritative outcomes. Robolectric's native
 * renderer needs no emulator. English writes the complete style/theme/text-size matrix as PNGs outside the repository
 * when `-Dpersona.shots=<dir>` is set; all three languages exercise the same semantics and confirmation rules.
 */
@RunWith(AndroidJUnit4::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MachineLinkSwitchShotsTest {
    @get:Rule val rule = createAndroidComposeRule<ComponentActivity>()

    private data class Words(
        val language: String,
        val intro: String,
        val action: String,
        val confirm: String,
        val cancel: String,
        val starting: String,
        val success: String,
        val kept: String,
        val checkStatus: String,
    )

    private val english = Words("en",
        "Connected over SSH. With Persona Link, this computer connects to the hub by itself, also from other networks.",
        "Switch to Persona Link…", "Switch", "Cancel", "Starting…",
        "Switched to Persona Link. This computer now connects to the hub by itself.",
        "Its name, settings and history stay. If something fails, it goes back to SSH.", "Check status")
    private val chinese = Words("zh",
        "当前通过 SSH 连接。改用 Persona Link 后，这台电脑会自己连上 hub，换了网络也能连。",
        "切换到 Persona Link…", "切换", "取消", "正在开始…",
        "已切换到 Persona Link。这台电脑现在会自己连上 hub。",
        "名称、设置和历史记录都会保留。如果哪一步失败，会回到 SSH 连接。", "检查状态")
    private val japanese = Words("ja",
        "SSHで接続しています。Persona Linkにすると、このコンピューターはほかのネットワークからでも自分でハブに接続します。",
        "Persona Linkに切り替え…", "切り替え", "キャンセル", "開始しています…",
        "Persona Linkに切り替えました。このコンピューターは自分でハブに接続するようになりました。",
        "名前、設定、履歴はそのまま残ります。途中で失敗した場合はSSH接続に戻ります。", "状態を確認")

    private data class Scene(
        val name: String,
        val listed: MachineLinkSwitch?,
        val attempt: LinkSwitchAttempt? = null,
        val expected: Int,
        val expectedArg: Int? = null,
        val expectedLiteralArg: String? = null,
    )

    private val eligible = MachineLinkSwitch(available = true)
    private var scene by mutableStateOf<Scene?>(null)
    private var style by mutableStateOf(AppStyle.Glass)
    private var dark by mutableStateOf(false)
    private var starts = 0
    private var checks = 0

    @Config(sdk = [35], qualifiers = "en-w400dp-h2400dp-xhdpi")
    @Test fun english() = verify(english, 1f)

    @Config(sdk = [35], qualifiers = "en-w400dp-h2400dp-xhdpi", fontScale = 2f)
    @Test fun englishAtLargestText() = verify(english, 2f)

    @Config(sdk = [35], qualifiers = "zh-w400dp-h2400dp-xhdpi")
    @Test fun simplifiedChinese() = verify(chinese, 1f)

    @Config(sdk = [35], qualifiers = "zh-w400dp-h2400dp-xhdpi", fontScale = 2f)
    @Test fun simplifiedChineseAtLargestText() = verify(chinese, 2f)

    @Config(sdk = [35], qualifiers = "ja-w400dp-h2400dp-xhdpi")
    @Test fun japanese() = verify(japanese, 1f)

    @Config(sdk = [35], qualifiers = "ja-w400dp-h2400dp-xhdpi", fontScale = 2f)
    @Test fun japaneseAtLargestText() = verify(japanese, 2f)

    private fun out(): File = File(System.getProperty("persona.shots") ?: "build/link-switch-shots").also { it.mkdirs() }
    private fun text(id: Int, vararg args: Any): String = rule.activity.getString(id, *args)

    private fun mount(fontScale: Float) {
        // Both the real Android configuration and Compose's density must change, including the dialog's own window.
        RuntimeEnvironment.setFontScale(fontScale)
        rule.setContent {
            val density = LocalDensity.current
            val configuration = Configuration(LocalConfiguration.current).apply {
                uiMode = (uiMode and Configuration.UI_MODE_NIGHT_MASK.inv()) or
                    if (dark) Configuration.UI_MODE_NIGHT_YES else Configuration.UI_MODE_NIGHT_NO
            }
            CompositionLocalProvider(
                LocalDensity provides Density(density.density, fontScale),
                LocalConfiguration provides configuration,
            ) {
                PersonaTheme(darkTheme = dark, reduceMotion = true, style = style) {
                    Box(
                        Modifier.testTag("link-switch-shot").width(400.dp)
                            .background(PersonaTheme.palette.background).padding(Tokens.Space.page),
                    ) {
                        scene?.let { current ->
                            MachineLinkSwitchSection(
                                Machine("lab", name = "Lab", kind = "ssh", online = true, home = "/home/owner",
                                    linkSwitch = current.listed),
                                current.attempt,
                                onStart = { starts++ },
                                onCheck = { checks++ },
                                now = 1_000,
                            )
                        }
                    }
                }
            }
        }
        rule.waitForIdle()
    }

    private fun show(next: Scene?, nextStyle: AppStyle = style, nextDark: Boolean = dark) {
        rule.runOnIdle { scene = next; style = nextStyle; dark = nextDark }
        rule.waitForIdle()
    }

    private fun assertScene(current: Scene) {
        val expected = when {
            current.expectedLiteralArg != null -> text(current.expected, current.expectedLiteralArg)
            current.expectedArg != null -> text(current.expected, text(current.expectedArg))
            else -> text(current.expected)
        }
        rule.onNodeWithText(expected).assertExists()
    }

    private fun capture(words: Words, name: String, dialog: Boolean = false) {
        if (words.language != "en") return
        rule.waitForIdle()
        val node = if (dialog) rule.onNode(isDialog()) else rule.onNodeWithTag("link-switch-shot")
        val bitmap = node.captureToImage().asAndroidBitmap()
        assertTrue("$name has no rendered area", bitmap.width > 0 && bitmap.height > 0)
        val background = bitmap.getPixel(0, 0)
        var different = 0
        for (y in 0 until bitmap.height step 7) for (x in 0 until bitmap.width step 7) {
            if (bitmap.getPixel(x, y) != background) different++
        }
        assertTrue("$name drew no content", different > 20)
        File(out(), "$name.png").outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
    }

    private fun suffix() = "${style.name.lowercase()}-${if (dark) "dark" else "light"}"

    private fun verify(words: Words, fontScale: Float) {
        starts = 0; checks = 0
        mount(fontScale)
        // An older hub has no field, and quiet ineligible reasons add no new UI.
        show(null)
        rule.onNodeWithTag("machine-link-switch").assertDoesNotExist()
        show(Scene("offline", MachineLinkSwitch(available = false, reason = "offline"), expected = R.string.machines_link_switch_intro))
        rule.onNodeWithTag("machine-link-switch").assertDoesNotExist()
        show(Scene("unsupported", MachineLinkSwitch(available = false, reason = "unsupported_os"),
            expected = R.string.machines_link_switch_why_unsupported_os))
        assertScene(scene!!)
        rule.onNodeWithTag("machine-link-switch-action").assertDoesNotExist()
        show(Scene("future", eligible.copy(phase = "a_future_phase"), expected = R.string.machines_link_switch_intro))
        assertScene(scene!!)
        rule.onNodeWithTag("machine-link-switch-action").assertDoesNotExist()

        // Opening and cancelling the explanation cannot start anything; only its explicit confirm calls onStart.
        show(Scene("eligible", eligible, expected = R.string.machines_link_switch_intro))
        rule.onNodeWithText(words.intro).assertIsDisplayed()
        rule.onNodeWithText(words.action).assertIsDisplayed()
        rule.onNodeWithTag("machine-link-switch-action").performClick()
        rule.onNode(isDialog()).assertIsDisplayed()
        rule.onNodeWithText(text(R.string.machines_link_switch_title, "Lab")).assertExists()
        rule.onNodeWithText(text(R.string.machines_link_switch_body_what)).assertExists()
        rule.onNodeWithText(text(R.string.machines_link_switch_body_reconnect)).assertExists()
        rule.onNodeWithText(words.kept).assertExists()
        assertEquals(0, starts)
        rule.onNodeWithText(words.cancel).performClick()
        rule.onNode(isDialog()).assertDoesNotExist()
        assertEquals(0, starts)
        rule.onNodeWithTag("machine-link-switch-action").performClick()
        rule.onNodeWithText(words.confirm).performClick()
        rule.onNode(isDialog()).assertDoesNotExist()
        assertEquals(1, starts)

        // Every phase has a distinct authoritative presentation; only one phase needs the full pixel matrix below.
        for (phase in listOf("checking", "installing", "connecting")) {
            show(Scene(phase, MachineLinkSwitch(available = false, phase = phase),
                expected = R.string.machines_link_switch_switching))
            assertScene(scene!!)
            val step = when (phase) {
                "checking" -> R.string.machines_link_switch_step_checking
                "installing" -> R.string.machines_link_switch_step_installing
                else -> R.string.machines_link_switch_step_connecting
            }
            rule.onNodeWithText(text(step), substring = true, useUnmergedTree = true).assertExists()
            rule.onNodeWithTag("machine-link-switch-action").assertDoesNotExist()
        }

        val startingAttempt = LinkSwitchAttempt(linkSwitchKey(eligible))
        // A pushed terminal state is newer than the local placeholder even before the POST answers.
        show(Scene("authoritative-before-answer", MachineLinkSwitch(available = true, phase = "failed",
            error = "link_timeout", rollbackUnconfirmed = true), startingAttempt,
            R.string.machines_link_switch_failed_unconfirmed, R.string.machines_link_switch_why_link_timeout))
        assertScene(scene!!)
        rule.onNodeWithText(words.starting).assertDoesNotExist()
        // A definite refusal stays visible during its read-back, which is checking rather than still starting.
        show(Scene("refused-checking", eligible, startingAttempt.copy(notice = LinkSwitchNotice.Refused("busy")),
            R.string.machines_link_switch_refused, R.string.machines_link_switch_why_busy))
        assertScene(scene!!)
        rule.onNodeWithText(text(R.string.machines_link_switch_checking_status)).assertExists()
        val uncertainAttempt = LinkSwitchAttempt(linkSwitchKey(eligible), starting = false,
            notice = LinkSwitchNotice.Uncertain, settledAt = 0)
        val scenes = listOf(
            Scene("eligible", eligible, expected = R.string.machines_link_switch_intro),
            Scene("starting", eligible, startingAttempt, R.string.machines_link_switch_starting),
            Scene("installing", MachineLinkSwitch(available = false, phase = "installing"),
                expected = R.string.machines_link_switch_switching),
            Scene("rolling-back", MachineLinkSwitch(available = false, phase = "rolling_back", error = "install_failed"),
                expected = R.string.machines_link_switch_rolling_back),
            Scene("succeeded", MachineLinkSwitch(available = false, phase = "succeeded"),
                expected = R.string.machines_link_switch_succeeded),
            Scene("failed", MachineLinkSwitch(available = true, phase = "failed", error = "install_failed"),
                expected = R.string.machines_link_switch_failed, expectedArg = R.string.machines_link_switch_why_install_failed),
            Scene("rollback-unconfirmed", MachineLinkSwitch(available = true, phase = "failed", error = "link_timeout",
                rollbackUnconfirmed = true), expected = R.string.machines_link_switch_failed_unconfirmed,
                expectedArg = R.string.machines_link_switch_why_link_timeout),
            Scene("uncertain", eligible, uncertainAttempt, R.string.machines_link_switch_uncertain),
            Scene("uncertain-read-failed", eligible, uncertainAttempt.copy(refreshFailed = true),
                R.string.machines_link_switch_uncertain_failed),
            Scene("stale", eligible, uncertainAttempt.copy(notice = null, settledAt = -LINK_SWITCH_STALE_AFTER_MILLIS,
                accepted = MachineLinkSwitch(available = false, phase = "connecting")),
                R.string.machines_link_switch_stale),
            Scene("forbidden", eligible, uncertainAttempt.copy(notice = LinkSwitchNotice.Forbidden),
                R.string.machines_link_switch_forbidden),
            Scene("linger-off", MachineLinkSwitch(available = false, reason = "linger_off"),
                expected = R.string.machines_link_switch_why_linger_off,
                expectedLiteralArg = "sudo loginctl enable-linger owner"),
        )
        val size = if (fontScale > 1f) "large" else "default"
        for (s in AppStyle.entries) for (d in listOf(false, true)) for (current in scenes) {
            show(current, s, d)
            assertScene(current)
            capture(words, "link-switch-${current.name}-${suffix()}-$size")
            when (current.name) {
                "starting" -> rule.onNodeWithText(words.starting).assertExists()
                "succeeded" -> rule.onNodeWithText(words.success).assertExists()
                "uncertain" -> rule.onNodeWithTag("machine-link-switch-action").assertExists()
                "uncertain-read-failed", "stale" -> {
                    rule.onNodeWithText(words.checkStatus).assertExists()
                    rule.onNodeWithTag("machine-link-switch-action").assertDoesNotExist()
                    val before = checks
                    rule.onNodeWithTag("machine-link-switch-check").performClick()
                    assertEquals(before + 1, checks)
                }
                "forbidden", "starting", "installing", "rolling-back", "linger-off" ->
                    rule.onNodeWithTag("machine-link-switch-action").assertDoesNotExist()
            }
            if (current.name == "rolling-back") {
                rule.onNodeWithText(text(R.string.machines_link_switch_why_install_failed)).assertExists()
            }
        }

        // The actual dialog decor, not the host card, at every style/theme/text size.
        for (s in AppStyle.entries) for (d in listOf(false, true)) {
            show(Scene("eligible", eligible, expected = R.string.machines_link_switch_intro), s, d)
            rule.onNodeWithTag("machine-link-switch-action").performClick()
            rule.onNode(isDialog()).assertIsDisplayed()
            assertEquals(1, starts)
            capture(words, "link-switch-confirm-${suffix()}-$size", dialog = true)
            rule.onNodeWithText(words.cancel).performClick()
            assertEquals(1, starts)
        }
    }
}
