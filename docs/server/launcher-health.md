# Launcher health checks and automatic repair (#2873)

Launchers on your computers break silently: the gateway rejects a bare model name (`400 Bare Claude model name cannot be routed`, and after switching models `no account match`),
the gateway key expires, the CLI is too old, or starting an Agent keeps timing out (`context deadline exceeded`). Nobody used to notice, and a dispatched Agent failed on its first message.
Now a hub manager (`server/internal/assistant/launcher_health*.go`) periodically checks every launcher on every computer, routes work around broken ones, repairs known errors itself, and records what it cannot repair as blocked.
**Record only, never push**: no notifications, nothing on the Dynamic Island; the App and the board read it themselves.
**Off by default** (decided 2026-11-05): periodic checks and automatic repair only run when the manager's `manager.json` sets `agents.launcher_health.enabled: true` (`docs/operations/config.md` ("manager.json")); when it is off, failed Agent starts are still recorded and still routed around.

## Checks

- It reuses "Test availability" (`docs/integrations/launchers.md`): the node's `launchers.check` runs `claude -p … --max-turns 1` / `codex exec` once in an empty directory, with a 90-second timeout, spending a few tokens.
- When: it looks every 5 minutes (`healthTick`); on computers that are online (`Reachable`), any launcher last checked longer ago than the interval (default 30 minutes) is rerun.
  When a computer goes from offline to online, all its launchers are checked right away. Launchers on one computer run one after another (no burst of claude processes); computers run in parallel.
- What is checked: launchers this computer can start (`machineLaunchers`: those from ~/.zshrc minus hidden ones, plus Persona-managed ones; `kind: api` launchers do not run on a computer and are not checked).
  Runtime launchers (DSH and others, whose `source` is not `zshrc` / `persona`) are not checked.
- A failed Agent start is recorded too: when `agent_start`, a task plan starting an Agent (`startTaskAgent`) or a repair Agent's start gets an error from `session.start`, it is recorded as a failure (source `start`),
  and a check runs right away to confirm (one network blip should not leave a launcher unavailable; if the check passes, it recovers).
  The App's own starts count as well (#1642, `launcher_health_starts.go`): the hub tells the manager how a start it waited for went (`session.start_ended`: a failure, or a start the node ran before answering that worked),
  and a "New Agent" answered at once (`async`) is followed in the machine's session list, from `starting` to started or to `start_error` (counted once while the node lists it: the starts counted are kept with the records, so one is not counted again after the manager restarts; the failure's `updated_at` is the node's clock and is not compared with the record's).
  A start whose outcome the hub does not know (the computer offline, the call timed out) and a request the node refused before running anything are not recorded.
- **On-demand recheck before a reroute** (#2018): a start aimed at a machine whose launcher has a failing record probes that machine once on the spot before moving elsewhere.
  If the probe passes (the launcher was fixed after the record, for example a CLI installed or a PATH corrected), the record is cleared to `ok` at once and the start stays on the asked machine.
  If it fails again, or the machine cannot answer, the usual reroute applies. Concurrent starts for the same machine × launcher wait for one probe (single flight), and a probe is not repeated
  within 30 seconds (`healthRecheckThrottle`), so a burst of starts does not turn into a burst of probes. This also covers the case where periodic checks are off: no manual button is needed.
  Two records are never rechecked this way: `usage_limit` (the request would be refused until the reset anyway) and `not_in_terminal` (the probe's login shell runs the launcher, so it cannot
  reproduce the terminal's failure; only a real successful start clears it).
- An Agent's own usage limit is recorded too (#1710, source `session`): when a turn ends on the subscription's usage limit (`docs/assistant/agent-work.md` "Usage limits") with a reset still ahead,
  and the session names its launcher, that launcher on that computer is marked `usage_limit` until the reset. Nothing runs for it: no check (it would spend a request to be refused) and no repair.
  The main chat's conversations count as well (they run the computer's launcher under the same sign-in). Each turn a background task wakes meets the same limit again; it is recorded once.
- Stored in the settings key `launchers.health` (Postgres), one entry per computer × launcher: success / failure, error text (redacted, truncated), error category,
  check time, consecutive failure count, repair log, state. It survives hub restarts.

## Error categories (`classifyLauncherError`)

| Category | Matches | Automatic repair |
| --- | --- | --- |
| `usage_limit` | Checked first: Claude Code's own line (`You've hit your session limit · resets 4:20pm (Area/City)`, also after a prefix such as `Error:`; an older `Claude AI usage limit reached`), Codex's (`You've hit your usage limit`, `usage_limit_exceeded`, `usageLimitExceeded`, `usage limit reached`). A gateway's generic 429, `rate limit` or `quota` stays in its own class | None, however many in a row: nothing on the computer is broken. Unavailable until the reset (below) |
| `model_route` | `Bare Claude model name`, `cannot be routed`, `no account match`, model does not exist / not supported | Repair Agent (reconfigure the gateway's model) |
| `auth` | 401 / 403, `invalid … api key`, `authentication_error`, `Not logged in`, expired token | Repair Agent (reconfigure the gateway's key) |
| `cli_outdated` | `unknown option`, asks to upgrade / version too old / no longer supported | First the node upgrades itself (`claude update`); if that fails, a repair Agent |
| `timeout` | `context deadline exceeded`, no answer within 90 seconds | Counts only after two in a row; handed to a repair Agent |
| `other` | Everything else | Same: repaired only after two in a row |
| `not_in_terminal` | A start's new terminal tab could not run the launcher or its claude (`launcher_missing` / `claude_missing` with `where: "terminal"`, #1642, `docs/integrations/launchers.md` "The terminal's shell is another environment") | None: `blocked` at once, the reason says to define the launcher where the terminal's shell reads it or add it as a model provider |

## States

`ok` → on failure `failing` → while repairing `repairing` → back to `ok` when the retest after repair passes, `blocked` if the retest still fails. At any point, one passing check returns it to `ok` and clears the block.
Except `not_in_terminal`: the check runs in the node's login shell, where the launcher works, so neither the check right after the failed start nor the periodic rounds run for it, and a check asked for by hand does not clear it.
It clears when a start with that launcher on that computer works (any of the starts above: the user retrying from the App counts), or once it is older than 6 hours like any failure. It does not count against the main chat's readiness (`docs/assistant/onboarding.md`), which runs in the login shell.
`failing` / `repairing` / `blocked` all count as **unavailable** (when the latest result is within 6 hours).

A **usage limit** (`usage_limit`) is `failing`, never `repairing` or `blocked`: the request got through to the subscription, so a block left by an earlier error is lifted and no repair round takes it.
Its reset (`resets_at`) is read off the words when they name a time zone (`resets 4:20pm (Area/City)`), or comes from the Agent's own record of the limit (source `session`); Codex's "try again at 3:05 PM" names none.
With a reset, the launcher is unavailable exactly until then (not bounded by the 6 hours; a reset more than 8 days off is taken as misread, as if there were none), and available again from that moment.
The periodic rounds spend no check on it before the reset (it would be refused too), nor does a forced check of all launchers; the first round from a minute after the reset checks it whatever the interval, and a passing check clears it.
A check asked for by hand of that launcher ("Test", `POST …/check?machine=&launcher=`) still runs.
Without a reset it follows the usual rules: unavailable for 6 hours, checked at the interval. Until the reset it counts in the main chat's readiness as a rate limit that passes by itself (`rate_limited`), and not at all after it.

- `agents_list`: gets an extra "unavailable launchers" section at the end, one line each: computer / launcher / state / category / error; a usage limit adds `resets_at=<RFC 3339>, in …`.
- The launcher list for creating an Agent (`GET /api/v1/machines/{mid}/launchers`) carries `health` on each item (`{state, error, kind, checked_at, resets_at?}`), and the clients gray out the blocked ones ("In the App" below).
- `agent_start` and explicitly flexible `task_plan` tasks: when the launcher to use is unavailable on this computer, it is rechecked on the spot first (see "On-demand recheck before a reroute"); a passing probe keeps the start on the asked computer, otherwise it switches to another online computer where this launcher is healthy (a directory under the original computer's home becomes the new computer's home),
  and the "reason" in the answer / card says it switched and why (for a usage limit with a reset: `is at its subscription usage limit on <machine> (resets_at=…, in …)`); if no healthy one is found, start as originally planned and warn in the answer that the latest check failed. A task plan switches once when the card is produced and checks again when it actually starts.
  Healthy first: last check passed > never checked; never checked is not treated as broken.
- `task_plan` defaults to `machine_policy: "fixed"`, also for saved plans without the field. Preview, start and retry retain that machine and its directory. A known unusable launcher fails on that target without starting elsewhere; normal health recovery permits a later retry there. A terminal-shell `not_in_terminal` refusal can only be cleared by a real terminal start, so an explicit task retry rechecks that shell on the same computer; it still runs the normal native authentication and startup gates. Set `machine_policy: "flexible"` only for portable work that may move. A card edit that changes the machine pins the new target unless it explicitly chooses flexible placement.
- The tool answers `Queued` while asynchronous dispatch is pending. This is not proof of an Agent start: errors appear on the same task and its failure receipt. A missing or foreign session in a start response is rejected; a `starting` session is not recorded as a successful launcher check.

## Automatic repair

- One repair round per computer at a time (several launchers on one computer usually share one cause: the gateway config is broken and every launcher returns 400); at most two repairs per round, then it is recorded as `blocked` and waits for a person.
  Blocked launchers are still retested at the normal interval and recover on their own when they pass.
- **Upgrade the CLI**: the node's `launchers.repair {name, action: "upgrade_cli"}` does only a fixed thing: for Claude launchers it runs `claude update` (login shell, 5-minute timeout);
  for anything else it answers "not supported". It does not accept arbitrary commands. Old nodes lack this method, which counts as "cannot upgrade".
- **Repair Agent**: when `agents.launcher_health.repair` in `manager.json` sets `skill`, it starts an Agent with a healthy launcher on `machine` (default: the computer the assistant runs on) in `cwd` (default home).
  The first message (`server/internal/prompts/agents/launcher_repair.md`) says which launchers on which computer are broken, the raw error and category, which skill to use for the repair, and to self-test with `claude -p` afterwards.
  It hangs off a **quiet to-do** (`Quiet`, no source chat): the hub sends no pushes for it, but it is visible on the Agents page and in to-dos. When it stops (neither working nor waiting for you) or after 45 minutes,
  the manager retests this round's launchers: pass means `ok`, fail means `blocked` (the reason includes the repair Agent's last message). Without `skill`, upgrading the CLI is the only repair; everything else goes straight to `blocked`.
  The skill name and the computer used for repair are configuration; the code names no specific tool or gateway.

The configuration, in the manager's `manager.json`:

```json
"agents": {
  "launcher_health": {
    "enabled": true,
    "interval_minutes": 30,
    "repair": { "skill": "<skill that repairs launchers>", "machine": "<machine id>", "cwd": "~", "launcher": "", "permission": "" }
  }
}
```

`enabled` missing or `false`: no periodic checks, no automatic repair (state can still be read; failed `agent_start` calls are still recorded, rechecked once, and routed around).
`permission` is the repair Agent's permission mode; missing means `bypassPermissions` (decided 2026-11-05: it has to run ssh to change config, and any other mode stops at an approval card waiting for a person);
set `default` if you want it to wait for your approval at every step. Member hubs in a mesh do not check (computers belong to the primary hub).

## API for the board

- `GET /api/v1/launchers/health?state=blocked&machine=`: all records (or filtered by state or computer), `{records: [{machine, launcher, state, kind, error, checked_at, source, fails, repair?, blocked_reason?, resets_at?}]}`.
- `POST /api/v1/launchers/health/check?machine=&launcher=`: check right now (without parameters, all online computers), returns this run's results.
- A state change broadcasts a `launcher_health` event (`{type, record}`, only to connected clients, not a push).
  The "decisions / blocked / progress" board reads `state=blocked` records, or subscribes to this event, to list what is blocked.

## In the App (#1643)

Decided with the user: gray out with the reason, never hide.

- **New Agent** (iOS / Mac / visionOS `LauncherPicker` in `NewChatView`, web `NewAgent.tsx`): a launcher whose `health.state` is `blocked` on the picked computer stays listed, dimmed, with one line saying why
  (`not_in_terminal`: "Not available in this computer's terminal"; the other classes a line each: key or sign-in rejected, gateway can't use the model, CLI too old, didn't answer, failed its last check).
  Choosing it does not pick it: it opens how to fix it (iOS / Mac a sheet, web a panel under the launchers): why, what to do (define it where the terminal's shell reads it, or add it as a model provider,
  with **Add Provider** going to the provider wizard; other classes: fix its key, gateway or CLI), the record's `error` under Details, and **Try Anyway**, which picks it after all:
  a start with it that works is what clears a `not_in_terminal` record, so the user must be able to start it once it is fixed. `failing` / `repairing` launchers are offered as before (the hub is still on them); an older hub sends no `health` and nothing changes.
- The first pick when nothing is configured to follow (an older hub without model configuration) skips blocked launchers: the one used last on that computer unless blocked, else the first that is not (`LauncherChoices.initial`, web `initialLauncher`).
  When the launcher that will run is blocked anyway (Model Settings' default, or Try Anyway) the form says so; Start stays enabled. iOS / Mac say the line under the launcher row (and under the picker's Default row);
  the web marks the Default chip and puts that launcher's line first, tagged "Used this time".
- The list is read fresh each time the sheet opens (iOS / Mac `AppModel.launchersForStart`, which refreshes the app's launcher cache; the web reads it per dialog): health changes behind any cache, as a start that fails blocks a launcher and one that works clears it.
- **Run Agent here** (a shell pane is a terminal too) and the web's hand-off of a to-do to a new agent: a blocked launcher stays a choice, listed after the usable ones with its line before it is chosen
  (iOS / Mac under a section of the picker's menu titled with the line, which a menu shows where it drops an option's second line, and the line in the footer once picked; web a description under the menu row, or the reason in the option and under the select), and the first pick is a usable one.
  Neither has a separate help, so it stays selectable on both clients for the same reason as Try Anyway.
- **Add Provider** in the help is offered only for a computer in the hub group the provider wizard saves to (the hub-level one); on another group's computer the words still say to add a provider there.
- The assistant's launcher menus (headless `claude -p` in the login shell) are unchanged: `not_in_terminal` does not apply there, and the main chat's readiness covers its model.
- A failed start whose session has `start_error_code` (#1642) shows the hub's whole sentence under "Didn't Start" in the chat (iOS `StartingNote`, web chat page) instead of clipping it after three lines; without a code it is shown as before.

## Not done

- An Agent that starts but fails on its first turn (the gateway rejecting the model name; `session.start` succeeded) is not recorded from the session; periodic checks find it. Only a usage limit with a known reset is recorded from the session.
- Android does not read `health` yet.
- **New Agent** in the App (`agents.start_params`, on a computer the user picked) does not switch computers; Agents started by the Lark bot are not wired in yet.
