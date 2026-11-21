import SwiftUI

// Machines › Add Computer (docs/assistant/onboarding.md 4.2「你的电脑 ↔ hub」, 5.3; docs/protocol/protocol.md「Computer pairing codes」「电脑的信任」, #4196): a
// signed-in device adds a computer with POST /api/v1/auth/pair-codes {"add"}, hands the pairing link to that computer's
// Persona Link (a line for a Linux terminal, a link to share for a Mac, a line for Windows' PowerShell), counts the code
// down (a new one with {"machine"} once it expires) and waits for the computer to come online. It then asks, on the
// same screen, whether it is the user's: It's Mine (POST /machines/{id}/confirm) or Not Mine (…/reject). The same sheet
// confirms a computer that was added elsewhere (`AddComputerRequest.confirm`). After the web's (#3934) and Android's.

/// The systems there are instructions for; the hub takes the same words (`add.os`).
enum ComputerOS: String, CaseIterable, Identifiable, Sendable {
    case mac, linux, windows
    var id: String { rawValue }
    var title: String {
        switch self {
        case .mac: "Mac"
        case .linux: "Linux"
        case .windows: "Windows"
        }
    }
    /// The hub's `add.icon`.
    var icon: String { self == .linux ? "server" : self == .windows ? "desktop" : "laptop" }
    var symbol: String { self == .linux ? Theme.Icon.server : self == .windows ? Theme.Icon.machine : Theme.Icon.laptop }

    /// What a computer's node reported (`os`: darwin, linux, windows…); nil before it connected.
    init?(reported: String?) {
        let os = (reported ?? "").lowercased()
        if os.hasPrefix("linux") { self = .linux }
        else if os.hasPrefix("windows") { self = .windows }
        else if os.hasPrefix("darwin") || os.hasPrefix("mac") { self = .mac }
        else { return nil }
    }
}

/// What Add Computer opens on: the form, or one computer waiting to be confirmed (the readiness card's `confirm_computer`,
/// a pending machine's card).
enum AddComputerRequest: Identifiable, Hashable {
    case add
    case confirm(MachineRef)
    var id: String {
        switch self {
        case .add: "add"
        case .confirm(let ref): "confirm|" + ref.path
        }
    }
}

extension AppModel {
    /// The hub lets this device add a computer (`machines_add`); an older hub keeps the old pages.
    var canAddComputer: Bool { hubFeatures.contains(APIClient.machinesAddFeature) }
    /// The group's hub takes It's Mine / Not Mine (`machine_trust`).
    func canConfirmComputer(_ group: String) -> Bool { features(group).contains(APIClient.machineTrustFeature) }
    /// The current assistant of `group`'s hub, for a computer added there: only the hub-level group's (the one whose assistants
    /// the switcher lists) has one; nil for another, whose request names none.
    func addingAssistant(in group: String) -> AddAssistant? {
        AddComputerText.assistant(in: group, hubGroup: hub?.group, roster: managerRosters[group], pick: assistantPicks[group],
                                  settled: managerRosterUnsupported.contains(group))
    }
    /// Add Computer over the whole app (nothing on a hub without `machines_add`).
    func openAddComputer() { if canAddComputer { addComputer = .add } }
    /// Confirm Computer for one computer; nil: the one the hub most likely means (`pendingToConfirm`), else the form.
    func openConfirmComputer(_ ref: MachineRef? = nil) {
        if let ref = ref ?? pendingToConfirm()?.ref { addComputer = .confirm(ref) } else { openAddComputer() }
    }
    /// The computer Confirm Computer is about: `preferred` while it waits, else the first one waiting that is online (it can be
    /// asked about now), else the first one waiting.
    func pendingToConfirm(preferred: String? = nil) -> Machine? { AddComputerText.pendingToConfirm(machines, preferred: preferred) }
}

/// Why the hub did not do it, in words, and what to offer.
struct AddComputerFailure: Equatable {
    var message: String
    var code: String?
}

/// What the app knows of the assistant of the hub a computer is added to: that hub's roster (nil from a hub without one, or until
/// it is read) and the pick this device kept for it (nil: the default).
nonisolated struct AddAssistant: Equatable, Sendable {
    var roster: ManagerRoster?
    var pick: String?
    /// The hub has answered, in this run, that it has no roster (`AppModel.managerRosterUnsupported`); a roster it has is `roster`.
    var settled = true

    /// Nothing says which assistant is current yet: no roster, no pick, and the hub hasn't answered. A hub without a
    /// roster is settled by its 404, so it is never kept waiting.
    var unknown: Bool { roster == nil && (pick ?? "").isEmpty && !settled }
}

/// The words, kept apart from the view to test.
enum AddComputerText {
    static let maxName = 40
    /// How to build and install Persona Link on a Mac until there is a download (docs/assistant/onboarding.md 6.3, decision 1).
    static let macInstallDoc = URL(string: "https://github.com/nwverse/persona/blob/main/docs/operations/persona-link-install.md")!
    /// How to install it on a Linux without the hub's install script (docs/operations/link.md「Linux（headless）」).
    static let linuxInstallDoc = URL(string: "https://github.com/nwverse/persona/blob/main/docs/operations/link.md#linux-headless")!

    /// The name as sent: trimmed; nil when empty or too long.
    /// `AppModel.pendingToConfirm`: `preferred` while it waits, else the first one waiting that is online, else the first.
    static func pendingToConfirm(_ machines: [Machine], preferred: String? = nil) -> Machine? {
        let waiting = machines.filter(\.isPending)
        return waiting.first { preferred != nil && $0.id == preferred } ?? waiting.first(where: \.online) ?? waiting.first
    }

    static func validName(_ name: String) -> String? {
        let n = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return n.isEmpty || n.count > maxName ? nil : n
    }

    /// Why the hub refused, by its code (docs/protocol/protocol.md「Computer pairing codes」); the hub's own words for any other code.
    static func failure(_ error: Error) -> AddComputerFailure {
        let e = error as? APIError
        let code = e?.code
        let theirs = e?.message.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let message: String
        switch code {
        case "forbidden": message = String(localized: "This device can't add computers. Try again from Persona on a phone or computer that is already signed in.")
        // The hub's words, in plain language and in the user's language (the acceptance of #4196).
        case "tls_required": message = theirs.isEmpty ? String(localized: "The hub hasn't turned on secure connections yet, so computers can't join. Turn them on in the hub's admin page, then try again.") : theirs
        case "mesh_member": message = String(localized: "This hub is a member of a group. Add computers on the group's central hub.")
        case "too_many_pending": message = String(localized: "3 computers you added are still waiting (some may never have connected). Remove the ones you don't need on the Machines page, then add again.")
        case "not_pending": message = String(localized: "This computer is already confirmed. To pair it again, use the hub's admin page.")
        case "not_found": message = String(localized: "This computer is no longer there.")
        case "invalid_name": message = String(localized: "Give it a name of up to 40 characters.")
        case "invalid_os": message = String(localized: "Pick a system: Mac, Linux or Windows.")
        // add.manager's own refusals: the hub says which assistant and why, in the request's language. The generic words for the
        // status (or for assistant_offline) would hide that.
        case "no_manager", "installing", "assistant_offline", "owners_unknown":
            message = theirs.isEmpty ? (error.shown ?? error.localizedDescription) : theirs
        default: message = error.shown ?? error.localizedDescription
        }
        return AddComputerFailure(message: message, code: code)
    }

    /// A machine's `error_code` in words (docs/assistant/onboarding.md 5.3); nil for a code this app does not know (show `error`).
    static func machineProblem(_ code: String?) -> String? {
        switch code {
        case "node_service_down": String(localized: "Persona's background service isn't running on this computer. Open Persona Link to install it again.")
        case "link_offline": String(localized: "This computer is offline: Persona Link isn't running, or the computer is asleep.")
        case "never_connected": String(localized: "Never connected: pair it once on that computer with the pairing link.")
        case "node_outdated": String(localized: "Persona on this computer is too old to update from here. Install it again on that computer.")
        default: nil
        }
    }

    /// What the machine's line says under its name: the known code in words, else the hub's own `error`.
    /// The card's lines for a computer that is not online: a known error code in words, a pending one's request, the hub's
    /// error read in plain words (why, then what to do) with the raw error under it, else the raw error itself.
    static func machineLines(_ m: Machine) -> (line: String, detail: String?)? {
        if m.linkSwitchRunning { return nil } // the switch's own card explains this expected disconnect (#10671)
        if let p = machineProblem(m.errorCode) { return (p, nil) }
        if m.isPending { return m.online ? (String(localized: "A computer wants to join. Confirm whether it's yours."), nil) : nil }
        if let reason = NodeHealth.offlineReason(m) { return (reason, m.error) }
        return m.error.flatMap { $0.isEmpty ? nil : ($0, nil) }
    }

    /// The system the joining computer says it runs, for 「…（macOS）」.
    static func systemName(_ m: Machine) -> String? {
        if let s = m.system?.name, !s.isEmpty { return s }
        switch ComputerOS(reported: m.os) {
        case .mac: return "macOS"
        case .linux: return "Linux"
        case .windows: return "Windows"
        case nil: return nil
        }
    }

    /// The line that pairs an installed Persona Link; the hub's own when it gave one.
    static func pairCommand(_ code: APIClient.ComputerCode) -> String {
        code.command.flatMap { $0.isEmpty ? nil : $0 } ?? "persona-link pair " + quoted(code.pairLink)
    }

    /// What a Linux computer runs: the line that installs and pairs, or only the pairing when the hub has no site.
    static func linuxCommand(_ code: APIClient.ComputerCode) -> (line: String, installs: Bool) {
        if let line = code.install?.linux, !line.isEmpty { return (line, true) }
        return (pairCommand(code), false)
    }

    /// What Windows runs in PowerShell, in the folder with the two programs (docs/operations/windows.md).
    static func windowsCommand(_ code: APIClient.ComputerCode) -> String {
        #".\persona-link.exe install --personad .\personad.exe --pair "# + quoted(code.pairLink)
    }

    /// A pairing link in single quotes for sh and PowerShell; it has none of its own (it is URL-encoded).
    private static func quoted(_ s: String) -> String { "'" + s.replacingOccurrences(of: "'", with: "%27") + "'" }

    /// The code's end by this device's clock: the hub's, unless it is already past or more than ten minutes away
    /// (a clock that is off), then ten minutes from now.
    static func expiry(_ code: APIClient.ComputerCode, now: Date) -> Date {
        let left = code.expiresAt.timeIntervalSince(now)
        return left > 0 && left <= 600 ? code.expiresAt : now.addingTimeInterval(600)
    }

    /// The assistant a new computer is added to, as `AddComputerModel` asks for it: the roster's id for the current one (the default's
    /// real id too, also with one assistant), else, before a roster is read or from a hub without one, the pick this device kept
    /// for that hub; nil when neither names one, and the request is as it was before `add.manager`.
    static func manager(for assistant: AddAssistant?) -> String? {
        guard let assistant else { return nil }
        if let roster = assistant.roster { return roster.isMulti ? roster.owner(assistant.pick) : roster.defaultID }
        return assistant.pick.flatMap { $0.isEmpty ? nil : $0 }
    }

    /// What is known of the current assistant for a computer added to `group`: only the hub-level group's hub (`hubGroup`) has one;
    /// any other group's request names none.
    static func assistant(in group: String, hubGroup: String?, roster: ManagerRoster?, pick: String?, settled: Bool) -> AddAssistant? {
        guard let hubGroup, hubGroup == group else { return nil }
        return AddAssistant(roster: roster, pick: pick, settled: settled)
    }

    /// The current assistant when a computer can't be added to it: it is offline. Only with several assistants (one is never told
    /// apart), and never when the roster is not known (an unknown one is not an offline one).
    static func offline(_ assistant: AddAssistant?) -> HubManager? {
        guard let roster = assistant?.roster, roster.isMulti else { return nil }
        let id = roster.owner(assistant?.pick)
        return roster.installed.first { $0.id == id && !$0.online }
    }

    /// The name of the assistant the new computer is under, when that is not the one it was asked for (docs/protocol/protocol.md
    /// "Computer pairing codes"): the reply's `manager`; a hub from before the field leaves it out and puts a computer it added
    /// under its own assistant, while one it reused stays where it was (`listed`: the manager the machines list has it under).
    /// nil when it is where it was asked for, when none was asked for, and with one assistant.
    static func addedUnder(asked: String?, code: APIClient.ComputerCode, listed: String?, roster: ManagerRoster?) -> String? {
        guard let asked, !asked.isEmpty, let roster, roster.isMulti else { return nil }
        let reported = code.manager.flatMap { $0.isEmpty ? nil : $0 }
        let listedManager = listed.flatMap { $0.isEmpty ? nil : $0 }
        let owner = reported ?? ((code.added ?? false) ? ManagerRoster.assistant : (listedManager ?? ManagerRoster.assistant))
        guard owner != asked else { return nil }
        return roster.managers.first { $0.id == owner }?.name ?? owner
    }

    static func namePlaceholder(_ os: ComputerOS) -> String {
        switch os {
        case .mac: String(localized: "e.g. My MacBook")
        case .linux: String(localized: "e.g. Home server")
        case .windows: String(localized: "e.g. Office desktop")
        }
    }
}

/// Add Computer / Confirm Computer: the form, the code and its countdown, the computer joining, confirmed or rejected.
@Observable final class AddComputerModel {
    struct Deps {
        /// The client of a group's hub.
        var api: (String) -> APIClient?
        var now: () -> Date = { .now }
        /// The group's hub takes confirm / reject.
        var canConfirm: (String) -> Bool = { _ in true }
        /// After confirm / reject: the app's lists again.
        var refreshed: () async -> Void = {}
        /// The system a computer was added as, by machine id (the hub does not keep it).
        var rememberOS: (String, ComputerOS) -> Void = { _, _ in }
        var rememberedOS: (String) -> ComputerOS? = { _ in nil }
        /// The current assistant of a group's hub; nil for any other group's (a computer added there names no assistant here).
        var assistant: (String) -> AddAssistant? = { _ in nil }
        /// The computers the app lists, whichever assistant's: where a reused computer is.
        var machines: () -> [Machine] = { [] }

        static var live: Deps {
            .init(api: { AppModel.shared.api($0) }, canConfirm: { AppModel.shared.canConfirmComputer($0) },
                  refreshed: { await AppModel.shared.refresh() },
                  rememberOS: { id, os in UserDefaults.standard.set(os.rawValue, forKey: "addComputer.os." + id) },
                  rememberedOS: { id in UserDefaults.standard.string(forKey: "addComputer.os." + id).flatMap(ComputerOS.init(rawValue:)) },
                  assistant: { AppModel.shared.addingAssistant(in: $0) }, machines: { AppModel.shared.machines })
        }
    }

    enum Stage: Equatable {
        /// Pick the system, give a name.
        case form
        /// The code is being made.
        case working
        /// The code is out, counting down (expired at 0), waiting for the computer.
        case waiting
        /// It is online and waits: Is it yours?
        case joined
        /// Confirm Computer for one that is not online (no code made here).
        case offline
        case confirmed
        case rejected
        /// Removed elsewhere, or it waited too long.
        case gone
    }

    var os: ComputerOS = .mac
    var name = ""
    private(set) var stage: Stage = .form
    private(set) var code: APIClient.ComputerCode?
    /// The computer as the hub lists it now.
    private(set) var machine: Machine?
    /// The hub's id of the computer being added or confirmed.
    private(set) var machineID: String?
    private(set) var failure: AddComputerFailure?
    /// Confirming or rejecting.
    private(set) var busy = false
    private(set) var now: Date
    private(set) var expiresAt: Date?
    let group: String
    /// The assistant the new computer was asked for (`add.manager`) and the roster then, to name the one the hub put it under;
    /// nil when none was named. A new code for the same computer keeps them.
    private(set) var asked: String?
    @ObservationIgnored private var askedRoster: ManagerRoster?
    @ObservationIgnored private let deps: Deps
    /// The list has had it since the code was made: missing later means it is gone.
    @ObservationIgnored private var seen = false

    /// `request`: .add opens on the form, .confirm(ref) on that computer; `group` is where .add adds it.
    init(_ request: AddComputerRequest = .add, group: String = "", deps: Deps = .live) {
        self.deps = deps
        now = deps.now()
        switch request {
        case .add:
            self.group = group
        case .confirm(let ref):
            self.group = ref.group
            machineID = ref.id
            stage = .offline
            if let os = deps.rememberedOS(ref.id) { self.os = os }
        }
    }

    var validName: String? { AddComputerText.validName(name) }
    var secondsLeft: Int { expiresAt.map { ConnectDeviceText.secondsLeft($0, now: now) } ?? 0 }
    var expired: Bool { stage == .waiting && secondsLeft == 0 }
    /// What the screens call it.
    var computerName: String { machine?.name ?? code?.name ?? validName ?? machineID ?? "" }
    /// A new code is offered for one waiting to be confirmed that never connected (a new code voids the pairing a
    /// sleeping computer has).
    var canReissue: Bool { machine.map { $0.isPending && !$0.online && $0.neverConnected } ?? false }

    /// The current assistant while it is offline: nothing is added to it now. Only on the form.
    var assistantOffline: HubManager? { stage == .form ? AddComputerText.offline(deps.assistant(group)) : nil }

    /// The assistant the computer was added under when that is not the one asked for, by name: said while it waits.
    var addedUnder: String? {
        guard let code else { return nil }
        let listed = machine?.manager ?? deps.machines().first { $0.ref == MachineRef(group: group, id: code.machine) }?.manager
        return AddComputerText.addedUnder(asked: asked, code: code, listed: listed, roster: deps.assistant(group)?.roster ?? askedRoster)
    }

    /// How long Next waits for a roster that isn't known yet (a few seconds in all).
    static let rosterWait: [Duration] = Array(repeating: .milliseconds(250), count: 20)

    /// Next: adds the computer and shows its code.
    func submit() async {
        if assistantOffline != nil { return }
        guard let n = validName else {
            failure = AddComputerFailure(message: String(localized: "Give it a name of up to 40 characters."), code: "invalid_name")
            return
        }
        guard let api = deps.api(group) else { failure = Self.disconnected; return }
        stage = .working
        failure = nil
        // After a launch with no kept roster the first answer may be moments away: a computer added before it would go to
        // the hub's own assistant, whichever is the default. A hub that never answers gets the plain request.
        var waited = 0
        while deps.assistant(group)?.unknown == true, waited < Self.rosterWait.count {
            try? await Task.sleep(for: Self.rosterWait[waited])
            waited += 1
        }
        let assistant = deps.assistant(group)
        if AddComputerText.offline(assistant) != nil { stage = .form; return }
        let manager = AddComputerText.manager(for: assistant)
        do {
            let c = try await api.computerCode(addNamed: n, os: os.rawValue, icon: os.icon, manager: manager)
            deps.rememberOS(c.machine, os)
            asked = manager
            askedRoster = assistant?.roster
            show(c)
        } catch {
            stage = .form
            failure = AddComputerText.failure(error)
        }
    }

    /// Regenerate / New Link: a new code for the same computer ({"machine": id}).
    func reissue() async {
        guard let id = machineID else { return }
        guard let api = deps.api(group) else { failure = Self.disconnected; return }
        let before = stage
        stage = .working
        failure = nil
        do {
            show(try await api.computerCode(machine: id))
        } catch {
            let f = AddComputerText.failure(error)
            switch f.code {
            case "not_found": stage = .gone
            case "not_pending": stage = .confirmed
            default: stage = before; failure = f
            }
        }
    }

    private func show(_ c: APIClient.ComputerCode) {
        now = deps.now()
        code = c
        machineID = c.machine
        expiresAt = AddComputerText.expiry(c, now: now)
        seen = false
        if c.trust == "confirmed" { stage = .confirmed } else { stage = .waiting }
    }

    func tick() { now = deps.now() }

    /// Reads the machine list once and moves on when the computer joined, was confirmed or went.
    func poll() async {
        guard let id = machineID, let api = deps.api(group) else { return }
        guard let list = try? await api.machines() else { return }
        update(list.first { $0.id == id })
    }

    /// Where it is, from what the list says of it now (as the web's addStage / confirmStage).
    func update(_ m: Machine?) {
        guard [.waiting, .joined, .offline].contains(stage) else { return }
        if let m { machine = m; seen = true }
        switch stage {
        case .waiting:
            guard let m else { if seen { stage = .gone }; return }
            if m.online { stage = m.isPending && deps.canConfirm(group) ? .joined : .confirmed }
        case .joined, .offline:
            guard let m else { stage = .gone; return }
            if !m.isPending { stage = .confirmed }
            else { stage = m.online ? .joined : .offline }
        default: break
        }
    }

    /// It's Mine.
    func confirm() async {
        guard let id = machineID, let api = deps.api(group) else { return }
        busy = true
        defer { busy = false }
        failure = nil
        do {
            try await api.confirmMachine(id)
            stage = .confirmed
            await deps.refreshed()
        } catch {
            let f = AddComputerText.failure(error)
            if f.code == "not_found" { stage = .gone } else { failure = f }
        }
    }

    /// Not Mine / Remove: the hub removes it.
    func reject() async {
        guard let id = machineID, let api = deps.api(group) else { return }
        busy = true
        defer { busy = false }
        failure = nil
        do {
            try await api.rejectMachine(id)
            stage = .rejected
            await deps.refreshed()
        } catch {
            let f = AddComputerText.failure(error)
            switch f.code {
            case "not_found": stage = .gone
            case "not_pending": stage = .confirmed
            default: failure = f
            }
        }
    }

    /// The countdown each second and the machine list every three, while there is something to wait for.
    func run() async {
        var n = 0
        if stage == .offline { await poll() }
        while !Task.isCancelled {
            try? await Task.sleep(for: .seconds(1))
            guard !Task.isCancelled else { return }
            tick()
            guard [.waiting, .joined, .offline].contains(stage) else { continue }
            n += 1
            if n % 3 == 0 { await poll() }
        }
    }

    private static var disconnected: AddComputerFailure {
        AddComputerFailure(message: String(localized: "Not connected to the hub yet"), code: nil)
    }
}

extension View {
    /// Add Computer / Confirm Computer as a sheet while `request` is set.
    func addComputerSheet(_ request: Binding<AddComputerRequest?>) -> some View {
        modifier(AddComputerSheetModifier(request: request))
    }

    /// The app-wide one (`AppModel.addComputer`), on the root.
    func addComputerPresentation() -> some View { modifier(AppAddComputerPresentation()) }
}

private struct AddComputerSheetModifier: ViewModifier {
    @Environment(AppModel.self) private var model
    @Binding var request: AddComputerRequest?

    func body(content: Content) -> some View {
        content.sheet(item: $request) { r in
            AddComputerSheet(request: r)
                .environment(model)
                #if os(macOS)
                .macSheet()
                #endif
        }
    }
}

private struct AppAddComputerPresentation: ViewModifier {
    @Environment(AppModel.self) private var model

    func body(content: Content) -> some View {
        @Bindable var model = model
        content
            #if os(macOS)
            // On a view of its own: a second sheet on the Mac's root next to New Agent's broke the window's layout.
            .background { Color.clear.addComputerSheet($model.addComputer) }
            #else
            .addComputerSheet($model.addComputer)
            #endif
            #if DEBUG
            // `-showAddComputer YES | add:<mac|linux|windows>:<name> | <machine id>`: Add Computer (filled in and sent with add:…),
            // or Confirm Computer for that computer, as the app opens: for screenshots.
            .task {
                guard let v = UserDefaults.standard.string(forKey: "showAddComputer"), !v.isEmpty else { return }
                try? await Task.sleep(for: .seconds(1.5))
                model.addComputer = v == "YES" || v.hasPrefix("add:") ? .add : .confirm(MachineRef(path: v))
            }
            #endif
    }
}

/// The sheet Add Computer / Confirm Computer open.
struct AddComputerSheet: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    @State private var page: AddComputerModel
    @State private var docLink: LinkTarget?
    @FocusState private var nameFocused: Bool
    private let confirming: Bool

    init(request: AddComputerRequest, group: String? = nil) {
        _page = State(initialValue: AddComputerModel(request, group: group ?? AppModel.shared.hub?.group ?? AppModel.shared.hubGroup))
        if case .confirm = request { confirming = true } else { confirming = false }
    }

    var body: some View {
        NavigationStack {
            SettingsForm { content }
                .themedPage()
                .navigationTitle(confirming ? String(localized: "Confirm Computer") : String(localized: "Add Computer"))
                .navigationBarTitleDisplayMode(.inline)
                .toolbar { toolbar }
                .pluginLinks($docLink)
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
        .interactiveDismissDisabled(page.stage == .working || page.busy)
        .task(id: page.stage == .form) {
            // Where the user starts typing: the name, once the sheet is up.
            if page.stage == .form { try? await Task.sleep(for: .seconds(0.5)); nameFocused = true }
        }
        .task {
            #if DEBUG
            if let v = UserDefaults.standard.string(forKey: "showAddComputer"), v.hasPrefix("add:"), page.stage == .form {
                let parts = v.split(separator: ":", maxSplits: 2).map(String.init)
                if parts.count == 3, let os = ComputerOS(rawValue: parts[1]) { page.os = os; page.name = parts[2]; await page.submit() }
            }
            #endif
            await page.run()
        }
        .accessibilityIdentifier("addComputer")
    }

    @ToolbarContentBuilder private var toolbar: some ToolbarContent {
        // ✕ at the leading corner at every stage (#9653); Next while the form is up.
        SheetClose(id: "addComputer.close") { dismiss() }
        if page.stage == .form || page.stage == .working {
            ToolbarItem(placement: .confirmationAction) {
                Button { Task { await page.submit() } } label: { Text("Next [Inbox Line acfc4e]").lineLimit(1) }
                    .disabled(page.validName == nil || page.stage == .working || page.assistantOffline != nil)
                    .accessibilityIdentifier("addComputer.next")
            }
        }
    }

    @ViewBuilder private var content: some View {
        if let f = page.failure { failure(f) }
        switch page.stage {
        case .form, .working:
            if let offline = page.assistantOffline { assistantOffline(offline) } else { form }
        case .waiting:
            addedUnder
            waiting
        case .joined:
            addedUnder
            joined
        case .offline: offline
        case .confirmed: outcome(symbol: Theme.Icon.successFilled, tone: Theme.positive, title: String(localized: "Added: \(page.computerName)"),
                                 hint: String(localized: "It appears in the machine list, and you can pick it when you start a new agent."), id: "addComputer.confirmed")
        case .rejected: outcome(symbol: Theme.Icon.delete, tone: .secondary, title: String(localized: "Removed “\(page.computerName)”"),
                                hint: String(localized: "The pairing link it used no longer works either. To add a computer, start again."), id: "addComputer.rejected")
        case .gone: outcome(symbol: Theme.Icon.info, tone: .secondary, title: String(localized: "“\(page.computerName)” is no longer there"),
                            hint: String(localized: "It may have been removed elsewhere, or it waited too long to be confirmed. To add a computer, start again."), id: "addComputer.gone")
        }
    }

    private func failure(_ f: AddComputerFailure) -> some View {
        Section {
            Label(f.message, systemImage: Theme.Icon.errorFilled)
                .font(Theme.Font.secondary).foregroundStyle(Theme.danger)
                .accessibilityIdentifier("addComputer.error")
        }
    }

    // MARK: The current assistant is offline

    private func assistantOffline(_ manager: HubManager) -> some View {
        Section {
            VStack(spacing: Theme.Space.s) {
                ThemedIcon(Theme.Icon.machine).font(Theme.Font.display).foregroundStyle(.secondary)
                Text("\(manager.name) is offline, so a computer can't be added to it right now.")
                    .font(Theme.Font.secondary).foregroundStyle(.secondary).multilineTextAlignment(.center)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, Theme.Space.l)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("addComputer.assistantOffline")
        }
    }

    // MARK: Under another assistant

    /// The computer went to another assistant than the one asked for (or the hub didn't say where: one from before `add.manager`).
    @ViewBuilder private var addedUnder: some View {
        if let name = page.addedUnder {
            Section {
                Label("This computer was added under \(name).", systemImage: Theme.Icon.info)
                    .font(Theme.Font.secondary)
                    .accessibilityIdentifier("addComputer.addedUnder")
            }
        }
    }

    // MARK: The form

    @ViewBuilder private var form: some View {
        @Bindable var page = page
        Section {
            Picker("This computer's system", selection: $page.os) {
                ForEach(ComputerOS.allCases) { Text($0.title).lineLimit(1).tag($0) }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .accessibilityIdentifier("addComputer.os")
        } header: {
            Text("This computer's system").textCase(nil)
        }
        Section {
            TextField(AddComputerText.namePlaceholder(page.os), text: $page.name)
                .focused($nameFocused)
                .submitLabel(.next)
                .onSubmit { Task { await page.submit() } }
                .accessibilityIdentifier("addComputer.name")
        } header: {
            Text("Name [Add Computer View 677331]").textCase(nil)
        } footer: {
            Text("Shown in the apps and on the web. Any language, up to 40 characters.")
        }
        if page.stage == .working {
            Section { ProgressView().frame(maxWidth: .infinity) }
        }
    }

    // MARK: Waiting for it

    @ViewBuilder private var waiting: some View {
        if let code = page.code {
            if page.expired {
                Section {
                    Button { Task { await page.reissue() } } label: {
                        Label("Regenerate", systemImage: Theme.Icon.reload).lineLimit(1)
                    }
                    .accessibilityIdentifier("addComputer.renew")
                } header: {
                    Text("This link has expired, and that computer hasn't connected yet.").textCase(nil)
                }
            } else {
                instructions(code)
                Section {
                    HStack(spacing: Theme.Space.m) {
                        ProgressView()
                        VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                            Text("Waiting for “\(page.computerName)” to connect…").font(Theme.Font.body).lineLimit(2)
                            Text("Expires in \(ConnectDeviceText.clock(page.secondsLeft))")
                                .font(Theme.Font.caption.monospacedDigit()).foregroundStyle(.secondary).lineLimit(1)
                                .accessibilityIdentifier("addComputer.countdown")
                        }
                    }
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("addComputer.waiting")
                }
            }
        }
    }

    @ViewBuilder private func instructions(_ code: APIClient.ComputerCode) -> some View {
        switch page.os {
        case .linux:
            let line = AddComputerText.linuxCommand(code)
            Section {
                step(line.installs ? String(localized: "On that computer, open a terminal, paste the line below and press Return. It installs Persona and connects.")
                                   : String(localized: "Install Persona Link on that computer first, then open a terminal, paste the line below and press Return."))
                if !line.installs { docButton(String(localized: "How to install [Add Computer View e8921c]"), AddComputerText.linuxInstallDoc) }
                command(line.line)
            } footer: {
                Text("Once it says it's paired, you're done: this screen moves on by itself.")
            }
        case .mac:
            Section {
                step(String(localized: "Install Persona Link on that Mac. There is no installer yet: you build it from source."))
                docButton(String(localized: "Install guide"), AddComputerText.macInstallDoc)
            }
            Section {
                step(String(localized: "Send the pairing link to that Mac, click the Persona Link icon in the menu bar › Pair…, and paste it."))
                if let url = URL(string: code.pairLink) {
                    ShareLink(item: url, subject: Text("Persona pairing link")) {
                        Label("Share Pairing Link", systemImage: Theme.Icon.share).lineLimit(1)
                    }
                    .accessibilityIdentifier("addComputer.share")
                }
                copyButton(code.pairLink, title: String(localized: "Copy Pairing Link"), said: String(localized: "Pairing link copied"))
            }
        case .windows:
            Section {
                step(String(localized: "On that computer, put Persona's two programs in one folder. There is no installer yet: you build them yourself, and the tray app is still being made."))
                step(String(localized: "Open PowerShell in that folder, paste the line below and press Enter."))
                command(AddComputerText.windowsCommand(code))
            } footer: {
                Text("It installs as a background task that starts when you sign in, with no window and no administrator rights.")
            }
        }
    }

    private func step(_ text: String) -> some View {
        Text(text).font(Theme.Font.body).fixedSize(horizontal: false, vertical: true)
    }

    @ViewBuilder private func command(_ line: String) -> some View {
        Text(line)
            .font(Theme.Font.code)
            .textSelection(.enabled)
            .lineLimit(4)
            .truncationMode(.middle)
            .accessibilityIdentifier("addComputer.command")
        copyButton(line, title: String(localized: "Copy [Readiness Views d37380]"), said: String(localized: "Copied [Mac Hover 07d6bb]"))
    }

    private func copyButton(_ text: String, title: String, said: String) -> some View {
        Button {
            UIPasteboard.general.string = text
            app.toast = Toast(said, style: .success)
        } label: {
            Label(title, systemImage: Theme.Icon.copy).lineLimit(1)
        }
        .accessibilityIdentifier("addComputer.copy")
    }

    private func docButton(_ title: String, _ url: URL) -> some View {
        Button { docLink = LinkTarget(url: url) } label: {
            Label(title, systemImage: Theme.Icon.openDocument).lineLimit(1)
        }
    }

    // MARK: It joined: is it yours?

    @ViewBuilder private var joined: some View {
        let system = page.machine.flatMap(AddComputerText.systemName)
        Section {
            VStack(spacing: Theme.Space.s) {
                ThemedIcon(page.machine.map { ComputerOS(reported: $0.os)?.symbol ?? $0.symbol } ?? page.os.symbol)
                    .font(Theme.Font.display)
                    .foregroundStyle(Theme.accent)
                Text(system.map { String(localized: "A computer wants to join: \(page.computerName) (\($0)). Is it yours?") }
                     ?? String(localized: "A computer wants to join: \(page.computerName). Is it yours?"))
                    .font(Theme.Font.heading).multilineTextAlignment(.center)
                Text("Only once you confirm does Persona work on it and give it your model keys. If you don't recognize it, say no and it is removed.")
                    .font(Theme.Font.secondary).foregroundStyle(.secondary).multilineTextAlignment(.center)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, Theme.Space.l)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("addComputer.joined")
        }
        if app.canConfirmComputer(page.group) { decision }
    }

    private var decision: some View {
        Section {
            FormAction {
                Button { Task { await page.confirm() } } label: {
                    Group {
                        if page.busy { ProgressView() } else { Text("It's Mine").lineLimit(1) }
                    }
                    .formActionWidth()
                }
                .themedButton(prominent: true)
                .disabled(page.busy)
                .accessibilityIdentifier("addComputer.confirm")
            }
            FormAction {
                Button(role: .destructive) { Task { await page.reject() } } label: {
                    Text("Not Mine").lineLimit(1).formActionWidth()
                }
                .themedButton()
                .disabled(page.busy)
                .accessibilityIdentifier("addComputer.reject")
            }
        }
        #if !os(macOS)
        .listRowSeparator(.hidden) // two buttons on clear rows: no line between them
        #endif
    }

    // MARK: Confirming one that is not here

    @ViewBuilder private var offline: some View {
        Section {
            VStack(alignment: .leading, spacing: Theme.Space.s) {
                Text("“\(page.computerName)” hasn't connected yet").font(Theme.Font.heading)
                (page.canReissue ? Text("Once it connects, you'll be asked here whether it's yours. If that computer never got the pairing link, make a new one.")
                                 : Text("Once it connects, you'll be asked here whether it's yours."))
                    .font(Theme.Font.secondary).foregroundStyle(.secondary)
            }
            .padding(.vertical, Theme.Space.xs)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("addComputer.offline")
            if page.canReissue {
                Button { Task { await page.reissue() } } label: {
                    Label("New Link", systemImage: Theme.Icon.reload).lineLimit(1)
                }
                .accessibilityIdentifier("addComputer.reissue")
            }
            if app.canConfirmComputer(page.group) {
                Button(role: .destructive) { Task { await page.reject() } } label: {
                    Label("Remove [Add Computer View c94330]", systemImage: Theme.Icon.delete).lineLimit(1)
                }
                .disabled(page.busy)
            }
        }
    }

    private func outcome(symbol: String, tone: Color, title: String, hint: String, id: String) -> some View {
        Section {
            VStack(spacing: Theme.Space.s) {
                ThemedIcon(symbol).font(Theme.Font.display).foregroundStyle(tone)
                Text(title).font(Theme.Font.heading).multilineTextAlignment(.center)
                Text(hint).font(Theme.Font.secondary).foregroundStyle(.secondary).multilineTextAlignment(.center)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, Theme.Space.l)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier(id)
        }
    }
}
