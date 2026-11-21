import Foundation
import Observation
import PersonaWire

/// What each address of a hub group came to the last time the app tried it (#11390): the lines Connection Details lists
/// under its explanation, in the order the addresses are tried (`HubRoute.candidates`). Only from attempts the app makes
/// anyway (the route picker's probes, the race that restores an SSH tunnel used last time, a tunnel's direct check, the
/// recheck of preferred addresses, and for a group with one address and no tunnel its socket); in memory only, for this
/// launch.
///
/// The lines are worked out here when something changes, not in the details' `body`, which runs every second
/// (docs/apple/performance.md P4, P5); each has the address as its stable id (P6).
@MainActor @Observable
final class HubAddressBoard {
    struct Line: Identifiable, Equatable, Sendable {
        /// The address as the candidates key it (`HubRoute.key`).
        let id: String
        /// The address as shown.
        let address: String
        /// What its last try came to; nil before the first.
        let outcome: HubOutcome?
        /// The outcome's word, or that it was not tried yet.
        let word: String
        /// When it was last tried, as the line shows it (the time of day); nil before the first try.
        let tried: String?
        /// The same, as VoiceOver says it ("Tried at …").
        let spokenTime: String?
    }

    /// Each group's lines, in the order its addresses are tried.
    private(set) var lines: [String: [Line]] = [:]
    /// Each group's addresses, in that order.
    @ObservationIgnored private var order: [String: [URL]] = [:]
    /// Each group's last outcome per address (`HubRoute.key`) and when.
    @ObservationIgnored private var last: [String: [String: (outcome: HubOutcome, at: Date)]] = [:]
    /// When the app last went to the background (`AppModel.suspendLinks`) and, once it is back, when it came to the front
    /// (`resumeLinks`, nil while it is away): what an attempt under way at the notice learned after it was cut short,
    /// except what it asked after the app came back.
    @ObservationIgnored private(set) var lastBackground: Date?
    @ObservationIgnored private(set) var lastFront: Date?
    /// The addresses the group's current socket attempt probed and found answering (`beginAttempt`): only there may
    /// the socket's own failure replace an Answers, since only there did this attempt try the address itself.
    @ObservationIgnored private var answeredNow: [String: Set<String>] = [:]
    #if DEBUG
    /// `-showConnectionAddresses YES`: made-up outcomes stay, real ones are not taken (screenshots).
    @ObservationIgnored private var seeded = false
    #endif

    /// The group's addresses changed (paired, added, removed, listed by the mesh): its lines follow, keeping what is
    /// known.
    func arrange(_ group: String, addresses: [URL]) {
        guard order[group] != addresses else { return }
        order[group] = addresses
        rebuild(group)
    }

    /// Only these groups are left: the others' lines go.
    func keep(_ groups: Set<String>) {
        for g in order.keys where !groups.contains(g) {
            order[g] = nil
            last[g] = nil
            lines[g] = nil
        }
    }

    /// The last outcome known for `url` in `group`.
    func outcome(_ group: String, _ url: URL) -> HubOutcome? {
        last[group]?[HubRoute.key(url)]?.outcome
    }

    /// One attempt's outcomes, each address once.
    func record(_ group: String, _ tried: [(url: URL, outcome: HubOutcome, at: Date)]) {
        #if DEBUG
        if seeded { return }
        #endif
        guard !tried.isEmpty else { return }
        var known = last[group] ?? [:]
        for t in tried { known[HubRoute.key(t.url)] = (t.outcome, t.at) }
        last[group] = known
        rebuild(group)
    }

    func wentToBackground(at date: Date) {
        lastBackground = date
        lastFront = nil
    }

    func cameToFront(at date: Date) {
        if lastBackground != nil { lastFront = date }
    }

    /// One of the group's socket attempts begins (its route picker runs).
    func beginAttempt(_ group: String) { answeredNow[group] = [] }

    /// The current attempt's probes found these addresses answering.
    func attemptAnswered(_ group: String, _ urls: [URL]) {
        answeredNow[group, default: []].formUnion(urls.map(HubRoute.key))
    }

    /// Whether the current attempt's own probe found `url` answering; asked once, when the socket failed there.
    func takeAnswered(_ group: String, _ url: URL) -> Bool {
        answeredNow[group]?.remove(HubRoute.key(url)) != nil
    }

    /// The outcomes of an attempt that began at `start`, its tries asked at `asked` (the attempt's start for a race; a
    /// recheck asks one address after another), with what came after the app went to the background in the meantime as
    /// unknown: that part of the attempt was cut short (#11390), unless the try was asked after the app came back. An
    /// attempt begun after the notice (a silent push's wake, or back in front) is left as it is.
    func cutShort(_ tried: [HubRoute.Tried], since start: Date, asked: Date? = nil) -> [HubRoute.Tried] {
        guard let back = lastBackground, back >= start, (asked ?? start) < (lastFront ?? .distantFuture) else { return tried }
        return tried.map { t in
            guard t.at >= back else { return t }
            var t = t
            t.outcome = .unknown
            return t
        }
    }

    #if DEBUG
    /// `-showConnectionAddresses YES`: every outcome once, in turn, over the group's addresses, a few seconds apart.
    func seed(_ group: String, now: Date = .now) {
        let all: [HubOutcome] = [.unreachable, .untrusted, .notYourHub, .answers, .unknown]
        var known: [String: (outcome: HubOutcome, at: Date)] = [:]
        for (i, url) in (order[group] ?? []).enumerated() {
            known[HubRoute.key(url)] = (all[i % all.count], now.addingTimeInterval(-Double(i) * 7))
        }
        last[group] = known
        seeded = true
        rebuild(group)
    }
    #endif

    private func rebuild(_ group: String) {
        let known = last[group] ?? [:]
        let new = (order[group] ?? []).map { url -> Line in
            let id = HubRoute.key(url)
            guard let k = known[id] else {
                return Line(id: id, address: id, outcome: nil, word: String(localized: "Not Tried Yet [Hub Address]"), tried: nil, spokenTime: nil)
            }
            let time = k.at.formatted(.dateTime.hour().minute().second())
            return Line(id: id, address: id, outcome: k.outcome, word: k.outcome.title, tried: time,
                        spokenTime: String(localized: "Tried at \(time)"))
        }
        if lines[group] != new { lines[group] = new }
    }
}

extension HubOutcome {
    /// The word Connection Details shows for the outcome (#11390): one per outcome, the same set the tally writes
    /// (docs/operations/developing-persona.md). The address the socket is on shows In Use instead, as on Settings › hub
    /// › Addresses.
    var title: String {
        switch self {
        case .answers: String(localized: "Answers [Hub Address]")
        case .untrusted: String(localized: "Certificate Not Trusted [Hub Address]")
        case .notYourHub: String(localized: "Not Your Hub [Hub Address]")
        case .unreachable: String(localized: "Couldn't Connect [Hub Address]")
        case .unknown: String(localized: "Unknown [Hub Address]")
        }
    }
}
