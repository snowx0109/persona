import SwiftUI
import PersonaWire

// Agents › ⋯ › Terminals (#4754, docs/integrations/engines.md「Agents run in a terminal」): every pane of each computer's
// terminal engine, the Agents' and plain terminals alike, to open in the terminal (watch and type) and close. A plain pane
// opens by its `pane:` address; an Agent's, and a stopped Agent's left behind, by its session, whose terminal page offers
// Resume and closes as any (#4328). The list is the hub's `GET /machines/{mid}/panes`; nothing on a screen is read for it.
// A plain terminal is renamed from its row and its terminal page's ⋯ (#6996): its herdr or Tern tab, as the web's node
// menu does (`tabs/rename`).

/// The Terminals page: one computer's (`MachineRef.path`), or every computer's when nil.
nonisolated struct TerminalsRoute: Hashable {
    var machine: String? = nil
}

struct TerminalsView: View {
    @Environment(AppModel.self) private var model
    let only: String?
    /// Each computer's answer and its request, by `MachineRef.path`.
    @State private var loads: [String: TerminalsLoad] = [:]
    @State private var open: TerminalTarget?
    @State private var closing: CloseTarget?
    @State private var finished: FinishedTarget?
    /// The tab whose Rename Terminal prompt is open.
    @State private var renaming: TabRename.Target?
    /// The Agent whose rename prompt is open (its row's Rename Agent…, as the Agents list has it).
    @State private var renamingSession: Session?
    /// Computers with a rename on its way: the quiet refresh leaves their lists alone meanwhile.
    @State private var renamingOn: Set<String> = []

    /// A terminal to show over the page; `rename`: the tab its ⋯ renames (a plain terminal's).
    struct TerminalTarget: Identifiable {
        let key: SessionKey
        var rename: TabRename.Target? = nil
        var id: String { key.group + "|" + key.machineID + "|" + key.sessionID }
    }
    /// A pane to close, asked first.
    struct CloseTarget: Identifiable { let machine: MachineRef; let pane: TerminalPanes.Pane; let title: String; var id: String { machine.path + pane.pane } }
    /// The terminals stopped Agents left on a computer, as the node lists them now (preview), to close once confirmed.
    struct FinishedTarget: Identifiable { let machine: MachineRef; let name: String; let panes: [FinishedTerminal]; var id: String { machine.path } }

    private var machines: [MachineChoice] { model.machineChoices.filter { only == nil || $0.id == only } }

    var body: some View {
        List {
            ForEach(machines) { m in
                let ref = MachineRef(path: m.id)
                Section {
                    content(ref)
                } header: {
                    header(m, ref)
                }
            }
        }
        .listStyle(.plain)
        .themedPage()
        .overlay {
            if machines.isEmpty {
                EmptyStateView(symbol: Theme.Icon.terminal, title: String(localized: "No computers"),
                               message: String(localized: "Terminals on your computers show up here once one is connected."))
            }
        }
        .navigationTitle("Terminals")
        #if os(iOS) || os(visionOS)
        .navigationBarTitleDisplayMode(.inline)
        #endif
        // Again when the computers change: right after the app connects the list is still empty.
        .task(id: machines.map(\.id)) { await load() }
        // Then every few seconds while the page is in front, as the web's Terminal page reads it: a rename, a new or a
        // closed terminal on the computer or on another device shows up here (P9: stops when the page is not in front).
        .tabActiveTask(id: machines.map(\.id)) {
            while !Task.isCancelled {
                do { try await Task.sleep(for: TerminalsLoad.refreshEvery) } catch { return }
                await refreshQuietly()
            }
        }
        .refreshable { await load() }
        #if os(iOS)
        .fullScreenCover(item: $open) { t in TerminalCover(key: t.key, rename: coverRename(t)).presentationBackground(.clear) }
        #else
        .sheet(item: $open) { t in TerminalCover(key: t.key, rename: coverRename(t)) }
        #endif
        .renameTerminalAlert($renaming) { t, name in Task { await rename(t, to: name) } }
        .renameSessionAlert($renamingSession)
        .accessibleConfirmationDialog(closing.map { String(localized: "Close \"\($0.title)\"?") } ?? "", isPresented: Binding(get: { closing != nil }, set: { if !$0 { closing = nil } }),
                            titleVisibility: .visible, hasCancel: true, presenting: closing) { c in
            Button(String(localized: "Close Terminal"), role: .destructive) { close(c) }
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            Text("Whatever runs in it stops. This can't be undone.")
        }
        .accessibleConfirmationDialog(finished.map { String(localized: "Close \($0.panes.count) finished terminals on \($0.name)?") } ?? "",
                            isPresented: Binding(get: { finished != nil }, set: { if !$0 { finished = nil } }),
                            titleVisibility: .visible, hasCancel: true, presenting: finished) { f in
            Button(String(localized: "Close Terminals"), role: .destructive) { closeFinished(f) }
            Button("Cancel", role: .cancel) {}
        } message: { f in
            Text(f.panes.map { title(finished: $0.sessionID, on: f.machine) }.joined(separator: "\n"))
        }
    }

    // MARK: - Rows

    @ViewBuilder private func content(_ ref: MachineRef) -> some View {
        let load = loads[ref.path] ?? TerminalsLoad()
        switch load.shown {
        case .failed(let error):
            problem(error, ref)
        case .late(let request):
            problem(TerminalsLoad.withRequest(String(localized: "This computer hasn't answered yet."), request), ref)
        case .rows(let list):
            let rows = TerminalsList.rows(list)
            if rows.isEmpty { empty(list, ref) }
            ForEach(rows) { r in row(r, on: ref, in: list).themedRow() }
        case .waiting:
            // Its panes are on their way; no answer within `patience` says so, with Retry (#6153). The row's own task
            // times it, so the page on screen can't sit on a bare spinner even if no load of its own ran.
            ProgressView().frame(maxWidth: .infinity).themedRow()
                .task(id: load.request) {
                    try? await Task.sleep(for: TerminalsLoad.patience)
                    if !Task.isCancelled { loads[ref.path, default: TerminalsLoad()].expire(load.request) }
                }
        }
    }

    /// A computer with no terminals: New Terminal when its engine can open one (in its home folder when it has no
    /// workspace yet), otherwise where terminals come from.
    @ViewBuilder private func empty(_ list: TerminalPanes, _ ref: MachineRef) -> some View {
        if let space = TerminalsList.spaces(list, newSpace: hubHasNewSpace(ref)).first {
            HStack(spacing: Theme.Space.m) {
                Text("No terminals open on this computer.").font(Theme.Font.label).foregroundStyle(.secondary)
                Spacer(minLength: 0)
                Button("New Terminal") { newTerminal(on: ref, space: space) }
                    .foregroundStyle(Theme.accent).lineLimit(1).accessibilityIdentifier("terminals.new")
            }
            .themedRow()
        } else {
            Text("Terminals open here when an Agent starts on this computer.").font(Theme.Font.label).foregroundStyle(.secondary).themedRow()
        }
    }

    /// Why a computer's terminals aren't listed, with Retry.
    private func problem(_ text: String, _ ref: MachineRef) -> some View {
        HStack(spacing: Theme.Space.m) {
            Text(text).font(Theme.Font.label).foregroundStyle(.secondary)
            Spacer(minLength: 0)
            Button("Retry") { Task { await load([ref.path]) } }.foregroundStyle(Theme.accent).lineLimit(1).accessibilityIdentifier("terminals.retry")
        }
        .themedRow()
    }

    private func row(_ r: TerminalsList.Row, on ref: MachineRef, in list: TerminalPanes) -> some View {
        let name = rowTitle(r, on: ref, in: list)
        return Button { show(r, on: ref, in: list) } label: {
            HStack(spacing: Theme.Space.m) {
                ThemedIcon(r.kind == .agent ? Theme.Icon.agents : Theme.Icon.terminal)
                    .font(Theme.Font.body)
                    .foregroundStyle(r.kind == .finished ? .secondary : .primary)
                    .scaledFrame(width: Theme.Size.icon) // one column: the titles and separators line up
                VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                    Text(name).font(Theme.Font.body).foregroundStyle(Color.primary).lineLimit(1)
                    Text(subtitle(r, on: ref, title: name)).font(Theme.Font.label).foregroundStyle(.secondary).lineLimit(1)
                }
                Spacer(minLength: 0)
            }
            .padding(.vertical, Theme.Space.xs)
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(.rect)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("terminals.row")
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            if r.kind != .agent {
                Button(role: .destructive) { closing = CloseTarget(machine: ref, pane: r.pane, title: name) } label: {
                    Label("Close Terminal", systemImage: Theme.Icon.end)
                }
            }
        }
        .contextMenu {
            Button { show(r, on: ref, in: list) } label: { Label("Open Terminal", systemImage: Theme.Icon.terminal) }
            if let tab = TabRename.target(r, in: list, on: ref) {
                Button { renaming = tab } label: { Label("Rename… [Assistant Sidebar ec4cd0]", systemImage: Theme.Icon.edit) }
            } else if r.kind == .agent, let s = agentSession(r, on: ref), s.canRename {
                Button { renamingSession = s } label: {
                    Label(String(localized: "agent.rename", defaultValue: "Rename Agent…"), systemImage: Theme.Icon.edit)
                }
            }
            if r.kind != .agent {
                Button(role: .destructive) { closing = CloseTarget(machine: ref, pane: r.pane, title: name) } label: {
                    Label("Close Terminal…", systemImage: Theme.Icon.end)
                }
            }
        }
    }

    @ViewBuilder private func header(_ m: MachineChoice, _ ref: MachineRef) -> some View {
        HStack(spacing: Theme.Space.sm) {
            ThemedIcon(m.symbol)
            Text(m.name).lineLimit(1)
            Spacer(minLength: 0)
            if let list = loads[ref.path]?.list {
                Menu {
                    ForEach(TerminalsList.spaces(list, newSpace: hubHasNewSpace(ref)), id: \.id) { s in
                        Button { newTerminal(on: ref, space: s) } label: {
                            Label(String(localized: "New Terminal in \(s.label)"), systemImage: Theme.Icon.add)
                        }
                    }
                    if TerminalsList.hasFinished(list) {
                        Divider()
                        Button(role: .destructive) { askCloseFinished(ref, name: m.name) } label: {
                            Label("Close Finished Terminals…", systemImage: Theme.Icon.cleanUpAll)
                        }
                    }
                } label: {
                    // A finger's square, the glyph at its trailing edge where it was. A menu is hit only in its label's
                    // frame, and the header's cell holds no more than it lays out: the square takes `Space.s` above and
                    // below as room, so the header grows 10 pt rather than 26, and all of the square is inside it (#6577).
                    ThemedIcon(Theme.Icon.more).tapTargetFrame(alignment: .trailing)
                }
                .padding(.vertical, -Theme.Space.s)
                .accessibilityLabel(String(localized: "Terminal actions for \(m.name)"))
                .accessibilityIdentifier("terminals.more")
            }
        }
        .font(Theme.Font.secondaryStrong)
    }

    private func agentSession(_ r: TerminalsList.Row, on ref: MachineRef) -> Session? {
        r.pane.sessionID.flatMap { model.session(SessionKey(group: ref.group, machineID: ref.id, sessionID: $0)) }
    }

    private func rowTitle(_ r: TerminalsList.Row, on ref: MachineRef, in list: TerminalPanes) -> String {
        switch r.kind {
        case .agent:
            let s = agentSession(r, on: ref)
            return s?.title.nilIfEmpty ?? r.pane.title?.nilIfEmpty ?? NodeMenu.shellTitle(r.pane, in: list)
        case .finished:
            return title(finished: r.pane.agentSession ?? "", on: ref)
        case .plain:
            return NodeMenu.shellTitle(r.pane, in: list)
        }
    }

    private func title(finished id: String, on ref: MachineRef) -> String {
        let s = model.session(SessionKey(group: ref.group, machineID: ref.id, sessionID: id))
        let name = s?.title.nilIfEmpty ?? String(id.prefix(8))
        return String(localized: "Finished: \(name)")
    }

    private func subtitle(_ r: TerminalsList.Row, on ref: MachineRef, title: String) -> String {
        let dir = TerminalsList.folder(r.pane.cwd)
        switch r.kind {
        case .agent:
            // The name the app gives the Agent everywhere (Session.agentLabel), not the runtime's id.
            let s = agentSession(r, on: ref)
            return [s?.agentLabel ?? TerminalsList.agentName(r.pane.agent), dir].filter { !$0.isEmpty }.joined(separator: " · ")
        case .finished, .plain:
            return ([String(localized: "Terminal")] + TerminalsList.plainDetails(title: title, foreground: r.pane.foreground, folder: dir))
                .joined(separator: " · ")
        }
    }

    // MARK: - Actions

    private func show(_ r: TerminalsList.Row, on ref: MachineRef, in list: TerminalPanes) {
        let session: String
        switch r.kind {
        case .agent: session = r.pane.sessionID ?? r.pane.terminal
        case .finished: session = r.pane.agentSession ?? r.pane.terminal // its session's terminal: the shell it left, Resume
        case .plain: session = r.pane.terminal
        }
        open = TerminalTarget(key: SessionKey(group: ref.group, machineID: ref.id, sessionID: session), rename: TabRename.target(r, in: list, on: ref))
    }

    /// The terminal page's Rename… (a plain terminal's), saved as the row's is.
    private func coverRename(_ t: TerminalTarget) -> TerminalCover.Rename? {
        t.rename.map { TerminalCover.Rename(target: $0, save: { await rename($0, to: $1) }) }
    }

    /// Rename Terminal's Save: the new name shows in the list at once and goes back if the computer refuses it, with one
    /// plain sentence (offline, an engine that can't, a terminal already closed). True once the computer took it.
    @discardableResult
    private func rename(_ t: TabRename.Target, to raw: String) async -> Bool {
        let name = TabRename.cleaned(raw), path = t.machine.path
        guard TabRename.canSave(name, current: t.name), let api = model.api(t.machine.group) else { return false }
        renamingOn.insert(path)
        defer { renamingOn.remove(path) }
        if let list = loads[path]?.list { loads[path]?.list = TabRename.renamed(list, host: t.host, tab: t.tabID, to: name) }
        do {
            try await api.nodeAction(machine: t.machine.id, "tabs/rename", TabRename.body(t, name: name))
            model.toast = Toast(String(localized: "Renamed to \(name)"), style: .success)
            Haptics.success()
            await load([path])
            return true
        } catch {
            // The old name back on whatever is shown now (a newer list may have come meanwhile).
            if let list = loads[path]?.list { loads[path]?.list = TabRename.renamed(list, host: t.host, tab: t.tabID, to: t.name) }
            if error.isOffline || error.isQuiet {
                model.show(error)
            } else {
                let computer = model.machineChoices.first { $0.id == path }?.name ?? t.machine.id
                model.toast = Toast(TabRename.failure(error, computer: computer), style: .error)
                Haptics.error()
            }
            return false
        }
    }

    /// The page's every-few-seconds read: an answer that differs from what is shown replaces it; a failure, an unchanged
    /// answer, or one overtaken by a newer request changes nothing (and so redraws nothing). A computer whose list isn't
    /// in yet, or with a rename on its way, is left to its own request.
    private func refreshQuietly() async {
        var jobs: [TerminalsFetch.Job] = []
        for m in machines where !renamingOn.contains(m.id) {
            let ref = MachineRef(path: m.id)
            guard let load = loads[m.id], load.list != nil, load.failure == nil, let request = load.request,
                  let api = model.api(ref.group) else { continue }
            jobs.append(TerminalsFetch.Job(path: m.id, request: request) { try await api.terminalPanes(machine: ref.id) })
        }
        // Each computer's answer as it arrives, as `load` takes them (#7115: a task group read with `for await` never
        // answered on a Release build).
        await TerminalsFetch.run(jobs) { job, outcome in
            guard case .answered(let list) = outcome, var load = loads[job.path], !renamingOn.contains(job.path),
                  load.refreshed(list, during: job.request) else { return }
            loads[job.path] = load
        }
    }

    /// A new request for each computer (`only`: those), under its own short id (`X-Client-Request-Id`, which the hub's
    /// log keeps), so a report of a stuck page can be matched to the hub's log.
    private func load(_ only: Set<String>? = nil) async {
        var jobs: [TerminalsFetch.Job] = []
        for m in machines where only?.contains(m.id) ?? true {
            let ref = MachineRef(path: m.id)
            var api = model.api(ref.group)
            #if DEBUG
            if UserDefaults.standard.bool(forKey: "showTerminalsUnlinked") { api = nil } // screenshots of the state below
            #endif
            guard let api else {
                loads[m.id] = TerminalsLoad(failure: String(localized: "Not connected to this computer's hub yet"))
                continue
            }
            let id = TerminalsLoad.newRequest()
            loads[m.id, default: TerminalsLoad()].start(id)
            jobs.append(TerminalsFetch.Job(path: m.id, request: id) { try await api.terminalPanes(machine: ref.id, requestID: id) })
        }
        await TerminalsFetch.run(jobs) { job, outcome in
            switch outcome {
            case .answered(let list): loads[job.path]?.answer(job.request, list)
            case .failed(let error): loads[job.path]?.fail(job.request, TerminalsLoad.withRequest(error.shown ?? error.localizedDescription, job.request))
            case .left: break // no answer runs late, with Retry
            }
        }
    }

    /// The hub checks the node can open a workspace for New Terminal (`pane_new_space`, #6362); an unknown list offers it.
    private func hubHasNewSpace(_ ref: MachineRef) -> Bool {
        let f = model.features(ref.group)
        return f.isEmpty || f.contains("pane_new_space")
    }

    private func newTerminal(on ref: MachineRef, space: TerminalsList.Space) {
        guard let api = model.api(ref.group) else { return }
        Task {
            do {
                // no workspace yet: the node opens one (new_space: the hub answers node_outdated for a node that can't)
                var body: [String: JSONValue] = space.id.isEmpty ? ["cwd": .string("~"), "new_space": .bool(true)] : ["workspace_id": .string(space.id)]
                if !space.kind.isEmpty { body["kind"] = .string(space.kind) }
                let data = try await api.nodeAction(machine: ref.id, "panes", body)
                await load()
                if let made = try? JSONDecoder().decode(NewShell.self, from: data), !made.terminal.isEmpty {
                    let list = loads[ref.path]?.list
                    let row = list.flatMap { l in TerminalsList.rows(l).first { $0.pane.terminal == made.terminal } }
                    let tab = list.flatMap { l in row.flatMap { TabRename.target($0, in: l, on: ref) } }
                    open = TerminalTarget(key: SessionKey(group: ref.group, machineID: ref.id, sessionID: made.terminal), rename: tab)
                }
            } catch {
                NodeActions.shared.fail(String(localized: "Couldn't open a terminal"), error)
            }
        }
    }

    private func close(_ c: CloseTarget) {
        guard let api = model.api(c.machine.group) else { return }
        Task {
            do {
                try await api.nodeAction(machine: c.machine.id, "panes/close", ["pane": c.pane.pane])
                model.toast = Toast(String(localized: "Terminal closed"), style: .success)
                await load()
            } catch {
                NodeActions.shared.fail(String(localized: "Couldn't close the terminal"), error)
            }
        }
    }

    private func askCloseFinished(_ ref: MachineRef, name: String) {
        guard let api = model.api(ref.group) else { return }
        Task {
            do {
                let data = try await api.nodeAction(machine: ref.id, "panes/close_finished", ["preview": true], timeout: 70)
                let got = try JSONDecoder().decode(FinishedAnswer.self, from: data)
                if got.closed.isEmpty {
                    model.toast = Toast(String(localized: "No finished terminals to close"), style: .info)
                } else {
                    finished = FinishedTarget(machine: ref, name: name, panes: got.closed)
                }
            } catch {
                NodeActions.shared.fail(String(localized: "Couldn't list finished terminals"), error)
            }
        }
    }

    private func closeFinished(_ f: FinishedTarget) {
        guard let api = model.api(f.machine.group) else { return }
        Task {
            do {
                let data = try await api.nodeAction(machine: f.machine.id, "panes/close_finished",
                                                    ["panes": f.panes.map(\.pane).joined(separator: ",")], timeout: 70)
                let got = try JSONDecoder().decode(FinishedAnswer.self, from: data)
                model.toast = Toast(String(localized: "Closed \(got.closed.count) terminals"), style: .success)
                await load()
            } catch {
                NodeActions.shared.fail(String(localized: "Couldn't close the terminals"), error)
            }
        }
    }
}

/// What `panes/close_finished` answers (`docs/protocol/protocol.md`).
nonisolated struct FinishedAnswer: Decodable, Sendable {
    var closed: [FinishedTerminal]
    enum CodingKeys: String, CodingKey { case closed }
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        closed = try c.decodeIfPresent([FinishedTerminal].self, forKey: .closed) ?? []
    }
}

nonisolated struct FinishedTerminal: Decodable, Sendable, Equatable {
    var pane: String
    var sessionID: String
    enum CodingKeys: String, CodingKey { case pane, sessionID = "session_id" }
}

/// What `POST /machines/{mid}/panes` opened.
nonisolated struct NewShell: Decodable, Sendable {
    var terminal: String
}

/// The rows of the Terminals page from a computer's panes: pure, so it is tested without a hub.
nonisolated enum TerminalsList {
    enum Kind: Equatable { case agent, finished, plain }

    struct Row: Identifiable, Equatable {
        var pane: TerminalPanes.Pane
        var kind: Kind
        /// The terminal engine it runs in, as the hub names it ("herdr", "tern"…; "" from a node that does not say).
        var host: String = ""
        var id: String { pane.pane }
    }

    struct Space: Equatable {
        var id: String
        var label: String
        var kind: String
    }

    /// Every pane of every host, Agents' first within a tab, in the hosts' order of workspaces and tabs.
    static func rows(_ list: TerminalPanes) -> [Row] {
        list.hosts.flatMap { h in
            h.panes.map { p in
                let kind: Kind = !(p.agent ?? "").isEmpty || !(p.sessionID ?? "").isEmpty ? .agent
                    : !(p.agentSession ?? "").isEmpty ? .finished : .plain
                return Row(pane: p, kind: kind, host: h.kind)
            }
        }
    }

    /// The workspaces a new terminal can open in: those of hosts that open tabs. A host that opens tabs but has no
    /// workspace yet offers its home folder (id "") when the hub has `pane_new_space` (`newSpace`): the node opens a new
    /// workspace there.
    static func spaces(_ list: TerminalPanes, newSpace: Bool) -> [Space] {
        list.hosts.filter(\.newTab).flatMap { h in
            h.workspaces.isEmpty ? (newSpace ? [Space(id: "", label: String(localized: "Home Folder"), kind: h.kind)] : [])
                : h.workspaces.map { Space(id: $0.id, label: $0.label.isEmpty ? $0.id : $0.label, kind: h.kind) }
        }
    }

    static func hasFinished(_ list: TerminalPanes) -> Bool {
        list.hosts.contains { $0.panes.contains { !($0.agentSession ?? "").isEmpty } }
    }

    /// An Agent's name as the app shows it (Session.agentLabel) from a pane's runtime id, when no session row says.
    static func agentName(_ raw: String?) -> String {
        switch (raw ?? "").lowercased() {
        case "", "claude": "Claude"
        case "codex": "Codex"
        case "traex": "TRAEX"
        default: raw ?? ""
        }
    }

    /// What follows "Terminal" under a plain pane: the program in front unless its title already names it, then the
    /// folder.
    static func plainDetails(title: String, foreground: String?, folder: String) -> [String] {
        var out: [String] = []
        if let fg = foreground, !fg.isEmpty, !title.localizedCaseInsensitiveContains(fg) { out.append(fg) }
        if !folder.isEmpty { out.append(folder) }
        return out
    }

    /// A pane's folder by its last name ("" when not known): the computer's paths mean little on the phone.
    static func folder(_ cwd: String?) -> String {
        guard let cwd, !cwd.isEmpty else { return "" }
        return (cwd as NSString).lastPathComponent
    }
}

/// One request per computer, each answer applied as it arrives (#7115). Not a task group read with `for await`: on the
/// phone's Release build that loop ended without a single answer, and the page sat on its spinners until it said "hasn't
/// answered yet". Each computer's task applies its own answer; a cancelled page cancels them all.
nonisolated enum TerminalsFetch {
    struct Job: Sendable {
        var path: String
        var request: String
        var panes: @Sendable () async throws -> TerminalPanes
    }

    enum Outcome {
        case answered(TerminalPanes)
        case failed(Error)
        /// Cancelled: no answer, the computer runs late with Retry.
        case left
    }

    static func outcome(_ job: Job) async -> Outcome {
        do { return .answered(try await job.panes()) } catch {
            return error.isCancellation ? .left : .failed(error)
        }
    }

    /// Returns when every computer has been answered, failed or left.
    @MainActor static func run(_ jobs: [Job], apply: @escaping @MainActor (Job, Outcome) -> Void) async {
        let tasks = jobs.map { job in Task { @MainActor in apply(job, await outcome(job)) } }
        await withTaskCancellationHandler {
            for t in tasks { await t.value }
        } onCancel: {
            for t in tasks { t.cancel() }
        }
    }
}

/// One computer's answer on the Terminals page and the request on its way (#6153): pure, so it is tested without a
/// hub or a clock. A computer always ends up listed, failed or late (with Retry), never on a bare spinner.
nonisolated struct TerminalsLoad {
    var list: TerminalPanes?
    var failure: String?
    /// The latest request's short id; an answer to an older one is ignored, the newer one answers or runs late.
    var request: String?
    /// No answer to `request` within `patience`.
    var late = false

    /// How long a computer's spinner turns before the page says it hasn't answered (the request itself waits 30 s).
    static let patience: Duration = .seconds(15)
    /// How often the page in front reads the lists again (the web's Terminal page: `REFRESH_MS`).
    static let refreshEvery: Duration = .seconds(8)

    enum Shown: Equatable { case waiting, rows(TerminalPanes), failed(String), late(String?) }

    var shown: Shown {
        if let failure { return .failed(failure) }
        if let list { return .rows(list) }
        return late ? .late(request) : .waiting
    }

    /// A new request: the old failure and lateness go; the list stays until the answer.
    mutating func start(_ id: String) {
        request = id
        failure = nil
        late = false
    }

    mutating func answer(_ id: String, _ list: TerminalPanes) {
        guard id == request else { return }
        self.list = list
        failure = nil
        late = false
    }

    mutating func fail(_ id: String, _ message: String) {
        guard id == request else { return }
        failure = message
    }

    /// The every-few-seconds read's answer, sent while `request` was the latest: taken when no newer request started, the
    /// list isn't failed and the answer differs. False: nothing changed (the page writes nothing, so redraws nothing).
    mutating func refreshed(_ list: TerminalPanes, during request: String?) -> Bool {
        guard request == self.request, failure == nil, self.list != nil, self.list != list else { return false }
        self.list = list
        return true
    }

    /// `patience` ran out for `id` (nil: the spinner showed with no request of this page's on its way).
    mutating func expire(_ id: String?) {
        guard id == request, list == nil, failure == nil else { return }
        late = true
    }

    /// An id as `APIClient` makes one: 8 lowercase hex characters.
    static func newRequest() -> String { String(UUID().uuidString.prefix(8)).lowercased() }

    /// `text` naming request `id`, unless it does already (a server error says its request).
    static func withRequest(_ text: String, _ id: String?) -> String {
        guard let id, !text.contains(id) else { return text }
        return String(localized: "\(text) (request \(id))")
    }
}

/// Renaming a terminal's tab (#6996), as the web's node menu does (`tabs/rename`, `web/src/lib/node-actions.ts`): which
/// rows offer it, the name the computer takes, and what a refusal says. Pure, so it is tested without a hub.
nonisolated enum TabRename {
    /// What the computer takes: 1 to 80 characters once each run of white space is one space (`node/panes_actions.go`).
    static let maxLength = 80

    /// A tab to rename: a plain terminal's (its row, its terminal page).
    nonisolated struct Target: Identifiable, Equatable, Sendable {
        var machine: MachineRef
        var tabID: String
        /// The engine's kind ("" from a node that does not say: herdr), sent as `kind`.
        var host: String
        /// The tab's name now: what the prompt starts from.
        var name: String
        var id: String { machine.path + "|" + host + "|" + tabID }
    }

    /// The engines that rename tabs: herdr, Tern and Rex (the web's `canRenameIn`); Orca and the others are not asked.
    static func canRename(host: String) -> Bool { host.isEmpty || host == "herdr" || host == "tern" || host == "rex" }

    /// A plain terminal's tab on an engine that renames; nil for an Agent's row (its own Rename Agent…) and a stopped
    /// Agent's leftover (titled by that Agent, not its tab).
    static func target(_ r: TerminalsList.Row, in list: TerminalPanes, on machine: MachineRef) -> Target? {
        guard r.kind == .plain, canRename(host: r.host), !r.pane.tabID.isEmpty else { return nil }
        let label = list.hosts.first { $0.kind == r.host }?.tabs.first { $0.id == r.pane.tabID }?.label ?? ""
        return Target(machine: machine, tabID: r.pane.tabID, host: r.host, name: label)
    }

    /// The name as the computer keeps it: each run of white space one space, none at the ends.
    static func cleaned(_ name: String) -> String { name.split(whereSeparator: \.isWhitespace).joined(separator: " ") }

    /// A name the computer takes that isn't the one it has: anything else saves nothing.
    static func canSave(_ name: String, current: String) -> Bool {
        let c = cleaned(name)
        return !c.isEmpty && c.count <= maxLength && c != cleaned(current)
    }

    static func body(_ t: Target, name: String) -> [String: String] {
        var body = ["tab_id": t.tabID, "label": cleaned(name)]
        if !t.host.isEmpty { body["kind"] = t.host }
        return body
    }

    /// The list with the tab named `name`: shown before the computer answers, and the old name put back the same way.
    static func renamed(_ list: TerminalPanes, host: String, tab: String, to name: String) -> TerminalPanes {
        var list = list
        for h in list.hosts.indices where list.hosts[h].kind == host {
            for i in list.hosts[h].tabs.indices where list.hosts[h].tabs[i].id == tab { list.hosts[h].tabs[i].label = name }
        }
        return list
    }

    /// Why the name didn't change, in one plain sentence: the computer is away, its terminal engine can't rename (an
    /// engine this app thinks can, on an older or different computer), the terminal closed meanwhile; else the hub's words.
    static func failure(_ error: Error, computer: String) -> String {
        if let e = error as? APIError {
            if e.code == "node_unreachable" { return String(localized: "Couldn't rename: “\(computer)” is offline or asleep.") }
            // the node's words (`node/hosts.go` unsupported, `node/panes_actions.go` listedTab), relayed as 502 internal
            let said = e.message.lowercased()
            if said.hasPrefix("unsupported") {
                return String(localized: "Couldn't rename: the terminal app on “\(computer)” doesn't support it.")
            }
            if said.contains("is not running") || said == "no tab" {
                return String(localized: "Couldn't rename: this terminal has already closed.")
            }
        }
        return String(localized: "Couldn't rename: \(error.shown ?? error.localizedDescription)")
    }
}

/// Rename Terminal… (#6996): one field filled with the tab's name, taking up to `TabRename.maxLength` characters. Save is
/// never disabled: the alert draws a disabled button as an enabled one (`docs/lessons/ios.md`), so as Rename Space does,
/// an empty or unchanged name saves nothing.
private struct RenameTerminalAlert: ViewModifier {
    @Binding var target: TabRename.Target?
    var save: (TabRename.Target, String) -> Void
    @State private var name = ""

    func body(content: Content) -> some View {
        content
            .alert("Rename Terminal", isPresented: Binding(get: { target != nil }, set: { if !$0 { target = nil } }), presenting: target) { t in
                // As New Folder's name in Files: a terminal's name is taken as typed, never corrected or capitalized.
                TextField(String(localized: "Terminal name"), text: $name)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("terminals.renameField")
                Button("Cancel", role: .cancel) {}
                Button("Save") { if TabRename.canSave(name, current: t.name) { save(t, name) } }
            } message: { _ in
                Text("The name changes on the computer too.")
            }
            .onChange(of: target) { _, t in if let t { name = t.name } }
            .onChange(of: name) { _, typed in
                if typed.count > TabRename.maxLength { name = String(typed.prefix(TabRename.maxLength)) }
            }
    }
}

extension View {
    /// The Rename Terminal prompt for a terminal's tab (the Terminals page, a plain terminal's page).
    func renameTerminalAlert(_ target: Binding<TabRename.Target?>, save: @escaping (TabRename.Target, String) -> Void) -> some View {
        modifier(RenameTerminalAlert(target: target, save: save))
    }
}

private extension String {
    var nilIfEmpty: String? { isEmpty ? nil : self }
}
