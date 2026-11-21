# Machine monitoring

Each computer's OS, current load and curves for the past 24 hours. The node collects, the manager (the one connecting to computers; see "Who connects to computers" below) stores in Postgres, and the hub forwards App requests to it. The App shows these on **Machines** and machine details alongside Agent counts.

| Part | Code |
| --- | --- |
| Collection (parsing /proc, vm_stat, iostat, sw_vers, Win32 responses, etc.) | `server/internal/sysmon/` (`sysmon.go`, `winparse.go` pure parsing; `linux.go` / `darwin.go` / `windows.go` read the OS) |
| Node sending | `server/internal/node/metrics.go` |
| Manager storage, downsampling, cleanup, API (hub forwards `devicewire.MetricsRoutes`) | `server/internal/devices/metrics.go`, `server/internal/db/metrics.go`, migration `0051_machine_metrics.sql` |
| Token usage: node metering, manager storage, API (see "Token usage" below) | `server/internal/tokenuse/`, `server/internal/node/usage.go`, `server/internal/node/usage_adapters.go`, `server/internal/devices/usage.go`, `server/internal/db/usage.go`, migration `manager/0421_machine_usage.sql` |
| App | `apple/Persona/Views/Machines/` (`MachinesView`, `MachineDetailView`, `MetricCharts`, `OSBadge`), model `apple/Packages/PersonaWire/Sources/PersonaWire/MachineMetrics.swift` |
| Inspect local samples / overhead | `personad sysmon [n]`: prints system info and n+1 samples (one every 10 seconds, n ≤ 8640, one day); final line gives actual sample count, collection duration and personad CPU (Ctrl-C stops immediately) |

For node `session.send` and `codex.answer`, the devices manager supplies the optional `prompt_language` native
language name from the owner locale. It replaces caller-provided variants and affects only generated Agent prompt
wrappers. Missing language preserves legacy Chinese; old nodes ignore the field. It neither supplies nor changes
owner provenance or permission mode. Details: `docs/assistant/prompts.md` ("Agent prompt migration (#6268)").

## Who connects to computers

Product decision: **computers belong to the manager, which connects to them itself**; the hub sees no computers directly, so the bus has nothing to gate between computers and manager (physical separation). During migration there is only one manager, the chief manager (assistant process `personad assistant`), holding all computers.

| Part | Code |
| --- | --- |
| Manager: connections, per-computer state, routing, Persona Link protocol, kind service, tailcat, warm start, engines, herdr auto-start, Claude config directories, permanently listed sessions, node updates | `server/internal/devices/` (`devices.go` entry/reporting, `machine.go`, `conn.go`, `paths.go`, `link.go`, `relay.go`, `service.go`, `tailcat.go`, `warm.go`, `engine.go`, `herdr.go`, `claudedirs.go`, `deploy*.go`, `bus.go`) |
| Shared messages | `server/internal/devicewire/` (method names, parameters, errors, manager-owned route table `Routes`) |
| Hub: forwarding, retained snapshot, its own on-connect work | `server/internal/hub/devices_link.go`, `devices_relay.go`, `machines.go`, `machine.go` (`Machine` is now the manager's report plus hub-owned data) |
| Wiring in the assistant process | `server/internal/assistant/main.go`, `devices.go` (several places in `bus.go` only call it) |

**Manager responsibilities** (`internal/devices`; neither it nor hub imports the other):

- Connect each entry in hub.json `machines`: the hub machine's own node (`node --stdio` child, now the manager's child), SSH, tailcat (hub's tailcat key file, same HOME), kind service (dial the helper; credentials still encrypted by the hub with its key, retrieved over the bus via `devices.connector_cred`, returned for storage after pairing via `devices.connector_paired`),
  kind link (Persona Link still connects to the hub address, forwarded unchanged to the manager; below), and computers reached through member hubs in the linked group (routing, de-duplication and path changes in manager `paths.go`; the hub opens and relays streams through another hub).
- Per-computer state: online, `node.info`, session list and revs, sessions watched by Apps (repeat `session.watch` after node reconnection, resuming from the last item), warm start, engine (settings `machine_engines`), herdr auto-start, Claude config directories (and blocked ones),
  sessions the node must list regardless of inactivity (assistant profile `keep`; when the assistant snapshot arrives, tell each node, `SetKeep`), node updates (`node.deploy.<id>`), node `state.put` / `state.load` (table `node_sessions`).
  A `state.put` or `claude.dirs` write runs on that node connection's read loop, so its store calls get at most 10 seconds (`nodeStoreWait`); then the write is logged and dropped. A `claude.dirs` can wait longer for a `PUT …/claude-dirs/blocked` running at the same time, which holds the same lock for its own store calls.
  Nothing retries a dropped write: the node puts a session's state only when it changes, not on reconnect (`state.load` copies the hub's rows to the node, not back), so a node restarted before then (after an update, say) loses that session's record (launcher, cwd, pane, whether Persona started it); it reports its Claude config directories again only when it sees one it had not seen since it started, or one is unblocked.
  Existing keys and tables stay; no data migration.
- Manager-owned routes (`devicewire.Routes`, forwarded by hub as `http.request` like assistant routes): `POST …/reconnect`, `PUT …/engine`, `GET/PUT …/herdr`, `POST …/herdr/start`, `POST …/deploy`, `GET …/claude-dirs`, `PUT …/claude-dirs/blocked`; also subscription quota `GET /api/v1/usage/windows` (`devicewire.UsageRoutes`) and admin quota-source page (`AdminUsageRoutes`, forwarded only from admin, `docs/server/usage-windows.md`, "Who owns this").
- Node requests: answers `state.load`, assistant requests (`assistant.model`, `assistant.context`) and computer-event requests (`interaction.create`, `assistant.lark`, `lark.sent`, `mcp.tools`, `mcp.call`; "Who handles computer events" below) in-process; passes the rest (`notify`: `personad notify` on computers) to hub (`devices.request`).
  If that node connection closes before the hub answers, the manager sends `devices.request_end`, so the hub stops waiting.
  One-way node events are first processed by the manager (`devices.Options.Event`), then App-needed events forwarded unchanged in order to the hub (`devices.event`): items, screen, live state, terminal frames, port bytes, `sessions.revs`, `hook.event`, `fs.changed`, plugin events; session lists, interactions, usage and monitoring samples stop at the manager.
- One-way hub-to-node events (`devices.notify`) queue per computer, with independent writers; a slow computer delays only itself, not the bus or others. Reconnect if a write stalls 30 seconds or queued data exceeds 32 MiB.
- When a machine entry changes in hub.json (hub restarts after config edits while manager stays running), create a fresh machine object and reconnect (rewatch App-watched sessions), rather than editing config underneath an active connection.
- Assistant calls to machines (node methods in `device.call`) run directly in-process, no hub round trip; hub-owned methods (`devices.list` including phone online state, send-once `session.send_once`) still ask hub. Since 2026-11-05 the manager answers `interactions.list`, `sessions.list`, `session.get`, channel `session.follow` / `session.unfollow` itself ("Who handles computer events"). Sending "continue" for Agent triage, starting Agents for the assistant and opening scheduled-task chats all call the node directly in-process.

**Hub responsibilities**:

- Hand hub.json `machines` (plus computers added live through admin) to the manager as defaults: `devices` in handshake response, then `devices.config` on changes (computer added, linked group changed, hub addresses or name changed).
- Keep the manager's report (`devices.state`: per-computer snapshots with monitoring `system`, `node.info`, connection state/start, warm deadline and paths), list them to Apps in its order, then add manager extensions' `computer` / `browser` reports (Cua, ego plugin state, `extensions.report`, `docs/extensions/extensions.md`, "Who owns what").
- App machine calls and streams still pass through hub: `devices.call` (deadline included), node-bound one-way events (terminal input, forwarded port bytes) via `devices.notify`, node events (items, terminal frames, port bytes…) via `devices.event`,
  then pushed to subscribed Apps. Interactions, state and pushes belong to the manager ("Who handles computer events"). Thus terminal, port forwarding, screen and downloads all use this one bus connection, no extra channel.
  The manager deadline is shortened by one tenth, at most 500 milliseconds, so its timeout arrives first; calls without deadlines (plugin `nodes.call`) aren't given one. Failures after dispatch—node disconnect, deadline or bus disconnect—mean "may have executed":
  message results are `unknown`, and the same message isn't resent (same as when hub directly connected to node); only definite non-dispatch (offline, manager absent) counts as not sent.
- Hub self-update confirmation that previously online computers all reconnected, and subsequent node updates through the manager, use reported `connected` (actual node connection), not App-visible online (also true during warm start after manager restart).
- Every new manager-hub handshake ends both sides of forwarded streams (`port.dial`, `plugin.node.dial`, `preview.open`) and terminals opened through the previous connection: hub closes its side (terminal ends with "Can’t reach the assistant right now. Try again later", reopened by App), manager closes node-side `stream.close` / `terminal.unwatch`, avoiding abandoned port connections, preview browser tabs and terminal frames after hub restart.
  Hub asks the new connection to rewatch App-visible sessions, at most 8 concurrently; a slow computer doesn't block others.
- On node connection, the manager first asks the hub for its part (`devices.connected`): additions to `node.configure` (Codex settings) and immediately subsequent sends (managed launchers, remembered messages), then asks for the session list. Hub also starts its own on-connect work (monitoring system info); Cua / ego state, extension programs on computers, connectors and extension Agent filtering added to `node.configure` belong to manager extensions (2026-11-05, `docs/extensions/extensions.md`, "Who owns what"), even while hub is away.
  Every new manager-hub connection repeats this step for each connected computer.
- When a machine connection ends (reported `connected` false or `connected_at` changes), hub ends its terminal streams and generating text; manager ends pending non-resumable interactions (`devices.Options.Detached`).

**Stream forwarding**: App ↔ hub ↔ manager ↔ node; hub-manager uses the bus over local loopback. Measured (isolated hub, hub's own computer, 32-byte round trips over a forwarded port, 300 times, 2026-11-04): master median 0.3–1.2 ms, p95 about 1.5 ms; this version median 0.9–1.4 ms, p95 about 1.9 ms, adding about 0.2–0.4 ms (one loopback hop and encode/decode).
Terminal frames use the same path (node event → manager → hub → App; input in reverse).

**Persona Link (active mode)**: still connects to the hub address with unchanged pairing codes and link credentials. Hub authenticates (kind-link machine, TLS), then forwards the WebSocket unchanged:
`devices.accept {id, kind: link, machine_id, token_id}`; the manager dials `GET /api/v1/bus/relay?id=<id>` with its bus token; hub forwards messages individually (text: Persona Link messages, binary: node bytes), preserving close codes and reasons.
hello, `endpoints`, `open`, `node` come from the manager; Persona Link streams (`/api/v1/link/stream`) are likewise forwarded. Heartbeat ping runs between hub and Persona Link; missing pong closes both sides and passes the reason to the manager. Interval and timeout are configurable in hub.json `link`, and the manager tells Persona Link in `endpoints` ([link.md](../operations/link.md), "Heartbeat").
Credential revocation closes the relayed link with 4002; absent manager returns 503 on `/api/v1/link`, with normal Persona Link backoff and automatic reconnection when the manager returns.

**Linked hubs (mesh)**: groups, primary hub, records, credential handoff and member App forwarding remain hub-owned (`docs/server/mesh.md`); computers and paths belong to the manager. Hub provides group state in `devices.config.mesh` (whether this hub is a member, primary name, startup wait, other hubs' reachability and distance, member computers),
which the manager uses for de-duplication, routing and path changes. The primary hub opens the member route (`GET /api/v1/bus/relay?hub=<member>&machine=<id>`, opens a node stream over mesh and relays to manager); a member receiving the primary's node-stream request (`/api/v1/peer/node`) delegates to its own manager (`devices.accept` kind node).
Only after the manager opens the node does the member return `101`; failure returns 502 with the reason, displayed on that machine by the primary (as when members opened nodes themselves).
Persona Link RTT comes from hub heartbeats, reported to manager (`devices.link_rtt`) and used for route distances and records.
Reachability and distance of the hub's own computers in its records come from manager reports (`entries` in `devices.state`).

**While the manager is away** (restarting or crashed):

- All of that manager's machines show offline (another manager's stay as it reports them), `error` is "Can’t reach the assistant right now. Try again later", sessions become unknown (hub keeps the last list, sessions with pending cards stay **Needs you**); terminal streams end with the same message and forwarded ports close. Hub keeps listing the last cards, but answers fail (`503 assistant_offline`).
- Manager-dependent endpoints (items, send, create, files, git, terminal, ports, manager-owned routes…) return `503 {"code":"assistant_offline"}`; `GET /api/v1/machines` and `GET /api/v1/sessions` return hub's retained snapshot normally.
- When the manager returns (hub restarts it with backoff beginning at 1 second), it reconnects every computer; hub requests rewatch of App-visible sessions automatically. Persona Link reconnects on its next retry.
  Calls to a machine still connecting after startup (hub-held card answers, new App Agents) wait for its first connection attempt to finish (up to 20 seconds), rather than immediately returning offline.
- No new Agents, sending messages or observing Agent turn completion while manager is absent: all need its node connection (previously hub connected computers itself, so Agents could still start without assistant).
- A resident node keeps what its Agents' hooks and `personad notify` send meanwhile, and permission / question hooks wait up to two minutes for the manager instead of dropping to the terminal; the next connection replays it in order (`../protocol/protocol.md` ("When the manager is away")).

**Bus sizes**: other messages, including heartbeats, wait until a hub-manager message finishes writing, so large payloads are chunked: downloads, uploads and node update packages ≤ 1 MiB per chunk, forwarded port bytes 32 KB;
an items page, image or screenshot is one message (same as the former node connection). Maximum bus line is 64 MiB; manager doesn't forward node messages over 60 MiB (drops/logs events, calls return `too_large`), avoiding a bus disconnect.

**Keys**: Persona Link credentials stop at hub (after authentication, manager receives only credential id and bound machine); pairing codes and pairing stay hub-owned. Kind-service helper credentials are needed for manager dialing:
manager generates them during pairing, hands to hub for encrypted storage, then retrieves once and caches in memory (retrieves again only after rejection, re-pairing or pairing to another hub), not on every probe.

**Hub restart**: the manager is a hub child and restarts too. Before stopping, it hands online machines' `node.info` and session lists to its next instance (its settings `hub.warm_machines`), then disconnects; the hub keeps what it saw reported in its own settings (`hub.devices_warm`, [hub-restart.md](../operations/hub-restart.md));
the new hub takes its own snapshot to display machines (including engines), and calls to them wait for their own manager's first report (up to 30 seconds). Manager consumes its snapshot, shows machines online for 30 seconds and reconnects nodes.
The hub records per manager whether it is connected and has reported (`devicesUpFor`, `waitDevicesFor` in `server/internal/hub/devices_reports.go`): a machine is reachable, a call to it goes out and its manager's `devices.list` is `complete` by that manager's own state, never the built-in assistant's for everyone. So the built-in assistant restarting leaves another manager's computers reachable and its list complete, a manager's list is complete only after its own report, and a call to another manager's computer during the start waits for that manager the same way (after the start, a manager connected but not yet reported is offline for its machines' calls, as the built-in assistant always was).
Hub listening starts before manager: App requests and manager-owned routes arriving during these seconds wait (up to 30 seconds) for the manager they go to; Persona Link links whose manager is not ready wait up to 5 seconds for the machine's manager (Persona Link gives a handshake 8 seconds),
then return 503 if not ready (normal Persona Link backoff). After this startup window, absent manager returns 503 immediately, without waiting. A computer nobody is known to carry (the ownership setting could not be read) is offline at once.

Observable differences from before:

- Absent manager means offline machines and 503 assistant_offline (formerly hub connected computers independently of assistant restarts).
- On machine connection, Cua / ego state and extension programs start a little later (after hub answers `devices.connected`); `waitNodeIdle` before node updates uses node-listed sessions and unanswered questions forwarded by manager to hub, no longer **Needs you** on hub cards.
- With `assistant.external` (manager on another computer), that computer makes connections: its SSH config, tailcat key, `node --stdio` binary and node update packages are used;
  hub restart doesn't restart it (computers stay connected), but for the second or two before it reconnects to the bus, hub shows machines from hub.json as offline.

### A dropped connection that resumes (#10663)

A computer whose resident node announces `node_resume_v1` is not offline when its connection's transport ends (a relay
ended, a link dropped, the address changed and pings went unanswered): for up to `nodeproto.ResumeWindowSeconds` (30 s)
the manager keeps the connection and shows the computer as it was, its calls waiting rather than failing, and takes
the connection back over a new transport on the same path (`devices/resume.go`; protocol.md "Resuming"). The node
keeps it longer, `nodeproto.ResumeKeepSeconds` (60 s), since it may notice the drop first. Nothing is watched again
and the apps see no change. Not resumed in that time, or refused (the node restarted), the connection
ends as any does: the reconnect grace and offline below follow. The lines about the computer's connections log its
drops, resumes, fall backs and the time to resume (`drops=… resumed=… fell_back=… recover=latest/longest`).

## Who handles computer events

Computers belong to the manager, and so does everything they send: the hub's existing code moved unchanged into the manager (`server/internal/assistant/fleet*.go`, monitoring in `server/internal/devices/metrics.go`); the hub only forwards and delivers.

| Manager responsibility | Code |
| --- | --- |
| Interactions: waiting for `interaction.create`, resumable cards (retain for 3 minutes after node disconnect for hook reclaim; recover from `interactions` after manager restart, without another push on reclaim), expiry, always allow for session, screen cards (read, keypress, recheck), Codex cards, plans answered in terminal, history table, assistant questions/cards, questions from hub-owned features (`fleet.ask`: sending mail, Cua apps, persona_settings) | `fleet_ix.go`, `fleet_ix_screen.go`, `fleet_ix_codex.go`, `fleet_ix_terminal.go`, `fleet_ix_own.go` |
| Session lists: unified state, background tasks, model errors, seen (`session_seen`), state start time, to-do / quiet / work state / channel markers / assistant-chat titles | `fleet.go`, `fleet_sessions.go`, `fleet_seen.go`, `fleet_work.go`, `fleet_text.go` |
| Push decisions: waiting and filtering for done notifications (seen, muted, answered through a channel, Inbox twin, notify_user already pushed, no duplicate push for one Stop), card notifications | `fleet_push.go` |
| Live Activity content (FleetState) and Dynamic Island counts | `fleet_live.go` |
| Node MCP tools (`mcp.tools` / `mcp.call` dispatch) and `assistant.lark` | `fleet_tools.go` |
| `lark.sent`: a write the node's gate proxy forwarded, kept in the sent-as-user ledger | `fleet_events.go` (`NodeRequest`), `extmgr/sent_ledger.go` |
| Channel-followed chats (`session.follow`: node sends items and generating text to manager, delivered to channels with turn starts/ends in node order) | `fleet_follow.go`, `devices.Follow` |
| Machine monitoring (samples, 24-hour history, downsampling, cleanup) and token usage | `devices/metrics.go` |

**Hub responsibilities** (forwarding and delivery only, `server/internal/hub/fleet_link.go`):

- Keep manager-provided session lists per machine (`fleet.sessions`, machine `sessions`), pending interactions (`fleet.interaction` / `fleet.interaction_resolved`, replaced in full by `fleet.interactions` after each handshake), and Live Activity content (`fleet.live`). App `GET /api/v1/sessions`, `GET /api/v1/interactions`, connection snapshots and pushed events use these; while manager is absent, use the last copy.
- Deliver pushes: manager decides content (`notify.request`, `kind` `assistant` / `done` / `input` / `reply` / `decide`); hub selects devices according to their settings, localizes to device language, sends through APNs / relay, sends `notify` to `ws_notify` Apps, and informs extensions (`agent.finished`, `agent.needs_you`). Live Activity delivery cadence (per-device intervals, start/end) also belongs to hub; content is computed by manager.
- Forward App answer, history, seen and monitoring routes to manager (`http.request`): `POST /api/v1/interactions/{id}/resolve` (hub first checks answer-only devices), `GET /api/v1/interactions/history` (hub supplies `X-Persona-Elicitation` for App MCP-form support), `POST /api/v1/sessions/{mid}/{sid}/seen`, `GET /api/v1/metrics`, `GET /api/v1/machines/{id}/metrics`, `GET /api/v1/usage`, `GET /api/v1/usage/days`, `GET /api/v1/usage/hours`.
- Tell manager about Apps: an MCP-form-capable App connected, or a phone registered to be pushed MCP forms (`fleet.clients`), WebSocket `seen` (`fleet.seen`), mute changes (`fleet.mutes`).
- Run what still belongs to hub for manager: phone tools (`app.call phone`: list tools, run on connected phones, silent push wake-up when needed). Connector / Cua / ego tools, extension descriptions for write-action cards, lark-cli settings and owner actions for channel card answers belong to manager extensions (`internal/extmgr`, queried directly in-process and cancelled when node stops waiting; `docs/extensions/extensions.md`, "Who owns what").

While manager is away: lists and cards use the last copy; answering, seen, history and monitoring return `503 assistant_offline`; nothing is lost. On return, it first hands full interaction and session lists to hub, then hooks reclaim resumable cards (same card, no new push).

Observable differences:

- Manager cards (triage cards, assistant questions) previously had answers held by hub until manager returned; now cards live in manager, so answers fail with 503 while absent and work normally after return.
- A newly opened session (`POST /api/v1/sessions`) returns `state` only after hub waits for manager to list it (up to 2 seconds); omitted if not available.

## System information

The node reads this once at startup and sends `system` in `node.info` to the manager, which stores it in `machine_systems` (so OS information survives offline periods and restarts),
includes it in reported `Machine.system`, and the hub sends it to Apps.

| Field | Linux | macOS | Windows |
| --- | --- | --- | --- |
| `distro` / `like` / `name` / `version` | `ID` / `ID_LIKE` / `PRETTY_NAME` / `VERSION_ID` from `/etc/os-release` (fallback `/usr/lib/os-release`) | `macos`, ProductName + ProductVersion and BuildVersion from `sw_vers` | `windows`, ProductName + DisplayVersion from registry `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion` ("Windows 11 Home 25H2"; Windows 11's ProductName still says Windows 10, corrected when CurrentBuild ≥ 22000); `build` is CurrentBuild.UBR |
| `kernel` | `/proc/sys/kernel/osrelease` | `Darwin ` + `kern.osrelease` | `NT ` + version from `RtlGetVersion` (`NT 10.0.26200`) |
| `model` | DMI `sys_vendor` + `product_name` (strips placeholders such as "To be filled by O.E.M.") | `product-name` from `ioreg -p IODeviceTree -rd1 -n product` (Apple silicon, without its parenthesised size and year, as system_profiler words it), else Model Name from `system_profiler SPHardwareDataType` (Intel); only that line of either is read, since both outputs hold serial numbers. Looked up in the background and kept for the process once found: `node.info` waits at most a second for it and otherwise sends `hw.model`, and later connections get the name (#6028); a lookup that ends without a name is tried again by a connection at least five minutes later | SystemManufacturer + SystemProductName from registry `HARDWARE\DESCRIPTION\System\BIOS` (same placeholder removal) |
| `cpu` | First `model name` in `/proc/cpuinfo` (`Hardware` / `Model` on ARM boards) | `machdep.cpu.brand_string` | ProcessorNameString from registry `CentralProcessor\0` |
| `cores` / `memory` / `boot_time` | `runtime.NumCPU`, `MemTotal`, `btime` in `/proc/stat` | `runtime.NumCPU`, `hw.memsize`, `kern.boottime` | `runtime.NumCPU`, total physical memory from `GlobalMemoryStatusEx`, now minus milliseconds since boot |

OS icons (`OSBadge`): macOS uses the SF Symbols Apple logo; Linux distributions use their official logos (Simple Icons vectors, `Theme.Brand.os`),
tinted with distribution colors. Unknown distributions fall back to `ID_LIKE` (Pop!_OS → Ubuntu), then the generic Linux penguin. Vector assets stay clear at every size and in light/dark.
(Before 2026-10-31 these were drawn circles and letters; the user requested official logos.)

## Collection definitions

The node samples every 10 seconds (`proto.MetricEvery`, aligned to whole 10-second boundaries so all machines' points share timestamps), sending a `metrics` event to the manager.
The first sample lacks CPU and network rates (need two readings); fields are omitted, not zero.

| Metric | Linux | macOS | Windows |
| --- | --- | --- | --- |
| Claude processes (`claude_procs`) | The node user's running claude command lines in the cached `/proc` walk (native binary and node/bun running a claude script, every config directory); omitted off Linux, so the hub skips its process cap rather than reading zero | Omitted | Omitted |
| CPU | Difference of `/proc/stat` readings: busy = total − idle − iowait (I/O wait is idle, like top id + wa; guest is already in user, not counted again); same per-core calculation, latest sample only (not stored) | Resident `iostat -n 0 -w 10`, latest line's 100 − id (past 10-second average; first line is since-boot average, discarded) | Difference of `GetSystemTimes`: busy = kernel + user − idle (kernel includes idle, DPC and interrupts); per-core via `NtQuerySystemInformation(SystemProcessorPerformanceInformation)`, latest sample only; no per-core above 64 logical processors (processor groups) |
| Load | First column of `/proc/loadavg` | `sysctl vm.loadavg` (binary struct loadavg) | None (Windows has no load average), omitted |
| Memory | Used = MemTotal − MemAvailable (page cache available, like `free` available); old kernels without MemAvailable use free + buffers + cached. In a container (the v2 `memory.max` / v1 `memory.limit_in_bytes` under `/sys/fs/cgroup` has a limit below MemTotal) the total is that limit and used = usage − inactive_file (like `docker stats` and `kubectl top`): without lxcfs, `/proc/meminfo` in a container is the host's; the system info's memory uses the same limit | Resident `vm_stat 10`: used = anonymous pages − purgeable + wired + compressor size, matching Activity Monitor **Memory Used** (App Memory + Wired Memory + Compressed); file cache available | `GlobalMemoryStatusEx`: used = total − available (standby-list file cache is available, like Linux MemAvailable) |
| Swap | SwapTotal − SwapFree | `sysctl vm.swapusage` | Page files: sum size and used amount from `NtQuerySystemInformation(SystemPageFileInformation)` |
| Network | Physical-interface receive/send byte differences in `/proc/net/dev` / elapsed time; in a container's own network namespace, where every interface is virtual, the interfaces the name keeps (eth0, not lo); discard sample when set changes | Physical-interface differences from routing socket `NET_RT_IFLIST2` (`if_msghdr2` 64-bit counters) / elapsed time | InOctets / OutOctets from `GetIfEntry2Ex`, physical-interface differences / elapsed time; eligible interfaces listed once a minute (~7 ms to list, ~0.2 ms to read counters), discard sample when set changes |
| Disk | Sum `statfs` for local block devices in `/proc/self/mountinfo` (`/dev/…`, excluding loop and read-only squashfs / iso9660 images, counting bind mounts of a device once): data disks often mount at `/data00`, so `/` alone makes a 5 TB machine show 118 GB. A mount whose root is not `/` (one directory of a device) is skipped, except `/` itself and btrfs subvolumes: in a container, Kubernetes / Docker bind directories of the host's data disk in (an emptyDir on `/tmp`, `/etc/hosts`), and that disk is shared by every container on the host; without mountinfo it falls back to `/proc/self/mounts`; used = total blocks − free blocks, total = used + ordinary-user available (like df) | `statfs("/System/Volumes/Data")` (`/` is a read-only system snapshot, only a few GB) | Sum `GetDiskFreeSpaceEx` for fixed drives actually on disks (`GetDriveType` = `DRIVE_FIXED`, volume has a positive-length disk extent, each volume once); used = total − free, total = used + user available (like df); excludes network, USB, optical and sync-client virtual drives such as Google Drive (claims FAT32 fixed drive and copies C capacity, but disk extent length is 0); drive letters listed once a minute |
| Battery (`battery_percent`, `battery_charging`) | `/sys/class/power_supply/*` where `type` is `Battery`, `present != 0`, and `scope != Device` (skips Bluetooth peripherals); uses `capacity` or `energy_now`/`energy_full` or `charge_now`/`charge_full`. On Android Termux when `/sys/class/power_supply` is unreadable, falls back to cached (`>= 60s`) `termux-battery-status` if `TERMUX_VERSION` is set and the binary is in `PATH`; omitted (`nil`) when absent or unreadable | Cached (`>= 60s`) `pmset -g batt` when `InternalBattery` is present; omitted (`nil`) on desktops (Mac mini, Mac Studio, Mac Pro) | Win32 `GetSystemPowerStatus` (`kernel32.dll`, no subprocesses); omitted (`nil`) when `BatteryFlag & 128 != 0` (no system battery), unknown, or `> 100` |
| Temperature (`temp_c`) | `/sys/class/hwmon/hwmon*` and `/sys/class/thermal/thermal_zone*` (millidegrees / 1000), preferring CPU/SoC/package zones (`x86_pkg_temp`, `coretemp`, `k10temp`, `cpu*`, `cpuss*`, `soc*`, `aoss*`), filtering invalid readings (`<= 0` or `> 125` °C) and non-host/RF/battery probes (`pa*`, `sdr*`, `bcl*`, `ibat*`, `vbat*`); omitted (`nil`) when unavailable | Omitted (`nil`) | Omitted (`nil`, no unprivileged zero-subprocess Win32 thermal API) |

Why macOS starts two resident processes: builds use `CGO_ENABLED=0`, so Mach calls such as `host_statistics` are unavailable; `vm.page_*` sysctls lack anonymous-page and compressor counts.
Each process prints one line every 10 seconds, much cheaper than starting `vm_stat` / `top` each sample; if one exits, its CPU or memory reading is dropped (samples leave it out, as at startup, instead of repeating the last value) and the process restarts at once the first time, then after a minute; they stop with the node (`exec.CommandContext`).

Windows starts no processes: also `CGO_ENABLED=0`, but these Win32 calls work in pure Go (`golang.org/x/sys/windows`; `GetSystemTimes` and `GlobalMemoryStatusEx` directly through kernel32).

### Which network interfaces count

Count only traffic actually leaving the machine. Virtual interfaces are either local (lo, containers) or counted again on underlying physical interfaces (tunnels, proxies, bridges):

- **Linux**: exclude `/sys/class/net/<name>` links into `/sys/devices/virtual/net/`. The kernel puts software interfaces there: lo, docker0, veth*, br-*,
  virbr*, tun* (TUN proxies), tailscale0, wg*, bond / VLAN (member interfaces still count, without duplication). Real interfaces, including VM virtio, live under buses and count.
  Without `/sys`, exclude name prefixes (`sysmon.virtualName`: lo, docker, br-, veth, virbr, tailscale, tun, tap, wg, zt, cni, flannel, cali, etc.).
  The names also decide when `/sys` calls every interface virtual: in a container's own network namespace (Docker, Kubernetes) eth0 is one end of a veth pair,
  and its traffic is the container's own; counting nothing would draw a flat 0.
- **macOS**: Ethernet type (IFT_ETHER), `en*` name, non-loopback only: Wi-Fi, wired Ethernet, Thunderbolt. Exclude utun* (Tailscale, VPN, TUN proxies),
  bridge* (Thunderbolt bridge, member en* already counted), awdl / llw (AirDrop), anpi, gif, stf, ap.
- **Windows**: only `MIB_IF_ROW2` with `HardwareInterface` set and `FilterInterface` unset: Wi-Fi, Ethernet, USB tethering.
  Tailscale (wintun), VPN adapters, Hyper-V / WSL vEthernet, Wi-Fi Direct virtual adapters, Bluetooth PAN and loopback are software interfaces and excluded;
  filter layers atop physical interfaces (WFP, QoS) duplicate counters and are excluded too.

If counters decrease between readings (interface vanished or counters reset), discard that sample instead of drawing a huge spike. Linux and Windows also
discard it when the set of counted interfaces changes: a newcomer brings its bytes since boot.

### Overhead

Measured with `personad sysmon 30` (2026-10-30):

| Machine | Collection time | personad CPU | Other |
| --- | --- | --- | --- |
| Mac Studio (M1 Ultra, 20 cores, macOS 26) | 0.55–0.70 ms | 51 ms / 5 minutes = 0.017% | iostat + vm_stat together used 0.34 s CPU over 2 hours 21 minutes (0.004%) |
| Linux VM (32 cores, Ubuntu 24.04, 6 interfaces) | 0.76 ms | 46 ms / 5 minutes = 0.015% | No subprocesses |
| Windows 11 desktop (16 threads, 4 drive letters, 7 interfaces, 2026-11-04) | 1.8 ms (interface/drive enumeration once a minute ~8 ms, other samples ~0.5 ms) | 47 ms / 5 minutes = 0.016% | No subprocesses |

Five minutes after node starts, it logs average duration once (`metrics: a sample takes …`). Startup transcript scanning can inflate it
(4.7 ms observed on the same Linux machine where standalone `personad sysmon` takes 0.76 ms).

## Storage and retention

On receipt, the manager retains the latest sample in memory (Dashboard **Now**, not current if older than 30 seconds) and writes it to `machine_metrics` (raw `step = 10` row).
The nodes sample on the same wall-clock tick, so one writer stores a tick's samples of every machine in one transaction (`db.InsertMetrics`: one commit and one WAL flush, not one a machine).
It writes once every machine that sent in the last 30 seconds has sent this tick's, at most 2 seconds after the first (a single machine's sample is written at once); one machine's bad sample does not cost the others theirs.
Samples wait in a queue of 1024; when Postgres is that far behind, a sample is dropped with a log line. A stopping manager still writes what it has, for at most 2 seconds.

The main assistant's read-only `machines_list` tool combines the trusted machine list with the manager's last system information and sample, working agent count, recent failing launchers and node version status. It reports the list's completeness and excluded count; machines that belong to another manager or are not trusted are not exposed. Connection, session and launcher records have separate availability: an empty failure list means no recent failure is recorded, not that every launcher was checked. Values whose source is missing, ambiguous, offline, stale or has a future timestamp are null with a reason. Samples over 30 seconds old retain their age but are not current; a sample from before the current connection is not current either. System information is the last reported information and may persist while offline; memory and disk availability follow the collection definitions above, and disk is an aggregate rather than necessarily the work directory's filesystem. Node builds that differ cannot be ordered by their identifiers, so a different build alone is not called outdated. The tool returns facts without ranking machines.

The assistant-only `machine_redeploy` tool accepts a machine ID or an exact, unambiguous display name. Name selection requires a complete machine inventory; an ID remains valid when the list is incomplete. It invokes the same explicit `Machine.startDeploy` operation as `POST /api/v1/machines/{mid}/deploy`, waits for that job's outcome and returns success or the failure reason. The app's route remains asynchronous. Canceling the tool's wait does not cancel the deployment or change its card phases, persisted outcome or notifications; an unconfirmed wait is not reported as a failed deployment. A call cannot target the machine running its own conversation, because the update waits for that conversation to become idle; use a conversation on another machine instead.
Every minute `metricsLoop`:

1. **Downsamples** raw rows older than an hour into one row per machine per minute (`step = 60`): averages values and takes maximum totals (memory, disk, etc.).
   Delete and insert are one statement (`WITH moved AS (DELETE … RETURNING *) INSERT … SELECT … GROUP BY`), so a minute isn't merged twice.
   Raw rows are found through partial index `machine_metrics_raw` (`(ts) WHERE step = 10`, containing only the latest hour's raw rows), avoiding scanning the full 24-hour table each minute; SQL uses constant `step = 10` so cached statements' generic plans can still use it.
2. **Cleans up** all rows older than 24 hours 10 minutes (10-minute margin keeps the left edge full when opening a 24-hour chart).

Queries average both row types weighted by covered duration (raw 10, aggregate 60), keeping curves continuous across the one-hour boundary.

### Volume

Per machine at steady state: 360 raw rows for the latest hour + about 1390 aggregate rows for the preceding 23 hours 10 minutes ≈ **1750 rows**; 6 machines ≈ **10,500 rows**.
Measured row data is 116 bytes (`pg_column_size`), roughly 200 bytes including headers, pointers and primary index; whole table about **2 MB**. Writes per machine per day: 8640 raw + 1440 aggregate rows;
6 machines about 60,000 inserts and 50,000 deletes daily (autovacuum reclaims them; table doesn't grow).

## Token usage (#4973)

How many tokens each machine runs, for the Machines page's cards and charts (the Dynamic Island's totals come from the same data, `docs/apple/live-activity.md` ("Usage")).

- **Metering** (`node/usage.go`): every 10 seconds the node reads what its agents' records gained. An adapter per agent (`node/usage_adapters.go`, #9923) says which files hold its usage,
  which session a file belongs to and which parser reads it: Claude Code's main and subagent transcripts in every config directory, and the agents of its workflow runs
  (`<session>/subagents/workflows/<run>/agent-*.jsonl`, which it missed until #9255); the rollouts of Codex (every Codex home: the default and the registered ones) and of the compatible traex CLI, each segment of a
  continued thread (the session list follows a thread's newest), and for a backfill reaching further back than two weeks every rollout on disk, archived ones too (#10002); the sessions of pi and omp
  (`$PI_CODING_AGENT_DIR`, else `~/.pi/agent` and `~/.omp/agent`: `sessions/<cwd>/<time>_<id>.jsonl`, #9992). opencode keeps its steps in a SQLite database (`$PERSONA_OPENCODE_DB`,
  else `$XDG_DATA_HOME/opencode/opencode.db`, default `~/.local/share/opencode/opencode.db`): its adapter queries it with the system `sqlite3` (`node/usage_opencode.go`), the rows added
  since the last read and the unfinished ones again, nothing while the database has not changed; without `sqlite3` (a Linux machine often has none) opencode is not metered.
  Cursor's CLI and Antigravity keep no token counts the node can read and check: they are not metered. A runtime with no record of its own can report per-request deltas through the runtime SDK (`docs/integrations/runtimes.md`, "Runtime token usage"). The parsers (`server/internal/tokenuse`)
  are the same ones the usage on a chat's replies and the subagent summaries read (`docs/protocol/protocol.md`, "Token usage on items"), so a request counts the same everywhere
  and sums the replies per minute, model, **agent** (its adapter's, `proto.Session.Agent`'s names, #9984) and **launcher**: the launcher Persona started the session with (`sessionState.Launcher`, looked up by the transcript's session or thread id;
  a subagent's or workflow agent's transcript is its session's). A session typed in a terminal has no launcher (`""`, shown as "Other"). The node keeps 26 hours and reports changed minutes as `usage`
  (`"launchers": true` says its buckets name the launcher; an older node's do not, and the apps grey out the launcher filter for it).
- **Runtime reports** (`node/runtime_usage.go`): `usage.report` becomes a `usage` notification with `add: true`, one request in a minute/model/launcher/agent bucket. Memory and Postgres add all four counts and the request count; `full` file scans preserve runtime-only agents. Reports are not reconstructable and can be lost during reconnects or storage failures; they have no backfill. Reasoning remains part of output.
- **Counted once**: a reply is counted once in the window even when it is in several files. A fork (Claude Code's `--fork-session`, Codex's fork) copies the history it starts from,
  timestamps included, into the new file; the meter remembers every reply it counted (Claude Code's message id, Codex's timestamp and running total) for the window.
  Measured on the transcripts of two machines over three days (2026-11-06): no reply was in two files and none repeated out of order in one file, so today's numbers do not change; the rule covers forks.
- **What a reply counts**: Claude Code's `usage` (input not from the cache, cache creation as cache write, cache read, output), once a request at the most its lines say (Claude Code up to 2.1.306
  wrote the output so far on each content block); Claude Code's own replies for a failed request (`<synthetic>`, `isApiErrorMessage`) and usage of all zeros are skipped. Codex's `last_token_usage`:
  its input includes what was read from the cache and what was written to it (`cached_input_tokens`, `cache_write_input_tokens`), counted as cache read and cache write and taken out of input,
  as Codex itself does for its fresh input (OpenAI's models write none: 0); reasoning tokens are in its output. pi's and omp's usage is already in the four kinds (an assistant
  message's `usage` with its `model`; pi's `usage` entries, omp's `model_usage` entries and pi's compaction and branch summaries count too, a summary under the session's latest model;
  omp's `orchestration` tokens are added to their kinds); a fork copies their entries as they are, so the entry's id and time name a request. An opencode step counts once it finished
  (`data.time.completed`; a compaction once completed or failed), its `reasoning` added to its output, which leaves it out. Reasoning is output for every agent.
  Cache hit rate = cache read / (input + cache write + cache read).
- **Storage** (`devices/usage.go`, `db/usage.go`): the manager keeps the last 26 hours in memory (the island, the cards) and writes every notification to `machine_usage` in order, from one goroutine.
  Each node reports on a 10-second tick of its own, so while another machine has written in the last 25 seconds, the notifications that come within 5 seconds of one go in one transaction with it (`db.PutUsages`, in order; if Postgres rejects one, each goes under a savepoint of its own); a single busy machine's are written at once.
  A bucket is its minute's whole total, so a later one replaces it; a node's whole window (`full`, after it connects) first deletes only the file-metered agents' minute rows (and legacy unknown-agent rows) of that window, so what the node
  counts now is the record (an older node's rows without a launcher do not stay next to the new ones). Each minute the minute rows older than 27 hours are summed into one row per 10 minutes
  (`step = 600`), the 10-minute rows older than 7 days and an hour into one row per hour (`step = 3600`, `db.RollupUsageHours`), and rows older than 53 weeks and a day are deleted.
  About 1 to 3 rows a machine and minute (one per model, launcher and agent in use), 10,000 to 30,000 rows a machine for the week, then 24 to 72 a machine and day for the year (about 9,000 to 27,000).
  Rows keep their agent through the rollups (`machine_usage.agent`, migration `manager/0551_usage_by_agent.sql`); rows stored before agents were kept apart, and an older node's, have `''`.
- **Ranges**: 1 hour by minute (60 points), 24 hours by 10 minutes (144), 7 days by hour (168). A point is the step's sums; for "per minute" the apps divide by the step's minutes, the step under way by the part of it that had passed when the report came (at least a minute, `web/src/lib/usage-rate.ts` `stepMinutes`, PersonaWire `MachineUsageSeries.minutes(at:)`; #9205).
  A step with no monitoring sample and nothing counted is a break (`gap`), as in the monitoring charts; a machine that never sent monitoring samples has no breaks.
- **Card** (`GET /api/v1/metrics`'s `usage` per machine, from memory): today's sums, all tokens per minute now (over the last two to three minutes) and the last 60 minutes by minute.

### Token usage history

The calendar of tokens a day on the assistant's home and its page's last week by the hour (`devices/usage_history.go`): every machine's usage summed, every kind of token (cache reads included),
so a day's number is the same sum as today's tokens on the Machines page.

- **Days** (`GET /api/v1/usage/days?days=N`, 1 to 371, default 140): each of the last N days in the user's zone (`Options.Location`), midnight to midnight, oldest first; the last is today.
- **Hours** (`GET /api/v1/usage/hours?days=N`, 1 to 7, default 7): each hour on the clock from the first day's midnight to the hour under way, with its day and hour on the clock
  (a day the clocks change has 23 or 25; the hour that comes twice is there twice), each machine's share of them, largest first (a machine no longer here has no name), each agent's share
  (`agents`, #9984; `''` for the rows stored before agents were kept apart: they leave the week a week after the update, and a node's first whole window after it re-sends its last 26 hours by agent), and their total.
- **Sums** (`db.UsageSpans`): Go works out the day or hour starts in the zone and Postgres sums the rows into them (`width_bucket` over the starts), every machine's together or each machine's or agent's apart,
  through the `machine_usage_ts` index. A row counts in the span its start is in: the minute and 10-minute rows of the last week fall exactly, while the hour rows split a day at the hour
  in a zone whose clock is not a whole hour from UTC.
- Without Postgres (`stored: false`) both come from the buckets in memory, the last 26 hours.

### Backfill

The history starts the day the manager began keeping it; the days before are only in the transcripts on each computer. Once a computer (#9484, `node/usage_backfill.go`,
`devices/usage_backfill.go`), its node reads them and sends the hours:

- **When**: three minutes after the node connects (its sessions' state loaded, as the live meter's launchers need), it asks `usage.backfill.status`. The manager answers
  `done`, or the range: from where the history begins (53 weeks back) or the hour after the last one it stored, to the hour the node's live window (26 hours) starts in.
  Without Postgres it answers `done`. An older manager does not know it, and the node does nothing.
- **What**: the live meter's rules (a reply counted once across files, `<synthetic>` skipped, the same four kinds) over every transcript changed since the range began,
  each read from about the range's start (`startOffset`), summed by the hour, model and launcher. Only those numbers go to the manager: no text, no file names or paths.
  A year of reply ids is kept as 64-bit hashes. What is on disk is the limit: Claude Code deletes transcripts after 30 days unless `cleanupPeriodDays` says otherwise; Codex keeps its rollouts.
- **A week at a time** (#9562): the node reads one week, oldest first, sends its hours and only then reads the next. A computer with many gigabytes of
  transcripts takes longer than the time between two hub restarts (several an hour), and a scan that kept everything to the end started over on every
  connection and never finished. Now each week is stored as it is read, so a cut scan goes on from the next week, and the node keeps nothing. A fork's
  copy of a reply has its timestamp, so it is in the same week and still counted once. A file is read from about the week's start (`startOffset`; files
  under 256 KB from their start) and stops once 50 lines in a row are past the week, so the weeks together read about one pass of the transcripts.
  The weeks start at the earliest transcript's first line, not 53 weeks back, and a week reads only the files that changed after it began and whose
  first line is before its end (#9616: a computer with recent transcripts only spent its first hour on empty weeks, looking for each week's start in every
  large file). Each file's first line is read once a run (64 KB); those reads and the searches for a week's start count against the rate as well.
- **Gently**: the reads are limited to 4 MB a second for all transcripts together, at most 256 KB at a time, and the scan ends with the connection.
- **Stored once** (`db.PutUsageBackfill`, table `usage_backfill`): the node sends each week's hours oldest first, whole hours a request (about 2,000 buckets), the week's last request saying it is read through its last hour. The manager stores an hour
  (as hour rows, `step = 3600`) only when the machine had no row in it before the request, so the live meter's count of an hour always stays; in the same transaction it records
  the last hour sent and, with the last request, when the scan finished. A scan cut off (a restart, a dropped connection) asks again on the next connection and reads from the
  hour after the last stored; a request sent twice adds nothing. Once finished, the node is not asked again.
- **By agent** (#9984): the node names the agents it reads and the manager keeps a record per agent (`usage_backfill.agent`). Claude Code's is the machine's first
  record (`''`): done stays done. Every other agent has its own, so an agent whose adapter a node gains later has its history read (only its files) from the history's start,
  and its hours are stored beside the others: an hour is stored for an agent only when the machine had none of that agent's usage in it, nor, for an agent metered before
  agents were kept apart (Claude Code, Codex, traex), usage stored then (`''`, which may hold theirs). Codex and traex got records of their own with #10002: the first
  backfill listed only the rollouts written in the last two weeks, so their older history is read again, and fills the hours that held none of it. A request names the
  agents it read; only their buckets are stored and only their records move on. An older node names none (the first record, as before); an older manager answers its one
  range, which the node then reads for every agent.

## Gaps

Periods without machine samples are breaks in the chart, not connected lines:

- Manager `points` inserts `{"t": …, "gap": true}` between adjacent points more than 3 steps apart; also appends one when the last point is more than 4 steps old (machine offline now).
  Gaps shorter than 3 steps (occasionally missing one sample) stay connected. Steps: 1 hour = 10 seconds, 6 hours = 1 minute, 24 hours = 4 minutes (about 360 points per chart).
- App draws each segment between gaps as a separate line (detail Swift Charts `series:` uses "metric#segment"; card sparklines split by the same segment numbers), so lines never cross gaps; holding on a gap shows no value.
- Card sparklines use custom `Canvas`, not Swift Charts (`Sparkline` / `SparklineGeometry`: monotone cubic curves like chart `.monotone`, with a light fill beneath a single line):
  with a dozen charts per page, each chart's own layout adds about 130 ms on Mac each time Machines opens; vectorized `LinePlot` / `AreaPlot` is equally slow (overhead per chart, not point count, #390). Appearance is unchanged and compared in screenshots.

## App

Previously **Machines** (Agent counts) and **Machine Monitoring** (Dashboard) were separate pages, and machine details needed another tap into **System & Monitoring** to see curves. Requested 2026-11-01: merge into one **Machines** page,
removing the **Machine Monitoring** name and entry point.

- **Entry points** (all open `MachinesView`): **Machines** in the new shell drawer; **Settings › Machines**; **Machines** at the bottom of the Agents list's top-left menu (`MachineListRoute`, opens in the right column on iPad);
  Mac sidebar **Machines** (⌘5). Top-right ⓘ on Projects / machine projects opens machine detail (`MachineDetailRoute`).
- **Machines page** (`MachinesView`): top line "N/M online · K under heavy load · J Agents need you". Under it **Today's Tokens** (`TodayTokensCard`, #4973): the total, every token per minute now,
  the cache hit rate, and each computer's share as a bar in shades of the text ink (not the kinds' colors). One card per machine (`MachineCard`): OS icon, name, OS · cores · memory (one line),
  online / offline (last online time and SSH error when offline); alert tags ("Alerts" below); Agent counts **Needs you / Working / Idle** (Idle = Unread + Seen + Unknown,
  live sessions only, excluding assistant chats, matching Agents list); current CPU / memory / network values and past-hour sparklines, and **Tokens** (per minute now, today's total, the past hour) when its node reports usage; **Redeploy** when personad and hub aren't from the same deployment.
  iPad / Mac cards form multiple columns according to width. Tap to open machine detail.
  Web deployment entry depends on `can_deploy`: old Link nodes prompt a one-time local upgrade; supported nodes show waiting for idle / transfer / restart / version confirmation; details in [node-update.md](../operations/node-update.md).
- **Ordering** (`MachineConcern`): attention-needed first: offline → severe alerts → caution alerts → Agents needing you → version mismatch; within each category and for remaining machines, hub config order.
  Reorder only when something shown on the card changes, with animation.
- **Machine detail** (`MachineDetailView`): everything on one page, common actions first. Same card at top (without metrics or the name, the title has it); when offline, next comes **Hub connects through SSH …** and **Reconnect**.
  Then **Agents**: Needs you first, Working next, first 5 shown, **Show all N** for the rest; tap to open chat. Next **Engine** (which engine's sessions this machine shows; tap to choose, `EngineSelectionView`). Then **Token Usage** (`MachineUsageSection`, see "Token usage charts" below). Then **Monitoring**: current CPU / memory / network / load / disk / swap,
  1 hour / 6 hours / 24 hours selector, four charts for CPU, memory, network (download + upload), load (dashed core-count line); Linux also has per-core bars.
  Hold and drag any chart to inspect a time; all four share the selected timestamp, showing its value and time in the header. Then **What This Computer Can Do** (two rows when everything works:
  the summary and **Show All Checks**; the features show when something needs attention or after **Show All Checks**), the computer and browser sections, **System** (OS, kernel, architecture, model, processor,
  cores, memory, hostname, boot time and uptime, herdr / Claude Code / node versions), **Launchers**, and, when online, connection method and **Reconnect** last.
- **Updates**: machine online state and Agents update live through hub pushes; numbers poll only while page is visible and App foreground: Machines every 10 seconds, detail's 1-hour range every 10 seconds, other ranges every minute;
  stops in background (`task(id: scenePhase)`). Pull to refresh (Mac ⌘R, `macRefreshes`) also reloads the machine list.
- **Alerts** (`MetricAlert`): CPU ≥ 75% caution, ≥ 90% severe; memory ≥ 80% / 90%; disk ≥ 80% / 90%; load ≥ 1 / 2 times core count. Cards get a status-colored border,
  and tags use icon (status color) + text (body color), not color alone.
- With old hubs lacking these endpoints (404), machine cards omit metrics (Agent counts remain), and detail monitoring says **Deploy newer hub and node versions to see this**; old nodes sending no samples show **—**.

### Charts

Follow dataviz rules:

- One color per metric, consistent on Machines and details; separate light/dark shades (`MetricColor`), both validated with `validate_palette.js`
  (CPU blue, memory teal, download orange, upload purple, load magenta; network lines' color-blind separation ΔE > 25).
- Lines 2pt (sparklines 1.5pt), one line has 10% same-color fill below, two lines have none; one-pixel grid; right-side y-axis with at most 3 ticks, x-axis at most 4 times.
- Values use body color; series color appears only on lines and legend dots. Two-line charts (network) have dot + name legends; single-line titles name the metric.
- Status colors (caution yellow, severe red) only indicate alerts, not ordinary series.
- Each chart has a VoiceOver description (current value, maximum).

### Token usage charts

`MachineUsageSection` (`apple/Persona/Views/Machines/UsageCharts.swift`), from `GET /api/v1/machines/{id}/usage`:

- **Controls**: 1 hour / 24 hours (default) / 7 days; **All Models** and **All Launchers** menus (from the response's shares; the unknown launcher is **Other**; greyed out with a note for an older node).
- **Numbers**: the range's total, output, cache hit rate and replies.
- **Throughput** per minute: **Total** (one line) or **By Kind** (default): input, output, cache write and cache read stacked from the bottom. Tapping a legend entry hides that kind (cache reads are often nine tenths of all tokens;
  hiding them shows input and output). The legend lists the range's sums, or the held moment's per-minute values. Offline time is shaded.
- **Cache hit rate**: cache read / (input + cache write + cache read), one line broken at steps with fewer than three replies or offline.
- **By Model**: each model's total and hit rate; tapping one filters by it.
- **Holding** either chart shows that moment in both. Polls every 20 seconds on the 1-hour range, every minute otherwise, only in front.
- **States**: an older hub (404) says to deploy a newer one; a range or filter with no usage says so in the chart (with **Clear Filters**).
- **Colors** (`UsageColor`, `apple/Shared/Design`): input blue, output orange, cache write violet, cache read teal, light and dark steps of the categorical palette, validated in the stacking order
  (adjacent pairs ΔE ≥ 24.7 light / 17.3 dark for color-blind readers). Light teal is under 3:1 on a light card, so the legend always writes the numbers.
- **Numbers** use `TokenFormat` (`apple/Packages/PersonaWire/Sources/PersonaWire/TokenUsage.swift`): K / M / B, three significant digits, no trailing zeros, under 1,000 as is, a rounding to 1,000 moves up a unit (999,500 → 1M). The web uses the same rules.
- **Web** (`web/src/views/machines/UsageSection.tsx`, `web/src/lib/tokens.ts`): the same section on a machine's page, before Monitoring (the engine and the self-check moved after it),
  with the chart and the numbers on the left and **By Model** / **By Launcher** on the right on a wide screen; hovering a chart or ← → on it holds a moment in both, Esc lets go.
  The Machines list has **Today's tokens** across the top and a **Tokens** cell on each card, and uses up to three columns on a wide screen (it used to stop at 1000px).
  Colors are the CSS tokens `--usage-input` … `--usage-cache-read` (`styles.css`), the same values as `UsageColor`; `formatTokens` follows `TokenFormat`'s rules (`tokens.test.ts` has the same table).
- Screenshots: the mock hub serves usage for every machine (the Mac Studio's node is an older one, the NAS is offline); `MOCK_TOKENS=none` is a hub without it, `=empty` one where nothing ran, `=lopsided` one where the server ran nearly all of it; `-openUsageRange 1h|24h|7d` opens a range.

## Doctor: what this computer can do (#2429)

The web Machines page has **What this computer can do** (modeled on Moshi's Doctor page), presenting `personad node doctor` results (`docs/integrations/claude-integration.md`, "Self-check") by feature, not a string of check names.
iOS / Mac machine detail (#3018, `apple/Persona/Views/Machines/NodeHealthSection.swift`, grouping in `NodeHealthModel.swift`'s `NodeHealth`) places it after Engine. Grouping, states and copy match web; changing one requires updating the other.
The retired native Mac app showed this section through the shared iOS view and received no Mac-only changes for it ([mac.md](../archive/apple/mac.md)).

- **Fetch**: hub routes `GET /api/v1/machines/{mid}/doctor` (read-only) and `POST …/doctor/fix` (rewrite Persona-generated files, recheck and return a new report) are **`full` devices only** (reports contain paths and user hook commands; fix writes files). Node RPC: `node.doctor` (read-only unless `{"fix":true}`), `node.doctor.fix` (`node/doctor_rpc.go`); checks run in node with CLI `RunDoctor`, same `DoctorReport` JSON. Hub GET never passes `fix` to node.
- **Report language**: current clients send `Accept-Language`, which the hub forwards as the RPC's optional `locale`; a new node then words `detail`, `hint` and human `items` in the page's language without changing the report shape or grouping fields. A header-less request and the assistant's direct readiness call keep canonical zh-Hans. A new node with an old hub therefore keeps today's report, and an old node with a new hub ignores `locale` and has its Chinese report relayed unchanged. The command's human text instead uses `i18n.Terminal`; its `--json` always retains canonical zh-Hans bytes.
- **Group by feature** (`FEATURES` in `web/src/lib/node-health.ts`): phone approval cards (`node.sock`, claude / assistant settings and hooks, `command-paths`), state and notifications (`node.sock`, `command-paths`), terminal and engine (`herdr`), MCP tools (`node.sock`, `mcp.json`, `command-paths`), skills (`agent-plugin`, `claude-version`), runtime integrations (`runtime.sock`).
  Each feature takes its worst check: any `fail` → **Has issues**, any `warn` → **Partly available**, all normal → **Ready**. Terminal/engine and runtime integration are optional: warn-only means **Not enabled** (in **Not set up: …**, not counted as needing attention),
  except when this computer uses herdr (including herdr + Herdr App, and no selection defaults to herdr), unreachable herdr means **Has issues**. Unmapped extra checks reported by node go to **Other checks** if abnormal, never hidden.
  `global-permission-hooks` (user's synchronous PermissionRequest hooks) is not a feature; displayed as an informational line below features, read-only, when not ok. Hooks of extensions installed on that computer alone are ok, so they show no line (#11973).
  `launchd-process-type` (Mac) and `path-access` (#5944) are in no feature either: when not fine they show under **Other checks** with the node's hint. Their state is empty, so **Fix** doesn't offer them: reloading the LaunchAgent from inside the node would stop it, and only `personad node doctor --fix` on that Mac does it.
- **UI**: **Everything works** or **N feature(s) need attention** at top (Moshi wording), then **Not set up: …**; **View all checks (N)** expands individual state, name, finding, remedy and differing files; **Check again**;
  **Fix** appears for generated files marked `stale` / `missing`, with confirmation listing files to rewrite and stating that user Claude Code settings won't change. Check once on opening, no polling.
- **Old node**: unsupported RPC returns `unknown method`; card says **This computer's node is too old. Update it to run checks here**, beside the existing deployment button (same `POST …/deploy`, same `can_deploy`). Old hub route 404 and non-`full` device 403 each have specific explanations. Offline computers make no request.
- **Connection failures** (`classifyConnection`): offline machine cards/pages classify recorded hub error text as **Authentication failed / Can't reach this computer / Connection timed out / Version mismatch**, each with a next step (different SSH / Persona Link wording);
  unrecognized text adds nothing, no guessing. Planned restarts during deployment don't count.
- Mock-hub screenshots: `MOCK_DOCTOR=healthy|stale|broken|mixed|old|error|slow`, `MOCK_DOCTOR_OLD=<machine id,…>` (`scripts/mock/mock_hub.py`).
- Tests: `node/doctor_rpc_test.go` (temporary directories; no writes without fix, fix only generated files and never user settings.json, check names match web grouping), `hub/node_doctor_test.go` (permissions), `web/src/lib/node-health.test.ts` (grouping, summary, fixing, old node, connection classification), `apple/PersonaTests/NodeHealthTests.swift` (same grouping table, decoding, old node / hub, model check/fix).

## Diagnosis and cleanup (#8037)

An alert on a machine card says what is wrong; the sheet it opens also says why and offers the action that fixes it. A launcher's alert shows the recorded error without terminal escapes or
the shell's startup noise, and the redacted line is already clean (`docs/server/launcher-health.md` "Checks"). For memory, CPU and disk the "why" comes from the computer
itself: the manager routes `GET /api/v1/machines/{mid}/diagnosis` and `POST /api/v1/machines/{mid}/cleanup` (`devices/diagnosis.go`, `full` devices only) call the node's
`machine.diagnose` and `machine.cleanup` (`node/machdiag/`, capability `machine_diagnose`). The wire is in [protocol.md](../protocol/protocol.md) ("Data types").

The diagnosis is a description, not a tunnel: it answers a fixed, bounded question, and nothing in a request names a path or a command. Sections and cache ids are closed
lists (`proto.DiagnoseSections`, `proto.CacheIDs`); the manager refuses anything else with 400 before the node is asked, and the node checks again.

| Section | What is read | Bounds |
| --- | --- | --- |
| `processes` | Linux: `/proc/<pid>/stat` of every process (name, CPU ticks, resident pages), read twice half a second apart for CPU; no process started. macOS: one `ps -axo pid=,rss=,pcpu=,comm=` (only the executable's last path element is kept). Grouped by executable name, tagged `agent` (claude, codex, opencode, pi, gemini), `engine` (the engines and tmux), `persona` (personad, persona-node, persona-link) or `other`; node processes cannot be attributed by name and stay `other` | The 15 largest groups by memory plus the 5 busiest by CPU. Names only: never arguments, command lines or environment, which may hold secrets |
| `disk` | The volumes the metrics add up (`sysmon.Volumes`), the folders directly in the home directory and the cache catalog, sized by allocated blocks | One filesystem (a mount under a folder is skipped), symbolic links never followed, hard links counted once, 4 walks at a time, 20 s in all: what is cut short is marked `partial`. At most 12 folders; paths leave the node as `~/…` |
| `tools` | The tool catalog, each found with `exec.LookPath` on the node's PATH and asked its version | 2 s a tool, 6 at a time, 8 s in all; the result is kept for 10 minutes. On macOS the `/usr/bin` stand-ins (`git`, `python3`, `swift`, `xcodebuild`, `java`) run only when the developer tools (`xcode-select -p`) or a Java runtime (`java_home`) exist, since running one otherwise opens an install dialog |

Windows answers `tools` only; the processes and disk sections are left out, and the web says the computer cannot show them.

**Cleanup** accepts ids from the cache catalog only. For each the node resolves the folder itself, every time, and empties it only when all of these hold: it is inside the
home directory and not the home directory itself; it is a real folder, reached through no symbolic link (its real path is the home directory's real path joined with the
same relative path); and no process of its toolchain runs (`in_use`; when processes cannot be read, every cache counts as in use). It removes the folder's contents and keeps
the folder; a link inside is removed as a link. Go's module cache is read-only on purpose, so a folder that refuses is made writable by its owner and tried again.
A cache that is not there or is empty is `missing`. What it freed is measured with `statfs` on the home volume before and after, never below 0. The built-in cleaner never touches
container data, projects, sessions or anything outside the catalog; deeper cleanup belongs to a storage extension (`machine_section`).

| Cache id | Linux | macOS | In use while running |
| --- | --- | --- | --- |
| `go-build` | `$GOCACHE`, else `~/.cache/go-build` | `$GOCACHE`, else `~/Library/Caches/go-build` | go, gopls, compile |
| `go-mod` | `$GOMODCACHE`, else `$GOPATH/pkg/mod`, else `~/go/pkg/mod` | same | go, gopls |
| `npm-cache` | `$npm_config_cache/_cacache`, else `~/.npm/_cacache` (not npm's logs or npx's installs) | same | npm, npx, node |
| `pip-cache` | `$PIP_CACHE_DIR`, else `~/.cache/pip` | `$PIP_CACHE_DIR`, else `~/Library/Caches/pip` | pip, python (any version) |
| `uv-cache` | `$UV_CACHE_DIR`, else `~/.cache/uv` | same | uv, uvx |
| `yarn-cache` | `$YARN_CACHE_FOLDER`, else `~/.cache/yarn` | `$YARN_CACHE_FOLDER`, else `~/Library/Caches/Yarn` | yarn |
| `homebrew-cache` | `$HOMEBREW_CACHE`, else `~/.cache/Homebrew` | `$HOMEBREW_CACHE`, else `~/Library/Caches/Homebrew` | brew |
| `xcode-derived-data` | none | `~/Library/Developer/Xcode/DerivedData` | xcodebuild, Xcode, XCBBuildService, SWBBuildService |
| `gradle-cache` | `$GRADLE_USER_HOME/caches`, else `~/.gradle/caches` | same | gradle, java |
| `cargo-registry` | `$CARGO_HOME/registry`, else `~/.cargo/registry` | same | cargo, rustc, rust-analyzer |

`~/.cache` is `$XDG_CACHE_HOME` when that is set. The variables are the node process's own; one that points outside the home directory takes its cache out of the catalog.
pnpm's store is left out: its files are hard-linked into projects, so emptying it frees little.

| Tool id | Command (first found) | Tool id | Command (first found) |
| --- | --- | --- | --- |
| `xcode` | `xcodebuild -version` (macOS only; a failure means no Xcode) | `gh` | `gh --version` |
| `docker` | `docker --version` (the client: the daemon is not asked) | `brew` | `brew --version` |
| `podman` | `podman --version` | `bun` | `bun --version` |
| `go` | `go version` | `pnpm` | `pnpm --version` |
| `node` | `node --version` | `uv` | `uv --version` |
| `python` | `python3 --version`, `python --version` | `tmux` | `tmux -V` |
| `rust` | `rustc --version` | `claude` | `claude --version` |
| `java` | `java -version` | `codex` | `codex --version` |
| `swift` | `swift --version` | `opencode` | `opencode --version` |
| `git` | `git --version` | `gemini` | `gemini --version` |

The manager waits 30 s for a diagnosis and 120 s for a cleanup. Errors are `{error, code}` with `error` in the reader's language (`devices-diagnosis`): unknown machine 404 `not_found`,
a bad section or id 400 `bad_request`, offline 503 `machine_offline`, a node without the capability 409 `node_outdated`, out of time 504 `deadline`, anything else 502 `diagnosis_failed`
(the node's own words are logged, not shown). A hub from before these routes answers 404 without a code, which the web reads as "update the hub".

Tests: `node/machdiag` (stat and ps fixtures, grouping and the top-N union, the walker's links, hard links, other filesystems and deadline, the catalog per OS and its variables,
in-use matching, the cleanup's guard rails on temporary home directories, the tool probe with fake commands), `node/diagnose_rpc_test.go` (dispatch),
`devices/diagnosis_test.go` (pass-through, capability, offline, bad input, roles, node errors), `hub/diagnosis_routes_test.go` (answer-only devices and extension tokens).

## Renaming a computer

A computer's name is its hub.json `name`; its id never changes. It can be renamed from the admin page, the web app's machine details (and, once PR 2 lands, the iOS / Mac / Android ones), the computer's
Persona Link (`persona-link name`, the Windows tray) — see [protocol.md](../protocol/protocol.md) "Renaming a computer" and [link.md](../operations/link.md) "The computer's name". The hub writes hub.json and takes it at once:
`hub/machine_rename.go` updates the entries it hands the manager and the name it shows (`Machine.name()`, never `cfg.Name`: the entry is read without a lock and stays as it was made); the manager keeps the machine
when only the name changed (`sameConfig` ignores it) and tells the link (`endpoints`'s `machine_name`, part of `linkSays`); a member's rename reaches the center through its record (`derivedName`).

## Switching an ssh computer to Persona Link

The one other live edit of an entry is its kind, from `ssh` to `link` and back (#10662; the steps and what runs on the computer are in
[link.md](../operations/link.md) "Switching from SSH"). Who does what follows the rule above:

- **The manager** owns the switch (`devices/link_switch*.go`). It holds a write-ahead record (setting `link_switch.<machine id>`) and
  runs the SSH probe, transfer, install and rollback with its own SSH access. It waits for the node over the new link and reports
  `link_switch` in each machine's snapshot. The Persona Link build it sends is found like a node build (`devices/connector_package.go`).
- **The hub** answers `devices.link_switch.begin` / `end` (`hub/link_switch.go`), because the entry, the pairing code and the
  credential are its. It compare-and-swaps the kind in hub.json, or in the manager's own list through `devices.machines.put`, and
  takes the change live, as a rename does (so the admin page's restart list does not count it). It makes or revokes the code and
  credential, and relays the admin page's `POST /api/v1/admin/machines/{mid}/link-switch` to the machine's manager.
- When the kind changes, the manager replaces the machine (`sameConfig` sees a change), which ends the SSH connection. The new
  machine then waits for Persona Link. Everything kept by id stays: pins, mutes, trust, engine choice, sessions, metrics, usage and
  `node.deploy.<id>`.

The iPhone / iPad client decodes the optional `link_switch` in `apple/Shared/Models.swift` and starts through `APIClient`; an absent
field is the older-hub gate. `MachineLinkSwitchStore` keeps one attempt per `MachineRef` across detail-page remounts and reconciles
the accepted answer with pushed/read machine snapshots. Settled attempts expire after two minutes; an uncertain answer never reposts
automatically, while a new explicit confirmation may follow the hub's unchanged eligibility. `MachineLinkSwitchView` is the
advanced-detail card; its expected-disconnect guard also suppresses the ordinary offline explanation and Reconnect while the switch
runs (#10671).

## API

See `docs/protocol/protocol.md`: `GET /api/v1/metrics` (Machines), `GET /api/v1/machines/{id}/metrics?range=1h|6h|24h` (detail), `GET /api/v1/machines/{id}/usage?range=1h|24h|7d` (token usage), `GET /api/v1/usage/days` and `GET /api/v1/usage/hours` (token usage history),
node → manager `metrics` event, `system` in `node.info` / `Machine`; hub forwards these routes unchanged to manager (`devicewire.MetricsRoutes`, `503 assistant_offline` if absent).

## Tests

- Go: `sysmon` fixtures (/proc/stat, meminfo, net/dev, os-release, cpuinfo, vm_stat, iostat, sw_vers, system_profiler in `testdata/`),
  `db.TestMachineMetrics` (downsampling, weighted averages, no points in gaps, cleanup), `devices.TestMetricGaps` (gap markers); `./scripts/test/test-server.sh`.
- Real collection: local test hub (fake HOME, separate database through SSH tunnel, one `kind: local` Mac and one SSH Linux), simulator paired to inspect Machines and details.
- Fake data: `scripts/mock/mock_hub.py` has 4 machines: server with high load over latest 25 minutes (CPU / memory / load alerts), Mac Studio disconnected for two hours that morning (gap),
  NAS offline for three hours. UI `MachinesPageTests`: open Machines from machine menu (offline NAS first, server alerts, MacBook **Needs you**), open server detail
  (Agents above charts, no **System & Monitoring** layer), switch to 24 hours, hold network chart, inspect system info, tap Agent into chat; offline detail (**Reconnect** on first screen).
  Ordering unit tests `MachineConcernTests`. **Show all** for 36 Agents in `MachinePagesTests.testMachineProjects`; **Redeploy** in `DeployButtonTests`.
- Screenshots: `-tab dev -openMonitor all` (Machines), `-openMonitor <machine id> [-openMonitorRange 6h|24h] [-openMonitorScrub 0.55]` (detail, holding at 55% of range);
  `-tab settings -openMachines YES [-openMachineDetail <machine id>]` enters through settings.
