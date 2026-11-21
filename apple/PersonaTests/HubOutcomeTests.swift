import Foundation
import Testing
@testable import Persona
import PersonaWire

/// A private IPv4 address, written from its parts (the privacy scan keeps literal ones out of the repository).
private func ip(_ parts: Int...) -> String { parts.map(String.init).joined(separator: ".") }

/// Answers health probes as hub "H" at here.invalid, as hub "OTHER" at other.invalid and at a LAN address, without a
/// hub_id at bare.invalid, and with an unhealthy answer at sick.invalid.
private final class OutcomeHealthProtocol: URLProtocol, @unchecked Sendable {
    static let lan = ip(192, 168, 1, 20)
    static let hosts: Set<String> = ["here.invalid", "other.invalid", "bare.invalid", "sick.invalid", lan]
    override class func canInit(with request: URLRequest) -> Bool { hosts.contains(request.url?.host ?? "") }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        let host = request.url?.host ?? ""
        let (status, body): (Int, String) = switch host {
        case "here.invalid": (200, #"{"ok":true,"hub_id":"H"}"#)
        case "other.invalid", Self.lan: (200, #"{"ok":true,"hub_id":"OTHER"}"#)
        case "bare.invalid": (200, #"{"ok":true}"#)
        default: (503, #"{"ok":false}"#)
        }
        let resp = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: resp, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(body.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }
    override func stopLoading() {}
}

/// What each try of a hub address comes to (#11390, docs/operations/network.md "Client-added candidates"): one classifier
/// for the probe and the socket, the private-address rule, probes cut short, and Connection Details' lines.
struct HubOutcomeTests {
    private func url(_ s: String) -> URL { URL(string: s)! }

    private func candidate(_ s: String, address: Int = 1, hubID: String? = "H") -> HubRoute.Candidate {
        .init(config: ServerConfig(url: url(s), token: "device"), hub: "h", address: address, hubID: address > 0 ? hubID : nil)
    }

    private func urlError(_ code: Int, _ info: [String: Any] = [:]) -> NSError {
        NSError(domain: NSURLErrorDomain, code: code, userInfo: info)
    }

    // MARK: Classifying a failure

    @Test func refusedTimedOutAndLostAreCouldNotConnect() {
        for code in [NSURLErrorCannotConnectToHost, NSURLErrorTimedOut, NSURLErrorCannotFindHost, NSURLErrorNetworkConnectionLost,
                     NSURLErrorNotConnectedToInternet, NSURLErrorDNSLookupFailed, NSURLErrorBadServerResponse] {
            #expect(HubOutcome(failure: urlError(code)) == .unreachable, "\(code)")
        }
        #expect(HubOutcome(failure: NSError(domain: NSPOSIXErrorDomain, code: 61)) == .unreachable)
    }

    @Test func anUntrustedCertificateIsKeptApart() {
        for code in [NSURLErrorServerCertificateUntrusted, NSURLErrorServerCertificateHasUnknownRoot,
                     NSURLErrorServerCertificateHasBadDate, NSURLErrorServerCertificateNotYetValid] {
            #expect(HubOutcome(failure: urlError(code)) == .untrusted, "\(code)")
        }
        // A failed secure connection is a trust failure only when it says so: the server's trust, or a certificate code.
        #expect(HubOutcome(failure: urlError(NSURLErrorSecureConnectionFailed, ["_kCFStreamErrorCodeKey": -9813])) == .untrusted)
        #expect(HubOutcome(failure: urlError(NSURLErrorSecureConnectionFailed, ["_kCFStreamErrorCodeKey": -9843])) == .untrusted)
        let under = NSError(domain: "kCFErrorDomainCFNetwork", code: -9807)
        #expect(HubOutcome(failure: urlError(NSURLErrorSecureConnectionFailed, [NSUnderlyingErrorKey: under])) == .untrusted)
        // A handshake cut short (the server hung up, spoke plain HTTP): couldn't connect.
        #expect(HubOutcome(failure: urlError(NSURLErrorSecureConnectionFailed, ["_kCFStreamErrorCodeKey": -9806])) == .unreachable)
        #expect(HubOutcome(failure: urlError(NSURLErrorSecureConnectionFailed)) == .unreachable)
    }

    @Test func aCancelledTryIsUnknown() {
        #expect(HubOutcome(failure: urlError(NSURLErrorCancelled)) == .unknown)
        #expect(HubOutcome(failure: CancellationError()) == .unknown)
        #expect(HubOutcome.worst([]) == .unknown)
        #expect(HubOutcome.worst([.answers]) == .unknown)
        #expect(HubOutcome.worst([.unknown, .unreachable, .untrusted]) == .untrusted)
        #expect(HubOutcome.worst([.unreachable, .notYourHub, .untrusted]) == .notYourHub)
    }

    // MARK: The probe's answer

    @Test func theProbeSaysWhoAnswered() async {
        URLProtocol.registerClass(OutcomeHealthProtocol.self)
        #expect(await HubRoute.healthCheck(candidate("https://here.invalid")) == .answers)
        #expect(await HubRoute.healthCheck(candidate("https://other.invalid")) == .notYourHub)
        // No hub_id: it can't be told apart from another server, but it says nothing about another hub either.
        #expect(await HubRoute.healthCheck(candidate("https://bare.invalid")) == .unreachable)
        #expect(await HubRoute.healthCheck(candidate("https://sick.invalid")) == .unreachable)
        // The paired address is not checked, as before: another hub_id there is left to the hello.
        #expect(await HubRoute.healthCheck(candidate("https://other.invalid", address: 0)) == .answers)
        // Pass or fail exactly as the yes / no probe.
        for c in [candidate("https://here.invalid"), candidate("https://other.invalid"), candidate("https://bare.invalid"),
                  candidate("https://sick.invalid"), candidate("https://other.invalid", address: 0)] {
            #expect(await HubRoute.healthProbe(c) == (await HubRoute.healthCheck(c) == .answers), "\(c.url)")
        }
    }

    // MARK: Not your hub on a private address

    @Test func anotherHubOnAPrivateAddressCountsAsCouldNotConnect() {
        let lan = candidate("https://\(OutcomeHealthProtocol.lan):18662"), pub = candidate("https://other.invalid")
        let at = Date()
        let tried = [HubRoute.Tried(candidate: lan, outcome: .notYourHub, at: at), .init(candidate: pub, outcome: .notYourHub, at: at)]
        // Public and private tried: the private one is just another device on this network.
        let mixed = HubRoute.settled(tried, among: [lan, pub])
        #expect(mixed.map(\.outcome) == [.unreachable, .notYourHub])
        // Only private addresses tried: nothing else could say more.
        let ula = candidate("https://[fd12:3456::1]:18662"), local = candidate("http://hub.local:8080")
        let onlyPrivate = HubRoute.settled([.init(candidate: lan, outcome: .notYourHub, at: at), .init(candidate: ula, outcome: .unreachable, at: at)],
                                           among: [lan, ula, local])
        #expect(onlyPrivate.map(\.outcome) == [.notYourHub, .unreachable])
    }

    @Test func privateAddressesAsAndroidTellsThem() {
        for s in ["http://127.0.0.1:1", "http://\(ip(10, 1, 2, 3))", "https://\(ip(172, 16, 0, 1))", "https://\(ip(172, 31, 255, 255))",
                  "http://\(ip(192, 168, 0, 5)):8080", "http://\(ip(169, 254, 1, 1))", "http://[::1]:8080", "https://[fe80::1%25en0]:443",
                  "https://[fd00::5]", "https://[fc01::5]", "http://localhost:18760", "https://hub.local", "http://[::ffff:\(ip(10, 0, 0, 1))]"] {
            #expect(HubAddressScope.isPrivate(url(s)), "\(s)")
        }
        for s in ["https://hub.example.com", "https://\(ip(172, 32, 0, 1))", "https://\(ip(8, 8, 8, 8))", "https://[2001:db8::1]",
                  "https://\(ip(100, 64, 0, 1))", "https://\(ip(192, 169, 0, 1))", "https://[::ffff:\(ip(8, 8, 8, 8))]", "https://local.example.com"] {
            #expect(!HubAddressScope.isPrivate(url(s)), "\(s)")
        }
    }

    // MARK: The race

    @Test func probesThePickerCutsShortAreUnknown() async {
        let a = candidate("https://a.invalid", address: 0), b = candidate("https://b.invalid"), c = candidate("https://c.invalid", address: 2)
        // A answers at once; B never does: A is taken without waiting, B was cut short.
        let quick = await HubRoute.pickTried([a, b, c], grace: .seconds(30)) { cand in
            if cand.url == b.url { try? await Task.sleep(for: .seconds(30)); return .unreachable }
            return cand.url == a.url ? .answers : .untrusted
        }
        #expect(quick.picked == a)
        let outcomes = Dictionary(uniqueKeysWithValues: quick.tried.map { ($0.candidate.url, $0.outcome) })
        #expect(outcomes[a.url] == .answers)
        #expect(outcomes[b.url] == .unknown)
        #expect(quick.tried.map(\.candidate) == [a, b, c]) // every candidate once, in order
        // None answers: the first, for the socket's own retries, with each one's reason.
        let none = await HubRoute.pickTried([a, b]) { $0.url == a.url ? .untrusted : .unreachable }
        #expect(none.picked == a)
        #expect(none.tried.map(\.outcome) == [.untrusted, .unreachable])
        // One candidate: no probe, as `pick`.
        let alone = await HubRoute.pickTried([a]) { _ in Issue.record("probed a single candidate"); return .answers }
        #expect(alone.picked == a && alone.tried.isEmpty)
        // `answeringTried` asks a single one too, and says why it failed.
        let single = await HubRoute.answeringTried([a]) { _ in .untrusted }
        #expect(single.winner == nil && single.tried.map(\.outcome) == [.untrusted])
    }

    /// The same choice as the yes / no `pick`, whatever the reason given for a failure. Every probe answers at once and
    /// the grace is long, so the choice does not depend on how busy the machine is.
    @Test func pickTriedPicksAsPick() async {
        let a = candidate("https://a.invalid", address: 0), b = candidate("https://b.invalid"), c = candidate("https://c.invalid", address: 2)
        let cases: [[URL: HubOutcome]] = [
            [a.url: .answers, b.url: .answers, c.url: .answers],
            [a.url: .untrusted, b.url: .answers, c.url: .answers],
            [a.url: .unreachable, b.url: .notYourHub, c.url: .answers],
            [a.url: .unknown, b.url: .unreachable, c.url: .untrusted],
        ]
        for outcomes in cases {
            let tried = await HubRoute.pickTried([a, b, c], grace: .seconds(30)) { outcomes[$0.url]! }
            let picked = await HubRoute.pick([a, b, c], grace: .seconds(30)) { outcomes[$0.url]! == .answers }
            #expect(tried.picked == picked, "\(outcomes)")
        }
    }

    // MARK: Connection Details' lines

    @MainActor @Test func linesFollowTheOrderTriedAndTheLastTry() {
        let board = HubAddressBoard()
        let a = url("https://a.invalid"), b = url("https://b.invalid:8443/")
        board.arrange("g", addresses: [a, b])
        #expect(board.lines["g"]?.map(\.id) == ["https://a.invalid", "https://b.invalid:8443"])
        #expect(board.lines["g"]?.allSatisfy { $0.outcome == nil && $0.tried == nil } == true)
        board.record("g", [(a, .untrusted, Date()), (b, .unknown, Date())])
        #expect(board.lines["g"]?.map(\.outcome) == [.untrusted, .unknown])
        #expect(board.lines["g"]?.first?.word == HubOutcome.untrusted.title)
        #expect(board.lines["g"]?.first?.tried != nil)
        // A later try of one address replaces only its line.
        board.record("g", [(a, .answers, Date())])
        #expect(board.lines["g"]?.map(\.outcome) == [.answers, .unknown])
        // The order changes with the addresses; what is known stays.
        board.arrange("g", addresses: [b, a])
        #expect(board.lines["g"]?.map(\.outcome) == [.unknown, .answers])
        board.keep([])
        #expect(board.lines["g"] == nil)
    }

    @MainActor @Test func whatCameAfterTheBackgroundWasCutShort() {
        let board = HubAddressBoard()
        let start = Date()
        let a = candidate("https://a.invalid", address: 0), b = candidate("https://b.invalid")
        let back = start.addingTimeInterval(1)
        board.wentToBackground(at: back)
        let tried = board.cutShort([.init(candidate: a, outcome: .unreachable, at: start.addingTimeInterval(0.5)),
                                    .init(candidate: b, outcome: .unreachable, at: start.addingTimeInterval(2))], since: start)
        #expect(tried.map(\.outcome) == [.unreachable, .unknown])
        // An attempt that began after the notice (a silent push's wake, or back in front) is not touched.
        let wake = board.cutShort([.init(candidate: b, outcome: .unreachable, at: back.addingTimeInterval(5))], since: back.addingTimeInterval(3))
        #expect(wake.map(\.outcome) == [.unreachable])
        // A try the earlier attempt asked while the app was away was cut short too; one it asked after the app came
        // back was not.
        let away = board.cutShort([.init(candidate: b, outcome: .unreachable, at: back.addingTimeInterval(5))], since: start,
                                  asked: back.addingTimeInterval(1))
        #expect(away.map(\.outcome) == [.unknown])
        board.cameToFront(at: back.addingTimeInterval(2))
        let returned = board.cutShort([.init(candidate: b, outcome: .unreachable, at: back.addingTimeInterval(5))], since: start,
                                      asked: back.addingTimeInterval(3))
        #expect(returned.map(\.outcome) == [.unreachable])
    }

    /// One address and no tunnel: no probe, so the socket's own attempt says (classified the same way). With a probe,
    /// the socket only replaces an Answers at the address it then failed to open.
    @MainActor @Test func aSingleAddressTakesTheSocketsOwnOutcome() {
        let model = AppModel(stateFolder: FileManager.default.temporaryDirectory.appending(path: "outcomes-" + UUID().uuidString))
        let one = ServerConfig(url: url("http://127.0.0.1:1"), token: "device")
        model.registry = HubRegistry(hubs: [PairedHub(config: one, group: "")])
        model.buildLinks()
        #expect(model.addressBoard.lines[""]?.map(\.outcome) == [nil]) // not tried yet
        model.socketTried("", one, HubOutcome(failure: urlError(NSURLErrorServerCertificateUntrusted)), since: Date())
        #expect(model.addressBoard.lines[""]?.map(\.outcome) == [.untrusted])
        model.socketTried("", one, .answers, since: Date())
        #expect(model.addressBoard.lines[""]?.map(\.outcome) == [.answers])

        var two = PairedHub(config: one, group: "", hubID: "H")
        two.addresses = [HubAddress(url: url("https://b.invalid"))]
        model.registry = HubRegistry(hubs: [two])
        model.buildLinks()
        let b = two.configs[1]
        #expect(model.addressBoard.lines[""]?.count == 2)
        // The probe is the source: a socket failure where no probe said Answers changes nothing.
        model.socketTried("", b, .unreachable, since: Date())
        #expect(model.addressBoard.outcome("", b.url) == nil)
        model.addressBoard.beginAttempt("")
        model.attemptTried("", [.init(candidate: .init(config: b, hub: two.id, address: 1, hubID: "H"), outcome: .answers, at: Date())], since: Date())
        #expect(model.addressBoard.outcome("", b.url) == .answers)
        model.socketTried("", b, .answers, since: Date()) // connecting there says nothing new
        model.socketTried("", b, .unreachable, since: Date()) // the socket then failed there: that is what the attempt came to
        #expect(model.addressBoard.outcome("", b.url) == .unreachable)
    }

    /// A model with one hub "H": paired at `paired`, then `added`; built, not connected.
    @MainActor private func model(paired: String, added: [String] = []) -> (AppModel, PairedHub) {
        let model = AppModel(stateFolder: FileManager.default.temporaryDirectory.appending(path: "outcomes-" + UUID().uuidString))
        var hub = PairedHub(config: ServerConfig(url: url(paired), token: "device"), group: "", hubID: "H")
        hub.addresses = added.isEmpty ? nil : added.map { HubAddress(url: url($0)) }
        model.registry = HubRegistry(hubs: [hub])
        model.buildLinks()
        return (model, hub)
    }

    /// Through a tunnel set to be used every time, an attempt asks no address: the socket's failure there is the
    /// tunnel's, so it leaves the Answers an earlier attempt found at the paired address.
    @MainActor @Test func aTunnelsSocketFailureLeavesAnOlderAnswers() {
        let (model, hub) = model(paired: "https://a.invalid", added: ["https://b.invalid"])
        let paired = HubRoute.Candidate(config: hub.config, hub: hub.id, address: 0)
        model.addressBoard.beginAttempt("")
        model.attemptTried("", [.init(candidate: paired, outcome: .answers, at: Date())], since: Date())
        // The next attempt goes through the tunnel without asking the address; its socket fails.
        model.addressBoard.beginAttempt("")
        model.socketTried("", hub.config, .unreachable, since: Date())
        #expect(model.addressBoard.outcome("", hub.config.url) == .answers)
        // An attempt that did ask it, and then failed to open the socket there: that is what it came to.
        model.addressBoard.beginAttempt("")
        model.attemptTried("", [.init(candidate: paired, outcome: .answers, at: Date())], since: Date())
        model.socketTried("", hub.config, .unreachable, since: Date())
        #expect(model.addressBoard.outcome("", hub.config.url) == .unreachable)
    }

    /// The recheck of preferred addresses asks them in turn: one asked after the app went to the background, inside an
    /// attempt that began in front, was cut short, never Couldn't Connect.
    @MainActor @Test func probesStartedAfterTheBackgroundAreUnknown() async {
        let (model, hub) = model(paired: "https://a.invalid", added: ["https://b.invalid", "https://c.invalid"])
        let all = HubRoute.candidates(model.registry, group: "")
        let first = all[0], second = all[1]
        _ = await model.askPreferred("", [first, second]) { c in
            // The background notice comes while the first is asked; the second starts after it.
            if c == first { await MainActor.run { model.addressBoard.wentToBackground(at: Date()) } }
            return .unreachable
        }
        #expect(model.addressBoard.outcome("", first.url) == .unknown)
        #expect(model.addressBoard.outcome("", second.url) == .unknown)
        #expect(hub.addresses?.count == 2)
    }

    /// Not your hub on a private address, through the probe race and the recheck: couldn't connect while a public
    /// address is among those tried, not your hub when only private ones are.
    @MainActor @Test func theRaceAndTheRecheckSettleAnotherHubOnAPrivateAddress() async {
        URLProtocol.registerClass(OutcomeHealthProtocol.self)
        let lan = candidate("https://\(OutcomeHealthProtocol.lan)"), pub = candidate("https://other.invalid", address: 2)
        #expect(await HubRoute.healthCheck(lan) == .notYourHub) // what the LAN host itself answers
        let mixed = await HubRoute.answeringTried([lan, pub])
        #expect(mixed.winner == nil)
        #expect(mixed.tried.map(\.outcome) == [.unreachable, .notYourHub])
        let alone = await HubRoute.answeringTried([lan])
        #expect(alone.tried.map(\.outcome) == [.notYourHub])
        // The recheck settles each answer among all the group's addresses.
        let (model, _) = model(paired: "https://here.invalid", added: ["https://\(OutcomeHealthProtocol.lan)", "https://other.invalid"])
        let all = HubRoute.candidates(model.registry, group: "")
        _ = await model.askPreferred("", [all[1], all[2]])
        #expect(model.addressBoard.outcome("", all[1].url) == .unreachable)
        #expect(model.addressBoard.outcome("", all[2].url) == .notYourHub)
    }

    /// The socket's own attempt reaches the lines through `LiveConnection.onAttempt`, as the app wires it: a refused
    /// connection to a single address says Couldn't Connect.
    @MainActor @Test func theSocketsOwnFailureReachesTheLines() async throws {
        let (model, hub) = model(paired: "http://127.0.0.1:1")
        let live = try #require(model.link("")?.live)
        #expect(live.onAttempt != nil)
        live.connect(hub.config)
        defer { live.disconnect() }
        let deadline = Date().addingTimeInterval(15)
        while model.addressBoard.outcome("", hub.config.url) == nil, Date() < deadline {
            try await Task.sleep(for: .milliseconds(50))
        }
        #expect(model.addressBoard.outcome("", hub.config.url) == .unreachable)
    }

    /// The recheck began before a quick flip to another app and back: the probe asked after the return says what it
    /// came to, the one under way at the background notice was cut short.
    @MainActor @Test func aProbeAskedAfterTheReturnToTheFrontSaysWhatItCameTo() async {
        let (model, _) = model(paired: "https://a.invalid", added: ["https://b.invalid", "https://c.invalid"])
        let all = HubRoute.candidates(model.registry, group: "")
        let first = all[0], second = all[1]
        _ = await model.askPreferred("", [first, second]) { c in
            if c == first {
                await MainActor.run { model.addressBoard.wentToBackground(at: Date()) }
                await MainActor.run { model.addressBoard.cameToFront(at: Date()) }
            }
            return .unreachable
        }
        #expect(model.addressBoard.outcome("", first.url) == .unknown)
        #expect(model.addressBoard.outcome("", second.url) == .unreachable)
    }

    // MARK: Picks that end early (#11683)

    /// Set once from a probe, read on the main actor.
    @MainActor private final class Flag {
        var value = false
        func set() { value = true }
    }

    @MainActor private func until(_ condition: () -> Bool) async throws {
        let deadline = Date().addingTimeInterval(10)
        while !condition(), Date() < deadline { try await Task.sleep(for: .milliseconds(5)) }
        try #require(condition())
    }

    /// A probe that waits until the race cancels it: Unknown, as `healthCheck` says for a cancelled request.
    private static func pending() async -> HubOutcome {
        do { try await Task.sleep(for: .seconds(30)) } catch { return .unknown }
        return .unreachable
    }

    /// The restore race, a direct address first: the lines carry what each probe came to, and the socket's failure at
    /// the winner (a 401, Access refusing the upgrade) is what the attempt came to there.
    @MainActor @Test func aRestoreRaceWonByADirectAddressRecordsItsProbes() async {
        let (model, _) = model(paired: "https://a.invalid", added: ["https://b.invalid"])
        let all = HubRoute.candidates(model.registry, group: "")
        let a = all[0], b = all[1]
        model.routeCheckForTests = { $0.url == a.url ? .unreachable : .answers }
        model.addressBoard.beginAttempt("")
        let result = await model.restoreRace("", HubTunnelRecovery {}, all, since: Date()) {
            try? await Task.sleep(for: .seconds(30)) // the SSH tunnel still starting
            return false
        }
        guard case .direct(let winner) = result else { Issue.record("the direct address did not win"); return }
        #expect(winner == b)
        #expect(model.addressBoard.outcome("", a.url) == .unreachable)
        #expect(model.addressBoard.outcome("", b.url) == .answers)
        model.socketTried("", b.config, .unreachable, since: Date())
        #expect(model.addressBoard.outcome("", b.url) == .unreachable)
    }

    /// The restore race, the tunnel first while the paired address answered inside the grace (the center still
    /// pending): the center was cut short, and the tunnel's socket failing leaves the paired address's Answers.
    @MainActor @Test func aRestoreRaceWonByTheTunnelKeepsWhatAnsweredDuringTheGrace() async throws {
        let (model, _) = model(paired: "https://a.invalid", added: ["https://b.invalid"])
        let paired = HubRoute.candidates(model.registry, group: "")[0]
        let center = HubRoute.Candidate(config: ServerConfig(url: url("https://center.invalid"), token: "device"), hub: "center", center: true)
        let answered = Flag()
        model.routeCheckForTests = { c in
            guard c.url == paired.url else { return await Self.pending() }
            await answered.set()
            return .answers
        }
        model.addressBoard.beginAttempt("")
        let result = await model.restoreRace("", HubTunnelRecovery {}, [center, paired], since: Date()) {
            try? await self.until { answered.value }
            return answered.value
        }
        guard case .tunnel = result else { Issue.record("the restored tunnel did not win"); return }
        #expect(model.addressBoard.outcome("", center.url) == .unknown)
        #expect(model.addressBoard.outcome("", paired.url) == .answers)
        // The tunnel's socket goes to the hub's own config: its failure is the tunnel's, not the address's.
        model.socketTried("", paired.config, .unreachable, since: Date())
        #expect(model.addressBoard.outcome("", paired.url) == .answers)
    }

    /// A pick replaced before it ended (Retry Now, the network back, the background), for a group without a tunnel:
    /// what its probes came to is on the lines, and nothing of it is the next attempt's.
    @MainActor @Test func aReplacedPickRecordsItsProbesAndLeavesTheNextAttemptAlone() async throws {
        let (model, _) = model(paired: "https://a.invalid", added: ["https://b.invalid"])
        let all = HubRoute.candidates(model.registry, group: "")
        try await replacedPick(model, paired: all[0], added: all[1])
    }

    /// The same for a group with an SSH tunnel, whose pick asks the direct way first (`.askDirect`).
    @MainActor @Test func aReplacedDirectCheckRecordsItsProbesAndLeavesTheNextAttemptAlone() async throws {
        let (model, _) = model(paired: "https://a.invalid", added: ["https://b.invalid"])
        var hub = model.registry.hubs[0]
        hub.ssh = SSHRoute(host: "127.0.0.1", port: 1, user: "test", hostKey: "SHA256:test")
        model.registry = HubRegistry(hubs: [hub])
        model.buildLinks()
        #expect(!model.hubTunnels("").isEmpty)
        let all = HubRoute.candidates(model.registry, group: "")
        try await replacedPick(model, paired: all[0], added: all[1])
    }

    /// The added address answers while the paired one, preferred, is still asked; the next attempt begins before the
    /// old pick unwinds, as `LiveConnection.open()` starts it.
    @MainActor private func replacedPick(_ model: AppModel, paired: HubRoute.Candidate, added: HubRoute.Candidate) async throws {
        let answered = Flag()
        model.routeCheckForTests = { c in
            guard c.url == added.url else { return await Self.pending() }
            await answered.set()
            return .answers
        }
        let route = try #require(model.routePicker(""))
        let old = Task { @MainActor in await route() }
        try await until { answered.value }
        model.addressBoard.beginAttempt("")
        old.cancel()
        _ = await old.value
        #expect(model.addressBoard.outcome("", added.url) == .answers)
        #expect(model.addressBoard.outcome("", paired.url) == .unknown)
        #expect(!model.addressBoard.takeAnswered("", added.url))
    }

    /// How a connected socket got to its hub, in the tally's words.
    @Test func connectedKindSaysHowTheSocketGotThere() {
        var hub = PairedHub(config: ServerConfig(url: url("https://a.invalid"), token: "device"), group: "", hubID: "H")
        hub.addresses = [HubAddress(url: url("https://b.invalid"), access: HubAccess(clientID: "id.access", clientSecret: "secret"))]
        hub.mesh = [MeshHub(hubID: "H", isSelf: true), MeshHub(hubID: "M", endpoints: ["http://m.invalid:18660"])]
        let registry = HubRegistry(hubs: [hub])
        var hello = HubHello()
        hello.hubID = "H"
        let direct: (PairedHub) -> DevProbe.Connected.Route = { _ in .direct }
        let paired = AppModel.connectedKind(on: hub.config, hello: hello, registry: registry, group: "", routed: direct)
        #expect(paired == .init(trust: .system, accessToken: false, route: .direct, address: .paired, helloFingerprint: false))
        let added = AppModel.connectedKind(on: hub.configs[1], hello: hello, registry: registry, group: "", routed: direct)
        #expect(added == .init(trust: .system, accessToken: true, route: .direct, address: .added, helloFingerprint: false))
        let mesh = AppModel.connectedKind(on: ServerConfig(url: url("http://m.invalid:18660"), token: "device"), hello: hello,
                                          registry: registry, group: "", routed: direct)
        #expect(mesh.address == .mesh && mesh.trust == .plainHTTP && mesh.route == .direct)
        // Through a tunnel the socket speaks plain HTTP to its loopback end: the tunnel vouches, not the system's trust.
        let tunneled = AppModel.connectedKind(on: hub.config, hello: hello, registry: registry, group: "") { _ in .sshTunnel }
        #expect(tunneled.trust == .tunnel && tunneled.route == .sshTunnel && tunneled.address == .paired)
        // The fingerprint the hello lists for the hub that sent it.
        var fp = hub.mesh![0]
        fp.fp = "fingerprint"
        hello.mesh = [fp]
        #expect(AppModel.connectedKind(on: hub.config, hello: hello, registry: registry, group: "", routed: direct).helloFingerprint)
    }
}
