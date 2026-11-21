import Foundation
import Observation
import PersonaWire

/// One group of paired hubs as the app runs it (docs/protocol/multi-hub.md): its socket, the hello of the hub it is on, and the
/// per-hub things that came with it. The default group's link is the only one until a second group is paired.
@Observable
final class HubLink: Identifiable {
    let group: String
    var id: String { group }
    let live = LiveConnection()
    let larkDisplay = LarkMessageDisplayModel()
    /// Who answered on the socket: the hub's id, name, features and group (its hello).
    private(set) var hello = HubHello()
    /// What the hub listed in its hello beyond the first protocol (`terminal_stream`, `file_upload`…).
    private(set) var features: Set<String> = []
    /// The credential whose authenticated hello supplied these capabilities; nil before the first hello.
    private(set) var surfaceCapabilitiesConfig: ServerConfig?
    /// The group's last Fleet state (hub/liveactivity.go): merged with the other groups' for the Live Activity.
    var fleet: FleetState?
    /// The hub's `desktops` list: Mac apps connected and computers where Persona Link reports you at them (the banner over
    /// the Agents list shows the hub-level group's).
    var desktops: [DesktopDevice] = []
    /// Pinned and muted spaces / agents: each hub keeps its own.
    @ObservationIgnored let pins: PinsStore
    /// The last good workspace snapshot for this group's @ menus; replaced, never accumulated, and dropped with the link.
    @ObservationIgnored let workspaceMentions = MentionWorkspaceCache()
    /// Where requests go when the socket has not picked a hub yet: the group's first hub.
    @ObservationIgnored var fallback: ServerConfig
    #if DEBUG
    /// Unit tests: what answers this link's requests instead of the network or `APIClient.local`
    /// (`AppModel.linkHooksForTests`).
    @ObservationIgnored var transportForTests: APIClient.Transport?
    #endif

    init(group: String, config: ServerConfig, pins: PinsStore) {
        self.group = group
        self.fallback = config
        self.pins = pins
        live.usesSessionList = true
        #if os(iOS) || os(macOS)
        live.usesExtensionUIStream = true
        #endif
    }

    /// The hub requests go to: the one the socket is on.
    var config: ServerConfig { live.currentConfig ?? fallback }
    var api: APIClient {
        var c = APIClient(config: config, group: group)
        c.requestRoute = { [weak live] in try await live?.waitForRoute() }
        c.extensionsAPI = hello.features.isEmpty ? Self.remembersExtensions(group) : features.contains("extensions")
        c.personaNaming = live.personaContracts
        c.sessionLookup = features.contains("sessions_lookup")
        c.sessionList = features.contains("sessions_list")
        c.sessionsHierarchy = features.contains(ChildSessions.feature)
        #if DEBUG
        c.transport = transportForTests
        #endif
        return c
    }

    /// Whether the group's hub had the extension routes at its last hello (#1859): what requests made before this launch's
    /// hello use, so a hub from #1844 on is not asked on its old /api/v1/plugins (counted there until nobody calls it).
    /// Written only when it changes, so rarely.
    private static func remembersExtensions(_ group: String) -> Bool {
        UserDefaults.standard.bool(forKey: "hubExtensionsAPI." + group)
    }
    var isDemo: Bool { config.isDemo }

    /// The hub folds side sessions into their parent's chat (`SideFold.feature`, #7944): by its hello, or before this
    /// launch's hello by what the last one said, so a restored list does not show its sides for a moment.
    var foldsSides: Bool { hello.features.isEmpty ? SideFold.remembered(group) : features.contains(SideFold.feature) }

    /// The hub's messaging sources (#2138, docs/extensions/extension-messaging.md): the Feishu screens go through the first.
    private(set) var messagingSources: [ExtensionMessagingSource] = []

    /// Reads the sources when the hub has the feature, forgets them when it does not.
    func refreshMessagingSources(again: Bool = false) async {
        let hub = config.url
        guard features.contains("messaging") else {
            messagingSources = []
            if MessagingSources.set(nil, for: hub) { await sourceChanged() }
            live.followsMessaging = false
            return
        }
        guard let raw = try? await api.request("GET", "/api/v1/messaging"),
              let list = try? JSON.decoder.decode(ExtensionMessagingSources.self, from: raw) else { return }
        // An extension starting again lists no source for a moment (a reconnect then): the one this hub had is let go
        // only when it is still not there a little later.
        if list.sources.isEmpty, !again, MessagingSources.source(for: hub) != nil {
            try? await Task.sleep(for: .seconds(15))
            await refreshMessagingSources(again: true)
            return
        }
        messagingSources = list.sources
        if MessagingSources.set(list.sources.first?.id, for: hub) { await sourceChanged() }
        // The legacy `lark` event is dropped while a source is used (`AppModel.handle`): the hub need not send it (#9591).
        live.followsMessaging = api.messagingSource != nil
    }

    /// The Feishu screens moved between the old API and a source (a first launch reads before the hello came): their
    /// settings and what is on screen are read again from the one they use now, nothing kept from the other.
    private func sourceChanged() async {
        await larkDisplay.refresh(using: api)
        LarkSync.shared.displayChanged()
    }

    func received(_ hello: HubHello) {
        self.hello = hello
        if surfaceCapabilitiesConfig != config { surfaceCapabilitiesConfig = config }
        let f = Set(hello.features)
        let changed = f != features
        if changed { features = f }
        // The sources are read again when the hub's features change (a source coming or going changes `messaging`), or
        // while none is known; not on every reconnect.
        if changed || messagingSources.isEmpty { Task { await refreshMessagingSources() } }
        let ext = f.contains("extensions")
        if !f.isEmpty, ext != Self.remembersExtensions(group) { UserDefaults.standard.set(ext, forKey: "hubExtensionsAPI." + group) }
        if !f.isEmpty { SideFold.remember(f.contains(SideFold.feature), group: group) }
        if !f.isEmpty { TodosV1.shared.heard(TodoFeatures(f), from: group) }
    }
}

extension AppModel {
    /// Sends the error reports kept on disk to `group`'s hub (the caller passes the hub-level one), or deletes them when its last hello has no
    /// `error_reports` (#5621, docs/protocol/protocol.md "Error reports"). The watch and visionOS do not report yet.
    func flushErrorReports(_ group: String) {
        #if os(iOS) || os(macOS)
        guard let link = link(group), !link.isDemo else { return }
        let features = link.features, api = link.api
        Task.detached(priority: .utility) { _ = await ErrorReportSender.shared.flush(features: features, api: api) }
        #endif
    }

    // MARK: Groups (docs/protocol/multi-hub.md)

    /// The groups in order (the default group first).
    var groupIDs: [String] { links.map(\.group) }
    /// Several groups: lists say which group each row is from and can show one group only. One group: nothing changes.
    var isMultiHub: Bool { links.count > 1 }

    func link(_ group: String) -> HubLink? { links.first { $0.group == group } }

    /// The group hub-level pages (Persona, Inbox, the hub's settings) show: the one picked, else the first.
    var hub: HubLink? { link(hubGroup) ?? links.first }

    /// A client for the group a session, machine or interaction is from; the assistant's routes go to the manager its
    /// Persona page shows (`APIClient.showing`).
    func api(_ group: String) -> APIClient? {
        #if DEBUG
        if let apiForTests { return apiForTests }
        #endif
        return link(group)?.api.showing(assistantPick(in: group))
    }

    /// Back in the app: every group's socket opens again.
    func resumeLinks() {
        addressBoard.cameToFront(at: .now) // probes from now on say what they came to (#11390)
        reportGroups() // a period in front begins for Developing Persona's connection tally
        for link in links {
            link.live.suspended = false
            link.live.ensureConnected()
        }
    }

    /// In the background: every socket closes (the hubs wake the phone with a silent push when they need it).
    func suspendLinks() {
        addressBoard.wentToBackground(at: .now) // probes still running are cut short (#11390)
        for link in links {
            link.live.suspended = true
            link.live.disconnect()
        }
        // iOS cuts them in the background anyway; back in front each socket picks its way again (docs/operations/network.md).
        closeTunnelsForBackground()
    }

    /// A silent push in the background: the group that sent it connects for its call (nil: every group).
    func wakeLinks(_ group: String?) {
        for link in links where group == nil || link.group == group { link.live.wake() }
    }

    /// The group's name as lists label it: the user's name for it, else its first hub's (`hub_name`), else what the
    /// hub the socket is on calls itself, else the address. Not the connected hub's first: that changes with the route.
    func groupName(_ group: String) -> String {
        if link(group)?.isDemo == true { return String(localized: "Demo Mode") }
        if let n = registry.groupNames[group]?.trimmingCharacters(in: .whitespacesAndNewlines), !n.isEmpty { return n }
        if let n = registry.primary(group)?.hubName?.trimmingCharacters(in: .whitespacesAndNewlines), !n.isEmpty { return n }
        if let name = link(group)?.hello.hubName, !name.isEmpty { return name }
        return registry.name(group)
    }

    /// Picks the group hub-level pages show.
    func selectHub(_ group: String) {
        guard group != hub?.group, link(group) != nil else { return }
        hubGroup = group
        UserDefaults.standard.set(group, forKey: Self.hubGroupKey)
        hubChanged()
    }

    static let hubGroupKey = "hub.group"

    /// A key from outside the app's own lists (a deep link, a notification, Siri, a launch argument). `group` nil:
    /// not said (an older link, a push from an older hub) — the group that has this session, else the first group.
    /// A group that is gone is treated the same way.
    func resolve(machineID: String, sessionID: String, group: String?) -> SessionKey {
        if let group, link(group) != nil { return SessionKey(group: group, machineID: machineID, sessionID: sessionID) }
        let found = links.first { latest(SessionKey(group: $0.group, machineID: machineID, sessionID: sessionID)) != nil }?.group
        return SessionKey(group: found ?? links.first?.group ?? "", machineID: machineID, sessionID: sessionID)
    }

    /// A key a hub-level page read off its hub (a routine's conversation, the AI Twin's): in the hub-level group.
    func inHub(_ key: SessionKey) -> SessionKey {
        var k = key
        k.group = hub?.group ?? ""
        return k
    }

    /// `resolve` for a key read from a string (`SessionKey.path`): a path without a group says nothing about it.
    func resolve(path: String) -> SessionKey? {
        guard let k = SessionKey(path: path) else { return nil }
        return resolve(machineID: k.machineID, sessionID: k.sessionID, group: path.hasPrefix("|") ? "" : k.hubGroup)
    }

    func openExternal(path: String, latestAnswer: Bool = false) {
        guard let k = SessionKey(path: path) else { return }
        openExternal(machineID: k.machineID, sessionID: k.sessionID, group: path.hasPrefix("|") ? "" : k.hubGroup,
                     latestAnswer: latestAnswer)
    }

    /// The group a push came from: the hub that sent it (`hub_id`), the push key that opened it, else the group that
    /// has its session (docs/protocol/multi-hub.md「Push and notifications」).
    func pushGroup(_ info: [AnyHashable: Any]) -> String? {
        if let id = info["hub_id"] as? String, let g = registry.group(hubID: id), link(g) != nil { return g }
        if let g = PushCrypto.group(from: info), link(g) != nil { return g }
        return nil
    }
}

extension AppModel {
    // MARK: Routes, hellos, groups changing

    /// What a group's socket asks before each attempt when the group has more than one place to go: the quickest
    /// hub that answers (HubRoute), or a tunnel when none does (`tunnelRoute`, docs/operations/network.md「Connecting to hub through SSH」
    /// 「经 App 内隧道连 hub」). One place and no tunnel: nil, the socket goes where it was told.
    func routePicker(_ group: String) -> (@MainActor () async -> ServerConfig?)? {
        let tunneled = !hubTunnels(group).isEmpty
        guard tunneled || HubRoute.candidates(registry, group: group).count > 1 else { return nil }
        return { [weak self] in
            guard let self else { return nil }
            self.addressBoard.beginAttempt(group)
            if !self.hubTunnels(group).isEmpty { return await self.tunnelRoute(group) }
            let candidates = HubRoute.candidates(self.registry, group: group)
            let start = Date()
            let (picked, tried) = await HubRoute.pickTried(candidates, check: self.routeCheck)
            self.attemptTried(group, tried, since: start)
            return picked?.config
        }
    }

    /// A hello came: remember which hub answered, its name and its group (`mesh`), and regroup the paired hubs when
    /// that changed what belongs together (docs/protocol/multi-hub.md「Grouping」).
    func learned(_ hello: HubHello, group: String) {
        guard let link = link(group), !link.isDemo else { return }
        reportConnected(hello, group: group)
        defer { watchCenter(group) } // on a member now? (after the registry learned who the center is)
        var r = registry
        let on = link.live.currentConfig
        // The paired hub the socket is on (at any of its addresses); on an address only the mesh gave, the group's first
        // hub stands for it.
        let at = { (h: PairedHub) in on.map { h.configs.contains($0) } == true }
        guard let i = r.hubs.firstIndex(where: { $0.group == group && at($0) })
                ?? r.hubs.firstIndex(where: { $0.group == group }) else { return }
        let paired = at(r.hubs[i])
        // On an added address, a hello from another hub (the probe let through what it could not tell apart) changes
        // nothing: the paired hub keeps who it is.
        if paired, on != r.hubs[i].config, let known = r.hubs[i].hubID, !known.isEmpty, hello.hubID != known { return }
        if paired {
            if let id = hello.hubID, !id.isEmpty { r.hubs[i].hubID = id }
            if let name = hello.hubName, !name.isEmpty { r.hubs[i].hubName = name }
            // No mesh in its hello (a hub before the mesh, or one that left it): it lists nobody now. A mesh kept from
            // an earlier hello would show its old peers (and itself, under an old hub_id) as hubs of the group.
            if hello.mesh == nil { r.hubs[i].mesh = nil }
        }
        if let mesh = hello.mesh {
            r.hubs[i].mesh = mesh
            // Paired hubs the socket was never on: who they are, from the address the mesh lists for them.
            for j in r.hubs.indices where r.hubs[j].hubID == nil || r.hubs[j].hubName == nil {
                let url = r.hubs[j].config.url.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
                guard let m = mesh.first(where: { $0.endpoints?.contains { $0.trimmingCharacters(in: CharacterSet(charactersIn: "/")) == url } == true })
                else { continue }
                if r.hubs[j].hubID == nil { r.hubs[j].hubID = m.hubID }
                if r.hubs[j].hubName == nil, let n = m.name, !n.isEmpty { r.hubs[j].hubName = n }
            }
        }
        let regrouped = HubGrouping.regroup(r)
        guard regrouped != registry else { return }
        defer { relabelMachines() } // the hub may have a new name
        let groupsChanged = regrouped.groups != registry.groups || regrouped.hubs.map(\.group) != registry.hubs.map(\.group)
        registry = regrouped
        saveRegistry(regrouped)
        guard groupsChanged else {
            // Same groups, a new address to try: the socket picks among them from its next attempt.
            for l in links where !l.isDemo { l.live.route = routePicker(l.group) }
            arrangeAddresses()
            return
        }
        // Groups merged or split: the groups that are gone take their lists with them (the group that stays lists
        // the same computers), and the new ones connect.
        let before = Set(groupIDs)
        buildLinks()
        let gone = before.subtracting(groupIDs)
        // A merged-away group's push key stays: its hubs may still seal to it until the phone registers again.
        if !gone.isEmpty { forgetGroups(gone, keepingPushKeys: true) }
        for l in links where !before.contains(l.group) { l.live.connect(l.fallback) }
        pushWatchConfigs()
    }

    /// The socket is on a member while the group's center is another hub (docs/protocol/multi-hub.md「Connections and routing」), or on a
    /// hub's later address while an earlier one is there (#4670): ask the preferred ones every few minutes and move once
    /// one answers. Ends when the socket is on the best-ranked candidate, or the group is gone.
    func watchCenter(_ group: String) {
        guard centerWatch[group] == nil, onMember(group) else { return }
        centerWatch[group] = Task { @MainActor [weak self] in
            defer { self?.centerWatch[group] = nil }
            while !Task.isCancelled {
                try? await Task.sleep(for: HubRoute.centerRecheck)
                guard let self, !Task.isCancelled, self.onMember(group) else { return }
                if await self.preferredAnswers(group) { self.link(group)?.live.reroute(); return }
            }
        }
    }

    /// The network changed (another Wi-Fi, a VPN on or off): a socket on a fallback moves back now when a preferred
    /// candidate answers, instead of at the next recheck.
    func recheckRoutes() {
        for l in links where onMember(l.group) {
            Task { @MainActor [weak self] in
                if await self?.preferredAnswers(l.group) == true { l.live.reroute() }
            }
        }
    }

    /// One of the candidates ranked before the one the socket is on answers. Nothing while in the background: nothing
    /// opens the socket anyway.
    private func preferredAnswers(_ group: String) async -> Bool {
        guard let link = link(group), !link.live.suspended else { return false }
        return await askPreferred(group, preferred(group))
    }

    /// Asks `candidates` in turn until one answers, each try onto the group's lines and the tally. One attempt: a probe
    /// under way at the background notice, or asked while the app was away, was cut short; one asked after the app came
    /// back says what it came to.
    func askPreferred(_ group: String, _ candidates: [HubRoute.Candidate], check: HubRoute.Check = HubRoute.healthCheck) async -> Bool {
        let all = HubRoute.candidates(registry, group: group)
        let start = Date()
        for c in candidates {
            let asked = Date()
            let outcome = await check(c)
            tried(group, HubRoute.settled([.init(candidate: c, outcome: outcome, at: Date())], among: all), since: start, asked: asked)
            if outcome == .answers { return true }
        }
        return false
    }

    /// The candidates ranked before the one the group's socket is on.
    private func preferred(_ group: String) -> [HubRoute.Candidate] {
        guard let link = link(group), !link.isDemo, let on = link.live.currentConfig else { return [] }
        let candidates = HubRoute.candidates(registry, group: group)
        guard let current = candidates.first(where: { $0.url == on.url }) else {
            return candidates.filter(\.center) // on an address no candidate has (the center known only now)
        }
        return candidates.filter { $0.rank < current.rank }
    }

    /// A group's socket is on a candidate behind another (a member while the center is known, a hub's later address).
    func onMember(_ group: String) -> Bool { !preferred(group).isEmpty }

    // MARK: What each address came to (#11390)

    /// The groups' addresses in the order they are tried, for Connection Details' lines; a group that went takes its
    /// lines with it. Also tells Developing Persona's tally which groups there are.
    func arrangeAddresses() {
        let groups = links.filter { !$0.isDemo }.map(\.group)
        addressBoard.keep(Set(groups))
        for g in groups { addressBoard.arrange(g, addresses: HubRoute.candidates(registry, group: g).map(\.url)) }
        #if DEBUG
        // `-showConnectionAddresses YES`: every outcome on the hub-level group's addresses, made up (screenshots).
        if UserDefaults.standard.bool(forKey: "showConnectionAddresses"), let g = hub?.group, groups.contains(g) { addressBoard.seed(g) }
        #endif
        reportGroups()
    }

    /// What an attempt that began at `start` found at each address (the probes, asked at `asked` when not all at once):
    /// onto the group's lines, and into Developing Persona's tally while the switch is on. What came after the app went
    /// to the background meanwhile was cut short: unknown, unless it was asked after the app came back.
    func tried(_ group: String, _ tried: [HubRoute.Tried], since start: Date, asked: Date? = nil) {
        guard !tried.isEmpty, let link = link(group), !link.isDemo else { return }
        let tried = addressBoard.cutShort(tried, since: start, asked: asked)
        addressBoard.record(group, tried.map { ($0.candidate.url, $0.outcome, $0.at) })
        tally(.tried(group: group, tried.map(\.outcome), since: start))
    }

    /// What the route's health probes answer: `HubRoute.healthCheck` (unit tests answer for it).
    var routeCheck: HubRoute.Check {
        #if DEBUG
        if let routeCheckForTests { return routeCheckForTests }
        #endif
        return HubRoute.healthCheck
    }

    /// Tells Developing Persona's tally what the lines learned; nothing while the switch is off.
    private func tally(_ event: @autoclosure () -> DevProbe.Link) {
        #if DEBUG
        if let tallyForTests { tallyForTests(event()); return }
        #endif
        if DevProbe.listening { DevProbe.link(event()) }
    }

    /// `tried` for the probes of one of the group's socket attempts (its route picker): the addresses they found
    /// answering are the ones where the socket's own failure may then say what the attempt came to. A pick that was
    /// cancelled or replaced (Retry Now, the network back, a renewed credential, the background) keeps what its probes
    /// came to and hands the socket nothing: the next attempt may have begun already (#11683).
    func attemptTried(_ group: String, _ tried: [HubRoute.Tried], since start: Date) {
        self.tried(group, tried, since: start)
        guard !Task.isCancelled else { return }
        attemptAnswered(group, tried, since: start)
    }

    /// The addresses this attempt's probes found answering, for the socket (`HubAddressBoard.takeAnswered`).
    func attemptAnswered(_ group: String, _ tried: [HubRoute.Tried], since start: Date) {
        addressBoard.attemptAnswered(group, addressBoard.cutShort(tried, since: start).filter { $0.outcome == .answers }.map(\.candidate.url))
    }

    /// What one of the group's socket attempts, begun at `start`, came to at the address it went to
    /// (`LiveConnection.onAttempt`). One source per attempt and address: a group the socket goes to straight (one
    /// address, no tunnel) has no probe, so the socket says; otherwise the probe said, and the socket only replaces an
    /// Answers that the same attempt's probe found at the address it then failed to open (`attemptAnswered`).
    func socketTried(_ group: String, _ config: ServerConfig, _ outcome: HubOutcome, since start: Date) {
        guard let link = link(group), !link.isDemo else { return }
        if link.live.route != nil {
            // Only where this attempt's own probe found the address answering: through a tunnel set to be used every
            // time no probe runs, and the socket's failure there is the tunnel's, not the address's.
            guard outcome != .answers, addressBoard.takeAnswered(group, config.url) else { return }
        }
        addressBoard.record(group, [(config.url, outcome, Date())])
        // Counted from when the attempt began, as the probes' tries: one begun before the period is left out of it.
        tally(.tried(group: group, [outcome], since: start))
    }

    /// The group's socket changed state (`LiveConnection.onChange`): not connected any more, for Developing Persona's
    /// tally. Nothing while the switch is off.
    func socketChanged(_ group: String) {
        guard DevProbe.listening, let link = link(group), !link.isDemo, link.live.state != .connected else { return }
        DevProbe.link(.disconnected(group: group))
    }

    /// Developing Persona's tally: the groups there are (Demo Mode left out). Nothing while the switch is off.
    func reportGroups() {
        guard DevProbe.listening else { return }
        DevProbe.link(.groups(Set(links.filter { !$0.isDemo }.map(\.group))))
    }

    /// Developing Persona's tally: the group's socket is on its hub, and how it got there (only fixed words, never the
    /// address). Nothing while the switch is off.
    func reportConnected(_ hello: HubHello, group: String) {
        guard DevProbe.listening, let link = link(group), !link.isDemo, let on = link.live.currentConfig else { return }
        DevProbe.link(.connected(group: group, Self.connectedKind(on: on, hello: hello, registry: registry, group: group) { hub in
            if SSHTunnels.shared.routed.contains(hub.id) { return .sshTunnel }
            if LocalProxies.shared.routed.contains(hub.id) { return .localProxy }
            if InAppTunnels.shared.routed.contains(hub.id) { return .inAppTunnel }
            return .direct
        }))
    }

    /// How a group's socket on `on` got to its hub, as the tally words it: which address it is, the way it went
    /// (`routed`: the paired hub's tunnel or proxy carrying it, if any), what vouched for it, whether that address
    /// carries its own Access token, and whether the hello lists a key fingerprint for the hub that sent it.
    nonisolated static func connectedKind(on: ServerConfig, hello: HubHello, registry: HubRegistry, group: String,
                                          routed: (PairedHub) -> DevProbe.Connected.Route) -> DevProbe.Connected {
        let key = HubRoute.key(on.url)
        let candidate = HubRoute.candidates(registry, group: group).first { HubRoute.key($0.url) == key }
        let address: DevProbe.Connected.Address = candidate.map { $0.hub == nil ? .mesh : $0.address == 0 ? .paired : .added } ?? .paired
        let route = registry.members(group).first(where: { $0.configs.contains { HubRoute.key($0.url) == key } }).map(routed) ?? .direct
        let own = hello.mesh?.first { $0.isSelf == true } ?? hello.mesh?.first { $0.hubID == hello.hubID }
        // Through a tunnel or proxy the socket goes to its loopback end in plain HTTP: the way vouches, not the system.
        let trust: DevProbe.Connected.Trust = route != .direct ? .tunnel : on.url.scheme?.lowercased() == "https" ? .system : .plainHTTP
        return .init(trust: trust, accessToken: on.access != nil, route: route, address: address,
                     helloFingerprint: own?.fp?.isEmpty == false)
    }

    /// The SSH tunnels that go with group (#2379): each of its hubs may have one, kept under the hub's id.
    nonisolated static func tunnelIDs(removing group: String, from registry: HubRegistry) -> [String] {
        registry.members(group).map(\.id)
    }

    /// Settings › hub › Disconnect: a whole group (every hub of it) goes; the others keep everything. The last group: `unpair`.
    func removeGroup(_ group: String) {
        guard links.count > 1 else { unpair(); return }
        // Before the registry changes: afterwards the group's hubs (and their tunnels' ids) are gone from it.
        for id in Self.tunnelIDs(removing: group, from: registry) { closeTunnels(id) }
        var r = registry
        r.hubs.removeAll { $0.group == group }
        r.groupNames[group] = nil
        registry = r
        saveRegistry(r)
        buildLinks()
        forgetGroups([group])
        if hubGroup == group { hubChanged() }
        pushWatchConfigs()
    }

    /// One hub of a group goes (the group stays with its other hubs). Its last hub: the group goes.
    func removeHub(_ id: String) {
        guard let hub = registry.hubs.first(where: { $0.id == id }) else { return }
        guard registry.members(hub.group).count > 1 else { removeGroup(hub.group); return }
        closeTunnels(id)
        updateHeldRetryReceipts { receipts in receipts.filter { $0.target.pairing != id } }
        var r = registry
        r.hubs.removeAll { $0.id == id }
        registry = r
        saveRegistry(r)
        buildLinks()
        // A socket on that hub's credential moves to the group's other hubs: at its address, or at an address only the
        // mesh listed (those go with the first hub's credential). Disconnect This Hub has revoked it (HubSignOut.swift).
        if let link = link(hub.group), let on = link.live.currentConfig,
           DeviceCredentialTrade.current(on, traded: tradedTokens).token == DeviceCredentialTrade.current(hub.config, traded: tradedTokens).token {
            link.live.disconnect()
            link.live.connect(link.fallback)
        }
        pushWatchConfigs()
    }

    /// Settings › hub › Rename: what lists call a group ("" goes back to its hub's own name).
    func renameGroup(_ group: String, to name: String) {
        var r = registry
        let n = name.trimmingCharacters(in: .whitespacesAndNewlines)
        r.groupNames[group] = n.isEmpty ? nil : n
        registry = r
        saveRegistry(r)
        relabelMachines()
        pushWatchConfigs()
    }

    /// The registry with one hub changed (its device token, its watch's credential), saved.
    func updateHub(_ id: String, _ change: (inout PairedHub) -> Void) {
        guard let i = registry.hubs.firstIndex(where: { $0.id == id }) else { return }
        var r = registry
        change(&r.hubs[i])
        registry = r
        saveRegistry(r)
    }

    /// Settings › hub › Cloudflare Access saved (nil: cleared): the hub's requests carry it from now on, the group's
    /// socket reconnects with it, and the watch and the widgets get it.
    func setHubAccess(_ access: HubAccess?, hub id: String) {
        guard let hub = registry.hubs.first(where: { $0.id == id }), hub.config.access != access else { return }
        updateHub(id) { $0.config.access = access }
        buildLinks() // the links' fallback config, and the widgets' credentials
        pushWatchConfigs()
        guard let link = link(hub.group), !link.isDemo else { return }
        link.live.disconnect()
        link.live.connect(link.fallback) // its route picks the group's hub again, from the registry
    }

    /// Settings › hub › Addresses saved: the hub's added addresses in the order to try (#4670). The group's socket picks
    /// its way again among them.
    func setHubAddresses(_ addresses: [HubAddress], hub id: String) {
        guard let hub = registry.hubs.first(where: { $0.id == id }), (hub.addresses ?? []) != addresses else { return }
        updateHub(id) { $0.addresses = addresses.isEmpty ? nil : addresses }
        buildLinks() // the links' route pickers
        guard let link = link(hub.group), !link.isDemo else { return }
        link.live.disconnect()
        link.live.connect(link.fallback)
    }

    // MARK: Live Activity

    /// Every group's Fleet state as one (docs/protocol/multi-hub.md「Live Activities」): counts added up, the requests of every group
    /// first, then the working and finished agents; another group's rows carry it in their ids ("group|…"), so a tap
    /// or a button reaches the right hub. One group: its state as the hub sent it.
    /// Agents of quiet tasks (#1731) are left out of each group's first: a hub that knows them never sends them, this
    /// catches a fleet from before the task was marked.
    func mergedFleet() -> FleetState {
        let states = links.compactMap { l in l.fleet.map { (group: l.group, state: quietless($0, group: l.group)) } }
        guard states.count > 1 || states.first?.group.isEmpty == false else { return states.first?.state ?? FleetState() }
        return FleetMerge.merge(states, base: hub?.fleet.map { quietless($0, group: hub?.group ?? "") } ?? states.first?.state)
    }

    private func quietless(_ fleet: FleetState, group: String) -> FleetState {
        fleet.leavingOut { item in
            guard let k = item.key else { return false }
            return isQuiet(SessionKey(group: group, machineID: k.machine, sessionID: k.session))
        }
    }

    // MARK: Watch

    /// What the watch gets (#1185): each group's answer-only credential for the watch, and the groups without one yet,
    /// never the phone's own (it opens everything); a group the watch has no credential for is left off it.
    func watchConfigs() -> WatchConfigs {
        let (configs, missing) = registry.watchCredentials()
        return WatchConfigs(configs: configs, names: Dictionary(uniqueKeysWithValues: registry.groups.map { ($0, groupName($0)) }),
                            missing: missing,
                            deviceIDs: Dictionary(uniqueKeysWithValues: configs.keys.compactMap { group in
                                registry.primary(group)?.watchDeviceID.map { (group, $0) }
                            }), features: extensionSurfaceCapabilities.filter { configs[$0.key] != nil })
    }

    /// Only the authenticated hello of the hub whose credential is exported, never another mesh member's.
    var extensionSurfaceCapabilities: [String: [String]] {
        Dictionary(uniqueKeysWithValues: registry.groups.compactMap { group in
            guard let primary = registry.primary(group), let link = link(group),
                  link.surfaceCapabilitiesConfig == primary.config else { return nil }
            return (group, link.features.sorted())
        })
    }

    func pushWatchConfigs() {
        guard !isDemo else { return }
        WatchSync.shared.push(watchConfigs())
    }
}

/// What a push names: its machine and session, and which group it came from when it says (`hub_id` of the hub that
/// sent it, or the group whose key opened it). Read off the main thread; `key` resolves it on it.
nonisolated struct PushRef: Sendable, Equatable {
    var machine: String
    var session: String
    var hubID: String?
    var group: String?

    init?(_ info: [AnyHashable: Any]) {
        guard let machine = info["machine_id"] as? String, let session = info["session_id"] as? String,
              !machine.isEmpty, !session.isEmpty else { return nil }
        self.machine = machine
        self.session = session
        hubID = info["hub_id"] as? String
        group = PushCrypto.group(from: info)
    }

    /// The paired group identified by the sender or decryption key, if the push names one.
    @MainActor func sourceGroup(_ model: AppModel) -> String? {
        var info: [AnyHashable: Any] = [:]
        if let hubID { info["hub_id"] = hubID }
        if let group { info[PushCrypto.groupKey] = group }
        return model.pushGroup(info)
    }

    /// Notification actions need a concrete key even when the push did not specify a group.
    @MainActor func key(_ model: AppModel) -> SessionKey {
        model.resolve(machineID: machine, sessionID: session, group: sourceGroup(model))
    }
}

/// Merging the groups' Fleet states (pure, for the tests).
nonisolated enum FleetMerge {
    static func merge(_ states: [(group: String, state: FleetState)], base: FleetState?) -> FleetState {
        var out = base ?? FleetState()
        out.needsYou = states.reduce(0) { $0 + $1.state.needsYou }
        out.working = states.reduce(0) { $0 + $1.state.working }
        out.unread = states.reduce(0) { $0 + $1.state.unread }
        out.phase = out.needsYou > 0 ? .needsYou : out.working > 0 ? .working : out.unread > 0 ? .done : .none
        out.workingSince = states.compactMap(\.state.workingSince).min()
        out.assistant = states.contains { $0.state.assistant }
        let items = states.flatMap { g, s in s.items.map { qualified($0, group: g) } }
        out.items = items.filter(\.needsYou) + items.filter { !$0.needsYou }
        return out
    }

    static func qualified(_ item: FleetState.Item, group: String) -> FleetState.Item {
        guard !group.isEmpty else { return item }
        var i = item
        i.id = group + "|" + item.id
        i.interaction = item.interaction.map { group + "|" + $0 }
        return i
    }

    /// An interaction id from the activity: its group ("" when the hub pushed it, unqualified) and the hub's id.
    static func split(_ id: String) -> (group: String?, id: String) {
        guard let bar = id.firstIndex(of: "|") else { return (nil, id) }
        return (String(id[..<bar]), String(id[id.index(after: bar)...]))
    }
}
