import Foundation
import PersonaWire

/// A way through to a paired hub when its own addresses do not answer: its SSH tunnel
/// (docs/operations/network.md「Connecting to hub through SSH」), its local SOCKS5 proxy ("Connecting to hub through a local SOCKS5
/// proxy") or, in builds that have the in-app tunnel core for a hub whose extension contributes one, its in-app tunnel
/// (「经 App 内隧道连 hub」). A hub may have several; each is tried in turn, in that order.
struct HubTunnel: Hashable {
    enum Kind: Hashable {
        case ssh
        case localProxy
        case inApp
    }

    let hub: PairedHub
    let kind: Kind
    /// The in-app route whose core dials the SSH server, for an SSH tunnel set to go through it.
    var carrier: InAppTunnelRoute?

    /// Through it every time, not only when the hub's own addresses do not answer.
    var always: Bool {
        switch kind {
        case .ssh: hub.ssh?.always == true
        case .localProxy: hub.localProxy?.always == true
        case .inApp: hub.inAppTunnel?.always == true
        }
    }

    /// The group went through it last time (kept while the app was in the background).
    var recent: Bool {
        switch kind {
        case .ssh: SSHTunnels.shared.recent.contains(hub.id)
        case .localProxy: LocalProxies.shared.recent.contains(hub.id)
        case .inApp: InAppTunnels.shared.recent.contains(hub.id)
        }
    }

    /// Up and routing the hub's requests: true; it could not be made: false.
    func open(owner: UUID? = nil) async -> Bool {
        switch kind {
        case .ssh:
            guard let route = hub.ssh else { return false }
            return (try? await SSHTunnels.shared.open(hub.id, route: route, hub: hub.config.url, carrier: carrier, owner: owner)) != nil
        case .localProxy:
            guard let route = hub.localProxy else { return false }
            return (try? await LocalProxies.shared.open(hub.id, route: route, hub: hub.config.url)) != nil
        case .inApp:
            guard let route = hub.inAppTunnel else { return false }
            return (try? await InAppTunnels.shared.open(hub.id, route: route, hub: hub.config.url)) != nil
        }
    }

    func close() {
        switch kind {
        case .ssh: SSHTunnels.shared.close(hub.id)
        case .localProxy: LocalProxies.shared.close(hub.id)
        case .inApp: InAppTunnels.shared.close(hub.id)
        }
    }

    func watchDirect(answers: @escaping @MainActor () async -> Bool, moveBack: @escaping @MainActor () -> Void) {
        switch kind {
        case .ssh: SSHTunnels.shared.watchDirect(hub.id, answers: answers, moveBack: moveBack)
        case .localProxy: LocalProxies.shared.watchDirect(hub.id, answers: answers, moveBack: moveBack)
        case .inApp: InAppTunnels.shared.watchDirect(hub.id, answers: answers, moveBack: moveBack)
        }
    }
}

/// Buttons pressed outside the app still talking to their hubs (`AppModel.outside`), by group: the last one of a group
/// to finish closes the tunnels they opened, and the app going to the background leaves them to it.
@MainActor private var outsideRuns: [String: Int] = [:]

/// Only recovery picks: a newer pick closes its predecessor's SSH attempt before it can join that attempt.
@MainActor private var tunnelRecoveries: [String: HubTunnelRecovery] = [:]

/// Restores a previously used SSH route without waiting for a direct address that no longer answers. Both waits
/// must honor cancellation; the SSH owner closes only the attempt this pick created, fencing late completions.
@MainActor final class HubTunnelRecovery {
    enum Result: Sendable {
        case direct(HubRoute.Candidate)
        case tunnel
        case unavailable
        case cancelled
    }

    private enum Answer: Sendable {
        case direct(HubRoute.Candidate?)
        case tunnel(Bool)
    }

    private var closeTunnel: (() -> Void)?
    private(set) var cancelled = false

    init(closeTunnel: @escaping () -> Void) {
        self.closeTunnel = closeTunnel
    }

    func cancel() {
        cancelled = true
        close()
    }

    /// Ownership passes to the selected connection once both race waiters have ended.
    func keepTunnel() { closeTunnel = nil }

    private func close() {
        let action = closeTunnel
        closeTunnel = nil
        action?()
    }

    func pick(direct: @escaping @MainActor () async -> HubRoute.Candidate?,
              tunnel: @escaping @MainActor () async -> Bool) async -> Result {
        guard !cancelled, !Task.isCancelled else { cancel(); return .cancelled }
        return await withTaskGroup(of: Answer.self) { tasks in
            tasks.addTask { .direct(await direct()) }
            tasks.addTask { .tunnel(await tunnel()) }
            defer { tasks.cancelAll() }
            for await answer in tasks {
                guard !self.cancelled, !Task.isCancelled else { self.cancel(); return .cancelled }
                switch answer {
                case .direct(let candidate?):
                    self.close()
                    return .direct(candidate)
                case .tunnel(true):
                    return .tunnel
                case .direct(nil), .tunnel(false):
                    continue
                }
            }
            self.close()
            return .unavailable
        }
    }
}

/// Which of two racing tasks answered first.
@MainActor private final class Once {
    var done = false
}

/// What the restore race's direct probes came to (`AppModel.restoreRace`).
@MainActor private final class Asked {
    var tried: [HubRoute.Tried] = []
}

extension AppModel {
    // MARK: Tunnels to the hub (docs/operations/network.md「Connecting to hub through SSH」「经 App 内隧道连 hub」)

    /// The group's tunnels ready to use, each paired hub's SSH one, then its local proxy, then its in-app one.
    func hubTunnels(_ group: String) -> [HubTunnel] {
        registry.members(group).flatMap { h -> [HubTunnel] in
            var out: [HubTunnel] = []
            if let ssh = h.ssh, ssh.isUsable {
                // Through the in-app tunnel: only with a core and a hub's in-app route to carry it.
                let carrier = ssh.viaInAppTunnel ? inAppCarrier(for: h) : nil
                if !ssh.viaInAppTunnel || carrier != nil { out.append(HubTunnel(hub: h, kind: .ssh, carrier: carrier)) }
            }
            if h.localProxy?.isUsable == true { out.append(HubTunnel(hub: h, kind: .localProxy)) }
            if InAppTunnel.available, h.inAppTunnel?.isUsable == true { out.append(HubTunnel(hub: h, kind: .inApp)) }
            return out
        }
    }

    /// The in-app route an SSH tunnel set to go through the in-app tunnel uses (its sign-in, its probe): the hub's own,
    /// else another paired hub's (the core and its sign-in are one per device). Nil in a build without the core.
    func inAppCarrier(for hub: PairedHub) -> InAppTunnelRoute? {
        #if DEBUG
        // `-showSSHViaTunnel YES`: a stand-in route so the SSH page shows its switch in builds without the core (screenshots).
        if UserDefaults.standard.bool(forKey: "showSSHViaTunnel") { return InAppTunnelRoute(targetHost: "hub.internal.example") }
        #endif
        guard InAppTunnel.available else { return nil }
        if let own = registry.hubs.first(where: { $0.id == hub.id })?.inAppTunnel, own.isUsable { return own }
        return registry.hubs.lazy.compactMap(\.inAppTunnel).first(where: \.isUsable)
    }

    /// Where a group's socket goes next when it has a tunnel, step by step as `HubWay.next` says: straight to the hub
    /// when one of its addresses answers (the tunnels close), else through a tunnel; tunnels set to be used every time
    /// first. Restoring the last SSH session races the direct addresses, so an unreachable address cannot delay an
    /// already-restored tunnel. Nothing answering at all: the first address, for the socket's own retries.
    func tunnelRoute(_ group: String) async -> ServerConfig? {
        guard !Task.isCancelled else { return nil }
        tunnelRecoveries.removeValue(forKey: group)?.cancel()
        let attemptStart = Date() // what its direct check learns after the app went to the background was cut short
        let candidates = HubRoute.candidates(registry, group: group)
        let tunnels = hubTunnels(group)
        let options = tunnels.map {
            HubWay.Option(always: $0.always, recent: $0.recent,
                          recovering: $0.kind == .ssh && !SSHTunnels.shared.routed.contains($0.hub.id))
        }
        var seen = HubWay.Seen()
        var direct: HubRoute.Candidate?
        while true {
            guard !Task.isCancelled else { return nil }
            switch HubWay.next(options, seen) {
            case .open(let i):
                seen.opened[i] = await tunnels[i].open()
            case .restore(let i):
                let t = tunnels[i]
                let owner = UUID()
                let recovery = HubTunnelRecovery { SSHTunnels.shared.close(t.hub.id, openedBy: owner) }
                tunnelRecoveries[group] = recovery
                let result = await restoreRace(group, recovery, candidates, since: attemptStart) { await t.open(owner: owner) }
                if tunnelRecoveries[group] === recovery { tunnelRecoveries[group] = nil }
                guard !Task.isCancelled, !recovery.cancelled else { recovery.cancel(); return nil }
                switch result {
                case .direct(let answer):
                    direct = answer
                    seen.direct = true
                case .tunnel:
                    recovery.keepTunnel()
                    return through(only(t, of: tunnels), group)
                case .unavailable:
                    seen.direct = false
                    seen.opened[i] = false
                case .cancelled:
                    return nil
                }
            case .askDirect(let warm):
                // Through a tunnel last time (back from the background, a dropped socket): it starts while the direct
                // way is asked, so an address that does not answer costs no wait (up to the probe's 3 s) on top.
                let warming: Task<[Int: Bool], Never>? = warm.isEmpty ? nil : Task { @MainActor in
                    var tried: [Int: Bool] = [:]
                    for i in warm {
                        guard !Task.isCancelled else { break }
                        tried[i] = await tunnels[i].open()
                        if tried[i] == true { break }
                    }
                    return tried
                }
                defer { warming?.cancel() }
                let asked = await HubRoute.answeringTried(candidates, check: routeCheck)
                attemptTried(group, asked.tried, since: attemptStart)
                guard !Task.isCancelled else { return nil }
                direct = asked.winner
                seen.direct = direct != nil
                if let warming {
                    let tried = await withTaskCancellationHandler {
                        await warming.value
                    } onCancel: {
                        warming.cancel()
                    }
                    seen.opened.merge(tried) { $1 }
                }
            case .direct:
                for t in tunnels { t.close() }
                return direct?.config
            case .through(let i, let watchDirect):
                let t = only(tunnels[i], of: tunnels)
                return watchDirect ? through(t, group) : t.hub.config
            case .none:
                return candidates.first?.config
            }
        }
    }

    /// The `.restore` step's race (#11683): a closed SSH tunnel used last time against the group's own addresses, with
    /// what each address's probe came to on the lines and in the tally, however the race ends (#11390): a direct
    /// address won, the tunnel did, nothing answered, or the pick was cancelled (probes cut short are Unknown). Only a
    /// direct winner hands the socket the addresses that answered: through the tunnel, the socket's failure is the
    /// tunnel's, and a cancelled pick's answers are not the next attempt's. `.cancelled` when cancelled meanwhile.
    func restoreRace(_ group: String, _ recovery: HubTunnelRecovery, _ candidates: [HubRoute.Candidate], since attemptStart: Date,
                     tunnel: @escaping @MainActor () async -> Bool) async -> HubTunnelRecovery.Result {
        let check = routeCheck, asked = Asked()
        // `pick` waits for its direct side, cancelled or not, before it returns: `asked` is complete by then.
        let result = await recovery.pick(direct: {
            let r = await HubRoute.answeringTried(candidates, check: check)
            asked.tried = r.tried
            return r.winner
        }, tunnel: tunnel)
        tried(group, asked.tried, since: attemptStart)
        guard !Task.isCancelled, !recovery.cancelled else { return .cancelled }
        if case .direct = result { attemptAnswered(group, asked.tried, since: attemptStart) }
        return result
    }

    /// `t` carries its hub's traffic: the hub's other tunnel closes (both route the same hub address in HubTransport, so
    /// the one left open would later clear the route of the one in use).
    private func only(_ t: HubTunnel, of tunnels: [HubTunnel]) -> HubTunnel {
        for o in tunnels where o.hub.id == t.hub.id && o.kind != t.kind { o.close() }
        return t
    }

    /// The group goes through `t`, asking the direct way again every so often.
    private func through(_ t: HubTunnel, _ group: String) -> ServerConfig {
        t.watchDirect(answers: { [weak self] in
            guard let self else { return false }
            let start = Date()
            let asked = await HubRoute.answeringTried(HubRoute.candidates(self.registry, group: group), check: self.routeCheck)
            self.tried(group, asked.tried, since: start)
            return asked.winner != nil
        }, moveBack: { [weak self] in self?.link(group)?.live.reroute() })
        return t.hub.config
    }

    /// How long a button pressed outside the app may take to find its way, leaving the request and the failure
    /// notification room in the ~30 s iOS gives it.
    static let outsideWayLimit: Duration = .seconds(20)

    /// A button pressed outside the app (a notification's, the Live Activity's) for a group with tunnels (#3142). Its
    /// socket on its way: the request goes to the hub the socket is on, routed already. Otherwise (closed for the
    /// background with its tunnels, `suspendLinks`; just woken by a silent push, not there yet) a hub reached only
    /// through a tunnel would not answer at its own address, so the group's way is picked as its socket would pick it
    /// and the request goes there (the hub the tunnel reaches, not necessarily the group's first). Not found within
    /// `outsideWayLimit`: timed out. The tunnels close again once the group's last such button is done, if its socket
    /// is still closed (`recent` kept, so back in front they start while the direct way is asked).
    @discardableResult
    func outside<T: Sendable>(_ client: APIClient, _ work: @Sendable (APIClient) async throws -> T) async throws -> T {
        let group = client.group
        // A group with tunnels, or with added addresses (#4670): its paired address alone may not answer.
        let added = registry.members(group).contains { !($0.addresses ?? []).isEmpty }
        guard !hubTunnels(group).isEmpty || added else { return try await work(client) }
        var through = client
        if let live = link(group)?.live, live.state == .connected, let config = live.currentConfig {
            through.config = config
            return try await work(through)
        }
        outsideRuns[group, default: 0] += 1
        defer {
            outsideRuns[group, default: 1] -= 1
            if outsideRuns[group] == 0 {
                outsideRuns[group] = nil
                // Not those a call to Persona still goes through (#9290): closing them would cut its voice.
                if link(group)?.live.suspended != false, !callKeepsTunnels(group) {
                    for h in registry.members(group) { closeTunnels(h.id, forget: false) }
                }
            }
        }
        // ponytail: a way still being found after the limit carries on by itself; its tunnel closes when the app next
        // goes to the background, or stays as the socket's way when the app comes to the front.
        let once = Once()
        let config: ServerConfig? = try await withCheckedThrowingContinuation { c in
            Task { @MainActor in
                do {
                    // A notification action can arrive while foreground recovery already owns the group's route.
                    // Join it; a second route pick would cancel that recovery and release its reads too early.
                    let config: ServerConfig?
                    if let live = self.link(group)?.live, live.state == .connecting || live.state == .connected {
                        config = try await live.waitForRoute()
                    } else {
                        config = await self.tunnelRoute(group)
                    }
                    guard let config else { throw CancellationError() }
                    if !once.done { once.done = true; c.resume(returning: config) }
                } catch {
                    if !once.done { once.done = true; c.resume(throwing: error) }
                }
            }
            Task { @MainActor in
                try? await Task.sleep(for: AppModel.outsideWayLimit)
                if !once.done { once.done = true; c.resume(throwing: URLError(.timedOut)) }
            }
        }
        if let config { through.config = config }
        return try await work(through)
    }

    /// The app went to the background: every tunnel closes (iOS would cut them anyway), but those of a group whose
    /// button is still on its way (`outside`), which closes them itself when it is done, and those of a call to Persona's
    /// group (#9290): the call goes on in the background, and its voice's socket may go through one.
    func closeTunnelsForBackground() {
        var keep = Set(outsideRuns.keys)
        if let group = callTunnelGroup { keep.insert(group) }
        guard !keep.isEmpty else { return closeAllTunnels() }
        for h in registry.hubs where !keep.contains(h.group) { closeTunnels(h.id, forget: false) }
    }

    /// The group a system call is on (#9290), or the phone's voice chat on iOS (#9887 one button: it goes on in the
    /// background too): its voice's socket may go through one of the group's tunnels, so none of them closes for the
    /// background while it is on.
    var callTunnelGroup: String? {
        #if DEBUG
        let calls = voiceCallsForTests ?? .shared
        #else
        let calls = VoiceCalls.shared
        #endif
        guard let call = calls.current, !call.phase.isEnded,
              call.surface == .call || (call.surface == .phone && VoiceCalls.keepsChatAway && call.meeting == nil) else { return nil }
        return call.key.group
    }

    /// Whether a call to Persona keeps `group`'s tunnels open: every background cleanup asks (`closeTunnelsForBackground`,
    /// a button outside the app done, the call's own end).
    func callKeepsTunnels(_ group: String) -> Bool { callTunnelGroup == group }

    /// A call to Persona on `group` ended (#9290). In the background its tunnels close now, as they would have when the
    /// app went there, unless a button outside the app still uses them (it closes them itself) or another call does.
    func callEnded(group: String) {
        guard link(group)?.live.suspended == true, outsideRuns[group] == nil, !callKeepsTunnels(group) else { return }
        for h in registry.members(group) { closeTunnels(h.id, forget: false) }
    }

    /// Every tunnel of every hub closes (the app went to the background, or there is no hub left).
    func closeAllTunnels() {
        SSHTunnels.shared.closeAll()
        LocalProxies.shared.closeAll()
        InAppTunnels.shared.closeAll()
    }

    /// One paired hub's tunnels close (it is removed, or its group is; `forget: false`: the app went to the background).
    func closeTunnels(_ id: String, forget: Bool = true) {
        SSHTunnels.shared.close(id, forget: forget)
        LocalProxies.shared.close(id, forget: forget)
        InAppTunnels.shared.close(id, forget: forget)
    }
}

/// How a group's next connection picks its way when its hubs have tunnels
/// (docs/operations/network.md「Connecting to hub through SSH」): the tunnels set to Always Use first, in order, the first that
/// comes up taken; then the hub's own addresses, while the tunnels used last time start meanwhile; an address
/// answering: straight to the hub; else the first of those to have come up, then the other tunnels in order. A closed
/// SSH tunnel used last time races the direct addresses during recovery only; a live tunnel keeps direct-first
/// selection so the periodic check can move back to the hub's own address. Going
/// through a tunnel only because the direct way did not answer, the direct way is asked again every `recheck`. Pure:
/// `AppModel.tunnelRoute` carries out each step and tells it what came of it, and the tests go through the steps by
/// hand.
nonisolated enum HubWay {
    /// How often a group that went through a tunnel because its hub did not answer asks the direct way again.
    static let recheck: Duration = .seconds(120)

    /// A tunnel as the pick sees it.
    struct Option: Hashable, Sendable {
        /// Set to Always Use.
        var always: Bool
        /// Used the last time (kept while the app was in the background).
        var recent: Bool
        /// Its SSH session was closed, so restoring it may win before the direct probe finishes. An already-routed
        /// tunnel keeps direct-first selection, including the periodic check that moves back to a direct address.
        var recovering = false
    }

    /// What the pick learned so far.
    struct Seen: Equatable, Sendable {
        /// By tunnel index: whether it came up.
        var opened: [Int: Bool] = [:]
        /// Whether one of the hub's own addresses answered; nil before they were asked.
        var direct: Bool?
    }

    enum Step: Equatable, Sendable {
        /// Bring up tunnel `i`.
        case open(Int)
        /// Race a previously used, closed SSH tunnel against the direct addresses; the first success restores service.
        case restore(Int)
        /// Ask the hub's own addresses, bringing up these tunnels meanwhile, in order, until one comes up.
        case askDirect(warming: [Int])
        /// Straight to the hub; every tunnel closes.
        case direct
        /// Through tunnel `i`; `watchDirect`: asking the direct way again every `recheck`.
        case through(Int, watchDirect: Bool)
        /// No way: the first address, for the socket's own retries.
        case none
    }

    static func next(_ options: [Option], _ seen: Seen) -> Step {
        for i in options.indices where options[i].always {
            switch seen.opened[i] {
            case true?: return .through(i, watchDirect: false)
            case nil: return .open(i)
            case false?: continue
            }
        }
        let fallback = options.indices.filter { !options[$0].always }
        let warm = fallback.filter { options[$0].recent }
        if seen.direct == nil, let first = warm.first, options[first].recovering { return .restore(first) }
        guard let direct = seen.direct else { return .askDirect(warming: warm) }
        if direct { return .direct }
        for i in warm + fallback.filter({ !options[$0].recent }) {
            switch seen.opened[i] {
            case true?: return .through(i, watchDirect: true)
            case nil: return .open(i)
            case false?: continue
            }
        }
        return .none
    }
}
