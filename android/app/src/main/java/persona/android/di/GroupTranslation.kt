package persona.android.di

import persona.core.contracts.*
import persona.core.contracts.api.*
import persona.core.domain.fleet.FleetSummary
import persona.feature.agents.AgentsDraftSource
import persona.feature.agents.AgentsOperationsBindings
import persona.feature.agents.AgentsWiringState
import persona.feature.chat.ChatAppBindings
import persona.feature.inbox.InboxSnapshot
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.*
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import java.io.File

/**
 * Moves one group's values between its own plain machine ids (what its hubs, data and repositories use) and the ids
 * the merged lists show (GroupKeys: `~<group id>` on every group but the primary one). A null [namespace] is the
 * primary group: every function is then the identity and every wrapper below returns its argument unchanged.
 */
internal class GroupNames(val namespace: String?) {
    val plain: Boolean get() = namespace == null

    fun machineId(raw: String) = GroupKeys.machine(raw, namespace)
    fun key(raw: SessionKey) = GroupKeys.key(raw, namespace)
    fun path(raw: String) = GroupKeys.path(raw, namespace)

    /** Whether a merged-list machine id belongs to this group. */
    fun owns(machineId: String) = GroupKeys.namespaceOf(machineId) == namespace

    fun machine(value: Machine) = if (plain) value else value.copy(id = machineId(value.id), name = value.name.ifEmpty { value.id })
    fun session(value: Session) = if (plain) value else value.copy(machineId = machineId(value.machineId))
    /** Interaction ids are the hub's own and two hubs can use the same one: other groups' carry the suffix too. */
    fun interactionId(raw: String) = GroupKeys.machine(raw, namespace)
    fun interaction(value: Interaction) = if (plain) value else value.copy(id = interactionId(value.id), machineId = machineId(value.machineId))
    fun rawInteraction(value: Interaction) = if (plain) value else
        value.copy(id = GroupKeys.raw(value.id), machineId = rawMachine(value.machineId))
    fun fleet(value: FleetState) = if (plain) value else value.copy(items = value.items.map { item ->
        // The id is "machine/session": FleetSummary.key opens and answers by its machine part.
        item.copy(id = path(item.id), interaction = item.interaction?.let(::interactionId))
    })
    fun outbox(value: OutboxMessage) = if (plain) value else value.copy(session = key(value.session))
    fun receipt(value: MessageSendReceipt) = if (plain) value else value.copy(session = key(value.session))
    fun draftKey(value: DraftKey) = if (plain || value !is DraftKey.Chat) value else DraftKey.Chat(key(value.session))
    fun draft(value: DraftSnapshot) = if (plain) value else value.copy(key = draftKey(value.key))
    fun metrics(value: MachineMetrics) = if (plain) value else value.copy(machineId = machineId(value.machineId))

    fun preferences(value: HubPreferencesSnapshot) = if (plain) value else HubPreferencesSnapshot(
        preferenceSet(value.pins), preferenceSet(value.mutes), value.seen.mapKeys { path(it.key) })
    fun preferenceSet(value: HubPreferenceSet) = if (plain) value else
        HubPreferenceSet(value.spaces.map(::path), value.sessions.map(::path), value.allowed.mapTo(LinkedHashSet(), ::path))
    fun operationPreferences(value: AgentOperationPreferences) = if (plain) value else
        AgentOperationPreferences(value.spaces.map(::path), value.sessions.map(::path), value.allowed.mapTo(LinkedHashSet(), ::path))

    fun event(value: ServerEvent): ServerEvent = if (plain) value else when (value) {
        is MessageSendStateEvent -> value.copy(machineId = machineId(value.machineId))
        is ItemsEvent -> value.copy(machineId = machineId(value.machineId))
        is SessionsEvent -> value.copy(machineId = machineId(value.machineId), sessions = value.sessions.map(::session))
        is SessionRowEvent -> value.copy(session = session(value.session))
        is MachinesEvent -> value.copy(machines = value.machines.map(::machine))
        is RevsEvent -> value.copy(machineId = machineId(value.machineId))
        is InteractionEvent -> value.copy(interaction = interaction(value.interaction))
        is InteractionResolvedEvent -> value.copy(id = interactionId(value.id))
        is InteractionReansweredEvent -> value.copy(id = interactionId(value.id))
        is NotifyEvent -> value.copy(machineId = machineId(value.machineId), interactionId = value.interactionId?.let(::interactionId),
            id = value.id?.let(::interactionId))
        is FleetEvent -> value.copy(fleet = fleet(value.fleet))
        is DeltaEvent -> value.copy(machineId = machineId(value.machineId))
        is ScreenEvent -> value.copy(machineId = machineId(value.machineId))
        is HookEvent -> value.copy(machineId = machineId(value.machineId))
        is TerminalEvent -> value.copy(machineId = machineId(value.machineId))
        is TerminalControlEvent -> value.copy(machineId = machineId(value.machineId))
        is ErrorEvent -> value.copy(machineId = value.machineId?.let(::machineId))
        else -> value
    }

    // Plain values for the group's own services. Ids of another group never reach them (the facades route by suffix).
    fun raw(key: SessionKey) = GroupKeys.rawKey(key)
    fun rawMachine(machineId: String) = GroupKeys.raw(machineId)
    fun rawSession(value: Session) = if (plain) value else value.copy(machineId = GroupKeys.raw(value.machineId))
    fun rawDraftKey(value: DraftKey) = if (plain || value !is DraftKey.Chat) value else DraftKey.Chat(raw(value.session))
    fun rawDraft(value: DraftSnapshot) = if (plain) value else value.copy(key = rawDraftKey(value.key))
    fun rawOperationPreferences(value: AgentOperationPreferences) = if (plain) value else
        AgentOperationPreferences(value.spaces.map(GroupKeys::rawPath), value.sessions.map(GroupKeys::rawPath),
            value.allowed.mapTo(LinkedHashSet(), GroupKeys::rawPath))
}

internal fun <T, R> StateFlow<T>.mapIn(scope: CoroutineScope, transform: (T) -> R): StateFlow<R> =
    map(transform).stateIn(scope, SharingStarted.Eagerly, transform(value))

/** A group's chat bindings as the merged chat pages use them. */
internal fun ChatAppBindings.inGroup(names: GroupNames): ChatAppBindings {
    if (names.plain) return this
    val source = this
    return copy(
        drafts = drafts.inGroup(names),
        connection = connection.inGroup(names, coroutineScope),
        actions = actions.inGroup(names),
        composer = composer.inGroup(names, coroutineScope),
        recovery = recovery.inGroup(names),
        assistantManagement = assistantManagement.inGroup(names),
        mainSender = mainSender.inGroup(names, coroutineScope),
        draftVersions = { key -> source.draftVersions(names.raw(key)) },
        ports = ports?.inGroup(names),
        agentShares = agentShares?.let { provider -> AgentSharesProvider { session, manager ->
            if (GroupKeys.namespaceOf(session.machineId) != names.namespace) throw AgentShareFailure("session_conflict")
            val api = provider.forSession(names.raw(session), manager)
            object : AgentShares by api {
                override suspend fun create(request: ShareCreateRequest): ShareCreated {
                    if (request.machine != session.machineId || request.sessionId != session.sessionId) throw AgentShareFailure("session_conflict")
                    return api.create(request.copy(machine = names.rawMachine(request.machine)))
                }
            }
        } },
    )
}

internal fun DraftRepository.inGroup(names: GroupNames): DraftRepository {
    if (names.plain) return this
    val source = this
    return object : DraftRepository {
        override val structured get() = source.structured
        override fun observe(key: DraftKey) = source.observe(names.rawDraftKey(key)).map(names::draft)
        override suspend fun read(key: DraftKey) = names.draft(source.read(names.rawDraftKey(key)))
        override suspend fun save(key: DraftKey, text: String, expectedVersion: Long) =
            names.draft(source.save(names.rawDraftKey(key), text, expectedVersion))
        override suspend fun save(key: DraftKey, text: String, expectedVersion: Long,
            attachments: List<DraftAttachment>, sessionVersion: DraftSessionVersion?) =
            names.draft(source.save(names.rawDraftKey(key), text, expectedVersion, attachments, sessionVersion))
        override suspend fun save(key: DraftKey, text: String, expectedVersion: Long,
            attachments: List<DraftAttachment>, sessionVersion: DraftSessionVersion?, hasPickedSavedPrompt: Boolean) =
            names.draft(source.save(names.rawDraftKey(key), text, expectedVersion, attachments, sessionVersion, hasPickedSavedPrompt))
    }
}

internal fun HubConnection.inGroup(names: GroupNames, scope: CoroutineScope): HubConnection {
    if (names.plain) return this
    val source = this
    return object : HubConnection {
        override val state = source.state
        override val currentHello = source.currentHello
        override val resumed get() = source.resumed
        override val events: SharedFlow<ServerEvent> = source.events.map(names::event)
            .shareIn(scope, SharingStarted.WhileSubscribed())
        override fun reconnect() = source.reconnect()
        override fun send(message: JsonObject) = source.send(message)
    }
}

/**
 * A group's terminal as the merged pages use it: keys lose the suffix on the way to its hub (a `pane:` address passes
 * as it is), its stream comes back in merged ids, and its connection and features are that hub's own.
 */
internal fun TerminalTransport.inGroup(names: GroupNames): TerminalTransport {
    if (names.plain) return this
    val source = this
    return object : TerminalTransport {
        override val connection = source.connection
        override val features = source.features
        override val events = source.events.map(names::event)
        override fun subscribe(key: SessionKey, epoch: String?, after: Int, size: TerminalSize?, takeover: Boolean) =
            source.subscribe(names.raw(key), epoch, after, size, takeover)
        override fun unsubscribe(key: SessionKey) = source.unsubscribe(names.raw(key))
        override fun input(key: SessionKey, text: String, size: TerminalSize?, scroll: Int, at: TerminalCellPosition?, click: TerminalCellPosition?) =
            source.input(names.raw(key), text, size, scroll, at, click)
        override suspend fun screen(key: SessionKey) = source.screen(names.raw(key))
        override suspend fun keys(key: SessionKey, keys: List<String>, text: String) = source.keys(names.raw(key), keys, text)
    }
}

internal fun ChatActions.inGroup(names: GroupNames): ChatActions {
    if (names.plain) return this
    val source = this
    return object : ChatActions {
        override suspend fun isMuted(session: Session) = source.isMuted(names.rawSession(session))
        override suspend fun execute(session: Session, action: ChatAction) = source.execute(names.rawSession(session), action)
        override suspend fun verify(session: Session, action: ChatAction) = source.verify(names.rawSession(session), action)
        override suspend fun editQueue(session: Session, keep: List<Int>) = source.editQueue(names.rawSession(session), keep)
        override suspend fun sendQueuedNow(session: Session) = source.sendQueuedNow(names.rawSession(session))
        override suspend fun retryUndelivered(session: Session, messageId: String) =
            source.retryUndelivered(names.rawSession(session), messageId)
        override suspend fun deleteUndelivered(session: Session, messageId: String) =
            source.deleteUndelivered(names.rawSession(session), messageId)
        override suspend fun info(session: Session) = source.info(names.rawSession(session))
        override suspend fun completeRow(session: Session) = source.completeRow(names.rawSession(session))?.let(names::session)
        override suspend fun childRows(session: Session) = source.childRows(names.rawSession(session)).map(names::session)
        override suspend fun childPage(session: Session, since: String?): SessionListPage {
            val page = source.childPage(names.rawSession(session), since)
            return page.copy(sessions = page.sessions.map(names::session), remove = page.remove.map { encoded ->
                val key = JsonCodec.decodeFromString<List<String>>(encoded)
                require(key.size == 2) { "Invalid session removal key" }
                JsonCodec.encodeToString(listOf(names.machineId(key[0]), key[1]))
            })
        }
        override suspend fun interactionHistory(session: Session) = source.interactionHistory(names.rawSession(session))
            .map { it.copy(interaction = names.interaction(it.interaction)) }
        override suspend fun restoreModel(session: Session) = source.restoreModel(names.rawSession(session))
        override suspend fun catalogPrices(ids: List<String>) = source.catalogPrices(ids)
        override suspend fun fork(session: Session, atItemId: String) = source.fork(names.rawSession(session), atItemId)
        override suspend fun rewind(session: Session, target: RewindTarget, restore: RewindRestore) = source.rewind(names.rawSession(session), target, restore)
    }
}

internal fun ChatComposerService.inGroup(names: GroupNames, scope: CoroutineScope): ChatComposerService {
    if (names.plain) return this
    val source = this
    fun delivery(value: ComposerDelivery) = value.copy(refusal = value.refusal?.let { it.copy(session = names.key(it.session)) })
    return object : ChatComposerService {
        override val deliveries = source.deliveries.mapIn(scope) { map -> map.mapValues { delivery(it.value) } }
        override suspend fun upload(session: SessionKey, name: String, file: File) = source.upload(names.raw(session), name, file)
        override suspend fun upload(session: SessionKey, name: String, file: File, destination: String) =
            source.upload(names.raw(session), name, file, destination)
        override suspend fun interrupt(session: SessionKey) = source.interrupt(names.raw(session))
        override suspend fun retryRefused(clientMessageId: String, clearDraft: Boolean) =
            delivery(source.retryRefused(clientMessageId, clearDraft))
    }
}

internal fun ChatRecoveryService.inGroup(names: GroupNames): ChatRecoveryService {
    if (names.plain) return this
    val source = this
    return object : ChatRecoveryService {
        override suspend fun resume(session: Session) = when (val result = source.resume(names.rawSession(session))) {
            is ChatRecoveryResult.Restored -> ChatRecoveryResult.Restored(names.key(result.session))
            else -> result
        }
        override suspend fun reconnect(machineId: String) = source.reconnect(names.rawMachine(machineId))
    }
}

/** One group's machine management as the merged pages use it: plain ids to its hub, the machine it returns suffixed. */
internal fun MachineManagement.inGroup(names: GroupNames): MachineManagement {
    if (names.plain) return this
    val source = this
    return object : MachineManagement {
        override suspend fun launchers(machineId: String) = source.launchers(names.rawMachine(machineId))
        // The machine that read it comes back in the merged ids the pages look machines up by.
        override suspend fun launcherQuota(machineId: String, name: String, refresh: Boolean) =
            source.launcherQuota(names.rawMachine(machineId), name, refresh).let { a -> a.copy(machine = a.machine?.takeIf { it.isNotBlank() }?.let(names::machineId)) }
        override suspend fun reconnect(machineId: String) = source.reconnect(names.rawMachine(machineId))
        override suspend fun startLinkSwitch(machineId: String) = source.startLinkSwitch(names.rawMachine(machineId))
        override suspend fun rename(machineId: String, name: String) = source.rename(names.rawMachine(machineId), name)
        override suspend fun deploy(machineId: String) = source.deploy(names.rawMachine(machineId))
        override suspend fun herdr(machineId: String) = source.herdr(names.rawMachine(machineId))
        override suspend fun startHerdr(machineId: String) = source.startHerdr(names.rawMachine(machineId))
        override suspend fun setEngine(machineId: String, engine: String) = names.machine(source.setEngine(names.rawMachine(machineId), engine))
        override suspend fun botmux(machineId: String) = source.botmux(names.rawMachine(machineId))
        override suspend fun setBotmux(machineId: String, on: Boolean) = source.setBotmux(names.rawMachine(machineId), on)
        override suspend fun claudeSettings(machineId: String) = source.claudeSettings(names.rawMachine(machineId))
        override suspend fun setClaudeSetting(machineId: String, key: String, value: JsonElement) =
            source.setClaudeSetting(names.rawMachine(machineId), key, value)
        override suspend fun codexSettings(machineId: String) = source.codexSettings(names.rawMachine(machineId))
        override suspend fun setCodexSetting(machineId: String, key: String, value: JsonElement) =
            source.setCodexSetting(names.rawMachine(machineId), key, value)
        override suspend fun agySettings(machineId: String) = source.agySettings(names.rawMachine(machineId))
        override suspend fun setAgySetting(machineId: String, key: String, value: JsonElement) =
            source.setAgySetting(names.rawMachine(machineId), key, value)
        override suspend fun claudeDirs(machineId: String) = source.claudeDirs(names.rawMachine(machineId))
        override suspend fun setBlockedClaudeDirs(machineId: String, blocked: List<String>) =
            source.setBlockedClaudeDirs(names.rawMachine(machineId), blocked)
        override suspend fun claudeLiveReplies(machineId: String) = source.claudeLiveReplies(names.rawMachine(machineId))
        override suspend fun setClaudeLiveReplies(machineId: String, on: Boolean) =
            source.setClaudeLiveReplies(names.rawMachine(machineId), on)
        override suspend fun doctor(machineId: String) = source.doctor(names.rawMachine(machineId))
        override suspend fun fixDoctor(machineId: String) = source.fixDoctor(names.rawMachine(machineId))
        override suspend fun browser(machineId: String) = source.browser(names.rawMachine(machineId))
        override suspend fun setBrowser(machineId: String, enabled: Boolean) = source.setBrowser(names.rawMachine(machineId), enabled)
        override suspend fun browserJob(machineId: String, action: String) = source.browserJob(names.rawMachine(machineId), action)
        override suspend fun usageWindows() = source.usageWindows()
        override suspend fun moveToManager(machineId: String, manager: String) =
            source.moveToManager(names.rawMachine(machineId), manager).let { it.copy(machineId = names.machineId(it.machineId)) }
    }
}

internal fun AssistantManagement.inGroup(names: GroupNames): AssistantManagement {
    if (names.plain) return this
    val source = this
    return object : AssistantManagement {
        override fun forManager(manager: String?) = source.forManager(manager).inGroup(names)
        override suspend fun warm() = source.warm()
        // The hub's plain machine ids, as the merged lists name them.
        override suspend fun readiness(probe: Boolean) = source.readiness(probe).let { r ->
            r.copy(machine = r.machine?.let { it.copy(id = names.machineId(it.id)) },
                steps = r.steps.map { step -> step.copy(machine = step.machine?.takeIf(String::isNotBlank)?.let(names::machineId)) })
        }
        override suspend fun dismissReadiness(dismissed: Boolean) = source.dismissReadiness(dismissed)
        override suspend fun config() = source.config().let { it.copy(machineId = names.machineId(it.machineId)) }
        override suspend fun updateConfig(update: AssistantConfigUpdate) =
            source.updateConfig(update.copy(machineId = update.machineId?.let(names::rawMachine)))
                .let { it.copy(machineId = names.machineId(it.machineId)) }
        override suspend fun launchers(machineId: String) = source.launchers(names.rawMachine(machineId))
        override suspend fun assistantLaunchers(machineId: String) = source.assistantLaunchers(names.rawMachine(machineId))
        override suspend fun assistantLauncherList(machineId: String) = source.assistantLauncherList(names.rawMachine(machineId))
        override suspend fun assistantLauncherList(machineId: String, refresh: Boolean) =
            source.assistantLauncherList(names.rawMachine(machineId), refresh)
        override suspend fun launcherModels(machineId: String, launcher: String) = source.launcherModels(names.rawMachine(machineId), launcher)
        override suspend fun setModelPick(session: SessionKey, pick: persona.core.contracts.ModelPick) = source.setModelPick(names.raw(session), pick)
        override suspend fun clearModelPick(session: SessionKey) = source.clearModelPick(names.raw(session))
        override suspend fun endConversation(session: SessionKey) = source.endConversation(names.raw(session))
        override suspend fun compactConversation(session: SessionKey) = source.compactConversation(names.raw(session))
        override suspend fun archiveConversation(session: SessionKey) = source.archiveConversation(names.raw(session))
        override suspend fun unarchiveConversation(session: SessionKey) = source.unarchiveConversation(names.raw(session))
        override suspend fun setConversationTags(session: SessionKey, tags: List<String>) = source.setConversationTags(names.raw(session), tags)
        override suspend fun archivedConversations() = source.archivedConversations().map(names::session)
        override suspend fun searchChats(query: String, limit: Int) = source.searchChats(query, limit)?.let { answer ->
            answer.copy(results = answer.results.map { result ->
                result.copy(machineId = result.machineId.takeIf { it.isNotBlank() }?.let(names::machineId).orEmpty())
            })
        }
        override suspend fun renameConversation(session: SessionKey, title: String) = source.renameConversation(names.raw(session), title)
        override suspend fun makeDirection(session: SessionKey) = source.makeDirection(names.raw(session))
        override suspend fun endDirection(session: SessionKey) = source.endDirection(names.raw(session))
        override suspend fun newMainConversation(carrySummary: Boolean, expect: String?) =
            source.newMainConversation(carrySummary, expect)
        override suspend fun clearMainConversation(expect: String?) = source.clearMainConversation(expect)
        private fun ScheduledTask.local() =
            copy(machineId = machineId?.takeIf { it.isNotBlank() }?.let(names::machineId),
                runMachineId = runMachineId?.takeIf { it.isNotBlank() }?.let(names::machineId))
        override suspend fun tasks() = source.tasks().map { it.local() }
        override suspend fun saveTask(task: ScheduledTask) = source.saveTask(task).local()
        override suspend fun deleteTask(id: String) = source.deleteTask(id)
        override suspend fun runTask(id: String) = names.key(source.runTask(id))
        override suspend fun freshTask(id: String) = source.freshTask(id).local()
        override suspend fun runs() = source.runs().map { it.copy(machineId = it.machineId?.let(names::machineId)) }
        override suspend fun notes() = source.notes().let { it.copy(machineId = names.machineId(it.machineId)) }
        override suspend fun setNotes(text: String) = source.setNotes(text)
        override suspend fun memory() = source.memory()
        override suspend fun saveMemory(name: String, content: String) = source.saveMemory(name, content)
        override suspend fun deleteMemory(name: String) = source.deleteMemory(name)
        override suspend fun places() = source.places()
        override suspend fun savePlace(place: SavedPlace) = source.savePlace(place)
        override suspend fun deletePlace(id: String) = source.deletePlace(id)
        override suspend fun skills() = source.skills().let { it.copy(machineId = names.machineId(it.machineId)) }
        override suspend fun alwaysGrants(session: SessionKey) = source.alwaysGrants(names.raw(session))
        override suspend fun revokeAlwaysGrant(session: SessionKey, keyHash: String) = source.revokeAlwaysGrant(names.raw(session), keyHash)
        override suspend fun standingAuthorizations() = source.standingAuthorizations().let { list ->
            list.copy(authorizations = list.authorizations.map { grant ->
                grant.copy(machineId = grant.machineId.takeIf { it.isNotBlank() }?.let(names::machineId).orEmpty())
            })
        }
        override suspend fun revokeStandingAuthorization(id: String) = source.revokeStandingAuthorization(id)
    }
}

internal fun MainConversationSender.inGroup(names: GroupNames, scope: CoroutineScope): MainConversationSender {
    if (names.plain) return this
    val source = this
    return object : MainConversationSender {
        override val outbox = source.outbox.mapIn(scope) { list -> list.map(names::outbox) }
        override val sendReceipts = source.sendReceipts.mapIn(scope) { list -> list.map(names::receipt) }
        override suspend fun sendToMain(text: String, clientMessageId: String, draft: DraftSnapshot?) =
            sendToMain(text, clientMessageId, draft, null)
        override suspend fun sendToMain(text: String, clientMessageId: String, draft: DraftSnapshot?, via: String?) =
            source.sendToMain(text, clientMessageId, draft?.let(names::rawDraft), via)
        override suspend fun mainReplaced(previous: SessionKey) = source.mainReplaced(names.raw(previous))
        override suspend fun mainChanged() = source.mainChanged()
    }
}

internal fun AssistantConversations.inGroup(names: GroupNames, scope: CoroutineScope): AssistantConversations {
    if (names.plain) return this
    val source = this
    return object : AssistantConversations {
        override val assistant = source.assistant.mapIn(scope) { snapshot ->
            AssistantSnapshot(snapshot.mainSession?.let(names::key), snapshot.conversations.map(names::session), snapshot.manager, snapshot.managerId,
                snapshot.leaders, snapshot.directionNames,
                directionTree = snapshot.directionTree?.mapMachines(names::machineId),
                backgroundSupervisor = snapshot.backgroundSupervisor, recentTaskChats = snapshot.recentTaskChats)
        }
        override val pinned = source.pinned.mapIn(scope) { keys -> keys.mapTo(LinkedHashSet(), names::key) }
        override suspend fun refresh() = source.refresh()
        override suspend fun updateRecentTaskChats(value: Boolean?) = source.updateRecentTaskChats(value)
        override suspend fun recentTaskChatsUpdater() = source.recentTaskChatsUpdater()
        override suspend fun startConversation() = names.key(source.startConversation())
        override suspend fun startConversation(launcher: String) = names.key(source.startConversation(launcher))
        override suspend fun startConversation(pick: persona.core.contracts.ModelPick) = names.key(source.startConversation(pick))
        // another manager's main conversation (docs/assistant/managers.md step 4), in this group's merged ids
        override suspend fun mainConversationOf(manager: String) = names.key(source.mainConversationOf(manager))
        override suspend fun spaceConversations(space: String) = source.spaceConversations(space).map(names::session)
        override suspend fun startConversationIn(space: String) = names.key(source.startConversationIn(space))
    }
}

internal fun AgentOperations.inGroup(names: GroupNames): AgentOperations {
    if (names.plain) return this
    val source = this
    return object : AgentOperations {
        override val scope get() = source.scope
        override suspend fun pins() = names.operationPreferences(source.pins())
        override suspend fun mutes() = names.operationPreferences(source.mutes())
        override suspend fun savePins(value: AgentOperationPreferences) = source.savePins(names.rawOperationPreferences(value))
        override suspend fun saveMutes(value: AgentOperationPreferences) = source.saveMutes(names.rawOperationPreferences(value))
        override suspend fun renameSession(key: SessionKey, title: String?, tabName: String?) =
            source.renameSession(names.raw(key), title, tabName)
        override suspend fun renameSpace(machineId: String, workspaceId: String, name: String) =
            source.renameSpace(names.rawMachine(machineId), workspaceId, name)
        override suspend fun closeSession(key: SessionKey) = source.closeSession(names.raw(key))
        override suspend fun attachSession(key: SessionKey) = source.attachSession(names.raw(key))
        override suspend fun interruptSession(key: SessionKey) = source.interruptSession(names.raw(key))
        override suspend fun deleteSession(key: SessionKey) = source.deleteSession(names.raw(key))
    }
}

internal fun AgentCreationService.inGroup(names: GroupNames): AgentCreationService {
    if (names.plain) return this
    val source = this
    fun result(value: AgentStartResult) = if (value is AgentStartResult.Created) AgentStartResult.Created(names.session(value.session)) else value
    return object : AgentCreationService {
        override val scope get() = source.scope
        override suspend fun launchers(machineId: String) = source.launchers(names.rawMachine(machineId))
        override suspend fun launcherList(machineId: String) = source.launcherList(names.rawMachine(machineId))
        override suspend fun directories(machineId: String, path: String) = source.directories(names.rawMachine(machineId), path)
        override suspend fun start(request: AgentStartRequest, operationId: String) =
            result(source.start(request.copy(machineId = names.rawMachine(request.machineId)), operationId))
        override suspend fun operation(operationId: String) = result(source.operation(operationId))
        override suspend fun newAgentModel() = source.newAgentModel()
        override suspend fun launcherModels(machineId: String, launcher: String, refresh: Boolean) =
            source.launcherModels(names.rawMachine(machineId), launcher, refresh)
        override suspend fun launcherModels(machineId: String, launcher: String, refresh: Boolean, host: String?) =
            source.launcherModels(names.rawMachine(machineId), launcher, refresh, host)
        override suspend fun worktreeStatus(machineId: String, cwd: String, kind: String?) = source.worktreeStatus(names.rawMachine(machineId), cwd, kind)
        override suspend fun addWorktree(machineId: String, cwd: String, nameHint: String, branch: String, kind: String?) =
            source.addWorktree(names.rawMachine(machineId), cwd, nameHint, branch, kind)
        override suspend fun removeWorktree(machineId: String, path: String, deleteBranch: Boolean, force: Boolean, kind: String?) =
            source.removeWorktree(names.rawMachine(machineId), path, deleteBranch, force, kind)
        // Rows name the hub's own machine id: back in the merged one, so the form's filter by machine keeps them.
        override suspend fun workspaces(machineId: String) =
            source.workspaces(names.rawMachine(machineId)).map { it.copy(machineId = names.machineId(it.machineId)) }
        override suspend fun workspaceCatalog(machineId: String) =
            source.workspaceCatalog(names.rawMachine(machineId)).let { catalog ->
                catalog.copy(workspaces = catalog.workspaces.map { it.copy(machineId = names.machineId(it.machineId)) })
            }
    }
}

internal fun AgentsWiringState.inGroup(names: GroupNames, scope: CoroutineScope): AgentsWiringState {
    if (names.plain) return this
    val drafts = drafts
    return copy(
        operations = operations?.let { bindings ->
            // The assistant list is the selected group's, already in merged ids.
            AgentsOperationsBindings(bindings.operations.inGroup(names), bindings.assistant) { key ->
                bindings.onSessionClosed(names.raw(key))
            }
        },
        creation = creation?.inGroup(names),
        archives = archives?.inGroup(names),
        drafts = drafts?.let { source -> AgentsDraftSource { key -> source.observe(names.raw(key)) } },
        mainSender = mainSender?.inGroup(names, scope),
    )
}

internal fun PortOpener.inGroup(names: GroupNames): PortOpener {
    if (names.plain) return this
    val source = this
    return object : PortOpener {
        override suspend fun open(machineId: String, port: Int, path: String, secure: Boolean, listed: Boolean) =
            source.open(names.rawMachine(machineId), port, path, secure, listed)
        override suspend fun probe(machineId: String, port: Int) = source.probe(names.rawMachine(machineId), port)
    }
}

internal fun SessionFiles.inGroup(names: GroupNames): SessionFiles {
    if (names.plain) return this
    val source = this
    return object : SessionFiles {
        override suspend fun directory(machineId: String, cwd: String, path: String) = source.directory(names.rawMachine(machineId), cwd, path)
        override suspend fun file(machineId: String, cwd: String, path: String) = source.file(names.rawMachine(machineId), cwd, path)
        override suspend fun filePreview(machineId: String, cwd: String, path: String) = source.filePreview(names.rawMachine(machineId), cwd, path)
        override suspend fun status(machineId: String, cwd: String) = source.status(names.rawMachine(machineId), cwd)
        override suspend fun diff(machineId: String, cwd: String, path: String, staged: Boolean) =
            source.diff(names.rawMachine(machineId), cwd, path, staged)
        override suspend fun log(machineId: String, cwd: String, skip: Int, limit: Int) = source.log(names.rawMachine(machineId), cwd, skip, limit)
        override suspend fun pull(machineId: String, cwd: String) = source.pull(names.rawMachine(machineId), cwd)
        override suspend fun show(machineId: String, cwd: String, hash: String) = source.show(names.rawMachine(machineId), cwd, hash)
        override suspend fun stat(machineId: String, cwd: String, paths: List<String>) = source.stat(names.rawMachine(machineId), cwd, paths)
        override suspend fun download(machineId: String, cwd: String, path: String, into: java.io.File) =
            source.download(names.rawMachine(machineId), cwd, path, into)
        override suspend fun readLogTail(machineId: String, cwd: String, path: String, manager: String?) =
            source.readLogTail(names.rawMachine(machineId), cwd, path, manager)
        override fun comparesVersions(machineId: String) = source.comparesVersions(names.rawMachine(machineId))
        override suspend fun blob(machineId: String, cwd: String, path: String, rev: String) =
            source.blob(names.rawMachine(machineId), cwd, path, rev)
        override suspend fun thumbnail(machineId: String, cwd: String, path: String, size: Int) =
            source.thumbnail(names.rawMachine(machineId), cwd, path, size)
        override suspend fun convertToPdf(machineId: String, cwd: String, path: String) = source.convertToPdf(names.rawMachine(machineId), cwd, path)
        override fun editsFiles(machineId: String) = source.editsFiles(names.rawMachine(machineId))
        override suspend fun rename(machineId: String, cwd: String, path: String, name: String) =
            source.rename(names.rawMachine(machineId), cwd, path, name)
        override suspend fun move(machineId: String, cwd: String, path: String, directory: String, conflict: SessionMoveConflictPolicy?) =
            source.move(names.rawMachine(machineId), cwd, path, directory, conflict)
        override suspend fun trash(machineId: String, cwd: String, paths: List<String>) = source.trash(names.rawMachine(machineId), cwd, paths)
        override suspend fun restore(machineId: String, trashIds: List<String>) = source.restore(names.rawMachine(machineId), trashIds)
    }
}

/** The inbox items and Persona outbox of the selected group, whose own transports speak plain ids. */
internal fun InboxSnapshot.inGroup(names: GroupNames) = if (names.plain) this else copy(
    records = records.map { record -> record.agent?.let { agent ->
        record.copy(agent = agent.copy(machineId = agent.machineId?.let(names::machineId)))
    } ?: record },
)

/**
 * Keeps [group]'s latest fleet (in its own plain ids, pushed or fetched) translated by [names], forgets groups no
 * longer in [open], and returns the merged fleet of every open group.
 */
internal fun MutableMap<String, FleetState>.putFleet(group: String, names: GroupNames, fleet: FleetState,
    open: List<String>, selected: String?): FleetState {
    this[group] = names.fleet(fleet)
    val ids = (open + group).distinct()
    keys.retainAll(ids.toSet())
    return mergeFleets(ids.mapNotNull { this[it] }, selected?.let { this[it] })
}

/** Forgets the fleets of groups no longer in [open]; the merged fleet of the rest when that dropped one, else null. */
internal fun MutableMap<String, FleetState>.retainFleets(open: List<String>, selected: String?): FleetState? {
    if (!keys.retainAll(open.toSet())) return null
    return mergeFleets(open.mapNotNull { this[it] }, selected?.let { this[it] })
}

/**
 * The fleet the ongoing notification shows, over every group: counts add up, the most urgent phase wins, items
 * follow one another with every group's requests first (iOS FleetMerge), since only the first few are drawn. One
 * group's fleet is passed through as it is.
 */
internal fun mergeFleets(fleets: List<FleetState>, selected: FleetState?): FleetState {
    if (fleets.size == 1) return fleets.single()
    val base = selected ?: fleets.firstOrNull() ?: return FleetState()
    val needsYou = fleets.sumOf { it.needsYou }
    val working = fleets.sumOf { it.working }
    val unread = fleets.sumOf { it.unread }
    val phase = when {
        needsYou > 0 -> "needsYou"
        working > 0 -> "working"
        fleets.any { it.phase == "done" } -> "done"
        else -> base.phase
    }
    val (requests, others) = fleets.flatMap { it.items }.partition(FleetSummary::needsYou)
    return base.copy(phase = phase, needsYou = needsYou, working = working, unread = unread,
        items = requests + others, workingSince = fleets.mapNotNull { it.workingSince }.minOrNull())
}
