import Foundation
import LiveCommunicationKit
import Testing
@testable import Persona

/// Outgoing calls in Phone › Recents (#9887, docs/apple/calls.md "Recents and intents"): the system writes a call into
/// Recents for its members, so an outgoing call reports who it is with (the owner as the local member, the call's handle
/// as the remote one) once the system has the conversation, before it says it connects. Developing Persona's Call handle
/// picks the handle's kind for the on-device experiment; the manager declares both kinds from the start.
@MainActor
struct CallRecentsTests {
    /// The members update goes first, then startedConnecting, both only once the system has the conversation (it drops
    /// reports before), and the voice's first ready reports connected after them. When the conversation joins, the
    /// roster is sent once more, even if the update causes another callback with the same state.
    @Test func anOutgoingCallReportsItsMembersBeforeItConnects() async throws {
        let rig = CallRig()
        rig.system.knowsAtOnce = false
        let call = await rig.dial()
        rig.system.act(.start, call.id)
        #expect(rig.system.fulfilled == [.start])
        #expect(rig.system.reports.isEmpty, "nothing before the system has the conversation")
        rig.calls.system(changed: call.id, state: .joining)
        #expect(rig.system.reports == [.members, .startedConnecting(rig.voice.clock.now)])
        rig.calls.system(changed: call.id, state: .joined)
        rig.calls.system(changed: call.id, state: .joined) // The roster changed; the state didn't.
        #expect(rig.system.reports.filter { $0 == .members }.count == 2, "said again once the conversation joined (#10495)")
        await rig.voice.waitUntil { rig.voice.socket != nil }
        rig.system.activate()
        let socket = try #require(rig.voice.socket)
        socket.ready(rig.voice.key)
        #expect(rig.system.reports.last.map { if case .connected = $0 { true } else { false } } == true)
    }

    /// Known right after the Start's fulfil (the system lists the conversation then): the same order at once; when the
    /// conversation joins, the members are reported again so the system persists them for Recents (#10495).
    @Test func knownRightAfterTheFulfilReportsTheMembersAtOnce() async {
        let rig = CallRig()
        let call = await rig.dial()
        rig.system.act(.start, call.id)
        #expect(rig.system.reports == [.members, .startedConnecting(rig.voice.clock.now)])
        rig.calls.system(changed: call.id, state: .joined)
        #expect(rig.system.reports.filter { $0 == .members }.count == 2, "re-reported once joined")
    }

    /// The members re-report on joined is only for outgoing calls: an incoming call's members went with its ring.
    @Test func anIncomingCallDoesNotReReportMembersOnJoined() async throws {
        let rig = CallRig()
        let id = UUID()
        try await rig.calls.ring(id, callID: "call_1", name: "Urgent: the build broke")
        rig.system.act(.join, id)
        rig.calls.system(changed: id, state: .joined)
        rig.system.activate()
        #expect(!rig.system.reports.contains(.members))
    }

    /// The incoming path is unchanged: an urgent call's members go with its ring (`reportNewIncomingConversation`), and
    /// its machine reports no members update of its own.
    @Test func anIncomingCallReportsNoMembersUpdate() async throws {
        let rig = CallRig()
        let id = UUID()
        try await rig.calls.ring(id, callID: "call_1", name: "Urgent: the build broke")
        rig.system.act(.join, id)
        rig.system.activate()
        #expect(!rig.system.reports.contains(.members))
        #expect(rig.system.rings.count == 1)
    }

    /// The handle's kind follows Developing Persona's Call handle: the assistant's name as a generic handle, or the
    /// fixed placeholder address with the name as the display name.
    @Test func theHandleSwitchPicksTheKind() async {
        let generic = CallRig()
        generic.assistant = "Mochi"
        _ = await generic.dial()
        #expect(generic.system.startHandles == [CallHandle(kind: .generic, value: "Mochi", displayName: "Mochi")])

        let email = CallRig()
        email.assistant = "Mochi"
        email.handleKind = .email
        _ = await email.dial()
        #expect(email.system.startHandles == [CallHandle(kind: .email, value: CallHandle.placeholderAddress, displayName: "Mochi")])
        #expect(CallHandle.placeholderAddress.hasSuffix(".invalid"), "a reserved domain: it can never reach anyone")
        #expect(LCKCallSystem.lck(.outgoing(name: "Mochi", kind: .email)).type == .emailAddress)
        #expect(LCKCallSystem.lck(.outgoing(name: "Mochi", kind: .generic)).type == .generic)
    }

    /// The manager's configuration is fixed for its life: it declares both kinds, keeps calls in Recents, and the owner's
    /// own handle is the incoming calls' local member.
    @Test func theConfigurationListsBothKinds() {
        let config = LCKCallSystem.configuration(icon: nil)
        #expect(config.supportedHandleTypes == [.generic, .emailAddress])
        #expect(config.includesConversationInRecents)
        #expect(LCKCallSystem.owner == Handle(type: .generic, value: "owner"))
    }

    /// The switch counts only while Developing Persona is on, and is kept across launches.
    @Test func theSwitchCountsOnlyWhileDevelopingPersonaIsOn() throws {
        let suite = "persona.tests.call-handle.\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let dev = DevMode(defaults: defaults)
        #expect(dev.callHandle == .generic && dev.callHandleInUse == .generic)
        dev.setCallHandle(.email)
        #expect(dev.callHandleInUse == .generic, "off: the default")
        defaults.set(true, forKey: DevMode.onKey)
        let again = DevMode(defaults: defaults)
        #expect(again.callHandle == .email && again.callHandleInUse == .email)
    }

    /// A redial with the placeholder address calls the main conversation as any other value does.
    @Test func aRedialWithThePlaceholderIsACall() {
        let activity = NSUserActivity(activityType: "INStartAudioCallIntent")
        #expect(PersonaCalls.isCall(activity))
    }

    /// The log says each step, greppable on a device: the members update and the final `left` start with "recents:".
    @Test func theLogSaysEachStep() throws {
        let url = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Persona/Calls/CallSystem.swift")
        let system = try String(contentsOf: url, encoding: .utf8)
        for line in ["Start requested, handle", "(Start received)", "Start fulfilled, the system",
                     "recents: call \\(id.uuidString, privacy: .public): members reported",
                     "recents: call \\(id.uuidString, privacy: .public): the system's conversation is left",
                     "sent, the conversation", "dropped: the system does not have the conversation",
                     // #12341: whether the system kept a local member while the call was on, and the End around it.
                     "local member \\(conversation.localMember == nil ? \"none\" : \"set\", privacy: .public) before it",
                     "the system's conversation is \\(String(describing: state), privacy: .public) (local member",
                     "logSnapshot(id, at: \"before End\")", "logSnapshot(id, at: \"after End\")", "End fulfilled",
                     "manager list: \\(state, privacy: .public), local member: \\(member, privacy: .public)"] {
            #expect(system.contains(line), "\(line)")
        }
        #expect(system.contains("Self.outgoingMembers(remote, state: state)"), "one roster for both reports")
    }

    /// The members update's roster (#12341): the owner is the local member and never one of `members` (Apple: the remote
    /// participants, "including invited, but inactive" ones), and the assistant is invited until the conversation joins,
    /// active from then on (a pause keeps it on the call).
    @Test func theMembersUpdateNamesTheAssistantActiveOnlyOnceJoined() {
        let remote = LCKCallSystem.lck(.outgoing(name: "Mochi", kind: .generic))
        for state in [CallSystemState.idle, .joining] {
            let update = LCKCallSystem.outgoingMembers(remote, state: state)
            #expect(update.localMember == LCKCallSystem.owner)
            #expect(update.members == [remote], "the remote participants only, never the owner")
            #expect(update.activeRemoteMembers == [], "invited, not on the call yet: \(state)")
            #expect(update.capabilities == [.pausing])
        }
        for state in [CallSystemState.joined, .paused] {
            let update = LCKCallSystem.outgoingMembers(remote, state: state)
            #expect(update.members == [remote] && update.activeRemoteMembers == [remote], "on the call: \(state)")
        }
    }

    /// An End the system asks for is fulfilled only once the call's voice stopped (#12341): `fulfill(dateEnded:)` says the
    /// conversation ended for the local member, and the action stops every stream (as Apple's Speakerbox stops its audio
    /// before it fulfils `CXEndCallAction`). Nothing is reported back: the end is the system's own.
    @Test func anEndIsFulfilledOnlyOnceTheVoiceStopped() async {
        let rig = CallRig()
        let (call, socket) = await rig.connected()
        #expect(rig.voice.audio.running && !socket.closed)
        let seen = AtFulfil()
        rig.calls.system(perform: .end, for: call.id, fulfil: {
            seen.state = (socket.closed, rig.voice.audio.running, call.voice?.phase.isEnded == true)
        }, fail: { seen.failed = true })
        let state = seen.state
        #expect(state?.closed == true, "the socket closed before the fulfil")
        #expect(state?.running == false, "the microphone and the player stopped before the fulfil")
        #expect(state?.ended == true, "the voice ended before the fulfil")
        #expect(!seen.failed && call.phase == .ended(.hungUp))
        #expect(rig.system.ended.isEmpty, "the system's own end: nothing reported back")
    }
}

/// What an End's fulfil saw of the call, when it ran.
@MainActor private final class AtFulfil {
    var state: (closed: Bool, running: Bool, ended: Bool)?
    var failed = false
}
