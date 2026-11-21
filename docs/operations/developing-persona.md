# Developing Persona: recording and exporting a device's diagnostics

**Who it serves**: people developing or investigating Persona (the owner, testers), not everyday Assistant users. So it
lives only in Settings › About, is off by default and shows nothing anywhere else (#9460).

A switch, **Developing Persona**, in each client's About. While it is on, the device records its own logs, errors and
energy use; **Export Diagnostics…** packs them into one zip the owner can share or send to an Agent. Nothing goes to the
hub or anyone else unless the owner sends the file, and then only as an ordinary file in one of their own chats.

| Client | State |
| --- | --- |
| iPhone / iPad | #9460: switch, recording, MetricKit, export with Send to Agent and Share File; #10589: Replay a Meeting Tape |
| Web, desktop app | #9617: switch, recording, export with Send to Agent and Download; #9675: the desktop app's main process |
| Android | #9618: switch, local recording, export with Share File and Send to Agent |
| Watch | not in the first version (MetricKit is not available on watchOS) |

## The switch

- Settings › About, under Version. Stored on the device (iOS `UserDefaults` `devMode.on`, `devMode.since`), never
  synced. Footer: what it records, that the data stays on the device, that turning it off deletes it.
- While on, the group adds **Recorded** (since when, how much) and **Export Diagnostics…**, and on iOS **Replay a Meeting
  Tape…** ("Replaying a meeting tape" below) and **Call Handle**
  (**Generic**, the default, or **Email**, `devMode.callHandle`): the kind of handle a call to Persona carries to the
  system, an on-device experiment for the Phone app's Recents (#9887, `docs/apple/calls.md` "Recents and intents"). It
  counts only while the switch is on; turned off, calls carry the generic handle again.
- Turning it off deletes everything recorded, and on iOS the meeting tapes kept for replays. When something is recorded
  or a tape is kept it asks first (saying the tapes go too): **Export First…** opens the export page, **Turn Off and
  Delete** goes ahead.

## Replaying a meeting tape (iOS)

**Who it serves**: people tuning the meeting room (its live transcript, highlights and map) who want to watch a recorded
meeting again on a real phone, the way the room showed it (#10589). It needs no hub change.

**Replay a Meeting Tape…** opens a page: the speed (1×, 2×, 4×) and **Play Sound** (the tape's audio at 1× and 2×,
silent at 4×), then the tapes **On This iPhone**, those **On** each of the owner's computers, and **Choose from Files…**.
A tap starts the tape in the meeting room: its time of its length and its speed (changeable while it plays) under the
title, only Pause and End; after End, the tape's minutes. Starting asks to stop first while something records.

- **Nothing reaches the hub**: no upload, no hub meeting, no transcription, nothing in the knowledge base, no push, no
  quota used. What it shows is what the hub produced when the tape was recorded. Re-running the audio through today's hub
  is a separate issue (#10590).
- **Where tapes come from**: a computer's `~/.persona/meeting-tapes` (a folder per tape: `tape.json` and the audio it
  names), listed and downloaded through the hub, which relays the bytes and keeps nothing; or the Files app (AirDrop,
  a folder on the phone).
- **What is kept**: the tapes, in Application Support/meeting-tapes, out of backups (a tape holds other people's words).
  Swipe to delete one; turning the switch off deletes them all. They are never part of the diagnostics package.

The room, its guards and the tape's format: docs/apple/recording.md "A meeting's tape and its replay".

## What is recorded (iOS)

Only numbers and words the app's code chose. Kept at most 24 hours of events, 14 days of MetricKit payloads, 7 days of
errors, 8 weeks of the connection tally and 50 MB in all, the oldest dropped first (`DevStore`, `Application Support/dev-mode`,
out of backups).

- **Events** (`events.jsonl`, one JSON object a line, `t` in UTC and `kind`):
  - `minute`: one a minute while the app is in front, one for each stretch in the background. The app process's CPU
    time, interrupt and idle wakeups and energy for that span (`task_info(TASK_POWER_INFO_V2)`, the numbers the
    simulator's `proc_pid_rusage` gives), hub requests by route pattern (count, bytes in and out, median and longest
    time, status codes), hub WebSocket frames by message type, bytes, connects and drops, seconds per screen, network
    path (`NWPathMonitor`: Wi-Fi, cellular, other; expensive, constrained), thermal state, battery, Low Power Mode, build.
  - `foreground`: one per period in front: length, CPU, wakeups, energy, requests, bytes, frames, screens and their
    seconds, thermal state and battery at start and end.
  - `app`, `page`, `thermal`, `power`, `battery`, `path`, `socket`, `request_error` (the first of each failing route and
    status in a minute).
- **Errors** (`errors.jsonl`): the app's error reports (`ErrorReports`, uncaught exceptions with call stacks), kept here
  too while the switch is on; whether they also go to the hub's `error_reports` is unchanged.
- **MetricKit** (`metrickit/`): daily `MXMetricPayload`s and `MXDiagnosticPayload`s (crashes, hangs, CPU and disk-write
  exceptions) as delivered, plus `pastPayloads` / `pastDiagnosticPayloads` when the switch is turned on.
- **Log** (`log.txt`): this launch's own `Logger` lines (subsystem `persona`), read from `OSLogStore` at export; iOS
  keeps no earlier launch's log for the app.
- **Connection tally** (`connections.json`, #11390): per week, how often the app was opened and never reached the hub, and
  why, and how it reached it when it did. Below, "Connection tally (iOS)".

Screens are named by the code: the tab (`assistant`, `agents`, `inbox`, `settings`, `feishu`) or a screen opened over it
with `.devScreen(…)` (`chat`, `assistant-chat`, `screen`, `knowledge`).

**Cost while on**: no timer in the background; in front one wakeup a minute rolls the summary. Counters live in memory
behind a lock and lines are appended in batches on a utility queue (crash reports are written at once). While off, the
hooks (`DevProbe`, `HubRedirects.finished`) are nil checks.

## Connection tally (iOS)

**Who it serves**: people developing or testing how Persona reaches the hub. The hub can't see connections that never
reach it, so only the phone can count how often it fails to; any later change to how the iPhone trusts the hub needs this
as its baseline (#11390). It is recorded only while the switch is on, like everything above; whether it may run with the
switch off is asked in #11400 and not done.

- **An open** is one period with the app in front, per hub group (Demo Mode left out), from the moment the phone has a
  network path; it ends when the app leaves the front. Tries made while the phone has no network path are left out (they
  say nothing about the hub), and so are the tries of an attempt that began before the period (the app left the front
  and came back while it ran, or a silent push woke the socket in the background and its attempt ends after the app is
  opened). An open whose socket is already connected when it begins (it came up in the background for a silent push and
  is still up) counts as connected. A period already under way when the switch is turned on is not counted (what
  happened before is not known): counting starts with the next one, or at launch with the switch on.
- **A week** is an ISO week in UTC (`2026-W47`, starting Monday 00:00 UTC). An open counts in the week it began.
- **What decides an open**: what the group's tries came to while it lasted, from the attempts the app makes anyway (the
  route picker's probes, the race that restores an SSH tunnel used last time, a tunnel's check of the direct way, the
  recheck of preferred addresses, and for a group with one address and no tunnel its socket); no probe, timer or polling
  is added. A pick that a newer attempt replaced still counts what its probes came to before it ended; one that ended
  because the app left the front usually reports after the period is over, so the tally leaves it out (Connection
  Details still shows it). The outcomes are those of Connection Details (`docs/operations/network.md`, "Client-added
  candidates": Answers, Certificate not trusted, Not your hub, Couldn't connect, Unknown).

`connections.json` holds `format` (`persona-connection-tally`), `version` (1) and `weeks`, oldest first; per week:

| Number | What it counts |
| --- | --- |
| `week`, `starts` | The ISO week (`2026-W47`) and its Monday (`2026-11-16`), UTC. |
| `recorded_seconds` | Seconds the app was in front that week with the switch on, counted from the start of a period in front. 0 or no week at all: nothing was recorded then, not "no failures". |
| `not_connected.not_your_hub` | Opens whose socket never connected and where an added address answered as another hub (on a private address only when every address tried was private). |
| `not_connected.certificate_not_trusted` | Opens that never connected, where no address said Not your hub and one failed the system's trust. |
| `not_connected.couldnt_connect` | Opens that never connected, where the most telling outcome was Couldn't connect (refused, timed out, not found, lost, not a healthy hub answer). |
| `not_connected.unknown` | Opens that never connected with nothing more telling: tries cut short, or the period ended before any try failed (a probe that answered while the socket had not connected yet counts here). |
| `connected[]` | Opens whose socket connected, one entry per way, counted once by the first connect of the open; `opens` is how many. |
| `connected[].trust` | What vouched for the connection: `system` (HTTPS straight to the address in use, checked against the system's trust), `plain_http` (plain HTTP straight to it), or `tunnel` (through an SSH tunnel, the in-app tunnel or a local proxy, whose loopback end the socket reaches in plain HTTP: no TLS to the hub is checked by the app). |
| `connected[].access_token` | The address in use carries its own Cloudflare Access service token (`ServerConfig.access`). |
| `connected[].route` | `direct`, `ssh_tunnel`, `in_app_tunnel` or `local_proxy`: how the socket went. |
| `connected[].address` | `paired`, `added` (Settings › hub › Addresses) or `mesh` (only the group's hellos listed it). |
| `connected[].hello_fingerprint` | The hub's hello lists a key fingerprint for itself (`mesh.hubs[].fp`), yes or no. |

What "most telling" means: Not your hub, then Certificate not trusted, then Couldn't connect, then Unknown. Whether the
hub machine itself was down is not told apart (that needs a hub-side field).

- **Kept** in `Application Support/dev-mode/connections.json`: this week and the seven before it, written off the main
  actor on the recorder's queue when a period in front ends (the period during an export is in the next one), never in
  `UserDefaults`. A period that ends with the app still in front (it crashed or was killed) is never written. Turning
  the switch off deletes it with the rest.
- **Never in it**: an address, host name, port, hub id, fingerprint or the group's own id; only fixed words, yes / no and
  counts (`ConnectionTallyTests`).
- **Cost**: while off, `DevProbe.listening` is checked and nothing is worked out. While on, a few counters behind the
  recorder's lock and one small file write per period in front.

## What is recorded (web and the desktop app)

The same lines and limits in spirit, kept in IndexedDB (`persona-dev-mode`) at most 24 hours and 5 MB. The switch is
`localStorage` `persona.devMode` (when it was turned on) in Settings › About Persona; turning it off deletes the records,
asking first when something is recorded.

- `minute` (a minute while the page is visible, one per hidden stretch): hub requests by route pattern (count, bytes as
  declared, median and longest time to the answer, status codes), hub WebSocket frames by type, bytes, connects and drops,
  seconds per page (the address's first word: `s` for an Agent's chat, `settings`, `home`), `online`, `memory_mb` where the
  browser tells it.
- `foreground` per visible period; `app` (recording, visible, hidden), `page`, `network`, `socket`, `request_error`.
- While the hub lists `todo_view`, from the To-dos view's shadow (`docs/web/web.md` "Developing Persona"): `todo_view`, at
  most a minute apart, counts the rows with a view: `compared`; `same`; `by_design` (apart by a decided difference, by
  which); `unknown` (a newer hub's section or reason); `client_gap` (a run whose session this page lacks); `boundary`
  (the view changes by time alone within 2 minutes); `stale_view` (a run moved since the list was read: the list is read
  again); `unsettled` (apart, until a run 10 s later still sees it); `mismatch` (apart, nothing explains it); `errors`
  (runs that failed). `todo_view_mismatch`, one per unexplained row an hour: its id, its manager's id, and both sides'
  section and reason words. Drawing the To-dos sections from the view also needs `localStorage`
  `persona.todos.viewDraw` = `1` (no settings row; reload the page after setting it).
- `errors.jsonl`: uncaught errors, unhandled rejections and freezes of 5 s or more (`lib/freeze-watch.ts`), whether or not
  the hub takes error reports. `log.txt`: `console.error` / `console.warn` lines, scrubbed (an error a boundary caught is
  logged there too).
- The manifest names the platform (`web`, or `desktop` inside the desktop app), the system and browser, the web build (its
  entry script's name) and the hub's version.

**No first-screen cost.** Everything lives in chunks that load only while the switch is on (`lib/dev-mode.ts`,
`lib/dev-package.ts` with jszip, `views/settings/DevExportDialog.tsx`). The start-up check and the WebSocket wrap are in
`lib/dev-probe.ts`, loaded by `lib/error-reports-start.ts`, which runs before the app opens its socket and is outside the
first screen's budget. The recorder hooks `fetch`, `history`, `popstate`, `error`, `unhandledrejection`, the console and
the freeze watch itself and puts them back when the switch goes off; it imports nothing from the first screen's modules
that they do not already export, so its route word list is a copy of `lib/error-reports.ts`'s (`dev-mode.test.ts` keeps
them equal). A socket already open when the switch is turned on is counted from its next connection. The To-dos view's
shadow (`web/src/features/todos/model/view-shadow.ts`, a chunk of its own) reads the triage through App's module: the
recorder can load before App, so importing `lib/todo-triage.ts` would move it out of App's chunk into one of its own.

## The desktop app (#9675)

The desktop app shows the web page, so its switch, recording and export are the web's. While the switch is on the page
also tells the shell (`personaDesktop.setDevDiagnostics(true)`; off: `false`, which deletes what the shell kept), and the
main process records what the page cannot see, in `<user data>/dev-diagnostics/main.jsonl` (2 MB, then moved to
`main.1.jsonl`; about 4 MB in all): `shell_minute` (each process type's count, CPU percent, idle wakeups a second and
memory, from `app.getAppMetrics()`, and whether on battery), `power` (suspend, resume, on battery / AC, lock),
`thermal` (macOS), `process_gone` (a renderer or child process: type, reason, exit code), `window` (unresponsive /
responsive) and `shell` (recording started, versions). `crashReporter` runs with `uploadToServer: false`: dumps stay on
the computer; the package only counts those written since recording began (`counts.crash_dumps`).

The export reads the lines with `personaDesktop.readDevDiagnostics()` and adds them as `desktop.jsonl`, by the same
categories (`shell_minute`, `power`, `thermal` with Energy; `process_gone`, `window` with Errors; the rest with Logs).
Both operations answer only the hub page's main frame on an http(s) address (`readerAllowed`); a browser or an older
shell has neither and the package goes without. Code: `desktop/src/dev-diagnostics-core.ts` (lines, rotation, metrics,
the sender check; tests in `desktop/test/dev-diagnostics.test.ts`), `desktop/src/dev-diagnostics.ts` (Electron wiring,
installed at the end of `desktop/src/main.ts`), `desktop/src/preload.cts`, `spec/desktop-bridge.d.ts`.

## What is recorded (Android)

Android keeps the same event/error limits and ZIP contract in `noBackupFilesDir/developing-persona`. Its recorder starts
from `ProcessLifecycleOwner`; only foreground work has the one-minute ticker. Battery, power-save, thermal and default
network changes come from `BatteryManager`, `PowerManager` and `ConnectivityManager`. Own-process CPU time comes from
`Process.getElapsedCpuTime`. The OkHttp interceptor counts streamed request/response bytes without retaining bodies;
the WebSocket listener keeps only frame type, UTF-8 byte count, connects and drops.

`ApplicationExitInfo` supplies previous exits. ANR and crash reports, including the ANR's parsed main-thread frames,
use the existing error-report path and leave a local copy while recording is on. Every other exit reason becomes an
`app` event with `event: "exit"` and a fixed reason word. This does not add new exit kinds to hub error reporting.
The export reads at most 5000 lines from this process's logcat, only when the owner creates an archive; there is no
persistent logcat process.

**Send to Agent** uses the ordinary Forward targets and file upload. It saves the archive as an uploaded file attachment
and appends the note to the current structured draft, then opens that chat. The owner still sends the draft; this flow
never sends a message itself. A missing binding, failed upload, draft conflict or pairing change leaves the chat closed
and shows an error. The system share URI covers only a real file under `cache/dev-exports`; the provider and target
picker are not exported.

## The package

`persona-diagnostics-<platform>-<yyyyMMdd-HHmm>.zip` (local time), a folder of the same name inside:

```
README.md          for the Agent reading it: what each file is, how to read it, what was scrubbed
manifest.json      format "persona-diagnostics", version 1, platform, os, os_version, device_model, app_version, build,
                   commit, utc_offset_minutes, recording_since, exported_at, window {from, to}, categories, counts
events.jsonl       the timeline above
errors.jsonl       error reports, in the hub's error report format
metrickit/*.json   MetricKit payloads as delivered (strings scrubbed, identifiers kept)
log.txt            this launch's log lines
connections.json   iOS: the connection tally, with the logs, when something was counted
```

The connection tally (#11390) adds a file and a count (`counts.connection_weeks`, on iOS) and changes nothing a reader
already relied on, so the package stays `version` 1: a reader that does not know the file leaves it alone.

On iOS, `commit` is the app's `PersonaCommit`, which only a Release build (every TestFlight build) is stamped with: a
Debug build's package has an empty `commit` unless it was built with `PERSONA_STAMP_COMMIT=YES`
(`docs/operations/testing.md`, "Development builds").

The export page lists four categories; the first three can be left out: **Logs** (`log.txt`, the event kinds
other than the energy ones and, on iOS, `connections.json`, beside the socket's own lines), **Errors and Crashes** (`errors.jsonl`, `metrickit/diagnostics-*`), **Energy and
Performance** (`minute`, `foreground`, `thermal`, `power`, `battery`, `path`, `metrickit/metrics-*`), and **Device and App
Version** (`manifest.json`, `README.md`, always).

## Send to Agent

The export page's first button. It builds the package and opens **Send To**, the targets of Forward To
(`ForwardTargets`, the same rows `ForwardFixedRow` / `ForwardSessionRow`): Persona's main chat, the recent targets, each
computer's live Agents, and search. Picking one opens that chat with the zip as a file card over its box, uploaded to the
computer the chat runs on as any picked file is (`ComposerAttachments.files`), and a note in the box ("Here are the Persona
diagnostics from my device (since …). Please find out what is using the battery and what is failing.") unless the box
already holds words. Nothing is sent until the owner sends it; the Agent reads the zip from that computer. Persona with no
main conversation yet: the new conversation's box. **Share File…** is the system share sheet.

On the web the export dialog's first button is the same: Send To lists Forward to's targets with its rows (exported from
`views/chat/ForwardDialog.tsx`), and the zip goes into the chosen chat's composer files (`lib/composer-files.ts` `addFiles`,
uploaded to that chat's computer) with the note put in the box before any words it holds (`sendDraft` `unsent`).
**Download** saves the zip.


## What is never included

Message text, knowledge base content, keys, tokens, passwords, cookies, pairing links, the device's name, the hub's
address, machine names, accounts. Recorded lines carry no free text; free text that does go in (error messages, log
lines, MetricKit strings) passes `DevScrub.text`:

- a URL keeps its path only (no scheme, host, query or fragment, so no pairing code or token);
- `Bearer` / `Basic` values, `token=`, `key=`, `secret=`, `password=`, `cookie=`, `authorization:` values, JWTs and runs of 32
  or more letters and digits become `<redacted>`; e-mail addresses `<email>`; IPv4 and IPv6 addresses `<ip>`; host names
  `<host>`; `/Users/<name>` and `/home/<name>` `~`;
- each text is cut to 1 KB.

Route patterns are the web error reports' (`pathPattern` in `web/src/lib/error-reports.ts`): every path segment that is
not one of the API's words is `:id`. `DevModeTests` keeps the Swift list equal to the web's.

## Code (iOS)

`apple/Persona/DevMode/`: `DevMode` (the switch, lifecycle, device observers, the minute roll), `DevRecorder` (counting
and lines), `DevConnections` (the connection tally: its opens and weeks), `DevStore` (files, limits), `DevMetrics`
(MetricKit), `DevPackage` (the zip), `DevScrub`, `DevModeViews`
(About's group, Export Diagnostics), `DevSend` (Send To and the hand-off to a chat's box), `MeetingTapesPage` (Replay a
Meeting Tape; the tapes are `MeetingTapeLibrary`, the player `MeetingReplay` in `apple/Persona/Recording/`). Shared code reports through `apple/Shared/DevProbe.swift`; requests through
`HubRedirects.finished` in PersonaWire. DEBUG: `-openAbout YES` opens About, `-openAbout export` its export page.
Tests: `apple/PersonaTests/DevModeTests.swift`, `apple/PersonaTests/ConnectionTallyTests.swift`.

## Code (web)

`web/src/lib/dev-probe.ts` (the switch's key, the WebSocket wrap), `web/src/lib/dev-mode.ts` (recorder, store, hooks),
`web/src/lib/dev-scrub.ts`, `web/src/lib/dev-package.ts`, `web/src/views/settings/DevModeSettings.tsx` (About Persona's
rows), `web/src/views/settings/DevExportDialog.tsx` (export, Send To, Download), `web/src/features/todos/model/view-shadow.ts`
(the To-dos view's shadow). Tests: `web/src/lib/dev-mode.test.ts`, `web/src/views/settings/DevModeSettings.test.tsx`,
`web/src/features/todos/model/view-shadow.test.ts`.

## Code (Android)

- `core/contracts/DevScrub.kt`, `DevProbe.kt`: shared scrub rules and no-op-by-default probes.
- `core/data/dev/`: `DevStore`, `DevRecorder`, `DevPackage` (bounded JSONL storage, counters and the real ZIP writer).
- `core/hub/dev/DevHttpInterceptor.kt` and the WebSocket hooks in `OkHttpHubConnection.kt`: network numbers without
  payloads. `ErrorReportQueue.localCopy` leaves the existing hub queue semantics unchanged.
- `app/dev/DevelopingPersona.kt`: the device-local switch, lifecycle/system observers, export and sharing.
- `feature/settings/DevelopingDiagnosticsPages.kt`: About's switch and deletion warning, and the export page.
- `feature/chat/DiagnosticsHandoff.kt` and `app/dev/DiagnosticsShare*`: Forward targets, uploaded file draft and chat
  handoff. `app/navigation/DevScreens.kt` supplies fixed page names without route arguments.

The JVM tests exercise real files, ZIPs, loopback HTTP/WebSocket traffic, structured drafts and Robolectric Intents.
`DevelopingDiagnosticsShotsTest` and `DiagnosticsShareShotsTest` render the pages in every style, language, light/dark,
phone/tablet width and default/large text. These are native JVM Compose captures, not evidence of physical-device
thermal, battery, ANR, logcat or system-share behavior; those still require a device when such a change needs it.
