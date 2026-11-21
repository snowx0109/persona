import Foundation
import PersonaWire

// Wire types mirroring docs/protocol/protocol.md. Explicit CodingKeys everywhere: key strategies would also rewrite
// arbitrary JSON keys inside tool inputs and answer dictionaries.

nonisolated struct Machine: Codable, Identifiable, Hashable, Sendable {
    var id: String
    var name: String
    var kind: String
    var icon: String?
    var sshHost: String?
    var online: Bool
    var error: String?
    var hostname: String?
    var os: String?
    var arch: String?
    var home: String?
    var herdrVersion: String?
    var claudeVersion: String?
    var lastSeen: Date?
    /// The machine's personad is not from the hub's build (newer hubs): some answers may be refused until it is deployed.
    var nodeMismatch: Bool?
    /// The machine's personad build.
    var nodeVersion: String?
    /// The hub is installing its own build there (Redeploy); deployError: why the last try did not work.
    var deploying: Bool?
    var deployError: String?
    /// The hub's own answer on updating the node from here (newer hubs): whether it can (`can_deploy`), why not
    /// (`deploy_reason`, in the request's language), and the stage of a deploy that is running (`deploy_phase`:
    /// waiting_idle, transferring, restarting or confirming). nil from older hubs.
    var canDeploy: Bool? = nil
    var deployReason: String? = nil
    var deployPhase: String? = nil
    /// Switching this SSH computer to Persona Link (#10662); nil from older hubs and managers.
    var linkSwitch: MachineLinkSwitch? = nil
    /// OS and hardware (docs/server/machines.md); nil from hubs before machine monitoring, or a machine never connected.
    var system: MachineSystem?
    var engine: MachineEngine = .herdr
    var engines: [DetectedEngine] = []
    /// The set of engine hosts the computer runs (docs/integrations/engines.md "Host sets"); nil from a hub before sets.
    /// `[]` is no terminal engine: `engine` still says herdr then, for older clients.
    var engineHosts: [String]? = nil
    /// The host for a start without an explicit workspace host when the machine has a host set.
    var engineDefault: String? = nil
    /// Operating it through Cua Driver; nil while the hub's plugin cua is off or it is not a Mac.
    var computer: MachineComputer? = nil
    /// Its ego lite for the assistant; nil while the hub's plugin ego is off or it is not a Mac.
    var browser: MachineBrowser? = nil
    /// The iPhones paired with it the assistant may operate; nil while the hub's extension iphone is off or it can't reach iPhones.
    var iphone: MachineIPhone? = nil
    /// Sharing its clipboard with the phone (#3064); nil from hubs before it.
    var clipboard: MachineClipboard? = nil
    /// Hub mesh (docs/archive/operations/plan-hub-mesh.md 2.6): the hub that owns the computer in its group, its name, the computer's own
    /// id across hubs, and how the hub reaches it. nil from hubs before the mesh.
    var hubID: String? = nil
    var hubName: String? = nil
    var machineUID: String? = nil
    var route: MachineHubRoute? = nil
    /// The group of paired hubs this machine came from (docs/protocol/multi-hub.md); nil for the default group. The app's own,
    /// never sent by a hub.
    var hubGroup: String? = nil
    /// With several groups, the group's name, for lists to say where a machine is (`title`); nil with one group.
    var groupLabel: String? = nil
    /// `pending` while it waits for the user's It's Mine (feature machine_trust); nil = confirmed. Kept as the hub's
    /// word: another value reads as confirmed.
    var trust: String? = nil
    /// What kind of trouble `error` is (node_service_down, link_offline, never_connected, node_outdated, or a word this
    /// app does not know, which shows `error`); `error` keeps the hub's own words.
    var errorCode: String? = nil
    /// The manager it belongs to (docs/protocol/protocol.md "Per-manager routing"); nil from a hub before managers. While
    /// that manager is offline the computer is too, and only comes back with it (`ManagerRoster.offlineManager`).
    var manager: String? = nil

    enum CodingKeys: String, CodingKey {
        case id, name, kind, icon, online, error, hostname, os, arch, home, system, engine, engines, computer, browser, iphone, clipboard, route, trust, manager
        case engineHosts = "engine_hosts", engineDefault = "engine_default"
        case errorCode = "error_code"
        case sshHost = "ssh_host", herdrVersion = "herdr_version", claudeVersion = "claude_version", lastSeen = "last_seen"
        case nodeMismatch = "node_mismatch", nodeVersion = "node_version", deploying, deployError = "deploy_error"
        case canDeploy = "can_deploy", deployReason = "deploy_reason", deployPhase = "deploy_phase"
        case linkSwitch = "link_switch"
        case hubID = "hub_id", hubName = "hub_name", machineUID = "machine_uid", hubGroup = "app_hub_group",
             groupLabel = "app_group_label"
    }

    var symbol: String {
        switch icon {
        case "server": Theme.Icon.server
        case "desktop": Theme.Icon.machine
        default: Theme.Icon.laptop
        }
    }
}

/// An SSH computer's eligibility for, or progress through, the hub-owned switch to Persona Link (#10662).
/// `phase` and the reason/error codes stay as wire words so newer hubs remain decodable; clients localize them.
nonisolated struct MachineLinkSwitch: Codable, Hashable, Sendable {
    var available: Bool
    var reason: String?
    var phase: String?
    var error: String?
    var rollbackUnconfirmed: Bool?
    var at: Date?

    init(available: Bool, reason: String? = nil, phase: String? = nil, error: String? = nil,
         rollbackUnconfirmed: Bool? = nil, at: Date? = nil) {
        self.available = available
        self.reason = reason
        self.phase = phase
        self.error = error
        self.rollbackUnconfirmed = rollbackUnconfirmed
        self.at = at
    }

    enum CodingKeys: String, CodingKey {
        case available, reason, phase, error, at
        case rollbackUnconfirmed = "rollback_unconfirmed"
    }
}

// Keep memberwise construction while decoding older hubs and future engines safely.
extension Machine {
    nonisolated init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        kind = try c.decode(String.self, forKey: .kind)
        icon = try c.decodeIfPresent(String.self, forKey: .icon)
        sshHost = try c.decodeIfPresent(String.self, forKey: .sshHost)
        online = try c.decode(Bool.self, forKey: .online)
        error = try c.decodeIfPresent(String.self, forKey: .error)
        hostname = try c.decodeIfPresent(String.self, forKey: .hostname)
        os = try c.decodeIfPresent(String.self, forKey: .os)
        arch = try c.decodeIfPresent(String.self, forKey: .arch)
        home = try c.decodeIfPresent(String.self, forKey: .home)
        herdrVersion = try c.decodeIfPresent(String.self, forKey: .herdrVersion)
        claudeVersion = try c.decodeIfPresent(String.self, forKey: .claudeVersion)
        lastSeen = try c.decodeIfPresent(Date.self, forKey: .lastSeen)
        nodeMismatch = try c.decodeIfPresent(Bool.self, forKey: .nodeMismatch)
        nodeVersion = try c.decodeIfPresent(String.self, forKey: .nodeVersion)
        deploying = try c.decodeIfPresent(Bool.self, forKey: .deploying)
        deployError = try c.decodeIfPresent(String.self, forKey: .deployError)
        canDeploy = try c.decodeIfPresent(Bool.self, forKey: .canDeploy)
        deployReason = try c.decodeIfPresent(String.self, forKey: .deployReason)
        deployPhase = try c.decodeIfPresent(String.self, forKey: .deployPhase)
        linkSwitch = try? c.decodeIfPresent(MachineLinkSwitch.self, forKey: .linkSwitch)
        system = try c.decodeIfPresent(MachineSystem.self, forKey: .system)
        engine = try c.decodeIfPresent(MachineEngine.self, forKey: .engine) ?? .herdr
        engines = try c.decodeIfPresent([DetectedEngine].self, forKey: .engines) ?? []
        engineHosts = try? c.decodeIfPresent([String].self, forKey: .engineHosts)
        engineDefault = try? c.decodeIfPresent(String.self, forKey: .engineDefault)
        computer = try? c.decodeIfPresent(MachineComputer.self, forKey: .computer)
        browser = try? c.decodeIfPresent(MachineBrowser.self, forKey: .browser)
        iphone = try? c.decodeIfPresent(MachineIPhone.self, forKey: .iphone)
        clipboard = try? c.decodeIfPresent(MachineClipboard.self, forKey: .clipboard)
        hubID = try? c.decodeIfPresent(String.self, forKey: .hubID)
        hubName = try? c.decodeIfPresent(String.self, forKey: .hubName)
        machineUID = try? c.decodeIfPresent(String.self, forKey: .machineUID)
        route = try? c.decodeIfPresent(MachineHubRoute.self, forKey: .route)
        hubGroup = try c.decodeIfPresent(String.self, forKey: .hubGroup)
        groupLabel = try? c.decodeIfPresent(String.self, forKey: .groupLabel)
        trust = try? c.decodeIfPresent(String.self, forKey: .trust)
        errorCode = try? c.decodeIfPresent(String.self, forKey: .errorCode)
        manager = try? c.decodeIfPresent(String.self, forKey: .manager)
    }
}

/// How the hub the app is connected to reaches a machine (docs/archive/operations/plan-hub-mesh.md 2.6): directly (`local`, `ssh`,
/// `tailcat`, `link`) or through another hub of its group (`peer`, `hops` links away).
nonisolated struct MachineHubRoute: Codable, Hashable, Sendable {
    var via: String
    var hops: Int = 0
    var rttMS: Int? = nil

    enum CodingKeys: String, CodingKey {
        case via, hops
        case rttMS = "rtt_ms"
    }

    init(via: String, hops: Int = 0, rttMS: Int? = nil) {
        self.via = via
        self.hops = hops
        self.rttMS = rttMS
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        via = (try? c.decodeIfPresent(String.self, forKey: .via)) ?? ""
        hops = (try? c.decodeIfPresent(Int.self, forKey: .hops)) ?? 0
        rttMS = try? c.decodeIfPresent(Int.self, forKey: .rttMS)
    }
}

nonisolated extension Machine {
    /// The group it came from ("" = the default group).
    var group: String {
        get { hubGroup ?? "" }
        set { hubGroup = newValue.isEmpty ? nil : newValue }
    }
    /// Which machine across groups: the same machine id can be in two groups.
    var ref: MachineRef { MachineRef(group: group, id: id) }
    /// What lists call it: its name, and with several groups the group's ("MacBook · Work").
    var title: String { groupLabel.map { "\(name) · \($0)" } ?? name }
    /// Waiting for the user to say it is theirs: no agents, no keys, not offered where a computer is picked.
    var isPending: Bool { trust == "pending" }
    /// Added but never online once (`never_connected`, or waiting with no `last_seen`): not Offline, it was never there.
    var neverConnected: Bool {
        !online && (errorCode == "never_connected" || (isPending && (lastSeen?.timeIntervalSince1970 ?? 0) <= 0))
    }
    /// Its disconnect is expected while the hub moves it from SSH to Persona Link, not a new connection failure.
    var linkSwitchRunning: Bool {
        switch linkSwitch?.phase {
        case "checking", "installing", "connecting", "rolling_back": true
        default: false
        }
    }
}

/// A machine across groups of hubs (docs/protocol/multi-hub.md): its group and the id its hub gives it.
nonisolated struct MachineRef: Hashable, Sendable, Codable {
    var group: String
    var id: String

    init(group: String = "", id: String) {
        self.group = group
        self.id = id
    }

    /// The machine id, or "group|machine id" for another group: what pickers and filters remember.
    var path: String { group.isEmpty ? id : group + "|" + id }

    init(path: String) {
        if let bar = path.firstIndex(of: "|") {
            self.init(group: String(path[..<bar]), id: String(path[path.index(after: bar)...]))
        } else {
            self.init(id: path)
        }
    }
}


nonisolated struct MachineEngine: RawRepresentable, Codable, Hashable, CaseIterable, Identifiable, Sendable {
    var rawValue: String
    init(rawValue: String) { self.rawValue = rawValue }
    static let herdr = Self(rawValue: "herdr")
    static let herdrApp = Self(rawValue: "herdr-core")
    static let herdrBoth = Self(rawValue: "herdr-both")
    static let orca = Self(rawValue: "orca")
    /// Tern's terminals (docs/integrations/engines.md): its sessions are spaces, its tabs slots, its blocks panes
    /// (`tern:<block id>`).
    static let tern = Self(rawValue: "tern")
    /// Rex's terminals (docs/integrations/engines.md): its sessions are spaces, its windows slots, its blocks panes
    /// (`rex:<block id>`).
    static let rex = Self(rawValue: "rex")
    /// Paseo's agents (docs/integrations/paseo.md): no terminal; the node runs a bridge to the Paseo daemon.
    static let paseo = Self(rawValue: "paseo")
    /// Cindy's agents (docs/integrations/engines.md): no terminal; the node runs a bridge to Cindy.
    static let cindy = Self(rawValue: "cindy")
    /// No terminal engine: the empty host set (docs/integrations/engines.md "Host sets"), for a computer whose sessions
    /// are only the desktop apps' (#3302). Not a hub engine id: saved as `{"hosts": []}`, and not in `allCases`.
    static let noEngine = Self(rawValue: "none")
    static let allCases: [Self] = [.herdr, .herdrApp, .herdrBoth, .orca, .tern, .rex, .paseo, .cindy]
    var id: String { rawValue }
    var name: String {
        switch self {
        case .herdr: "herdr"
        case .herdrApp: "Herdr App"
        case .herdrBoth: "herdr + Herdr App"
        case .orca: "Orca"
        case .tern: "Tern"
        case .rex: "Rex"
        case .paseo: "Paseo"
        case .cindy: "Cindy"
        case .noEngine: String(localized: "No Terminal Engine")
        default: rawValue
        }
    }
    var detail: String? {
        switch self {
        case .herdr: String(localized: "herdr in a terminal")
        case .herdrApp: String(localized: "Terminals in the desktop app")
        case .herdrBoth: String(localized: "Sessions from both the terminal and the desktop app")
        case .orca: String(localized: "Terminals in Orca")
        case .tern: String(localized: "Terminals in Tern")
        case .rex: String(localized: "Terminals in Rex")
        case .paseo: String(localized: "Agents in Paseo")
        case .cindy: String(localized: "Agents in Cindy")
        case .noEngine: String(localized: "Sessions of the desktop apps only")
        default: nil
        }
    }

    init(from decoder: Decoder) throws {
        rawValue = try decoder.singleValueContainer().decode(String.self)
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        try c.encode(rawValue)
    }
}

nonisolated struct DetectedEngine: Codable, Hashable, Identifiable, Sendable {
    var id: String
    var name: String
    var available: Bool
    var version: String?
    var note: String?
}

/// What a Mac can do for the live-screen hardware video channel (#3954, docs/extensions/computer-use.md "Live screen
/// video"): the capture-and-encode helper Persona Screen is present (`version` > 0), which codecs it can encode, and
/// whether Screen Recording is granted for it. Absent from a Mac without the helper or from an older node, so the app
/// simply keeps showing RFB. `advertised` is the test the Picture / Quality switcher uses: the node offers the channel
/// at all, so there is a control worth showing.
nonisolated struct MachineScreenVideo: Codable, Hashable, Sendable {
    /// The wire protocol version the helper speaks; 0 or absent means no video.
    var version = 0
    /// The codecs the helper can encode, in preference order ("hevc", "h264").
    var codecs: [String] = []
    /// Screen Recording for Persona Screen: granted | missing | unknown. Missing keeps the phone on RFB.
    var permission: String? = nil
    /// The node has asked macOS for this helper version's grant (once), so Persona Screen is in System Settings' list;
    /// false while macOS's alert is still to come.
    var asked = false
    /// The ffmpeg with libx264 the user installed on the Mac, as its node found and tested it ("ffmpeg 9.0.2", #8859):
    /// x264 can be turned on for the Mac's live screen. Nil: none, or an older node.
    var x264: String? = nil

    enum CodingKeys: String, CodingKey { case version, codecs, permission, asked, x264 }

    init(version: Int = 0, codecs: [String] = [], permission: String? = nil, asked: Bool = false, x264: String? = nil) {
        self.version = version
        self.codecs = codecs
        self.permission = permission
        self.asked = asked
        self.x264 = x264
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        version = try c.decodeIfPresent(Int.self, forKey: .version) ?? 0
        codecs = (try? c.decodeIfPresent([String].self, forKey: .codecs)) ?? []
        permission = try c.decodeIfPresent(String.self, forKey: .permission)
        asked = (try? c.decodeIfPresent(Bool.self, forKey: .asked)) ?? false
        x264 = (try? c.decodeIfPresent(String.self, forKey: .x264)) ?? nil
    }

    /// The node offers the video channel at all: the Picture / Quality switcher appears only then (#3954).
    var advertised: Bool { version > 0 }
}

/// What a Mac can do for pixel scrolling (#7775, docs/extensions/computer-use.md "Pixel scrolling"): its node takes the
/// live screen's scroll records out of the RFB stream for Persona Input (`version`: the record's version it reads), and
/// whether that helper may post events (Accessibility). Absent from a Mac without the helper or from an older node: the
/// phone then scrolls by wheel notches only and never sends a record.
nonisolated struct MachineScreenInput: Codable, Hashable, Sendable {
    /// The record's version the node reads; 0 or absent: none.
    var version = 0
    /// Accessibility (posting events) for Persona Input: granted | missing | unknown.
    var permission: String? = nil
    /// The node has asked macOS for this helper version's permission (once), so Persona Input is in System Settings' list.
    var asked = false

    enum CodingKeys: String, CodingKey { case version, permission, asked }

    init(version: Int = 0, permission: String? = nil, asked: Bool = false) {
        self.version = version
        self.permission = permission
        self.asked = asked
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        version = try c.decodeIfPresent(Int.self, forKey: .version) ?? 0
        permission = try c.decodeIfPresent(String.self, forKey: .permission)
        asked = (try? c.decodeIfPresent(Bool.self, forKey: .asked)) ?? false
    }

    /// The node reads this phone's records (version 1).
    var reads: Bool { version >= 1 }
    /// Scroll records may go: the node reads them and the helper may post them.
    var granted: Bool { reads && permission == "granted" }
}

/// What a Mac can do for the live screen's Phone Screen mode (#9314, docs/extensions/computer-use.md "Phone Screen"): its
/// node's Persona Input can show one window on a portrait virtual display (`version`, the request set it speaks), and
/// whether that helper may move windows (Accessibility). Absent from a Mac without it or an older node: no mode offered.
nonisolated struct MachinePhoneScreen: Codable, Hashable, Sendable {
    var version = 0
    /// Accessibility (moving windows) for Persona Input: granted | missing.
    var permission: String? = nil

    var offered: Bool { version > 0 }
    var granted: Bool { permission == "granted" }
}

/// A request for Phone Screen (#9314): `op` start (`width` × `height`, the picture in points), windows, place (`pid`,
/// `window`) or stop, for the live screen whose RFB stream carries `pair`.
nonisolated struct PhoneScreenRequest: Encodable, Sendable {
    var pair: String
    var op: String
    var width: Double? = nil
    var height: Double? = nil
    var pid: Int32? = nil
    var window: UInt32? = nil
}

/// What the Mac answered: the virtual display and every display (start), the windows, the one placed.
nonisolated struct PhoneScreenReply: Decodable, Sendable {
    var display: MachineDisplay?
    var displays: [MachineDisplay]?
    var windows: [PhoneScreenWindow]?
    var placed: PhoneScreenWindow?
}

/// One of the Mac's windows for Phone Screen: its app, title, and whether it is on the phone's display now.
nonisolated struct PhoneScreenWindow: Decodable, Hashable, Sendable, Identifiable {
    var pid: Int32
    var window: UInt32
    var app: String
    var bundle: String?
    var title: String
    var minimized: Bool?
    var placed: Bool?

    var id: String { "\(pid)/\(window)" }
}

/// A Mac the assistant may operate through Cua Driver (the hub's plugin cua, docs/extensions/computer-use.md). `enabled`: the user
/// turned it on for this machine; the rest is what its node found when last asked, and only for an enabled one.
nonisolated struct MachineComputer: Codable, Hashable, Sendable {
    var enabled = false
    var installed = false
    var version: String? = nil
    var daemon = false
    /// granted | missing | unknown
    var accessibility: String? = nil
    var screenRecording: String? = nil
    /// active | locked | away
    var desktop: String? = nil
    var ready = false
    /// What is missing and what to do, in Chinese.
    var note: String? = nil
    /// Installing Cua Driver or asking for its permissions (the page's buttons): the running one, or the last.
    var job: ComputerJob? = nil
    /// macOS's Screen Sharing answers on the Mac: its live screen can be shown (VNC).
    var screenSharing = false
    /// 「扩展开着时不自动锁屏」 is on for it (#1507 b2).
    var noAutoLock = false
    /// x264 is on for its live screen (#8859; the plugin's screen_x264): its own ffmpeg encodes on a direct LAN route.
    var screenX264 = false
    /// cua or lcu. Nil from a hub that only knows Cua Driver.
    var backend: String? = nil
    /// The official ChatGPT desktop app is installed (LCU's runtime).
    var chatgpt = false
    /// The Mac's displays as arranged in its System Settings, read with `screenSharing` (empty from an older node).
    var displays: [MachineDisplay] = []
    /// The live-screen hardware video channel this Mac can carry (#3954); nil from a Mac without the helper or an older node.
    var screenVideo: MachineScreenVideo? = nil
    /// Pixel scrolling (#7775); nil from a Mac without Persona Input or an older node.
    var screenInput: MachineScreenInput? = nil
    /// Phone Screen (#9314); nil from a node or helper without it.
    var phoneScreen: MachinePhoneScreen? = nil

    enum CodingKeys: String, CodingKey {
        case enabled, installed, version, daemon, accessibility, desktop, ready, note, job, backend, chatgpt, displays
        case screenRecording = "screen_recording", screenSharing = "screen_sharing", noAutoLock = "no_auto_lock", screenVideo = "screen_video"
        case screenInput = "screen_input", screenX264 = "screen_x264", phoneScreen = "phone_screen"
    }

    init(enabled: Bool = false, installed: Bool = false, version: String? = nil, daemon: Bool = false, accessibility: String? = nil,
         screenRecording: String? = nil, desktop: String? = nil, ready: Bool = false, note: String? = nil, job: ComputerJob? = nil,
         backend: String? = nil, chatgpt: Bool = false) {
        self.enabled = enabled
        self.installed = installed
        self.version = version
        self.daemon = daemon
        self.accessibility = accessibility
        self.screenRecording = screenRecording
        self.desktop = desktop
        self.ready = ready
        self.note = note
        self.job = job
        self.backend = backend
        self.chatgpt = chatgpt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        installed = try c.decodeIfPresent(Bool.self, forKey: .installed) ?? false
        version = try c.decodeIfPresent(String.self, forKey: .version)
        daemon = try c.decodeIfPresent(Bool.self, forKey: .daemon) ?? false
        accessibility = try c.decodeIfPresent(String.self, forKey: .accessibility)
        screenRecording = try c.decodeIfPresent(String.self, forKey: .screenRecording)
        desktop = try c.decodeIfPresent(String.self, forKey: .desktop)
        ready = try c.decodeIfPresent(Bool.self, forKey: .ready) ?? false
        note = try c.decodeIfPresent(String.self, forKey: .note)
        job = try? c.decodeIfPresent(ComputerJob.self, forKey: .job)
        screenSharing = try c.decodeIfPresent(Bool.self, forKey: .screenSharing) ?? false
        noAutoLock = try c.decodeIfPresent(Bool.self, forKey: .noAutoLock) ?? false
        screenX264 = (try? c.decodeIfPresent(Bool.self, forKey: .screenX264)) ?? false
        backend = try c.decodeIfPresent(String.self, forKey: .backend)
        chatgpt = try c.decodeIfPresent(Bool.self, forKey: .chatgpt) ?? false
        // An unreadable list only costs the display choice, not the machine.
        displays = (try? c.decodeIfPresent([MachineDisplay].self, forKey: .displays)) ?? []
        // An unreadable channel only costs the video switcher, not the machine (the page stays on RFB).
        screenVideo = (try? c.decodeIfPresent(MachineScreenVideo.self, forKey: .screenVideo)) ?? nil
        // An unreadable one only costs pixel scrolling (wheel notches instead).
        screenInput = (try? c.decodeIfPresent(MachineScreenInput.self, forKey: .screenInput)) ?? nil
        phoneScreen = (try? c.decodeIfPresent(MachinePhoneScreen.self, forKey: .phoneScreen)) ?? nil
    }

    /// The node has said how it is (a never-asked one has neither a verdict nor a note).
    var checked: Bool { ready || !(note ?? "").isEmpty }

    /// The macOS permissions it still lacks, by their names in System Settings.
    var missingPermissions: [String] {
        [(accessibility, String(localized: "Accessibility", comment: "macOS privacy setting name")), (screenRecording, String(localized: "Screen Recording", comment: "macOS privacy setting name"))].compactMap { $0.0 == "missing" ? $0.1 : nil }
    }
}

/// A Mac's ego lite, the browser the assistant drives (the hub's plugin ego, docs/plans/plan-ego-browser.md). `enabled`: the
/// user turned it on for this machine; the rest is what its node found when last asked.
nonisolated struct MachineBrowser: Codable, Hashable, Sendable {
    var enabled = false
    var installed = false
    var version: String? = nil
    /// Its first-run setup is done; `imported`, that setup took another browser's data.
    var setUp = false
    var imported = false
    /// Chromium's usage statistics: on | off | unknown.
    var analytics: String? = nil
    var ready = false
    var note: String? = nil
    var screenSharing = false
    var job: ComputerJob? = nil
    /// As `MachineComputer.displays`.
    var displays: [MachineDisplay] = []
    /// As `MachineComputer.screenVideo` (#3954): a Mac the plugin ego is on for shares the same live screen and helper.
    var screenVideo: MachineScreenVideo? = nil
    /// As `MachineComputer.screenInput` (#7775).
    var screenInput: MachineScreenInput? = nil
    /// Phone Screen (#9314), as for the computer.
    var phoneScreen: MachinePhoneScreen? = nil

    enum CodingKeys: String, CodingKey {
        case enabled, installed, version, imported, analytics, ready, note, job, displays
        case setUp = "set_up", screenSharing = "screen_sharing", screenVideo = "screen_video", screenInput = "screen_input"
        case phoneScreen = "phone_screen"
    }

    init(enabled: Bool = false, installed: Bool = false, version: String? = nil, setUp: Bool = false, imported: Bool = false,
         analytics: String? = nil, ready: Bool = false, note: String? = nil, screenSharing: Bool = false, job: ComputerJob? = nil) {
        self.enabled = enabled
        self.installed = installed
        self.version = version
        self.setUp = setUp
        self.imported = imported
        self.analytics = analytics
        self.ready = ready
        self.note = note
        self.screenSharing = screenSharing
        self.job = job
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        installed = try c.decodeIfPresent(Bool.self, forKey: .installed) ?? false
        version = try c.decodeIfPresent(String.self, forKey: .version)
        setUp = try c.decodeIfPresent(Bool.self, forKey: .setUp) ?? false
        imported = try c.decodeIfPresent(Bool.self, forKey: .imported) ?? false
        analytics = try c.decodeIfPresent(String.self, forKey: .analytics)
        ready = try c.decodeIfPresent(Bool.self, forKey: .ready) ?? false
        note = try c.decodeIfPresent(String.self, forKey: .note)
        screenSharing = try c.decodeIfPresent(Bool.self, forKey: .screenSharing) ?? false
        job = try? c.decodeIfPresent(ComputerJob.self, forKey: .job)
        displays = (try? c.decodeIfPresent([MachineDisplay].self, forKey: .displays)) ?? []
        screenVideo = (try? c.decodeIfPresent(MachineScreenVideo.self, forKey: .screenVideo)) ?? nil
        screenInput = (try? c.decodeIfPresent(MachineScreenInput.self, forKey: .screenInput)) ?? nil
        phoneScreen = (try? c.decodeIfPresent(MachinePhoneScreen.self, forKey: .phoneScreen)) ?? nil
    }

    /// The node has said how it is.
    var checked: Bool { ready || installed || !(note ?? "").isEmpty }
}

/// One of a Mac's displays, as its node read it (docs/extensions/computer-use.md "Displays"): the frame in points in the
/// global display space (origin at the top-left corner of the main display, y down); `id` is macOS's display number.
nonisolated struct MachineDisplay: Codable, Hashable, Sendable {
    var id: Int
    var x: Double
    var y: Double
    var width: Double
    var height: Double
    var main: Bool = false
    /// Pixels per point (2 for Retina, #9314): Screen Sharing draws each display at its own pixels. 0 from an older node.
    var scale: Double = 0

    init(id: Int, x: Double, y: Double, width: Double, height: Double, main: Bool = false, scale: Double = 0) {
        self.id = id
        self.x = x
        self.y = y
        self.width = width
        self.height = height
        self.main = main
        self.scale = scale
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(Int.self, forKey: .id)
        x = try c.decode(Double.self, forKey: .x)
        y = try c.decode(Double.self, forKey: .y)
        width = try c.decode(Double.self, forKey: .width)
        height = try c.decode(Double.self, forKey: .height)
        main = try c.decodeIfPresent(Bool.self, forKey: .main) ?? false
        scale = (try? c.decodeIfPresent(Double.self, forKey: .scale)) ?? 0
    }

    var frame: CGRect { CGRect(x: x, y: y, width: width, height: height) }
}

/// One install or grant the node runs for the machine page (docs/extensions/computer-use.md「One-tap installation」).
/// An app the user always allowed the assistant to operate on a Mac (docs/extensions/computer-use.md「Locking, preventing sleep and per-App approval」c).
nonisolated struct ComputerApp: Codable, Hashable, Sendable, Identifiable {
    var bundleID: String
    var name: String
    var id: String { bundleID }

    enum CodingKeys: String, CodingKey {
        case bundleID = "bundle_id", name
    }
}

nonisolated struct ComputerJob: Codable, Hashable, Sendable {
    /// install | grant (Cua, ego); pair | forget | install | prepare (an iPhone's, #8199)
    var kind: String
    /// running | done | failed | cancelled
    var state: String
    var step: String? = nil
    var error: String? = nil
    /// An iPhone job's code: while it runs, what it waits for at the phone (trust_pending, locked); once failed, why.
    var code: String? = nil
    /// An iPhone job's iPhone.
    var device: String? = nil

    var running: Bool { state == "running" }
}

extension Machine {
    var engineName: String {
        engineHosts?.isEmpty == true ? MachineEngine.noEngine.name : engines.first { $0.id == engine.rawValue }?.name ?? engine.name
    }

    /// The host a new session uses without a workspace. Older hubs report only the legacy engine.
    nonisolated var defaultStartHost: String? {
        guard engineHosts?.isEmpty != true else { return nil }
        if let engineDefault, !engineDefault.isEmpty { return engineDefault }
        return engine == .herdrBoth ? MachineEngine.herdr.rawValue : engine.rawValue
    }

    /// An explicit workspace host wins over the machine default, provided the machine enables it.
    nonisolated func startsInHerdr(host: String?) -> Bool {
        let herdr = MachineEngine.herdr.rawValue
        let legacyHerdr = engine == .herdr || engine == .herdrBoth
        guard engineHosts?.contains(herdr) ?? legacyHerdr else { return false }
        if let host, !host.isEmpty { return host == herdr }
        return defaultStartHost == herdr
    }
}

nonisolated struct Launcher: Codable, Identifiable, Hashable, Sendable {
    var name: String
    /// What people call it where it has a name of its own: a Persona-managed provider's name, or the label a runtime
    /// gives its provider (Paseo's "Grok" for `grok`, #8261); nil for the others, which go by `name` (`title`).
    var displayName: String? = nil
    var baseURL: String?
    var model: String?
    /// "codex" for plain codex or an alias running it (docs/integrations/codex.md「Starting from the phone」); nil: Claude Code.
    var agent: String? = nil
    /// The configured runtime kind, when the hub supplies one (claude, codex, or another runtime).
    var agentType: String? = nil
    /// An agent runtime's launcher that can run the Persona assistant (DSH's, docs/integrations/runtimes.md「Assistant」).
    var assistant: Bool? = nil
    /// How its last check or start went on the computer the list is for (`GET …/machines/{id}/launchers`,
    /// docs/server/launcher-health.md); nil: never checked, a failure too old to count, or an older hub.
    var health: LauncherHealth? = nil
    /// A Codex launcher the computer also starts with no first message (docs/integrations/codex.md「Starting without a
    /// first message」); nil from an older computer, whose Codex starts need one.
    var promptOptional: Bool? = nil
    /// Where the computer has it: zshrc, path, persona, a runtime's id, `claude_desktop` (`claudeDesktopSource`) or
    /// `codex_app` (`codexAppSource`).
    var source: String? = nil
    /// A Codex launcher the computer starts in a shell pane the user picked (Run Agent Here) and keeps that pane when the
    /// start fails (#5504); nil from an older computer, where Run Agent Here offers Claude Code only.
    var runHere: Bool? = nil
    /// The permission modes a start on it takes when its agent is not Claude Code (#8261, the node's `permission_modes`:
    /// Paseo's Grok, OpenCode or Codex): `default`, its own approval, sent as no mode, and `bypassPermissions` where it
    /// has an Allow Everything (`StartModes.listed`). Nil for Claude Code's launchers, which take its five, for the
    /// others, which take none, and from an older computer.
    var permissionModes: [String]? = nil
    var id: String { name }
    /// What the pickers show: its own name, else its startup name (the web's `providerTitle`).
    var title: String {
        guard let d = displayName?.trimmingCharacters(in: .whitespacesAndNewlines), !d.isEmpty else { return name }
        return d
    }
    /// The launcher that starts the session in Claude's desktop app (docs/integrations/claude-integration.md「New sessions
    /// in the desktop app」): with no folder it runs in the app's scratch folder, and a new folder is made for it.
    var startsInDesktopApp: Bool { source == Self.claudeDesktopSource || source == Self.codexAppSource }
    static let claudeDesktopSource = "claude_desktop"
    /// The launcher that starts the chat in the Codex App (#5726, docs/integrations/codex.md):
    /// a Codex launcher (`agent` codex) whose chat with no folder goes among the App's chats in no project.
    static let codexAppSource = "codex_app"
    /// A Codex start sends its first message with it and waits for the thread Codex makes.
    var isCodex: Bool { agent == "codex" }
    /// A CLI launcher discovered on the computer, including the source-less responses of older hubs.
    private var isMachineLauncher: Bool {
        source == nil || source == "" || source == "zshrc" || source == "path" || source == "builtin"
    }
    /// The node starts Oh My Pi only under its own launcher name.
    var startsOmp: Bool {
        guard name == "omp", isMachineLauncher else { return false }
        guard let kind = runtimeAgent, !kind.isEmpty else { return true }
        return kind == "omp"
    }
    /// The hub's configured kind wins when present; an empty or missing value falls back to the node's agent.
    var runtimeAgent: String? { agentType?.isEmpty == false ? agentType : agent }
    /// Starting it needs a first message: a Codex on a computer that cannot open it without one.
    var needsFirstMessage: Bool { isCodex && promptOptional != true }
    /// It runs omp (docs/integrations/omp.md): omp's own config.yml picks its model and thinking level, so a start on it
    /// takes no permission mode, effort, ultracode or model, and its first message goes with the start.
    var isOmp: Bool { runtimeAgent == "omp" }

    enum CodingKeys: String, CodingKey {
        case name, model, agent, assistant, health, source
        case displayName = "display_name", agentType = "agent_type"
        case baseURL = "base_url"
        case promptOptional = "prompt_optional"
        case runHere = "run_here"
        case permissionModes = "permission_modes"
    }
}

nonisolated extension Launcher {
    /// A health record this app cannot read is left out, never the launcher (the list is what New Agent starts from);
    /// so is a `display_name`, `agent_type` or `permission_modes` it cannot read (#8261).
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        displayName = (try? c.decodeIfPresent(String.self, forKey: .displayName)).flatMap { $0 }
        baseURL = try c.decodeIfPresent(String.self, forKey: .baseURL)
        model = try c.decodeIfPresent(String.self, forKey: .model)
        agent = try c.decodeIfPresent(String.self, forKey: .agent)
        agentType = (try? c.decodeIfPresent(String.self, forKey: .agentType)).flatMap { $0 }
        assistant = try c.decodeIfPresent(Bool.self, forKey: .assistant)
        health = (try? c.decodeIfPresent(LauncherHealth.self, forKey: .health)) ?? nil
        promptOptional = try c.decodeIfPresent(Bool.self, forKey: .promptOptional)
        source = try c.decodeIfPresent(String.self, forKey: .source)
        runHere = try c.decodeIfPresent(Bool.self, forKey: .runHere)
        permissionModes = (try? c.decodeIfPresent([String].self, forKey: .permissionModes)) ?? nil
    }
}

/// A launcher's health on one computer (#1643): the hub's record of its latest check or Agent start there, within the
/// hours a failure counts. `state` ok / failing / repairing / blocked; `kind` the failure's class (not_in_terminal,
/// auth, model_route, cli_outdated, timeout, other); `error` the redacted raw text.
nonisolated struct LauncherHealth: Codable, Hashable, Sendable {
    var state: String
    var kind: String? = nil
    var error: String? = nil
    var checkedAt: Date? = nil

    enum CodingKeys: String, CodingKey {
        case state, kind, error
        case checkedAt = "checked_at"
    }

    static let blockedState = "blocked"
    /// A start's new terminal tab could not run the launcher or its claude, though the login shell can (#1642).
    static let notInTerminalKind = "not_in_terminal"
}

extension Launcher {
    /// Runs Claude Code or Codex, or is an agent runtime's launcher that says it can: what the Persona assistant can
    /// use (headless `claude -p`, `codex app-server`, the runtime). Other agents (TRAEX…) are left out; the node
    /// refuses them with a reason too.
    var runsAssistant: Bool {
        if let assistant { return assistant }
        guard baseURL != "(traex)" else { return false }
        // Older hubs can ignore purpose=assistant. A runtime's provider is not a shell command, and managed Codex
        // launchers cannot start the assistant's app-server. Keep the shell fallback for hubs without capabilities.
        let managed = source == "persona" || source == LegacyNames.product
        guard isMachineLauncher || managed, !(managed && agent == "codex") else { return false }
        return agent == nil || agent == "" || agent == "claude" || agent == "codex"
    }
}

/// The launcher / model / effort picked for one of the assistant's conversations, which its next messages run on
/// (#5033, `model_pick` on the session; docs/assistant/model-config.md "A conversation's own model"). Absent: the
/// conversation follows Model configuration › Main chat (the main one) or keeps what it started with.
nonisolated struct ModelPick: Codable, Hashable, Sendable {
    var launcher: String
    var model: String?
    var effort: String?
}

/// A launcher or provider the main chat can't run on a machine, and why in the hub's words
/// (`unavailable` of `GET …/machines/{id}/launchers?purpose=assistant`, #5033). New Agent's list (`agent.new`) has
/// the same shape for the Persona-managed providers whose own-service rule leaves them out there (#8109).
nonisolated struct UnavailableLauncher: Codable, Identifiable, Hashable, Sendable {
    var name: String
    var displayName: String?
    /// launcher | api
    var kind: String?
    var agent: String?
    /// A direct API provider's protocol: messages | responses | chat.
    var api: String?
    /// The hub's machine-readable refusal code (`launcher_out_of_scope`, `api_protocol`…).
    var code: String?
    /// The refusal, in the request's language.
    var reason: String
    var id: String { name }

    enum CodingKeys: String, CodingKey {
        case name, kind, agent, code, reason
        case displayName = "display_name", api
    }
}

/// What a launcher's agent offers in its own /model menu (`GET /api/v1/machines/{id}/launchers/{name}/models`,
/// docs/assistant/model-config.md「Choosing a model for a new Agent」): Claude Code's aliases and its gateway's models, or Codex's model/list.
nonisolated struct LauncherModels: Codable, Hashable, Sendable {
    /// What the launcher runs: claude, codex, traex, a runtime's.
    var agent: String
    /// Empty: its agent has no list to ask for.
    var models: [AgentModel]
    /// Why a picked model would not reach the agent (the launcher passes its own --model); nil: it would.
    var locked: String?
    /// Model Settings' list (#943): the machine whose CLI answered.
    var machine: String? = nil

    /// The node confirms this launcher and host can honor a model for this start. Absent on older nodes.
    var supportsStartModel: Bool? = nil

    enum CodingKeys: String, CodingKey {
        case agent, models, locked, machine
        case supportsStartModel = "supports_start_model"
    }

    /// The agent takes a model ID typed by hand (#1644): Claude Code's `--model` / `/model` and Codex's `--model` take any
    /// string, and so does a work agent started in herdr (`pi`, the Cursor CLI, the Antigravity CLI: `workagent.go`).
    /// New Agent separately supports a typed ID for a native Oh My Pi start.
    static func takesTypedModel(agent: String) -> Bool {
        agent == "claude" || agent == "codex" || agent == "pi" || agent == "cursor" || agent == "agy"
    }

    /// The shared model settings can take a typed ID and the launcher does not pass its own --model.
    var takesTypedModel: Bool { locked == nil && Self.takesTypedModel(agent: agent) }
}

nonisolated struct AgentModel: Codable, Hashable, Sendable, Identifiable {
    /// What a start sends as `model` (Claude's aliases too: opus, sonnet…).
    var id: String
    var name: String
    var description: String?
    /// The model an alias stands for with this launcher's settings (opus → its model id).
    var resolved: String?
    /// What the agent runs when no model is given: picking it is picking none.
    var isDefault: Bool = false
    /// The thinking efforts it takes, weakest first; nil or empty: none.
    var efforts: [String]?
    var defaultEffort: String?
    /// An ID the user typed before that ran, remembered on the hub for this computer and launcher (#1644); listed after
    /// the agent's own, most recent first, with no efforts (not known). It can be forgotten.
    var custom = false

    enum CodingKeys: String, CodingKey {
        case id, name, description, resolved, efforts, custom
        case isDefault = "default", defaultEffort = "default_effort"
    }

    init(id: String, name: String, description: String? = nil, resolved: String? = nil, isDefault: Bool = false,
         efforts: [String]? = nil, defaultEffort: String? = nil, custom: Bool = false) {
        self.id = id
        self.name = name
        self.description = description
        self.resolved = resolved
        self.isDefault = isDefault
        self.efforts = efforts
        self.defaultEffort = defaultEffort
        self.custom = custom
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? id
        description = try c.decodeIfPresent(String.self, forKey: .description)
        resolved = try c.decodeIfPresent(String.self, forKey: .resolved)
        isDefault = try c.decodeIfPresent(Bool.self, forKey: .isDefault) ?? false
        efforts = try c.decodeIfPresent([String].self, forKey: .efforts)
        defaultEffort = try c.decodeIfPresent(String.self, forKey: .defaultEffort)
        custom = try c.decodeIfPresent(Bool.self, forKey: .custom) ?? false
    }
}

nonisolated struct DirEntry: Codable, Hashable, Sendable, Identifiable {
    var name: String
    var path: String
    var isGit: Bool
    var id: String { path }
    enum CodingKeys: String, CodingKey { case name, path; case isGit = "is_git" }
}

nonisolated struct RecentDir: Codable, Hashable, Sendable, Identifiable {
    var path: String
    var name: String
    var lastUsed: Date?
    var id: String { path }
    enum CodingKeys: String, CodingKey { case path, name; case lastUsed = "last_used" }
}

nonisolated struct DirList: Codable, Sendable {
    var path: String
    var parent: String?
    var entries: [DirEntry]
    var recent: [RecentDir]?
}

nonisolated struct SessionTodoRef: Codable, Hashable, Sendable {
    var id: String
    var title: String
}

nonisolated enum SessionStatus: String, Codable, Sendable {
    case idle, working, blocked, done, unknown, offline

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = SessionStatus(rawValue: raw) ?? .unknown
    }

    var isActive: Bool { self == .working || self == .blocked }
    var isLive: Bool { self != .offline }
}

/// New sources opt in to every operation; a partial capability object never grants missing permissions.
/// `modes` alone is optional: an older hub omits it and the client keeps its agent-based fallback.
nonisolated struct SessionCapabilities: Codable, Hashable, Sendable {
    var read = false
    var send = false
    var interrupt = false
    var approve = false
    var terminal = false
    var attach = false
    var modes: Bool?
    var takeover: CodexTakeoverCapability? = nil
    /// The node's word on closing, renaming, switching the model and rewinding (#6294; docs/protocol/protocol.md "Codex
    /// sources and capabilities"): a runtime's session has no terminal, yet does these where its runtime can. Nil from an
    /// older node, or a value this app cannot read: the app keeps its terminal rules (`Session.canClose`, `canRename`,
    /// `switchesModel`, `Rewind.block`).
    var close: Bool?
    var rename: Bool?
    var model: Bool?
    var rewind: Bool?
    /// Rewind Code and Chat as well (`rewind_code`).
    var rewindCode: Bool?
    /// The permission modes an agent that is not Claude Code's takes (#8261, `permission_modes`: Paseo's Grok, OpenCode
    /// or Codex): `default`, its own approval, and `bypassPermissions` where it has an Allow Everything
    /// (`Session.listedModes`). Nil from an older node, on any other row, or a value this app cannot read.
    var permissionModes: [String]?

    enum CodingKeys: String, CodingKey {
        case read, send, interrupt, approve, terminal, attach, modes, takeover, close, rename, model, rewind
        case rewindCode = "rewind_code"
        case permissionModes = "permission_modes"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        read = try c.decodeIfPresent(Bool.self, forKey: .read) ?? false
        send = try c.decodeIfPresent(Bool.self, forKey: .send) ?? false
        interrupt = try c.decodeIfPresent(Bool.self, forKey: .interrupt) ?? false
        approve = try c.decodeIfPresent(Bool.self, forKey: .approve) ?? false
        terminal = try c.decodeIfPresent(Bool.self, forKey: .terminal) ?? false
        attach = try c.decodeIfPresent(Bool.self, forKey: .attach) ?? false
        modes = try c.decodeIfPresent(Bool.self, forKey: .modes)
        takeover = try c.decodeIfPresent(CodexTakeoverCapability.self, forKey: .takeover)
        close = try? c.decodeIfPresent(Bool.self, forKey: .close)
        rename = try? c.decodeIfPresent(Bool.self, forKey: .rename)
        model = try? c.decodeIfPresent(Bool.self, forKey: .model)
        rewind = try? c.decodeIfPresent(Bool.self, forKey: .rewind)
        rewindCode = try? c.decodeIfPresent(Bool.self, forKey: .rewindCode)
        permissionModes = try? c.decodeIfPresent([String].self, forKey: .permissionModes)
    }
}

nonisolated struct CodexTakeoverCapability: Codable, Hashable, Sendable {
    var running: Bool
    var canResume: Bool
    var reason: String?
    enum CodingKeys: String, CodingKey { case running, reason; case canResume = "can_resume" }
}

/// Resume in Terminal (#5521, docs/integrations/engines.md "Agents run in a terminal"): what the chat's ⋯ offers for an Agent
/// that isn't running, the one rule its ⋯, its stopped line and its terminal page go by (web `terminalResume`,
/// lib/agent-resume.ts). `Session.terminalResume(on:)` decides.
nonisolated enum TerminalResume: Equatable, Sendable {
    /// A stopped Agent: the node marks one so only where a terminal engine can host the resume.
    case available
    /// Not running, on a computer that enables no terminal engine (the empty engine set, #3302): offered disabled, saying so.
    case noEngine
    /// Not offered: it runs (in Persona's terminal or in one of its own: a second claude would write the same transcript),
    /// it starts, an assistant conversation, a desktop app's session (a message reopens it there, #4300), another source,
    /// or a row from an older node, which does not say.
    case hidden
}

/// Folded children of one session (`children` on a session row). Counts the hub omitted are zero.
nonisolated struct SessionChildren: Codable, Hashable, Sendable {
    var total: Int = 0
    var working: Int = 0
    var needsYou: Int = 0
    var workers: Int = 0
    /// Of them, the side sessions (`folded_as: "side"`, #7944) by their own states: how many, working, needing the user
    /// and with a result not looked at yet. Omitted from a hub without side folding, and when 0 (`SideFold`).
    var sides: Int = 0
    var sidesWorking: Int = 0
    var sidesNeedsYou: Int = 0
    var sidesUnread: Int = 0

    enum CodingKeys: String, CodingKey {
        case total, working, workers, sides
        case needsYou = "needs_you"
        case sidesWorking = "sides_working", sidesNeedsYou = "sides_needs_you", sidesUnread = "sides_unread"
    }

    init(total: Int = 0, working: Int = 0, needsYou: Int = 0, workers: Int = 0,
         sides: Int = 0, sidesWorking: Int = 0, sidesNeedsYou: Int = 0, sidesUnread: Int = 0) {
        self.total = total
        self.working = working
        self.needsYou = needsYou
        self.workers = workers
        self.sides = sides
        self.sidesWorking = sidesWorking
        self.sidesNeedsYou = sidesNeedsYou
        self.sidesUnread = sidesUnread
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        // An odd count reads as 0, never dropping the rollup.
        func count(_ key: CodingKeys) -> Int { (try? c.decodeIfPresent(Int.self, forKey: key)).flatMap { $0 } ?? 0 }
        total = count(.total)
        working = count(.working)
        needsYou = count(.needsYou)
        workers = count(.workers)
        sides = count(.sides)
        sidesWorking = count(.sidesWorking)
        sidesNeedsYou = count(.sidesNeedsYou)
        sidesUnread = count(.sidesUnread)
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(total, forKey: .total)
        try c.encode(working, forKey: .working)
        try c.encode(needsYou, forKey: .needsYou)
        try c.encode(workers, forKey: .workers)
        // The side keys only when there are sides, as the hub sends them.
        if sides > 0 { try c.encode(sides, forKey: .sides) }
        if sidesWorking > 0 { try c.encode(sidesWorking, forKey: .sidesWorking) }
        if sidesNeedsYou > 0 { try c.encode(sidesNeedsYou, forKey: .sidesNeedsYou) }
        if sidesUnread > 0 { try c.encode(sidesUnread, forKey: .sidesUnread) }
    }
}

/// A native Codex speed choice advertised for this session. Labels stay localized in the client; these are wire metadata.
nonisolated struct SessionServiceTier: Codable, Hashable, Sendable {
    var id: String
    var name: String
    var description: String? = nil
}

nonisolated struct Session: Codable, Identifiable, Hashable, Sendable {
    var machineID: String
    var sessionID: String
    var title: String
    var cwd: String
    var status: SessionStatus
    var paneID: String?
    var tabID: String?
    var workspaceID: String?
    var agentName: String?
    /// The coding agent in the pane: "claude" (or nil, from older hubs), "codex" or "traex" (TRAEX, an in-house Codex fork);
    /// for a session a runtime lists (DSH…, docs/integrations/runtimes.md), the agent it runs.
    var agent: String? = nil
    var sourceKind: String? = nil
    var sourceThreadID: String? = nil
    var sourceInstanceID: String? = nil
    var parentThreadID: String? = nil
    /// A Codex subagent (`thread_spawn`, docs/integrations/codex.md), or a runtime's session another one started (a Paseo
    /// agent another agent created, #4494): `is_subagent`, the session that started it (`parent_session_id`) and the
    /// conversation at the top of its tree (`root_session_id`). The Agents list folds it under that session, as the Codex
    /// App and Android do (`Session.isSubtask`, `CodexTasks`). Nil from older nodes.
    var isSubagent: Bool? = nil
    var parentSessionID: String? = nil
    var rootSessionID: String? = nil
    /// The session a question or approval is asking (`asking_parent_session_id`). The hub folds that child under it.
    var askingParentSessionID: String? = nil
    /// The parent this row folds under (`folded_under`), when the hub computed one. Empty from a hub without
    /// `sessions_hierarchy`, and for an orphan or a cycle. The row's own `state` is never replaced by the fold.
    var foldedUnder: String? = nil
    /// How it folds (`folded_as`): "worker" for a tab worker, "side" for a Claude desktop app's side session (#7944).
    /// Absent otherwise.
    var foldedAs: String? = nil
    /// The side session whose chat a folded row belongs to when it folds through one and is not the side itself
    /// (`folded_side`: a side's own children and workers, #7944, `SideFold`). Absent otherwise.
    var foldedSide: String? = nil
    /// The session on the same machine this one was started from while staying a session of its own (`parent_agent_id`,
    /// #5935): a Claude desktop app's side chat, /btw question or spawned session. Nil from older nodes, and when empty.
    var parentAgentID: String? = nil
    /// Folded children of this row (`children`). Omitted when there are none. Counts use folded state; this stays.
    var children: SessionChildren? = nil
    /// When the child started (`started_at`). A card is placed by this, not by a later `updated_at`.
    var startedAt: Date? = nil
    /// The hub's session status was `interrupted`. `SessionStatus` has no such case (it would be `.unknown`).
    var wasInterrupted = false
    var readOnlyReason: String? = nil
    var capabilities: SessionCapabilities? = nil
    var attached: Bool? = nil
    /// Branch to New Chat works for it (a Claude Code conversation the node can copy, #2137); older nodes never say so.
    var canFork: Bool? = nil
    /// How Persona reaches a Codex in a terminal (docs/integrations/codex.md「Sessions on the daemon」): "app_server" (joined on its shared
    /// app-server) or "terminal" (its screen and session record); the note says why it is the terminal.
    var codexRoute: String? = nil
    var codexRouteNote: String? = nil
    /// What the hub says to call an agent this app does not know (a runtime's own name, "DSH"); nil for the known ones.
    var agentLabelHint: String? = nil
    /// The client the agent runs in (`agent_surface`, #6390): "desktop" (Claude's or Codex's desktop app) or "cli" (its
    /// command line); nil when the node cannot tell (an SDK, a runtime, an older node). The command line's mark carries </>.
    var agentSurface: String? = nil
    var launcher: String?
    var model: String?
    /// Claude Code's permission mode and effort level right now (`SessionModes`); nil while unknown and for Codex
    /// (a Codex thread the hub drives through its app-server has an effort, never a mode).
    var permissionMode: String? = nil
    /// This Claude Code can be switched to bypassPermissions (Allow Everything) while it runs: its claude was started with
    /// a flag that puts that mode in its shift+tab cycle (`bypass_available`, #1379, read off the process by the node).
    /// False while unknown and from older hubs; then Allow Everything is only a start's choice.
    var bypassAvailable: Bool = false
    /// Allow Everything was picked for this Claude Code while it cannot reach it with shift+tab (`bypassAvailable` false):
    /// the node restarts it in that mode once its turn is over, resuming the same conversation (`bypass_pending`, #6476).
    /// Until then `permissionMode` stays the mode it runs in. False from older hubs.
    var bypassPending: Bool = false
    var effort: String? = nil
    var pendingEffort: String? = nil
    var effortChangeFailed: Bool? = nil
    /// The levels `effort` may be set to when they are not Claude Code's (`SessionModes.effortOptions`): a Codex thread
    /// the hub drives through its app-server, where only the effort changes (node/codex_effort.go). Nil elsewhere.
    var efforts: [String]? = nil
    /// The native Codex speed the hub last acknowledged (`service_tier`), nil while unknown. Never an optimistic pick.
    var serviceTier: String? = nil
    /// The native speed choices this session advertises (`service_tiers`), including `default` for Standard.
    var serviceTiers: [SessionServiceTier]? = nil
    /// The launcher / model / effort picked for this assistant conversation (#5033, `model_pick`); nil: it follows the
    /// Main chat default or keeps what it runs. Only the manager names it, on assistant conversations.
    var modelPick: ModelPick? = nil
    /// What the model saw in the agent's latest reply, and the model's window (#3289, node: `context_tokens` /
    /// `context_window`, #2430); nil while the node cannot tell (no reply yet, a window it does not know, an older node).
    var contextTokens: Int? = nil
    var contextWindow: Int? = nil
    /// Claude Code's session-only ultracode switch (#3427, docs/protocol/protocol.md "Claude Code ultracode"): what the
    /// node last saw, nil while unknown (no evidence yet, an older node, not Claude); never what was asked for.
    var ultracode: Bool? = nil
    /// The node's reason (fixed text, in Chinese) why the switch is unavailable, was refused, could not be confirmed
    /// or still waits; nil when it has none.
    var ultracodeNote: String? = nil
    /// A switch request waits for the turn to end or the terminal's box to clear: it applies next turn. A queue flag,
    /// not the value asked for.
    var ultracodePending: Bool = false
    /// When a session that had it on was resumed with it off: the chat offers to turn it back on (never done by itself).
    var ultracodeLostAt: Date? = nil
    var startedByPersona: Bool
    var assistant: Bool
    /// An assistant conversation moved out of the sidebar (listed under Archived; nothing is deleted).
    var archived: Bool? = nil
    /// Manager-owned visibility; active supervised tasks are exempt.
    var hiddenByDirectory: Bool? = nil
    /// The assistant's space it belongs to (the hub's `space`, docs/assistant/health.md): `healthSpace` for a health conversation,
    /// nil for the assistant's own. Not a host's space (`spaceKind` / `spaceID` are herdr's and Orca's).
    var assistantSpace: String? = nil
    var spaceKind: String?
    var spaceID: String?
    var spaceName: String?
    var spaceIndex: Int?
    /// The space this one is nested under in its host (same machine and `spaceKind`): an Orca child worktree's parent
    /// (`parent_space_id`, #2073). Nil for a top-level space and from older nodes.
    var parentSpaceID: String? = nil
    var slotID: String?
    var slotName: String?
    var slotIndex: Int?
    /// The pane's place in its tab (`pane_index`, #2320): 0 is the top left one, the others split from it, whose agent is
    /// the tab's Started It. Nil when the host does not say (older nodes, hosts without layouts).
    var paneIndex: Int? = nil
    /// The main checkout of the repository `cwd` belongs to, when its host knows it (`repo_root`, #2840: Paseo keeps its
    /// worktrees under its own folder, whose paths name no repository). Group by repository uses it (`RepoPlace`).
    var repoRoot: String? = nil
    /// The top of the git working tree `cwd` is in (`repoRoot` for the main checkout) and the branch checked out there,
    /// as the node reads them (`worktree`, `branch`, #1805); nil from an older node. By Repository lists machine →
    /// project → branch → agents with them (`RepoPlace`).
    var worktree: String? = nil
    var branch: String? = nil
    var state: AgentState
    var seenAt: Date?
    /// When the hub saw it enter its current state (`state_since`).
    var stateSince: Date? = nil
    var queued: [String]?
    /// The last this many queued messages came from other conversations and arrive when this turn ends (#4342).
    var queuedRelayed: Int? = nil
    /// In a Persona chat, the owner's messages that could not go in and that the assistant keeps and sends again by
    /// itself, oldest first (#10194): drawn as the owner's bubbles marked not delivered, on every device.
    var undelivered: [UndeliveredMessage]? = nil
    /// The user's own messages come first; only these can be edited, reordered, deleted or sent now.
    var ownQueuedCount: Int {
        let count = queued?.count ?? 0
        return count - min(max(queuedRelayed ?? 0, 0), count)
    }
    /// Shells / subagents Claude left running in the background.
    var background: [BackgroundTask]?
    /// Bounded list summaries supply scalars instead of chat-only arrays. Nil means a complete/legacy row.
    var listWaiting: Bool? = nil
    var listBackgroundCount: Int? = nil
    var listRunningSubagents: Int? = nil
    var listRunningWorkflows: Int? = nil
    /// Summary projection (#3588): independent live jobs; zero clears cached activity.
    var listBackgroundTasksCount: Int? = nil
    /// Newest running workflow, without chat-only agent/phase arrays.
    var listWorkflowProgress: WorkflowRun? = nil
    /// The /loop driving this agent (it wakes itself up on a schedule).
    var loop: LoopInfo?
    /// The goal it works toward until it holds (Claude Code's /goal, a Codex thread goal; docs/integrations/agent-goal.md).
    var goal: GoalInfo?
    /// Agents it launched (the Agent tool); each reads like a session ("<id>@<agent id>").
    var subagents: [Subagent]?
    /// Multi-agent workflows it runs in the background (Claude Code's Workflow tool, `Workflow.swift`); nil from older hubs.
    var workflows: [WorkflowRun]? = nil
    /// The ports its processes listen on that answer HTTP (docs/operations/ports.md): its dev servers, opened through the hub.
    var ports: [ListeningPort]? = nil
    var updatedAt: Date?
    var lastText: String?
    /// What the agent's last reply asks the user (`asks`, #5096): the question or decision its closing words put to
    /// them, read off the whole reply (`lastText` is only its beginning). Nil when it asks nothing, and from older hubs.
    var asks: String? = nil
    /// It stopped on a question to the user nobody answered yet (`awaits_reply`, docs/protocol/agent-states.md "Awaiting your reply"): set by
    /// the hub on a Unread or Seen one; an Seen one is listed as Awaiting Reply, not folded with the idle ones. False from
    /// older hubs.
    var awaitsReply: Bool = false
    /// Its turn has ended and all it still runs in the background only waits (`sleep`, `gh … --watch`, `tail -f`; the
    /// hub's `background_waiting`, #6045): set only with `state` working, never while a subagent or workflow runs. False
    /// from older hubs. The rule is `waitsInBackground`.
    var backgroundWaiting: Bool = false
    /// Its turn has ended while background commands, subagents, Monitors or workflows it started still run (the hub's
    /// `background_working`, #2935): set only with `state` working. False from older hubs. A task counts it as waiting
    /// (`waitsToBeWoken`, #3274).
    var backgroundWorking: Bool = false
    /// What it is doing when its latest step is a tool call no words came after (「在跑测试」「在读 tasks.go」), and since
    /// when; nil once it writes again. The hub's `activity` / `activity_at` (docs/protocol/protocol.md); older hubs send neither.
    var activity: String? = nil
    var activityAt: Date? = nil
    var pendingInteractions: Int
    /// Changes whenever the transcript does: the app pulls sessions whose rev it has not synced (`AgentSync`).
    var rev: Int64?
    /// The next message Claude Code or the hub proposes after a turn, until it works again. A risky one is fill-in only.
    var suggestion: String?
    /// Whether the suggestion needs review before sending. Missing, null or malformed values are safe (not risky).
    var suggestionRisky: Bool = false
    /// Persona is still opening its tab and starting claude: messages sent now go in once it is ready.
    var starting: Bool? = nil
    /// A Codex opened with no first message: it runs in its terminal with no thread yet, so this row's id is the
    /// computer's. Its first message (text only) goes in here or in the terminal; then the row goes and the thread's comes
    /// in the same pane (docs/integrations/codex.md「Starting without a first message」).
    var awaitingFirstMessage: Bool? = nil
    /// Starting it failed (shown in the chat; sends get the same error).
    var startError: String? = nil
    /// The node refused the start with a code (launcher_missing / claude_missing in the new tab's shell, #1642): then
    /// `startError` is the hub's whole sentence for it, in the user's language, its remedy last. Nil from older hubs.
    var startErrorCode: String? = nil
    /// The refusal's details as strings (launcher, where, missing, pane_shell…).
    var startErrorArgs: [String: String]? = nil
    /// The todo this session is linked to, filled by the hub (docs/protocol/protocol.md).
    var todo: SessionTodoRef? = nil
    /// It works for a task the main conversation started while Settings › Assistant › Notify Main-Chat Tasks Only in the Main Chat was on (#1731):
    /// the main conversation tells the user about it, so it stays off the island, notifications, badges and the watch.
    /// The hub keeps it after the task is done; false from a hub without it.
    var quiet: Bool = false
    /// Why it waits for the user, in its host's words (a botmux attention's reason); only while it needs you.
    var attentionReason: String? = nil
    /// Its host stopped its turn on an error it will not retry by itself (a botmux notice), until a message goes in.
    var failureReason: String? = nil
    /// How the assistant's last turn failed (docs/protocol/protocol.md「Error body」, #3531): the words, their kind, and a code when it
    /// is the model's set-up to fix (not_logged_in / auth_failed / model_route / rate_limited). Nil when it answered and
    /// from older hubs.
    var turnError: String? = nil
    var turnErrorKind: String? = nil
    var turnErrorCode: String? = nil
    /// A subscription usage limit's reset time, and whether the CLI goes on by itself after it (`turn_error_resets_at`,
    /// `turn_error_resumes`; docs/assistant/agent-work.md "Usage limits"). Nil from older hubs and for other errors.
    var turnErrorResetsAt: Date? = nil
    var turnErrorResumes: Bool? = nil
    /// Where it lives outside Persona (a botmux session's Feishu topic): Open in Lark.
    var externalURL: String? = nil
    /// The channels messages came into it from, newest first ("wechat", "feishu"; docs/extensions/channels.md「Source markers」): the
    /// sidebar draws the latest one's mark. Nil from hubs without channels.
    var channels: [String]? = nil
    /// Who started an assistant conversation (#3266): "user" for the user's own, else what opened it ("inbox": the Inbox's
    /// AI twin, "routine": a scheduled task, "lark_agent": Lark /agent…). The sidebar keeps the others in Auto Chats. Nil
    /// from hubs that do not say, which list every conversation as before.
    var origin: String? = nil
    /// The extension whose token opened an assistant conversation (protocol.md「Origin of assistant chats」); nil for any
    /// other. Such a chat never attends a meeting (#2981).
    var openedBy: String? = nil
    /// What of `origin` opened it (`origin_ref`): for Lark /agent and another extension's, that extension's id, which the
    /// rows draw the icon of when `openedBy` is not there (`ConversationOpener`, #10358). Nil from hubs that do not say.
    var originRef: String? = nil
    /// The last compaction the user asked for in an assistant conversation (POST …/compact, #1727): running, done or
    /// failed. Nil from hubs without `assistant_compact`, and before the first one.
    var compaction: CompactionRun? = nil
    /// Where it stands in its work (docs/assistant/agent-work.md): set by the hub once it triaged a stopped turn. Nil from hubs
    /// without the triage, and before it triaged.
    var work: WorkState? = nil
    /// An assistant conversation set to run on the Agent SDK (#2455): `agentsdk` while it does; and why the node ran it
    /// on the CLI instead (「找不到 Node.js 22+」). Both nil otherwise.
    var runtime: String? = nil
    var runtimeFallback: String? = nil
    /// After an upstream refusal Claude Code moved the session to a fallback model, still in use (#4489): the card above
    /// the composer. And going back to the configured model, while that is pending. Nil from older hubs.
    var modelFallback: ModelRefusal? = nil
    var modelRestore: ModelRestore? = nil
    /// The manager it belongs to (docs/protocol/protocol.md "Per-manager routing"); nil from hubs with one manager and
    /// older ones. Requests the hub routes by the manager they name (its answered questions) name it.
    var manager: String? = nil
    /// The group of paired hubs it came from (docs/protocol/multi-hub.md); nil for the default group. The app's own.
    var hubGroup: String? = nil

    /// Awaiting Reply (#5096): seen, stopped on a question to the user nobody answered yet, and not muted. The one rule
    /// for the lists' order and folds (`AgentListOrder.awaitsReply`) and for every word, color and question a row shows:
    /// a muted one is plain Seen everywhere, so callers say whether it is muted.
    func awaitsYourReply(muted: Bool) -> Bool { state == .idle && awaitsReply && !muted }
    func isJustFinished(muted: Bool) -> Bool { state == .idle && work?.justFinished == true && !muted }

    /// Group actions check mute preferences only for flagged idle rows.
    func needsSeen(muted: @autoclosure () -> Bool = false) -> Bool {
        state == .unread || (isJustFinished(muted: false) && !muted())
    }

    mutating func markLocallySeen() {
        if state == .unread { state = .idle }
        if work?.justFinished == true { work?.justFinished = nil }
    }

    /// Waiting in the background (#6045): working, but all it runs is waiting. Shown apart from Working and lighter, and
    /// left out of Working's counts and filter; whatever acts on a working agent (stop, queue) still treats it as one.
    /// A list copy whose state was rewritten to working because a child is working is not waiting in the background.
    var waitsInBackground: Bool {
        state == .working && backgroundWaiting && (children?.working ?? 0) == 0 && (children?.needsYou ?? 0) == 0
    }
    /// How many subagents the row says are running: its own reported count and its working folded children, once.
    var shownRunningChildren: Int { ChildSessions.shownRunning(self) }
    /// Its turn ended, yet something brings it back, so its task waits rather than stopped (#3274; the hub's agentWaits):
    /// only its background work runs and will report back, or the hub waits out another agent, a deploy / CI / a time,
    /// or a moment's model failure for it and wakes it (work blocked on anything but the user).
    var waitsToBeWoken: Bool {
        if state == .working { return backgroundWorking || backgroundWaiting }
        guard state != .needsYou, state != .unread, let work, work.phase == "blocked" else { return false }
        return work.on == "agent" || work.on == "external" || work.on == "transient"
    }
    /// What it waits for, in its own words, while it waits to be woken.
    var waitLine: String? {
        guard waitsToBeWoken, let work, work.phase == "blocked" else { return nil }
        let line = (work.line ?? work.wait ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        return line.isEmpty ? nil : line
    }
    /// The state counts and the quick filters go by: one that only waits in the background is counted with the quiet
    /// ones, not as Working (#6045).
    var countedState: AgentState { waitsInBackground ? .idle : state }
    /// Unique across groups; for the default group exactly what it always was.
    var id: String { (hubGroup.map { $0 + "|" } ?? "") + machineID + "/" + sessionID }
    /// What lists sort by, newest first: a working agent's updated_at moves on every step, so working ones keep
    /// the time they started working and stay put.
    var sortDate: Date { (state == .working ? stateSince : nil) ?? updatedAt ?? .distantPast }
    var key: SessionKey { SessionKey(group: group, machineID: machineID, sessionID: sessionID) }
    var machineRef: MachineRef { MachineRef(group: group, id: machineID) }
    /// The group it came from ("" = the default group).
    var group: String {
        get { hubGroup ?? "" }
        set { hubGroup = newValue.isEmpty ? nil : newValue }
    }
    var folderName: String { (cwd as NSString).lastPathComponent }

    /// The health space's name on the wire (`space: "health"`, docs/assistant/health.md).
    static let healthSpace = "health"
    /// A conversation of the health space: kept off the Persona page's lists, shown on Health.
    var isHealth: Bool { assistantSpace == Self.healthSpace }

    var hasSource: Bool { !(sourceKind ?? "").isEmpty }
    var canRead: Bool { !hasSource || capabilities?.read == true }
    // An Agent's own row with capabilities is one the node says what it can do (a stopped Agent: read and terminal only,
    // #4754); without them it can do everything, as before.
    var canSend: Bool { capabilities.map(\.send) ?? !hasSource }
    var canInterrupt: Bool { capabilities.map(\.interrupt) ?? !hasSource }
    var canApprove: Bool { capabilities.map(\.approve) ?? !hasSource }
    var canTerminal: Bool { capabilities.map(\.terminal) ?? !hasSource }
    /// Close it (#6294, the web's `sessionCan(s, "close")`): the node's `capabilities.close` where it says (a runtime's
    /// session closes without a terminal), else a terminal's, as before (an older node).
    var canClose: Bool { capabilities?.close ?? canTerminal }
    /// Rename it: the node's `capabilities.rename` where it says, else a terminal's (its tab and Claude's /rename).
    var canRename: Bool { capabilities?.rename ?? canTerminal }
    /// Its model can be switched while it runs (#3119; the web's `canSwitchModel`): where the node says so
    /// (`capabilities.model`: a runtime's session takes a pick through POST …/modes, #6294), else Claude Code in a
    /// terminal pane, which takes `/model`. Never an assistant conversation (its own chip) or one that cannot send.
    var switchesModel: Bool {
        guard !assistant, canSend else { return false }
        if let said = capabilities?.model { return said }
        return canTerminal && paneID != nil && isClaude && !isTraex
    }
    /// A model pick goes through POST …/modes rather than a typed `/model` (#6294): the node said it takes one. Such a
    /// session's menus offer its launcher's list only, never Claude Code's fixed aliases.
    var picksModelByModes: Bool { capabilities?.model == true }
    /// `/model` typed as a message switches its model: Claude Code in a terminal pane, which reads the command itself
    /// (the web's `paneClaude && canSwitchModel`). Only such a session gets `/model` in the composer's "/" list. A
    /// runtime's session (Paseo's Grok, #8144) would get the words as a plain message; its model menus switch it
    /// through POST …/modes instead.
    var takesModelCommand: Bool { switchesModel && isClaude && !hasSource && paneID != nil && canTerminal }
    /// Rewind Code and Chat is offered as well (the web's `rewind_code ?? !rewind`): the node's `rewind_code` where it
    /// declares rewind (#6294); a Claude terminal, which declares nothing, keeps both choices.
    var rewindsCode: Bool { capabilities?.rewindCode ?? (capabilities?.rewind != true) }
    /// An Agent of a terminal engine the node has no reader for (#6170): a CLI such as Kiro or gemini, or a pi / omp /
    /// opencode without Persona's extension. The node says so with `read_only_reason` on the row (node sessions.go
    /// `otherAgent`, pi.go, opencode.go); a pi or opencode with its extension has none and keeps its chat. Its terminal is the
    /// useful view, so the chat offers it at once.
    var isTerminalOnly: Bool {
        !assistant && !hasSource && canTerminal && !isClaude && !isCodex && !isTraex && !isStopped && !(readOnlyReason ?? "").isEmpty
    }
    /// A stopped Agent (#4754): its conversation and terminal stay, it takes no messages; Resume on its terminal page runs
    /// it again (node stopped.go). A Codex thread nothing has open is marked the same way (node codex_registry.go, #5521),
    /// though its row keeps its Codex history source.
    var isStopped: Bool {
        // "interrupted" (cut off by a restart) reads as unknown here; only the node's stopped rows carry capabilities.
        guard !assistant, let c = capabilities, !c.send, status == .offline || status == .unknown else { return false }
        return !hasSource || (sourceKind == "codex_history" && c.terminal)
    }
    var canAttach: Bool { hasSource && capabilities?.attach == true && attached != true }
    /// Delete Chat (docs/protocol/protocol.md `DELETE /sessions`) only for one that is not running: offline, finished (seen or not) or unknown,
    /// not still starting, and not the assistant's (those archive). What the hub hides once deleted (`stillDeleted`).
    var canDelete: Bool { !assistant && starting != true && (status == .offline || status == .done || status == .idle || status == .unknown) }
    var sourceLabel: String? {
        switch sourceKind {
        case "codex_app_server": String(localized: "Codex live")
        case "identity_conflict": String(localized: "Identity conflict")
        case "codex_history": String(localized: "Codex history")
        case "claude_bg": String(localized: "Claude background")
        // Claude outside Persona's terminals (docs/integrations/claude-integration.md "Claude outside Persona's terminals")
        case "claude_desktop": String(localized: "Claude desktop app")
        case "claude_cli": String(localized: "Claude in a terminal")
        case "runtime": agentLabel // an agent runtime's session (docs/integrations/runtimes.md): its own name, "DSH"
        case .some(let kind) where !kind.isEmpty: kind
        default: nil
        }
    }
    private static let nodeReadOnlyPrefix = "仅查看：" // i18n-ignore: the node's prefix it strips

    var sourceSubtitle: String? {
        guard let sourceLabel else { return nil }
        let detail = sourceKind == "identity_conflict" ? nil : agentName.flatMap { $0.isEmpty ? nil : $0 }
        let label = detail.map { sourceLabel + " · " + $0 } ?? sourceLabel
        // The node writes some reasons with their own "View only: " in Chinese, which this line adds already (#7803;
        // Android's `READ_ONLY_PREFIX`).
        let given = readOnlyReason.map { $0.hasPrefix(Self.nodeReadOnlyPrefix) ? String($0.dropFirst(Self.nodeReadOnlyPrefix.count)) : $0 }
        let reason = given.map { $0.trimmingCharacters(in: .whitespaces) }.flatMap { $0.isEmpty ? nil : $0 }
            ?? String(localized: "Sending isn't allowed from this source yet")
        return canSend ? label : String(localized: "\(label) · View only: \(reason)", comment: "A session's source, and why it can only be viewed")
    }

    /// The host tool it lives in, from its pane ref: `botmux:…`, `orca:…`, `herdr-core:…`, `tern:…`; any other pane is
    /// herdr's (its ids go bare and may hold a colon); nil without a pane. Keep the kinds in step with Kinds in
    /// server/internal/host/host.go.
    var hostKind: String? {
        guard let paneID, !paneID.isEmpty else { return nil }
        let kind = paneID.firstIndex(of: ":").map { String(paneID[..<$0]) } ?? ""
        return ["orca", "herdr-core", "botmux", "tern", "rex"].contains(kind) ? kind : "herdr"
    }

    /// Where it was opened (#2526): a botmux session is a Feishu group's topic, a herdr, Tern or Rex pane a terminal; nil
    /// for the rest (Orca, runtimes, apps), which carry no source label.
    var source: SessionSource? {
        switch hostKind {
        case "botmux": .feishu
        case "herdr", "herdr-core", "tern", "rex": .terminal
        default: nil
        }
    }

    var isCodex: Bool { agent == "codex" || sourceKind == "codex_app_server" || sourceKind == "codex_history" }
    var isTraex: Bool { agent == "traex" }
    /// Claude Code (or an older hub that does not name the agent): what the Claude-only controls (modes, /clear,
    /// /model) are for. Any other agent — named or not yet known to this app — does not get them.
    var isClaude: Bool { !isCodex && (agent == nil || agent == "" || agent == "claude") }
    /// Its queue can be sent now and reordered (#6088): a Claude Code in a terminal pane, whose own send-now ("ctrl+enter
    /// to send now" or "ctrl+x ctrl+s to send now", #2467) the node presses. Other agents refuse it
    /// (`send_now_unsupported`).
    var steersQueue: Bool { isClaude && !assistant && !hasSource && paneID != nil && canSend }
    /// A declared modes capability wins, including a runtime such as Orca or agy; an older hub that omits it keeps the old rule.
    /// As on the web, a non-assistant must be live, while an assistant can apply its modes to the next run even while offline.
    var canChangeModes: Bool { canSend && (assistant || status.isLive) && (capabilities?.modes ?? (assistant || isClaude)) }
    /// The permission modes an agent that is not Claude Code's switches between (#8261, `capabilities.permission_modes`:
    /// Paseo's Grok, OpenCode or Codex): its own approval and Allow Everything. Only with two or more, never for a
    /// Persona conversation, nor where Claude Code's modes stand (`capabilities.modes`, or a Claude Code row from a node
    /// that does not say).
    var listedModes: [String]? {
        guard let list = capabilities?.permissionModes, list.count >= 2, !assistant, capabilities?.modes != true,
              capabilities?.modes != nil || !isClaude else { return nil }
        return list
    }
    /// The listed modes can be switched now (`POST …/modes`): it runs and takes messages.
    var canSwitchListedModes: Bool { canSend && status.isLive && listedModes != nil }
    /// Only the thinking effort changes from the phone, among `efforts` (the hub says so by listing them): a Codex
    /// thread driven through its app-server. Claude Code, the assistant and a runtime that declares `capabilities.modes`
    /// (such as Antigravity) change both, the rest neither.
    var setsEffortOnly: Bool { canSend && !assistant && !isClaude && !canChangeModes && !(efforts ?? []).isEmpty }
    /// Who the user talks to in this session: "Claude", "Codex", "TRAEX", or what the hub calls a runtime's agent ("DSH",
    /// a Paseo provider's label such as "Grok"). Never the engine of several providers that runs it: a row that names
    /// only Paseo (no label of its own, or the node's "Paseo") names its provider by its launcher instead (#8144).
    var agentLabel: String {
        if isCodex { return "Codex" }
        if isTraex { return "TRAEX" }
        if isClaude { return "Claude" }
        if Self.namesOnlyEngine(agent: agent, label: agentLabelHint), let launcher, !launcher.isEmpty { return launcher }
        return agentLabelHint ?? agent ?? "Claude"
    }
    /// The engines that run agents of several providers and report every agent that is neither Claude Code nor Codex
    /// under their own id (Paseo's Grok, Kimi, Cursor…: `agent` "paseo"). Their name says nothing about who answers.
    static let providerEngines: Set<String> = [MachineEngine.paseo.rawValue]
    /// `label` names only the engine that runs `agent`, not its provider: an engine of several providers with no label
    /// for it, or its own name, which the node puts on a row or a card that has none ("Paseo"). The session's provider
    /// (its launcher, `agentLabel`) is the better name then.
    static func namesOnlyEngine(agent: String?, label: String?) -> Bool {
        guard let agent, providerEngines.contains(agent.lowercased()) else { return false }
        let name = label?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return name.isEmpty || name.caseInsensitiveCompare(agent) == .orderedSame
    }
    /// Resume in Terminal (#5521): whether the chat's ⋯ offers to bring it back in a terminal on its computer
    /// (`TerminalResume`); `machine` is its computer.
    func terminalResume(on machine: Machine?) -> TerminalResume {
        guard !assistant, starting != true, status == .offline || status == .unknown else { return .hidden }
        let agentRow = (!hasSource || sourceKind == "codex_history") && (isClaude || isCodex || isTraex)
        if agentRow && agentSurface != "desktop" && machine?.engineHosts?.isEmpty == true { return .noEngine }
        return isStopped ? .available : .hidden
    }
    /// How to pick the conversation up again on the computer (offered only with a terminal, canTerminal: a runtime's
    /// sessions are picked up in the runtime's own UI).
    var resumeCommand: String { isCodex ? "codex resume" : isTraex ? "traex resume" : "claude --resume" }
    /// `cd '<folder>' && claude --resume <id>`: the folder single-quoted, so a space, `;` or `$(…)` in it stays a path.
    /// Without a folder just the command: a bare `cd` would go home and resume in the wrong place.
    static func resumeLine(cwd: String?, command: String, id: String) -> String {
        guard let cwd, !cwd.isEmpty else { return "\(command) \(id)" }
        return "cd '" + cwd.replacingOccurrences(of: "'", with: #"'\''"#) + "' && \(command) \(id)"
    }

    enum CodingKeys: String, CodingKey {
        case queuedRelayed = "queued_relayed"
        case undelivered
        case listWaiting = "waiting", listBackgroundCount = "background_count", listRunningSubagents = "running_subagents", listRunningWorkflows = "running_workflows"
        case listBackgroundTasksCount = "background_tasks_count", listWorkflowProgress = "workflow_progress"
        case title, cwd, status, launcher, model, agent, effort, efforts, capabilities, attached
        case pendingEffort = "pending_effort", effortChangeFailed = "effort_change_failed"
        case hiddenByDirectory = "hidden_by_directory"
        case serviceTier = "service_tier", serviceTiers = "service_tiers"
        case modelPick = "model_pick"
        case canFork = "can_fork"
        case sourceKind = "source_kind", sourceThreadID = "source_thread_id", sourceInstanceID = "source_instance_id",
             parentThreadID = "parent_thread_id", readOnlyReason = "read_only_reason"
        case isSubagent = "is_subagent", parentSessionID = "parent_session_id", rootSessionID = "root_session_id"
        case askingParentSessionID = "asking_parent_session_id", foldedUnder = "folded_under", foldedAs = "folded_as"
        case children, startedAt = "started_at", foldedSide = "folded_side", parentAgentID = "parent_agent_id"
        case codexRoute = "codex_route", codexRouteNote = "codex_route_note"
        case agentLabelHint = "agent_label"
        case agentSurface = "agent_surface"
        case permissionMode = "permission_mode", bypassAvailable = "bypass_available", bypassPending = "bypass_pending"
        case contextTokens = "context_tokens", contextWindow = "context_window"
        case ultracode, ultracodeNote = "ultracode_note", ultracodePending = "ultracode_pending", ultracodeLostAt = "ultracode_lost_at"
        case repoRoot = "repo_root", worktree, branch
        case machineID = "machine_id", sessionID = "session_id", paneID = "pane_id", tabID = "tab_id",
             workspaceID = "workspace_id", agentName = "agent_name", startedByPersona = "started_by_persona", assistant, archived, state,
             assistantSpace = "space",
             spaceKind = "space_kind", spaceID = "space_id", spaceName = "space_name", spaceIndex = "space_index", parentSpaceID = "parent_space_id",
             slotID = "slot_id", slotName = "slot_name", slotIndex = "slot_index", paneIndex = "pane_index", seenAt = "seen_at", stateSince = "state_since", queued, background, loop, goal, subagents, workflows, ports,
             updatedAt = "updated_at", lastText = "last_text", asks, awaitsReply = "awaits_reply", backgroundWaiting = "background_waiting", backgroundWorking = "background_working", activity, activityAt = "activity_at", pendingInteractions = "pending_interactions", rev, suggestion, suggestionRisky = "suggestion_risky",
             starting, awaitingFirstMessage = "awaiting_first_message", startError = "start_error", startErrorCode = "start_error_code", startErrorArgs = "start_error_args", todo, quiet, hubGroup = "app_hub_group",
             attentionReason = "attention_reason", failureReason = "failure_reason",
             turnError = "turn_error", turnErrorKind = "turn_error_kind", turnErrorCode = "turn_error_code", turnErrorResetsAt = "turn_error_resets_at", turnErrorResumes = "turn_error_resumes", externalURL = "external_url", channels, origin, originRef = "origin_ref", openedBy = "opened_by", compaction, work,
             runtime, runtimeFallback = "runtime_fallback", manager, modelFallback = "model_fallback", modelRestore = "model_restore"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        machineID = try c.decodeIfPresent(String.self, forKey: .machineID) ?? ""
        sessionID = try c.decode(String.self, forKey: .sessionID)
        runtime = try c.decodeIfPresent(String.self, forKey: .runtime)
        runtimeFallback = try c.decodeIfPresent(String.self, forKey: .runtimeFallback)
        modelFallback = try c.decodeIfPresent(ModelRefusal.self, forKey: .modelFallback)
        modelRestore = try c.decodeIfPresent(ModelRestore.self, forKey: .modelRestore)
        manager = try c.decodeIfPresent(String.self, forKey: .manager)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        cwd = try c.decodeIfPresent(String.self, forKey: .cwd) ?? ""
        let rawStatus = try c.decodeIfPresent(String.self, forKey: .status)
        status = rawStatus.flatMap(SessionStatus.init(rawValue:)) ?? .unknown
        wasInterrupted = rawStatus == "interrupted"
        paneID = try c.decodeIfPresent(String.self, forKey: .paneID)
        tabID = try c.decodeIfPresent(String.self, forKey: .tabID)
        workspaceID = try c.decodeIfPresent(String.self, forKey: .workspaceID)
        agentName = try c.decodeIfPresent(String.self, forKey: .agentName)
        agent = try c.decodeIfPresent(String.self, forKey: .agent)
        sourceKind = try c.decodeIfPresent(String.self, forKey: .sourceKind)
        sourceThreadID = try c.decodeIfPresent(String.self, forKey: .sourceThreadID)
        sourceInstanceID = try c.decodeIfPresent(String.self, forKey: .sourceInstanceID)
        parentThreadID = try c.decodeIfPresent(String.self, forKey: .parentThreadID)
        isSubagent = try c.decodeIfPresent(Bool.self, forKey: .isSubagent)
        parentSessionID = try c.decodeIfPresent(String.self, forKey: .parentSessionID)
        rootSessionID = try c.decodeIfPresent(String.self, forKey: .rootSessionID)
        askingParentSessionID = try c.decodeIfPresent(String.self, forKey: .askingParentSessionID)
        // The hierarchy's keys: an odd one never drops the session; an empty one names nothing (#7944).
        foldedUnder = Self.lenientText(c, .foldedUnder)
        foldedAs = Self.lenientText(c, .foldedAs)
        foldedSide = Self.lenientText(c, .foldedSide)
        parentAgentID = Self.lenientText(c, .parentAgentID)
        // An odd rollup never drops the session. Omitted is none.
        children = (try? c.decodeIfPresent(SessionChildren.self, forKey: .children)).flatMap { $0 }
        startedAt = (try? c.decodeIfPresent(Date.self, forKey: .startedAt)).flatMap { $0 }.flatMap { $0.isMeaningful ? $0 : nil }
        readOnlyReason = try c.decodeIfPresent(String.self, forKey: .readOnlyReason)
        capabilities = try c.decodeIfPresent(SessionCapabilities.self, forKey: .capabilities)
        attached = try c.decodeIfPresent(Bool.self, forKey: .attached)
        canFork = try c.decodeIfPresent(Bool.self, forKey: .canFork)
        codexRoute = try c.decodeIfPresent(String.self, forKey: .codexRoute)
        codexRouteNote = try c.decodeIfPresent(String.self, forKey: .codexRouteNote)
        agentLabelHint = try c.decodeIfPresent(String.self, forKey: .agentLabelHint)
        agentSurface = try c.decodeIfPresent(String.self, forKey: .agentSurface)
        launcher = try c.decodeIfPresent(String.self, forKey: .launcher)
        model = try c.decodeIfPresent(String.self, forKey: .model)
        permissionMode = try c.decodeIfPresent(String.self, forKey: .permissionMode)
        bypassAvailable = (try? c.decodeIfPresent(Bool.self, forKey: .bypassAvailable)).flatMap { $0 } ?? false
        bypassPending = (try? c.decodeIfPresent(Bool.self, forKey: .bypassPending)).flatMap { $0 } ?? false
        effort = try c.decodeIfPresent(String.self, forKey: .effort)
        pendingEffort = try c.decodeIfPresent(String.self, forKey: .pendingEffort)
        effortChangeFailed = try c.decodeIfPresent(Bool.self, forKey: .effortChangeFailed)
        efforts = try c.decodeIfPresent([String].self, forKey: .efforts)
        serviceTier = try c.decodeIfPresent(String.self, forKey: .serviceTier)
        serviceTiers = try c.decodeIfPresent([SessionServiceTier].self, forKey: .serviceTiers)
        // The conversation's own pick (#5033): an odd one never drops the session, which then follows its default.
        modelPick = (try? c.decodeIfPresent(ModelPick.self, forKey: .modelPick)).flatMap { $0 }
        contextTokens = try? c.decodeIfPresent(Int.self, forKey: .contextTokens) // an odd one never drops the session
        contextWindow = try? c.decodeIfPresent(Int.self, forKey: .contextWindow)
        // An odd ultracode field never drops the session; omitted is unknown (an older hub or node), not off.
        ultracode = (try? c.decodeIfPresent(Bool.self, forKey: .ultracode)).flatMap { $0 }
        ultracodeNote = (try? c.decodeIfPresent(String.self, forKey: .ultracodeNote)).flatMap { $0 }.flatMap { $0.isEmpty ? nil : $0 }
        ultracodePending = (try? c.decodeIfPresent(Bool.self, forKey: .ultracodePending)).flatMap { $0 } ?? false
        ultracodeLostAt = (try? c.decodeIfPresent(Date.self, forKey: .ultracodeLostAt)).flatMap { $0 }.flatMap { $0.isMeaningful ? $0 : nil }
        let legacy = try decoder.container(keyedBy: HubNaming.Field.self)
        startedByPersona = try c.decodeIfPresent(Bool.self, forKey: .startedByPersona)
            ?? legacy.decodeIfPresent(Bool.self, forKey: HubNaming.Field(LegacyNames.startedByField)) ?? false
        assistant = try c.decodeIfPresent(Bool.self, forKey: .assistant) ?? false
        archived = try c.decodeIfPresent(Bool.self, forKey: .archived)
        hiddenByDirectory = try c.decodeIfPresent(Bool.self, forKey: .hiddenByDirectory)
        assistantSpace = try c.decodeIfPresent(String.self, forKey: .assistantSpace).flatMap { $0.isEmpty ? nil : $0 }
        spaceKind = try c.decodeIfPresent(String.self, forKey: .spaceKind)
        spaceID = try c.decodeIfPresent(String.self, forKey: .spaceID)
        spaceName = try c.decodeIfPresent(String.self, forKey: .spaceName)
        spaceIndex = try c.decodeIfPresent(Int.self, forKey: .spaceIndex)
        parentSpaceID = try c.decodeIfPresent(String.self, forKey: .parentSpaceID)
        slotID = try c.decodeIfPresent(String.self, forKey: .slotID)
        slotName = try c.decodeIfPresent(String.self, forKey: .slotName)
        slotIndex = try c.decodeIfPresent(Int.self, forKey: .slotIndex)
        paneIndex = try c.decodeIfPresent(Int.self, forKey: .paneIndex)
        repoRoot = try c.decodeIfPresent(String.self, forKey: .repoRoot)
        worktree = try c.decodeIfPresent(String.self, forKey: .worktree)
        branch = try c.decodeIfPresent(String.self, forKey: .branch)
        seenAt = try c.decodeIfPresent(Date.self, forKey: .seenAt)
        stateSince = try c.decodeIfPresent(Date.self, forKey: .stateSince)
        queued = try c.decodeIfPresent([String].self, forKey: .queued)
        queuedRelayed = try c.decodeIfPresent(Int.self, forKey: .queuedRelayed)
        undelivered = try? c.decodeIfPresent([UndeliveredMessage].self, forKey: .undelivered) // a malformed one never hides the row
        background = try c.decodeIfPresent([BackgroundTask].self, forKey: .background)
        listWaiting = try c.decodeIfPresent(Bool.self, forKey: .listWaiting)
        listBackgroundCount = try c.decodeIfPresent(Int.self, forKey: .listBackgroundCount)
        listRunningSubagents = try c.decodeIfPresent(Int.self, forKey: .listRunningSubagents)
        listRunningWorkflows = try c.decodeIfPresent(Int.self, forKey: .listRunningWorkflows)
        listBackgroundTasksCount = try c.decodeIfPresent(Int.self, forKey: .listBackgroundTasksCount)
        listWorkflowProgress = (try? c.decodeIfPresent(WorkflowRun.self, forKey: .listWorkflowProgress)).flatMap { $0 }
        loop = try c.decodeIfPresent(LoopInfo.self, forKey: .loop)
        goal = try c.decodeIfPresent(GoalInfo.self, forKey: .goal)
        subagents = try c.decodeIfPresent([Subagent].self, forKey: .subagents)
        // An odd workflow (or a shape this app does not know) never drops the session.
        workflows = (try? c.decodeIfPresent([Lossy<WorkflowRun>].self, forKey: .workflows))?.compactMap(\.value)
        ports = try c.decodeIfPresent([ListeningPort].self, forKey: .ports)
        state = try c.decodeIfPresent(AgentState.self, forKey: .state) ?? AgentState(status: status)
        updatedAt = try c.decodeIfPresent(Date.self, forKey: .updatedAt)
        lastText = try c.decodeIfPresent(String.self, forKey: .lastText)
        asks = (try? c.decodeIfPresent(String.self, forKey: .asks)).flatMap { $0 } // an odd one never drops the session
        awaitsReply = (try? c.decodeIfPresent(Bool.self, forKey: .awaitsReply)).flatMap { $0 } ?? false
        backgroundWaiting = (try? c.decodeIfPresent(Bool.self, forKey: .backgroundWaiting)).flatMap { $0 } ?? false
        backgroundWorking = (try? c.decodeIfPresent(Bool.self, forKey: .backgroundWorking)).flatMap { $0 } ?? false
        activity = try c.decodeIfPresent(String.self, forKey: .activity)
        activityAt = try c.decodeIfPresent(Date.self, forKey: .activityAt)
        pendingInteractions = try c.decodeIfPresent(Int.self, forKey: .pendingInteractions) ?? 0
        rev = try c.decodeIfPresent(Int64.self, forKey: .rev)
        suggestion = try c.decodeIfPresent(String.self, forKey: .suggestion)
        suggestionRisky = (try? c.decodeIfPresent(Bool.self, forKey: .suggestionRisky)).flatMap { $0 } ?? false
        starting = try c.decodeIfPresent(Bool.self, forKey: .starting)
        awaitingFirstMessage = try c.decodeIfPresent(Bool.self, forKey: .awaitingFirstMessage)
        startError = try c.decodeIfPresent(String.self, forKey: .startError)
        startErrorCode = try? c.decodeIfPresent(String.self, forKey: .startErrorCode)
        startErrorArgs = try? c.decodeIfPresent([String: String].self, forKey: .startErrorArgs)
        todo = try c.decodeIfPresent(SessionTodoRef.self, forKey: .todo)
        quiet = try c.decodeIfPresent(Bool.self, forKey: .quiet) ?? false
        attentionReason = try c.decodeIfPresent(String.self, forKey: .attentionReason)
        failureReason = try c.decodeIfPresent(String.self, forKey: .failureReason)
        turnError = try c.decodeIfPresent(String.self, forKey: .turnError)
        turnErrorKind = try c.decodeIfPresent(String.self, forKey: .turnErrorKind)
        turnErrorCode = try c.decodeIfPresent(String.self, forKey: .turnErrorCode)
        turnErrorResetsAt = try? c.decodeIfPresent(Date.self, forKey: .turnErrorResetsAt)
        turnErrorResumes = try? c.decodeIfPresent(Bool.self, forKey: .turnErrorResumes)
        externalURL = try c.decodeIfPresent(String.self, forKey: .externalURL)
        channels = try c.decodeIfPresent([String].self, forKey: .channels)
        origin = try c.decodeIfPresent(String.self, forKey: .origin)
        openedBy = try c.decodeIfPresent(String.self, forKey: .openedBy)
        originRef = try? c.decodeIfPresent(String.self, forKey: .originRef) // an odd one never drops the session
        compaction = try? c.decodeIfPresent(CompactionRun.self, forKey: .compaction) // an odd one never drops the session
        work = try? c.decodeIfPresent(WorkState.self, forKey: .work) // an odd one never drops the session
        hubGroup = try c.decodeIfPresent(String.self, forKey: .hubGroup)
    }

    /// A text key a newer hub added: another type reads as none, and so does an empty one.
    private static func lenientText(_ c: KeyedDecodingContainer<CodingKeys>, _ key: CodingKeys) -> String? {
        (try? c.decodeIfPresent(String.self, forKey: key)).flatMap { $0 }.flatMap { $0.isEmpty ? nil : $0 }
    }
}

/// A text key a newer hub added to a card: another type, or an empty string, reads as none and never drops the card.
nonisolated struct LenientText: Codable, Hashable, Sendable {
    var value: String?

    init(_ value: String?) { self.value = value }

    init(from decoder: Decoder) throws {
        value = (try? decoder.singleValueContainer().decode(String.self)).flatMap { $0.isEmpty ? nil : $0 }
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        if let value { try c.encode(value) } else { try c.encodeNil() }
    }
}

/// One of `Session.undelivered`: the owner's message the assistant keeps after it could not go in (#10194).
nonisolated struct UndeliveredMessage: Codable, Hashable, Sendable, Identifiable {
    /// Its client_msg_id.
    var id: String
    var text: String
    /// How many pictures it carries.
    var images: Int? = nil
    var at: Date
    /// `waiting`: sent again by itself; `failed`: it gave up (Retry / Delete).
    var state: String
    var attempts: Int? = nil
    var nextAt: Date? = nil
    var error: String? = nil
    var deleteOnly: Bool? = nil
    /// An Agent message held by its computer's manager. Its error is the complete status line.
    var computer: String? = nil

    static let agentHoldFeature = "agent_message_hold"
    var heldForComputer: Bool { computer?.isEmpty == false }
    var heldStatus: String? {
        guard heldForComputer else { return nil }
        if let error, !error.isEmpty { return error }
        return gaveUp ? String(localized: "Delivery status unknown") : String(localized: "Waiting to send")
    }

    /// The hub's names (`proto.UndeliveredMessage`): one enum, #10775 (`delete_only`) and #10440 (`next_at`) each added one.
    private enum CodingKeys: String, CodingKey {
        case id, text, images, at, state, attempts, error, computer
        case nextAt = "next_at", deleteOnly = "delete_only"
    }

    var gaveUp: Bool { state == "failed" }
    var canRetry: Bool { gaveUp && deleteOnly != true }

    /// The row's kept messages to draw: not one the transcript has (`landed`: its items' client_msg_ids) nor one this
    /// phone still draws as sending or arriving (`drawn`: those bubbles' ids), compared case-insensitively. The row can
    /// come before the send's answer, or lag behind the transcript's line (web `shownUndelivered`).
    static func shown(_ list: [UndeliveredMessage]?, landed: [String], drawn: [String]) -> [UndeliveredMessage] {
        guard let list, !list.isEmpty else { return [] }
        let skip = Set((landed + drawn).map { $0.lowercased() })
        return list.filter { !skip.contains($0.id.lowercased()) }
    }
}

/// A session's work state (docs/assistant/agent-work.md): the hub triages each stopped turn into done or blocked on something
/// named, with one line for the lists. Only `phase` is always there.
nonisolated struct WorkState: Codable, Hashable, Sendable {
    /// working / blocked / done.
    var phase: String
    /// What a blocked one waits on: user (a decision), agent, external (a deploy, CI, a time), transient (a model error).
    var on: String? = nil
    /// One line for the lists: what it waits for, or what it concluded.
    var line: String? = nil
    var wait: String? = nil
    var then: String? = nil
    /// When the hub acts next on its own: Continue (transient) or a wake (external, agent).
    var wakeAt: Date? = nil
    /// When a usage limit resets (`resets_at`), for a wait on one.
    var resetsAt: Date? = nil
    var attempts: Int? = nil
    var max: Int? = nil
    /// A recently finished turn nobody has seen. Hubs predating the flag omit it.
    var justFinished: Bool? = nil
    /// The agent it waits for.
    var agent: WorkAgent? = nil
    /// What the work produced: pull requests, commits, documents, builds (`WorkArtifact`). Nil from a hub without them.
    var artifacts: [WorkArtifact]? = nil
    /// What the agent said of its own work as the turn ended (`Closeout`, #3612). A turn the hub could not call finished
    /// carries it with an empty `phase` and a line. Nil when it wrote none, and from a hub before closeouts.
    var closeout: Closeout? = nil

    enum CodingKeys: String, CodingKey {
        case phase, on, line, wait, then, attempts, max, agent, artifacts, closeout
        case wakeAt = "wake_at", resetsAt = "resets_at", justFinished = "just_finished"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        phase = try c.decode(String.self, forKey: .phase)
        on = try c.decodeIfPresent(String.self, forKey: .on)
        line = try c.decodeIfPresent(String.self, forKey: .line)
        wait = try c.decodeIfPresent(String.self, forKey: .wait)
        then = try c.decodeIfPresent(String.self, forKey: .then)
        wakeAt = try c.decodeIfPresent(Date.self, forKey: .wakeAt)
        resetsAt = try c.decodeIfPresent(Date.self, forKey: .resetsAt)
        attempts = try c.decodeIfPresent(Int.self, forKey: .attempts)
        max = try c.decodeIfPresent(Int.self, forKey: .max)
        justFinished = try c.decodeIfPresent(Bool.self, forKey: .justFinished)
        agent = try c.decodeIfPresent(WorkAgent.self, forKey: .agent)
        // New (#3612): one that does not decode is as if there were none, never the whole work state failing.
        artifacts = try? c.decodeIfPresent([WorkArtifact].self, forKey: .artifacts)
        closeout = try? c.decodeIfPresent(Closeout.self, forKey: .closeout)
    }

    /// A turn nobody can call finished (docs/assistant/agent-work.md "Closeout"): the hub publishes it with an empty
    /// phase, the closeout and a line saying why. It stays Unread and never counts as waiting.
    var notFinished: Bool { phase.isEmpty && closeout != nil && !(line ?? "").isEmpty }

    /// The hub waits it out: it wakes it (`wake_at`), or it wakes itself (its background tasks, a /loop). Waiting in the
    /// lists (the web's `hubWaits`, lib/work.ts).
    var hubWaits: Bool {
        guard phase == "blocked", on != "user" else { return false }
        return wakeAt != nil || (on == "external" && (wait ?? "").isEmpty && agent == nil)
    }
}

/// The session triage's switches (`GET / PUT /api/v1/agents/triage`, docs/assistant/agent-work.md「Switches」): the triage itself, then
/// what it may do, each only while it is on. A switch the hub leaves out is off, apart from `replyReminder`.
nonisolated struct TriageSwitches: Codable, Hashable, Sendable {
    var enabled = false
    /// Waits for the user's decision become Needs You cards.
    var decide = false
    /// Continue after a passing model API error.
    var autoContinue = false
    /// Wakes the agent to check on a deploy, CI or another agent.
    var wake = false
    /// Remind When Seen, Unanswered (#5096): one push when an agent asked the user something, they looked and did not answer for a while.
    /// Rules only (no model), so it is apart from the triage and on when the hub leaves it out.
    var replyReminder = true
    /// After how many minutes since the user looked (the hub's default, 30, when nil). Not changed here: sent back
    /// as it came so a PUT keeps it.
    var replyReminderMinutes: Int? = nil
    /// The hub's wait when it names none (or 0): 30 minutes.
    static let defaultReplyReminderMinutes = 30
    /// After how many minutes since the user looked the hub reminds (what Settings › Session Triage says).
    var replyReminderAfter: Int { replyReminderMinutes.flatMap { $0 > 0 ? $0 : nil } ?? Self.defaultReplyReminderMinutes }
    /// Ask for a Closeout (#3612): hold a stop once when a turn that did work ends without one. Nil from a hub that
    /// does not know it: Settings shows no switch then, and a PUT leaves it out, so the hub keeps its value.
    var closeout: Bool? = nil
    /// The switch as Settings flips it (off while the hub names none).
    var asksCloseout: Bool {
        get { closeout ?? false }
        set { closeout = newValue }
    }
    /// Ask for the Status Afterwards (#3616): ask once for the status of an unsupervised agent that stopped after work
    /// without a closeout or a clear finish. The same missing-field rule as `closeout`, independently of that switch.
    var closeoutAsk: Bool? = nil
    var asksForStatusAfterwards: Bool {
        get { closeoutAsk ?? false }
        set { closeoutAsk = newValue }
    }

    enum CodingKeys: String, CodingKey {
        case enabled, decide, wake, closeout
        case autoContinue = "auto_continue"
        case closeoutAsk = "closeout_ask"
        case replyReminder = "reply_reminder", replyReminderMinutes = "reply_reminder_minutes"
    }

    init(enabled: Bool = false, decide: Bool = false, autoContinue: Bool = false, wake: Bool = false,
         replyReminder: Bool = true, replyReminderMinutes: Int? = nil, closeout: Bool? = nil, closeoutAsk: Bool? = nil) {
        self.enabled = enabled
        self.decide = decide
        self.autoContinue = autoContinue
        self.wake = wake
        self.replyReminder = replyReminder
        self.replyReminderMinutes = replyReminderMinutes
        self.closeout = closeout
        self.closeoutAsk = closeoutAsk
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        decide = try c.decodeIfPresent(Bool.self, forKey: .decide) ?? false
        autoContinue = try c.decodeIfPresent(Bool.self, forKey: .autoContinue) ?? false
        wake = try c.decodeIfPresent(Bool.self, forKey: .wake) ?? false
        replyReminder = (try? c.decodeIfPresent(Bool.self, forKey: .replyReminder)).flatMap { $0 } ?? true
        replyReminderMinutes = (try? c.decodeIfPresent(Int.self, forKey: .replyReminderMinutes)).flatMap { $0 }
        closeout = (try? c.decodeIfPresent(Bool.self, forKey: .closeout)).flatMap { $0 }
        closeoutAsk = (try? c.decodeIfPresent(Bool.self, forKey: .closeoutAsk)).flatMap { $0 }
    }
}

/// The agent a blocked session waits for (`work.agent`).
nonisolated struct WorkAgent: Codable, Hashable, Sendable {
    var machineID: String
    var sessionID: String
    var label: String? = nil

    enum CodingKeys: String, CodingKey {
        case label
        case machineID = "machine_id", sessionID = "session_id"
    }
}

/// A TCP port an agent's processes listen on (docs/operations/ports.md).
nonisolated struct ListeningPort: Codable, Identifiable, Hashable, Sendable {
    var port: Int
    var addr: String?
    var pid: Int?
    var process: String?
    /// The <title> of its page.
    var title: String?

    var id: Int { port }
}

/// An agent a session launched (the Agent tool), with its own transcript under the session id "<parent>@<id>".
nonisolated struct Subagent: Codable, Identifiable, Hashable, Sendable {
    var id: String
    var sessionID: String
    var type: String?
    var description: String?
    var toolUseID: String?
    var running: Bool
    /// The subagent that launched it; nil when the session did. One not in the list: shown at the top level.
    var parentID: String?
    /// A fork of its launcher's conversation.
    var fork: Bool?
    var background: Bool?
    var startedAt: Date?
    var updatedAt: Date?
    var lastText: String?
    /// How many tool calls it made, and its latest one (what it is doing while it runs); nil from older hubs (#5602).
    var steps: Int?
    var lastTool: LastTool?

    /// A subagent's latest tool call.
    nonisolated struct LastTool: Codable, Hashable, Sendable {
        var id: String?
        var name: String?
        /// What it does, in a few words ("running the tests").
        var summary: String?
    }

    var title: String { (description?.isEmpty == false ? description : nil) ?? type ?? String(localized: "Subagent [Task Note Row 70da69]") }

    enum CodingKeys: String, CodingKey {
        case id, type, description, running, fork, background, steps
        case sessionID = "session_id", toolUseID = "tool_use_id", parentID = "parent_id", startedAt = "started_at",
             updatedAt = "updated_at", lastText = "last_text", lastTool = "last_tool"
    }
}

nonisolated extension Subagent {
    /// Whether it runs, as the chat's own lines tell it (#5602, the web's rule; Claude Desktop goes by the call's result
    /// too): one its session waits on is over once its Agent call's result is in, whatever the hub's list says (that
    /// list goes by when its transcript was last written, and one dropped from it kept its last word: Running for ever);
    /// with no result yet it runs while its session is at work or the list says it does. One sent to the background (its
    /// result is the launch receipt) is the list's word until it reported back (a notice for its call). A call not in the
    /// loaded lines: the list's word.
    func running(in tasks: TaskIndex, sessionWorks: Bool = false) -> Bool {
        guard let call = toolUseID else { return running }
        if let status = tasks.agentStatuses[call] { return !SubagentDelivery.isTerminal(status) }
        if tasks.started.contains(call) { return running && !tasks.notes[call, default: []].contains { $0.status != "event" } }
        if tasks.answered.contains(call) { return false }
        return running || (sessionWorks && tasks.asked.contains(call))
    }

    /// The same with `running` as the chat's lines tell it.
    func shown(in tasks: TaskIndex, sessionWorks: Bool = false) -> Subagent {
        var a = self
        a.running = running(in: tasks, sessionWorks: sessionWorks)
        return a
    }

    /// A line of `tree(_:)`: the subagent and how deep it sits (0: the session launched it, or its launcher is not
    /// listed; 1: a subagent did; …).
    struct TreeRow: Hashable, Sendable {
        var agent: Subagent
        var depth: Int
    }

    /// Every subagent by who launched whom (`parentID`), flattened for a list: each right under its launcher, one level
    /// deeper; one whose launcher is not listed (it ended before the node's one-day cutoff) at the top level. Within a
    /// level running ones first, then the newest first (as the node lists them; web `subagentTree`).
    static func tree(_ all: [Subagent]) -> [TreeRow] {
        let listed = Set(all.map(\.id))
        let ordered = all.enumerated().sorted { a, b in
            if a.element.running != b.element.running { return a.element.running }
            let x = a.element.startedAt ?? .distantPast, y = b.element.startedAt ?? .distantPast
            return x != y ? x > y : a.offset < b.offset
        }.map(\.element)
        var kids: [String: [Subagent]] = [:]
        for a in ordered {
            let p = a.parentID.flatMap { $0 != a.id && listed.contains($0) ? $0 : nil } ?? ""
            kids[p, default: []].append(a)
        }
        var out: [TreeRow] = []
        var placed: Set<String> = []
        func place(_ a: Subagent, depth: Int) {
            guard placed.insert(a.id).inserted else { return }
            out.append(TreeRow(agent: a, depth: depth))
            for k in kids[a.id] ?? [] { place(k, depth: depth + 1) }
        }
        for a in kids[""] ?? [] { place(a, depth: 0) }
        // Launchers naming each other (only a runtime could write that) are reached from nowhere: list them at the top.
        for a in ordered { place(a, depth: 0) }
        return out
    }
}

/// A goal the agent keeps working toward until it holds (docs/integrations/agent-goal.md): Claude Code's /goal, whose hook checks the
/// condition each time it would stop, or a Codex thread goal.
nonisolated struct GoalInfo: Codable, Hashable, Sendable {
    var objective: String
    /// active / paused / blocked / usage_limited / budget_limited / complete (Claude Code's are only active or complete).
    var status: String
    /// Claude Code's latest check of the condition: why it holds, or what is still missing.
    var reason: String?
    var tokensUsed: Int?
    var tokenBudget: Int?
    var seconds: Int?
    var updatedAt: Date?

    var isComplete: Bool { status == "complete" }
    /// Still being worked on (anything else has stopped: done, paused, out of budget…).
    var isActive: Bool { status == "active" }

    /// The status in words, as the goal bar and its sheet say it.
    var statusLabel: String {
        switch status {
        case "active": String(localized: "Active")
        case "complete": String(localized: "Achieved")
        case "paused": String(localized: "Paused")
        case "blocked": String(localized: "Blocked")
        case "usage_limited": String(localized: "Usage Limit")
        case "budget_limited": String(localized: "Out of Budget")
        default: status
        }
    }

    /// The objective's first line, for the one-line bar.
    var headline: String {
        objective.split(whereSeparator: \.isNewline).first.map(String.init)?.trimmingCharacters(in: .whitespaces) ?? objective
    }

    enum CodingKeys: String, CodingKey {
        case objective, status, reason, seconds
        case tokensUsed = "tokens_used", tokenBudget = "token_budget", updatedAt = "updated_at"
    }
}

/// A /loop: Claude re-runs a prompt on its own, scheduling each next wakeup (kind "wakeup") or by cron ("cron").
nonisolated struct LoopInfo: Codable, Hashable, Sendable {
    var kind: String
    var prompt: String?
    var reason: String?
    var intervalSeconds: Int?
    var nextAt: Date?
    var cron: String?
    var ticks: Int?

    /// The task, without the "/loop" command word.
    var task: String {
        let p = (prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        return p.hasPrefix("/loop") ? String(p.dropFirst(5)).trimmingCharacters(in: .whitespaces) : p
    }

    /// "Every 30 min" / "cron 33 17 * * *".
    var cadence: String? {
        if kind == "cron", let cron { return String(localized: "Cron \(cron)") }
        guard let s = intervalSeconds, s > 0 else { return nil }
        if s < 120 { return String(localized: "Every \(s) seconds") }
        if s < 7200 { return String(localized: "Every \(s / 60) minutes") }
        return String(localized: "Every \(s / 3600) hours")
    }

    enum CodingKeys: String, CodingKey {
        case kind, prompt, reason, cron, ticks
        case intervalSeconds = "interval_seconds", nextAt = "next_at"
    }
}

/// A Bash command or subagent running in the background (`run_in_background`), until it reports back.
nonisolated struct BackgroundTask: Codable, Identifiable, Hashable, Sendable {
    var id: String
    var kind: String // "shell" | "agent"
    var description: String?
    var command: String?
    var outputFile: String?
    var startedAt: Date?
    /// A Monitor stops by itself then; nil when it did not say.
    var expiresAt: Date? = nil
    /// A workflow's run: which of the session's `workflows` it is.
    var runID: String? = nil

    var isAgent: Bool { kind == "agent" }
    /// A multi-agent workflow (`kind: workflow`; its progress is in the session's `workflows`).
    var isWorkflow: Bool { kind == "workflow" }
    /// What to call it in one line: the description Claude gave, else the command.
    var title: String {
        if let d = description, !d.isEmpty { return d }
        return command?.components(separatedBy: "\n").first ?? (isWorkflow ? "Workflow" : isAgent ? String(localized: "Subagent [Models acd37b]") : String(localized: "Background command"))
    }

    enum CodingKeys: String, CodingKey {
        case id, kind, description, command
        case outputFile = "output_file", startedAt = "started_at", expiresAt = "expires_at", runID = "run_id"
    }
}

nonisolated extension Session {
    /// The same apart from what ticks on every step of a working agent: `rev` (the transcript grew) and `updatedAt`.
    /// Views that show neither take a push of only those as no change (docs/apple/ios-state.md「Step 2」).
    static func sameApartFromTicks(_ a: Session?, _ b: Session?) -> Bool {
        guard var a, var b else { return a == nil && b == nil }
        a.rev = nil
        b.rev = nil
        a.updatedAt = nil
        b.updatedAt = nil
        return a == b
    }

    /// The same as far as views about this one session show it (`AppModel.session(_:)`: its chat, its top bar, its
    /// menus): apart from the ticks and the last line and step, which only the lists and the task cards show
    /// (docs/apple/ios-state.md「Step 3」).
    static func sameForItsPage(_ a: Session?, _ b: Session?) -> Bool {
        sameApartFromTicks(a.map(\.withoutLine), b.map(\.withoutLine))
    }

    private var withoutLine: Session {
        var x = self
        x.lastText = nil
        x.activity = nil
        x.activityAt = nil
        // A running subagent's transcript time and last line tick with each of its steps: the page shows neither (its
        // sheet shows the time it started, its write time only when that is missing), and the whole chat page ran
        // again for each (#5602).
        x.subagents = x.subagents?.map { a in
            var a = a
            a.lastText = nil
            if a.startedAt != nil { a.updatedAt = nil }
            return a
        }
        return x
    }
}

extension Session {
    /// Space this session belongs to, unique across machines (nil when it is not running in a host like herdr).
    var spaceKey: String? { spaceID.map { "\(machineID)/\($0)" } }

    /// Tab label, or "#n" when the label is just herdr's default number.
    nonisolated var slotLabel: String? {
        guard let name = slotName, !name.isEmpty else { return slotIndex.map { "#\($0)" } }
        if let i = slotIndex, name == String(i) { return "#\(i)" }
        return name
    }

    nonisolated var displayTitle: String { title.isEmpty || title == "Claude Code" ? folderName : title }
    /// The line a list row shows under the title: why it waits for you when its host says so, what it concluded or
    /// waits for once the hub triaged its turn (`workLine`), else the last words; Awaiting Reply's question first unless
    /// it is muted (`replyLine`).
    func listPreview(muted: Bool) -> String? {
        if awaitingFirstMessage == true, state != .needsYou { return String(localized: "Waiting for its first message") } // a Codex opened with no first message
        if state == .needsYou, let r = attentionReason, !r.isEmpty { return r } // a stale reason (offline, seen) stays out
        if state != .working, let r = failureReason, !r.isEmpty { return Session.failureText(r) }
        if let r = replyLine(muted: muted) { return r }
        // The native Mac app is frozen (docs/apple/mac.md): its rows keep the line they had.
        #if !os(macOS)
        if let c = closeoutPreview() { return c }
        #endif
        if let w = workLine() { return w }
        return lastText.flatMap { $0.isEmpty ? nil : Fmt.plain($0) }
    }
    /// The closeout to show (the web's `shownCloseout`): one with a status this build knows, while the session is not
    /// working (a new turn has no closeout yet, and the previous turn's no longer says where the work stands).
    var shownCloseout: Closeout? {
        guard let w = work, let c = w.closeout, c.known != nil, state != .working, w.phase != "working" else { return nil }
        return c
    }
    /// A stopped session's line when its agent ended the turn with a closeout (the web's `closeoutPreview`): the hub's
    /// line as before (the closeout's own words when the hub gave none: the last words would be the raw block), then
    /// "merged · not deployed" where the agent said or the hub found it, a claim the hub found untrue marked. One
    /// string: the row truncates it like any other line.
    func closeoutPreview(now: Date = Date()) -> String? {
        guard let c = shownCloseout else { return nil }
        let line = workLine(now: now) ?? c.line ?? ""
        let suffix = c.suffix
        if line.isEmpty { return suffix.isEmpty ? nil : suffix }
        return suffix.isEmpty ? line : String(localized: "\(line) · \(suffix)")
    }
    /// The question an Awaiting Reply row shows: what the triage says it waits on the user for (blocked · user), else what
    /// its last reply asks; nil when it is not Awaiting Reply or names nothing.
    func replyLine(muted: Bool) -> String? {
        guard awaitsYourReply(muted: muted) else { return nil }
        if let w = work, w.phase == "blocked", w.on == "user", let line = w.line, !line.isEmpty { return line }
        return asks.flatMap { $0.isEmpty ? nil : Fmt.plain($0) }
    }
    /// The row's line for a stopped session the hub triaged (docs/assistant/agent-work.md, the web's `workLine`): 「✓ 结论」 when
    /// done, what it waits for when blocked, with when the hub acts next; nil while it works or before the triage.
    func workLine(now: Date = Date()) -> String? {
        guard let w = work, let line = w.line, !line.isEmpty, w.phase != "working", state != .working else { return nil }
        if w.phase == "done" { return "✓ " + line }
        guard let wake = w.wakeAt else { return line }
        let retry = w.on == "transient"
        let left = wake.timeIntervalSince(now)
        let when: String
        if left < 60 {
            when = retry ? String(localized: "retrying now") : String(localized: "waking now")
        } else {
            let after = Session.inMinutes(left)
            when = retry ? String(localized: "retrying in \(after)") : String(localized: "waking in \(after)")
        }
        return String(localized: "\(line) (\(when))")
    }
    /// 「12 分钟」「1 小时 5 分」「2 小时」: when the hub acts next, minutes are enough.
    static func inMinutes(_ seconds: TimeInterval) -> String {
        let m = Int((seconds / 60).rounded())
        if m < 60 { return String(localized: "\(m) min [Task Views 135d12]") }
        let h = m / 60
        return m % 60 == 0 ? String(localized: "\(h) hr") : String(localized: "\(h) hr \(m % 60) min")
    }
    /// A failure notice without the warning sign botmux starts it with: the app draws its own.
    static func failureText(_ reason: String) -> String {
        var t = Substring.UnicodeScalarView(reason.trimmingCharacters(in: .whitespaces).unicodeScalars)
        while let c = t.first, c == "\u{26A0}" || c == "\u{FE0F}" { t.removeFirst() }
        return String(t).trimmingCharacters(in: .whitespaces)
    }
    /// Open in Lark: the session's place outside Persona, a web link only.
    var externalLink: URL? {
        guard let s = externalURL, let u = URL(string: s), u.scheme?.lowercased() == "https" else { return nil }
        return u
    }

    /// Idle or unclassified: folded away in busy spaces (an Awaiting Reply one stays out: `AgentListOrder.stays`).
    nonisolated var isQuiet: Bool { state == .idle || state == .unknown }
}

/// Unified agent state (docs/protocol/agent-states.md), computed by the hub from herdr status, pending interactions and
/// whether the result has been seen.
nonisolated enum AgentState: String, Codable, Sendable, CaseIterable {
    case needsYou = "needs_you", working, unread, idle, offline, unknown

    init(from decoder: Decoder) throws {
        self = AgentState(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .unknown
    }

    /// Fallback for hubs older than the state field.
    init(status: SessionStatus) {
        switch status {
        case .blocked: self = .needsYou
        case .working: self = .working
        case .done: self = .unread
        case .idle: self = .idle
        case .offline: self = .offline
        case .unknown: self = .unknown
        }
    }

    /// Lower sorts first: what needs attention comes before what is quiet.
    var priority: Int {
        switch self {
        // Finished-unseen comes before working: it is waiting for you, a working agent is not (yet).
        case .needsYou: 0
        case .unread: 1
        case .working: 2
        case .idle: 3
        case .unknown: 4
        case .offline: 5
        }
    }

    var isLive: Bool { self != .offline }
}

/// A session across groups of hubs (docs/protocol/multi-hub.md): group, machine id, session id. The default group ("") has no
/// group in its JSON or its string forms, so everything keyed by its sessions stays byte for byte what it was.
nonisolated struct SessionKey: Hashable, Sendable, Codable {
    var machineID: String
    var sessionID: String
    /// nil for the default group.
    var hubGroup: String?

    init(group: String = "", machineID: String, sessionID: String) {
        self.machineID = machineID
        self.sessionID = sessionID
        hubGroup = group.isEmpty ? nil : group
    }

    var group: String {
        get { hubGroup ?? "" }
        set { hubGroup = newValue.isEmpty ? nil : newValue }
    }
    var machineRef: MachineRef { MachineRef(group: group, id: machineID) }
    /// A terminal addressed by its pane (`pane:<id>`, docs/integrations/terminal.md section 19): a plain terminal of the
    /// Terminals page or Open Terminal Here, with no session row of its own.
    var isPane: Bool { sessionID.hasPrefix("pane:") }

    /// "machine/session", or "group|machine/session" for another group: what launch arguments, remembered chats and
    /// deep links write.
    var path: String { (hubGroup.map { $0 + "|" } ?? "") + machineID + "/" + sessionID }

    /// Reads `path` back (also the old "machine/session").
    init?(path: String) {
        var rest = Substring(path)
        var group = ""
        if let bar = rest.firstIndex(of: "|"), rest[..<bar].firstIndex(of: "/") == nil {
            group = String(rest[..<bar])
            rest = rest[rest.index(after: bar)...]
        }
        guard let slash = rest.firstIndex(of: "/") else { return nil }
        let machine = String(rest[..<slash]), session = String(rest[rest.index(after: slash)...])
        guard !machine.isEmpty, !session.isEmpty else { return nil }
        self.init(group: group, machineID: machine, sessionID: session)
    }
}

/// A lazy picture's ref: its chat's `SessionKey.path` and its item id, "<machine>/<session>/<item id>", or
/// "<group>|<machine>/<session>/<item id>" for a chat on another group of hubs. The group says which hub has the bytes:
/// asked of the current group's hub, a chat on another group's pictures never came (#1555).
nonisolated enum ImageRef {
    static func make(_ key: SessionKey, item: String) -> String { key.path + "/" + item }

    /// The group whose hub has it, and the "<machine>/<session>/<item id>" that hub's `APIClient.image(ref:)` takes.
    static func split(_ ref: String) -> (group: String, ref: String) {
        guard let bar = ref.firstIndex(of: "|"), ref[..<bar].firstIndex(of: "/") == nil else { return ("", ref) }
        return (String(ref[..<bar]), String(ref[ref.index(after: bar)...]))
    }

    /// The chat it is from.
    static func session(_ ref: String) -> SessionKey? {
        let (group, rest) = split(ref)
        let parts = rest.split(separator: "/", maxSplits: 2).map(String.init)
        guard parts.count == 3 else { return nil }
        return SessionKey(group: group, machineID: parts[0], sessionID: parts[1])
    }
}

nonisolated struct ImagePayload: Codable, Hashable, Sendable {
    var mime: String
    /// Base64; empty when `lazy`: history pages leave the bytes out and each image is fetched when it is shown.
    var data: String
    var lazy: Bool? = nil
    var size: Int? = nil
    var width: Int? = nil
    var height: Int? = nil
    /// Where a lazy image lives: "<machine>/<session>/<item id>", "<group>|…" for another group of hubs (`ImageRef`,
    /// filled in by ChatStore).
    var ref: String? = nil

    init(mime: String, data: String) {
        self.mime = mime
        self.data = data
    }

    /// Tells images apart without comparing megabytes of base64.
    var identity: String { ref ?? (String(data.prefix(64)) + String(data.count)) }

    /// Width / height, when the hub told us (lazy images): lets a placeholder take the image's shape.
    var aspect: CGFloat? {
        guard let width, let height, width > 0, height > 0 else { return nil }
        return CGFloat(width) / CGFloat(height)
    }
}

nonisolated enum ItemKind: String, Codable, Sendable {
    case text, thinking, toolUse = "tool_use", toolResult = "tool_result", notice, image, recap, fold
    /// Codex's duration row. Counted in the turn's usage, never drawn (#5426).
    case turnDuration = "turn_duration"
    case unknown

    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = ItemKind(rawValue: raw) ?? .unknown
    }
}

/// One model's request on an item (`usage` / `fold.usage`, docs/protocol/protocol.md "Token usage on items").
/// An entry with an `id` is a subagent or workflow report and is counted once. Counts are tokens; prices are elsewhere.
nonisolated struct TokenUsage: Codable, Hashable, Sendable {
    var id: String?
    var model: String?
    var input: Int64
    var output: Int64
    var cacheRead: Int64
    var cacheWrite: Int64
    var cacheWrite1h: Int64
    var fast: Bool

    /// A report (a subagent or a workflow), counted once per model and speed.
    var report: Bool { !(id ?? "").isEmpty }

    enum CodingKeys: String, CodingKey {
        case id, model, fast
        case input = "in", output = "out"
        case cacheRead = "cache_read", cacheWrite = "cache_write", cacheWrite1h = "cache_write_1h"
    }

    init(id: String? = nil, model: String? = nil, input: Int64 = 0, output: Int64 = 0, cacheRead: Int64 = 0,
         cacheWrite: Int64 = 0, cacheWrite1h: Int64 = 0, fast: Bool = false) {
        self.id = id
        self.model = model
        self.input = input
        self.output = output
        self.cacheRead = cacheRead
        self.cacheWrite = cacheWrite
        self.cacheWrite1h = cacheWrite1h
        self.fast = fast
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id)
        model = try c.decodeIfPresent(String.self, forKey: .model)
        input = try c.decodeIfPresent(Int64.self, forKey: .input) ?? 0
        output = try c.decodeIfPresent(Int64.self, forKey: .output) ?? 0
        cacheRead = try c.decodeIfPresent(Int64.self, forKey: .cacheRead) ?? 0
        cacheWrite = try c.decodeIfPresent(Int64.self, forKey: .cacheWrite) ?? 0
        cacheWrite1h = try c.decodeIfPresent(Int64.self, forKey: .cacheWrite1h) ?? 0
        fast = try c.decodeIfPresent(Bool.self, forKey: .fast) ?? false
    }
}

/// A built-in catalog price, USD per million tokens (`POST /api/v1/launchers/catalog`). A missing kind has no price.
nonisolated struct ModelPrice: Codable, Hashable, Sendable {
    var input: Double?
    var output: Double?
    var cacheRead: Double?
    var cacheWrite: Double?
    var cacheWrite1h: Double?

    enum CodingKeys: String, CodingKey {
        case input, output
        case cacheRead = "cache_read", cacheWrite = "cache_write", cacheWrite1h = "cache_write_1h"
    }

    init(input: Double? = nil, output: Double? = nil, cacheRead: Double? = nil, cacheWrite: Double? = nil,
         cacheWrite1h: Double? = nil) {
        self.input = input
        self.output = output
        self.cacheRead = cacheRead
        self.cacheWrite = cacheWrite
        self.cacheWrite1h = cacheWrite1h
    }
}

/// A run of the agent's work a page left out (`Item.fold`; docs/protocol/protocol.md「Folded work」): its counts for the folded
/// work line, and its range for fetching it when the line is opened.
nonisolated struct Fold: Codable, Hashable, Sendable {
    var items: Int
    var tools: Int
    var thoughts: Int
    var until: Date?
    var throughSeq: Int
    var throughID: String
    /// The images the run's calls looked at (docs/protocol/protocol.md「Images viewed this turn」): its turn shows them before the answer
    /// while the run stays folded.
    var images: [FoldImage]? = nil
    /// The task lists the run's calls left (docs/protocol/protocol.md「Folded work」 `fold.tasks`), in order: the chat's plan
    /// while its TodoWrite is folded away (`ChatPlan.latest`, #5602).
    var tasks: [FoldTasks]? = nil
    /// The hub proved this supervision round is user-visible, even while its visibility tools are folded away.
    var supervisionUser: Bool? = nil
    /// What the folded items used (#5426). Omitted when none of them carried usage, and from older nodes.
    var usage: [TokenUsage]? = nil

    enum CodingKeys: String, CodingKey {
        case items, tools, thoughts, until, images, tasks, usage
        case supervisionUser = "supervision_user"
        case throughSeq = "through_seq", throughID = "through_id"
    }
}

/// A task list a folded run's call left (`Fold.tasks`): a TodoWrite's whole list (`replace` "todo") or a task tool's.
nonisolated struct FoldTasks: Codable, Hashable, Sendable {
    nonisolated struct Entry: Codable, Hashable, Sendable {
        var id: String
        var subject: String?
        var description: String?
        /// pending, in_progress, completed, unknown or deleted.
        var status: String?
    }

    var replace: String?
    var entries: [Entry]
    /// The call's item.
    var sourceItemID: String

    enum CodingKeys: String, CodingKey {
        case replace, entries
        case sourceItemID = "source_item_id"
    }
}

/// An image a folded run's call looked at (`Fold.images`): one its result carried (`itemID`), or a picture file Read
/// opened whose result carried none (only `path`).
nonisolated struct FoldImage: Codable, Hashable, Sendable {
    var itemID: String?
    var toolUseID: String
    var tool: String
    /// read, screenshot, browser, mcp or tool.
    var source: String
    var path: String?
    /// The machine a screenshot's or a browser's call named (an id or a name).
    var machine: String?
    var mime: String?
    var size: Int?
    var width: Int?
    var height: Int?
    /// Where its bytes are, like `ImagePayload.ref` (`ImageRef`, filled in by ChatStore).
    var ref: String? = nil

    enum CodingKeys: String, CodingKey {
        case tool, source, path, machine, mime, size, width, height, ref
        case itemID = "item_id", toolUseID = "tool_use_id"
    }
}

/// Where Claude Code compacted the context (`Item.compact`): from the transcript's compact_boundary line (`trigger`:
/// manual for /compact, auto otherwise; idle when the app saw the note of an idle compaction after it), and the notes
/// that came with it, shown when the divider is tapped.
nonisolated struct Compaction: Codable, Hashable, Sendable {
    var trigger: String?
    var preTokens: Int?
    /// The notices folded into the divider (the app's, never sent): 「上下文已压缩」「已从压缩的上下文继续」, the idle note.
    var notes: [String]?

    enum CodingKeys: String, CodingKey {
        case trigger, notes
        case preTokens = "pre_tokens"
    }

    /// Manual / Auto / While idle, for the divider; nil when the hub did not say.
    var source: String? {
        switch trigger {
        case "manual": String(localized: "Manual [Models 962f41]", comment: "How a compaction started")
        case "auto": String(localized: "Auto", comment: "How a compaction started")
        case "idle": String(localized: "While idle", comment: "How a compaction started")
        default: nil
        }
    }
}

/// A compaction the user asked for (`Session.compaction`, the hub's proto.Compaction): its state, why it failed, and
/// when it started and finished. Not a turn: the session's status stays as it was.
nonisolated struct CompactionRun: Codable, Hashable, Sendable {
    enum State: String, Codable, Sendable { case running, done, failed }
    var state: State
    var error: String?
    var startedAt: Date?
    var finishedAt: Date?

    enum CodingKeys: String, CodingKey {
        case state, error
        case startedAt = "started_at", finishedAt = "finished_at"
    }

    init(state: State, error: String? = nil, startedAt: Date? = nil, finishedAt: Date? = nil) {
        self.state = state
        self.error = error
        self.startedAt = startedAt
        self.finishedAt = finishedAt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        state = try c.decode(State.self, forKey: .state)
        error = try c.decodeIfPresent(String.self, forKey: .error)
        startedAt = try? c.decodeIfPresent(Date.self, forKey: .startedAt)
        finishedAt = try? c.decodeIfPresent(Date.self, forKey: .finishedAt)
    }
}

/// A background task reporting back (`Item.task` on a notice): a command run in the background, a Monitor's event or
/// end, a subagent. The hub reads it from the notification (decoded); `TaskIndex` finds the call it came from.
nonisolated struct TaskNote: Codable, Hashable, Sendable {
    var kind: String // shell | monitor | agent | task
    var status: String?
    var title: String
    var taskID: String?
    var toolUseID: String?
    var exitCode: Int?
    var outputFile: String?
    /// A Monitor's event, a subagent's result, or the whole command when the title is its first line.
    var detail: String?

    enum CodingKeys: String, CodingKey {
        case kind, status, title, detail
        case taskID = "task_id", toolUseID = "tool_use_id", exitCode = "exit_code", outputFile = "output_file"
    }

    var failed: Bool { status == "failed" || (exitCode ?? 0) != 0 }
}

/// What a tool call came to (`outcome` on a tool_result, docs/protocol/protocol.md "Tool outcomes"), read by the node from
/// the runtime's own record so the app need not guess from the text. Only what the app draws is kept; every field is
/// optional, and a value of an unexpected type is left unread rather than dropping the item it came with.
nonisolated struct ToolOutcome: Codable, Hashable, Sendable {
    /// ok, failed, no_match, denied, blocked, interrupted, timed_out, background; or one a later node adds.
    var status: String?
    var exitCode: Int?
    /// The output's last lines, sent only when the page cut the result's text.
    var tail: String?

    enum CodingKeys: String, CodingKey { case status, tail; case exitCode = "exit_code" }

    init(status: String? = nil, exitCode: Int? = nil, tail: String? = nil) {
        self.status = status
        self.exitCode = exitCode
        self.tail = tail
    }

    init(from decoder: Decoder) {
        let c = try? decoder.container(keyedBy: CodingKeys.self)
        status = (try? c?.decodeIfPresent(String.self, forKey: .status)) ?? nil
        exitCode = (try? c?.decodeIfPresent(Int.self, forKey: .exitCode)) ?? nil
        tail = (try? c?.decodeIfPresent(String.self, forKey: .tail)) ?? nil
    }

    /// The call did not run, or was stopped partway: denied (by the user, a rule or a hook), blocked (refused before it
    /// ran), interrupted. Nothing it ran failed, so it is not drawn in red.
    var stopped: Bool { ["denied", "blocked", "interrupted"].contains(status) }
}

nonisolated struct Item: Codable, Identifiable, Hashable, Sendable {
    var id: String
    var seq: Int
    var ts: Date?
    var role: String
    var kind: ItemKind
    var text: String?
    var toolUseID: String?
    var toolName: String?
    var toolInput: JSONValue?
    var isError: Bool?
    /// The hub cut `text` for the page (it ends in a 「…（已截断，点开看全文，共 N 字节）」 line): the whole is this many bytes,
    /// `APIClient.fullItem` fetches it.
    var fullSize: Int?
    var image: ImagePayload?
    /// User input typed while the agent worked and absorbed mid-turn.
    var queued: Bool?
    /// Sent by another Claude session (cross-session messaging), not typed by the user: its name.
    var from: String?
    /// A background task's notice that woke the idle agent: new work after a finished turn.
    var wake: Bool?
    /// kind == .fold: the work this item stands for.
    var fold: Fold?
    /// kind == .notice or .toolResult: a background task reporting back.
    var task: TaskNote?
    /// The id the app sent the message this item came from with (`PendingMessage.id`): on every item of it — its text,
    /// its images, a queued copy, or the notice it became (`/compact`). Nil from older hubs, or when the node could not
    /// tie it to one.
    var clientMsgID: String?
    /// kind == .notice: the context was compacted here (the hub's `compact`, or recognized by the app,
    /// `ChatStore.compactions`). Drawn as a divider across the chat (`CompactDivider`).
    var compact: Compaction?
    /// User text Persona typed in for the model (a task's notice, a scheduled run…; docs/protocol/protocol.md「Messages injected by Persona」):
    /// drawn as one line, not a bubble. Set by the hub; nil from older hubs, which `Injected.of` reads off the text.
    var injected: Injected? = nil
    /// Client-retained evidence of user-visible supervision. Survives overlap replacing a promoted work fold and is
    /// encoded in the chat cache, not a request to change the hub's visibility.
    var supervisionUser: Bool? = nil
    /// Additive display provenance; kept as JSON so future kinds and malformed optional fields remain readable.
    var source: JSONValue? = nil
    /// kind == .toolResult: what the call came to. Nil from older nodes.
    var outcome: ToolOutcome? = nil
    /// kind == .notice: an upstream refusal, drawn as a line of its own after the exchange (#4489, `RefusalNotice`).
    var modelRefusal: ModelRefusal? = nil
    /// Text belonging to the work summary, including when the node sends it unfolded.
    var narration: Bool? = nil
    /// A generated heartbeat acknowledgement from a DONT_NOTIFY envelope.
    var codexHeartbeatQuiet: Bool? = nil
    /// What the model requests this item stands for used (#5426). Older hubs omit it.
    var usage: [TokenUsage]? = nil
    /// Raw record identity for a text item with a positional ID, stable before clipping. Older nodes omit it.
    var sourceFingerprint: String? = nil

    enum CodingKeys: String, CodingKey {
        case id, seq, ts, role, kind, text, image, queued, from, wake, fold, task, compact, injected, source, outcome, usage
        case modelRefusal = "model_refusal", narration, codexHeartbeatQuiet = "codex_heartbeat_quiet"
        case sourceFingerprint = "source_fingerprint"
        case toolUseID = "tool_use_id", toolName = "tool_name", toolInput = "tool_input", isError = "is_error", fullSize = "full_size",
             clientMsgID = "client_msg_id", supervisionUser = "supervision_user"
    }

    /// The text without the line the hub adds where it cut it (the app offers the whole text itself).
    var cutText: String {
        guard fullSize != nil, let text, let r = text.range(of: "\n…（已截断", options: .backwards) else { return text ?? "" } // i18n-ignore: matches the hub's text
        return String(text[..<r.lowerBound])
    }
}

nonisolated enum InteractionKind: String, Codable, Sendable {
    /// `elicitation`: an MCP server's form or link (docs/protocol/protocol.md "MCP elicitation"). The hub sends it only to an
    /// app that declared it in `client_hello` (the iOS app, `LiveConnection.helloFeatures`).
    case question, plan, permission, screen, elicitation
}

nonisolated struct QuestionOption: Codable, Hashable, Sendable {
    var label: String
    var description: String?
    /// "defer" on a manager work card's put-off choices (#6753): picking one only clears the card on the hub and sends
    /// nothing to the Agent. Missing on every other option.
    var action: String?

    /// "Take the longest one (Recommended)" → ("Take the longest one", true): the words to show and whether Claude recommends it.
    /// The answer sent back keeps the original label. Phone and watch draw 「推荐」 instead of the suffix.
    static func display(_ label: String) -> (String, Bool) {
        for suffix in [" (Recommended)", "(Recommended)", "（推荐）", "(推荐)"] where label.hasSuffix(suffix) { // i18n-ignore: matches option labels from the agent
            return (String(label.dropLast(suffix.count)).trimmingCharacters(in: .whitespaces), true)
        }
        return (label, false)
    }
}

nonisolated struct Question: Codable, Hashable, Sendable, Identifiable {
    var question: String
    var header: String?
    var multiSelect: Bool
    var options: [QuestionOption]
    var id: String { question }

    enum CodingKeys: String, CodingKey { case question, header, options; case multiSelect = "multi_select" }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        question = try c.decode(String.self, forKey: .question)
        header = try c.decodeIfPresent(String.self, forKey: .header)
        multiSelect = try c.decodeIfPresent(Bool.self, forKey: .multiSelect) ?? false
        options = try c.decodeIfPresent([QuestionOption].self, forKey: .options) ?? []
    }

    init(question: String, header: String?, multiSelect: Bool, options: [QuestionOption]) {
        self.question = question
        self.header = header
        self.multiSelect = multiSelect
        self.options = options
    }
}

nonisolated struct Interaction: Codable, Identifiable, Hashable, Sendable {
    var id: String
    var machineID: String
    var sessionID: String
    var sessionTitle: String?
    /// The task (docs/assistant/unified-entry.md) the asking agent works for: filled in by the phone for the watch, not sent by
    /// the hub.
    var taskTitle: String?
    var cwd: String?
    var kind: InteractionKind
    var createdAt: Date?
    var expiresAt: Date?
    var toolName: String?
    var toolUseID: String?
    var questions: [Question]?
    /// Whether this question's runtime can release its waiter without an answer (`cancel` on Resolution).
    var canCancel: Bool?
    var plan: String?
    /// The permission mode a plan-mode start goes on in after approval (#3784). Older hubs and sessions omit it.
    var planThen: String?
    var toolInput: JSONValue?
    var summary: String?
    /// A script ask's opt-in reply-Markdown rendering; missing and unrecognized values stay literal (#10857).
    var format: String?
    var screen: String?
    /// The screen's dialog as the hub read it (newer hubs): answer with `Resolution.option`.
    var dialog: ScreenDialog?
    /// The terminal asks for a password: typed into a password field, not kept anywhere.
    var secret: Bool?
    var fields: [InteractionField]?
    /// Something to fill in before it can be allowed: every quick Allow (lists, menus, Siri, the watch) is off. The
    /// password card counts even without its field, so no path allows it without a password, and so does an MCP form or
    /// link, answered only on its card, and a hand-off card, approved on its card alone.
    var requiresInput: Bool { !(fields ?? []).isEmpty || isSecretRun || isSecretHandoff || kind == .elicitation }
    /// Who asks when it is not Claude Code: "codex" / "traex" (a card read off a Codex TUI, docs/integrations/codex.md「Questions and approvals」).
    var agent: String?
    /// Why the agent asks for this permission, in its words.
    var reason: String?
    /// What "always allow" allows for this request ("don't ask about this command again this session"); a Codex card without it offers no such button.
    var alwaysLabel: String?
    /// The persisted scope granted by this card; absent on older hubs and terminal-only sessions.
    var alwaysKeys: [String]?
    /// Whether answering Always is kept, as the manager says (#3590): false on its own cards, which keep nothing, so
    /// they offer only Allow and Deny; absent (an older hub, an Agents session's card) the card's own rule (`offersAlways`).
    var alwaysHonored: Bool?
    /// The manager's own tool by its name for people ("iPhone control" for iphone_do); absent on other cards.
    var toolTitle: String?
    /// The tool the card asks about: the manager's name for its own tools, else as it is called.
    var toolDisplayName: String? {
        if let toolTitle, !toolTitle.isEmpty { return toolTitle }
        return toolName
    }
    /// What the hub calls an agent this app does not know (a runtime's own name, "DSH"; docs/integrations/runtimes.md).
    var agentLabelHint: String?
    /// The group of paired hubs it came from (docs/protocol/multi-hub.md); nil for the default group. The app's own.
    var hubGroup: String?
    /// The hub muted this agent or its space: stamped on the cards of GET /api/v1/interactions (#4360) so the
    /// answer-only watch hides them and never buzzes; the phone filters with its own mutes. Absent (an older hub,
    /// and every loud card: the hub sends it `omitempty`), the card shows as before.
    var muted: Bool {
        get { mutedFlag ?? false }
        set { mutedFlag = newValue }
    }
    /// What `muted` is stored as: optional, so a card without the key decodes (a synthesized `Bool` with a default
    /// still requires it, #4763) and encodes without it again.
    var mutedFlag: Bool?
    /// The assistant is deciding whether it may answer this question for the user (docs/assistant/assistant.md "Assistant
    /// answers questions first"): nothing was pushed yet, and an answer given here wins. The same card comes again
    /// without it, by `id`.
    var delegating: Bool?
    /// What the assistant filled in when it left the question to the user.
    var delegate: Delegate?
    /// A standing rule the hub suggests on a question a supervisor passed up (#4929, #5802); the card comes again
    /// without it once it was answered. Absent on every other card and from a hub before it.
    var suggestion: RuleSuggestion?
    /// The manager that reported it (docs/protocol/protocol.md "Per-manager routing"). Once it is no longer pending
    /// the hub routes by the manager a request names, so Change Answer names this one.
    var manager: String?
    /// An `elicitation` card's form or link, and who asks: the MCP server, where it is configured, and the subagent that
    /// called it, each only when the source gave it.
    var elicitation: ElicitationRequest?
    var mcpServerName: String?
    var mcpConfigSource: String?
    var agentID: String?
    var agentType: String?
    /// The asking session's `folded_under` (`Interaction.folded_under`). The card is shown on that parent. Absent from
    /// a hub without `sessions_hierarchy`, and on a card whose session is not folded.
    var foldedUnder: String? {
        get { foldedUnderText?.value }
        set { foldedUnderText = newValue.map(LenientText.init) }
    }
    /// How the asking session folds (`folded_as`): "worker", "side" (#7944).
    var foldedAs: String? {
        get { foldedAsText?.value }
        set { foldedAsText = newValue.map(LenientText.init) }
    }
    /// The side session whose chat shows the card when its session folds through one (`folded_side`, #7944, `SideFold`).
    var foldedSide: String? {
        get { foldedSideText?.value }
        set { foldedSideText = newValue.map(LenientText.init) }
    }
    /// What the three are stored as: a value of another type, or an empty one, reads as none and never drops the card.
    var foldedUnderText: LenientText? = nil
    var foldedAsText: LenientText? = nil
    var foldedSideText: LenientText? = nil

    /// A server-owned exact proposal. These fields never turn another approval into an Always grant.
    var approvalBatchID: String?
    var approvalItemID: String?
    var ownerProposal: Bool?
    var noExpiry: Bool?
    var ownerApprovalPhrases: [String]?
    var ownerApprovalPhrase: String? {
        guard kind == .permission, ownerProposal == true, let approvalBatchID, !approvalBatchID.isEmpty,
              !machineID.isEmpty, !sessionID.isEmpty,
              let phrase = ownerApprovalPhrases?.first, !phrase.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return nil }
        return phrase
    }

    var sessionKey: SessionKey { SessionKey(group: group, machineID: machineID, sessionID: sessionID) }
    /// The session the card is shown on. Answering still uses `id` and `sessionID`.
    var requestHome: String {
        if let under = foldedUnder, !under.isEmpty, under != sessionID { return under }
        return sessionID
    }
    var machineRef: MachineRef { MachineRef(group: group, id: machineID) }

    var group: String {
        get { hubGroup ?? "" }
        set { hubGroup = newValue.isEmpty ? nil : newValue }
    }
    /// Unique across groups (`id` is the hub's, what resolve takes); for the default group just `id`.
    var uid: String { (hubGroup.map { $0 + "|" } ?? "") + id }

    /// The dialog to show: the hub's reading, else the app's own (older hubs).
    var screenDialog: ScreenParser.Dialog? { dialog?.parsed ?? screen.flatMap(ScreenParser.dialog) }

    enum CodingKeys: String, CodingKey {
        case id, kind, questions, plan, summary, format, screen, cwd, dialog, secret, fields, agent, reason, delegating, delegate, manager
        case suggestion, canCancel = "can_cancel"
        case planThen = "plan_then"
        case machineID = "machine_id", sessionID = "session_id", sessionTitle = "session_title", taskTitle = "task_title",
             createdAt = "created_at", expiresAt = "expires_at", toolName = "tool_name", toolUseID = "tool_use_id",
             toolInput = "tool_input", alwaysLabel = "always_label", alwaysKeys = "always_keys", alwaysHonored = "always_honored",
             toolTitle = "tool_title", agentLabelHint = "agent_label", hubGroup = "app_hub_group",
             mutedFlag = "muted", elicitation, mcpServerName = "mcp_server_name", mcpConfigSource = "mcp_config_source",
             agentID = "agent_id", agentType = "agent_type", foldedUnderText = "folded_under", foldedAsText = "folded_as",
             foldedSideText = "folded_side", approvalBatchID = "approval_batch_id", approvalItemID = "approval_item_id",
             ownerProposal = "owner_proposal", noExpiry = "no_expiry", ownerApprovalPhrases = "owner_approval_phrases"
    }
}

/// The context the assistant filled in when it left a question to the user (`Interaction.delegate`, the hub's
/// proto.Delegate). Every field may be missing; an odd one never drops the card.
nonisolated struct Delegate: Codable, Hashable, Sendable {
    var what: String?
    var whyNow: String?
    /// For a card with options, an option's exact label (several questions: their answers joined by `；`).
    var recommend: String?
    var cost: String?
    var reason: String?
    var deadline: Date?

    enum CodingKeys: String, CodingKey {
        case what, recommend, cost, reason, deadline
        case whyNow = "why_now"
    }

    init(what: String? = nil, whyNow: String? = nil, recommend: String? = nil, cost: String? = nil, reason: String? = nil,
         deadline: Date? = nil) {
        self.what = what
        self.whyNow = whyNow
        self.recommend = recommend
        self.cost = cost
        self.reason = reason
        self.deadline = deadline
    }

    init(from decoder: Decoder) throws {
        let c = try? decoder.container(keyedBy: CodingKeys.self)
        what = (try? c?.decodeIfPresent(String.self, forKey: .what)).flatMap { $0 }
        whyNow = (try? c?.decodeIfPresent(String.self, forKey: .whyNow)).flatMap { $0 }
        recommend = (try? c?.decodeIfPresent(String.self, forKey: .recommend)).flatMap { $0 }
        cost = (try? c?.decodeIfPresent(String.self, forKey: .cost)).flatMap { $0 }
        reason = (try? c?.decodeIfPresent(String.self, forKey: .reason)).flatMap { $0 }
        deadline = (try? c?.decodeIfPresent(Date.self, forKey: .deadline)).flatMap { $0 }
    }
}

nonisolated struct InteractionField: Codable, Hashable, Sendable, Identifiable {
    var key: String
    var label: String
    var placeholder: String?
    var secret: Bool?
    var optional: Bool?
    var id: String { key }
}

nonisolated struct Resolution: Codable, Sendable, Equatable {
    var answers: [String: String]?
    var approve: Bool?
    var mode: String?
    var feedback: String?
    var allow: Bool?
    var always: Bool?
    var message: String?
    var keys: [String]?
    var text: String?
    var dismiss: Bool?
    /// Close a question whose interaction advertised `can_cancel`, without sending an answer or another action.
    var cancel: Bool?
    /// Keys before `text` (`keys` go after it): pick "Type something." by its number, then type.
    var preKeys: [String]?
    /// A screen dialog's option (Interaction.dialog): the node works out its keys, after checking the dialog is unchanged.
    var option: String?
    /// A multi-select screen dialog: the options to have checked when it is sent (`text`: its free-text field).
    var options: [String]?
    var values: [String: String]?
    /// "assistant" on a question the assistant answered for the user (the hub's history and its `already` reply); the
    /// app never sends it.
    var answeredBy: String?
    /// Why the assistant answered so (with `answeredBy`).
    var reason: String?
    /// An MCP form or link (`Elicitation.resolution`): accept, decline or cancel, and with a form's accept what was filled in.
    var action: String?
    var content: [String: JSONValue]?
    /// Runtime-only exact proposal reply. Excluded from CodingKeys so it can never become an ordinary resolution field.
    var ownerApprovalPhrase: String? = nil

    enum CodingKeys: String, CodingKey {
        case answers, approve, mode, feedback, allow, always, message, keys, text, dismiss, cancel, option, options, values, reason, action, content
        case preKeys = "pre_keys", answeredBy = "answered_by"
    }

    static let dismissed = Resolution(dismiss: true)
    static let cancelled = Resolution(cancel: true)
}

/// One device's push settings, kept by the hub per device (Settings › Notifications). The timings come back from the hub's
/// hub.json "notify" so the page can say what "long" means; they are not sent back.
/// Settings › Translation as the hub keeps it with a device's prefs.
nonisolated struct TranslatePrefsBody: Codable, Equatable, Sendable {
    var enabled: Bool
    var target: String
    var mail: Bool
}

/// A desktop the hub lists in its `desktops` event (docs/protocol/protocol.md): the Mac app connected to the hub, like WeChat's
/// "WeChat for Mac is logged in", or a computer where Persona Link reports you at it (`kind: "link"`, only sent to clients that declare
/// `desktops_link`). With Phone Muted on, the Mac app mutes "Needs you" and "Agent done"; Persona Link mutes only "Needs you".
nonisolated struct DesktopDevice: Codable, Hashable, Sendable, Identifiable {
    /// Its device token's id (what Sign Out revokes); empty when it connected with the pairing token, and for Persona Link.
    var device: String
    var name: String
    var since: Date?
    /// Absent for the Mac app.
    var kind: Kind?
    /// The hub's machine id, for a Persona Link entry.
    var machineID: String?

    enum CodingKeys: String, CodingKey { case device, name, since, kind, machineID = "machine_id" }

    /// `link` is Persona Link; a kind this build doesn't know reads as `unknown` and is left out of what the app shows (`shown`).
    nonisolated enum Kind: String, Codable, Sendable {
        case link, unknown

        init(from decoder: Decoder) throws {
            self = Kind(rawValue: try decoder.singleValueContainer().decode(String.self)) ?? .unknown
        }
    }

    /// The Mac app: only an entry without `kind`.
    var isApp: Bool { kind == nil }
    var isLink: Bool { kind == .link }
    /// Only a Mac app with its own device token can be signed out from the phone.
    var canSignOut: Bool { isApp && !device.isEmpty }

    /// The entries the banner and the sheet show: Mac apps and Persona Link. A kind this build doesn't know is dropped, since
    /// neither what it mutes nor how it identifies its computer is known.
    static func shown(_ list: [DesktopDevice]) -> [DesktopDevice] { list.filter { $0.isApp || $0.isLink } }

    /// A Persona Link entry and a Mac app on the same computer stay two entries.
    var id: String {
        if isLink { return "link:" + (machineID ?? name) }
        return device.isEmpty ? "name:" + name : device
    }

    /// The banner's line. Mac apps alone keep "Persona is signed in on your Mac" (a count when several); Persona Link names the computer
    /// you're at, or counts the computers once it adds one. The hub lists each machine's Persona Link once. A Mac app has no machine
    /// id, so Persona Link counts once with a Mac app of the same name: an approximation for this line only (two Macs sharing a
    /// name count once, a Mac whose app and Persona Link go by different names twice); ids and the sheet keep every entry.
    static func bannerTitle(_ list: [DesktopDevice]) -> String {
        let apps = list.filter(\.isApp)
        let appNames = Set(apps.map(\.name))
        let links = list.filter { $0.isLink && !appNames.contains($0.name) }
        guard let only = links.first else {
            return apps.count > 1 ? String(localized: "Persona is signed in on \(apps.count) Macs") : String(localized: "Persona is signed in on your Mac")
        }
        let computers = apps.count + links.count
        return computers > 1 ? String(localized: "Using \(computers) computers") : String(localized: "Using “\(only.name)”")
    }

    /// The footer under Phone Muted. With a Mac app online the hub mutes as for the Mac app ("Needs you" and "Agent done"); with
    /// only Persona Link it mutes "Needs you" until no listed computer is in use, which with several takes every one of them.
    static func muteNote(_ list: [DesktopDevice]) -> String {
        if list.isEmpty || list.contains(where: \.isApp) {
            return String(localized: "While Persona is online on a Mac, this iPhone stops sending “Needs you” and “Agent done” notifications and the Mac alerts you instead. Reminders you asked Persona to set still come through.")
        }
        return list.count == 1
            ? String(localized: "While you're using a computer with Persona Link, this iPhone stops sending “Needs you” notifications and Persona Link shows them there. “Agent done” and reminders you asked Persona to set still come to this iPhone. Lock that computer or quit Persona Link, and this iPhone sends “Needs you” again.")
            : String(localized: "While you're using a computer with Persona Link, this iPhone stops sending “Needs you” notifications and Persona Link shows them there. “Agent done” and reminders you asked Persona to set still come to this iPhone. Lock each of these computers or quit Persona Link on it, and this iPhone sends “Needs you” again.")
    }
}

/// The body that sets Phone Muted and nothing else (APIClient.setMuteWhileDesktop).
nonisolated struct MuteWhileDesktopPut: Encodable, Sendable {
    var on: Bool
    enum CodingKeys: String, CodingKey { case on = "mute_while_desktop" }
}

/// The Live Activity's persona alone, for the prefs PUT (it merges): FleetState.look.
nonisolated struct FleetLookPut: Encodable, Sendable {
    var look: FleetLook
}

/// Whether this device has Persona's Home Screen widgets, alone (the PUT merges): the hub wakes it for them (#5721).
nonisolated struct WidgetsPut: Encodable, Sendable {
    var widgets: Bool
}

/// The managers whose pushes this device doesn't want, alone (APIClient.setMutedManagers): a write of the whole prefs
/// never carries them, so a page that saved other choices can't bring back one muted elsewhere.
nonisolated struct MutedManagersPut: Encodable, Sendable {
    var ids: [String]
    enum CodingKeys: String, CodingKey { case ids = "muted_managers" }
}

/// Quiet hours alone (APIClient.setQuietHours).
nonisolated struct QuietHoursPut: Encodable, Sendable {
    var quiet: QuietHours
}

/// This device's quiet hours (the prefs' `quiet`, docs/protocol/protocol.md): no "Agent finished" inside the window, and
/// "Needs you" without a sound. Times are "HH:MM" in `tz` (an IANA zone; empty: the user's zone the hub knows); the
/// window may cross midnight, and start = end is no window.
nonisolated struct QuietHours: Codable, Equatable, Sendable {
    var enabled = false
    var start = "23:00"
    var end = "07:00"
    var tz: String?

    enum CodingKeys: String, CodingKey { case enabled, start, end, tz }

    init(enabled: Bool = false, start: String = "23:00", end: String = "07:00", tz: String? = nil) {
        self.enabled = enabled
        self.start = start
        self.end = end
        self.tz = tz
    }

    /// Empty times (never set) take the web's defaults, 23:00 to 07:00.
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        start = (try c.decodeIfPresent(String.self, forKey: .start)).flatMap { $0.isEmpty ? nil : $0 } ?? "23:00"
        end = (try c.decodeIfPresent(String.self, forKey: .end)).flatMap { $0.isEmpty ? nil : $0 } ?? "07:00"
        tz = try c.decodeIfPresent(String.self, forKey: .tz)
    }

    /// "HH:MM" as minutes after midnight (nil: not a time).
    static func minutes(_ hhmm: String) -> Int? {
        let parts = hhmm.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]), (0..<24).contains(h), (0..<60).contains(m) else { return nil }
        return h * 60 + m
    }

    /// Minutes after midnight as "HH:MM".
    static func clock(_ minutes: Int) -> String {
        let m = ((minutes % 1440) + 1440) % 1440
        return String(format: "%02d:%02d", m / 60, m % 60)
    }

    /// The name of the zone the times are in when it isn't `here` (they were set somewhere else); nil when it is, or
    /// when they have none.
    func elsewhere(here: TimeZone = .current, locale: Locale = .current) -> String? {
        guard let tz, !tz.isEmpty, let zone = TimeZone(identifier: tz), zone.identifier != here.identifier else { return nil }
        return zone.localizedName(for: .generic, locale: locale) ?? zone.identifier
    }

    /// Whether `date` is inside the window, read in `tz` or else in `fallback` (the hub's `activeAt`, quiethours.go):
    /// what a Mac's own banners follow while the hub sends it no pushes.
    func active(at date: Date, fallback: TimeZone = .current) -> Bool {
        guard enabled, let from = Self.minutes(start), let until = Self.minutes(end), from != until else { return false }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = tz.flatMap { $0.isEmpty ? nil : TimeZone(identifier: $0) } ?? fallback
        let c = calendar.dateComponents([.hour, .minute], from: date)
        let now = (c.hour ?? 0) * 60 + (c.minute ?? 0)
        return from < until ? (from..<until).contains(now) : now >= from || now < until
    }
}

nonisolated struct NotifyPrefs: Codable, Equatable, Sendable {
    enum Done: String, Codable, CaseIterable, Sendable { case off, long, always }
    var input: Bool = true
    /// Phone Muted (WeChat's): no pushes to this phone while a desktop app is connected (hub/desktops.go).
    var muteWhileDesktop = false
    var done: Done = .long
    /// The Fleet Live Activity on the lock screen / Dynamic Island (docs/apple/live-activity.md).
    var liveActivity: Bool = true
    /// Agent names on the lock screen activity; off, only counts.
    var lockDetails: Bool = false
    /// The compact Dynamic Island's two sides (FleetState.swift).
    var island = IslandPrefs()
    /// Settings › Translation, so the hub translates new mail before it is opened (Translator.swift). Kept as the hub sent it.
    var translate: TranslatePrefsBody?
    var longTurnSeconds: Int?
    var doneDelaySeconds: Int?
    /// The managers whose pushes this device doesn't get (docs/assistant/managers.md "Clients"); nil from a hub before
    /// per-manager switches, which then shows none. Written alone (`MutedManagersPut`), never with the rest.
    var mutedManagers: [String]?
    /// Quiet hours; nil from a hub without them, which then shows none. Written alone (`QuietHoursPut`).
    var quiet: QuietHours?

    enum CodingKeys: String, CodingKey {
        case input, done, quiet
        case mutedManagers = "muted_managers"
        case liveActivity = "live_activity", lockDetails = "lock_details", island, translate
        case muteWhileDesktop = "mute_while_desktop"
        case longTurnSeconds = "long_turn_seconds", doneDelaySeconds = "done_delay_seconds"
    }

    init(input: Bool = true, done: Done = .long, liveActivity: Bool = true, lockDetails: Bool = false, island: IslandPrefs = IslandPrefs()) {
        self.input = input
        self.done = done
        self.liveActivity = liveActivity
        self.lockDetails = lockDetails
        self.island = island
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        input = try c.decodeIfPresent(Bool.self, forKey: .input) ?? true
        done = (try? c.decodeIfPresent(Done.self, forKey: .done)) ?? .long
        liveActivity = try c.decodeIfPresent(Bool.self, forKey: .liveActivity) ?? true
        lockDetails = try c.decodeIfPresent(Bool.self, forKey: .lockDetails) ?? false
        island = (try? c.decodeIfPresent(IslandPrefs.self, forKey: .island)) ?? IslandPrefs()
        longTurnSeconds = try c.decodeIfPresent(Int.self, forKey: .longTurnSeconds)
        doneDelaySeconds = try c.decodeIfPresent(Int.self, forKey: .doneDelaySeconds)
        muteWhileDesktop = try c.decodeIfPresent(Bool.self, forKey: .muteWhileDesktop) ?? false
        mutedManagers = try? c.decodeIfPresent([String].self, forKey: .mutedManagers)
        quiet = try? c.decodeIfPresent(QuietHours.self, forKey: .quiet)
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(input, forKey: .input)
        try c.encode(done, forKey: .done)
        try c.encode(liveActivity, forKey: .liveActivity)
        try c.encode(lockDetails, forKey: .lockDetails)
        try c.encode(island, forKey: .island)
        // muteWhileDesktop, mutedManagers and quiet are not sent back: each is written alone (MuteWhileDesktopPut,
        // MutedManagersPut, QuietHoursPut).
    }

    /// Whether this device turned off the pushes of manager `id`; nil is the hub's own assistant (`assistant`), whose things
    /// the hub leaves unstamped.
    func mutes(manager id: String?) -> Bool {
        guard let muted = mutedManagers, !muted.isEmpty else { return false }
        return muted.contains(id ?? "assistant")
    }

    /// "1 min" / "90 s": how the page words a duration from the hub.
    static func duration(_ seconds: Int?) -> String {
        let s = seconds ?? 60
        if s >= 60, s % 60 == 0 { return String(localized: "\(s / 60) min [Task Views 135d12]") }
        return String(localized: "\(s) s")
    }
}

nonisolated struct ToolSpec: Codable, Sendable {
    var name: String
    var description: String
    var inputSchema: JSONValue

    enum CodingKeys: String, CodingKey { case name, description; case inputSchema = "input_schema" }
}

nonisolated struct ToolContent: Codable, Sendable {
    var type: String
    var text: String?
    var data: String?
    var mimeType: String?

    enum CodingKeys: String, CodingKey { case type, text, data; case mimeType = "mime_type" }

    static func text(_ s: String) -> ToolContent { ToolContent(type: "text", text: s) }
    static func image(_ data: Data, mime: String) -> ToolContent {
        ToolContent(type: "image", data: data.base64EncodedString(), mimeType: mime)
    }
}

// MARK: - Mail

nonisolated struct MailAddress: Codable, Hashable, Sendable {
    var name: String?
    var email: String

    var display: String { (name?.isEmpty == false ? name! : email) }
}

nonisolated struct MailAccount: Codable, Identifiable, Hashable, Sendable {
    var id: String
    var name: String?
    var email: String
    var provider: String?
    var auth: String?
    var username: String?
    var imapHost: String?
    var imapPort: Int?
    var smtpHost: String?
    var smtpPort: Int?
    var smtpSecurity: String?
    var status: String?
    var error: String?

    enum CodingKeys: String, CodingKey {
        case id, name, email, provider, auth, username, status, error
        case imapHost = "imap_host", imapPort = "imap_port", smtpHost = "smtp_host", smtpPort = "smtp_port",
             smtpSecurity = "smtp_security"
    }
}

nonisolated struct MailSummary: Codable, Identifiable, Hashable, Sendable {
    var accountID: String
    var mailbox: String
    var uid: UInt32
    var messageID: String?
    var subject: String?
    var from: MailAddress?
    var to: [MailAddress]?
    var date: Date?
    var snippet: String?
    var seen: Bool
    var flagged: Bool
    var hasAttachments: Bool

    var id: String { "\(accountID)/\(mailbox)/\(uid)" }

    enum CodingKeys: String, CodingKey {
        case mailbox, uid, subject, from, to, date, snippet, seen, flagged
        case accountID = "account_id", messageID = "message_id", hasAttachments = "has_attachments"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        accountID = try c.decode(String.self, forKey: .accountID)
        mailbox = try c.decodeIfPresent(String.self, forKey: .mailbox) ?? "INBOX"
        uid = try c.decode(UInt32.self, forKey: .uid)
        messageID = try c.decodeIfPresent(String.self, forKey: .messageID)
        subject = try c.decodeIfPresent(String.self, forKey: .subject)
        from = try c.decodeIfPresent(MailAddress.self, forKey: .from)
        to = try c.decodeIfPresent([MailAddress].self, forKey: .to)
        date = try c.decodeIfPresent(Date.self, forKey: .date)
        snippet = try c.decodeIfPresent(String.self, forKey: .snippet)
        seen = try c.decodeIfPresent(Bool.self, forKey: .seen) ?? true
        flagged = try c.decodeIfPresent(Bool.self, forKey: .flagged) ?? false
        hasAttachments = try c.decodeIfPresent(Bool.self, forKey: .hasAttachments) ?? false
    }
}

nonisolated struct MailAttachment: Codable, Hashable, Sendable {
    var filename: String?
    var mime: String?
    var size: Int?
}

nonisolated struct MailMessage: Codable, Hashable, Sendable {
    var accountID: String
    var mailbox: String?
    var uid: UInt32
    var messageID: String?
    var subject: String?
    var from: MailAddress?
    var to: [MailAddress]?
    var cc: [MailAddress]?
    var date: Date?
    var text: String?
    var html: String?
    var attachments: [MailAttachment]?
    var inReplyTo: String?
    var references: [String]?
    var seen: Bool?
    var flagged: Bool?

    enum CodingKeys: String, CodingKey {
        case mailbox, uid, subject, from, to, cc, date, text, html, attachments, references, seen, flagged
        case accountID = "account_id", messageID = "message_id", inReplyTo = "in_reply_to"
    }
}

nonisolated struct Mailbox: Codable, Hashable, Sendable, Identifiable {
    var name: String
    var role: String?
    var unseen: Int?
    var total: Int?
    var id: String { name }
}

/// A terminal dialog read on the hub (server/internal/screen): what the card shows, and `rev`, which the node checks
/// before pressing anything for the answer.
nonisolated struct ScreenDialog: Codable, Hashable, Sendable {
    struct Option: Codable, Hashable, Sendable {
        var key: String
        var label: String
        var detail: String?
        var kind: String?
        /// Ticked at the computer (a multi-select box), or an answered question's pick.
        var checked: Bool?
        /// What is typed into the free-text field (`type_something`).
        var text: String?
    }
    struct Step: Codable, Hashable, Sendable {
        var label: String
        var done: Bool
    }
    var header: String?
    var question: String
    var detail: String?
    var options: [Option]
    var rev: String
    /// A multi-select question: check boxes, sent together by its `submit` row ("Submit", or "Next" when another
    /// question follows). Steps are the questions of its tab bar; review is the tab that submits them all
    /// (incomplete: one has no answer).
    var multi: Bool?
    var submit: String?
    var steps: [Step]?
    var review: Bool?
    var incomplete: Bool?
    /// A permission prompt for a shell command (#5266): the command, the agent's words for it (`description`), the boxed
    /// warning as Claude Code words it and what it is about (`risk`: "delete"), the countdown as on screen, what it does
    /// when it runs out (`deadline_action`: "deny" / "allow") and when, by the hub's clock. A hub before it sends none.
    var command: String?
    var commandDescription: String?
    var warning: String?
    var risk: String?
    var deadline: String?
    var deadlineAction: String?
    var deadlineAt: Date?

    enum CodingKeys: String, CodingKey {
        case header, question, detail, options, rev, multi, submit, steps, review, incomplete
        case command, commandDescription = "description", warning, risk, deadline
        case deadlineAction = "deadline_action", deadlineAt = "deadline_at"
    }

    var parsed: ScreenParser.Dialog {
        var d = ScreenParser.Dialog(
            header: header.flatMap { $0.isEmpty ? nil : $0 }, question: question, detail: detail.flatMap { $0.isEmpty ? nil : $0 },
            options: options.map { o in
                let kind: ScreenParser.Dialog.Kind = o.kind == "type_something" ? .typeSomething : o.kind == "chat" ? .chat : .plain
                return .init(key: o.key, label: o.label, detail: o.detail.flatMap { $0.isEmpty ? nil : $0 }, kind: kind,
                             checked: o.checked ?? false, text: o.text.flatMap { $0.isEmpty ? nil : $0 })
            })
        if let steps, !steps.isEmpty { d.steps = steps.map { .init(label: $0.label, done: $0.done) } }
        d.multi = multi ?? false
        d.submit = submit
        d.review = review ?? false
        d.incomplete = incomplete ?? false
        d.commandAsk = CommandAsk(command: command, description: commandDescription, warning: warning, risk: risk,
                                  deadline: deadline, deadlineAction: deadlineAction, deadlineAt: deadlineAt)
        return d
    }
}

/// The words of Delete Chat's confirmation (the web's `DeleteSessionDialog`).
nonisolated enum DeleteSessionText {
    /// `machine`: where its record stays (nil: unknown); `resume`: the command that continues it there (nil: none).
    static func message(title: String, machine: String?, resume: String?) -> String {
        switch (machine, resume) {
        case let (machine?, resume?):
            String(localized: "“\(title)” will be removed from the list on all your devices. Its chat history on \(machine) won't be deleted; continue it on the computer with \(resume) and it'll come back.")
        case let (machine?, nil):
            String(localized: "“\(title)” will be removed from the list on all your devices. Its chat history on \(machine) won't be deleted; it'll come back the next time it's used.")
        case let (nil, resume?):
            String(localized: "“\(title)” will be removed from the list on all your devices. Its chat history on the computer won't be deleted; continue it there with \(resume) and it'll come back.")
        case (nil, nil):
            String(localized: "“\(title)” will be removed from the list on all your devices. Its chat history on the computer won't be deleted; it'll come back the next time it's used.")
        }
    }
}

/// Where an agent was opened (`Session.source`, #2526).
nonisolated enum SessionSource: String, Sendable {
    case feishu, terminal
    var label: String { self == .feishu ? String(localized: "Lark Group [Models e86099]") : String(localized: "Terminal") }
}

nonisolated extension Int64 {
    /// "0 bytes", "30 bytes", 12 KB, 3.4 MB, 1.2 GB: every size the app shows goes through here, so an empty file reads like
    /// a small one (the default formatter says "Zero KB" for 0 but "30 bytes" for 30).
    var storageText: String {
        let f = ByteCountFormatter()
        f.countStyle = .file
        f.allowsNonnumericFormatting = false
        return f.string(fromByteCount: self)
    }
}
