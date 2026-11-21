import Foundation
import OSLog
import PersonaWire

/// Developing Persona's export (#9460, docs/operations/developing-persona.md "The package"): one zip an Agent can read,
/// `README.md` first. Built off the main actor from what `DevStore` kept, filtered by the categories the user left
/// ticked; the device and app versions always go in.
nonisolated enum DevPackage {
    enum Category: String, CaseIterable, Codable, Sendable { case logs, errors, energy }

    /// Event kinds by category; a kind not listed is a log line.
    static let energyKinds: Set<String> = ["minute", "foreground", "thermal", "power", "battery", "path"]

    static let format = "persona-diagnostics"
    static let version = 1
    /// The most log lines read from this launch's log.
    static let maxLogLines = 5000

    struct Device: Sendable {
        var platform = ErrorReports.source
        var os = ErrorReports.os
        var osVersion = ErrorReports.osVersion
        var model = ErrorReports.deviceModel
        var appVersion = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? ""
        var build = Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? ""
        var commit = Bundle.main.object(forInfoDictionaryKey: "PersonaCommit") as? String ?? ""
        var utcOffsetMinutes = TimeZone.current.secondsFromGMT() / 60
    }

    struct Summary: Equatable, Sendable {
        var events = 0
        var errors = 0
        var metricPayloads = 0
        var diagnosticPayloads = 0
        var logLines = 0
        /// Weeks in the connection tally (#11390).
        var connectionWeeks = 0
    }

    /// The file name: `persona-diagnostics-ios-20261117-1032.zip`, in local time.
    static func fileName(_ device: Device, at now: Date) -> String {
        let stamp = now.formatted(Date.VerbatimFormatStyle(
            format: "\(year: .defaultDigits)\(month: .twoDigits)\(day: .twoDigits)-\(hour: .twoDigits(clock: .twentyFourHour, hourCycle: .zeroBased))\(minute: .twoDigits)",
            timeZone: .current, calendar: Calendar(identifier: .gregorian)))
        return "persona-diagnostics-\(device.platform)-\(stamp).zip"
    }

    /// Writes the package's files into `folder` (made empty first) and says what went in.
    @discardableResult
    static func writePackage(into folder: URL, store: DevStore, categories: Set<Category>, since: Date?, device: Device = Device(),
                      log: [String]? = nil, now: Date = Date()) throws -> Summary {
        let fm = FileManager.default
        try? fm.removeItem(at: folder)
        try fm.createDirectory(at: folder, withIntermediateDirectories: true)
        store.trimRecords(now: now)
        var summary = Summary()

        // Events: the hour files in order, the lines of the ticked categories.
        var lines: [String] = []
        var from: Date?
        for file in store.eventFiles() {
            guard let text = try? String(contentsOf: file, encoding: .utf8) else { continue }
            for line in text.split(separator: "\n") where !line.isEmpty {
                let kind = Self.kind(line)
                let category: Category = energyKinds.contains(kind) ? .energy : .logs
                guard categories.contains(category) else { continue }
                lines.append(String(line))
            }
            if from == nil { from = DevStore.hourDate(file.lastPathComponent) }
        }
        summary.events = lines.count
        try (lines.joined(separator: "\n") + (lines.isEmpty ? "" : "\n")).write(to: folder.appending(path: "events.jsonl"), atomically: true, encoding: .utf8)

        if categories.contains(.errors) {
            let errors = store.errorLines(now: now)
            summary.errors = errors.count
            try (errors.joined(separator: "\n") + (errors.isEmpty ? "" : "\n")).write(to: folder.appending(path: "errors.jsonl"), atomically: true, encoding: .utf8)
        }

        let metrics = store.metricFiles().filter { f in
            let name = f.lastPathComponent
            return (name.hasPrefix("metrics-") && categories.contains(.energy)) || (name.hasPrefix("diagnostics-") && categories.contains(.errors))
        }
        if !metrics.isEmpty {
            let dir = folder.appending(path: "metrickit", directoryHint: .isDirectory)
            try fm.createDirectory(at: dir, withIntermediateDirectories: true)
            for f in metrics {
                try fm.copyItem(at: f, to: dir.appending(path: f.lastPathComponent))
                if f.lastPathComponent.hasPrefix("metrics-") { summary.metricPayloads += 1 } else { summary.diagnosticPayloads += 1 }
            }
        }

        if categories.contains(.logs) {
            let log = log ?? Self.processLog(since: since)
            summary.logLines = log.count
            try (log.joined(separator: "\n") + (log.isEmpty ? "" : "\n")).write(to: folder.appending(path: "log.txt"), atomically: true, encoding: .utf8)
            // The connection tally (#11390) goes with the logs, as the socket's own lines do; only when something was counted.
            let tally = store.connectionTally(now: now)
            if !tally.isEmpty, let data = tally.encoded() {
                summary.connectionWeeks = tally.weeks.count
                try data.write(to: folder.appending(path: "connections.json"))
            }
        }

        let window = ["from": (since.map { max($0, from ?? $0) } ?? from).map(DevRecorder.stamp) as Any, "to": DevRecorder.stamp(now)]
        let manifest: [String: Any] = [
            "format": format, "version": version, "platform": device.platform, "os": device.os, "os_version": device.osVersion,
            "device_model": device.model, "app_version": device.appVersion, "build": device.build, "commit": device.commit,
            "utc_offset_minutes": device.utcOffsetMinutes, "recording_since": since.map(DevRecorder.stamp) as Any,
            "exported_at": DevRecorder.stamp(now), "window": window,
            "categories": (["device"] + Category.allCases.filter(categories.contains).map(\.rawValue)),
            "counts": ["events": summary.events, "errors": summary.errors, "metric_payloads": summary.metricPayloads,
                       "diagnostic_payloads": summary.diagnosticPayloads, "log_lines": summary.logLines,
                       "connection_weeks": summary.connectionWeeks],
        ]
        let data = try JSONSerialization.data(withJSONObject: Self.nulls(manifest), options: [.sortedKeys, .prettyPrinted, .withoutEscapingSlashes])
        try data.write(to: folder.appending(path: "manifest.json"))
        try readme(categories).write(to: folder.appending(path: "README.md"), atomically: true, encoding: .utf8)
        return summary
    }

    /// Writes the package and zips it: the zip's URL in a temporary folder of its own.
    static func exportZip(store: DevStore, categories: Set<Category>, since: Date?, device: Device = Device(), now: Date = Date()) throws -> URL {
        let fm = FileManager.default
        let work = fm.temporaryDirectory.appending(path: "dev-mode-export-\(UUID().uuidString)", directoryHint: .isDirectory)
        let name = fileName(device, at: now)
        let folder = work.appending(path: String(name.dropLast(4)), directoryHint: .isDirectory)
        try writePackage(into: folder, store: store, categories: categories, since: since, device: device, now: now)
        let zip = work.appending(path: name)
        var failure: Error?
        var coordinated: NSError?
        NSFileCoordinator().coordinate(readingItemAt: folder, options: .forUploading, error: &coordinated) { zipped in
            do { try fm.copyItem(at: zipped, to: zip) } catch { failure = error }
        }
        if let e = coordinated ?? failure { throw e }
        try? fm.removeItem(at: folder)
        return zip
    }

    /// This launch's own log lines (subsystem `persona`), oldest first, scrubbed; the system keeps no earlier launch's
    /// for the app to read.
    static func processLog(since: Date?) -> [String] {
        guard let store = try? OSLogStore(scope: .currentProcessIdentifier) else { return [] }
        let start = store.position(date: since ?? Date.distantPast)
        guard let entries = try? store.getEntries(at: start, matching: NSPredicate(format: "subsystem == %@", "persona")) else { return [] }
        var out: [String] = []
        for case let entry as OSLogEntryLog in entries {
            out.append("\(DevRecorder.stamp(entry.date)) [\(DevScrub.word(entry.category))] \(level(entry.level)): \(DevScrub.text(entry.composedMessage))")
        }
        return Array(out.suffix(maxLogLines))
    }

    private static func level(_ l: OSLogEntryLog.Level) -> String {
        switch l {
        case .debug: "debug"
        case .info: "info"
        case .notice: "notice"
        case .error: "error"
        case .fault: "fault"
        default: "log"
        }
    }

    private static func kind(_ line: Substring) -> String {
        guard let r = line.range(of: "\"kind\":\"") else { return "" }
        let rest = line[r.upperBound...]
        return String(rest.prefix { $0 != "\"" })
    }

    private static func nulls(_ value: Any) -> Any {
        switch value {
        case let d as [String: Any]: return d.mapValues(nulls)
        case let l as [Any]: return l.map(nulls)
        default:
            let m = Mirror(reflecting: value)
            if m.displayStyle == .optional { return m.children.first.map { nulls($0.value) } ?? NSNull() }
            return value
        }
    }

    /// The package's README, for the Agent that reads it (not shown in the app, so not localized).
    static func readme(_ categories: Set<Category>) -> String {
        var s = """
        # Persona diagnostics

        Recorded on one device by Persona's "Developing Persona" switch (Settings › About) and exported by its owner, to find \
        out why the app uses energy, fails or crashes. Only numbers and words chosen by the app's code are recorded: no message \
        text, knowledge base content, keys, tokens, passwords, cookies, pairing links, device name, hub address, machine names \
        or accounts. Free text (error messages, log lines) was scrubbed: URLs are cut to their path, secrets, e-mail and IP \
        addresses, host names and long keys are replaced with `<redacted>`, `<email>`, `<ip>`, `<host>`.

        Times are UTC (ISO 8601); `manifest.json` has the device's offset (`utc_offset_minutes`).

        ## Files

        - `manifest.json`: the device model, system, app version, build and commit, the time window, the categories included and counts.
        - `events.jsonl`: one JSON object a line, oldest first, by `kind`:
          - `minute`: one a minute while the app is in front, one for each stretch in the background (`background: true`, \
        from the moment it left to the moment it came back). `cpu_ms`, `wakeups` (`interrupt`, `idle`) and `energy_mj` are \
        the app process's own use in that span (`task_info(TASK_POWER_INFO_V2)`); `http` lists requests to the hub by \
        route pattern (`:id` stands for ids) with count, bytes in and out, median and longest time and status codes \
        (`failed`: no answer); `ws` counts the hub WebSocket's frames by message type, its bytes, connects and drops; \
        `pages` the seconds on each screen; `net` the network path; `thermal`, `battery` (0–1, while in front) and `low_power`.
          - `foreground`: one per period in front: its length, CPU seconds, wakeups, energy, requests, bytes, frames, \
        screens and their seconds, thermal state and battery at its start and end.
          - `app` (launch, foreground, background), `page` (the screen in front changed), `thermal`, `power` (Low Power \
        Mode), `battery`, `path` (network), `socket` (the hub WebSocket connected or dropped), `request_error` (the first \
        of each failing route and status in a minute).
        - `errors.jsonl`: the app's error reports (uncaught exceptions, with call stacks), in the hub's error report format.
        - `metrickit/metrics-*.json`: MetricKit's daily `MXMetricPayload`s as the system delivered them (`jsonRepresentation()`): \
        cumulative CPU time and instructions, foreground / background run time, network transfer, disk writes, GPU, \
        display, scroll hitches, hang time, memory, app exits by reason, cellular conditions.
        - `metrickit/diagnostics-*.json`: `MXDiagnosticPayload`s: crashes, hangs, CPU and disk-write exceptions, with \
        unsymbolicated call stacks (binary name, UUID and offset); the app's dSYMs for `build` / `commit` symbolicate them.
        - `log.txt`: this launch's own log lines (subsystem `persona`); values the app marks private show as `<private>`.
        - `connections.json` (with the logs, when something was counted): the connection tally, one entry per ISO week \
        in UTC (`week`, its Monday `starts`), the last 8 kept. An open is one period with the app in front per hub group, \
        from the moment the phone had a network path (Demo Mode left out); a period already under way when recording was \
        turned on is not counted, nor one that ended with the app in front (a crash or a kill). `recorded_seconds`: seconds in front with recording on that week (0 or missing: \
        nothing was recorded, not "no failures"). `not_connected`: opens whose socket never connected, by the most \
        telling outcome its addresses had (`not_your_hub`: an added address answered as another hub; \
        `certificate_not_trusted`: TLS failed the system's trust; `couldnt_connect`: refused, timed out, not found, \
        lost, or not a healthy hub answer; `unknown`: cut short, or the period ended before any try failed). \
        `connected`: opens that connected, by the address in use: `trust` (`system`: HTTPS straight to it; \
        `plain_http`: plain HTTP straight to it; `tunnel`: through a tunnel or proxy, plain HTTP to its loopback end), \
        `access_token` (it carries its own Cloudflare Access token), `route` (`direct`, `ssh_tunnel`, `in_app_tunnel`, \
        `local_proxy`), `address` (`paired`, `added`, `mesh`), `hello_fingerprint` (the hub's hello lists a key \
        fingerprint for itself) and `opens`. No address, host, port, hub id or fingerprint is in it.

        """
        let missing = DevPackage.Category.allCases.filter { !categories.contains($0) }
        if !missing.isEmpty {
            s += "\nLeft out by the owner: \(missing.map(\.rawValue).joined(separator: ", ")).\n"
        }
        return s
    }
}
