package persona.feature.machines

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import persona.core.contracts.Machine
import persona.core.contracts.MachineLinkSwitch
import persona.core.contracts.parseTime
import persona.core.designsystem.InlineTone
import persona.core.designsystem.JButton
import persona.core.designsystem.JButtonKind
import persona.core.designsystem.JButtonSize
import persona.core.designsystem.JCard
import persona.core.designsystem.JDialog
import persona.core.designsystem.JInlineStatus
import persona.core.designsystem.PersonaTheme
import persona.core.designsystem.ProgressStep
import persona.core.designsystem.StepProgress
import persona.core.designsystem.StepState
import persona.core.designsystem.Tokens

internal const val LINK_SWITCH_ATTEMPT_TTL_MILLIS = 2 * 60_000L
/** The hub shows a terminal record for an hour; after that plus a clock margin, an old accepted state needs a fresh read. */
internal const val LINK_SWITCH_STALE_AFTER_MILLIS = 62 * 60_000L

/** What one confirmed start got back, before the machine list can prove its outcome. */
internal sealed interface LinkSwitchNotice {
    data object Forbidden : LinkSwitchNotice
    data object Uncertain : LinkSwitchNotice
    /** The accepted operation is older than the hub's terminal-state window and has not been read back. */
    data object Stale : LinkSwitchNotice
    data class Refused(val code: String?) : LinkSwitchNotice
}

internal data class LinkSwitchAttempt(
    /** The complete listed state when the user confirmed; without timestamps, any change is newer. */
    val seen: String,
    /** A POST or its read-back is in flight, so another confirmation cannot start one. */
    val starting: Boolean = true,
    val accepted: MachineLinkSwitch? = null,
    val notice: LinkSwitchNotice? = null,
    /** The uncertain response could not be reconciled: only another read is safe. */
    val refreshFailed: Boolean = false,
    val settledAt: Long? = null,
)

internal fun linkSwitchKey(state: MachineLinkSwitch?): String = state?.let {
    listOf(it.available.toString(), it.reason.orEmpty(), it.phase.orEmpty(), it.error.orEmpty(),
        it.rollbackUnconfirmed.toString(), it.at.orEmpty()).joinToString("|")
}.orEmpty()

internal fun linkSwitchRunning(state: MachineLinkSwitch?): Boolean =
    state?.phase in setOf("checking", "installing", "connecting", "rolling_back")

/** An accepted active operation cannot simply age into another start: only a successful owning-hub read can retire it. */
internal fun linkSwitchNeedsStatus(attempt: LinkSwitchAttempt, now: Long): Boolean {
    val settled = attempt.settledAt ?: return false
    return !attempt.starting && linkSwitchRunning(attempt.accepted) &&
        now >= settled && now - settled >= LINK_SWITCH_STALE_AFTER_MILLIS
}

/** Reconnecting SSH while a start or its read-back is unresolved could interrupt the switch itself. */
internal fun linkSwitchConnectionBusy(
    listed: MachineLinkSwitch?,
    attempt: LinkSwitchAttempt?,
    now: Long = System.currentTimeMillis(),
): Boolean {
    val current = attempt?.takeIf { linkSwitchAttemptLive(it, now) }
    if (current?.starting == true) return true
    if (current?.notice is LinkSwitchNotice.Uncertain && current.refreshFailed) return true
    if (current != null && linkSwitchNeedsStatus(current, now)) return true
    return linkSwitchRunning(shownLinkSwitch(listed, current, now))
}

internal fun linkSwitchAttemptLive(attempt: LinkSwitchAttempt, now: Long): Boolean {
    if (attempt.starting || attempt.refreshFailed || attempt.settledAt == null) return true
    // Never age an active accepted operation back into an enabled start on a stale list.
    if (linkSwitchRunning(attempt.accepted)) return true
    return now >= attempt.settledAt && now - attempt.settledAt < LINK_SWITCH_ATTEMPT_TTL_MILLIS
}

internal fun linkSwitchCaughtUp(listed: MachineLinkSwitch, attempt: LinkSwitchAttempt): Boolean {
    val acceptedAt = parseTime(attempt.accepted?.at)
    val listedAt = parseTime(listed.at)
    return if (acceptedAt != null) listedAt != null && !listedAt.isBefore(acceptedAt)
    else linkSwitchKey(listed) != attempt.seen
}

/** The accepted 202 wins over an older pre-start list; otherwise the hub's current list wins. */
internal fun shownLinkSwitch(
    listed: MachineLinkSwitch?,
    attempt: LinkSwitchAttempt?,
    now: Long = System.currentTimeMillis(),
): MachineLinkSwitch? {
    val current = attempt?.takeIf { linkSwitchAttemptLive(it, now) } ?: return listed
    // Never present a pre-retention accepted phase as current forever. The remaining Check status action is read-only.
    if (linkSwitchNeedsStatus(current, now)) return listed
    val accepted = current.accepted ?: return listed
    if (listed == null) return accepted
    return if (linkSwitchCaughtUp(listed, current)) listed else accepted
}

internal fun linkSwitchNotice(
    listed: MachineLinkSwitch?,
    attempt: LinkSwitchAttempt?,
    now: Long = System.currentTimeMillis(),
): LinkSwitchNotice? {
    val current = attempt?.takeIf { linkSwitchAttemptLive(it, now) } ?: return null
    if (linkSwitchNeedsStatus(current, now)) return LinkSwitchNotice.Stale
    if (current.notice is LinkSwitchNotice.Uncertain && current.refreshFailed) return current.notice
    return current.notice?.takeIf { notice ->
        linkSwitchKey(listed) == current.seen && (!current.starting || notice is LinkSwitchNotice.Refused)
    }
}

/** Only the hub makes a computer eligible. Unknown future phases never turn into a start action. */
internal fun canStartLinkSwitch(
    listed: MachineLinkSwitch?,
    attempt: LinkSwitchAttempt?,
    now: Long = System.currentTimeMillis(),
): Boolean {
    val current = attempt?.takeIf { linkSwitchAttemptLive(it, now) }
    if (current != null && linkSwitchNeedsStatus(current, now)) return false
    val shown = shownLinkSwitch(listed, current, now) ?: return false
    if (!shown.available || shown.phase !in setOf(null, "failed", "succeeded")) return false
    if (current?.starting == true) return false
    if (current?.refreshFailed == true && linkSwitchNotice(listed, current, now) is LinkSwitchNotice.Uncertain) return false
    return linkSwitchNotice(listed, current, now) !is LinkSwitchNotice.Forbidden
}

/** Local Starting / Checking is only a placeholder until a changed authoritative operation phase arrives. */
internal fun linkSwitchWaiting(listed: MachineLinkSwitch?, attempt: LinkSwitchAttempt?): Boolean =
    attempt?.starting == true &&
        (attempt.accepted == null || attempt.notice is LinkSwitchNotice.Uncertain) &&
        (listed?.phase == null || linkSwitchKey(listed) == attempt.seen)

internal fun showsLinkSwitch(listed: MachineLinkSwitch?, attempt: LinkSwitchAttempt?): Boolean {
    if (listed == null && attempt == null) return false
    if (attempt != null) return true
    return listed != null &&
        (listed.reason !in setOf("offline", "busy") || listed.phase != null)
}

@StringRes
internal fun linkSwitchReasonText(code: String?): Int = when (code) {
    "offline" -> R.string.machines_link_switch_why_offline
    "unsupported_os" -> R.string.machines_link_switch_why_unsupported_os
    "link_unavailable" -> R.string.machines_link_switch_why_link_unavailable
    "hub_old" -> R.string.machines_link_switch_why_hub_old
    "pending" -> R.string.machines_link_switch_why_pending
    "custom_node" -> R.string.machines_link_switch_why_custom_node
    "busy" -> R.string.machines_link_switch_why_busy
    "in_progress" -> R.string.machines_link_switch_why_in_progress
    "no_systemd" -> R.string.machines_link_switch_why_no_systemd
    "linger_off" -> R.string.machines_link_switch_why_linger_off
    "existing_link" -> R.string.machines_link_switch_why_existing_link
    "no_node" -> R.string.machines_link_switch_why_no_node
    "no_package" -> R.string.machines_link_switch_why_no_package
    "transfer_failed" -> R.string.machines_link_switch_why_transfer_failed
    "install_failed" -> R.string.machines_link_switch_why_install_failed
    "link_timeout" -> R.string.machines_link_switch_why_link_timeout
    "config_changed" -> R.string.machines_link_switch_why_config_changed
    "interrupted" -> R.string.machines_link_switch_why_interrupted
    "unsafe_permissions" -> R.string.machines_link_switch_why_unsafe_permissions
    "hub_unanswered" -> R.string.machines_link_switch_why_hub_unanswered
    "unknown" -> R.string.machines_link_switch_why_unknown
    else -> R.string.machines_link_switch_why_unavailable
}

internal fun machineLogin(home: String?): String? = home?.split('/')?.lastOrNull { it.isNotEmpty() }

@Composable
private fun linkSwitchReason(code: String?, home: String?): String {
    if (code != "linger_off") return stringResource(linkSwitchReasonText(code))
    val user = machineLogin(home) ?: stringResource(R.string.machines_link_switch_linger_user)
    return stringResource(R.string.machines_link_switch_why_linger_off, "sudo loginctl enable-linger $user")
}

@StringRes
private fun linkSwitchStepText(phase: String): Int = when (phase) {
    "checking" -> R.string.machines_link_switch_step_checking
    "installing" -> R.string.machines_link_switch_step_installing
    else -> R.string.machines_link_switch_step_connecting
}

@Composable
private fun LinkSwitchProgress(phase: String) {
    val phases = listOf("checking", "installing", "connecting")
    val current = phases.indexOf(phase).coerceAtLeast(0)
    Column(verticalArrangement = Arrangement.spacedBy(Tokens.Space.m)) {
        Text(stringResource(R.string.machines_link_switch_switching), style = PersonaTheme.type.callout, color = PersonaTheme.palette.ink)
        StepProgress(phases.mapIndexed { index, step ->
            ProgressStep(stringResource(linkSwitchStepText(step)), when {
                index < current -> StepState.Complete
                index == current -> StepState.Active
                else -> StepState.Pending
            })
        })
    }
}

/** The advanced computer detail's explicit SSH-to-Link action and its persistent operation state. */
@Composable
internal fun MachineLinkSwitchSection(
    machine: Machine,
    attempt: LinkSwitchAttempt?,
    onStart: () -> Unit,
    onCheck: () -> Unit,
    now: Long = System.currentTimeMillis(),
) {
    if (!showsLinkSwitch(machine.linkSwitch, attempt)) return
    val listed = machine.linkSwitch
    val shown = shownLinkSwitch(listed, attempt, now)
    val notice = linkSwitchNotice(listed, attempt, now)
    val canStart = canStartLinkSwitch(listed, attempt, now)
    var confirming by rememberSaveable(machine.id) { mutableStateOf(false) }

    Column(
        Modifier.testTag("machine-link-switch").semantics { liveRegion = LiveRegionMode.Polite },
        verticalArrangement = Arrangement.spacedBy(Tokens.Space.s),
    ) {
        JCard {
            Column(verticalArrangement = Arrangement.spacedBy(Tokens.Space.m)) {
                when {
                    linkSwitchWaiting(listed, attempt) -> JInlineStatus(
                        stringResource(if (attempt?.notice != null) R.string.machines_link_switch_checking_status
                        else R.string.machines_link_switch_starting), maxLines = Int.MAX_VALUE)
                    shown?.phase in setOf("checking", "installing", "connecting") -> LinkSwitchProgress(shown!!.phase!!)
                    shown?.phase == "rolling_back" -> Column(verticalArrangement = Arrangement.spacedBy(Tokens.Space.xs)) {
                        JInlineStatus(stringResource(R.string.machines_link_switch_rolling_back), maxLines = Int.MAX_VALUE)
                        shown.error?.let {
                            Text(linkSwitchReason(it, machine.home), style = PersonaTheme.type.callout, color = PersonaTheme.palette.ink2)
                        }
                    }
                    shown?.phase == "succeeded" -> JInlineStatus(stringResource(R.string.machines_link_switch_succeeded),
                        tone = InlineTone.Done, maxLines = Int.MAX_VALUE)
                    shown?.phase == "failed" -> JInlineStatus(
                        stringResource(if (shown.rollbackUnconfirmed) R.string.machines_link_switch_failed_unconfirmed
                        else R.string.machines_link_switch_failed, linkSwitchReason(shown.error, machine.home)),
                        tone = InlineTone.Failed, maxLines = Int.MAX_VALUE)
                    shown != null -> Text(
                        if (shown.available) stringResource(R.string.machines_link_switch_intro) else linkSwitchReason(shown.reason, machine.home),
                        style = PersonaTheme.type.callout, color = PersonaTheme.palette.ink2)
                }
                if (canStart && attempt?.starting != true) {
                    JButton(stringResource(R.string.machines_link_switch_action), { confirming = true },
                        Modifier.testTag("machine-link-switch-action"), kind = JButtonKind.Secondary, size = JButtonSize.Small)
                }
                if (notice != null && (attempt?.starting != true || notice is LinkSwitchNotice.Refused)) {
                    val text = when (notice) {
                        LinkSwitchNotice.Forbidden -> stringResource(R.string.machines_link_switch_forbidden)
                        LinkSwitchNotice.Stale -> stringResource(R.string.machines_link_switch_stale)
                        LinkSwitchNotice.Uncertain -> stringResource(if (attempt?.refreshFailed == true) R.string.machines_link_switch_uncertain_failed
                        else R.string.machines_link_switch_uncertain)
                        is LinkSwitchNotice.Refused -> stringResource(R.string.machines_link_switch_refused,
                            linkSwitchReason(notice.code, machine.home))
                    }
                    JInlineStatus(text, tone = if (notice is LinkSwitchNotice.Uncertain || notice is LinkSwitchNotice.Stale) InlineTone.Caution else InlineTone.Failed,
                        maxLines = Int.MAX_VALUE)
                    if ((notice is LinkSwitchNotice.Uncertain && attempt?.refreshFailed == true) ||
                        (notice is LinkSwitchNotice.Stale && attempt != null && linkSwitchNeedsStatus(attempt, now))) {
                        JButton(stringResource(R.string.machines_link_switch_check_status), onCheck,
                            Modifier.testTag("machine-link-switch-check"), kind = JButtonKind.Secondary, size = JButtonSize.Small)
                    }
                }
            }
        }
    }

    if (confirming) {
        JDialog(
            onDismiss = { confirming = false },
            title = stringResource(R.string.machines_link_switch_title, machine.displayName),
            confirm = stringResource(R.string.machines_link_switch_confirm) to {
                confirming = false
                onStart()
            },
        ) {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(Tokens.Space.m)) {
                Text(stringResource(R.string.machines_link_switch_body_what), style = PersonaTheme.type.callout, color = PersonaTheme.palette.ink2)
                Text(stringResource(R.string.machines_link_switch_body_reconnect), style = PersonaTheme.type.callout, color = PersonaTheme.palette.ink2)
                Text(stringResource(R.string.machines_link_switch_body_kept), style = PersonaTheme.type.callout, color = PersonaTheme.palette.ink2)
            }
        }
    }
}
