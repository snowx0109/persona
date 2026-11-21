import Darwin
import Foundation
import os
import PersonaWire

/// Developing Persona's recorder (#9460, docs/operations/developing-persona.md "What is recorded"): counts in memory
/// what the app does (requests by route pattern, socket frames by type, screens) and writes a summary line a minute
/// while the app is in front, one for each stretch in the background, one per foreground period, and single lines for
/// what changes (thermal state, Low Power Mode, battery, network path, errors). No timer of its own: `DevMode` calls
/// `roll` once a minute while the app is active. Lines are buffered and appended in batches.
nonisolated final class DevRecorder: DevProbe.Sink, @unchecked Sendable {
    /// The app's CPU time, wakeups and energy so far (`TASK_POWER_INFO_V2`).
    struct Power: Equatable, Sendable {
        var cpuNanos: UInt64 = 0
        var interruptWakeups: UInt64 = 0
        var idleWakeups: UInt64 = 0
        var energyNanojoules: UInt64 = 0

        static func - (a: Power, b: Power) -> Power {
            Power(cpuNanos: a.cpuNanos &- b.cpuNanos, interruptWakeups: a.interruptWakeups &- b.interruptWakeups,
                  idleWakeups: a.idleWakeups &- b.idleWakeups, energyNanojoules: a.energyNanojoules &- b.energyNanojoules)
        }

        /// This process's numbers; zero when the kernel does not answer.
        static func now() -> Power {
            var info = task_power_info_v2()
            var count = mach_msg_type_number_t(MemoryLayout<task_power_info_v2>.size / MemoryLayout<natural_t>.size)
            let result = withUnsafeMutablePointer(to: &info) {
                $0.withMemoryRebound(to: integer_t.self, capacity: Int(count)) {
                    task_info(mach_task_self_, task_flavor_t(TASK_POWER_INFO_V2), $0, &count)
                }
            }
            guard result == KERN_SUCCESS else { return Power() }
            // CPU times are in Mach absolute time units (125/3 ns on Apple silicon).
            let ticks = info.cpu_energy.total_user &+ info.cpu_energy.total_system
            #if arch(arm64) || arch(arm)
            let energy = info.task_energy
            #else
            let energy: UInt64 = 0
            #endif
            return Power(cpuNanos: ticks &* UInt64(timebase.numer) / UInt64(max(timebase.denom, 1)),
                         interruptWakeups: info.cpu_energy.task_interrupt_wakeups,
                         idleWakeups: info.cpu_energy.task_platform_idle_wakeups, energyNanojoules: energy)
        }

        private static let timebase: mach_timebase_info_data_t = {
            var tb = mach_timebase_info_data_t()
            mach_timebase_info(&tb)
            return tb
        }()
    }

    /// What the device says when a summary is closed (read on the main actor by `DevMode`).
    struct Device: Equatable, Sendable {
        var thermal = "nominal"
        var battery: Double?
        var lowPower = false
    }

    /// The network path (`NWPathMonitor`), as a minute's line carries it.
    struct NetPath: Codable, Equatable, Sendable {
        var type = "none"
        var expensive = false
        var constrained = false
    }

    private struct Route: Hashable, Comparable {
        var method: String
        var pattern: String
        static func < (a: Route, b: Route) -> Bool { (a.pattern, a.method) < (b.pattern, b.method) }
    }

    private struct RouteStat {
        var count = 0
        var bytesIn: Int64 = 0
        var bytesOut: Int64 = 0
        var millis: [Int] = []
        var status: [String: Int] = [:]
    }

    /// One summary being counted: a minute (or a stretch in the background) or a foreground period.
    private struct Tally {
        var from: Date
        var power: Power
        var device: Device
        var background: Bool
        var routes: [Route: RouteStat] = [:]
        var frames: [String: Int] = [:]
        var frameBytes: Int64 = 0
        var connects = 0
        var drops = 0
        var pages: [String: TimeInterval] = [:]

        var requests: Int { routes.values.reduce(0) { $0 + $1.count } }
        var bytesIn: Int64 { routes.values.reduce(0) { $0 + $1.bytesIn } }
        var bytesOut: Int64 { routes.values.reduce(0) { $0 + $1.bytesOut } }
        var frameCount: Int { frames.values.reduce(0, +) }
    }

    private struct State {
        var on = false
        var active = false
        var minute: Tally?
        var period: Tally?
        var page = "launch"
        var pageSince = Date.distantPast
        var path = NetPath()
        var buffer: [String] = []
        /// Request errors already written this minute (route and status): one line each, the rest only counted.
        var errorsNoted: Set<String> = []
        /// The connection tally's opens in this period in front (#11390).
        var links = ConnectionOpens()
    }

    static let shared = DevRecorder(store: DevStore.shared)

    let store: DevStore
    private let clock: @Sendable () -> Date
    private let power: @Sendable () -> Power
    private let build: String
    private let state = OSAllocatedUnfairLock(initialState: State())
    /// Where lines are written: never on the caller's thread (the main actor rolls the minutes).
    private let writer = DispatchQueue(label: "persona.dev-mode.write", qos: .utility)
    /// The most lines kept before they are written even without a roll.
    static let bufferLimit = 200

    init(store: DevStore, clock: @escaping @Sendable () -> Date = { Date() }, power: @escaping @Sendable () -> Power = { Power.now() },
         build: String = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "") {
        self.store = store
        self.clock = clock
        self.power = power
        self.build = build
    }

    var isOn: Bool { state.withLock { $0.on } }

    // MARK: Switching

    /// Starts counting, in front (`active`) or not, from `device` as it is now. `fresh`: at launch, so a period in front
    /// is counted for the connection tally from its start; turned on by hand, the period under way is not.
    func start(active: Bool, page: String, device: Device, fresh: Bool = true) {
        let now = clock(), p = power()
        state.withLock { s in
            guard !s.on else { return }
            s.on = true
            s.active = active
            s.links.start(active: active, fresh: fresh, at: now)
            s.minute = Tally(from: now, power: p, device: device, background: !active)
            s.period = active ? Tally(from: now, power: p, device: device, background: false) : nil
            s.page = DevScrub.word(page)
            s.pageSince = now
            s.buffer.append(Self.line(now, "app", ["event": "recording", "active": active, "build": build]))
        }
    }

    /// Stops counting and drops what was not written; the caller deletes the files.
    func stop() {
        state.withLock { $0 = State() }
    }

    /// Runs `work` after the writes queued so far (deleting the files after the last of them).
    func afterWrites(_ work: @escaping @Sendable () -> Void) { writer.async(execute: work) }

    // MARK: What the app tells it

    /// The app came to the front or left it: the open summary closes, and a foreground period starts or ends.
    func lifecycle(active: Bool, device: Device) {
        let now = clock(), p = power()
        let (lines, tally): ([String], ConnectionTally) = state.withLock { s in
            guard s.on, s.active != active else { return ([], ConnectionTally()) }
            s.active = active
            var tally = ConnectionTally()
            if active { s.links.front(at: now) } else { tally = s.links.back(at: now) }
            Self.settlePage(&s, now)
            var out: [String] = []
            if let m = s.minute, m.background == active { out.append(close(m, now, p, device, s)) }
            if active {
                if s.minute?.background != false { s.minute = Tally(from: now, power: p, device: device, background: false) }
                s.period = Tally(from: now, power: p, device: device, background: false)
                out.append(Self.line(now, "app", ["event": "foreground"]))
            } else {
                if let period = s.period { out.append(foreground(period, now, p, device)) }
                s.period = nil
                if s.minute?.background != true { s.minute = Tally(from: now, power: p, device: device, background: true) }
                out.append(Self.line(now, "app", ["event": "background"]))
            }
            s.errorsNoted = []
            return (out, tally)
        }
        append(lines, flush: true)
        if !tally.isEmpty {
            let store = store
            writer.async { store.addConnections(tally, now: now) }
        }
    }

    /// Once a minute while active: the minute's line, and a fresh minute.
    func roll(device: Device) {
        let now = clock(), p = power()
        let lines: [String] = state.withLock { s in
            guard s.on, s.minute != nil else { return [] }
            Self.settlePage(&s, now)
            guard let m = s.minute else { return [] }
            let out = close(m, now, p, device, s)
            s.minute = Tally(from: now, power: p, device: device, background: m.background)
            s.errorsNoted = []
            return [out]
        }
        append(lines, flush: true)
    }

    /// The screen in front now (`DevMode.page`), a code-chosen name.
    func page(_ name: String) {
        let now = clock(), name = DevScrub.word(name)
        let lines: [String] = state.withLock { s in
            guard s.on, name != s.page else { return [] }
            Self.settlePage(&s, now)
            s.page = name
            return [Self.line(now, "page", ["page": name])]
        }
        append(lines)
    }

    /// A change of thermal state, Low Power Mode or battery level: one line saying what and on which screen.
    func device(_ kind: String, _ fields: [String: Any]) {
        let now = clock()
        guard let page: String = state.withLock({ $0.on ? $0.page : nil }) else { return }
        var f = fields
        f["page"] = page
        append([Self.line(now, kind, f)])
    }

    func path(_ path: NetPath) {
        let now = clock()
        let lines: [String] = state.withLock { s in
            guard s.on else { return [] }
            s.links.path(path.type != "none", at: now)
            guard path != s.path else { return [] }
            s.path = path
            return [Self.line(now, "path", ["type": path.type, "expensive": path.expensive, "constrained": path.constrained])]
        }
        append(lines)
    }

    /// A finished request to the hub (`HubRedirects.finished`).
    func request(method: String, url: URL?, status: Int?, bytesIn: Int64, bytesOut: Int64, millis: Int) {
        let route = Route(method: DevScrub.word(method.uppercased()), pattern: DevScrub.route(url?.path ?? "/"))
        let code = status.map(String.init) ?? "failed"
        let now = clock()
        let lines: [String] = state.withLock { s in
            guard s.on else { return [] }
            Self.count(&s.minute, route, code, bytesIn, bytesOut, millis)
            Self.count(&s.period, route, code, bytesIn, bytesOut, millis)
            guard status.map({ $0 >= 400 }) ?? true else { return [] }
            let key = "\(route.method) \(route.pattern) \(code)"
            guard s.errorsNoted.insert(key).inserted else { return [] }
            return [Self.line(now, "request_error", ["route": "\(route.method) \(route.pattern)", "status": code, "page": s.page])]
        }
        append(lines)
    }

    func request(_ task: URLSessionTask, _ metrics: URLSessionTaskMetrics) {
        let last = metrics.transactionMetrics.last
        let bytesIn = (last?.countOfResponseHeaderBytesReceived ?? 0) + (last?.countOfResponseBodyBytesReceived ?? 0)
        let bytesOut = (last?.countOfRequestHeaderBytesSent ?? 0) + (last?.countOfRequestBodyBytesSent ?? 0)
        let status = (task.response as? HTTPURLResponse)?.statusCode
        request(method: task.originalRequest?.httpMethod ?? "GET", url: task.originalRequest?.url, status: status,
                bytesIn: bytesIn, bytesOut: bytesOut, millis: Int((metrics.taskInterval.duration * 1000).rounded()))
    }

    // MARK: DevProbe.Sink

    func frame(type: String, bytes: Int) {
        let type = DevScrub.word(type.isEmpty ? "unknown" : type)
        state.withLock { s in
            guard s.on else { return }
            s.minute?.frames[type, default: 0] += 1
            s.minute?.frameBytes += Int64(bytes)
            s.period?.frames[type, default: 0] += 1
            s.period?.frameBytes += Int64(bytes)
        }
    }

    func socket(_ event: DevProbe.Socket) {
        let now = clock()
        let lines: [String] = state.withLock { s in
            guard s.on else { return [] }
            switch event {
            case .connected: s.minute?.connects += 1; s.period?.connects += 1
            case .dropped: s.minute?.drops += 1; s.period?.drops += 1
            }
            return [Self.line(now, "socket", ["event": event.rawValue, "page": s.page])]
        }
        append(lines)
    }

    /// A hub group's groups, attempts and connects, for the connection tally (#11390).
    func link(_ event: DevProbe.Link) {
        let now = clock()
        state.withLock { s in
            guard s.on else { return }
            s.links.link(event, at: now)
        }
    }

    func error(_ report: ExtensionErrorReport) {
        guard isOn else { return }
        let store = store, clean = DevStore.scrubbed(report)
        // A crash's report is written at once: it is the last thing the app does.
        if report.kind == "crash" { writer.sync { store.appendError(clean) } } else { writer.async { store.appendError(clean) } }
    }

    // MARK: Writing

    /// Writes what is buffered (the app is leaving).
    func flush() { append([], flush: true) }

    /// Writes what is buffered and waits for every write: before an export or a size reads the files. Not on the main
    /// actor.
    func settle() {
        flush()
        writer.sync {}
    }

    private func append(_ lines: [String], flush: Bool = false) {
        let batch: [String] = state.withLock { s in
            guard s.on else { return [] }
            s.buffer += lines
            guard flush || s.buffer.count >= Self.bufferLimit else { return [] }
            defer { s.buffer = [] }
            return s.buffer
        }
        guard !batch.isEmpty else { return }
        let store = store, now = clock()
        writer.async { store.appendEvents(batch, at: now) }
    }

    private static func settlePage(_ s: inout State, _ now: Date) {
        let spent = max(0, now.timeIntervalSince(s.pageSince))
        if spent > 0, s.pageSince != .distantPast {
            s.minute?.pages[s.page, default: 0] += spent
            s.period?.pages[s.page, default: 0] += spent
        }
        s.pageSince = now
    }

    private static func count(_ t: inout Tally?, _ route: Route, _ code: String, _ bytesIn: Int64, _ bytesOut: Int64, _ millis: Int) {
        guard t != nil else { return }
        var stat = t!.routes[route] ?? RouteStat()
        stat.count += 1
        stat.bytesIn += bytesIn
        stat.bytesOut += bytesOut
        if stat.millis.count < 2000 { stat.millis.append(millis) }
        stat.status[code, default: 0] += 1
        t!.routes[route] = stat
    }

    private func close(_ m: Tally, _ now: Date, _ p: Power, _ device: Device, _ s: State) -> String {
        let used = p - m.power
        let http: [[String: Any]] = m.routes.keys.sorted().map { r in
            let stat = m.routes[r]!
            let sorted = stat.millis.sorted()
            return ["route": "\(r.method) \(r.pattern)", "n": stat.count, "in": stat.bytesIn, "out": stat.bytesOut,
                    "p50_ms": sorted.isEmpty ? 0 : sorted[sorted.count / 2], "max_ms": sorted.last ?? 0, "status": stat.status]
        }
        var f: [String: Any] = [
            "from": Self.stamp(m.from), "to": Self.stamp(now), "background": m.background, "build": build,
            "page": s.page, "pages": Self.seconds(m.pages),
            "net": ["type": s.path.type, "expensive": s.path.expensive, "constrained": s.path.constrained],
            "cpu_ms": Double(used.cpuNanos / 1000) / 1000, "wakeups": ["interrupt": used.interruptWakeups, "idle": used.idleWakeups],
            "energy_mj": Double(used.energyNanojoules / 1000) / 1000,
            "requests": m.requests, "http": http,
            "ws": ["frames": m.frames, "bytes": m.frameBytes, "connects": m.connects, "drops": m.drops],
            "thermal": device.thermal, "low_power": device.lowPower,
        ]
        if let b = device.battery { f["battery"] = b }
        return Self.line(now, "minute", f)
    }

    private func foreground(_ t: Tally, _ now: Date, _ p: Power, _ device: Device) -> String {
        let used = p - t.power
        var f: [String: Any] = [
            "from": Self.stamp(t.from), "to": Self.stamp(now), "seconds": Int(now.timeIntervalSince(t.from).rounded()), "build": build,
            "cpu_s": Double(used.cpuNanos / 1_000_000) / 1000, "wakeups": ["interrupt": used.interruptWakeups, "idle": used.idleWakeups],
            "energy_j": Double(used.energyNanojoules / 1_000_000) / 1000,
            "requests": t.requests, "bytes_in": t.bytesIn, "bytes_out": t.bytesOut, "ws_frames": t.frameCount,
            "ws_connects": t.connects, "ws_drops": t.drops, "pages": Self.seconds(t.pages),
            "thermal": ["start": t.device.thermal, "end": device.thermal],
        ]
        if t.device.battery != nil || device.battery != nil {
            f["battery"] = ["start": t.device.battery.map { $0 as Any } ?? NSNull(), "end": device.battery.map { $0 as Any } ?? NSNull()]
        }
        return Self.line(now, "foreground", f)
    }

    private static func seconds(_ pages: [String: TimeInterval]) -> [String: Int] {
        pages.mapValues { Int($0.rounded()) }.filter { $0.value > 0 }
    }

    // MARK: Lines

    static func stamp(_ date: Date) -> String {
        date.formatted(Date.ISO8601FormatStyle(includingFractionalSeconds: true))
    }

    /// One JSON line: `t`, `kind` and the fields, keys sorted.
    static func line(_ at: Date, _ kind: String, _ fields: [String: Any]) -> String {
        var all = fields
        all["t"] = stamp(at)
        all["kind"] = kind
        let clean = all.mapValues(jsonSafe)
        guard let data = try? JSONSerialization.data(withJSONObject: clean, options: [.sortedKeys, .withoutEscapingSlashes]),
              let text = String(data: data, encoding: .utf8) else { return "{\"kind\":\"\(kind)\"}" }
        return text
    }

    private static func jsonSafe(_ value: Any) -> Any {
        switch value {
        case let v as [String: Any]: return v.mapValues(jsonSafe)
        case let v as [Any]: return v.map(jsonSafe)
        case let v as Double: return v.isFinite ? v : 0
        case let v as UInt64: return NSNumber(value: v)
        case let v as Int64: return NSNumber(value: v)
        default: return value
        }
    }
}
