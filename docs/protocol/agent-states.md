# Agent states and the space model

## Unified states

The App, the watch and push notifications all use this one set of states (`proto.Session.state`, computed by the manager connected to the computers and passed to the App unchanged by the hub, `server/internal/assistant/fleet.go`). The source is the host's state (currently herdr), plus two things Persona knows itself:
whether there's a pending interaction, and whether the user has looked at it in the App.

| State | Meaning | From herdr | Persona adds | Color |
| --- | --- | --- | --- | --- |
| `needs_you` Needs you | Stuck somewhere a person must step in | `blocked` (a question / confirm box in the TUI) | A pending question, plan, permission or terminal interaction (herdr still shows working while a hook holds it) | Blue, breathing |
| `working` Working | Running | `working` | Claude: herdr reports `done` / `idle` / `unknown`, but the session has hooks (Persona's, or the global report-only ones) and the hook turn count says the turn isn't over (see "Hook turn signals" below); for sessions without hooks, the terminal title shows Claude Code's working spinner glyph (◐◓◑◒; older versions used braille dots), or the transcript shows this turn has started and not ended (no `turn_duration`, not interrupted) and was written within 45 seconds (#2069; if the transcript shows the turn ended, herdr's state is used). "Written" is its newest timestamped line, not the file's mtime: Claude Code rewrites its trailing metadata in place while idle (#5967); or the turn has stopped but its background commands / sub-Agents / Monitor / workflow are still running (#2228, #2935: when they finish they hand it the result and it carries on; assistant chats excluded). Codex: herdr reports `done` / `idle` / `unknown`, but its app-server runs a turn in its thread or its title shows Codex's spinner ("Codex" below, #4865) | Amber, breathing |
| Waiting in the background (a kind of `working`) | The turn is over and all the agent still runs only waits: background commands that sleep, loop on a check (`until … sleep`), `gh … --watch`, `tail -f` (#6045, `Session.background_waiting`) | — | The node looks at the processes under the session's `claude`: every background command is a shell Claude Code starts there (it sources its shell snapshot). One `ps` at most every 20 s, and only while an agent with background commands has ended its turn; CPU is what they used between two looks (Linux `ps` counts whole seconds). Waiting: only waiters, shells and filters in their pipes, near-zero CPU, a loop's short probes (younger than 10 s) left out. Working: a compiler, build tool, test runner or download runs, or CPU is used, or a process nothing here knows runs while their output grew in the last 2 minutes. No look to compare with yet, or no shell found: Working, as before. Subagents and workflows are work. `state` stays `working` (older apps show Working); new clients leave it out of the Working count | Amber, faded, still |
| `unread` Unread | A turn finished and the result hasn't been seen | `done` (never focused in herdr) | And not viewed in the App since it finished | Green |
| `idle` Seen | Finished and seen, waiting for input | `idle` (seen in herdr) | Or herdr still says `done` but it was viewed in the App | Gray |
| Just finished (a kind of `idle`) | The triage found the turn done before anybody looked, less than three hours ago ("Just finished" below, #9751, `work.just_finished`) | — | Session Triage on, and the turn Unread before the triage took it out | Green, a plain dot |
| `offline` Not running | Not in the host, only a transcript. An Agent of a terminal engine is read-only (#4754): a message is refused (`agent_stopped`), and Resume on its terminal page brings it back (`--resume`) | — | — | Light gray |
| `interrupted` Interrupted by restart | Was running before a restart, and the restart took herdr's panes with it (#1917) | — | The record has a pane, the start time is earlier than this node's start, and there's no live agent now | Light gray, with "Continue" |
| `unknown` Unknown | The host recognized the agent but can't tell its state | `unknown` | — | Gray |

A dispatched session with an unknown host status reads as `state unknown` in `dispatch_read`. Its last reply may be partial. A session marked `offline` on a connected computer is interrupted. When the computer itself is offline, the manager reports `computer offline` and leaves the transcript unread.

List order (each client's rank: web `lib/format.ts`, iOS / Mac `AgentListOrder.rank`, Android `agentRank`): Needs you → Unread → Awaiting reply → Working → Just finished → Seen → Unknown → Not running.

Sort priority of a folded row or tab (which agent it speaks for and which pane it opens on; web `lib/herdr-tree.ts`, Android `tabUrgency`): Needs you → Working → Unread → Seen → Unknown → Not running, with Awaiting reply and then Just finished right after Unread on Android.
iOS / Mac tab rows (`TabPanes.swift`) go by `AgentState.priority` instead: Needs you → Unread → Working → Seen → Unknown → Not running.

### Awaiting your reply (#5096)

"Seen" only says the user looked; it does not say whether the agent still waits on them. A turn that stops on a question
("Push it to beta?", "A or B?") that the user glanced at and left was Seen like a finished one, folded into Idle Spaces, and
forgotten. So a stopped session carries `awaits_reply` (`state` is unchanged; older apps still show Seen):

- **What it asks** (`asks`): the node takes the question to the user from the turn's whole last reply (`last_text` is only its
  first 200 runes, and the question is usually at the end) with `closeout.AskOf` (the words through `transcript.AskOf`), rules only, no model: one of the last three
  sentences ending in a question mark, or carrying a phrase that hands a decision over (要不要 / 要我 / 是否需要 / 请确认 / 选哪个 /
  告诉我 / 跟我说一声 / 选定后…, "Should I / Do you want / Let me know…", ますか / でしょうか…). Code blocks don't count, nor a
  phrase when the close says there is nothing to decide (没有需要你拍板的问题; a real question mark before it still counts), nor a reply a newer message came after (the user already
  answered; a host's answer such as botmux's counts the same way when the host gives its time). Marks inside inline code and links end no sentence (`user?.name`,
  `…?w=1` ask nothing; the question keeps their words), an ASCII `?` only ends a sentence before a space or the line's end, and a Chinese sentence ending in
  吗 asks even without its question mark (not 么 / 呢: "…为什么。" is a statement). A reply whose closeout
  (`../assistant/agent-work.md` "Closeout") says `status="done"` with no `missing` and no `waiting` line asks nothing,
  wherever its question stands (#4110, `closeout.WaitsOnNobody`): it is the Agent's own word that the turn waits on
  nobody, so a closing question is conversation or an offer, as triage reads the same closeout. `not_done`, `blocked`,
  `done` with something missing or waited for, and no closeout at all leave the words to decide, as before. The node marks the session `asks_read`; only without it (an older node, a
  host that does not say) the manager reads `last_text` itself, and only when it was not cut.
- **Still waiting** (`awaits_reply`, the manager's): Unread or Seen, no pending interaction (that is Needs You), no model
  error, no background work or `/loop`, and either the triage says it waits on the user (not put off on its card:
  `work.deferred`), or the triage has no word on the turn (off, not yet, unsure) and the turn has `asks`. A turn the triage
  found done does not count (a closing "shall I also…" is an offer).
- **Lists**: a Seen, not muted agent with `awaits_reply` gets its own rank, Awaiting Reply, after Unread and before Working;
  it and its space stay out of Idle Spaces and the "N more idle" folds, and its row shows what it asks (the triage's line when
  it has one).
- **Reminder** (the manager, `assistant/agents_reply.go`): once a minute it looks; when one has waited 30 minutes since the user
  looked (seen on the computer only: since it asked), one push "<agent> is still waiting for your answer" with the question,
  through the Needs You push switch, answerable from the notification. Once per turn; not for muted agents, inbox agents,
  questions asked in Feishu, quiet scheduled tasks, or questions older than a day. The record is in memory: the manager's first
  look at each machine's list after a restart (whenever its node is back) takes what is already due as reminded (no burst
  after a deploy); at worst one that was about to be reminded is not. A question is its turn end and its words: the same words
  with a moved `updated_at` (a line written after the turn) are not reminded of again, unless the agent worked in between (any turn start drops the mark).
  Turned off and on again, the reminders prime each machine again; until the seen times are read back after a restart
  nothing is reminded of. Agents in the health space are never reminded of (their questions are not for the lock screen). The reminder is laid out as a finished
  turn's push but is no `agent.finished` to extensions; on the WebSocket it is a `reply` notify (Android files it with the
  finished ones and puts it away when the session is seen). A reminder the hub refuses (one that is behind) is tried again
  the next minute. It reads the lists as last told to the hub, not decorating them again. The switch is in Settings › Session Triage (`reply_reminder`, `reply_reminder_minutes` in `agents.triage`):
  it spends no tokens, so it is on by default and does not depend on the triage switch; an older app changing another switch
  leaves it as it was.
- The finished push of a turn that asks says the question, not the reply's first lines.
- **The assistant knows**: its live snapshot has a section for agents asking the user that the user saw and has not answered,
  and `agents_list`'s `active` lists them with an "asking you, unanswered" line.

### Just finished (#9751)

With Session Triage on, a turn the triage finds done stops being Unread ("Work state" below): the green dot stays for
what needs a look. But it went straight to Seen, and the Agents list sorted and folded it with sessions seen long ago
(Idle Spaces, "N more idle", the folded Seen section), so the work that had just finished was hard to find. A stopped
session therefore carries `work.just_finished` (`state` is unchanged; older apps still show Seen):

- **Set** by the manager while it makes the lists (`decorateWork`, `server/internal/assistant/fleet_work.go`): the triage
  found the turn done (`work.phase` `done`), the session was Unread before that took it out (nobody had seen it: not
  opened or marked seen in an app, not focused in herdr, not viewed in Paseo's own App), no `/loop` wakes it again (that
  is Waiting), and the turn ended less than `agents.triage.just_finished_hours` ago (manager.json, 3 by default;
  `work.turn_at`, else `updated_at`). Only on the lists; the triage's record never holds it. With Session Triage off no
  turn has a work state, so a finished one is Unread as before.
- **Gone** when the session is seen (opening its chat, Mark as Seen, Mark all as seen, a look in the host), when a new
  turn starts, and when the window ends. Nothing else tells a list again when only time passes, so the manager keeps
  when each machine's first Just finished row runs out and tells that list again once it has (`republishFinished`,
  looked at every 30 seconds).
- **Lists**: a Just finished, not muted agent gets its own rank, after Working and Waiting in the background, before
  Waiting, Muted and Seen: work still going comes first, finished work before the parked waits and the seen ones. It
  and its space stay out of Idle Spaces and the "N more idle" folds. A muted one is plain Seen, as a muted Awaiting
  reply is. Its row draws the unread tint as a plain dot (not the glowing Unread one), names its state "Just finished"
  (`lib-state-labels.just_finished`, the To-dos' "Just finished" section's word), and keeps the triage's "✓
  conclusion" as its second line. Opening it, or Mark all as seen, takes it out on that device at once and waits in the
  seen outbox for the hub ("Seen" below).
- **Why three hours**, not the To-dos' 24 (`docs/assistant/todo-status-sop.md`): the Agents list sees far more turns than
  the To-dos page; on a busy hub a day kept dozens of rows, the pile the section was made to end. Three hours covers a
  meeting or a stretch of focused work; the setting changes it.
- **Not counted**: no push, badge, Needs you count, Live Activity or widget count changes, and the assistant's snapshot
  still lists these turns under "Done (within 24 hours)".
- **Clients** (#9751): the web, Android (#9786), iOS and Mac (#9787) and the shared Rust sort core use this rank.
  Android and Apple keep an open chat's row in its original place until the chat is left.

### Codex (#4865)

herdr's word for a Codex can be wrong for a whole turn: a reporter holding the pane decides it instead of the screen
(#4657). herdr said `idle` through a turn of a Codex joined on its app-server, so the Agents list, `agents_list` and its
to-do said Seen while its chat page showed the turn's live words. When the host says `done` / `idle` / `unknown`, node lists a Codex
as Working while the app-server it is connected to runs a turn in its thread (`turn/started`, the reply to node's own
`turn/start`, or the status its read every 5 seconds gives), or while its terminal title shows Codex's spinner (one of
`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` as a word of its own, what herdr's `osc_title_working` rule reads; a title with "Action Required" never
counts). `blocked` and `working` stay the host's, botmux's sessions keep botmux's state, and a Working whose rollout says
the turn ended more than 30 seconds ago is still listed as done (`codexStatus` and `codexTurnOver` in `server/internal/node/`).
So a message sent from the phone or the web is listed Working as soon as `turn/start` answers.

### Non-Claude herdr agents (opencode, pi, unrecognized agents)

When the agent herdr reports isn't Claude / Codex / compatible Codex forks, the state is **always what herdr reports** (`blocked` / `working` / `done` / `idle` / `unknown`); there are no hooks, transcripts or on-screen boxes to add to it.
Deciding "Needs you" / "done" and pushing are the hub's existing logic (based on session state changes, regardless of which agent). Per-agent additions:

- **opencode** (`docs/integrations/opencode.md`): herdr's opencode plugin reports `working` (turn start, tool execution, after a permission answer), `blocked` (`permission.asked`, `question.asked`, errors) and `idle` (turn end).
  With Persona's plugin installed, the plugin also reports `blocked` while it's waiting on a permission card (or a question that can only be answered in the terminal); node combines both: herdr's word stands, and the plugin saying `blocked` counts as "Needs you".
- **pi** (`docs/integrations/pi.md`): same; the extension having a card waiting on the phone counts as "Needs you".
- **Unrecognized agents** (`gemini`, `amp`…): used to be dropped entirely; now listed as a terminal-only row (`docs/integrations/opencode.md` ("Terminal rows for other agents")). The state is herdr's, so when it's stuck in a confirm box the phone shows "Needs you",
  but there's no card to answer (the hub doesn't parse screens of TUIs it doesn't know); answer it on the terminal page.

### Claude in a Tern or Rex block

Tern and Rex report no agent state: every agent they list is `unknown` (`node/hosttern.go`, `node/hostrex.go`). For a Claude, node takes the status Claude Code keeps in its own
`sessions/<pid>.json` instead (the file of the block's `claude` process, the one that names its session; `ternClaudeStatus` in `server/internal/node/agent_status.go`),
before the hook and transcript rules below:

- `busy` → `working`.
- `waiting` (a question, permission or other dialog of its own has the keyboard) → `blocked`: Needs you, and the manager makes its screen card as for herdr's `blocked`.
- `idle` → `done` once the session has a transcript (`idle` before its first message). Tern and Rex have nothing like herdr's "seen in the terminal", so a finished
  turn is Unread until it is opened in an app, and its finished push follows the usual rules.
- `shell` (Claude Code 2.1.31x and later: the turn is over while a background command or Monitor still runs) is read like `idle`: `done` once
  there is a transcript, and the row then shows Working (background) from the transcript's background tasks.
- No status to go by (no pid found for the block yet, an older Claude Code, a file that already names another session after `/clear`): the rules below,
  as for herdr's `unknown`; a resting Claude's screen is read for a dialog as in a herdr pane.

Why (#6312): Claude Code 2.1.321 marks itself `waiting` the moment its question shows, but Persona listed such a Claude as Working for 45 seconds (its transcript's
latest write), then as Unknown, which the overview does not show: it never needed the user and got no card. A finished turn stayed Unknown too.

### Claude in the desktop app

A Claude Code session of Claude's desktop app takes its status from its own `sessions/<pid>.json`, as above (`busy` → `working`, `waiting` → `blocked`). `idle` and `shell` are `done` while the session's last turn ended unseen. It counts as seen once the app brought it into view, or once the node found someone at the app with the session shown. Otherwise it is `idle`. The phone shows Unread for `done`, and the finished push follows the usual rules (#6477, `../integrations/claude-integration.md` ("Unread when a turn ends unseen")).

### Hook turn signals

herdr's state is often wrong (`done` while still working, #2069), and the title glyph and the transcript's latest write are only inference. Claude sessions Persona starts carry Persona's hooks; sessions the user starts themselves, once `claude_global_hooks` is on, get global report-only hooks (`../integrations/claude-integration.md` ("Global report-only hooks"))
that report the same `UserPromptSubmit` / `Stop`. Both feed the same counter: they say directly when a turn starts and ends, and node decides from that (`judgeHookTurn` in `server/internal/node/hookturn.go`, called by `claudeStatus`).
Borrowing moshi-hook's approach, node keeps per session in memory:

| Field | Source | Purpose |
| --- | --- | --- |
| `prompts` | +1 on each `UserPromptSubmit` | How many turns started |
| `completed` | Set to the current `prompts` on `Stop` | How many turns finished |
| Process pid + start time | The hook process walks up to claude (the ancestor whose `~/.claude/sessions/<pid>.json` `sessionId` matches); on Linux, field 22 of `/proc/<pid>/stat`, stored as `linux:<starttime>`; macOS can't get the start time, so only the pid is stored | Tell a live claude from a reused pid |
| Reply signature | First 8 bytes of SHA-256 of `Stop`'s `last_assistant_message` with whitespace removed | Hub dedups "done" pushes |

The two sources differ in only two ways; everything else is the same. Sessions Persona started (including the assistant) count only the plugin hook's copy; the global copy is dropped **before** counting, otherwise one submit counts twice.
Global events carry `report_only`, and the hub doesn't take them as meaning the session has Persona's hooks (otherwise screen cards would wait in vain for a hook card that never comes).

Decisions (when herdr says `done` / `idle` / `unknown`; `blocked` and `working` follow herdr):

- **A dialog is open in the terminal** (#2553, `node/dialogstatus.go`): for a Claude still `done` / `idle` / `unknown` after the checks above (excluding botmux, those in Claude's agents list, and ones where Persona itself is operating a picker)
  in a herdr, Tern or Rex pane, node reads its screen in the background every 5 seconds (the list doesn't wait for the read). If the bottom is a dialog (`screen.Choosing`) it's recorded as `blocked`, `attention_reason` is the dialog's first line, and the "last said" in the App list and the assistant's `agents_list` shows that line.
  Why: Claude Code 2.1.309's auto mode billing notice blocked 3 Agents for several minutes; herdr reported done and Persona showed Unread.
- **An allow Claude did not take** (#3752, `node/allowwatch.go`): whatever herdr says, `working` included. Once node hands a PermissionRequest
  hook an allow (a terminal Agent's, not the assistant's), it reads the pane 1.5 s later; a dialog still at the bottom (`screen.DialogHead`) lists
  the session as `blocked` with the dialog's first line as `attention_reason`, so the manager makes its screen card. The pane is read again every
  3 s while the dialog stays, until it is gone (or after 3 minutes); keys pressed into the pane drop what was read at once.
  Why: Claude Code 2.1.322 runs the PermissionRequest hooks for its own safety checks ("Dangerous rm operation …", asked in bypass mode too)
  but ignores an allow for them. The phone's Always Allow closed the card, herdr kept saying working, and the dialog held the terminal with
  nothing on the phone to answer it: every message was refused as `dialog_open`.

- **Has hooks and `prompts > completed`**: Working, overriding herdr's `done`.
- **Has hooks, `prompts == completed`, and a `Stop` was seen**: not working; a leftover spinner glyph in the terminal title doesn't count. Exception: if the transcript has a turn written after the `Stop` (with a 5-second grace for the closing lines) that hasn't ended,
  it's a new turn the hooks didn't report (a background task or `/loop` woke Claude, or a Stop hook made it keep going), and that turn is judged by the no-hook rules.
- **Nothing counted** (node just restarted, only `SessionStart` received): the hooks have nothing to say; judge as a session without hooks. The counter is in memory and becomes valid again from the first `UserPromptSubmit` / `Stop` after a restart. That errs on the safe side,
  so it isn't stored in Postgres and needs no migration.
- **The agent process is gone or its start time doesn't match** (pid reused by another program): drop the session's hook state, so "an unfinished turn" left by a dead process no longer counts as Working; use the no-hook rules instead.
  It isn't judged Not running outright, because the same session may already have been continued by hand with `claude --resume` in the same pane (without hooks then); if the pane really has no agent, herdr doesn't list it anyway.

**Interrupting (Esc) doesn't fire `Stop`**, so `prompts > completed` never balances. The turn ends in this order:

1. The end of the transcript has an end marker written **after** this prompt (`[Request interrupted by user]` or `turn_duration`): ended.
2. The title has no spinner glyph, the transcript has been quiet for 8 seconds since the prompt or the last write, and the title shows Claude's waiting mark ✳: ended (✳ is Claude's own "waiting for input" mark, strong enough evidence).
3. Same but the title isn't recognizable (old version, user-changed title): ends only after 90 seconds of quiet (`2 × claudeFresh`), because long tool calls can go a long time without writing the transcript.
4. The title has a spinner glyph: counts as Working indefinitely, so long silent tool calls aren't misjudged.

A `UserPromptSubmit` without a `Stop` (whether local commands do this hasn't been tested) takes the same path.

**Dedup of done pushes**: the hub sends the push on working → done, and state flapping (working → done → working → done) could push twice for one turn. node puts the `Stop` reply signature into `hook.event` (`stop_sig`),
and before sending a "done" push the hub compares: same session, same signature, no new `UserPromptSubmit` in between, then no second push (`Machine.stopDuplicate`).
Global report-only `Stop`s carry the signature too and are deduped the same way. Without a signature (no hooks, `Stop` without a reply) nothing is blocked; a new prompt clears the record, so two turns that happen to reply the same ("OK") both push. After a hub restart the record is lost; worst case one extra push.

**The same pid-reuse defense is used for "open elsewhere"** (`openelsewhere.go`): the `sessions/<pid>.json` Claude Code writes carries `procStart` (on Linux it's field 22 of `/proc/<pid>/stat`; verified to match).
When a leftover old file's pid is taken by another program, it used to be treated as "this session is open elsewhere" and resuming from the phone was refused; now a start time mismatch means that claude has exited.

Not verified / known limits: whether Claude Code's background-task wakeups and `/loop` timers fire `UserPromptSubmit` hasn't been tested (both cases are judged correctly, see the exception above);
on macOS the pid-reuse defense is only "is the process still alive"; on Windows the ancestor process can't be found, so no pid is recorded; the other signals work as usual.
The `sessions/<pid>.json` Claude Code writes also has `status` (busy / idle) and `statusUpdatedAt`, which could become another signal later (not used this time).

**Work state** (2026-11-04, #1686, [agent-work.md](../assistant/agent-work.md)): everything above is the host's view; "Unread" only says the turn stopped. After a turn stops, the hub triages it
and adds a work state `work` to the session: working / blocked on a named precondition (waiting for your decision, another Agent, an external event, a transient error) / done, with a line saying "what it's waiting for" or the conclusion.
State computation takes it into account: **done**, or **blocked on something the hub waits on for it** (the hub retries automatically, wakes it at the set time), is no longer `unread` but `idle` (a done one nobody had seen is Just finished for three hours, "Just finished" above);
for a decision you need to make, the hub opens a question card, so it's `needs_you`. Anything not triaged (just stopped, model failure, can't tell) stays `unread`.
There is only one set of colors: `apple/Shared/StatusPalette.swift`, shared by the App, the watch and Live Activities (`docs/apple/ios.md` ("Design language")).
The Agents list also has "Waiting": the state is Seen, but a `/loop` hasn't fired yet (`Session.isWaiting`), so a new message will come. Sessions whose background commands / sub-Agents are still running are counted as Working directly by the hub (#2228, `backgroundState`) and no longer go into Waiting; the assistant chat's background tasks and Monitor are always running and don't count.
A long-running process started in the background (such as a dev server) keeps the Agent Working until it exits (node stops listing background tasks older than 24 hours). Stop on such a session, once its turn is over, interrupts nothing: the node presses the Escape as always and answers `turn_over_background`, so the app says the turn had ended and the background work still runs instead of "Interrupted" (#10828); ending the background work itself is done in the terminal. Turn-end decisions such as the "done" push and hub self-update look at the host state and aren't affected by this.

### Background workflows and Monitor (#2935)

Claude Code's Workflow tool runs a multi-agent script in the background; the turn ends right away and is woken again by the notification that it finished (`<task-notification>`). Monitor works the same way, waking it when an event arrives.
They count as background work just like background commands: the Agent is Working, the "done" push is held back, and to-do receipts, completion judgment and auto-close (#1696) all wait for them to end. Once a notification wakes the main session, the end of that turn is judged as usual.

- **Where it's read from**: the Workflow tool result in the transcript, "Workflow launched in background. Task ID: … Run ID: wf_…" (`turn_duration`'s `pendingWorkflowCount` also says some are running),
  and Monitor's "Monitor started (task …, expires in 30m …)". The end is the notification for the same task id (with `<status>`) or `TaskStop`. node's background scan reads the whole transcript, so nothing is missed by only reading the tail.
  The result alone starts a run: that scan skips lines over 64 KiB, and a call with its script inline is as long as the script (#6003).
- **A Monitor event is not its end** (#10356). A notification parsed as `kind: monitor`, `status: event` keeps the
  running task's ID, description and `expires_at`, including after repeated events. A stream-ended notification or
  `TaskStop` still removes it; node's existing expiry filter still excludes an expired Monitor. Commands and Agent
  notifications keep their own lifecycle rules. This uses the existing background inventory and wire fields.
- **Progress is on disk**: `<session>/subagents/workflows/<run>/journal.jsonl` (one `started` line per agent, with label and phase; `result` means it came back), and in the same directory `agent-<id>.jsonl` / `.meta.json` (each agent's conversation, read as a sub-Agent).
  When done it writes `<session>/workflows/<run>.json` (status, error, duration); the `meta` of the script `<session>/workflows/scripts/<name>-<run>.js` has the name and phases.
- **Session field** `workflows` (`docs/protocol/protocol.md`): name, status, current phase, finished / started counts, duration, the sub-agent list and each one's last line. While running it's also put into `background` (kind `workflow`), so old clients and old hubs can still tell "there's background work".
- **Timely updates** (#3433, #5511): each workflow's task id, status, current phase and agent counts (done / failed / running / started), and each listed agent's id and latest tool call (its name and its call count) are part of the node's session list change detection (the id since #2323: a retried agent can replace its earlier attempt without changing any count). Appending a journal `result` or an agent calling a tool sends `sessions.changed` even when the main transcript hasn't grown. A running workflow's progress is cached for at most 1.5 seconds; polling every second leaves scheduling room within the three-second update deadline. Tokens and last words ride along with those pushes.
- **Reading cost** (#5511): node reads the journal only for the lines it gained, and an ended run once (the transcript said it ended and there is no state file: once more when the state file appears; a stalled run keeps being read). An agent's transcript is read whole once and then only for what it gained: its tool calls (a sub-Agent's `steps`, #5602; a workflow agent's `calls`) need every line, but only the reply lines that carry a tool call are parsed, and only for their blocks' types and ids; the latest call, words and exchange are reread from the lines they're on, and the usage and model (a workflow agent's; a sub-Agent's too since #5030) from the last replies it gained back. Caches drop the least recently used entry instead of starting over when full. On the dev machine's 580 real workflow agent transcripts (600 MB), the first list after a node restart went from about 9 s to under 1.1 s.
- **Phases and usage** (#3433): `phase_stats` reports `title / started / done / failed / running` for each phase, in declared `phases` order (including zero counts for phases that haven't started), followed by any extra phases found in the journal or agent metadata. Agents with no phase are grouped under `title: ""`. Counts and run-level `tokens / calls` are calculated before `agents` is limited to the latest 60, so clients must not infer totals from that array.
  The journal's `failed` lines (an API error ended the agent) count as `failed` (`agents_failed`, the agent's `status: failed`), not as `done` (#5511; older nodes counted them as done, and leave the fields out). Agents still out when the run ended are `stopped` and in none of the counts but `started`.
  A retried agent counts once (#2323): Claude Code's workflow view can retry a stalled agent, which starts the same `agent()` call again under a new agent id (the journal line's `key` names the call; two identical calls in one script get different keys). The attempt it replaced never gets a `result` or `failed` line, only `[Request interrupted by user]` at the end of its transcript, so node drops it from the counts, `phase_stats` and `agents` and keeps only its `tokens` / `calls`. Without this the phase stayed current with one agent too many (`2/3`) until the run ended. A retry of an agent that had already returned replaces it the same way. Journals without `key` (older Claude Code) are read as before.
  Each agent also carries `model`, `latest_tool: {id, name, summary}`, `ended_at`, `tokens` and `calls`. `tokens` is the latest assistant usage's `input_tokens + output_tokens + cache_read_input_tokens + cache_creation_input_tokens`, not the sum of earlier usage records; Claude Code's own error replies (`<synthetic>`, all-zero usage), usage without input (a gateway's on a turn's other replies, `{"input_tokens":0,"output_tokens":1}`) and a gateway's whole-turn sum don't replace it (#5511, the same rule as a session's context tokens). `calls` counts tool calls. Run-level `tokens` sums those latest snapshots and is not cumulative token consumption either. `latest_tool.summary` comes in the request's language (node writes Chinese, the hub rewrites it). Clients keep their existing display when these fields are absent.
- **End notifications and timing** (#3433): `Dynamic workflow "…" completed / failed / was stopped` is parsed as `Item.task.kind: workflow`, preserving the notification's `tool_use_id` and `task_id` so clients can find the initiating call or run. Its one-line text is the hub's, in the reader's language like the other task notes ("Background workflow finished · …", #5511); iOS / Mac's `ChatCache` went to `chats-v10` with it, so copies cached with the Chinese text are dropped. A Live Activity item with `background_working` uses the earliest known `started_at` among running workflows for `since`; without one, it falls back to the turn's start time, then the session's update time. `detail` remains a one-line progress summary.
- **Summary rows** (#5888, #5950): a `summary=1` row (for example iOS Agents › Recent) has no `workflows`. Clients take its `workflow_progress` as the running run (name, phase with `phase_count`, counts, time) and `running_workflows > 0` alone as background work, so the row's workflow line and "Working (Background)" read the same in every grouping (iOS `Session.runningWorkflows` / `isBackgroundWorking`, web `runningWorkflows`, Android `Session.runningWorkflows` / `AgentSession.isBackgroundWorking`). They key on the summary marker (`waiting` present), not on `workflows` being absent: a client keeps an older complete row's `workflows` under a summary (iOS `preservingDetails`, Android `keepingDetails`, the web store's merge), and the summary's own counts win over that kept array, as in "Summary rows" in `docs/protocol/protocol.md` (#3588). A chat whose row a summary last updated reads its running run the same way; its header cards (`workflows`) wait for the complete row. A summary names only the newest run, so its line has no "and N more".
- **Fallback**: with no notification and no status file, 30 minutes without any write counts as `stalled` (its claude is gone) and no longer as running; a Monitor past `expires_at` doesn't count; finished ones stop being listed after a day.
- The assistant's live snapshot and `agents_list` spell out running workflows ("this turn stopped to wait for results, it isn't done"), so it won't nudge or close these Agents.
List order (the same for all three groupings): Needs you → Unread → Awaiting reply → Working → Waiting in the background → Just finished → Waiting → Muted → Seen; within a state, by last update time (`docs/apple/ios.md`).

### "Seen"

- Seen in herdr = the tab was focused on the computer (reads via the CLI don't count). Persona **does not** focus the tab to mark it seen (that would steal focus on the computer), so after you look on the phone,
  the green dot in herdr's sidebar on the computer is still there.
- Seen in the App: opening the chat page, watching it finish on the chat page, swiping left in the list to **Mark as Seen**, **Mark All as Seen**, opening the session on the watch. The App sends WS `{"type":"seen"}`
  (when the connection is stuck, or the hub hasn't recorded it 5 seconds after the WS send, it sends `POST /api/v1/sessions/{mid}/{sid}/seen` instead), and the hub stores it in Postgres `session_views`.
  WS messages have no acknowledgment and are silently dropped on a half-open connection (#1588), so the App keeps reads the hub hasn't recorded yet in `SeenOutbox`: it counts as recorded only when a list from the hub has this session's
  `seen_at` later than the mark. Meanwhile it still shows as Seen and is resent on reconnect; it stops waiting if the session does another turn, starts working or needs you, disappears from the list,
  or still isn't recorded after 10 minutes.
- Rule: herdr `done` and `seen_at` earlier than the session's last update time (the transcript's modification time, i.e. when the turn ended) → `unread`, otherwise `idle`.
  So if it does another turn after you looked, it becomes Unread again.

### Ordering

- The list is grouped by state, newest first within a group. Normally that's `updated_at`, but a working agent's `updated_at` changes with every step and would jump around within the group,
  so "Working" uses `state_since`: the hub records when each agent entered its current state (in memory; after a hub restart it counts from the restart).

## Spaces / slots (multi-tab collaboration)

"A group of related agents" in a host is abstracted as a **space**, and each position in the group is a **slot**:

| Field | herdr mapping | Other tools later |
| --- | --- | --- |
| `space_kind` | `herdr` | Tool name |
| `space_id` / `space_name` / `space_index` | workspace id / label / number | Project, room, session group |
| `slot_id` / `slot_name` / `slot_index` | tab id / label / number | Role, window, channel |
| `pane_index` (optional, #2320) | The pane's positional index within the tab (from 0, top-left is 0) | Which position within the slot |
| `parent_space_id` (optional, #2073) | Not set | Which space this space sits under (same machine, same `space_kind`): an Orca child workspace sets the parent workspace's `worktreeId` (`worktree.ps`'s `parentWorktreeId`) |

DSH (`docs/integrations/dsh.md`): `space_kind` is `dsh`, the space is the session's directory (`space_id` = directory, `space_name` = directory name), no slots;
state comes from the plugin: running → working, a permission / question waiting → blocked, otherwise done (the hub splits it into Unread / Seen by whether it's been seen, as usual).

Codex App (`docs/integrations/codex.md` ("Sessions in the Codex desktop App (App-local IPC)")): `space_kind` is `codex_app`, the space is a project of the App's sidebar
(`space_id` = `codex-project:<repository root>`, `space_name` = the App's name for it) or its chats in no project (`codex-chats:<CODEX_HOME>`);
`space_index` is the project's place in the App's sidebar, from 1 (its `project-order`), absent for a project the App does not order and for chats in no project.

Claude desktop app (`docs/integrations/claude-integration.md` ("The desktop app's sidebar"), #8521): `space_kind` is `claude_desktop`, the space is the repository the session works in
(a worktree the app made is its repository's; until that is read, the folder the app started the session in), the same `space_id` as the Codex App's chats there (`codex-project:<repository root>`, `space_name` its folder name); a folder that is no repository is a space of its own, the home folder none. No slots.

Paseo (`docs/integrations/paseo.md` ("Which sessions appear")): `space_kind` is `paseo`, the space is a Paseo project (`space_id` = project record id, `space_name` = project name);
a repository's main checkout and its worktrees share a space. No slots (#4493): a workspace's Agents are separate conversations, not the panes of one tab;
`slot_name` alone carries the Paseo workspace's title, as the Agent's place label.

opencode (`docs/integrations/opencode.md`): those in herdr panes are herdr sessions (`space_kind` is `herdr`); those outside herdr (web, other terminals) are runtime sessions,
`space_kind` is `opencode`, the space is the session's directory, no slots, and state comes from the plugin (running → working, waiting on a permission → blocked, otherwise done / idle depending on whether there's an assistant message).

node fills these fields from herdr's `session.snapshot` (workspaces, tabs and agents in one call): once whenever herdr pushes a change via `events.subscribe`
(the list is rebuilt at most once a second; entering / leaving blocked doesn't wait, see `docs/server/server.md` ("Card fast lane (#751)")),
and every 10 seconds as a fallback (every second when the event stream is down or there's a compatible Codex forks pane, see `docs/server/server.md` ("Connecting to herdr")). When a tab name is the default number, the App shows `#2`.

If herdr cuts a large snapshot at its socket's 8192-byte limit (`herdr.ErrTruncated`, #6083), the client rebuilds it from
`workspace.list`, then `tab.list` and `pane.list` per workspace, one `pane.layout` per tab, and `agent.get` per detected agent.
This preserves workspace and tab names, shell panes, split layouts and pane indices, agent names and metadata, and the globally focused workspace / tab / pane.
The workspace catalog and terminal focus reader use the same fallback. No configuration is needed. The cut snapshot is logged once until a whole snapshot succeeds;
successful smaller calls do not reset that streak. These reads are not atomic. If a required small call fails (including another oversized answer),
the client returns an error and the node keeps its existing agent-list fallback without replacing its cached layout with a partial tree.
The workspace catalog retains its separate `workspace.list` fallback.

### Several panes in one tab

A common pattern: an Agent runs `herdr pane split` in its own tab, then starts Claude / Codex in the new pane to split up the same job. Agents in the same tab (`slot_id`) form **a group**; the Agent doesn't need to write anything extra:

- herdr doesn't record who started a pane, only positions: in the snapshot's `layouts`, each tab has each pane's `rect` and the split tree `splits`. node orders by the split tree
  (`server/internal/herdr/layout.go`) and fills `pane_index` (from 0) for each Agent session:
  each item in `splits` is a tree node `{id, direction, ratio, rect}` without its children; `direction` is `right` (split left/right) or `down` (split top/bottom), and `rect` is the area it splits.
  node starts from the whole tab, finds the node whose `rect` is exactly that area, and splits it in two along its direction (the left / top half first; if several lines in the same direction could split it, it takes the one where each half is exactly one pane or one node), then orders each half the same way.
  So a 2×2 made by splitting top/bottom first and then each half left/right (A, B on top, C, D below) orders as A, B, C, D; looking only at rects, both lines could split it and the order would be ambiguous.
  With no `splits` (old herdr) or no node for an area, it falls back to rects only: find a straight line that crosses no pane, trying left/right first, then top/bottom.
  When zoomed, herdr still gives each pane its original rect; when rects overlap and can't be split, herdr's own listing order is used. If the host gives no layout (herdr-core, Orca, etc.) or the pane isn't in the layout, there's no `pane_index`.
  node also pushes `sessions.changed` when only `pane_index` changes (`herdr pane swap`, closing the top-left shell): it's part of the list's change detection (present or not, and its value).
- Within a group, order by `pane_index`. Number 0 (top-left; every split is cut from it) is the **starter**, the rest are branches it started; without `pane_index`, fall back to the old order (session id).
  If the top-left is a shell rather than an Agent, the group has no number 0 and the Agents are peers (the web doesn't mark a starter).
- Parent/child relationships across tabs (an Agent opening a subtask in another tab / workspace) are not inferred; if needed later, agree on an explicit marker (like to-dos' `persona_from`).

For the web, see `docs/web/web.md`; Android, `docs/android/android.md` ("Session controls, session origin and capabilities") (#2515). iOS / Mac (#2516; the pure logic is in `apple/Persona/Views/Chats/TabPanes.swift`, same rules as the web's `herdr-tree.ts`):

- In the Agents list (by repository / by project; cards, capsules, compact list), a tab with several Agents first gets a tab row: the status dot of the one most in need of attention, the tab name and its title, "N Agents"; tap it to collapse / expand.
  Below it, ordered by pane position: the starter on top, labeled **Started It**, the rest indented one level beneath it. Each row names its own Agent (when both are Claude they're numbered "Claude 2"; the starter is told apart by its label) and no longer repeats the tab name.
  Expanded by default; the collapsed state is remembered per `machine/space name/tab number` (`tab/…` in `dev.folded`, still recognized after renames). The whole group counts as one item: if any member is busy, it doesn't go into "N more idle".
  Expanding an idle space / repository adds no tab rows; members of the same tab are kept together and the starter is still labeled. The number on a space counts Agents ("N Agents"). Grouping by state / by machine / by to-do doesn't group by tab.
- The peer capsules and ⋯ popover at the top of the chat page, the Mac peer menu, and the space page: members of the same tab are kept together and ordered by position, named "#1 · Codex"; the starter is marked **Starter** (the popover and menu write **Started It**, branches are indented; the Mac menu has one section per tab).

How it looks in the App:

- **The dev list is grouped by space**: each active space (with Needs you / Working / Unread) gets a card whose header shows the space name, machine and per-state counts,
  listing each agent in tab order (tab name, title, state, latest line). Spaces that are all Seen are folded into **Idle Spaces**, one row each, expandable;
  sessions that aren't running are under "Recent (not running)". On Android (since 2026-11-02) it's a computer → space → session tree, with no **Idle Spaces** group or heading:
  idle spaces stay under their computer, after the active spaces, collapsed by default and styled more subtly, showing how many sessions they contain when collapsed (`docs/android/android-design.md` section 2.4).
- **The peer menu in the chat page's top bar**: when a space has several agents, the leftmost button on the top bar's second line lists them (tab name + state; the button has a red dot when another agent needs you); tap one to switch straight to it without going back to the list.
  The subtitle is "machine · space › tab · state".
- **Space page**: opened from a space card's header; a grid shows each tab's state, title, latest progress and pending count. You can **Open a new tab in this space**,
  which creates a tab in the same herdr workspace (`workspace_id` of `session.start`).
