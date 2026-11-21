package persona.feature.chat

import persona.core.designsystem.dense
import persona.core.designsystem.text
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import persona.core.contracts.*
import persona.core.contracts.api.*
import persona.core.designsystem.*
import persona.core.designsystem.icons.PersonaIcons
import persona.core.designsystem.icons.PhIcons
import persona.core.domain.lines.isLiveStatus
import persona.core.domain.sort.AgentSession
import persona.core.domain.sort.SessionRename
import persona.core.domain.sort.SideRollup
import persona.core.domain.sort.canAttach
import persona.core.domain.sort.canClose
import persona.core.domain.sort.canInterrupt
import persona.core.domain.sort.canRename
import persona.core.domain.sort.canTerminal
import persona.core.domain.sort.closeConfirmation
import persona.core.domain.sort.deleteConfirmation
import persona.core.domain.sort.isRex
import persona.core.domain.sort.isTern
import persona.core.domain.sort.resumeCommand
import persona.core.domain.sort.slotLabel
import persona.core.domain.sort.stateLabel
import persona.core.domain.sort.source
import persona.core.domain.sort.sourceSubtitle
import persona.core.domain.sort.TerminalResume
import persona.core.domain.sort.terminalResume
import persona.core.domain.sort.worktreeKind

@Composable
internal fun ChatHeader(
    session: Session?, machine: Machine?, sessions: List<Session>, state: ChatChromeState,
    connection: ConnectionState?, readOnly: Boolean, onBack: () -> Unit,
    onOpenSession: ((SessionKey) -> Unit)?, onConnectionDetails: (() -> Unit)?,
    onOverlay: (Boolean) -> Unit,
    onFiles: (() -> Unit)? = null, onGit: (() -> Unit)? = null, gitLabel: String = "Git",
    onSearch: (() -> Unit)? = null,
    statusState: StatusLineState = StatusLineState(), onDetails: () -> Unit = {},
    onBackgroundTasks: (() -> Unit)? = null,
    onOpenTodo: ((String) -> Unit)? = null,
    agentShares: (() -> AgentShares)? = null,
    worktrees: (() -> persona.core.contracts.api.AgentCreationService?)? = null,
    /** Opens the session's terminal: ⋯'s Terminal, and the header's button on Brief and Board ([ChatTerminalPlaces]). */
    onTerminal: (() -> Unit)? = null,
    /** The chat has a terminal it can't open from here: Brief's and Board's header button explains it instead. */
    onTerminalHelp: (() -> Unit)? = null,
    /**
     * Resume in Terminal (#5565): opens the session's terminal and resumes the stopped Agent there (iOS / web same
     * action). Nil when the shell has no terminal to open.
     */
    onResumeInTerminal: (() -> Unit)? = null,
    /** When the turn that runs now began (the newest turn's start, iOS `Turn.startedAt`): "Working 22 min" counts from it. */
    turnStart: java.time.Instant? = null,
    /**
     * The side sessions folded under this session (#7944), in their panel's order, and what the header's Side sessions
     * control says ([SideRollup]); [onOpenSide] opens one's own chat. Shown only while it has sides.
     */
    sides: List<Session> = emptyList(),
    sideRollup: SideRollup = SideRollup(),
    onOpenSide: ((SessionKey) -> Unit)? = null,
    menuOpen: MutableState<Boolean> = remember { mutableStateOf(false) },
    actions: @Composable RowScope.() -> Unit = {},
) {
    var menu by menuOpen
    var sharing by remember(session?.key, agentShares) { mutableStateOf<AgentShares?>(null) }
    var dialog by rememberSaveable { mutableStateOf<String?>(null) }
    var title by rememberSaveable { mutableStateOf("") }
    var tabName by rememberSaveable { mutableStateOf("") }
    val copy = rememberCopy()
    val peers = remember(session, sessions) { spaceCompanions(session, sessions) }
    val places = remember(session, peers) { companionPlaces(session, peers) }
    val children = session?.subagents.orEmpty().filter { it.running && it.sessionId.isNotBlank() }
    val linkTitle = connectionTitle(connection)
    val online = connection == null || connection == ConnectionState.Online
    val canAct = state.available && online && machine?.online != false && !readOnly && session != null
    // Renaming changes the terminal tab and Claude's /rename; a session without a terminal renames where its node says it
    // can (a runtime's title, #6294).
    val canRename = state.canRename && online && machine?.online != false && !readOnly && session != null && session.canRename
    LaunchedEffect(session?.key, menu) { if (session != null && menu) state.loadMute(session) }
    var modeOverlay by remember { mutableStateOf(false) }
    LaunchedEffect(menu, dialog, sharing, modeOverlay) { onOverlay(menu || dialog != null || sharing != null || modeOverlay) }
    DisposableEffect(Unit) { onDispose { onOverlay(false) } }
    val line = resolveStatusLine(statusState)
    val status = headerStatus(statusState, session)
    // iOS `stateWords`: its turn ended and a workflow or background work keeps it going, or all it runs only waits; else how
    // long this turn has worked, in minutes (the work line in the chat has the seconds), read again every 30 s.
    val agent = remember(session) { session?.let(::AgentSession) }
    val inBackground = agent?.runsInBackground == true
    val since = workedSince(agent, turnStart)?.takeIf { status == Status.Working }
    val now by produceState(java.time.Instant.now(), since) {
        while (since != null) { kotlinx.coroutines.delay(30_000); value = java.time.Instant.now() }
    }
    val worked = workedMinutes(since, now)?.let { m ->
        if (m < 60) stringResource(R.string.chat_chrome_worked_minutes, m) else stringResource(R.string.chat_chrome_worked_hours, m / 60, m % 60)
    }
    // While the hub is out of reach the status line under the title says so ("Waiting for network" / "Connecting…" / "Updating…") instead of
    // the session's state, which may be stale; it is back the moment the connection is.
    ChatTopBar(
        title = session?.displayTitle ?: stringResource(R.string.chat_chrome_title_default), onBack = onBack,
        status = if (linkTitle != null) null else status,
        statusText = linkTitle ?: when (line?.kind) {
            StatusLineKind.Ended -> stringResource(if (statusState.replaced) R.string.chat_chrome_status_replaced else R.string.chat_chrome_status_ended)
            StatusLineKind.MachineOffline -> stringResource(R.string.chat_chrome_status_machine_offline)
            StatusLineKind.Submitting -> if (statusState.answering) stringResource(R.string.chat_chrome_status_submitting)
                else stringResource(persona.core.designsystem.R.string.designsystem_message_sending)
            StatusLineKind.Processing -> stringResource(persona.core.designsystem.R.string.designsystem_message_processing)
            StatusLineKind.Unconfirmed -> stringResource(R.string.chat_chrome_status_unconfirmed)
            StatusLineKind.NeedsYou -> stringResource(R.string.chat_chrome_status_needs_you)
            StatusLineKind.Stopping -> stringResource(R.string.chat_chrome_status_stopping)
            // As iOS (#4339 C4): Seen and the other quiet states say nothing in words, the dot carries them; a turn
            // that has run a minute says for how long ("Working 22 min").
            else -> when {
                inBackground -> agent!!.stateLabel.text()
                status in QuietStatuses -> null
                worked != null -> "${status.localizedLabel} $worked"
                // The hub counts background commands as working: the state's name, not the status line's own "Waiting".
                status == Status.Waiting && agent?.state == AgentState.Working -> Status.Working.localizedLabel
                else -> status.localizedLabel
            }
        },
        source = session?.let { chatPlace(it, machine?.displayName, it.sourceSubtitle?.text()) },
        sourceShort = session?.let { chatPlace(it, machine?.displayName, it.sourceSubtitle?.text(), short = true) },
        busy = connection in setOf(ConnectionState.Connecting, ConnectionState.Updating),
        // Glass's title opens Session Info, as iOS's (#4339 C4), or why the hub is out of reach while it is; Brief and
        // Board keep the status sheet.
        onTitleClick = when (ChatTitleTap.of(PersonaTheme.style, online = linkTitle == null, hasSession = session != null)) {
            ChatTitleTap.SessionInfo -> ({ dialog = "info" })
            ChatTitleTap.Connection -> ({ dialog = "connection" })
            ChatTitleTap.Status -> onDetails
        },
        agentMark = session?.agentMark,
        waiting = agent?.waitsInBackground == true,
        titleClickLabel = if (PersonaTheme.style == AppStyle.Glass)
            stringResource(if (linkTitle == null) R.string.chat_chrome_title_info else R.string.chat_chrome_title_connection) else null,
    ) {
        // Glass opens the terminal from ⋯ and the work line, as iOS (#4339 C7); Brief and Board keep their header.
        val headerTerminal = onTerminal ?: onTerminalHelp
        if (headerTerminal != null && ChatTerminalPlaces.inHeader(PersonaTheme.style))
            JIconButton(PersonaIcons.terminal, stringResource(if (onTerminal != null) R.string.chat_screen_open_terminal else R.string.chat_screen_terminal_help),
                headerTerminal)
        // Side sessions (#7944): how many, with the needs-you dot while one waits for the user; never opens by itself.
        val sidesShown = sideRollup.count > 0 && onOpenSide != null
        if (sidesShown) JIconButton(PersonaIcons.sideSessions, sideSessionsLabel(sideRollup), { dialog = "sides" },
            badge = sideRollup.needsYou, count = sideRollup.count)
        actions()
        // The permission mode, always in view and changed here (#3957, iOS #3719).
        // Glass has the mode in the composer, by the model pill (Q11, #5055); Brief and Board keep it here.
        if (session != null && SessionModes.showsInHeader(session) && PersonaTheme.style != AppStyle.Glass)
            HeaderModeButton(session, state, online && !readOnly) { modeOverlay = it }
        JIconButton(PhIcons.DotsThree, stringResource(R.string.chat_chrome_more), { menu = true })
    }
    val resumeCopied = stringResource(R.string.chat_chrome_resume_copied)
    if (menu) ChatActionsSheet(title = stringResource(R.string.chat_chrome_actions_title), onDismiss = { menu = false }, actions = buildList {
        session?.todo?.let { todo ->
            add(JAction(stringResource(R.string.chat_chrome_todo, todo.title), PhIcons.ListChecks) { onOpenTodo?.invoke(todo.id) })
        }
        // iOS's first item after the to-do: the terminal comes up over the chat.
        onTerminal?.let { add(JAction(stringResource(R.string.chat_chrome_terminal), PersonaIcons.terminal, onClick = it)) }
        // Resume in Terminal (#5565): offered for a stopped Agent; disabled with why where its computer has no terminal engine.
        when (terminalResume(session, machine)) {
            TerminalResume.Available -> onResumeInTerminal?.let {
                add(JAction(stringResource(R.string.chat_chrome_resume_in_terminal), PersonaIcons.run, onClick = it))
            }
            TerminalResume.NoEngine -> add(
                JAction(
                    stringResource(R.string.chat_chrome_resume_in_terminal),
                    PersonaIcons.run,
                    enabled = false,
                    subtitle = stringResource(
                        R.string.chat_chrome_resume_no_engine,
                        machine?.displayName?.takeIf(String::isNotBlank) ?: session?.machineId.orEmpty(),
                    ),
                ) {},
            )
            TerminalResume.Hidden -> Unit
        }
        onSearch?.let { add(JAction(stringResource(R.string.chat_chrome_search), PhIcons.MagnifyingGlass, onClick = it)) }
        onFiles?.let { add(JAction(stringResource(R.string.chat_chrome_files), PhIcons.FolderSimple, onClick = it)) }
        onGit?.let { add(JAction(gitLabel, onClick = it)) }
        onBackgroundTasks?.let { add(JAction(stringResource(R.string.chat_background_title), PersonaIcons.backgroundTasks, onClick = it)) }
        add(JAction(stringResource(R.string.chat_chrome_details), onClick = onDetails))
        add(JAction(stringResource(R.string.chat_chrome_info), PhIcons.Info, enabled = session != null) { dialog = "info" })
        if (session?.assistant == false) {
            val unavailable = stringResource(R.string.chat_agent_share_authority)
            add(JAction(stringResource(R.string.chat_agent_share_title), PhIcons.ShareNetwork, enabled = agentShares != null && online) {
                try { sharing = agentShares?.invoke() } catch (_: Exception) { state.notice = unavailable }
            })
        }
        // A Codex app-server thread the phone may join live (iOS "Join live"); joined, it can answer its approvals.
        session?.takeIf { it.canAttach }?.let { target ->
            add(JAction(stringResource(R.string.chat_chrome_attach), PhIcons.PlugsConnected, enabled = canAct) { state.run(target, ChatAction.Attach) })
        }
        add(JAction(pluralStringResource(R.plurals.chat_chrome_peers, peers.size, peers.size), enabled = session != null) { dialog = "peers" })
        if (sideRollup.count > 0 && onOpenSide != null) add(JAction(stringResource(R.string.chat_side_sessions_count, sideRollup.count),
            PersonaIcons.sideSessions, subtitle = if (sideRollup.needsYou) stringResource(R.string.chat_side_sessions_needs_you) else null) {
            dialog = "sides"
        })
        add(JAction(stringResource(R.string.chat_chrome_switch_session), enabled = session != null) { dialog = "peers" })
        children.forEach { child -> add(JAction(stringResource(R.string.chat_chrome_subagent, child.description?.takeIf(String::isNotBlank) ?: child.type),
            enabled = onOpenSession != null) { onOpenSession?.invoke(SessionKey(session!!.machineId, child.sessionId)) }) }
        if (linkTitle != null) add(JAction(linkTitle) { dialog = "connection" })
        add(JAction(stringResource(R.string.chat_chrome_interrupt), enabled = canAct && session?.status in listOf("working", "blocked") && session?.canInterrupt != false) {
            state.run(session!!, ChatAction.Interrupt)
        })
        add(JAction(stringResource(R.string.chat_chrome_rename), PhIcons.NotePencil, enabled = canRename) {
            title = session!!.title; tabName = session.slotName.orEmpty(); dialog = "rename"
        })
        add(JAction(stringResource(if (state.muted == true) R.string.chat_chrome_unmute else R.string.chat_chrome_mute), enabled = canAct && state.muted != null) {
            state.run(session!!, ChatAction.Mute(state.muted != true))
        })
        // Only a session in a terminal resumes there; a Codex thread by its own thread id.
        session?.resumeCommand()?.let { command ->
            add(JAction(stringResource(R.string.chat_chrome_copy_resume), enabled = !readOnly) {
                copy(command, resumeCopied)
            })
        }
        // Persona's conversations are not agents in a pane; a session without a terminal closes where its node says it can
        // (a runtime's, #6294).
        if (session?.assistant != true && session?.canClose != false)
            add(JAction(stringResource(R.string.chat_chrome_close_agent), destructive = true, enabled = canAct && session?.isLive == true) { dialog = "close" })
        // Only one that is not running (core:contracts Session.canDelete); the same words as the Agents list's.
        if (session?.canDelete == true) add(JAction(stringResource(R.string.chat_chrome_delete_chat), destructive = true, enabled = canAct) { dialog = "delete" })
    })
    sharing?.let { api -> session?.let { AgentShareSheet(it.key, api, onDismiss = { sharing = null }) } }
    val feedback = LocalAppFeedback.current
    LaunchedEffect(state.notice) {
        state.notice?.let { if (feedback?.notice(it) == true && state.pending == null) state.notice = null }
    }
    state.notice?.let { notice ->
        JNotice(notice, actionLabel = stringResource(if (state.pending != null && session != null) R.string.chat_chrome_notice_verify else R.string.chat_chrome_notice_dismiss), onAction = {
            if (state.pending != null && session != null) {
                if (!state.busy && online) state.run(session, state.pending!!, verify = true)
            } else state.notice = null
        })
    }
    when (dialog) {
        "rename" -> JDialog(onDismiss = { dialog = null }, title = stringResource(R.string.chat_chrome_rename),
            text = chatRenameNote(session)?.let { stringResource(it) },
            confirm = stringResource(R.string.chat_chrome_save) to { dialog = null; state.rename(session!!, title, tabName) },
            confirmEnabled = canRename && session?.let { SessionRename.changes(it, title, tabName).changed } == true,
            content = {
                Column(verticalArrangement = Arrangement.spacedBy(Tokens.Space.m)) {
                    if (!session?.paneId.isNullOrBlank()) JTextField(tabName, { tabName = it }, singleLine = true,
                        label = stringResource(when {
                            session?.isTern == true -> R.string.chat_chrome_tab_name_tern
                            session?.isRex == true -> R.string.chat_chrome_tab_name_rex
                            else -> R.string.chat_chrome_tab_name
                        }))
                    JTextField(title, { title = it }, label = stringResource(R.string.chat_chrome_session_name), singleLine = true)
                }
            })
        // The same words as the Agents list's long press and swipe (core:domain closeConfirmation).
        // Keep or remove the worktree Persona made for it, when it worked in one (#4532); removed once it is gone.
        "close", "delete" -> session?.let { target ->
            val closing = dialog == "close"
            val confirmation = if (closing) target.closeConfirmation(machine?.displayName) else target.deleteConfirmation(machine?.displayName)
            val service = remember(worktrees) { worktrees?.invoke() }
            val rawMachine = persona.core.contracts.GroupKeys.raw(target.machineId)
            val worktree = rememberOurWorktree(service, rawMachine, target.cwd, machine?.online == true, target.worktreeKind)
            var cleanup by remember(target.key) { mutableStateOf(WorktreeCleanup()) }
            val res = androidx.compose.ui.platform.LocalResources.current
            JDialog(onDismiss = { dialog = null }, title = confirmation.title.text(), text = confirmation.message.text(),
                confirm = stringResource(if (closing) R.string.chat_chrome_close_agent else R.string.chat_chrome_delete) to {
                    dialog = null
                    state.run(target, if (closing) ChatAction.Close else ChatAction.Delete) { done -> if (done && service != null && worktree != null)
                        WorktreeCleanups.cleanUp(service, rawMachine, worktree, cleanup, feedback, res, target.worktreeKind) }
                }, destructive = true, confirmEnabled = canAct && WorktreeCleanups.ready(worktree, cleanup),
                content = worktree?.let { st -> { WorktreeCleanupSection(st, cleanup, { cleanup = it }, machine?.home) } })
        }
        "info" -> session?.let { target -> ChatSessionInfoSheet(target, machine, state, onOpenTodo = onOpenTodo, onDismiss = { dialog = null }) }
        "peers" -> JSheet(onDismiss = { dialog = null }, title = stringResource(R.string.chat_chrome_peers_title)) {
            if (peers.isEmpty()) JEmptyState(stringResource(R.string.chat_chrome_peers_empty_title), stringResource(R.string.chat_chrome_peers_empty_text))
            LazyColumn(Modifier.fillMaxWidth().heightIn(max = 400.dp), contentPadding = PaddingValues(bottom = 16.dp.dense)) {
                items(peers, key = { it.key.path }) { peer ->
                    val place = places[peer.key]
                    val where = peer.slotName ?: peer.agentLabel
                    // the launcher says so; the panes it split off sit one step in, as on the Agents list (#2515)
                    JRow(peer.displayTitle,
                        modifier = if (place == CompanionPlace.Branch) Modifier.padding(start = Tokens.Space.l) else Modifier,
                        subtitle = if (place == CompanionPlace.Launcher) stringResource(R.string.chat_tabs_launcher_subtitle, where) else where,
                        leading = { JStatusDot(Status.entries.firstOrNull { it.name == peer.agentState.name } ?: Status.Unknown) },
                        enabled = onOpenSession != null, onClick = { dialog = null; onOpenSession?.invoke(peer.key) })
                }
            }
        }
        "sides" -> ChatSideSessionsSheet(sides, sessions, sideRollup, onDismiss = { dialog = null }) { side ->
            dialog = null; onOpenSide?.invoke(side)
        }
        "connection" -> JDialog(onDismiss = { dialog = null }, title = linkTitle ?: stringResource(R.string.chat_chrome_connected), text = stringResource(when (connection) {
            ConnectionState.CertificateMismatch -> R.string.chat_chrome_conn_cert
            ConnectionState.Revoked -> R.string.chat_chrome_conn_revoked
            ConnectionState.NeedsLocalNetworkPermission -> R.string.chat_chrome_conn_local_network
            else -> R.string.chat_chrome_conn_other
        }), confirm = stringResource(if (onConnectionDetails != null) R.string.chat_chrome_conn_details else R.string.chat_chrome_got_it) to { dialog = null; onConnectionDetails?.invoke() }, dismiss = null)
    }
}

/**
 * The rename dialog's note: what else the new name renames, the terminal's tab and Claude's /rename (as the Agents
 * list's `renameNote`). None for a session without a terminal (a runtime's, #6294), which has neither.
 */
@StringRes
internal fun chatRenameNote(session: Session?): Int? = when {
    session?.canTerminal == false -> null
    session?.isTern == true -> R.string.chat_chrome_rename_text_tern
    session?.isRex == true -> R.string.chat_chrome_rename_text_rex
    else -> R.string.chat_chrome_rename_text
}

/** A scrollable sheet until JActionSheet supports long action lists. */
@Composable
internal fun ChatActionsSheet(actions: List<JAction>, onDismiss: () -> Unit, title: String) {
    // Actions without an icon keep the icon's place (its 22dp and the 16dp after it, less the 14dp JRow puts after a
    // leading slot), so every label starts in one column.
    val iconColumn = actions.any { it.icon != null }
    JSheet(onDismiss = onDismiss, title = title) {
        LazyColumn(Modifier.fillMaxWidth(), contentPadding = PaddingValues(bottom = 16.dp.dense)) {
            items(actions) { action ->
                JRow(action.label, leadingIcon = action.icon, enabled = action.enabled, destructive = action.destructive,
                    subtitle = action.subtitle, titleLines = 2,
                    leading = if (action.icon == null && iconColumn) ({ Spacer(Modifier.width(24.dp.dense)) }) else null,
                    chevron = false, onClick = if (action.enabled) ({ onDismiss(); action.onClick() }) else null)
            }
        }
    }
}

/**
 * The state the header shows: the status line's (an ended chat, an offline machine, a request), else the session's. The status
 * line's own "Waiting" (a loop or background work on a chat that is not working) is not a state of the session: the header says
 * the session's own, as iOS does (`stateWords`, `StatusDot`), so a seen chat with a loop says nothing in words and has Seen's dot,
 * and a finished one says Unread; the loop and the background work have their pill (Glass) and their chips above the composer.
 * A chat the hub counts as working keeps it: its words are [stateLabel]'s.
 */
internal fun headerStatus(state: StatusLineState, session: Session?): Status {
    val own = Status.entries.firstOrNull { it.name == session?.agentState?.name } ?: Status.Unknown
    val line = resolveStatusLine(state) ?: return own
    return if (line.kind == StatusLineKind.Waiting && own != Status.Working) own else line.status
}

/**
 * The slim bar an Agent chat's header folds into on Glass (iOS `compactChrome`, #4339 C4): back, the agent's mark, the
 * state's dot and the title, or "Connecting…" while the hub is out of reach; a tap opens the header again.
 * The compact phone layout keeps the open header's session actions on the right.
 */
@Composable
internal fun ChatSlimHeader(
    session: Session?, statusState: StatusLineState, connection: ConnectionState?, onBack: () -> Unit,
    onMore: (() -> Unit)? = null, onOpen: () -> Unit,
) {
    val title = session?.displayTitle ?: stringResource(R.string.chat_chrome_title_default)
    val link = connectionTitle(connection)
    val haptics = rememberHaptics()
    ChatSlimBar(title, stringResource(R.string.chat_chrome_slim_open, link ?: title), onOpen = { haptics(Haptic.Tap); onOpen() },
        onBack = onBack, status = headerStatus(statusState, session), agentMark = session?.agentMark, linkText = link,
        waiting = session?.let(::AgentSession)?.waitsInBackground == true,
        trailing = onMore?.let { open -> {
            JIconButton(PersonaIcons.moreMenu, stringResource(R.string.chat_chrome_more), open, raised = false, tint = PersonaTheme.palette.accent)
        } })
}

/** The states the line under the title names by its dot only (iOS `ChatView.quiet`). */
internal val QuietStatuses = setOf(Status.Idle, Status.Offline, Status.Unknown)

/**
 * Where a conversation runs, as the line under its title says it (iOS `devSubtitle`, #4339 C4): its [source] first (a
 * Codex app-server, Codex history or an agent runtime, `sourceSubtitle` in words), then the computer and "space › tab".
 * Never the working directory, which DESIGN_SYSTEM.md 5.6 keeps off primary screens; null when there is nothing to say.
 */
internal fun chatPlace(session: Session, machine: String?, source: String? = null, short: Boolean = false): String? {
    val place = listOfNotNull(session.spaceName?.takeIf(String::isNotBlank), session.slotLabel).joinToString(" › ")
    // [short], for a line too narrow for all of it: the place without the computer, or the computer when there is no place.
    val computer = machine?.takeIf { it.isNotBlank() && (!short || place.isEmpty()) }
    return listOfNotNull(source?.takeIf(String::isNotBlank), computer, place.takeIf(String::isNotEmpty))
        .joinToString(" · ").takeIf(String::isNotEmpty)
}

/** Its turn has ended and a workflow or background work keeps it going, or all it runs only waits (iOS `stateWords`' first rule). */
internal val AgentSession.runsInBackground: Boolean get() = isBackgroundWorking || waitsInBackground

/**
 * Where "Working 22 min" counts from (iOS `ChatView.stateWords`): the start of the turn that runs now ([turnStart]), only while
 * the agent works, its turn is live ([isLiveStatus]) and it is not in the background ([runsInBackground]); else null, and the
 * header says the state's name alone.
 */
internal fun workedSince(agent: AgentSession?, turnStart: java.time.Instant?): java.time.Instant? =
    turnStart.takeIf { agent != null && agent.state == AgentState.Working && isLiveStatus(agent.session.status) && !agent.runsInBackground }

/** Whole minutes from [since] to [now], null under one minute or without a start. */
internal fun workedMinutes(since: java.time.Instant?, now: java.time.Instant): Long? =
    since?.let { java.time.Duration.between(it, now).toMinutes() }?.takeIf { it >= 1 }
