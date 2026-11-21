import SwiftUI

/// A main screen's title spot (Agents, Assistant, Inbox, Machines): its title, or while the hub is out of reach a small spinner
/// and Waiting for Network / Connecting… / Updating… (ConnectionStatus); a tap on that explains. Put it in the `.principal` toolbar item.
struct ConnectionTitle: View {
    /// nil: the screen shows no title (Agents) and the spot is empty while all is well.
    var title: String?
    @Environment(AppModel.self) private var model
    @State private var details = false

    var body: some View {
        let phase = model.connection.shown
        ZStack {
            if phase != .online {
                Button { Haptics.tap(); details = true } label: { ConnectionLabel(phase: phase) }
                    .buttonStyle(.plain)
                    // What the label shows (Hub updating… while it restarts), not the bare phase.
                    .accessibilityLabel("\(ConnectionLabel.text(phase, hubRestarting: model.hubUpdate?.restarting == true)), view connection details")
                    .accessibilityIdentifier("connection-title")
                    .transition(.opacity)
            } else if let title {
                Text(title).font(Theme.Font.heading).lineLimit(1).transition(.opacity)
            }
        }
        .animation(Theme.Motion.standard, value: phase)
        .connectionDetails($details)
        .titleSaysLink() // a page pushed in the shell (Inbox, Machines): its pill does not say it again
        #if DEBUG
        // `-openConnectionDetails YES` (with `-showConnection …`): the details open (screenshots).
        .task { if phase != .online, UserDefaults.standard.bool(forKey: "openConnectionDetails") { details = true } }
        #endif
    }
}

/// Spinner and state, at title size or (compact) subtitle size.
struct ConnectionLabel: View {
    @Environment(AppModel.self) private var model
    var phase: ConnectionStatus.Phase
    var compact = false

    var body: some View {
        HStack(spacing: compact ? 5 : 7) {
            if phase == .rejected {
                // Not trying anything the user can wait out: no spinner.
                ThemedIcon(Theme.Icon.error)
                    .font(compact ? Theme.Font.label : Theme.Font.heading)
                    .foregroundStyle(Theme.caution)
            } else {
                ProgressView().controlSize(compact ? .mini : .small)
            }
            Text(Self.text(phase, hubRestarting: model.hubUpdate?.restarting == true))
                .font(compact ? Theme.Font.label : Theme.Font.heading)
                .foregroundStyle(compact ? .secondary : .primary)
                .lineLimit(1)
                .contentTransition(.opacity)
        }
    }

    /// The words for `phase`. The hub restarting into a new version: the connection going away is expected
    /// (docs/operations/hub-update.md), so Connecting… says so. The title's accessibility label reads the same words.
    static func text(_ phase: ConnectionStatus.Phase, hubRestarting: Bool) -> String {
        phase == .connecting && hubRestarting ? String(localized: "Hub updating…") : phase.title
    }
}

extension View {
    /// The connection details as a small popover from the view (the spinner title).
    func connectionDetails(_ shown: Binding<Bool>) -> some View {
        popover(isPresented: shown) {
            ConnectionDetails().presentationCompactAdaptation(.popover)
        }
    }
}

/// Why the title says what it says, since when, and Retry Now: what used to be a banner over the list.
struct ConnectionDetails: View {
    @Environment(AppModel.self) private var model
    @State private var pairing = false

    var body: some View {
        let status = model.connection
        // What the title says; once it is back to normal, the live state (already connected again, or not yet shown).
        let phase = status.shown == .online ? status.phase : status.shown
        TimelineView(.periodic(from: .now, by: 1)) { ctx in
            // Taller than the screen (many addresses, large text): it scrolls instead of being cut off (#11390).
            ScrollView {
                VStack(alignment: .leading, spacing: Theme.Space.m) {
                    HStack(spacing: Theme.Space.s) {
                        if phase == .online {
                            ThemedIcon(Theme.Icon.successFilled).foregroundStyle(Theme.positive)
                            Text("Connected to Hub").font(Theme.Font.heading)
                        } else {
                            ConnectionLabel(phase: phase)
                        }
                    }
                    Text(phase == .connecting && model.hubUpdate?.restarting == true
                         ? String(localized: "The hub is updating to a new version. It'll disconnect briefly while it restarts, then reconnect automatically.")
                         : Self.explanation(phase, stalled: model.live.stalled)).font(Theme.Font.secondary).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    let lines = Self.facts(phase: phase, link: model.live.state, nextTry: model.live.nextTry, lastOnline: status.lastOnline,
                                           rtt: model.live.rtt, outbox: model.outboxCount, now: ctx.date, ssh: sshFact)
                    if !lines.isEmpty {
                        VStack(alignment: .leading, spacing: Theme.Space.xs) {
                            ForEach(lines, id: \.self) { Text($0) }
                        }
                        .font(Theme.Font.caption)
                        .foregroundStyle(.secondary)
                    }
                    if let group = model.hub?.group { ConnectionAddresses(group: group) }
                    if phase == .rejected {
                        Button {
                            Haptics.tap()
                            pairing = true
                        } label: {
                            Text("Pair Again").lineLimit(1).frame(maxWidth: .infinity)
                        }
                        .themedButton(prominent: true)
                        .disabled(model.hubSignOut != nil) // AppModel.pair would refuse it
                        .padding(.top, Theme.Space.xxs)
                        .accessibilityIdentifier("connection.repair")
                    }
                    if phase != .online {
                        Button {
                            Haptics.tap()
                            model.live.reconnectNow()
                            Task { await model.refresh() }
                        } label: {
                            Text("Retry Now [Connection Title 77c30e]").lineLimit(1).frame(maxWidth: .infinity)
                        }
                        .themedButton(prominent: phase != .rejected)
                        .padding(.top, Theme.Space.xxs)
                    }
                }
                .padding(Theme.Space.l)
                .scaledFrame(width: Theme.Size.box300, alignment: .leading)
            }
            .scrollBounceBehavior(.basedOnSize)
        }
        .sheet(isPresented: $pairing) { PairView() }
    }

    /// The hub-level group's tunnel, when its hub has one: the in-app one or the local proxy while it carries the
    /// traffic, else the SSH one, else the in-app one, else the local proxy.
    private var sshFact: SSHFact? {
        guard let group = model.hub?.group else { return nil }
        let inApp: SSHFact? = model.inAppTunnelState(group).map { t in
            var problem: String?
            if case .down(let why) = t.state { problem = why }
            return SSHFact(server: "", routed: t.routed, problem: problem, inApp: t.route.title)
        }
        if inApp?.routed == true { return inApp }
        let proxy: SSHFact? = model.localProxyState(group).map { t in
            var problem: String?
            if case .down(let why) = t.state { problem = why }
            return SSHFact(server: "", routed: t.routed, problem: problem, inApp: String(localized: "Local Proxy"))
        }
        if proxy?.routed == true { return proxy }
        guard let t = model.tunnel(group) else { return inApp ?? proxy }
        var problem: String?
        if case .down(let why) = t.state { problem = why }
        return SSHFact(server: t.hub.ssh?.label ?? "", routed: t.routed, problem: problem)
    }

    static func explanation(_ phase: ConnectionStatus.Phase, stalled: Bool) -> String {
        switch phase {
        case .online: String(localized: "Messages and status are up to date.")
        #if os(macOS)
        case .waitingForNetwork: String(localized: "This Mac is offline. It'll reconnect when the network is back, and messages you write in the meantime will be sent automatically.")
        #else
        case .waitingForNetwork: String(localized: "Your iPhone is offline. It'll reconnect when the network is back, and messages you write in the meantime will be sent automatically.")
        #endif
        case .connecting:
            stalled ? String(localized: "The network is slow: the hub hasn't responded for several seconds. Waiting for it.")
                : String(localized: "You're online but can't reach the hub (the hub machine may be off, asleep, or offline). Persona keeps retrying automatically.")
        case .updating: String(localized: "Connected. Catching up on messages and pending requests from while you were disconnected.")
        #if os(macOS)
        case .rejected: String(localized: "The hub no longer accepts this Mac's credentials. They may have been revoked in Settings › Devices, or the last pairing didn't finish. Paste the pairing link again to fix it.")
        #else
        case .rejected: String(localized: "The hub no longer accepts this device's credentials. They may have been revoked in Settings › Devices, or the last pairing didn't finish. Generate a new pairing code on the hub machine and scan it again to fix it.")
        #endif
        }
    }

    /// The hub's SSH tunnel as the details mention it (docs/operations/network.md「Connecting to hub through SSH」): which server, whether the
    /// traffic goes through it now, and why it is not up.
    struct SSHFact: Equatable {
        var server: String
        var routed: Bool
        var problem: String?
        /// The way's name when it is not the SSH tunnel: the in-app tunnel's
        /// (docs/operations/network.md「Connecting to hub through an in-app tunnel」), or Local Proxy.
        var inApp: String?
    }

    /// Short lines under the explanation: why the last try failed, when the next is, since when, what is waiting, and
    /// the way to the hub when it has an SSH tunnel (straight, or through it).
    static func facts(phase: ConnectionStatus.Phase, link: LiveConnection.State, nextTry: Date?, lastOnline: Date?,
                      rtt: TimeInterval?, outbox: Int, now: Date, ssh: SSHFact? = nil) -> [String] {
        var out: [String] = []
        if let ssh {
            if let name = ssh.inApp {
                out.append(ssh.routed ? String(localized: "Route: via \(name)") : String(localized: "Route: direct"))
                if phase != .online, let why = ssh.problem { out.append(String(localized: "\(name): \(why)")) }
            } else {
                out.append(ssh.routed ? String(localized: "Route: SSH tunnel (\(ssh.server))") : String(localized: "Route: direct"))
                if phase != .online, let why = ssh.problem { out.append(String(localized: "SSH tunnel: \(why)")) }
            }
        }
        if phase == .connecting || phase == .rejected, case .failed(let why) = link { out.append(String(localized: "Last failure: \(why)")) }
        if phase == .connecting || phase == .rejected, let nextTry {
            let s = Int(ceil(nextTry.timeIntervalSince(now)))
            out.append(s > 0 ? String(localized: "Retrying in \(s) seconds") : String(localized: "Retrying…"))
        }
        if phase != .online, let lastOnline { out.append(String(localized: "Last connected: \(ago(lastOnline, now: now))")) }
        if phase == .online, let rtt {
            let ms = String(format: "%.0f", rtt * 1000)
            out.append(String(localized: "Latency \(ms) ms"))
        }
        if outbox > 0 { out.append(String(localized: "\(outbox) messages will be sent when connected")) }
        return out
    }

    /// "12 seconds ago", "3 minutes ago", then the time of day.
    static func ago(_ date: Date, now: Date) -> String {
        let s = max(0, Int(now.timeIntervalSince(date)))
        if s < 60 { return String(localized: "\(s) seconds ago") }
        if s < 3600 { return String(localized: "\(s / 60) min ago") }
        return Fmt.relative(date, now: now)
    }
}

/// One line per address the app tries for the group, in that order, with what its last try came to and when (#11390,
/// `HubAddressBoard`); the address the socket is on says In Use, as Settings › hub › Addresses does. Its own view, so
/// the details' redraw every second does not run it: it reads the lines worked out when a try ends and the socket's
/// state (docs/apple/performance.md P4, P5).
struct ConnectionAddresses: View {
    @Environment(AppModel.self) private var model
    let group: String

    var body: some View {
        let lines = model.addressBoard.lines[group] ?? []
        if !lines.isEmpty {
            let live = model.socket(group)
            let inUse = live?.state == .connected ? live?.currentConfig.map { HubRoute.key($0.url) } : nil
            VStack(alignment: .leading, spacing: Theme.Space.sm) {
                Text("Addresses").font(Theme.Font.captionStrong).foregroundStyle(.secondary)
                ForEach(lines) { line in row(line, inUse: line.id == inUse) }
            }
            .accessibilityIdentifier("connection.addresses")
        }
    }

    private func row(_ line: HubAddressBoard.Line, inUse: Bool) -> some View {
        VStack(alignment: .leading, spacing: Theme.Space.xxs) {
            Text(verbatim: line.address).font(Theme.Font.caption).lineLimit(1).truncationMode(.middle)
            if inUse {
                Chip(text: String(localized: "In Use [Hubs View 564c83]"), look: .outline)
            } else if line.outcome != nil {
                // The outcome and its time on one line when they fit, else the time under it (long words, large text).
                ViewThatFits(in: .horizontal) {
                    HStack(spacing: Theme.Space.sm) { outcome(line) }
                    VStack(alignment: .leading, spacing: Theme.Space.xxs) { outcome(line) }
                }
            } else {
                Text(line.word).font(Theme.Font.caption).foregroundStyle(.secondary)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("connection.address")
    }

    @ViewBuilder private func outcome(_ line: HubAddressBoard.Line) -> some View {
        Chip(text: line.word, look: .neutral)
        if let tried = line.tried {
            Text(tried).font(Theme.Font.caption).foregroundStyle(.secondary).fixedSize().accessibilityLabel(line.spokenTime ?? tried)
        }
    }
}
