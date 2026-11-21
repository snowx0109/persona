import Foundation
import Network
import PersonaWire
#if canImport(UIKit)
import UIKit
#endif

/// Developing Persona (Settings › About, #9460, docs/operations/developing-persona.md): the device's own switch. While
/// it is on the app records its logs, errors and energy use on the device (`DevRecorder`, `DevMetrics`) for the owner
/// to export; turning it off deletes what was recorded. Off by default, never synced. Costs nothing while off but a
/// string set on each tab change; while on, one wakeup a minute while the app is in front and none in the background.
@MainActor @Observable
final class DevMode {
    static let shared = DevMode()
    static let onKey = "devMode.on"
    static let sinceKey = "devMode.since"
    static let callHandleKey = "devMode.callHandle"

    private(set) var isOn: Bool
    /// When the switch was turned on.
    private(set) var since: Date?
    /// Call handle (#9887): the kind of handle a call to Persona carries to the system, an on-device experiment for
    /// Phone › Recents, which only a device shows. Read once at launch; it counts only while the switch is on
    /// (`callHandleInUse`).
    private(set) var callHandle: CallHandleKind
    /// The bytes recorded, read by `refresh` (off the main actor).
    private(set) var size: Int64?

    @ObservationIgnored let recorder: DevRecorder
    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private var observers: [NSObjectProtocol] = []
    @ObservationIgnored private var ticker: Task<Void, Never>?
    @ObservationIgnored private var path: NWPathMonitor?
    @ObservationIgnored private var tab = "launch"
    /// Screens opened over the tabs (`devScreen`), the newest last, with the tab they opened on.
    @ObservationIgnored private var screens: [(id: UUID, name: String, tab: String)] = []

    init(recorder: DevRecorder = .shared, defaults: UserDefaults = .standard) {
        self.recorder = recorder
        self.defaults = defaults
        let on = defaults.bool(forKey: Self.onKey)
        isOn = on
        since = on ? defaults.object(forKey: Self.sinceKey) as? Date : nil
        callHandle = defaults.string(forKey: Self.callHandleKey).flatMap(CallHandleKind.init(rawValue:)) ?? .generic
    }

    /// The handle kind a new call carries: the switch's while Developing Persona is on, else the generic one.
    var callHandleInUse: CallHandleKind { isOn ? callHandle : .generic }

    func setCallHandle(_ kind: CallHandleKind) {
        guard kind != callHandle else { return }
        callHandle = kind
        defaults.set(kind.rawValue, forKey: Self.callHandleKey)
    }

    /// At launch: carries on recording when the switch was left on.
    func launch(tab: AppTab) {
        self.tab = Self.name(tab)
        guard isOn else { return }
        begin(active: Self.inFront, fresh: true)
    }

    func setOn(_ on: Bool) {
        guard on != isOn else { return }
        isOn = on
        defaults.set(on, forKey: Self.onKey)
        if on {
            let now = Date()
            since = now
            defaults.set(now, forKey: Self.sinceKey)
            size = nil
            begin(active: Self.inFront, fresh: false)
        } else {
            end()
            since = nil
            size = nil
            defaults.removeObject(forKey: Self.sinceKey)
            let store = recorder.store
            recorder.afterWrites { store.clear() }
            #if os(iOS)
            // The meeting tapes kept for replays (#10589) go with it: they hold other people's words.
            MeetingTapeLibrary.shared.deleteAll()
            #endif
        }
    }

    /// Reads the size recorded (the About page as it shows).
    func refresh() async {
        guard isOn else { size = nil; return }
        let recorder = recorder
        size = await Task.detached(priority: .userInitiated) {
            recorder.settle()
            return recorder.store.recordedBytes()
        }.value
    }

    /// Builds the package with `categories` and gives its zip's URL.
    func exportDiagnostics(_ categories: Set<DevPackage.Category>) async throws -> URL {
        let recorder = recorder, since = since
        return try await Task.detached(priority: .userInitiated) {
            recorder.settle()
            return try DevPackage.exportZip(store: recorder.store, categories: categories, since: since)
        }.value
    }

    // MARK: Screens

    /// The tab in front changed (`AppModel.selectedTab`).
    func tab(_ new: AppTab) {
        tab = Self.name(new)
        guard isOn else { return }
        recorder.page(current)
    }

    /// A screen opened over a tab (`View.devScreen`); `leave` with what it returns.
    func enter(_ name: String) -> UUID {
        let id = UUID()
        screens.append((id, name, tab))
        if isOn { recorder.page(current) }
        return id
    }

    func leave(_ id: UUID) {
        screens.removeAll { $0.id == id }
        if isOn { recorder.page(current) }
    }

    /// The newest screen opened on the tab in front, else the tab.
    var current: String { screens.last { $0.tab == tab }?.name ?? tab }

    /// The note Send to Agent puts in the chat's box with the package (the owner can change it before sending).
    static func note(since: String) -> String {
        String(localized: "Here are the Persona diagnostics from my device (since \(since)). Please find out what is using the battery and what is failing.")
    }

    static func name(_ tab: AppTab) -> String {
        switch tab {
        case .assistant: "assistant"
        case .chats: "agents"
        case .mail: "inbox"
        case .settings: "settings"
        #if os(iOS) || os(visionOS)
        case .feishu: "feishu"
        #endif
        }
    }

    // MARK: Recording

    /// `fresh`: at launch, so the connection tally counts the period in front from its start (#11390); turned on by
    /// hand, it starts with the next one.
    private func begin(active: Bool, fresh: Bool) {
        let recorder = recorder
        #if os(iOS)
        UIDevice.current.isBatteryMonitoringEnabled = true
        #endif
        recorder.start(active: active, page: current, device: device(), fresh: fresh)
        DevProbe.install(recorder)
        HubRedirects.finished = { @Sendable task, metrics in recorder.request(task, metrics) }
        Task.detached(priority: .utility) { DevMetrics.shared.start() }
        observe()
        let monitor = NWPathMonitor()
        monitor.pathUpdateHandler = { @Sendable p in
            let type = p.status != .satisfied ? "none" : p.usesInterfaceType(.wifi) ? "wifi" : p.usesInterfaceType(.cellular) ? "cellular"
                : p.usesInterfaceType(.wiredEthernet) ? "wired" : "other"
            recorder.path(.init(type: type, expensive: p.isExpensive, constrained: p.isConstrained))
        }
        monitor.start(queue: DispatchQueue(label: "persona.dev-mode.path", qos: .utility))
        path = monitor
        if active { tick() }
    }

    private func end() {
        ticker?.cancel()
        ticker = nil
        path?.cancel()
        path = nil
        for o in observers { NotificationCenter.default.removeObserver(o) }
        observers = []
        HubRedirects.finished = nil
        DevProbe.install(nil)
        DevMetrics.shared.stop()
        recorder.stop()
        #if os(iOS)
        UIDevice.current.isBatteryMonitoringEnabled = false
        #endif
    }

    /// One roll a minute while in front; none in the background.
    private func tick() {
        ticker?.cancel()
        ticker = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(60), tolerance: .seconds(5))
                guard !Task.isCancelled, let self else { return }
                self.recorder.roll(device: self.device())
            }
        }
    }

    private func observe() {
        let center = NotificationCenter.default
        func on(_ name: Notification.Name, _ run: @escaping @MainActor @Sendable (DevMode) -> Void) {
            observers.append(center.addObserver(forName: name, object: nil, queue: .main) { @Sendable [weak self] _ in
                Task { @MainActor in if let self, self.isOn { run(self) } }
            })
        }
        #if canImport(UIKit)
        on(UIApplication.didBecomeActiveNotification) { m in
            m.recorder.lifecycle(active: true, device: m.device())
            m.tick()
        }
        on(UIApplication.didEnterBackgroundNotification) { m in
            m.ticker?.cancel()
            m.recorder.lifecycle(active: false, device: m.device())
        }
        on(UIApplication.willTerminateNotification) { m in m.recorder.flush() }
        #endif
        on(ProcessInfo.thermalStateDidChangeNotification) { m in m.recorder.device("thermal", ["state": m.device().thermal]) }
        on(Notification.Name.NSProcessInfoPowerStateDidChange) { m in m.recorder.device("power", ["low_power": m.device().lowPower]) }
        #if os(iOS)
        on(UIDevice.batteryLevelDidChangeNotification) { m in
            guard Self.inFront, let level = m.device().battery else { return }
            m.recorder.device("battery", ["level": level, "state": Self.batteryState()])
        }
        on(UIDevice.batteryStateDidChangeNotification) { m in
            m.recorder.device("battery", ["level": m.device().battery.map { $0 as Any } ?? NSNull(), "state": Self.batteryState()])
        }
        #endif
    }

    private func device() -> DevRecorder.Device {
        var d = DevRecorder.Device()
        d.thermal = switch ProcessInfo.processInfo.thermalState {
        case .nominal: "nominal"
        case .fair: "fair"
        case .serious: "serious"
        case .critical: "critical"
        @unknown default: "unknown"
        }
        d.lowPower = ProcessInfo.processInfo.isLowPowerModeEnabled
        #if os(iOS)
        let level = UIDevice.current.batteryLevel
        if level >= 0 { d.battery = (Double(level) * 100).rounded() / 100 }
        #endif
        return d
    }

    #if os(iOS)
    private static func batteryState() -> String {
        switch UIDevice.current.batteryState {
        case .charging: "charging"
        case .full: "full"
        case .unplugged: "unplugged"
        default: "unknown"
        }
    }
    #endif

    private static var inFront: Bool {
        #if canImport(UIKit)
        UIApplication.shared.applicationState != .background
        #else
        true
        #endif
    }
}

/// The kinds of handle a call to Persona can carry (`CallHandle`; Developing Persona's Call handle, #9887): the
/// assistant's name as a generic handle (the default), or a fixed placeholder email address.
nonisolated enum CallHandleKind: String, CaseIterable, Equatable, Sendable {
    case generic, email
}
