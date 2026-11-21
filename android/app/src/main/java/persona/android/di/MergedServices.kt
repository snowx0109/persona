package persona.android.di

import persona.android.AppText
import persona.android.R
import persona.core.contracts.*
import persona.core.contracts.api.*
import persona.core.hub.HubError
import persona.core.hub.HubFailure
import persona.core.hub.HttpTerminalPanes
import persona.feature.agents.AgentsDraftSource
import persona.feature.agents.AgentsOperationsBindings
import persona.feature.agents.AgentsWiringState
import persona.feature.chat.ChatLocalStorage
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.*

/*
 * The services the pages use, over every hub group (docs/android/android.md "Multiple hubs"). Lists are concatenated in the
 * registry's order with each group's machine ids as GroupKeys shows them; a call goes to the group its id names and
 * reaches that group's own services with plain ids. With one group every list is that group's own flow and every
 * call goes straight to it, so a phone paired with a single hub behaves as before groups existed.
 */

private fun <T> GroupStack.mapped(flow: Flow<T>, transform: GroupNames.(T) -> T): Flow<T> =
    if (names.plain) flow else flow.map { names.transform(it) }

internal suspend fun refreshOwnedSessions(machineId: String, call: suspend (namespace: String?, rawMachineId: String) -> Unit) =
    call(GroupKeys.namespaceOf(machineId), GroupKeys.raw(machineId))

internal fun mergedSessions(stacks: GroupStacks): SessionsRepository = object : SessionsRepository, VerifiedSessionsRepository {
    override fun supportsFeature(machineId: String, feature: String) =
        feature in stacks.of(machineId)?.connection?.serverFeatures?.value.orEmpty()
    private val listNamespaces = MutableStateFlow<Set<String?>?>(null)
    private fun selected(stack: GroupStack) = listNamespaces.value?.contains(stack.names.namespace) != false
    override suspend fun setListNamespaces(namespaces: Set<String?>?) { listNamespaces.value = namespaces }
    override val machines = stacks.merged(emptyList<Machine>(), { s ->
        s.mapped(s.repositories.sessions.machines) { list -> list.map(::machine) }
    }) { it.flatten() }
    override val sessions = stacks.merged(emptyList<Session>(), { s ->
        s.mapped(s.repositories.shownSessions) { list -> list.map(::session) }
    }) { it.flatten() }
    override val adoptedSessions = stacks.merged(emptyList<Session>(), { s ->
        s.mapped(s.repositories.sessions.adoptedSessions) { list -> list.map(::session) }
    }) { it.flatten() }
    override val startedThreads = stacks.merged(emptyMap<SessionKey, Session>(), { s ->
        s.mapped(s.repositories.sessions.startedThreads) { entries ->
            entries.map { (previous, thread) -> key(previous) to session(thread) }.toMap()
        }
    }) { maps -> maps.fold(emptyMap()) { acc, one -> acc + one } }
    override val listStatus = stacks.merged(SessionListStatus(), { stack ->
        combine(stack.repositories.sessions.listStatus, listNamespaces) { status, namespaces ->
            if (namespaces?.contains(stack.names.namespace) == false) SessionListStatus(loaded = true)
            else status.copy(
                filter = status.filter.copy(hiddenBotmuxHubs = if (null in status.filter.hiddenBotmuxHubs) setOf(stack.namespace) else emptySet()),
                counts = status.counts.copy(machine = status.counts.machine.mapKeys { (id, _) -> stack.names.machineId(id) }),
                machineCounts = status.machineCounts.copy(machine = status.machineCounts.machine.mapKeys { (id, _) -> stack.names.machineId(id) }),
            )
        }
    }) { statuses ->
        fun totals(map: (SessionListCounts) -> Map<String, Int>, machines: Boolean = false) = statuses.flatMap { map(if (machines) it.machineCounts else it.counts).entries }
            .groupBy({ it.key }, { it.value }).mapValues { (_, values) -> values.sum() }
        SessionListStatus(
            paged = statuses.any { it.paged }, loaded = statuses.isNotEmpty() && statuses.all { it.loaded },
            loading = statuses.any { it.loading }, failed = statuses.any { it.failed },
            hasMore = statuses.any { it.hasMore }, hasPrevious = statuses.any { it.hasPrevious },
            total = statuses.sumOf { it.total },
            counts = SessionListCounts(total = statuses.sumOf { it.counts.total }, machine = totals({ it.machine }),
                project = totals({ it.project }), group = totals({ it.group }), state = totals({ it.state }),
                revision = statuses.joinToString("|") { it.counts.revision }, backgroundWaiting = statuses.sumOf { it.counts.backgroundWaiting },
                paneSource = totals({ it.paneSource })),
            machineCounts = SessionListCounts(total = statuses.sumOf { it.machineCounts.total }, machine = totals({ it.machine }, true),
                project = totals({ it.project }, true), group = totals({ it.group }, true), state = totals({ it.state }, true)),
            filter = SessionListFilter(hiddenBotmuxHubs = statuses.flatMapTo(mutableSetOf()) { it.filter.hiddenBotmuxHubs }),
        )
    }
    override suspend fun setListFilter(filter: SessionListFilter) = stacks.forEach { stack ->
        if (!selected(stack)) return@forEach
        val machine = filter.machineId?.let { id -> if (stack.names.owns(id)) GroupKeys.raw(id) else id }
        stack.repositories.sessions.setListFilter(filter.forHub(stack.namespace).copy(machineId = machine))
    }
    override suspend fun loadMore() = stacks.forEach { if (selected(it)) it.repositories.sessions.loadMore() }
    override suspend fun loadPrevious() = stacks.forEach { if (selected(it)) it.repositories.sessions.loadPrevious() }
    override suspend fun listCounts(filter: SessionListFilter): SessionListCounts {
        val machine = requireNotNull(filter.machineId)
        return stacks.use(GroupKeys.namespaceOf(machine)) { stack ->
            val counts = stack.repositories.sessions.listCounts(filter.forHub(stack.namespace).copy(machineId = GroupKeys.raw(machine)))
            counts.copy(machine = counts.machine.mapKeys { (id, _) -> stack.names.machineId(id) })
        }
    }
    override suspend fun refresh() = stacks.forEach { it.repositories.sessions.refresh() }
    override suspend fun refreshMachine(machineId: String) = refreshOwnedSessions(machineId) { namespace, raw ->
        // Unlike the all-hubs refresh, this read proves this machine's own hub answered (#10671).
        stacks.use(namespace) { it.repositories.sessions.refreshMachine(raw) }
    }
    override fun verifiedSessions(): Map<String, List<Session>> = stacks.stacks.value.flatMap { stack ->
        stack.repositories.sessions.verifiedSessions().map { (machine, sessions) ->
            stack.names.machineId(machine) to sessions.map { stack.names.session(it) }
        }
    }.toMap()
    // A session just started (async start) and a machine whose engine changed go to their own group, in its plain ids.
    override suspend fun adopt(session: Session) = stacks.use(GroupKeys.namespaceOf(session.machineId)) {
        it.repositories.sessions.adopt(it.names.rawSession(session))
    }
    override suspend fun adoptThread(previous: SessionKey, session: Session) = stacks.use(GroupKeys.namespaceOf(previous.machineId)) {
        it.repositories.sessions.adoptThread(it.names.raw(previous), it.names.rawSession(session))
    }
    override suspend fun applyMachine(machine: Machine) = stacks.use(GroupKeys.namespaceOf(machine.id)) {
        it.repositories.sessions.applyMachine(if (it.names.plain) machine else machine.copy(id = GroupKeys.raw(machine.id)))
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
internal fun mergedInteractions(stacks: GroupStacks): InteractionCenter = object : InteractionCenter {
    override val pending = stacks.merged(emptyList<Interaction>(), { s ->
        s.mapped(s.repositories.interactions.pending) { list -> list.map(::interaction) }
    }) { it.flatten() }
    override val arrivals = stacks.stacks.flatMapLatest { list ->
        list.map { s -> s.mapped(s.repositories.interactions.arrivals) { interaction(it) } }.merge()
    }
    override suspend fun refresh() = stacks.forEach { it.repositories.interactions.refresh() }
    // Only the request's own group: another group answering says nothing about this one (#5697).
    override suspend fun refresh(id: String) = stacks.use(GroupKeys.namespaceOf(id)) { it.repositories.interactions.refresh() }
    override suspend fun resolve(id: String, resolution: Resolution) {
        // Merged interaction ids carry their group's suffix like machine ids (GroupNames.interactionId).
        stacks.use(GroupKeys.namespaceOf(id)) { it.repositories.interactions.resolve(GroupKeys.raw(id), resolution) }
    }
    override suspend fun resolveOutcome(id: String, resolution: Resolution) =
        resolveOutcome(id, resolution, handoffCard = false)
    override suspend fun resolveOutcome(id: String, resolution: Resolution, handoffCard: Boolean) =
        stacks.use(GroupKeys.namespaceOf(id)) { it.repositories.interactions.resolveOutcome(GroupKeys.raw(id), resolution, handoffCard) }
    override suspend fun replyOwnerApproval(interaction: Interaction, phrase: String) =
        stacks.use(GroupKeys.namespaceOf(interaction.id)) {
            it.repositories.interactions.replyOwnerApproval(it.names.rawInteraction(interaction), phrase)
        }
    override suspend fun discardLocal(id: String) =
        stacks.use(GroupKeys.namespaceOf(id)) { it.repositories.interactions.discardLocal(GroupKeys.raw(id)) }
    override val delegated = stacks.merged(emptyList<DelegatedAnswer>(), { s ->
        s.mapped(s.repositories.interactions.delegated) { list -> list.map { it.copy(interaction = interaction(it.interaction)) } }
    }) { lists -> lists.flatten().sortedBy { it.at } }
    override val reanswered = stacks.merged(emptyMap<String, Map<String, String>>(), { s ->
        s.mapped(s.repositories.interactions.reanswered) { changes -> changes.mapKeys { interactionId(it.key) } }
    }) { maps -> maps.fold(emptyMap()) { all, one -> all + one } }
    override fun dropDelegated(id: String) {
        stacks.stacks.value.firstOrNull { it.namespace == GroupKeys.namespaceOf(id) }?.repositories?.interactions?.dropDelegated(GroupKeys.raw(id))
    }
    override suspend fun reanswer(id: String, answers: Map<String, String>, manager: String?) =
        stacks.use(GroupKeys.namespaceOf(id)) { it.repositories.interactions.reanswer(GroupKeys.raw(id), answers, manager) }
}

@OptIn(ExperimentalCoroutinesApi::class)
internal fun mergedChats(stacks: GroupStacks, storage: ChatLocalStorage): ChatRepository =
    object : ChatRepository, StructuralChatRepository, ChatLocalStorage by storage {
        override fun observe(session: SessionKey) = stacks.stacks
            .map { list -> list.firstOrNull { it.names.owns(session.machineId) } }
            .distinctUntilChanged()
            .flatMapLatest { stack ->
                stack?.repositories?.chats?.observe(stack.names.raw(session))
                    // A group that closed under the page (merged or disconnected) just stops; its own failures surface.
                    ?.catch { failure -> if (stack.current) throw failure }
                    ?: flowOf(ChatSnapshot(loading = false, verified = false))
            }
        override fun observeStructure(session: SessionKey) = stacks.stacks
            .map { list -> list.firstOrNull { it.names.owns(session.machineId) } }
            .distinctUntilChanged()
            .flatMapLatest { stack ->
                stack?.repositories?.chats?.observeStructure(stack.names.raw(session))
                    ?.catch { failure -> if (stack.current) throw failure }
                    ?: flowOf(ChatSnapshot(loading = false, verified = false))
            }
        override suspend fun loadOlder(session: SessionKey) = stacks.use(session) { it.repositories.chats.loadOlder(it.names.raw(session)) }
        override suspend fun earlierEnginePage(session: SessionKey, beforeSeq: Int?) =
            stacks.use(session) { it.repositories.chats.earlierEnginePage(it.names.raw(session), beforeSeq) }
        override suspend fun modeChanges(session: SessionKey): List<ModeChange> =
            stacks.use(session) { it.repositories.chats.modeChanges(it.names.raw(session)) }
        override suspend fun expandFold(session: SessionKey, fromSeq: Int, toSeq: Int) =
            stacks.use(session) { it.repositories.chats.expandFold(it.names.raw(session), fromSeq, toSeq) }
        override suspend fun loadRange(session: SessionKey, fromSeq: Int, throughSeq: Int) =
            stacks.use(session) { it.repositories.chats.loadRange(it.names.raw(session), fromSeq, throughSeq) }
        override suspend fun loadImage(session: SessionKey, itemId: String) =
            stacks.use(session) { it.repositories.chats.loadImage(it.names.raw(session), itemId) }
        override suspend fun fullText(session: SessionKey, itemId: String) =
            stacks.use(session) { it.repositories.chats.fullText(it.names.raw(session), itemId) }
        override suspend fun markSeen(session: SessionKey, itemId: String) =
            stacks.use(session) { it.repositories.chats.markSeen(it.names.raw(session), itemId) }
    }

internal fun mergedPreferences(stacks: GroupStacks): HubPreferences = object : HubPreferences {
    override val preferences = stacks.merged(HubPreferencesSnapshot(), { s ->
        val source = s.repositories.preferences?.preferences ?: flowOf(HubPreferencesSnapshot())
        s.mapped(source) { preferences(it) }
    }) { list ->
        fun union(sets: List<HubPreferenceSet>) = HubPreferenceSet(sets.flatMap { it.spaces }.distinct(),
            sets.flatMap { it.sessions }.distinct(), sets.flatMapTo(LinkedHashSet()) { it.allowed })
        HubPreferencesSnapshot(union(list.map { it.pins }), union(list.map { it.mutes }),
            list.fold(emptyMap()) { seen, snapshot -> seen + snapshot.seen })
    }
    private suspend fun <T> withPreferences(namespace: String?, block: suspend (HubPreferences) -> T) =
        stacks.use(namespace) { block(requireNotNull(it.repositories.preferences)) }
    override suspend fun refresh() = stacks.forEach { requireNotNull(it.repositories.preferences).refresh() }
    override suspend fun togglePin(path: String, space: Boolean) =
        withPreferences(GroupKeys.pathNamespace(path)) { it.togglePin(GroupKeys.rawPath(path), space) }
    override suspend fun toggleMute(path: String, space: Boolean) =
        withPreferences(GroupKeys.pathNamespace(path)) { it.toggleMute(GroupKeys.rawPath(path), space) }
    override suspend fun markSeen(session: SessionKey, version: String) =
        withPreferences(GroupKeys.namespaceOf(session.machineId)) { it.markSeen(GroupKeys.rawKey(session), version) }
}

/**
 * Machine management (machines page, engine, Claude Code / Codex / Antigravity settings) over every group (#634): each call goes to the group its machine id
 * names ([routes]: that group's service, already speaking merged ids), so another group's machine is no longer asked of the
 * selected group's hub. Hub-level services (model configuration, voice) stay with the selected group, like the other hub-level pages.
 * Subscription quota belongs to a hub too, but the merged overview shows every group's machines, so it lists every group's
 * accounts ([GroupUsage]).
 */
internal fun mergedMachineManagement(
    routes: MachineRoutes,
    /** Every open group's own read of its hub's subscription quota, in the groups' order. */
    usage: (() -> List<GroupUsageRead>)? = null,
): MachineManagement = object : MachineManagement {
    private val groups = GroupUsage()
    override suspend fun launchers(machineId: String) = routes.on(machineId) { it.launchers(machineId) }
    override suspend fun launcherQuota(machineId: String, name: String, refresh: Boolean) =
        routes.on(machineId) { it.launcherQuota(machineId, name, refresh) }
    override suspend fun reconnect(machineId: String) = routes.on(machineId) { it.reconnect(machineId) }
    override suspend fun startLinkSwitch(machineId: String) = routes.on(machineId) { it.startLinkSwitch(machineId) }
    override suspend fun rename(machineId: String, name: String) = routes.on(machineId) { it.rename(machineId, name) }
    override suspend fun deploy(machineId: String) = routes.on(machineId) { it.deploy(machineId) }
    override suspend fun herdr(machineId: String) = routes.on(machineId) { it.herdr(machineId) }
    override suspend fun startHerdr(machineId: String) = routes.on(machineId) { it.startHerdr(machineId) }
    override suspend fun setEngine(machineId: String, engine: String) = routes.on(machineId) { it.setEngine(machineId, engine) }
    override suspend fun botmux(machineId: String) = routes.on(machineId) { it.botmux(machineId) }
    override suspend fun setBotmux(machineId: String, on: Boolean) = routes.on(machineId) { it.setBotmux(machineId, on) }
    override suspend fun claudeSettings(machineId: String) = routes.on(machineId) { it.claudeSettings(machineId) }
    override suspend fun setClaudeSetting(machineId: String, key: String, value: kotlinx.serialization.json.JsonElement) =
        routes.on(machineId) { it.setClaudeSetting(machineId, key, value) }
    override suspend fun codexSettings(machineId: String) = routes.on(machineId) { it.codexSettings(machineId) }
    override suspend fun setCodexSetting(machineId: String, key: String, value: kotlinx.serialization.json.JsonElement) =
        routes.on(machineId) { it.setCodexSetting(machineId, key, value) }
    override suspend fun agySettings(machineId: String) = routes.on(machineId) { it.agySettings(machineId) }
    override suspend fun setAgySetting(machineId: String, key: String, value: kotlinx.serialization.json.JsonElement) =
        routes.on(machineId) { it.setAgySetting(machineId, key, value) }
    override suspend fun claudeDirs(machineId: String) = routes.on(machineId) { it.claudeDirs(machineId) }
    override suspend fun setBlockedClaudeDirs(machineId: String, blocked: List<String>) =
        routes.on(machineId) { it.setBlockedClaudeDirs(machineId, blocked) }
    override suspend fun claudeLiveReplies(machineId: String) = routes.on(machineId) { it.claudeLiveReplies(machineId) }
    override suspend fun setClaudeLiveReplies(machineId: String, on: Boolean) =
        routes.on(machineId) { it.setClaudeLiveReplies(machineId, on) }
    override suspend fun doctor(machineId: String) = routes.on(machineId) { it.doctor(machineId) }
    override suspend fun fixDoctor(machineId: String) = routes.on(machineId) { it.fixDoctor(machineId) }
    override suspend fun browser(machineId: String) = routes.on(machineId) { it.browser(machineId) }
    override suspend fun setBrowser(machineId: String, enabled: Boolean) = routes.on(machineId) { it.setBrowser(machineId, enabled) }
    override suspend fun browserJob(machineId: String, action: String) = routes.on(machineId) { it.browserJob(machineId, action) }
    override suspend fun usageWindows() = groups.read(usage?.invoke().orEmpty())
    override suspend fun usageWindows(machineId: String) = routes.on(machineId) { it.usageWindows() }
    override suspend fun moveToManager(machineId: String, manager: String) = routes.on(machineId) { it.moveToManager(machineId, manager) }
}

/** One open group's read of its hub's subscription quota; [group] is its namespace ("" for the primary group). */
internal class GroupUsageRead(val group: String, val read: suspend () -> List<UsageAccount>)

/**
 * Subscription limits on the merged machines overview (#1845, as iOS / Mac: docs/server/usage-windows.md): every group's hub
 * is asked on its own, at once, and the answers are listed in the groups' order, so the same account in two groups shows
 * once in each. An old hub without the route (404) has none; a group whose read fails keeps its last answer while the
 * others show theirs, and a group that went away during the read has none.
 */
internal class GroupUsage {
    private val last = ConcurrentHashMap<String, List<UsageAccount>>()

    suspend fun read(groups: List<GroupUsageRead>): List<UsageAccount> = coroutineScope {
        groups.map { group -> async { answer(group) } }.awaitAll().flatten()
    }

    private suspend fun answer(group: GroupUsageRead): List<UsageAccount> = try {
        group.read().also { last[group.group] = it }
    } catch (gone: CancellationException) {
        // Its group closed under the call (GroupStacks.use); the overview itself going away still cancels the read.
        currentCoroutineContext().ensureActive()
        emptyList()
    } catch (failure: Exception) {
        if ((failure as? HubFailure)?.status == 404) emptyList<UsageAccount>().also { last[group.group] = it }
        else last[group.group].orEmpty()
    }
}

/** Runs a call on the machine management of the group [machineId] names, in that group's lifetime (GroupStacks.use). */
internal interface MachineRoutes {
    suspend fun <T> on(machineId: String, call: suspend (MachineManagement) -> T): T
}

/** Terminal, files, chat actions and machine metrics for the shell: each call goes to its machine's group. */
internal class MergedShellServices(private val stacks: GroupStacks) {
    private fun stack(machineId: String) = stacks.of(machineId) ?: throw IllegalStateException(AppText.get(R.string.app_error_terminal_unavailable))

    fun terminal(key: SessionKey): TerminalTransport = stack(key.machineId).terminal

    /**
     * The terminal of a machine in another group (its id carries the suffix), in merged ids and on that group's own
     * connection; null for the primary group's machines, or when the group is not open.
     */
    fun groupTerminal(machineId: String): TerminalTransport? =
        if (GroupKeys.namespaceOf(machineId) == null) null else stacks.of(machineId)?.mergedTerminal

    /**
     * Terminal's pane lists and new tabs (docs/integrations/terminal.md "Standalone terminal"): each call goes to the machine's group with its plain
     * id. What comes back names no machine (host-local ids, session ids, `pane:` addresses), so it needs no translating.
     */
    @OptIn(ExperimentalCoroutinesApi::class)
    val panes: TerminalPanes = object : TerminalPanes {
        override val available: Flow<Boolean> = stacks.stacks.flatMapLatest { list ->
            if (list.isEmpty()) flowOf(false)
            else combine(list.map { it.connection.serverFeatures }) { features -> features.any { HttpTerminalPanes.TERMINAL_PANES in it } }
        }.distinctUntilChanged()
        override fun lists(machineId: String) =
            stacks.of(machineId)?.connection?.serverFeatures?.value?.contains(HttpTerminalPanes.TERMINAL_PANES) == true
        override fun opensNewSpaces(machineId: String) =
            HttpTerminalPanes.offersNewSpace(stacks.of(machineId)?.connection?.serverFeatures?.value.orEmpty())
        override suspend fun panes(machineId: String) =
            stacks.use(GroupKeys.namespaceOf(machineId)) { HttpTerminalPanes(it.connection).panes(GroupKeys.raw(machineId)) }
        override suspend fun newTab(machineId: String, workspaceId: String, kind: String?, cwd: String?) =
            stacks.use(GroupKeys.namespaceOf(machineId)) { HttpTerminalPanes(it.connection).newTab(GroupKeys.raw(machineId), workspaceId, kind, cwd) }
        override suspend fun close(machineId: String, pane: String) =
            stacks.use(GroupKeys.namespaceOf(machineId)) { HttpTerminalPanes(it.connection).close(GroupKeys.raw(machineId), pane) }
        override suspend fun closeFinished(machineId: String, preview: Boolean, panes: List<String>) =
            stacks.use(GroupKeys.namespaceOf(machineId)) { HttpTerminalPanes(it.connection).closeFinished(GroupKeys.raw(machineId), preview, panes) }
    }

    val files: SessionFiles = routedFiles { stacks.of(it)?.files }

    val actions: ChatActions = object : ChatActions {
        private fun of(session: Session) = stacks.of(session.machineId)?.actions ?: throw HubFailure(HubError.NoNetwork)
        override suspend fun isMuted(session: Session) = of(session).isMuted(session)
        override suspend fun execute(session: Session, action: ChatAction) = of(session).execute(session, action)
        override suspend fun verify(session: Session, action: ChatAction) = of(session).verify(session, action)
        override suspend fun editQueue(session: Session, keep: List<Int>) = of(session).editQueue(session, keep)
        override suspend fun sendQueuedNow(session: Session) = of(session).sendQueuedNow(session)
        override suspend fun retryUndelivered(session: Session, messageId: String) = of(session).retryUndelivered(session, messageId)
        override suspend fun deleteUndelivered(session: Session, messageId: String) = of(session).deleteUndelivered(session, messageId)
        override suspend fun info(session: Session) = of(session).info(session)
        override suspend fun completeRow(session: Session) = of(session).completeRow(session)
        override suspend fun childRows(session: Session) = of(session).childRows(session)
        override suspend fun childPage(session: Session, since: String?) = of(session).childPage(session, since)
        override suspend fun interactionHistory(session: Session) = of(session).interactionHistory(session)
        override suspend fun restoreModel(session: Session) = of(session).restoreModel(session)
        override suspend fun catalogPrices(ids: List<String>): Map<String, ModelPrice?> {
            val stack = stacks.selected.value ?: stacks.stacks.value.firstOrNull() ?: throw HubFailure(HubError.NoNetwork)
            return stack.actions.catalogPrices(ids)
        }
        override suspend fun fork(session: Session, atItemId: String) = of(session).fork(session, atItemId)
        override suspend fun rewind(session: Session, target: RewindTarget, restore: RewindRestore) = of(session).rewind(session, target, restore)
    }

    /** Every group's dashboard in one; [plain] builds the API of one group's connection (null while it has no route). */
    fun metrics(plain: (GroupStack) -> MachineMetricsApi?): MachineMetricsApi = object : MachineMetricsApi {
        private fun of(stack: GroupStack) = plain(stack) ?: throw HubFailure(HubError.NoNetwork)
        override suspend fun dashboard(): MetricsDashboard = mergedMetrics(stacks.stacks.value.map { stack ->
            MetricsGroup(stack.names, { plain(stack) }, stack.metricSnapshots) { stack.current }
        })
        override suspend fun history(machineId: String, range: MetricRange): MetricsHistory {
            val stack = stacks.of(machineId) ?: throw HubFailure(HubError.NoNetwork)
            val history = of(stack).history(GroupKeys.raw(machineId), range)
            return history.copy(metrics = stack.names.metrics(history.metrics))
        }
    }
}

/**
 * [MergedShellServices.files]: each call goes to the files of the group [lookup] finds for its machine, in merged ids;
 * no open group fails as offline. Every method is forwarded, the ones with a default implementation too, or the app
 * would get the default ("not supported") for them.
 */
internal fun routedFiles(lookup: (machineId: String) -> SessionFiles?): SessionFiles = object : SessionFiles {
    private fun of(machineId: String) = lookup(machineId) ?: throw HubFailure(HubError.NoNetwork)
    override suspend fun directory(machineId: String, cwd: String, path: String) = of(machineId).directory(machineId, cwd, path)
    override suspend fun file(machineId: String, cwd: String, path: String) = of(machineId).file(machineId, cwd, path)
    override suspend fun filePreview(machineId: String, cwd: String, path: String) = of(machineId).filePreview(machineId, cwd, path)
    override suspend fun status(machineId: String, cwd: String) = of(machineId).status(machineId, cwd)
    override suspend fun diff(machineId: String, cwd: String, path: String, staged: Boolean) = of(machineId).diff(machineId, cwd, path, staged)
    override suspend fun log(machineId: String, cwd: String, skip: Int, limit: Int) = of(machineId).log(machineId, cwd, skip, limit)
    override suspend fun show(machineId: String, cwd: String, hash: String) = of(machineId).show(machineId, cwd, hash)
    override suspend fun pull(machineId: String, cwd: String) = of(machineId).pull(machineId, cwd)
    override suspend fun stat(machineId: String, cwd: String, paths: List<String>) = of(machineId).stat(machineId, cwd, paths)
    override suspend fun download(machineId: String, cwd: String, path: String, into: java.io.File) =
        of(machineId).download(machineId, cwd, path, into)
    override suspend fun readLogTail(machineId: String, cwd: String, path: String, manager: String?) =
        of(machineId).readLogTail(machineId, cwd, path, manager)
    override fun comparesVersions(machineId: String) = lookup(machineId)?.comparesVersions(machineId) == true
    override suspend fun blob(machineId: String, cwd: String, path: String, rev: String) = of(machineId).blob(machineId, cwd, path, rev)
    override suspend fun thumbnail(machineId: String, cwd: String, path: String, size: Int) = of(machineId).thumbnail(machineId, cwd, path, size)
    override suspend fun convertToPdf(machineId: String, cwd: String, path: String) = of(machineId).convertToPdf(machineId, cwd, path)
    override fun editsFiles(machineId: String) = lookup(machineId)?.editsFiles(machineId) == true
    override suspend fun rename(machineId: String, cwd: String, path: String, name: String) = of(machineId).rename(machineId, cwd, path, name)
    override suspend fun move(machineId: String, cwd: String, path: String, directory: String, conflict: SessionMoveConflictPolicy?) =
        of(machineId).move(machineId, cwd, path, directory, conflict)
    override suspend fun trash(machineId: String, cwd: String, paths: List<String>) = of(machineId).trash(machineId, cwd, paths)
    override suspend fun restore(machineId: String, trashIds: List<String>) = of(machineId).restore(machineId, trashIds)
}

/**
 * The Agents page's pairing-owned operations over every group. Pins and mutes are one list per hub: a merged save
 * writes only the groups whose part changed, so a group that cannot be reached is never overwritten with nothing.
 */
internal fun mergedAgentsState(list: List<GroupNames>, states: List<AgentsWiringState>, chosen: Int): AgentsWiringState {
    fun stateFor(machineId: String) = list.indexOfFirst { it.owns(machineId) }.takeIf { it >= 0 }?.let(states::get)
    val scope = states.joinToString("+") { it.scope }
    val known = ConcurrentHashMap<Int, Pair<AgentOperationPreferences?, AgentOperationPreferences?>>()
    fun owned(value: AgentOperationPreferences, names: GroupNames) = AgentOperationPreferences(
        value.spaces.filter { names.owns(it.substringBefore('/')) },
        value.sessions.filter { names.owns(it.substringBefore('/')) },
        value.allowed.filterTo(LinkedHashSet()) { names.owns(it.substringBefore('/')) })
    fun union(values: List<AgentOperationPreferences>) = AgentOperationPreferences(values.flatMap { it.spaces },
        values.flatMap { it.sessions }, values.flatMapTo(LinkedHashSet()) { it.allowed })
    fun operations(index: Int) = states[index].operations?.operations ?: throw HubFailure(HubError.NoNetwork)
    fun operationsFor(machineId: String) = stateFor(machineId)?.operations?.operations ?: throw HubFailure(HubError.NoNetwork)
    val operations = object : AgentOperations {
        override val scope = scope
        private suspend fun read(pins: Boolean): AgentOperationPreferences = union(list.indices.map { index ->
            try {
                val value = if (pins) operations(index).pins() else operations(index).mutes()
                known.compute(index) { _, old -> if (pins) value to old?.second else old?.first to value }
                value
            } catch (cancelled: kotlinx.coroutines.CancellationException) { throw cancelled }
            catch (_: Exception) {
                val old = known[index]
                (if (pins) old?.first else old?.second) ?: AgentOperationPreferences()
            }
        })
        private suspend fun write(pins: Boolean, value: AgentOperationPreferences) {
            list.forEachIndexed { index, names ->
                val part = owned(value, names)
                val old = known[index]?.let { if (pins) it.first else it.second }
                if (old == part) return@forEachIndexed
                // A group never read (unreachable) is only left alone: writing it would drop what it already has.
                if (old == null) {
                    if (part == AgentOperationPreferences()) return@forEachIndexed
                    throw HubFailure(HubError.NoNetwork)
                }
                if (pins) operations(index).savePins(part) else operations(index).saveMutes(part)
                known.compute(index) { _, previous -> if (pins) part to previous?.second else previous?.first to part }
            }
        }
        override suspend fun pins() = read(true)
        override suspend fun mutes() = read(false)
        override suspend fun savePins(value: AgentOperationPreferences) = write(true, value)
        override suspend fun saveMutes(value: AgentOperationPreferences) = write(false, value)
        override suspend fun renameSession(key: SessionKey, title: String?, tabName: String?) =
            operationsFor(key.machineId).renameSession(key, title, tabName)
        override suspend fun renameSpace(machineId: String, workspaceId: String, name: String) =
            operationsFor(machineId).renameSpace(machineId, workspaceId, name)
        override suspend fun closeSession(key: SessionKey) = operationsFor(key.machineId).closeSession(key)
        override suspend fun attachSession(key: SessionKey) = operationsFor(key.machineId).attachSession(key)
        override suspend fun deleteSession(key: SessionKey) = operationsFor(key.machineId).deleteSession(key)
    }
    val assistant = states[chosen].operations?.assistant ?: states.firstNotNullOfOrNull { it.operations?.assistant }
    val creations = states.map { it.creation }
    val started = ConcurrentHashMap<String, Int>()
    val creation = if (creations.all { it == null }) null else object : AgentCreationService {
        override val scope = scope
        private fun of(machineId: String) = list.indexOfFirst { it.owns(machineId) }.takeIf { it >= 0 }
        private fun creation(index: Int?) = index?.let { creations[it] } ?: throw HubFailure(HubError.NoNetwork)
        override suspend fun launchers(machineId: String) = creation(of(machineId)).launchers(machineId)
        override suspend fun launcherList(machineId: String) = creation(of(machineId)).launcherList(machineId)
        override suspend fun directories(machineId: String, path: String) = creation(of(machineId)).directories(machineId, path)
        override suspend fun start(request: AgentStartRequest, operationId: String): AgentStartResult {
            val index = of(request.machineId)
            index?.let { started[operationId] = it }
            return creation(index).start(request, operationId)
        }
        override suspend fun operation(operationId: String) = creation(started[operationId] ?: chosen).operation(operationId)
        // The machine names the hub that took the start: [started] is lost when the wiring is rebuilt or the process dies.
        override suspend fun operation(operationId: String, machineId: String) = creation(of(machineId)).operation(operationId)
        // "Follow model configuration" reads the selected group's agent.new purpose.
        override suspend fun newAgentModel() = creations.getOrNull(chosen)?.newAgentModel()
        override suspend fun launcherModels(machineId: String, launcher: String, refresh: Boolean) =
            creation(of(machineId)).launcherModels(machineId, launcher, refresh)
        override suspend fun launcherModels(machineId: String, launcher: String, refresh: Boolean, host: String?) =
            creation(of(machineId)).launcherModels(machineId, launcher, refresh, host)
        override suspend fun worktreeStatus(machineId: String, cwd: String, kind: String?) = creation(of(machineId)).worktreeStatus(machineId, cwd, kind)
        override suspend fun addWorktree(machineId: String, cwd: String, nameHint: String, branch: String, kind: String?) =
            creation(of(machineId)).addWorktree(machineId, cwd, nameHint, branch, kind)
        override suspend fun removeWorktree(machineId: String, path: String, deleteBranch: Boolean, force: Boolean, kind: String?) =
            creation(of(machineId)).removeWorktree(machineId, path, deleteBranch, force, kind)
        // A computer's catalog workspaces come from the hub that owns it; none when no hub does.
        override suspend fun workspaces(machineId: String) = of(machineId)?.let { creations[it] }?.workspaces(machineId).orEmpty()
        override suspend fun workspaceCatalog(machineId: String) = creation(of(machineId)).workspaceCatalog(machineId)
    }
    return AgentsWiringState(
        scope = scope,
        operations = assistant?.let { conversations ->
            AgentsOperationsBindings(operations, conversations) { key -> stateFor(key.machineId)?.operations?.onSessionClosed?.invoke(key) }
        },
        creation = creation,
        drafts = AgentsDraftSource { key -> stateFor(key.machineId)?.drafts?.observe(key) ?: emptyFlow() },
        mainSender = states[chosen].mainSender,
        creationPreferences = states.firstNotNullOfOrNull { it.creationPreferences },
        archives = mergedAgentArchives(list, states.map { it.archives }),
    )
}
