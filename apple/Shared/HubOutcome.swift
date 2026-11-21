import Foundation

/// What one try of a hub address came to (#11390, docs/operations/network.md "Client-added candidates"): the health
/// probe's answer (`HubRoute.healthCheck`), or, for a group with one address and no tunnel, the socket's own. Only
/// `answers` passes, exactly as the probe's yes or no did before; the rest say why an address failed. The raw values are
/// the words Developing Persona's connection tally writes (docs/operations/developing-persona.md).
nonisolated enum HubOutcome: String, CaseIterable, Codable, Hashable, Sendable {
    /// A healthy hub answer, with the paired hub's `hub_id` where the probe checks it (added addresses).
    case answers
    /// TLS failed the system's trust (an unknown root, a self-signed certificate, a wrong host name or date). Kept apart
    /// from the rest: without key pinning the phone can't tell the hub's own self-signed certificate from someone else's.
    case untrusted = "certificate_not_trusted"
    /// An added public address answered with another `hub_id`.
    case notYourHub = "not_your_hub"
    /// Refused, timed out, host not found, connection lost, or not a healthy hub answer.
    case unreachable = "couldnt_connect"
    /// Cut short: another address won and the rest were cancelled, a newer attempt replaced this one, a restored SSH tunnel
    /// came up first, or the app went to the background first.
    case unknown

    /// Which says more about why an open never connected: not your hub, then the certificate, then couldn't connect,
    /// then unknown. `answers` says nothing about a failure (nil).
    var failureRank: Int? {
        switch self {
        case .notYourHub: 0
        case .untrusted: 1
        case .unreachable: 2
        case .unknown: 3
        case .answers: nil
        }
    }

    /// The most telling failure among `outcomes`, unknown when there is none (an open that ended before any try failed).
    static func worst(_ outcomes: some Sequence<HubOutcome>) -> HubOutcome {
        outcomes.filter { $0.failureRank != nil }.min { $0.failureRank! < $1.failureRank! } ?? .unknown
    }

    /// A request or a socket that failed, by its error: one classifier for both. Cancelled: unknown (another address
    /// won, a newer attempt or a restored tunnel came first, or the app went away). A certificate the system doesn't trust: untrusted, also when it shows only as a failed
    /// secure connection carrying the server's trust or a certificate error code. Anything else: couldn't connect.
    init(failure error: any Error) {
        if error is CancellationError { self = .unknown; return }
        let ns = error as NSError
        guard ns.domain == NSURLErrorDomain else { self = .unreachable; return }
        switch ns.code {
        case NSURLErrorCancelled:
            self = .unknown
        case NSURLErrorServerCertificateUntrusted, NSURLErrorServerCertificateHasBadDate,
             NSURLErrorServerCertificateHasUnknownRoot, NSURLErrorServerCertificateNotYetValid:
            self = .untrusted
        case NSURLErrorSecureConnectionFailed:
            self = Self.trustFailed(ns) ? .untrusted : .unreachable
        default:
            self = .unreachable
        }
    }

    /// The socket's handshake answered but did not open: the hub turning the credential down (401), Cloudflare Access in
    /// front, anything but a socket. Not a healthy hub answer.
    static let refusedHandshake: HubOutcome = .unreachable

    /// Secure Transport's codes for a certificate the system would not trust (`SecureTransport.h`): chain invalid, bad
    /// certificate, unknown root, no root, expired, not yet valid, host name mismatch. A handshake cut short
    /// (`errSSLClosedAbort`, `errSSLClosedNoNotify`, a protocol error) is not among them.
    static let certificateCodes: Set<Int> = [-9807, -9808, -9812, -9813, -9814, -9815, -9843]

    private static func trustFailed(_ error: NSError) -> Bool {
        if error.userInfo[NSURLErrorFailingURLPeerTrustErrorKey] != nil { return true }
        if let code = error.userInfo["_kCFStreamErrorCodeKey"] as? Int, certificateCodes.contains(code) { return true }
        if let under = error.userInfo[NSUnderlyingErrorKey] as? NSError {
            if certificateCodes.contains(under.code) { return true }
            if under.userInfo[NSURLErrorFailingURLPeerTrustErrorKey] != nil { return true }
        }
        return false
    }
}

/// Whether an address only names the hub on the network it was seen on: loopback, a private IPv4 range (10/8,
/// 172.16/12, 192.168/16), link-local (169.254/16, fe80::/10), a unique local IPv6 address (fc00::/7), `localhost` and
/// `.local` names. The same rule as Android's (`LocalAddressPolicy`, `endpointKind`); no DNS is asked.
nonisolated enum HubAddressScope {
    static func isPrivate(_ url: URL) -> Bool {
        guard var host = url.host(percentEncoded: false)?.lowercased(), !host.isEmpty else { return false }
        if host.hasPrefix("["), host.hasSuffix("]") { host = String(host.dropFirst().dropLast()) }
        if let zone = host.firstIndex(of: "%") { host = String(host[..<zone]) }
        if host == "localhost" || host.hasSuffix(".local") || host.hasSuffix(".local.") { return true }
        if let v4 = ipv4(host) { return privateV4(v4) }
        if let v6 = ipv6(host) { return privateV6(v6) }
        return false
    }

    private static func ipv4(_ host: String) -> [UInt8]? {
        var addr = in_addr()
        guard host.contains("."), !host.contains(":"), inet_pton(AF_INET, host, &addr) == 1 else { return nil }
        return withUnsafeBytes(of: addr.s_addr) { Array($0) }
    }

    private static func ipv6(_ host: String) -> [UInt8]? {
        var addr = in6_addr()
        guard host.contains(":"), inet_pton(AF_INET6, host, &addr) == 1 else { return nil }
        return withUnsafeBytes(of: addr) { Array($0) }
    }

    private static func privateV4(_ b: [UInt8]) -> Bool {
        b[0] == 127 || b[0] == 10 || (b[0] == 172 && (16...31).contains(b[1])) || (b[0] == 192 && b[1] == 168)
            || (b[0] == 169 && b[1] == 254)
    }

    private static func privateV6(_ b: [UInt8]) -> Bool {
        if b[0..<15].allSatisfy({ $0 == 0 }) && b[15] == 1 { return true } // ::1
        if b[0] == 0xfe && (b[1] & 0xc0) == 0x80 { return true } // fe80::/10, link-local
        if b[0] == 0xfe && (b[1] & 0xc0) == 0xc0 { return true } // fec0::/10, the old site-local (Java's isSiteLocal)
        if (b[0] & 0xfe) == 0xfc { return true } // fc00::/7, unique local
        // An IPv4 address written as IPv6 (`::ffff:a.b.c.d`): by its IPv4 part.
        if b[0..<10].allSatisfy({ $0 == 0 }) && b[10] == 0xff && b[11] == 0xff { return privateV4(Array(b[12..<16])) }
        return false
    }
}
