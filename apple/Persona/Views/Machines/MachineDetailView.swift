import BeautifulUI
import SwiftUI
import PersonaWire

/// One machine (docs/server/machines.md「App」), most used first: how it is (the card of Machines, without its numbers), its
/// agents (a tap opens the chat), its values now and CPU / memory / network / load over 1, 6 or 24 hours (hold a chart
/// to read a moment, all charts follow), the system, the launchers, how the hub reaches it (offline: right under the
/// card, with Reconnect). The metrics refresh while on screen and the app is in front: every 10 s for the hour, every
/// minute for the longer ranges.
struct MachineDetailView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.scenePhase) private var phase
    var ref: MachineRef
    private var machineID: String { ref.id }

    init(ref: MachineRef) {
        self.ref = ref
        _browserLogins = State(initialValue: BrowserLoginsModel(machine: ref.id, deps: .live(group: ref.group)))
    }
    @State private var range: MetricRange = Self.initialRange
    @State private var data: MachineMetrics?
    @State private var preparedCharts: [MetricSeriesKey: PreparedMetricSeries] = [:]
    @State private var chartEnd = Date.now
    @State private var metricsRequest = 0
    @State private var shownRange: MetricRange?
    @State private var failure: String?
    @State private var selection: Date?
    @State private var launchers: [Launcher] = []
    @State private var launcherError: String?
    @State private var reconnecting = false
    @State private var allAgents = false
    @State private var plugins = ExtensionMachineModel()
    @State private var pluginLink: LinkTarget?
    @State private var filesOpen = Self.initialFilesOpen
    /// Lives through the Files push / pop, so a remote POST can regain its progress row on return.
    @State private var folderTransfers = FolderTransfers()
    /// Browser Sign-ins (#5821) and their whole list: owned here, beside Files, not by the section, which exists only
    /// while this page has its machine; a destination inside it went away with it and broke the open list (#6050).
    @State private var browserLogins: BrowserLoginsModel
    @State private var browserLoginsOpen = false
    /// The rename alert is open, and what it holds.
    @State private var renaming = false
    @State private var newName = ""

    private static var initialFilesOpen: Bool {
        #if DEBUG
        return UserDefaults.standard.bool(forKey: "openMachineFiles") // `-openMachineFiles YES`: straight into Files (screenshots)
        #else
        return false
        #endif
    }

    /// A machine's agents: live ones, not the Chat tab's assistant conversations (the Agents tab leaves those out too),
    /// nor a subtask, which the Agents list folds under the session that started it and does not count unless it needs
    /// the user (`Session.isNestedSubtask`, as Android's Machines card).
    static func agents(_ model: AppModel, _ ref: MachineRef) -> [Session] {
        ChildSessions.fold(model.sessionsByMachine[ref] ?? []).filter(isListed)
    }

    /// Every machine's agents (`agents(_:_:)`), for a page that can be kept hidden behind another: watched only in front
    /// (`AppModel.allSessions(inFront:)`).
    static func agentsByMachine(_ model: AppModel, inFront: Bool) -> [MachineRef: [Session]] {
        Dictionary(grouping: ChildSessions.fold(model.allSessions(inFront: inFront)).filter(isListed), by: \.machineRef)
    }

    private static func isListed(_ s: Session) -> Bool { s.status.isLive && !s.assistant && !s.isNestedSubtask }

    /// The agents before Show All: with dozens on one machine its charts would be screens away.
    static let agentsShown = 5

    private static var initialRange: MetricRange {
        #if DEBUG
        // `-openMonitorRange 6h|24h`: the page opens on that range (screenshots, UI tests).
        if let r = UserDefaults.standard.string(forKey: "openMonitorRange").flatMap(MetricRange.init(rawValue:)) { return r }
        #endif
        return .hour
    }

    var body: some View {
        let machine = model.machine(ref)
        let system = data?.system ?? machine?.system
        let online = machine?.online ?? data?.online ?? false
        ScrollViewReader { scroller in
            ScrollView {
                VStack(alignment: .leading, spacing: Theme.Space.section) {
                    if let machine {
                        VStack(spacing: Theme.Space.m) {
                            MachineCard(machine: machine, metrics: data, showsMetrics: false, showsName: false)
                            // The card says a known error_code in words; the hub's own words stay here.
                            if !machine.linkSwitchRunning, AddComputerText.machineProblem(machine.errorCode) != nil, let raw = machine.error, !raw.isEmpty {
                                problemDetails(raw)
                            }
                            // Offline with its manager (the card says so): it comes back with that manager, and Reconnect
                            // goes through it.
                            if !online, !machine.linkSwitchRunning, !machine.isPending, model.offlineManager(of: machine, group: ref.group) == nil {
                                connection(machine)
                            }
                            MachineLinkSwitchView(machine: machine).id(Self.linkSwitchAnchor)
                        }
                        agentsSection(machine)
                        filesRow(online: online)
                        #if os(iOS)
                        // Its clipboard and the phone's, both ways (#3064), next to its files: what is handed over to it.
                        // The phone only for now; the Mac and visionOS apps follow later.
                        if let clipboard = machine.clipboard {
                            ClipboardSection(ref: ref, machine: machine, listed: clipboard, online: online).id(Self.clipboardAnchor)
                        }
                        #endif
                        if canRename(machine) { renameRow(machine) }
                        // Its token usage, then how busy it is, then what it can do (#4973).
                        MachineUsageSection(ref: ref)
                        VStack(alignment: .leading, spacing: Theme.Space.m) {
                            heading(String(localized: "Monitor [Message Views fdc390]"))
                            if let s = data?.latest, online { NowGrid(sample: s, cores: system?.cores) }
                            SegmentedChoice(String(localized: "Time Range"), MetricRange.allCases.map { ($0, $0.title) }, selection: $range)
                                .accessibilityIdentifier("metric-range")
                            charts(system: system)
                            if let cores = data?.latest?.cores, online, cores.count > 1 { CoreBars(cores: cores) }
                        }
                        // The node's self-check by feature (#3018); two rows when everything works.
                        NodeHealthSection(ref: ref, machine: machine)
                        if let computer = machine.computer { ComputerSection(ref: ref, computer: computer, online: online) }
                        if let browser = machine.browser { BrowserSection(ref: ref, browser: browser, online: online) }
                        if machine.browser?.setUp == true { BrowserLoginsSection(ref: ref, online: online, logins: browserLogins, listOpen: $browserLoginsOpen).id(Self.browserLoginsAnchor) }
                        #if os(iOS)
                        // The iPhones paired with this computer, for the assistant (docs/extensions/iphone.md); the frozen Mac app skips it.
                        if let iphone = machine.iphone { IPhoneControlSection(ref: ref, iphone: iphone, online: online) }
                        #endif
                        if let system {
                            VStack(alignment: .leading, spacing: Theme.Space.m) {
                                heading(String(localized: "System"))
                                SystemFacts(system: system, machine: machine)
                            }
                        }
                        launchersSection(machine)
                        // What plugins draw for this machine (docs/extensions/extensions.md「UI slots」machine_section).
                        ExtensionMachineSections(page: plugins, link: $pluginLink)
                        VStack(alignment: .leading, spacing: Theme.Space.m) {
                            if online { connection(machine) }
                            Text("Monitoring history is kept for 24 hours: one point every 10 seconds for the last hour, one a minute before that. Gaps while disconnected aren't joined.")
                                .font(Theme.Font.label).foregroundStyle(.secondary)
                                .padding(.horizontal, Theme.Space.xs)
                        }
                    } else if model.machines.isEmpty {
                        ProgressView().frame(maxWidth: .infinity).padding(.vertical, Theme.Space.xxl)
                    } else {
                        EmptyStateView(symbol: Theme.Icon.machines, title: String(localized: "Can't find this machine"), message: String(localized: "It's no longer in the hub's configuration."))
                    }
                }
                .padding(Theme.Space.l)
            }
            #if DEBUG
            // `-openMachineClipboard YES`: the page opens scrolled to Clipboard's top (`bottom`: its end, for the largest
            // text sizes; screenshots).
            .task(id: machine?.clipboard != nil) {
                let at = UserDefaults.standard.string(forKey: "openMachineClipboard")
                guard machine?.clipboard != nil, at == "YES" || at == "bottom" else { return }
                try? await Task.sleep(for: .milliseconds(300))
                scroller.scrollTo(Self.clipboardAnchor, anchor: at == "bottom" ? .bottom : .top)
            }
            // `-openBrowserLogins YES`: the page opens scrolled to Browser Sign-ins' top (screenshots, #5821).
            .task(id: machine?.browser?.setUp == true) {
                guard machine?.browser?.setUp == true, UserDefaults.standard.string(forKey: "openBrowserLogins") != nil else { return }
                try? await Task.sleep(for: .seconds(1)) // after the charts above have their height
                scroller.scrollTo(Self.browserLoginsAnchor, anchor: .top)
            }
            // `-openLinkSwitch YES`: the page opens with its Link-switch card at the top (screenshots, #10671).
            .task(id: machine?.linkSwitch) {
                guard machine?.linkSwitch != nil, UserDefaults.standard.string(forKey: "openLinkSwitch") != nil else { return }
                try? await Task.sleep(for: .milliseconds(300))
                scroller.scrollTo(Self.linkSwitchAnchor, anchor: .top)
                // A different phase can change the card's height after that first layout, especially at xxxLarge.
                try? await Task.sleep(for: .milliseconds(300))
                scroller.scrollTo(Self.linkSwitchAnchor, anchor: .top)
            }
            #endif
        }
        .themedGroupedPage()
        .pluginLinks($pluginLink)
        .navigationDestination(isPresented: $filesOpen) { // a leaf: the browser walks folders and opens files in place
            FilesPane(machineID: machineID, group: ref.group, cwd: "~", transfers: folderTransfers)
                .detailTitle(String(localized: "Files"))
                .navigationBarTitleDisplayMode(.inline)
        }
        .navigationDestination(isPresented: $browserLoginsOpen) { // a leaf: nothing is pushed from the list
            BrowserLoginsListView(logins: browserLogins, online: online)
        }
        .extensionPushes(Array(plugins.models.values))
        .detailTitle(machine?.name ?? machineID)
        .navigationBarTitleDisplayMode(.inline)
        .alert(String(localized: "Rename Computer"), isPresented: $renaming) {
            TextField(machineID, text: $newName)
                .accessibilityIdentifier("machine.rename.field")
            Button("Cancel", role: .cancel) {}
            Button("Save") { Task { await model.renameMachine(ref, to: newName) } }
        } message: {
            Text("Shown in every app and on this computer's Persona Link. Leave it empty to use the id; the id stays the same.")
        }
        .refreshable {
            // Four independent reloads of disjoint state (the hub snapshot, the metrics, the launchers, the
            // plugin sections): run them together instead of one after another. A launchers read that races
            // the machine coming online is covered again by the `.task(id: machine?.online == true)` below.
            async let refreshed: Void = model.refresh()
            async let metrics: Void = load()
            async let launchersLoaded: Void = loadLaunchers(refresh: true)
            async let sections: Void = plugins.load(ref, app: model, redraw: true)
            _ = await (refreshed, metrics, launchersLoaded, sections)
        }
        #if os(macOS)
        .macRefreshes(id: machineID) {
            async let refreshed: Void = model.refresh()
            async let metrics: Void = load()
            async let launchersLoaded: Void = loadLaunchers(refresh: true)
            async let sections: Void = plugins.load(ref, app: model, redraw: true)
            _ = await (refreshed, metrics, launchersLoaded, sections)
        }
        #endif
        .tabActiveTask(id: Poll(range: range, active: phase == .active)) { // stops in the background and in another tab
            guard phase == .active else { return }
            while !Task.isCancelled {
                await load()
                try? await Task.sleep(for: .seconds(range == .hour ? 10 : 60))
            }
        }
        // Again when it comes online (after Reconnect): an offline machine has no launchers to list.
        .task(id: machine?.online == true) { await loadLaunchers(refresh: false) }
        // Again once the hub says it has the slots (the page can open before its hello).
        .task(id: ExtensionsKey(ref: ref, slots: model.features(ref.group).hasExtensionFeature("ui_slots"))) {
            await plugins.load(ref, app: model, redraw: false)
        }
    }

    private static let clipboardAnchor = "machine-clipboard"
    private static let browserLoginsAnchor = "machine-browser-logins"
    private static let linkSwitchAnchor = "machine-link-switch"

    nonisolated private struct Poll: Hashable, Sendable { var range: MetricRange; var active: Bool }
    nonisolated private struct ExtensionsKey: Hashable, Sendable { var ref: MachineRef; var slots: Bool }

    private func heading(_ title: String, count: Int? = nil) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: Theme.Space.sm) {
            Text(title).font(Theme.Font.heading)
            if let count { Text("\(count)").font(Theme.Font.secondary.monospacedDigit()).foregroundStyle(.secondary) }
        }
        .padding(.horizontal, Theme.Space.xs)
    }

    // MARK: Agents

    /// Needs-you first, then working, then the rest, each in the hub's order.
    private func agentsSection(_ machine: Machine) -> some View {
        let rank: (AgentState) -> Int = { $0 == .needsYou ? 0 : $0 == .working ? 1 : 2 }
        let sessions = Self.agents(model, ref).enumerated()
            .sorted { (rank($0.element.state), $0.offset) < (rank($1.element.state), $1.offset) }
            .map(\.element)
        let shown = allAgents ? sessions : Array(sessions.prefix(Self.agentsShown))
        return VStack(alignment: .leading, spacing: Theme.Space.m) {
            heading("Agent", count: machine.online ? sessions.count : nil)
            if sessions.isEmpty {
                Text(machine.online ? String(localized: "No agents running [Machine Detail View 658973]") : String(localized: "Machine offline. Its agents appear once it connects."))
                    .font(Theme.Font.secondary).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .card(padding: 14)
            } else {
                VStack(spacing: 0) {
                    ForEach(Array(shown.enumerated()), id: \.element.id) { i, s in
                        if i > 0 { Divider().padding(.leading, Theme.Space.l) }
                        NavigationLink(value: s.key) {
                            HStack(spacing: Theme.Space.s) {
                                SessionRow(session: s, machine: machine, showMachine: false)
                                ThemedIcon(Theme.Icon.disclosure).font(Theme.Font.labelStrong).foregroundStyle(.tertiary)
                            }
                            .padding(.horizontal, Theme.Space.l)
                            .padding(.vertical, Theme.Space.m)
                            .contentShape(.rect)
                        }
                        .buttonStyle(.plain)
                    }
                    if sessions.count > shown.count {
                        Divider()
                        Button {
                            withAnimation(Theme.Motion.layout) { allAgents = true }
                        } label: {
                            Text("Show All \(sessions.count)").font(Theme.Font.secondaryMedium).lineLimit(1)
                                .frame(maxWidth: .infinity)
                                .padding(.vertical, Theme.Space.m)
                                .contentShape(.rect)
                        }
                        .buttonStyle(.plain)
                        .foregroundStyle(Theme.accent)
                    }
                }
                .card(padding: 0)
            }
            engineRow(machine)
            if machine.online { terminalsRow }
        }
    }

    /// This computer's terminals, plain ones too (#4754), pushed by value as the Agents menu pushes them: a view push in
    /// this value stack could leave on screen a page whose load never ran (#6153). Every stack that shows this page
    /// declares `TerminalsRoute`.
    private var terminalsRow: some View {
        NavigationLink(value: TerminalsRoute(machine: ref.path)) {
            HStack(spacing: Theme.Space.s) {
                ThemedIcon(Theme.Icon.terminal).font(Theme.Font.body).foregroundStyle(Theme.accent).scaledFrame(width: Theme.Size.icon)
                Text("Terminals").font(Theme.Font.body).lineLimit(1)
                Spacer(minLength: Theme.Space.s)
                ThemedIcon(Theme.Icon.disclosure).font(Theme.Font.labelStrong).foregroundStyle(.tertiary)
            }
            .padding(.horizontal, Theme.Space.l)
            .padding(.vertical, Theme.Space.m)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("machine.terminals")
        .card(padding: 0)
    }

    /// Which engine's sessions the machine shows (herdr, Herdr App…): it decides the list above.
    private func engineRow(_ machine: Machine) -> some View {
        VStack(alignment: .leading, spacing: Theme.Space.s) {
            NavigationLink { // a leaf: nothing is pushed from the picker, so a view push is fine in this value stack
                EngineSelectionView(ref: ref)
            } label: {
                HStack(spacing: Theme.Space.s) {
                    Text("Engine").font(Theme.Font.body).lineLimit(1)
                    Spacer(minLength: Theme.Space.s)
                    Text(machine.engineName).font(Theme.Font.body).foregroundStyle(.secondary).lineLimit(1)
                    ThemedIcon(Theme.Icon.disclosure).font(Theme.Font.labelStrong).foregroundStyle(.tertiary)
                }
                .padding(.horizontal, Theme.Space.l)
                .padding(.vertical, Theme.Space.m)
                .contentShape(.rect)
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("machine.engine")
            .card(padding: 0)
            Text(machine.engineHosts?.isEmpty == true ? String(localized: "Shows the desktop apps' sessions.")
                 : String(localized: "Only sessions in the selected engine are shown."))
                .font(Theme.Font.label).foregroundStyle(.secondary)
                .padding(.horizontal, Theme.Space.xs)
        }
    }

    /// Its hub renames computers, and the computer is this hub's own (a linked hub's is renamed on that hub).
    private func canRename(_ machine: Machine) -> Bool {
        model.canRenameMachine(ref.group) && machine.kind != "hub"
    }

    /// Rename: what the hub calls the computer (docs/protocol/protocol.md "Renaming a computer"), in an alert.
    private func renameRow(_ machine: Machine) -> some View {
        Button {
            Haptics.tap()
            newName = machine.name == machine.id ? "" : machine.name
            renaming = true
        } label: {
            HStack(spacing: Theme.Space.s) {
                ThemedIcon(Theme.Icon.edit).font(Theme.Font.body).foregroundStyle(Theme.accent).scaledFrame(width: Theme.Size.icon)
                Text("Rename").font(Theme.Font.body).lineLimit(1)
                Spacer(minLength: Theme.Space.s)
                Text(machine.name).font(Theme.Font.body).foregroundStyle(.secondary).lineLimit(1)
                ThemedIcon(Theme.Icon.disclosure).font(Theme.Font.labelStrong).foregroundStyle(.tertiary)
            }
            .padding(.horizontal, Theme.Space.l)
            .padding(.vertical, Theme.Space.m)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("machine.rename")
        .card(padding: 0)
    }

    /// Files: the machine's home in the file browser (upload into a folder, Save to Files; #4858).
    private func filesRow(online: Bool) -> some View {
        Button {
            Haptics.tap()
            filesOpen = true
        } label: {
            HStack(spacing: Theme.Space.s) {
                ThemedIcon(Theme.Icon.folder).font(Theme.Font.body).foregroundStyle(Theme.accent).scaledFrame(width: Theme.Size.icon)
                Text("Files").font(Theme.Font.body).lineLimit(1)
                Spacer(minLength: Theme.Space.s)
                Text(online ? String(localized: "Home") : String(localized: "Offline [Machine Entity be1b4f]")).font(Theme.Font.body).foregroundStyle(.secondary).lineLimit(1)
                ThemedIcon(Theme.Icon.disclosure).font(Theme.Font.labelStrong).foregroundStyle(.tertiary)
            }
            .padding(.horizontal, Theme.Space.l)
            .padding(.vertical, Theme.Space.m)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .disabled(!online)
        .accessibilityIdentifier("machine.files")
        .card(padding: 0)
    }

    // MARK: Launchers, connection

    private func launchersSection(_ machine: Machine) -> some View {
        VStack(alignment: .leading, spacing: Theme.Space.m) {
            heading(String(localized: "Launcher"), count: machine.online && !launchers.isEmpty ? launchers.count : nil)
            VStack(alignment: .leading, spacing: 0) {
                if !machine.online {
                    Text("Machine offline. Shown once it connects.").font(Theme.Font.secondary).foregroundStyle(.secondary).padding(.vertical, Theme.Space.s)
                } else if let launcherError, launchers.isEmpty {
                    Label(launcherError, systemImage: Theme.Icon.error).font(Theme.Font.caption).foregroundStyle(Theme.danger)
                        .padding(.vertical, Theme.Space.s)
                } else if launchers.isEmpty {
                    ProgressView().frame(maxWidth: .infinity).padding(.vertical, Theme.Space.s)
                }
                ForEach(Array(launchers.enumerated()), id: \.element.id) { i, l in
                    if i > 0 { Divider() }
                    HStack {
                        Text(l.name).font(Theme.Font.body.monospaced())
                        Spacer()
                        Text(Fmt.model(l.model) ?? "").font(Theme.Font.label).foregroundStyle(.secondary).lineLimit(1)
                    }
                    .padding(.vertical, Theme.Space.s)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .card(padding: 14)
        }
    }

    /// Details: what the hub said, under the card's plain words.
    private func problemDetails(_ raw: String) -> some View {
        DisclosureGroup {
            Text(raw).font(Theme.Font.label).foregroundStyle(.secondary).textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.top, Theme.Space.xs)
        } label: {
            Text("Details [Readiness Card 979a33]").font(Theme.Font.secondary).lineLimit(1)
        }
        .accessibilityIdentifier("machine.problemDetails")
        .card(padding: 14)
    }

    /// How the hub reaches the machine, and Reconnect.
    private func connection(_ machine: Machine) -> some View {
        HStack(spacing: Theme.Space.m) {
            Text(machine.kind == "local" ? String(localized: "The hub machine runs the node directly.")
                 : machine.kind == "link" ? String(localized: "This computer connects to the hub through Persona Link.") : String(localized: "The hub connects via `ssh \(machine.sshHost ?? machine.id)`."))
                .font(Theme.Font.label).foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button { Task { await reconnect() } } label: {
                if reconnecting {
                    ProgressView().controlSize(.small)
                } else {
                    Label("Reconnect", systemImage: Theme.Icon.retry).font(Theme.Font.labelStrong).lineLimit(1)
                }
            }
            .themedButton()
            .disabled(reconnecting)
            .accessibilityLabel(reconnecting ? String(localized: "Reconnecting") : String(localized: "Reconnect"))
        }
        .card(padding: 14)
    }

    // MARK: Loading

    private func load() async {
        guard let api = model.api(ref.group) else { return }
        let r = range
        metricsRequest += 1
        let request = metricsRequest
        do {
            let fresh = try await api.machineMetrics(machineID, range: r)
            let total = Double(fresh.latest?.memTotal ?? fresh.system?.memory ?? model.machine(ref)?.system?.memory ?? 0)
            let definitions: [(MetricSeriesKey, String)] = [
                (.cpu, "CPU"), (.memory(total: total), String(localized: "Memory [Machine Detail View 7d8f8c]")),
                (.down, String(localized: "Down")), (.up, String(localized: "Up")), (.load, String(localized: "Load")),
            ]
            let prepared = await Task.detached(priority: .userInitiated) {
                Dictionary(uniqueKeysWithValues: definitions.map { key, name in
                    (key, PreparedMetricSeries.build(fresh.points, key: key, name: name))
                })
            }.value
            try Task.checkCancellation()
            guard request == metricsRequest, r == range else { return }
            preparedCharts = prepared
            chartEnd = .now
            data = fresh
            shownRange = r
            failure = nil
            #if DEBUG
            // `-openMonitorScrub 0.8`: the charts open held at that share of the range (screenshots of a scrub).
            if selection == nil, let f = UserDefaults.standard.object(forKey: "openMonitorScrub") as? Double ?? Double(UserDefaults.standard.string(forKey: "openMonitorScrub") ?? "") {
                selection = Date.now.addingTimeInterval(-r.seconds * (1 - f))
            }
            #endif
        } catch {
            if error.isCancellation || request != metricsRequest || r != range { return }
            // Keep the time domain advancing on failed polls too; unrelated session pushes do not move it.
            chartEnd = .now
            if let e = error as? APIError, e.status == 404 {
                failure = String(localized: "This hub has no monitoring data yet. Deploy a newer hub and the node on this computer, then check again.")
            } else if data == nil {
                failure = error.shown ?? String(localized: "Can't reach the hub. This refreshes automatically once it's back.")
            }
        }
    }

    private func loadLaunchers(refresh: Bool) async {
        guard model.machine(ref)?.online == true else { launchers = []; return }
        do {
            launchers = try await model.launchers(ref, refresh: refresh)
            launcherError = nil
        } catch {
            launcherError = error.shown.map { String(localized: "Couldn't load launchers: \($0)") }
        }
    }

    /// Asks the hub to reconnect, then waits (up to 15 s) for the machine to come online; says so if it does not.
    private func reconnect() async {
        reconnecting = true
        defer { reconnecting = false }
        do {
            try await model.api(ref.group)?.reconnect(machine: machineID)
        } catch {
            if let m = error.shown { model.toast = Toast(String(localized: "Couldn't reconnect: \(m)"), style: .error) }
            return
        }
        for _ in 0..<10 {
            try? await Task.sleep(for: .seconds(1.5))
            await model.refresh()
            if model.machine(ref)?.online == true {
                Haptics.success()
                await loadLaunchers(refresh: true)
                return
            }
        }
        model.toast = Toast(Self.stillOffline(model.machine(ref)), style: .error)
    }

    /// What Reconnect says when the machine is still offline: why and what to do, in plain words, read from the error the
    /// hub keeps (`NodeHealth.connectionFailure`, the same kinds as the web's `classifyConnection`). The raw error
    /// stays on the card.
    static func stillOffline(_ m: Machine?) -> String {
        let text = m?.error ?? ""
        let link = m?.kind == "link" || m?.kind == "service"
        switch m.flatMap(NodeHealth.connectionFailure) {
        case .auth:
            return link ? String(localized: "Not connected yet: authentication failed. Pair Persona Link again on this computer, then reconnect")
                : String(localized: "Not connected yet: authentication failed. Check that this computer's SSH key and account are still valid, then reconnect")
        case .version:
            if m.map(NodeHealth.canDeploy) == true {
                return String(localized: "Not connected yet: version mismatch. Reconnect first; then Redeploy updates the node to the hub's version")
            }
            return String(localized: "Not connected yet: version mismatch. Upgrade the node on this computer so it matches the hub")
        case .timeout:
            return String(localized: "Not connected yet: the connection timed out. The network is unstable or the computer is asleep. Wait for it to wake up or switch networks, then reconnect")
        case .unreachable:
            return link ? String(localized: "Not connected yet. Make sure this computer is on and Persona Link is running, then reconnect")
                : String(localized: "Not connected yet. Make sure this computer is on, the network is up and the node is running, then reconnect")
        case nil:
            return text.isEmpty ? String(localized: "Not connected yet. Try again in a moment") : String(localized: "Not connected yet. See the card above for the reason")
        }
    }

    // MARK: Charts

    private var networkPrepared: [PreparedMetricSeries]? {
        guard let down = preparedCharts[.down], let up = preparedCharts[.up] else { return nil }
        return [down, up]
    }

    @ViewBuilder private func charts(system: MachineSystem?) -> some View {
        if let data, shownRange != nil {
            let r = shownRange ?? range
            let end = chartEnd
            let domain = end.addingTimeInterval(-r.seconds)...end
            let step: TimeInterval = r == .hour ? 10 : r == .sixHours ? 60 : 240
            let memTotal = Double(data.latest?.memTotal ?? system?.memory ?? 0)
            let live = data.online ? data.latest : nil
            let cores = Double(system?.cores ?? 0)
            Group {
                MetricChart(title: "CPU", points: data.points, series: [ChartSeries(name: "CPU", color: MetricColor.cpu, key: .cpu) { $0.cpu }],
                            domain: domain, step: step, yDomain: 0...100, format: { MetricFormat.percent($0) },
                            now: [live?.cpu], prepared: preparedCharts[.cpu].map { [$0] }, formatKey: "percent",
                            selectionSnapshot: selection, selection: $selection).equatable()
                MetricChart(title: memTotal > 0 ? String(localized: "Memory (\(MetricFormat.bytes(memTotal)) total)") : String(localized: "Memory [Machine Detail View 7d8f8c]"), points: data.points,
                            series: [ChartSeries(name: String(localized: "Memory [Machine Detail View 7d8f8c]"), color: MetricColor.mem, key: .memory(total: memTotal)) { p in p.mem.map { memTotal > 0 ? 100 * $0 / memTotal : 0 } }],
                            domain: domain, step: step, yDomain: 0...100, format: { MetricFormat.percent($0) },
                            now: [live?.memPercent], prepared: preparedCharts[.memory(total: memTotal)].map { [$0] }, formatKey: "percent",
                            selectionSnapshot: selection, selection: $selection).equatable()
                MetricChart(title: String(localized: "Network"), points: data.points, series: [
                    ChartSeries(name: String(localized: "Down"), color: MetricColor.rx, key: .down) { $0.rx },
                    ChartSeries(name: String(localized: "Up"), color: MetricColor.tx, key: .up) { $0.tx },
                ], domain: domain, step: step, format: { MetricFormat.rate($0) }, now: [live?.netRx, live?.netTx],
                            prepared: networkPrepared, formatKey: "rate", selectionSnapshot: selection, selection: $selection).equatable()
                MetricChart(title: String(localized: "Load (1 min)"), points: data.points, series: [ChartSeries(name: String(localized: "Load"), color: MetricColor.load, key: .load) { $0.load }],
                            domain: domain, step: step, reference: cores > 0 ? (cores, String(localized: "\(Int(cores)) cores")) : nil,
                            format: { MetricFormat.load($0) }, now: [live?.load], prepared: preparedCharts[.load].map { [$0] },
                            formatKey: "load", selectionSnapshot: selection, selection: $selection).equatable()
            }
            .animation(.default, value: shownRange)
            if data.points.allSatisfy(\.isGap) {
                Text(data.online ? String(localized: "Recording just started. Charts will appear here shortly.") : String(localized: "This machine wasn't connected during this period."))
                    .font(Theme.Font.caption).foregroundStyle(.secondary).frame(maxWidth: .infinity)
            }
        } else if let failure {
            EmptyStateView(symbol: "chart.xyaxis.line", title: String(localized: "No monitoring data"), message: failure)
                .padding(.vertical, 30)
        } else {
            ProgressView().frame(maxWidth: .infinity).padding(.vertical, 60)
        }
    }
}

/// The values now, two columns: CPU, memory, network, load, disk, swap.
private struct NowGrid: View {
    var sample: MetricSample
    var cores: Int?

    var body: some View {
        let cols = [GridItem(.flexible(), spacing: 10), GridItem(.flexible(), spacing: 10)]
        LazyVGrid(columns: cols, spacing: 10) {
            tile("CPU", MetricFormat.percent(sample.cpu), detail: nil, fraction: sample.cpu.map { $0 / 100 }, color: MetricColor.cpu,
                 level: MetricLevel.of(sample.cpu, warn: 75, crit: 90))
            tile(String(localized: "Memory [Machine Detail View 7d8f8c]"), MetricFormat.percent(sample.memPercent), detail: MetricFormat.bytes(sample.memUsed) + " / " + MetricFormat.bytes(sample.memTotal),
                 fraction: sample.memPercent.map { $0 / 100 }, color: MetricColor.mem, level: MetricLevel.of(sample.memPercent, warn: 80, crit: 90))
            tile(String(localized: "Network"), "↓ " + MetricFormat.rate(sample.netRx), detail: "↑ " + MetricFormat.rate(sample.netTx), fraction: nil, color: MetricColor.rx, level: .normal)
            tile(String(localized: "Load"), MetricFormat.load(sample.load), detail: cores.map { String(localized: "\($0) cores") }, fraction: nil, color: MetricColor.load,
                 level: MetricLevel.of(sample.load.flatMap { l in cores.map { l / Double(max($0, 1)) } }, warn: 1, crit: 2))
            tile(String(localized: "Disk"), MetricFormat.percent(sample.diskPercent), detail: MetricFormat.bytes(sample.diskUsed) + " / " + MetricFormat.bytes(sample.diskTotal),
                 fraction: sample.diskPercent.map { $0 / 100 }, color: .secondary, level: MetricLevel.of(sample.diskPercent, warn: 80, crit: 90))
            tile(String(localized: "Swap"), sample.swapTotal > 0 ? MetricFormat.bytes(sample.swapUsed) : String(localized: "Off [Machine Detail View f95ea7]"),
                 detail: sample.swapTotal > 0 ? String(localized: "\(MetricFormat.bytes(sample.swapTotal)) total") : nil,
                 fraction: sample.swapPercent.map { $0 / 100 }, color: .secondary, level: .normal)
            if let battery = sample.batteryPercent {
                let charging = sample.batteryCharging == true
                tile("Battery", MetricFormat.percent(battery), detail: charging ? "⚡" : nil,
                     fraction: battery / 100, color: .secondary,
                     level: !charging && battery <= 15 ? .critical : !charging && battery <= 25 ? .warning : .normal)
            }
            if let temp = sample.tempC {
                tile("Temp", String(format: "%.1f°C", temp), detail: nil,
                     fraction: min(1.0, max(0.0, temp / 125.0)), color: .secondary,
                     level: MetricLevel.of(temp, warn: 75, crit: 90))
            }
        }
    }

    @ViewBuilder private func tile(_ title: String, _ value: String, detail: String?, fraction: Double?, color: Color, level: MetricLevel) -> some View {
        if AppStyle.current.usesBeautifulUI {
            // Its stat tile: what it is (High / Critical after it), the number, the line under it, a meter for what has a
            // share; a high reading colors the number and its meter.
            BUIStatTile(level > .normal ? (level == .critical ? String(localized: "\(title) · Critical") : String(localized: "\(title) · High")) : title, value: value, detail: detail ?? " ",
                        tone: level == .critical ? .red : level > .normal ? .orange : .neutral, progress: fraction)
        } else {
            plainTile(title, value, detail: detail, fraction: fraction, color: color, level: level)
        }
    }

    private func plainTile(_ title: String, _ value: String, detail: String?, fraction: Double?, color: Color, level: MetricLevel) -> some View {
        VStack(alignment: .leading, spacing: Theme.Space.xs) {
            HStack(spacing: Theme.Space.xs) {
                Text(title).font(Theme.Font.label).foregroundStyle(.secondary)
                if level > .normal {
                    HStack(spacing: Theme.Space.xxs) {
                        ThemedIcon(Theme.Icon.errorFilled).foregroundStyle(MetricColor.level(level))
                        Text(level == .critical ? String(localized: "Critical") : String(localized: "High [Machine Detail View cde631]")).foregroundStyle(.secondary)
                    }
                    .font(Theme.Font.labelStrong).lineLimit(1)
                }
            }
            Text(value).font(Theme.Font.title.monospacedDigit()).lineLimit(1).minimumScaleFactor(0.6)
            Text(detail ?? " ").font(Theme.Font.label.monospacedDigit()).foregroundStyle(.secondary).lineLimit(1).minimumScaleFactor(0.7)
            if let fraction { MeterBar(fraction: fraction, color: level > .normal ? MetricColor.level(level) : color) }
            else { Color.clear.scaledFrame(height: Theme.Size.dotXS) } // every tile the same height
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .card(padding: 12)
        .accessibilityElement(children: .combine)
    }
}

/// Each core now (Linux), one thin column each.
private struct CoreBars: View {
    var cores: [Double]

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.Space.s) {
            HStack {
                Text("Per Core").font(Theme.Font.secondaryStrong)
                Spacer()
                Text("Busiest \(MetricFormat.percent(cores.max()))").font(Theme.Font.label.monospacedDigit()).foregroundStyle(.secondary)
            }
            GeometryReader { g in
                let gap: CGFloat = cores.count > 32 ? 1 : 2
                let w = max(1, (g.size.width - gap * CGFloat(cores.count - 1)) / CGFloat(cores.count))
                HStack(alignment: .bottom, spacing: gap) {
                    ForEach(cores.indices, id: \.self) { i in
                        ZStack(alignment: .bottom) {
                            RoundedRectangle(cornerRadius: min(2, w / 2)).fill(Color.primary.opacity(0.07))
                            RoundedRectangle(cornerRadius: min(2, w / 2)).fill(MetricColor.cpu)
                                .frame(height: max(2, g.size.height * cores[i] / 100))
                        }
                        .frame(width: w)
                    }
                }
            }
            .scaledFrame(height: Theme.Size.tapTarget)
        }
        .card(padding: 14)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Usage per core")
        .accessibilityValue("\(cores.count) cores, busiest \(MetricFormat.percent(cores.max()))")
    }
}

/// System: OS, kernel, hardware, host name, uptime, versions. A long value wraps under itself (a CPU's full name).
private struct SystemFacts: View {
    var system: MachineSystem
    var machine: Machine?

    var body: some View {
        let facts = facts
        VStack(alignment: .leading, spacing: 0) {
            ForEach(facts.indices, id: \.self) { i in
                if i > 0 { Divider() }
                HStack(alignment: .firstTextBaseline, spacing: Theme.Space.m) {
                    Text(facts[i].0).foregroundStyle(.secondary).fixedSize()
                    Spacer(minLength: Theme.Space.s)
                    Text(facts[i].1).multilineTextAlignment(.trailing).textSelection(.enabled)
                        .fixedSize(horizontal: false, vertical: true)
                }
                .font(Theme.Font.secondary)
                .padding(.vertical, Theme.Space.s)
            }
        }
        .card(padding: 14)
    }

    private var facts: [(String, String)] {
        var out: [(String, String)] = [(String(localized: "System"), system.build.map { String(localized: "\(system.name) (\($0))") } ?? system.name)]
        if let k = system.kernel, !k.isEmpty { out.append((String(localized: "Kernel"), k)) }
        out.append((String(localized: "Architecture"), system.arch ?? machine?.arch ?? "—"))
        if let m = system.model, !m.isEmpty { out.append((String(localized: "Model [Machine Detail View d082d0]"), m)) }
        if let c = system.cpu, !c.isEmpty { out.append((String(localized: "Processor"), c)) }
        out.append((String(localized: "Cores"), "\(system.cores)"))
        out.append((String(localized: "Memory [Machine Detail View 7d8f8c]"), MetricFormat.bytes(system.memory)))
        if let h = machine?.hostname, !h.isEmpty { out.append((String(localized: "Host Name"), h)) }
        if let boot = system.bootDate {
            out.append((String(localized: "Booted"), boot.formatted(date: .abbreviated, time: .shortened)))
            if machine?.online == true { out.append((String(localized: "machine.uptime", defaultValue: "Uptime"), MetricFormat.uptime(since: boot))) }
        }
        if let h = machine?.herdrVersion, !h.isEmpty { out.append(("herdr", h)) }
        if let c = machine?.claudeVersion, !c.isEmpty { out.append(("Claude Code", c)) }
        if let n = machine?.nodeVersion, !n.isEmpty { out.append(("Persona node", n)) }
        return out
    }
}
