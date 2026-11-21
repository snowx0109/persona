import Foundation

/// Developing Persona's connection tally (#11390, docs/operations/developing-persona.md "Connection tally"): per ISO
/// week in UTC, how many opens never connected (by the most telling outcome their addresses had) and how many did (by
/// how the address in use was trusted and reached), and how long it was recorded, so a week with nothing recorded
/// never reads as zero failures. Only fixed words, yes / no and counts: no address, host, port, hub id or fingerprint.
/// Kept in `connections.json` beside the other records (`DevStore.connections`) for `keptWeeks` weeks.
nonisolated struct ConnectionTally: Codable, Equatable, Sendable {
    static let format = "persona-connection-tally"
    static let version = 1
    /// The weeks kept: this one and the seven before it (a four-week reading fits twice).
    static let keptWeeks = 8

    var format = Self.format
    var version = Self.version
    /// Oldest first.
    var weeks: [Week] = []

    struct Week: Codable, Equatable, Sendable {
        /// The ISO week, `2026-W47` (UTC).
        var week: String
        /// Its Monday, `2026-11-16` (UTC).
        var starts: String
        /// Seconds the app was in front this week with recording on, counted from the start of a period in front.
        var recordedSeconds = 0
        /// Opens whose socket never connected, by the most telling outcome: `not_your_hub`, `certificate_not_trusted`,
        /// `couldnt_connect`, `unknown`.
        var notConnected: [String: Int] = Self.noFailures
        /// Opens that connected, by how.
        var connected: [Connected] = []

        static let noFailures: [String: Int] = Dictionary(uniqueKeysWithValues: HubOutcome.allCases.filter { $0.failureRank != nil }.map { ($0.rawValue, 0) })

        enum CodingKeys: String, CodingKey {
            case week, starts, connected
            case recordedSeconds = "recorded_seconds", notConnected = "not_connected"
        }
    }

    /// The opens that connected one way.
    struct Connected: Codable, Equatable, Sendable {
        var trust: String
        var accessToken: Bool
        var route: String
        var address: String
        var helloFingerprint: Bool
        var opens: Int

        init(_ how: DevProbe.Connected, opens: Int) {
            trust = how.trust.rawValue
            accessToken = how.accessToken
            route = how.route.rawValue
            address = how.address.rawValue
            helloFingerprint = how.helloFingerprint
            self.opens = opens
        }

        func same(as other: Connected) -> Bool {
            (trust, accessToken, route, address, helloFingerprint) == (other.trust, other.accessToken, other.route, other.address, other.helloFingerprint)
        }

        enum CodingKeys: String, CodingKey {
            case trust, route, address, opens
            case accessToken = "access_token", helloFingerprint = "hello_fingerprint"
        }
    }

    var isEmpty: Bool { weeks.isEmpty }

    // MARK: Counting

    /// An open that never connected, under `outcome` (a failure word), in the week of `at`.
    mutating func failed(_ outcome: HubOutcome, at: Date) {
        update(at) { $0.notConnected[outcome.rawValue, default: 0] += 1 }
    }

    /// An open that connected `how`, in the week of `at`.
    mutating func connected(_ how: DevProbe.Connected, at: Date) {
        let one = Connected(how, opens: 1)
        update(at) { w in
            if let i = w.connected.firstIndex(where: { $0.same(as: one) }) { w.connected[i].opens += 1 } else { w.connected.append(one) }
        }
    }

    /// Time in front with recording on, split across the weeks it spans.
    mutating func recorded(from start: Date, to end: Date) {
        var from = start
        while from < end {
            let next = min(end, Self.weekStart(from).addingTimeInterval(7 * 86400))
            let seconds = Int(next.timeIntervalSince(from).rounded())
            if seconds > 0 { update(from) { $0.recordedSeconds += seconds } }
            from = next
        }
    }

    /// `other`'s counts added to these.
    mutating func merge(_ other: ConnectionTally) {
        for w in other.weeks {
            guard let i = weeks.firstIndex(where: { $0.week == w.week }) else { weeks.append(w); continue }
            weeks[i].recordedSeconds += w.recordedSeconds
            for (k, n) in w.notConnected { weeks[i].notConnected[k, default: 0] += n }
            for c in w.connected {
                if let j = weeks[i].connected.firstIndex(where: { $0.same(as: c) }) { weeks[i].connected[j].opens += c.opens } else { weeks[i].connected.append(c) }
            }
        }
        weeks.sort { $0.starts < $1.starts }
    }

    /// Only the weeks from seven before `now`'s on.
    mutating func trim(now: Date) {
        let first = Self.weekStart(now).addingTimeInterval(-Double(Self.keptWeeks - 1) * 7 * 86400)
        let cut = Self.day(first)
        weeks.removeAll { $0.starts < cut }
    }

    private mutating func update(_ at: Date, _ change: (inout Week) -> Void) {
        let id = Self.weekID(at)
        if let i = weeks.firstIndex(where: { $0.week == id }) {
            change(&weeks[i])
        } else {
            var w = Week(week: id, starts: Self.day(Self.weekStart(at)))
            change(&w)
            weeks.append(w)
            weeks.sort { $0.starts < $1.starts }
        }
    }

    // MARK: Weeks

    private static let calendar: Calendar = {
        var c = Calendar(identifier: .iso8601)
        c.timeZone = .gmt
        return c
    }()

    static func weekID(_ date: Date) -> String {
        let c = calendar.dateComponents([.yearForWeekOfYear, .weekOfYear], from: date)
        let year = c.yearForWeekOfYear ?? 0, week = c.weekOfYear ?? 0
        return "\(year)-W" + (week < 10 ? "0\(week)" : "\(week)")
    }

    static func weekStart(_ date: Date) -> Date {
        calendar.dateInterval(of: .weekOfYear, for: date)?.start ?? date
    }

    private static let dayFormat: Date.VerbatimFormatStyle = .init(
        format: "\(year: .defaultDigits)-\(month: .twoDigits)-\(day: .twoDigits)", timeZone: .gmt, calendar: Calendar(identifier: .gregorian))

    static func day(_ date: Date) -> String { date.formatted(dayFormat) }

    // MARK: The file

    func encoded() -> Data? {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys, .prettyPrinted, .withoutEscapingSlashes]
        return try? encoder.encode(self)
    }

    static func decoded(_ data: Data) -> ConnectionTally? {
        guard let t = try? JSONDecoder().decode(ConnectionTally.self, from: data), t.format == format else { return nil }
        return t
    }
}

/// The opens of the period in front being counted (#11390). An open is one period with the app in front, per hub group
/// (Demo Mode left out), from the moment the phone has a network path; it ends when the app leaves the front. It
/// counts as connected when the group's socket reached its hub in it (by how, at the first time), else under the most
/// telling outcome its addresses' tries had, unknown when none failed before it ended. A period already under way when
/// the switch is turned on is not counted: what happened before it is not known.
nonisolated struct ConnectionOpens: Sendable {
    private struct Open: Sendable {
        var began: Date
        var outcomes: Set<HubOutcome> = []
        var connected: DevProbe.Connected?
    }

    /// This period in front counts.
    private(set) var counting = false
    private var frontSince: Date?
    private var pathUp = false
    private var groups: Set<String> = []
    private var opens: [String: Open] = [:]
    /// The groups whose socket is connected now, and how, kept across periods: a socket that came up in the background
    /// (a silent push woke the app) and is still up when the app comes to the front sends no new hello.
    private var live: [String: DevProbe.Connected] = [:]

    /// Recording starts: in front (`active`) at launch the period counts (`fresh`); turned on by hand it does not.
    mutating func start(active: Bool, fresh: Bool, at now: Date) {
        counting = active && fresh
        frontSince = counting ? now : nil
        opens = [:]
        begin(now)
    }

    /// The app came to the front: a new period.
    mutating func front(at now: Date) {
        counting = true
        frontSince = now
        opens = [:]
        begin(now)
    }

    /// The app left the front: the period's opens are counted, with its time.
    mutating func back(at now: Date) -> ConnectionTally {
        var tally = ConnectionTally()
        guard counting else { return tally }
        for open in opens.values {
            if let how = open.connected { tally.connected(how, at: open.began) } else { tally.failed(HubOutcome.worst(open.outcomes), at: open.began) }
        }
        if let since = frontSince { tally.recorded(from: since, to: now) }
        counting = false
        frontSince = nil
        opens = [:]
        return tally
    }

    /// Whether the phone has a network path now.
    mutating func path(_ up: Bool, at now: Date) {
        pathUp = up
        begin(now)
    }

    mutating func link(_ event: DevProbe.Link, at now: Date) {
        switch event {
        case .groups(let set):
            groups = set
            opens = opens.filter { set.contains($0.key) }
            live = live.filter { set.contains($0.key) }
            begin(now)
        case .tried(let group, let outcomes, let since):
            // A try without a network path says nothing about the hub, in an open or before one; an attempt that began
            // before this period (the app left the front and came back while it ran) belongs to the period it began in.
            guard counting, pathUp, let front = frontSince, since >= front else { return }
            open(group, now)
            opens[group]?.outcomes.formUnion(outcomes)
        case .connected(let group, let how):
            live[group] = how
            // Connected: there is a network, whatever the path monitor said so far.
            guard counting else { return }
            open(group, now)
            if opens[group]?.connected == nil { opens[group]?.connected = how }
        case .disconnected(let group):
            live[group] = nil
        }
    }

    private mutating func open(_ group: String, _ now: Date) {
        groups.insert(group)
        if opens[group] == nil { opens[group] = Open(began: now, connected: live[group]) }
    }

    /// Opens for the groups there are once the phone has a network path, and at once for those already connected (a
    /// connected socket proves one): an open already connected counts as connected.
    private mutating func begin(_ now: Date) {
        guard counting else { return }
        for g in (pathUp ? groups : []).union(live.keys) where opens[g] == nil { opens[g] = Open(began: now, connected: live[g]) }
    }
}
