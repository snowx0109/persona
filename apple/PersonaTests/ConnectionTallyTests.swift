import Foundation
import os
import Testing
@testable import Persona
import PersonaWire

/// Developing Persona's connection tally (#11390, docs/operations/developing-persona.md "Connection tally"): nothing
/// without the switch, opens counted in their week and kind, unknown for a period that ended before any try failed, 8
/// weeks kept, deleted with the switch, in the export with its README line, and no address, host, port, hub id or
/// fingerprint in the file.
struct ConnectionTallyTests {
    /// Monday 2027-01-11T00:00:00Z: ISO week 2027-W02.
    private static let monday = Date(timeIntervalSince1970: 1_799_625_600)

    private final class Clock: @unchecked Sendable {
        private let lock: OSAllocatedUnfairLock<Date>
        init(_ start: Date) { lock = OSAllocatedUnfairLock(initialState: start) }
        var now: Date { lock.withLock { $0 } }
        func advance(_ seconds: TimeInterval) { lock.withLock { $0 = $0.addingTimeInterval(seconds) } }
    }

    private func scratch() -> URL {
        FileManager.default.temporaryDirectory.appending(path: "connection-tally-tests-\(UUID().uuidString)", directoryHint: .isDirectory)
    }

    private func recorder(_ dir: URL, _ clock: Clock) -> DevRecorder {
        DevRecorder(store: DevStore(directory: dir), clock: { clock.now }, power: { .init() }, build: "612")
    }

    private let direct = DevProbe.Connected(trust: .system, accessToken: false, route: .direct, address: .paired, helloFingerprint: true)
    private let tunneled = DevProbe.Connected(trust: .plainHTTP, accessToken: true, route: .sshTunnel, address: .added, helloFingerprint: false)
    private let wifi = DevRecorder.NetPath(type: "wifi")

    @Test func weeksAreISOWeeksInUTC() {
        #expect(ConnectionTally.weekID(Self.monday) == "2027-W02")
        #expect(ConnectionTally.weekID(Self.monday.addingTimeInterval(-1)) == "2027-W01") // Sunday 23:59:59Z
        #expect(ConnectionTally.weekID(Date(timeIntervalSince1970: 1_798_761_600)) == "2026-W53") // 2027-01-01, a Friday
        #expect(ConnectionTally.day(ConnectionTally.weekStart(Self.monday.addingTimeInterval(3 * 86400))) == "2027-01-11")
    }

    @Test func nothingIsRecordedWhileTheSwitchIsOff() {
        let dir = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.path(wifi)
        r.link(.groups(["g"]))
        r.link(.tried(group: "g", [.unreachable], since: clock.now))
        r.link(.connected(group: "g", direct))
        r.lifecycle(active: true, device: .init())
        clock.advance(60)
        r.lifecycle(active: false, device: .init())
        r.settle()
        #expect(!FileManager.default.fileExists(atPath: r.store.connections.path(percentEncoded: false)))
        #expect(r.store.connectionTally(now: clock.now).isEmpty)
        #expect(r.store.recordedBytes() == 0)
    }

    @Test func opensLandInTheirWeekAndKind() throws {
        let dir = scratch(), clock = Clock(Self.monday.addingTimeInterval(3 * 86400)) // Thursday of 2027-W02
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init()) // at launch: this period counts
        r.path(wifi)
        r.link(.groups(["home", "work", "lab"]))
        r.link(.tried(group: "home", [.answers, .unknown], since: clock.now))
        r.link(.connected(group: "home", direct))
        r.link(.connected(group: "home", tunneled)) // a reconnect in the same open: counted once, as first connected
        r.link(.tried(group: "work", [.unreachable, .untrusted], since: clock.now))
        r.link(.tried(group: "work", [.unreachable], since: clock.now))
        clock.advance(90)
        r.lifecycle(active: false, device: .init())
        // Next period: the lab's socket never finished a try (unknown), work's added address answered as another hub.
        clock.advance(600)
        r.lifecycle(active: true, device: .init())
        r.link(.tried(group: "work", [.notYourHub, .unreachable], since: clock.now))
        r.link(.connected(group: "home", tunneled))
        clock.advance(30)
        r.lifecycle(active: false, device: .init())
        r.settle()

        let tally = r.store.connectionTally(now: clock.now)
        let week = try #require(tally.weeks.only)
        #expect(week.week == "2027-W02" && week.starts == "2027-01-11")
        #expect(week.recordedSeconds == 120)
        #expect(week.notConnected == ["not_your_hub": 1, "certificate_not_trusted": 1, "couldnt_connect": 0, "unknown": 2])
        #expect(week.connected.count == 2)
        let first = try #require(week.connected.first { $0.route == "direct" })
        #expect(first.opens == 1 && first.trust == "system" && first.address == "paired" && first.helloFingerprint && !first.accessToken)
        let second = try #require(week.connected.first { $0.route == "ssh_tunnel" })
        #expect(second.opens == 1 && second.trust == "plain_http" && second.address == "added" && !second.helloFingerprint && second.accessToken)
    }

    @Test func aPeriodThatEndedBeforeAnyTryFailedIsUnknown() throws {
        let dir = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init())
        r.path(wifi)
        r.link(.groups(["g"]))
        r.link(.tried(group: "g", [.answers], since: clock.now)) // the probe answered, the socket had not connected yet
        clock.advance(2)
        r.lifecycle(active: false, device: .init())
        r.settle()
        let week = try #require(r.store.connectionTally(now: clock.now).weeks.only)
        #expect(week.notConnected["unknown"] == 1)
        #expect(week.connected.isEmpty)
    }

    @Test func noNetworkPathAndAPeriodUnderWayAreNotCounted() throws {
        let dir = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        // Turned on by hand in front: what this period did before is not known, so it is not counted.
        r.start(active: true, page: "settings", device: .init(), fresh: false)
        r.path(wifi)
        r.link(.groups(["g"]))
        r.link(.tried(group: "g", [.unreachable], since: clock.now))
        clock.advance(10)
        r.lifecycle(active: false, device: .init())
        // The next period, without a network path: tries fail for want of a network, not the hub.
        r.path(.init(type: "none"))
        r.lifecycle(active: true, device: .init())
        r.link(.tried(group: "g", [.unreachable], since: clock.now))
        clock.advance(10)
        r.lifecycle(active: false, device: .init())
        // The third: the network goes away during the open; the tries after that say nothing about the hub.
        r.lifecycle(active: true, device: .init())
        r.path(wifi)
        r.path(.init(type: "none"))
        r.link(.tried(group: "g", [.unreachable], since: clock.now))
        clock.advance(5)
        r.lifecycle(active: false, device: .init())
        r.settle()
        let week = try #require(r.store.connectionTally(now: clock.now).weeks.only)
        #expect(week.recordedSeconds == 15) // the second and third periods were in front
        #expect(week.notConnected == ["not_your_hub": 0, "certificate_not_trusted": 0, "couldnt_connect": 0, "unknown": 1])
        #expect(week.connected.isEmpty)
    }

    /// The socket came up in the background (a silent push woke the app) and is still up when the user opens it: no
    /// new hello comes, and the open counts as connected all the same.
    @Test func aSocketAlreadyUpWhenThePeriodBeginsCountsAsConnected() throws {
        let dir = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: false, page: "assistant", device: .init()) // launched in the background
        r.path(wifi)
        r.link(.groups(["home", "work"]))
        r.link(.connected(group: "home", direct)) // the wake's socket, while nothing is counted
        r.link(.connected(group: "work", tunneled))
        r.link(.disconnected(group: "work")) // work's closed again before the app is opened
        clock.advance(10)
        r.lifecycle(active: true, device: .init())
        clock.advance(20)
        r.lifecycle(active: false, device: .init())
        r.settle()
        let week = try #require(r.store.connectionTally(now: clock.now).weeks.only)
        #expect(week.connected.only?.route == "direct" && week.connected.only?.opens == 1)
        #expect(week.notConnected["unknown"] == 1) // work
    }

    /// An attempt that began before the app left the front and reports after it came back (a quick flip) belongs to
    /// the period it began in, not the new one.
    @Test func triesFromAnAttemptBeforeThePeriodAreLeftOut() throws {
        let dir = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init())
        r.path(wifi)
        r.link(.groups(["g"]))
        let began = clock.now
        clock.advance(1)
        r.lifecycle(active: false, device: .init())
        clock.advance(1)
        r.lifecycle(active: true, device: .init())
        r.link(.tried(group: "g", [.untrusted, .unreachable], since: began)) // the old attempt ends now
        r.link(.tried(group: "g", [.unreachable], since: clock.now)) // the new period's own attempt
        clock.advance(5)
        r.lifecycle(active: false, device: .init())
        r.settle()
        let week = try #require(r.store.connectionTally(now: clock.now).weeks.only)
        #expect(week.notConnected == ["not_your_hub": 0, "certificate_not_trusted": 0, "couldnt_connect": 1, "unknown": 1])
    }

    /// A model with one hub, paired at `paired` and added `added`, built and not connected; what its lines tell the
    /// tally is kept in `events`.
    @MainActor private final class Lines {
        let model: AppModel
        var events: [DevProbe.Link] = []
        init(paired: String, added: [String] = []) {
            model = AppModel(stateFolder: FileManager.default.temporaryDirectory.appending(path: "tally-lines-" + UUID().uuidString))
            var hub = PairedHub(config: ServerConfig(url: URL(string: paired)!, token: "device"), group: "", hubID: "H")
            hub.addresses = added.isEmpty ? nil : added.map { HubAddress(url: URL(string: $0)!) }
            model.registry = HubRegistry(hubs: [hub])
            model.buildLinks()
            model.tallyForTests = { [unowned self] in self.events.append($0) }
        }
    }

    /// The restore race (#11683) with nothing answering, the tunnel down too: each address's failure is on the lines
    /// and in the tally, so the open says why it never connected instead of `unknown`.
    @MainActor @Test func aRestoreRaceWithNothingAnsweringSaysWhy() async throws {
        let dir = scratch(), clock = Clock(Date())
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init())
        r.path(wifi)
        r.link(.groups([""]))
        let lines = Lines(paired: "https://a.invalid", added: ["https://b.invalid"])
        let model = lines.model
        let all = HubRoute.candidates(model.registry, group: "")
        model.routeCheckForTests = { $0 == all[0] ? .untrusted : .unreachable }
        model.addressBoard.beginAttempt("")
        let start = Date()
        let result = await model.restoreRace("", HubTunnelRecovery {}, all, since: start) { false }
        guard case .unavailable = result else { Issue.record("something answered"); return }
        #expect(model.addressBoard.outcome("", all[0].url) == .untrusted)
        #expect(model.addressBoard.outcome("", all[1].url) == .unreachable)
        // The socket then goes to the first address for its own retries, and fails: the probe already said.
        model.socketTried("", all[0].config, .unreachable, since: start)
        #expect(model.addressBoard.outcome("", all[0].url) == .untrusted)
        for e in lines.events { r.link(e) }
        clock.advance(5)
        r.lifecycle(active: false, device: .init())
        r.settle()
        let week = try #require(r.store.connectionTally(now: clock.now).weeks.only)
        #expect(week.notConnected == ["not_your_hub": 0, "certificate_not_trusted": 1, "couldnt_connect": 0, "unknown": 0])
    }

    /// One address, no tunnel: the socket's own attempt says when it began, not when it ended. One begun before the
    /// period (a silent push's wake) that fails after it began is left out of it, as the probes' attempts are.
    @MainActor @Test func theSocketsAttemptCountsFromWhenItBegan() async throws {
        let lines = Lines(paired: "http://127.0.0.1:1")
        let live = try #require(lines.model.link("")?.live)
        live.connect(lines.model.registry.hubs[0].config)
        defer { live.disconnect() }
        let opened = Date() // the attempt began before this; its failure comes after
        let deadline = Date().addingTimeInterval(15)
        while lines.events.isEmpty, Date() < deadline { try await Task.sleep(for: .milliseconds(20)) }
        guard case .tried(_, let outcomes, let since)? = lines.events.first else { Issue.record("no try reached the tally"); return }
        #expect(outcomes == [.unreachable])
        #expect(since <= opened)

        // The period began between the two: the try is the earlier period's.
        let dir = scratch(), clock = Clock(opened)
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init())
        r.path(wifi)
        r.link(.groups([""]))
        r.link(lines.events[0])
        clock.advance(5)
        r.lifecycle(active: false, device: .init())
        r.settle()
        let week = try #require(r.store.connectionTally(now: clock.now).weeks.only)
        #expect(week.notConnected["couldnt_connect"] == 0 && week.notConnected["unknown"] == 1)
    }

    /// An open that begins before Monday 00:00 UTC and ends after it counts in the week it began; its time is split.
    @Test func anOpenCountsInTheWeekItBegan() throws {
        let dir = scratch(), clock = Clock(Self.monday.addingTimeInterval(-60)) // Sunday 23:59 UTC, 2027-W01
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init())
        r.path(wifi)
        r.link(.groups(["g"]))
        r.link(.tried(group: "g", [.unreachable], since: clock.now))
        clock.advance(120) // Monday 00:01 UTC, 2027-W02
        r.lifecycle(active: false, device: .init())
        r.settle()
        let weeks = r.store.connectionTally(now: clock.now).weeks
        #expect(weeks.map(\.week) == ["2027-W01", "2027-W02"])
        #expect(weeks.first?.notConnected["couldnt_connect"] == 1)
        #expect(weeks.last?.notConnected.values.reduce(0, +) == 0)
        #expect(weeks.map(\.recordedSeconds) == [60, 60])
    }

    @Test func weeksOlderThanEightAreDropped() {
        let dir = scratch()
        defer { try? FileManager.default.removeItem(at: dir) }
        let store = DevStore(directory: dir)
        for w in 0..<10 {
            let at = Self.monday.addingTimeInterval(Double(w) * 7 * 86400 + 3600)
            var t = ConnectionTally()
            t.failed(.unreachable, at: at)
            t.recorded(from: at, to: at.addingTimeInterval(60))
            store.addConnections(t, now: at)
        }
        let now = Self.monday.addingTimeInterval(9 * 7 * 86400 + 7200)
        let weeks = store.connectionTally(now: now).weeks
        #expect(weeks.count == ConnectionTally.keptWeeks)
        #expect(weeks.first?.week == "2027-W04" && weeks.last?.week == "2027-W11")
        #expect(weeks.allSatisfy { $0.notConnected["couldnt_connect"] == 1 && $0.recordedSeconds == 60 })
    }

    @Test func recordedTimeIsSplitAcrossWeeks() throws {
        var t = ConnectionTally()
        t.recorded(from: Self.monday.addingTimeInterval(-30), to: Self.monday.addingTimeInterval(90))
        #expect(t.weeks.map(\.week) == ["2027-W01", "2027-W02"])
        #expect(t.weeks.map(\.recordedSeconds) == [30, 90])
    }

    @Test func theFileHoldsNoAddressHostPortHubIDOrFingerprint() throws {
        let dir = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init())
        r.path(wifi)
        // Group ids are the app's own; nothing the hub or the user named reaches the recorder.
        r.link(.groups(["g1a2b3c4d"]))
        r.link(.tried(group: "g1a2b3c4d", [.notYourHub, .untrusted, .unreachable, .unknown, .answers], since: clock.now))
        r.link(.connected(group: "g1a2b3c4d", tunneled))
        clock.advance(5)
        r.lifecycle(active: false, device: .init())
        r.settle()
        let text = try String(contentsOf: r.store.connections, encoding: .utf8)
        for banned in ["g1a2b3c4d", "://", ".invalid", "example", "127.0.0.1", "18662", "hub_id", "\"fp\""] {
            #expect(!text.contains(banned), "\(banned)")
        }
        // Only these keys and fixed words.
        let json = try #require(JSONSerialization.jsonObject(with: Data(text.utf8)) as? [String: Any])
        #expect(Set(json.keys) == ["format", "version", "weeks"])
        let week = try #require((json["weeks"] as? [[String: Any]])?.first)
        #expect(Set(week.keys) == ["week", "starts", "recorded_seconds", "not_connected", "connected"])
        let connected = try #require((week["connected"] as? [[String: Any]])?.first)
        #expect(Set(connected.keys) == ["trust", "access_token", "route", "address", "hello_fingerprint", "opens"])
    }

    @Test func theExportHoldsTheTallyWithTheLogs() throws {
        let dir = scratch(), out = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir); try? FileManager.default.removeItem(at: out) }
        let r = recorder(dir, clock)
        r.start(active: true, page: "assistant", device: .init())
        r.path(wifi)
        r.link(.groups(["g"]))
        r.link(.connected(group: "g", direct))
        clock.advance(60)
        r.lifecycle(active: false, device: .init())
        r.settle()

        let all = try DevPackage.writePackage(into: out, store: r.store, categories: Set(DevPackage.Category.allCases), since: Self.monday,
                                              log: ["l1"], now: clock.now)
        #expect(all.connectionWeeks == 1)
        let tally = try #require(ConnectionTally.decoded(Data(contentsOf: out.appending(path: "connections.json"))))
        #expect(tally.weeks.only?.connected.only?.opens == 1)
        let manifest = try #require(JSONSerialization.jsonObject(with: Data(contentsOf: out.appending(path: "manifest.json"))) as? [String: Any])
        #expect((manifest["counts"] as? [String: Any])?["connection_weeks"] as? Int == 1)
        let readme = try String(contentsOf: out.appending(path: "README.md"), encoding: .utf8)
        #expect(readme.contains("`connections.json`"))
        #expect(readme.contains("`recorded_seconds`") && readme.contains("`not_connected`") && readme.contains("`hello_fingerprint`"))

        // Logs left out: so is the tally.
        let energy = try DevPackage.writePackage(into: out, store: r.store, categories: [.energy], since: Self.monday, log: [], now: clock.now)
        #expect(energy.connectionWeeks == 0)
        #expect(!FileManager.default.fileExists(atPath: out.appending(path: "connections.json").path(percentEncoded: false)))
    }

    /// Turning the switch off deletes the tally with everything else it recorded.
    @MainActor @Test func turningTheSwitchOffDeletesTheTally() async throws {
        let dir = scratch(), clock = Clock(Self.monday)
        defer { try? FileManager.default.removeItem(at: dir) }
        let suite = "persona.tests.connection-tally.\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let r = recorder(dir, clock)
        var t = ConnectionTally()
        t.failed(.untrusted, at: clock.now)
        r.store.addConnections(t, now: clock.now)
        #expect(FileManager.default.fileExists(atPath: r.store.connections.path(percentEncoded: false)))
        let dev = DevMode(recorder: r, defaults: defaults)
        dev.setOn(true)
        dev.setOn(false)
        await Task.detached { r.settle() }.value // the deletion runs after the queued writes
        #expect(!FileManager.default.fileExists(atPath: r.store.connections.path(percentEncoded: false)))
    }
}

private extension Array {
    /// The one element, nil unless there is exactly one.
    var only: Element? { count == 1 ? first : nil }
}
