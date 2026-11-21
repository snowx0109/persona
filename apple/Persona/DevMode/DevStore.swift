import Foundation
import os
import PersonaWire

/// Where Developing Persona keeps what it recorded (#9460): `Application Support/dev-mode`, out of backups. Events go
/// into one file an hour (`events/2026-11-17T10.jsonl`, UTC), so the oldest go whole; errors into `errors.jsonl`;
/// MetricKit's payloads into `metrickit/` as delivered; the connection tally into `connections.json` (#11390). Kept at
/// most 24 hours of events, 14 days of MetricKit, 7 days of errors, 8 weeks of the tally, and 50 MB in all, the oldest
/// dropped first.
nonisolated final class DevStore: @unchecked Sendable {
    static let defaultMaxBytes: Int64 = 50 << 20
    static let eventAge: TimeInterval = 24 * 3600
    static let metricAge: TimeInterval = 14 * 24 * 3600
    static let errorAge: TimeInterval = 7 * 24 * 3600

    static let shared = DevStore(directory: URL.applicationSupportDirectory.appending(path: "dev-mode", directoryHint: .isDirectory))

    let directory: URL
    /// The most kept in all.
    let maxBytes: Int64
    private let lock = OSAllocatedUnfairLock()
    private let fm = FileManager.default

    init(directory: URL, maxBytes: Int64 = DevStore.defaultMaxBytes) {
        self.directory = directory
        self.maxBytes = maxBytes
    }

    var events: URL { directory.appending(path: "events", directoryHint: .isDirectory) }
    var metrics: URL { directory.appending(path: "metrickit", directoryHint: .isDirectory) }
    var errors: URL { directory.appending(path: "errors.jsonl") }
    var connections: URL { directory.appending(path: "connections.json") }

    // MARK: Writing

    /// Appends `lines` to the hour's file and drops what is past its age or over the size.
    func appendEvents(_ lines: [String], at now: Date) {
        guard !lines.isEmpty else { return }
        let name = Self.hourName(now)
        lock.withLock {
            append(lines.joined(separator: "\n") + "\n", to: events.appending(path: name))
            trimRecordsLocked(now)
        }
    }

    func appendError(_ report: ExtensionErrorReport) {
        guard let json = try? JSON.encoder.encode(report), let line = String(data: json, encoding: .utf8) else { return }
        lock.withLock { append(line.replacingOccurrences(of: "\n", with: " ") + "\n", to: errors) }
    }

    /// Writes a MetricKit payload's JSON as `name` unless it is there already (`pastPayloads` repeats them).
    func writeMetric(_ data: Data, name: String) {
        lock.withLock {
            let url = metrics.appending(path: name)
            guard !fm.fileExists(atPath: url.path(percentEncoded: false)) else { return }
            try? fm.createDirectory(at: metrics, withIntermediateDirectories: true)
            try? data.write(to: url, options: .atomic)
        }
    }

    /// Adds a period's opens to the connection tally and keeps its last `ConnectionTally.keptWeeks` weeks. On the
    /// recorder's queue, never the main actor.
    func addConnections(_ delta: ConnectionTally, now: Date) {
        lock.withLock {
            var tally = (try? Data(contentsOf: connections)).flatMap(ConnectionTally.decoded) ?? ConnectionTally()
            tally.merge(delta)
            tally.trim(now: now)
            guard let data = tally.encoded() else { return }
            try? fm.createDirectory(at: directory, withIntermediateDirectories: true)
            try? data.write(to: connections, options: .atomic)
            excludeFromBackup()
        }
    }

    /// Everything recorded goes (the switch was turned off).
    func clear() {
        lock.withLock { _ = try? fm.removeItem(at: directory) }
    }

    // MARK: Reading

    /// The bytes on disk.
    func recordedBytes() -> Int64 {
        lock.withLock { recordFiles(under: directory).reduce(0) { $0 + $1.size } }
    }

    /// The event files, oldest first.
    func eventFiles() -> [URL] {
        lock.withLock { recordFiles(under: events).map(\.url).sorted { $0.lastPathComponent < $1.lastPathComponent } }
    }

    func metricFiles() -> [URL] {
        lock.withLock { recordFiles(under: metrics).map(\.url).sorted { $0.lastPathComponent < $1.lastPathComponent } }
    }

    /// The connection tally's weeks still kept at `now`; empty when nothing was counted.
    func connectionTally(now: Date) -> ConnectionTally {
        lock.withLock {
            var tally = (try? Data(contentsOf: connections)).flatMap(ConnectionTally.decoded) ?? ConnectionTally()
            tally.trim(now: now)
            return tally
        }
    }

    /// The kept error lines younger than `errorAge`, oldest first.
    func errorLines(now: Date) -> [String] {
        lock.withLock {
            guard let text = try? String(contentsOf: errors, encoding: .utf8) else { return [] }
            let cutoff = now.addingTimeInterval(-Self.errorAge)
            return text.split(separator: "\n").map(String.init).filter { line in
                guard let data = line.data(using: .utf8),
                      let report = try? JSON.decoder.decode(ExtensionErrorReport.self, from: data),
                      let at = ErrorReports.date(report.time) else { return false }
                return at >= cutoff
            }
        }
    }

    // MARK: Keeping it small

    func trimRecords(now: Date) { lock.withLock { trimRecordsLocked(now) } }

    private func trimRecordsLocked(_ now: Date) {
        let eventCut = now.addingTimeInterval(-Self.eventAge - 3600) // a file covers up to an hour after its name
        var kept = recordFiles(under: events).filter { f in
            guard let at = Self.hourDate(f.url.lastPathComponent), at >= eventCut else { try? fm.removeItem(at: f.url); return false }
            return true
        }
        var metricKept = recordFiles(under: metrics).filter { f in
            guard f.modified >= now.addingTimeInterval(-Self.metricAge) else { try? fm.removeItem(at: f.url); return false }
            return true
        }
        let errorBytes = (try? errors.resourceValues(forKeys: [.fileSizeKey]).fileSize).map(Int64.init) ?? 0
        var total = kept.reduce(0) { $0 + $1.size } + metricKept.reduce(0) { $0 + $1.size } + errorBytes
        // Over the size: the oldest event hours first (never the hour being written), then the oldest MetricKit files.
        kept.sort { $0.url.lastPathComponent < $1.url.lastPathComponent }
        while total > maxBytes, kept.count > 1 {
            let f = kept.removeFirst()
            try? fm.removeItem(at: f.url)
            total -= f.size
        }
        metricKept.sort { $0.modified < $1.modified }
        while total > maxBytes, !metricKept.isEmpty {
            let f = metricKept.removeFirst()
            try? fm.removeItem(at: f.url)
            total -= f.size
        }
        if total > maxBytes { try? fm.removeItem(at: errors) }
    }

    // MARK: Files

    private struct File { var url: URL; var size: Int64; var modified: Date }

    private func recordFiles(under dir: URL) -> [File] {
        let keys: [URLResourceKey] = [.fileSizeKey, .contentModificationDateKey, .isRegularFileKey]
        guard let walk = fm.enumerator(at: dir, includingPropertiesForKeys: keys) else { return [] }
        return walk.compactMap { item in
            guard let url = item as? URL, let v = try? url.resourceValues(forKeys: Set(keys)), v.isRegularFile == true else { return nil }
            return File(url: url, size: Int64(v.fileSize ?? 0), modified: v.contentModificationDate ?? .distantPast)
        }
    }

    private func append(_ text: String, to url: URL) {
        guard let data = text.data(using: .utf8) else { return }
        try? fm.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !fm.fileExists(atPath: url.path(percentEncoded: false)) {
            try? data.write(to: url)
            excludeFromBackup()
            return
        }
        guard let handle = try? FileHandle(forWritingTo: url) else { return }
        defer { try? handle.close() }
        _ = try? handle.seekToEnd()
        try? handle.write(contentsOf: data)
    }

    private func excludeFromBackup() {
        var dir = directory
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? dir.setResourceValues(values)
    }

    // MARK: Names

    private static let hourFormat: Date.VerbatimFormatStyle = .init(
        format: "\(year: .defaultDigits)-\(month: .twoDigits)-\(day: .twoDigits)T\(hour: .twoDigits(clock: .twentyFourHour, hourCycle: .zeroBased))",
        timeZone: .gmt, calendar: Calendar(identifier: .gregorian))

    static func hourName(_ date: Date) -> String { date.formatted(hourFormat) + ".jsonl" }

    static func hourDate(_ name: String) -> Date? {
        guard name.hasSuffix(".jsonl") else { return nil }
        return try? Date(String(name.dropLast(6)) + ":00:00Z", strategy: .iso8601)
    }

    /// A report as the package may hold it: its message and exception values through `DevScrub.text`.
    static func scrubbed(_ report: ExtensionErrorReport) -> ExtensionErrorReport {
        var r = report
        r.message = r.message.map(DevScrub.text)
        r.exceptions = r.exceptions?.map { e in
            var e = e
            e.value = e.value.map(DevScrub.text)
            return e
        }
        r.breadcrumbs = r.breadcrumbs?.map { b in
            var b = b
            b.message = b.message.map(DevScrub.text)
            return b
        }
        return r
    }
}
