@file:OptIn(androidx.compose.material3.adaptive.ExperimentalMaterial3AdaptiveApi::class)

package persona.android.ui

import persona.android.R
import kotlinx.coroutines.flow.first
import androidx.compose.ui.res.stringResource
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.repeatOnLifecycle
import androidx.lifecycle.viewmodel.compose.rememberViewModelStoreOwner
import androidx.lifecycle.viewmodel.navigation3.rememberViewModelStoreNavEntryDecorator
import androidx.navigation3.runtime.*
import androidx.navigation3.ui.NavDisplay
import androidx.compose.material3.adaptive.ExperimentalMaterial3AdaptiveApi
import androidx.compose.material3.adaptive.currentWindowAdaptiveInfo
import androidx.compose.material3.adaptive.layout.PaneScaffoldDirective
import androidx.compose.material3.adaptive.layout.calculatePaneScaffoldDirective
import androidx.compose.material3.adaptive.layout.calculatePaneScaffoldDirectiveWithTwoPanesOnMediumWidth
import androidx.compose.material3.adaptive.navigation3.ListDetailSceneStrategy
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.window.core.layout.WindowSizeClass
import androidx.navigationevent.compose.LocalNavigationEventDispatcherOwner
import androidx.navigationevent.compose.rememberNavigationEventDispatcherOwner
import androidx.compose.ui.layout.layout
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.layout.positionInWindow
import persona.android.MainActivity
import persona.android.data.Hub
import persona.android.navigation.*
import persona.core.contracts.*
import persona.core.contracts.api.ConnectionState
import persona.core.contracts.api.MainConversationSender
import persona.core.contracts.api.MessageSender
import dev.chrisbanes.haze.HazeState
import persona.core.designsystem.*
import persona.core.designsystem.dense
import persona.core.designsystem.icons.PhIcons
import androidx.compose.foundation.background
import persona.core.domain.inbox.buildInboxList
import persona.core.navigation.*
import persona.feature.agents.AgentsCardOpen
import persona.feature.agents.agentsEntries
import persona.feature.agents.machineSpacesEntries
import persona.feature.agents.spaceEntries
import persona.feature.agents.AgentsDependencies
import persona.feature.agents.AgentsPreferencesSnapshot
import persona.feature.agents.AgentsWiring
import persona.feature.assistant.AssistantAppHost
import persona.feature.assistant.AssistantScreen
import persona.feature.assistant.AssistantSidebarHost
import persona.feature.assistant.AssistantSidebarContent
import persona.feature.assistant.LocalAssistantSidebarHost
import persona.feature.assistant.AssistantSidebarShade
import persona.feature.assistant.LocalAssistantHost
import persona.feature.assistant.assistantEntries
import persona.feature.assistant.proactive.AssistantProactiveScreen
import persona.feature.chat.*
import persona.feature.inbox.InboxItemPages
import persona.feature.inbox.InboxItemExtensionScreen
import persona.feature.inbox.InboxItemKey
import persona.feature.inbox.InboxRepository
import persona.feature.inbox.LocalInboxItemPages
import persona.feature.inbox.InboxLanding
import persona.feature.inbox.LocalInboxLanding
import persona.feature.inbox.LocalInboxListActive
import persona.feature.inbox.ManagerLinks
import persona.feature.inbox.InboxTriage
import persona.feature.inbox.InboxTriageSetup
import persona.feature.inbox.LocalInboxTriageSetup
import persona.feature.inbox.inboxEntries
import persona.feature.inbox.inboxItemPage
import persona.feature.inbox.needsYouCount
import persona.feature.inbox.inboxUnreadCount
import persona.feature.inbox.agentWorking
import persona.feature.inbox.inboxSourcePage
import persona.feature.knowledge.*
import persona.feature.machines.LocalSidebarUsage
import persona.feature.machines.SidebarUsageStrip
import persona.feature.machines.machinesEntries
import persona.feature.machines.rememberSidebarUsage
import persona.feature.settings.*
import persona.feature.terminal.terminalEntries
import persona.feature.voice.voiceEntries
import persona.platform.notify.Notify
import persona.platform.phonetools.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@Composable
fun PersonaRoot(
    hub: Hub?,
    openNavigation: MutableState<((Router) -> ResolvedRoute?)?>,
    services: SettingsServices,
    shell: AppServices,
    onPaired: () -> Unit = {},
) {
    val active by shell.session.collectAsStateWithLifecycle()
    val features = persona.core.designsystem.LocalHubFeatures.current
    val api = services.extensions?.takeIf { active != null && active?.demo == null &&
        ("extension_manager" in features || "plugin_manager" in features) }
    persona.feature.settings.ExtensionDeclarationsHost(api, active?.identity.orEmpty()) {
        PersonaRootContent(hub, openNavigation, services, shell, onPaired)
    }
}

@Composable
private fun PersonaRootContent(
    hub: Hub?,
    openNavigation: MutableState<((Router) -> ResolvedRoute?)?>,
    services: SettingsServices,
    shell: AppServices,
    onPaired: () -> Unit = {},
) {
    val session by shell.session.collectAsStateWithLifecycle()
    val toast = persona.core.designsystem.rememberToastState()
    val scope = rememberCoroutineScope()
    // First pairing keeps the pairing flow, including its readiness checklist, until it reports ready.
    var entering by remember { mutableStateOf(hub == null) }
    LaunchedEffect(hub, session) {
        if (session?.demo != null) entering = false
        else if (hub == null) entering = true
    }
    val link = MainActivity.pendingPairLink.value
    // One toast slot for the window, as iOS: the newest notice replaces the one showing; the host plays its haptic.
    LaunchedEffect(shell) { shell.feedback.notices.collect { toast.show(it) } }
    // The GitHub star toast owed since first setup finished (#5761): here, so leaving the checklist for the shell keeps it.
    persona.feature.settings.StarPromptWelcome(toast::show)
    CompositionLocalProvider(LocalAppFeedback provides shell.feedback) {
        Surface(Modifier.fillMaxSize()) {
            Box(Modifier.fillMaxSize()) {
                val active = session
                if (link != null || active == null && hub == null || entering && !shell.automation) {
                    var diagnostics by remember { mutableStateOf(false) }
                    val router = remember { Router() }
                    LaunchedEffect(openNavigation.value, active) {
                        openNavigation.value?.invoke(router)?.let { route ->
                            if (route.key == SettingsPageRoute(SettingsPage.Diagnostics)) diagnostics = true
                            val retainImport = ProviderImportRequests.retainProviderNavigationDuringPairing(route.key)
                            if (route.key == PairingRoute || route.key is SettingsPageRoute && !retainImport) openNavigation.value = null
                        }
                    }
                    // Keep the pairing state in memory while diagnostics is open.
                    PairingScreen(services.pairing, services.sessions, initialLink = link,
                        onReady = { MainActivity.pendingPairLink.value = null; entering = false; onPaired() },
                        onBack = if (link != null && hub != null) ({ MainActivity.pendingPairLink.value = null }) else null,
                        onDiagnostics = { diagnostics = true }, diagnostics = services.diagnostics,
                        onCredentialsSaved = { MainActivity.pendingPairLink.value = null }, machines = services.machines,
                        attempts = services.pairingAttempts, mainReadiness = services.readiness,
                        footer = {
                            if (hub == null) JTextButton(stringResource(R.string.app_try_demo), onClick = { scope.launch {
                                try { shell.enterDemo(); MainActivity.pendingPairLink.value = null }
                                catch (error: Exception) {
                                    android.util.Log.w("PersonaShell", "Unable to start demo", error)
                                    shell.feedback.failure(error)
                                }
                            } })
                        })
                    if (diagnostics) {
                        val close = { diagnostics = false }
                        JFullScreen(onDismiss = close) { SettingsScreen(services, initialPage = "diag", onBack = close) }
                    }
                } else if (active == null) {
                    // The pairing is restored; its repositories are still being assembled.
                    Box(Modifier.fillMaxSize().background(PersonaTheme.palette.background), contentAlignment = Alignment.Center) { JLoading() }
                } else key(active.identity) {
                    val displayed = LocalDeveloperTools.current?.session(active) ?: active
                    key(displayed.identity) {
                        // Demo mode has only the default face (docs/apple/persona.md), like iOS.
                        val demo = displayed.demo != null
                        // What the pages' connection titles open (ConnectionDetails.kt): this hub's transport and outbox.
                        val connectionDetails = remember(displayed) {
                            ConnectionDetailsSource(
                                diagnostics = (displayed.connection as? persona.core.contracts.api.ConnectionDiagnosticsSource)?.diagnostics,
                                outbox = displayed.sender.outbox,
                                reconnect = displayed.connection::reconnect,
                            )
                        }
                        CompositionLocalProvider(
                            LocalConnectionDetails provides connectionDetails,
                            persona.core.designsystem.LocalPersonaDesign provides if (demo) persona.core.designsystem.PersonaDesign.Default else persona.core.designsystem.LocalPersonaDesign.current,
                            persona.core.designsystem.LocalPersonaPack provides if (demo) null else persona.core.designsystem.LocalPersonaPack.current,
                            persona.core.designsystem.LocalPersonaName provides if (demo) "Persona" else persona.core.designsystem.LocalPersonaName.current,
                            // The demo serves its own Feishu chats, whatever the paired hub has.
                            persona.core.designsystem.LocalLarkFeatures provides if (demo) persona.core.contracts.LarkFeatures.All else persona.core.designsystem.LocalLarkFeatures.current,
                            // The demo's mail and chats are made up: nothing to send to a hub for translating.
                            persona.feature.inbox.LocalTranslator provides if (demo) null else persona.feature.inbox.LocalTranslator.current,
                            // A reply's ```persona-view / ```persona-card drawn with the extensions' components (#1851).
                            persona.core.designsystem.LocalViewBlocks provides persona.feature.settings.PersonaViewBlocks.blocks,
                        ) {
                            SavedPromptsHost(displayed.savedPromptsConnection) {
                                PersonaShell(displayed, openNavigation, services.copy(connection = displayed.connection, sessions = displayed.sessions, demo = displayed.demo != null), remember(shell) { AppShellHost(shell) })
                            }
                        }
                    }
                    active.phone?.let { PhoneConfirmationHost(it.confirmations) }
                }
                persona.core.designsystem.JToastHost(toast, Modifier.align(Alignment.TopCenter).statusBarsPadding().padding(top = Tokens.Space.m))
            }
        }
    }
}

@Composable
internal fun PersonaShell(session: ShellSession, openNavigation: MutableState<((Router) -> ResolvedRoute?)?>, shellServices: SettingsServices, shell: ShellHost) {
    // Extension page badges live on the sidebar's Settings entry, never on a third home tab.
    val extensionSlots = persona.feature.settings.rememberExtensionSlots(shellServices.extensions?.takeUnless { shellServices.demo }, session.connection.state)
    val services = remember(shellServices, extensionSlots, session.preferences) {
        // The Agents list's display, as its ⋯ › Display sets it: Settings › Appearance offers the same choice.
        shellServices.copy(extensionSlots = extensionSlots, agentsDisplay = session.preferences)
    }
    val slotsState = extensionSlots?.state?.collectAsStateWithLifecycle()?.value
    val slotBadge = slotsState?.tabBadge ?: persona.feature.settings.SlotBadge()
    val context = LocalContext.current
    val store = remember(session.identity) { ShellPositionStore(context, session.identity) }
    val appearance = LocalAppStyleStore.current
    val merged = appearance?.sidebar?.collectAsState()?.value == SidebarLayout.Merged
    val currentMerged = rememberUpdatedState(merged)
    val agentsListOwner = key(session.identity) { rememberViewModelStoreOwner() }
    // Automated captures start from each tab's root instead of whatever the previous capture left open;
    // they still save, since scripts/android.sh waits for the saved position to know the shell is up.
    val saved = remember(store) { if (shell.automation) ShellPosition() else store.load() }
    // With the embedded assistant chat (not in demo), assistant conversations open inside the Persona home.
    val embeddedAssistant = session.demo == null
    fun hubRouter(kind: (String, String) -> ConversationKind?) = Router(
        hubLink = { link ->
            resolveHubRoute(link, session.hubAddresses(), session.groups.groups.value.firstOrNull { it.selected }?.namespace,
                session.sessions.sessions.value.map { it.key }, session.assistant.assistant.value, session.groups.groups.value)
        },
        conversationKind = kind,
    )
    val initialLaunch = remember { openNavigation.value }
    val initialRoute = remember {
        initialLaunch?.invoke(hubRouter { m, s ->
            val key = SessionKey(m, s)
            val assistant = session.assistant.assistant.value
            if (key == assistant.mainSession || assistant.conversations.any { it.key == key }) ConversationKind.Assistant
            else session.sessions.sessions.value.firstOrNull { it.key == key }?.let {
                if (it.assistant) ConversationKind.Assistant else ConversationKind.Agent
            }
        })?.takeIf { route ->
            route.hubGroupId == null || route.hubGroupId == session.groups.groups.value.firstOrNull { it.selected }?.id
        }
    }
    var selected by rememberSaveable { mutableStateOf(initialRoute?.tab ?: if (session.demo != null) AppTab.Persona else saved.tab) }
    var machine by rememberSaveable { mutableStateOf(saved.machine) }
    val assistantStack = rememberNavBackStack(AssistantRoute,
        *listOfNotNull(saved.chats[AppTab.Persona]?.takeIf { !embeddedAssistant }).toTypedArray())
    val agentsStack = rememberNavBackStack(AgentsRoute, *listOfNotNull(saved.chats[AppTab.Agents]).toTypedArray())
    // Each regular-width root keeps its own stack. Persona points at whichever one is in front.
    val hosts = remember { SidebarRootHosts(assistantStack) }
    val stacks = remember {
        mutableStateMapOf<AppTab, MutableList<NavKey>>(AppTab.Persona to assistantStack, AppTab.Agents to agentsStack)
    }
    // Not saveable: pageRoots is remember, so a saved flag would skip adopt after process death (item 7).
    val adoptedVisibleRoot = remember { mutableStateOf(false) }
    // Pending only while the window size is unknown. A known compact window leaves nothing pending (#9667).
    val window = remember { SidebarPlaceWindow() }
    val sidebar = remember { AssistantSidebarHost() }
    val sidebarOperations = rememberCoroutineScope()
    val sidebarState = rememberSaveableStateHolder()
    var settingsOpened by rememberSaveable { mutableStateOf(false) }
    // The sidebar's column beside the Assistant page in a regular window; its avatar hides it and shows it again.
    // Both beside-the-page column choices start from what was kept and only persist when toggled beside the page.
    val rememberColumns = !shell.automation && session.demo == null
    val keptSidebarColumnShown = rememberSaveable { mutableStateOf(if (rememberColumns) saved.sidebarColumnShown else true) }
    val keptAgentsListShown = rememberSaveable { mutableStateOf(if (rememberColumns) saved.agentsListShown else true) }
    val columnMemory = remember(store, rememberColumns) {
        ColumnMemory(keptSidebarColumnShown.value, keptAgentsListShown.value, persist = rememberColumns)
    }
    val sidebarColumnShown = rememberSaveable { mutableStateOf(columnMemory.sidebarShown) }
    val agentsListShown = rememberSaveable { mutableStateOf(columnMemory.agentsShown) }
    // Overlays close when the tab or beside/over placement changes; the beside-column choice stays saved.
    val sidebarOverlayShown = remember { mutableStateOf(false) }
    val agentsOverlayShown = remember { mutableStateOf(false) }
    val pageRoots = remember { SidebarPageRoots() }
    fun dismissSidebar() {
        sidebar.opened.value = false
        sidebarOverlayShown.value = false
    }
    val settingsSheetStack = rememberNavBackStack(SettingsRoute)
    persona.android.navigation.TrackDevScreen(selected, stacks.getValue(selected).lastOrNull(), settingsOpened, settingsSheetStack.lastOrNull())
    // Where each stack's Inbox list goes when an item is opened from outside it (#9053): one per stack, so a list kept
    // composed in a hidden tab never takes another stack's landing.
    val inboxLandings = remember(session.identity) { AppTab.entries.associateWith { InboxLanding() } }
    fun showAssistantHome() {
        window.clearPending()
        hosts.showConversation()
        while (assistantStack.size > 1) assistantStack.removeAt(assistantStack.lastIndex)
        stacks[AppTab.Persona] = assistantStack
        pageRoots.adoptVisible(AssistantRoute)
        // The Inbox list this closed, if any, never takes its landing; the next entry opens on Needs you.
        inboxLandings.getValue(AppTab.Persona).clear()
        selected = AppTab.Persona
    }
    /** Point Persona's navigator at the host now in front. */
    fun showHost(root: NavKey, next: List<NavKey>) {
        hosts.activate(root, next)
        stacks[AppTab.Persona] = hosts.current
    }
    fun sidebarDestination(key: NavKey) {
        dismissSidebar()
        window.clearPending()
        selected = AppTab.Persona
        // Regular: SidebarPageRoots.select, then the Inbox enters or lands (inboxPicked,
        // AssistantView.swift:633–640). Compact: the drawer keeps a host, and an Inbox item on it
        // (show, AssistantView.swift:567–569). applySidebarDestination marks the root.
        applySidebarDestination(
            key, window.regular || currentMerged.value, pageRoots, hosts, assistantStack, ::showHost,
            { stacks[AppTab.Persona] = it },
            inboxLandings.getValue(AppTab.Persona),
            session.inbox.snapshot.value.records,
        )
    }
    /**
     * A route to a place at regular width. Before the window is measured this remembers [key] and lets the
     * compact push stand; the first regular frame applies it before promoting whatever sits above the conversation.
     * Returns true when the caller must not also push.
     */
    fun openPlace(key: NavKey): Boolean = openSidebarPlace(
        key, window, pageRoots, hosts, ::showHost, { stacks[AppTab.Persona] = it },
    ) { selected = AppTab.Persona }
    /**
     * The stack is only a place root. Switch to the conversation without a push or a pop
     * (`leavePlace`, AssistantView.swift:682–686; `assistantReveal`, AppModel.swift:1381 and :1393;
     * `onChange` calls `leavePlace()` at AssistantView.swift:446).
     */
    fun leavePlaceRoot(): Boolean = window.leavePlaceRoot(
        persona = selected == AppTab.Persona,
        stack = stacks.getValue(AppTab.Persona),
        pageRoots = pageRoots,
        showHost = ::showHost,
        allowCompactRoot = true,
    )
    val sessions by session.sessions.sessions.collectAsStateWithLifecycle()
    val assistant by session.assistant.assistant.collectAsStateWithLifecycle()
    val pending by session.interactions.pending.collectAsStateWithLifecycle()
    // A request that just arrived is felt while the app is in front, as on iOS (DESIGN_SYSTEM.md 4.3).
    val haptics = rememberHaptics()
    val lifecycle = androidx.lifecycle.compose.LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(session.interactions, lifecycle) {
        lifecycle.repeatOnLifecycle(androidx.lifecycle.Lifecycle.State.STARTED) {
            session.interactions.arrivals.collect { haptics(Haptic.Attention) }
        }
    }
    val inbox by session.inbox.snapshot.collectAsStateWithLifecycle()
    val connection by session.connection.state.collectAsStateWithLifecycle()
    val sidebarUsage = rememberSidebarUsage(session.machines, active = merged)
    val terminalsAvailable = remember(shell.terminalPanes, session.demo) {
        shell.terminalPanes?.available?.takeIf { session.demo == null } ?: kotlinx.coroutines.flow.flowOf(false)
    }
    // With several hub groups paired the Persona menu's heading ends with the selected group's name, as on iOS.
    val hubGroups by session.groups.groups.collectAsStateWithLifecycle()
    val preferences by session.preferences.snapshots.collectAsStateWithLifecycle(AgentsPreferencesSnapshot())
    val todos by (session.tasks?.todos ?: kotlinx.coroutines.flow.MutableStateFlow(emptyList())).collectAsStateWithLifecycle()
    val todosSupported by (session.tasks?.todosSupported ?: kotlinx.coroutines.flow.MutableStateFlow(false)).collectAsStateWithLifecycle()
    // Every page shows the current assistant (docs/assistant/managers.md "Clients"): the counts beside them are its too.
    val currentAssistant = persona.core.designsystem.currentAssistant()
    val counts = remember(currentAssistant, sessions, preferences.mutes) { ShellCounts(currentAssistant, sessions, preferences.mutes) }
    val uncompletedTodosCount = remember(todos, counts) { counts.todos(todos) }
    fun kind(key: SessionKey): ConversationKind? = when {
        key == assistant.mainSession || assistant.conversations.any { it.key == key } -> ConversationKind.Assistant
        else -> sessions.firstOrNull { it.key == key }?.let { if (it.assistant) ConversationKind.Assistant else ConversationKind.Agent }
    }
    val currentKind by rememberUpdatedState<(SessionKey) -> ConversationKind?>(::kind)
    var assistantRequest by remember { mutableStateOf<SessionKey?>(null) }
    var assistantPending by remember { mutableStateOf<SessionKey?>(null) }
    fun assistantTarget(key: NavKey): SessionKey? = assistantHomeTarget(key, embeddedAssistant, currentKind)
    /** Show an assistant conversation in the Persona home rather than as a second chat page above it. */
    fun openAssistant(key: NavKey, select: Boolean): Boolean {
        val conversation = assistantTarget(key) ?: return false
        if (select) {
            window.clearPending()
            if (window.regular || currentMerged.value) {
                // Another conversation while this one is in front starts at its root, with no pop
                // (SidebarPageRoots.select contentChanged, SidebarPageRoots.swift:90–91).
                val step = pageRoots.select(AssistantRoute, hosts.current.drop(1), contentChanged = true)
                showHost(step.root, step.stack)
            } else {
                hosts.showConversation()
                while (assistantStack.size > 1) assistantStack.removeAt(assistantStack.lastIndex)
                stacks[AppTab.Persona] = assistantStack
            }
            selected = AppTab.Persona
        }
        assistantPending = conversation
        return true
    }
    val visibleAgentChat = remember { kotlinx.coroutines.flow.MutableStateFlow<SessionKey?>(null) }
    val heldAgentChat = remember { kotlinx.coroutines.flow.MutableStateFlow<SessionKey?>(null) }
    val finishedAgentChat = remember { kotlinx.coroutines.flow.MutableStateFlow<SessionKey?>(null) }
    val agentPreferences = remember(session) { session.preferences.snapshots }
    val navigator = remember { AppNavigator(stacks, { selected }, { selected = it }, chatTab = { route ->
        currentKind(SessionKey(route.machineId, route.sessionId))?.let { if (it == ConversationKind.Assistant) AppTab.Persona else AppTab.Agents }
    }, openAssistant = { chat, select -> openAssistant(chat, select) }, inboxReturns = saved.inboxReturns,
        openPlace = { openPlace(it) }, leavePlaceRoot = { leavePlaceRoot() },
        onInboxEntered = { tab -> inboxLandings.getValue(tab).enter() }) }
    fun openResolved(route: ResolvedRoute): AppTab {
        // The restored ledger can hold the source even when the visible key does not. Check it before this direct open
        // updates the ledger: a sourced assistant stays a secondary page instead of replacing the embedded home.
        val hadInboxOrigin = navigator.hasInboxOrigin(route.key)
        navigator.noteDirectOpen(route.key)
        if (currentMerged.value && route.key == AssistantRoute) {
            showAssistantHome()
            return AppTab.Persona
        }
        if (!hadInboxOrigin && openAssistant(route.key, true)) return AppTab.Persona
        if (openPlace(route.key)) {
            // A widget, a quick action or a link to the Inbox enters it: on Needs you, also when a list of it sat under
            // other pages with another choice (#9053).
            if (route.key == InboxRoute) inboxLandings.getValue(AppTab.Persona).enter()
            return AppTab.Persona
        }
        val tab = route.open(stacks)
        if (tab == AppTab.Agents && currentMerged.value) {
            when (route.key) {
                is ChatRoute, is InteractionRoute, is TerminalRoute -> dismissSidebar()
            }
            keepMergedAgentTarget(agentsStack, route.key)
        }
        if (route.key == InboxRoute) inboxLandings.getValue(tab).enter()
        return tab
    }
    /** Opens [route]; a card outside any session (#4808) also has Agents open its sheet, as its notification does. */
    fun openWithCard(route: ResolvedRoute, fromPage: Boolean = false) {
        if (currentMerged.value && fromPage && route.card == null) {
            navigator.navigate(route.key)
        } else {
            selected = openResolved(route)
        }
        route.card?.let(AgentsCardOpen::request)
    }
    remember {
        initialRoute?.let { route ->
            navigator.noteDirectOpen(route.key)
            // The first frame already shows the requested conversation, not the home's default one.
            assistantTarget(route.key)?.let { assistantRequest = it } ?: run {
                if (!openPlace(route.key)) {
                    val tab = route.open(stacks)
                    if (tab == AppTab.Agents && currentMerged.value) keepMergedAgentTarget(agentsStack, route.key)
                }
            }
            route.pairingLink?.let { MainActivity.pendingPairLink.value = it }
            if (openNavigation.value === initialLaunch) openNavigation.value = null
        }
        true
    }
    LaunchedEffect(assistantPending) {
        val conversation = assistantPending ?: return@LaunchedEffect
        // Asking again for the conversation already requested must still reopen it in the home.
        if (assistantRequest == conversation) { assistantRequest = null; withFrameNanos { } }
        assistantRequest = conversation
        assistantPending = null
    }
    val router = remember(session) { hubRouter { m, s -> currentKind(SessionKey(m, s)) } }
    var hubSelectionFrom by remember { mutableStateOf<String?>(null) }
    val agentsWiring by session.agentsWiring.state.collectAsStateWithLifecycle()
    val navigationWiring = remember { MutableStateFlow(agentsWiring) }
    SideEffect {
        navigationWiring.value = agentsWiring.copy(
            operations = agentsWiring.operations?.let { bindings ->
                bindings.copy(onSessionClosed = { key -> bindings.onSessionClosed(key); navigator.sessionClosed(key) })
            },
            tasks = session.tasks,
        )
    }
    val agents = remember(session, navigator) {
        object : AgentsDependencies by session.agents {
            override fun wiring() = object : AgentsWiring { override val state = navigationWiring }
            override fun visibleChat() = visibleAgentChat
            override fun heldChat() = heldAgentChat
            override fun finishedChat() = finishedAgentChat
        }
    }
    val assistantNavigator = remember {
        object : Navigator by navigator {
            override fun navigate(key: NavKey) {
                // Ignore late automatic navigation from an assistant entry that is leaving the screen.
                if (selected == AppTab.Persona) navigator.navigate(key)
            }
        }
    }
    val pendingHubLink = MainActivity.pendingHubLink.value
    LaunchedEffect(sessions, assistant, openNavigation.value, pendingHubLink, session.identity) {
        navigator.restoreChatTabs()
        val fromActivity = openNavigation.value != null
        val route = if (fromActivity) openNavigation.value?.invoke(router)
            else pendingHubLink?.let { router.parse(RouteInput.DeepLink(it)) }
        if (route == null) return@LaunchedEffect
        val selectedGroup = session.groups.groups.value.firstOrNull { it.selected }?.id
        val targetGroup = route.hubGroupId
        if (targetGroup != null && targetGroup != selectedGroup) {
            hubSelectionFrom = session.identity
            session.groups.select(targetGroup)
            return@LaunchedEffect
        }
        // Selection publishes a replacement session. Do not open another hub's page with the old one's assistant state.
        if (hubSelectionFrom != null && hubSelectionFrom == session.identity) return@LaunchedEffect
        settingsOpened = false
        sidebar.opened.value = false
        selected = openResolved(route)
        route.pairingLink?.let { link -> MainActivity.pendingPairLink.value = link }
        hubSelectionFrom = null
        if (fromActivity) openNavigation.value = null else MainActivity.pendingHubLink.value = null
    }
    LaunchedEffect(store) {
        snapshotFlow {
            ShellPosition(
                selected,
                machine,
                stacks.mapNotNull { (tab, stack) ->
                    stack.mapNotNull { when (it) { is ChatRoute -> it; is InteractionRoute -> it.chat; else -> null } }
                        .lastOrNull()?.let { tab to it }
                }.toMap(),
                navigator.inboxReturns(),
                columnMemory.sidebarShown,
                columnMemory.agentsShown,
            )
        }.distinctUntilChanged().collect { position -> withContext(Dispatchers.IO) { store.save(position) } }
    }
    val visible = (if (selected == AppTab.Persona) hosts.current else stacks.getValue(selected)).lastOrNull()
    val visibleChat = when (visible) { is ChatRoute -> visible; is InteractionRoute -> visible.chat; else -> null }
    LaunchedEffect(visibleChat) {
        val agent = visibleChat?.takeIf { it.kind != ConversationKind.Assistant }?.let { SessionKey(it.machineId, it.sessionId) }
        // Capture on entry, before ChatScreen's receipt or a conflated hub snapshot turns the row idle (iOS markSeen). A Just
        // finished row (#9786) is held the same way, in its own place: the hub's list loses the flag once the seen is recorded.
        val row = agent?.let { key -> sessions.firstOrNull { it.key == key } }
        val unread = row?.agentState == AgentState.Unread
        val finished = row?.agentState == AgentState.Idle && row.work?.justFinished == true
        val unseen = row != null && (unread || finished) &&
            agentPreferences.first().seen[row.key.path] != "${row.updatedAt.orEmpty()}|${row.rev ?: ""}"
        heldAgentChat.value = agent?.takeIf { unseen && unread }
        finishedAgentChat.value = agent?.takeIf { unseen && finished }
        visibleAgentChat.value = agent
    }
    DisposableEffect(visibleChat, session.demo) {
        if (session.demo == null) Notify.viewing(visibleChat?.let { SessionKey(it.machineId, it.sessionId) })
        onDispose { if (session.demo == null) Notify.viewing(null) }
    }
    /** Swap a placeholder destination (an inbox deep link) for the page that shows it, keeping its position. */
    fun replace(from: NavKey, to: NavKey) {
        val seen = java.util.IdentityHashMap<MutableList<NavKey>, Unit>()
        (hosts.allStacks() + listOf(agentsStack)).forEach { stack ->
            if (seen.put(stack, Unit) == null) stack.indexOf(from).takeIf { it >= 0 }?.let { stack[it] = to }
        }
    }
    // The pages extensions draw for their connectors' inbox items (inbox_item, #1683): the inbox list and inbox links
    // open those. An item whose page broke opens its own page, which says so until "Retry" tries the extension's page again.
    val nativeInboxChats = persona.core.designsystem.LocalAppStyleStore.current?.nativeInboxChats?.collectAsState()?.value ?: true
    val currentNativeInboxChats by rememberUpdatedState(nativeInboxChats)
    val itemPagePlaces = slotsState?.inboxItems.orEmpty()
    val itemPageFailures = remember(session.identity) { mutableStateMapOf<String, String>() }
    val currentItemPlaces by rememberUpdatedState(itemPagePlaces)
    val currentRecords by rememberUpdatedState(inbox.records)
    val retryItemPage = remember(itemPageFailures) {
        { from: NavKey, itemId: String ->
            itemPageFailures.remove(itemId)
            currentRecords.firstOrNull { it.item.id == itemId }?.let { InboxItemPages(currentItemPlaces, nativeChats = currentNativeInboxChats).route(it) }?.let { replace(from, it) }
            Unit
        }
    }
    // An item opened before the hub's list was read (at launch, before its hello) opens its own page; once the list
    // comes, those its connector's extension draws get the extension's page in their place, as if opened now.
    val itemPagesKnown = extensionSlots == null || slotsState?.inboxItemsKnown == true
    val provisionalPages = remember(session.identity) { mutableSetOf<NavKey>() }
    val notePage = remember(provisionalPages) { { page: NavKey -> provisionalPages += page; Unit } }
    val inboxItemPages = InboxItemPages(itemPagePlaces, itemPageFailures.toMap(), retryItemPage, itemPagesKnown, notePage, nativeChats = nativeInboxChats)
    // A landing follows the Inbox host, which stays composed while another root is in front (#9667).
    // The visible stack alone would clear it on the way to To-dos or the conversation. A page that
    // moved the list beside it still puts its choice back when that page leaves the Inbox host (#9053).
    LaunchedEffect(inboxLandings) {
        snapshotFlow {
            hosts.visited.toList()
            hosts.current
            hosts.allStacks().map { it.toList() } to stacks.mapValues { (_, stack) -> stack.toList() }
        }.collect { (personaHosts, held) -> settleLandings(held, inboxLandings, personaHosts) }
    }
    LaunchedEffect(itemPagesKnown) {
        if (!itemPagesKnown) return@LaunchedEffect
        val pages = InboxItemPages(currentItemPlaces, nativeChats = currentNativeInboxChats)
        val seen = java.util.IdentityHashMap<MutableList<NavKey>, Unit>()
        (hosts.allStacks() + listOf(agentsStack)).forEach { stack ->
            if (seen.put(stack, Unit) == null) pages.swaps(stack.toList(), provisionalPages, currentRecords).forEach { (from, to) -> replace(from, to) }
        }
        provisionalPages.clear()
    }
    // An extension enabled since the hub was last read draws its items as soon as the inbox shows.
    LaunchedEffect(selected, hosts.current.lastOrNull(), agentsStack.lastOrNull(), extensionSlots) {
        val stack = if (selected == AppTab.Persona) hosts.current else stacks.getValue(selected)
        if (stack.any { it == InboxRoute }) extensionSlots?.loadInboxItems()
    }
    // The inbox's triage banner (#1773): "Configure" opens Settings › Models › "Message filtering", offered only when this device can read that
    // model use (a device that may only read and answer cannot, nor a hub without it). Asked once per hub when the
    // banner wants it; a yes is kept.
    val triageSetup = remember(services.models, services.demo, session.identity) {
        val models = services.models.takeUnless { services.demo }
        var allowed = false
        InboxTriageSetup(
            mayConfigure = {
                allowed || (models != null && try { models.purpose(InboxTriage.Purpose); true }
                    catch (cancelled: CancellationException) { throw cancelled }
                    catch (_: Exception) { false }).also { allowed = it }
            },
            configure = { navigator.navigate(ModelPurposeRoute(InboxTriage.Purpose)) },
        )
    }
    val tools = LocalDeveloperTools.current
    val decorators = AppTab.entries.associateWith { tab -> key(tab) {
        listOf(rememberSaveableStateHolderNavEntryDecorator<NavKey>(), rememberViewModelStoreNavEntryDecorator<NavKey>())
    } }
    // The current assistant's cards (ShellCounts): the tab badges and "Needs you now" count them.
    val shownPending = pending.filter(counts::owned)
    val assistantRequests = shownPending.filter { currentKind(SessionKey(it.machineId, it.sessionId)) == ConversationKind.Assistant }
    val agentRequests = shownPending - assistantRequests.toSet()
    // The Agents tab counts its waiting agents, one per session (ShellCounts.agentsWaiting), as iOS and the web do.
    val agentRows = counts.agentRows(agentRequests)
    val agentNeeds = agentRows.size
    // What waits at each assistant, for the switcher's badges and the dot on the sidebar's button: its agents waiting
    // (each once), counted here, and its inbox's "Needs you" from the roster.
    val managerRoster = persona.core.designsystem.LocalManagerRoster.current
    val waiting = remember(managerRoster, pending, counts) { counts.waiting(managerRoster, pending) }
    val othersWaiting = currentAssistant.othersWaiting(waiting)
    // Only when what the count reads changed: the shell recomposes on every push, and the whole list is built to count
    // one group. Of the sessions it reads only which space each is in (for mutes), not their state or activity.
    val inboxSpaces = inbox.sessions.map { it.id to it.spacePreferenceKey }
    val shownRecords = remember(inbox.records, currentAssistant) { ShellCounts.records(currentAssistant, inbox.records) }
    val shownInteractions = remember(inbox.interactions, currentAssistant) { ShellCounts.interactions(currentAssistant, inbox.interactions) }
    val inboxNeeds = remember(shownRecords, shownInteractions, inboxSpaces, inbox.mutes) {
        // Rows the AI twin is on are Persona's: the same count as the inbox page's "Needs you" (#4848).
        needsYouCount(
            buildInboxList(shownRecords.map { it.item }, shownInteractions, inbox.sessions, inbox.mutes), shownRecords)
    }
    val inboxConversations by session.inbox.conversationInbox.collectAsStateWithLifecycle()
    val inboxConversationList by (inboxConversations?.state ?: remember {
        kotlinx.coroutines.flow.MutableStateFlow(persona.feature.inbox.InboxConversationList())
    }).collectAsStateWithLifecycle()
    val inboxUnread = remember(shownRecords, inboxConversationList) {
        inboxUnreadCount(shownRecords, inboxConversationList)
    }
    val badges = mapOf(AppTab.Agents to agentNeeds, AppTab.Persona to assistantRequests.size)
    // "Needs you now" on the Persona home: the agents waiting (not Persona's own, not muted), then the inbox's "Needs you".
    val needsMessages = remember(shownRecords) {
        shownRecords.map { persona.feature.assistant.NeedsYouMessage(it.item, lark = it.feedId != null, working = it.agentWorking) }
    }
    val needsYou = remember(agentRequests, sessions, preferences.mutes, needsMessages) {
        persona.feature.assistant.needsYou(agentRequests, sessions, preferences.mutes, needsMessages)
    }
    // On Glass over the blank page's composer (#4339 C17): the sessions the Agents tab's badge counts, then the inbox's.
    val shellNeedsYou = remember(agentRows, sessions, needsMessages) {
        persona.feature.assistant.needsYouBySession(agentRows, sessions, needsMessages)
    }
    // Voice chat is offered while the hub runs a voice extension: asked each time the Persona tab shows while connected.
    val voiceExtension = persona.feature.voice.rememberVoiceExtension(session.voice,
        active = selected == AppTab.Persona && connection == ConnectionState.Online)
    // Links over the whole shell, persona:// links from conversations, "Hand to Persona" from pages (ShellLinks.kt).
    ShellLinkHost(session, navigator, shell.feedback) {
        val uriHandler = LocalUriHandler.current
        // Each page shows the connection state in its own title, so the shell adds no banner above it.
        val style = PersonaTheme.style
        val connected = connection == ConnectionState.Online
        val working = sessions.any { it.agentState == AgentState.Working && currentAssistant.ownsOn(it.machineId, it.manager) }
        val ambience = when {
            !connected && connection != ConnectionState.Connecting && connection != ConnectionState.Updating -> Ambience.Alert
            agentNeeds + inboxNeeds > 0 -> Ambience.NeedsYou
            working -> Ambience.Working
            else -> Ambience.Calm
        }
        val demoBadge = if (session.demo != null) stringResource(R.string.app_demo_badge) else null
        val items = navigationItems(badges)
        val extensionPages = slotsState?.navPages.orEmpty()
        val sidebarPages = listOfNotNull(
            NavItem("machines", stringResource(R.string.app_machines), PhIcons.Desktop).takeIf { !merged || session.metrics != null },
        ) +
            extensionPages.map { page ->
                NavItem(extensionRailKey(page.extension, page.id), page.title.ifBlank { page.id }, extensionNavIcon(page.icon),
                    badge = page.badge?.text?.trim()?.toIntOrNull()?.coerceAtLeast(0) ?: 0,
                    dot = page.badge?.text?.trim()?.let { it.isNotEmpty() && it.toIntOrNull() == null } == true)
            }
        // The window decides the layout (WindowSizeClass and the fold posture), never the device model: split screen,
        // free-form windows, rotation and folding all just recompose this.
        val windowSize = LocalWindowInfo.current.containerSize
        val adaptive = currentWindowAdaptiveInfo().let { real -> shell.posture.value?.let { pretendFold(real, it, windowSize) } ?: real }
        val expanded = adaptive.windowSizeClass.isWidthAtLeastBreakpoint(WindowSizeClass.WIDTH_DP_EXPANDED_LOWER_BOUND)
        val hinge = adaptive.windowPosture.hingeList.firstOrNull { !it.isVertical && it.isSeparating }
        val windowLayout = WindowLayout(expanded = expanded, tabletop = adaptive.windowPosture.isTabletop && hinge != null,
            hingeTop = hinge?.bounds?.top ?: 0f, hingeBottom = hinge?.bounds?.bottom ?: 0f)
        val windowWidth = with(LocalDensity.current) { LocalWindowInfo.current.containerSize.width.toDp() }
        val windowHeight = with(LocalDensity.current) { LocalWindowInfo.current.containerSize.height.toDp() }
        // A regular window (an open foldable, a tablet) is laid out as an open iPhone Duo or an iPad: the shared sidebar
        // is a column beside the Assistant page, and Settings shows its list beside the page it opens (RegularWindow.kt).
        val regular = isRegularWindow(windowWidth.value, windowHeight.value)
        // Zero size is not compact: the first frame can be unmeasured, and a place route then stays pending.
        SideEffect { window.noteSize(windowWidth.value, windowHeight.value) }
        // Once, from the restored visible stack. A saved flag would skip this after process death, because
        // pageRoots is remember and resets with it (item 7). Offscreen place hosts are not restored.
        SideEffect {
            if (!adoptedVisibleRoot.value) {
                adoptedVisibleRoot.value = true
                pageRoots.adoptVisible(hosts.current.firstOrNull() ?: AssistantRoute)
            }
        }
        // Width changes run in SidebarRootsRuntime, beside the host displays.
        var seenAssistant by remember { mutableStateOf<String?>(null) }
        LaunchedEffect(currentAssistant.id) {
            val previous = seenAssistant
            seenAssistant = currentAssistant.id
            if (previous == null || previous == currentAssistant.id) return@LaunchedEffect
            // Another assistant forgets every root, at any width (SidebarPageRoots.forget, SidebarPageRoots.swift:108–111).
            window.clearPending()
            pageRoots.forget()
            hosts.forgetPlaces()
            replaceBackStack(assistantStack, listOf(AssistantRoute))
            stacks[AppTab.Persona] = assistantStack
        }
        val widePhone = !regular && listsBesideDetails(windowWidth.value, windowHeight.value)
        // Read here so a host switch recomposes the column, the shade and the pane policy together.
        val personaStack = hosts.current
        fun personaPlacePrefix(key: NavKey) = isSidebarPlaceRoot(key) && pageRoots.openedAsRoot(key)
        val personaRoot = if (merged) mergedVisibleRoot(AppTab.Persona, personaStack, compactPlace = ::personaPlacePrefix)
            else personaStack.firstOrNull() ?: AssistantRoute
        val agentRoot = if (merged && selected == AppTab.Agents) {
            mergedVisibleRoot(AppTab.Agents, agentsStack, agentDrillDown = {
                navigator.sourceTab(it) == AppTab.Persona || navigator.hasInboxOrigin(it)
            })
        } else AgentsRoute
        val agentDrill = merged && selected == AppTab.Agents && agentRoot == AgentsRoute && agentsStack.size > 1
        val visibleRoot = if (selected == AppTab.Persona) personaRoot else agentRoot
        val currentVisibleRoot = rememberUpdatedState(visibleRoot)
        val panePolicies = AppTab.entries.associateWith { tab ->
            assistantPanePolicy(tab, if (tab == AppTab.Persona) personaStack else stacks.getValue(tab), regular, widePhone)
        }
        // Lists sit beside their details in a regular window and when expanded; also on a wide phone on its side.
        // Panes share the window's one background; between them only a little space, no second surface or rule.
        // The scaffold must not move focus into the pane it shows (its default): focus entered the list pane's first
        // focusable, the search field, and with a hardware keyboard Agents and Inbox opened with the input method up.
        val directive = (if (!expanded && listsBesideDetails(windowWidth.value, windowHeight.value)) calculatePaneScaffoldDirectiveWithTwoPanesOnMediumWidth(adaptive)
            else calculatePaneScaffoldDirective(adaptive)).let {
                PaneScaffoldDirective(
                    maxHorizontalPartitions = it.maxHorizontalPartitions,
                    horizontalPartitionSpacerSize = minOf(it.horizontalPartitionSpacerSize, 8.dp),
                    maxVerticalPartitions = it.maxVerticalPartitions,
                    verticalPartitionSpacerSize = it.verticalPartitionSpacerSize,
                    defaultPanePreferredWidth = it.defaultPanePreferredWidth,
                    defaultPanePreferredHeight = it.defaultPanePreferredHeight,
                    excludedBounds = it.excludedBounds,
                    shouldAutoFocusCurrentDestination = false,
                )
            }
        val wide = expanded || directive.maxHorizontalPartitions > 1
        // Each tab's list-detail scene (rememberShellListDetail) keeps its own divider. Back there closes the detail and
        // leaves the list with its placeholder: the default (pop until the panes change) emptied the whole stack in two
        // panes, where the placeholder keeps the panes as they were, and back left the app.
        val paneStore = remember { PaneSplitStore(context) }
        var paneAreaLeft by remember { mutableFloatStateOf(0f) }
        val paneDensity = LocalDensity.current.density
        val sides = WindowInsets.safeDrawing.only(WindowInsetsSides.Horizontal)
        val sideLeft = sides.getLeft(LocalDensity.current, LocalLayoutDirection.current)
        val sideRight = sides.getRight(LocalDensity.current, LocalLayoutDirection.current)
        // The same inset area measured by the pane box below, including in split-screen windows.
        val paneWidth = windowWidth - with(LocalDensity.current) { (sideLeft + sideRight).toDp() }
        // The vertical folds across the panes' area, in dp from its left edge: snap points for the divider, or (when
        // the two sides are apart) where the split is fixed.
        val paneHinges = adaptive.windowPosture.hingeList.filter { it.isVertical }.map { hinge ->
            PaneHinge((hinge.bounds.left - paneAreaLeft) / paneDensity, (hinge.bounds.right - paneAreaLeft) / paneDensity, hinge.isSeparating)
        }
        val columnsOver = regular && columnPlacement(paneWidth.value, paneHinges) == ColumnPlacement.Over
        val currentColumnsOver = rememberUpdatedState(columnsOver)
        LaunchedEffect(merged, columnsOver, selected) {
            sidebarOverlayShown.value = false
            agentsOverlayShown.value = false
        }
        fun revealMergedSidebar() {
            when {
                !regular -> sidebar.opened.value = true
                columnsOver -> sidebarOverlayShown.value = true
                else -> sidebarColumnShown.value = true
            }
        }
        val standaloneCard = if (merged) AgentsCardOpen.requests.collectAsStateWithLifecycle().value else null
        LaunchedEffect(merged, regular, columnsOver, selected, visible, standaloneCard) {
            if (merged && selected == AppTab.Agents && (visible == AgentsRoute || standaloneCard != null)) {
                revealMergedSidebar()
            }
        }
        val agentsListNavigator = remember(navigator) {
            object : Navigator by navigator {
                override fun navigate(key: NavKey) {
                    pickAgentsList(navigator, agentsStack, agentsOverlayShown, key)
                }
            }
        }
        fun pickSidebarAgent(key: NavKey) {
            dismissSidebar()
            navigator.pickSidebarAgent(key)
        }
        val mergedListNavigator = object : Navigator by navigator {
            override fun navigate(key: NavKey) {
                when (key) {
                    MachinesRoute, is TodoDetailRoute -> sidebarDestination(key)
                    else -> if (isMergedAgentPick(key)) pickSidebarAgent(key) else {
                        dismissSidebar()
                        navigator.navigate(key)
                    }
                }
            }
        }
        val mergedContent = if (merged) mergedSidebar(
            session, sessions, currentAssistant, preferences, agentRows,
            agentRoot = visibleRoot.takeIf { selected == AppTab.Agents },
            agentsInFront = selected == AppTab.Agents && !agentDrill,
            agents = agents,
            navigator = mergedListNavigator,
            machine = machine,
            onMachineSelected = { machine = it },
            terminalsAvailable = terminalsAvailable,
            listOwner = agentsListOwner,
            usage = { SidebarUsageStrip(sidebarUsage.value) { sidebarDestination(MachinesRoute) } },
        ) else null
        val rootKeys: Set<Any> = remember { listOf<NavKey>(AssistantRoute, AgentsRoute).map { NavEntry(it) {}.contentKey }.toSet() }
        val assistantRootKey: Any = remember { NavEntry<NavKey>(AssistantRoute) {}.contentKey }
        // Separate can have a hidden Assistant column and an open Agents drawer at the same time.
        // Merged uses one key across the phone drawer and the regular column.
        val mergedSidebarKey = "merged-sidebar"
        val sharedSidebar: @Composable (String, (() -> Unit)?) -> Unit = { stateKey, onFoldColumn ->
            sidebarState.SaveableStateProvider(stateKey) {
                val sidebarContent: @Composable () -> Unit = {
                    AssistantSidebarContent(assistant, session.assistant, sidebar.active, sidebarOperations, session.demo != null,
                        onOpen = { key ->
                            dismissSidebar()
                            selected = openResolved(ResolvedRoute(AppTab.Persona, ChatRoute(key.machineId, key.sessionId, ConversationKind.Assistant)))
                        },
                        onAction = { action ->
                            dismissSidebar()
                            if (window.regular || merged) {
                                // Assistant actions use its embedded conversation host. A Merged Scheduled Tasks pick
                                // uses the same screen on a place host.
                                val step = pageRoots.select(AssistantRoute, hosts.current.drop(1), contentChanged = true)
                                showHost(step.root, step.stack)
                                selected = AppTab.Persona
                            } else showAssistantHome()
                            sidebar.action = action
                        },
                        onSettings = {
                            dismissSidebar()
                            settingsSheetStack.clear()
                            settingsSheetStack.add(SettingsRoute)
                            settingsOpened = true
                        },
                        onInbox = { sidebarDestination(InboxRoute) },
                        onKnowledge = { sidebarDestination(KnowledgeRoute) },
                        onTodos = { sidebarDestination(TodosRoute) }, todoCount = uncompletedTodosCount, showTodos = todosSupported,
                        inboxCount = inboxUnread, settingsCount = slotBadge.count, settingsDot = slotBadge.dot, settingsBadge = demoBadge,
                        pages = sidebarPages,
                        onPage = { key ->
                            if (key == "machines") sidebarDestination(MachinesRoute)
                            else extensionPages.firstOrNull { extensionRailKey(it.extension, it.id) == key }?.let { page ->
                                sidebarDestination(ExtensionUiRoute(page.extension, page.id, page.title.ifBlank { page.id }))
                            }
                        },
                        onRetired = { key -> sidebar.retired = sidebar.retired + key },
                        onOpenTodo = { id -> sidebarDestination(TodoDetailRoute(id)) },
                        onOpenAgent = { key ->
                            if (merged) pickSidebarAgent(ChatRoute(key.machineId, key.sessionId, ConversationKind.Agent))
                            else {
                                dismissSidebar()
                                selected = openResolved(ResolvedRoute(AppTab.Agents, ChatRoute(key.machineId, key.sessionId, ConversationKind.Agent)))
                            }
                        },
                        // The current assistant at the sidebar's head; its roster opens under it.
                        switcher = { head, trailing, fallback ->
                            persona.feature.settings.AssistantSwitcher(services.managers.takeUnless { services.demo }, waiting,
                                hubOnline = connection == ConnectionState.Online || connection == ConnectionState.Updating,
                                onManage = {
                                    dismissSidebar()
                                    settingsSheetStack.clear()
                                    settingsSheetStack.add(SettingsRoute)
                                    settingsSheetStack.add(SettingsPageRoute(SettingsPage.Managers))
                                    settingsOpened = true
                                }, headModifier = head, trailing = trailing, fallback = fallback)
                        },
                        // Regular window: Search opens over the whole window as the open iPhone Duo (#9171).
                        fullWindowSearch = regular || merged,
                        shadedPlace = if (merged) {
                            val root = if (agentDrill) personaRoot else visibleRoot
                            if (root == AssistantRoute && sidebar.healthInFront) AssistantSidebarShade.Health else sidebarShadeKey(root)
                        } else visibleSidebarShade(
                            regular = regular,
                            selected = selected,
                            personaRoot = personaRoot,
                            visible = visible,
                            visibleSource = navigator.sourceTab(visible),
                        ),
                        onFoldColumn = onFoldColumn,
                        merged = mergedContent,
                        onSchedule = if (merged) ({ sidebarDestination(SidebarScheduleRoute) }) else null)
                }
                if (merged) AssistantAppHost(navigator, sidebarContent) else sidebarContent()
            }
        }
        // What the decorator below reads: it is remembered across compositions, these change with the window.
        val currentRegular = rememberUpdatedState(regular)
        val currentWide = rememberUpdatedState(wide)
        val currentSelected = rememberUpdatedState(selected)
        val currentLifecycle = rememberUpdatedState(lifecycle)
        val currentOthersWaiting = rememberUpdatedState(othersWaiting)
        val currentAgentNeeds = rememberUpdatedState(agentNeeds)
        val currentConnection = rememberUpdatedState(connection)
        // The one place the page's button and the column's own header fold or reopen the column (#9336), so both keep
        // the same column memory (#9302).
        val setSidebarColumnBeside: (Boolean) -> Unit = { shown ->
            sidebarColumnShown.value = shown
            if (columnMemory.toggleSidebarBeside(
                    shown = shown,
                    regular = currentRegular.value,
                    columnsOver = currentColumnsOver.value,
                    tabShown = currentMerged.value || currentSelected.value == AppTab.Persona,
                    foreground = currentLifecycle.value.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.STARTED),
                )
            ) {
                keptSidebarColumnShown.value = shown
            }
        }
        val leading: @Composable (Boolean) -> Unit = { assistantRoot ->
            val leadingAction = resolveHomeLeadingAction(
                assistantRoot = currentMerged.value || assistantRoot,
                regular = currentRegular.value,
                columnsOver = currentColumnsOver.value,
                sidebarColumnShown = sidebarColumnShown.value,
            )
            when (leadingAction) {
                HomeLeadingAction.HiddenSpacer -> JPersonaButtonRoom()
                HomeLeadingAction.OpenDrawer,
                HomeLeadingAction.ToggleOverlay,
                HomeLeadingAction.ShowColumn -> Box {
                    // The dot is spoken too: that something waits at another assistant.
                    val sidebarLabel = stringResource(R.string.app_sidebar).let { label ->
                        if (!currentOthersWaiting.value) label
                        else stringResource(persona.core.designsystem.R.string.designsystem_manager_spoken, label,
                            stringResource(persona.core.designsystem.R.string.designsystem_manager_others_waiting))
                    }
                    JPersonaButton(sidebar.personaState, sidebarLabel, {
                        when (leadingAction) {
                            HomeLeadingAction.OpenDrawer -> sidebar.opened.value = true
                            HomeLeadingAction.ToggleOverlay -> sidebarOverlayShown.value = !sidebarOverlayShown.value
                            HomeLeadingAction.ShowColumn -> setSidebarColumnBeside(true)
                            HomeLeadingAction.HiddenSpacer -> Unit
                        }
                    })
                    // Something waits at another assistant: the switcher's badges say where, as iOS's AssistantWaitingDot.
                    if (currentMerged.value && currentAgentNeeds.value > 0) {
                        JBadge(currentAgentNeeds.value, Modifier.align(Alignment.TopEnd))
                    }
                    if (currentOthersWaiting.value) {
                        JStatusDot(Status.NeedsYou, Modifier.align(if (currentMerged.value) Alignment.TopStart else Alignment.TopEnd)
                            .padding(Tokens.Space.xs), breathes = false)
                    }
                }
            }
        }
        val currentLeading = rememberUpdatedState(leading)
        // Only primary roots carry the shared top switcher. Secondary pages keep their own Back control.
        val chrome = remember(style, items, selected) {
            NavEntryDecorator<NavKey> { entry ->
                val glass = remember { HazeState() }
                val webSheets = remember { mutableIntStateOf(0) }
                val level = LocalGlassLevel.current
                val primary = entry.contentKey in rootKeys
                val assistantRoot = entry.contentKey == assistantRootKey
                // A place root uses the conversation's sidebar button, never a chevron (sidebar-navigation.md section 6;
                // iOS sidebarRootPage, SidebarPageRoots.swift:159–165, and SplitNavigation.swift:130–133).
                // Each host supplies its own root (LocalHostRoot). A hidden host stays a place root, so showing it
                // does not flash Back. Settings and Pairing also set LocalRootLeading; LocalIsSidebarRoot is this test.
                val hostRoot = LocalHostRoot.current
                val persona = currentSelected.value == AppTab.Persona
                val rootKey = hostRoot ?: if (persona) hosts.current.firstOrNull() else null
                val stackRootKey = rootKey?.let { NavEntry<NavKey>(it) {}.contentKey }
                val mergedHere = currentMerged.value
                val mergedRootKey = NavEntry<NavKey>(currentVisibleRoot.value) {}.contentKey
                val placeRoot = if (mergedHere) !primary && entry.contentKey == mergedRootKey else showsPlaceRootLeading(
                    regular = currentRegular.value,
                    persona = persona,
                    primary = primary,
                    entryIsStackRoot = stackRootKey != null && entry.contentKey == stackRootKey,
                )
                val barHeight = remember { mutableIntStateOf(0) }
                val tabSplit = LocalAgentsTabSplit.current
                val entrySplit = rememberEntrySplitDetail(tabSplit, entry.contentKey)
                val page: @Composable () -> Unit = {
                    val content: @Composable () -> Unit = {
                        CompositionLocalProvider(LocalGlassSource provides glass, LocalWebSheets provides webSheets,
                            LocalGlassLevel provides if (webSheets.intValue > 0) GlassLevel.Solid else level,
                            LocalIsSidebarRoot provides placeRoot,
                            LocalRootLeading provides if (placeRoot) { { currentLeading.value(true) } } else null,
                            LocalRootBack provides if (primary || placeRoot) null else navigator::back,
                            LocalHomeChrome provides if (primary) HomeChrome(items, selected.name,
                                onSelect = { navigator.selectTab(AppTab.valueOf(it)) }, barHeight = barHeight,
                                leading = { currentLeading.value(assistantRoot) }, showSwitch = !mergedHere) else null) {
                            // Always the same Box. glassPageChrome() is identity outside Glass and under Reduce Motion, so switching
                            // style does not replace this node and drop the page's rememberSaveable state.
                            Box(Modifier.fillMaxSize().glassPageChrome()) {
                                entry.Content()
                                if (primary) {
                                    // The home bar holds no title to say the link is down: a pill under it does, as on iOS (#4339 C14).
                                    HomeLinkPill(currentConnection.value, barHeight.intValue)
                                }
                            }
                        }
                    }
                    if (tabSplit != null) {
                        CompositionLocalProvider(LocalSplitDetail provides entrySplit, content = content)
                    } else {
                        content()
                    }
                    // Register after the root page's own Back handler, even when NavDisplay composes it later.
                    BackHandler(!mergedHere && (assistantRoot || placeRoot) && selected == AppTab.Persona && currentColumnsOver.value && sidebarOverlayShown.value) {
                        sidebarOverlayShown.value = false
                    }
                }
                page()
            }
        }
        // A chat's terminal cover reads its machine's own hub group, as the terminal page does (`routed`).
        fun chatTerminal(route: ChatRoute) = session.terminal?.let { shell.groupTerminal(SessionKey(route.machineId, route.sessionId)) ?: it }
        val askSubscriptions = rememberSubscriptionAsk(session.sender, shell.feedback)
        val provider = entryProvider<NavKey> {
            assistantEntries(navigator) {
                // Persona's conversation offers the next card an Agent waits on you with, as an Agent's chat does (#4339 C18).
                val waiting = rememberWaitingRequests(session, sessions)
                val elsewhere = remember(waiting) {
                    persona.feature.chat.WaitingElsewhere(waiting) { chat, request -> openWithCard(persona.android.navigation.waitingRoute(chat, request), fromPage = true) }
                }
                CompositionLocalProvider(persona.feature.chat.LocalWaitingElsewhere provides elsewhere) {
                    AssistantScreen(assistantNavigator, session.assistant, session.sender, session.assistant::startConversation,
                        connectionLabel = connection.title(), connection = connection,
                        hubOnline = connection == ConnectionState.Online || connection == ConnectionState.Updating, demo = session.demo != null,
                        requestedSession = assistantRequest, onKnowledge = { sidebarDestination(KnowledgeRoute) },
                        needsYou = needsYou, shellNeedsYou = shellNeedsYou, onOpenNeed = { line -> openWithCard(needsYouRoute(line), fromPage = true) },
                        onOpenInbox = { sidebarDestination(InboxRoute) },
                        onVoice = voiceExtension?.let { extension -> { navigator.navigate(VoiceRoute(extension.id, extension.name)) } },
                        onTodos = { sidebarDestination(TodosRoute) },
                        todoCount = uncompletedTodosCount,
                        showTodos = todosSupported, mentionSessions = session.sessions.sessions,
                        hubGroup = hubGroups.takeIf { it.size > 1 }?.firstOrNull { it.selected }?.name)
                }
            }
            agentsEntries(navigator, agents, machine, { machine = it },
                terminalPanes = terminalsAvailable, listNavigator = agentsListNavigator, listOwner = agentsListOwner,
                listReplacement = if (merged) ({ AgentsEmptyPage(null) }) else null)
            entry<SidebarScheduleRoute> {
                AssistantAppHost(navigator) {
                    val management = LocalAssistantHost.current?.management
                    val scoped = remember(management, currentAssistant.id) { management?.forManager(currentAssistant.id) }
                    AssistantProactiveScreen(
                        onBack = navigator::back,
                        onOpenConversation = { key -> navigator.navigate(ChatRoute(key.machineId, key.sessionId, ConversationKind.Assistant)) },
                        onModelConfiguration = { purpose -> navigator.navigate(ModelPurposeRoute(purpose, currentAssistant.id)) },
                        management = scoped,
                    )
                }
            }
            spaceEntries(navigator, agents)
            machineSpacesEntries(navigator, agents)
            chatEntries(navigator) {
                ShellChat(it, session, navigator, shell.feedback, onSessionClosed = navigator::sessionClosed, onOpenCard = { openWithCard(it, fromPage = true) },
                    onGroupGone = navigator::groupGone, terminal = chatTerminal(it))
            }
            entry<InteractionRoute> { ShellChat(it.chat, session, navigator, shell.feedback, it.requestId, navigator::sessionClosed,
                { route -> openWithCard(route, fromPage = true) }, navigator::groupGone, chatTerminal(it.chat)) }
            inboxEntries(navigator, session.inbox, pictureMarkdown = { text, source, modifier ->
                persona.feature.chat.ExternalReplyMarkdown(text, source, modifier)
            }, connectors = services.connectors.takeUnless { services.demo }, onOpenCard = { card ->
                openWithCard(ResolvedRoute(AppTab.Agents, AgentsRoute, card = card))
            })
            entry<InboxItemRoute> { InboxLink(it, session.inbox, navigator, ::replace) }
            entry<LarkChatRoute> { InboxLink(it, session.inbox, navigator, ::replace) }
            entry<InboxItemUiRoute> { InboxItemUi(it, session.inbox, services.extensionSlots, navigator, ::replace, itemPageFailures) }
            entry<SettingsRoute> {
                SettingsWithTools(if (shell.automation) null else tools, navigator) { ShellSettings(services, navigator, shell) }
            }
            entry<SettingsPageRoute> { route ->
                if (route.page == SettingsPage.PhoneData && session.phone != null) PhoneToolsScreen(session.phone, navigator)
                else ShellSettings(services, navigator, shell, route.page.toInitialPage())
            }
            entry<PairingRoute> { ShellSettings(services, navigator, shell, "pair") }
            entry<PhoneToolsRoute> { session.phone?.let { PhoneToolsScreen(it, navigator) }
                ?: UnavailablePage(stringResource(R.string.app_unavailable_phone_tools), navigator) }
            session.terminal?.let { terminalEntries(navigator, it, { key -> sessions.firstOrNull { s -> s.key == key }?.displayTitle },
                panes = shell.terminalPanes.takeIf { session.demo == null }, sessions = session.sessions,
                routed = shell::groupTerminal,
                resumer = SessionTerminalResumer({ session.sessions.sessions.value }, { session.agentsWiring.state.value.creation },
                    { session.sessions.machines.value })) }
                ?: run {
                    entry<TerminalRoute> { UnavailablePage(stringResource(R.string.app_demo_no_terminal), navigator) }
                    entry<persona.core.navigation.TerminalPanesRoute> { UnavailablePage(stringResource(R.string.app_demo_no_terminal), navigator) }
                }
            session.metrics?.let { machinesEntries(navigator, it, session.sessions, session.machines,
                launcherPages = services.extensions?.takeUnless { services.demo }?.available ?: kotlinx.coroutines.flow.flowOf(false),
                extensionSections = { id -> persona.feature.settings.rememberMachineExtensionSections(services.extensionSlots, id, onOpen = navigator::navigate) },
                computers = session.computers, operationScope = session.identity) } ?: run {
                entry<MachinesRoute> { UnavailablePage(stringResource(R.string.app_demo_no_machines), navigator) }
                entry<persona.core.navigation.AddComputerRoute> { UnavailablePage(stringResource(R.string.app_demo_no_machines), navigator) }
                entry<MachineDetailRoute> { UnavailablePage(stringResource(R.string.app_demo_no_machines), navigator) }
            }
            session.knowledge?.let { knowledgeEntries(navigator, it, session.assistant, session.sender, session.sessions,
                session.knowledgeNamespace, uriHandler::openUri,
                onNewAgent = { start -> navigator.navigate(NewAgentRoute(start.machineId, start.cwd, start.prompt)) }) } ?: run {
                entry<KnowledgeRoute> { UnavailablePage(stringResource(R.string.app_demo_no_knowledge), navigator) }
                entry<KnowledgeKindRoute> { UnavailablePage(stringResource(R.string.app_demo_no_knowledge), navigator) }
                entry<KnowledgeDetailRoute> { UnavailablePage(stringResource(R.string.app_demo_no_knowledge), navigator) }
            }
            settingsDetailEntries(navigator, services, askSubscriptions)
            session.voice?.let { voiceEntries(navigator, it, session.identity, onModelConfiguration = { purpose -> navigator.navigate(ModelPurposeRoute(purpose)) }) } ?: run { entry<VoiceRoute> { UnavailablePage(stringResource(R.string.app_demo_no_voice), navigator) } }
            tools?.run { register(navigator) }
        }
        val panes: (NavKey, Boolean) -> NavEntry<NavKey> = { key, singleRightStack ->
            val entry = provider(key)
            NavEntry(key, entry.contentKey, entry.metadata + paneMetadata(key, singleRightStack)) { entry.Content() }
        }
        // Tabs stay composed once visited: switching back shows the page as it was instead of rebuilding it (which
        // flashed empty and loading states). A hidden tab is measured but never placed or drawn, and cannot take back.
        val visited = remember { mutableStateListOf(AppTab.Persona) }
        LaunchedEffect(selected) { if (selected !in visited) visited += selected }
        // The tab left behind stays composed: it must not keep the field's focus, the keyboard or a paste menu (#894).
        ReleaseTextInputOnChange(selected)
        ReleaseTextInputOnChange(sidebar.opened.value)
        ReleaseTextInputOnChange(sidebarOverlayShown.value)
        ReleaseTextInputOnChange(agentsOverlayShown.value)
        ReleaseTextInputOnChange(settingsOpened)
        // A root switch leaves the conversation composed. Clear the field it still holds
        // (AssistantView.swift:666 `.id(value)`).
        ReleaseFocusOnSidebarHost(hosts.currentId())
        // Back on the first page of Agents goes to Persona. Back on a place root returns to the conversation
        // (the handler on that host). Back on the conversation leaves the app (2026-11-01, the user's call).
        BackHandler(selected != AppTab.Persona && stacks.getValue(selected).size <= 1) {
            if (currentMerged.value) showAssistantHome() else navigator.selectTab(AppTab.Persona)
        }
        // Turned on its side, a phone's camera cutout or its three-button navigation bar sits on the left or right: the
        // shell keeps clear of them, while the background still runs under them (2026-11-02, the old app's shell did so).
        val sideRoom = sideLeft > 0 || sideRight > 0
        val stackEvidence = assistantStackEvidence(visible, selected)
        CompositionLocalProvider(LocalAmbience provides ambience, LocalWindowLayout provides windowLayout, LocalLargeWindowInput provides regular,
            LocalInboxItemPages provides inboxItemPages,
            LocalInboxTriageSetup provides triageSetup, LocalAssistantSidebarHost provides sidebar,
            LocalSidebarUsage provides sidebarUsage.takeIf { merged && session.machines != null }) {
          AssistantStackNavigationProbe(stackEvidence.route, stackEvidence.selectedKey)
          // Only roots own the drawer's opening gesture. A pushed page keeps the edge for system Back.
          JShellDrawer(
              sidebar.opened.value && (!merged || !regular),
              onDismiss = { sidebar.opened.value = false },
              dragEnabled = !regular && (if (merged) visible == visibleRoot else selected == AppTab.Persona && assistantStack.size <= 1),
              onOpen = { sidebar.opened.value = true },
              drawer = { if (!merged || !regular) sharedSidebar(if (merged) mergedSidebarKey else "shared-sidebar", null) },
          ) {
          Box(Modifier.fillMaxSize().background(PersonaTheme.palette.background)) {
            // One background for the window. Pages paint the same light at the same place (PageBackdrop), so in wide
            // windows the rail, the panes and the room between them read as one surface; cards are what separate them.
            // The strips beside a cutout or a side navigation bar show it too.
            if (wide || sideRoom) PageBackdrop()
            Row(Modifier.fillMaxSize().windowInsetsPadding(sides)) {
              // The same top navigation and sidebar apply on phones, tablets and foldables.
              Box(Modifier.weight(1f).fillMaxHeight().onGloballyPositioned { paneAreaLeft = it.positionInWindow().x }) {
                val foldMergedColumn: (() -> Unit)? = if (showsSidebarColumnFoldButton(regular, columnsOver)) {
                    { setSidebarColumnBeside(false) }
                } else null
                AssistantPaneFrame(
                    regular, showSidebar = merged,
                    shown = if (columnsOver) sidebarOverlayShown.value else sidebarColumnShown.value,
                    sidebar = { sharedSidebar(mergedSidebarKey, foldMergedColumn) },
                    hinges = paneHinges, overlay = columnsOver, onDismiss = { sidebarOverlayShown.value = false },
                    drags = merged, detailAtRoot = visible == visibleRoot,
                    onDragSettle = { value ->
                        if (currentColumnsOver.value) sidebarOverlayShown.value = value else setSidebarColumnBeside(value)
                    },
                ) {
                BoxWithConstraints(Modifier.fillMaxSize()) {
                AppTab.entries.forEach { tab ->
                    if (tab == selected || tab in visited) key(tab) {
                        val shown = tab == selected
                        val back = rememberNavigationEventDispatcherOwner(enabled = shown)
                        // The row of what the detail pane shows is marked in the list, however the page was opened.
                        val tabStack = if (tab == AppTab.Persona) personaStack else stacks.getValue(tab)
                        val detail = detailOf(tabStack)
                        // Agents may close its list beside an open conversation; Inbox and Settings keep theirs (as the old app did).
                        val policy = panePolicies.getValue(tab)
                        val sceneDirective = if (policy.singleRightStack || (merged && tab == AppTab.Agents)) directive.onePane() else directive
                        val listDetail = rememberShellListDetail(
                            tab, sceneDirective, maxWidth, paneHinges, detailOpen = detail != null,
                            store = paneStore, collapsible = canCollapseList(tabStack),
                            initialCollapsed = tab == AppTab.Agents && !agentsListShown.value,
                            onCollapsedChange = if (tab == AppTab.Agents) { collapsed ->
                                val nextShown = !collapsed
                                agentsListShown.value = nextShown
                                if (columnMemory.toggleAgentsBeside(
                                        shown = nextShown,
                                        twoPanes = currentWide.value,
                                        columnsOver = currentColumnsOver.value,
                                        tabShown = currentSelected.value == AppTab.Agents,
                                        foreground = currentLifecycle.value.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.STARTED),
                                    )
                                ) {
                                    keptAgentsListShown.value = nextShown
                                }
                            } else null,
                        )
                        val agentsOverlay = rememberAgentsOverlayStrategy(!merged && columnsOver && tab == AppTab.Agents &&
                            tabStack.drop(1).all(::isDetailDestination), agentsOverlayShown) {
                            val barHeight = remember { mutableIntStateOf(0) }
                            CompositionLocalProvider(LocalHomeChrome provides HomeChrome(items, selected.name,
                                onSelect = { navigator.selectTab(AppTab.valueOf(it)) }, barHeight = barHeight,
                                leading = { currentLeading.value(false) })) {
                                Box(Modifier.fillMaxSize()) {
                                    AgentsEmptyPage { agentsOverlayShown.value = true }
                                    HomeLinkPill(currentConnection.value, barHeight.intValue)
                                }
                            }
                        }
                        val agentsTabSplit = rememberAgentsTabSplit(
                            tab = tab,
                            stack = tabStack,
                            twoPanes = sceneDirective.maxHorizontalPartitions > 1 && !columnsOver,
                            listHidden = listDetail.listHidden,
                            onShowList = listDetail::expand,
                        )
                        ShellTabLifecycle(shown) {
                            CompositionLocalProvider(LocalNavigationEventDispatcherOwner provides back, LocalDetailKey provides detail,
                                LocalAgentsTabSplit provides agentsTabSplit,
                                persona.feature.settings.LocalExtensionPageVisible provides shown, LocalInboxLanding provides inboxLandings.getValue(tab)) {
                                val pageTransitions = rememberPageTransitions()
                                val display: @Composable () -> Unit = {
                                    NavDisplay(backStack = stacks.getValue(tab),
                                        modifier = Modifier.fillMaxSize().then(if (shown || tab == AppTab.Persona) Modifier else Modifier.unplaced()),
                                        onBack = navigator::back, entryDecorators = decorators.getValue(tab) + chrome,
                                        sceneStrategies = listOf(agentsOverlay, listDetail.strategy),
                                        transitionSpec = {
                                            // A root switch is instant (UnifiedSidebar.swift:67–74). Drill-downs keep the push.
                                            if (currentMerged.value && mergedAgentRootSwapped(initialState, targetState)) noPageMotion()
                                            else pageTransitions.push
                                        },
                                        popTransitionSpec = {
                                            if (currentMerged.value && mergedAgentRootSwapped(initialState, targetState)) noPageMotion()
                                            else pageTransitions.pop
                                        },
                                        predictivePopTransitionSpec = { pageTransitions.predictivePop },
                                        entryProvider = { key -> panes(key, policy.singleRightStack) })
                                }
                                if (tab == AppTab.Persona) {
                                    SideEffect { stacks[AppTab.Persona] = hosts.current }
                                    val foldSidebarColumn: (() -> Unit)? =
                                        if (showsSidebarColumnFoldButton(regular, columnsOver)) ({ setSidebarColumnBeside(false) }) else null
                                    // AssistantPaneFrame's content is not inline, so BoxWithConstraints.maxWidth
                                    // cannot be read inside it. Capture it here, where this scope still has it.
                                    val paneMaxWidth = maxWidth
                                    // One sidebar owner outside every root's NavDisplay. Each visited root keeps its own
                                    // display, placed only while it is in front, so a swap does not pop the one left behind.
                                    // Assistant history drags (`AssistantView.swift:149`). A pushed page keeps the edge
                                    // for Back (`SplitColumnControl.swift:137`, `:160-169`). Settings is a sheet and has no column drag.
                                    AssistantPaneFrame(regular, !merged && policy.keepSharedSidebar,
                                        shown = if (columnsOver) sidebarOverlayShown.value else sidebarColumnShown.value,
                                        modifier = if (shown) Modifier else Modifier.unplaced(),
                                        sidebar = { sharedSidebar("shared-sidebar-column", foldSidebarColumn) }, hinges = paneHinges,
                                        overlay = columnsOver, onDismiss = { sidebarOverlayShown.value = false },
                                        drags = true, detailAtRoot = hosts.current.size <= 1,
                                        // Unplaced-but-composed Persona must not publish an exclusion on Agents.
                                        paneShown = shown,
                                        onDragSettle = { value ->
                                            if (currentColumnsOver.value) sidebarOverlayShown.value = value
                                            else setSidebarColumnBeside(value)
                                        }) {
                                        Box(Modifier.fillMaxSize()) {
                                            // SidebarRootsRuntime keeps one key() per host, so a host's SaveableStateHolder
                                            // is not recreated when another host appears. Place-root Back is registered
                                            // before that host's NavDisplay.
                                            SidebarRootsRuntime(
                                                window = window,
                                                hosts = hosts,
                                                pageRoots = pageRoots,
                                                assistantStack = assistantStack,
                                                regular = currentRegular.value,
                                                overlayOpen = currentColumnsOver.value && sidebarOverlayShown.value,
                                                shown = shown,
                                                showHost = ::showHost,
                                                assignPersona = { stacks[AppTab.Persona] = it },
                                                onBack = navigator::back,
                                                merged = merged,
                                            ) { _, hostStack, hostShown ->
                                                val hostPolicy = assistantPanePolicy(tab, hostStack, currentRegular.value, widePhone)
                                                val hostDirective = if (hostPolicy.singleRightStack) directive.onePane() else directive
                                                val hostDetail = detailOf(hostStack)
                                                val hostList = rememberShellListDetail(
                                                    tab, hostDirective, paneMaxWidth, paneHinges, detailOpen = hostDetail != null,
                                                    store = paneStore, collapsible = canCollapseList(hostStack),
                                                )
                                                val hostSplit = rememberAgentsTabSplit(
                                                    tab = tab, stack = hostStack,
                                                    twoPanes = hostDirective.maxHorizontalPartitions > 1 && !columnsOver,
                                                    listHidden = hostList.listHidden, onShowList = hostList::expand,
                                                )
                                                val hostTransitions = rememberPageTransitions()
                                                val hostDecorators = listOf(
                                                    rememberSaveableStateHolderNavEntryDecorator<NavKey>(),
                                                    rememberViewModelStoreNavEntryDecorator<NavKey>(),
                                                    chrome,
                                                )
                                                CompositionLocalProvider(
                                                    LocalHostRoot provides hostStack.firstOrNull(),
                                                    LocalDetailKey provides hostDetail,
                                                    LocalAgentsTabSplit provides hostSplit,
                                                    persona.feature.settings.LocalExtensionPageVisible provides hostShown,
                                                    // Hosts share one landing. Only the list in front takes it.
                                                    LocalInboxListActive provides hostShown,
                                                ) {
                                                    NavDisplay(
                                                        backStack = hostStack,
                                                        modifier = Modifier.fillMaxSize().then(if (hostShown) Modifier else Modifier.unplaced()),
                                                        onBack = {
                                                            if (hosts.backFromCompactPrefix(currentRegular.value)) {
                                                                stacks[AppTab.Persona] = hosts.current
                                                                pageRoots.adoptVisible(AssistantRoute)
                                                            } else navigator.back()
                                                        },
                                                        entryDecorators = hostDecorators,
                                                        sceneStrategies = listOf(hostList.strategy),
                                                        transitionSpec = {
                                                            if (personaSceneRootSwapped(currentRegular.value, true, initialState, targetState)) noPageMotion()
                                                            else hostTransitions.push
                                                        },
                                                        popTransitionSpec = {
                                                            if (personaSceneRootSwapped(currentRegular.value, true, initialState, targetState)) noPageMotion()
                                                            else hostTransitions.pop
                                                        },
                                                        predictivePopTransitionSpec = { hostTransitions.predictivePop },
                                                        entryProvider = { key -> panes(key, hostPolicy.singleRightStack) },
                                                    )
                                                }
                                            }
                                        }
                                    }
                                } else display()
                            }
                        }
                    }
                }
                }
                }
              }
            }

          }
          }
          BackHandler(merged && columnsOver && sidebarOverlayShown.value) { sidebarOverlayShown.value = false }
          BackHandler(sidebar.opened.value) { sidebar.opened.value = false }
          // Unfolded with the drawer open over the Assistant page: the sidebar it showed becomes the column beside it.
          LaunchedEffect(regular) {
              if (regular && (merged || selected == AppTab.Persona) && sidebar.opened.value) {
                  sidebar.opened.value = false
                  if (columnsOver) sidebarOverlayShown.value = true else sidebarColumnShown.value = true
              }
          }
          // A switch of assistant closes the current assistant's settings pages over the list (docs/assistant/managers.md
          // "Clients"), in the Settings sheet and on each tab (a link or a readiness card opens them there): none shows one
          // assistant's settings, or sends what it holds, once another is current; the list shows the new one's.
          persona.feature.settings.CloseAssistantSettingsOnSwitch(currentAssistant.id) { listOf(settingsSheetStack) + stacks.values }
          if (settingsOpened) {
              val settingsNavigator = remember(navigator, settingsSheetStack) {
                  SettingsSheetNavigator(settingsSheetStack, close = { settingsOpened = false }, outside = navigator)
              }
              JSettingsSheet(onDismiss = { settingsOpened = false }) {
                CompositionLocalProvider(LocalAssistantWaiting provides waiting) {
                  val settingsEntries = entryProvider<NavKey> {
                      entry<SettingsRoute> { ShellSettings(services, settingsNavigator, shell, onClose = { settingsOpened = false }) }
                      entry<SettingsPageRoute> { route ->
                          if (route.page == SettingsPage.PhoneData && session.phone != null) PhoneToolsScreen(session.phone, settingsNavigator)
                          else ShellSettings(services, settingsNavigator, shell, route.page.toInitialPage())
                      }
                      entry<PairingRoute> { ShellSettings(services, settingsNavigator, shell, "pair") }
                      entry<PhoneToolsRoute> { session.phone?.let { PhoneToolsScreen(it, settingsNavigator) }
                          ?: UnavailablePage(stringResource(R.string.app_unavailable_phone_tools), settingsNavigator) }
                      assistantEntries(settingsNavigator)
                      settingsDetailEntries(settingsNavigator, services, askSubscriptions)
                  }
                  // In a regular window the list of settings stays beside the page it opens, as on an open iPhone Duo
                  // (its divider is not kept: the sheet starts from the usual split each time). Elsewhere one page.
                  SettingsSheetPanes(settingsSheetStack, settingsNavigator::back, regular,
                      if (regular) directive else directive.onePane(),
                      adaptive.windowPosture.hingeList.filter { it.isVertical }, settingsEntries) { paneMetadata(it) }
                }
              }
          }
        }
    }
}

/**
 * A fold where the window's middle is, for captures on emulators whose system image reports no FoldingFeature
 * (debug launches only): "tabletop" is a horizontal half-open hinge, "book" a vertical one.
 */
private fun pretendFold(real: androidx.compose.material3.adaptive.WindowAdaptiveInfo, posture: String, size: androidx.compose.ui.unit.IntSize): androidx.compose.material3.adaptive.WindowAdaptiveInfo {
    val tabletop = posture == "tabletop"
    val bounds = if (tabletop) androidx.compose.ui.geometry.Rect(0f, size.height / 2f - 1f, size.width.toFloat(), size.height / 2f + 1f)
        else androidx.compose.ui.geometry.Rect(size.width / 2f - 1f, 0f, size.width / 2f + 1f, size.height.toFloat())
    val hinge = androidx.compose.material3.adaptive.HingeInfo(bounds, isFlat = false, isVertical = !tabletop, isSeparating = true, isOccluding = false)
    return androidx.compose.material3.adaptive.WindowAdaptiveInfo(real.windowSizeClass, androidx.compose.material3.adaptive.Posture(isTabletop = tabletop, hingeList = listOf(hinge)))
}

/**
 * Which pane each destination takes where list-detail remains active. A single-right-stack Assistant removes Inbox's
 * parallel roles; Agents and Settings keep their existing metadata.
 */
@OptIn(ExperimentalMaterial3AdaptiveApi::class)
internal fun paneMetadata(key: NavKey, singleRightStack: Boolean = false): Map<String, Any> {
    if (singleRightStack && isInboxDestination(key)) return emptyMap()
    return when (key) {
        AgentsRoute -> ListDetailSceneStrategy.listPane(detailPlaceholder = {
            DetailPlaceholder(stringResource(R.string.app_pane_agents_title), stringResource(R.string.app_pane_agents_detail), PhIcons.ChatsCircle)
        })
        InboxRoute -> ListDetailSceneStrategy.listPane(detailPlaceholder = {
            DetailPlaceholder(stringResource(R.string.app_pane_inbox_title), stringResource(R.string.app_pane_inbox_detail), PhIcons.Tray)
        })
        SettingsRoute -> ListDetailSceneStrategy.listPane(detailPlaceholder = {
            DetailPlaceholder(stringResource(R.string.app_tab_settings), stringResource(R.string.app_pane_settings_detail), PhIcons.GearSix)
        })
        else -> if (isDetailDestination(key)) ListDetailSceneStrategy.detailPane() else emptyMap()
    }
}

/** The same directive with one pane across (a copy would lose that the scaffold never moves focus). */
@OptIn(ExperimentalMaterial3AdaptiveApi::class)
internal fun PaneScaffoldDirective.onePane() = PaneScaffoldDirective(
    maxHorizontalPartitions = 1,
    horizontalPartitionSpacerSize = horizontalPartitionSpacerSize,
    maxVerticalPartitions = maxVerticalPartitions,
    verticalPartitionSpacerSize = verticalPartitionSpacerSize,
    defaultPanePreferredWidth = defaultPanePreferredWidth,
    defaultPanePreferredHeight = defaultPanePreferredHeight,
    excludedBounds = excludedBounds,
    shouldAutoFocusCurrentDestination = shouldAutoFocusCurrentDestination,
)

/** The rail's key for an extension's page: never one of the tabs' names. */
private fun extensionRailKey(extension: String, ui: String) = extensionSidebarKey(extension, ui)

/** The root of the NavDisplay this entry is composed in. Hidden hosts keep their own so they do not flash Back. */
private val LocalHostRoot = compositionLocalOf<NavKey?> { null }

/** Placed nowhere: measured and kept composed, never drawn and never hit. */
private fun Modifier.unplaced(): Modifier = layout { measurable, constraints ->
    val placeable = measurable.measure(constraints)
    layout(placeable.width, placeable.height) {}
}

/** The only primary destinations; all other entries live in the shared sidebar. */
@Composable
internal fun navigationItems(badges: Map<AppTab, Int>): List<NavItem> = listOf(
    NavItem(AppTab.Persona.name, stringResource(R.string.app_tab_assistant), PhIcons.Sparkle, badges[AppTab.Persona] ?: 0),
    NavItem(AppTab.Agents.name, stringResource(R.string.app_tab_agents), PhIcons.SquaresFour, badges[AppTab.Agents] ?: 0),
)

@Composable
private fun ShellSettings(services: SettingsServices, navigator: Navigator, shell: ShellHost, page: String = "settings", onClose: (() -> Unit)? = null) {
    // Every settings page is its own destination, so a wide window shows it beside the list of settings.
    SettingsScreen(services, initialPage = page, onPhoneData = { navigator.navigate(PhoneToolsRoute) },
        onExitDemo = shell::exitDemo, onOpen = { navigator.navigate(SettingsPageRoute(it)) },
        onBack = if (page == "settings") null else navigator::back, onNavigate = navigator::navigate, onClose = onClose)
}

/** Inbox deep links wait for the item list, then read a historical item by id when the list no longer has it. */
@Composable
private fun InboxLink(route: NavKey, inbox: InboxRepository, navigator: Navigator, replace: (NavKey, NavKey) -> Unit) {
    val snapshot by inbox.snapshot.collectAsStateWithLifecycle()
    var attempt by remember(route) { mutableIntStateOf(0) }
    var destination by remember(route) { mutableStateOf<InboxDestination?>(null) }
    val pages = LocalInboxItemPages.current
    val landing = LocalInboxLanding.current
    LaunchedEffect(route, inbox, attempt) {
        destination = null
        try { inbox.refresh() }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (_: Exception) { /* Cached records and the single-item lookup still resolve what they can. */ }
        // Opened from outside the list: the list under it moves to where the item is (#9053).
        destination = inboxDestinationWithLookup(route, snapshot.records, true, pages, inbox, found = { landing?.land(it) })
    }
    LaunchedEffect(destination) {
        (destination as? InboxDestination.Open)?.let {
            // An item link (not a chat link) opened before the hub's list was read may be the extension's to draw.
            if (route is InboxItemRoute) pages.opened(it.key)
            replace(route, it.key)
        }
    }
    when (destination) {
        InboxDestination.Missing -> UnavailablePage(stringResource(R.string.app_inbox_item_missing), navigator)
        InboxDestination.Failed -> Box(Modifier.fillMaxSize().statusBarsPadding(), contentAlignment = Alignment.Center) {
            JNotice(stringResource(persona.feature.inbox.R.string.inbox_refresh_failed),
                actionLabel = stringResource(persona.feature.inbox.R.string.inbox_retry), onAction = { attempt++ })
        }
        else -> Box(Modifier.fillMaxSize().statusBarsPadding(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
    }
}

/**
 * An inbox item whose connector's extension draws its body (inbox_item, #1683): the inbox's header of the item over the
 * extension's view. When the extension cannot draw it, the item's own page takes its place: quietly when the extension declines
 * (a read-only device, 404, unsupported, nothing drawn), with a line and "Retry" there when it broke ([failures]). The
 * extension's `persona://inbox/<id>/source` opens that item's own page on top.
 */
@Composable
private fun InboxItemUi(route: InboxItemUiRoute, inbox: InboxRepository, slots: ExtensionSlots?, navigator: Navigator,
    replace: (NavKey, NavKey) -> Unit, failures: MutableMap<String, String>) {
    val snapshot by inbox.snapshot.collectAsStateWithLifecycle()
    val records by rememberUpdatedState(snapshot.records)
    val feedback = LocalAppFeedback.current
    val missing = stringResource(R.string.app_inbox_item_missing)
    val record = snapshot.records.firstOrNull { it.item.id == route.itemId }
    // Its links, the extension's buttons' too, open as the item's manager's (a Feishu document, a Moments post).
    ManagerLinks(record?.manager) {
        // The extension draws the item again when it changes: new messages joined the event, it was done or reopened.
        val revision = record?.let { listOf(it.item.version, it.item.updatedAt, it.count, it.item.doneAt) }
        val body = rememberInboxItemBody(slots, route, revision, record?.manager, InboxItemKey.hubId(route.itemId), onSource = { id ->
            inboxSourcePage(id, records)?.let(navigator::navigate) ?: feedback?.notice(missing)
        })
        val fallback = body.fallback
        val drawn = fallback == null && !body.loading
        LaunchedEffect(drawn) { if (drawn) failures.remove(route.itemId) }
        val own by rememberUpdatedState(record?.let(::inboxItemPage))
        LaunchedEffect(fallback, own != null) {
            val page = own ?: return@LaunchedEffect
            if (fallback is InboxItemFallback.Notice) failures[route.itemId] = fallback.message
            if (fallback != null) replace(route, page)
        }
        when {
            fallback == null -> InboxItemExtensionScreen(inbox, route, navigator, body.title, body.loading, body.refreshing, body.refresh, body.items)
            record == null -> UnavailablePage(missing, navigator)
            else -> Box(Modifier.fillMaxSize())
        }
    }
}

/**
 * Set Up with … on Inbox Sources (#5308): the same hand-off as Ask Persona. The page has already closed itself.
 * Without a main-conversation sender the Persona tab still opens, so the page is not left up doing nothing.
 */
@Composable
private fun rememberSubscriptionAsk(sender: MessageSender, feedback: AppFeedback): (Navigator, String) -> Unit {
    val scope = rememberCoroutineScope()
    val failed = stringResource(persona.feature.chat.R.string.chat_handoff_failed)
    return remember(sender, scope, feedback, failed) {
        { nav, text ->
            val main = sender as? MainConversationSender
            if (main == null) {
                nav.selectTab(AppTab.Persona)
            } else scope.launch {
                try {
                    handToPersona(main, text, java.util.UUID.randomUUID().toString(),
                        onQueued = { nav.selectTab(AppTab.Persona) },
                        onBound = { key -> nav.navigate(ChatRoute(key.machineId, key.sessionId, kind = ConversationKind.Assistant)) })
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    if (feedback.failure(e) != true) feedback.notice(failed)
                }
            }
        }
    }
}

/** Debug builds add one row under the settings page; release builds and automated captures show nothing. */
@Composable
private fun SettingsWithTools(tools: DeveloperTools?, navigator: Navigator, content: @Composable () -> Unit) {
    if (tools == null) return content()
    Column(Modifier.fillMaxSize()) {
        Box(Modifier.weight(1f)) { content() }
        HorizontalDivider()
        TextButton(onClick = { navigator.navigate(tools.home) }, Modifier.fillMaxWidth()) { Text(tools.homeTitle, maxLines = 1) }
    }
}

@Composable
internal fun UnavailablePage(message: String, navigator: Navigator) {
    Column(Modifier.fillMaxSize().statusBarsPadding().padding(24.dp.dense), verticalArrangement = Arrangement.spacedBy(12.dp.dense)) {
        Text(message)
        TextButton(onClick = navigator::back) { Text(stringResource(R.string.app_back), maxLines = 1) }
    }
}

/** The link pill under the home bar ([JLinkPill]) while the hub isn't connected; it appears and goes without moving the page. */
@Composable
internal fun HomeLinkPill(state: ConnectionState, barHeight: Int) {
    val top = with(LocalDensity.current) { barHeight.toDp() }
    Box(Modifier.fillMaxWidth().padding(top = top + Tokens.Space.xs), contentAlignment = Alignment.TopCenter) {
        androidx.compose.animation.AnimatedVisibility(state != ConnectionState.Online && barHeight > 0,
            enter = androidx.compose.animation.fadeIn(), exit = androidx.compose.animation.fadeOut()) {
            // The last state stays drawn while it fades out.
            var last by remember { mutableStateOf(ConnectionState.Connecting) }
            if (state != ConnectionState.Online) SideEffect { last = state }
            JLinkPill(if (state != ConnectionState.Online) state else last)
        }
    }
}
