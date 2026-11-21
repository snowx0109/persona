# Persona Link: the computer connects to the hub, or the hub connects to the computer

> Naming (#1159): the connection helper is **Persona Link**, and the command is `persona-link`. The installer also puts the old command name beside it as an alias; both names run the same program,
> and its prompts use whichever name you ran. The executable `~/.persona/bin/persona-link`, the download `persona-link-<os>-<arch>.gz`, the systemd unit `persona-link.service`, the launchd identity
> and the helper inside the Mac App carry the new name; what the old names still reach is in [rolling-upgrade-compatibility.md](../protocol/rolling-upgrade-compatibility.md). Copyable commands from the hub and admin page use `persona-link`.

The hub can't reliably SSH into computers such as laptops whose IP and network keep changing. Requested 2026-10-31: no Tailscale or reverse tunnels;
instead, the client on the computer connects to the hub and reconnects on its own after a disconnect, and the hub reuses that link to talk to its node. No SSH or VPN is needed anywhere along the path.

The user decided again on 2026-11-02 ([plan-hub-center.md](../archive/operations/plan-hub-center.md), sections 0 and 4): hubs and computers always connect through a preinstalled service on the computer, not SSH (too much permission);
the computer can connect to the hub or accept a connection from it. The recommendation is one hub for the whole household, with a **node service** on every other computer: resident node plus persona-link, installed by one command or one App.

## On computers: the web app and Persona Link

On a computer, Persona has an Electron desktop app and an independently installable Persona Link ([desktop.md](../desktop/desktop.md)).

- **The desktop app** wraps the hub-hosted web UI and adds native notifications, presence and desktop integration. A computer you only chat from, or use another computer's Agents from, does not need a local node or Link.
- **Persona Link** runs on a computer whose Agents the hub should reach, and does what a browser cannot: it connects the hub to the resident node service and owns the Persona Screen and Persona Input helpers. Its CLI/service works without the desktop app, including on Linux and headless computers.
- The local setup window offers fixed status/pairing/reconnect/node-service operations for a separately installed Link. It does not silently install a service. The Mac launchd agent and Windows scheduled task run independently of the desktop app. The former menu bar and Windows tray implementations are archived in [link-menu-tray.md](../archive/operations/link-menu-tray.md).

A desktop need goes into the shared web UI first, and into Persona Link only when the browser cannot meet it (for example local node connectivity, pairing or a recording helper). The desktop app and Link are not a bundled installation.

### Independent Link service

| | Mac | Linux | Windows |
| --- | --- | --- | --- |
| Install | `scripts/app/mac-link.sh install` builds the independent helpers and registers both services | `persona-link install` or the release install command | `persona-link install --personad <personad.exe>` |
| Connector | launchd `com.persona.link`, running `~/.persona/bin/persona-link` | systemd user `persona-link.service` | Scheduled task `Persona Link` and its supervisor |
| Resident node | launchd `com.persona.node`, a separate service | systemd user `personad-node` | The same scheduled task |
| Management | Fixed desktop setup actions or the same CLI; neither needs a menu bar app | CLI or the desktop setup window | CLI or the desktop setup window; no tray required |

The connector always reads `PERSONA_HOME` in its service environment, otherwise its default data directory. The Electron setup window invokes only the fixed `~/.persona/bin/persona-link` (`.exe` on Windows) and does not pass arbitrary paths or credentials. Closing or uninstalling the desktop app does not stop an independently installed Link service.

The former menu bar and tray are in [the archive](../archive/operations/link-menu-tray.md), including the historical migration procedure. Stop the old parent UI before installing the new service so two connector instances do not fight over the run lock; keep the existing `link.json` and resident-node service. `persona-link install` refuses to take over an unmanaged running connector and explains this.
## Two modes

The same persona-link supports both, selected during pairing and changeable anytime (`persona-link mode active|passive` or the desktop setup window). Each mode stores its pairing separately, so switching back doesn't require pairing again.

| Mode | Who connects to whom | Best for | Requirements |
| --- | --- | --- | --- |
| **Active** (connect to the hub) | persona-link dials the hub's WSS `/api/v1/link` ("Link" below) | Laptops, computers behind NAT or frequently changing networks | The hub has a TLS port (`public.tls_listen`) or sits behind an HTTPS reverse proxy ("[Hub behind a reverse proxy](#hub-behind-a-reverse-proxy)" below); this computer can reach the hub directly or through an SSH tunnel ("[SSH tunnel](#ssh-tunnel)" below) |
| **Passive** (let the hub connect) | The hub dials the TLS port persona-link listens on locally ("Passive mode" below) | Computers with stable addresses reachable by the hub: servers, development workstations, home desktops | The hub can reach this computer's port (directly or through a SOCKS proxy); the computer's firewall opens this port only to the hub |

Choosing: the computer can reach the hub but the hub can't reach it (a laptop, a computer on a company intranet connecting to a home hub) → active; the hub can reach it but it can't reach the hub (the hub is somewhere unreachable from the public internet,
but can enter the computer's network through a proxy), or you don't want the computer initiating outbound connections → passive. If both directions work, either is fine; active needs no open port and is simpler.

## Install the node service

Where to get a pairing link: preferably from **Machines › Add Computer** on the signed-in web app (iOS and Android entry points are in progress). It provides a full copyable command or link for the selected OS;
after the computer connects, confirm **This is my computer** on the same screen ("[Connecting a computer](#connecting-a-computer-mac-example)" below). The admin interface and command-line steps on the hub machine are in that section's "Advanced" subsection.

**Linux** (one command, the "node" role in `scripts/deploy/install.sh`):

```sh
curl -fsSL <site>/install.sh | sh -s -- --node --pair '<pairing link>'   # Active
curl -fsSL <site>/install.sh | sh -s -- --node --listen [:18663] [--addr <address the hub uses to reach this computer>]         # Passive
```

- Downloads persona-node, the device's own program ("[The device's program](#the-devices-program)"), and persona-link (both verified against `SHA256SUMS`) to `~/.persona/bin/`, then hands off to `persona-link install`: installs the resident node's systemd user service
  `personad-node` (running persona-node; one that is there already is left as it is, then switched with `persona-link node switch`) and persona-link's own service, pairs, and waits for connection (passive mode: waits for the listener to start, then prints a link for the hub admin interface).
  A release without persona-node, or a computer that runs a hub or a manager, gets personad instead, as before.
- `--pair` / `--listen` imply `--node`; so do `PERSONA_ROLE=node`, `PERSONA_PAIR`, `PERSONA_LISTEN`. With no options and a terminal, it asks;
  if a hub is already detected on the LAN (mDNS `_persona-hub._tcp`, `persona-link discover`), it first says "One hub is enough for the whole household; install the node service on this computer."
- Without systemd, it still installs the binaries and prints alternative instructions (see "Linux (headless)").
- On Mac, `--node` installs the connector and resident-node launchd agents and pairs; the Electron desktop is not required.
- Publishing: `scripts/release/publish.sh` builds four extra `persona-link-<os>-<arch>.gz` and `persona-node-<os>-<arch>.gz` files alongside personad and adds them to the same `SHA256SUMS`. Persona Link also has Windows amd64 and arm64 downloads named `persona-link-windows-<arch>.exe.gz`; `latest.json` indexes every connector under `links`.

**Mac**: install the independent **Persona Link** helpers (`./scripts/app/mac-link.sh install`; a step-by-step guide is in [persona-link-install.md](persona-link-install.md)). The installer places persona-node in `Contents/Helpers/persona-node` and invokes
`persona-link install --node-bin <bundled persona-node>`, which installs the connector LaunchAgent `com.persona.link` and the resident-node LaunchAgent `com.persona.node`.
The connector is copied to `~/.persona/bin/persona-link`, the fixed path the Electron setup window uses. An existing node agent is left unchanged (for example one from `scripts/deploy/node-launchd.sh install`); one that runs personad moves over with `persona-link node switch`. A deleted node service is not silently reinstalled.
The agent it writes has `ProcessType=Interactive` (the same as `node-launchd.sh`), so launchd doesn't throttle the node like a background job. Since an existing installation is left unchanged, one written before #5944 lacks it: `personad node doctor` reports it and `personad node doctor --fix` adds it (docs/server/server.md "Scheduling and privacy on the Mac").
App updates don't replace the installed resident node. Nodes supporting the update protocol can be remotely updated by the hub over the existing connection, with backup, rollback and online confirmation; first-time local upgrades for old nodes, progress and installation boundaries are in [node-update.md](node-update.md) (#1158).

**Windows** (Orca engine, [windows.md](windows.md) ("Install and start at logon")): download `persona-link-windows-amd64.exe.gz` or `persona-link-windows-arm64.exe.gz` from the GitHub release, verify and unpack it using the PowerShell commands there, then run `persona-link install [--personad <personad.exe>] [--pair '<link>']`, without admin privileges.
Registers the current user's scheduled task **Persona Link**, starting at sign-in and checked every 5 minutes; it runs a windowless supervisor (`persona-link supervise`), which starts
persona-link and the resident node and restarts them on exit. Both are in one task (no separate `node install`). The retired tray is not installed. There is no one-command installer script or installation package; the release provides the connector executable.

### The device's program

A device runs persona-node (`server/cmd/persona-node`), the node and the commands its Agents run (hook, MCP, ask, notify, launch, …) without the hub or the manager, at `~/.persona/bin/persona-node`. A hub or a manager runs personad at `~/.persona/bin/personad`. The two are separate files, so a computer that has both keeps both.

- **New node services run persona-node** where the computer has it: `persona-link node install` (and `persona-link install`, including on Mac) picks `bin/persona-node`, else `bin/personad`. On a computer that runs a hub or a manager (its data directory has `hub.json` or `managers/*/manager.json`) the node stays on personad: their services run personad too, and it updates with the node.
- **Existing node services keep running personad** until they are switched. `persona-link node switch` (run by `install.sh --node`) does it whole or not at all, holding a lock (`~/.persona/node-switch.lock`) the whole time:
  1. It keeps the service file as it is (`~/.persona/node-switch.backup`, written and synced to disk first).
  2. It points the service at `bin/persona-node`. Only the program changes; the file's other settings stay. When the computer has no persona-node yet but persona-link came with one (the independent helper package's `Contents/Helpers/persona-node`), that one is put in place first.
  3. It restarts the service (systemd `daemon-reload` and `restart`; launchd `bootout` and `bootstrap`, the latter tried again for a few seconds). On a Mac it runs only in the desktop's session (`launchctl managername` is `Aqua`, as in Terminal on the Mac); over ssh it is refused before anything changes, and so is undoing an unfinished switch, because launchd there may take the agent away and then refuse to load one again. The node stays on the program it runs.
  4. It waits up to 60 s for the node to answer `ping` on `node.sock` as persona-node, then drops the backup.

  When persona-node does not answer, or the switch is stopped (Ctrl-C, TERM, the terminal closing), the old file comes back, the service restarts on personad, and the command says why. The backup goes only once the service manager has taken the restart (or a node answers all the same): when launchd refuses the old agent's `bootstrap` for longer than the retries, it stays, and the next recovery starts it again. A switch cut off harder (`kill -9`, the computer losing power) leaves the backup: `persona-link node status` says so, and the running `persona-link run` (it looks every 30 s and when it starts, and writes what it did to `link.log`) or the next `persona-link node install`/`switch` puts it back; `persona-link node uninstall` drops it with the service. None of that touches a switch that holds the lock (`node install`, `persona-link install`'s node step and `node uninstall` refuse while one runs), and a backup that is no usable service file (empty, or naming a program that is not there) is dropped rather than put back. `bin/personad` is never touched.
  The switch restarts the node at once: an answer the assistant is writing on that computer at that moment is cut off (Agents in herdr go on).
- **Back to personad**: `persona-link node switch --to personad` does the same the other way. `install.sh --hub` and the manager's install script run it when the node service file names `persona-node` (whoever wrote it), because a computer that runs a hub or a manager keeps its node on personad, which updates with the node. `personad hub setup` and `personad manager install` run directly do not: run the switch yourself there. A manager joined into a directory outside `~/.persona/managers/` is not seen as one.
- The node writes the hook and MCP configs of the sessions it starts with its own path at every connection, so after a switch they run persona-node, and after a rollback personad again.
- `node.info` says which program runs the node and from which file (`program`, `executable`, [protocol.md](../protocol/protocol.md) "Node handshake"); Redeploy sends the device's file persona-node of the build it should run (the hub's own, or the release's, [node-update.md](node-update.md) "Node packages"), else personad to that same file, which persona-node replaces again later. `node-update-check` prints `program` too, which is how scripts tell the two apart. `persona-link node install --node-bin <persona-node>` (and `persona-link install --node-bin`) puts the device's program in beside a personad the computer may have (unless it runs a hub or a manager, where it is refused).
- The office skill the node gives its sessions runs `office` with whichever program is there (personad, else persona-node).
- Windows has no persona-node build yet: its node runs personad.

**On the hub side**: active-mode computers are kind `link` in hub.json; passive-mode ones are kind `service` (the hub side implemented in #435; paste the link in admin **Computers › Add Computer › Hub connects**).

## Claude Code plugin

The Persona plugin for Claude Code (`integrations/claude-code-plugin`) lets the owner's own messages reach a Claude Code session Persona has no terminal for, such as a Code session in Claude's desktop app, the way typed input does. The resident node carries the plugin of its own build and keeps it installed; Persona Link shows it and switches it. Implementation: `server/internal/claudeplugin`, wired into the node in `server/internal/node/claudeplugin.go`.

**Where it comes from.** The plugin's files are packed into personad and persona-node at build time (`go generate ./internal/runtimepkg`, which `scripts/deploy/deploy.sh`, `scripts/release/publish.sh`, the `Dockerfile` and `scripts/app/mac-link.sh` run; `runtimepkg.Assets`), so the installed plugin is always the node's own version. A node built without that step (a plain `go build`) reports `unavailable`. Updating the node, by Redeploy ([node-update.md](node-update.md)) or a new Link install, brings a new plugin, which the node puts in place at its next check.

**How it is installed.** Only with Claude Code's own CLI, never by editing Claude Code's files, and every command names this plugin or its marketplace alone:

```sh
claude plugin marketplace add ~/.persona/claude-code-plugin      # the folder is the plugin's own marketplace, "persona"
claude plugin install persona@persona --scope user                # an update: claude plugin update persona@persona --scope user
```

The node writes the plugin to `~/.persona/claude-code-plugin` (under `PERSONA_HOME` when that is set): a stable folder that Link and node updates keep. A plugin from a folder marketplace is read from that folder, so an update is new files there plus `claude plugin update`. The commands run with `CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL=1`, in the data directory (never a project), and with `--json`; their results, not their wording, are read.

**Which Claude Code.** The newest of the CLI the node finds (`PATH`, `~/.local/bin` and the other places it looks for `claude`) and the desktop app's own copy (`<app data>/Claude*/claude-code/<version>/…`). Without either, or without a config directory, nothing is installed: the state is `claude_missing`.

**Which config directories.** The user's `~/.claude`, which the desktop app's Code sessions use whatever the environment says, and the directory the node's own `CLAUDE_CONFIG_DIR` names when that is another one outside the data directory. Commands for `~/.claude` run with `CLAUDE_CONFIG_DIR` unset, never set to `~/.claude` ([lessons/agents.md](../lessons/agents.md) "Claude Code plugins from a folder marketplace"). Persona's own profiles and the throwaway directories other claudes were seen with are left alone, and a directory that does not exist yet is never created.

**When.** The resident node checks about 20 seconds after it starts, then every hour; a check that failed is tried again after 1, 5 and 15 minutes, then hourly. A check runs `claude --version` and `claude plugin list --json` for each directory (a few seconds) and installs or updates only when something differs. Checks never overlap: the node and the commands below take `~/.persona/claude-plugin.lock`.

**Your choices win.**

- The automatic install is on by default on macOS and Windows, where the desktop app runs, and off on Linux until `persona-link plugin on`. The choice is the computer's, kept in `~/.persona/claude-plugin.json` (`"auto": false` turns it off).
- A plugin turned off in Claude Code (`claude plugin disable`, or **+ › Plugins** in the desktop app) stays off and is not updated; the state is `disabled`. Turned on again there, it is kept up to date again.
- A plugin removed in Claude Code after the node installed it is not put back (`opted_out: uninstalled`). `persona-link plugin install`, `persona-link plugin on` or the desktop toggle undo either choice.

**Running sessions.** Claude Code loads plugins when a session starts. After an install or update, new sessions (terminal and desktop app alike) have it, and sessions already open keep what they loaded until they restart or run `/reload-plugins`. Nothing is restarted for it; `changed: true` in the status says a check installed or updated it.

| Command | What it does |
| --- | --- |
| `persona-link plugin status [--json]` | Checks now and says how it stands in each config directory |
| `persona-link plugin install` | Installs or updates it now, turns it back on where it was turned off in Claude Code, forgets earlier opt-outs and turns the automatic install on |
| `persona-link plugin uninstall` | Removes it and its marketplace from every config directory, deletes `~/.persona/claude-code-plugin` and turns the automatic install off |
| `persona-link plugin on` / `off` | Turns only the automatic install on (and checks now) or off (what is installed stays, without updates) |

`persona-link plugin` runs the resident node's program (`personad claude-plugin <op>` or `persona-node claude-plugin <op>`, which print the status as JSON) and needs a node on the computer; a node from before this says to update it. `persona-link status` shows the last check without running anything (`claude-plugin.status.json`). The Electron setup window's **Run Agents on this computer** section shows the state and a **Keep the Persona plugin for Claude Code installed** toggle, which calls `plugin install` / `plugin uninstall`; it receives the state words only, never paths. `persona-link uninstall --purge` removes the connector, not the node or its plugin: run `persona-link plugin uninstall` first to take the plugin out of Claude Code too.

**Status to the hub.** `node.info` carries `claude_plugin` and the node sends a `claude.plugin` notification when it changes ([protocol.md](../protocol/protocol.md) "Node handshake"); the manager puts it on the machine as `claude_plugin` (`proto.ClaudePlugin`): `state` (`installed`, `outdated`, `disabled`, `not_installed`, `error`, `claude_missing`, `unavailable`), `auto`, `version`, `bundled`, `claude_version`, `config_dirs` (`path`, `state`, `version`, `opted_out`, `error`), `changed`, `error`, `checked_at`, `next_check`. The hub forwards it; no client shows it yet, and there is no per-machine switch in the apps.

## Responsibilities

- **persona-link** (`server/cmd/persona-link`, also reachable as `server/cmd/persona-link`; `server/internal/link`): a separate small program doing only these things:
  - Discovery: find the hub among candidate addresses and connect;
  - Reporting: identify the computer, version and address used;
  - Link: maintain the connection and forward streams opened by the hub unchanged to the local resident node;
  - Summary: put the hub's "Needs you" summary into its status (see "Needs you").

  It implements no specific feature.
- **node is unchanged**: all features live in `personad node`. This computer needs a resident node (installed together under "Install the node service" above; separately, use `persona-link node install`
  or `scripts/deploy/node-launchd.sh install`); persona-link forwards streams only to its socket `~/.persona/node-attach.sock`. This is the same path that SSH-started `node --stdio` forwarded to
  (`internal/node/attach.go`). Writing a systemd / launchd service for the resident node is packaging, not functionality: the service contents match those written by `scripts/deploy/node-launchd.sh`.
  Because the resident node runs in the user's login session, features that need that session live there too, such as
  sharing the computer's clipboard with the phone ([clipboard.md](clipboard.md)).
- **hub and manager** (since 2026-11-04 computers belong to the manager, [machines.md](../server/machines.md), "Who connects to computers"):
  - This computer is kind `link` in hub.json; nobody SSHs into it;
  - Persona Link still connects to the hub address; after authenticating, the hub forwards the connection unchanged to the manager (`devices.accept` + `GET /api/v1/bus/relay`, preserving individual messages and close codes);
    the manager speaks the protocol below;
  - As soon as the link says hello, the manager opens a stream to connect to the node (streams Persona Link opens are also relayed through the hub); the node protocol on the stream is exactly the SSH protocol;
  - When the link disconnects, this computer goes offline immediately; while the manager is away the hub returns 503 on `/api/v1/link`, and Persona Link backs off normally and connects when the manager returns.
- **Mac** packages persona-link and Persona Screen under `apple/PersonaLink/` and runs the connector as the independent launchd agent `com.persona.link`.
  On **Linux** (servers, NAS, Raspberry Pi, no GUI), `persona-link install` installs itself as a systemd user service; see "Linux (headless)";
  on **Windows**, it installs as a sign-in scheduled task supervising both it and the resident node; see [windows.md](windows.md), "Install and start at logon".

## Link (active mode)

```text
persona-link ── WSS /api/v1/link ──▶ hub TLS port (public.tls_listen)
  → hello {machine, version, address, features?}
                                               First message; hub closes with 4003 if machine id doesn't match credentials;
                                               features: ["attention"] asks for the "Needs you" summary
  ← endpoints {endpoints, hub_name, mesh_hubs, ping_ms?, ping_timeout_ms?, pending?, machine_name?}
                                               Sent on connect and again when hub addresses, linked group, heartbeat, pending status or this computer's name changes
  ← open {id}                                  hub wants to connect to node
      persona-link connects ~/.persona/node-attach.sock first, then opens WSS /api/v1/link/stream?id=<id>, forwarding bytes unchanged both ways
  → open_failed {id, error}                    Can't open (resident node not running, etc.); hub records the reason as this computer's error
  ← node {online, error}                       Whether hub connected to node, reported by `persona-link status`: sent when the
                                               node's connection changes or is resumed, and on connect when it is already up (#11262)
  → rename {id, machine_name}                  Asks the hub to call this computer machine_name ("": its id again); see "The computer's name"
  ← renamed {id, machine_name | code, error}   The hub's answer to that rename: the name it has now, or why not
  ← attention {attention}                      The "Needs you" summary, to a link whose hello asked for it: right after endpoints, then on every change
```

Both sides ignore a message type they don't know.

**Route selection**
- Candidate addresses are ordered "LAN → public IPv6 → domain → public IPv4", with manually added addresses first within their category.
- Starts trying the next candidate every 300 ms, several concurrently; the first connection wins and late successes are closed. Each candidate has an 8-second timeout.
- Away from home, even if LAN addresses fail or hang, it waits only another 300 ms before trying the next. These are configured values, not measured results.
- With an optional SSH tunnel (`ssh` in `link.json`, see "[SSH tunnel](#ssh-tunnel)" below): **Always use SSH** dials only the tunnel; otherwise direct candidates are raced first and the tunnel is used if all fail (and tried first on the next reconnect when the tunnel was the last working route, while probing direct candidates every 2 minutes to switch back).
- With an outbound HTTP(S) proxy (`proxy` in `link.json`, or `HTTPS_PROXY` / `ALL_PROXY`, see "[Outbound proxy](#outbound-proxy)" below): when every direct candidate fails, the candidates are raced through the proxy, before the SSH tunnel; it is sticky and switches back to direct the same way.

**Hub name and linked group**
- The hub includes `hub_name` in `endpoints`: `name` from hub.json, otherwise the short hostname of the hub machine; in Docker (where the hostname is a container id), `hub-<first 6 characters of hub_id>`
  ([config.md](config.md)); and `mesh_hubs`: the number of hubs in its linked group, including itself. Before linked hubs (#294) landed, this was fixed at 1, returned by `meshHubCount()`
  and wired up by the mesh lane; when the group changes the hub sends `endpoints` again (`sendLinkEndpoints`).
- persona-link saves the name as `hub_name` in link.json, so it knows which hub even when disconnected; status includes `hub_name`, and `mesh_hubs` while connected.
- The name is restricted to one printable line (control characters become spaces), up to 80 characters.
- Old hubs send neither: both fields are absent from the status JSON, and `persona-link status` displays the hub id. An `endpoints` message containing a name but no addresses does not clear candidate addresses.

**Pending confirmation** (#4655)
- A computer added through **Add Computer** on the phone / web is pending until the user taps **This is my computer** (`hub/machine_trust.go`). The hub hands pending machine ids to the manager (`pending` in `devices.config`),
  and the manager's `endpoints` to that computer's link carries `pending: true`; once confirmed (or rejected or expired), the hub hands over the config again and the manager sends `endpoints` without `pending`.
- `endpoints` is per machine: every message describes that computer's current state; absent `pending` means confirmed. persona-link updates `pending` in its status line accordingly.
- Old hubs don't send it: status omits `pending` and shows confirmed (Persona Link's status page already handles this in #4198). Passive mode (hub connects to computer) doesn't have it yet.

**The computer's name**
- The hub calls each computer by its hub.json `name` (its id when it has none). The id never changes: pins, mutes, trust and credentials are all by id; only the name is shown.
- Hubs that rename computers (hello feature `machine_rename`) say the name in `endpoints` (`machine_name`) and send `endpoints` again when it changes. persona-link keeps it as `machine_name` in link.json,
  shows it in its status (`machine_name`), in `persona-link status` (with the id in parentheses when they differ) and in the desktop setup window. Old hubs say nothing: everything shows the id as before.
- It can be renamed from either end, and every side follows without a restart or reconnect:
  - On the computer: `persona-link name <new>` (`--reset`: back to the id; no argument: shows it), the desktop setup window. They ask the running persona-link, which sends `rename` on its link;
    the manager asks the hub (`devices.rename`), the hub writes hub.json and answers, and persona-link reports the answer (`rename` in its status, by the request's id). The request is a file beside link.json
    (`link.rename.json`), taken within half a second; one older than 30 seconds is dropped.
  - On the hub: the admin page (`PUT /api/v1/admin/machines/{id}`) or a signed-in device (`PUT /api/v1/machines/{id}/name`); see [protocol.md](../protocol/protocol.md) "Computers".
- Only a confirmed computer renames itself: one waiting for the user to confirm it (`pending`) is refused (`pending`), so it can't pass itself off as another computer by name. Passive mode has no link to ask on:
  rename it on the hub; the hub's info reads carry the name (`X-Persona-Machine-Name`, URL-escaped) and the connector keeps it in link.json's `passive.machine_name`.
- Pairing hands the hub what this computer's system calls it (`system_name` with the pairing request: the Mac's Computer Name, else the host name). The hub names a computer after it once, when the computer has
  no name of its own yet (added without one, so its name is its id); a name it was given, or one the user set later, is kept. Nothing renames the computer itself.
- Names are one line, at most 40 characters (the hub refuses longer ones with `invalid_name`).

**TLS**
- Pins the hub public-key fingerprint received during pairing (SHA-256 of SubjectPublicKeyInfo); LAN connections also use TLS.
- The hub certificate is self-signed; trust comes from this fingerprint, just like the App's public direct connection ([network.md](network.md), "TLS and public-key pinning").
- If the pairing link has no fingerprint (the hub doesn't serve TLS itself and sits behind an HTTPS reverse proxy), validate the proxy certificate against system-trusted CAs and the hostname; see "[Hub behind a reverse proxy](#hub-behind-a-reverse-proxy)".
  Pairings with a fingerprint accept only the pinned public key and never fall back to system CAs.

**Reporting**
- The first message after connecting is hello, reporting this computer's machine id, persona-link version and chosen address.
- The manager checks that the machine id matches the computer bound to the credentials (the hub includes that binding when forwarding), then immediately opens a stream to the node without waiting for the SSH retry interval.
- This computer gets an extra `link` field in the machine list: `{version, address, since}`.

**Heartbeat**
- Both sides ping every 10 seconds and disconnect after 5 seconds without pong. Dead links are detected in about 15 seconds.
- Slow links can be configured in hub.json's `link` (`ping_seconds`, `ping_timeout_seconds`, [config.md](config.md)): the hub pings accordingly, and when configured the manager includes
  `ping_ms`, `ping_timeout_ms` in `endpoints`; Persona Link adopts them without reconnecting. If absent (old hub, unconfigured) or not both positive, it uses its own values. New connections use their own values until the hub speaks.
  Both sides clamp them between 50 milliseconds and 10 minutes (`link.PingTimes`). Older Persona Link versions don't recognize these fields and retain 10 seconds / 5 seconds.
- Ping queues behind data already being sent on the connection: at only tens of KB per second from hub to computer, a 1 MiB block takes over 30 seconds; any shorter timeout guarantees disconnects, repeating after reconnect.
- Two deadlines heartbeat tuning doesn't control (`docs/lessons/server.md`, "Heartbeat timeouts on slow links"): coder/websocket waits at most 5 seconds for the write lock when sending ping / pong, so it still disconnects while a message larger than the send buffer is being written;
  the hub relay waits at most 1 minute to write one message (`spliceWS`), so a 1 MiB block still times out below roughly 17 KB/s. The hub's link-stop reason includes the ping error to distinguish these cases.
- The hub also pings streams, detecting when the link is alive but a stream's TCP connection silently died.
- Ping runs between hub and Persona Link (the hub owns this end of the connection): without pong, the hub closes it, and the relay to the manager closes with "no pong within …"; the manager treats that as a disconnected link.
  The hub reports each pong round-trip to the manager (`devices.link_rtt`), which supplies the path distance for routing and records sent to the primary hub.
- These are configured values, not measured results.

**Reconnect**
- Exponential backoff: starts at 1 second, doubles up to 1 minute; randomized between half and full delay each time. After a link connected for a full minute disconnects, it starts over.
- When a normal wait exceeds 3 seconds, Link races eligible direct addresses and outbound proxy targets with an unauthenticated `GET /api/v1/health`, using the same TLS verification as the link. If none returns a 2xx response with `"ok": true` at the start, Link probes every 2 seconds and retries as soon as one does. SSH logins happen only on full connection attempts. SSH-only configurations, including **Always use SSH**, skip these probes, as do credential, replacement and rate-limit waits.
- Special cases:

  | Case | Wait |
  | --- | --- |
  | Replaced by a new link from the same computer (4001) | 30 seconds |
  | Credentials rejected (401 / 403) or revoked (4002) | 5 minutes, or retry within 2 seconds when link.json changes (re-pairing) |
  | Hub restarts (1001) | Normal backoff |
  | Manual **Reconnect now** | No wait |
  | Paired again or SSH tunnel edited while connected (new token, pin, trust, machine or `ssh` in link.json) | No wait: the link ends as with **Reconnect now** and comes back with the new configuration within about 2 seconds |

- Any wait ends within 2 seconds when link.json changes (re-paired, an address added). The change counts from when the attempt read link.json, so a new credential written while the hub was refusing the old one is tried at the next check, not after 5 minutes; while a link is up, only a new credential counts (the link writes the hub's addresses and names into link.json itself).
- While connected, persona-link looks at link.json every 2 seconds when its modification time changed and compares the pairing itself (token, pin, trust, machine), not just the time: the hub's endpoints and renames rewrite link.json with the same pairing and leave the link alone.
- Only one persona-link can run per computer (`~/.persona/link.json.run` file lock), or they replace each other repeatedly.

**Network changes**
- Mac: the independent connector watches network changes through its address watcher and periodic reconnect checks:
  - If the link's original local address is gone, reconnect immediately;
  - If still present, ping immediately (3 seconds, or the hub-configured heartbeat timeout if longer), reconnecting on failure;
  - If already disconnected, retry immediately.
- Linux: persona-link watches address changes itself with netlink (`internal/netaddr`); Windows: with `NotifyIpInterfaceChange` (same package).
- **Reconnect now** is `SIGUSR2`. Windows has no such signals: there `persona-link reconnect` and the pairing flow set the corresponding named events ([windows.md](windows.md), "Install and start at logon"). Waiting for pong blocks neither this nor the network-change check: on a slow link, ping may wait the hub's entire configured timeout.

**Forwarding**
- Only forwards to `~/.persona/node-attach.sock` and only connects back to the hub address used by this link; stream ids may contain only letters, digits, `-`, `_`. The hub can't make it connect elsewhere.
- Streams enable permessage-deflate compression without context takeover (`link.StreamCompression`): each message is compressed on its own, so an idle stream keeps no compression state (with context takeover, about 0.8 MB per end, for about a sixth fewer bytes on the wire). Either end asking for it is enough, so it also applies when only the hub or only Persona Link is new. SSH previously used `Compression=yes`; see the comment in `devices/machine.go` (the hub-to-manager hop is local loopback and isn't compressed).
- To the resident node, each stream is a new attach connection, replacing the previous one just as with SSH.

## Hub behind a reverse proxy

When the hub doesn't serve TLS itself and is mounted under a reverse proxy's subpath (`public_url` is `https://<domain>/<prefix>`, the proxy has a public-CA certificate and strips the prefix before forwarding to the hub's plaintext port),
the phone App and web already work ([network.md](network.md), "Reverse-proxy subpaths (Android)"), and active-mode Persona Link works too (#2573):

```text
persona-link ── WSS https://<domain>/<prefix>/api/v1/link ──▶ reverse proxy (TLS, public CA) ── strip /<prefix>, plaintext ──▶ hub listen port
```

- **Pairing**: without hub TLS, `personad hub pair --machine <id>` (and **Pairing link** in admin) still returns a link if `public_url` is `https://…`: its `url` is `public_url`,
  with no `fp` or `tls`. persona-link accepts this link, preserving the path prefix and spelling out the port (`https://<domain>:443/<prefix>`, matching candidates sent by the hub);
  link.json stores `"trust": "system"` and no fingerprint. A link with no `fp` and no `https://` `url` is rejected (no usable address).
- **Validation**: every candidate in this pairing is checked against system-trusted CAs and the hostname (TLS 1.2 or later, `http/1.1`), like opening the web app in a browser. Pairings with `fp` still accept only the pinned public key:
  if the proxy certificate doesn't match, the error is "TLS public-key fingerprint differs from pairing", with no fallback to system CAs.
- **Link**: `/api/v1/link`, `/api/v1/link/stream` and pairing's `/api/v1/auth/tokens` are appended after the prefix; heartbeat, reconnect backoff and routing remain as above. The hub's candidates already include the prefix in `public_url`.
- **Hub side**: requests arriving through the proxy are plaintext; the hub accepts links only in these two cases (credentials must still match):
  - Proxy and hub are on the same machine (loopback source);
  - Proxy is on another LAN machine (private source address), and forwarding headers say it received HTTPS: `X-Forwarded-Proto: https` (or `Forwarded: proto=https`).

  Plaintext from public sources is always rejected (`403 tls_required`), whatever the headers; other entry points (App APIs, bus, mesh) are unaffected. If the hub runs in Docker and port mapping makes the source appear as a container gateway, it is treated as public;
  add the gateway to `security.trusted_cidrs` ([network.md](network.md), "Source classification and reverse proxies"). Forwarding headers can be spoofed: on the LAN, spoofing only lets one's own link credentials travel in plaintext; Persona Link itself never sends plaintext.
- **Status**: `persona-link status` adds "Validation: via reverse proxy · system certificates"; `trust` in `--json` is `system` (`pin` for pinning); the running status line carries `"trust": "system"`;
  `persona-link status` and the redacted desktop setup status show system trust when it is in use. Untrusted certificates and hostname mismatches report "Certificate not trusted by the system" and "Certificate hostname doesn't match" respectively.

nginx example (replace placeholders; put `map` in `http { }`, and `location` in the port-443 `server { }`):

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

location /<prefix>/ {
    proxy_pass http://<hub address>:<hub listen port>/;   # Trailing / strips the prefix
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;              # WebSocket: /api/v1/ws, /api/v1/link, /api/v1/link/stream
    proxy_set_header Connection $connection_upgrade;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto https;            # Allows a plaintext link when proxy is on another LAN machine
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_buffering off;
    proxy_read_timeout 1h;                               # Long connection: exceed heartbeat (default 10s, configurable in hub.json link) with margin
    proxy_send_timeout 1h;
}
```

**Trade-off**: pinning trusts one hub public key; system trust trusts every CA in the system. Any CA that signs for this domain (a compromised or mistaken CA, or device management able to install roots on this computer) can impersonate the hub,
obtain this computer's link credentials and connect hub streams to its resident node. This is the same as accessing the web app in a browser: better than plaintext, weaker than pinning. The proxy-to-hub hop is plaintext and must be on a trusted network (same machine or LAN).
Prefer the hub's own TLS and fingerprint when possible; when hub TLS is enabled, pairing links always carry `fp` and won't use system trust.

## Outbound proxy

A computer that reaches the internet only through an HTTP(S) proxy (common on company networks) can still use active mode: persona-link reaches the hub with an HTTP `CONNECT` through that proxy (#10659). Passive mode, the hub and the manager are unchanged.

- **Which proxy**: `proxy` in `link.json` when it is set (`persona-link proxy <url>`, or `--proxy <url>` on `pair` and `install`, which saves it before pairing so the pairing goes through it too), else `HTTPS_PROXY`, then `ALL_PROXY` (each in lower case before upper case, as Go's `net/http` reads them) from persona-link's own environment. A launchd agent, systemd user service or scheduled task rarely has those variables, so on a service set the proxy with `persona-link proxy`. `persona-link proxy --off` (`"proxy": "off"`) uses no proxy, not even the environment's; `persona-link proxy --env` removes the setting again; `persona-link proxy` shows it. The URL is `http://` or `https://` with `[user:password@]host[:port]`; a bare `host:port` means `http://`, as for `HTTPS_PROXY`. SOCKS proxies are not supported: one in the environment is left to the programs it is for, and `persona-link proxy` refuses one (a hand-edited unusable `proxy` shows as invalid in `status` and fails only the proxy route). `NO_PROXY` / `no_proxy` exempts hub addresses, with Go's rules: `localhost` and loopback addresses never go through a proxy.
- **Routing order**: direct candidates are raced first by default. If they all fail, Link races the same candidates through the proxy (LAN addresses last), then tries the SSH tunnel if configured. **Always use SSH** dials only the tunnel. A pinned hub that refuses the credential ends the attempt on any route. An unusable proxy setting affects only the proxy route. Pairing follows the default order.
- **Reconnects and restarts**: Link tries the last working proxy or SSH route first while the pairing, route settings (including environment proxy and `NO_PROXY`), and local network still match. It keeps this hint in the disposable `link.route.json` file beside `link.json`. Link compares usable IPv4 addresses and IPv6 `/64` prefixes, so IPv6 privacy address rotation on the same prefix does not clear the hint. A new pairing, changed settings, a detected network change, or a missing or unreadable hint restores the default order. If no usable address is known yet, Link skips the saved hint without deleting it and checks again on the next dial. While connected through a fallback, Link probes direct candidates every 2 minutes (`link.DirectRetryEvery`) and on network changes. When one connects, it takes over and Link forgets the saved fallback.
- **Why the proxy comes before the tunnel**: the proxy is what the network itself asks for and needs nothing on the hub machine, no `ssh` process, keys or hardware-key touches; the tunnel stays the explicit fallback for a hub that is reachable only over SSH.
- **One route for the link and its streams**: the link WebSocket and every Agent stream opened on it go through the same proxy, each with a `CONNECT` of its own.
- **End-to-end TLS unchanged**: the hub's TLS handshake runs inside the tunnel with the check a direct connection uses, pinned to `fp` or checked against system CAs and the hub's host name when `trust` is `system`, so a wrong hub key is refused through the proxy with the same words as directly. The check is the HTTP transport's `tls.Config` (the pin in `VerifyConnection`) on every route. An `https://` proxy is checked first and separately, against system CAs and its own name; it can never stand in for the hub.
- **Credentials**: proxy credentials stay in `link.json` (0600) and are sent only in each `CONNECT`'s `Proxy-Authorization` header (Basic), which an `http://` proxy receives in plain text, as with any HTTP proxy; prefer `https://` when the proxy offers it. `persona-link proxy`, `persona-link status` (text and `--json`) and errors show the proxy without its user name and password; the log says only `(proxy)`, never the proxy's address. A failing proxy has its own sentence (cannot reach it, its certificate is not trusted, it did not complete TLS, it refused with HTTP `<status>`, it did not open the connection, the setting is not a usable URL), shown as **Proxy** `<hub address>`: `<sentence>`; a tunnel that breaks later fails as the hub's handshake does, without the proxy's address.
- **Status**: connected through the proxy, the status line has `"kind": "proxy"`, `address` the hub URL reached through it and `proxy` the proxy without credentials. `persona-link status` shows **Connected · Proxy** `<hub>` and lists the proxy after the direct addresses (marked **In use**); `status --json` also has `proxy`, the `link.json` setting without credentials, `off`, or `invalid`.
- **Changing it while connected**: `persona-link proxy` does not interrupt the running link; the link reads `link.json` itself, and a new proxy setting reconnects a link on a fallback route (the proxy, or an SSH tunnel without **Always use SSH**, as the proxy comes before the tunnel). A direct link stays as it is.

## SSH tunnel

When a hub machine exposes only SSH (port 22) — its HTTP and TLS ports listen on `127.0.0.1` only, with no open public port or reverse proxy — active-mode Persona Link can reach the hub's TLS port through an SSH tunnel (#3775), matching the apps' SSH tunnel ([network.md](network.md), "Connecting to hub through SSH").

> **How this fits the "No SSH" decision at the top of this page.** "No SSH" there is about the hub SSH-ing into computers, which requires inbound reachability and gives the hub shell-level permission on the computer. Here the computer still dials out as before; SSH is only an optional transport to reach the hub's TLS port, and the hub gains no access to the computer.

### Setting up an SSH-only hub

1. Keep the hub's HTTP and TLS listeners on loopback in `hub.json` (for example `listen` `127.0.0.1:18660`, `public.tls_listen` `127.0.0.1:18662`).
2. Generate the pairing link on the hub machine:
   ```sh
   personad hub pair --machine <id> --add --name "<display name>"
   ```
   The resulting `persona://link?…` carries `url=http://127.0.0.1:18660`, `tls=18662` and `fp=<hub fingerprint>`.
3. Optionally dedicate an SSH key to Persona Link and restrict it in `~/.ssh/authorized_keys` on the SSH server so it can forward only to the hub's loopback TLS port and cannot open a shell:
   ```text
   restrict,port-forwarding,permitopen="127.0.0.1:18662" ssh-ed25519 AAAA... persona-link
   ```
   Replace `18662` with the hub's TLS port (`public.tls_listen`).

### Pairing window and CLI

- **Desktop setup**: active/passive pairing, status and reconnect use the fixed Link command bridge. SSH tunnel configuration remains available through the CLI below; the retired tray-specific tunnel editor is not part of the desktop shell:
  - **SSH server**: `[user@]host[:port]`, or a host alias from `~/.ssh/config`.
  - **Hub address as that server sees it**: defaults to `127.0.0.1:<TLS port in the pairing link>`; with no link and no saved address yet, to `127.0.0.1:18662`, the TLS port `personad hub setup` uses.
  - **Always use SSH**: uses only the tunnel. Enabled automatically when the pairing link names only loopback addresses; otherwise off so direct addresses are tried first.
  - **Test connection**: opens a one-off tunnel, verifies TLS against the hub, and shows the result in plain words.
  - After pairing, the same section edits, switches **Always use SSH**, or removes the tunnel (**Remove tunnel**) without pairing again; the running link reconnects within about 2 seconds.
- **Command line** (Linux, scripts, or the helper called by the GUI windows):
  ```sh
  persona-link pair --ssh user@example.com [--ssh-target 127.0.0.1:18662] [--ssh-always | --no-ssh-always] '<link>'
  persona-link ssh                                      # Show saved tunnel
  persona-link ssh user@example.com [--target host:port] [--always | --no-always]
  persona-link ssh --test [--link '<link>']             # Test tunnel end to end
  persona-link ssh --off                                # Remove tunnel without unpairing
  ```
  `persona-link install --pair '<link>' --ssh user@example.com …` accepts the same `--ssh*` flags during installation.
- **`link.json`**: stored alongside `endpoints` as `"ssh": {"server": "user@example.com", "target": "127.0.0.1:18662", "always": true}` (`always` omitted when false).

### Transport, TLS, routing and errors

- **System `ssh` client**: `persona-link` runs the system `ssh` binary (`-N -L 127.0.0.1:<ephemeral>:<target> -o BatchMode=yes -o ExitOnForwardFailure=yes`, plus `-p <port>` when specified). It uses the keys, `ssh-agent`, `ProxyJump` and `known_hosts` the user already configured, stores no SSH key or password, and never prompts interactively.
- **One SSH session for the link and all streams**: the active link WebSocket (`/api/v1/link`) and every Agent stream (`/api/v1/link/stream`) dial through the same local forward on that single `ssh` process, so opening Agent sessions triggers no extra SSH authentication or hardware-key touch. Killing `ssh` or losing the network drops the link and reconnects automatically; quitting, switching to passive mode, switching back to a direct route, or unpairing terminates the `ssh` process cleanly.
- **End-to-end TLS unchanged**: the TLS handshake (pinned to `fp`, or checked against system CAs and hostname when `trust` is `system`) still runs end to end through the tunnel to the hub's TLS port; a wrong hub key is refused identically to a direct connection. Over the tunnel, `hello.address` is left empty so the hub does not record the SSH server string as one of its own HTTPS URLs.
- **Routing order**:
  - **Always use SSH** on: only the tunnel is dialed; direct hub candidates are skipped.
  - **Always use SSH** off: the default order is direct, outbound proxy if configured, then SSH. If there are no direct candidates, Link dials only the tunnel. If SSH was the last working route, Link tries it first on reconnect and after a restart while the pairing, route settings and local network still match (see "Outbound proxy"). Link probes direct candidates every 2 minutes (`link.DirectRetryEvery`) and on network changes. When one connects, it takes over, the tunnel closes and Link forgets the saved fallback.
- **Status and plain-word errors**: when connected through the tunnel, status reports `"kind": "ssh"` and `"address": "<server>"`, labelled **SSH** in `en`, `zh-Hans` and `ja` across `persona-link status` and the desktop setup status (and marked **In use** in the address list). Five failure modes each produce a distinct single-sentence explanation (`ssh_error` in the status line): no `ssh` command installed, authentication refused, server host key not trusted yet (with `ssh [-p <port>] <server>` to accept it once in a terminal), port forwarding refused by the server (`AllowTcpForwarding` / `permitopen`), and the hub not answering at `<target>`.

## Passive mode (the hub connects)

The protocol is fixed in [plan-hub-center.md](../archive/operations/plan-hub-center.md), section 4; details are here. The dialing side (#435) follows this specification and has belonged to the manager since 2026-11-04
(`server/internal/devices/service.go`; credentials generated during pairing are handed to the hub for encrypted storage with its key and retrieved when dialing). Implementation: `server/internal/link/`
(`listen.go` for the listener and three endpoints, `passive.go` for pairing codes and links, `tlsid.go` for identity, `ratelimit.go` for rate limiting); CLI in `server/cmd/persona-link/passive.go`.

### Listener and identity

- Listens on `passive.listen` in link.json, default `:18663` (all addresses); change with `persona-link mode passive --listen <address>` or `pair --listen <address>`. Only this one port is opened.
- TLS 1.2 or later, ALPN only `http/1.1`. Identity is the helper's own ECDSA P-256 key and self-signed certificate in `~/.persona/link-tls/` (directory 0700, `link.key` and `link.crt` 0600),
  generated on the first `pair --listen` or first listener start. The certificate contains only `connector.persona.invalid`, no hostname or addresses (anyone connecting to this port can see the certificate).
- Fingerprint format matches the hub's: SHA-256 of the certificate's SubjectPublicKeyInfo DER, unpadded base64url (43 characters). The hub pins the fingerprint from the pairing link and doesn't validate the certificate name.
- Certificate lifetime is one year; with fewer than 30 days left it is renewed with the same key (also while listening, checked during handshakes), keeping the fingerprint. Changing the key (deleting `link-tls/`) requires re-pairing. These are configured values.

### Pairing

The direction is reversed from active mode (the computer may not be able to reach the hub):

1. On the computer: `persona-link pair --listen [<listen address>] [--addr <address>]…` (or passive pairing in the desktop setup window).
   It switches link.json to passive mode and generates a one-time pairing code (128 random bits, `jc_` prefix, only SHA-256 stored); **stdout contains only the link**, explanations go to stderr:

   ```text
   persona://connector?addr=<host:port>[&addr=<host:port>…]&fp=<helper fingerprint>&code=<one-time code>
   ```

   - `addr` may repeat, best first, always `host:port` (no scheme, IPv6 in brackets, URL-encoded as a whole). Without `--addr`, local addresses plus port are ordered like active-mode hub candidates:
     LAN (private IPv4, ULA) → public IPv6 (excluding temporary, deprecated and tentative addresses on Linux) → public IPv4; exclude loopback, link-local, tunnel and bridge interfaces
     (Docker, VPN, Tailscale…). If all interfaces are virtual (container eth0, NAS interface attached to a virtual switch), fall back to those, still excluding tunnels and bridges.
     If none remain, `pair --listen` errors asking for `--addr`; `install --listen` still installs the service and instructs you to run `pair --listen --addr` later. If `--addr` is supplied, only those addresses are used.
   - Codes expire after 10 minutes and can be used once; another `pair --listen` generates a new code and invalidates the old one. The previously paired hub can keep connecting until the new hub consumes the new code.
2. Paste the link into hub admin **Computers › Add Computer › Hub connects** (addresses can be edited and a proxy supplied). The hub connects with `fp` pinning and exchanges the code for long-term credentials through `POST /api/v1/connector/pair`:
   the hub generates the random token itself; the helper stores only its SHA-256.
3. Afterwards the hub uses that credential to open `stream` to the node, as in active mode.

### Endpoints

Only these four exist: no shell, files or port forwarding; streams can connect only to `~/.persona/node-attach.sock`. All other paths and methods return `404 {"error":"not_found"}`.
Errors are JSON: `{"error":"<code>","message":"<human-readable Chinese>"}`.

| Endpoint | Authentication | Request | Response |
| --- | --- | --- | --- |
| `POST /api/v1/connector/pair` | One-time code | Body ≤ 16 KiB: `{"code","token","machine","hub_id","hub_name"}`; `code`, `token`, `machine` required (`token`: 32–512 printable ASCII characters, no whitespace), `hub_id`, `hub_name` optional but please include them | `200 {"version"}`; invalid fields: `400 bad_request` (not rate-limited, doesn't invalidate the code); wrong, expired or used code: `401 bad_code` (rate-limited) |
| `GET /api/v1/connector/info` | `Authorization: Bearer <credential>` | — | `200 {"machine","version","features","attention_rev"?}`: `machine` is the machine id supplied by the hub during pairing; `features: ["attention"]`: it takes the "Needs you" summary; `attention_rev`: the `rev` of the summary it holds (absent: none); missing / wrong credential: `401 unauthorized` |
| `POST /api/v1/connector/attention` | Same | One summary object (see "Needs you") and nothing after it but white space, the whole body ≤ 64 KiB | `204`; no `rev`, anything else in the body or over the cap: `400 bad_request`; missing / wrong credential: `401 unauthorized` |
| `GET /api/v1/connector/stream` | Same | WebSocket upgrade; optional `X-Persona-Ping` (heartbeat, below) | Authenticate, connect node-attach.sock, then upgrade: node unreachable: `503 node_unavailable` (`message` gives the reason for the hub to record as this computer's error); non-upgrade request: `400`; after upgrading, see below |

- Successful pairing replaces the previously paired hub ("only trusts the hub it paired with": the group's primary; a new primary re-pairs after a switch). Old credentials stop working immediately, and streams opened with them close with code **4002**.
- Streams are binary messages containing unchanged node bytes in both directions. Accepts permessage-deflate without context takeover, as active-mode streams use it (SSH formerly used `Compression=yes`); a manager that still asks for context takeover gets none.
  The helper pings every 10 seconds and disconnects after 5 seconds without pong, while answering hub pings normally. When the hub closes a stream, the helper half-closes the node connection (write end), letting the node finish reading and clean up; the stream ends when the node closes.
  With a heartbeat configured in hub.json (`link`), the stream request carries `X-Persona-Ping: <interval milliseconds> <timeout milliseconds>`; the helper uses it on that stream (also clamped to 50 milliseconds–10 minutes);
  absent or not two numbers, it uses its own values. Old helpers ignore the header and retain 10 seconds / 5 seconds.
- Each stream is a new attach to the resident node, replacing the previous one as in active mode and SSH; the helper itself doesn't decide which is newest.
- When the helper stops or changes back to active mode, streams close with **1001**. Passive pairing stays in link.json; switching back to passive makes the same credential usable again.

### Authentication and rate limiting

- Only the credential hash is stored (`passive.token_sha256` in link.json), compared in constant time.
- Failed authentication (wrong code, missing / wrong credential) is counted per source: one IPv4 address or one IPv6 /64. **LAN and loopback count too** (the hub is usually on the LAN, which must not exempt it).
  10 failures within 60 seconds block the source for 15 minutes; blocked sources are rejected before authentication, returning `429 {"error":"rate_limited"}` and `Retry-After` (seconds) even with correct credentials. At most 4096 sources are tracked;
  when full, unblocked ones are evicted first. These are configured values, not measured results.
- A request must arrive whole within 30 seconds, body included (headers within 10): when the body stalls, the helper answers (`400` on pairing) and closes the connection, so a peer that never authenticates cannot hold one longer. An upgraded stream is not bound by this.
- Logs: at most one line per 30 seconds for authentication and TLS handshake failures (with the number omitted); sources are recorded only as /24 or /64.

### Recommendations for hub implementers

- Try `addr` values in link order (or start with only the first), and remember the working one; use edited addresses if the user changes them in admin.
- Back off on 401; don't keep retrying bad credentials (you'll block your own source). On 429, wait for `Retry-After`.
- Stream closed with 4002: another hub paired this computer; mark **Pairing required** and don't retry. On 1001 or connection failure, back off and reconnect normally.
- 503 `node_unavailable`: the link works but the resident node isn't running; show `message` as this computer's error and retry later.
- To check helper liveness and RTT: `GET /api/v1/connector/info`. The helper records the hub's last visit time and source, reported by `persona-link status` and `persona-link status`.

### Passive mode in link.json

```json
{"mode":"passive","passive":{"listen":":18663","hub_id":"…","hub_name":"Home","machine":"example-workstation",
 "token_sha256":"…","paired_at":"…","code":{"sha256":"…","expires":"…"}}}
```

- Separate from active-mode fields (`hub`, `machine`, `fp`, `token`, `endpoints`), neither overwrites the other; absent `mode` or `active` means active mode (old files still work).
- `persona-link pair <link>` (active) sets `mode` to `active`, `pair --listen` sets `passive`, and `mode` changes only the mode. A running persona-link checks link.json every second,
  switching when mode changes (closing the link or listener and opening the other); a changed listen address changes the port.
- `persona-link uninstall --purge` also deletes `link-tls/`.

### Security boundary

- Someone able to reach this port can do nothing without a code or credential, only see the certificate (no hostname or addresses) and 404; guesses are rate-limited, and both are random values of at least 128 bits.
- A hub holding the credential can only open streams to the resident node, with exactly the same capabilities as via SSH (the node protocol), no more.
- For 10 minutes until consumed, the pairing link grants pairing authority: don't paste it into an untrusted place. Its `fp` prevents the hub connecting to an impersonating helper.
- Firewall: open this TCP port only to the hub (or its proxy), not everyone. `pair --listen`, `install --listen` and `mode passive` all show a reminder, for example
  `sudo ufw allow from <hub address> to any port 18663 proto tcp`; likewise for routers and cloud security groups. With the Mac application firewall enabled, the system may ask whether to allow
  incoming connections on first listen (not tested).

## Needs you

The manager that carries this computer tells its Persona Link which agents wait for the user. The connector records the summary; the Electron desktop uses the shared web Needs-you data and notifications rather than implementing a second notification policy.
persona-link only relays the summary into its status: it shows nothing itself, and nothing on this computer can answer or approve a card with it.

**What is in it**
- The list and the count are the web app's "Needs you" badge as it is (`stateCounts().badge` in `web/src/lib/sessions.ts`): agents whose row says "Needs you" (the same rows the web app gets; a card read
  just after its row does not count before the row says so), without quiet tasks' agents and the assistant's conversations;
  muted agents and agents waiting only on a question the assistant is still deciding on (`delegating`) stay. One item per agent, by its oldest card handed to the user; while it has none,
  by its oldest card (one the manager has not decided on yet, or a question the assistant is still deciding on). Cards of the same time go by id. An agent with no card that can stand for it
  (a terminal dialog, an MCP elicitation) is `screen`.
- Whether an item notifies is the phone's push rule, worked out by the manager into `notify`: only once one of its cards is the user's (the manager rang it, when the phone is pushed; not a question
  the assistant is still deciding on: such a question is pushed only once the assistant hands it to the user), and never for a muted agent (or one in a muted space;
  `server/internal/assistant/fleet_mutes.go`). An agent in a terminal dialog notifies once the screen card made for it is announced (a moment later, or when its dialog can be read),
  as the phone is pushed then; the web app rings such a dialog at once. Otherwise the web app's system notifications follow the same rule. Clients notify on `notify` alone
  and never work the rule out themselves; `muted` is only for showing the item muted. An item notifies the first time it shows with `notify: true`, which may come after it was listed
  without it: a question the assistant hands to the user keeps its `interaction_id` and `since`.
- A card made again (a screen card read again once its node is back, a card the manager takes back after a restart, a Codex card listed again) notifies only when the manager can tell it
  rang before: a screen card read again as the very card that rang (its own id; one matched only by its session is not). Any other comes back not notifying, until it rings again: fewer notifications rather than more. A hook's card waits in the manager
  while its node reconnects: it is the same card and keeps what it was. (Persona Link takes the first summary of a link as its baseline, so a card pushed before a restart does not ring
  there again either.)
- `spec/fixtures/attention` holds the manager and the web app to the same list, count and `notify`.
- Order: by when the agent started waiting (`since`), newest first, then by machine and session id. Past 20 items the oldest are cut, so an agent that just started waiting is always listed and can be notified.
  The same agents and cards always give the same summary.
- Scope: the agents of the manager that carries this computer; with one manager (the default), every agent.

```json
{"rev": "9f2c41d0a1b2c3d4", "needs_you": 3, "more": 0, "web_url": "https://hub.example.com/persona",
 "items": [{"machine_id": "laptop", "session_id": "<session id>", "title": "Fix the login redirect", "place": "Laptop · web › 2",
            "kind": "permission", "detail": "Bash: npm test", "muted": false, "notify": true, "interaction_id": "<card id>",
            "since": "2026-11-09T07:00:00Z", "route": "/s/laptop/<session id>"}]}
```

- `needs_you`: the count, muted and not-notifying items included; `items`: at most 20, newest first; `more`: the rest. Nothing needs you: `"needs_you": 0, "items": []`.
- `kind`: `permission` / `plan` / `question` / `screen`. `interaction_id`: the card's id (a screen card has one too); absent for an agent no card stands for. `since`: when it started waiting, RFC 3339 UTC to the second; absent when unknown.
- `title`, `place`: at most 80 characters, `detail` at most 60, the ellipsis that marks a cut included. The largest summary stays far below the 64 KiB post cap and the link's 1 MiB read limit.
- `route`: the web app's path for the session (`/s/<machine>/<session>`, percent-encoded); open `web_url` + `route`.
- `web_url`: the hub's https `public_url`, sub-path included (`devicewire.Config.web_url`); without one, in active mode the hub address of the link's hello, in passive mode absent.
  The Electron desktop uses its configured hub origin, not an arbitrary address from a summary.
- `rev`: a hash of the rest; the same summary has the same `rev`, so sending it again changes nothing.

**When it is sent**
- The manager works it out in `liveLoop` from the Live Activity's inputs (woken by session lists, cards and mutes, at most every 100 ms, and every 15 seconds) and hands it on only when it changed.
  The Live Activity's own count does not change: it leaves muted agents out and counts questions the assistant is still deciding on.
- Active mode: to a link whose hello listed `features: ["attention"]`, once right after `endpoints`, then on every change.
- Passive mode: `POST /api/v1/connector/attention` to a connector whose `info` listed `attention`, on every change; and again after an `info` read whose `attention_rev` is not the summary's
  (a post that failed, a connector that restarted). Not while the connector holds the hub off (`401`, `429`), as for `info` and streams.
- A computer waiting to be confirmed or marked `trust: "work"` gets an empty summary (`needs_you: 0`, no items, no `web_url`): the summary names every agent's work, which neither may read.
  That also takes back what a computer had when it may no longer have it, without the link reconnecting. It gets the summary once it may. While the hub cannot say which computers wait to be
  confirmed, nothing new goes to a computer that is not known to be a work one: what it has stays, and one that has nothing gets nothing (an empty summary and the full one back would ring every
  item again). A work computer gets the empty summary all the same.

**On the computer**
- persona-link puts the latest summary into its status as `attention`: the JSON status line and `link.status.json` (`persona-link status`).
- Active mode: the status is reset when the link drops and when it connects, so the last link's list never shows on the next one. Passive mode: the summary goes when another hub pairs,
  and after 2 minutes without the hub coming in (it may have answered the cards meanwhile); the hub's next `info` read sees that and posts it again.

**Old and new**
- Old persona-link: no `features` in its hello, so the manager sends nothing (an unknown type would be ignored anyway). Old connector: no `features` in its `info`, so it is never posted to.
- Old manager: ignores `features` and the extra `info` fields and sends nothing; persona-link's status has no `attention`.

## Presence

The Electron desktop decides presence from lock, idle and notification state and sends `register_desktop` over its authenticated web socket. It does not delegate this to Link ([desktop.md](../desktop/desktop.md)). The compatibility parent-presence protocol below is retained for older helper integrations; an independent CLI service sends neither message and cannot mute the phone by itself. Active mode only: a passive-mode connector is no desktop.

**Compatibility parent protocol**
- An older parent can provide presence and notification confirmations; the retired menu bar's rules are in [the archive](../archive/operations/link-menu-tray.md). The independent Link service does not send either.
- The CLI Link reports nothing on stdin: it cannot show notifications, so muting the phone for it would lose alerts. Use the Electron desktop for desktop notifications and Phone Muted.

**persona-link** (`internal/link/presence.go`)
- `run --parent` reads stdin line by line; a line that is neither presence nor a confirmation is skipped, and the end of stdin still means the parent went away.
- It keeps the latest presence line and forwards every one, the parent's repeats included; it never repeats one by itself, so presence lapses when the parent goes quiet.
  An active line older than 120 seconds counts as inactive.
- On the link: `{"type":"presence","active","notifies","ttl_ms"}` as the link comes up and for every presence line (`ttl_ms` is what is left of the 120 seconds since the App wrote it),
  and `{"type":"rang","interaction_id"}` for every confirmation that names this link (`conn`): persona-link numbers its links in the status line, rising with each link and not reused by a restarted persona-link, so a confirmation for an earlier link, still in the pipe when the link changed or persona-link restarted, is dropped.
  The number stays between the parent and persona-link; the hub never sees it.

**The manager** (`internal/devices/presence.go`)
- Keeps the presence until when it heard it + `ttl_ms` (120 seconds when missing, at most 4 minutes), and reports its desktops to the hub in `devices.desktops`: one report at a time from one task, in order, a run of requests making one;
  on every presence it hears (each renewal), when a delivery may change which computers count (a confirmation never does: it only wakes the pushes waiting for it), after a `devices.state` when the list changed, and in full, as it is then, after the full `devices.state` each handshake sends.
  Another report may reach the hub before that state: the hub drops the entry of a computer it does not yet have as this manager's, and the full list after the state brings it back.
  Each says the TTL left, which the hub counts on its own clock. A separate event, so a renewal once a minute does not resend the apps' machine list.
- Counts only a computer that gets the "Needs you" summary as it is (see "Needs you" above): its Persona Link takes the summary, it is not waiting to be confirmed and not a work computer.
  While the hub cannot say which computers wait to be confirmed, none counts: the summary freezes then, so a new card would reach neither the computer nor the phone. A work computer never counts.
- Counts only while its link keeps up with the summary: it took one (each change then goes out at once), and no send failed since; only a summary taken again brings it back.
- The presence ends when the link drops or another link of the computer takes over, and when its time runs out.
- Keeps the confirmations on the link they came on: a new link starts with none; only cards the summary that link holds (or the one on its way to it) lists are kept. A confirmation changes no computer's being a desktop.
- **Which cards it leaves to Persona Link.** When it pushes a card, it works out with the summary's own rule whether the card is the one that stands for its agent, notifying, among the listed items (not in More).
  If so, the push waits until one of its Persona Link desktops with `notifies` confirmed that card on its current link, then until the desktops as they are then were reported, and then checks that the same link still is such a desktop with that confirmation; only then does it say `link_covered`.
  A link replaced or stopping meanwhile does not count, and the wait goes on for a confirmation that does. The parent's confirmation is what lets the hub mute the phone at all; the hub then mutes only for a Persona Link desktop it still has a valid record of, and pushes the phone when it has none.
  The whole wait has one deadline, 2 seconds, and ends with the manager: past it, or without a desktop, the push goes to the phone. Another card of the same agent, a card past the cap (also one handed to the user late), and a card the parent did not confirm go to the phone.

**The hub** (`internal/hub/desktops.go`)
- A computer of the sending manager's own, with `notifies`, is a Persona Link desktop for the TTL it says, counted on the hub's clock from when it takes it and at most 4 minutes; a mesh member's computer does not count yet.
- "Phone Muted" while only Persona Link desktops are active holds back a "Needs you" card (the APNs card push and the `interaction` WebSocket alert) only when its push says `link_covered` and one of that manager's Persona Link desktops is active.
  The hub does not judge which cards a Persona Link took over: the manager says. Cards without `link_covered`, "Agent finished", reply reminders, decisions, MCP forms and links, `POST /api/v1/notify` alerts and the Live Activity's start alert still go to the phone. An Electron desktop's `register_desktop` has its own withdrawal rules.
- The deadline is checked when an alert goes out, so a computer whose manager or parent went quiet stops muting the phone once it passes (2 minutes after the parent last spoke); the `desktops` event follows when it lapses, also when a manager's list arrives first and leaves it out.
- The `desktops` event lists a Persona Link desktop as `{"kind":"link","machine_id","device":"","name","since"}`, only to clients that list `desktops_link` (`docs/protocol/protocol.md`); others get the list as before.

**Old and new**
- Old parent: writes neither presence nor confirmations, so a new persona-link reports nothing. New parent with an old persona-link: the old one discards its stdin.
- Old manager: ignores the `presence` and `rang` messages, and its card pushes carry no `link_covered`, so a new hub mutes none of them for a Persona Link desktop. Old hub: ignores `devices.desktops` and `link_covered`. Either way the phone gets the cards.
- The iOS App lists `desktops_link`. Clients that do not (older iOS builds, Android, the web app) do not show Persona Link desktops; a phone with "Phone Muted" on still has the confirmed cards held back while one is active.

**Limits**
- Passive mode: a connector has no way to send a confirmation back within the wait, so a computer in passive mode is no desktop, and the phone gets every card.
- Compatibility presence does not inspect Focus or Do Not Disturb: a notification the parent handed over may stay unseen there while the phone is muted for it.
- The Electron desktop has its own notification presence and is not marked by Link's compatibility `devices.desktops` entry.

## Credentials (active mode)

- A kind of device credential: kind `link`, role `link`, bound to that machine in hub.json (`device_tokens.machine_id`, migration 0131).
  - The hub stores only the hash.
  - Can open only `/api/v1/link` and `/api/v1/link/stream`; every other endpoint returns 403.
  - Visible and revocable in App **Settings › Devices**; revocation immediately disconnects the link (4002).
  - Deleting the computer in the admin console revokes them too, and adding a computer from the admin console, an app or
    `personad hub pair --add` first revokes any left under its id, so an earlier computer of that id does not come back as
    the new one.
- Both kinds of pairing code can be exchanged for it:
  - `personad hub pair --machine <id>`: a code specifically for this computer (`pair_tokens.machine_id`). The link is `persona://link?…&machine=<id>`,
    exchangeable only for this computer's link credentials and nothing else. Stdout contains only the link, explanations go to stderr for easy piping.
    If the computer isn't in hub.json yet, add `--add [--name <name>]`: first adds `kind: link` (effective while running, no restart), already confirmed; without `--add`, it asks you to include it.
  - **Add Computer** in a signed-in App / web (`POST /api/v1/auth/pair-codes` with `add`, [protocol.md](../protocol/protocol.md), "Temporary pairing"): the hub adds `kind: link` just like admin's **Add Computer**,
    with a code for that machine. Computers added this way are **pending confirmation** and don't receive Persona-managed keys until the user taps **This is my computer** in the App ([protocol.md](../protocol/protocol.md), "Computer trust").
  - A normal App pairing code (`personad hub pair`) plus a machine id: CLI `persona-link pair --machine <id>`, or fill in **Machine id** in the App.
- The hub needs TLS (`public.tls_listen`) or an `https://` reverse-proxy `public_url`; without either, `--machine` returns 409.
- Failed authentication follows the hub's existing rate limits (public sources counted). Public connections require TLS; LAN plaintext isn't accepted either, except loopback
  or a LAN reverse proxy reporting HTTPS (`X-Forwarded-Proto: https`; see "[Hub behind a reverse proxy](#hub-behind-a-reverse-proxy)").
- Credentials are stored in `~/.persona/link.json` (0600): hub id, machine id, fingerprint (or `"trust": "system"`: no fingerprint, system-CA validation), credential, candidate addresses (hub-provided plus manually added).

## Connecting a computer (Mac example)

Prefer adding it in the App: no admin interface or sign-in to the hub machine needed. Below is active mode on Mac; for Linux, replace steps 2 and 3 with pasting the web app's one-line command into that computer's terminal;
for Windows see [windows.md](windows.md). Instructions for people who don't use terminals are in [getting-started.md](getting-started.md), "Connect a computer".

1. Install Persona Link on the Mac: `./scripts/app/mac-link.sh install` ([persona-link-install.md](persona-link-install.md)). It builds persona-node and Persona Screen, copies the helpers to `/Applications`, and registers both launchd services. Existing node installations are left untouched.
2. In the signed-in web app, open **Machines › Add Computer**, select Mac and enter a name (Chinese is allowed; the hub generates the id from the name). The hub adds `kind: link`, **pending confirmation**, and provides its pairing link
   (`POST /api/v1/auth/pair-codes` with `add`, [protocol.md](../protocol/protocol.md), "Computer pairing codes"). iOS and Android entry points are in progress.
3. Send the link to this Mac (**Copy pairing link** or **Share…**) and paste it into the Electron desktop's **Persona Link** setup section, or use `~/.persona/bin/persona-link pair -` with the link on stdin.
4. Once it connects, the same web screen asks "A computer wants to join: <name>. Is it yours?"; tap **This is my computer** (`POST /api/v1/machines/{id}/confirm`).
   Before confirmation it receives no Persona-managed keys, isn't considered for assistant-machine selection, and doesn't appear in the computer list for creating Agents; unconfirmed computers are removed after 24 hours ([protocol.md](../protocol/protocol.md), "Computer trust").
   If you closed that screen, its card in the machine list also lets you confirm or delete it.
5. Verify: `persona-link status` shows the connector running and connected, and the computer is online in App **Settings › Machines**.
6. `com.persona.link` starts at login and restarts the connector independently of Electron. No login-item setting in the desktop app controls it.

Links expire after 10 minutes and are single-use; if expired, tap **Regenerate** on the web screen (`{"machine": <id>}` issues a new code and also revokes old Persona Link credentials).
Without hub TLS, the web reports "The hub hasn't enabled a secure connection yet" (409 `tls_required`); see "Advanced" below.

### Advanced

- **Add without the App** (already confirmed): admin **Computers › Add Computer › Computer connects**; or generate a pairing link on the hub machine:
  - Native deployment: `personad hub pair --machine macbook --add --name MacBook` (`--add` does nothing if the computer exists, but still returns a link);
  - Docker on NAS: `sudo /usr/local/bin/docker exec personad /opt/persona/bin/personad hub pair --machine macbook --add`.
- **Hand-edit hub.json**: add a `machines` entry (or change the previous SSH entry) to `{"id":"macbook","name":"MacBook","kind":"link","icon":"laptop"}`, restart the hub, then generate the link with the command above.
- If claude is a shell wrapper function, configure its launcher in `machines[].launchers` in hub.json.
- **CLI pairing**:
  ```sh
  … | "/Applications/Persona Link.app/Contents/Helpers/persona-link" pair -
  ```
- **Passive mode**: use `persona-link pair --listen` (or the desktop's passive pairing action), then paste the link into hub admin **Computers › Add Computer › Hub connects** ("Passive mode" above).

For Linux see "Linux (headless)" below: build `persona-link`, then `persona-link install --pair -` pairs, installs and waits for connection in one command.

## Switching from SSH

A Linux computer the hub already reaches over SSH (`kind: ssh`) can move to Persona Link with one confirmed click (#10662), under
**Switch to Persona Link** on its machine detail in the signed-in web app, on the iPhone / iPad app's advanced computer detail
page (#10671), and on its row in the admin **Computers** page. The computer keeps its id and name, and so its pins, mutes, trust,
settings and history, which are all stored by id. Afterwards it connects to the hub by itself, also from other networks. It is for
people who manage their computers, so it sits with the machine's details, not in the Assistant.

**What it needs**: the hub takes Persona Link (`public.tls_listen` or an `https://` `public_url`, as for pairing). The computer
must also meet all of these:

- It runs Linux on amd64 or arm64.
- It is online over SSH and confirmed as the user's.
- It has a running systemd user manager with linger on (`/var/lib/systemd/linger/<user>`). Without linger the user's services stop
  when the last session ends, and that would be the switch's own SSH session.
- It has no Persona Link yet: no connector, no `link.json`, no `persona-link.service`, and none under the old names.
- It has a node program (`~/.persona/bin/personad` or `persona-node`) or already a node service.
- Its node uses the default `remote_bin` and data directory: no `PERSONA_HOME` in its ssh environment, and no separate
  `~/.jarvis`.

A Mac or Windows computer is not switched from here, because launchd over SSH is unreliable (see "The device's program", step 3)
and Windows has no service install over SSH. Install Persona Link on such a computer itself. Each condition the computer does not
meet shows as a plain reason instead of the button.

**What happens** (the manager that carries the computer runs it, `server/internal/devices/link_switch.go`; the hub only does two
steps, `devices.link_switch.begin` / `end`). The computer disconnects and reconnects once. Agents running in a terminal (herdr and
the other engines) keep running, and their hooks reach the new node. A reply the assistant is writing on that computer at that
moment is cut off, as when a node restarts.

1. **Checking** (30 s): one read-only SSH command looks at the system, systemd and linger, the files above, and which node service
   exists. Nothing changes, and a refusal ends here.
2. **Installing**:
   1. The manager sends the Persona Link build of the hub's version for that platform (from the hub's node builds or its release,
      [node-update.md](node-update.md) "Node packages") over the same SSH access, so the computer downloads nothing. It is staged
      as `~/.persona/bin/.persona-link-switch-<op>`, and its SHA-256 and `version` are checked (10 min).
   2. The hub then checks that the machine's entry is still what the manager saw. It sets `kind: link` and leaves every other field,
      `ssh_host` included. It makes one pairing code for this machine, ten minutes and single-use, which only the manager gets.
   3. From here the manager no longer runs the node over SSH. One more SSH command (3 min) puts the staged file in place as
      `~/.persona/bin/persona-link` (never over an existing one) and runs `persona-link install --pair -`, with the pairing link on
      its stdin. The link never goes on a command line, into a log, an API answer or the manager's store. The installer redeems
      the code, adds the node service `personad-node` only when the computer has none (an existing one is left unchanged), starts
      `persona-link.service`, and waits for the connection.
      Just before the installer runs, the same command checks again for a node service, or the backup of one
      (`node-switch.backup`) that the installer may put back. If it finds one, it leaves a marker so a rollback never removes
      that service. Where `timeout` exists, the installer gets 150 s, so it cannot outlive the SSH command and race a rollback.
3. **Connecting** (90 s): done when the computer's node answers through Persona Link. If the installer failed before it
   paired (no `link.json`), nothing can connect, and the switch fails at once.

**When something fails**, the computer goes back to SSH:

- **Before the hub step**, the manager removes the staged file.
- **After the hub step**, the manager asks the hub to end the switch (`devices.link_switch.end`). The hub revokes this switch's
  code and the credential it was traded for (the link closes with 4002) and puts the kind back to `ssh`.
  - **Until the hub answers**, the manager touches nothing on the computer. Persona Link stays installed, so the computer
    is not left with neither a link nor SSH. The switch shows as rolling back while the manager asks again: after 1 s, then
    twice as long each time, up to every minute, and again after a restart. The asking is bounded to one hour from the
    first try, a limit a restart does not reset. After that the manager stops and leaves the computer as it is. The
    switch ends failed with `hub_unanswered`, saying it could not be undone, and **Redeploy** and automatic updates are
    no longer held. A hub that does not know the call (`unknown_method`, an older build) ends it the same way at once.
  - **When the hub has confirmed**, the manager removes over SSH only what this switch installed: the connector with its
    `link.json` and unit (`persona-link uninstall --purge`), and the node service only if the computer had none before
    (`persona-link node uninstall`). Both run only through the program this switch sent, recognized by its SHA-256, never
    through a `persona-link` found there. Then it waits up to 2 minutes for the computer to answer over SSH again.
- **If the hub refuses to undo it** (it cannot tie the request to this switch), the manager stops asking and leaves Persona
  Link on the computer. If the hub has undone the switch but the computer is another manager's by now, or has been
  removed, it answers `rolled_back_gone`. The manager then removes what this switch installed, as for any confirmed
  rollback, but does not wait for the computer over SSH, since the computer is no longer its own.
- **If any step cannot be confirmed**, the result says the switch could not be undone for sure, rather than that it was.

The reason the switch failed is shown in plain words either way.

**What the hub keeps** (`machine_audit`, action `link_switch`): one row per step of an operation, written before the step
takes effect.

- `begin`: before the kind changes.
- `begun`: once the code exists, with its hash.
- One final row: `succeeded`, `rolled_back` or `begin_failed`.

How the hub uses these rows:

- **Undoing**: an end is accepted only for the computer's latest operation, and its hash must be the one recorded for it.
  A final row is final: a repeated rollback changes nothing, a rollback of a switch that succeeded answers
  `already_succeeded`, and an operation id cannot begin twice.
- **Survives a hub restart**: the rows stand in for the hub's memory. A hub that stops between changing the kind and making
  the code can still be undone from the `begin` row.
- **Bounded**: a switch counts as running for at most 30 minutes after its `begin` row.

**Restarts and repeats**: only one switch per computer runs at a time. While one runs, the hub refuses these with 409, and the
checks hold across a hub restart:

- a second start (`in_progress`);
- **Redeploy** of its node;
- moving the computer to another manager, removing it, or removing its manager (`link_switch_in_progress`). A move or removal
  that is already under way holds the computer, and a new switch is refused until it finishes.

A device signed in with a pairing code it has not traded yet cannot start a switch (403).

The manager writes down each step before it takes it (setting `link_switch.<machine id>`; the pairing link is never kept). A
manager that restarts during a switch carries on as follows:

- it finishes the switch if the computer connects through Persona Link before the deadline;
- it rolls the switch back at once if the installer never ran;
- it goes on asking the hub to end one it was rolling back;
- it goes on telling the hub of a success the hub has not acknowledged yet.

If the manager stops waiting for the hub's `begin`, the hub gives up the begin when it has not taken the computer yet.

**The computer's own files**: the check, the transfer and the install refuse (`unsafe_permissions`) unless the home folder,
`~/.persona` and its `bin` belong to the user, and none of them can be written by other users. A group-writable one is
accepted only when the group is the user's own private group. The message says how to fix it, for example
`chmod go-w ~ ~/.persona ~/.persona/bin`.
After the build is put in place, its SHA-256 is checked once more before it runs.

| Route | Who | Answer |
| --- | --- | --- |
| `POST /api/v1/machines/{mid}/link-switch` | a signed-in full device | 202 `{link_switch}`; 409 `{code}` (a reason above, or `in_progress`); 403 otherwise |
| `POST /api/v1/admin/machines/{mid}/link-switch` | the admin page (session and CSRF) | the same, relayed to the machine's manager |

Progress and the result come with the machine itself (`link_switch` in the machine list, [protocol.md](../protocol/protocol.md)),
so every open client sees each step at once. A finished switch stays shown for an hour.

On iPhone and iPad, the optional field is also the compatibility gate: an older hub or manager sends no field and the page does not
change, and a computer owned by a linked hub is switched there. The confirmation uses the large-text-safe system dialog. One
confirmation creates one app-lifetime attempt per computer, shared across detail-page remounts. **Starting…** is local feedback;
the accepted 202 is held only until the machine list catches up by the hub's `at` clock (or changes when timestamps are absent).
While a phase is running, its expected disconnect does not also show the ordinary offline error or **Reconnect**.

A definite refusal other than 403 reads the machine list once and keeps its localized reason recoverable; 403 explains the full-access requirement without another read. A lost, unreadable, cancelled, 408, 429
or 5xx answer is uncertain: the app reads the list once and never retries automatically. If the unchanged hub state still offers
**Switch to Persona Link**, the owner may tap it and confirm again; a duplicate already running is the hub's ordinary 409 refusal.
**Check Status** only reads the list again. A changed authoritative list clears the notice, and every settled attempt expires after
two minutes as on web. A failed result with `rollback_unconfirmed` never claims the SSH fallback was restored and offers the same
status check.


## Status line

The status line is a machine contract and is not translated. The same applies to `status --json`, `node status --json`, `ssh --test --json`, the saved `link.status.json`, passive HTTP `message` fields, and the pairing URL written by `pair --listen` on stdout. Candidate errors retain the full-width-semicolon delimiter (`；`) that current and older clients split. Human `status` and `install` output may translate a copy of a recognized saved error, but unknown hub, peer, OS and parser details pass through unchanged.

Plain terminal output uses `i18n.Terminal()`: `PERSONA_LANG`, then `LC_ALL`, `LC_MESSAGES` and `LANG`, with English for C, POSIX, an unset locale or an unknown language. The `link-cli` catalogs ship `en`, `zh-Hans` and `ja`; Chinese terminal output retains its previous wording. Maintainer logs are English. An early `persona-link run` failure follows the terminal language when stderr is a terminal, but is English when a service redirects stderr into a log. One pre-existing exception to locale-independent JSON is `service.missing`: the unavailable-systemd reason was already locale-coupled and keeps that behavior for compatibility.

```json
{"state":"connected","machine":"macbook","hub_name":"Home","mesh_hubs":1,"address":"https://192.0.2.10:18662","kind":"lan",
 "since":"…","node":"online","endpoints":[{"url":"https://192.0.2.10:18662","kind":"lan"}]}
```

- `mode`: `active` / `passive`.
- `state`: `unpaired` / `connecting` / `connected` / `waiting` / `refused`; passive mode uses `unpaired` (listening, waiting for pairing), `listening` (paired, hub not connected),
  `connected` (hub has a stream open; `address` is the hub's address and `since` the stream start time), `waiting` (unable to listen).
- Passive mode also includes `listen` (listen address), `addrs` (local addresses usable by the hub, `host:port`, best first), `last_seen` and `from` (time and address of the hub's last visit),
  `code_expires` (expiration of the `pair --listen` code).
- `node`: `online` / `offline` / `unknown`, reason in `node_error`. Each link connection starts at `unknown` ("the hub is connecting")
  until the hub's `node` message; a connection to the node resumed over the new link ([machines.md](../server/machines.md)
  "A dropped connection that resumes") is `online` once resumed.
- `pending` (planned, #4655): `true` means the hub is waiting for the user to confirm this computer in the App; absent means confirmed.
- `waiting` / `refused` include `error` (candidate failures separated by "；", a full-width semicolon) and `retry` (next attempt time); when an SSH tunnel attempt failed, `ssh_error` carries `{code, server, target}` so localized surfaces render the sentence in `en`, `zh-Hans` or `ja`.
- `ssh`: the active-mode SSH tunnel from `link.json` (`{server, target, always?}`), when configured; while connected through the tunnel, `kind` is `ssh` and `address` is `<server>`.
- `proxy`: while connected through an outbound proxy (`kind` is `proxy`, `address` the hub URL reached through it), the proxy's URL without its user name and password (see "Outbound proxy").
- `hub_name`, `mesh_hubs`: see "Hub name and linked group"; absent with old hubs.
- `machine_name`: what the hub calls this computer (see "The computer's name"); absent with old hubs. `rename`: how the last rename asked of this persona-link went: `{id, name}` or `{id, code, error}`.
- `pending`: meaningful only when `connected`; `true` means the hub is waiting for the user to confirm this computer (see "Pending confirmation"); absent or `false` means confirmed.
- `trust`: `system` when active mode validates the hub through system CAs (reverse proxy, no fingerprint); absent when pinned.
- `conn`: active mode while `connected`: which link it is, a number that rises with each link (across mode switches) and that a restarted persona-link does not reuse (it starts from the time it started). Compatibility parent confirmations name it (see "Presence"); absent otherwise and with old helpers.
- `attention`: the hub's latest "Needs you" summary, as it sent it; the full shape is in "Needs you". In short: `needs_you` (the count), `items` (at most 20,
  newest first, each with `machine_id`, `session_id`, `title`, `place`, `kind`, `detail`, `muted`, `notify`, `route`, and `interaction_id`, `since` when known), `more` (the rest),
  `web_url` (where `route` opens; in active mode the manager already fell back to the hub address when the hub has no https `public_url`, so the App needs no fallback of its own;
  absent in passive mode without one) and `rev` (the same summary has the same `rev`). Notify only items with `notify: true` (an item without it does not notify), also one listed before without it;
  `muted` only shows an item muted.
  Absent before the first summary, with old hubs, when the link drops or reconnects, and in passive mode once the hub stopped coming in for 2 minutes. A computer that may not have it
  (waiting to be confirmed, or a work computer) has an empty one (`needs_you: 0`, no items).
- No credentials included.
- Each line is also written to `~/.persona/link.status.json` (with `pid`, `version`, `updated`; normal exit writes `"state":"stopped"`) for
  `persona-link status` to read, since it can't query the running instance.

## Linux (headless)

On servers, NAS boxes and Raspberry Pis without a GUI, `persona-link` alone is enough.

**Build**
- Pure Go, no cgo: `cd server && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o persona-link ./cmd/persona-link`.
  Builds for `amd64`, `arm64` and 32-bit Raspberry Pi (`GOARCH=arm GOARM=7`); Windows also builds (file locks and signals in platform-specific files); the scheduled task works without a tray ([windows.md](windows.md), "Install and start at logon").
- `personad` also builds for Windows (`GOOS=windows go build ./...`; process groups, signals, file locks and CPU time are platform-specific in `internal/osproc` and each package's `_windows.go`),
  and the Windows resident node works with Orca; `persona-link install` installs a sign-in scheduled task on Windows (the Windows paragraph under "Install the node service"), see [windows.md](windows.md).
- The resident node runs the device's program, persona-node, built from the same commit (`~/.persona/bin/persona-node`; personad where the computer has no persona-node or runs a hub or a manager, "The device's program"); `scripts/deploy/node-launchd.sh install` installs a personad node on Linux as systemd user service `personad-node`.

**One-command installation**

Get the link from signed-in web **Machines › Add Computer** (requires confirmation on the web), or generate it on the hub machine via CLI (already confirmed):

```sh
personad hub pair --machine nas --add             # On hub machine; adds it if absent; stdout contains only the link
./persona-link install --pair - <<< '<link>'       # On this computer; reads link from stdin
```

Passive mode: `./persona-link install --listen [:18663] [--addr <address the hub uses to reach this computer>]`; after installation, prints a link for hub admin.

`install` runs these steps:
1. With `--pair`, pair first (same as `persona-link pair`; no link after `--pair`, or `-`, reads stdin). With `--listen`, switch to passive mode and generate a link if never paired
   (same as `pair --listen`); an existing pairing stays unchanged. These options can't be combined.
2. Copy itself to `~/.persona/bin/persona-link`, unless already there. Running `install` again with a new version upgrades it.
3. If absent, install and start the resident node service (systemd user service `personad-node`, same contents as `scripts/deploy/node-launchd.sh`): runs `~/.persona/bin/persona-node`, else `~/.persona/bin/personad` (the install script puts one there first),
   or `--node-bin <path>` / `--personad <path>` to copy a supplied binary. Existing installations stay unchanged; `--no-node` leaves it alone.
4. Write `~/.config/systemd/user/persona-link.service` (under `XDG_CONFIG_HOME` if set), then `daemon-reload`, `enable`, `restart`.
5. If linger is off, suggest `sudo loginctl enable-linger <user>`: otherwise the user instance is absent when nobody is signed in, services stop at logout and don't start at boot.
6. Wait up to 20 seconds for hub connection and the hub's local-node check, then print a one-line conclusion. Connected: exit 0; not connected: print every candidate's failure reason, exit 1, leaving the service installed and retrying.
   Passive mode waits for listening (or explains why it can't), then prints the pairing link and firewall guidance.

The unit contains neither mode nor listen address: both live in link.json. Changing mode or port requires no service reinstall; the running persona-link switches within a second or two.

Running `install` before pairing starts the service as unpaired; later `persona-link pair` makes it adopt new credentials within seconds, without restart (checks link.json changes every 2 seconds, also while connected, **Reconnect** above).

Generated unit:

```ini
[Unit]
Description=Persona link: keeps this computer reachable from the Persona hub

[Service]
ExecStart="/home/<user>/.persona/bin/persona-link" run --log "/home/<user>/.persona/link.log"
Environment="PERSONA_HOME=/home/<user>/.persona"
Environment="PERSONA_HOME=/home/<user>/.persona"
Restart=always
RestartSec=5
StandardOutput=null

[Install]
WantedBy=default.target
```

- Paths use this computer's data directory (`~/.persona` for new installs, `~/.persona` on machines using an old version, or `PERSONA_HOME` if set; see [rename.md](rename.md)); both `Environment` lines pass the same directory to the service so it doesn't resolve another.
- Doesn't wait for `network-online.target`: user instances don't have this target, and persona-link retries itself.
- Status JSON lines are not needed by the service; logs go to `~/.persona/link.log` (`run --log`; at 1 MB, rotates to `link.log.1`, a configured value).
  On machines without persistent journals, ordinary users can't see user-service logs with `journalctl --user`, so logging doesn't depend on it.
- Sessions entered through `su`, `sudo -u`, `docker exec` lack `XDG_RUNTIME_DIR`, so `systemctl --user` can't find the user instance; install / status / uninstall
  fill in `/run/user/<uid>` if unset and present. This doesn't help if the user instance itself is absent (no linger and nobody signed in); prompts to enable linger or SSH in as that user before installing.
- Installing as root warns that the service runs as root with root's data directory, and the resident node must also run as root (the prompt hard-codes `/root/.persona`, but the actual directory is resolved by the rules above).

**Without systemd** (containers, some NAS systems)

`install` still pairs and copies binaries, then prints copyable alternatives and exits 1:
- Run in the background now: `nohup ~/.persona/bin/persona-link run --log ~/.persona/link.log >/dev/null 2>&1 &`
- At boot: cron `@reboot` with the same command;
- Synology: a user script triggered at startup in Task Scheduler, containing the nohup line above;
- Container: a small supervisor script as the main process (under `docker run --init`) that runs `persona-link run --log …` and `personad node --attach --supervised` and starts each again when it exits; restart policy `unless-stopped`, `~/.persona` mounted as a volume ([lessons/server.md](../lessons/server.md) "A node service in a Docker container on the hub's own NAS");
- supervisord, runit, s6, OpenRC: run `persona-link run --log …` in foreground and restart on exit, and the resident node the same way as `personad node --attach --supervised`.

Resident node likewise: `nohup ~/.persona/bin/personad node --attach >> ~/.persona/node-attach.log 2>&1 &`.

Two things to know about a node without systemd:
- **The hub updates it only when its supervisor says so.** systemd and launchd are recognized by the environment they set; anything else
  that starts the node again whenever it exits declares itself with `node --attach --supervised` (personad or persona-node; Linux and macOS).
  The machine then shows `can_deploy: true`, and Redeploy and the hub's node updates reach it as they reach any other computer
  ([node-update.md](node-update.md)). nohup, cron `@reboot` and Synology's Task Scheduler restart nothing: leave the flag out there. Such a
  node shows `can_deploy: false` ("not managed by launchd/systemd") and is upgraded by hand: replace `~/.persona/bin/personad` with the hub's
  own build for that platform (same version as the hub) and restart the node process. What the supervisor has to do:
  - Start the node again, with the same command line, whenever it exits. A normal update doesn't need it: the node runs the new program in
    place (exec, same process id), so the supervisor sees nothing. It matters when the new version crashes, or never confirms with the hub
    within three minutes: the rollback watchdog then puts the old file back and stops the node, and the restart runs the old program.
  - Stop and restart only the node's own process. The watchdog runs from the old program in a session of its own, outside the node's process
    group, so killing the group or the session leaves it alone; killing every process of the user or the container doesn't (a container
    stop is fine: on the next start the node finds the unfinished update and starts another watchdog).
  - Have a `ps` that reports a process's start time (`ps -p <pid> -o lstart=`; procps on Linux, macOS's own). BusyBox's can't: the machine
    then says "ps here cannot report a process’s start time…" instead of offering Redeploy. Install procps.
  - Keep the folder holding the node's program (the real file behind `~/.persona/bin/personad`) writable by the node's user: the new file is
    staged and swapped there, next to a backup of the old one.

  As on any other computer, only personad is updated: persona-link, herdr and the Agents' CLIs keep their versions. herdr's server runs in a
  session of its own and keeps its panes when the node restarts. Windows nodes still can't be updated by the hub ([windows.md](windows.md)).
- A container on the hub's own machine may not reach the hub at the hub's LAN address or public name (a NAS's policy routing sends the
  replies elsewhere; a default bridge network has no IPv6). Its bridge's gateway does reach it: pair with the link's `url` host replaced by the
  gateway address, and add `https://<gateway>:<TLS port>` with `persona-link address add` so it stays first ([lessons/server.md](../lessons/server.md)
  "A node service in a Docker container on the hub's own NAS").

**Status, logs, uninstall**

| Command | Action |
| --- | --- |
| `persona-link status` | Mode, link (address, duration; candidate failure reasons when disconnected), hub name and group size, local node, this computer's name on the hub (its id in parentheses when they differ), candidate addresses, systemd service (active / enabled / linger), pid and version, log location. Passive mode instead shows: listen address, listening or hub connected, paired hub and pairing time, hub's last visit time and address, helper fingerprint, local addresses usable by hub. Exit 0 when connected (also listening in passive mode), otherwise 1 |
| `persona-link status --json` | Same for scripts: `running`, `mode`, `paired`, `machine`, `machine_name`, `hub_id`, `hub_name`, `endpoints`, `ssh`, `trust` (`pin` / `system`), `link` (last reported status), `passive` (`listen`, `paired`, `hub_id`, `hub_name`, `machine`, `machine_name`, `paired_at`, `code_expires`, `fp`), `service`, `config`, `log`; `error` if link.json can't be read; no credential or hash |
| `persona-link logs [-f] [-n lines]` | Last 100 lines of `~/.persona/link.log` (change with `-n` / `--lines`); `-f` / `--follow` keeps following, including rotation; without a log file but with an installed service, reads journal. `log` is the same command |
| `persona-link uninstall` | Stop, disable and delete systemd user service (Windows: scheduled task), preserving credentials. Linux / Mac resident-node service remains (`node uninstall` removes it); on Windows the node shares the task and stops too, but `personad.exe` remains |
| `persona-link uninstall --purge` | Also remove credentials (link.json and `.lock` / `.run`), status, logs (including `link.log.1`), passive TLS identity `link-tls/`, `~/.persona/bin/persona-link`, and install-script `persona-link` alias (only if truly an alias); refuses while another persona-link is running (a legacy parent, manually started). Revoke hub-side credentials in App **Settings › Devices** |
| `persona-link pair [--machine <id>] [--ssh <server> [--ssh-target <host:port>] [--ssh-always \| --no-ssh-always]] [<link>\|-]` | Active pairing, credentials (and optional SSH tunnel) saved to `~/.persona/link.json`. No link or `-` reads stdin; prompts to paste in a terminal and reads one line on Enter. If the link lacks a machine id (normal App code), specify `--machine` |
| `persona-link ssh [<server> \| --off] [--target <host:port>] [--always \| --no-always] [--test [--link <link>]] [--json]` | Show, configure, test or remove (`--off`) the active-mode SSH tunnel in `link.json` without re-pairing; see "[SSH tunnel](#ssh-tunnel)" |
| `persona-link pair --listen [<listen address>] [--addr <address>]…` | Passive pairing link (only link on stdout, for piping); default listen `:18663`; can't combine with pairing link or `--machine` |
| `persona-link mode [active\|passive] [--listen <address>]` | No arguments: display current mode (with passive listen address); supplied mode: switch, retaining both pairings; `--listen` alone changes passive listen address without switching mode |
| `persona-link node install [--node-bin <path> \| --personad <path>]` / `node switch [--to personad]` / `node status [--json]` / `node uninstall` | Resident node service (Linux systemd `personad-node`, Mac launchd `com.persona.node`); a new one runs `~/.persona/bin/persona-node` when there is one (else personad, and always on a computer that runs a hub or a manager), existing services stay unchanged; `--node-bin` puts the given program in under its own name. `switch` moves an existing service to persona-node (`--to personad`: back), whole or not at all ("The device's program" above). `uninstall` deletes neither program. On Windows the node shares persona-link's task; only `node status` exists |
| `persona-link discover [--wait <seconds>] [--json]` | Hubs heard on the LAN (mDNS), viewing only, no pairing; listens 3 seconds by default, `--wait` allows decimals up to 60; exit 0 if found, otherwise 1; any LAN host can advertise a name, so the text output shows control characters as `<ESC>` / `<U+XXXX>`, one line per hub (`--json` is for scripts and keeps the name as heard: JSON escapes C0 control characters, ESC included, but writes DEL and C1 characters U+0080-U+009F raw, so don't print it on a terminal) |
| `persona-link run [--parent] [--log <file>]` | Maintain the link in foreground, one JSON status per stdout line (see "Status line"); `--parent` exits on stdin close (compatibility with older parent integrations); `--log` writes there (rotates to `<file>.1` at 1 MB), not stderr. Used by systemd and the alternatives under "Without systemd" |
| `persona-link address add <address>` / `address remove <address>` | Manually add / remove a hub candidate (`https://domain:port`, or just domain), stored in link.json; pairing required before adding. `remove` deletes that entry, manual or hub-provided (the hub may send it again on next connect) |
| `persona-link name [<new>\|--reset]` | No argument: what the hub calls this computer (with the id when they differ). A name: asks the running, connected persona-link to have the hub rename this computer and prints the hub's answer; `--reset`: back to the id. Exit 1 with the reason when it can't (not running, offline, passive mode, a hub that doesn't rename computers, not confirmed yet, too long); see "The computer's name" |
| `persona-link version` | Print version (`--version`, `-v` equivalent) |
| `persona-link help` | Print usage (`--help`, `-h` equivalent); unknown commands / options also print usage and exit 2 |

`status` uses:
- Running: whether the file lock `~/.persona/link.json.run` is held (regardless of who started it: a platform service, legacy parent or manual invocation). It briefly acquires the lock to check;
  a persona-link starting at that exact moment exits as "already running", then systemd starts it again after 5 seconds.
- Link state: `~/.persona/link.status.json` (see "Status line"); when not running, shows the last report and time.
- **Does not connect to the resident node socket** to check liveness: every connection is a new attach and would replace the hub's current connection. Local-node status comes only from the hub.
- On Mac the launchd agent writes the same status file, and `status` also reports whether `com.persona.link` is installed and loaded. The Electron setup reads only the allowlisted status fields, never the credential file.

`scripts/deploy/link-systemd.sh` remains a thin wrapper: `install` / `uninstall` / `status` delegate to `~/.persona/bin/persona-link`; `restart` runs `systemctl --user restart`.

**Verification** (2026-11-02): a clean Debian 12 container (arm64) in local Docker, with a test hub on the host listening only on 127.0.0.1; the container connects through
`host.docker.internal` to its TLS port. Everything was stopped afterwards.
- Without systemd: `install --pair -` pairs, copies and prints alternatives (exit 1); after the suggested `nohup … run --log …`, `status` shows connected and
  `logs` contains logs; starting a resident node makes the local node online and the hub machine list shows this computer online.
- With systemd (`--privileged --cgroupns=host`, image needs `libpam-systemd` or the user instance won't start; `loginctl enable-linger`):
  enter with `docker exec -u` (no `XDG_RUNTIME_DIR`), install and connect with one `install --pair -`; after `scripts/deploy/node-launchd.sh install`, local node is online;
  container restart (equivalent to boot) reconnects automatically; disabling linger makes `status` suggest enabling it; another install (upgrade), install before pair,
  uninstall and uninstall --purge all behave as expected.
- amd64 version ran version and status in an emulated amd64 container. Not tested on a real Raspberry Pi / NAS.

## Use it away from home too

The link uses only the hub's TLS port. Away-from-home use also needs:
- DSM firewall allows 18662/tcp (not 18660).
- Router allows IPv6 inbound to NAS.
- Domain in hub.json `public.advertise` (DDNS points to NAS IPv6). After one connection the hub sends it to persona-link; it can also be manually added in the App.

IPv4-only networks can't reach a home IPv6 address, just as with the phone.

## Troubleshooting

| Symptom | Action |
| --- | --- |
| Passive: "Can't listen on :18663: port in use" | Another program owns it: change port with `persona-link mode passive --listen :18664` (then edit this computer's address in hub admin), or stop that program |
| Passive: hub can't connect, and "Last hub connection" in `status` never changes | Firewall / router doesn't allow the port; hub can't reach the link addresses (edit address or proxy in admin); fingerprint mismatch (deleted `link-tls/`: re-pair) |
| Passive: hub reports 429 | Too many failed authentications from that source (usually retrying old credentials); wait 15 minutes or restart persona-link to clear counters |
| `pair --listen` finds no address | Only loopback, tunnel or temporary addresses: use `--addr <domain or address the hub can reach>` |
| Link connected, but the hub cannot reach the local node | Resident node isn't running: `launchctl print gui/$(id -u)/com.persona.node`; log at `~/.persona/node-attach.log` |
| Mac: sessions started from the phone never come up (Claude stays at "Loading commands and agents", no hook fires) | Run `personad node doctor`: `launchd-process-type` (launchd throttling the node; `--fix` rewrites the agent and reloads it) and `path-access` (a working directory, setting or skill link in Desktop / Documents / Downloads; grant Full Disk Access to the node binary, or move it out) (docs/server/server.md "Scheduling and privacy on the Mac") |
| "Hub rejects this computer's credentials" | Credentials revoked or hub database changed: re-pair |
| Reverse proxy: "Certificate not trusted by the system" / "Certificate hostname doesn't match" | Proxy certificate isn't public-CA signed, expired, or doesn't match `public_url`: fix certificate or `public_url`, then re-pair |
| Reverse proxy: hub returns HTTP 403 | Hub logs show `tls_required`: proxy on another machine lacks `X-Forwarded-Proto: https`, or hub treats its address as public (container gateway, public address); see "[Hub behind a reverse proxy](#hub-behind-a-reverse-proxy)" |
| Reverse proxy: HTTP 404 / 400, or immediate disconnect | Prefix not stripped, WebSocket not forwarded (`Upgrade` / `Connection` headers), or `proxy_read_timeout` too short |
| "This computer opened another link elsewhere" | Same link.json copied to another computer, or two persona-link instances running |
| LAN candidates "unreachable" though network works | Check local-network permission |
| Linux: `install` says "Can't reach your systemd user instance" | Entered through `su` / `sudo -u` / `docker exec` without linger: SSH directly as this user, or first `sudo loginctl enable-linger <user>` |
| Linux: link disconnects at logout and doesn't connect after reboot | Enable linger: `sudo loginctl enable-linger <user>` (`persona-link status`'s background-service line explains) |
| Linux: link connected, local node offline | Resident node isn't running: `systemctl --user status personad-node`; log at `~/.persona/node-attach.log` |

Logs:
- Helper / Linux service: `~/.persona/link.log` (`persona-link logs -f`); passive mode uses `passive: …` (listener start, pairing, stream start / end, rejected requests);
- Hub: `[<machine>] link up …` / `link down …`.


### Redacted connection diagnostics

The existing Link logger emits one `link: dial` record per attempted WebSocket handshake,
including active control candidates, SSH tunnel handshakes, direct recovery probes and node
streams. `attempt` is a process-local sequence, not a persistent identifier. `role` distinguishes
`control` from `stream`; `candidate` is the one-based position in the ranked, deduplicated
control candidates, the tunnel bind attempt, or 1 for the selected stream route. `kind` is
`lan`, `ipv6`, `ipv4`, `name`, `ssh`, `proxy` or `other`; unrecognized input is never echoed.

`phase` is the last observed boundary (`dial`, `dns`, `tcp`, `tls`, `upgrade`). `outcome` is
`success`, `error`, `timeout`, `canceled` or `http_refused`. The phase distinguishes a TLS
stall from an Upgrade stall; an RPC pong fix does not explain either automatically.
`dns_us`, `tcp_us`, `tls_us`, `upgrade_us` and `total_us` are monotonic elapsed microseconds.
Zero means no observed work, reuse, or less than a microsecond; a literal IP needs no DNS.
On a proxy route, TCP time measures the connection to the proxy. TLS time measures the Hub handshake inside CONNECT.
For SSH, TCP time measures connecting to the local forwarding listener, not a public TCP
connection or the SSH authentication duration. TCP time sums observed address attempts,
including overlapping address-family attempts,
so phase totals need not equal wall-clock total. Upgrade time starts when the checked
connection is handed to HTTP and includes writing the request and validating the response.
Canceled phases include their observed elapsed time. A losing raced candidate can emit its
cancellation after the winning candidate succeeds; attempt numbers need not appear in order.

Each existing retry wait emits `link: retry_wait` with `planned_us`, `actual_us`, and `wake`:
`timer`, `canceled`, `network_check`, `redial` or `config_changed`. The actual duration includes
scheduling delay; a configuration change is observed by the existing poll interval. These
records do not change candidate racing, TLS verification, timeouts, backoff, heartbeat,
queues or offline detection, and introduce no new setting.

These new records contain no endpoint addresses, stream IDs, credentials, certificates,
HTTP headers, response bodies or raw errors. Existing status and legacy error logs keep
their established behavior; this does not make an entire historical log safe to share.
The diagnostics cover outbound Link WebSocket handshakes; pairing, passive listener
acceptance and SSH process startup failures retain their existing diagnostics.
