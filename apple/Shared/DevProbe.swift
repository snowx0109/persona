import Foundation
import os
import PersonaWire

/// Where shared code tells Developing Persona (Settings › About, #9460, docs/operations/developing-persona.md) what it
/// counts: the hub socket's frames by type, its connects and drops, the error reports the app keeps, and what each hub
/// group's attempts to connect came to (#11390). A no-op until the app installs a sink, which it does only while the
/// switch is on; the watch and the extensions never do.
nonisolated enum DevProbe {
    enum Socket: String, Sendable { case connected, dropped }

    /// One hub group's connection, for the weekly connection tally (#11390). `group` is the app's own id for the group
    /// (`PairedHub.group`): it tells groups apart in memory and is never written.
    enum Link: Sendable {
        /// The groups there are now (Demo Mode left out).
        case groups(Set<String>)
        /// What one attempt's addresses came to, each once; `since`: when the attempt began.
        case tried(group: String, [HubOutcome], since: Date)
        /// The group's socket connected, and how (said once the hub's hello came).
        case connected(group: String, Connected)
        /// The group's socket is no longer connected (it dropped, failed, or closed for the background).
        case disconnected(group: String)
    }

    /// How a connected group reached its hub: only fixed words and yes / no, never an address, host, port, hub id or
    /// fingerprint.
    struct Connected: Hashable, Sendable {
        /// What vouched for the connection: the system's trust (HTTPS straight to the address in use), nothing (plain
        /// HTTP straight to it), or the tunnel or proxy it went through (its loopback end is plain HTTP; no TLS to the
        /// hub is checked by the app).
        enum Trust: String, CaseIterable, Sendable { case system, plainHTTP = "plain_http", tunnel }
        /// The way the socket went: straight, or through the hub's SSH tunnel, its in-app tunnel or its local proxy.
        enum Route: String, CaseIterable, Sendable { case direct, sshTunnel = "ssh_tunnel", inAppTunnel = "in_app_tunnel", localProxy = "local_proxy" }
        /// Which address it is: the one paired, one the user added, or one only the mesh listed.
        enum Address: String, CaseIterable, Sendable { case paired, added, mesh }

        var trust: Trust
        /// The address carries its own Cloudflare Access service token (`ServerConfig.access`).
        var accessToken: Bool
        var route: Route
        var address: Address
        /// The hub's hello lists a key fingerprint for itself (`mesh.hubs[].fp`), yes or no only.
        var helloFingerprint: Bool
    }

    protocol Sink: AnyObject, Sendable {
        func frame(type: String, bytes: Int)
        func socket(_ event: Socket)
        func error(_ report: ExtensionErrorReport)
        func link(_ event: Link)
    }

    private static let sink = OSAllocatedUnfairLock<(any Sink)?>(initialState: nil)

    static func install(_ new: (any Sink)?) { sink.withLock { $0 = new } }

    /// A sink is installed: the switch is on. Callers check it before working anything out for `link`.
    static var listening: Bool { sink.withLock { $0 } != nil }

    static func frame(type: String, bytes: Int) { sink.withLock { $0 }?.frame(type: type, bytes: bytes) }
    static func socket(_ event: Socket) { sink.withLock { $0 }?.socket(event) }
    static func error(_ report: ExtensionErrorReport) { sink.withLock { $0 }?.error(report) }
    static func link(_ event: Link) { sink.withLock { $0 }?.link(event) }
}
