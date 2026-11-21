package persona.feature.machines

import android.content.res.Resources
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import persona.core.contracts.Machine
import persona.core.contracts.api.AgentLauncher
import persona.core.contracts.api.MachineManagement
import persona.core.contracts.api.SessionsRepository
import persona.core.contracts.api.hubCodedFailure
import persona.core.designsystem.UiText
import persona.core.designsystem.failureReasonText
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

/** What a finished action says; the route turns it into text in the page's language. */
internal sealed interface MachineNotice {
    data object Connected : MachineNotice
    /**
     * Still offline after the wait, with why: the plain words of [connectionFailureText] when the hub's error says,
     * else the hub's own error, else nothing (#4537).
     */
    data class NotYet(val reason: UiText?) : MachineNotice
    data class ReconnectFailed(val reason: UiText) : MachineNotice
    data class DeployFailed(val reason: UiText) : MachineNotice
    /** "Also show botmux sessions" was not saved; the switch is back where it was. */
    data class BotmuxFailed(val reason: UiText) : MachineNotice
    /** Move to…: the computer is [name]'s now (docs/assistant/managers.md). */
    data class Moved(val name: String) : MachineNotice
    data class MoveFailed(val reason: UiText) : MachineNotice
    /** Renamed: the hub calls the computer [name] now. */
    data class Renamed(val name: String) : MachineNotice
    data class RenameFailed(val reason: UiText) : MachineNotice

    fun text(resources: Resources): String = when (this) {
        Connected -> resources.getString(R.string.machines_notice_connected)
        is NotYet -> reason?.let { resources.getString(R.string.machines_notice_not_yet_reason, it.resolve(resources)) }
            ?: resources.getString(R.string.machines_notice_not_yet)
        is ReconnectFailed -> resources.getString(R.string.machines_notice_reconnect_failed, reason.resolve(resources))
        is DeployFailed -> resources.getString(R.string.machines_notice_deploy_failed, reason.resolve(resources))
        is BotmuxFailed -> resources.getString(R.string.machines_notice_botmux_failed, reason.resolve(resources))
        is Moved -> resources.getString(R.string.machines_notice_moved, name)
        is MoveFailed -> resources.getString(R.string.machines_notice_move_failed, reason.resolve(resources))
        is Renamed -> resources.getString(R.string.machines_notice_renamed, name)
        is RenameFailed -> resources.getString(R.string.machines_notice_rename_failed, reason.resolve(resources))
    }
}

private const val RECONNECT_POLL_MILLIS = 3_000L

/**
 * Why a machine is still offline, as the offline card reads it (#4463): a known error_code, else plain words read from
 * the hub's error, else that error.
 */
internal fun notYetReason(machine: Machine): UiText? =
    (machineErrorText(machine.errorCode) ?: connectionFailureText(machine))?.let { UiText.Res(it) }
        ?: machine.error?.takeIf { it.isNotBlank() }?.let { UiText.Plain(it) }

internal data class MachineActionsState(
    /** The machine's launchers; null until read (offline machines have none to list). */
    val launchers: List<AgentLauncher>? = null,
    val launchersLoading: Boolean = false,
    /** Why the launchers could not be read (the hub's reason), shown as "Couldn't read launchers: ..." */
    val launchersError: UiText? = null,
    val reconnecting: Boolean = false,
    /** Machines whose "Redeploy" was sent and not answered yet; the hub's `deploying` takes over after that. */
    val deployStarting: Set<String> = emptySet(),
    /** "Also show botmux sessions" (#2575); null until the hub answered, and on an older hub without the route: no switch. */
    val botmux: Boolean? = null,
    val botmuxSaving: Boolean = false,
    /** Move to… was sent and not answered yet. */
    val moving: Boolean = false,
    /** Rename was sent and not answered yet. */
    val renaming: Boolean = false,
    /** One confirmed SSH-to-Link start per computer, retained until the machine list resolves it. */
    val linkSwitches: Map<String, LinkSwitchAttempt> = emptyMap(),
)

/**
 * What the machine pages change on a machine (iOS `MachineDetailView`): its launchers, "Also show botmux sessions", "Reconnect" (waits up to
 * [reconnectWaitMillis] for it to come online and says how it went) and "Redeploy". Results are short notices.
 */
internal class MachineActionsViewModel(
    private val management: MachineManagement,
    private val sessions: SessionsRepository,
    private val reconnectWaitMillis: Long = 15_000,
    private val linkSwitchClock: () -> Long = { System.currentTimeMillis() },
) : ViewModel() {
    private val mutable = MutableStateFlow(MachineActionsState())
    val state = mutable.asStateFlow()
    private val noticeChannel = Channel<MachineNotice>(Channel.BUFFERED)
    /** One notice per finished action, for the app's feedback. */
    val notices = noticeChannel.receiveAsFlow()
    private var launchersJob: Job? = null
    private val observedLinkSwitches = mutableSetOf<String>()

    /** Again when it comes online (after "Reconnect"): an offline machine has no launchers to list. */
    fun loadLaunchers(machineId: String, online: Boolean) {
        launchersJob?.cancel()
        if (!online) {
            mutable.update { it.copy(launchers = null, launchersLoading = false, launchersError = null) }
            return
        }
        mutable.update { it.copy(launchersLoading = true, launchersError = null) }
        launchersJob = viewModelScope.launch {
            try {
                val launchers = management.launchers(machineId)
                mutable.update { it.copy(launchers = launchers, launchersLoading = false) }
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (error: Exception) {
                mutable.update { it.copy(launchersLoading = false, launchersError = failureReasonText(error)) }
            }
        }
    }

    /** Reads "Also show botmux sessions"; a hub that cannot answer leaves it null, so the page draws no switch (iOS `BotmuxSwitchModel`). */
    fun loadBotmux(machineId: String) {
        viewModelScope.launch {
            val on = try { management.botmux(machineId) }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { null }
            mutable.update { it.copy(botmux = on) }
        }
    }

    /** Shows [on] at once and saves it (not part of the engine choice); a failed save puts the old value back and says why. */
    fun setBotmux(machineId: String, on: Boolean) {
        val before = state.value.botmux ?: return
        if (state.value.botmuxSaving) return
        mutable.update { it.copy(botmux = on, botmuxSaving = true) }
        viewModelScope.launch {
            try {
                val saved = management.setBotmux(machineId, on)
                mutable.update { it.copy(botmux = saved) }
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (error: Exception) {
                mutable.update { it.copy(botmux = before) }
                noticeChannel.trySend(MachineNotice.BotmuxFailed(failureReasonText(error)))
            } finally { mutable.update { it.copy(botmuxSaving = false) } }
        }
    }

    /** Renames the computer on its hub ("" is its id again); the hub's `machines` push brings the name to the page, read now too. */
    fun rename(machineId: String, name: String) {
        if (state.value.renaming) return
        mutable.update { it.copy(renaming = true) }
        viewModelScope.launch {
            try {
                val shown = management.rename(machineId, name.trim())
                noticeChannel.trySend(MachineNotice.Renamed(shown))
                try { sessions.refresh() } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { }
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (error: Exception) { noticeChannel.trySend(MachineNotice.RenameFailed(failureReasonText(error))) }
            finally { mutable.update { it.copy(renaming = false) } }
        }
    }

    fun reconnect(machineId: String) {
        if (state.value.reconnecting) return
        mutable.update { it.copy(reconnecting = true) }
        viewModelScope.launch {
            try {
                try { management.reconnect(machineId) }
                catch (cancelled: CancellationException) { throw cancelled }
                catch (error: Exception) {
                    noticeChannel.trySend(MachineNotice.ReconnectFailed(failureReasonText(error)))
                    return@launch
                }
                val back = withTimeoutOrNull(reconnectWaitMillis) {
                    coroutineScope {
                        // The hub pushes the machine list when the node is back; reading it now and then covers a missed push.
                        val poll = launch {
                            while (true) {
                                delay(RECONNECT_POLL_MILLIS)
                                try { sessions.refresh() } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { }
                            }
                        }
                        sessions.machines.first { list -> list.any { it.id == machineId && it.online } }.also { poll.cancel() }
                    }
                }
                noticeChannel.trySend(if (back != null) MachineNotice.Connected else {
                    MachineNotice.NotYet(sessions.machines.value.firstOrNull { it.id == machineId }?.let(::notYetReason))
                })
            } finally { mutable.update { it.copy(reconnecting = false) } }
        }
    }

    /** Move to… [manager] (named [name] in the notice): the hub's `machines` push then shows the new stamp; read now too. */
    fun moveTo(machineId: String, manager: String, name: String) {
        if (state.value.moving) return
        mutable.update { it.copy(moving = true) }
        viewModelScope.launch {
            try {
                management.moveToManager(machineId, manager)
                noticeChannel.trySend(MachineNotice.Moved(name))
                try { sessions.refresh() } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { }
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (error: Exception) { noticeChannel.trySend(MachineNotice.MoveFailed(failureReasonText(error))) }
            finally { mutable.update { it.copy(moving = false) } }
        }
    }

    /** Reconciles one computer's accepted start or warning with every authoritative machine-list update. */
    fun observeLinkSwitch(machineId: String) {
        if (!observedLinkSwitches.add(machineId)) return
        viewModelScope.launch {
            sessions.machines.collect { machines ->
                reconcileLinkSwitch(machineId, machines.firstOrNull { it.id == machineId }?.linkSwitch)
            }
        }
    }

    private fun putLinkSwitch(machineId: String, change: (LinkSwitchAttempt?) -> LinkSwitchAttempt?) {
        mutable.update { state ->
            val attempts = state.linkSwitches.toMutableMap()
            change(attempts[machineId])?.let { attempts[machineId] = it } ?: attempts.remove(machineId)
            state.copy(linkSwitches = attempts.toMap())
        }
    }

    private fun reconcileLinkSwitch(machineId: String, listed: persona.core.contracts.MachineLinkSwitch?) {
        val now = linkSwitchClock()
        putLinkSwitch(machineId) { attempt ->
            attempt ?: return@putLinkSwitch null
            if (attempt.starting) return@putLinkSwitch attempt
            if (!linkSwitchAttemptLive(attempt, now)) return@putLinkSwitch null
            // A changed cached/pushed row cannot turn a failed reconciliation into permission to start again.
            if (attempt.refreshFailed) return@putLinkSwitch attempt
            val accepted = attempt.accepted
            if (accepted != null && listed != null && linkSwitchCaughtUp(listed, attempt)) return@putLinkSwitch null
            if (accepted == null && attempt.notice != null && linkSwitchKey(listed) != attempt.seen) return@putLinkSwitch null
            attempt
        }
    }

    /**
     * Starts once for this explicit confirmation. A refused or uncertain answer is read back through `machines`, never
     * automatically posted again. A failed uncertain read leaves only Check status; after a successful one, another POST
     * still needs a new explicit confirmation.
     */
    fun startLinkSwitch(machineId: String) {
        observeLinkSwitch(machineId)
        val listed = sessions.machines.value.firstOrNull { it.id == machineId }?.linkSwitch
        val existing = state.value.linkSwitches[machineId]
        if (!canStartLinkSwitch(listed, existing, linkSwitchClock())) return
        val seen = linkSwitchKey(listed)
        putLinkSwitch(machineId) { LinkSwitchAttempt(seen) }
        viewModelScope.launch {
            try {
                val accepted = management.startLinkSwitch(machineId)
                putLinkSwitch(machineId) { current ->
                    current?.takeIf { it.starting && it.seen == seen }?.copy(
                        starting = false, accepted = accepted, settledAt = linkSwitchClock())
                }
                reconcileLinkSwitch(machineId, sessions.machines.value.firstOrNull { it.id == machineId }?.linkSwitch)
                return@launch
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (error: Exception) {
                val coded = hubCodedFailure(error)
                val status = coded?.status
                val notice = when {
                    status == 403 -> LinkSwitchNotice.Forbidden
                    status == 409 || (status != null && status >= 400 && status < 500 && status != 408 && status != 429) ->
                        LinkSwitchNotice.Refused(coded.code)
                    else -> LinkSwitchNotice.Uncertain
                }
                putLinkSwitch(machineId) { current ->
                    current?.takeIf { it.starting && it.seen == seen }?.copy(notice = notice)
                }
                if (notice is LinkSwitchNotice.Forbidden) {
                    putLinkSwitch(machineId) { current ->
                        current?.takeIf { it.seen == seen }?.copy(starting = false, settledAt = linkSwitchClock())
                    }
                    return@launch
                }
                var refreshFailed = false
                try { sessions.refreshMachine(machineId) }
                catch (cancelled: CancellationException) { throw cancelled }
                catch (_: Exception) { refreshFailed = true }
                putLinkSwitch(machineId) { current ->
                    current?.takeIf { it.starting && it.seen == seen && it.notice == notice }?.copy(
                        starting = false, refreshFailed = refreshFailed, settledAt = linkSwitchClock())
                }
                reconcileLinkSwitch(machineId, sessions.machines.value.firstOrNull { it.id == machineId }?.linkSwitch)
            } finally {
                // A repository can cancel its own refresh without destroying this ViewModel. Never leave an
                // unanswerable "Starting" card: an interrupted POST/read-back is uncertain and offers Check status.
                putLinkSwitch(machineId) { current ->
                    if (current == null || !current.starting || current.seen != seen) current
                    else {
                        val notice = current.notice ?: LinkSwitchNotice.Uncertain
                        current.copy(starting = false, notice = notice,
                            refreshFailed = notice is LinkSwitchNotice.Uncertain, settledAt = linkSwitchClock())
                    }
                }
            }
        }
    }

    /** A read-only recovery after an uncertain response or after an accepted operation outlives the hub's terminal window. */
    fun checkLinkSwitch(machineId: String) {
        val attempt = state.value.linkSwitches[machineId] ?: return
        val stale = linkSwitchNeedsStatus(attempt, linkSwitchClock())
        if (attempt.starting || attempt.notice !is LinkSwitchNotice.Uncertain && !stale ||
            !attempt.refreshFailed && !stale) return
        putLinkSwitch(machineId) { current ->
            current?.copy(starting = true, refreshFailed = false, notice = current.notice ?: LinkSwitchNotice.Uncertain)
        }
        viewModelScope.launch {
            var refreshFailed = false
            var refreshed = false
            try {
                try {
                    sessions.refreshMachine(machineId)
                    refreshed = true
                }
                catch (cancelled: CancellationException) { throw cancelled }
                catch (_: Exception) { refreshFailed = true }
                putLinkSwitch(machineId) { current ->
                    current?.takeIf { it.starting && it.seen == attempt.seen && it.notice is LinkSwitchNotice.Uncertain }?.copy(
                        starting = false, refreshFailed = refreshFailed,
                        // Keep an old accepted operation old: a failed read must not give it another hour of stale progress.
                        settledAt = if (stale) current.settledAt else linkSwitchClock())
                }
                if (stale && refreshed) {
                    // This successful owning-hub response, not a timer or a cached empty row, retires the old overlay.
                    // The response's own running/terminal/eligible/absent state is now the only thing displayed.
                    putLinkSwitch(machineId) { current ->
                        current?.takeUnless { !it.starting && it.seen == attempt.seen && !it.refreshFailed }
                    }
                } else {
                    reconcileLinkSwitch(machineId, sessions.machines.value.firstOrNull { it.id == machineId }?.linkSwitch)
                }
            } finally {
                putLinkSwitch(machineId) { current ->
                    if (current == null || !current.starting || current.seen != attempt.seen ||
                        current.notice !is LinkSwitchNotice.Uncertain) current
                    else current.copy(starting = false, refreshFailed = true, settledAt = current.settledAt)
                }
            }
        }
    }

    fun deploy(machineId: String) {
        if (machineId in state.value.deployStarting) return
        mutable.update { it.copy(deployStarting = it.deployStarting + machineId) }
        viewModelScope.launch {
            try {
                management.deploy(machineId)
                // Progress and the outcome come with the machine list (deploying, deploy_error, node_mismatch).
                try { sessions.refresh() } catch (cancelled: CancellationException) { throw cancelled } catch (_: Exception) { }
            } catch (cancelled: CancellationException) { throw cancelled }
            catch (error: Exception) { noticeChannel.trySend(MachineNotice.DeployFailed(failureReasonText(error))) }
            finally { mutable.update { it.copy(deployStarting = it.deployStarting - machineId) } }
        }
    }
}
