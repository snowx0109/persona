#if os(iOS)
import AVFoundation
import Foundation
import LiveCommunicationKit
import os

// Between `PersonaCalls` and the system (#9290): LiveCommunicationKit on a device (`LCKCallSystem`), a stand-in on the
// Simulator (`SimulatedCallSystem`: it has no call service and ends every conversation in about 80 ms), a fake in the
// tests. LiveCommunicationKit's action objects cannot be made by anyone else, so each one reaches `PersonaCalls` as a
// `CallSystemAction` with its own fulfil and fail.

/// An action the system asks the app to carry out.
nonisolated enum CallSystemAction: Equatable, Sendable {
    case start, join, end
    case mute(Bool)
    case pause(Bool)
    case other
}

/// An action the app asks the system for.
nonisolated enum CallRequest: Equatable, Sendable {
    case join, end
    case mute(Bool)
    case pause(Bool)
}

/// Where the system's conversation is (LiveCommunicationKit's `Conversation.State`).
nonisolated enum CallSystemState: Equatable, Sendable {
    case idle, joining, joined, paused, leaving, left
}

/// What the system tells the app, on the main actor.
@MainActor protocol CallSystemHandler: AnyObject {
    func system(perform action: CallSystemAction, for id: UUID, fulfil: @escaping @MainActor () -> Void, fail: @escaping @MainActor () -> Void)
    func system(changed id: UUID, state: CallSystemState)
    func system(timedOut action: CallSystemAction, for id: UUID)
    func systemActivatedAudio()
    func systemDeactivatedAudio()
    func systemReset()
}

/// The system's side of the app's calls: one per app, made the first time a call needs it (never at launch, #9264).
@MainActor protocol CallSystem: AnyObject {
    var handler: (any CallSystemHandler)? { get set }
    /// Asks the system to start a call to Persona with `handle` (`CallHandle.outgoing`: the assistant's name, or
    /// Developing Persona's placeholder address).
    func start(_ id: UUID, handle: CallHandle) async throws
    /// An incoming call (an urgent call's ring) from the assistant called `name` (the handle's value), with the call's
    /// own `title` as its display name only.
    func reportIncoming(_ id: UUID, name: String, title: String) async throws
    func perform(_ request: CallRequest, for id: UUID) async throws
    func report(_ report: CallReport, for id: UUID)
    /// The conversation is in the system's list (reports before that are dropped).
    func knows(_ id: UUID) -> Bool
    /// Activates the audio session itself, once, where the system did not: true when it did.
    func activateAudio() async -> Bool
}

/// The handle a call to Persona carries to the system: what its call UI, the Dynamic Island, the Lock Screen and Recents
/// show, and what a Recents redial gives back. Its value is the assistant's name as the main conversation's top bar says
/// it (#9887: `AppModel.assistantShownName`, the current assistant's on a hub with several, else the look's, "Persona"
/// for the built-in Mochi), the same name the call screen, its bar and the questions say: the system shows the value and
/// ignores the display name (FB24933340). Any value redials the current hub's main conversation, so a name changed since
/// still calls Persona. Developing Persona's Call handle (#9887, an
/// on-device experiment for Phone › Recents) can make it an email handle with a fixed placeholder address instead.
nonisolated struct CallHandle: Equatable, Sendable {
    var kind: CallHandleKind
    var value: String
    var displayName: String

    /// The product's name, for an assistant with none.
    static let fallback = "Persona" // i18n-ignore: the product's name, shown as is everywhere
    /// The email handle's address: fixed, on the reserved `.invalid` domain (RFC 2606), so it can never reach anyone.
    static let placeholderAddress = "persona@persona.invalid" // i18n-ignore: a placeholder address, never shown as text

    /// The assistant's name as a call carries it: trimmed, or the product's name when it has none.
    static func name(_ assistant: String?) -> String {
        let trimmed = assistant?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return trimmed.isEmpty ? fallback : trimmed
    }

    /// A call's handle (#9887 one button: one fixed handle for every call, whichever conversation it talks in): the
    /// assistant's name as the value of a generic handle, or the placeholder address. The display name may carry the
    /// `conversation`'s title ("Persona · <title>"; the system may show only the value).
    static func outgoing(name: String, kind: CallHandleKind, conversation: String? = nil) -> CallHandle {
        let shown = conversation.map { "\(name) · \($0)" } ?? name // i18n-ignore: a name and a title, as the system shows them
        switch kind {
        case .generic: return CallHandle(kind: .generic, value: name, displayName: shown)
        case .email: return CallHandle(kind: .email, value: placeholderAddress, displayName: shown)
        }
    }

    /// The value as the log may say it (public): the fixed ones as they are, an assistant's own name only by its length.
    var logged: String {
        [Self.fallback, Self.placeholderAddress].contains(value) ? value : "<the assistant's name, \(value.count) characters>"
    }
}

/// An error from the system, by its domain and code. LiveCommunicationKit's errors keep CallKit's domain strings
/// (`com.apple.CallKit.error.requesttransaction`…), sometimes under `NSUnderlyingErrorKey`: they are compared as strings,
/// so the app never imports CallKit (App Store China rejects a binary that links it).
nonisolated struct CallSystemError: Equatable, Sendable {
    var domain: String
    var code: Int

    init(domain: String, code: Int) {
        self.domain = domain
        self.code = code
    }

    /// The innermost error of the system's call service, or the error itself.
    init(_ error: any Error) {
        var e = error as NSError
        while !e.domain.hasPrefix(Self.callService), let inner = e.userInfo[NSUnderlyingErrorKey] as? NSError { e = inner }
        self.init(domain: e.domain, code: e.code)
    }

    static let callService = "com.apple.CallKit.error"
    static let requestTransaction = callService + ".requesttransaction"

    /// The request came while the app was still coming to the front (code 6, `invalidAction`, seen on device): once more
    /// a moment later.
    var isTooEarly: Bool { domain == Self.requestTransaction && code == 6 }
    /// Every call group is taken (code 7).
    var isAtCapacity: Bool { domain == Self.requestTransaction && code == 7 }

    var description: String { "\(domain) \(code)" }
}

/// Runs `body` on the main actor: at once on the main thread, else as soon as the main queue can, in the order asked.
nonisolated enum CallMain {
    static func run(_ body: @escaping @MainActor @Sendable () -> Void) {
        if Thread.isMainThread {
            MainActor.assumeIsolated { body() }
        } else {
            DispatchQueue.main.async { MainActor.assumeIsolated { body() } }
        }
    }
}

/// One platform fact, injected into the call factory, push registration and Settings by their tests.
nonisolated enum CallPlatform {
    static var isiOSAppOnMac: Bool { ProcessInfo.processInfo.isiOSAppOnMac }
}

/// An iPhone app on a Mac cannot use LiveCommunicationKit. Calls that somehow still ask for its system side fail
/// without constructing a manager or touching its delegate conformance; launch and Settings never offer them there.
@MainActor final class UnavailableCallSystem: CallSystem {
    weak var handler: (any CallSystemHandler)?
    func start(_ id: UUID, handle: CallHandle) async throws { throw CancellationError() }
    func reportIncoming(_ id: UUID, name: String, title: String) async throws { throw CancellationError() }
    func perform(_ request: CallRequest, for id: UUID) async throws { throw CancellationError() }
    func report(_ report: CallReport, for id: UUID) {}
    func knows(_ id: UUID) -> Bool { false }
    func activateAudio() async -> Bool { false }
}

// MARK: LiveCommunicationKit

/// The app's one `ConversationManager` (one per process: a second one takes every action from the first). Its
/// configuration is read-only, so it is decided here once (docs: #9290 "LCK configuration").
@MainActor final class LCKCallSystem: CallSystem {
    /// Tests set the platform explicitly. Production guards before naming this type, then calls it with false.
    static func make(isiOSAppOnMac: Bool = CallPlatform.isiOSAppOnMac) -> LCKCallSystem? {
        guard !isiOSAppOnMac else { return nil }
        return LCKCallSystem()
    }

    private static let log = Logger(subsystem: "persona", category: "call")
    weak var handler: (any CallSystemHandler)?
    /// Used only from the main actor (its async calls run on its own queue): held unchecked rather than sent between regions.
    nonisolated(unsafe) private let manager: ConversationManager
    private let delegate = PersonaCallsLCKDelegate()

    /// Two groups so an urgent call during a call to Persona offers Hold & Accept (with one the system offers only End &
    /// Accept and Decline, and a Start at capacity fails); calls in Recents for the redial; two handle types, `.generic`
    /// (the assistant's name) and `.emailAddress` (Developing Persona's Call handle, #9887: the configuration is fixed for
    /// the manager's life, so both are declared whichever the switch says). The system call UI's icon is the assistant's
    /// mark as a template (`CallIcon`, Mochi's silhouette): its PNG drawn and encoded off the main actor before the
    /// manager is made (`PersonaCalls.readySystem`), and only handed in here, on the main actor.
    static func configuration(icon: Data?) -> ConversationManager.Configuration {
        ConversationManager.Configuration(ringtoneName: nil, iconTemplateImageData: icon,
                                          maximumConversationGroups: 2, maximumConversationsPerConversationGroup: 1,
                                          includesConversationInRecents: true, supportsVideo: false,
                                          supportedHandleTypes: handleTypes)
    }

    /// The handle types the manager is made with.
    static let handleTypes: Set<Handle.Kind> = [.generic, .emailAddress]

    /// The owner's own side of every conversation: the local member of an incoming call and of an outgoing one's members
    /// update.
    static let owner = Handle(type: .generic, value: "owner") // i18n-ignore: a handle's value the system never shows

    /// `handle` as LiveCommunicationKit has it.
    static func lck(_ handle: CallHandle) -> Handle {
        let type: Handle.Kind = switch handle.kind {
        case .generic: .generic
        case .email: .emailAddress
        }
        return Handle(type: type, value: handle.value, displayName: handle.displayName)
    }

    /// The outgoing call's members update (#9887, #12341): the owner as the local member, the assistant as the one remote
    /// member, and active only once the conversation is joined. Apple's `members` are the remote participants "including
    /// invited, but inactive" ones, so the owner is never among them; until the call joins, the assistant is invited, not
    /// active.
    static func outgoingMembers(_ remote: Handle, state: CallSystemState) -> Conversation.Update {
        Conversation.Update(localMember: owner, members: [remote],
                            activeRemoteMembers: state == .joined || state == .paused ? [remote] : [], capabilities: [.pausing])
    }

    /// Each outgoing call's handle, from its Start until it ends: the remote member of its members update.
    private var outgoing: [UUID: Handle] = [:]

    init() {
        let icon = CallIcon.ready
        if icon == nil { Self.log.notice("call: the manager is made before its icon was drawn: the system call UI shows none") }
        manager = ConversationManager(configuration: Self.configuration(icon: icon))
        manager.delegate = delegate
        delegate.system = self
    }

    func start(_ id: UUID, handle: CallHandle) async throws {
        let assistant = Self.lck(handle)
        outgoing[id] = assistant
        Self.log.notice("call \(id.uuidString, privacy: .public): Start requested, handle \(handle.kind.rawValue, privacy: .public) \(handle.logged, privacy: .public)")
        nonisolated(unsafe) let lck = manager
        do {
            try await lck.perform([StartConversationAction(conversationUUID: id, handles: [assistant], isVideo: false)])
        } catch {
            outgoing[id] = nil
            throw error
        }
    }

    func reportIncoming(_ id: UUID, name: String, title: String) async throws {
        // The call's id stays out of the handle: it showed as "call_…" and would land in Recents. The title goes only in
        // the display name, which the system may show one day and Recents never keeps.
        let update = Conversation.Update(localMember: Self.owner,
                                         members: [Handle(type: .generic, value: name, displayName: title)],
                                         capabilities: [.pausing])
        nonisolated(unsafe) let lck = manager
        try await lck.reportNewIncomingConversation(uuid: id, update: update)
    }

    func perform(_ request: CallRequest, for id: UUID) async throws {
        let action: ConversationAction = switch request {
        case .join: JoinConversationAction(conversationUUID: id)
        case .end: EndConversationAction(conversationUUID: id)
        case .mute(let on): MuteConversationAction(conversationUUID: id, isMuted: on)
        case .pause(let on): PauseConversationAction(conversationUUID: id, isPaused: on)
        }
        Self.log.notice("call \(id.uuidString, privacy: .public): asks the system for \(String(describing: request), privacy: .public)")
        nonisolated(unsafe) let lck = manager
        nonisolated(unsafe) let one = action
        try await lck.perform([one])
    }

    func report(_ report: CallReport, for id: UUID) {
        guard let conversation = manager.conversations.first(where: { $0.uuid == id }) else {
            Self.log.notice("call \(id.uuidString, privacy: .public): report \(report.logName, privacy: .public) dropped: the system does not have the conversation")
            if case .ended = report { outgoing[id] = nil }
            return
        }
        let state = Self.state(conversation.state)
        let event: Conversation.Event
        switch report {
        case .startedConnecting(let at):
            event = .conversationStartedConnecting(at)
        case .connected(let at):
            event = .conversationConnected(at)
        case .members:
            // The outgoing call's members (#9887): with no remote member the system has nobody to write into Recents.
            if let remote = outgoing[id] {
                let update = Self.outgoingMembers(remote, state: state)
                event = .conversationUpdated(update)
                let active = update.activeRemoteMembers?.isEmpty == false ? "active" : "invited"
                Self.log.notice("recents: call \(id.uuidString, privacy: .public): members reported (remote \(String(describing: remote.type), privacy: .public) handle \(active, privacy: .public), local owner), the conversation \(String(describing: state), privacy: .public), local member \(conversation.localMember == nil ? "none" : "set", privacy: .public) before it")
            } else {
                event = .conversationUpdated(Conversation.Update(capabilities: [.pausing]))
                Self.log.notice("recents: call \(id.uuidString, privacy: .public): no Start handle for the members update, only pausing reported")
            }
        case .ended(let at, let why):
            event = .conversationEnded(at, Self.reason(why))
            outgoing[id] = nil
        }
        manager.reportConversationEvent(event, for: conversation)
        Self.log.notice("call \(id.uuidString, privacy: .public): report \(report.logName, privacy: .public) sent, the conversation \(String(describing: state), privacy: .public)")
    }

    func knows(_ id: UUID) -> Bool { manager.conversations.contains { $0.uuid == id } }

    func activateAudio() async -> Bool {
        await Task.detached {
            do {
                try AVAudioSession.sharedInstance().setActive(true)
                return true
            } catch {
                Logger(subsystem: "persona", category: "call").notice("call: activating the audio session failed: \(error.localizedDescription, privacy: .public)")
                return false
            }
        }.value
    }

    private static func reason(_ r: CallEndedReport) -> Conversation.EndedReason {
        switch r {
        case .failed: .failed
        case .remoteEnded: .remoteEnded
        case .unanswered: .unanswered
        case .joinedElsewhere: .joinedElsewhere
        case .declinedElsewhere: .declinedElsewhere
        }
    }

    // MARK: From the delegate, on the main actor

    fileprivate func received(_ action: ConversationAction) {
        let id = action.conversationUUID
        nonisolated(unsafe) let action = action
        let kind: CallSystemAction
        let fulfil: @MainActor () -> Void
        switch action {
        case let a as StartConversationAction:
            kind = .start
            fulfil = { [weak self] in
                a.fulfill(dateStarted: .now)
                // LiveCommunicationKit lists an outgoing conversation only once its Start is fulfilled (#9887).
                let known = self?.knows(id) == true
                Self.log.notice("call \(id.uuidString, privacy: .public): Start fulfilled, the system \(known ? "has" : "does not have yet", privacy: .public) the conversation")
            }
        case let a as JoinConversationAction:
            kind = .join
            fulfil = { a.fulfill(dateConnected: .now) }
        case let a as EndConversationAction:
            kind = .end
            // Fulfilled only once the engine stopped (`CallMachine`, #12341). A second later the system's list says
            // whether it let the conversation go: only read, never ended or reported again.
            fulfil = { [weak self] in
                a.fulfill(dateEnded: .now)
                Self.log.notice("call \(id.uuidString, privacy: .public): End fulfilled")
                Task { [weak self] in
                    try? await Task.sleep(for: .seconds(1))
                    self?.logSnapshot(id, at: "after End")
                }
            }
        case let a as MuteConversationAction:
            kind = .mute(a.isMuted)
            fulfil = { a.fulfill() }
        case let a as PauseConversationAction:
            kind = .pause(a.isPaused)
            fulfil = { a.fulfill() }
        default:
            kind = .other
            fulfil = { action.fulfill() }
        }
        Self.log.notice("call \(id.uuidString, privacy: .public): the system asks \(String(describing: kind), privacy: .public)\(kind == .start ? " (Start received)" : "", privacy: .public)")
        if kind == .end { logSnapshot(id, at: "before End") }
        guard let handler else { return action.fail() }
        handler.system(perform: kind, for: id, fulfil: fulfil, fail: { action.fail() })
    }

    /// One `recents:` line on the conversation `id` as the system's list has it now (its state, or `absent`, and whether
    /// it has a local member), around an End (#12341). Only this conversation: another call may rightly still be listed.
    private func logSnapshot(_ id: UUID, at point: String) {
        let conversation = manager.conversations.first { $0.uuid == id }
        let state = conversation.map { String(describing: Self.state($0.state)) } ?? "absent"
        let member = conversation.map { $0.localMember == nil ? "none" : "set" } ?? "n/a"
        Self.log.notice("recents: call \(id.uuidString, privacy: .public): \(point, privacy: .public), manager list: \(state, privacy: .public), local member: \(member, privacy: .public)")
    }

    private static func state(_ lckState: Conversation.State) -> CallSystemState {
        switch lckState {
        case .idle: .idle
        case .joining: .joining
        case .joined: .joined
        case .paused: .paused
        case .leaving: .leaving
        case .left: .left
        @unknown default: .idle
        }
    }

    fileprivate func changed(_ id: UUID, _ lckState: Conversation.State, localMember: Bool) {
        let state = Self.state(lckState)
        if state == .left {
            // The conversation is over: what the system writes into Recents is decided now.
            outgoing[id] = nil
            Self.log.notice("recents: call \(id.uuidString, privacy: .public): the system's conversation is left (local member \(localMember ? "set" : "none", privacy: .public))")
        } else {
            Self.log.notice("call \(id.uuidString, privacy: .public): the system's conversation is \(String(describing: state), privacy: .public) (local member \(localMember ? "set" : "none", privacy: .public))")
        }
        handler?.system(changed: id, state: state)
    }

    fileprivate func timedOut(_ action: ConversationAction) {
        let kind: CallSystemAction = switch action {
        case is StartConversationAction: .start
        case is JoinConversationAction: .join
        case is EndConversationAction: .end
        case let a as MuteConversationAction: .mute(a.isMuted)
        case let a as PauseConversationAction: .pause(a.isPaused)
        default: .other
        }
        Self.log.notice("call \(action.conversationUUID.uuidString, privacy: .public): the system gave up waiting for \(String(describing: kind), privacy: .public)")
        handler?.system(timedOut: kind, for: action.conversationUUID)
    }

    fileprivate func activated() {
        Self.log.notice("call: the system activated the audio session")
        handler?.systemActivatedAudio()
    }

    fileprivate func deactivated() {
        Self.log.notice("call: the system deactivated the audio session")
        handler?.systemDeactivatedAudio()
    }

    fileprivate func reset() {
        Self.log.notice("call: the conversation manager reset")
        handler?.systemReset()
    }
}

/// The conversation manager's delegate. LiveCommunicationKit calls it on its own queue, so every method is
/// `nonisolated` and hops to the main actor itself: in this main-actor-by-default target an unmarked method is the
/// main actor's, and the runtime's isolation check stops the app when the system calls it from another queue (the
/// TestFlight crash at launch of #8734's first build). Callbacks keep their order (`CallMain`).
nonisolated final class PersonaCallsLCKDelegate: ConversationManagerDelegate, @unchecked Sendable {
    /// Set once, before the manager has the delegate; read on the main actor only.
    @MainActor weak var system: LCKCallSystem?

    nonisolated func conversationManager(_ manager: ConversationManager, conversationChanged conversation: Conversation) {
        // Only Sendable values cross to the main actor: the conversation object need not be Sendable in every SDK.
        let id = conversation.uuid, lckState = conversation.state, localMember = conversation.localMember != nil
        CallMain.run { [weak self] in self?.system?.changed(id, lckState, localMember: localMember) }
    }

    nonisolated func conversationManagerDidBegin(_ manager: ConversationManager) {}

    nonisolated func conversationManagerDidReset(_ manager: ConversationManager) {
        CallMain.run { [weak self] in self?.system?.reset() }
    }

    nonisolated func conversationManager(_ manager: ConversationManager, perform action: ConversationAction) {
        nonisolated(unsafe) let action = action
        CallMain.run { [weak self] in
            guard let system = self?.system else { return action.fail() }
            system.received(action)
        }
    }

    nonisolated func conversationManager(_ manager: ConversationManager, timedOutPerforming action: ConversationAction) {
        nonisolated(unsafe) let action = action
        CallMain.run { [weak self] in self?.system?.timedOut(action) }
    }

    nonisolated func conversationManager(_ manager: ConversationManager, didActivate audioSession: AVAudioSession) {
        CallMain.run { [weak self] in self?.system?.activated() }
    }

    nonisolated func conversationManager(_ manager: ConversationManager, didDeactivate audioSession: AVAudioSession) {
        CallMain.run { [weak self] in self?.system?.deactivated() }
    }
}

// MARK: The Simulator

#if targetEnvironment(simulator)
/// The Simulator has no call service: LiveCommunicationKit ends every conversation there in about 80 ms. This stands in
/// for it: it carries each request out at once, as the system would hand it back (`perform`), and activates and
/// deactivates the audio session itself where the system would (`didActivate` / `didDeactivate`). `-showSystemCallService
/// YES` takes the real one instead, to see what it does.
@MainActor final class SimulatedCallSystem: CallSystem {
    private static let log = Logger(subsystem: "persona", category: "call")
    weak var handler: (any CallSystemHandler)?
    private var known: Set<UUID> = []
    /// The call whose audio the session is: the one not paused.
    private var active: UUID?

    func start(_ id: UUID, handle: CallHandle) async throws {
        #if DEBUG
        // `-showCallFallback YES`: the Start refused, as a device that will not place the call refuses it, so the voice
        // chat in the app takes over (#9887 one button: the fallback's own path).
        if UserDefaults.standard.bool(forKey: "showCallFallback") {
            Self.log.notice("call \(id.uuidString, privacy: .public): the stand-in refuses the Start (-showCallFallback)")
            throw NSError(domain: CallSystemError.requestTransaction, code: 1)
        }
        #endif
        known.insert(id)
        Self.log.notice("call \(id.uuidString, privacy: .public): the stand-in starts it")
        await Task.yield() // `perform` returns before the delegate hears the action, as on a device
        deliver(.start, id)
        await activate(id)
    }

    func reportIncoming(_ id: UUID, name: String, title: String) async throws {
        known.insert(id)
        Self.log.notice("call \(id.uuidString, privacy: .public): the stand-in rings (silently)")
    }

    func perform(_ request: CallRequest, for id: UUID) async throws {
        guard known.contains(id) else { throw CallSystemStandInError.unknownCall }
        switch request {
        case .join:
            deliver(.join, id)
            await activate(id)
        case .end:
            deliver(.end, id)
            known.remove(id)
            handler?.system(changed: id, state: .left)
            if active == id { await deactivate() }
        case .mute(let on):
            deliver(.mute(on), id)
        case .pause(let on):
            deliver(.pause(on), id)
            if on, active == id { await deactivate() } else if !on { await activate(id) }
        }
    }

    func report(_ report: CallReport, for id: UUID) {
        guard known.contains(id) else { return }
        if case .ended = report {
            known.remove(id)
            if active == id { Task { await deactivate() } }
        }
    }

    func knows(_ id: UUID) -> Bool { known.contains(id) }

    func activateAudio() async -> Bool { await Self.setActive(true) }

    private func deliver(_ action: CallSystemAction, _ id: UUID) {
        handler?.system(perform: action, for: id, fulfil: {}, fail: {
            Self.log.notice("call \(id.uuidString, privacy: .public): the app failed \(String(describing: action), privacy: .public)")
        })
    }

    private func activate(_ id: UUID) async {
        guard known.contains(id), active != id else { return }
        active = id
        guard await Self.setActive(true), active == id else { return }
        handler?.systemActivatedAudio()
    }

    private func deactivate() async {
        active = nil
        _ = await Self.setActive(false)
        handler?.systemDeactivatedAudio()
    }

    /// Off the main actor: activating and deactivating the session block for tens of milliseconds.
    private static func setActive(_ on: Bool) async -> Bool {
        await Task.detached {
            do {
                try AVAudioSession.sharedInstance().setActive(on, options: on ? [] : .notifyOthersOnDeactivation)
                return true
            } catch {
                Logger(subsystem: "persona", category: "call").notice("call: the stand-in could not \(on ? "activate" : "deactivate", privacy: .public) the audio session: \(error.localizedDescription, privacy: .public)")
                return false
            }
        }.value
    }
}

nonisolated enum CallSystemStandInError: Error {
    case unknownCall
}
#endif
#endif
