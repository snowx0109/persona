import Foundation
import PersonaWire

// Several hubs on one device (docs/protocol/multi-hub.md, docs/archive/operations/plan-hub-mesh.md 4). Every hub paired is kept with its own
// credential; hubs that know each other (the `mesh` of their hello) form a group, reached over one connection; groups
// that do not are shown side by side. The default group ("") is the one the old single pairing migrates into (or the
// first one paired): while it is the only group, every key, path and screen is what it was before.

/// One hub of a group as its hello lists it (`mesh.hubs`, docs/archive/operations/plan-hub-mesh.md 2.6).
nonisolated struct MeshHub: Codable, Hashable, Sendable {
    var hubID: String
    var name: String?
    var online: Bool?
    var isSelf: Bool?
    var endpoints: [String]?
    var tlsPort: Int?
    var fp: String?
    /// The group's center (docs/archive/operations/plan-hub-center.md 5): the hub that serves the apps; the others pass requests on to it.
    /// nil from hubs before the single center.
    var center: Bool?

    enum CodingKeys: String, CodingKey {
        case name, online, endpoints, fp, center
        case hubID = "hub_id", isSelf = "self", tlsPort = "tls_port"
    }

    init(hubID: String, name: String? = nil, online: Bool? = nil, isSelf: Bool? = nil, endpoints: [String]? = nil,
         center: Bool? = nil) {
        self.hubID = hubID
        self.name = name
        self.online = online
        self.isSelf = isSelf
        self.endpoints = endpoints
        self.center = center
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        hubID = try c.decode(String.self, forKey: .hubID)
        name = try? c.decodeIfPresent(String.self, forKey: .name)
        online = try? c.decodeIfPresent(Bool.self, forKey: .online)
        isSelf = try? c.decodeIfPresent(Bool.self, forKey: .isSelf)
        endpoints = try? c.decodeIfPresent([String].self, forKey: .endpoints)
        tlsPort = try? c.decodeIfPresent(Int.self, forKey: .tlsPort)
        fp = try? c.decodeIfPresent(String.self, forKey: .fp)
        center = try? c.decodeIfPresent(Bool.self, forKey: .center)
    }
}

/// Who the hub at the other end of a socket is (its hello): id, name and the group it is in (docs/protocol/multi-hub.md), and
/// its build and protocol for the compatibility notices (docs/operations/hub-update.md「Compatibility」).
nonisolated struct HubHello: Hashable, Sendable {
    var features: [String] = []
    var hubID: String?
    var hubName: String?
    /// nil from hubs before the mesh (or a hello without authentication).
    var mesh: [MeshHub]?
    var channel: String?
    var build: Int64?
    var protocolVersion: Int?
    var minAppBuild: Int?
    /// The address the hub's own links use (hub.json `public_url`, #2696); nil when not set or from an older hub.
    var publicURL: URL?
}

/// One hub this device paired with: its address, this device's credential there, and what the app learned about it.
nonisolated struct PairedHub: Codable, Hashable, Sendable, Identifiable {
    /// The app's own id for the pairing (stable; the hub's `hub_id` is only known once it answered).
    var id: String
    var config: ServerConfig
    /// The group it is in ("" = the default group).
    var group: String = ""
    var hubID: String?
    /// What the hub calls itself (`hub_name`).
    var hubName: String?
    /// This device's own token id there (`POST /auth/tokens`); nil while it still uses the pairing token.
    var deviceID: String?
    /// The watch's answer-only credential there.
    var watch: ServerConfig?
    /// The watch's own issued token id, used only to report its complication configurations.
    var watchDeviceID: String? = nil
    /// The hubs its last hello listed in its group (itself included).
    var mesh: [MeshHub]?
    var addedAt: Date = .now
    /// Its SSH tunnel, when the user set one up (docs/operations/network.md「Connecting to hub through SSH」).
    var ssh: SSHRoute?
    /// Its in-app tunnel, when the hub has one (docs/operations/network.md「Connecting to hub through an in-app tunnel」).
    var inAppTunnel: InAppTunnelRoute?
    /// Its local SOCKS5 proxy, when the user set one up (docs/operations/network.md "Connecting to hub through a local
    /// SOCKS5 proxy").
    var localProxy: LocalProxyRoute?
    /// SHA-256 (hex) of the pairing token `deviceID`'s token was traded for: the same pairing link handled again (scanned
    /// twice, opened from the camera too) keeps the device's token instead of going back to one the hub drops (#2766).
    var tradedPairing: String?
    /// Other addresses of the same hub the user added (Settings › hub › Addresses, docs/operations/network.md
    /// "Client-added candidates"), in the order to try after the paired one. nil in pairings saved before them.
    var addresses: [HubAddress]?

    init(id: String = PairedHub.newID(), config: ServerConfig, group: String = "", hubID: String? = nil, hubName: String? = nil,
         deviceID: String? = nil, watch: ServerConfig? = nil, mesh: [MeshHub]? = nil, addedAt: Date = .now) {
        self.id = id
        self.config = config
        self.group = group
        self.hubID = hubID
        self.hubName = hubName
        self.deviceID = deviceID
        self.watch = watch
        self.mesh = mesh
        self.addedAt = addedAt
    }

    static func newID() -> String { "h" + String(UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(10)).lowercased() }

    /// The hub's name, else its host.
    var displayName: String {
        if let n = hubName?.trimmingCharacters(in: .whitespacesAndNewlines), !n.isEmpty { return n }
        return config.url.host() ?? config.url.absoluteString
    }

    /// Whether this hub's hello lists `other` in its group.
    func lists(_ other: PairedHub) -> Bool {
        guard let id = other.hubID, !id.isEmpty, id != hubID else { return false }
        return mesh?.contains { $0.hubID == id } == true
    }

    /// Every address of the hub in the order to try: the paired one, then the added ones, each with this device's
    /// credential and only its own Cloudflare Access token.
    var configs: [ServerConfig] {
        [config] + (addresses ?? []).map { ServerConfig(url: $0.url, token: config.token, access: $0.access) }
    }
}

/// An address the user added for a paired hub, with the Cloudflare Access service token for that address, if any.
nonisolated struct HubAddress: Codable, Hashable, Sendable {
    var url: URL
    var access: HubAccess?
}

/// What Settings › hub › Addresses makes of the text typed (docs/operations/network.md "Client-added candidates"). Text
/// without a scheme gets `https://` in front, a bare IP or `localhost` too: `http://` only when typed.
nonisolated enum HubAddressInput: Equatable, Sendable {
    /// Nothing typed.
    case empty
    /// The address that would be added.
    case ready(URL)
    /// A scheme other than http or https, such as `ftp://`.
    case unsupportedScheme
    /// Not an address, even with `https://` in front.
    case invalid
    /// The hub has this address already, paired or added.
    case duplicate(URL)

    /// [existing]: the hub's addresses so far, the paired one included.
    init(_ text: String, existing: [URL]) {
        let typed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !typed.isEmpty else { self = .empty; return }
        guard !typed.contains(where: \.isWhitespace) else { self = .invalid; return }
        var full = typed.contains("://") ? typed : "https://" + typed
        if full.hasSuffix("/") { full.removeLast() }
        guard let parts = URLComponents(string: full), let scheme = parts.scheme?.lowercased() else { self = .invalid; return }
        guard scheme == "http" || scheme == "https" else { self = .unsupportedScheme; return }
        guard parts.host?.isEmpty == false, parts.port.map({ (1...65535).contains($0) }) ?? true,
              let url = URL(string: full), let key = Self.key(url) else { self = .invalid; return }
        self = existing.contains { Self.key($0) == key } ? .duplicate(url) : .ready(url)
    }

    /// The same address however it is written: scheme and host in any case, the default port written or not, a
    /// trailing `/` or none.
    static func key(_ url: URL) -> String? {
        guard let parts = URLComponents(url: url, resolvingAgainstBaseURL: false), let scheme = parts.scheme?.lowercased(),
              let host = parts.host?.lowercased(), !host.isEmpty else { return nil }
        let port = parts.port ?? (scheme == "https" ? 443 : 80)
        var path = parts.percentEncodedPath
        while path.hasSuffix("/") { path.removeLast() }
        return "\(scheme)://\(host):\(port)\(path)"
    }
}

/// Every hub paired, in the order they were paired, and the names the user gave the groups. Kept in the keychain
/// (the credentials are in it).
nonisolated struct HubRegistry: Codable, Hashable, Sendable {
    var hubs: [PairedHub] = []
    /// group id → the name the user gave it (Settings › hub › Rename).
    var groupNames: [String: String] = [:]

    /// The groups, in the order their first hub was paired; the default group first when there is one.
    var groups: [String] {
        var seen = Set<String>(), out: [String] = []
        for h in hubs where seen.insert(h.group).inserted { out.append(h.group) }
        if let i = out.firstIndex(of: ""), i > 0 { out.insert(out.remove(at: i), at: 0) }
        return out
    }

    func members(_ group: String) -> [PairedHub] { hubs.filter { $0.group == group } }

    /// The hub of a group whose credential the app uses first: the first one paired.
    func primary(_ group: String) -> PairedHub? { hubs.first { $0.group == group } }

    /// What the watch may use (#1185): each group's answer-only watch credential at its first hub, and the groups that
    /// have none yet. Never this device's own credential in its place: that one opens everything.
    func watchCredentials() -> (configs: [String: ServerConfig], missing: [String]) {
        var configs: [String: ServerConfig] = [:], missing: [String] = []
        for g in groups {
            guard let hub = primary(g), var w = hub.watch else { missing.append(g); continue }
            // The hub's Cloudflare Access token as it is now (set or cleared since the watch's token was issued), for the
            // same hub's address only.
            w.access = CloudflareAccess.sameOrigin(w.url, hub.config.url) ? hub.config.access : nil
            configs[g] = w
        }
        return (configs, missing)
    }

    /// What the group is called: the user's name for it, else its first hub's name.
    func name(_ group: String) -> String {
        if let n = groupNames[group]?.trimmingCharacters(in: .whitespacesAndNewlines), !n.isEmpty { return n }
        return primary(group)?.displayName ?? "hub"
    }

    /// The group a hub (by its hub_id) belongs to: paired itself, or listed in a paired hub's mesh.
    func group(hubID: String) -> String? {
        if let h = hubs.first(where: { $0.hubID == hubID }) { return h.group }
        return hubs.first { $0.mesh?.contains { $0.hubID == hubID } == true }?.group
    }

    /// A fresh group id: the default group's for the first hub paired, else a random one (never "|" or "/": keys use
    /// them). The default group gone, the others stay as they are and no later group takes its place
    /// (docs/archive/operations/plan-hub-mesh.md 4: no new primary group is promoted).
    func newGroupID() -> String {
        if hubs.isEmpty { return "" }
        let used = Set(hubs.map(\.group))
        while true {
            let id = "g" + String(UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(8)).lowercased()
            if !used.contains(id) { return id }
        }
    }
}

/// Which paired hubs belong together (docs/protocol/multi-hub.md「Grouping」). Pure, so the tests drive it.
nonisolated enum HubGrouping {
    /// Groups the hubs by what their hellos list: two paired hubs are in one group when either lists the other, and
    /// so is everything connected to them. Group ids stay what they were: a merged group keeps the id of its
    /// earliest-paired hub's group; a split keeps the id with the part holding its earliest hub, the other parts get
    /// another member's old id that is free, else a new one. Order is kept.
    static func regroup(_ registry: HubRegistry) -> HubRegistry {
        let hubs = registry.hubs
        guard hubs.count > 1 else { return registry }
        // Union-find over "lists" in either direction.
        var parent = Array(hubs.indices)
        func find(_ i: Int) -> Int {
            var i = i
            while parent[i] != i { parent[i] = parent[parent[i]]; i = parent[i] }
            return i
        }
        for i in hubs.indices {
            for j in hubs.indices where j > i && (hubs[i].lists(hubs[j]) || hubs[j].lists(hubs[i]) || sameHub(hubs[i], hubs[j])) {
                let a = find(i), b = find(j)
                if a != b { parent[max(a, b)] = min(a, b) }
            }
        }
        // Components in the order of their earliest hub (the root is the smallest index).
        var components: [Int: [Int]] = [:]
        for i in hubs.indices { components[find(i), default: []].append(i) }
        let roots = components.keys.sorted()
        var out = registry
        var claimed = Set<String>()
        var pending: [Int] = []
        for root in roots {
            let wanted = hubs[root].group
            if claimed.insert(wanted).inserted {
                for i in components[root]! { out.hubs[i].group = wanted }
            } else {
                pending.append(root)
            }
        }
        // Split-off parts: one of their own old ids that is still free, else a new one.
        for root in pending {
            let members = components[root]!
            let free = members.map { hubs[$0].group }.first { !claimed.contains($0) }
            let id = free ?? freshID(avoiding: claimed.union(hubs.map(\.group)))
            claimed.insert(id)
            for i in members { out.hubs[i].group = id }
        }
        // Names of groups that no longer exist go with them.
        let groups = Set(out.hubs.map(\.group))
        out.groupNames = out.groupNames.filter { groups.contains($0.key) }
        return out
    }

    /// Two pairings of the same hub (paired twice): one group.
    private static func sameHub(_ a: PairedHub, _ b: PairedHub) -> Bool {
        guard let x = a.hubID, !x.isEmpty else { return false }
        return x == b.hubID
    }

    private static func freshID(avoiding used: Set<String>) -> String {
        while true {
            let id = "g" + String(UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(8)).lowercased()
            if !used.contains(id) { return id }
        }
    }
}

/// Where a group's connection may go (docs/protocol/multi-hub.md「Connections and routing」): every paired hub of the group with its own
/// credential, then the addresses the group's hellos list for hubs not paired here, with the group's first credential
/// (credentials work on every hub of a group, docs/archive/operations/plan-hub-mesh.md 2.6). The group's center comes first
/// (docs/archive/operations/plan-hub-center.md 5): a member passes everything on to it, one hop more. A paired hub's
/// added addresses follow its paired one (#4670).
nonisolated enum HubRoute {
    struct Candidate: Hashable, Sendable {
        var config: ServerConfig
        /// The paired hub it is, nil for an address only the mesh gave.
        var hub: String?
        /// It is the group's center (`mesh.hubs[].center`).
        var center = false
        /// Its place among its hub's addresses (`PairedHub.configs`): 0 the paired one, then the added ones in order.
        var address = 0
        /// The `hub_id` an added address must answer as (the paired hub's); nil: not checked.
        var hubID: String?

        var url: URL { config.url }
        /// Lower is preferred: the center, then a hub's earlier addresses.
        var rank: (Int, Int) { (center ? 0 : 1, address) }
    }

    /// The group's center as its hellos say (nil: a hub before the single center, or no mesh).
    static func centerID(_ registry: HubRegistry, group: String) -> String? {
        registry.members(group).lazy.compactMap { $0.mesh?.first(where: { $0.center == true })?.hubID }.first
    }

    static func candidates(_ registry: HubRegistry, group: String) -> [Candidate] {
        let members = registry.members(group)
        guard let primary = members.first else { return [] }
        let center = centerID(registry, group: group)
        var out: [Candidate] = []
        var seen = Set<String>()
        func add(_ c: Candidate) {
            let key = c.config.url.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            if seen.insert(key).inserted { out.append(c) }
        }
        for h in members {
            for (i, config) in h.configs.enumerated() {
                add(Candidate(config: config, hub: h.id, center: center != nil && h.hubID == center, address: i, hubID: i > 0 ? h.hubID : nil))
            }
        }
        let paired = Set(members.compactMap(\.hubID))
        for h in members {
            for m in h.mesh ?? [] where !paired.contains(m.hubID) && m.online != false {
                for e in m.endpoints ?? [] {
                    guard let url = URL(string: e), url.scheme == "http" || url.scheme == "https" else { continue }
                    add(Candidate(config: ServerConfig(url: url, token: primary.config.token), hub: nil, center: m.hubID == center))
                }
            }
        }
        // The center first (also the one taken when nobody answers); otherwise the order above.
        return out.filter(\.center) + out.filter { !$0.center }
    }

    typealias Probe = @Sendable (Candidate) async -> Bool
    /// A probe that says what the try came to (#11390), not only whether the candidate passed.
    typealias Check = @Sendable (Candidate) async -> HubOutcome

    /// One candidate's part in an attempt: what its probe came to and when (`at`: when the attempt learned it, for a
    /// probe the picker cut short the moment it stopped waiting).
    struct Tried: Hashable, Sendable {
        var candidate: Candidate
        var outcome: HubOutcome
        var at: Date
    }

    /// How long a candidate that answered first waits for a preferred one (the center, a hub's earlier address) to
    /// answer too.
    static let centerGrace: Duration = .milliseconds(1500)
    /// How often a socket on a candidate behind a preferred one asks whether that one answers again (AppModel.watchCenter).
    static let centerRecheck: Duration = .seconds(300)

    /// Asks every candidate at once (GET /api/v1/health, a few seconds at most) and takes the best-ranked that answers
    /// (`Candidate.rank`: the center, then a hub's earlier addresses), the quickest among equals: one that answers is
    /// taken once every better-ranked probe failed, or once `grace` went by since the first answer. None answering: the
    /// first (the center when there is one), for the socket's own retries. One candidate: no probe.
    static func pick(_ candidates: [Candidate], grace: Duration = centerGrace, probe: @escaping Probe = healthProbe) async -> Candidate? {
        guard candidates.count > 1 else { return candidates.first }
        return await answering(candidates, grace: grace, probe: probe) ?? candidates.first
    }

    /// `pick`, but nil when none answers, and a single candidate is asked too: whether the group can be reached
    /// directly at all (its SSH tunnel is the way when not, docs/operations/network.md「Connecting to hub through SSH」).
    static func answering(_ candidates: [Candidate], grace: Duration = centerGrace, probe: @escaping Probe = healthProbe) async -> Candidate? {
        await race(candidates, grace: grace, check: { await probe($0) ? .answers : .unreachable }).winner
    }

    /// `pick` with what each candidate's probe came to: the same choice, at the same moment.
    static func pickTried(_ candidates: [Candidate], grace: Duration = centerGrace,
                          check: @escaping Check = healthCheck) async -> (picked: Candidate?, tried: [Tried]) {
        guard candidates.count > 1 else { return (candidates.first, []) }
        let r = await race(candidates, grace: grace, check: check)
        return (r.winner ?? candidates.first, r.tried)
    }

    /// `answering` with what each candidate's probe came to.
    static func answeringTried(_ candidates: [Candidate], grace: Duration = centerGrace,
                               check: @escaping Check = healthCheck) async -> (winner: Candidate?, tried: [Tried]) {
        let r = await race(candidates, grace: grace, check: check)
        return (r.winner, r.tried)
    }

    /// The race `pick` and `answering` run: every candidate's probe at once, the best-ranked that answers taken as
    /// `pick` says. Probes still running when it is decided are cancelled and count as unknown (#11390).
    private static func race(_ candidates: [Candidate], grace: Duration, check: @escaping Check) async -> (winner: Candidate?, tried: [Tried]) {
        guard !candidates.isEmpty else { return (nil, []) }
        enum Answer: Sendable { case done(Int, HubOutcome), graceOver }
        return await withTaskGroup(of: Answer.self) { group in
            for (i, c) in candidates.enumerated() { group.addTask { .done(i, await check(c)) } }
            var pending = Set(candidates.indices)
            var best: Candidate?
            var got: [Int: (HubOutcome, Date)] = [:]
            func decided() -> (winner: Candidate?, tried: [Tried]) {
                let now = Date()
                let tried = candidates.indices.map { i in
                    Tried(candidate: candidates[i], outcome: got[i]?.0 ?? .unknown, at: got[i]?.1 ?? now)
                }
                return (best, settled(tried, among: candidates))
            }
            for await answer in group {
                switch answer {
                case .done(let i, let outcome):
                    pending.remove(i)
                    got[i] = (outcome, Date())
                    if outcome == .answers {
                        if best == nil { group.addTask { try? await Task.sleep(for: grace); return .graceOver } }
                        if best.map({ candidates[i].rank < $0.rank }) ?? true { best = candidates[i] }
                    }
                case .graceOver:
                    group.cancelAll()
                    return decided()
                }
                if let best, !pending.contains(where: { candidates[$0].rank < best.rank }) {
                    group.cancelAll()
                    return decided()
                }
            }
            return decided()
        }
    }

    /// Another hub answering a private address (a LAN IPv4 address, ULA, link-local, `.local`) says only that this
    /// network has another device there, not that someone else answers for the hub: couldn't connect, unless every
    /// address the attempt tries (`among`) is private, when nothing else could say more (as Android's
    /// `HubRouteSelector.worst`).
    static func settled(_ tried: [Tried], among candidates: [Candidate]) -> [Tried] {
        let onlyPrivate = candidates.allSatisfy { HubAddressScope.isPrivate($0.url) }
        return tried.map { t in
            guard t.outcome == .notYourHub, !onlyPrivate, HubAddressScope.isPrivate(t.candidate.url) else { return t }
            var t = t
            t.outcome = .unreachable
            return t
        }
    }

    /// The same address however the candidates list writes it (their own dedupe key): its URL without a trailing `/`.
    static func key(_ url: URL) -> String {
        url.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    }

    /// Whether the hub answers at its own address (never through its SSH tunnel: this is how the app finds out it
    /// does not need it). An added address must answer as the paired hub (docs/operations/network.md "Client-added
    /// candidates"); the answer is only evidence the address reaches it, never a source of trust.
    static let healthProbe: Probe = { await healthCheck($0) == .answers }

    /// `healthProbe` with why it failed (#11390): passes (`answers`) exactly when `healthProbe` does.
    static let healthCheck: Check = { candidate in
        let config = candidate.config
        var req = URLRequest(url: APIClient(config: config).url("/api/v1/health", direct: true), timeoutInterval: 3)
        req.httpMethod = "GET"
        RequestClientMetadata.current.apply(to: &req)
        CloudflareAccess.apply(config.access, to: &req, hub: config.url) // Access in front would answer with its login
        let data: Data, resp: URLResponse
        do {
            (data, resp) = try await URLSession.shared.data(for: req, delegate: HubRedirects.shared)
        } catch {
            return HubOutcome(failure: error)
        }
        guard (resp as? HTTPURLResponse)?.statusCode == 200,
              let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return .unreachable }
        // Known hub_id: the answer must carry the same one (an answer without it could be any server, or an older hub).
        if let want = candidate.hubID, !want.isEmpty, obj["hub_id"] as? String != want {
            if let other = obj["hub_id"] as? String, !other.isEmpty { return .notYourHub }
            return .unreachable
        }
        return obj["ok"] as? Bool == true ? .answers : .unreachable
    }
}

/// The paired hubs, kept in the keychain. The old single pairing (`Keychain.legacyServerAccount`, the device id in
/// UserDefaults, the watch's credential) moves into the default group the first time it is read.
///
/// Losing the pairing is the thing to avoid: a read that fails (the keychain before the first unlock after boot) is
/// "unknown", not "none", and nothing is written over an unknown registry (`Loaded.unreadable`; AppModel keeps it from
/// saving); the old items are dropped only once the registry is written and reads back the same.
nonisolated enum HubStore {
    private static let account = "hubs"
    static let legacyDeviceIDKey = "auth.deviceID"

    /// What reading the registry came to.
    enum Loaded: Equatable, Sendable {
        /// Read (an empty registry: nothing paired, for sure).
        case registry(HubRegistry)
        /// The keychain could not be read, or held something this build cannot decode: unknown, not unpaired.
        case unreadable
    }

    static func read(_ kc: KeychainAccess = .system, defaults: UserDefaults = .standard) -> Loaded {
        switch kc.read(account) {
        case .found(let data):
            guard let r = try? JSONDecoder().decode(HubRegistry.self, from: data) else { return .unreadable }
            return .registry(r)
        case .failed:
            return .unreadable
        case .missing:
            return migrated(kc, defaults: defaults)
        }
    }

    /// For callers that only read (intents, notification and Live Activity buttons, push registration): unknown
    /// reads as nothing paired. Never save what this returns.
    static func load(_ kc: KeychainAccess = .system) -> HubRegistry {
        if case .registry(let r) = read(kc) { return r }
        return HubRegistry()
    }

    /// Writes the registry and checks it reads back; true when it is there. An empty one deletes the item (and the
    /// old single pairing with it, so it does not come back as a migration): only for a registry known to be empty
    /// (unpairing), never for one that could not be read. Once a registry is there, the old items go too.
    @discardableResult
    static func save(_ registry: HubRegistry, _ kc: KeychainAccess = .system, defaults: UserDefaults = .standard) -> Bool {
        if registry.hubs.isEmpty {
            kc.delete(account)
            dropLegacy(kc, defaults: defaults)
            return true
        }
        guard let data = try? JSONEncoder().encode(registry), kc.write(data, account),
              case .found(let back) = kc.read(account), back == data else { return false }
        dropLegacy(kc, defaults: defaults)
        return true
    }

    /// What the app saves through: nothing at all while the registry it would replace could not be read (`known`
    /// false) — not even an empty one's delete.
    @discardableResult
    static func save(_ registry: HubRegistry, known: Bool, _ kc: KeychainAccess = .system, defaults: UserDefaults = .standard) -> Bool {
        guard known else { return false }
        return save(registry, kc, defaults: defaults)
    }

    /// The old pairing as the default group's only hub. Its old items are dropped only once the registry is written
    /// and reads back; otherwise they stay, the registry is returned from memory, and the next launch moves them again.
    private static func migrated(_ kc: KeychainAccess, defaults: UserDefaults) -> Loaded {
        let legacy: ServerConfig
        switch kc.read(Keychain.legacyServerAccount) {
        case .missing: return .registry(HubRegistry())
        case .failed: return .unreadable
        case .found(let data):
            guard let c = try? JSONDecoder().decode(ServerConfig.self, from: data) else { return .registry(HubRegistry()) }
            legacy = c
        }
        var watch: ServerConfig?
        switch kc.read(Keychain.legacyWatchAccount) {
        case .found(let data): watch = try? JSONDecoder().decode(ServerConfig.self, from: data)
        case .missing: watch = nil
        case .failed: return .unreadable // not now: the watch's credential would be lost
        }
        let hub = PairedHub(config: legacy, group: "", deviceID: defaults.string(forKey: legacyDeviceIDKey), watch: watch,
                            addedAt: .distantPast)
        let r = HubRegistry(hubs: [hub])
        save(r, kc, defaults: defaults) // drops the old items only when it stuck
        return .registry(r)
    }

    private static func dropLegacy(_ kc: KeychainAccess, defaults: UserDefaults) {
        kc.delete(Keychain.legacyServerAccount)
        kc.delete(Keychain.legacyWatchAccount)
        defaults.removeObject(forKey: legacyDeviceIDKey)
    }

    /// The credential to use for a group outside the app's connection (intents, notification buttons): its first hub's.
    static func config(group: String) -> ServerConfig? { load().primary(group)?.config }

    /// The group a push came from: the hub that sent it (`hub_id`), else nil.
    static func group(hubID: String?) -> String? {
        guard let hubID, !hubID.isEmpty else { return nil }
        return load().group(hubID: hubID)
    }

    static var isPaired: Bool { !load().hubs.isEmpty }

    /// A client for a group outside the app's connection (a notification button, the Live Activity's): its first hub
    /// (credentials work on every hub of a group); the first group when it is not known or gone.
    static func client(group: String?) throws -> APIClient {
        let registry = load()
        guard let g = group.flatMap({ registry.primary($0) != nil ? $0 : nil }) ?? registry.groups.first,
              let config = registry.primary(g)?.config else { throw NotPaired() }
        return APIClient(config: config, group: g)
    }

    /// No hub paired (what a notification or Live Activity button says when it has nothing to talk to).
    struct NotPaired: LocalizedError, Sendable {
        var errorDescription: String? { String(localized: "Not paired yet. Open Persona and connect to a hub first.") }
    }
}
