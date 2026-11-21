import Foundation
import Observation
import os
import PersonaWire

/// One message of a terminal stream (docs/protocol/protocol.md「Terminal stream」).
nonisolated enum TerminalEvent: Sendable {
    /// VT bytes for a cols × rows grid; a full one is the whole screen.
    case frame(epoch: String, seq: Int, full: Bool, cols: Int, rows: Int, data: Data)
    /// The hub dropped the stream; unsupported: read the screen instead of subscribing again.
    case ended(String, unsupported: Bool)
    /// The WebSocket connected again: subscribe again (continuing from the last frame).
    case reconnected
    /// How driving the terminal at the phone's size went (docs/integrations/terminal.md): active (cols × rows), busy (another
    /// program holds it: ask before taking over), taken / lost (back to watching), unsupported (only watching).
    case control(state: String, cols: Int?, rows: Int?, why: String?)
}


nonisolated enum ServerEvent: Sendable {
    /// features: what the hub supports beyond the first protocol (proto.Features); which hub it is and its group
    /// (docs/archive/operations/plan-hub-mesh.md 2.6); its build and protocol (docs/operations/hub-update.md).
    case hello(HubHello)
    case machines([Machine])
    case sessions(machineID: String, [Session])
    /// What changed in a machine's list since the last (a hub that has `sessions_delta`, asked for on connect): the
    /// sessions new or different, whole, and the ids gone (docs/protocol/protocol.md「Session deltas」).
    case sessionsDelta(machineID: String, upsert: [Session], remove: [String])
    /// A lightweight hint, never an authoritative row checkpoint. `changes`: which rows changed since which revision,
    /// when the hub can say (`SessionListChanges`).
    case sessionsList(revision: String, changes: SessionListChanges? = nil)
    /// hasMore is nil when the event cannot know (a catch-up after since_seq): only pages from the end tell. moreAfter:
    /// the hub capped a catch-up, more came after these.
    case items(SessionKey, [Item], backlog: Bool, hasMore: Bool?, span: ItemSpan, moreAfter: Bool)
    case screen(SessionKey, text: String, status: SessionStatus)
    case delta(SessionKey, text: String)
    /// Claude's current status line; nil clears it on recovery, turn end or unwatch.
    case live(SessionKey, SessionLive?)
    case interaction(Interaction)
    /// assistant: what the assistant answered and why, when it answered the question for the user (#544).
    case interactionResolved(String, assistant: AssistantAnswer?)
    /// The user changed the assistant's answer, on this device or another: only the questions that changed (#544).
    case interactionReanswered(id: String, answers: [String: String])
    case toolCall(callID: String, name: String, args: JSONValue)
    case hookEvent(SessionKey, event: String, message: String?, lastText: String?)
    /// Transcripts that grew on a machine (session id → rev): pull them before they are opened.
    case revs(machineID: String, [String: Int64])
    /// Feishu conversations with new or changed messages (the Lark gateway's realtime events, relayed by the hub); account:
    /// the Feishu account they are in when it is not the hub's own (`ol_account`).
    case lark(feedIDs: [String], settingsChanged: Bool, account: Int?)
    /// A messaging source's changes (#2138, docs/extensions/extension-messaging.md「Live events and the Inbox」): its conversations, of `account` (nil: the
    /// default one).
    case messaging(source: String, conversations: [String], settingsChanged: Bool, account: String?)
    /// Personal agent sessions (the gateway's agent-bridge, hub/agents.go) that changed: `<source>:<sessionKey>` ids, empty
    /// for "something changed, re-pull the list".
    case agents(sessionIDs: [String])
    /// The unified inbox changed (hub/inbox.go): pull it.
    case inbox
    /// The Fleet Live Activity's content changed (hub/liveactivity.go).
    case fleet(FleetState)
    /// The hub updating itself: checking, downloading, waiting for idle, restarting… (docs/operations/hub-update.md).
    case hubUpdate(HubUpdateStatus)
    /// An extension install / upgrade / removal moved on (#3307): the whole operation as it stands. `manager`: the
    /// manager whose extension it is, as the event says (nil: the hub's own assistant, docs/assistant/managers.md
    /// "Clients"); the same for the two below.
    case extensionOperation(ExtensionOperation, manager: String?)
    /// An extension's sign-in changed (#4507): signed in, out, expired; nil, it no longer signs in.
    case extensionAuth(extension: String, ExtensionAuth?, manager: String?)
    /// The desktop apps connected to the hub now (sent on connect and whenever one comes or goes).
    case desktops([DesktopDevice])
    /// Tasks from an assistant conversation (docs/assistant/unified-entry.md): the todos changed (pull them), a conversation's
    /// feed grew, a plan changed.
    case todos
    /// The user's saved prompts changed (#7329, #9436): read `GET /api/v1/assistant/saved-prompts` again. `manager`: the
    /// assistant whose list it is when not the default one; only the shown assistant's counts.
    case savedPrompts(manager: String?)
    /// An invalidation for the assistant's starred snapshots. The event does not carry saved text.
    case starredMessages(manager: String?)
    /// The personas the hub offers changed, e.g. a tiered look moved to another tier (docs/apple/persona.md): list them again.
    case personas
    /// The avatar chosen on the hub changed (another device chose one, or the hub went back to the default when its
    /// package or extension went; docs/apple/persona.md): `effective` is the one to show, `id` the one chosen.
    case look(id: String?, effective: String?)
    /// The hub's roster of managers changed (docs/assistant/managers.md): Settings › Managers reads it again. Sent only to a
    /// socket that asked for feature `managers` (`queryItems`).
    case managers
    /// The direction tree changed on the hub (#4352): a to-do, an Agent's session, a direction or its name. No payload:
    /// the clients read `GET /api/v1/assistant/directions/tree` again, and on every (re)connect.
    case directions
    case assistantFeed(FeedItem)
    case assistantView(ExtensionAssistantView)
    case assistantViewAction(conversation: String, action: ExtensionViewAction)
    case assistantViewBlock(conversation: String, key: String)
    case taskPlan(TaskPlan)
    case terminal(SessionKey, TerminalEvent)
    /// Folders a Files / Git pane watches (`fs_watch`) changed on the computer, or the Git status of `cwd` did (#2439,
    /// docs/protocol/protocol.md「File auto-refresh」).
    case fsChanged(machineID: String, cwd: String, dirs: [String], git: Bool)
    /// A plugin changed the badge on one of its main-navigation entries (docs/extensions/extensions.md「UI slots」 badges): nil clears it.
    case pluginBadge(plugin: String, id: String, badge: ExtensionBadge?, manager: String?)
    /// An extension's data changed (`ui.invalidate`, docs/extensions/extensions.md「ui.invalidate」): its interface `id` (nil: all of
    /// them) is drawn again where it is shown.
    case extensionInvalidate(extension: String, id: String?)
    /// A meeting's conversation changed (#3862, docs/assistant/meeting-minutes.md「The `meeting_live` event」): content-free,
    /// only the parts that changed (an open set) and their revisions; the app reads the newer parts with `GET …/live?parts=`.
    case meetingLive(recordingID: String, changed: [String], revs: [String: Int64])
    /// An urgent call moved on (docs/assistant/urgent-call.md): a phone still ringing for one answered or settled
    /// elsewhere stops.
    case urgentCall(id: String, state: String)
    case error(SessionKey?, String)

    private struct EventType: Decodable { var type: String }
    private static let subscriptionType = Data(#""type":"subscription""#.utf8)

    private struct Envelope: Decodable {
        var type: String
        var machineID: String?
        var sessionID: String?
        var machines: [Machine]?
        var sessions: [Session]?
        var live: SessionLive?
        var upsert: [Session]?
        var remove: [String]?
        var revision: String?
        var items: [Item]?
        var backlog: Bool?
        var hasMore: Bool?
        var moreAfter: Bool?
        var history: String?
        var fromSeq: Int?
        var toSeq: Int?
        var text: String?
        var status: SessionStatus?
        var interaction: Interaction?
        var id: String?
        var callID: String?
        var name: String?
        var args: JSONValue?
        var event: String?
        var message: String?
        var lastAssistantMessage: String?
        var error: String?
        var revs: [String: Int64]?
        var feedIDs: [String]?
        var settingsChanged: Bool?
        var olAccount: Int?
        /// messaging's.
        var source: String?
        var conversations: [String]?
        /// messaging's account is a string; a lark event carries a number here (its `ol_account` is what it means), so
        /// either decodes: a String here dropped every lark event of a second account.
        var account: JSONValue?
        var sessionIDs: [String]?
        var fleet: FleetState?
        var desktops: [DesktopDevice]?
        var features: [String]?
        var channel: String?
        var build: Int64?
        var protocolVersion: Int?
        var minAppBuild: Int?
        var publicURL: String?
        var update: HubUpdateStatus?
        var operation: ExtensionOperation?
        var auth: LenientAuth?
        var epoch: String?
        var seq: Int?
        var full: Bool?
        var cols: Int?
        var rows: Int?
        var data: String?
        var unsupported: Bool?
        var state: String?
        var hubID: String?
        var hubName: String?
        var mesh: MeshField?
        var item: FeedItem?
        var view: ExtensionAssistantView?
        /// Keep this raw so another event's optional action string cannot break its envelope decoding.
        var action: JSONValue?
        var key: String?
        var conversation: String?
        var plan: TaskPlan?
        /// look's: the avatar to show (`id` is the one chosen).
        var effective: String?
        var plugin: String?
        /// extension_ui's (plugin_ui says `plugin`).
        var extensionID: String?
        var invalidate: Bool?
        var badge: ExtensionBadge?
        var cwd: String?
        var dirs: [String]?
        var git: Bool?
        /// interaction_resolved's, for a question the assistant answered; raw so no other event's keys can break the envelope.
        var answeredBy: JSONValue?
        var reason: JSONValue?
        var answers: JSONValue?
        /// interaction_reanswered's.
        var resolution: JSONValue?
        /// The manager a manager's own message is from, when not the hub's own assistant (docs/protocol/protocol.md
        /// "Messages to the App"); raw, so another event's `manager` of another shape cannot break the envelope.
        var manager: JSONValue?
        /// meeting_live's.
        var recordingID: String?
        /// meeting_live's list of parts, and the ids a `sessions_list` hint names; raw, since the shapes differ.
        var changed: JSONValue?
        /// A `sessions_list` hint's starting revision; raw, so another message's `since` of another shape cannot break it.
        var since: JSONValue?

        enum CodingKeys: String, CodingKey {
            case type, machines, sessions, live, upsert, remove, revision, items, backlog, text, status, interaction, id, name, args, event, message, error, revs, fleet, history
            case features, epoch, seq, full, cols, rows, data, unsupported, state, desktops, mesh, channel, build, update, item, view, action, key, conversation, plan
            case plugin, badge, invalidate, cwd, dirs, git, operation, auth, source, conversations, account, reason, answers, resolution, manager
            case effective, changed, since
            case answeredBy = "answered_by", recordingID = "recording_id"
            case extensionID = "extension"
            case hubID = "hub_id", hubName = "hub_name", protocolVersion = "protocol", minAppBuild = "min_app_build", publicURL = "public_url"
            case settingsChanged = "settings_changed", olAccount = "ol_account"
            case machineID = "machine_id", sessionID = "session_id", hasMore = "has_more", callID = "call_id",
                 moreAfter = "more_after", fromSeq = "from_seq", toSeq = "to_seq",
                 lastAssistantMessage = "last_assistant_message", feedIDs = "feed_ids", sessionIDs = "session_ids"
        }
    }

    /// An `extension_auth`'s sign-in; anything else under `auth` (another event's) does not lose the event.
    private struct LenientAuth: Decodable {
        var value: ExtensionAuth?
        init(from decoder: Decoder) throws { value = try? ExtensionAuth(from: decoder) }
    }

    /// The hello's `mesh`; anything else there is no mesh rather than a lost hello.
    private struct MeshField: Decodable {
        var hubs: [MeshHub]?
        enum CodingKeys: String, CodingKey { case hubs }
        init(from decoder: Decoder) throws {
            hubs = (try? decoder.container(keyedBy: CodingKeys.self)).flatMap { try? $0.decodeIfPresent([MeshHub].self, forKey: .hubs) }
        }
    }

    static func decode(_ data: Data) -> ServerEvent? {
        // A source subscription was saved, ran or was deleted (#2041): its pages are drawn again, as an invalidate of theirs.
        // Read before the envelope, whose `revision` is a string where this event's is a number; the hub's compact JSON is
        // looked for first, so every other message skips the extra decode.
        if data.range(of: subscriptionType) != nil, (try? JSON.decoder.decode(EventType.self, from: data))?.type == "subscription" {
            return .extensionInvalidate(extension: SubscriptionsUI.id, id: nil)
        }
        guard let e = try? JSON.decoder.decode(Envelope.self, from: data) else { return nil }
        let key = e.machineID.flatMap { m in e.sessionID.map { SessionKey(machineID: m, sessionID: $0) } }
        switch e.type {
        case "hello": return .hello(HubHello(features: e.features ?? [], hubID: e.hubID, hubName: e.hubName, mesh: e.mesh?.hubs,
                                             channel: e.channel, build: e.build, protocolVersion: e.protocolVersion,
                                             minAppBuild: e.minAppBuild, publicURL: e.publicURL.flatMap { URL(string: $0) }))
        case "hub_update": return e.update.map { .hubUpdate($0) }
        case "extension_operation", "plugin_operation": return e.operation.map { .extensionOperation($0, manager: e.manager?.string) }
        case "extension_auth": return e.extensionID.map { .extensionAuth(extension: $0, e.auth?.value, manager: e.manager?.string) }
        case "machines": return e.machines.map { .machines($0) }
        case "sessions":
            guard let m = e.machineID else { return nil }
            return .sessions(machineID: m, (e.sessions ?? []).map { var s = $0; s.machineID = m; return s })
        case "sessions_list": return e.revision.map { .sessionsList(revision: $0, changes: SessionListChanges(since: e.since, changed: e.changed)) }
        case "sessions_delta":
            guard let m = e.machineID else { return nil }
            return .sessionsDelta(machineID: m, upsert: (e.upsert ?? []).map { var s = $0; s.machineID = m; return s }, remove: e.remove ?? [])
        case "items":
            guard let key else { return nil }
            return .items(key, e.items ?? [], backlog: e.backlog ?? false, hasMore: e.hasMore,
                          span: ItemSpan(history: e.history, from: e.fromSeq, to: e.toSeq), moreAfter: e.moreAfter ?? false)
        case "screen":
            guard let key else { return nil }
            return .screen(key, text: e.text ?? "", status: e.status ?? .unknown)
        case "delta":
            guard let key else { return nil }
            return .delta(key, text: e.text ?? "")
        case "live":
            guard let key else { return nil }
            return .live(key, e.live)
        case "interaction": return e.interaction.map { .interaction(Elicitation.withFieldOrder($0, from: data)) }
        case "interaction_resolved":
            guard let id = e.id else { return nil }
            let assistant = e.answeredBy?.string == AssistantAnswer.assistant
                ? AssistantAnswer(answers: Delegated.answers(in: e.answers), reason: e.reason?.string ?? "") : nil
            return .interactionResolved(id, assistant: assistant)
        case "interaction_reanswered":
            guard let id = e.id else { return nil }
            return .interactionReanswered(id: id, answers: Delegated.answers(in: e.resolution?["answers"]))
        case "tool_call":
            guard let id = e.callID, let name = e.name else { return nil }
            return .toolCall(callID: id, name: name, args: e.args ?? .object([:]))
        case "hook_event":
            guard let key else { return nil }
            return .hookEvent(key, event: e.event ?? "", message: e.message, lastText: e.lastAssistantMessage)
        case "error": return .error(key, e.error ?? String(localized: "Unknown error"))
        case "revs":
            guard let m = e.machineID, let revs = e.revs else { return nil }
            return .revs(machineID: m, revs)
        case "lark": return .lark(feedIDs: e.feedIDs ?? [], settingsChanged: e.settingsChanged ?? false, account: e.olAccount)
        case "messaging":
            guard let source = e.source else { return nil }
            return .messaging(source: source, conversations: e.conversations ?? [], settingsChanged: e.settingsChanged ?? false,
                              account: e.account?.string ?? e.account?.int.map(String.init))
        case "agents": return .agents(sessionIDs: e.sessionIDs ?? [])
        // the list prefs changed on another device: a non-session id wakes only the lists, which re-pull the prefs
        case "agents.prefs": return .agents(sessionIDs: ["agents.prefs"])
        case "inbox": return .inbox
        case "todos": return .todos
        case "assistant_saved_prompts": return .savedPrompts(manager: e.manager?.string)
        case "assistant_starred": return .starredMessages(manager: e.manager?.string)
        case "personas": return .personas
        case "look":
            guard (e.effective ?? e.id).map({ !$0.isEmpty }) == true else { return nil }
            return .look(id: e.id, effective: e.effective)
        case "managers": return .managers
        case "directions": return .directions
        case "assistant_feed": return e.item.map { .assistantFeed($0) }
        case "assistant_view":
            guard let view = e.view, !view.id.isEmpty, view.version > 0,
                  !view.conversation.isEmpty, view.conversation == e.conversation else { return nil }
            return .assistantView(view)
        case "assistant_view_block":
            guard let conversation = e.conversation, !conversation.isEmpty, let key = e.key, !key.isEmpty else { return nil }
            return .assistantViewBlock(conversation: conversation, key: key)
        case "assistant_view_action":
            guard let conversation = e.conversation, !conversation.isEmpty,
                  let raw = e.action, let data = try? JSON.encoder.encode(raw),
                  let action = try? JSON.decoder.decode(ExtensionViewAction.self, from: data),
                  !action.key.isEmpty, !action.action.isEmpty, !action.clientMsgId.isEmpty, !action.status.isEmpty,
                  action.revision.map({ $0 > 0 }) ?? true else { return nil }
            return .assistantViewAction(conversation: conversation, action: action)
        case "task_plan": return e.plan.map { .taskPlan($0) }
        case "fleet": return e.fleet.map { .fleet($0) }
        case "desktops": return .desktops(e.desktops ?? [])
        case "terminal":
            guard let key else { return nil }
            if let error = e.error { return .terminal(key, .ended(error, unsupported: e.unsupported ?? false)) }
            guard let epoch = e.epoch, let seq = e.seq, let data = e.data.flatMap({ Data(base64Encoded: $0) }) else { return nil }
            return .terminal(key, .frame(epoch: epoch, seq: seq, full: e.full ?? false, cols: e.cols ?? 0, rows: e.rows ?? 0, data: data))
        // A hub from #1844 on sends each as plugin_ui and then extension_ui; the badge is set, so twice does no harm.
        case "plugin_ui", "extension_ui":
            // An invalidate comes as extension_ui only (an older app would take a plugin_ui for a badge).
            if e.invalidate == true, let ext = e.extensionID ?? e.plugin { return .extensionInvalidate(extension: ext, id: e.id) }
            guard let plugin = e.extensionID ?? e.plugin, let id = e.id else { return nil }
            return .pluginBadge(plugin: plugin, id: id, badge: e.badge.flatMap { $0.text.isEmpty ? nil : $0 }, manager: e.manager?.string)
        case "fs_changed":
            guard let m = e.machineID, let cwd = e.cwd, !cwd.isEmpty else { return nil }
            return .fsChanged(machineID: m, cwd: cwd, dirs: e.dirs ?? [], git: e.git ?? false)
        case "terminal_control":
            guard let key, let state = e.state else { return nil }
            return .terminal(key, .control(state: state, cols: e.cols, rows: e.rows, why: e.error))
        case "urgent_call":
            guard let id = e.id, !id.isEmpty, let state = e.state else { return nil }
            return .urgentCall(id: id, state: state)
        case "meeting_live":
            guard let id = e.recordingID, !id.isEmpty else { return nil }
            let changed = (e.changed?.array ?? []).compactMap { part -> String? in
                if case .string(let name) = part { return name }
                return nil
            }
            return .meetingLive(recordingID: id, changed: changed, revs: e.revs ?? [:])
        default: return nil
        }
    }
}

nonisolated extension ServerEvent {
    /// The event with the group of the connection it came on (docs/protocol/multi-hub.md) on every key, session, machine and
    /// interaction in it. The default group's events are left as they are.
    func stamped(_ group: String) -> ServerEvent {
        guard !group.isEmpty else { return self }
        func k(_ key: SessionKey) -> SessionKey { var key = key; key.group = group; return key }
        switch self {
        case .machines(let list): return .machines(list.map { var m = $0; m.group = group; return m })
        case .sessions(let machine, let list): return .sessions(machineID: machine, list.map { var s = $0; s.group = group; return s })
        case .items(let key, let items, let backlog, let hasMore, let span, let moreAfter):
            return .items(k(key), items, backlog: backlog, hasMore: hasMore, span: span, moreAfter: moreAfter)
        case .screen(let key, let text, let status): return .screen(k(key), text: text, status: status)
        case .delta(let key, let text): return .delta(k(key), text: text)
        case .live(let key, let live): return .live(k(key), live)
        case .interaction(let ix): var ix = ix; ix.group = group; return .interaction(ix)
        case .hookEvent(let key, let event, let message, let lastText):
            return .hookEvent(k(key), event: event, message: message, lastText: lastText)
        case .terminal(let key, let event): return .terminal(k(key), event)
        case .error(let key, let message): return .error(key.map(k), message)
        case .sessionsDelta(let machine, let upsert, let remove):
            return .sessionsDelta(machineID: machine, upsert: upsert.map { var s = $0; s.group = group; return s }, remove: remove)
        case .sessionsList, .hello, .interactionResolved, .interactionReanswered, .toolCall, .revs, .lark, .messaging, .agents, .inbox, .fleet, .hubUpdate, .desktops, .todos,
             .personas, .look, .managers, .directions, .assistantFeed, .assistantView, .assistantViewAction, .assistantViewBlock, .taskPlan, .pluginBadge,
             .extensionInvalidate, .fsChanged, .extensionOperation, .extensionAuth, .meetingLive, .urgentCall, .savedPrompts, .starredMessages: return self
        }
    }
}

/// The Demo Mode hub's end of the socket (Persona/App/DemoHub.swift), and the sync scenarios' (PersonaTests/Sync): what
/// the app sends goes to `receive`, and it answers through `deliver` with the same JSON a hub sends. `dropped`: the
/// connection broke, as a WebSocket's failure (the app's grace and reconnect follow).
protocol LocalSocket: AnyObject {
    func open(deliver: @escaping (Data) -> Void)
    func open(deliver: @escaping (Data) -> Void, dropped: @escaping () -> Void)
    func receive(_ message: [String: Any])
    func close()
}

extension LocalSocket {
    /// A socket that never breaks (Demo Mode's).
    func open(deliver: @escaping (Data) -> Void, dropped: @escaping () -> Void) { open(deliver: deliver) }
}

/// The app's WebSocket to the hub: reconnects with backoff and replays subscriptions.
@Observable
final class LiveConnection {
    enum State: Equatable {
        case idle, connecting, connected
        case failed(String)
    }

    private(set) var state: State = .idle { didSet {
        if state != oldValue {
            onChange?()
            if state != .connected { extensionStreamFeatures = nil; notifyExtensionStreams(.disconnected) }
        }
    } }
    /// Only native full-page clients opt in. Watch/system surfaces keep fitted HTTP snapshots.
    @ObservationIgnored var usesExtensionUIStream = false
    @ObservationIgnored private(set) var extensionStreamFeatures: Set<String>?
    @ObservationIgnored private var extensionStreamObservers: [UUID: (ExtensionStreamSignal) -> Void] = [:]

    enum ExtensionStreamSignal { case hello(Set<String>), frame(Data), hostInteraction(Interaction), disconnected }
    func observeExtensionStream(_ observer: @escaping (ExtensionStreamSignal) -> Void) -> UUID {
        let id = UUID(); extensionStreamObservers[id] = observer; return id
    }
    func removeExtensionStreamObserver(_ id: UUID) { extensionStreamObservers[id] = nil }
    /// A mount can finish after its page disappears. Retire only that correlated mount on this
    /// connection, even when the original page/controller has already been disposed.
    func retireExtensionStreamOpen(_ requestID: String, manager: String?) {
        let id = UUID()
        extensionStreamObservers[id] = { [weak self] signal in
            guard let self else { return }
            if case .disconnected = signal { self.extensionStreamObservers[id] = nil; return }
            guard case .frame(let data) = signal, data.count <= 1_048_576,
                  let raw = try? ExtensionStreamJSON.parse(data), raw["request_id"]?.string == requestID,
                  (manager == nil || raw["manager"] == nil || raw["manager"]?.string == manager),
                  (try? ExtensionStreamSchema.check(raw, definition: "UIStreamServerEnvelope")) != nil,
                  case .snapshot(let frame)? = try? JSONDecoder().decode(ExtensionUIStreamServerEnvelope.self, from: data) else { return }
            self.send(encodable: ExtensionUIStreamClientEnvelope(type: "extension_ui_stream", manager: frame.manager ?? manager,
                request: .close(.init(protocol: 1, requestId: UUID().uuidString, session: frame.snapshot.session, epoch: frame.snapshot.epoch))))
            self.extensionStreamObservers[id] = nil
        }
        Task { [weak self] in
            try? await Task.sleep(for: .seconds(30))
            self?.extensionStreamObservers[id] = nil
        }
    }
    private func notifyExtensionStreams(_ signal: ExtensionStreamSignal) {
        for observer in Array(extensionStreamObservers.values) { observer(signal) }
    }
    private func extensionStreamFrame(_ data: Data?, event: ServerEvent?) {
        if case .hello(let hello) = event {
            extensionStreamFeatures = Set(hello.features)
            notifyExtensionStreams(.hello(Set(hello.features)))
        }
        if case .interaction(let interaction) = event, interaction.toolName == "extension.ui" {
            notifyExtensionStreams(.hostInteraction(interaction))
        }
        // Private trees never enter ServerEvent, the broadcast cursor, replay or a different connection.
        if let data, data.range(of: Data(#""extension_ui_stream""#.utf8)) != nil,
           FrameMeta.decode(data).type == "extension_ui_stream" { notifyExtensionStreams(.frame(data)) }
    }
    /// Connected, but a ping has gone unanswered for `slowAfter`, or its pong came that late: a weak or stalled
    /// network. Only a prompt pong clears it (a late one alone made the title flip every ping), and pings go every
    /// `slowPingEvery` meanwhile. The socket is only declared dead after `deadAfter`; until then the app says
    /// Connecting… (ConnectionStatus).
    private(set) var stalled = false { didSet { if stalled != oldValue { onChange?() } } }
    /// Round trip of the last answered ping.
    private(set) var rtt: TimeInterval?
    /// The hub turned the socket's credential down (401 on the handshake): a revoked device, or a pairing token past its
    /// grace (docs/protocol/protocol.md「临时配对」). Retrying alone will not fix it; the app says Pairing Expired (ConnectionStatus) and
    /// tries again only slowly (`rejectedRetry`). Cleared once a socket opens, and by `connect` (paired again).
    private(set) var rejected = false { didSet { if rejected != oldValue { onChange?() } } }
    static let rejectedRetry: TimeInterval = 60
    /// Cloudflare Access in front of the hub turned the last handshake away (docs/operations/network.md "Behind
    /// Cloudflare Access"); kept through the retries, cleared once a socket opens or by `connect`.
    private(set) var accessBlocked: CloudflareAccessError?
    /// When the next attempt starts while the hub is out of reach.
    private(set) var nextTry: Date?
    /// The socket went while connected and is not back yet (docs/operations/hub-restart.md): nil once it is, or after
    /// `disconnect`.
    private(set) var drop: LinkDrop?
    /// Within the drop's grace (8 s, 30 s after the hub said it was restarting): the app keeps looking connected and
    /// writes keep trying instead of waiting in the outbox. Off once connected again, or when the grace runs out.
    private(set) var inGrace = false { didSet { if inGrace != oldValue { onChange?() } } }
    /// The grace ran out without the socket coming back: the titles say so right away (no second delay on top).
    private(set) var graceOver = false
    /// Connected, or dropped a moment ago and coming back: what the app shows as "connected".
    var looksConnected: Bool { state == .connected || inGrace }
    /// A write may go now: connected and answering, or in a drop's grace (it keeps trying through the gap; after the
    /// grace it waits in the outbox).
    var takesWrites: Bool { (state == .connected && !stalled) || inGrace }
    /// This connection's hello resumed the hub's broadcast stream (`resumed: true`): it replays what this app missed,
    /// so nothing needs fetching again (AppModel.didConnect). Set before `onConnected`.
    @ObservationIgnored private(set) var resumed = false
    /// Only this connection's authenticated hello may enable canonical outbound contracts.
    private(set) var personaContracts = false
    /// Where the app is in the hub's numbered broadcasts: kept across reconnects, cleared with another hub.
    @ObservationIgnored private(set) var cursor = EventCursor()
    /// iOS/Mac list clients opt in; watch/extension consumers retain their existing full-list contract.
    @ObservationIgnored var usesSessionList = false
    @ObservationIgnored var onEvent: ((ServerEvent) -> Void)?
    /// The state or `stalled` changed.
    @ObservationIgnored var onChange: (() -> Void)?
    @ObservationIgnored var prepareEvent: (@Sendable (ServerEvent) -> Void)?
    @ObservationIgnored var onConnected: (() -> Void)?
    /// Picks where each attempt goes when a group has several hubs (HubRoute, docs/protocol/multi-hub.md「Connections and routing」): asked before
    /// every attempt; nil keeps the config given to `connect`.
    @ObservationIgnored var route: (@MainActor () async -> ServerConfig?)?
    /// The credential each attempt goes with (AppModel.currentCredential): a pairing token this device already traded
    /// for its own comes back as the device token, whatever address or route the attempt takes. nil: as given.
    @ObservationIgnored var credential: ((ServerConfig) -> ServerConfig)?

    @ObservationIgnored private var config: ServerConfig?
    /// Where the socket goes (or went last): the config given to `connect`, or the one `route` picked.
    var currentConfig: ServerConfig? { config }
    @ObservationIgnored private var task: URLSessionWebSocketTask?
    /// The demo config's socket, made by `makeLocal` (DemoHub registers it at launch).
    @ObservationIgnored private var local: LocalSocket?
    @ObservationIgnored static var makeLocal: (() -> LocalSocket)?
    #if DEBUG
    /// Unit tests: this connection's own socket instead of `makeLocal`'s, for several models side by side, one per
    /// device (`AppModel.linkHooksForTests`).
    @ObservationIgnored var makeLocalForTests: (() -> LocalSocket)?
    #endif
    @ObservationIgnored private var generation = 0
    /// Requests made while the foreground connection chooses its way wait for that same attempt. They do not wait
    /// for the WebSocket hello: once the route is installed, HTTP can use it independently.
    @ObservationIgnored private var routeTask: Task<Void, Never>?
    @ObservationIgnored private var routeWaiters: [UUID: CheckedContinuation<ServerConfig?, Error>] = [:]
    @ObservationIgnored private var backoff: TimeInterval = 1
    /// The hub said `hub_restarting` on this socket: its close is a restart whatever the close code.
    @ObservationIgnored private var restartNotice = false
    /// Attempts since the drop: in the quick phase after a restart, and after it (LinkDrop.retryDelay).
    @ObservationIgnored private var fastTries = 0
    @ObservationIgnored private var tries = 0
    /// The heartbeat gave up on a quiet socket: our own close, not the hub's 1001.
    @ObservationIgnored private var closedQuiet = false
    @ObservationIgnored private let session = URLSession(configuration: .default)
    /// When the hub last answered (any message or a pong): a socket that went quiet on a network change can sit
    /// "connected" forever, and the chat would silently stop updating.
    @ObservationIgnored private var lastHeard = Date.distantPast
    static let pingEvery: Duration = .seconds(20)
    static let deadAfter: TimeInterval = 45
    static let slowAfter: TimeInterval = 5
    static let slowPingEvery: Duration = .seconds(5)
    @ObservationIgnored private var lastPong = Date.distantPast
    /// The app is in the background: nothing opens the socket (a reconnect after a failure, the network coming back, a
    /// refresh finishing) but `wake()`, for a silent push. iOS suspends an app with its socket open, and the hub takes
    /// a connected phone for an open app: no wake push for its phone_* calls, no Live Activity pushes (it draws its
    /// own), until the pings time out.
    @ObservationIgnored var suspended = false

    func connect(_ config: ServerConfig) {
        if config != self.config { cursor = EventCursor() } // another hub or pairing: its stream is not ours
        self.config = config
        backoff = 1
        rejected = false
        accessBlocked = nil
        open()
    }

    /// The device traded the token this socket uses (`old`, its pairing token) for its own (docs/protocol/protocol.md「临时配对」):
    /// the same address or route goes on with `new`, at once while it is up (the hub cuts the old one's sockets after a
    /// short grace). The broadcast stream stays ours: it is the same hub. False when the socket is not on `old`.
    @discardableResult
    func renewCredential(from old: String, to new: String) -> Bool {
        guard var c = config, c.token == old, old != new else { return false }
        c.token = new
        config = c
        rejected = false
        if state == .idle { return true } // closed on purpose (the background): it opens with the new one later
        backoff = 1
        open()
        return true
    }

    func disconnect() {
        personaContracts = false
        generation += 1
        cancelRoute()
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        local?.close()
        local = nil
        endDrop()
        state = .idle
        stalled = false
        nextTry = nil
    }

    /// A silent push (`persona_wake`) in the background: connect for the hub's call, though suspended.
    func wake() {
        suspended = false
        ensureConnected()
    }

    /// Reconnect now if the socket is down (e.g. app came to foreground).
    func ensureConnected() {
        guard config != nil else { return }
        if state != .connected && state != .connecting { backoff = 1; open() }
    }

    /// The network came back or the user asked: try right away instead of after the backoff (up to 20 s), and also
    /// replace an attempt that hangs. A socket that works is left alone.
    func reconnectNow() {
        guard config != nil else { return }
        backoff = 1
        if state != .connected || stalled { open() }
    }

    /// The group's center answers again while the socket is on a member (AppModel.watchCenter): open a new socket,
    /// which asks `route` first and so goes to the center. Nothing while suspended or with no route to ask.
    func reroute() {
        guard config != nil, route != nil, !suspended else { return }
        backoff = 1
        open()
    }

    /// The network path changed (Wi-Fi to cellular): a socket on the old interface goes quiet without failing. Ping
    /// now, so a dead one shows as stalled in seconds instead of at the next heartbeat.
    func probe() {
        guard let task, state == .connected else { return }
        ping(task, generation)
    }

    /// The config selected by an attempt already in progress, or the current config when routing has finished.
    /// A closed or superseded attempt cancels its requests instead of sending directly while the tunnel is absent.
    func waitForRoute() async throws -> ServerConfig? {
        try Task.checkCancellation()
        guard routeTask != nil else { return config }
        let id = UUID()
        return try await withTaskCancellationHandler {
            try Task.checkCancellation()
            return try await withCheckedThrowingContinuation { continuation in
                routeWaiters[id] = continuation
            }
        } onCancel: { [weak self] in
            Task { @MainActor in
                self?.routeWaiters.removeValue(forKey: id)?.resume(throwing: CancellationError())
            }
        }
    }

    private func finishRoute(_ result: Result<ServerConfig?, Error>) {
        let waiting = routeWaiters
        routeWaiters.removeAll()
        for continuation in waiting.values { continuation.resume(with: result) }
    }

    private func cancelRoute() {
        routeTask?.cancel()
        routeTask = nil
        finishRoute(.failure(CancellationError()))
    }

    #if DEBUG
    /// Deterministic ordering in route-recovery tests; no polling is added to the connection itself.
    var waitingForRouteForTests: Int { routeWaiters.count }
    #endif

    private func open() {
        guard let config, !suspended else { return }
        personaContracts = false
        generation += 1
        cancelRoute()
        attemptBegan = Date()
        let gen = generation
        task?.cancel(with: .goingAway, reason: nil)
        local?.close()
        local = nil
        if config.isDemo {
            openLocal(gen)
            return
        }
        guard let route else { openSocket(config, gen); return }
        state = .connecting
        stalled = false
        nextTry = nil
        routeTask = Task { @MainActor [weak self] in
            guard !Task.isCancelled else { return }
            let picked = await route()
            guard let self, gen == self.generation, !self.suspended, !Task.isCancelled else { return }
            if let picked, picked != self.config {
                self.config = picked
                self.onRoute?(picked)
            }
            self.routeTask = nil
            self.openSocket(self.config ?? config, gen)
            self.finishRoute(.success(self.config))
        }
    }

    /// The hub `route` picked for the next attempt.
    @ObservationIgnored var onRoute: ((ServerConfig) -> Void)?
    /// What an attempt came to at the address it went to (#11390): `answers` once it connects, and when it fails before
    /// connecting, why (`HubOutcome(failure:)`), with when the attempt began. A connected socket dropping is not an
    /// attempt: the next one says.
    @ObservationIgnored var onAttempt: ((ServerConfig, HubOutcome, Date) -> Void)?
    /// When the current attempt began (`open`, before its route is picked): what `onAttempt` reports belongs to it.
    @ObservationIgnored private var attemptBegan = Date()

    private func openSocket(_ given: ServerConfig, _ gen: Int) {
        let config = credential?(given) ?? given
        if config != given { self.config = config }
        var comps = URLComponents(url: config.url.appending(path: "/api/v1/ws"), resolvingAgainstBaseURL: false)!
        comps.scheme = config.url.scheme == "https" ? "wss" : "ws"
        comps.queryItems = Self.queryItems(cursor, sessionList: usesSessionList, extensionStream: usesExtensionUIStream, sessionLive: true)
        var req = URLRequest(url: HubTransport.resolve(comps.url!, hub: config.url))
        config.authorize(&req)
        req.setValue(LanguageStore.shared.launched, forHTTPHeaderField: "Accept-Language")
        RequestClientMetadata.current.apply(to: &req)
        req.timeoutInterval = 15
        resumed = false // until this socket's hello says so
        let t = session.webSocketTask(with: req)
        t.delegate = HubRedirects.shared
        t.maximumMessageSize = 64 << 20
        task = t
        state = .connecting
        stalled = false
        nextTry = nil
        t.resume()
        receive(t, gen)
        heartbeat(t, gen)
    }

    /// The socket URL's query: session lists as deltas (an older hub ignores the question and sends whole lists), the
    /// extensions' install steps (`extension_operation`, #3307), the managers' roster changes (`managers`,
    /// docs/assistant/managers.md), Persona Link entries in the `desktops` event (`desktops_link`, `DesktopDevice`),
    /// Claude's live status (`session_live`), and where to resume the broadcasts from once a hello gave a stream.
    static func queryItems(_ cursor: EventCursor, sessionList: Bool = false, extensionStream: Bool = false, sessionLive: Bool = false) -> [URLQueryItem] {
        [URLQueryItem(name: "features", value: (extensionStream ? "extension_ui_stream_v1," : "") + (sessionLive ? "session_live," : "") + (sessionList ? "sessions_list," + SessionListChanges.feature + "," : "") + "sessions_delta,extension_operations,managers,desktops_link," + HubNaming.feature)] + cursor.queryItems
    }

    private func heartbeat(_ t: URLSessionWebSocketTask, _ gen: Int) {
        Task { @MainActor [weak self] in
            while true {
                try? await Task.sleep(for: self?.stalled == true ? Self.slowPingEvery : Self.pingEvery)
                guard let self, gen == self.generation else { return }
                guard self.state == .connected else { continue }
                if Date.now.timeIntervalSince(self.lastHeard) > Self.deadAfter {
                    self.closedQuiet = true
                    t.cancel(with: .goingAway, reason: nil) // its receive fails and schedules the reconnect
                    return
                }
                self.ping(t, gen)
            }
        }
    }

    /// One ping: its pong refreshes `lastHeard` and the round trip, and decides `stalled`; no answer within
    /// `slowAfter` marks the link stalled.
    private func ping(_ t: URLSessionWebSocketTask, _ gen: Int) {
        let sent = Date.now
        t.sendPing { [weak self] error in
            guard error == nil else { return }
            Task { @MainActor in
                guard let self, gen == self.generation else { return }
                let rtt = Date.now.timeIntervalSince(sent)
                self.lastHeard = .now
                self.lastPong = .now
                self.rtt = rtt
                self.stalled = rtt > Self.slowAfter
            }
        }
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .seconds(Self.slowAfter))
            guard let self, gen == self.generation, self.state == .connected, self.lastPong < sent else { return }
            self.stalled = true
        }
    }

    /// One ping on this connection for the live screen's Connection Stats (#6558): the round trip to the hub, or nil
    /// without a connection or without a pong within 5 s. The heartbeat's kind of ping; it changes none of its state.
    func roundTrip() async -> TimeInterval? {
        guard state == .connected, let t = task else { return nil }
        let sent = Date.now
        return await withCheckedContinuation { (c: CheckedContinuation<TimeInterval?, Never>) in
            let done = OSAllocatedUnfairLock(initialState: false)
            let finish: @Sendable (TimeInterval?) -> Void = { v in
                let first = done.withLock { was -> Bool in
                    defer { was = true }
                    return !was
                }
                if first { c.resume(returning: v) }
            }
            t.sendPing { @Sendable error in finish(error == nil ? Date.now.timeIntervalSince(sent) : nil) }
            DispatchQueue.global().asyncAfter(deadline: .now() + 5) { finish(nil) }
        }
    }

    private func receive(_ t: URLSessionWebSocketTask, _ gen: Int) {
        let prepare = prepareEvent
        t.receive { [weak self, prepare] result in
            // Decode here, off the main thread: session lists are large and arrive every second while agents
            // work (decoding them on the main thread showed up in scrolling profiles).
            let data: Data? = switch result {
            case .success(.string(let s)): s.data(using: .utf8)
            case .success(.data(let d)): d
            default: nil
            }
            let event = data.flatMap(ServerEvent.decode)
            let meta = data.map(FrameMeta.read) ?? FrameMeta()
            if let data { DevProbe.frame(type: meta.type, bytes: data.count) }
            if let event { prepare?(event) }
            Task { @MainActor in
                guard let self, gen == self.generation else { return }
                switch result {
                case .success:
                    self.lastHeard = .now
                    let first = self.state != .connected
                    if first { self.endDrop() }
                    if meta.type != "extension_ui_stream" { self.note(meta) } // private sessions have no broadcast cursor
                    if case .hello(let hello)? = event { self.personaContracts = hello.features.contains(HubNaming.feature) }
                    if first {
                        DevProbe.socket(.connected)
                        if let on = self.config { self.onAttempt?(on, .answers, self.attemptBegan) }
                        self.state = .connected
                        self.sayHello()
                        self.backoff = 1
                        self.rejected = false
                        self.accessBlocked = nil
                        self.onConnected?()
                    }
                    self.extensionStreamFrame(data, event: event)
                    if let event { self.onEvent?(event) }
                    self.receive(t, gen)
                case .failure(let err):
                    DevProbe.socket(.dropped)
                    let attempt = self.state != .connected // not a drop: this attempt never connected
                    self.personaContracts = false
                    let planned = self.restartNotice || (!self.closedQuiet && t.closeCode == .goingAway)
                    self.dropped(restart: planned)
                    // The handshake's answer: 401 is the hub refusing this credential, not the hub being away.
                    let refused = (t.response as? HTTPURLResponse)?.statusCode == 401
                    if refused != self.rejected { self.rejected = refused }
                    // Cloudflare Access in front of the hub turned the handshake away: say so, not a generic failure.
                    let access = CloudflareAccess.blocked(t.response) ? CloudflareAccessError(tokenSent: self.config?.access != nil) : nil
                    if access != self.accessBlocked { self.accessBlocked = access }
                    self.state = .failed(refused ? String(localized: "The hub doesn't accept this device's credentials (401)")
                                         : access?.localizedDescription ?? err.localizedDescription)
                    self.stalled = false
                    if attempt, let on = self.config {
                        self.onAttempt?(on, refused || access != nil ? HubOutcome.refusedHandshake : HubOutcome(failure: err), self.attemptBegan)
                    }
                    self.scheduleReconnect(gen)
                }
            }
        }
    }

    /// Demo Mode: no network, no heartbeat; the made-up hub's messages arrive like a socket's.
    private func openLocal(_ gen: Int) {
        var made = Self.makeLocal
        #if DEBUG
        if let mine = makeLocalForTests { made = mine }
        #endif
        guard let make = made else {
            state = .failed(String(localized: "Demo mode isn't available"))
            return
        }
        let socket = make()
        local = socket
        state = .connecting
        let prepare = prepareEvent
        // As a WebSocket's frames (`receive`): the hello's place in the hub's broadcasts and whether it resumed them,
        // then the connection's start.
        socket.open(deliver: { [weak self] data in
            guard let self, gen == self.generation else { return }
            let event = ServerEvent.decode(data)
            let meta = FrameMeta.read(data)
            if let event { prepare?(event) }
            let first = self.state != .connected
            if first { self.endDrop() }
            if meta.type != "extension_ui_stream" { self.note(meta) }
            if case .hello(let hello)? = event { self.personaContracts = hello.features.contains(HubNaming.feature) }
            if first {
                self.state = .connected
                self.sayHello()
                self.onConnected?()
            }
            self.lastHeard = .now
            self.extensionStreamFrame(data, event: event)
            if let event { self.onEvent?(event) }
        }, dropped: { [weak self] in
            guard let self, gen == self.generation else { return }
            self.personaContracts = false
            self.dropped(restart: self.restartNotice)
            self.state = .failed(String(localized: "Connection lost"))
            self.stalled = false
            self.scheduleReconnect(gen)
        })
    }

    /// What a message says about the connection: the broadcast stream's place, whether this hello resumed it, a
    /// restart coming.
    private func note(_ meta: FrameMeta) {
        if meta.type == "hello" {
            resumed = meta.resumed && meta.eventStream != nil && meta.eventStream == cursor.stream
            cursor = cursor.afterHello(stream: meta.eventStream, seq: meta.eventSeq, resumed: resumed)
        } else {
            cursor = cursor.afterEvent(meta.eventSeq)
        }
        if meta.isRestarting { restartNotice = true }
    }

    /// The socket failed. One that was connected starts a drop and its grace (a planned restart: the longer one);
    /// a failed attempt during a drop keeps it.
    private func dropped(restart: Bool) {
        restartNotice = false
        closedQuiet = false
        if drop == nil, state == .connected {
            let d = LinkDrop(at: .now, restart: restart)
            drop = d
            fastTries = 0
            tries = 0
            graceOver = false
            inGrace = true
            Task { @MainActor [weak self] in
                // Until this drop's grace is over, however long it is by then (a restart can lengthen it).
                while let self, let now = self.drop, now.at == d.at, self.state != .connected {
                    let left = now.graceUntil.timeIntervalSinceNow
                    guard left > 0 else {
                        self.graceOver = true
                        self.inGrace = false
                        return
                    }
                    try? await Task.sleep(for: .seconds(left))
                }
            }
        } else if restart, var d = drop, !d.restart {
            d.restart = true // the hub said so on a socket that came back only for a moment: the longer grace
            drop = d
        }
    }

    private func endDrop() {
        drop = nil
        graceOver = false
        inGrace = false
        restartNotice = false
        closedQuiet = false
    }

    private func scheduleReconnect(_ gen: Int) {
        var delay: TimeInterval
        if let drop {
            let since = Date.now.timeIntervalSince(drop.at)
            let fast = drop.restart && since < LinkDrop.restartFast
            delay = LinkDrop.retryDelay(attempt: fast ? fastTries : tries, restart: drop.restart, sinceDrop: since,
                                        rand: Double.random(in: 0..<1))
            if fast { fastTries += 1 } else { tries += 1 }
        } else {
            delay = backoff
            backoff = min(backoff * 2, 20)
        }
        // A refused credential: slowly (a public address counts failed logins and blocks the address, docs/protocol/protocol.md).
        if rejected { delay = max(delay, Self.rejectedRetry) }
        let wait = delay
        nextTry = .now + wait
        Task { @MainActor in
            try? await Task.sleep(for: .seconds(wait))
            guard gen == self.generation else { return }
            self.open()
        }
    }

    /// client_hello (docs/protocol/protocol.md, docs/protocol/i18n.md「Protocol: telling hub the language」): this connection's interface language, so the hub's
    /// notices and errors on it, and the user's language for what it produces later, match the app. The naming
    /// contract and MCP forms and links are advertised here (`helloFeatures`), with the platform and version this app's
    /// requests carry (`X-Persona-Client`): the hub lists those cards in a REST answer only to a client whose hello
    /// matches it. The version ("1.4.0+52": the app's version and build) and `os` are also what the hub keeps of this
    /// device for the assistant (`AppVersion`). Older hubs ignore unknown fields and keep their existing defaults.
    private func sayHello() {
        let client = RequestClientMetadata.current
        send(["type": "client_hello", "locale": LanguageStore.shared.launched,
              "features": Self.helloFeatures(client) + (followsMessaging ? [Self.larkViaMessagingFeature] : []),
              "platform": client.platform, "version": client.version, "os": AppVersion.helloOS])
    }

    /// The app's Feishu screens follow the hub's `messaging` events (a messaging source, #2138), so the legacy `lark`
    /// event, which `AppModel.handle` drops then, need not come: said in `client_hello` (`lark_via_messaging`), and
    /// said again in a new one when it changes (#9591). A busy Feishu account sent 40–50 of them a minute.
    var followsMessaging = false {
        didSet { if followsMessaging != oldValue, state == .connected { sayHello() } }
    }

    nonisolated static let larkViaMessagingFeature = "lark_via_messaging"

    /// `elicitation` (MCP forms and links, `ElicitationForm`) from the iOS app alone: the frozen Mac app and visionOS
    /// don't declare it, so the hub hands those cards to the terminal when they are the only apps open.
    /// `persona_card_ring` (```persona-card rings, `PersonaCard.swift`) from the apps that build the chat (iOS, Mac,
    /// visionOS), not the watch or the extensions: a client without it gets each ring as a stacked bar (the hub's
    /// card_compat.go), which every build before the ring draws.
    nonisolated static func helloFeatures(_ client: RequestClientMetadata) -> [String] {
        let iOSApp = RequestClientMetadata.Platform.iOS.rawValue + "-" + RequestClientMetadata.Surface.app.rawValue
        let chatApps = [RequestClientMetadata.Platform.iOS, .macOS, .visionOS].map { $0.rawValue + "-" + RequestClientMetadata.Surface.app.rawValue }
        return [HubNaming.feature] + (client.platform == iOSApp ? ["elicitation", UndeliveredMessage.agentHoldFeature] : []) + (chatApps.contains(client.platform) ? [cardRingFeature, "closeout.markdown.v1"] : [])
    }

    /// The client_hello feature that says this app draws ```persona-card rings (#3068).
    nonisolated static let cardRingFeature = "persona_card_ring"

    func send(_ object: [String: Any]) {
        if let local {
            local.receive(object)
            return
        }
        guard let task, state == .connected,
              let data = try? JSONSerialization.data(withJSONObject: object),
              let s = String(data: data, encoding: .utf8) else { return }
        task.send(.string(s)) { _ in }
    }

    func send<T: Encodable>(encodable: T) {
        if let local {
            if let data = try? JSON.encoder.encode(encodable), let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                local.receive(object)
            }
            return
        }
        guard let task, state == .connected, let data = try? JSON.encoder.encode(encodable),
              let s = String(data: data, encoding: .utf8) else { return }
        task.send(.string(s)) { _ in }
    }
}
