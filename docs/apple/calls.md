# System calls on the iPhone (calling Persona, urgent calls)

Calling Persona is a phone call through the system's call UI (#9290): on the iPhone the composer's phone icon in any
conversation (#9887, "One phone button" below), a redial from the Phone app's Recents, Siri, Shortcuts or Spotlight
(`docs/apple/siri.md`, "Calling Persona"). It shows on the Lock Screen and in the Dynamic Island like any call, and it
goes on when the owner switches apps, goes Home, opens another page or locks the phone. Persona's urgent calls
(`docs/assistant/urgent-call.md`) are the same kind of system call and share everything below. The in-app voice chat
(`docs/assistant/voice.md` section 6) is what the phone icon is where a system call can't be used (the iPad, Apple
Vision Pro, a refused Start); CarPlay's voice chat (`docs/apple/carplay.md`) stays a call inside the app.

## One phone button (#9887)

Owner's principle: similar parallel entries merge into one, the system picks the kind, and switching away or locking
never stops the call.

- **The one voice entry** is the composer's phone icon (`ShellComposerBar`'s `voice`, `ComposerVoiceButton`). + › More's
  **Call Persona** and **Voice Chat** rows are gone, and with them the iPad's + menu and the iPhone's touch-and-hold
  twins (`AttachMoreSection.common`). CarPlay, Siri and App Shortcuts, Recents and the DEBUG switches stay. The icon
  shows where a call can start (`VoiceAvailability`, as before: a read-only conversation, a meeting's room or a hub
  without a usable voice extension has none); the call asks the same rules again once the hub answered and refuses with
  the same note (`VoiceUnavailableNote.make`). VoiceOver hears what a tap does there (`VoiceEntry.iconLabel`, through
  `ShellComposerBar`'s `voiceLabel` to both of `ComposerVoiceButton`'s shapes): "Call This Conversation" on the iPhone
  (asked first while a call is on elsewhere), "Show Call" in the conversation the call is with, "Start Voice Chat" where
  the icon is the voice chat in the app; the meeting's own button stays "Voice Chat".
- **On the iPhone it places a system call to the conversation it is in** (`VoiceEntry.start` → `VoiceEntry.choose` →
  `PersonaCalls.callConversation`): the main conversation, another assistant conversation or an Agent session. The call's
  kind carries the key (`CallKind.owner(group:key:)`; nil for the main conversation found as the call is placed, which is
  what Recents, Siri and CarPlay ask for), and its voice starts bound to it with surface `.call` (the `start` frame's
  `target` is that session, `"surface":"call"`).
- **One fixed handle** for every call (value: the assistant's name as the main conversation's top bar says it, "Persona"
  when it has none; `CallHandle.outgoing`, "One name" below).
  Its display name carries the conversation's title for another conversation ("Persona · <title>"); the system may show
  only the value (what Recents shows is a device-checklist line). **Recents, Siri and CarPlay always call the main
  conversation** (`callPersona`).
- **The call screen and the bar show the conversation** (`PersonaCall.conversation`, `CallConversation`, fixed when the
  call is made): for the main conversation the assistant's look and name, as before; for an Agent session its runtime's
  mark (`CallAgentFace`: Claude's, Codex's, TRAE's, else the agent's letter, on the well's disc) and its title, with
  "<agent> · <machine>" under it; for another assistant conversation the assistant's look and the conversation's title,
  with the name of the assistant it belongs to under it (`assistantShownName(in:)`: another assistant's conversation
  opened from the Inbox, a notification or a link, without switching, names its own, as its top bar does; the system's
  handle stays the selected assistant's name). The urgent call's page is always the assistant's.
- **While a call is live**: the icon in the conversation the call is with opens the call screen; in any other it asks
  "End the call with <name> and call <this conversation>?" (**End and Call**, destructive, or **Cancel**, which keeps
  the call; `VoiceUnavailableNote.callElsewhereFirst`, `VoiceProceed.callConversation`). The yes hangs the call up and
  waits (at most 2 s, `PersonaCalls.endForVoiceChat`) for it to end, which the system's End brings back a moment later
  whether or not the call had its audio yet (still dialling, or held with its audio gone), and for the system to let its
  audio go; then it places the new one (a call still on after the wait is shown instead). #9290's guards (dictation,
  recordings, disconnect) apply to every system call.
- **Fallback to the voice chat in the app**, only where a system call can't be used: a device other than the iPhone
  (the iPad and Apple Vision Pro; the Mac and the watch have no voice), the system refusing the Start (the existing
  fallback: a toast says why, `insteadLine`, and the voice chat starts on the same conversation). When the call path's
  upstream is unavailable (no voice extension, one not set up, the realtime service down), the voice chat in the app
  could not connect either: the call refuses with the existing note that says why, and no voice chat starts.
- **The fallback survives switching away** (iOS, `VoiceCalls`): the background and the lock no longer end it (it keeps
  its audio session, the app's `audio` background mode keeps it running, the microphone indicator shows; the idle rule
  stays; in the background the voice's levels stop, so only the audio and its socket work: iOS's background CPU limit);
  leaving its page keeps it, with a bar back to it at the top in the call's window (`VoiceReturnPill`,
  `voice-return-bar`; a tap goes back to its conversation and closes what covers its page, a full-screen picture or the
  terminal (`VoiceCalls.returnTo`), and the bar goes once the page shows again, `VoiceCalls.returned`); an interruption
  (a phone call, Siri) pauses it on hold and it goes on when the interruption ends with `shouldResume` and the session
  comes back (`PhoneVoiceAudio.resume`). Its group's tunnels stay open in the background while it is on
  (`AppModel.callTunnelGroup`). On Apple Vision Pro, with no bar to come back by, the background and leaving the page
  still end it.
- **Where the fallback still ends**: Stop; another call or voice chat starting (one at a time); a recording or dictation
  taking the microphone; the idle rule (ten quiet minutes with nothing handed over open and the conversation not
  working); three failed reconnects, the hub refusing the socket, an incompatible extension, an unusable model, a fatal
  error, no hub left; the audio route changing and the microphone not coming back, or the media services resetting; an
  interruption that ends without `shouldResume`, or whose session will not come back (ended with Continue, as before);
  Persona's urgent call answered; iOS ending the app (a swipe up in the app switcher, memory or background CPU limits).
- **Caveat** (documented, not changed): the idle rule does not end a call while its conversation works, so a call to an
  Agent on a long task can stay open in the background for as long as the task runs (the upstream is paused while quiet,
  so it is not billed). In the background whether it works is the voice connection's word, not the app's copy of the
  session, which is not updated there (docs/assistant/voice.md 6.5 "Idle"): the upstream paused or waking for the work,
  or a card asked by voice. Once the task is done the quiet minutes count again.

## The avatar shows each state (#9887)

The avatar on the call screen says what the call does; the avatar stays decorative, and VoiceOver hears it in words on
the state line: its value is the row (`CallAvatarLook.Row.words`: Listening, Hearing you, Thinking, Speaking, Working,
Waiting for your decision, Muted; nothing more where the line's own words say it; Working is the state's own name,
`StateName.working`, 工作中 / 作業中, #10502) beside the running time the line shows,
and "Waiting for your decision" is announced once as a card comes to wait (`CallScreenStateLine`, #9887 review); the
minimised bar's face says Thinking, Working or Waiting for your decision within the bar's words. `CallScreenState.make` derives the row from what the call screen already observes: the call's
phase and hold, the voice's phase and link, the mute, and what the call's conversation and voice do (`CallActivity`: the
session working or a run the voice handed it still open, a card waiting for the owner, the voice thinking). The first row
that applies wins: connecting, held, ended, speaking, needs you, thinking, working, muted, hearing, listening.
`CallAvatarLook.make(row, muted:, kind:, motion:)` draws it in the call's kind of picture: the main conversation's look
(the built-in Mochi, a package, an uploaded picture) or another conversation's (`other`: an Agent session's harness mark,
another assistant conversation's look).

| Row | When | Mochi (built-in) | A package / an uploaded picture | Another conversation (Agent mark, other look) |
| --- | --- | --- | --- | --- |
| Connecting | Calling…, Connecting…, Reconnecting… | idle, blinking; the ring laps | its idle frames; the ring laps | the same still picture; the ring laps |
| Listening | nobody speaks | idle, blinking; a faint ring following the microphone | its idle frames | still; the ring |
| Hearing | the owner speaks | idle; the ring swells with the microphone | its idle frames; the ring swells | still; the ring swells |
| Thinking | the owner stopped and the voice has not begun to answer (`VoiceConversation.thinking`) | thinking (eyes up, the bubble) | its `thinking` frames | still |
| Speaking | the voice plays | the shared speaking layer, its idle face with its mouth moving with the voice | its idle frames under the shared speaking layer | the same picture under the shared speaking layer |
| Working | the call's conversation works (its session, or a run sent, working, waiting outside or answered and still at work) | working (the arc, the bob, the eyes scanning) | its `working` frames | still |
| Needs you | a card in the call's conversation waits (asked by voice or not; a run or the session waiting on the owner) | needs you (the red ring, waving) | its `needs_you` frames | still |
| Held | the call's own hold (the system's pause, an interruption) | still, eyes shut; the pause mark, the caution ring | its idle frame, dimmed (`Theme.Call.heldPicture`); the pause mark, the caution ring | still; the pause mark, the caution ring |
| Muted | the owner's microphone is off | still, its state's still frame; the slashed microphone, a gray ring | its state's first frame; the mark, the gray ring | still; the mark, the gray ring |
| Ended | the call is over | asleep (`offline`), greyed; a faint ring | its `offline` frames, greyed | the same picture, greyed |

A state a package has no frames for shows its idle ones, as `PersonaDot` does (an uploaded picture usually has only idle).
Every state's frames are decoded once, off the main actor, when the call screen first shows (`CallAssistant.states`,
`PersonaStore.decodedFrames`); until then, and for a package not on the phone, the built-in drawing stands in.

- **The shared speaking layer**, the same for every call (Mochi, a package, an uploaded photo, an Agent's mark, another
  conversation's look), so a call to an Agent and one to the main conversation look alike while it speaks: the avatar
  bounces gently with the voice's output level (`CallSpeech`: the level followed as an envelope, up within
  `callSpeechAttack` and down within `callSpeechRelease`; a scale of 1.00–1.04, `callAvatarBounce`, and a lift of
  `callAvatarLift` of the box, about four points; the picture is never warped, no mouth is guessed on a photo), the ring
  brightens and swells with it, and a soft ripple leaves the avatar's edge on each peak of the voice (at most one per
  `callRippleGap`, each `callRipple` long, at most three at once). Muted, it still speaks (the owner hears it): the layer
  shows with the muted mark.
- **What stays the main conversation's**: the look's own state pictures (a package's thinking, working, needs-you and
  offline frames; Mochi's), Mochi's mouth and its shut eyes on hold, a package's dimmed idle frame on hold. A harness mark
  has none of these, so it stays the same picture; another assistant conversation's look does too.
- **Mochi's call-only drawing**: `PersonaFace.mouth` (0 a small smile … 1 wide open, its tongue in the cheek's orange,
  tilted with the eyes) and `PersonaFace.eyesShut` (the asleep face's closed eyes, without its grey). Their defaults draw
  Mochi as everywhere else, pixel for pixel (`CallAvatarStatesTests.mochisDefaultDrawingIsUnchanged`); only `CallAvatar`
  passes them.
- **Performance** (docs/apple/performance.md P4, P5, P7, P9): one clock, the ring's own (`TimelineView` at `voiceFrame`),
  drives the ring, the ripples and the avatar's transform (its scale and lift); the picture is not drawn again for them,
  except Mochi's face as its mouth moves (in eighths, so only when it visibly moves, as its working state redraws). The
  clock runs only while the ring has a level to follow or a lap to run, motion is allowed and the avatar is seen
  (`CallAvatarLook.ticks`, #9887 review): its own visibility reader (`VisibleWork`: mounted, on screen in the page's scroll
  view, the scene active), the app in front and the call's details closed (`CallAvatar.covered`); a minimised call has
  no avatar at all. Unseen, the speaking pose rests at once, the voice's level is not read and no step is taken
  (`CallSpeechClock.pose`). What the conversation does is read in the avatar's own small view (`CallScreenAvatar`), so
  only it redraws as that changes. Reduce Motion and Low Power Mode: every picture still (its state's first frame), no
  bounce, ripples, mouth or clock; a bright, still ring alone says it speaks. Low Power Mode is followed as it is
  switched (`CallMotionPolicy`, from `NSProcessInfoPowerStateDidChange`), so the look is made again at once, whatever
  the call does, and a package's frames hold still (or play again) with it.
- **The minimised bar's small avatar** (`CallBarFace`): the same state, still: Mochi's still drawing of the state (the
  thinking bubble, the working arc, the needs-you ring, its eyes shut on hold, asleep once over), a package's first frame
  of the state (dimmed on hold), greyed once over; an Agent's mark as it is. No motion: the bar sits over every page for
  the whole call, where a moving face would pull the eye from what the owner reads and keep a clock running on every
  page, and at 28 points a bounce or a mouth would not read; the call screen is where it moves. The bar's own hold and
  mute marks stay beside its words.
- **The voice's pause is not a hold.** The extension pauses its upstream while nobody speaks or while the conversation
  works (`link{paused}`, then `waking` as the owner speaks): on a call that is the call listening (or working while its
  conversation works), the line stays the running time. Only the call's own hold (`PersonaCall.held`) shows On Hold, the
  caution ring and the pause mark (`CallScreenState.make`; the voice bar's own words for the pause are unchanged).
- A future idea (a pack-format change through the hub and docs/extensions, not now): an optional `speaking` dot state in
  pack manifests, played under the shared layer when a package has it.

## One name for the main conversation (#9887)

Everything about a call says what the main conversation's top bar says: `AppModel.assistantShownName` (the current
assistant's name on a hub with several assistants, else the look's, "Persona" for the built-in), taken when the call is
made, again as its conversation is decided and once more just before the system's Start, then fixed for the call
(`CallAssistant.name`, `PersonaCalls.assistantName`): the call screen and its bar, the question elsewhere ("End the call
with <name> and call …?"), the handle's value and display name, and an urgent call's handle and its page's header
(`PersonaCalls.urgentCallerName`: the call's own name; the DEBUG sample with no call behind it, the name as it is now).
The call is bound to whose main conversation it calls as that is decided (`CallAssistant.Owner`: the selected hub and
the assistant picked there, #9887 review): the name and look are taken again before the Start only for that same
assistant (a cold launch's roster and look), so picking another assistant or hub while the call waits for the extension
or the microphone renames nothing; the voice bar in the main conversation says it too (`VoiceEntry.label`), and the
voice chat's bar. A redial with any value still calls the current hub's main conversation.

The page is a full-screen call screen that can be minimised to a "return to call" bar
(`docs/design/DESIGN_SYSTEM.md`, "Call screen" and "Return-to-call bar"). The call is the assistant's: the screen shows
its own look and name, and the system's call UI, the Dynamic Island, the Lock Screen and Recents show its name. Both the
screen and the bar live in a window of their own above the app's, so the bar is on every page and over every sheet,
and nothing in the app ends a live call by itself ("Nothing in the app ends a call" below).

## Architecture (`apple/Persona/Calls/`)

| Piece | What it does |
| --- | --- |
| `PersonaCalls` (`PersonaCalls.swift`, `OwnerCall.swift`) | The app's one call manager (`PersonaCalls.shared`, `@MainActor @Observable`). It owns the calls list the pages read (`calls`, `ownerCall`, `presented`, `refusal`, `route`), places calls to Persona (`callPersona`), rings urgent calls for `UrgentCallCenter` (`ring`, `answer`, `settle`), sends each system action to its call by conversation UUID, and carries out what the call's machine says. |
| `CallMachine` (`CallMachine.swift`) | One per call, both kinds: a pure `nonisolated` value type with no LiveCommunicationKit and no audio. Events in (the system's actions and callbacks, the page, the engine, timers), effects out (fulfil or fail, report, engine start/stop/mute/hold, ask the system to mute, unpause or end, activate the session once, schedule a timer). Every rule below is a unit test. |
| `CallSystem` (`CallSystem.swift`) | The seam to the system. `LCKCallSystem` on devices: the app's only `ConversationManager`, made the first time a call needs it (never at launch, #9264), and its delegate `PersonaCallsLCKDelegate`, whose methods are all `nonisolated` and hop to the main actor in order (`CallMain`; an unmarked delegate method traps when LiveCommunicationKit calls it from its own queue). `SimulatedCallSystem` in the Simulator. A fake in the tests. LiveCommunicationKit's action objects cannot be made by anyone else, so each action reaches `PersonaCalls` as a `CallSystemAction` with its own fulfil and fail. |
| `CallEngine` (`CallEngine.swift`) | What runs the conversation: `OwnerCallEngine` for a call to Persona (a `VoiceConversation(surface: .call)` on the system's audio session, `PhoneVoiceAudio(session: .system)`), `UrgentCallEngine` for an urgent call (`UrgentCallSession`, turn by turn). |
| `CallRoute` (`CallRoute.swift`) | Where the sound goes now (receiver, speaker, Bluetooth by name, headphones, car), read from the session on every route change while there is a call, never from what the app asked for. Speaker on is `overrideOutputAudioPort(.speaker)`, handed back when the last call ends. |
| `CallScreen`, `CallAvatar`, `CallControls`, `CallDetailsSheet`, `UrgentCallView` | The pages. The assistant in the middle of the call screen is `CallAvatar` (its look moving with the call, a ring of the voice's level). What the call screen and the bar shorten (a long title, a long answer) stays whole for VoiceOver and in Call Details (`CallDetailsSheet`, a tap on the name or the caption: the whole name, whose it is, the call's line and every caption); why a call ended and every word of the urgent call's page wrap whole (#9887 polish, docs/design/DESIGN_SYSTEM.md "Call screen"). Who a call is with is the call's own `CallAssistant` (`PersonaCall.assistant`): the look and name taken together when the call is made and once more just before the system's Start, then fixed, so the face beside the name never follows an avatar chosen during the call; a package's frames and face are read and decoded off the main actor into it (`PersonaStore.decodedFrames`, `decodedFace`), and the pages (`CallAssistantDot`, `CallAssistantFace`) only arrange what is there, the built-in drawing meanwhile. Every control goes through `PersonaCalls`, never to the voice directly, so the system's call UI and AirPods say the same. |
| `CallWindow` (`CallWindow.swift`) | The call screen and the bar back to a minimised call, in a passthrough window at `.normal + 0.75` over the app's windows (the + panel's, the screen viewer's) and under the toasts', the system's alerts and the keyboard. It takes touches only where it shows something: the whole screen for the call screen and a refusal's alert, the bar alone while minimised. The pages under the bar get its room (`additionalSafeAreaInsets` on the app's root, and on every sheet or cover whose content would start under it; never on an alert or action sheet, `CallWindow.takesRoom`; the toasts' window above the call's gets it too, so a toast stays just below the top bars, `TakesCallBarRoom`), looked at again four times a second while the bar shows and the app is active; the watch is cancelled as the app leaves the front (Home, the lock, another app) and the room put right at once as it comes back (`CallWindow.watchesRoom`, docs/apple/performance.md P9). |
| `CallIcon` (`CallIcon.swift`) | The system call UI's template icon: Mochi's silhouette, 40 pt at 3×, drawn and encoded off the main actor before the manager is made (`PersonaCalls.readySystem`, the first call placed or rung), which only takes the finished PNG. |
| `CallGuards` (`CallGuards.swift`), `LiveCallGate` (`apple/Shared/Recording/LiveCallGate.swift`) | What keeps in-app actions from ending a live call: the manager answers whether a call is live (`guardsLive`), the assistant's name for the questions, the call's own conversation; web pages' media waits (`CallMediaGuard`). |

The voice side of a call to Persona (the `.call` surface, the system's audio session, `surface` in `start`, hold frames)
is `docs/assistant/voice.md` "6.12 System calls"; the urgent call's side is `docs/assistant/urgent-call.md`.

**Why a stand-in in the Simulator.** The Simulator has no call service. The real LiveCommunicationKit accepts the Start
there, the conversation joins, then callservicesd logs "Disconnecting call because there wont be a UI to host the call"
(disconnect reason 55) and ends it with an End action about 80–200 ms later; `didActivate` never comes. So
`SimulatedCallSystem` (`#if targetEnvironment(simulator)`) carries every request out at once, as the system would hand
it back, and activates and deactivates the audio session itself where the system would call `didActivate` /
`didDeactivate`. An incoming urgent call rings silently there. DEBUG `-showSystemCallService YES` takes the real
LiveCommunicationKit in the Simulator, to see and log what it does.

## LiveCommunicationKit configuration and the handle

`LCKCallSystem.configuration`, read-only once the manager exists:

- `maximumConversationGroups: 2`, one conversation per group: an urgent call during a call to Persona offers
  **Hold & Accept** (with one group the system offers only End & Accept and Decline, and a Start at capacity fails with
  `requesttransaction` code 7).
- `includesConversationInRecents: true` (the redial), `supportsVideo: false`,
  `supportedHandleTypes: [.generic, .emailAddress]` (the email kind for Developing Persona's Call handle below: the
  configuration is fixed for the manager's life, so both are declared whatever the switch says), no ringtone. `iconTemplateImageData` is the assistant's mark as a template (`CallIcon`): an alpha silhouette of Mochi
  (its body and outline, its eyes cut out), 40 pt at 3× as Apple's guidance for a call app's icon asks, drawn with plain
  Core Graphics and encoded once, on a background thread, before the manager is made (`CallIcon.prepare`, awaited by
  `PersonaCalls.readySystem` before the first Start or ring; the manager's configuration is fixed for its life and is
  made on the main actor from the finished data, `CallIcon.ready`). Mochi stands for every look:
  a package's or an uploaded picture has no outline of its own to cut.
- Every conversation is pausable: `capabilities: [.pausing]` in the incoming report and in the outgoing call's members
  update (below).

The handle is the assistant's name: `Handle(type: .generic, value: <name>, displayName: <name>)`, where the name is the
one the main conversation's top bar says ("One name" above: the current assistant's on a hub with several, else
`Persona.name` of the look on screen, "Persona" for the built-in Mochi), trimmed, and "Persona" when it is empty
(`CallHandle.name`). The call takes the look and its name together when it is made, and
once more just before the system's Start (after every wait, so a cold launch's hub and look count), then keeps both
(`PersonaCall.assistant`): the system, the call screen and the bar say the same name beside the same face for the whole
call, whatever avatar or hub is chosen meanwhile. The system UI, the Dynamic Island, the Lock Screen and Recents show
the handle's **value** and ignore its display name (FB24933340), and a redial gives back only the value. A redial with
any value (an old "Persona", a look's name since changed) calls the current hub's main conversation. An urgent call's
member is the same value with the call's name ("Urgent: <title>" when the Lock Screen may show details) only as its
display name: the call id no longer goes in the handle (it showed as "call_…"), and the title never reaches Recents.
The opt-in SiriKit extension still names its one contact "Persona" (it cannot read the app's look; `docs/apple/siri.md`).

Developing Persona's **Call Handle** (Settings › About, while the switch is on; `docs/operations/developing-persona.md`)
is an on-device experiment for Recents, which only a device shows: **Generic** (the default, above) or **Email**, a
handle of kind `.emailAddress` whose value is a fixed placeholder on the reserved `.invalid` domain
(`CallHandle.placeholderAddress`), with the assistant's name as its display name. A redial with either calls the main
conversation. Urgent calls keep the generic handle.

LiveCommunicationKit reports errors in CallKit's domains (`com.apple.CallKit.error.requesttransaction`, sometimes under
`NSUnderlyingErrorKey`). `CallSystemError` compares them as strings. The app never imports CallKit
(`docs/operations/release.md`, "No CallKit").

## The state machine

A call has an id (the conversation UUID), a kind (`.owner(group:)` or `.urgent(callID:)`), a direction, a phase
(`dialing`, `ringing`, `connecting`, `connected(since:)`, `ended(reason)`), `muted`, a set of hold reasons (`.system`:
the system paused it; `.interruption`: the audio session is not the call's outside a pause, whether interrupted,
deactivated with no word of why, or not given back after the system resumed the call) and whether the system's audio is
the call's (`audioActive`). `held` is "any hold reason".

- **Start (outgoing).** `StartConversationAction` → fulfil, phase `connecting`, report the call's members and then
  `startedConnecting`, begin the engine (the voice starts), start the activation watchdog. The members update (#9887,
  `LCKCallSystem.outgoingMembers`) is `conversationUpdated(Update(localMember: owner, members: [the call's handle],
  activeRemoteMembers: …, capabilities: [.pausing]))`, the owner being the incoming calls' local member: without a
  remote member the system has nobody to write into Recents (an incoming call names its member as it is reported).
  Apple's `members` are the remote participants, "including invited, but inactive" ones, so the owner is never among
  them, and the call's handle counts as active only once the conversation is `joined` (or `paused`); before that
  `activeRemoteMembers` is empty: the assistant is invited, not on the call (#12341). Reports wait until the system
  knows the conversation (LiveCommunicationKit lists an outgoing conversation only once its Start is fulfilled: checked
  right after the fulfil, else on the next `conversationChanged`), in the order they were made: the system drops
  reports made before. The first `conversationChanged` that says `joined` reports the members once more, now with the
  handle active (#10495); later `joined` callbacks (one can follow the update itself) report nothing again.
  The system giving up on the Start ends the call as failed.
- **Connected.** A call to Persona connects on the voice's first `ready` (report `connected`); an urgent call on Join.
- **Audio.** The engine's audio starts only on `didActivate`. `didDeactivate` stops it and never ends the call; outside
  a pause, with no interruption to say why, it holds the call (`.interruption`, either kind's engine holds) and the
  recovery below runs at once, so the call is never left connected and silent. No `didActivate` 5 s after the Start or
  Join: logged, `setActive(true)` tried once; if that fails too, the call ends as failed (both are device-checklist
  lines). The watchdog waits while the call is held (the system's pause or an interruption has the audio) and runs again
  from the system's resume; the result of an activation asked before a hold or an interruption counts for nothing. An
  activation that comes before the Start is handled is kept.
- **Mute.** `MuteConversationAction` is fulfilled and applied, idempotent by value: the system echoes the app's own mute
  8–17 ms later and sends an unmute after a muted call ends. The page's Mute asks the system (so its call UI and AirPods
  agree) and applies here if the system refuses (it does while dialling: the mute then reaches the system once it knows
  the call). From `didActivate` until 0.5 s after the engine runs, a system mute is the engine's own (turning voice
  processing on surfaces as one on iOS 17+): ignored, and the owner's mute asserted again once per engine (the first, one
  after a hold or an interruption, and one a route change rebuilt, which the audio says before it starts it).
- **Hold.** `PauseConversationAction` is fulfilled at once (a failed pause, or one left about 5 s, makes the system end
  the call). While held the microphone sends silence, what the voice says is dropped and nothing plays, the idle clock
  stops, and an extension that lists `hold` gets `{"type":"hold","on":true}` (again after a reconnect's `ready`, before
  any audio; what the microphone caught while waiting for `ready` is dropped, never sent). The call goes on only once
  every hold reason is gone and the audio is the call's again. The system's resume with no `didActivate` after it (the
  session was active already, our other call had it) keeps the call On Hold and runs the recovery below.
- **Interruption.** LiveCommunicationKit has no call observer, so a cellular call or another app's call may reach the
  app only as an audio-session interruption. Its `began` once the call started or was answered holds the call
  (`.interruption`), never ends it, and stops the engine. Its `ended`: nothing while the system still pauses the call (its
  resume starts the recovery); otherwise the recovery: after 1 s without `didActivate` the app asks the system to
  unpause, 0.5 s later activates the session once itself (not while the system pauses the call), and at 5 s the call
  stays On Hold with a **Resume** button (never silently ended). Resume asks again.
- **Our own second call.** When any of our calls ends, every other call the system held is unpaused by the app (the
  system does not resume a held call of the same app): an urgent call answered with Hold & Accept, then over, gives the
  call to Persona back.
- **End.** `EndConversationAction` → end the engine (`VoiceCalls.end(.user, from: .call)`: the socket, the microphone
  and the player stop), then fulfil (#12341). The action "removes the local participant from a conversation and stops
  all audio and video streams", and `fulfill(dateEnded:)` says the conversation "was successfully ended for the local
  member", so the streams stop before it is said, as Apple's CallKit sample (Speakerbox) stops its audio before it
  fulfils `CXEndCallAction`. Before #12341 the fulfil came first and the voice-processing engine went on for about
  200 ms on the main actor after it, while the system tore the call down. The engine ending
  by itself reports `conversationEnded`: `failed` for failures (`VoiceEnd.failsCall`), `remoteEnded` otherwise. The page's
  Hang Up goes through the system, ending here if it refuses. A conversation that left without an End action, or a
  manager reset, ends the call without a report. Each side ends exactly once. An ended call stays in the list 2 s so the
  page can say why, then leaves.

## Placing a call (`callPersona`, `callConversation`)

1. The call screen shows "Calling…" at once, before anything slow (a call already on is shown instead, unless the owner
   said End and Call to the question: it ends first). Whatever page or sheet the app was on stays under it (the call's
   window goes over everything, Settings' sheet included). A cold launch from Siri, Shortcuts or Recents waits (at most
   8 s) for the hub links and the main conversation.
2. Refused with the voice alert (and the page goes): an urgent call rings or is on, a recording or dictation has the
   microphone, no hub, no main conversation, no usable voice extension (the same checks and words as the chat's Voice
   Chat, after `VoiceSupport.refresh`), the microphone not allowed (asked in the app first: a locked phone mid-call
   cannot show the prompt). The urgent call and the recording are asked again after every wait and before each Start.
3. The phone icon's conversation, or the selected hub's main conversation, and its voice extension
   (`VoiceAvailability.pick`).
4. An in-app voice chat that is on ends first: its audio would end the call's.
5. The audio category, off the main actor: `.playAndRecord`, `.voiceChat`, `[.allowBluetoothHFP, .allowBluetoothA2DP]`
   (`PhoneVoiceAudio.callSessionOptions`, no `.defaultToSpeaker`: the earpiece by default, like a phone call). The app
   never activates the session; the system does.
6. `StartConversationAction`. From any origin but the app's own button the Start waits until the app is active (at most
   5 s): one started while the app is still coming to the front is refused (code 6), and is tried once more 0.5 s later.
   Any other refusal falls back to the in-app voice chat on the same conversation, with a toast saying why.
7. On the system's Start: the voice, `VoiceCalls.begin` of a `VoiceConversation(surface: .call)` bound to the
   conversation with its recent lines; its `start` frame carries `"surface":"call"`.

The urgent call sets the same category as it rings (`PersonaCalls.prepareAudio`), unless a call to Persona has set it
already (one still being placed has not), or an in-app voice chat is on: its loudspeaker stays while the call rings, and
answering ends the chat and sets the category before Join is fulfilled. So both kinds start on the earpiece; the pages'
**Speaker** (or **Audio**, the system's route picker, while Bluetooth, a car or headphones can take the call) and the
system's call UI move it.

A refusal that comes after its attempt was hung up (the main conversation or the microphone prompt answered late) is
dropped: a newer call's page and alert stay.

## Background and the Lock Screen

- Neither a `.call` nor, on iOS, the in-app `.phone` voice chat ends in the background (#9887; `docs/assistant/voice.md`
  "6.5 Lifecycle"). A `.call` does not keep the screen awake: the phone may auto-lock during it, as during a phone call.
- The tunnels of the call's hub group stay open in the background (`AppModel.callTunnelGroup`), and close once the call
  ends in the background.
- Hiding the page, another page, another app, Home and the Lock Screen never end the call. Coming back to the app with
  the call on (the Dynamic Island, the status bar's call, the app switcher) shows the call screen again
  (`returnedToApp`), over whatever page or sheet the app was on.
- A held call has no audio session, so iOS may suspend the app: its socket may be dead on resume, and the voice's
  usual reconnect (1, 4 and 10 s) brings it back and says the hold again.

## With voice chat, urgent calls and recording

- One call at a time, and nothing ends a system call by itself. The phone icon in another conversation asks "End the
  call with <name> and call <this conversation>?" on the iPhone ("One phone button"). A voice chat in another
  conversation (the phone icon on the iPad, Continue on an ended voice chat) asks first: "End the call with <name> and
  start a voice chat here?" (**End Call and Start**, destructive, or **Cancel**, which keeps the call;
  `VoiceUnavailableNote.endCallFirst`). Only the yes ends the call and waits (at most 2 s) for it to end and the system
  to let its audio go before the voice chat opens its own (`VoiceCalls.start(endingCall: true)`,
  `VoiceCalls.endSystemCall`, which `PersonaCalls.endForVoiceChat` answers); it is refused if an urgent call rang
  meanwhile. Without the yes `VoiceCalls` refuses it, touching nothing (`VoiceUnavailableNote.systemCallOn`). The phone
  icon in the call's own conversation shows the call screen.
- The voice chat in the app survives leaving its page, so it asks before it is replaced too (#9887): the phone icon in
  another conversation while one is on elsewhere (on the iPad and Apple Vision Pro, or on the iPhone where it is the
  fallback after a refused Start) asks "End the voice chat with <name> and start one here?" (**End and Start**,
  destructive, or **Cancel**, which keeps it; `VoiceUnavailableNote.endVoiceChatFirst`, `VoiceProceed.replaceVoice`), and
  on the iPhone, where the icon places a system call, "End the voice chat with <name> and call <this conversation>?"
  (**End and Call**). Same question component (`voiceUnavailableAlert`); `VoiceEntry.choose` asks it
  (`askVoiceChat`), and the yes goes ahead as a call's does (`VoiceEntry.proceed`).
- An urgent call rings during a call to Persona: it is reported as always (iOS ends an app that does not report a VoIP
  push's call), and the system offers Hold & Accept, End & Accept or Decline. Pause and End reach the call to Persona
  through its machine; after the urgent call, the app unpauses it.
- An urgent call answered during an in-app voice chat ends the voice chat first (`VoiceEnd.urgentCall`, "Ended for
  Persona's urgent call", Continue offered), so its audio cannot deactivate the session under the urgent call.
- While an urgent call rings or is on, a voice chat (any page, CarPlay, Continue) and a call to Persona are refused with
  "Finish the urgent call first" (`VoiceUnavailableNote.urgentCall`; `VoiceCalls.urgentCallOn`). One the system would
  not show (Focus or Do Not Disturb refused its ring, only its companion notification says so) refuses nothing until it
  is answered from that notification.
- Answered from its notification without the system (it refused the ring or Join): a call to Persona that is on goes on
  hold through the system first, as Hold & Accept does, and the urgent call talks once the system let that call's audio
  go; after it, the call to Persona is resumed. If the system will not hold the call to Persona, the urgent call is not
  answered here and its conversation opens instead.
- A recording or dictation in progress refuses a call to Persona; one asked for during a call is refused with a note and
  the call goes on (below). The call's voice keeps the microphone (`RecordingMicrophone.claim` without giving way), so
  even a path that asked nothing cannot take it.

## Nothing in the app ends a call

While a call to Persona (from "Calling…") or an urgent call (ringing, once the system showed it, or answered) is live,
an in-app action never silently ends it, takes its microphone or sets up the audio session under it
(`PersonaCalls.guardsLive`, `LiveCallGate`):

- **Dictation** (the composer's mic tapped or held, #9619; the to-do fields): refused at once with a toast, "On a call —
  dictation is off until it ends"; neither the dictation bar nor a hold opens (`Dictation.callGate`). A dictation that
  was waiting for its permission when the call went live is refused once it is answered, before the microphone or the
  session; its session is opened on the audio session's queue the call sets its category on (`LiveCallGate.sessionQueue`),
  and a block that finds a call live there touches nothing (`LiveCallGate.liveNow`). A dictation the call came during
  (an urgent call answered) does not deactivate the session as it ends.
- **A recording** (a meeting from the + list, the home screen's menu, Siri, the Control or the Recordings page; a note;
  all day): refused, "End the call first" (a toast where the start came from elsewhere, the Recordings page's line under
  its button); a recording a call paused stays paused, saying the same (`RecordingCapture.callGate`, checked before the
  permission, after it, after the Bluetooth microphone's search, at the resume's turn and just before the session is set
  up or the microphone opens: a start the call came during stops there). A recording that exists under a call (paused
  before an urgent call was answered) can still be stopped and is saved; its Stop (the Live Activity's, the system's)
  gives back only its microphone and leaves the call's session as it is, and Persona speaking or not (its category),
  echo cancellation and the meeting speaker's route wait for the call to end.
- **Voice chat**: asks first in another conversation, shows the call screen in the call's own (above). A refusal that
  comes after the yes (an urgent call rang while the call's audio went) is said by the same alert once it has gone
  (`VoiceEntry.proceed` awaits the start to its end); the yes after the call already ended by itself (AirPods, the
  system's call UI) hangs up nothing but still waits for the system to let its audio go.
- **Disconnecting the call's hub** (Settings' Disconnect All, Disconnect This Group or This Hub, Devices' Revoke and
  Unpair of this device): refused with a toast, "On a call with <name>. End the call before disconnecting.", before any
  credential is revoked or anything forgotten (`AppModel.disconnectRefusal`, `PersonaCalls.disconnectRefusal`); another
  group's hub disconnects as before. A call whose group is not known yet (an urgent call without its ring's details)
  counts for every group.
- **CarPlay**: the car's Talk to Persona (its screen, its Start, its Continue, Siri's Talk to Persona with the car
  connected) does not start while a call to Persona is on the phone: the car says "You're on a call with <name> on the
  phone" (`CarPlayVoiceSession.onPhoneCall`), and the call plays through the car when the car is its audio. The choice:
  the car's voice chat would have to end the call, and a question on the phone is the wrong thing while driving.
- **Web pages' media** (the in-app browser and its preloads, HTML cards and files, mail): suspended while a call is live
  and let go after (`CallMediaGuard`, `WKWebView.setAllMediaPlaybackSuspended`): WebKit sets up its own audio for a
  page's video, which would interrupt the call's session (the call would go on hold).
- **The screen viewer's picture in picture** sets up no playback session during a call (`VNCPiP.prepare`).

Every place that sets up the audio session has a verdict for a live call (`CallSourceTests.everyAudioSessionChangeHasAVerdictForALiveCall`
fails on a new one until it is given one):

| Where | Runs during a call? | Disturbs it? | Guard |
| --- | --- | --- | --- |
| `PersonaCalls.setCallCategory` (category before Start, as an urgent call rings) | yes, the call's own | no: the same category, before the system activates the session | none needed |
| `CallSystem` (`activateAudio` once after the 5 s watchdog; the Simulator's stand-in) | yes, the call's own | no | none needed |
| `CallRoute.setSpeaker` (`overrideOutputAudioPort`) | yes, on Speaker | no: the call's own route | none needed |
| `PhoneVoiceAudio` (`.phone`, `.carPlay`: category, activation, preferred input, speaker override) | only through `VoiceCalls` | would: its session replaces the call's | refused, or asked first (`VoiceCalls.systemCallName`); the system mode (a call's) never touches the session (`aSystemCallsAudioNeverTouchesTheSystemsSession`) |
| `Dictation` (`.record` category, activation, deactivation) | would, when tapped, or when its permission is answered | would: cuts the call's microphone, its deactivation stops the call's audio | refused before and after the permission (`LiveCallGate.dictationRefusal`); opened on `LiveCallGate.sessionQueue`, in order with the call's category; no deactivation while a call is live |
| `RecordingCapture.configureSession` (start, resume, the resume when the app comes back) | would | would | refused, and asked again after every wait and before the session or the microphone (`LiveCallGate.recordingRefusal`) |
| `MeetingReplay` (playback category, activation and deactivation) | would, starting a tape or opening / ending its audio | would: it replaces or deactivates the call's session | refused before the replay begins (`RecordingCapture.callGate`); activation and deactivation run on `LiveCallGate.sessionQueue` and are skipped when `LiveCallGate.liveNow` |
| `RecordingCapture` stop, cancel, failed start (preferred input, deactivation), `setPlaysAloud` (category), `turnOnEchoCancellation` | yes: a recording paused before an urgent call was answered | would: the deactivation stops the call's audio, the category moves its route | under a live call only the microphone goes back; the category and voice processing wait for the resume |
| `MeetingSpeaker` (speaker override and its restore), `MeetingVoicePlayer`, `MeetingCallAudio` | only during a meeting recording (a paused one exists under a call) | the override would move the call's route | skipped during a call (`LiveCallGate.callIsLive`) |
| `VNCPiP.prepare` (`.playback` category, activation) | would, opening a Mac's screen | would | skipped during a call (`LiveCallGate.callIsLive`) |
| `UrgentCallVoice` (its recognizer's engine, `MeetingVoicePlayer`, the phone's voice) | yes, the urgent call's own | no: on the system's session, no category change | none needed |
| `AVPlayer` / `AVPlayerViewController` / `VideoPlayer` (audio messages, video files, link cards' video, Lark media) | can | no: they play through the session as it is and change nothing | none: the sound plays in the call's route |
| WebKit media (in-app browser, HTML cards and files, mail) | can | can: WebKit's own audio interrupts the call's session | suspended (`CallMediaGuard`) |
| Camera (`UIImagePickerController`, photos only) and the QR scanner (`DataScannerViewController`) | can | no: video only, no audio input | none needed |
| Haptics (`UIFeedbackGenerator`), voice cues (through the voice's own player) | can | no | none needed |
| Hub switch | can | no session change | none needed |
| Disconnect (Settings' Disconnect, Devices' Revoke and Unpair) | would | would: it revokes the credential and closes the tunnels the call talks through, and the voice ends as unreachable | refused while a call talks through that hub (`LiveCallGate.disconnectRefusal`) |

## Recents and intents

The system writes a call into the Phone app's Recents when it ends, for its members: an outgoing call reports them once
the system has its conversation (the members update above) and again, the handle active, when the conversation joins
(#10495), and its end goes through `EndConversationAction` or a `conversationEnded` report. That the system discards
updates while `idle` (#10495's reading) is not something Apple documents: whether it keeps them shows only on a device,
in the call log's `local member` while the call is on (below). `local member none` on the final `left` line proves
nothing either way: End "removes the local participant from a conversation" by definition (#12341).
The app declares `NSUserActivityTypes` `INStartCallIntent` and `INStartAudioCallIntent`
(`apple/project.yml`): Recents may hand either. A redial reaches `PersonaCalls.continueActivity` from the scene's delegate (cold launch:
`connectionOptions.userActivities`; warm: `scene(_:continue:)`) or SwiftUI's `onContinueUserActivity`, whichever
fires; the same tap within 2 s is taken once. Any handle calls the current hub's main conversation. Siri, Shortcuts,
Spotlight and the opt-in SiriKit extension: `docs/apple/siri.md`, "Calling Persona".

### The call log (#9887)

Each step of a call is one line in the `persona` subsystem, category `call` (ids, kinds and states only; an assistant's
own name only by its length): Start requested (the conversation id, the handle's kind and value), the system's Start
received, Start fulfilled and whether the system had the conversation then, the system having it (right after the
fulfil, on `conversationChanged`, as it rang), every report sent (with the conversation's state), queued until the
system has the conversation, or dropped, connected (the voice's first ready), the end (by which event, and whether
`conversationEnded` was reported) and the conversation's state on every `conversationChanged`, each with whether the
system's conversation has a local member (`local member set` / `none`). The members update, the final `left` and the
End's two snapshots start with `recents:`, so `log stream --predicate 'subsystem == "persona" AND category == "call"'` (or
Console, or Developing Persona's Export Diagnostics, whose `log.txt` keeps this launch's lines) shows whether the system
had the members and saw the conversation leave. The members are reported twice (#10495): once when the system first has
the conversation (usually `idle`, the handle `invited`), once when it joins (`active`); each line also says whether the
system's conversation had a local member before that update. `local member set` on a later line (`the system's
conversation is joined (local member set)`, or the End's `before End` snapshot) is what says the system kept an update.

An End the system asks for (the system's call UI, AirPods, or the page's Hang Up through the system) writes, in order
(#12341): `the system asks end`; `recents: … before End, manager list: <state>, local member: <set|none>` (the
conversation as the system's list has it, read before anything stops); `ended (…hungUp…)` and `the end, by endAction`;
`End fulfilled` (after the engine stopped); the `left` callback; `the system deactivated the audio session`; and a
second after the fulfil `recents: … after End, manager list: <state|absent>, local member: <…|n/a>`. `absent` there means
the system has dropped the conversation from the app's manager; `left` (still listed) a second after the End is the
first thing to check when the system's call UI stays on screen. The snapshots only read the list: they never end or
report anything.

## CarPlay

- CarPlay's own call screen is not used: CarPlay asks VoIP calling apps for CallKit and its communication entitlement.
  A call to Persona made on the phone plays through the car when the car is the route (`CallRoute` says `car`).
- Talk to Persona with no car places a call to Persona on the phone (or shows the one on); with CarPlay connected it is
  the car's voice chat, or asks to open Persona on the car's screen (`docs/apple/carplay.md`).
- The car's voice chat is refused while an urgent call rings or is on, and while a call to Persona is on the phone: the
  car says the call is on the phone, and never ends it ("Nothing in the app ends a call").

## No CallKit

No source imports CallKit and no binary in the app links it, weakly or through a package: App Review rejects a build
offered in China otherwise. The source guard and the binary check run in CI and in the release
(`docs/operations/release.md`, "No CallKit"); `CallSourceTests` also checks that only `CallSystem.swift` makes a
`ConversationManager`.

## DEBUG switches

All are listed in `scripts/release/debug_switches.txt`; `release.sh` fails a Release binary that still has them.

- `-showCallSample <state>`: the call screen with no system, audio or hub: `dialing`, `connecting`, `connected`,
  `muted`, `held`, `interrupted`, `resume`, `speaker`, `ended`, `mutedSpeaker`, `reconnecting`, `audioRoute`, `speaking`,
  and each of the avatar's rows (#9887): `hearing`, `thinking`, `working`, `asking` (a card waiting), `mutedSpeaking`,
  `paused` (the voice's upstream paused: listening, not on hold) (`PersonaCalls.showSample`). A speaking or hearing
  sample has a made-up voice level (`CallSampleVoice`: phrases of syllables with breaths), so the bounce, the ripples,
  Mochi's mouth and the ring move on the Simulator. `-showCallSampleLook package|uploaded|<id>` shows that look (it
  waits up to 30 s for it to be on the phone; the mock's `MOCK_PERSONAS=builtin MOCK_LOOK=nwverse` has a package),
  `-showCallSampleAgent YES` a call to an Agent session (Claude's mark). A sample guards nothing ("Nothing in the app
  ends a call"). `-showCallSampleName <text>`, `-showCallSampleCaption <text>` and `-showCallSampleReason <text>` put
  long texts in the sample's name (the page's and the bar's), latest words and end reason (`CallScreenTests`'
  `testTheLongTexts`, which also opens Call Details from the name).
- `-openOwnerCall YES`: calls Persona once the hub is there, as a redial would (in the Simulator through
  `SimulatedCallSystem`); `-openOwnerCallSeconds <n>` hangs up n seconds after it connected.
- `-showSystemCallService YES`: the Simulator on the real LiveCommunicationKit (it ends every call, see above).
- `-showCallFallback YES`: the Simulator's stand-in refuses every Start, as a device that will not place the call does,
  so the phone icon's fallback (the voice chat in the app, with its line) runs (`VoiceModeTests`).
- `-showUrgentCallSample YES`: the answered urgent call's page (Mute, Speaker, Hang Up); `held`: on hold after an
  interruption its audio did not come back from (On Hold, with Resume beside them). `-showUrgentCallSampleTitle <text>`:
  its title instead of the mock's.

## Tests

- Unit (`apple/PersonaTests/`): `CallAvatarStatesTests` (#9887: each row from what the call screen observes and their
  order, each row's look for every kind of picture, the shared speaking layer for every kind, the look's own pictures
  only for the main looks, the mouth only for the built-in, Reduce Motion and Low Power holding still, the bar's still
  face, the speaking motion's envelope and ripples, Mochi's default drawing unchanged pixel for pixel, the voice
  thinking, the voice's pause not a hold, the real working and asking, one name for the main conversation, the voice
  chat's question in three languages), `CallOneButtonTests` (#9887: the icon's choice per context, the question with a call
  elsewhere, the call's kind carrying its key and its voice bound to it, the fixed handle and the redial to the main
  conversation, a refused Start falling back with its line, the conversation on the pages and whose it is with another
  assistant's conversation open, the voice chat's bar, End and Call while the old call dials or is held without its
  audio), `VoiceCallsTests` (the voice chat surviving the background and leaving its page, the bar staying until its
  page shows, the interruption's pause and resume, new rates during the pause, the idle rule in the background going by
  the voice connection), `CallScreenTests` (no voice or call rows in +; the long texts: whole for VoiceOver, the
  reason's line never cut, the name and the line between Minimize and the controls on a large and a short phone at the
  largest size, the bar's time kept; the phone icon's label in both shapes and three languages); `CallMachineTests`
  (every rule above), `PersonaCallsTests` (the manager with `FakeCallSystem` and fake voices: start, reports, activation
  and the watchdog, mute, hold, interruptions, our own second call, end paths, refusals before Start, tunnels, the
  urgent exclusion), `CallSourceTests` (one manager, no CallKit, the system's session left alone), `CallScreenTests`
  (the screen's states and layout), `CallPersonaIntentTests`, the `UrgentCall*Tests` suites, `VoiceCallsTests`,
  `VoiceConversationTests` (`.call`, hold frames, `surface`), `LocalProxyRouteTests` and the CarPlay suites;
  `CallRecentsTests` (#9887: the outgoing call's members update before `startedConnecting` once the system has the
  conversation, re-reported once when the conversation joins (#10495) and not again on a later `joined` callback, the
  members update's own roster: the owner local and never a member, the handle invited until `joined` or `paused` and
  active then (#12341), the incoming path unchanged for both the initial report and the joined re-report, an End
  fulfilled only after the engine stopped (#12341), Developing Persona's Call handle picking the kind, both kinds in
  the configuration, the log's steps and the End's snapshots); `CallFollowUpTests` (the assistant's name on the system call and the ring, the guards
  following the live call, voice chat refused or asked, dictation and recordings refused, web media suspended, the call
  keeping its microphone, the assistant's look per state, the call's window, the bar's room, the template icon; and from
  the second review: a disconnect refused while the call talks through the hub, the call keeping the look and name it
  was made with, a package's pictures decoded off the main actor, an ended call's audio waited for, an urgent call
  during the hand-off said to the question, dictation asked again after its permission, a paused recording stopped under
  a call leaving its session, a start stopped by a call during the Bluetooth search or the microphone prompt, the icon
  drawn off the main actor before the manager, the bar's room watched only in front); `CallSourceTests` also checks
  every audio-session site's verdict, that every way in asks first, and those rules
  (`whatTheSecondReviewFoundStaysFixed`).
- UI (`apple/PersonaUITests/`, against `MOCK_VOICE=1` `scripts/mock/mock_hub.py`, `docs/operations/testing.md`):
  `CallScreenTests` (each sample, largest text, the app's text size, the bar, the phone icon as the one voice entry
  (`testThePhoneIconIsTheOneVoiceEntry`), the urgent call's Mute
  and its page on hold with Resume, and the real path: a call that keeps sending audio on the Agents page, in an Agent's
  conversation, on the Home Screen, with another app in front and with the phone locked, the mock's audio count at each
  step written to `real-flow.json` beside the screenshots; `testNothingInTheAppEndsTheCall`, the walk through the app
  with the bar over the Agents page, a conversation, the + panel (no voice row) and Settings, dictation refused, the
  phone icon in an Agent's conversation asked (End and Call) and cancelled, a typed line, Home, another app and the
  lock, the counts in `walk.json`; `testACallFromAnAgentsConversation`, the phone icon in an Agent's conversation: the
  start's target is that session with `surface: "call"`, its title on the call screen and the bar, Home, another app
  15 s, locked 15 s, back, the audio rising and no socket closed, the counts in `agent-call.json`;
  `testTheAssistantOnTheCallScreen` (the top bar's name, the mock's 「助手」); `testTheAvatarsStates` (#9887: every
  avatar row's sample and the bar's still face, `PERSONA_CALL_LOOKS=mochi,agent,package` for the pictures, the voice's
  pause showing the running time); `testTheCallFollowsTheAssistant` with `MOCK_PERSONAS=builtin MOCK_LOOK=nwverse
  MOCK_MANAGERS=1`, one assistant, so the top bar says the look's name) and
  `CallPersonaShortcutTests` (Call Persona and Talk to Persona from Spotlight, over Settings and an Agent's
  conversation, the microphone refused).
- The mock's `GET /mock/voice2` keeps each start's `surface` (`"call"` for a system call) and the app's `hold` frames
  (`holds`, oldest first); its voice extension lists `hold` and answers a hold as the extension does.
- The microphone on a fresh install: `xcrun simctl privacy <udid> grant microphone <bundle id>` before a run that should
  not see the prompt (the Simulator's microphone may record only silence; the frames still count).

## Only a device shows

- The system's call UI, the Lock Screen and Dynamic Island call, the Recents row and which activity type a redial
  delivers. After hanging up an outgoing call in the app (#12341), the Dynamic Island's call goes at once (the system's
  short "Call Ended" at most), with no swipe of the app and no restart; Recents shows a row with the assistant's name (or
  the conversation's) and the app's icon, and tapping it calls the main conversation. The `recents:` lines say the
  members were reported twice (`invited` while `idle`, `active` while `joined`; #10495), whether the system's
  conversation had a local member while the call was on (`joined (local member set)`, `before End … local member: set`),
  and a second after `End fulfilled`, `after End, manager list: absent` (`left` there, with the call UI still showing, is
  the system keeping an ended conversation: report it with that line). `local member none` on the final `left` line is
  expected. With no Recents row and `local member none` while the call was on, Developing Persona's Call Handle set to
  Email and a new call is the next check.
- When `didActivate` arrives for an outgoing call, and whether the 5 s watchdog ever fires.
- The engine-start system mute and its 0.5 s window; Mute and hang-up from AirPods and from the system's call UI.
- Hold & Accept with an urgent call; whether a cellular call arrives as a pause or as an interruption, and the resume.
- A held call suspended in the background and its reconnect.
- The earpiece by default for both kinds, Speaker on the page and in the system UI, real audio with echo cancellation,
  Bluetooth and the car.
- Siri's phrases (`docs/apple/siri.md`, "Verification on a phone").
