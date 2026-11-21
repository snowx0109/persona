import Foundation
import Testing
@testable import Persona

/// One system call's rules (#9290, `CallMachine`), for a call to Persona and an urgent call alike: what the system's
/// actions, the audio, the voice and the page do to it, and what it asks `PersonaCalls` to do. Pure: no system, no audio.
struct CallMachineTests {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    private func at(_ seconds: TimeInterval) -> Date { t0.addingTimeInterval(seconds) }

    private func owner() -> CallMachine { CallMachine(id: UUID(), kind: .owner(group: "g"), direction: .outgoing) }

    private func urgent() -> CallMachine { CallMachine(id: UUID(), kind: .urgent(callID: "call_1"), direction: .incoming) }

    /// An urgent call answered, with its audio on.
    private func answered() -> CallMachine {
        var m = urgent()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.joinAction, at: at(0))
        _ = m.handle(.audioActivated, at: at(0.2))
        _ = m.handle(.engineRunning, at: at(0.3))
        return m
    }

    /// A call to Persona the system started and knows, with its audio on and the voice ready.
    private func connected() -> CallMachine {
        var m = owner()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.startAction, at: at(0))
        _ = m.handle(.audioActivated, at: at(0.2))
        _ = m.handle(.engineRunning, at: at(0.3))
        _ = m.handle(.voiceReady, at: at(1))
        return m
    }

    private func reports(_ effects: [CallEffect]) -> [CallReport] {
        effects.compactMap { if case .report(let r) = $0 { r } else { nil } }
    }

    // MARK: Start and connect

    @Test func startIsFulfilledThenReportedRightAwayWhenTheSystemKnowsTheConversation() {
        var m = owner()
        #expect(m.phase == .dialing)
        _ = m.handle(.known, at: at(0))
        let effects = m.handle(.startAction, at: at(1))
        #expect(effects.first == .fulfil)
        #expect(reports(effects) == [.members, .startedConnecting(at(1))], "who the call is with first (#9887), then connecting")
        #expect(effects.contains(.begin), "the voice starts with the call")
        #expect(effects.contains(.schedule(.activation)))
        #expect(!effects.contains(.audioOn), "no audio before the system activates it")
        #expect(m.phase == .connecting)
    }

    @Test func reportsBeforeTheSystemKnowsTheConversationWaitForIt() {
        var m = owner()
        let effects = m.handle(.startAction, at: at(1))
        #expect(effects.first == .fulfil)
        #expect(reports(effects).isEmpty, "the system drops events for a conversation it does not have yet")
        #expect(reports(m.handle(.known, at: at(1.1))) == [.members, .startedConnecting(at(1))], "in the same order once known")
        #expect(reports(m.handle(.known, at: at(1.2))).isEmpty, "said once")
    }

    @Test func conversationJoinedReReportsMembersForAnOutgoingCall() {
        var m = owner()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.startAction, at: at(0))
        #expect(reports(m.handle(.conversationJoined, at: at(0.1))) == [.members], "re-reported once the conversation joined (#10495)")
    }

    /// One members report per joined call (#12341): the update itself can bring another `joined` callback.
    @Test func conversationJoinedReReportsMembersOnlyOnce() {
        var m = owner()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.startAction, at: at(0))
        #expect(reports(m.handle(.conversationJoined, at: at(0.1))) == [.members])
        #expect(m.handle(.conversationJoined, at: at(0.2)).isEmpty, "a later joined callback reports nothing again")
    }

    /// The End's fulfil comes after the engine stopped, for every live call (#12341); an End after the call ended is
    /// fulfilled at once and stops nothing.
    @Test func anEndStopsTheEngineBeforeItIsFulfilled() {
        var dialling = owner()
        _ = dialling.handle(.known, at: at(0))
        #expect(dialling.handle(.endAction, at: at(1)) == [.endEngine, .fulfil])
        var m = connected()
        #expect(m.handle(.endAction, at: at(30)) == [.endEngine, .fulfil])
        #expect(m.handle(.endAction, at: at(31)) == [.fulfil])
    }

    @Test func conversationJoinedDoesNothingForAnIncomingCall() {
        var m = urgent()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.joinAction, at: at(0))
        #expect(m.handle(.conversationJoined, at: at(0.1)).isEmpty, "an incoming call's members went with its ring")
    }

    @Test func conversationJoinedDoesNothingBeforeKnown() {
        var m = owner()
        #expect(m.handle(.conversationJoined, at: at(0)).isEmpty, "not known yet: nothing to re-report")
    }

    @Test func aCallToPersonaConnectsOnTheVoicesFirstReady() {
        var m = owner()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.startAction, at: at(0))
        #expect(reports(m.handle(.voiceReady, at: at(2))) == [.connected(at(2))])
        #expect(m.phase == .connected(since: at(2)))
        #expect(m.handle(.voiceReady, at: at(9)).isEmpty, "a reconnect's ready connects nothing again")
    }

    @Test func anUrgentCallConnectsOnJoinAndBeginsBeforeItIsFulfilled() {
        var m = urgent()
        #expect(m.phase == .ringing)
        let effects = m.handle(.joinAction, at: at(3))
        #expect(effects.prefix(2) == [.begin, .fulfil], "answering ends a voice chat before the system activates the session")
        #expect(m.phase == .connected(since: at(3)))
        #expect(m.handle(.joinAction, at: at(4)) == [.fulfil])
    }

    // MARK: Audio

    @Test func theEngineStartsOnlyOnActivationAndDeactivationNeverEndsTheCall() {
        var m = connected()
        #expect(m.audioActive)
        #expect(m.handle(.audioDeactivated, at: at(5)).first == .audioOff)
        #expect(m.phase.isLive)
        #expect(m.handle(.audioActivated, at: at(6)) == [.audioOn, .hold(false)])
        #expect(m.handle(.audioActivated, at: at(6.1)).isEmpty, "once per activation")
    }

    /// `didDeactivate` alone, outside a pause and with no interruption to say why: held as for an interruption (the
    /// engine of either kind holds), the recovery asks for the audio back, and Resume is offered at last; the call is
    /// never left connected and silent.
    @Test func aDeactivationAloneHoldsTheCallAndRecovers() {
        for start in [connected(), answered()] {
            var m = start
            #expect(m.handle(.audioDeactivated, at: at(10)) == [.audioOff, .hold(true), .schedule(.recoveryUnpause), .schedule(.recoveryGiveUp)])
            #expect(m.held && m.holdReason == .interruption && m.phase.isLive)
            let unpause = m.handle(.timer(.recoveryUnpause), at: at(11))
            #expect(unpause.contains(.performUnpause) && unpause.contains(.schedule(.recoveryActivate)))
            #expect(m.handle(.timer(.recoveryActivate), at: at(11.5)) == [.activateSession(1)])
            _ = m.handle(.sessionActivationFailed(1), at: at(11.6))
            #expect(m.phase.isLive, "a refused activation is no reason to end a call that had its audio")
            _ = m.handle(.timer(.recoveryGiveUp), at: at(15))
            #expect(m.resumeOffered && m.held)
            #expect(m.handle(.audioActivated, at: at(20)) == [.audioOn, .hold(false)])
            #expect(!m.held && !m.resumeOffered)
        }
        // A deactivation the system's pause explains holds nothing more.
        var paused = connected()
        _ = paused.handle(.pauseAction(true), at: at(10))
        #expect(paused.handle(.audioDeactivated, at: at(10.1)) == [.audioOff] && paused.holds == [.system])
        // Nor one of a call still dialling.
        var dialling = owner()
        _ = dialling.handle(.audioActivated, at: at(0.1))
        #expect(dialling.handle(.audioDeactivated, at: at(0.2)) == [.audioOff] && !dialling.held)
    }

    @Test func noActivationActivatesOnceAndThenFails() {
        var m = owner()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.startAction, at: at(0))
        let first = m.handle(.timer(.activation), at: at(5))
        #expect(first.contains(.activateSession(1)))
        #expect(first.contains { if case .log = $0 { true } else { false } }, "a device-checklist line")
        #expect(!m.handle(.timer(.activation), at: at(6)).contains(.activateSession(2)), "once")
        let failed = m.handle(.sessionActivationFailed(1), at: at(6))
        #expect(m.phase == .ended(.failed))
        #expect(reports(failed) == [.ended(at(6), .failed)])
        #expect(failed.contains(.endEngine))
    }

    @Test func theAppsOwnActivationStartsTheEngine() {
        var m = owner()
        _ = m.handle(.startAction, at: at(0))
        _ = m.handle(.timer(.activation), at: at(5))
        #expect(m.handle(.sessionActivated(1), at: at(5.1)).contains(.audioOn))
        #expect(m.handle(.timer(.activation), at: at(9)).isEmpty)
    }

    /// The system holds the call before its audio ever came (another call took it right after the Start): the watchdog
    /// waits, so it neither takes the other call's session nor fails a call nothing is wrong with, and runs again from
    /// the resume.
    @Test func theWatchdogWaitsWhileTheSystemHoldsTheCall() {
        var m = owner()
        _ = m.handle(.known, at: at(0))
        _ = m.handle(.startAction, at: at(0))
        #expect(m.handle(.pauseAction(true), at: at(1)) == [.fulfil, .hold(true)])
        #expect(m.handle(.timer(.activation), at: at(5)).isEmpty, "held: no activation of its own")
        #expect(m.handle(.pauseAction(false), at: at(20)) == [.fulfil, .schedule(.activation)], "the watchdog again")
        #expect(m.phase.isLive && !m.held)
        #expect(m.handle(.timer(.activation), at: at(25)).contains(.activateSession(1)))
        #expect(m.handle(.audioActivated, at: at(25.1)) == [.audioOn, .hold(false)])
        // An interruption before the audio came holds the watchdog too.
        var interrupted = owner()
        _ = interrupted.handle(.startAction, at: at(0))
        _ = interrupted.handle(.interruptionBegan, at: at(1))
        #expect(interrupted.handle(.timer(.activation), at: at(5)).isEmpty)
    }

    /// The app's own activation asked before a hold or an interruption came: its result is not the call's any more. A
    /// refusal does not end the held call; a success does not start its engine.
    @Test func anActivationAskedBeforeAHoldCountsForNothing() {
        for success in [false, true] {
            var m = owner()
            _ = m.handle(.startAction, at: at(0))
            #expect(m.handle(.timer(.activation), at: at(5)).contains(.activateSession(1)))
            _ = m.handle(.pauseAction(true), at: at(5.05))
            let result = m.handle(success ? .sessionActivated(1) : .sessionActivationFailed(1), at: at(5.1))
            #expect(!result.contains(.audioOn) && !result.contains(.endEngine), "success \(success)")
            #expect(m.phase.isLive && m.held && !m.audioActive, "success \(success)")
        }
        var interrupted = connected()
        _ = interrupted.handle(.audioDeactivated, at: at(10))
        _ = interrupted.handle(.timer(.recoveryUnpause), at: at(11))
        #expect(interrupted.handle(.timer(.recoveryActivate), at: at(11.5)) == [.activateSession(1)])
        _ = interrupted.handle(.interruptionBegan, at: at(11.55))
        #expect(!interrupted.handle(.sessionActivated(1), at: at(11.6)).contains(.audioOn), "the interruption came since")
    }

    /// The recovery asked the system to resume, and the system paused the call meanwhile (another call): it does not
    /// activate the session the other call has, nor offer Resume; the system's resume starts it again.
    @Test func theRecoveryWaitsWhileTheSystemHoldsTheCall() {
        var m = connected()
        _ = m.handle(.interruptionBegan, at: at(10))
        _ = m.handle(.interruptionEnded, at: at(20))
        #expect(m.handle(.timer(.recoveryUnpause), at: at(21)).contains(.schedule(.recoveryActivate)))
        _ = m.handle(.pauseAction(true), at: at(21.2))
        #expect(m.handle(.timer(.recoveryActivate), at: at(21.5)).isEmpty)
        #expect(m.handle(.timer(.recoveryGiveUp), at: at(25)).isEmpty && !m.resumeOffered)
        #expect(m.handle(.pauseAction(false), at: at(30)) == [.fulfil, .schedule(.recoveryUnpause), .schedule(.recoveryGiveUp)])
    }

    /// The watchdog's own activation and the system's late `didActivate` for the same session, in either order: the
    /// engine starts once, and half a second after it runs a system mute (its call UI, AirPods) is the owner's again.
    @Test func aSecondWordOfTheSameActivationKeepsTheMuteWindowItHad() {
        for words in [[CallEvent.sessionActivated(1), .audioActivated], [.audioActivated, .sessionActivated(1)]] {
            var m = owner()
            _ = m.handle(.known, at: at(0))
            _ = m.handle(.startAction, at: at(0))
            _ = m.handle(.timer(.activation), at: at(5))
            #expect(m.handle(words[0], at: at(5.1)).contains(.audioOn))
            _ = m.handle(.engineRunning, at: at(5.2))
            #expect(!m.handle(words[1], at: at(5.3)).contains(.audioOn), "\(words): one engine")
            #expect(m.handle(.muteAction(true), at: at(6)) == [.fulfil, .mute(true)], "\(words): the owner's mute")
            #expect(m.muted)
        }
    }

    @Test func anActivationBeforeTheStartIsKept() {
        var m = owner()
        #expect(m.handle(.audioActivated, at: at(0.1)) == [.audioOn])
        let start = m.handle(.startAction, at: at(0.2))
        #expect(start.contains(.begin) && !start.contains(.schedule(.activation)), "the audio is there: no watchdog")
    }

    // MARK: Mute

    @Test func muteIsIdempotentByValue() {
        var m = connected()
        let page = m.handle(.muteTapped(true), at: at(10))
        #expect(page == [.performMute(true)], "the page's mute goes through the system")
        #expect(!m.muted)
        #expect(m.handle(.muteAction(true), at: at(10.01)) == [.fulfil, .mute(true)])
        #expect(m.handle(.muteAction(true), at: at(10.02)) == [.fulfil], "the system's echo 8–17 ms later")
        #expect(m.muted)
        #expect(m.handle(.muteAction(false), at: at(20)) == [.fulfil, .mute(false)], "the system's own (AirPods)")
        _ = m.handle(.endAction, at: at(30))
        #expect(m.handle(.muteAction(true), at: at(30.1)) == [.fulfil], "an unmute or mute after the end changes nothing")
    }

    @Test func aMuteTheSystemRefusedAppliesHere() {
        var m = connected()
        _ = m.handle(.muteTapped(true), at: at(10))
        #expect(m.handle(.muteRefused(true), at: at(10.1)) == [.mute(true)])
        #expect(m.muted)
        #expect(m.handle(.muteRefused(true), at: at(10.2)).isEmpty)
    }

    /// Mute while it dials, before the system has the conversation (it refuses the mute): applied here, and asked of the
    /// system again once it has the conversation, whose echo changes nothing. Unmuted, the system is asked nothing.
    @Test func aMuteBeforeTheSystemHasTheCallReachesItOnceItDoes() {
        var m = owner()
        #expect(m.handle(.muteTapped(true), at: at(0)) == [.performMute(true)])
        #expect(m.handle(.muteRefused(true), at: at(0.1)) == [.mute(true)])
        #expect(m.muted)
        _ = m.handle(.startAction, at: at(1))
        let known = m.handle(.known, at: at(1.1))
        #expect(reports(known) == [.members, .startedConnecting(at(1))] && known.last == .performMute(true))
        #expect(m.handle(.muteAction(true), at: at(1.12)) == [.fulfil], "its echo")
        #expect(m.muted && m.requestedMute == nil)
        #expect(m.handle(.known, at: at(1.2)).isEmpty, "said once")

        var unmuted = owner()
        _ = unmuted.handle(.startAction, at: at(1))
        #expect(!unmuted.handle(.known, at: at(1.1)).contains(.performMute(true)))
    }

    @Test func aSystemMuteWhileTheEngineStartsIsIgnoredAndTheOwnersMuteAssertedOnce() {
        var m = owner()
        _ = m.handle(.startAction, at: at(0))
        _ = m.handle(.audioActivated, at: at(0.2))
        #expect(m.handle(.muteAction(true), at: at(0.25)).contains(.performMute(false)), "voice processing starting shows as a mute")
        #expect(!m.muted)
        _ = m.handle(.engineRunning, at: at(0.4))
        #expect(m.handle(.muteAction(true), at: at(0.6)) == [.fulfil, .log("a system mute while the engine started, ignored")],
                "asserted once")
        #expect(!m.muted)
        #expect(m.handle(.muteAction(true), at: at(1.0)) == [.fulfil, .mute(true)], "half a second after the engine runs it is the owner's")
    }

    /// Every new engine's startup mute is corrected, not only the first's: after an interruption or a hold brought the
    /// audio back, and after a route change rebuilt the engine. A second word of the same activation corrects nothing more.
    @Test func theStartupMuteIsCorrectedForEveryNewEngine() {
        var m = connected()
        #expect(m.handle(.muteAction(true), at: at(0.5)).contains(.performMute(false)), "the first engine's")
        _ = m.handle(.interruptionBegan, at: at(10))
        _ = m.handle(.interruptionEnded, at: at(20))
        _ = m.handle(.audioActivated, at: at(20.1))
        #expect(m.handle(.muteAction(true), at: at(20.2)).contains(.performMute(false)), "the engine after the interruption")
        _ = m.handle(.engineRunning, at: at(20.3))
        _ = m.handle(.pauseAction(true), at: at(30))
        _ = m.handle(.audioDeactivated, at: at(30.1))
        _ = m.handle(.pauseAction(false), at: at(40))
        _ = m.handle(.audioActivated, at: at(40.1))
        #expect(m.handle(.muteAction(true), at: at(40.2)).contains(.performMute(false)), "the engine after the hold")
        #expect(m.handle(.audioActivated, at: at(40.25)).isEmpty)
        #expect(m.handle(.muteAction(true), at: at(40.3)) == [.fulfil, .log("a system mute while the engine started, ignored")],
                "once per engine")
        _ = m.handle(.engineRunning, at: at(40.4))
        // A route change rebuilt the engine: voice processing on again.
        #expect(m.handle(.engineStarting, at: at(50)).isEmpty)
        #expect(m.handle(.muteAction(true), at: at(50.1)).contains(.performMute(false)), "the rebuilt engine's")
        _ = m.handle(.engineRunning, at: at(50.2))
        #expect(m.handle(.muteAction(true), at: at(51)) == [.fulfil, .mute(true)], "the owner's once it runs")
        #expect(m.muted)
    }

    // MARK: Hold

    @Test func aPauseIsFulfilledAtOnceAndHoldsTheCall() {
        var m = connected()
        let paused = m.handle(.pauseAction(true), at: at(10))
        #expect(paused == [.fulfil, .hold(true)])
        #expect(m.held && m.holdReason == .system)
        #expect(m.handle(.audioDeactivated, at: at(10.2)) == [.audioOff])
        // Resumed: on hold until the audio is the call's again, which the recovery asks for if the system does not say so.
        #expect(m.handle(.pauseAction(false), at: at(20)) == [.fulfil, .schedule(.recoveryUnpause), .schedule(.recoveryGiveUp)])
        #expect(m.held && m.holdReason == .interruption)
        #expect(m.handle(.audioActivated, at: at(20.2)) == [.audioOn, .hold(false)])
        #expect(!m.held)
        #expect(m.handle(.timer(.recoveryUnpause), at: at(21)).isEmpty, "nothing left to recover")
        #expect(m.handle(.pauseAction(false), at: at(22)) == [.fulfil], "a resume of a call not held changes nothing")
    }

    /// The system resumed the call (our other call ended: Hold & Accept's urgent call) but never activated the session
    /// for it again (the other call had it active): the call stays on hold, the recovery asks the system and then
    /// activates the session itself, and Resume is offered if even that does not bring the audio back.
    @Test func aResumeWithoutTheSystemsActivationRecoversTheAudio() {
        for activates in [true, false] {
            var m = connected()
            _ = m.handle(.pauseAction(true), at: at(10))
            _ = m.handle(.audioDeactivated, at: at(10.2))
            _ = m.handle(.pauseAction(false), at: at(20))
            let unpause = m.handle(.timer(.recoveryUnpause), at: at(21))
            #expect(unpause.contains(.performUnpause) && unpause.contains(.schedule(.recoveryActivate)))
            #expect(m.handle(.timer(.recoveryActivate), at: at(21.5)) == [.activateSession(1)])
            if activates {
                #expect(m.handle(.sessionActivated(1), at: at(21.6)) == [.log("the call's audio session activated by the app"), .audioOn, .hold(false)])
                #expect(!m.held && m.audioActive)
            } else {
                _ = m.handle(.sessionActivationFailed(1), at: at(21.6))
                _ = m.handle(.timer(.recoveryGiveUp), at: at(25))
                #expect(m.resumeOffered && m.held && m.phase.isLive)
                let resume = m.handle(.resumeTapped, at: at(30))
                #expect(resume.first == .performUnpause && resume.contains(.schedule(.recoveryActivate)))
                #expect(m.handle(.timer(.recoveryActivate), at: at(30.5)) == [.activateSession(2)])
                #expect(m.handle(.sessionActivated(2), at: at(30.6)).contains(.audioOn) && !m.held)
            }
        }
    }

    /// An interruption that ended while the system held the call: the system's resume starts the recovery, so the
    /// audio comes back or Resume is offered.
    @Test func anInterruptionThatEndedWhileHeldRecoversOnTheResume() {
        var m = connected()
        _ = m.handle(.pauseAction(true), at: at(10))
        _ = m.handle(.interruptionBegan, at: at(10.1))
        #expect(m.handle(.interruptionEnded, at: at(15)).isEmpty)
        #expect(m.handle(.pauseAction(false), at: at(20)) == [.fulfil, .schedule(.recoveryUnpause), .schedule(.recoveryGiveUp)])
        _ = m.handle(.timer(.recoveryUnpause), at: at(21))
        _ = m.handle(.timer(.recoveryActivate), at: at(21.5))
        _ = m.handle(.sessionActivationFailed(1), at: at(21.6))
        _ = m.handle(.timer(.recoveryGiveUp), at: at(25))
        #expect(m.resumeOffered && m.held)
    }

    @Test func leavingHoldWaitsForEveryReason() {
        var m = connected()
        _ = m.handle(.pauseAction(true), at: at(10))
        _ = m.handle(.interruptionBegan, at: at(10.1))
        #expect(m.holds == [.system, .interruption])
        _ = m.handle(.audioDeactivated, at: at(10.2))
        #expect(m.handle(.interruptionEnded, at: at(11)).isEmpty, "still paused by the system: its resume brings the audio")
        #expect(m.handle(.pauseAction(false), at: at(12)) == [.fulfil, .schedule(.recoveryUnpause), .schedule(.recoveryGiveUp)])
        #expect(m.held, "the interruption still holds it")
        #expect(m.handle(.audioActivated, at: at(12.2)) == [.audioOn, .hold(false)])
        #expect(!m.held)
    }

    // MARK: Interruptions

    @Test func anInterruptionHoldsAndNeverEndsTheCall() {
        var m = connected()
        #expect(m.handle(.interruptionBegan, at: at(10)) == [.audioOff, .hold(true)])
        #expect(m.holdReason == .interruption && !m.audioActive)
        _ = m.handle(.audioDeactivated, at: at(10.1))
        #expect(m.handle(.interruptionEnded, at: at(40)) == [.schedule(.recoveryUnpause), .schedule(.recoveryGiveUp)])
        let unpause = m.handle(.timer(.recoveryUnpause), at: at(41))
        #expect(unpause.contains(.performUnpause) && unpause.contains(.schedule(.recoveryActivate)))
        #expect(m.handle(.timer(.recoveryActivate), at: at(41.5)) == [.activateSession(1)])
        _ = m.handle(.sessionActivationFailed(1), at: at(41.6))
        #expect(m.phase.isLive, "a failed activation after an interruption is no reason to end")
        _ = m.handle(.timer(.recoveryGiveUp), at: at(45))
        #expect(m.resumeOffered && m.held && m.phase.isLive)
        // Resume asks again.
        let resume = m.handle(.resumeTapped, at: at(60))
        #expect(resume.first == .performUnpause && resume.contains(.schedule(.recoveryActivate)))
        #expect(!m.resumeOffered)
        #expect(m.handle(.audioActivated, at: at(60.3)) == [.audioOn, .hold(false)])
    }

    @Test func theSystemsActivationEndsTheInterruption() {
        var m = connected()
        _ = m.handle(.interruptionBegan, at: at(10))
        _ = m.handle(.audioDeactivated, at: at(10.1))
        _ = m.handle(.interruptionEnded, at: at(20))
        #expect(m.handle(.audioActivated, at: at(20.3)) == [.audioOn, .hold(false)])
        #expect(m.handle(.timer(.recoveryUnpause), at: at(21)).isEmpty, "nothing left to recover")
        #expect(m.handle(.timer(.recoveryGiveUp), at: at(25)).isEmpty)
        #expect(!m.resumeOffered)
    }

    /// The interruption's notifications alone, with no `didDeactivate` from the system: the audio is gone all the same,
    /// so the recovery runs and the system's next activation starts the engine again.
    @Test func anInterruptionWithoutADeactivationStillTakesTheAudio() {
        var back = connected()
        #expect(back.handle(.interruptionBegan, at: at(10)) == [.audioOff, .hold(true)])
        #expect(back.handle(.audioDeactivated, at: at(10.1)).isEmpty, "a late didDeactivate changes nothing more")
        _ = back.handle(.interruptionEnded, at: at(20))
        #expect(back.handle(.audioActivated, at: at(20.3)) == [.audioOn, .hold(false)], "the engine starts again")
        #expect(!back.held && back.audioActive)

        var gone = connected()
        _ = gone.handle(.interruptionBegan, at: at(10))
        _ = gone.handle(.interruptionEnded, at: at(20))
        let unpause = gone.handle(.timer(.recoveryUnpause), at: at(21))
        #expect(unpause.contains(.performUnpause) && unpause.contains(.schedule(.recoveryActivate)), "the recovery runs")
        #expect(gone.handle(.timer(.recoveryActivate), at: at(21.5)) == [.activateSession(1)])
        _ = gone.handle(.sessionActivationFailed(1), at: at(21.6))
        _ = gone.handle(.timer(.recoveryGiveUp), at: at(25))
        #expect(gone.resumeOffered && gone.held && gone.phase.isLive)
        _ = gone.handle(.resumeTapped, at: at(30))
        #expect(gone.handle(.timer(.recoveryActivate), at: at(30.5)) == [.activateSession(2)])
        #expect(gone.handle(.sessionActivated(2), at: at(30.6)) == [.log("the call's audio session activated by the app"), .audioOn, .hold(false)])
    }

    /// Half a second after the engine starts again after an interruption, a system mute is the owner's.
    @Test func theMuteWindowOpensAgainOnlyForTheEngineStartingAgain() {
        var m = connected()
        _ = m.handle(.interruptionBegan, at: at(10))
        _ = m.handle(.interruptionEnded, at: at(20))
        _ = m.handle(.audioActivated, at: at(20.1))
        #expect(m.handle(.muteAction(true), at: at(20.2)).contains(.performMute(false)), "the engine starting again")
        _ = m.handle(.engineRunning, at: at(20.3))
        #expect(m.handle(.muteAction(true), at: at(21)) == [.fulfil, .mute(true)])
    }

    @Test func aRingOrADialHasNoAudioToInterrupt() {
        var m = urgent()
        #expect(m.handle(.interruptionBegan, at: at(1)).isEmpty)
        var o = owner()
        #expect(o.handle(.interruptionBegan, at: at(1)).isEmpty)
    }

    // MARK: Ending

    @Test func theSystemsEndEndsTheEngineAndIsNotReportedBack() {
        var m = connected()
        let effects = m.handle(.endAction, at: at(30))
        #expect(effects == [.endEngine, .fulfil])
        #expect(m.phase == .ended(.hungUp))
        #expect(m.handle(.engineEnded(failed: false), at: at(30.1)).isEmpty, "the engine's own end after it: once")
        #expect(m.handle(.endAction, at: at(31)) == [.fulfil])
    }

    @Test func theEngineEndingByItselfIsReported() {
        var lost = connected()
        #expect(lost.handle(.engineEnded(failed: true), at: at(30)) == [.report(.ended(at(30), .failed))])
        #expect(lost.phase == .ended(.failed))
        var idle = connected()
        #expect(idle.handle(.engineEnded(failed: false), at: at(30)) == [.report(.ended(at(30), .remoteEnded))])
        #expect(idle.handle(.endAction, at: at(31)) == [.fulfil], "the system's End after it ends nothing again")
    }

    @Test func hangUpGoesThroughTheSystemAndEndsHereIfItRefuses() {
        var m = connected()
        #expect(m.handle(.hangUpTapped, at: at(30)) == [.performEnd])
        #expect(m.phase.isLive)
        let refused = m.handle(.hangUpRefused, at: at(30.1))
        #expect(reports(refused) == [.ended(at(30.1), .remoteEnded)] && refused.contains(.endEngine))
        #expect(m.phase == .ended(.hungUp))
    }

    @Test func aDeclinedRingAndARingSettledElsewhere() {
        var declined = urgent()
        #expect(declined.handle(.endAction, at: at(5)) == [.endEngine, .fulfil])
        #expect(declined.phase == .ended(.declined))
        var missed = urgent()
        _ = missed.handle(.known, at: at(0))
        #expect(missed.handle(.settled(.unanswered), at: at(45)) == [.report(.ended(at(45), .unanswered))])
        var elsewhere = urgent()
        _ = elsewhere.handle(.known, at: at(0))
        #expect(reports(elsewhere.handle(.settled(.answeredElsewhere), at: at(9))) == [.ended(at(9), .joinedElsewhere)])
    }

    @Test func anEndBeforeTheSystemKnowsTheCallIsTheOnlyReportLeft() {
        var m = owner()
        _ = m.handle(.startAction, at: at(0))
        _ = m.handle(.engineEnded(failed: true), at: at(1))
        #expect(reports(m.handle(.known, at: at(1.1))) == [.ended(at(1), .failed)])
    }

    @Test func aResetOrTheSystemsOwnEndEndsWithoutReporting() {
        var reset = connected()
        #expect(reset.handle(.reset, at: at(9)) == [.endEngine])
        #expect(reset.phase == .ended(.failed))
        var left = connected()
        let effects = left.handle(.systemEnded, at: at(9))
        #expect(effects.contains(.endEngine) && reports(effects).isEmpty)
    }

    @Test func aStartTheSystemGaveUpOnFails() {
        var m = owner()
        let effects = m.handle(.actionTimedOut, at: at(30))
        #expect(m.phase == .ended(.failed) && effects.contains(.endEngine))
    }

    @Test func anActionItDoesNotTakeIsFailed() {
        var m = connected()
        #expect(m.handle(.otherAction, at: at(1)) == [.fail])
    }

    @Test func whatTheSystemHearsForEachEnding() {
        #expect(CallEndReason.failed.report == .failed)
        #expect(CallEndReason.hungUp.report == .remoteEnded)
        #expect(CallEndReason.remoteEnded.report == .remoteEnded)
        #expect(CallEndReason.unanswered.report == .unanswered)
        #expect(CallEndReason.answeredElsewhere.report == .joinedElsewhere)
        #expect(CallEndReason.declinedElsewhere.report == .declinedElsewhere)
    }
}
