# Engines: herdr, Herdr App, Orca, Tern, Rex, Paseo, Cindy

**One engine per computer**: Persona sees and drives Claude Code / Codex through an engine. Each computer selects one; Agents shows live sessions only from it.
Sessions elsewhere (other engines, ordinary terminals) appear only as "Not running" history. Default herdr preserves existing machines without an explicit choice.

Exception: **"herdr + Herdr App (side by side)"**. Choose this when both classic herdr and Herdr App operate on one computer (typically local terminal herdr,
plus Herdr App on another computer opening terminals through remote). Sessions merge; boundaries are in "Side by side" below.

On computers with **botmux**, its sessions appear alongside the selected engine ("botmux" below). It is not an engine option and needs no selection.

**Paseo** and **Cindy** are terminal-free: they run Agents through provider SDKs and runtimes, with node connected through a bridge; see `docs/integrations/paseo.md` and `server/internal/node/cindybridge.go`.

## Host sets (#2576)

A computer can **enable several engines at once**, any combination of herdr, Herdr App, Orca, Tern, Rex, Paseo and Cindy, or none at all, plus a **default engine**:
where new sessions started from the phone go. A retired session resumes in its previous terminal host while that host remains enabled;
an explicit host selection overrides it. Other offline sessions use the default host. Each single choice above is a special case of a set: herdr is `{herdr}`, and
"herdr + Herdr App" is `{herdr, herdr-core}` with herdr the default. API and storage: `docs/protocol/protocol.md` "Engines" ("Host sets").
The web is wired: the machine page's Engine section is a checklist with "Make default", New Agent has "Open in", and a session live in
two hosts shows "Also open in …" above the composer. iOS and Android still pick one engine, and the single engine picked keeps working;
iOS and Android also offer "No Terminal Engine", the empty set, last in Choose Engine when the hub reports `engine_hosts` (#3302).

- **Only enabled hosts are registered.** An engine that is not checked gets no client, no polling and no event subscription; it is only probed once in
  `node.info` (for "Detected"). An engine that is not installed or not running does not slow the others down.
- **Hosts do not hold each other up.** Each host refreshes in parallel with its own timeout, and its events refresh only itself. A host that is down
  (unreachable, `not_running`) clears only its own sessions; the others are listed and driven as usual. This is the same mechanism as herdr-both
  (`docs/plans/plan-orca-support.md` "Side-by-side engines").
- **Default engine**: new sessions, offline sessions without a remembered terminal host and the mode menus go to the default engine; renaming a space goes to the
  enabled host that lists it, the default one first (`spaceOrganizer` in `node/rename.go`: space ids carry no host kind). Per-pane
  operations (reading the screen, sending, keys, closing) still go to the host the pane belongs to. Changing only the default resets nothing; changing
  the set itself works like switching engines did (live terminal views end, the session list is rebuilt). A session without a remembered terminal host
  continues in the default engine: Orca continues only Claude sessions (`orca_resume`); Paseo takes over sessions it can resume.
  Retirement checks the session's own host under any default: it stops the agent and keeps its terminal,
  unless the user requests closing it. Runtime-owned sessions remain protected.
- **The empty set**: no terminal host. Agents shows transcripts, runtimes (DSH, pi, opencode) and botmux sessions only; starting or continuing a
  session from the phone is refused with "这台电脑没启用终端引擎" (`engine_missing`) and nothing is sent. This is the choice for a computer
  whose sessions are only the desktop apps' (Claude's and Codex's, `docs/integrations/claude-integration.md`, `docs/integrations/codex.md`):
  they stay listed, and the node probes no engine. New Agent with the launcher `claude-desktop` still starts a session there, in Claude's
  desktop app (`docs/integrations/claude-integration.md`, "New sessions in the desktop app"), and with `codex-app` a chat in the Codex App
  (`docs/integrations/codex.md`, "New chats in the Codex App").
- **One session live in two hosts** (say herdr `--resume`d a session that is still open in Orca): it is listed once and owned by one host
  (`node/ownership.go`). The owner is, in order: (1) the pane whose claude process runs it (`~/.claude/sessions/<pid>.json`); (2) the pane whose last
  SessionStart hook named it; (3) the default engine's pane; (4) otherwise herdr, Herdr App, Orca, Tern in that order. Messages, keys and interrupts go to
  the owner only, and its state is the only one used; the row's `also_open_in` lists the other hosts that have it open, so clients can warn of the
  conflict (the web does; the apps do not yet). Two panes of the same host, or a live pane and an offline row, behave as before.
- **Performance** (2026-11-06, one Mac with herdr 0.9.5 and Orca 1.4.250, `go test -tags enginehostslive -run EngineHostsLive ./internal/node/ -v`,
  40 rounds each, Orca refreshed once a second meanwhile as its watcher does; three runs, a typical one below, the other two of the same order):

  | | herdr only | herdr + Orca |
  | --- | --- | --- |
  | herdr send | p50 0.5 ms · p95 1.7 ms · max 2.5 ms | p50 0.5 ms · p95 1.1 ms · max 3.3 ms |
  | refresh after a herdr event (herdr only) | p50 82 ms · p95 205 ms · max 267 ms | p50 92 ms · p95 320 ms · max 430 ms |
  | refresh of every host | p50 62 ms · p95 125 ms · max 179 ms | p50 106 ms · p95 519 ms · max 1104 ms |

  In two of the three runs the herdr + Orca send had one max of about 170 ms (herdr alone had one of 149 ms in one run); p50 did not move.
  The refresh of every host is slower because it now includes Orca, the heaviest host. herdr's own refresh gains about 10 ms at p50 and about
  0.1 s at p95: the session merge after it (`setAgentsAt`: process lookups, session files) covers Orca's agents too.

  A herdr send waits for nothing of Orca's. Measured again for #5816 on fakes (2026-11-07,
  `PERSONA_BENCH_ENGINEHOSTS=1 go test -run EngineHostsLatencyOnFakes ./internal/node/ -v`: a fake herdr, a fake Orca whose process scan
  takes 170 ms as three `ps` runs do on a Mac, 400 rounds each), a herdr send is p95 0.1 ms with and without Orca. herdr itself answers
  about 0.1 s late now and then with nothing else asking it (`docs/lessons/agents.md`).

  The merge asks only the hosts that answered the refresh what their panes run; another host's pane keeps the pid its own refresh found.
  Only the owner of a session live in two hosts still asks both, so it does not change with the host that refreshed. When the merge asked
  every host, herdr's refresh ran Orca's process scan whenever an Orca pane was due for a look (p95 174 ms on the fakes, 1.6 ms now), and
  Orca's poll asked herdr about every herdr pane due for one.

"Engine" below means this choice. APIs, storage, and node switching: `docs/protocol/protocol.md` ("Engines"); design / trade-offs: `docs/plans/plan-orca-support.md`.

## Engines

| Engine | App name | Where Claude runs | Detection |
| --- | --- | --- | --- |
| `herdr` | herdr | Run `herdr` in a terminal, then `claude` / `codex` in its panes (node starts it on demand if absent; see "When herdr is not running") | Connectable herdr socket or installed `herdr` command |
| `herdr-core` | Herdr App | Terminals opened in Herdr desktop app (herdr-core behind it) | Responsive herdr-core socket |
| `herdr-both` | herdr + Herdr App | Both above | Both runtimes detected |
| `orca` | Orca | Terminals and native Claude chats (structured sessions) in Orca desktop app, or its remote runtime launched here over SSH by Orca on another computer | Orca desktop / remote runtime running, with a Persona-compatible version |
| `tern` | Tern | Terminal blocks in Tern's sessions, in any of its windows ("Tern" below) | `tern` found (PATH, `~/.local/bin`, the app bundle) and its session daemon answering `tern inspect` |
| `rex` | Rex | Terminal blocks in Rex's sessions and windows ("Rex" below) | `rex` found (PATH, `~/.local/bin`, macOS Application Support / app bundle) and `rex --autostart=false api call server.status` answering |
| `paseo` | Paseo | Paseo Agents (desktop app, `paseo` command; Claude Code, Codex, ACP all count) | Paseo daemon running; address in its home's `paseo.pid` responds |
| `cindy` | Cindy | Cindy Agents (desktop app and `cindy-host`; Claude Code, Codex, Pi) | Cindy installed, running, or its loopback host endpoint / profile database present |

herdr and Herdr App are from the same developer but separate: neither sees the other's sessions. If using only one, select it;
if both are used and should appear on the phone, choose "herdr + Herdr App (side by side)."

## Switching

1. **During pairing**: after QR / manual pairing, asks "Which engine does this computer use?" (the hub machine). Each option says "Available" or "Not detected."
   Select one; herdr is default (or previous choice), confirmation switches; no Skip.
2. **Anytime later**: **Settings › Machines › computer › Engine**, select another. Choices are per computer; one can use herdr, another Orca.
3. **Offline computers** can change too, taking effect when connected.

iOS / Android have both entry points (Android Machines is under Agents: **List settings › Machine monitoring › computer**, or tree computer-header dashboard button, then **Engine and connection › Engine**). Web selects directly in Machines (**sidebar Machines › computer**, Engine section, #2071). Choice is stored on hub, not phone, and matches across phones, web, and Android.

## After switching

- Agents immediately shows new-engine sessions; old-engine sessions become Not running, with history retained.
- Live terminal / phone takeover disconnects; reopen in a new-engine session.
- No running Claude in any engine is closed or modified.

## Capabilities by engine

| Capability | herdr | Herdr App | Orca | Tern | Rex | Paseo |
| --- | --- | --- | --- | --- | --- | --- |
| List sessions, show state (Needs you / Working / Unread) | Yes | Yes | Yes (polls each second) | Yes (`tern events` pushes changes) | Yes (`rex events --all --json` pushes changes) | Yes (Paseo pushes changes) |
| Send messages (multiline as one message) | Yes | Yes | Yes | Yes | Yes (`block.event` paste + `enter`) | Yes (while working, injects into current turn) |
| Keys (Enter, Esc, arrows…), interrupt | Yes | Yes | Yes | Yes | Yes (`rex send-key`) | Interrupt only |
| Answer dialogs (permissions, options) | Yes | Yes (screen-based) | Yes (screen-based) | Yes (screen-based) | Yes (screen-based) | Yes (cards, also answerable in Paseo app) |
| Close sessions from phone | Yes | Yes | Yes | Yes | Yes (`block.close` / `session.close_window`) | Backend supports archiving in Paseo; app button currently only for terminal sessions |
| Create sessions from phone; Resume a stopped one from its terminal page (a message does not, #4754) | Yes | Yes (a new tab in the Herdr App space for the directory, #2532) | Yes (create; resume a stopped Claude session, #5548) | Yes | Yes (`session.create` / `session.new_window` with `focus: false`) | Yes (each Paseo provider is a launcher; a message continues a Paseo session) |
| Create a worktree from a workspace ([terminal.md](terminal.md), "23. New worktree…") | Yes (herdr 0.9 creates it; older versions fall back to git) | Yes (Persona falls back to git and opens a Herdr App space) | Yes (Orca creates it under the original workspace and owns the checkout directory) | No | No (git fallback) | Yes on the web (Paseo creates it in its own worktrees folder as a Paseo workspace, #6295, and deletes one it made, #6297; `docs/integrations/paseo.md` ("Spaces and workspaces")) |
| Rename spaces / tabs | Yes | Yes (written into Herdr App's layout) | No | Yes | Yes (`session.set_label` / `session.set_window_label`) | Spaces (Paseo's projects) and workspace titles on the web, #6297; no tabs (backend supports session names; app button currently only for terminal sessions) |
| Live terminal, phone takeover | Yes | No | No | No | No | No (no terminal) |
| Colored screen reads, draft detection | Yes | No (plain text, dialogs only) | No (plain text, dialogs only) | Yes | Yes (`rex capture --format=vt`) | No (no terminal) |
| Change permission mode / thinking effort | Yes | Supported, untested | Supported, untested | Supported, untested | Supported, untested | Yes (end-to-end tested) |

Windows: herdr has native Windows support since 0.9.5; except live terminal, capabilities match above. Orca works too. See [windows.md](../operations/windows.md), "herdr."

Antigravity CLI panes can show chat history after the hook installed by `herdr integration install antigravity-cli`
reports the pane's conversation ID. Persona reads that conversation's local `transcript.jsonl` from the node's Antigravity data
directory and uses the reported ID to restore Persona-started panes with `agy --conversation`. Until herdr
reports a usable ID, the pane remains accessible through the terminal and can still receive prompts.

Orca's native Claude chats (structured, no terminal) connect through node's bridge: list, state, send text / images, interrupt, close,
answer approvals / questions, change permission / thinking (no plan mode); no screen reads, keys, or live terminal. Chats appear only after their first message;
Codex chats not listed in this version. See `docs/plans/plan-orca-support.md` ("Structured sessions").

Unsupported actions explain failure instead of partially acting. Orca rejects sending without entering anything when its pane is still
there with Claude not in front, saying "Claude is not in foreground (possibly suspended); nothing sent," or asking to reopen on the computer.

Orca New Agent creates terminals inside already-open Orca workspaces or subdirectories, starting in the selected directory. Unopened Git-repository directories attempt automatic registration; any other directory gets a terminal in the first local Orca workspace that `cd`s there (refused only when Orca has no workspace open). The `cd` is typed into the shell's line editor, so a directory whose name holds a control character (a tab, a line break) is refused before any terminal opens (`directory_untypable`). It never takes over an existing terminal.
Resume of a running session returns it. A Claude session whose agent exited is continued like on herdr (#5548): a new terminal in the Orca workspace holding its directory (the same rules as New Agent), `cd` there, the launcher that started it with `--resume`, then the message. Never a second writer: a session a live claude still has open (Claude's desktop app, another terminal, suspended) is refused (`open_elsewhere`), and so is one whose last pane is still open (its agent may be suspended there; close the pane to resume elsewhere). A session an Orca native chat owns is refused too. Codex, TRAEX and pi sessions are not resumed in Orca. Orca's own `terminal create --agent claude --resume` is not used: it exists only in some Orca builds' CLI, and it starts Orca's claude command, not the session's launcher.

Herdr App New Agent (#2532) opens a terminal the way the app does (the machine's shell, kept, with the app's terminal identity) as a new tab of
the Herdr App space whose folder matches the directory, else of a new space for the directory, and adds it to the app's layout document, so the app
shows it like a tab opened there. The core's returned shell determines command syntax, file sourcing and startup probes; an unknown foreground
never receives a start command. The document is edited, never replaced: only that tab (and space) is added, at the version read, redone after a
conflict, including folder selection and terminal labels. When there is no layout document (Herdr App never ran on this core) or it is in a form
Persona does not know, nothing is written and an automatic start opens outside the app's spaces, grouped by its directory. A start explicitly
targeting an app space is refused if its layout or space cannot be resolved. A Not running session is
continued as on herdr: a new tab, its launcher with `--resume`. Space and tab renames write the same document.

Selecting `herdr-both` attaches both herdr and herdr-core hosts:

- Agents merges sessions; spaces / tabs, state, screen reads, messages, keys, closing route to the pane's own host, as when selected alone.
- **Phone create / continue Not running / space rename use classic herdr** (complete capabilities); New Agent can also be pointed at Herdr App
  in its host picker. Continuing an exited Herdr App session from phone opens a new herdr pane with `--resume` (same Claude session ID, continuous
  history, different window). herdr-core live terminal / phone takeover remain unavailable.
- Neither slows the other: snapshots fetched concurrently with separate timeouts; herdr-core events refresh only herdr-core,
  herdr events only herdr; a failed side clears only its own online state.
- Available only when both herdr and herdr-core are detected (can preselect offline, effective on reconnect; if only one exists,
  it works normally while the other list is empty).
- Orca next to herdr goes through "Host sets" above; there are no more combination options.

## Task workspaces (#4786)

On classic herdr a top-level task leader and its workers share one workspace named after the leader's task. This is
placement and cleanup for that task only; ordinary tasks, Herdr App, Orca, Tern, Paseo and Cindy keep their existing
workspace behavior.

- **Opening.** A leader's `session.start` carries `task_id` and `workspace_label`. With no recorded `workspace_id`, its
  first tab opens a new workspace under the task title instead of the directory's basename. The manager records the
  intended label before dispatch and the actual workspace ID from the start response or a later live session row,
  verified against the node's persisted task record before that row may change the ID;
  the node also persists `task_id`, `workspace_id` and `workspace_label` with each session it starts.
- **Workers.** A worker's start carries the same `task_id` and the recorded `workspace_id`, so its tab opens beside the
  leader. A writable worker still gets its own Git worktree and runs there: `worktree.create` with
  `open_workspace:false` makes the checkout through the node's existing Git fallback, without opening another herdr
  workspace and without needing a new herdr RPC. The leader uses the same
  checkout-only rule when it needs a new worktree. The checkout and branch are not deleted when the task ends.
- **Rework and continue.** A stopped session's explicit resume reads its recorded task identity from the node, and the
  manager supplies it through the same start path. A new tab selects the recorded workspace by ID, never a workspace
  whose label happens to equal the directory's basename. If the task workspace is already gone, it is recreated under
  the retained task label and the new ID is recorded. A resume that can reuse the kept shell opens no tab, as before.
- **Closing.** Once the top-level task is accepted or cancelled and every Agent under it has been retired, the existing
  task-close pass calls `workspace.close` with the recorded `task_id` and `workspace_id`. A per-task lock keeps a start
  or resume from racing the final activity check. The node proves that exact pair against its own session records and
  refuses a live workspace, another task's workspace, a user-opened workspace or an unrecorded ID. A rename does not
  transfer ownership; matching only a label is never enough. Already-gone and repeated closes are idempotent.
  Closing the workspace closes any shells left in it after the Agents stopped; this task-owned exception does not
  change ordinary terminal retention. Git checkouts and transcripts stay.

## Agents run in a terminal

#4754. An Agent runs on top of a terminal: every Agent Persona starts runs in the computer's terminal engine, and its terminal outlives it for a grace period (#10111, "Finished terminals close after a grace period" below). The Agents list shows only Agents, but each has a terminal under it.

- **Starting**: every start or resume of an Agent goes through the node's `session.start` / `session.send`: New Agent in the apps and the web, the assistant's `agent_start`, `dispatch_start` and `task_plan`, auto-review reviewers, launcher repair, the Lark bot's `/start` card, extension tokens, Run Agent Here. On herdr, Herdr App, Orca and Tern the Agent runs in a new tab (or the pane picked), never headless (`claude -p`, `codex exec`, an SDK process). A start into a workspace it names starts herdr's server first when it has stopped, as every other start does ("When herdr is not running").
- **Not Agents, headless by design**: assistant conversations (the main chat, leader chats, scheduled tasks, channels, meetings, the Inbox AI twin, Lark `/agent`), listed in the Persona tab and not under Agents, and model calls (titles, triage, the launcher Test).
- **No terminal, by the user's choice**: an engine that runs Agents through provider SDKs (Paseo, Cindy), Orca's native chats and runtimes (DSH, pi, Antigravity, daemons) own their sessions; Persona leaves them as they are. So does Claude's desktop app, for a session New Agent starts there with the launcher `claude-desktop` (`docs/integrations/claude-integration.md`, "New sessions in the desktop app"), and the Codex App, for a chat started with `codex-app` (`docs/integrations/codex.md`, "New chats in the Codex App").
- **Stopping keeps the terminal**: Persona's own passes stop an Agent with node `session.stop`: a finished task's, an idle one, a cancelled task's, and `/exit` sent by `agent_send` (`docs/assistant/todos.md` "Stop Agents of completed tasks"). The Agent process quits and its tab stays with the shell it ran in, until the grace period below has passed.
- **Closing it when the session ends, by the owner's choice** (#10355): the setting `close_ended_terminals` (`persona_settings` key
  `assistant.close_ended_terminals`; Settings › Assistant › Agent follow-up on the web, Assistant Profile › Agent Follow-Up ›
  Terminals on iOS, Assistant settings › Terminals on Android) is **off by default**, and off nothing above changes. On, the
  manager closes an Agent's tab when its session ends, through the user's close paths only (`session.retire` with
  `close_terminal`, plus `after_exit` for the shell an Agent that quit left): a done or cancelled task's idle Agent instead of
  stopping it, and an Agent seen running that then quit, told to (`/exit`, stop cause `exit`) or by itself (no stop cause).
  Not an end, so the terminal stays (until the grace period below, if it is on): the user's Stop, an idle stop, a restart. Never closed: a busy Agent (working, waiting
  for an answer, background work, running subagents or workflows, a `/loop`, queued messages, a server it runs), assistant
  conversations, other tools' sessions, Agents Persona did not start, panes without Persona's mark for the session. An
  Agent that quit has its shell closed only on Herdr (the #9648 checks). The transcript stays and Resume opens a new tab
  (`server/internal/assistant/agent_terminal_close.go`).
- **Closing one task's tab after acceptance** (#10201): the owner can opt in when starting a new planned task. After
  `task_review` saves its acceptance as done, Persona can close the original Herdr tab of that task's Claude Code or
  Codex Agent. It checks the tab and running process from dispatch, the current layout and focus, open terminal views,
  input, pending interactions and live work. A split tab or a workspace's last tab stays open. The transcript and
  worktree stay. See `docs/assistant/todos.md` ("Close a task's tab after acceptance").
- **Close already dispatched when the Agent exits** (#9648): a normal `session.close` can still close its original
  marked Herdr shell. The node reads the selected server directly, requires a matching session marker, no active or
  rebound Agent and a known foreground shell, then rechecks ownership and layout before closing only that pane.
  An identified target counts as already gone only after a complete direct inventory confirms absence; uncertain or
  changed ownership returns a refusal. Historical state without any terminal identity keeps its existing idempotent
  response. These separate snapshot and close calls add no atomic engine claim. Sibling terminals, files and
  worktrees are left alone; ordinary Stop and automatic task completion still retain the terminal.
- **Retrying a stalled Orca start** (#9633): before closing its used kept tab for a replacement, the node rechecks
  that no session or Agent has taken the pane after terminal lookup, including a stale-handle refresh. Recovery
  during lookup leaves the old tab and its expiry intact; the retry uses a fresh tab, as when recovery is seen
  earlier. An actual close failure still fails the retry without a replacement. The check and engine request
  remain separate operations; this is not an atomic guard against recovery after the final check.
- **A stopped Agent is read-only** (#4754): an Agent of a terminal engine that is not running, stopped by Persona or by the user, exited by itself or cut off by a restart, keeps its conversation and its terminal (the session's terminal opens the shell it left), and takes no messages: node `session.send` refuses it as `agent_stopped` with nothing typed, before any resume, and its row's `capabilities` have `read` and `terminal` without `send` (`server/internal/node/stopped.go`). The assistant's `agent_send` and `agent_interrupt` refuse a plain message too. Two things start it again, both through the same node call, `session.start` with `resume_session_id`: the user's **Resume** on its terminal page, and the assistant's explicit resume, `agent_send` with `resume: true` and `resume_why` (#8099, `docs/assistant/todos.md` "How to reuse"). The call types the session's launcher with `--resume` into the shell the Agent left when that shell is at rest in the session's directory, else it opens a new tab. Not covered: assistant conversations, Claude's desktop-app sessions, runtimes and engines that run Agents themselves, and a branch nothing has started yet (its first message starts it).
- **Resume in Terminal** (#5521): the chat's ⋯ on iPhone and the web offers it for a stopped Agent, the same rule on both (`Session.terminalResume(on:)`, web `terminalResume` in `web/src/lib/agent-resume.ts`). It opens the session's terminal page and runs the user's Resume there. This covers a Claude Code typed into a terminal of the user's own (a plain terminal, an SSH login) that has closed since: the node lists it stopped like any other, and resumes it in a new tab in its folder. A Codex thread nothing has open is a stopped Agent too (`node/codex_registry.go` gives it `terminal` and the stopped reason while its source stays `codex_history`), with the same line and the same Resume.
  - **The node decides**: it marks a row stopped only where a terminal engine can host the resume, so the clients add nothing of their own beyond the row and the computer's engine set.
  - **No terminal engine** (the empty set, #3302): an Agent that isn't running gets the item disabled, with "No terminal engine on <computer>". Its terminal page says the same and offers no Resume.
  - **Never offered**: while the session runs, in Persona's terminal or in one of the user's own (`claude_cli`: a second `claude --resume` would write the same transcript, and the node refuses it as `open_elsewhere`); while it starts; for assistant conversations; for the desktop apps' sessions (a message reopens those in the app, #4300); for rows of older nodes, which carry no capabilities.
  - **Tests**: `server/internal/node/outside_resume_test.go` (herdr: a new tab with `--resume`; no engine: no terminal and `engine_missing`; open elsewhere: no terminal and `open_elsewhere`, nothing opened), `TerminalResumeTests`, `ResumeInTerminalTests` and `web/e2e/resume-in-terminal.mjs` (mock `MOCK_RESUME_TERMINAL=1`).
- **A task goes on with a new Agent**: when every Agent of a task has stopped, the user's message on the task's card or its Retry starts a new Agent for the task (same computer, directory and launcher as its latest Agent, linked to the to-do; `server/internal/assistant/tasks_successor.go`), its first message the user's after who worked on it before and that Agent's last reply. Persona's own follow-ups to a stopped task (relayed answers and other supervision) start nothing and are refused. The one task-scoped exception is rework of a retired herdr leader: it resumes the same session in the recorded task workspace (#4786).
- **Finished terminals close after a grace period** (#10111, `server/internal/node/finished_close.go`): the node closes a terminal a stopped Agent left once it has sat unchanged for `PERSONA_AGENT_TERMINAL_GRACE` (a Go duration in the node's environment, default `30m`; `0` or `off` keeps them until the user closes them, as before). It looks every minute, on a fresh snapshot, and closes a pane only when all of these hold, checked again under the session's recovery lock right before closing:
  - Persona opened it for an Agent: herdr's `persona_tab` mark, or on Orca, which keeps no marks, the node's in-memory record of the terminals it opened for Agents (a node restart forgets it, and those terminals are then left alone);
  - no Agent is in it or bound to it (working, idle or waiting for an answer), no start is in flight, no failed start keeps it for a retry and no Codex waits for its thread there;
  - only a shell is in front (herdr `pane.process_info`; Orca: no agent process carries the pane's key), and no client has its terminal open;
  - its screen has not changed for the whole grace period; new output restarts the clock.

  Terminals the user opened, a pane picked for Run Agent Here and the panes of other engines (Tern, Herdr App) are never touched. A workspace is never closed directly: closing the pane closes its tab when it was the only pane, and the engine drops a workspace with its last tab, so a workspace goes only when everything in it was such a terminal (the first shell of a worktree the assistant creates for an Agent is marked too, #10624: `worktree.create` with `for_agent`, herdr's `persona_worktree` token or the node's in-memory record; it closes with the same checks once every other pane of its workspace is a finished Agent terminal, so the whole workspace goes; a worktree the user creates is never marked). When the engine does not answer nothing is closed that round; a failed close is retried on a later round; the clocks live in memory, so a node restart closes later, never sooner. Closing never changes the session's stop cause: an Agent the hub stopped as `task_completed` or `idle` can still be resumed by the assistant afterwards (an Agent stopped by hand, or one that exited by itself, has no cause and waits for the user's Resume, as before). Resume needs nothing new: within the grace period it runs in the kept shell (the same recovery lock keeps the two apart), after it in a new tab. Running the Agent with `exec` so the pane exits with it was rejected: the last output and the screen a failed start is diagnosed from would go at once, Resume could not reuse the shell, and the typed start's `&&` chains and per-shell quoting do not survive `exec` in fish or PowerShell.
- **The Agent mark** (#10111): every terminal Persona opens for an Agent starts with `PERSONA_AGENT=1` in its environment (herdr `workspace.create` / `tab.create` `env`, the Herdr App's `terminal.run` `env`; `host.EnvLauncher`, with the startup switches in "What a start tab's shell starts with" below), so a shell's startup files can tell it from one the user opened, for example to skip a prompt's git status daemon: `[[ -n $PERSONA_AGENT ]] && …`. The typed start also puts `PERSONA_AGENT=1` in front of the Agent command (`agentMarkEnv`, `server/internal/node/agentenv.go`), so the Agent's processes carry it on every engine, including Orca, whose `terminal.create` takes no environment (there the shell itself does not have it). A herdr without `env` support is asked again without it. Terminals the user opens never get it.
- **Cleanup by hand**: `panes.list` labels each terminal a stopped Agent left with `agent_session`; **Close finished terminals** (`POST /api/v1/machines/{mid}/panes/close_finished`, preview first) closes the ones the user confirms now, after checking again that only a shell is in front.
- **Who closes a terminal**: the user (**Close** on a session, **Archive and close**, force archive of an Agent blocked on a confirmation, closing a pane or tab from the terminal list), the grace period above for a finished Agent's terminal, and a finished review's reviewer, an internal helper, which closes with its worktree copy.
- **Checks**: `server/internal/assistant/terminal_guard_test.go` pins the manager functions that ask a node to close something, `server/internal/node/terminal_guard_test.go` the node functions that close a pane or tab; `TestHerdrStopKeepsTheTerminal`, `TestOrcaRetireKeepsTheTerminal` and `TestStartStartsHerdrFirst` cover the behavior.

## Tern

[Tern](https://stencil.so) is Stencil's multiplexing terminal. The node drives it through its `tern` command line (`server/internal/terncli`,
`node/hosttern.go`, #2154): nothing else to install, and `tern` is found on PATH, in `~/.local/bin` or in the app bundle
(`/Applications/Tern.app/Contents/MacOS/tern`), so a node with only the system's PATH (launchd, systemd) finds it too.

- **Spaces, slots, panes**: Tern's sessions are spaces and their tabs slots; every terminal block is a pane, `tern:<block id>` (the number Tern
  puts in the block's processes as `TERN_PANE`, which hooks read: `nodeclient.HookPane`). Browser and file blocks are not panes. Every open window is
  listed (each `tern` command names its window with `--window`); a session two windows show counts once.
- **Agents**: Tern does not say what runs in a block (a block's `program` is the shell it started), so a look asks `tern process` for each terminal
  block's foreground process, four at a time: `claude`, `codex` (also behind `node` / `bun`). A shell or classic herdr's client in front is no agent
  of Tern's (herdr run in a Tern tab keeps its own panes). Tern reports no agent state, and its own `working` flag misses Claude thinking quietly:
  a Claude's state is the status Claude Code keeps in its `sessions/<pid>.json` (busy, idle, shell when idle with a background command running, or waiting on a dialog of its own), else its transcript
  and hooks as on the other hosts (`docs/protocol/agent-states.md`, "Claude in a Tern or Rex block").
- **Changes**: `tern events` for the first window (blocks spawned, exited or closed, layouts, titles, working directories, bells; a bell is urgent).
  The node still snapshots every 10 seconds, which covers other windows. Every `tern` call is a client of its own, so the `client_*` events are
  left out of the stream.
- **Screens and input**: `tern capture --ansi` (colors, so drafts are told from placeholders; rows end in `\r\n` as the node expects),
  `--scrollback` for "recent". `tern send … text` types, `tern send … keys` presses keys under Persona's own names (`enter`, `esc`,
  `shift+tab`, `ctrl+u`…). A prompt is a paste and then enter, each after checking an agent is in front: otherwise the shell would run the text,
  so it is refused with nothing typed.
- **Starting, renaming, closing**: a new space is `tern new session` (no window switches to it), a new slot `tern new tab` in the session's
  directory, then the launcher is typed with `tern run`. `tern rename` names sessions and tabs. Closing a pane closes its block; a session left with
  no blocks that no window shows (one Persona opened) is ended, since Tern keeps an empty session otherwise.
- **Not yet**: live terminal and phone takeover (no stream), new worktrees, events from windows other than the first.
- **Live test**: `go test -tags ternlive -run TernLive ./internal/node/ -v` opens a hidden session of its own, drives it and closes it again;
  the user's own sessions are only counted. Pitfalls: `docs/lessons/tern.md`.

## Rex

Rex is a multiplexing terminal driven through its `rex` CLI and server control API (`server/internal/rexcli`,
`server/internal/node/hostrex.go`, #12334). `rex` is located on PATH, in `~/.local/bin/rex`, or on macOS inside
`~/Library/Application Support/com.superlogical.rex/RexServer/Versions/Current/Rex Server.app/Contents/MacOS/rex` and
the Rex app bundles. Every command passes `--autostart=false` so probing and snapshotting never launch a Rex server
unexpectedly.

- **Spaces, slots, panes**: Rex sessions are spaces, windows are slots, and `com.superlogical.terminal` blocks are
  panes stored as `rex:<block id>` (where `<block id>` is Rex's full `block:<uuid>`, matching the `REX_BLOCK`
  environment variable Rex sets in block processes and Persona hooks read via `nodeclient.HookPane`). Non-terminal
  blocks (such as onboarding) are skipped.
- **Agents**: Rex does not report which Agent runs in a block, so each refresh calls `block call process` (up to four
  in parallel) and identifies `claude`, `codex` (including behind `node`/`bun`), and `traex` from the foreground
  process while ignoring shells and nested classic `herdr`. Claude Code state is read from `sessions/<pid>.json`,
  transcripts, hooks, and screen dialogs, using the same path as Tern.
- **Changes**: `rex --autostart=false events --all --json` streams server, session and block events (block events
  come only with `--all`). Every `rex` command is a client of its own, so the `client_*` server events and the
  `client_connected` / `client_disconnected` session events the node's own calls raise are ignored, or each refresh
  would set off the next. Sessions, windows, blocks and labels changing, and the terminal events `process_changed`,
  `child_exited`, `title_changed` and `pwd_changed`, trigger a refresh; `bell` and `desktop_notification` an urgent one.
- **Screens and input**: `rex capture --format=vt --trim` reads colored screens (and `--unwrap` for unwrapped recent
  reads). `Prompt` verifies an Agent is in front, sends a `block.event` paste (neutralizing escapes), pauses, rechecks
  foreground, and presses `enter` via `rex send-key`. `SendText` writes to `rex send` over stdin.
- **Starting, splitting, renaming, closing**: `session.create` opens a space, `session.new_window` (`focus: false`)
  opens a slot, and `session.new_split` (`focus: false`) splits a pane. `session.set_label` and
  `session.set_window_label` rename spaces and slots. `block.close` / `session.close_window` close panes and slots; an
  empty session with no attached clients is destroyed via `session.destroy`.
- **Not yet**: live terminal streaming and phone takeover.

## tmux (approvals only)

A Claude Code the user starts by hand in a tmux pane (`tmux new -s work`, then `claude`) is not an engine. It is listed as a session in an ordinary
terminal and stays read-only, with one exception: its approval can be answered from the app.

Claude Code writes the pane it runs in into its own `sessions/<pid>.json` (`"tmux": "work:@11.%11"`), so no process environment is scanned. When a
dialog holds that pane (a permission prompt, a question, a picker), the session lists as Needs you and gets a screen card on web, Android and iOS.
An answer presses the keys in the pane through the tmux command (`server/internal/node/hosttmux.go`, `server/internal/tmux`); a dialog that changed
since the card was made is refused without a keypress (`prompt_changed`).

Nothing else comes from this host: no spaces, slots, terminal tab, live stream or starting sessions. A message still goes through the session's peer
inbox when it has one. tmux is added next to whichever engine the machine selected and needs no choice.

## botmux

[botmux](https://github.com/deepcoldy/botmux) maps each Lark topic to a Claude Code / Codex / compatible CLI session in its own tmux. It is not an engine:
when `botmux` is on the computer's PATH, node adds a botmux host alongside the selected engine host (`server/internal/node/hostbotmux.go`).
Typical use: work Agents in botmux, personal Agents in herdr, viewed together.

- **Listing and state**: each refresh runs `botmux observe --include-raw` (all sessions from all daemons, reused within 5 seconds, immediately reread after sending; 0.3–0.9 seconds for about 500 local sessions; occasional observe failures retain the previous list). botmux exits 1 when one daemon's probe failed (`unauthorized`, `unreachable`, `daemon_offline`) and still prints every daemon's answer: the sessions of the daemons that answered are listed, a failed daemon's stay as it last listed them, and a message to one of those is refused with its probe error, nothing posted. Only when no daemon answered is the observe a failure (#8949).
  Lists Claude Code, Codex, and the compatible Codex fork (`cli.id: claude-code / codex / traex`), with identical state rules: running sessions use botmux state (dialog `tuiPromptActive` or botmux Needs you → Needs you; `working` / `starting` / `analyzing` → Working;
  others → Done). Suspended (`not_running`, not closed, active within 7 days) appear Not running. For Claude Code, botmux session ID is the Claude ID used to start it, so transcript handling is unchanged.
  For Codex / the compatible fork, observe omits CLI session ID (stored in botmux database), but every prompt starts with `<session_id><botmux-session-id></session_id>`.
  Node searches rollout beginnings in the same working directory (first 512 KB, stopping each file after finding the marker). Matching rollout is the thread (newest-written among forks),
  listed by thread ID and read as rollout history (`docs/integrations/codex.md`); title prefers thread name, otherwise botmux topic title (`botmuxThreads`).
  Without matching rollout, omit: newly opened sessions before initial prompt enters rollout (next refresh finds them), or custom bot-specific `CODEX_HOME` / `TRAE_HOME` directories unknown to node
  (Codex reads known homes, compatible fork only `~/.trae/cli`). `codex-app` (botmux drives Codex through app-server, without session-ID prompts) and other CLIs omitted.
  Space is cwd (like Herdr App terminals outside spaces; trim trailing `/` to avoid duplicates), named by final directory component. If it matches parent case-insensitively, include parent (`Botmux/botmux`, #1836). Derive only from its own path, not other listed sessions, so name-based pin / mute / group-to-do bindings stay stable.
- **Sending**: same for all three, delivered through botmux dashboard `POST /api/trigger` targeting the session, not terminal input. Botmux queues while busy,
  resumes suspended sessions itself (no separate herdr `claude --resume`, also for suspended beyond 7 days / unlisted), and sends replies back to Lark topics.
  Node gets dashboard token through `botmux dashboard current`, memory-only, connecting only to local 127.0.0.1. Claude receives botmux task tags and an external-event explanation.
- **Unsupported**: screen reads, keys, dialogs (answer in Lark), close, rename, create, live terminal; app hides or explains by capabilities.
- **Why it needs you / open in Lark**: reason from `botmux send --attention` goes into `attention_reason` while Needs you;
  app shows it in list rows / chat top. Raw observe topic link (`feishuThreadLink`, HTTPS only) becomes `external_url` for "Open in Lark."
  Both are raw botmux fields, not stable APIs; omit if absent.
- **Summary (`last_text`)**: when not Working / Needs you and botmux replied to the latest message (`previewBotState: replied`), use its last reply (`previewBotFullText`, otherwise `previewBotText`; cards stored as `[卡片: 标题]`, "card: title", strip wrapper, keep title).
  Agent answers go to Lark through `botmux send`, so transcript often ends only with `BOTMUX_NOTHING_TO_SEND`. While working, unreplied, Needs you (topic reply often explains the wait), or transcript has text over 10 minutes newer than this reply (work continued outside botmux), retain transcript's last text.
  Standalone `BOTMUX_NOTHING_TO_SEND` never counts as text in any session: retain earlier text if alone in a turn. A transcript containing only that still counts as spoken; after over 7 suspended days it is listed Not running.
- **Execution failures**: botmux failure notices (warning-symbol prefix, saying it stopped automatic continuation / recovery / restart / handling) are not questions and do not mean Needs you.
  State follows botmux turn (usually Done); reason in `failure_reason`; app shows a red error with Retry (send "Continue" through botmux) and Open in Lark.
  Agent-issued `botmux send --attention` still means Needs you, even with a warning prefix.
- **Stale attention**: botmux clears attention / failure notices only for messages passing through it (Lark, delivery), not terminal or other channels.
  A newer transcript user message after the attention / failure makes it stale: hide Needs you / reason, use turn state.
  Slash commands, `! cmd`, and other Agent reports do not count. Read only the final 384 KB; if a turn wrote more after that message, it may be invisible, conservatively retaining attention rather than falsely clearing.
- **No duplicates**: botmux sessions using herdr backend already are herdr panes, so botmux host omits them. If botmux Codex also appears in Codex history / desktop lists (same rollout),
  keep only botmux's actionable row, not read-only history. Botmux Codex / compatible-fork sessions do not attach app-server (`thread/resume`), read screens for question / approval cards, or inspect composer before sending;
  like botmux Claude Code, those actions stay in Lark / on computer. Codex suspended beyond 7 days and absent from botmux appears as read-only Codex history; reply in Lark.
- **Same session on both sides**: if also running in an engine pane (previously resumed from phone), the live pane wins and botmux's suspended record yields.
- **Detection**: find `botmux` when node connects and learns whether to list it (next item), and on engine switches: PATH first, then installer path `~/.botmux/bin/botmux` (launchd node has only system PATH). Installing later requires restarting that computer's personad.
- **Disable**: "Also show botmux sessions" (#2575; the web has it in the machine page header as **botmux · On / Off** beside the engine pill, one click flips it, #2842; iOS below Engine in machine details; Android next to Engine in Engine and connection, saved on tap, #2899), default on. Changes immediately send `node.configure {"botmux": …}` (the hub adds `claude_dirs` so the node keeps its Claude config directories), stored in hub settings.
  `"botmux": false` in the machine's `machines[]` hub.json entry is only its unset-setting default (`docs/operations/config.md`; old nodes ignore). Stops `botmux observe` / `botmux dashboard current` (in-flight results discarded); Agents has no `botmux:` panes.
  Hub specifies the setting in `node.configure` on connection; node does not attach botmux or push lists before that (preventing sessions briefly disappearing / returning). Old hubs omit it; attach as before when asked for sessions.
  After disabling, tmux botmux sessions still appear as Not running history. Replying on phone launches engine `claude --resume`, potentially colliding with botmux's instance; reply to these in Lark.
- **To-dos**: attach by session ID as usual; project-wide grouping uses botmux session cwd.
- Live test: `go test -tags botmuxlive -run BotmuxLive ./internal/node/ -v` (read-only listing plus one dry-run delivery: validation / rendering, no dispatch).

## Named herdr sessions (#2196)

herdr can run more than one server per account: besides the default one, named sessions (`herdr --session <name>`, listed by
`herdr session list`), each with its own socket under `<herdr config dir>/sessions/<name>/` and its own ids, so `w4:p1` can be a pane of two
of them at once. A node on the default herdr server drives every **running** named session as part of the herdr engine
(`node/hostherdr_sessions.go`):

- **Ids**: a named session's pane, tab and workspace ids are `@<session>:<id>` (`@build:w4:p1`, `herdr.QualifyID`); the default server's stay
  bare, so stored rows, hooks and clients read as before. No host kind starts with `@`, so the hub and every client take them for herdr panes, and they
  hold no `/` a path could split on. Sessions and their live rows are listed, read, sent to, interrupted, closed, renamed, split, streamed (observe,
  control, the full herdr view) and given resume commands on the session the id names; ids herdr hands back (a split, a new tab, a worktree) are
  qualified again.
- **Showing the session**: a named session's spaces are labelled `<session> · <workspace label>`. Renaming one sets herdr's label without that
  prefix (typed or not), and `space.rename` answers the name the space shows now, which the hub keys pins, mutes and to-do links by.
- **Left alone**: stopped sessions, the `persona-<hash>` sessions of other nodes' data directories (`herdr/session.go`), names that cannot stand in an
  id, and every other session on a node that runs on a session of its own or on a socket `HERDR_SOCKET_PATH` moved. Windows drives the default server only.
- **Pane identity from processes**: hooks, MCP calls, the pi extension (`herdrPaneOf`) and the process scans read `HERDR_SESSION` (or the session
  directory in `HERDR_SOCKET_PATH`) with `HERDR_PANE_ID` (`herdr.EnvSession`, `herdr.PaneOf`), so a hook in a named session's `w4:p1` never binds
  the default server's `w4:p1`. Hooks, MCP calls and the pi extension write panes as the default server sees them (`nodeclient.HookPane`), and the
  node translates them where they come in (`node ownPane`: the node socket, the standby spool, runtime hints); the node's own process scans write
  them as its herdr host does (`node envPane`). The two differ only on a node whose own server is a named session: its own session's panes are bare
  there, and the default server's are `@default:<id>`, which it does not drive.
- **Never in the default server's way**: `herdr session list` runs in the background, again once the list is 5 s old; an id of a session the list
  does not have makes the node ask again (at most once a second) and wait up to 6 s for the answer. Each listing asks the named sessions alongside
  the default server and waits at most 2 s from its start for them (and, the first time, for the list); what a session has not answered by then
  comes from its last answer, with one call to it out at a time. The default server failing is the herdr host failing, as before: while it is down
  the herdr host lists nothing, the named sessions included, and keeps its last list. Each server has its own event stream; the named ones are
  looked at again every 5 s (a session started since gets one, a stopped one's ends), and one that ends is opened again on its own.
- **Pane-less work stays on the default server**: new workspaces, and new sessions started without naming a workspace (a named session's labels
  never match a start's directory name). A start, a new tab or a worktree in a named session's workspace opens there.
- **Focus** following (`herdr.client.focus`) is the default server's: it is the computer's own herdr window. Focusing a named session's pane works
  but brings no echo back. A full herdr view is attached to one server (the session of the pane it opened on): asked to show a pane of another
  session it ends, and opening it again attaches to that session.
- **Known gap**: the iPhone and Android apps rename a space optimistically under the name typed; for a named session's space the name shown
  (`<session> · <name>`) differs, so they show the typed name until the app reloads. The web and the hub use the name `space.rename` answers.

## When herdr is not running

After reboot, launchd / systemd starts node, but no terminal has started classic `herdr` server (#1879). Node starts it on demand:

- **When**: node startup (after hub specifies settings in `node.configure`; old hubs omit them, so assume enabled after 10 seconds), and before phone create / continue Not running.
  Only with herdr or herdr + Herdr App, only classic herdr; herdr-core has its own lifecycle and is untouched. Not from polling,
  only those two occasions.
- **How**: `herdr server` (v0.9.5 headless foreground server), in a separate session (`setsid`) so node restarts do not kill it.
  Linux systemd-user nodes instead use `systemd-run --user --unit=persona-herdr-server` (stopping the node unit kills its whole cgroup).
  Wait for connectable socket (at most 20 seconds) before create / continue.
- **Clean environment**: herdr server passes its environment to every pane and claude. Node uses `env -i` with only `HOME`, `USER`, `LOGNAME`, `TMPDIR`, language,
  `XDG_*`, `HERDR_SOCKET_PATH`, `HERDR_SESSION`, `HERDR_CONFIG_PATH` (the server reads the config file the node reads for the worktrees directory), `PERSONA_HOME` (panes find the node socket in that directory), then user's login shell (`$SHELL -l -c 'exec herdr server'`; if launchd has no `SHELL`, read account login shell).
  PATH comes from that shell, which starts from the system directories with the tool directories after them: hub.json `machines[].extra_path` and the mise / asdf shims
  directories that exist (#10127, `docs/operations/config.md` ("hub runtime config (hub.json)"); a profile that sets PATH outright drops them). No node / launcher `ANTHROPIC_*` or proxy variables. Find herdr on node PATH first, then ask login shell.
- **What a start types** (#744, `node/agentenv.go`): a pane has its engine's environment, not the node's. A herdr server started some other way, or Orca's app, may lack the node's `PERSONA_HOME`, and an engine started from the assistant's Bash tool hands the assistant's own variables to every pane.
  So every command the node types into a pane to start or resume an Agent (Claude, Codex, compatible CLIs, pi; herdr's `resume_argv` too) does two things:
  - It puts `PERSONA_HOME=<the node's data directory>` in front, where the pane's shell takes `VAR=val cmd`.
  - In the sh family, it first runs `unset` on two sets of variables. One set is what the node gives only the assistant's processes: `PERSONA_ASSISTANT`, `PERSONA_SESSION_ID`, `PERSONA_SPACE`, … in both spellings, plus `CLAUDE_CODE_AUTO_COMPACT_WINDOW` and `ENABLE_TOOL_SEARCH`. The other is what a running Claude Code sets for the programs it runs: `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_EFFORT`, ….

  fish gets the directory only. A shell that takes no assignments (csh, nu, pwsh), and Windows, get the command as before. An Agent that should have `CLAUDE_CODE_AUTO_COMPACT_WINDOW` or `ENABLE_TOOL_SEARCH` gets it from its launcher or from Claude Code's settings, not from the pane.
  The generated `mcp.json` (the server's `env`) and the hook commands (`PERSONA_HOME=… personad hook`) name the directory as well, so they work even when the start command could not carry it.
- **What a start tab's shell starts with** (#10365, `node/startenv.go`): what a start types comes after the shell has read its startup files, too late for the questions those files ask (oh-my-zsh's "Would you like to update? [Y/n]" reads a key while `~/.zshrc` loads). So the tab a start opens (`newTab`, pi's resume tab) is created with `DISABLE_AUTO_UPDATE=true`, `DISABLE_UPDATE_PROMPT=true` (always together: the second alone means a silent update, a `git pull` in the tab), `ZSH_DISABLE_COMPFIX=true` (oh-my-zsh's insecure-directories check) and `POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true` in its shell's environment, next to the Agent mark `PERSONA_AGENT=1` (`startShellEnv`, one create through `host.EnvLauncher`), where the engine takes one:
  - herdr: `tab.create` / `workspace.create` `env` (an object of strings; the CLI's `--env KEY=VALUE`; herdr 0.9.5). A herdr that refuses the field (one from before environment support) gets the create again without it (`createWithEnv` in `server/internal/herdr/herdr.go`), and the tab opens without the variables. Never in `herdrServerEnv`: the user's own tabs keep updating.
  - the Herdr App: merged into `terminal.run`'s `env` with the app's terminal identity (which wins on a clash).
  - Orca and Tern have no environment for a new terminal yet: their tabs rely on the question being answered (`docs/protocol/protocol.md` "Answering a start's question").

  A terminal the user opens, a split and a preview's server tab never get them. The start line unsets them again in the sh family (`startupOnlyEnv`, next to the assistant's variables) and keeps the Agent mark, so the Agent and the shell the tab keeps have the user's environment plus `PERSONA_AGENT`; fish keeps them (no `unset`), which no Agent reads. A `zstyle ':omz:update' mode …` in `~/.zshrc` wins over the variables: that question, and the ones no variable turns off (a bare `compinit` outside oh-my-zsh on insecure directories, ssh's unknown host key, zsh-newuser-install, a passphrase), still stop the start with `shell_waiting`, answered from the App for a New Agent start and a planned task alike (`docs/protocol/protocol.md` "Answering a start's question").
- **When a stdio node ends** (#10114): a node serving one hub connection over stdio (`personad node --stdio`, a hub's `kind: local` machine) stops, as it exits, a herdr server it started itself in that process when the server has no panes, so a throwaway hub leaves no server behind. A server with panes, one started by anyone else, and the resident node's (`--attach`) are left running.
- **Only one**: in-node mutex plus `flock` on herdr configuration directory (another node for the same account); recheck socket after locking.
- **One server per data directory** (#6482): a herdr server's panes reach the node whose `PERSONA_HOME` the server was started with, so two nodes with different data directories on one account must not share a server.
  A node whose data directory is not `~/.persona` uses a herdr session named after it (`persona-<8 hex of the directory's hash>`): socket `<herdr config dir>/sessions/<name>/herdr.sock`, its own saved workspaces and log, the same `config.toml`.
  The node reaches it through that socket and passes `HERDR_SESSION` to the herdr CLI it runs and to the server it starts, so the panes carry it too. The node with the default data directory keeps the default server.
  An explicit `HERDR_SOCKET_PATH` or `HERDR_SESSION` in the node's environment wins (a node started inside a herdr pane has them, so it uses that pane's server; it logs when that is not the data directory's own session); under systemd the server of a session is a unit of its own (`persona-herdr-server-<session>`); Windows keeps the default server. A default server already running with another data directory's `PERSONA_HOME` (started by a node before this) is not detected: restart herdr.
- **Switch**: per machine, "Automatically start herdr when needed" below **Settings › Machines › Engine**. hub.json
  `machines[].herdr_autostart` is only the machine's unset-choice default (omitted means enabled). User choices live in hub settings
  (`herdr_autostart/<machine-id>`), sent through `node.configure` on connect / changes (a change also carries `claude_dirs`, so the node keeps its directories). Once disabled, manual `herdr server stop` stays stopped,
  and node does not restart it. `GET /api/v1/machines/{mid}/herdr` (`{autostart, running, known}`),
  `PUT` (`{"autostart": bool}`); row exists only for herdr / herdr-both. Client UI pending (#1915).
- **Manual start**: `node.info` includes `herdr_running` / `herdr_running_known` (old nodes omit both, distinguishing stopped from
  unknown). `POST /api/v1/machines/{mid}/herdr/start` forwards to node `herdr.server.start`, **independent of autostart**:
  that switch controls node-initiated starts; a user's Start tap explicitly requests it. Failure reasons below (`engine_missing`).
- **Current status**: `node.info` is requested only once on node connection. After reboot, node waits for `node.configure` (after `node.info`) to start herdr,
  so relying on `node.info` leaves stale notices until node reconnects; same after manual Start / computer-side `herdr server stop` (#1916).
  `GET …/herdr` therefore asks `herdr.server.status` live (one socket connection, `{running, known}`), reused for 3 seconds;
  successful `herdr/start` also records result. Old nodes without the method retain `node.info` values and avoid reasking within 3 seconds.
  Android Agents has the notice / Start button (#1916), shown when online and known stopped; tap explicitly starts.
  iOS / Mac too (`HerdrNotice.swift`): poll every 60 seconds while Agents is foregrounded and not covered by chat, and at once when a watched machine comes online or changes engine (#4685). One notice per machine atop list, respecting machine filters / hub groups.
  One read failure does not remove existing notices; endpoint 404 stops polling that machine. Start shows "Starting…"; failures show localized known reasons inline (immediate exit / killed, missing, no response, old node, offline, timeout, `HerdrFailure.says`; unknown reasons remain raw). Keep button; raw hub text, status, request ID under long-press / context-menu Copy details, not body.
  After success, use POST result for that machine during this run until hub also reports running (GET may still use connection-time value). Web same (`web/src/lib/herdr-notice.ts`, `views/agents/HerdrNotices.tsx`), below filters in sidebar / mobile Agents section, polling every 60 seconds while visible and at once when a watched machine comes online or changes engine or the tab is shown (#5060),
  same rules / localized failures, with raw hub text, status, request ID (`X-Request-Id`) in Copy details button / context menu.
- **While it is not running**: the node keeps asking herdr for its sessions about twice a second. With no socket it falls back to the herdr CLI, which can reach a server the socket cannot (another session, an older herdr). Once the CLI says no server runs (`server_not_running`), calls without a socket return that same answer without running herdr for 1 s, then 2 s, 4 s, … up to 30 s; the socket answering, or the CLI finding a server, ends the wait at once (`herdr.Client.call`). A computer with herdr installed and its server stopped used to run herdr twice a second (about 140 runs a minute, 240 ms of CPU a minute between the node and those runs; about 20 ms now). A started server is found on the next poll through its socket, so starting it from the phone or a terminal shows its sessions as fast as before. `herdr session list` (the named sessions, every 5 s) needs no server and succeeds, so it has no such answer; instead a list that found nothing running is kept while herdr itself, the entries of `<herdr config dir>/sessions/` and the absence of every listed socket stay the same, and given again without running herdr (`herdr/sessionlist.go`). A server starting creates its socket or its session's directory, so the next list runs herdr again; a list with any session running is never kept.
- **Failure**: still `engine_missing`. Missing herdr copy unchanged; installed-but-failed says "Automatic start also failed." Details include exit code / final output lines,
  output in `persona/herdr/persona-herdr-server.log` under the user's cache folder (`~/.cache` on Linux, `~/Library/Caches` on macOS; a folder of the node's own in the temp folder when there is none), also node logs. No autostart on Windows; behavior unchanged.

### When herdr is not installed

New computers often lack herdr (#2847); previously Start only said "herdr not installed," requiring someone to log in and install it. Node now installs it (`node/herdrinstall.go`):

- **What**: official herdr release binary for OS / CPU (linux / macos × x86_64 / aarch64), **version pinned** to match other computers
  (currently 0.8.0, not `latest.json`). Verify hardcoded sha256 before installing to `~/.local/bin/herdr` (no sudo, on node PATH).
  Write temporary file and rename only after verification, avoiding partial binaries on interruption. Windows / other architectures have no package and require manual install. Version upgrades update the version and all four sha256s.
- **When**: always install on user Start. Node-initiated starts (startup, before create / continue) install only if `machines[].herdr_autoinstall: true` in hub.json;
  omitted preserves "Not installed." One install at a time; concurrent requests share it.
- **Slow downloads**: background download up to 15 minutes; Start waits at most 15 seconds. If ready, start and return `running`; otherwise return "Installing herdr 0.8.0… It will start automatically when ready"
  (client shows original text after "Not started:"). Node starts server when done; next Agents poll sees running and removes notice.
  `herdr.server.status` / `GET /api/v1/machines/{mid}/herdr` add `installing`, `install_error` (last failure); client can show Installing, not wired yet.
- **Install failure**: GitHub unreachable, timeout, HTTP error, checksum mismatch all give a localized reason. Network failures say "A proxy may be needed: set node HTTPS_PROXY and restart it"
  (downloads use Go `ProxyFromEnvironment`). Also log reason on node.

### Resuming Persona-started sessions after restart

herdr restores workspaces, tabs, panes after server restart, then **after a client attaches** resumes Agents using built-in commands (`session.resume_agents_on_restore`, default on),
for Claude Code plain `claude --resume <id>`. Persona-created Claude uses launchers (`personad launch <name>`, ~/.zshrc functions),
`--settings`, `--mcp-config`, Persona hooks; plain resume loses all of these, switching gateway to user's subscription and dropping MCP / hooks.

Persona therefore reports its own recovery command for each Claude pane (`resume_argv`, only this pane):
`eval "<Persona's original launch command, --resume <id>>"`. herdr types it into the pane shell during restore, retaining ~/.zshrc launchers.

- User-started terminal claude is not reported; herdr restores normally; user `config.toml` untouched.
- Older Persona sessions / sessions predating node restart (launcher in node session records): report once on refresh if running in herdr.
- Claude changes sessions in a pane (`/clear`): report new ID when Persona SessionStart hook arrives; herdr does not track it automatically (v0.9.5 tested).
- Resume command omits permission / thinking settings; paths with single quotes are not reported (herdr rejects `'` arguments), falling back to built-in restore.
- Node-started headless server has no attached client, so herdr does not resume Agents until someone opens herdr on the computer. Resume on a Not running session's terminal page still works.
- herdr before 0.9.2 answers the report `ok` and drops `resume_argv`. The node asks the server's version first (`ping`, never the CLI's: a binary upgraded in place says the new one while the old server runs) and on such a server reports nothing, logs once per server that its Agents will resume bare until herdr is upgraded, and gives the command to the next server (#11792).
- An Agent resumed without Persona's command anyway (an old herdr, a resume typed by hand) is listed with `off_launcher`: the scan of running claudes sees its claude without Persona's `--mcp-config`. A launcher switch to the launcher it was started on brings it back there (`docs/protocol/protocol.md`, "Switching a running Agent to another provider").
- **Interrupted by restart** (#1917): sessions running before restart appear `interrupted` afterward (`docs/protocol/agent-states.md`), read-only like any stopped Agent, with Resume on the terminal page. Node records the pane and
  launch time (session records in hub settings); if launch predates current node startup and no Agent is alive, it is interrupted. Sessions launched after this node startup do not count,
  nor user-started terminal sessions (no recorded pane). No automatic continuation: restart may interrupt a tool call, so the user decides. Resume uses ordinary
  `session.start {resume}`, launcher `--resume` in the shell the Agent left when it is still there, else in a new pane.
- No "Continue all" yet (machine-wide batch start, client half).

## When an engine cannot be selected

- **"Not detected"**: engine is not running on that computer. Start `herdr` once in a terminal; open Herdr App / Orca. After a short wait,
  or Reconnect for that machine, detection refreshes.
- **"herdr + Herdr App" not detected**: both runtimes are not running (herdr started in terminal, Herdr App open with remote connected to this machine).
  If only one is used, select it directly.
- **Orca says "This Orca version is incompatible with Persona"**: Orca / personad versions mismatch; update both.
- **Paseo not detected**: open Paseo desktop app or run `paseo daemon start`. For a custom home, set personad `PASEO_HOME`.
  If available but a connection failure appears below, see `docs/integrations/paseo.md` ("Troubleshooting").
- **Engine selected, no sessions**: verify Claude actually runs in that engine; ordinary-terminal / other-app sessions appear only Not running.
- **Old node on hub machine** (version mismatch in machine info): old node does not understand switching and uses only herdr; update personad on that computer.

### Botmux display preference

There are two independent switches. A computer's **Also show botmux sessions**
(`GET/PUT /api/v1/machines/{mid}/botmux`) controls which sessions its node lists to
Persona. **Show botmux sessions in Agents**, in Web Settings › Agents,
is the paired hub's shared display preference (`agents.show_botmux`, kept by the
default manager). It defaults to on. Turning it off excludes botmux rows and
counts from both complete and paged Web Agents lists; the Lark group quick filter
is hidden when empty. It does not change node inventory, to-do attribution or
dispatch. Turn it back on from the same settings row.

A browser's old `persona.agentHideBotmux=1` is migrated only while the shared
preference is unset. The conditional write cannot replace an explicit choice,
even from another device at the same time. The browser removes the legacy key
after a confirmed stored value; failures retain it for retry. Reconnect and the
next Agents list refresh read the shared value again. Android's use of this
contract is tracked separately in #10242.
