package persona.feature.machines

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import persona.core.contracts.*
import persona.core.contracts.api.HubManager
import persona.core.contracts.api.ManagerRoster
import persona.core.designsystem.*
import persona.core.designsystem.dense
import persona.core.designsystem.icons.PhIcons
import persona.core.navigation.*

@Composable
internal fun machineSummaryText(summary: MachineSummary): String {
    val attention = pluralStringResource(R.plurals.machines_attention, summary.attention, summary.attention)
    val agents = pluralStringResource(R.plurals.machines_agents_needs_you, summary.needsYou, summary.needsYou)
    return stringResource(R.string.machines_summary_with_agents, summary.online, summary.total, attention, agents)
}

@Composable
internal fun MachinesScreen(
    machineId: String?, model: MachinesViewModel, navigator: Navigator, actions: MachineActionsViewModel? = null, launcherPages: Boolean = false,
    /** What extensions draw on this machine's page, after its own sections (docs/extensions/extensions.md "UI slots"). */
    extensionSections: (androidx.compose.foundation.lazy.LazyListScope.() -> Unit)? = null,
    /** "What this computer can do", "Browser" and "Subscription limits"; null in demo mode. */
    health: MachineHealthViewModel? = null,
    /** The phone adds and confirms computers ("Add a computer"); false in demo mode. */
    adds: Boolean = false,
    /** The hub adds computers from the phone (machines_add): the + and "Add a computer" open its page, else the help dialog. */
    canAdd: Boolean = false,
) {
    val state by model.state.collectAsStateWithLifecycle()
    val roster = LocalManagerRoster.current
    val managerGroup = LocalManagerGroup.current
    var choosingManager by remember(machineId) { mutableStateOf(false) }
    var renaming by remember(machineId) { mutableStateOf(false) }
    // The hub renames computers (feature machine_rename); a linked hub's computer is renamed on that hub.
    val renames = MachineRenameFeature in LocalHubFeatures.current
    val allMachines by model.sessions.machines.collectAsStateWithLifecycle()
    val sessions by model.sessions.sessions.collectAsStateWithLifecycle()
    // The list is the current assistant's computers (docs/assistant/managers.md "Clients"); one computer's page is
    // that computer's, whichever assistant it belongs to (a link or a notification opens it as it is).
    val assistant = currentAssistant()
    val machines = remember(allMachines, assistant, machineId) {
        if (machineId != null) allMachines else allMachines.filter { assistant.ownsOn(it.id, it.manager) }
    }
    val machine = machines.firstOrNull { it.id == machineId }
    val acting = actions?.state?.collectAsStateWithLifecycle()?.value ?: MachineActionsState()
    val linkAttempt = machineId?.let { acting.linkSwitches[it] }
    val linkSwitchBusy = linkSwitchConnectionBusy(machine?.linkSwitch, linkAttempt)
    if (actions != null && machineId != null) LaunchedEffect(actions, machineId) { actions.observeLinkSwitch(machineId) }
    // An offline machine has no launchers; they are read again when it comes online (after "Reconnect").
    if (actions != null && machineId != null) LaunchedEffect(actions, machineId, machine?.online) {
        actions.loadLaunchers(machineId, machine?.online == true)
    }
    // "Also show botmux sessions": read once per machine page; an older hub leaves it unanswered and the page draws no switch.
    if (actions != null && machineId != null) LaunchedEffect(actions, machineId) { actions.loadBotmux(machineId) }
    val deployLine: @Composable (Machine?) -> Unit = { info ->
        if (info != null && showsDeploy(info)) MachineDeployLine(info, info.id in acting.deployStarting, actions?.let { { it.deploy(info.id) } })
    }
    var selection by rememberSaveable(machineId) { mutableStateOf<Long?>(null) }
    val activeSelection = retainedMetricSelection(selection, state.now, state.range.seconds)
    LaunchedEffect(selection, activeSelection) {
        if (selection != activeSelection) selection = activeSelection
    }
    // The dashboard's are every manager's: only the ones of a listed computer, or of one the list doesn't know at all.
    val known = remember(allMachines) { allMachines.mapTo(HashSet()) { it.id } }
    val listed = remember(machines) { machines.mapTo(HashSet()) { it.id } }
    val metrics = state.dashboard?.machines.orEmpty().filter { it.machineId in listed || it.machineId !in known }
        .associate { it.machineId to it.live(machines.firstOrNull { m -> m.id == it.machineId }) }
    val baseIds = machineListIds(machines, metrics)
    val entries = remember(baseIds, machines, metrics, sessions, state.now) {
        machineListEntries(baseIds, machines, metrics, sessions, state.now) { model.sessions.supportsFeature(it, "sessions_hierarchy") }
    }
    val ids = entries.map { it.id }
    val listSummary = machineSummary(entries)
    val locale = currentLocale
    val healthState = health?.state?.collectAsStateWithLifecycle()?.value ?: MachineHealthState()
    val subscriptionUsage = LocalSidebarUsage.current?.value ?: healthState.usage
    var allChecks by rememberSaveable(machineId) { mutableStateOf(false) }
    var allAgents by rememberSaveable(machineId) { mutableStateOf(false) }
    var confirmFix by remember { mutableStateOf(false) }
    if (health != null && machineId != null) {
        LaunchedEffect(health, machineId, machine?.online) { health.open(machineId, machine?.online == true) }
        LaunchedEffect(health, machine?.browser) { health.browserFromList(machine?.browser) }
        val enabled = (healthState.browser ?: machine?.browser)?.enabled == true
        // ego lite is asked again when the page opens, and every few seconds while a job runs or its setup is under way.
        LaunchedEffect(health, machineId, machine?.online, enabled) { if (machine?.online == true && enabled) health.refreshBrowser(machineId) }
        val watching = health.watchesBrowser(machine?.online == true)
        // Only while the page is shown: a hidden tab, the app in the background or the screen off asks the hub nothing. Back in front it
        // asks at once (a job may have ended meanwhile); the first start doesn't, the page-open read or the answer that started the job is fresh.
        val lifecycle = LocalLifecycleOwner.current.lifecycle
        LaunchedEffect(health, machineId, watching, lifecycle) {
            if (!watching) return@LaunchedEffect
            var stale = !lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)
            lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
                val readFirst = stale
                stale = true
                health.watchBrowser(machineId, readFirst) { health.watchesBrowser(machine?.online == true) }
            }
        }
    }
    if (confirmFix && health != null && machineId != null) FixDialog(healthState, { confirmFix = false }) { health.fix(machineId) }
    var addComputerHelp by rememberSaveable { mutableStateOf(false) }
    if (addComputerHelp) ReadinessHelpDialog(ReadinessHelp.AddComputer, null, onDismiss = { addComputerHelp = false }, onAction = {})
    val addComputer = { if (adds && canAdd) navigator.navigate(AddComputerRoute()) else addComputerHelp = true }
    val confirmComputer: (String) -> Unit = { id -> navigator.navigate(AddComputerRoute(confirm = id)) }
    if (renaming && actions != null && machine != null) RenameMachineDialog(machine, onRename = { actions.rename(machine.id, it) }, onDismiss = { renaming = false })
    if (choosingManager && actions != null && machineId != null) roster?.let { r ->
        val current = r.ownerOf(machines.firstOrNull { it.id == machineId }?.manager)
        MoveToManagerSheet(r, current, onMove = { m -> actions.moveTo(machineId, m.id, m.shownName) }, onDismiss = { choosingManager = false })
    }
    MachineGridWidth { contentWidth ->
    if (machineId == null) AdaptiveMachineGridProbe(stringResource(R.string.machines_title), machineGridColumns(contentWidth))
    val listState = rememberMachineGridListState(ids, machineGridColumns(contentWidth))
    val chargingLabel = stringResource(R.string.machines_metric_charging)
    JListPage(
        title = if (machineId == null) stringResource(R.string.machines_title) else machine?.displayName ?: stringResource(R.string.machines_title_detail),
        summary = if (machineId == null) machineSummaryText(listSummary) else null,
        onBack = if (LocalIsSidebarRoot.current) null else navigator::back,
        largeTitle = machineId == null,
        state = listState,
        actions = {
            if (machineId == null && adds && canAdd) JIconButton(PhIcons.Plus, stringResource(ReadinessActionId.AddComputer.label), addComputer)
            JIconButton(PhIcons.ArrowClockwise, stringResource(R.string.machines_refresh), model::refresh)
        },
    ) {
        if (state.loading) item("loading") { JLinearProgress(null, Modifier.fillMaxWidth()) }
        state.failure?.let { failure -> item("failure") { JNotice(stringResource(failure.text)) } }
        if (state.sessionsFailure) item("sessions-failure") { JNotice(stringResource(R.string.machines_sessions_failure)) }
        if (machineId == null) {
            usageWindows(subscriptionUsage, java.time.Instant.ofEpochSecond(state.now))
            // An offline assistant with no computer the hub still keeps: out of reach, not a first run (no Add).
            if (!state.loading && ids.isEmpty() && assistant.offline != null) item("offline") { JAssistantOffline(assistant) }
            else if (!state.loading && ids.isEmpty()) item("empty") {
                JEmptyState(stringResource(if (state.failure == null) R.string.machines_empty_title else R.string.machines_empty_unavailable),
                    stringResource(R.string.machines_empty_body), icon = PhIcons.Desktop,
                    // How to add one (docs/assistant/onboarding.md 4.5), until the phone adds computers itself.
                    action = if (state.failure == null) ({
                        JButton(stringResource(ReadinessActionId.AddComputer.label), addComputer, kind = JButtonKind.Secondary)
                    }) else null)
            }
            val entriesById = entries.associateBy { it.id }
            machineGrid(ids, contentWidth) { id, cardModifier ->
                val entry = entriesById.getValue(id)
                MachineCard(entry.machine, entry.metrics, entry.tally, state.now, state.dashboard?.step ?: 60,
                    modifier = cardModifier, fillsRow = true, deploy = deployLine,
                    onConfirm = if (adds) confirmComputer else null) {
                    navigator.navigate(MachineDetailRoute(id))
                }
            }
        } else {
            val metric = state.history?.metrics?.live(machine) ?: MachineMetrics(machineId, machine?.online ?: false, machine?.system)
            val system = metric.system ?: machine?.system
            val current = metric.current(state.now)
            val agents = machineSessions(sessions, machineId, model.sessions.supportsFeature(machineId, "sessions_hierarchy"))
            item("machine") {
                MachineCard(machine, metric, AgentTally(agents), state.now, state.history?.step ?: state.range.step, deploy = deployLine,
                    onConfirm = if (adds) confirmComputer else null, linkSwitchExpected = linkSwitchBusy)
            }
            if (machine != null && actions != null) item("link-switch") {
                MachineLinkSwitchSection(machine, linkAttempt,
                    onStart = { actions.startLinkSwitch(machine.id) }, onCheck = { actions.checkLinkSwitch(machine.id) })
            }
            if (machine == null && machines.isNotEmpty()) item("gone") { JNotice(stringResource(R.string.machines_gone)) }
            // Move to… with several managers, for this hub's own computers (a linked hub's is that hub's to give).
            val owner = machine?.takeIf { roster?.isMulti == true && movable(it, managerGroup) }?.let { roster?.ownerOf(it.manager) }
            if (machine != null && actions != null) machineControls(machine, acting.reconnecting,
                // Reconnect goes through its manager: not while that one is offline
                canReconnect = offlineManagerOf(roster, machine, managerGroup) == null && !linkSwitchBusy,
                onEngine = { navigator.navigate(EngineRoute(machine.id)) }, onReconnect = { actions.reconnect(machine.id) },
                botmux = acting.botmux, botmuxSaving = acting.botmuxSaving, onBotmux = { actions.setBotmux(machine.id, it) },
                move = owner?.let { id -> MachineMoveRow(roster?.manager(id)?.shownName ?: id, acting.moving) { choosingManager = true } },
                rename = MachineRenameRow(acting.renaming) { renaming = true }.takeIf { renames && machine.kind != "hub" })
            item("sessions-title") { JSectionHeader(stringResource(R.string.machines_sessions_title), count = agents.size) }
            if (agents.isEmpty()) item("no-sessions") { JNotice(stringResource(R.string.machines_no_sessions)) }
            val shownAgents = visibleMachineAgents(agents, allAgents)
            items(shownAgents, key = { "session-${it.sessionId}" }, contentType = { "session" }) { session ->
                val status = when (session.agentState) {
                    AgentState.NeedsYou -> Status.NeedsYou; AgentState.Working -> Status.Working
                    AgentState.Unread -> Status.Unread; AgentState.Idle -> Status.Idle
                    AgentState.Offline -> Status.Offline; AgentState.Unknown -> Status.Unknown
                }
                JCard(Modifier.padding(bottom = Tokens.Space.m), onClick = { navigator.navigate(ChatRoute(machineId, session.sessionId)) }, status = status) {
                    JRow(session.displayTitle, leading = { JStatusDot(status) }, trailing = { JTag(status.localizedLabel, status = status) })
                    Text(session.agentLabel, style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink2)
                    Text(session.cwd, style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink3, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
            }
            if (agents.size > MACHINE_AGENTS_PREVIEW) item("sessions-toggle") {
                val toggleText = if (allAgents) stringResource(R.string.machines_agents_show_less)
                else stringResource(R.string.machines_agents_show_all, agents.size)
                JTextButton(toggleText, { allAgents = !allAgents }, Modifier.fillMaxWidth())
            }
            // As iOS: what the computer can do and its browser come before the monitoring.
            if (machine != null && health != null) {
                machineHealth(machine, healthState, allChecks, { allChecks = !allChecks }, { health.check(machine.id) }, { confirmFix = true },
                    onDeploy = actions?.let { { it.deploy(machine.id) } })
                machineBrowser(machine, healthState, { health.setBrowser(machine.id, it) }, { health.browserJob(machine.id, it) },
                    { health.recheckBrowser(machine.id) })
            }
            item("now") {
                JSectionHeader(stringResource(R.string.machines_now), modifier = Modifier.testTag("machine-now"),
                    trailing = uptimeText(system?.bootTime, state.now)?.takeIf { metric.online }?.let { stringResource(R.string.machines_up_for, it) })
            }
            // Keyed by the label's resource, so the list keeps its rows when the language changes.
            val swapOff = current?.swapTotal == 0L
            val chargingSuffix = if (current?.batteryCharging == true) " · $chargingLabel" else ""
            val values = buildList {
                add(R.string.machines_metric_cpu to MetricFormat.percent(current?.cpu, locale))
                add(R.string.machines_metric_memory to "${MetricFormat.bytes(current?.memUsed, locale)} / ${MetricFormat.bytes(current?.memTotal, locale)}")
                add(R.string.machines_metric_network to "↓ ${MetricFormat.rate(current?.netRx, locale)}  ↑ ${MetricFormat.rate(current?.netTx, locale)}")
                add(R.string.machines_metric_load to MetricFormat.number(current?.load, locale))
                add(R.string.machines_metric_disk to "${MetricFormat.bytes(current?.diskUsed, locale)} / ${MetricFormat.bytes(current?.diskTotal, locale)}")
                add(R.string.machines_metric_swap to if (swapOff) null else "${MetricFormat.bytes(current?.swapUsed, locale)} / ${MetricFormat.bytes(current?.swapTotal, locale)}")
                current?.batteryPercent?.takeIf { it.isFinite() }?.let { pct ->
                    val label = MetricFormat.percent(pct, locale) + chargingSuffix
                    add(R.string.machines_metric_battery to label)
                }
                current?.tempC?.takeIf { it.isFinite() }?.let { temp ->
                    add(R.string.machines_metric_temperature to "${MetricFormat.number(temp, locale)}°C")
                }
            }
            itemsIndexed(values, key = { _, it -> "now-${it.first}" }, contentType = { _, _ -> "value" }) { index, value ->
                JGroupItem(GroupPosition.of(index, values.size)) {
                    JRow(stringResource(value.first), value = value.second ?: stringResource(R.string.machines_swap_off))
                }
            }
            if (current == null) item("no-current") { JNotice(stringResource(if (metric.online) R.string.machines_no_sample else R.string.machines_offline_history)) }
            item("range") {
                JSectionHeader(stringResource(R.string.machines_history))
                JSegmented(MetricRange.entries.map { JSegment(stringResource(R.string.machines_range_hours, when (it) {
                    MetricRange.Hour -> 1; MetricRange.SixHours -> 6; MetricRange.Day -> 24
                })) }, MetricRange.entries.indexOf(state.range), { model.selectRange(MetricRange.entries[it]) })
                Text(stringResource(R.string.machines_long_press_hint), Modifier.padding(vertical = 8.dp.dense), style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink3)
            }
            if (activeSelection != null) item("selection") { JTextButton(stringResource(R.string.machines_back_to_now), { selection = null }) }
            // Disk history is in bytes; the chart reads it against the disk's size, so only when that is known.
            val diskTotal = (current?.diskTotal ?: metric.latest?.diskTotal)?.takeIf { it > 0 }
            val chartKinds = listOf(listOf(MetricKind.Cpu), listOf(MetricKind.Memory), listOf(MetricKind.Rx, MetricKind.Tx), listOf(MetricKind.Load)) +
                listOfNotNull(listOf(MetricKind.Disk).takeIf { diskTotal != null })
            items(chartKinds, key = { "chart-${it.first()}" }, contentType = { "chart" }) { kinds ->
                val disk = kinds.first() == MetricKind.Disk
                MetricChart(when {
                    kinds.size == 2 -> stringResource(R.string.machines_metric_network)
                    disk -> stringResource(R.string.machines_disk_total, MetricFormat.bytes(diskTotal, locale))
                    else -> stringResource(kinds.first().label)
                },
                    if (disk) diskPercentPoints(metric.points, diskTotal) else metric.points, kinds,
                    kinds.map { kind -> when (kind) {
                        MetricKind.Cpu -> current?.cpu; MetricKind.Memory -> current?.memUsed?.toDouble()
                        MetricKind.Rx -> current?.netRx; MetricKind.Tx -> current?.netTx; MetricKind.Load -> current?.load
                        MetricKind.Disk -> percent(current?.diskUsed, current?.diskTotal)
                    } }, state.now, state.range.seconds, state.history?.step ?: state.range.step, activeSelection, { selection = it },
                    reference = system?.cores?.takeIf { kinds.first() == MetricKind.Load && it > 0 }?.toDouble())
            }
            if (system?.os == "linux" && !current?.cores.isNullOrEmpty()) item("cores") { CoreBars(current.cores!!) }
            if (machine != null && actions != null) machineLaunchers(machine.online, acting,
                onLauncher = if (launcherPages) ({ name -> navigator.navigate(LauncherRoute(machine.id, name)) }) else null)
            item("system") { JSectionHeader(stringResource(R.string.machines_system_title)) }
            // Labels are resources (keys that stay put across languages) or, for product names, the name itself; a value
            // is text or a resource.
            val connection = when (machine?.kind) { "local" -> R.string.machines_connection_local; "ssh" -> R.string.machines_connection_ssh; else -> null }
            val systemValues: List<Pair<Any, Any?>> = listOf(
                R.string.machines_system_os to system?.name, R.string.machines_system_version to system?.version,
                R.string.machines_system_kernel to system?.kernel, R.string.machines_system_arch to system?.arch,
                R.string.machines_system_model to system?.model, R.string.machines_system_cpu to system?.cpu,
                R.string.machines_system_cores to system?.cores?.takeIf { it > 0 }?.let { LocalizedFormats.number(it, locale) },
                R.string.machines_metric_memory to MetricFormat.bytes(system?.memory?.takeIf { it > 0 }, locale),
                R.string.machines_system_connection to connection,
                R.string.machines_system_boot to MetricFormat.boot(system?.bootTime, locale),
                "herdr" to machine?.herdrVersion, "Claude Code" to machine?.claudeVersion, "Persona node" to machine?.nodeVersion,
            )
            itemsIndexed(systemValues, key = { _, it -> "system-${it.first}" }, contentType = { _, _ -> "value" }) { index, value ->
                val label = (value.first as? Int)?.let { stringResource(it) } ?: value.first.toString()
                val text = when (val v = value.second) { is Int -> stringResource(v); else -> v?.toString() }
                JGroupItem(GroupPosition.of(index, systemValues.size)) { JRow(label, value = text?.takeIf { it.isNotBlank() } ?: "—") }
            }
            item("method") { Text(stringResource(R.string.machines_method),
                Modifier.padding(vertical = 12.dp.dense), style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink3) }
            extensionSections?.invoke(this)
        }
    }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
internal fun MachineCard(
    machine: Machine?, metrics: MachineMetrics, tally: AgentTally, now: Long, step: Int,
    modifier: Modifier = Modifier,
    fillsRow: Boolean = false,
    deploy: (@Composable (Machine?) -> Unit)? = null,
    /** "Confirm computer" for a computer that waits to be confirmed; null where the phone cannot (demo mode). */
    onConfirm: ((String) -> Unit)? = null,
    /** A confirmed or listed Link switch makes this computer's temporary offline state expected. */
    linkSwitchExpected: Boolean = false,
    onClick: (() -> Unit)? = null,
) {
    val current = metrics.current(now)
    val system = metrics.system ?: machine?.system
    val alerts = alerts(current, system?.cores)
    val locale = currentLocale
    val pending = machine?.pending == true
    val switching = linkSwitchExpected || linkSwitchRunning(machine?.linkSwitch)
    val concern = machineConcern(machine, metrics.online, alerts, tally.needsYou)
    val status = when (concern) {
        MachineConcern.Pending, MachineConcern.Critical, MachineConcern.Warning,
        MachineConcern.NeedsYou, MachineConcern.Mismatch -> Status.NeedsYou
        MachineConcern.Offline -> Status.Offline
        MachineConcern.None -> Status.Unread
    }
    JCard(modifier.padding(bottom = Tokens.Space.m), onClick = onClick, status = status) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp.dense)) {
            OsBadge(system)
            Column(Modifier.weight(1f)) {
                Text(machine?.displayName ?: metrics.machineId, style = PersonaTheme.type.title, color = PersonaTheme.palette.ink, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(listOfNotNull(system?.name?.takeIf { it.isNotBlank() }, system?.cores?.takeIf { it > 0 }?.let { pluralStringResource(R.plurals.machines_cores, it, it) },
                    system?.memory?.takeIf { it > 0 }?.let { MetricFormat.bytes(it, locale) }).joinToString("  "), style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink2)
            }
            JTag(stringResource(when {
                pending -> R.string.machines_onb_status_pending
                status == Status.Offline -> R.string.machines_status_offline; status == Status.NeedsYou -> R.string.machines_status_attention
                else -> R.string.machines_status_online
            }), status = status)
        }
        machine?.hostname?.takeIf { it.isNotBlank() }?.let { Text(it, style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink3) }
        if (pending) {
            // The hub's error says "confirm it in a newer app"; this is that app.
            Text(stringResource(if (metrics.online) R.string.machines_onb_pending_online else R.string.machines_onb_pending_offline),
                style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink2)
            if (onConfirm != null && machine != null) JButton(stringResource(R.string.machines_onb_confirm_action), { onConfirm(machine.id) },
                Modifier.padding(top = Tokens.Space.s), kind = JButtonKind.Secondary, size = JButtonSize.Small)
        } else if (!metrics.online) {
            Text(stringResource(R.string.machines_last_seen, MetricFormat.lastSeen(machine?.lastSeen, locale) ?: stringResource(R.string.machines_unknown)), style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink2)
            // Its manager is offline (several managers): that is why, in our words, not the hub's for an unreachable assistant.
            val goneWith = offlineManagerOf(LocalManagerRoster.current, machine, LocalManagerGroup.current)
            if (goneWith != null) {
                Text(stringResource(R.string.machines_manager_offline, goneWith.shownName), style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink2)
            } else if (!switching) {
                // A known error_code in plain words; else why and what to do as read from the hub's error, its own words
                // after in a quieter colour; else the hub's error alone (#4463).
                val coded = machineErrorText(machine?.errorCode)
                val plain = if (coded == null && machine != null) connectionFailureText(machine) else null
                (coded ?: plain)?.let { Text(stringResource(it), style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink2) }
                // A computer being redeployed drops on purpose: its error is not news (the deploy line says what is going on).
                if (coded == null && machine?.let(::deploying) != true) machine?.error?.takeIf { it.isNotBlank() }?.let {
                    Text(it, style = PersonaTheme.type.caption, color = if (plain != null) PersonaTheme.palette.ink3 else PersonaTheme.palette.ink2)
                }
            }
        } else {
            Text(stringResource(R.string.machines_card_sessions, tally.total, tally.working, tally.needsYou),
                style = PersonaTheme.type.caption, color = PersonaTheme.palette.ink2)
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp.dense)) {
            listOfNotNull(machine?.herdrVersion?.let { "herdr $it" }, machine?.claudeVersion?.let { "Claude $it" }).forEach {
                Text(it, style = PersonaTheme.type.micro, color = PersonaTheme.palette.ink3)
            }
        }
        if (deploy != null && showsDeploy(machine)) deploy(machine)
        else if (machine?.nodeMismatch == true) Warning(stringResource(R.string.machines_node_mismatch))
        alerts.forEach { Warning(stringResource(it.label)) }
        if (fillsRow) Spacer(Modifier.weight(1f))
        val groups = listOf(listOf(MetricKind.Cpu), listOf(MetricKind.Memory), listOf(MetricKind.Rx, MetricKind.Tx))
        groups.forEach { kinds ->
            val value = when (kinds.first()) {
                MetricKind.Cpu -> MetricFormat.percent(current?.cpu, locale)
                MetricKind.Memory -> MetricFormat.percent(percent(current?.memUsed, current?.memTotal), locale)
                else -> "↓ ${MetricFormat.rate(current?.netRx, locale)}  ↑ ${MetricFormat.rate(current?.netTx, locale)}"
            }
            Row(Modifier.fillMaxWidth().padding(top = 12.dp.dense), horizontalArrangement = Arrangement.SpaceBetween) {
                Text(stringResource(if (kinds.size > 1) R.string.machines_metric_network else kinds.first().label), style = PersonaTheme.type.micro, color = PersonaTheme.palette.ink2)
                Text(value, style = PersonaTheme.type.micro, color = PersonaTheme.palette.ink)
            }
            Plot(metrics.points, kinds, now, 3600, step, Modifier.fillMaxWidth().height(32.dp))
        }
    }
}

@Composable
private fun Warning(text: String) {
    Row(Modifier.padding(top = 8.dp.dense), horizontalArrangement = Arrangement.spacedBy(6.dp.dense)) {
        JIcon(PhIcons.Warning, null, size = 18.dp, tint = PersonaTheme.palette.danger.dot)
        Text(text, color = PersonaTheme.palette.danger.text, style = PersonaTheme.type.caption)
    }
}

@Composable
private fun OsBadge(system: MachineSystem?) {
    JIcon(if (system?.os == "darwin" || system?.distro == "macos") PhIcons.Laptop else PhIcons.Desktop,
        system?.name ?: system?.os ?: stringResource(R.string.machines_computer), size = 36.dp, tint = PersonaTheme.palette.ink2)
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun CoreBars(cores: List<Double>) {
    val locale = currentLocale
    JCard(Modifier.padding(bottom = 12.dp.dense)) {
        Text(stringResource(R.string.machines_per_core), style = PersonaTheme.type.title, color = PersonaTheme.palette.ink)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp.dense), verticalArrangement = Arrangement.spacedBy(10.dp.dense)) {
            cores.forEachIndexed { index, usage ->
                Column(Modifier.width(76.dp)) {
                    Text(stringResource(R.string.machines_core_n, index + 1), style = PersonaTheme.type.micro, color = PersonaTheme.palette.ink2)
                    JLinearProgress(if (usage.isFinite()) (usage / 100).toFloat().coerceIn(0f, 1f) else 0f, Modifier.fillMaxWidth())
                    Text(MetricFormat.percent(usage, locale), style = PersonaTheme.type.micro, color = PersonaTheme.palette.ink)
                }
            }
        }
    }
}

/**
 * Whether a computer can be given to another manager here: one of the selected group's ([group]) and not a computer
 * only a linked member hub has (`kind` `hub`): the hub refuses those (`400 not_movable`).
 */
internal fun movable(machine: Machine, group: String?): Boolean =
    machine.kind != "hub" && persona.core.contracts.GroupKeys.namespaceOf(machine.id) == group

/**
 * The manager an offline [machine] is offline with (several managers only): its own, when that one isn't connected. The
 * hub then shows it offline with its words for an unreachable assistant; the card names the manager instead.
 */
internal fun offlineManagerOf(roster: ManagerRoster?, machine: Machine?, group: String?): HubManager? {
    if (roster == null || !roster.isMulti || machine == null || machine.online) return null
    // the roster is the selected group's hub's: another group's computer (its own hub's "assistant") is not its
    if (persona.core.contracts.GroupKeys.namespaceOf(machine.id) != group) return null
    return roster.manager(roster.ownerOf(machine.manager))?.takeIf { !it.online && !it.isInstalling }
}
