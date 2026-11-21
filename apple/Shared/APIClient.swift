import Foundation
import PersonaWire

nonisolated struct ServerConfig: Codable, Hashable, Sendable {
    var url: URL
    var token: String
    /// The hub's Cloudflare Access service token, when it sits behind Access (docs/operations/network.md "Behind
    /// Cloudflare Access"); nil: none.
    var access: HubAccess?

    /// Parses `persona://pair?url=…&token=…`. Not a computer's `…://link?…`
    /// (`isComputerLink`, for Persona Link on that computer): its one-time code would pair this device in its place.
    init?(pairURL: String) {
        guard let comps = URLComponents(string: pairURL.trimmingCharacters(in: .whitespacesAndNewlines)),
              HubNaming.accepts(scheme: comps.scheme),
              comps.host?.lowercased() != "link",
              let u = comps.queryItems?.first(where: { $0.name == "url" })?.value, let url = URL(string: u),
              let token = comps.queryItems?.first(where: { $0.name == "token" })?.value, !token.isEmpty
        else { return nil }
        self.url = url
        self.token = token
    }

    /// A `persona://pair` link, whether or not `init(pairURL:)` can read it: one cut short (no `url`
    /// or no `token`: copied in part, wrapped in a chat) is still someone trying to pair, and is told so (#4273).
    static func isPairLink(_ link: String) -> Bool {
        guard let comps = URLComponents(string: link.trimmingCharacters(in: .whitespacesAndNewlines)) else { return false }
        return HubNaming.accepts(scheme: comps.scheme) && comps.host?.lowercased() == "pair"
    }

    /// `persona://link?…`: the pairing link a signed-in device made for one computer (「添加电脑」, Add computer).
    /// It is opened on that computer, by Persona Link; a phone or the Mac app only says so.
    static func isComputerLink(_ url: URL) -> Bool {
        HubNaming.accepts(scheme: url.scheme) && url.host()?.lowercased() == "link"
    }

    init(url: URL, token: String, access: HubAccess? = nil) {
        self.url = url
        self.token = token
        self.access = access
    }

    /// What every request to the hub carries: this device's credential and, to the hub's own address only, its
    /// Cloudflare Access service token.
    func authorize(_ request: inout URLRequest) {
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        CloudflareAccess.apply(access, to: &request, hub: url)
    }

    /// Demo Mode: the made-up hub inside the app (Persona/App/DemoHub.swift). Its host can never resolve, so nothing
    /// that slipped past `APIClient.local` reaches a network.
    static let demo = ServerConfig(url: URL(string: "https://demo.invalid")!, token: "demo")
    var isDemo: Bool { url.host() == "demo.invalid" }
}

/// One request the app's own made-up hub answers instead of the network (Demo Mode).
nonisolated struct LocalRequest: Sendable {
    var method: String
    var path: String
    var query: [String: String]
    /// Query items a name can repeat in (`site=a&site=b`), after `query`'s, in their order.
    var repeated: [URLQueryItem] = []
    var body: Data?
    /// Operation semantics and anonymous caller diagnostics, matching the real request's headers.
    var headers: [String: String] = [:]
    /// The caller's bound on the real URL request, exposed to transport-level tests.
    var timeout: TimeInterval = 30
    /// The real request keeps off cellular data and expensive networks (`URLRequest.keepOnWiFi`), exposed to
    /// transport-level tests as `timeout` is.
    var wifiOnly = false
}

nonisolated extension URLRequest {
    /// **Hub transcription on Wi-Fi only** (#3862 phase 5): with `on`, the system itself refuses to send the request over
    /// cellular data or a network marked expensive (a phone's hotspot), whatever the app last saw of the network; a path
    /// that changes under it (Wi-Fi Assist on a weak Wi-Fi, Wi-Fi gone during a retry) fails the request instead.
    mutating func keepOnWiFi(_ on: Bool) {
        guard on else { return }
        allowsCellularAccess = false
        allowsExpensiveNetworkAccess = false
    }
}

nonisolated struct APIError: LocalizedError, Sendable {
    var status: Int
    var message: String
    /// The response's other string fields: a refused send (409) has code (dialog_open / input_draft), delivered
    /// (none / unknown), draft and clearable (docs/protocol/protocol.md「发消息」).
    var fields: [String: String] = [:]
    /// The hub logs failed requests under this id (X-Request-Id): a server error shows it, to find that line.
    var requestID: String?
    /// Structured error details the user has to confirm (e.g. the extensions that depend on it); not logged.
    var details: JSONValue?
    /// Route context for the one pre-code knowledge refusal; never match diagnostics from unrelated endpoints.
    var requestPath: String?
    /// A node-menu action relays the computer's actionable refusal as 502 internal; keep those words, not a generic service error.
    var showsNodeReason = false
    var errorDescription: String? {
        let text = businessMessage ?? (code == "internal" && status >= 500 && !showsNodeReason
            ? String(localized: "The service couldn't complete this request. Try again later. (HTTP \(status))") : message)
        guard status >= 500, let requestID else { return text }
        return String(localized: "\(text) (request \(requestID))")
    }
    var code: String? {
        let code = fields["code"]
        let path = requestPath?.split(separator: "?", maxSplits: 1).first.map(String.init)
        let knowledge = path == "/api/v1/kb" || path?.hasPrefix("/api/v1/kb/") == true || path == "/api/v1/kb-search"
            || path == "/api/v1/recordings" || path?.hasPrefix("/api/v1/recordings/") == true
        if knowledge, status == 503, code == "internal", fields["detail"] == "the knowledge base is off" { return "kb_disabled" }
        return code
    }

    /// Business codes precede status and free-form server wording. Raw fields remain available for diagnostics.
    var businessMessage: String? {
        switch code {
        case "kb_disabled": String(localized: "This assistant has turned off the knowledge base. Enable it in the assistant's configuration to use it.")
        case "kb_unavailable": String(localized: "The knowledge base isn't ready yet. Try again shortly.")
        case "kb_read_only": String(localized: "The knowledge base is read-only. Change the assistant's configuration before editing or syncing recordings.")
        case "assistant_offline": String(localized: "The assistant is offline or restarting. Try again shortly.")
        case "node_unreachable":
            fields["delivered"] == "none"
                ? String(localized: "This computer is offline: Persona Link isn't running, or the computer is asleep.")
                : String(localized: "The connection to this computer dropped. Reconnect it and check whether your last action went through before retrying.")
        case "timeout": String(localized: "The request timed out. Try again later.")
        default: nil
        }
    }

    /// The hub's words for a call that ran out of time (Go's context deadline, a node call's timeout).
    static func isTimeoutText(_ message: String) -> Bool {
        let m = message.lowercased()
        return m.contains("deadline exceeded") || m.contains("timed out") || m.contains("timeout") || m.contains("超时") // i18n-ignore: matches the hub's text
    }

    static func fields(in data: Data) -> [String: String] {
        guard case .object(let obj)? = try? JSON.decoder.decode(JSONValue.self, from: data) else { return [:] }
        return obj.compactMapValues { $0.string }
    }

    /// The hub's `{"error":"…"}`, or the Lark gateway's `{"error":{"code","message"}}` passed through the read-only proxy.
    static func message(in data: Data) -> String? {
        if let flat = (try? JSON.decoder.decode([String: String].self, from: data))?["error"] { return flat }
        guard let obj = try? JSON.decoder.decode(JSONValue.self, from: data), let err = obj["error"] else { return nil }
        // The hub's error body can also carry non-string fields such as `retryable`, which the flat decode above rejects.
        if case .string(let text) = err { return text }
        let message = err["message"]?.string
        let code = err["code"]?.string
        switch (message, code) {
        case let (m?, c?): return String(localized: "\(m) (\(c))", comment: "An error message and its code")
        case let (m?, nil): return m
        case let (nil, c?): return c
        default: return nil
        }
    }
}

nonisolated enum DemoText {
    /// What a Demo Mode action that would reach a computer, Feishu or mail says instead.
    static var notSent: String { String(localized: "Nothing is actually sent in demo mode") }
    /// `APIError.code` of such a refusal.
    static let code = "demo"
}

extension Set where Element == String {
    /// A hub feature of the extension system under its new name or the one hubs from before #1844 say: "ui_slots" is
    /// extension_ui_slots or plugin_ui_slots, "manager" extension_manager or plugin_manager.
    nonisolated func hasExtensionFeature(_ name: String) -> Bool { contains("extension_" + name) || contains("plugin_" + name) }
}

/// REST client for the hub. All endpoints return JSON objects; errors are `{"error": "..."}`.
nonisolated struct APIClient: Sendable {
    typealias Transport = @Sendable (LocalRequest) async throws -> Data

    var config: ServerConfig
    var session: URLSession = .shared
    /// Answers every request in place of the network: unit tests' `FakeHub`. The demo config uses `local`.
    var transport: Transport?
    /// A live hub link's route attempt, when one is in progress. Standalone clients and the direct health probes
    /// have no such wait. Returning a config updates the request's destination before its URL is constructed.
    var requestRoute: (@Sendable () async throws -> ServerConfig?)?
    /// The group of paired hubs this client talks to (docs/protocol/multi-hub.md): put on the sessions, machines,
    /// interactions and keys it returns. "" = the default group.
    var group: String = ""
    /// The Feishu account (gateway account number) the `/api/v1/lark` requests are about, sent as `ol_account`
    /// (docs/extensions/feishu.md「Multiple accounts」). nil: the hub's own (lark.account).
    var larkAccount: Int?
    /// The manager every request names (`X-Persona-Manager`, docs/protocol/protocol.md "Per-manager routing"): another
    /// manager's extension or connector. nil: none named, so the hub asks the default manager, as before.
    var managerID: String?
    /// The manager the Persona page shows (its switcher, docs/assistant/managers.md "Clients"), named on the assistant's
    /// own routes only (`assistantRoute`). nil: the default's, and nothing is named, as before.
    var shownManager: String?
    /// The hub has the extension routes (`features` has `extensions`, #1844): extension requests go to
    /// /api/v1/extensions and answer with the new keys; an older hub only has /api/v1/plugins (docs/extensions/extensions.md「Naming and compatibility」).
    var extensionsAPI = false
    /// Explicitly advertised by this hub's hello; absent/unknown stays compatible with pre-rename hubs.
    var personaNaming = false
    /// The hub's hello has `sessions_lookup`: one session's complete row can be asked for by machine and id.
    var sessionLookup = false
    /// The hub's hello has `sessions_list`: session lists come in pages with a revision that `since` follows.
    var sessionList = false
    /// The hub's hello has `sessions_hierarchy`: windowed lists and counts send `roots=1`. Older hubs do not, so a
    /// child stays a row of its own. A parent-session page is a separate request and never sends `roots`.
    var sessionsHierarchy = false
    /// Execution surface is diagnostic only; authorization still uses the paired hub credential.
    var clientMetadata: RequestClientMetadata = .current
    /// Answers the demo config's requests (DemoHub registers it at launch); every endpoint goes through `request`.
    nonisolated(unsafe) static var local: (@Sendable (LocalRequest) async throws -> Data)?
    /// The app's answer to a hub that retired this device's push registration (#9612): registers with the group again
    /// and says whether that reached its hub. Set once at launch (`PushRegistrar`); nil in the extensions and the watch.
    nonisolated(unsafe) static var onPushRegistrationRequired: (@Sendable (_ group: String) async -> Bool)?
    /// This client's own answer in place of `onPushRegistrationRequired`: a test's.
    var registerPush: (@Sendable (_ group: String) async -> Bool)?
    /// A write for this device (`/api/v1/devices/{id}/…`: notification settings, Live Activity tokens) whose push
    /// registration the hub retired (docs/protocol/protocol.md devices).
    static let pushRegistrationRequired = "push_registration_required"

    /// Resolve once per operation. The returned copy has no callback, so a transport helper can delegate to itself
    /// without waiting twice. URL creation, authorization, and the operation's manager headers then use one config.
    private func preparedForRequest() async throws -> APIClient {
        var prepared = self
        if let selected = try await requestRoute?() { prepared.config = selected }
        prepared.requestRoute = nil
        try Task.checkCancellation()
        return prepared
    }

    /// For bytes the app caches itself (chat images, Feishu media): through `.shared` the system's URLCache kept a
    /// second copy of every one of them.
    static let uncachedSession: URLSession = {
        let c = URLSessionConfiguration.default
        c.urlCache = nil
        c.requestCachePolicy = .reloadIgnoringLocalCacheData
        return URLSession(configuration: c, delegate: HubRedirects.shared, delegateQueue: nil)
    }()

    /// Where a request goes: under the hub's address, through its SSH tunnel when the app routes it there
    /// (HubTransport). `direct`: the hub's own address all the same (the probe that asks whether it answers there).
    func url(_ path: String, query: [String: String] = [:], repeated: [URLQueryItem] = [], direct: Bool = false) -> URL {
        var comps = URLComponents(url: config.url, resolvingAgainstBaseURL: false)!
        var base = comps.percentEncodedPath
        if base.hasSuffix("/") { base.removeLast() }
        comps.percentEncodedPath = base + Self.encodedPath(path)
        let items = query.filter { !$0.value.isEmpty }.map { URLQueryItem(name: $0.key, value: $0.value) }.sorted { $0.name < $1.name } + repeated
        if !items.isEmpty { comps.queryItems = items }
        return direct ? comps.url! : HubTransport.resolve(comps.url!, hub: config.url)
    }

    /// A path for the request line. Callers escape ids themselves (`kbDetail`: a slug with Chinese in it, an id with
    /// a slash); `URL.appending(path:)` escaped their `%` once more and the hub got another id — 404 (2026-10-30, Knowledge).
    /// So: a path that already has escapes is used as is, anything else is escaped like appending(path:) did.
    nonisolated static func encodedPath(_ path: String) -> String {
        let p = path.hasPrefix("/") ? path : "/" + path
        if p.range(of: "%[0-9A-Fa-f]{2}", options: .regularExpression) != nil { return p }
        return p.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? p
    }

    /// An extension route: `rest` ("", "/-/markets", "/<id>/-/…") under /api/v1/extensions, or under /api/v1/plugins on a
    /// hub from before the rename.
    func extensionPath(_ rest: String) -> String { (extensionsAPI ? "/api/v1/extensions" : "/api/v1/plugins") + rest }

    /// This client for one Feishu account's chats; nil: the hub's own.
    func lark(_ account: Int?) -> APIClient {
        var c = self
        c.larkAccount = account
        return c
    }

    static let managerHeader = "X-Persona-Manager"

    /// This client for one manager's things (an item a merged list stamped with that manager): every request names it.
    /// nil or empty: the default manager's, and nothing is named, as before, whichever manager the Persona page shows.
    /// Each page makes its requests through its own copy, so a request that goes out after the page moved on still names
    /// the manager it was made for.
    func manager(_ id: String?) -> APIClient {
        var c = self
        c.managerID = id?.isEmpty == false ? id : nil
        c.shownManager = nil
        return c
    }

    /// Freeze the current assistant for a settings page, including routes outside `/assistant`.
    /// An explicit item manager takes precedence; the default still sends no manager header.
    func currentAssistantSettings() -> APIClient { manager(managerID ?? shownManager) }

    /// Whether two captured settings requests still belong to the same hub and assistant.
    func sameSettingsScope(as other: APIClient?) -> Bool {
        guard let other else { return false }
        return config == other.config && group == other.group
            && (managerID ?? shownManager) == (other.managerID ?? other.shownManager)
    }

    /// This client for the Persona page while it shows manager `id` (nil or empty: the default's).
    func showing(_ id: String?) -> APIClient {
        var c = self
        c.shownManager = id?.isEmpty == false ? id : nil
        return c
    }

    /// Whether a request goes by the manager the Persona page shows: the assistant's routes (`/api/v1/assistant…`), but
    /// not one conversation's (`/api/v1/assistant/conversations/{id}…`), which the hub sends to the conversation's own
    /// manager and a named one would override, nor the health space's (`space`), which stays the default's.
    nonisolated static func assistantRoute(_ path: String, query: [String: String]) -> Bool {
        let base = "/api/v1/assistant"
        guard path == base || path.hasPrefix(base + "/") || path.hasPrefix(base + "?"),
              query["space"]?.isEmpty != false, !path.contains("space=") else { return false }
        let parts = path.dropFirst(base.count).split(separator: "?", maxSplits: 1).first?.split(separator: "/") ?? []
        return !(parts.count > 1 && parts[0] == "conversations")
    }

    /// The recordings' part of the knowledge base (`/api/v1/kb/recordings…`: a recording's minutes, a day's recap, the
    /// hub's list of them). This device syncs its recordings to the default manager whichever assistant is current
    /// (`/api/v1/recordings` names none), so what was made of them is read there too.
    nonisolated static func recordingsRoute(_ path: String) -> Bool {
        let base = "/api/v1/kb/recordings"
        return path == base || path.hasPrefix(base + "/") || path.hasPrefix(base + "?")
    }

    /// What a request to `path` carries besides the caller's own headers: the manager it names. None for the
    /// recordings' part of the knowledge base, whichever manager this client is for (`recordingsRoute`): the knowledge
    /// page's client names the current assistant on everything else.
    func routing(_ path: String, _ query: [String: String]) -> [String: String] {
        if Self.recordingsRoute(path) { return [:] }
        let named = managerID ?? (Self.assistantRoute(path, query: query) ? shownManager : nil)
        return named.map { [Self.managerHeader: $0] } ?? [:]
    }

    /// `query` with the Feishu account a `/api/v1/lark` request is about, unless it names one already (a media URL
    /// the hub handed out for that account).
    func larkScoped(_ path: String, _ query: [String: String]) -> [String: String] {
        guard let larkAccount, path.hasPrefix("/api/v1/lark/"), query["ol_account"] == nil else { return query }
        var q = query
        q["ol_account"] = String(larkAccount)
        return q
    }

    /// `opID`: the write's op id (`X-Persona-Op-Id`, docs/protocol/protocol.md「操作回执」): the hub does it once however often
    /// it arrives. A repeat it cannot answer yet comes back `202 {"state":"pending" | "unknown"}`: thrown as an
    /// APIError with `OpReceipt.pendingCode` / `unknownCode`, so no caller takes it for done.
    /// `extraHeaders`: what this one request says besides (a hand-off card's approval declares its feature).
    /// A request about the replayed meeting (`ReplayedMeeting`, #10589) never leaves the phone: it ends here, as an error
    /// with the code `replayed_meeting`, whatever route it was for.
    static func refuseReplayed(_ path: String, _ query: [String: String]) throws {
        guard ReplayedMeeting.refuses(path, query: query) else { return }
        var error = APIError(status: 409, message: String(localized: "A replayed meeting stays on this iPhone."),
                             fields: ["code": ReplayedMeeting.refusal])
        error.requestPath = path
        throw error
    }

    func request(_ method: String, _ path: String, query: [String: String] = [:], repeated: [URLQueryItem] = [], body: (any Encodable)? = nil,
                 timeout: TimeInterval = 30, opID: UUID? = nil, extraHeaders: [String: String] = [:]) async throws -> Data {
        // Keep the non-Sendable Encodable body in this request's task instead of sending it to a concurrent local function.
        nonisolated(nonsending) func send() async throws -> Data {
            let routed = try await preparedForRequest()
            return try await routed.sendRequest(method, path, query: query, repeated: repeated, body: body, timeout: timeout, opID: opID, extraHeaders: extraHeaders)
        }
        do {
            return try await send()
        } catch let e as APIError where e.status == 409 && e.code == Self.pushRegistrationRequired && path.hasPrefix("/api/v1/devices/") {
            // The hub retired this device's push registration (#9611): register again, then this write once more. A
            // registration that didn't reach the hub (or one the hub just took and refuses again) leaves the 409.
            guard let register = registerPush ?? Self.onPushRegistrationRequired, await register(group) else { throw e }
            return try await send()
        }
    }

    private func sendRequest(_ method: String, _ path: String, query: [String: String], repeated: [URLQueryItem], body: (any Encodable)?,
                             timeout: TimeInterval, opID: UUID?, extraHeaders: [String: String]) async throws -> Data {
        let query = larkScoped(path, query)
        try Self.refuseReplayed(path, query)
        let headers = clientMetadata.headers.merging(opID.map(HubNaming.operationHeaders) ?? [:]) { _, operation in operation }
            .merging(routing(path, query)) { _, manager in manager }
            .merging(extraHeaders) { _, extra in extra }
        if let answer = transport ?? (config.isDemo ? Self.local : nil) {
            let data: Data
            do {
                data = try await answer(LocalRequest(method: method, path: path, query: query.filter { !$0.value.isEmpty }, repeated: repeated,
                                                     body: try body.map { try JSON.encoder.encode(AnyEncodable($0)) }, headers: headers, timeout: timeout))
            } catch var error as APIError {
                error.requestPath = path
                throw error
            }
            if opID != nil { try Self.checkReceipt(data, status: nil) }
            return data
        }
        if config.isDemo { throw APIError(status: 503, message: String(localized: "Demo mode isn't available")) }
        var req = URLRequest(url: url(path, query: query, repeated: repeated), timeoutInterval: timeout)
        req.httpMethod = method
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        // The caller's own id when it names one (a page that shows it, to match the hub's log), else a new one.
        let requestID = headers["X-Client-Request-Id"] ?? String(UUID().uuidString.prefix(8)).lowercased()
        req.setValue(requestID, forHTTPHeaderField: "X-Client-Request-Id")
        for (name, value) in headers { req.setValue(value, forHTTPHeaderField: name) }
        if let body {
            req.httpBody = try JSON.encoder.encode(AnyEncodable(body))
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let (data, resp) = try await session.data(for: req, delegate: HubRedirects.shared)
        let ok = try checked(data, resp, requestID: requestID, path: path)
        if opID != nil { try Self.checkReceipt(ok, status: (resp as? HTTPURLResponse)?.statusCode ?? 0) }
        return ok
    }

    /// A repeated write the hub could not answer from its receipt: `202 {"state":"pending"}` (the first is still
    /// running) or `{"state":"unknown"}` (nobody knows whether it ran). Only a 202 (a fake transport has no status, nil:
    /// its body alone).
    static func checkReceipt(_ data: Data, status: Int?) throws {
        guard status == 202 || status == nil, data.count < 256,
              case .object(let obj)? = try? JSON.decoder.decode(JSONValue.self, from: data), obj.count == 1,
              let state = obj["state"]?.string else { return }
        switch state {
        case "pending": throw APIError(status: 202, message: String(localized: "The last request is still being processed"), fields: ["code": OpReceipt.pendingCode])
        case "unknown": throw APIError(status: 202, message: String(localized: "Not sure whether the last request went through"), fields: ["code": OpReceipt.unknownCode])
        default: return
        }
    }

    private func checked(_ data: Data, _ resp: URLResponse, requestID: String, path: String? = nil) throws -> Data {
        try CloudflareAccessError.check(resp, body: data, access: config.access)
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else {
            throw APIError(status: status, message: APIError.message(in: data) ?? HTTPURLResponse.localizedString(forStatusCode: status),
                           fields: APIError.fields(in: data),
                           requestID: (resp as? HTTPURLResponse)?.value(forHTTPHeaderField: "X-Request-Id") ?? requestID,
                           details: try? JSON.decoder.decode(JSONValue.self, from: data), requestPath: path)
        }
        return data
    }

    /// A GET that may answer 304 Not Modified: `data` is nil when the copy tagged `etag` is still current
    /// (`If-None-Match`), else the new bytes and their tag. The fake transport has no headers: the tag rides in the query
    /// as `If-None-Match` and a 304 is an APIError with that status.
    func conditionalGet(_ path: String, query: [String: String], etag: String?) async throws -> (data: Data?, etag: String?) {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.conditionalGet(path, query: query, etag: etag)
        }
        try Self.refuseReplayed(path, query)
        if transport != nil || config.isDemo {
            var q = query
            if let etag { q["If-None-Match"] = etag }
            do { return (try await request("GET", path, query: q), nil) } catch let e as APIError where e.status == 304 { return (nil, etag) }
        }
        var req = URLRequest(url: url(path, query: query), timeoutInterval: 30)
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        let requestID = String(UUID().uuidString.prefix(8)).lowercased()
        req.setValue(requestID, forHTTPHeaderField: "X-Client-Request-Id")
        if let etag { req.setValue(etag, forHTTPHeaderField: "If-None-Match") }
        for (name, value) in routing(path, query) { req.setValue(value, forHTTPHeaderField: name) }
        let (data, resp) = try await Self.uncachedSession.data(for: req) // our own cache: URLSession's would hide the 304
        let http = resp as? HTTPURLResponse
        if http?.statusCode == 304 { return (nil, etag) }
        return (try checked(data, resp, requestID: requestID, path: path), http?.value(forHTTPHeaderField: "ETag"))
    }

    /// A request whose answer is read as it arrives (the realtime voice saying a text, `voiceSpeak`): the response once
    /// its headers are in, then its bytes. A status outside 2xx throws as `request` does, with the body it came with.
    /// Neither the fake transport nor Demo Mode streams: it throws there, and callers do what they do without it.
    func streamed(_ method: String, _ path: String, body: (any Encodable)? = nil,
                  timeout: TimeInterval = 30) async throws -> (HTTPURLResponse, URLSession.AsyncBytes) {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.streamed(method, path, body: body, timeout: timeout)
        }
        try Self.refuseReplayed(path, [:])
        if transport != nil || config.isDemo { throw APIError(status: 501, message: "Streaming isn't available here") }
        var req = URLRequest(url: url(path, query: [:]), timeoutInterval: timeout)
        req.httpMethod = method
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        let requestID = String(UUID().uuidString.prefix(8)).lowercased()
        req.setValue(requestID, forHTTPHeaderField: "X-Client-Request-Id")
        for (name, value) in routing(path, [:]) { req.setValue(value, forHTTPHeaderField: name) }
        if let body {
            req.httpBody = try JSON.encoder.encode(AnyEncodable(body))
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let (bytes, resp) = try await session.bytes(for: req, delegate: HubRedirects.shared)
        guard let http = resp as? HTTPURLResponse else { throw URLError(.badServerResponse) }
        if !(200..<300).contains(http.statusCode) {
            var data = Data()
            for try await byte in bytes {
                data.append(byte)
                if data.count >= 16 << 10 { break }
            }
            _ = try checked(data, resp, requestID: requestID, path: path)
        }
        return (http, bytes)
    }

    /// `PUT` with `data` as an `application/octet-stream` body (a meeting's gap audio, #3862 phase 5: `PUT …/audio?at=`),
    /// answered as `request` is. The fake transport gets the bytes as the body. `wifiOnly`: the system sends it over
    /// neither cellular data nor an expensive network (**Hub transcription on Wi-Fi only**, `URLRequest.keepOnWiFi`).
    func putBytes(_ path: String, query: [String: String] = [:], data: Data, timeout: TimeInterval = 120,
                  wifiOnly: Bool = false) async throws -> Data {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.putBytes(path, query: query, data: data, timeout: timeout, wifiOnly: wifiOnly)
        }
        let query = larkScoped(path, query)
        try Self.refuseReplayed(path, query)
        if config.isDemo { throw APIError(status: 403, message: DemoText.notSent) }
        let headers = clientMetadata.headers.merging(routing(path, query)) { _, manager in manager }
        if let transport {
            do {
                return try await transport(LocalRequest(method: "PUT", path: path, query: query, body: data, headers: headers, timeout: timeout,
                                                        wifiOnly: wifiOnly))
            } catch var error as APIError {
                error.requestPath = path
                throw error
            }
        }
        var req = URLRequest(url: url(path, query: query), timeoutInterval: timeout)
        req.httpMethod = "PUT"
        req.keepOnWiFi(wifiOnly)
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        req.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        let requestID = String(UUID().uuidString.prefix(8)).lowercased()
        req.setValue(requestID, forHTTPHeaderField: "X-Client-Request-Id")
        for (name, value) in headers { req.setValue(value, forHTTPHeaderField: name) }
        let (body, response) = try await session.upload(for: req, from: data, delegate: HubRedirects.shared)
        return try checked(body, response, requestID: requestID, path: path)
    }

    /// A bounded, staged attachment sent from disk. The fake transport reads it only in tests.
    func uploadBytes(file: URL, path: String, query: [String: String], timeout: TimeInterval = 180) async throws -> Data {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.uploadBytes(file: file, path: path, query: query, timeout: timeout)
        }
        let query = larkScoped(path, query)
        try Self.refuseReplayed(path, query)
        if config.isDemo { throw APIError(status: 403, message: DemoText.notSent) }
        if let transport {
            return try await transport(LocalRequest(method: "POST", path: path, query: query, body: Data(contentsOf: file),
                                                    headers: clientMetadata.headers.merging(routing(path, query)) { _, manager in manager }))
        }
        var req = URLRequest(url: url(path, query: query), timeoutInterval: timeout)
        req.httpMethod = "POST"
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        req.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        let requestID = String(UUID().uuidString.prefix(8)).lowercased()
        req.setValue(requestID, forHTTPHeaderField: "X-Client-Request-Id")
        for (name, value) in routing(path, query) { req.setValue(value, forHTTPHeaderField: name) }
        let (data, response) = try await session.upload(for: req, fromFile: file, delegate: HubRedirects.shared)
        return try checked(data, response, requestID: requestID, path: path)
    }

    nonisolated struct UploadedFile: Codable, Sendable {
        var path: String
        var name: String
        var size: Int64
    }

    /// Sends a file to the machine's temp directory (docs/protocol/protocol.md「Sending files」), streamed from disk; progress gets
    /// the bytes sent so far. Cancelling the task cancels the upload and the machine removes what it wrote.
    /// dest "terminal": a file pasted into a terminal, kept in Persona's own directory (docs/integrations/terminal.md section 18).
    /// dest "dir": into the folder `query` names (dir, cwd, conflict; docs/protocol/protocol.md "Changing files"), under its own name.
    func upload(file: URL, name: String, machine: String, dest: String? = nil, query extra: [String: String] = [:],
                progress: @escaping @Sendable (Int64) -> Void) async throws -> UploadedFile {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.upload(file: file, name: name, machine: machine, dest: dest, query: extra, progress: progress)
        }
        if config.isDemo { throw APIError(status: 403, message: DemoText.notSent) }
        var query = extra.merging(["name": name]) { _, new in new }
        if let dest { query["dest"] = dest }
        if let transport { // unit tests (FakeHub): the whole file in one request
            let body = try Data(contentsOf: file)
            let out = try await transport(LocalRequest(method: "POST", path: "/api/v1/machines/\(machine)/files", query: query, body: body, headers: clientMetadata.headers))
            progress(Int64(body.count))
            return try JSON.decoder.decode(UploadedFile.self, from: out)
        }
        var req = URLRequest(url: url("/api/v1/machines/\(machine)/files", query: query), timeoutInterval: 120)
        req.httpMethod = "POST"
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        let requestID = String(UUID().uuidString.prefix(8)).lowercased()
        req.setValue(requestID, forHTTPHeaderField: "X-Client-Request-Id")
        req.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        let (data, resp) = try await Self.uncachedSession.upload(for: req, fromFile: file, delegate: UploadProgress(progress))
        return try JSON.decoder.decode(UploadedFile.self, from: try checked(data, resp, requestID: requestID))
    }

    func download(_ path: String, query: [String: String], to destination: URL,
                  progress: @escaping @Sendable (Int64, Int64) -> Void) async throws {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.download(path, query: query, to: destination, progress: progress)
        }
        let query = larkScoped(path, query)
        try Self.refuseReplayed(path, query)
        if transport != nil || config.isDemo {
            let data = try await request("GET", path, query: query)
            try Task.checkCancellation()
            try data.write(to: destination, options: .atomic)
            progress(Int64(data.count), Int64(data.count))
            return
        }
        var req = URLRequest(url: url(path, query: query), timeoutInterval: 120)
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        let requestID = String(UUID().uuidString.prefix(8)).lowercased()
        req.setValue(requestID, forHTTPHeaderField: "X-Client-Request-Id")
        for (name, value) in routing(path, query) { req.setValue(value, forHTTPHeaderField: name) }
        let (temp, response) = try await Self.uncachedSession.download(for: req, delegate: DownloadProgress(progress))
        defer { try? FileManager.default.removeItem(at: temp) }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if !(200..<300).contains(status) {
            let handle = try FileHandle(forReadingFrom: temp)
            defer { try? handle.close() }
            _ = try checked(handle.read(upToCount: 64 << 10) ?? Data(), response, requestID: requestID, path: path)
        }
        try Task.checkCancellation()
        let size = (try FileManager.default.attributesOfItem(atPath: temp.path)[.size] as? NSNumber)?.int64Value ?? 0
        if response.expectedContentLength >= 0, size != response.expectedContentLength {
            throw APIError(status: 0, message: String(localized: "The file didn't fully download. Try again."))
        }
        try FileManager.default.moveItem(at: temp, to: destination)
        progress(size, size)
    }

    func get<T: Decodable>(_ path: String, query: [String: String] = [:], as: T.Type = T.self) async throws -> T {
        try JSON.decoder.decode(T.self, from: try await request("GET", path, query: query))
    }

    /// `get` for answers big enough to show in a frame: `get` decodes on the caller's actor (the main one, for a view),
    /// this one off it.
    func getDecodedOffMain<T: Decodable & Sendable>(_ path: String, query: [String: String] = [:], as: T.Type = T.self) async throws -> T {
        let data = try await request("GET", path, query: query)
        return try await Task.detached(priority: .userInitiated) { try JSON.decoder.decode(T.self, from: data) }.value
    }

    func post<T: Decodable>(_ path: String, body: (any Encodable)? = nil, timeout: TimeInterval = 30, as: T.Type = T.self) async throws -> T {
        try JSON.decoder.decode(T.self, from: try await request("POST", path, body: body ?? [String: String](), timeout: timeout))
    }

    @discardableResult
    func post(_ path: String, body: (any Encodable)? = nil, timeout: TimeInterval = 30, opID: UUID? = nil,
              extraHeaders: [String: String] = [:]) async throws -> Data {
        try await request("POST", path, body: body ?? [String: String](), timeout: timeout, opID: opID, extraHeaders: extraHeaders)
    }

    func delete(_ path: String) async throws {
        _ = try await request("DELETE", path)
    }

    // MARK: Endpoints

    struct MachinesResponse: Decodable { var machines: [Machine] }
    /// A machine's launchers and what runs with none picked (`default`, #2455; nil from an older hub). With
    /// `purpose=assistant` (#5033) it also carries `unavailable`: launchers and providers that can't run the main chat;
    /// for New Agent (`agent.new`, #8109) the Persona-managed providers left out there by their own-service rule.
    struct LaunchersResponse: Decodable, Sendable {
        var launchers: [Launcher]
        var `default`: LauncherDefault?
        var unavailable: [UnavailableLauncher]?
        /// New Agent's presets from the model profile in use; nil from an older hub or when none apply here.
        var tiers: [AgentPreset]? = nil
        var tiersProfile: AgentPresetProfile? = nil
        enum CodingKeys: String, CodingKey {
            case launchers, `default`, unavailable, tiers
            case tiersProfile = "tiers_profile"
        }
    }

    /// The body of PUT /api/v1/assistant/conversations/{id}/model (#5033): the pick kept for one conversation.
    struct ConversationModelBody: Encodable, Sendable {
        var launcher: String
        var model: String?
        var effort: String?
        var machineID: String
        enum CodingKeys: String, CodingKey { case launcher, model, effort, machineID = "machine_id" }
    }
    nonisolated struct SessionsResponse: Decodable, Sendable { var sessions: [Session] }
    struct SessionResponse: Decodable { var session: Session }
    struct ItemsResponse: Decodable, Sendable {
        var items: [Item]
        var hasMore: Bool
        var history: String?
        var toSeq: Int?
        /// A page counted back from the end (no from: it does not continue anything).
        var span: ItemSpan { ItemSpan(history: history, from: nil, to: toSeq) }
        enum CodingKeys: String, CodingKey { case items, history; case hasMore = "has_more", toSeq = "to_seq" }
    }
    struct ScreenResponse: Decodable { var text: String; var status: SessionStatus? }
    struct InteractionsResponse: Decodable { var interactions: [Interaction] }

    func health() async throws -> Bool {
        struct H: Decodable { var ok: Bool }
        return try await get("/api/v1/health", as: H.self).ok
    }

    /// Which hub this is (`GET /api/v1/health`: `hub_id`, `hub_name`; docs/archive/operations/plan-hub-mesh.md 2.6).
    struct Identity: Decodable, Sendable {
        var hubID: String?
        var hubName: String?
        enum CodingKeys: String, CodingKey { case hubID = "hub_id", hubName = "hub_name" }
    }
    func identity() async throws -> Identity { try await get("/api/v1/health", as: Identity.self) }


    func machines() async throws -> [Machine] {
        stamp(try await get("/api/v1/machines", as: MachinesResponse.self).machines)
    }
    func setEngine(machine: String, engine: MachineEngine) async throws -> Machine {
        struct Body: Encodable { var engine: String }
        // No terminal engine is the empty host set, which a single engine cannot say (docs/integrations/engines.md "Host sets").
        struct Hosts: Encodable { var hosts: [String] }
        let body: any Encodable = engine == .noEngine ? Hosts(hosts: []) : Body(engine: engine.rawValue)
        var m = try JSON.decoder.decode(Machine.self, from: await request("PUT", "/api/v1/machines/\(machine)/engine", body: body))
        m.group = group
        return m
    }

    /// Also Show botmux Sessions (#2575): whether the machine lists botmux's sessions beside its engine's. 404 on an older hub.
    func botmux(machine: String) async throws -> Bool {
        struct State: Decodable { var on: Bool }
        return try JSON.decoder.decode(State.self, from: await request("GET", "/api/v1/machines/\(machine)/botmux")).on
    }
    func setBotmux(machine: String, on: Bool) async throws -> Bool {
        struct Body: Encodable { var on: Bool }
        struct State: Decodable { var on: Bool }
        return try JSON.decoder.decode(State.self, from: await request("PUT", "/api/v1/machines/\(machine)/botmux", body: Body(on: on))).on
    }

    /// Settings › Session Triage (docs/assistant/agent-work.md「Switches」): the four switches, all off until the user turns them on. 404 on a hub
    /// without the triage, 503 while its assistant is not connected.
    func agentTriage() async throws -> TriageSwitches { try await get("/api/v1/agents/triage", as: TriageSwitches.self) }
    func setAgentTriage(_ switches: TriageSwitches) async throws -> TriageSwitches {
        try JSON.decoder.decode(TriageSwitches.self, from: await request("PUT", "/api/v1/agents/triage", body: switches))
    }

    /// What the machine has of Cua Driver now (its node is asked); 404 while the hub's plugin cua is off.
    func computer(machine: String) async throws -> MachineComputer {
        try JSON.decoder.decode(MachineComputer.self, from: await request("GET", "/api/v1/machines/\(machine)/computer"))
    }
    /// The machine page's Control Computer switch.
    func setComputer(machine: String, enabled: Bool) async throws -> MachineComputer {
        struct Body: Encodable { var enabled: Bool }
        return try JSON.decoder.decode(MachineComputer.self, from: await request("PUT", "/api/v1/machines/\(machine)/computer", body: Body(enabled: enabled)))
    }
    /// 「扩展开着时不自动锁屏」 for this Mac (#1507 b2; asked on the phone first).
    func setComputerNoAutoLock(machine: String, _ on: Bool) async throws -> MachineComputer {
        struct Body: Encodable { var no_auto_lock: Bool }
        return try JSON.decoder.decode(MachineComputer.self, from: await request("PUT", "/api/v1/machines/\(machine)/computer", body: Body(no_auto_lock: on)))
    }

    /// x264 for this Mac's live screen (#8859; the plugin's screen_x264, asked on the phone first).
    func setComputerScreenX264(machine: String, _ on: Bool) async throws -> MachineComputer {
        struct Body: Encodable { var screen_x264: Bool }
        return try JSON.decoder.decode(MachineComputer.self, from: await request("PUT", "/api/v1/machines/\(machine)/computer", body: Body(screen_x264: on)))
    }

    /// The machine page's Install Cua Driver, Grant Access and Cancel: `install`, `grant`, `cancel`.
    func computerJob(machine: String, _ action: String) async throws -> MachineComputer {
        try JSON.decoder.decode(MachineComputer.self, from: await request("POST", "/api/v1/machines/\(machine)/computer/\(action)"))
    }
    /// The Mac's clipboard through its node (#1343: text RFB's own cannot carry), read and set.
    func computerClipboard(machine: String) async throws -> String {
        struct Out: Decodable { var text: String }
        return try JSON.decoder.decode(Out.self, from: await request("GET", "/api/v1/machines/\(machine)/computer/clipboard")).text
    }
    func setComputerClipboard(machine: String, _ text: String) async throws {
        struct Body: Encodable { var text: String }
        _ = try await request("POST", "/api/v1/machines/\(machine)/computer/clipboard", body: Body(text: text))
    }
    /// Phone Screen (#9314, docs/protocol/protocol.md): start / windows / place / stop for the live screen whose RFB
    /// stream carries `pair`. A refusal keeps the node's code (`APIError.code`: busy, permission, not_signed_in…).
    func phoneScreen(machine: String, _ body: PhoneScreenRequest) async throws -> PhoneScreenReply {
        try JSON.decoder.decode(PhoneScreenReply.self, from: await request("POST", "/api/v1/machines/\(machine)/computer/phone-screen", body: body))
    }
    /// The apps always allowed for the assistant on this Mac (#1507), and taking one back (asked again next time).
    func computerApps(machine: String) async throws -> [ComputerApp] {
        struct Out: Decodable { var apps: [ComputerApp] }
        return try JSON.decoder.decode(Out.self, from: await request("GET", "/api/v1/machines/\(machine)/computer/apps")).apps
    }
    func removeComputerApp(machine: String, bundleID: String) async throws -> [ComputerApp] {
        struct Out: Decodable { var apps: [ComputerApp] }
        let id = bundleID.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/"))) ?? bundleID
        return try JSON.decoder.decode(Out.self, from: await request("DELETE", "/api/v1/machines/\(machine)/computer/apps/\(id)")).apps
    }

    /// What the Mac has of ego lite now; 404 while the hub's plugin ego is off.
    func browser(machine: String) async throws -> MachineBrowser {
        try JSON.decoder.decode(MachineBrowser.self, from: await request("GET", "/api/v1/machines/\(machine)/browser"))
    }
    /// The machine page's Let the Assistant Use This Browser.
    func setBrowser(machine: String, enabled: Bool) async throws -> MachineBrowser {
        struct Body: Encodable { var enabled: Bool }
        return try JSON.decoder.decode(MachineBrowser.self, from: await request("PUT", "/api/v1/machines/\(machine)/browser", body: Body(enabled: enabled)))
    }
    /// Install ego lite, Open ego lite, Cancel: `install`, `open`, `cancel`.
    func browserJob(machine: String, _ action: String) async throws -> MachineBrowser {
        try JSON.decoder.decode(MachineBrowser.self, from: await request("POST", "/api/v1/machines/\(machine)/browser/\(action)"))
    }

    /// The iPhones a computer reaches for the assistant (the extension iphone, docs/extensions/iphone.md).
    func iphone(machine: String) async throws -> MachineIPhone {
        try JSON.decoder.decode(MachineIPhone.self, from: await request("GET", "/api/v1/machines/\(machine)/iphone", timeout: 60))
    }
    /// The machine page's Allow the Assistant to Operate iPhones Through This Computer.
    func setIPhone(machine: String, enabled: Bool) async throws -> MachineIPhone {
        struct Body: Encodable { var enabled: Bool }
        return try JSON.decoder.decode(MachineIPhone.self, from: await request("PUT", "/api/v1/machines/\(machine)/iphone", body: Body(enabled: enabled), timeout: 60))
    }
    /// Pair (an iPhone plugged into the computer), Forget, Cancel, Install (the bridge), Prepare: `pair`, `forget`,
    /// `cancel`, `install`, `prepare`.
    func iphoneJob(machine: String, _ action: String, device: String?) async throws -> MachineIPhone {
        struct Body: Encodable { var device: String? }
        return try JSON.decoder.decode(MachineIPhone.self, from: await request("POST", "/api/v1/machines/\(machine)/iphone/\(action)", body: Body(device: device), timeout: 60))
    }
    /// The guided setup's Try It (#8199): a screenshot of this iPhone and Weather opened, without the model.
    func tryIPhone(device: String) async throws -> IPhoneTryResult {
        struct Body: Encodable { var device: String }
        return try JSON.decoder.decode(IPhoneTryResult.self, from: await request("POST", "/api/v1/iphone/try", body: Body(device: device), timeout: 120))
    }
    /// Every iPhone the computers reach, for this iPhone to find itself among them.
    func iphones() async throws -> IPhoneList {
        try JSON.decoder.decode(IPhoneList.self, from: await request("GET", "/api/v1/iphone", timeout: 60))
    }
    /// This iPhone's own Allow the Assistant to Operate This iPhone.
    /// With it goes this app's own bundle id and name, read from the bundle (never written down here): the hub keeps them
    /// so the assistant never operates Persona itself on the phone, where its approval cards show.
    func setIPhoneConsent(device: String, allowed: Bool) async throws -> IPhoneSeen {
        struct App: Encodable { var bundleID: String?; var name: String?
            enum CodingKeys: String, CodingKey { case bundleID = "bundle_id", name } }
        struct Body: Encodable { var device: String; var allowed: Bool; var app: App }
        let info = Bundle.main.infoDictionary
        let app = App(bundleID: Bundle.main.bundleIdentifier,
                      name: info?["CFBundleDisplayName"] as? String ?? info?["CFBundleName"] as? String)
        return try JSON.decoder.decode(IPhoneSeen.self, from: await request("PUT", "/api/v1/iphone/consent",
                                                                           body: Body(device: device, allowed: allowed, app: app)))
    }

    /// Browser Sign-ins (#1304): the sites with cookies in the Mac's ego lite; only the user's own apps may ask.
    func browserCookies(machine: String) async throws -> BrowserCookies {
        try JSON.decoder.decode(BrowserCookies.self, from: await request("GET", "/api/v1/machines/\(machine)/browser/cookies", timeout: 150))
    }
    /// Imports the cookies the user ticked; the hub passes them on and keeps nothing.
    func importBrowserCookies(machine: String, _ cookies: [BrowserCookie]) async throws -> BrowserCookies {
        struct Body: Encodable { var cookies: [BrowserCookie] }
        return try JSON.decoder.decode(BrowserCookies.self, from: await request("POST", "/api/v1/machines/\(machine)/browser/cookies", body: Body(cookies: cookies), timeout: 150))
    }
    /// Clears these sites' sign-ins in one call (`?site=a&site=b`), or (nil) all of them.
    func clearBrowserCookies(machine: String, sites: [String]?) async throws -> BrowserCookies {
        let path = "/api/v1/machines/\(machine)/browser/cookies"
        guard let sites else { return try JSON.decoder.decode(BrowserCookies.self, from: await request("DELETE", path, query: ["all": "1"], timeout: 150)) }
        let named = sites.map { URLQueryItem(name: "site", value: $0) }
        return try JSON.decoder.decode(BrowserCookies.self, from: await request("DELETE", path, repeated: named, timeout: 150))
    }

    /// The group on what the hub returned (docs/protocol/multi-hub.md).
    func stamp(_ list: [Machine]) -> [Machine] { group.isEmpty ? list : list.map { var m = $0; m.group = group; return m } }
    func stamp(_ list: [Session]) -> [Session] { group.isEmpty ? list : list.map { var s = $0; s.group = group; return s } }
    func stamp(_ list: [Interaction]) -> [Interaction] { group.isEmpty ? list : list.map { var i = $0; i.group = group; return i } }
    func stamp(_ s: Session) -> Session { var s = s; s.group = group; return s }
    func stamp(_ k: SessionKey) -> SessionKey { var k = k; k.group = group; return k }
    func launcherList(machine: String, purpose: String? = nil) async throws -> LaunchersResponse {
        let client = purpose == "assistant" ? currentAssistantSettings() : self
        return try await client.get("/api/v1/machines/\(machine)/launchers",
                                    query: purpose.map { ["purpose": $0] } ?? [:], as: LaunchersResponse.self)
    }
    /// Built-in catalog prices for the ids a turn used (`POST /api/v1/launchers/catalog`, #5426). An id the catalog
    /// leaves out is absent; a known id with no price is present and nil. Empty in, empty out.
    func catalogPrices(_ ids: [String]) async throws -> [String: ModelPrice?] {
        if ids.isEmpty { return [:] }
        struct Query: Encodable { var ids: [String] }
        struct Row: Decodable { var id: String; var price: ModelPrice? }
        struct List: Decodable { var models: [Row] }
        let list: List = try await post("/api/v1/launchers/catalog", body: Query(ids: ids))
        var out: [String: ModelPrice?] = [:]
        for row in list.models where !row.id.isEmpty { out[row.id] = row.price }
        return out
    }
    /// Keeps a pick (launcher, optional model and effort) for one of the assistant's conversations (#5033): its next
    /// messages run on it, on every device, until it is changed or follows the default again (`deleteConversationModel`).
    func putConversationModel(machine: String, conversation: String, body: ConversationModelBody) async throws {
        _ = try await request("PUT",
                              "/api/v1/assistant/conversations/\(conversation.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? conversation)/model",
                              body: body)
    }
    /// The conversation follows Model configuration › Main chat again (DELETE …/conversations/{id}/model, #5033).
    func deleteConversationModel(machine: String, conversation: String) async throws {
        try await delete("/api/v1/assistant/conversations/\(conversation.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? conversation)/model")
    }
    /// The models the launcher's agent lists in its /model menu (New Agent's model picker). The node starts the CLI to
    /// ask (a few seconds, kept 10 minutes); refresh asks it again. Assistant purpose asks the shell launcher's list,
    /// even when the computer's Agent engine has a provider of the same name.
    func launcherModels(machine: String, launcher: String, refresh: Bool = false, purpose: String? = nil, host: String? = nil) async throws -> LauncherModels {
        let path = "/api/v1/machines/\(machine)/launchers/\(launcher.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? launcher)/models"
        var query = refresh ? ["refresh": "1"] : [:]
        if let purpose { query["purpose"] = purpose }
        if let host, !host.isEmpty { query["host"] = host }
        let client = purpose == "assistant" ? currentAssistantSettings() : self
        return try JSON.decoder.decode(LauncherModels.self, from: await client.request("GET", path, query: query, timeout: 75))
    }
    /// Forgets one model ID typed for this launcher on this computer (#1644, the list's Forget): the hub stops listing
    /// it (`custom: true`). Forgetting one it doesn't remember is fine too.
    func forgetCustomModel(machine: String, launcher: String, model: String) async throws {
        let path = "/api/v1/machines/\(machine)/launchers/\(launcher.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? launcher)/models/custom"
        _ = try await request("DELETE", path, query: ["model": model])
    }
    /// Model Settings' list for a launcher provider (`launcher.<name>`, #943), asked on a machine that has the launcher (its
    /// `machine`); nil: an older hub, or a provider without a list (a plugin brings its own, an API has none).
    /// `machine`: the purpose's own (a task, Lark /agent run there), so the list is that machine's launcher.
    func providerModels(_ provider: String, refresh: Bool = false, machine: String? = nil) async throws -> LauncherModels? {
        let path = "/api/v1/model-config/providers/\(provider.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? provider)/models"
        var query: [String: String] = [:]
        if refresh { query["refresh"] = "1" }
        if let machine, !machine.isEmpty { query["machine"] = machine }
        do {
            return try JSON.decoder.decode(LauncherModels.self, from: await request("GET", path, query: query, timeout: 75))
        } catch let e as APIError where e.status == 404 {
            return nil
        }
    }
    func dirs(machine: String, path: String) async throws -> DirList {
        try await get("/api/v1/machines/\(machine)/dirs", query: ["path": path])
    }
    func reconnect(machine: String) async throws { try await post("/api/v1/machines/\(machine)/reconnect") }
    /// Renames the computer (hubs with feature `machine_rename`, docs/protocol/protocol.md "Renaming a computer"): "" is its id
    /// again; returns the name the hub calls it now. Every client gets it from the next `machines` push.
    func renameMachine(_ machine: String, to name: String) async throws -> String {
        struct Body: Encodable { var name: String }
        struct Renamed: Decodable { var name: String }
        return try JSON.decoder.decode(Renamed.self, from: await request("PUT", "/api/v1/machines/\(machine)/name", body: Body(name: name))).name
    }
    /// The hub installs its own personad build on the machine and reconnects; progress comes with the machine list.
    func deploy(machine: String) async throws { try await post("/api/v1/machines/\(machine)/deploy") }
    /// Starts the one confirmed switch from SSH to Persona Link (#10662). The accepted state is immediate; the machine
    /// list then carries every later phase. Callers never repeat this request when its answer is uncertain.
    func startLinkSwitch(machine: String) async throws -> MachineLinkSwitch {
        nonisolated struct R: Decodable, Sendable {
            var linkSwitch: MachineLinkSwitch
            enum CodingKeys: String, CodingKey { case linkSwitch = "link_switch" }
        }
        let id = machine.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/"))) ?? machine
        return try await post("/api/v1/machines/\(id)/link-switch", as: R.self).linkSwitch
    }

    /// Every machine's latest sample and its last hour by minute (docs/server/machines.md).
    func metricsOverview() async throws -> [MachineMetrics] {
        struct R: Decodable { var machines: [MachineMetrics] }
        return try await get("/api/v1/metrics", as: R.self).machines
    }
    /// The subscription accounts and their rate-limit windows (docs/server/usage-windows.md); [] from a hub before them (404).
    func usageWindows() async throws -> [UsageAccount] {
        struct R: Decodable { var accounts: [UsageAccount]? }
        do {
            return try await get("/api/v1/usage/windows", as: R.self).accounts ?? []
        } catch let e as APIError where e.status == 404 {
            return []
        }
    }
    func machineMetrics(_ machine: String, range: MetricRange) async throws -> MachineMetrics {
        nonisolated struct R: Decodable, Sendable { var metrics: MachineMetrics }
        let response = try await getDecodedOffMain("/api/v1/machines/\(machine)/metrics", query: ["range": range.rawValue], as: R.self)
        try Task.checkCancellation()
        return response.metrics
    }
    /// A machine's token usage over a range (docs/server/machines.md "Token usage"); model / launcher nil: all ("" is the
    /// unknown launcher, sent as "-"). 404 from a hub before it.
    func machineUsage(_ machine: String, range: UsageRange, model: String? = nil, launcher: String? = nil) async throws -> MachineUsageSeries {
        struct R: Decodable { var usage: MachineUsageSeries }
        var query = ["range": range.rawValue]
        if let model { query["model"] = model }
        if let launcher { query["launcher"] = launcher.isEmpty ? "-" : launcher }
        var usage = try await get("/api/v1/machines/\(machine)/usage", query: query, as: R.self).usage
        usage.receivedAt = Date() // the step under way has run until now (#9205)
        return usage
    }
    func sessions() async throws -> [Session] {
        let data = try await request("GET", "/api/v1/sessions")
        return stamp(try await Task.detached(priority: .userInitiated) {
            try JSON.decoder.decode(SessionsResponse.self, from: data).sessions
        }.value)
    }

    struct StartRequest: Encodable {
        var machineID: String
        var cwd: String
        var launcher: String
        /// The launcher's actual agent when a synchronous start needs it on the wire; nil preserves older starts.
        var agentType: String? = nil
        var prompt: String?
        var resumeSessionID: String?
        var workspaceID: String?
        /// The selected workspace's host; nil opens in the machine's default host.
        var host: String? = nil
        var permissionMode: String? = nil
        var effort: String? = nil
        /// The model picked in New Agent for this start (`LauncherModels`); nil: the launcher's, or Model Settings'.
        var model: String? = nil
        /// The herdr tab's name; nil: the hub makes one from the first message (else agent-<n>).
        var tabName: String? = nil
        /// Answer at once with the session it is going to be and start it in the background (docs/protocol/protocol.md「New session」).
        var async: Bool? = nil
        /// What the tab name is made from when the first message is sent as a message of its own (not `prompt`).
        var nameHint: String? = nil
        var todoID: String? = nil
        /// Claude Code's ultracode for this start (#3427): only `true` is ever sent (a new Claude is off anyway, and an
        /// older hub would ignore the field rather than refuse it); nil leaves it out.
        var ultracode: Bool? = nil
        enum CodingKeys: String, CodingKey {
            case cwd, launcher, prompt, host, effort, async, model, ultracode
            case machineID = "machine_id", agentType = "agent_type", resumeSessionID = "resume_session_id", workspaceID = "workspace_id",
                 permissionMode = "permission_mode", tabName = "tab_name", nameHint = "name_hint", todoID = "todo_id"
        }
    }

    func start(_ req: StartRequest) async throws -> Session {
        var body = req
        body.prompt = req.prompt.map { HubNaming.outbound($0, canonical: personaNaming) }
        return stamp(try await post("/api/v1/sessions", body: body, timeout: req.async == true ? 20 : 110, as: SessionResponse.self).session)
    }

    struct CodexTakeoverInfo: Decodable, Sendable {
        var running: Bool
        var canResume: Bool
        var reason: String?
        var token: String
        var cwd: String
        var transcriptPath: String
        var continuationPrompt: String
        enum CodingKeys: String, CodingKey {
            case running, reason, token, cwd
            case canResume = "can_resume", transcriptPath = "transcript_path", continuationPrompt = "continuation_prompt"
        }
    }

    private func codexTakeoverPath(machine: String, session: String) -> String {
        "/api/v1/machines/\(TodosLogic.pathComponent(machine))/codex/\(TodosLogic.pathComponent(session))/takeover"
    }

    func codexTakeoverInfo(machine: String, session: String) async throws -> CodexTakeoverInfo {
        try await get(codexTakeoverPath(machine: machine, session: session))
    }

    func codexTakeover(machine: String, session: String, token: String, confirmStop: Bool) async throws -> Session {
        struct Body: Encodable { var token: String; var confirmStop: Bool; enum CodingKeys: String, CodingKey { case token; case confirmStop = "confirm_stop" } }
        return stamp(try await post(codexTakeoverPath(machine: machine, session: session), body: Body(token: token, confirmStop: confirmStop),
                                   timeout: 110, as: SessionResponse.self).session)
    }

    private func sp(_ k: SessionKey) -> String { "/api/v1/sessions/\(k.machineID)/\(k.sessionID)" }

    /// Genuine user-input identities and full-history ordinals, without loading transcript bodies.
    func userInputIndex(_ k: SessionKey) async throws -> ChatUserInputIndex {
        let index = try await getDecodedOffMain(sp(k) + "/items", query: ["user_inputs": "1", "lazy_images": "1"], as: ChatUserInputIndex.self)
        guard index.isValid else { throw APIError(status: 0, message: "incomplete user input index") }
        return index
    }

    /// A page of history going back `turns` whole exchanges (user prompt + answer) before beforeSeq (nil: the latest
    /// page, with no before_seq at all: an empty one reached runtimes as 0, which some refuse).
    func items(_ k: SessionKey, beforeSeq: Int?, turns: Int = 2) async throws -> ItemsResponse {
        var query = ["turns": String(turns), "limit": "80", "lazy_images": "1", "fold_work": "1"]
        if let beforeSeq { query["before_seq"] = String(beforeSeq) }
        let data = try await request("GET", sp(k) + "/items", query: query)
        // Images come without their bytes (`image(ref:)` fetches one when it is shown); a page is still worth
        // decoding off the main thread, which called us.
        return try await Task.detached(priority: .userInitiated) { try JSON.decoder.decode(ItemsResponse.self, from: data) }.value
    }

    /// One range of a transcript (`from_seq` through `through_seq`), a single page. A child card reads the head for
    /// its prompt and a short fold tail for its last tool. It does not walk the whole transcript.
    func itemRange(_ k: SessionKey, from: Int, through: Int) async throws -> [Item] {
        struct Range: Decodable, Sendable { var items: [Item] }
        let data = try await request("GET", sp(k) + "/items",
                                     query: ["from_seq": String(from), "through_seq": String(through), "lazy_images": "1"])
        let page = try await Task.detached(priority: .userInitiated) { try JSON.decoder.decode(Range.self, from: data) }.value
        return page.items
    }

    /// The work a fold stands for (`Item.fold`): its lines, a page (about 1000 items) at a time.
    func foldedWork(_ k: SessionKey, _ fold: Item) async throws -> [Item] {
        struct Range: Decodable, Sendable { var items: [Item]; var more: Bool? }
        guard let through = fold.fold?.throughSeq else { return [] }
        var out: [Item] = []
        var from = fold.seq
        for _ in 0..<100 {
            let data = try await request("GET", sp(k) + "/items",
                                         query: ["from_seq": String(from), "through_seq": String(through), "lazy_images": "1"])
            let page = try await Task.detached(priority: .userInitiated) { try JSON.decoder.decode(Range.self, from: data) }.value
            out += page.items
            guard page.more == true, let last = page.items.last else { break }
            from = last.seq + 1
        }
        return out
    }

    /// The bytes of a lazy image on this hub: "<machine>/<session>/<item id>" (`ImageRef.split` of an `ImagePayload.ref`).
    func image(ref: String) async throws -> Data {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.image(ref: ref)
        }
        let parts = ref.split(separator: "/", maxSplits: 2).map(String.init)
        guard parts.count == 3 else { throw APIError(status: 0, message: "bad image ref") }
        if config.isDemo { return try await request("GET", "/api/v1/sessions/\(parts[0])/\(parts[1])/items/\(parts[2])/image") }
        var req = URLRequest(url: itemURL(machine: parts[0], session: parts[1], item: parts[2], "image"), timeoutInterval: 60)
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        let (data, resp) = try await Self.uncachedSession.data(for: req) // ChatImageLoader keeps it on disk
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else {
            throw APIError(status: status, message: APIError.message(in: data) ?? HTTPURLResponse.localizedString(forStatusCode: status))
        }
        return data
    }

    /// An item whole: the text a page cut (`Item.fullSize`).
    func fullItem(_ k: SessionKey, id: String) async throws -> Item {
        if requestRoute != nil {
            let routed = try await preparedForRequest()
            return try await routed.fullItem(k, id: id)
        }
        if config.isDemo || transport != nil {
            return try JSON.decoder.decode(Item.self, from: try await request("GET", sp(k) + "/items/\(id)/full"))
        }
        var req = URLRequest(url: itemURL(machine: k.machineID, session: k.sessionID, item: id, "full"), timeoutInterval: 60)
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        clientMetadata.apply(to: &req)
        let (data, resp) = try await Self.uncachedSession.data(for: req)
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else {
            throw APIError(status: status, message: APIError.message(in: data) ?? HTTPURLResponse.localizedString(forStatusCode: status),
                           fields: APIError.fields(in: data))
        }
        return try await Task.detached(priority: .userInitiated) { try JSON.decoder.decode(Item.self, from: data) }.value
    }

    /// …/items/<item id>/<what>. Item ids carry '#' and ':'; appending(path:) would leave '#' to start a fragment.
    private func itemURL(machine: String, session: String, item: String, _ what: String) -> URL {
        let id = item.addingPercentEncoding(withAllowedCharacters: .alphanumerics.union(CharacterSet(charactersIn: "-._~"))) ?? item
        var comps = URLComponents(url: config.url, resolvingAgainstBaseURL: false)!
        var base = comps.percentEncodedPath
        if base.hasSuffix("/") { base.removeLast() }
        comps.percentEncodedPath = base + "/api/v1/sessions/\(machine)/\(session)/items/\(id)/\(what)"
        return HubTransport.resolve(comps.url!, hub: config.url)
    }

    struct NewerResponse: Decodable, Sendable {
        var items: [Item]
        /// The hub stopped at its cap: more came after these (ask again after `toSeq`, or take the latest page).
        var moreAfter: Bool?
        var history: String?
        var fromSeq: Int?
        var toSeq: Int?
        var span: ItemSpan { ItemSpan(history: history, from: fromSeq, to: toSeq) }
        enum CodingKeys: String, CodingKey {
            case items, history
            case moreAfter = "more_after", fromSeq = "from_seq", toSeq = "to_seq"
        }
    }

    /// Everything after `afterSeq` (background sync of a session the app has not opened).
    func items(_ k: SessionKey, afterSeq: Int) async throws -> NewerResponse {
        let data = try await request("GET", sp(k) + "/items", query: ["after_seq": String(afterSeq), "lazy_images": "1"])
        return try await Task.detached(priority: .utility) { try JSON.decoder.decode(NewerResponse.self, from: data) }.value
    }

    /// How the app sent a message when it was not typed in the app itself (docs/protocol/protocol.md, `via`): such a
    /// message never counts as the user's own words for a secret hand-off's Always rule.
    enum MessageVia: String, Codable, Sendable {
        /// Siri, Shortcuts or another App Intent ("Ask Persona", which runs on a locked phone too).
        case intent
        /// The share sheet.
        case share
        /// A card's button that writes the message for the user from what the app read (the Today card's Prep, with a
        /// calendar event's title in it).
        case card
    }

    struct SendBody: Encodable {
        var text: String
        var images: [ImagePayload]?
        var launcher: String?
        var model: String?
        var clientMsgID: String?
        var clearDraft: String?
        var todoID: String?
        var via: MessageVia?
        var waitForEffort: Bool?
        var approvalReplyTo: String?
        enum CodingKeys: String, CodingKey {
            case text, images, launcher, model, via, clientMsgID = "client_msg_id", clearDraft = "clear_draft", todoID = "todo_id"
            case waitForEffort = "wait_for_effort", approvalReplyTo = "approval_reply_to"
        }
    }
    /// launcher / model switch what a headless (assistant) conversation runs on from this message on. clientMsgID
    /// stays the same on a retry (the node types it once); clearDraft is the computer's draft the user agreed to
    /// clear first.
    /// opID: the message's op id (the outbox passes its id, the same as clientMsgID: docs/protocol/protocol.md「Operation receipts」).
    /// via: how the app sent it when not typed in the app itself (Siri, the share sheet).
    /// Returns whether the message waits behind a question on the computer (`waits_on_computer`, #5890): a Claude
    /// outside Persona's terminals took it while a dialog only the computer can answer holds it. Only `sendPending`
    /// (the composer and the outbox flush) tells the user; the other callers drop it.
    @discardableResult
    func send(_ k: SessionKey, text: String, images: [ImagePayload] = [], launcher: String? = nil, model: String? = nil,
              clientMsgID: String? = nil, clearDraft: String? = nil, todoID: String? = nil, via: MessageVia? = nil, opID: UUID? = nil) async throws -> Bool {
        try await sendReply(k, text: text, images: images, launcher: launcher, model: model, clientMsgID: clientMsgID,
                            clearDraft: clearDraft, todoID: todoID, via: via, opID: opID).waitsOnComputer == true
    }

    /// The hub's answer to a message it took: whether it waits behind a question on the computer (#5890), or that it
    /// did not go in and the assistant keeps it to send by itself (`delivered` "later", `undelivered`: #10194).
    nonisolated struct SendReply: Decodable, Sendable {
        var waitsOnComputer: Bool?
        var delivered: String?
        var undelivered: UndeliveredMessage?
        var approvalReply: Bool?
        var interactionID: String?
        var kept: Bool { delivered == "later" }
        enum CodingKeys: String, CodingKey {
            case waitsOnComputer = "waits_on_computer", delivered, undelivered
            case approvalReply = "approval_reply", interactionID = "interaction_id"
        }
    }

    /// `send`, with the whole answer (`SendReply`).
    func sendReply(_ k: SessionKey, text: String, images: [ImagePayload] = [], launcher: String? = nil, model: String? = nil,
                   clientMsgID: String? = nil, clearDraft: String? = nil, todoID: String? = nil, via: MessageVia? = nil, opID: UUID? = nil, holdsAgentMessages: Bool = false, waitForEffort: Bool = false) async throws -> SendReply {
        let data = try await post(sp(k) + "/messages", body: SendBody(text: HubNaming.outbound(text, canonical: personaNaming), images: images.isEmpty ? nil : images, launcher: launcher,
                                                                      model: model, clientMsgID: clientMsgID, clearDraft: clearDraft, todoID: todoID, via: via,
                                                                      waitForEffort: waitForEffort ? true : nil),
                                  timeout: 100, opID: opID, extraHeaders: holdsAgentMessages ? ["X-Persona-Features": UndeliveredMessage.agentHoldFeature] : [:])
        return (try? JSON.decoder.decode(SendReply.self, from: data)) ?? SendReply()
    }

    /// A proposal-only exact owner reply. The event id is the answer attempt's stable UUID; there is no composer/outbox
    /// fallback and no via, quote, image, launcher, model or draft metadata. The hub must confirm the same target.
    func replyOwnerApproval(_ ix: Interaction, phrase: String, opID: UUID) async throws {
        guard ix.ownerApprovalPhrase == phrase else {
            throw APIError(status: 400, message: String(localized: "This card did not advertise that exact reply."))
        }
        let eventID = opID.uuidString.lowercased()
        let data = try await post(sp(ix.sessionKey) + "/messages",
                                  body: SendBody(text: phrase, clientMsgID: eventID, approvalReplyTo: ix.id),
                                  timeout: 100, opID: opID)
        let reply = try JSON.decoder.decode(SendReply.self, from: data)
        guard reply.approvalReply == true, reply.interactionID == ix.id else {
            throw APIError(status: 502, message: String(localized: "The hub did not confirm this exact approval reply."))
        }
    }

    private func keptPath(_ key: SessionKey, _ message: UndeliveredMessage) -> String {
        let sid = Self.pathPart(key.sessionID), id = Self.pathPart(message.id)
        if message.heldForComputer { return "/api/v1/sessions/\(Self.pathPart(key.machineID))/\(sid)/held/\(id)" }
        return "/api/v1/assistant/conversations/\(sid)/undelivered/\(id)"
    }

    /// Retry on a kept message that gave up.
    func retryUndelivered(_ key: SessionKey, _ message: UndeliveredMessage) async throws {
        try await post(keptPath(key, message) + "/retry", body: [String: String]())
    }

    /// Removes the kept copy and stops any further attempt.
    func deleteUndelivered(_ key: SessionKey, _ message: UndeliveredMessage) async throws {
        try await delete(keptPath(key, message))
    }

    private nonisolated static func pathPart(_ s: String) -> String {
        s.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/"))) ?? s
    }
    func interrupt(_ k: SessionKey) async throws { try await post(sp(k) + "/interrupt") }
    func attach(_ k: SessionKey) async throws { try await post(sp(k) + "/attach") }

    // MARK: Device credentials (DeviceCredentials.swift)

    struct NewToken: Decodable, Sendable { var id: String; var token: String; var role: String }
    struct DeviceToken: Decodable, Identifiable, Hashable, Sendable {
        var id: String
        var name: String
        var kind: String
        var role: String
        var createdAt: Date?
        var lastSeen: Date?
        var revokedAt: Date?
        enum CodingKeys: String, CodingKey {
            case id, name, kind, role, createdAt = "created_at", lastSeen = "last_seen", revokedAt = "revoked_at"
        }
    }
    struct DeviceTokens: Decodable, Sendable {
        var tokens: [DeviceToken]
        var current: String?
        /// Per credential, the push way of the device it registered last; nil from an older hub.
        var push: [String: PushRoute]?
    }

    /// Trades this credential for a device's own (kind "watch": answer-only).
    func createToken(name: String, kind: String) async throws -> NewToken {
        try JSON.decoder.decode(NewToken.self, from: try await request("POST", "/api/v1/auth/tokens", body: ["name": name, "kind": kind]))
    }
    func deviceTokens(timeout: TimeInterval = 30) async throws -> DeviceTokens {
        try JSON.decoder.decode(DeviceTokens.self, from: try await request("GET", "/api/v1/auth/tokens", timeout: timeout))
    }
    func revokeToken(_ id: String, timeout: TimeInterval = 30) async throws {
        _ = try await request("DELETE", "/api/v1/auth/tokens/\(id)", timeout: timeout)
    }

    /// A QR code's modules as the hub draws them: rows of "0" / "1".
    struct QRRows: Decodable, Hashable, Sendable { var size: Int; var rows: [String] }
    /// A one-time pairing code (`jp_…`, ten minutes, traded once) for another device (docs/protocol/protocol.md「临时配对」).
    struct PairCode: Decodable, Hashable, Sendable {
        var token: String
        var expiresAt: Date
        // Not `pairURL` (a DEBUG launch switch): property names stay in the Release binary, which release.sh checks.
        var pairLink: String
        /// The QR code of `pairLink`; an older hub leaves it out.
        var qr: QRRows?
        /// `<page>#token=…`, asked for with `web`: opening it signs a browser in.
        var webURL: String?
        var webQR: QRRows?
        enum CodingKeys: String, CodingKey {
            case token, qr, expiresAt = "expires_at", pairLink = "pair_url", webURL = "web_url", webQR = "web_qr"
        }
    }
    /// Settings › Devices › Connect New Device: a signed-in full device asks the hub for a pairing code; `web` is the page address the
    /// browser sign-in link opens.
    func pairCode(web: String?) async throws -> PairCode {
        struct Body: Encodable { var web: String? }
        return try JSON.decoder.decode(PairCode.self, from: try await request("POST", "/api/v1/auth/pair-codes", body: Body(web: web)))
    }

    /// The hub lets a signed-in device add a computer (pair-codes `add` / `machine`, docs/protocol/protocol.md「Computer pairing codes」).
    static let machinesAddFeature = "machines_add"
    /// Machines carry `trust` / `error_code` and take confirm / reject (docs/protocol/protocol.md「Computer trust」).
    static let machineTrustFeature = "machine_trust"

    /// A computer's pairing code (machine › Add Computer): the app code's fields without `qr`, and the computer's own.
    nonisolated struct ComputerCode: Decodable, Hashable, Sendable {
        nonisolated struct Install: Decodable, Hashable, Sendable { var linux: String? }
        var token: String
        var expiresAt: Date
        /// `persona://link?…`: what the computer's Persona Link pairs with.
        var pairLink: String
        /// The machine id the hub gave it (from the name).
        var machine: String
        var name: String?
        /// `pending` / `confirmed`.
        var trust: String?
        /// This request added it (omitted when a pending one of the same name was reused).
        var added: Bool?
        /// The line that pairs an installed Persona Link.
        var command: String?
        /// One line that installs and pairs; only when the hub knows the site it was published from.
        var install: Install?
        /// The manager the computer belongs to now (`add.manager`); nil from a hub before the field.
        var manager: String?
        enum CodingKeys: String, CodingKey {
            case token, machine, name, trust, added, command, install, manager, expiresAt = "expires_at", pairLink = "pair_url"
        }
    }

    /// Adds a computer, waiting to be confirmed (`add`): `name` may be Chinese (40 characters at most), `os` is
    /// mac / linux / windows (which instructions the app shows), `icon` laptop / desktop / server. `manager` is the assistant
    /// the computer is for (its real id, the default's too); nil leaves it to the hub, as a hub before `add.manager` does.
    func computerCode(addNamed name: String, os: String, icon: String, manager: String? = nil) async throws -> ComputerCode {
        struct Add: Encodable { var name: String; var os: String; var icon: String; var manager: String? }
        struct Body: Encodable { var add: Add }
        return try JSON.decoder.decode(ComputerCode.self, from: try await request("POST", "/api/v1/auth/pair-codes",
                                                                                  body: Body(add: Add(name: name, os: os, icon: icon, manager: manager))))
    }

    /// A new code for a computer still waiting to be confirmed (`machine`); a confirmed one is 409 `not_pending`.
    func computerCode(machine id: String) async throws -> ComputerCode {
        struct Body: Encodable { var machine: String }
        return try JSON.decoder.decode(ComputerCode.self, from: try await request("POST", "/api/v1/auth/pair-codes", body: Body(machine: id)))
    }

    /// It's Mine: the computer waiting to be confirmed is confirmed (it gets the keys, it can run agents).
    func confirmMachine(_ id: String) async throws { _ = try await post("/api/v1/machines/\(id)/confirm") }
    /// Not Mine: the hub removes the computer waiting to be confirmed and revokes its credential.
    func rejectMachine(_ id: String) async throws { _ = try await post("/api/v1/machines/\(id)/reject") }

    struct KeysBody: Encodable { var keys: [String]; var text: String }
    func keys(_ k: SessionKey, keys: [String], text: String = "") async throws {
        try await post(sp(k) + "/keys", body: KeysBody(keys: keys, text: text))
    }
    /// `recent` also reaches back into the pane's scrollback (page by asking for more lines); `ansi` keeps the colors
    /// (SGR escapes; an older node sends plain text either way).
    func screen(_ k: SessionKey, lines: Int = 60, recent: Bool = false, ansi: Bool = false) async throws -> ScreenResponse {
        var q = ["lines": String(lines), "source": recent ? "recent" : "visible"]
        if ansi { q["format"] = "ansi" }
        return try await get(sp(k) + "/screen", query: q)
    }
    func close(_ k: SessionKey) async throws { try await post(sp(k) + "/close") }
    /// Closes a headless Persona conversation so its next start uses the configured model (#4489, docs/protocol/protocol.md
    /// 「Going back to the configured model」). Claims nothing: the session's `model_restore` says it is pending.
    func restoreModel(_ k: SessionKey) async throws { try await post(sp(k) + "/restore-model") }
    private struct ForkBody: Encodable { var at_item_id: String }
    private struct Forked: Decodable { var session_id: String }
    /// Branch to New Chat (#2137): a new session with the conversation up to the end of item's turn; its id.
    func fork(_ k: SessionKey, at itemID: String) async throws -> String {
        try await post(sp(k) + "/fork", body: ForkBody(at_item_id: itemID), timeout: 60, as: Forked.self).session_id
    }
    private struct RewindBody: Encodable { var prompt: String; var occurrence: Int; var prompts_after: Int; var restore: String }
    /// Rewind to Here (#2915): the node walks Claude Code's /rewind to before the message (up to 90 s on the node). Its
    /// refusals (dialog_open, input_draft, rewind_busy, rewind_nothing, rewind_mismatch…) are 409 with a message to show.
    func rewind(_ k: SessionKey, prompt: String, occurrence: Int, promptsAfter: Int, restore: String) async throws -> Data {
        try await post(sp(k) + "/rewind", body: RewindBody(prompt: prompt, occurrence: occurrence, prompts_after: promptsAfter, restore: restore),
                       timeout: 110)
    }
    /// Delete Chat: the hub takes it out of every app's list; the transcript stays on its computer.
    func deleteSession(_ k: SessionKey) async throws { try await delete(sp(k)) }

    struct ScreenBefore: Decodable { var text: String; var hasMore: Bool; var atLimit: Bool
        enum CodingKeys: String, CodingKey { case text; case hasMore = "has_more", atLimit = "at_limit" } }
    private struct ScreenBeforeBody: Encodable { var anchor: [String]; var lines: Int; var loaded: Int; var format: String? }
    /// The pane's scrollback just before `anchor` (the first lines already shown, `loaded` lines in all), at most
    /// `lines` lines; `ansi`: with its colors.
    func screen(_ k: SessionKey, before anchor: [String], loaded: Int, lines: Int = 200, ansi: Bool = false) async throws -> ScreenBefore {
        try await post(sp(k) + "/screen/before", body: ScreenBeforeBody(anchor: anchor, lines: lines, loaded: loaded, format: ansi ? "ansi" : nil),
                       as: ScreenBefore.self)
    }

    struct SessionInfo: Decodable, Sendable {
        var sessionID: String
        var transcriptPath: String?
        var cwd: String?
        var model: String?
        var launcher: String?
        var gitBranch: String?
        var version: String?
        var permissionMode: String?
        var effort: String?
        var contextTokens: Int
        var outputTokens: Int
        var compactions: Int
        var turns: Int
        var startedAt: Date?
        var lastActivity: Date?
        /// Nobody gives this session's counts (#8261, `usage_unknown`: Paseo sends no usage for an agent that is not
        /// Claude Code's): Session Info leaves out the tokens, compactions and turns rather than showing zeros.
        var usageUnknown: Bool?
        enum CodingKeys: String, CodingKey {
            case cwd, model, launcher, version, compactions, turns, effort
            case sessionID = "session_id", transcriptPath = "transcript_path", gitBranch = "git_branch",
                 permissionMode = "permission_mode", contextTokens = "context_tokens", outputTokens = "output_tokens",
                 startedAt = "started_at", lastActivity = "last_activity", usageUnknown = "usage_unknown"
        }
    }
    func sessionInfo(_ k: SessionKey) async throws -> SessionInfo { try await get(sp(k) + "/info") }

    /// A session's permission mode changes the hub recorded, oldest first (#10297, docs/protocol/protocol.md「Session
    /// mode changes」): each change made through /modes, and what became of a queued restart into Allow Everything.
    nonisolated struct ModeChange: Decodable, Hashable, Sendable {
        /// "set", "restart_done", "restart_failed" or "restart_dropped".
        var kind: String
        var from: String?
        var to: String?
        /// For "set": "now", or "restart" for a restart into it that waits for the turn to end; empty from an older node.
        var applied: String?
        /// Who asked: "device", "hub" (the hub's own token), "extension".
        var actorKind: String?
        var deviceName: String?
        /// What became of a restart, why: a code ("process_exited", "replaced"…).
        var reason: String?
        var at: Date
        enum CodingKeys: String, CodingKey {
            case kind, from, to, applied, reason, at
            case actorKind = "actor_kind", deviceName = "device_name"
        }
    }
    private struct ModeChanges: Decodable, Sendable { var changes: [ModeChange]? }
    /// Off the main actor (`docs/apple/performance.md` P1): a chat opening reads it.
    func modeChanges(_ k: SessionKey) async throws -> [ModeChange] {
        try await getDecodedOffMain(sp(k) + "/mode-changes", as: ModeChanges.self).changes ?? []
    }

    struct ModesBody: Encodable {
        var permissionMode: String?
        var effort: String?
        /// Claude Code's model (#3119, the web's #2434): the node types `/model <id>` through the guards of a message
        /// and reads the screen back (node/model_switch.go).
        var model: String? = nil
        /// Claude Code's session-only ultracode switch (#3427); never sent with the others (`setUltracode`).
        var ultracode: Bool? = nil
        /// Only takes the resume notice away (`ultracode_lost_at`), the switch untouched.
        var ultracodeDismissLost: Bool? = nil
        var queueEffort: Bool? = nil
        enum CodingKeys: String, CodingKey {
            case effort, model, ultracode
            case permissionMode = "permission_mode", ultracodeDismissLost = "ultracode_dismiss_lost"
            case queueEffort = "queue_effort"
        }
    }
    /// Changes a session's permission mode, effort level and / or model (nil leaves it); a TUI agent takes a second or two.
    @discardableResult
    func setModes(_ k: SessionKey, permissionMode: String? = nil, effort: String? = nil, model: String? = nil, queueEffort: Bool = false) async throws -> String? {
        let body = ModesBody(permissionMode: permissionMode, effort: effort, model: model, queueEffort: queueEffort ? true : nil)
        let data = try await post(sp(k) + "/modes", body: body, timeout: 30)
        struct Answer: Decodable {
            var pendingEffort: String?
            enum CodingKeys: String, CodingKey { case pendingEffort = "pending_effort" }
        }
        return (try? JSON.decoder.decode(Answer.self, from: data))?.pendingEffort
    }

    private struct ServiceTierBody: Encodable {
        var tier: String
        enum CodingKeys: String, CodingKey { case tier = "service_tier" }
    }

    /// POST …/modes {"service_tier": tier}, always alone (docs/protocol/protocol.md "Codex speed settings"). The hub
    /// acknowledges the native value through the session; an HTTP success does not make `tier` the value to draw.
    func setServiceTier(_ k: SessionKey, tier: String) async throws {
        // The hub's native speed call may take 90 seconds; leave it five seconds to answer for itself, as the web does.
        try await post(sp(k) + "/modes", body: ServiceTierBody(tier: tier), timeout: 95)
    }

    /// POST …/modes {"ultracode": on} (docs/protocol/protocol.md "Claude Code ultracode"): its own request, never with
    /// the model or effort. Success says the node took it, maybe queued (`ultracode_pending`), not that it is on: the
    /// session's fields say that. 409 `node_outdated` (the computer's node predates it) or `ultracode_unsupported`.
    func setUltracode(_ k: SessionKey, on: Bool) async throws {
        // The node may wait for its session state after a restart (about 20 s) before it answers.
        try await post(sp(k) + "/modes", body: ModesBody(permissionMode: nil, effort: nil, ultracode: on), timeout: 60)
    }

    /// POST …/modes {"ultracode_dismiss_lost": true}: only takes the resume notice away.
    func dismissUltracodeLost(_ k: SessionKey) async throws {
        try await post(sp(k) + "/modes", body: ModesBody(permissionMode: nil, effort: nil, ultracodeDismissLost: true), timeout: 60)
    }

    struct QueueEdit: Encodable { var keep: [Int] }
    struct QueueResponse: Decodable {
        var queue: [String]
        /// Photos for each returned queue item; a Persona chat includes them so take-back can restore the composer.
        /// An item without photos is `[]` (the node writes `null` for it, #5858).
        var images: [[ImagePayload]]?

        private enum CodingKeys: String, CodingKey { case queue, images }

        /// A Persona chat's queue with photos on some messages only: the node answers `null` for a message without any,
        /// and a strict `[[ImagePayload]]` turned the whole take-back into an error although the hub had done it.
        init(from decoder: any Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            queue = try c.decode([String].self, forKey: .queue)
            images = try c.decodeIfPresent([[ImagePayload]?].self, forKey: .images)?.map { $0 ?? [] }
        }
    }
    /// Pulls the queued input back out of claude and re-queues the indexes in `keep`; returns what was queued.
    func editQueue(_ k: SessionKey, keep: [Int]) async throws -> QueueResponse {
        try await post(sp(k) + "/queue", body: QueueEdit(keep: keep), as: QueueResponse.self)
    }

    /// Gives the agent what waits in its queue now (#6088, Claude Code's own send-now: a command it runs moves to the
    /// background, a reply it writes starts again with the messages). 409 `nothing_queued`, `send_now_unsupported`,
    /// `send_now_unrecognized` (#2467), `send_now_in_doubt`; an older node `node_outdated`. The hub's words for them are
    /// in the reader's language.
    func sendQueuedNow(_ k: SessionKey) async throws {
        try await post(sp(k) + "/queue/send-now", timeout: 30)
    }

    struct AssistantState: Codable, Sendable {
        var machineID: String
        var launcher: String
        var cwd: String
        var conversations: [Session]
        /// What the assistant is told about the user (Settings › Assistant › About Me).
        var notes: String?
        var timezone: String?
        /// The user picked `timezone` by hand (#1855, Android's Time zone without Match phone): devices then leave it. nil
        /// from a hub that was never told, or one without picking.
        var timezonePicked: Bool?
        /// The main conversation's session id (the Persona tab opens it); nil until there is one.
        var main: String?
        /// The computer the hub actually routes the main conversation to. It can differ from `machine_id`, which is
        /// only the default for new work; older hubs omit it and use `machine_id`.
        var mainMachineID: String?
        /// Notify Main-Chat Tasks Only in the Main Chat (#1731): tasks the main conversation starts are quiet (`TodoItem.quiet`). The
        /// value in effect (on by default); nil from a hub without the setting, which hides the switch.
        var mainTasksQuiet: Bool?
        /// Fold Background Rounds (docs/apple/ios.md "Background rounds"): the main conversation folds rounds another
        /// conversation, a scheduled task or a follow-up started into one line. The value in effect; nil from a hub
        /// without the setting, which hides the switch and folds as if on (`backgroundFolded`).
        var mainBackgroundFolded: Bool?
        /// Show Scheduled Task Chats in the sidebar's Recent (#8324): the chats a scheduled task or the background
        /// supervisor started are listed there too. The value in effect, one for all of the user's devices; nil from a
        /// hub without the setting, which hides the switch and keeps them out of Recent, as before.
        var recentTaskChats: Bool?
        /// The role a meeting recording starts with (#2981, docs/assistant/meeting-minutes.md「Attendance fields」): `off`,
        /// `listener` or `participant`; nil from a hub without attending.
        var meetingAttendanceDefault: String?
        /// The side conversations that lead a direction of the owner's work (docs/assistant/unified-entry.md "Leader
        /// conversations"), in the hub's order; nil when there are none or from an older hub. The sidebar shows them as
        /// the Directions group, under the names `assistantLeaders()` gives.
        var leaders: [String]?
        /// The Main Chat Supervises Its Own Tasks (#4927, docs/assistant/unified-entry.md "Background supervisor"): the
        /// value in effect. Off (the default) means the built-in background supervisor follows up the tasks no leader
        /// supervises; nil from a hub without the background supervisor, which hides the switch.
        var mainSupervisesOwnTasks: Bool?
        /// Suggest the next message after replies in open chats (#4696). Off by default; nil from a hub without the
        /// setting, which hides the switch.
        var nextSuggestions: Bool?
        /// Organize knowledge points by topic (#3783, docs/assistant/knowledge.md "Topics"): on by default, so nil reads as on.
        var kbTopics: Bool?
        /// The background supervisor's session id once the hub has made it (read-only); nil until then and from an
        /// older hub.
        var backgroundSupervisor: String?
        /// Hub transcription (#3862 phase 5, docs/assistant/meeting-minutes.md「Choosing the mode」), all nil from a hub
        /// without `meeting_transcription`: the mode a meeting starts with (`device` until changed, or `hub`), the names
        /// and words the transcription should spell right (at most 200 of at most 40 characters), which contribution
        /// transcribes (`""` the built-in voice extension's, else `<extension id>/<contribution id>`), and whether it
        /// can now (read-only).
        var meetingTranscriptionDefault: String?
        var meetingGlossary: [String]?
        var meetingTranscriptionExtension: String?
        var meetingTranscriptionReady: Bool?
        /// The manager that answered (its identity, the default's included): what a write that must name its manager
        /// names (the session-name settings, #7005). nil from an older hub.
        var manager: String?
        // Settings › Assistant Profile › Agent Follow-Up (#7005; the web's Settings › Assistant, "Starting tasks" and
        // "Agent follow-up"): each the value in effect, nil from a hub without it, which hides its row.
        /// Start what the owner asked for in a chat right away (#2559). On by default.
        var startRequestedTasks: Bool?
        /// `prefer-existing` (the default) or `prefer-new`: the workspace a dispatched task prefers.
        var taskWorkspacePreference: String?
        /// Every Agent Persona starts gets a quiet, supervised task (#3617). Off by default.
        var agentTasks: Bool?
        /// Close an Agent's terminal tab when its session ends (#10355). Off by default.
        var closeEndedTerminals: Bool?
        /// Hours an idle Agent Persona started runs before it is stopped, its terminal kept (#4754): 0 is never, at
        /// most 8760; 12 by default.
        var idleCloseHours: Int?
        /// Days a closed session with no open task stays listed before it archives itself (#3618): 0 is never, at most
        /// 3650; 3 by default.
        var archiveClosedDays: Int?
        /// Fix unhelpful session names (#4965). On by default; with the two below.
        var agentAutoNaming: Bool?
        /// Successful automatic renames per session, 1–1000; 1 by default.
        var agentAutoNamingMax: Int?
        /// Hours between regular name updates after new output, 1–8760; 0 (the default) is off.
        var agentAutoNamingIntervalHours: Int?
        var ownerReplyApproval: Bool?
        var persistentOwnerActionCards: Bool?
        var ownerReplyApprovalPhrases: [String]?
        enum CodingKeys: String, CodingKey {
            case launcher, cwd, conversations, timezone, main, leaders, manager
            case machineID = "machine_id", mainMachineID = "main_machine_id", notes = "notes_effective", mainTasksQuiet = "main_tasks_quiet"
            case mainBackgroundFolded = "main_background_folded", recentTaskChats = "recent_task_chats"
            case meetingAttendanceDefault = "meeting_attendance_default", timezonePicked = "timezone_picked"
            case mainSupervisesOwnTasks = "main_supervises_own_tasks", backgroundSupervisor = "background_supervisor"
            case meetingTranscriptionDefault = "meeting_transcription_default", meetingGlossary = "meeting_glossary"
            case meetingTranscriptionExtension = "meeting_transcription_extension"
            case meetingTranscriptionReady = "meeting_transcription_ready"
            case nextSuggestions = "next_suggestions", kbTopics = "kb_topics"
            case startRequestedTasks = "start_requested_tasks", taskWorkspacePreference = "task_workspace_preference"
            case agentTasks = "agent_tasks", idleCloseHours = "idle_close_hours", archiveClosedDays = "archive_closed_days"
            case closeEndedTerminals = "close_ended_terminals"
            case agentAutoNaming = "agent_auto_naming", agentAutoNamingMax = "agent_auto_naming_max"
            case agentAutoNamingIntervalHours = "agent_auto_naming_interval_hours"
            case ownerReplyApproval = "owner_reply_approval", persistentOwnerActionCards = "persistent_owner_action_cards"
            case ownerReplyApprovalPhrases = "owner_reply_approval_phrases"
        }

        /// Whether conversation `id` is the built-in background supervisor (#5083): listed with the scheduled tasks'
        /// conversations, never archived, ended or made a direction. Never the main conversation, as on the hub.
        func isBackgroundSupervisor(_ id: String) -> Bool {
            !id.isEmpty && id == backgroundSupervisor && id != main
        }

        /// Whether the main conversation folds its background rounds: on unless the hub says off (an older hub omits it).
        var backgroundFolded: Bool { mainBackgroundFolded ?? true }

        /// The confirmed main conversation on its actual computer, not merely the default computer for new work.
        func mainKey(group: String = "") -> SessionKey? {
            guard let main, !main.isEmpty else { return nil }
            let route = mainMachineID?.isEmpty == false ? mainMachineID : nil
            return SessionKey(group: group, machineID: route ?? machineID, sessionID: main)
        }
    }
    /// Decoded off the main actor: it holds every conversation, and the app reads it again each time it comes back to
    /// the front (#8324).
    func assistant() async throws -> AssistantState { owned(try await getDecodedOffMain("/api/v1/assistant")) }
    /// A direction as `GET /api/v1/assistant/leaders` lists it (#3519, #3695): its leader conversation's id and the name
    /// the hub gives the direction (the name set, else its project's, else the title without its "MMDD | kind | " prefix).
    struct LeaderInfo: Codable, Sendable, Equatable {
        var id: String
        var name: String?
        var ended: Bool?
    }
    /// Makes a conversation a direction (#4331, `POST /api/v1/assistant/directions/{id}/make`): its work is led from
    /// there from now on.
    func makeDirection(_ id: String) async throws {
        _ = try await request("POST", "/api/v1/assistant/directions/\(id)/make", body: [String: String]())
    }
    /// Ends a direction (#4331, `POST /api/v1/assistant/directions/{id}/end`): its tasks keep their supervisor.
    func endDirection(_ id: String) async throws {
        _ = try await request("POST", "/api/v1/assistant/directions/\(id)/end")
    }
    /// The directions as `GET /api/v1/assistant/leaders` lists them: current ones only (#4331).
    func leaders() async throws -> [LeaderInfo] {
        struct Body: Decodable { var leaders: [LeaderInfo]? }
        let body = try await get("/api/v1/assistant/leaders", as: Body.self)
        return body.leaders ?? []
    }
    static func directionNames(_ list: [LeaderInfo]) -> [String: String] {
        var out: [String: String] = [:]
        for l in list where !(l.ended ?? false) && !l.id.isEmpty {
            if let name = l.name?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty { out[l.id] = name }
        }
        return out
    }
    /// The direction tree (#4352, docs/protocol/protocol.md, `DirectionTree`): the directions with their open to-dos
    /// and Agents, the secretary group last. nil from a hub before it (404): the caller keeps what `leaders()` gave it,
    /// the sidebar keeps its flat rows and the lists offer no grouping by it.
    func directionsTree() async throws -> DirectionTree? {
        do {
            return try await getDecodedOffMain("/api/v1/assistant/directions/tree", as: DirectionTree.self)
        } catch let e as APIError where e.status == 404 {
            return nil
        }
    }
    /// `st` with its conversations stamped with the manager this client named: the hub relays another manager's answer as
    /// it came, without the `manager` its own lists put on each session. Nothing named: the default's, left as they came.
    private func owned(_ st: AssistantState) -> AssistantState {
        var st = st
        st.conversations = st.conversations.map { owned($0) }
        return st
    }
    private func owned(_ s: Session) -> Session {
        guard s.manager == nil, let m = managerID ?? shownManager else { return s }
        var s = s
        s.manager = m
        return s
    }
    /// The health space's conversations (docs/assistant/health.md): `GET /api/v1/assistant?space=health`, the same body with the
    /// health directory as `cwd`. The plain list never holds them.
    func healthConversations() async throws -> [Session] {
        let st = try await get("/api/v1/assistant", query: ["space": Session.healthSpace], as: AssistantState.self)
        return st.conversations.map { s in
            var s = s
            if s.machineID.isEmpty { s.machineID = st.machineID }
            if s.assistantSpace == nil { s.assistantSpace = Session.healthSpace }
            return stamp(s)
        }
    }
    struct AssistantConfigBody: Encodable {
        var machineID: String?
        var launcher: String?
        var timezone: String?
        var notes: String?
        var mainTasksQuiet: Bool?
        var mainBackgroundFolded: Bool?
        var recentTaskChats: Bool?
        var meetingAttendanceDefault: String?
        var mainSupervisesOwnTasks: Bool?
        /// Hub transcription's settings (phase 5); `[]` clears the glossary. `meeting_transcription_ready` is read-only.
        var meetingTranscriptionDefault: String?
        var meetingGlossary: [String]?
        var meetingTranscriptionExtension: String?
        var nextSuggestions: Bool?
        var startRequestedTasks: Bool?
        var taskWorkspacePreference: String?
        var agentTasks: Bool?
        var closeEndedTerminals: Bool?
        var idleCloseHours: Int?
        var archiveClosedDays: Int?
        var agentAutoNaming: Bool?
        var agentAutoNamingMax: Int?
        var agentAutoNamingIntervalHours: Int?
        var kbTopics: Bool?
        var ownerReplyApproval: Bool?
        var persistentOwnerActionCards: Bool?
        var ownerReplyApprovalPhrases: [String]?
        enum CodingKeys: String, CodingKey {
            case launcher, timezone, notes
            case machineID = "machine_id", mainTasksQuiet = "main_tasks_quiet", meetingAttendanceDefault = "meeting_attendance_default"
            case mainBackgroundFolded = "main_background_folded", mainSupervisesOwnTasks = "main_supervises_own_tasks"
            case recentTaskChats = "recent_task_chats"
            case meetingTranscriptionDefault = "meeting_transcription_default", meetingGlossary = "meeting_glossary"
            case meetingTranscriptionExtension = "meeting_transcription_extension"
            case nextSuggestions = "next_suggestions", kbTopics = "kb_topics"
            case startRequestedTasks = "start_requested_tasks", taskWorkspacePreference = "task_workspace_preference"
            case agentTasks = "agent_tasks", idleCloseHours = "idle_close_hours", archiveClosedDays = "archive_closed_days"
            case closeEndedTerminals = "close_ended_terminals"
            case agentAutoNaming = "agent_auto_naming", agentAutoNamingMax = "agent_auto_naming_max"
            case agentAutoNamingIntervalHours = "agent_auto_naming_interval_hours"
            case ownerReplyApproval = "owner_reply_approval", persistentOwnerActionCards = "persistent_owner_action_cards"
            case ownerReplyApprovalPhrases = "owner_reply_approval_phrases"
        }
    }
    /// A sparse write of the assistant's settings: only the fields set are sent (Agent Follow-Up, #7005).
    func setAssistant(_ body: AssistantConfigBody) async throws -> AssistantState {
        owned(try JSON.decoder.decode(AssistantState.self, from: try await request("PUT", "/api/v1/assistant", body: body)))
    }
    func setAssistant(machine: String? = nil, launcher: String? = nil, timezone: String? = nil, notes: String? = nil,
                      mainTasksQuiet: Bool? = nil, mainBackgroundFolded: Bool? = nil,
                      meetingAttendanceDefault: String? = nil, mainSupervisesOwnTasks: Bool? = nil,
                      meetingTranscriptionDefault: String? = nil, meetingGlossary: [String]? = nil,
                      meetingTranscriptionExtension: String? = nil, nextSuggestions: Bool? = nil) async throws -> AssistantState {
        owned(try JSON.decoder.decode(AssistantState.self, from: try await request("PUT", "/api/v1/assistant",
            body: AssistantConfigBody(machineID: machine, launcher: launcher, timezone: timezone, notes: notes, mainTasksQuiet: mainTasksQuiet,
                                      mainBackgroundFolded: mainBackgroundFolded, meetingAttendanceDefault: meetingAttendanceDefault,
                                      mainSupervisesOwnTasks: mainSupervisesOwnTasks,
                                      meetingTranscriptionDefault: meetingTranscriptionDefault, meetingGlossary: meetingGlossary,
                                      meetingTranscriptionExtension: meetingTranscriptionExtension, nextSuggestions: nextSuggestions))))
    }
    /// The Chat tab is open: the hub gets a claude process ready so the first message starts at once. With no launcher
    /// chosen the field is omitted and the hub warms its own default (#4200: the client never sends a made-up name).
    /// A hub with `assistant_readiness` answers with the readiness it has (not probed), or none while it works it out.
    @discardableResult
    func warmAssistant(launcher: String?) async throws -> AssistantReadiness? {
        struct Warm: Decodable { var readiness: AssistantReadiness? }
        var body: [String: String] = [:]
        if let launcher, !launcher.isEmpty { body["launcher"] = launcher }
        let data = try await request("POST", "/api/v1/assistant/warm", body: body)
        return (try? JSON.decoder.decode(Warm.self, from: data))?.readiness
    }

    /// What the main conversation still needs (docs/protocol/protocol.md「Main chat readiness」). `probe`: also try one word on it
    /// (costs a little; only for Try a Message and 「我已经在用 Claude Code」). The hub may spend 100 s on the check
    /// and another 12 s on the node's self-check, so a probe waits 130 s rather than the ordinary 30 s read budget.
    func assistantReadiness(probe: Bool = false) async throws -> AssistantReadiness {
        try JSON.decoder.decode(AssistantReadiness.self, from: try await request("GET", "/api/v1/assistant/readiness",
                                                                                 query: probe ? ["probe": "1"] : [:], timeout: probe ? 130 : 30))
    }

    /// Later: the checklist stops coming up by itself (the card still shows when the readiness gets worse).
    func dismissAssistantReadiness(_ dismissed: Bool = true) async throws {
        _ = try await request("POST", "/api/v1/assistant/readiness/dismiss", body: ["dismissed": dismissed])
    }

    // MARK: Assistant schedule, memory (docs/assistant/assistant.md)

    /// Something the assistant does on its own (docs/assistant/assistant.md「Scheduled tasks」): once (a follow-up it scheduled), every
    /// day at a time (a morning briefing), every N minutes (a patrol), or on a cron expression (hubs with task_cron). Runs go into the conversation it came from, else into the
    /// task's own one (the first run starts it, the next ones continue it), with its own launcher / permission mode /
    /// effort (nil = the assistant's defaults); pushed when done.
    struct ScheduledTask: Codable, Identifiable, Hashable, Sendable {
        enum Repeat: String, Codable, Sendable, CaseIterable { case once, daily, interval, cron }
        var id: String
        var title: String
        var prompt: String
        var repeats: Repeat
        var at: Date?             // once: when; interval: the next run
        var time: String          // daily: "HH:MM" in the phone's time zone
        var weekdays: [Int]       // daily: 1 = Monday … 7 = Sunday; empty = every day
        var everyMinutes: Int     // interval
        /// cron: "minute hour day-of-month month day-of-week" (or @daily…) in the user's time zone; sent back as it is, so the
        /// hub keeps it (the app shows it, the web edits it).
        var cron: String = ""
        var cronText: String?     // read-only: the expression in words in the app's language, nil when the hub has none
        var nextRuns: [Date] = [] // read-only: a cron task's next runs
        var enabled: Bool
        var machineID: String?
        var sessionID: String?    // the conversation it came from, runs go into it; nil = the task's own one
        var runMachineID: String? // read-only: the task's own conversation; nil = the next run starts one
        var runSessionID: String?
        var launcher: String?
        var permissionMode: String?
        var effort: String?
        var includesModelSettings = true
        /// Push When Done (hubs with the task_push feature, docs/assistant/assistant.md「Push after running」): nil or "" every run's answer is pushed,
        /// "asked" only what a run sends with notify_user. Such a task always runs in its own conversation. Sent only when
        /// set: a hub keeps it when left out, and an older hub never gets it.
        var push: String?
        /// When Done, Put It in the Inbox (hubs with the task_inbox feature, docs/assistant/assistant.md「Put it in the Inbox when done」): nil or ""
        /// nothing, "worthy" a row only when a run finds something worth a look, "every" a row after every run. Independent of `push`.
        /// Sent only when set: a hub keeps it when left out, and an older hub never gets it.
        var inbox: String?
        var createdBy: String?    // "assistant": Persona set it up from a conversation
        var lastRun: Date?        // read-only, set by the hub
        var lastSessionID: String?
        var lastStatus: String?   // of the last run: ok / failed / skipped
        var lastError: String?
        var nextRun: Date?
        /// Read-only (#8045): the kinds of task this task's own runs start without the plan card. Only the owner adds to it,
        /// on one of its runs' cards; the editor shows it and removes kinds (`revokeStartGrant`), and never sends it.
        var startGrant: StartGrant?
        enum CodingKeys: String, CodingKey {
            case id, title, prompt, at, time, weekdays, enabled, launcher, effort, push, inbox, startGrant = "start_grant"
            case repeats = "repeat", everyMinutes = "every_minutes", machineID = "machine_id", sessionID = "session_id"
            case permissionMode = "permission_mode", createdBy = "created_by"
            case lastRun = "last_run", lastSessionID = "last_session_id", nextRun = "next_run"
            case lastStatus = "last_status", lastError = "last_error"
            case runMachineID = "run_machine_id", runSessionID = "run_session_id"
            case cron, cronText = "cron_text", nextRuns = "next_runs"
        }
        /// Push When Done is Only Its Alerts: a run's answer is not pushed, only what it sends with notify_user.
        var pushesOnlyAsked: Bool { push == Self.pushAsked }
        static let pushAsked = "asked"
        /// The hub feature that says it takes `push`; without it the editor leaves the setting out.
        static let pushFeature = "task_push"
        /// The hub feature that says it takes `inbox`; without it the editor leaves the setting out.
        static let inboxFeature = "task_inbox"
        static let inboxWorthy = "worthy", inboxEvery = "every"
        /// Where the next run goes when the task isn't continuing the conversation it came from.
        var ownConversation: SessionKey? {
            guard let m = runMachineID, let s = runSessionID else { return nil }
            return SessionKey(machineID: m, sessionID: s)
        }
        init(id: String = "", title: String, prompt: String, repeats: Repeat = .daily, at: Date? = nil, time: String = "09:00",
             weekdays: [Int] = [], everyMinutes: Int = 30, enabled: Bool = true) {
            self.id = id; self.title = title; self.prompt = prompt; self.repeats = repeats; self.at = at; self.time = time
            self.weekdays = weekdays; self.everyMinutes = everyMinutes; self.enabled = enabled
        }
        init(from d: Decoder) throws {
            let c = try d.container(keyedBy: CodingKeys.self)
            func text(_ k: CodingKeys) -> String? { (try? c.decodeIfPresent(String.self, forKey: k)).flatMap { $0?.isEmpty == false ? $0 : nil } }
            func date(_ k: CodingKeys) -> Date? { (try? c.decodeIfPresent(Date.self, forKey: k)).flatMap { $0?.realDate } }
            id = try c.decode(String.self, forKey: .id)
            title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
            prompt = try c.decode(String.self, forKey: .prompt)
            repeats = text(.repeats).flatMap(Repeat.init(rawValue:)) ?? .once
            at = date(.at)
            time = text(.time) ?? "09:00"
            weekdays = try c.decodeIfPresent([Int].self, forKey: .weekdays) ?? []
            everyMinutes = try c.decodeIfPresent(Int.self, forKey: .everyMinutes) ?? 0
            cron = text(.cron) ?? ""
            cronText = text(.cronText)
            nextRuns = ((try? c.decodeIfPresent([Date].self, forKey: .nextRuns)) ?? nil)?.compactMap(\.realDate) ?? []
            enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
            machineID = text(.machineID); sessionID = text(.sessionID)
            launcher = text(.launcher); permissionMode = text(.permissionMode); effort = text(.effort)
            createdBy = text(.createdBy)
            push = text(.push)
            inbox = text(.inbox)
            lastRun = date(.lastRun); lastSessionID = text(.lastSessionID); lastStatus = text(.lastStatus); lastError = text(.lastError)
            nextRun = date(.nextRun)
            runMachineID = text(.runMachineID); runSessionID = text(.runSessionID)
            startGrant = (try? c.decodeIfPresent(StartGrant.self, forKey: .startGrant)) ?? nil
        }
        func encode(to e: Encoder) throws { // only what the user edits; "" = the default
            var c = e.container(keyedBy: CodingKeys.self)
            try c.encode(id, forKey: .id); try c.encode(title, forKey: .title); try c.encode(prompt, forKey: .prompt)
            try c.encode(repeats, forKey: .repeats); try c.encodeIfPresent(at, forKey: .at)
            try c.encode(time, forKey: .time); try c.encode(weekdays, forKey: .weekdays); try c.encode(everyMinutes, forKey: .everyMinutes)
            if !cron.isEmpty { try c.encode(cron, forKey: .cron) } // only a cron task has one: an older hub never gets it
            try c.encode(enabled, forKey: .enabled); try c.encode(sessionID ?? "", forKey: .sessionID)
            try c.encode(permissionMode ?? "", forKey: .permissionMode)
            try c.encodeIfPresent(push, forKey: .push)
            try c.encodeIfPresent(inbox, forKey: .inbox)
            if includesModelSettings {
                try c.encode(launcher ?? "", forKey: .launcher)
                try c.encode(effort ?? "", forKey: .effort)
            }
        }
    }
    /// A scheduled task's own start grant (#8045): the kinds of task its runs start without the plan card, each a trust key
    /// ("machine|folder|read" or "…|edit", as Auto-Start Tasks' kinds), read into `kinds` once, as it is decoded.
    struct StartGrant: Decodable, Hashable, Sendable {
        /// One kind as the page names it: the computer, the folder (it covers the folders under it), whether the tasks only
        /// read. A key whose computer and folder can't be told apart (one of them holds "|") keeps it whole as the folder.
        struct Kind: Hashable, Sendable {
            var key: String
            var machine: String
            var folder: String
            var readonly: Bool

            init(key: String) {
                self.key = key
                let parts = key.components(separatedBy: "|")
                let mode = parts.count > 1 ? parts[parts.count - 1] : ""
                readonly = mode == "read"
                if parts.count == 3, mode == "read" || mode == "edit" {
                    machine = parts[0]; folder = parts[1]
                } else {
                    machine = ""
                    folder = mode == "read" || mode == "edit" ? parts.dropLast().joined(separator: "|") : key
                }
            }
        }
        var kinds: [Kind]
        var grantedAt: Date?
        enum CodingKeys: String, CodingKey { case kinds, grantedAt = "granted_at" }

        init(keys: [String], grantedAt: Date? = nil) {
            kinds = keys.filter { !$0.isEmpty }.map(Kind.init(key:))
            self.grantedAt = grantedAt
        }
        init(from d: Decoder) throws {
            let c = try d.container(keyedBy: CodingKeys.self)
            self.init(keys: (try? c.decodeIfPresent([String].self, forKey: .kinds)) ?? [],
                      grantedAt: (try? c.decodeIfPresent(Date.self, forKey: .grantedAt)) ?? nil)
        }
    }
    struct TasksBody: Decodable { var tasks: [ScheduledTask] }
    struct TaskBody: Codable { var task: ScheduledTask }
    func scheduledTasks() async throws -> [ScheduledTask] { try await get("/api/v1/assistant/tasks", as: TasksBody.self).tasks }
    /// Creates `t` (no id yet) or saves the user's changes to it; returns it as the hub has it.
    func saveTask(_ t: ScheduledTask, includesModelSettings: Bool = true) async throws -> ScheduledTask {
        var t = t
        t.includesModelSettings = includesModelSettings
        let data = t.id.isEmpty ? try await request("POST", "/api/v1/assistant/tasks", body: t)
                                : try await request("PUT", "/api/v1/assistant/tasks/\(t.id)", body: t)
        return try JSON.decoder.decode(TaskBody.self, from: data).task
    }
    func deleteTask(_ id: String) async throws { try await delete("/api/v1/assistant/tasks/\(id)") }
    /// Stops one kind (a trust key) from starting without asking in task `id`'s runs (#8045); returns the task as the hub
    /// then has it.
    func revokeStartGrant(task id: String, kind: String) async throws -> ScheduledTask {
        let data = try await request("DELETE", "/api/v1/assistant/tasks/\(TodosLogic.pathComponent(id))/start-grant", body: ["kind": kind])
        return try JSON.decoder.decode(TaskBody.self, from: data).task
    }

    /// A saved place (Settings › Assistant › Saved Places, #1598): what a reminder by place goes off at when the user says
    /// "home" or "the office". The hub keeps them; the assistant reads them (`list_places`).
    struct SavedPlace: Codable, Identifiable, Hashable, Sendable {
        /// The reminder's circle: 150 m by default, 100–2000 m (the hub and `PlaceTrigger` clamp to the same).
        static let defaultRadius = 150
        static let radiusRange = 100...2000

        var id: String = ""          // "" until the hub has it
        var name: String = ""
        var address: String = ""
        var latitude: Double = 0
        var longitude: Double = 0
        var radiusMeters: Int = SavedPlace.defaultRadius
        enum CodingKeys: String, CodingKey { case id, name, address, latitude, longitude, radiusMeters = "radius_meters" }

        init(id: String = "", name: String = "", address: String = "", latitude: Double = 0, longitude: Double = 0,
             radiusMeters: Int = SavedPlace.defaultRadius) {
            self.id = id; self.name = name; self.address = address
            self.latitude = latitude; self.longitude = longitude; self.radiusMeters = radiusMeters
        }
        init(from d: Decoder) throws {
            let c = try d.container(keyedBy: CodingKeys.self)
            id = try c.decode(String.self, forKey: .id)
            name = try c.decode(String.self, forKey: .name)
            address = try c.decodeIfPresent(String.self, forKey: .address) ?? ""
            latitude = try c.decode(Double.self, forKey: .latitude)
            longitude = try c.decode(Double.self, forKey: .longitude)
            radiusMeters = try c.decodeIfPresent(Int.self, forKey: .radiusMeters) ?? SavedPlace.defaultRadius
        }
    }
    struct PlacesBody: Decodable { var places: [SavedPlace] }
    struct PlaceBody: Decodable { var place: SavedPlace }
    func savedPlaces() async throws -> [SavedPlace] { try await get("/api/v1/assistant/places", as: PlacesBody.self).places }
    /// Adds `p` (no id yet) or saves the user's changes to it; returns it as the hub has it.
    func savePlace(_ p: SavedPlace) async throws -> SavedPlace {
        let data = p.id.isEmpty ? try await request("POST", "/api/v1/assistant/places", body: p)
                                : try await request("PUT", "/api/v1/assistant/places/\(p.id)", body: p)
        return try JSON.decoder.decode(PlaceBody.self, from: data).place
    }
    func deletePlace(_ id: String) async throws { try await delete("/api/v1/assistant/places/\(id)") }
    struct TaskRun: Decodable {
        var sessionID: String
        var machineID: String
        enum CodingKeys: String, CodingKey { case sessionID = "session_id", machineID = "machine_id" }
    }
    /// New Chat Next Time: the task's next run starts a new conversation of its own (the old one stays in Chat).
    func freshTaskConversation(_ id: String) async throws -> ScheduledTask {
        try await post("/api/v1/assistant/tasks/\(id)/fresh", body: [String: String](), as: TaskBody.self).task
    }
    /// Runs a task now instead of at its time; returns the conversation it went to.
    func runTask(_ id: String) async throws -> SessionKey {
        let r = try await post("/api/v1/assistant/tasks/\(id)/run", body: [String: String](), timeout: 130, as: TaskRun.self)
        return SessionKey(group: group, machineID: r.machineID, sessionID: r.sessionID)
    }

    /// One entry of the schedule's history: a routine or follow-up that ran, failed to start, or was skipped.
    struct ScheduleRun: Decodable, Identifiable, Hashable, Sendable {
        var id: String
        var kind: String          // routine / followup
        var refID: String
        var title: String
        var due: Date
        var at: Date
        var status: String        // ok / failed / skipped
        var error: String?
        var manual: Bool?
        var machineID: String?
        var sessionID: String?
        enum CodingKeys: String, CodingKey {
            case id, kind, title, due, at, status, error, manual
            case refID = "ref_id", machineID = "machine_id", sessionID = "session_id"
        }
        var key: SessionKey? {
            guard let machineID, let sessionID, !sessionID.isEmpty else { return nil }
            return SessionKey(machineID: machineID, sessionID: sessionID)
        }
    }
    struct RunsBody: Decodable { var runs: [ScheduleRun] }
    /// Newest first; `ref` limits it to one routine or follow-up.
    func scheduleRuns(ref: String? = nil) async throws -> [ScheduleRun] {
        try await get("/api/v1/assistant/runs", query: ["ref": ref ?? ""], as: RunsBody.self).runs
    }

    /// One file of the assistant's memory (Claude Code auto-memory): frontmatter (name / description / type) + body.
    /// MEMORY.md is the index the assistant keeps itself.
    struct MemoryFile: Decodable, Identifiable, Hashable, Sendable {
        var name: String
        var content: String
        var updatedAt: Date?
        var id: String { name }
        enum CodingKeys: String, CodingKey { case name, content; case updatedAt = "updated_at" }
    }
    struct MemoryBody: Decodable { var files: [MemoryFile] }
    /// One report in the health space's reports/ (docs/assistant/health.md「Checkup reports」): an original, its record, or both.
    struct HealthReport: Decodable, Identifiable, Hashable, Sendable {
        var name: String
        var title: String
        var path: String?
        var recordPath: String?
        var size: Int64
        var modifiedAt: Date?
        var id: String { name }
        enum CodingKeys: String, CodingKey { case name, title, path, size; case recordPath = "record_path", modifiedAt = "modified_at" }
    }
    struct HealthReportsBody: Decodable { var reports: [HealthReport] }
    func healthReports(machine: String) async throws -> [HealthReport] {
        try await get("/api/v1/machines/\(machine)/health/reports", as: HealthReportsBody.self).reports
    }
    /// `name` is a file name: anything may be in it, so it is escaped here (`encodedPath` leaves a path with escapes alone).
    func deleteHealthReport(name: String, machine: String) async throws {
        let escaped = name.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/"))) ?? name
        try await delete("/api/v1/machines/\(machine)/health/reports/\(escaped)")
    }
    /// One "always allow" remembered for an assistant conversation (docs/assistant/assistant.md「Always allow in this session」). The
    /// hub keeps a hash of what was allowed and the tool's name, so the exact command or folder can't be shown.
    struct AlwaysGrant: Decodable, Identifiable, Hashable, Sendable {
        var keyHash: String
        var toolName: String
        var createdAt: Date?
        var id: String { keyHash }
        enum CodingKeys: String, CodingKey { case keyHash = "key_hash", toolName = "tool_name", createdAt = "created_at" }
    }
    struct AlwaysGrantsBody: Decodable { var grants: [AlwaysGrant] }
    struct AlwaysRevoked: Decodable { var restartRequired: Bool?; enum CodingKeys: String, CodingKey { case restartRequired = "restart_required" } }
    private func alwaysPath(_ conversation: String) -> String {
        let escaped = conversation.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed.subtracting(CharacterSet(charactersIn: "/"))) ?? conversation
        return "/api/v1/assistant/conversations/\(escaped)/always"
    }
    func alwaysGrants(conversation: String) async throws -> [AlwaysGrant] {
        try await get(alwaysPath(conversation), as: AlwaysGrantsBody.self).grants
    }
    /// Whether the conversation still has to be restarted: the grant is gone, but the running process could not be reset.
    func revokeAlwaysGrant(hash: String, conversation: String) async throws -> Bool {
        let data = try await request("DELETE", alwaysPath(conversation) + "/\(hash)")
        return try JSON.decoder.decode(AlwaysRevoked.self, from: data).restartRequired ?? false
    }
    /// `space`: nil for the assistant's own memory, `Session.healthSpace` for the health space's (docs/assistant/health.md).
    func assistantMemory(space: String? = nil) async throws -> [MemoryFile] {
        try await get("/api/v1/assistant/memory", query: ["space": space ?? ""], as: MemoryBody.self).files
    }
    func saveMemory(name: String, content: String, space: String? = nil) async throws {
        _ = try await request("PUT", "/api/v1/assistant/memory/\(name)", query: ["space": space ?? ""], body: ["content": content])
    }
    func deleteMemory(name: String, space: String? = nil) async throws {
        _ = try await request("DELETE", "/api/v1/assistant/memory/\(name)", query: ["space": space ?? ""])
    }
    struct NewConversation: Encodable {
        var prompt: String
        var images: [ImagePayload]?
        var launcher: String?
        /// The model picked under `launcher` (#11472); nil: the launcher's own default. Never sent without a launcher.
        var model: String?
        var permissionMode: String?
        var effort: String?
        var main: Bool?
        /// "health": the conversation opens in the health space (docs/assistant/health.md); never with `main`.
        var space: String?
        var via: MessageVia?
        enum CodingKeys: String, CodingKey { case prompt, images, launcher, model, effort, main, space, via; case permissionMode = "permission_mode" }
    }
    /// `main`: the new conversation becomes the main one. `space`: the space it opens in (nil: the assistant's own).
    /// `launcher` / `model`: the new conversation's pick (`AssistantStarterChoice.request`); a model goes only with its launcher.
    /// `via`: how the app sent the prompt when not typed in the app itself (Siri, the share sheet).
    /// `opID`: the first message's op id (`AssistantStart`, hub feature `assistant_start_ops`): the hub starts it once.
    func startConversation(prompt: String, images: [ImagePayload] = [], launcher: String? = nil, model: String? = nil, permissionMode: String? = nil,
                           effort: String? = nil, main: Bool = false, space: String? = nil, via: MessageVia? = nil, opID: UUID? = nil) async throws -> Session {
        let body = NewConversation(prompt: HubNaming.outbound(prompt, canonical: personaNaming), images: images.isEmpty ? nil : images,
                                   launcher: launcher, model: launcher == nil ? nil : model, permissionMode: permissionMode, effort: effort,
                                   main: main ? true : nil, space: space, via: via)
        // The health space is the default manager's (docs/assistant/health.md), whichever the Persona page shows.
        let api = space == nil ? self : showing(nil)
        var s = try JSON.decoder.decode(SessionResponse.self, from: try await api.post("/api/v1/assistant/conversations", body: body, timeout: 110, opID: opID)).session
        if s.assistantSpace == nil { s.assistantSpace = space }
        return stamp(api.owned(s))
    }

    struct ClaudeSetting: Codable, Identifiable, Sendable {
        var key: String
        var title: String
        var detail: String
        var type: String
        var options: [String]?
        var `default`: JSONValue?
        var value: JSONValue?
        var id: String { key }
    }
    struct ClaudeSettingsResponse: Decodable { var settings: [ClaudeSetting] }
    func claudeSettings(machine: String) async throws -> [ClaudeSetting] {
        try await get("/api/v1/machines/\(machine)/claude-settings", as: ClaudeSettingsResponse.self).settings
    }
    /// Codex's options (~/.codex/config.toml), the same shape as Claude's.
    func codexSettings(machine: String) async throws -> [ClaudeSetting] {
        try await get("/api/v1/machines/\(machine)/codex-settings", as: ClaudeSettingsResponse.self).settings
    }
    func setCodexSetting(machine: String, key: String, value: JSONValue) async throws -> [ClaudeSetting] {
        try JSON.decoder.decode(ClaudeSettingsResponse.self, from: try await request("PUT", "/api/v1/machines/\(machine)/codex-settings",
            body: SetSetting(key: key, value: value))).settings
    }
    /// Antigravity's options (~/.gemini/antigravity-cli/settings.json), the same shape as Claude's.
    func agySettings(machine: String) async throws -> [ClaudeSetting] {
        try await get("/api/v1/machines/\(machine)/agy-settings", as: ClaudeSettingsResponse.self).settings
    }
    func setAgySetting(machine: String, key: String, value: JSONValue) async throws -> [ClaudeSetting] {
        try JSON.decoder.decode(ClaudeSettingsResponse.self, from: try await request("PUT", "/api/v1/machines/\(machine)/agy-settings",
            body: SetSetting(key: key, value: value))).settings
    }
    struct SetSetting: Encodable { var key: String; var value: JSONValue }
    func setClaudeSetting(machine: String, key: String, value: JSONValue) async throws -> [ClaudeSetting] {
        try JSON.decoder.decode(ClaudeSettingsResponse.self, from: try await request("PUT", "/api/v1/machines/\(machine)/claude-settings",
            body: SetSetting(key: key, value: value))).settings
    }

    /// One Claude Code config directory of a machine (CLAUDE_CONFIG_DIR), as Settings › Claude Code › Blocked Config Folders lists it.
    struct ClaudeDir: Codable, Identifiable, Sendable, Equatable {
        var path: String
        var isDefault: Bool?
        var blocked: Bool?
        /// The rule that blocks it, as written (a glob, or its own path).
        var blockedBy: String?
        /// "default", "config" (hub.json), "found" (a running claude used it).
        var source: String
        var running: Int
        var sessions: Int?
        var id: String { path }
        enum CodingKeys: String, CodingKey {
            case path, isDefault = "default", blocked, blockedBy = "blocked_by", source, running, sessions
        }
    }
    struct ClaudeDirsState: Codable, Sendable, Equatable {
        var dirs: [ClaudeDir]
        var blocked: [String]
        var home: String?
        /// false: the hub's own record (the machine is offline or its node is older).
        var live: Bool
    }
    struct SetBlockedClaudeDirs: Encodable { var blocked: [String] }
    func claudeDirs(machine: String) async throws -> ClaudeDirsState {
        try await get("/api/v1/machines/\(machine)/claude-dirs", as: ClaudeDirsState.self)
    }
    func setBlockedClaudeDirs(machine: String, blocked: [String]) async throws -> ClaudeDirsState {
        try JSON.decoder.decode(ClaudeDirsState.self, from: try await request("PUT", "/api/v1/machines/\(machine)/claude-dirs/blocked",
            body: SetBlockedClaudeDirs(blocked: blocked)))
    }

    /// Settings › Claude Code › Live Replies (#1309, docs/integrations/claude-integration.md "Live replies (MessageDisplay)"):
    /// whether the Claude sessions Persona starts on the machine send their replies as Claude writes them. On unless
    /// turned off; an older hub has no such route (404).
    struct LiveReplies: Codable, Sendable, Equatable { var on: Bool }
    func claudeLiveReplies(machine: String) async throws -> Bool {
        try await get("/api/v1/machines/\(machine)/claude-live-replies", as: LiveReplies.self).on
    }
    /// What the hub keeps after the change.
    func setClaudeLiveReplies(machine: String, on: Bool) async throws -> Bool {
        try JSON.decoder.decode(LiveReplies.self, from: try await request("PUT", "/api/v1/machines/\(machine)/claude-live-replies",
            body: LiveReplies(on: on))).on
    }

    func interactions() async throws -> [Interaction] {
        let data = try await request("GET", "/api/v1/interactions")
        // An MCP form's fields in their server's order, which the decoded dictionaries don't keep.
        return stamp(Elicitation.withFieldOrders(try JSON.decoder.decode(InteractionsResponse.self, from: data).interactions, from: data))
    }
    @discardableResult
    /// opID: one per answer (the same on every retry of it), so a retry through a hub restart presses nothing twice.
    /// `handoffCard`: a hand-off card's own approval, which says so (`SecretHandoff.feature`) on this request alone.
    func resolve(_ id: String, _ r: Resolution, opID: UUID = UUID(), handoffCard: Bool = false) async throws -> ResolveResult {
        let headers = handoffCard ? ["X-Persona-Features": SecretHandoff.feature] : [:]
        return try JSON.decoder.decode(ResolveResult.self, from: try await post("/api/v1/interactions/\(id)/resolve", body: r, opID: opID,
                                                                                 extraHeaders: headers))
    }

    /// One session's cards and how they ended (GET /api/v1/interactions/history), newest first; a row this app can't read
    /// is left out. `manager`: the session's (the hub asks the manager a request names, else the default one).
    func interactionHistory(machine: String, session: String, manager: String? = nil,
                            limit: Int = Delegated.historyLimit) async throws -> [InteractionRecord] {
        struct Body: Decodable { var interactions: [Lossy<InteractionRecord>]? }
        let rows = try await get("/api/v1/interactions/history",
                                 query: ["machine_id": machine, "session_id": session, "limit": String(limit), "manager": manager ?? ""],
                                 as: Body.self)
        return (rows.interactions ?? []).compactMap(\.value).map {
            var r = $0
            r.interaction.group = group
            r.interaction.manager = r.interaction.manager ?? manager
            return r
        }
    }

    /// Change Answer (#544): the user's answers to a question the assistant answered go to the agent as a message.
    /// `manager`: the card's; the question is no longer pending, so the hub routes by the manager named.
    func reanswer(_ id: String, answers: [String: String], manager: String? = nil) async throws {
        _ = try await request("POST", "/api/v1/interactions/\(id)/reanswer", query: ["manager": manager ?? ""],
                              body: ["answers": answers])
    }

    /// What the hub recorded for an op (`GET /api/v1/ops/{id}`): pending / done / failed / unknown; nil when it has no
    /// such op (an older hub, or one past its 30 days).
    func opState(_ id: UUID) async throws -> String? {
        struct Op: Decodable { var state: String? }
        do {
            return try await get("/api/v1/ops/\(id.uuidString.lowercased())", as: Op.self).state
        } catch let e as APIError where e.status == 404 {
            return nil
        }
    }

    struct DeviceBody: Encodable {
        var deviceID: String
        var name: String
        var kind: String
        var apnsToken: String?
        var apnsEnv: String?
        var topic: String?
        /// Push relay (docs/operations/push-relay.md): this phone's public key, the relay and the grant it gave.
        var pushKey: String?
        var relayURL: String?
        var relayGrant: String?
        /// Why the relay gave no grant, in its words ("403 topic not served", a redirect, a network error): the hub
        /// says it beside this device in Settings › Devices and `hub doctor` (#5998).
        var relayError: String?
        /// Urgent calls (docs/assistant/urgent-call.md): the PushKit token while calls are on, the relay's grant for it
        /// (topic `<bundle id>.voip`) and why there is none. An older hub ignores them.
        var voipToken: String?
        var voipGrant: String?
        var voipError: String?
        /// What this app understands beyond the first version's pushes: `withdraw`, a silent push naming the cards that
        /// are over, whose notifications it then removes (#4349). An older hub ignores it.
        var features: [String]?
        /// The interface language pushes to this device use (docs/protocol/i18n.md「Protocol: telling hub the language」).
        var locale: String? = LanguageStore.shared.launched
        enum CodingKeys: String, CodingKey {
            case name, kind, topic, locale, features
            case deviceID = "device_id", apnsToken = "apns_token", apnsEnv = "apns_env"
            case pushKey = "push_key", relayURL = "relay_url", relayGrant = "relay_grant", relayError = "relay_error"
            case voipToken = "voip_token", voipGrant = "voip_grant", voipError = "voip_error"
        }
    }
    /// Registers this device's push token; the hub answers which way its pushes take (nil: an older hub, which does
    /// not say).
    func register(device: DeviceBody) async throws -> PushRoute? {
        try await registerRoutes(device: device).push
    }

    /// What a registration answers: the way pushes reach the device, and the way an urgent call's ring does (nil: an
    /// older hub, which does not say).
    struct DeviceRoutes: Decodable, Sendable {
        var push: PushRoute?
        var call: PushRoute?
    }

    func registerRoutes(device: DeviceBody) async throws -> DeviceRoutes {
        try await post("/api/v1/devices", body: device, as: DeviceRoutes.self)
    }

    /// The way a hub's pushes reach a device (docs/operations/push-relay.md "Which way a device is pushed").
    struct PushRoute: Codable, Hashable, Sendable {
        /// direct: the hub's own APNs key; relay: the relay the device registered with; none: no way.
        var route: String
        /// none: no_token / relay_failed / no_relay; direct or relay: push_failed when the last push did not go.
        var reason: String?
        /// APNs', the relay's or the phone's words.
        var detail: String?
        var failedAt: Date?
        enum CodingKeys: String, CodingKey { case route, reason, detail, failedAt = "failed_at" }

        var reaches: Bool { route == "direct" || route == "relay" }
    }

    /// The Fleet Live Activity's tokens, or that it ended (hub: liveactivity.go).
    struct LiveActivityReport: Encodable {
        var activityID: String?
        var pushToken: String?
        var pushToStartToken: String?
        var ended: Bool?
        enum CodingKeys: String, CodingKey {
            case ended
            case activityID = "activity_id", pushToken = "push_token", pushToStartToken = "push_to_start_token"
        }
    }
    func reportLiveActivity(device: String, _ body: LiveActivityReport) async throws {
        try await post("/api/v1/devices/\(device)/live-activity", body: body)
    }
    struct FleetResponse: Decodable { var fleet: FleetState }
    func fleet() async throws -> FleetState { try await get("/api/v1/fleet", as: FleetResponse.self).fleet }

    /// Which pushes this device wants (hub: notify.go).
    func notifyPrefs(device: String) async throws -> NotifyPrefs { try await get("/api/v1/devices/\(device)/prefs") }
    /// Phone Muted alone (the PUT merges): nothing else this device chose is written back with it.
    func setMuteWhileDesktop(device: String, _ on: Bool) async throws -> NotifyPrefs {
        try JSON.decoder.decode(NotifyPrefs.self, from: try await request("PUT", "/api/v1/devices/\(device)/prefs", body: MuteWhileDesktopPut(on: on)))
    }
    /// The persona the hub keeps for this device's Live Activity; nil when it has none (an older hub never has one).
    func fleetLook(device: String) async throws -> FleetLook? {
        struct Prefs: Decodable { var look: FleetLook? }
        return try await get("/api/v1/devices/\(device)/prefs", as: Prefs.self).look
    }
    /// The persona this device's Live Activity draws, alone (the PUT merges): the hub echoes it in the device's pushes.
    func setFleetLook(device: String, _ look: FleetLook) async throws {
        _ = try await request("PUT", "/api/v1/devices/\(device)/prefs", body: FleetLookPut(look: look))
    }
    /// Today's tokens by the hour (`GET /api/v1/usage`'s `hourly`, #5721): 24 numbers from midnight; empty from an older
    /// hub, which has no such field.
    func usageHourly() async throws -> [Int] {
        struct Body: Decodable {
            struct Usage: Decodable { var hourly: [Int]? }
            var usage: Usage
        }
        return try await get("/api/v1/usage", as: Body.self).usage.hourly ?? []
    }
    /// Every computer's tokens on each of the last `days` days (1–371) in the user's zone, oldest first, the last today
    /// (`GET /api/v1/usage/days`, docs/server/machines.md "Token usage history"): the assistant's on the Persona page.
    /// 404 from a hub before it.
    func usageDays(_ days: Int) async throws -> UsageDays {
        struct Body: Decodable { var usage: UsageDays }
        return try await currentAssistantSettings().get("/api/v1/usage/days", query: ["days": String(days)], as: Body.self).usage
    }
    /// The last `days` days (1–7) by the hour, each computer's share and the total (`GET /api/v1/usage/hours`).
    func usageHours(_ days: Int = 7) async throws -> UsageHours {
        struct Body: Decodable { var usage: UsageHours }
        return try await currentAssistantSettings().get("/api/v1/usage/hours", query: ["days": String(days)], as: Body.self).usage
    }
    /// Whether this device has the Home Screen widgets, alone: while its app is not open, the hub wakes it to write them
    /// again when the agents' counts change (#5721). An older hub keeps the field and does nothing with it.
    func setHasWidgets(device: String, _ has: Bool) async throws {
        _ = try await request("PUT", "/api/v1/devices/\(device)/prefs", body: WidgetsPut(widgets: has))
    }
    func setNotifyPrefs(device: String, _ prefs: NotifyPrefs) async throws -> NotifyPrefs {
        try JSON.decoder.decode(NotifyPrefs.self, from: try await request("PUT", "/api/v1/devices/\(device)/prefs", body: prefs))
    }
    /// The managers this device mutes, alone (the PUT merges): the hub keeps only those of its roster.
    func setMutedManagers(device: String, _ ids: [String]) async throws -> NotifyPrefs {
        try JSON.decoder.decode(NotifyPrefs.self, from: try await request("PUT", "/api/v1/devices/\(device)/prefs", body: MutedManagersPut(ids: ids)))
    }
    /// Quiet hours alone (the PUT merges); the hub refuses bad times or an unknown zone (400).
    func setQuietHours(device: String, _ quiet: QuietHours) async throws -> NotifyPrefs {
        try JSON.decoder.decode(NotifyPrefs.self, from: try await request("PUT", "/api/v1/devices/\(device)/prefs", body: QuietHoursPut(quiet: quiet)))
    }
}

nonisolated struct AnyEncodable: Encodable {
    let value: any Encodable
    init(_ value: any Encodable) { self.value = value }
    func encode(to encoder: Encoder) throws { try value.encode(to: encoder) }
}

extension Error {
    /// A task or request that was cancelled (the screen went away, a newer load replaced it). iOS words it "Canceled";
    /// it is never something to show the user.
    nonisolated var isCancellation: Bool {
        if self is CancellationError { return true }
        if let u = self as? URLError, u.code == .cancelled { return true }
        let ns = self as NSError
        return ns.domain == NSURLErrorDomain && ns.code == NSURLErrorCancelled
    }

    /// A request that ran out of time: on the phone (URLError.timedOut) or behind the hub (a gateway timeout, a node
    /// call past its deadline). Not shown (2026-10-29 user request): the next poll, push or retry takes over.
    nonisolated var isTimeout: Bool {
        if let u = self as? URLError, u.code == .timedOut { return true }
        let ns = self as NSError
        if ns.domain == NSURLErrorDomain && ns.code == NSURLErrorTimedOut { return true }
        guard let api = self as? APIError else { return false }
        if api.code == "timeout" { return true }
        if api.businessMessage != nil { return false }
        return api.status == 504 || APIError.isTimeoutText(api.message)
    }

    /// A history read the node answered as gone, not a route-level or retryable failure.
    nonisolated var isHistoryNotFound: Bool {
        guard let api = self as? APIError else { return false }
        return api.status == 404 && api.code == "not_found"
    }

    /// The request never got an answer because the phone has no network or cannot reach the hub (it may still have
    /// arrived when the link broke midway: networkConnectionLost). The title already says Waiting for Network / Connecting….
    nonisolated var isOffline: Bool {
        let ns = self as NSError
        guard ns.domain == NSURLErrorDomain else { return false }
        return Self.offlineCodes.contains(ns.code)
    }

    nonisolated private static var offlineCodes: Set<Int> {
        [NSURLErrorNotConnectedToInternet, NSURLErrorNetworkConnectionLost, NSURLErrorCannotConnectToHost,
         NSURLErrorCannotFindHost, NSURLErrorDNSLookupFailed, NSURLErrorDataNotAllowed, NSURLErrorInternationalRoamingOff,
         NSURLErrorCallIsActive, NSURLErrorSecureConnectionFailed]
    }

    /// Nothing for the user to act on in a message: a cancellation, a timeout, or no network (2026-10-29: the title
    /// says it, a toast or a red line per failed request was noise).
    nonisolated var isQuiet: Bool { isCancellation || isTimeout || isOffline }

    /// The message to show, or nil for a cancellation, a timeout or no network.
    nonisolated var shown: String? { isQuiet ? nil : localizedDescription }
}

/// Spaces ("machine/space name") and agents ("machine/session") kept at the top of Develop.
nonisolated struct Pins: Codable, Hashable, Sendable {
    var spaces: [String] = []
    var sessions: [String] = []
    /// Mutes only: agents that keep notifications although their space is muted.
    var allowed: [String] = []

    init() {}

    /// Mute rule (the hub's too): the agent is muted itself, or its space is unless the agent was turned back on.
    func mutes(session: String, space: String?) -> Bool {
        if sessions.contains(session) { return true }
        if allowed.contains(session) { return false }
        return space.map(spaces.contains) ?? false
    }

    /// Flips one agent's notifications on top of its space's setting.
    func togglingMute(session: String, space: String?) -> Pins {
        let wasMuted = mutes(session: session, space: space)
        let spaceMuted = space.map(spaces.contains) ?? false
        var p = self
        p.sessions.removeAll { $0 == session }
        p.allowed.removeAll { $0 == session }
        if !wasMuted { p.sessions.insert(session, at: 0) } else if spaceMuted { p.allowed.insert(session, at: 0) }
        return p
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        spaces = try c.decodeIfPresent([String].self, forKey: .spaces) ?? []
        sessions = try c.decodeIfPresent([String].self, forKey: .sessions) ?? []
        allowed = try c.decodeIfPresent([String].self, forKey: .allowed) ?? []
    }
}

extension APIClient {
    func pins() async throws -> Pins { try await get("/api/v1/pins") }
    /// Muted spaces / agents: no push notifications, shown quietly (same shape as pins).
    func mutes() async throws -> Pins { try await get("/api/v1/mutes") }
    @discardableResult
    func setMutes(_ p: Pins) async throws -> Pins {
        try JSON.decoder.decode(Pins.self, from: try await request("PUT", "/api/v1/mutes", body: p))
    }
    @discardableResult
    func setPins(_ p: Pins) async throws -> Pins {
        try JSON.decoder.decode(Pins.self, from: try await request("PUT", "/api/v1/pins", body: p))
    }
}

extension APIClient {
    /// Renames a herdr workspace; the hub carries pins / mutes over to the new name.
    func renameSpace(machine: String, workspaceID: String, name: String) async throws {
        try await post("/api/v1/machines/\(machine)/spaces/\(workspaceID)/rename", body: ["name": name])
    }

    nonisolated struct RenameBody: Encodable, Sendable {
        var tabName: String?
        var title: String?
        enum CodingKeys: String, CodingKey { case title; case tabName = "tab_name" }
    }
    /// Archives an assistant conversation (out of the sidebar, under Archived) or brings it back.
    func archiveConversation(_ id: String, archived: Bool) async throws {
        try await post("/api/v1/assistant/conversations/\(id)/" + (archived ? "archive" : "unarchive"), body: [String: String]())
    }

    /// Renames the session's herdr tab and / or the Claude session (`/rename`).
    func renameSession(_ k: SessionKey, tabName: String?, title: String?) async throws {
        try await post("/api/v1/sessions/\(k.machineID)/\(k.sessionID)/rename", body: RenameBody(tabName: tabName, title: title))
    }
}


extension Date {
    /// nil for Go's zero time and other placeholder dates the hub sends for "never".
    nonisolated var realDate: Date? { timeIntervalSince1970 > 946_684_800 ? self : nil }
}

private nonisolated final class DownloadProgress: HubRedirects, URLSessionDownloadDelegate, @unchecked Sendable {
    let report: @Sendable (Int64, Int64) -> Void
    init(_ report: @escaping @Sendable (Int64, Int64) -> Void) { self.report = report }
    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {}
    func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData bytesWritten: Int64,
                    totalBytesWritten: Int64, totalBytesExpectedToWrite: Int64) {
        report(totalBytesWritten, totalBytesExpectedToWrite)
    }
}

private nonisolated final class UploadProgress: HubRedirects, @unchecked Sendable {
    let report: @Sendable (Int64) -> Void
    init(_ report: @escaping @Sendable (Int64) -> Void) { self.report = report }
    func urlSession(_ session: URLSession, task: URLSessionTask, didSendBodyData bytesSent: Int64, totalBytesSent: Int64,
                    totalBytesExpectedToSend: Int64) {
        report(totalBytesSent)
    }
}
