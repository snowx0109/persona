# New-user onboarding

An onboarding path for ordinary users who do not know the command line: after seeing an ad and downloading the app (phone, computer, or web), how do they install it, connect, add a model, and get an answer to their first message? Related to #287.

Code baseline: origin/master `59e8bae9a`. All `path:line` references in this document were checked against that commit. The current-state research was a fresh walkthrough on 2026-11-06 (code reading + testing an isolated hub + web screenshots). Conclusions are in section 2; raw records are not committed.

## 1. One sentence and the mental model

**In one sentence**: today's first screen asks users to "Run `personad hub pair` on the hub machine," which ordinary users cannot get past. The goal is "install one thing, scan one code, tap a few times, and get an answer to your first message." If anything is missing, the UI shows a card saying "What's missing; tap here," not a technical error in English or a blank screen.

User-facing explanation (use the same passage on the first screen's "How does this work?", the website, installer, and documentation):

> **Persona has three parts.**
> - **hub**: your home switchboard. Install it on a computer or NAS that stays on. Your messages, settings, and memory live here, only on your own machine. One is enough for the whole household.
> - **Your computers**: where Persona does the work—reading files, running programs, and writing code. The hub computer automatically counts as one.
> - **Phone / web**: the remote control. See what it is doing and tap "Allow" before it acts. Scan once to connect; it reconnects automatically after that.
>
> You also need a **model**, Persona's brain. If you already use Claude Code, use it directly; if you have an API key, paste it in; if you have neither, we show you where to get one.

```
   Phone / web ──scan once──▶ hub (always-on machine) ◀──Persona Link── Your laptop
   (remote control)          storage, messages, dispatch                (does the work)
                                      │
                            This also counts as a computer
                                      │
                            Model (Claude Code or API key)
```

Terminology rules (enforced by the copy guard tests in section 8.1):

- Onboarding, empty states, and error cards must not mention node, herdr, launcher, TLS, token, hub.json, zsh, ports, IPv6, `~/.` paths, or command names. Put them in "Details" or "Advanced" when needed.
- For ordinary users, call launchers / providers "models" (which provider, which key). Put run mode, kind, protocol, and Bearer / x-api-key in "Advanced".
- The user decides what hub is called in the UI ("hub", "host", "center"; decision 13 in section 6.3). Until then, keep "hub" and explain it as "your home switchboard" on first use.

## 2. Current state: the path new users face today

| Step | What users must do today | Where they get stuck | Evidence |
| --- | --- | --- | --- |
| Get the app | iOS is TestFlight-only and requires iOS 26+; Android requires sideloading an APK; the website FAQ says "Write to us if you'd like to try it" | The first step excludes most people | `site/public/index.html`, README "Platforms" |
| Install hub | `install.sh --hub` only installs a binary; it does not create Postgres, install a service, or start it. It ends with the English instruction `personad hub run … then personad hub pair` | Starting in an empty directory exits immediately: `database_url is empty (run scripts/deploy/deploy.sh …)`, pointing to a repository script the user does not have | `scripts/deploy/install.sh:421`, `server/internal/db/db.go:37` |
| First pairing code | All four platforms' first screens say "Run `personad hub pair`"; the web login page also teaches `jq -r .token ~/.persona/hub.json` | Users without a terminal stop here. `/admin` can generate a code, but setting its password locally requires a one-time setup code from the logs | `web/src/views/Login.tsx:45-48`, `server/internal/hub/admin_setupcode.go:51-52,75-79` |
| Address in the QR code | The default `public_url` is `http://<full-hostname>:18660` | Phones often cannot resolve the hostname; changing the port does not update it either | `server/internal/hub/config.go:215` |
| Add a computer | Only possible in `/admin`; phones are explicitly prohibited from generating computer codes. New hubs do not enable TLS, so "computer connects to hub" always returns 409 `tls_required`. Mac Persona Link must be built from source | Ordinary users mostly stop here | `hub/pairing.go:132-135`, `hub/admin_machines.go:307-310`, `docs/operations/persona-link-install.md` |
| Main chat | The first screen looks usable; failure comes only after sending the first message | No computer: 502 `{"code":"internal","error":"assistant machine is not configured"}`, with a web toast that disappears after a few seconds. No claude installed: `zsh:1: command not found: claude` after about 6 seconds. Not logged in: an ordinary English bubble, "Not logged in · Please run /login" | `assistant/start.go:29,157-158`, `web/src/views/PersonaAssistant.tsx:65`, `assistantwire/handler.go` (`APIErrorMarkers`) |
| Model | Providers, launchers, purposes, kind, protocol, and Bearer / x-api-key are all exposed. A fresh hub has no presets (`{"presets":[]}`). Names must be English. After adding a keyed provider, main chat and Agents still use "Claude Code default" | Adding a key does not connect it; "Import from ~/.zshrc" spins forever with zero computers | `assistant/models_presets.go:20-28`, `assistant/models_launchers.go:98` |
| Empty states | With zero computers, Overview says "Everything is ready"; Machines says "Once hub connects…" but offers no "Add computer". Assistant settings render as "Each chat runs in the background in [blank] on [blank]" | Misleading, with no next-step button | `web/src/i18n/locales/zh-Hans/home.json:31`, `machines.json:25` |

Facts that determine the design:

- **Main chat requires an online computer with Claude Code installed and logged in.** Even a direct-API provider used for main chat runs claude on a computer against its gateway (`assistant/models.go:950-982`: `finishModel` accepts only `launcher.<name>`). The Docker image has neither claude nor a local computer (`Dockerfile:79-81`, `PERSONA_LOCAL_NODE=0`), so main chat cannot initially run for NAS / Docker users.
- **No endpoint currently answers "Can main chat work now, and what is missing?"** `POST /api/v1/assistant/warm` returns `{ok:true}` whether it succeeds or fails (`assistant/profile.go:283-289`). Launcher health checks default to off (`assistant/launcher_health.go:136`), so they cannot assess readiness for new users.
- **Most parts already exist**: one-time pairing codes and hub-rendered QR codes (`hub/pairing.go:97`, 10 minutes, single-use), "Connect new device" on web and iOS, `/admin`'s add-computer flow (takes effect live, `hub/devices_handover.go:202`, `addMachineLive`), self-signed TLS, one-word launcher tests (`assistant/models_launchers_routes.go:242`), connection tests before saving (`assistant/models_probe.go:358`), adding from presets (`presetFill`), changing purposes (`assistant/models.go:821`), and Android's readiness checklist (`ReadinessRecords.kt`). What is missing is connecting them, making them available to logged-in apps, and providing machine-readable error codes.
- **The iOS `"ccc"` fallback** (`AssistantView.swift:738,921`) appears only when reading `GET /api/v1/assistant` fails. It is not a normal blocker; fix it alongside the work.
- **The official extension marketplace is inaccessible**: the research-time 403 was probably caused by the development machine's network. Fix graceful degradation when it cannot be read, rather than treating failure as the default.

## 3. Target path

### 3.1 Ideal shortest path (the direction to work toward)

Prerequisite: a signed and notarized Mac download containing hub + local computer support + database; see decisions 1 and 2 in section 6.3.

| # | User action | Persona action |
| --- | --- | --- |
| 1 | Click "Download for Mac" on the website, drag into Applications, and open | — |
| 2 | On the first screen, choose "Use this Mac as hub" and tap "Start" | Create the database, write hub.json (new defaults in 4.1), and install a LaunchAgent. Automatically count this machine as a computer (`kind: local`, `hub/config.go:208`); detect local Claude Code |
| 3 | Tap "Allow" when the system asks for local network access | LAN broadcast (mDNS, only with TLS enabled, `hub/mesh_discovery.go:26`) |
| 4 | Scan the on-screen QR code with the iPhone camera | One-time pairing code; if the app is not installed, land on the website download page |
| 5 | The app opens and connects automatically | Deep-link pairing (already wired on three platforms) |
| 6 | "Connect a model": if logged-in Claude Code is detected, show only "Use this"; otherwise choose a provider and paste a key | Section 4.3 |
| 7 | Tap "Try a message" | Run one real main-chat turn; on success, locally display a welcome and three small tappable tasks |

About 8 taps, without the command line.

### 3.2 Starting points by "What do I have?"

| What I have | Recommended hub location | How far this round gets | Blocker |
| --- | --- | --- | --- |
| One Mac + phone only | That Mac | Demo mode + an honest "No hub yet?" page; users with Docker Desktop / OrbStack can paste one command and run `personad hub setup` | Developer ID notarization; database without Docker (decisions 1, 2) |
| Mac + always-on Linux / NAS | Linux or NAS | Paste one command (Linux) or download compose (NAS) → scan code → "Add computer" in the app → install Persona Link on Mac (currently built from source) → connect a model | Persona Link installer (decision 1) |
| Phone only; wants a preview | No installation | Demo mode (already on iOS / Mac / Android); exiting lands on "No hub yet?", not back at the pairing screen | — |
| Work computer (cannot install software; managed computer) | Home Linux / NAS or cloud server | Hub can be installed and phone connected; main chat needs a computer that can install Claude Code, otherwise wait for "Chat only" mode | "Chat only" mode (decision 9); cloud templates (decision 6) |
| Developer already using Claude Code, with always-on Linux | That Linux machine | One command → scan code → automatically recognize logged-in Claude Code and chat immediately; `~/.zshrc` launchers still work | — |

### 3.3 Where hub lives: trade-offs

| Location | Benefit | Cost (plain-language text under the option) |
| --- | --- | --- |
| Always-on Linux desktop / mini PC | Online 24 hours; it is the first computer, so users do not need to "Add computer" | When it shuts down, the phone cannot connect |
| NAS (Docker) | Already at home and always on | The Docker image does not include claude. AI does not do work on the NAS; another computer must be added |
| User's own Mac | People with only one computer do not need another machine; it is also the work computer | Closing the lid puts Persona to sleep too. On wake, catch up on messages and scheduled tasks; push notifications still use the relay (`docs/operations/push-relay.md`) |
| Cloud server | Connect from anywhere, any time | Requires buying a server; data lives in the cloud. Without a computer, wait for "Chat only" |
| Windows | — | Cannot be hub (#1179); a Windows computer can only be one of "your computers" |

Recommendation for the website and installer: **if you have an always-on computer, install hub there**. A laptop can be hub, but the installer must explain what happens when the lid closes.

Away from home: no hole punching or relay tunnels in the first version (decision 7). After pairing, run one "Can you connect away from home?" check (Android already has a cellular-network test). If it fails, say "You can use it on your home Wi-Fi. Away from home, use a VPN or enable IPv6 on your router (illustrated instructions)."

## 4. Design

### 4.1 Installer: `personad hub setup`

Same namespace as `hub pair` and `hub admin`. Implemented in Go, with no dependency on the repository or Python. `install.sh --hub` calls it after installing the binary (replacing the English text at `install.sh:421-423`).

```
personad hub setup [--yes] [--listen :18660] [--tls :18662] [--outside] [--no-service] [--manager-service] [--json]
```

Run in order, printing one localized conclusion per step; on failure, provide the next action and stop:

1. **hub.json**: if missing, write a complete new file (new defaults below). If present, add only fields such as `token` that are required to run. **Never change existing values**.
2. **Database**: if `database_url` is empty and Docker is available, create a container following `scripts/deploy/db.sh` (same `persona-postgres` name, image, and volume, still recognized by `deploy.sh`), generate a password, and write it to hub.json. Without Docker, probe local Postgres (the #321 approach). If neither exists, stop and print "This computer still needs a database: install Docker Desktop or OrbStack, then rerun this command." Also change the error at `db/db.go:37` to "Run personad hub setup first."
3. **Persistent service**: on Linux, install socket-activation units following `hub-systemd.sh` (the same units, not a second set). If `loginctl enable-linger` needs sudo, ask once and explain why. On macOS, write a LaunchAgent (new implementation; `hub-systemd.sh` currently only finds existing ones). If another hub occupies the port, automatically choose another and save it.
4. **Start and self-check**: wait for `/api/v1/health`, have the built-in manager bound and installed as a service of its own (step `manager`, below), run doctor, and print only problematic items; do not print excluded virtual network interfaces.
5. **Finish** (localized): a pairing QR code for the phone (reuse `hub pair`'s terminal QR code); "Open `http://<LAN-IP>:18660/admin#setup=<setup-code>` in your phone browser to set the administrator password." The installer obtains the setup code through loopback + the main token with `POST /api/v1/admin/setup-code` (`admin_setupcode.go:94-99`), without asking the user to inspect journalctl. `#setup=` is a fragment, not sent to the server; the admin page reads and prefills it.

**Write new-install defaults only into new files; never add them to `defaultConfig()`.** `LoadConfig` deserializes an existing hub.json over `defaultConfig()` (`hub/config.go:241-278`). `Public` is a value type (`config.go:169`), so adding `TLSListen` to `defaultConfig` would enable TLS for every old hub.json that omits `public`. Also, `db.sh` writes a hub.json containing only `database_url` before hub's first run, so the file already exists in the deploy.sh path. Approach: explicitly write these fields when `hub setup` creates a file, or only in `LoadConfig`'s `os.IsNotExist` branch. Both share `newInstallConfig()`, with a golden test proving that loading old files gives the same result.

Fields written to a new file:

| Field | New installation | Why |
| --- | --- | --- |
| `public.tls_listen` | Bind only the LAN IPv4 address by default: `"<LAN-IP>:18662"`; change to `":18662"` only with `--outside` or when "Use away from home" is enabled later in the app (decision 8) | Persona Link requires TLS (`linkPairable`); mDNS broadcasts only with TLS. Binding only the IPv4 LAN address avoids exposing the port on public IPv6. `tls_listen` accepts an address (`public.go:84` uses `listenTCP`; `mesh_discovery.go:39` parses host). DHCP may change the LAN address; if binding fails at startup, hub must fall back and notify the app. Add this during implementation |
| `public_url` | `http://<LAN-IPv4>:<listen-port>`; for multiple interfaces, choose the default-route interface | Currently uses the full hostname (`config.go:215`), which phones often cannot resolve |
| `admin.first_setup` | `"code"`: setting the first administrator password from the LAN also requires a setup code | Currently only loopback requires a code (`admin_setupcode.go:52`), so someone else on a shared LAN could set the password first. Old hub.json files omit this field and keep their behavior |
| `machines` | Keep local `kind: local` if herdr or claude exists; remain empty in Docker | Same as today |

Implementation (#3349): `server/internal/hub/setup.go`, `setup_service.go`. Only `hub setup` writes the new defaults (`LoadConfig` still writes `defaultConfig()` on first run; Docker image startup uses `deploy/docker/entrypoint.sh` to write container defaults separately—see "Implementation (#4045)" below). A "new file" means a hub.json without `token` (missing, or prewritten by `db.sh` / `hub admin access`); fill missing fields from `newInstallConfig()` and leave existing fields alone. `--outside` is the only option that changes an existing value (`public.tls_listen` becomes `:<port>`). Without Docker, check for Postgres on port 5432; if present, ask the user to put its address in `database_url` (port check only; connecting to local Postgres and creating the persona user and database as in #321 is follow-up work). If the default ports are occupied, a new installation uses the pair +100 (18760 / 18762). If the LAN address bound by `tls_listen` disappears, hub starts normally without TLS and logs a line. The socket unit has `FreeBind=true` (also updated in `hub-systemd.sh`), so listening still starts under socket activation; hub logs a line if the address is not local (the app notice is follow-up work). The Mac LaunchAgent is `com.persona.hub`; if another agent already runs `<root>/bin/personad hub`, use it (like `hub-systemd.sh`, resolve symlinks and `~` in `ProgramArguments` for comparison; also recognize `~/.persona` pointing to `~/.persona`). If hub is already running without an agent (for example, in a terminal), do not install an agent that cannot acquire the port; ask the user to stop it first. Restart hub when `bin/personad` differs from the binary used to start the service last time (`install.sh --hub` replaced it on upgrade; recorded in `bin/.personad.setup-sha256`): Linux uses `restart personad`, seamless because the socket owns the port; Mac uses `launchctl kickstart -k`.

Implementation (the hub/manager split): with services (not `--no-service`, not in a container; Linux with a systemd user instance, or a Mac), a new `hub.json` gets `assistant.mode: service`, and the `manager` step after the start runs `<root>/bin/personad manager install --builtin --join http://127.0.0.1:<port>` with a one-time code in its environment. The manager's own program binds it and installs `persona-manager-assistant.service` (Mac: `com.persona.manager.assistant`); the hub writes none of its files. Then the step waits for it on the hub's bus. An existing `hub.json` keeps its mode, and the step only says how to switch (`--manager-service`, which checks first and undoes a switch it cannot finish). Details: [managers.md](managers.md) ("The built-in manager as its own service").

**Docker / NAS**: does not use `hub setup`. Keep compose; a separate small PR makes the entrypoint generate missing `.env` values on first run (random database password, PUID / PGID) and maps the TLS port in `compose.yaml` (the image currently only has `EXPOSE 18660`, `Dockerfile:83`). NAS instructions must explicitly say "You also need to add a computer."

Implementation (#4481, #4484): select the final admin UI address from hub's actual `listen`. If it listens only on loopback (`127.0.0.1` / `localhost` / `::1`), print a loopback address, explain "Only accessible on this computer," and show how to make it accessible from the phone (include the actual hub.json path, change its `listen` to `:<port>`, and rerun `hub setup`); the final `--json` line adds `admin_local_only: true`. If bound to a specific address, use that address; use the LAN IP only when listening on all addresses. On reruns, explain "Existing values are not changed" for `--listen` / `--tls` only if explicitly supplied and different from hub.json, showing both values. `--tls :<port>` and the new-install `LAN-IP:<port>` count as equivalent. Commands in the admin UI, hub logs, `hub doctor`, and usage / errors for `hub admin` / `hub peer` all use `personad` (the `personad` alias still works; guarded by server `command_name_test.go` and web `web/src/admin/command-name.test.ts`). The systemd unit remains `personad`; where the admin UI mentions `journalctl --user -u personad`, explain "The service retains its old name." The admin extension page matches **Settings › Extensions**: when the official marketplace is unreadable, show muted text "Official extensions are temporarily unavailable. Try again later." (reason on hover). When the marketplace is unreadable, the catalog must not say "No extensions here yet."

Implementation (#4045): generated values are **not written back to `.env`**. Compose reads `.env` on the host before the container starts. Instead, compose no longer requires `PERSONA_DB_PASSWORD`, `PERSONA_PUBLIC_URL`, or `PUID` / `PGID` (only `PERSONA_IMAGE` remains required). If no database password is set, the postgres entrypoint (a wrapper in compose) generates one on first startup and stores it in `data/docker/db-password` (postgres mounts only this subdirectory). The image's `personad` wrapper reads it on every invocation (including `docker compose exec`) and fills it into the database URL. Without `PUID` / `PGID`, use the owner of `data/` (1000 if owned by root). When `data/` has no `hub.json`, the entrypoint writes container-specific new-install defaults (`public_url: ""`, `public.tls_listen: ":18662"`, `admin.first_setup: "code"`, `machines: []`, without a token, which hub fills on first read). It does not use `newInstallConfig()`: the container cannot obtain the NAS's LAN address. `public_url` only recognizes `PERSONA_PUBLIC_URL`; when empty, both the entrypoint and `hub pair` explicitly ask for it. Compose maps `${PERSONA_TLS_PORT:-18662}:18662`; the image has `EXPOSE 18660 18662`. Deployments with existing `.env` / `hub.json` retain their behavior (`scripts/test/test_docker_entrypoint.py`). See `docs/operations/docker.md` ("Install").

**All-in-one Mac** (after decisions 1 and 2): add "Use this Mac as hub" to Persona Link, calling `personad hub setup` internally. After installation, the local machine is `kind: local`, and the window displays `qr.rows` returned by `pair-codes` directly. Whether to combine it with Mac app `PersonaMac` into one download is a separate decision (decision 3).

### 4.2 Pairing

**Principle**: initial trust comes only from a channel the user sees firsthand: scanning the on-screen QR code, pasting a link they copied themselves, or the hub's local terminal. Never put long-term credentials in a QR code, link, or log; use only one-time codes (`jp_…`, 10 minutes, single-use).

#### Phone / web ↔ hub

| Scenario | Source of the first code |
| --- | --- |
| Just ran `hub setup` | Terminal QR code; or open the printed `/admin#setup=…` in a phone browser, set a password, then "Pair phone" |
| Already has a logged-in device | **Settings › Devices › Connect new device** on that device (already on web and iOS; Android added this round) |
| Same LAN, hub has TLS enabled (P1) | "Find nearby hubs" on the app's first screen browses `_persona-hub._tcp` and lists "Home hub 'xxx'". **mDNS provides discovery, not trust**: broadcast `fp` cannot establish initial trust (`docs/operations/network.md:75`). A one-time code is still required after discovery; the benefit is telling the user "Hub is installed; here is where to get a code" |

The web login page (`Login.tsx:45-48`) prefers "Scan with a logged-in phone to sign in" (existing `#token=jp_…` web login code), followed by "Paste pairing link". Put the main-token field and `jq` / `--legacy` in "Advanced," labeled "This is hub's master key." When signing in again in the same browser, reuse or replace the old device, with a date in the device name, rather than accumulating identically named "Web (Chrome · Linux)" entries. Implementation (#3356): web sends `replaces` (the previous device ID) when exchanging credentials; hub revokes only that `kind: web` device (`docs/protocol/protocol.md`, "Authentication"). "No hub yet?" is an explanation within the page; web cannot obtain the website URL.

#### Your computers ↔ hub

This round's goal: **"Add computer" in the app is enough**, without `/admin`, enabling TLS first, or manually adding it beforehand.

1. A logged-in full device calls `POST /api/v1/auth/pair-codes` with `add` (section 5.3). Hub adds a `kind: link` entry using the same logic as `/admin` (extract a shared function; `addMachineLive` needs no restart), and returns the computer pairing link.
2. The computer stays **pending** until the user taps "This is my computer" in the app. Why: Persona-managed API keys currently go to "every node" (`assistant/models_launchers.go:14-18`; `pushManaged` sends to every online machine, `assistant/models_launchers_push.go:84-112`). When a Docker hub has no local computer, `defaultAssistantMachine` falls back to the first machine (`hub/assistant_profile.go:117-118`). Without confirmation, a forwarded or screenshotted link would leak keys and run main chat on someone else's computer. A pending computer:
   - receives no Persona-managed keys (`pushManaged` filters it);
   - is excluded from assistant-machine selection (`defaultAssistantMachine` skips it; assistant `effective()` rejects it too, `assistant/profile.go:139-142`);
   - is absent from computer pickers for new Agents, dispatch, and to-dos;
   - is automatically deleted after 24 hours without confirmation (both hub.json entry and Postgres record).
   Confirmation belongs on the "Waiting for this computer to connect" screen. Once connected, show "A computer wants to join: <name> (macOS). Is it yours?" with one button, not another screen.
3. **Compatibility**: store trust state in a new Postgres table; **no record = confirmed**. Existing machines, ones added through `/admin`, and ones added through loopback + main token (`hub pair --machine`) therefore remain confirmed on upgrade, with no behavior change. For old apps, a pending machine's `error` says "Confirm in a newer app or on the web."
4. **Visible receipts**: when a device generates a computer code, a new device redeems an app code, or a new computer connects, notify other paired devices with "Not you? Revoke" (revocation endpoint already exists). Today this only appears in hub logs (`pairing.go:187`).
5. Send the link to that computer, with a screen for its OS:
   - **Linux**: a single command containing the link (`install.sh --node --pair '<link>'`, already exists), "Copy" + "Open a terminal on that computer, paste, and press Enter."
   - **Mac**: "Download Persona Link" (before decision 1, link to `docs/operations/persona-link-install.md` and honestly say "Currently requires building it yourself") + "Send this link to that computer" (system share). Persona Link registers both `persona` and `persona` URL schemes; opening a link shows and prefills the pairing window (today `apple/PersonaLink/Info.plist` has no URL scheme). Hub still sends `persona://link?…` (`pairing.go:57`), recognized by old Persona Link and node.
   - **Windows**: provide commands from `docs/operations/windows.md`; explain the tray app is still in development (#299).
6. `personad hub pair --machine <id>`: if the machine does not exist, `--add [--name]` adds it automatically; without it, ask "Add --add," rather than returning 400 `hub.json has no machine …`. Update Persona Link's pairing window and `install.sh` instructions accordingly.
7. Old hubs without TLS: 409 `tls_required` + action `enable_tls`; the app says "Hub has not enabled secure connections, so computers cannot join." If managed, enable and restart in one tap.
8. **Member hubs in linked hubs**: app codes are currently forwarded to the primary hub (`pairing.go:161-164`); the computer flow is undecided. This round, adding a computer from a member-hub device returns 409 `mesh_member`, "Add it on the primary hub." New users will not reach this.

P1: **computer initiates, phone allows**. Persona Link's first screen shows a QR code with its one-time public key and LAN address. The phone scans it and sends the full link to the computer over LAN, avoiding manual transfer of a long link. Do not implement "phone shows a 6-digit code; computer enters it": a short code needs LAN discovery to find hub, and mDNS can be spoofed by someone on the same network.

### 4.3 Connecting a model

#### No dependency on zsh

- Do not mention "~/.zshrc" in onboarding. Move "Import from ~/.zshrc" to "Advanced" in **Settings › Model providers**; disable it with an explanation when there are no computers (today it spins forever).
- Existing developer aliases / functions are still discovered and selectable (`mergeLaunchers` in `node/launchers.go`); the onboarding wizard does not depend on them.
- The default `claude` launcher currently always starts through `zsh -ic` (`assistantCLICommand` in `node/assistant_start.go`), causing `command not found` for users whose PATH is in `.zprofile`. Prefer the absolute path found by `claudeExecutable()` (`node/managed.go:104`), then fall back to the shell. This overlaps with #2854 (bash launchers / bundled claude, claimed by someone else); coordinate in #2854 first rather than duplicating the work.

#### Detect first, then ask

After pairing (or when main chat's first screen lacks a model), check readiness first (section 5.1). Users see conclusions:

| Detected | User sees | Button |
| --- | --- | --- |
| Online computer, Claude Code installed and logged in | "Claude Code on your MacBook is ready" | "Use this" → try a message |
| Installed, not logged in | "Claude Code is not logged in yet" | "I have an API key" / "How to sign in" (illustrated: open Terminal on that computer, enter claude, and follow the sign-in instructions; the only step this round still requiring a terminal) |
| Computer exists, Claude Code not installed | "This computer does not have Claude Code yet" | "How to install" (official instructions); P1 "Install for me" |
| No computer | "Add a computer first; once connected, it can do the work" | "Add computer"; P1 "Chat only for now" if a key is available |
| A working keyed provider already exists | Skip this step | — |

Free sign-in detection: add a node RPC running `claude auth status --json` (no model request). Two caveats:

- It must run in **the launcher's actual environment**: the default launcher uses the same shell (`zsh -ic`) or absolute path; Persona-managed launchers use the environment of `personad launch <name>`. Testing with an empty HOME showed that an environment API key makes it report `loggedIn: true, authMethod: api_key`. Running outside the actual environment can falsely report "ready" or "not logged in."
- Node does not currently use this command anywhere. Check versions; an old version without the subcommand, or unreadable output, reports `unknown`, not failure. With `authMethod: api_key`, display "Using a key from the environment," record the status as "Unverified," and wait for a test message.

#### Three routes

**A. Already has Claude Code**: do not change configuration (the default is `claude`, `assistant/profile.go:148-150`); try a message directly. P1: node runs a headless sign-in command such as `claude setup-token` and pushes the sign-in link to open on the phone. Test this first and confirm this use of subscription sign-in complies with Anthropic's terms (decision 10).

**B. API key only**:

1. Choose a provider (built-in presets below), paste only the key; generate the name automatically, allowing a Chinese display name.
2. Test automatically (`POST /api/v1/launchers/test`; if no model is supplied, use the preset's default rather than requiring a model). On failure, retain the existing plain-language explanations (401 / 403 / 404 / 429 / timeout, `server/internal/i18n/locales/zh-Hans/assistant-models_probe.json`) and add a "Where to get a key" link. Testing costs a little money; keep the existing notice.
3. Include `assign` on save (section 5.4): **change main chat / Agents only if they are still at their untouched defaults**, and return `assigned`. The wizard's final screen explicitly says "Main chat and new Agents will use this." If readiness reports working Claude Code, default this switch to off; do not silently replace a working default.
4. Computer exists, Claude Code missing: say honestly "Your key is saved. Install Claude Code on the computer before main chat can use it," because `finishModel` still starts claude on a computer. P1: node's `claude.install` (follow `herdrinstall.go` for download, verification, installation to `~/.local/bin`, and progress reporting), triggered only by the user's button tap. Accept download URLs only on official domains; report `download_blocked` if downloading fails on networks in China.
5. No computer: P1 "Chat only" mode (4.4) uses this key directly.

Implementation (#4482, `assistant/models_probe.go`): distinguish Anthropic regional blocking within 403 responses (response `message` is `Request not allowed`, `error.type` is `forbidden`; blocking happens before the key is checked. If `error.type` is empty, count it only when the address is the official Anthropic endpoint; the same generic phrase from another gateway remains an ordinary 403). Change the copy to "Anthropic's official service is unavailable in your current network's region; changing your key will not help," followed by the built-in providers not limited to overseas service (names in the request language), without quoting the gateway's raw wording. Test and model-list responses add `code: "region_blocked"`; the three clients currently only show the copy (the wizard already lets users go back and choose another provider). A code-based switching entry point is follow-up work (#4526). Anthropic `permission_error` and other providers' 403s retain "No permission (403)" plus the original message.

Web implementation (#4037, `views/settings/ProviderWizard.tsx`): "Working Claude Code already exists" requires `ok` for all three readiness steps: computer, Claude Code, and model (an untested `unknown` model does not count; a new key still defaults to assignment). Hide the switch until the first result arrives; saving also waits for it before deciding whether to include `assign`.
For the step 4 notice, check readiness both before saving and after refetching: show it if Claude Code was `claude_missing` in either result and is not `ok` after saving. Checking only after saving is insufficient: once main chat switches to a Persona-managed provider, hub returns `unknown` for this step (`readiness.go` asserts absence only for the default `claude` launcher).

**C. Has neither**: one screen, "Where to get a model." Sort built-in presets by request language (Chinese lists providers available directly in China first; other languages list Anthropic first). Give each a one-sentence explanation and "Sign up" (open `signup_url` in the in-app browser); on return, proceed to B. Beside it, offer "Try demo first." No affiliate links; recommendations and order are decision 11.

Android implementation (#4297; details in `docs/android/android.md`, "Onboarding: models, devices, computers"): when hub has `builtin_presets`, use the same three steps as web. Differences: "Where to get a key" opens `signup_url` in the system browser (Settings has no in-app browser); while readiness is pending, the switch area says "Checking…" and saving waits for the result. Old hubs retain "Enter manually / Add from extension"; connection tests and fetching model lists exist only in the custom-gateway form.

#### Built-in presets

- Add an embedded presets_builtin.json under `server/internal/assistant/`, using `assistantwire.LauncherPreset` (`assistantwire/models.go:61-75`), `plugin: "builtin"`, and IDs `builtin/<vendor>`. Add two optional fields: `signup_url`, `name_zh` (or an i18n key).
- `launcherPresets()` (`models_presets.go:20`) returns "built-in + plugins"; a plugin overrides a built-in with the same ID. Leave `presetFill` unchanged.
- Initial candidates: official Anthropic, DeepSeek, Kimi (Moonshot), Zhipu GLM, Alibaba Bailian, and MiniMax's Anthropic-compatible endpoints. **Addresses and model names were not verified while drafting**. Before adding any provider, check its official documentation and pass a one-word request test, with sources in the PR. Include only vendors' public addresses, not relay services or company gateways. Tests ensure every entry has all required fields.
- The first wizard step on all three clients lists presets directly, with "Custom compatible gateway" last. Base URL and authentication method appear only in the custom flow. Replace hardcoded "Official Anthropic" in client wizards (`web/src/lib/providers.ts`, `ProviderWizardModel.swift`) with preset data. Add "Test connection" and "Get model list" on Android (currently does not call `/launchers/test` or `/discover`).

### 4.4 Main-chat readiness and degradation

Hub computes main-chat status (`mode` in `GET /api/v1/assistant/readiness`, section 5.1); all four platforms present it consistently:

| `mode` | Conditions | First screen | Sending |
| --- | --- | --- | --- |
| `full` | Confirmed online computer + Claude Code can start + no model error | Current appearance; example questions reflect connected capabilities | Normal |
| `setup` | Missing computer, offline computer, missing Claude Code, not logged in, or invalid key | Keep the greeting. Above the composer, show one card for only the first missing step (title + one-sentence reason + one button). Replace example questions with "What can Persona do?" and "How do I connect a model?"; taps open local help, not a model request. Stop saying it "can also do work on your computers" | Allow typing and preserve the draft. Replace Send with "Connect a model first" (or "Add a computer first"), opening that step. **Do not queue messages**, avoiding a flood of late answers once setup finishes |
| `chat_only` (P1, decision 9) | A keyed provider hub can call directly, but no usable computer | Persistent top notice: "Chat only for now. Connect your computer so Persona can do the work" + "Add computer" | Hub answers through the API directly |

This round, hub returns only `full` / `setup`. Add `chat_only` to the protocol now; clients handle it as "top notice + normal sending" so P1 will not require another app release.

Failures are cards, never toasts:

- **Failure discovered after sending** (stale readiness): pin a card below that message (copy from 4.6 + button + "Details"); offer "Retry" on the message.
- **Computer asleep / offline**: "Your MacBook is offline, possibly asleep. Open the lid to continue." Do not describe this as failure to connect to hub (Android currently does).
- **Model errors**: Claude Code's synthesized replies such as "Not logged in" and "Invalid API key" no longer appear as ordinary bubbles. Show a "Model not connected: not logged in / incorrect key" card + "Fix it." Add `Not logged in` to `APIErrorMarkers` (`assistantwire/handler.go:123-124`); use the existing `user` for `turn_error_kind` (`proto/session.go:271-275`; `auth` is a health-check category, not a turn kind). `CutOff` also drives `TurnError` in hub's session list and scheduled-task / to-do handling (comment at `handler.go:118-122`), so this PR needs scheduled-task and to-do tests, not just a main-chat card.

**Scheduled tasks**: new hubs enable a weekday 09:00 morning briefing by default (`assistant/schedule.go:212`); with zero computers, it will always fail. Change this to skip while readiness is not `full`, record "Skipped: computer or model setup is incomplete," and send no failure notification. Mention it in main chat before the first run after setup. Do not change the default itself; old hubs are unaffected.

Differences and additions in the implementation (#4253, `assistant/schedule_ready.go`):

- Rather than "skip whenever `mode` is not `full`," skip only if the step blocking `mode` is needed by this run. A follow-up sent to a chat on another computer does not wait for the assistant's computer. A task using another launcher waits only for its computer, not main chat's Claude Code or model. This preserves tasks that can run even in `setup` mode (anything that ran before can still run).
- The run record has `status: skipped` and an `error` saying "Computer or model setup is incomplete; skipping this run" (same phrasing as "The previous turn is still running; skipping this run"). No push notification or retry after 3 minutes. Do not block "Run now" or one-time tasks (reminders and assistant-created follow-ups): they disable themselves at their due time, so skipping would lose them. Send as before, and on failure push "Scheduled task failed" and retry once.
- Use the non-probing readiness path (node's two checks are cached), wait at most 12 seconds for node, and spend no tokens.
- The "mention" is a new `notice` entry in main chat's activity feed (`{"text"}`, `docs/protocol/protocol.md`, "Tasks and chat activity"). Hub writes the copy without model cost. Emit it only after a skip caused by a step requiring user setup or repair (`todo`, login, key, or model problems), and only once (persist `routines_waiting` in `assistant.readiness`, surviving restarts). Sleeping / offline computers (`node_unreachable`) still cause skips, but do not set the flag, avoiding "Setup complete" on every wake. None of the four clients renders `notice` yet (unknown entries are skipped); each needs one centered line of small text. Until then, users can see skips and recovery only in run history.

**After setup**: when "Try a message" succeeds, main chat shows a locally written welcome (no model cost) and three small tappable tasks, chosen from connected capabilities: "Find the largest folders on this computer," "What's on my calendar today?" (request phone calendar permission only on tap), and "Help me connect email."

Web implementation (#3847; details in `docs/web/web.md`, "What main chat still needs") differs as follows: the message that failed stays in the chat as a failed bubble with Retry and Delete, as in the apps (#1122), and the card's Retry retries it. `add_computer` opens the Machines page's add flow in place when hub has `machines_add`; old hubs show an explanatory dialog with "Open admin UI" (hub's `/admin/machines`). `confirm_computer` opens that pending computer's confirmation UI in place (#4037). The zero-computer Overview empty state is separate work in #3729.

Android implementation (#4032; details in `docs/android/android.md`, "What main chat still needs") differs as follows: failed-send cards appear only when starting main chat is rejected (409 `no_machine` / `machine_pending` / `claude_missing` / `launcher_missing` / `not_logged_in` / `auth_failed`). The message remains "Not sent," with a card pinned below, the step's button, and "Retry" (refetch readiness first; if still incomplete, open that step without resending). These messages previously stayed queued forever; after hub rejects them, creation is no longer retried. Before item 16 in 7.2 merges, `add_computer` shows an explanation pointing to web **Machines › Add computer** (from #4297 it opens the in-app add-computer page, and `confirm_computer` opens that computer's confirmation; see "Android implementation" after 5.3 below). "Try a message" uses `?probe=1`, like web, without opening a real main-chat turn. The full-page checklist after first pairing has no navigation; `connect_model` / `edit_provider` opens a "How to connect a model" explanation (no "Connect model" or "Use an API key" buttons, only "Got it"); `confirm_computer` has no button. Only "Skip unfinished items" sends `dismiss`; "Done" does not.

#### "Chat only" mode (P1, a separate issue, decision 9)

For people with only an API key and no computer (Docker / NAS / work computers that prohibit installation), main chat is permanently empty today. This round provides a guidance card rather than a blank dead end; P1 adds a limited mode:

- Build on hub's existing direct-model client (`server/internal/ai/ai.go:110`, `Complete`, used for information processing / filtering). Select a provider using `fallbackAPIProvider` rules (`assistant/models.go:370`); support only the Messages protocol.
- Plain-text chat, with read-only context hub can answer locally (connection status, knowledge base summaries, Inbox counts). No computer tools, phone writes, or tool-call loop.
- Store chats in a dedicated Postgres table (check all worktrees for migration numbers during implementation), separate from main chat on node. Show a persistent "Chat only for now" notice. After connecting a computer, switch back to normal main chat and include a summary of this segment in the new process's first context message.
- Put prompts in a new assistant/chat_only directory under `server/internal/prompts/`, with golden tests. When asked to act, answer "I can do that once you connect a computer."

Risks (why a decision is needed): memory, transcript, and permissions split into two systems; the work lies in storage, streaming, and client rendering.

### 4.5 First-launch onboarding and empty states by platform

#### First screen (not paired yet)

Same structure on all four platforms:

```
Persona
Connect your computers and phone, and let AI do the work.

[ Scan to connect to hub ]       ← Primary button
Find nearby hubs                ← P1, mDNS
No hub yet?                     ← In-app browser opens "Get started" (choices in 3.3)
Try demo first (all data is fictional) ← Hidden on web, which has no demo mode
Other ways                      ← Manual entry, SSH tunnel, paste link, command-line instructions
```

- Replace the small text below the primary button, "Run `personad hub pair` on the hub machine," with "A QR code appears when hub is installed. You can also open Settings › Devices › Connect new device on an already connected device."
- Web: if the page opens, hub already exists; show only "Scan with your phone to sign in" and "Paste pairing link," plus a "No hub yet?" link.
- Android: remove `JCodeBlock("personad hub pair")` from the welcome page. Replace "Run personad hub pair again" in failure copy with "Generate a new code on a logged-in device."
- Exiting demo mode lands on "No hub yet?".

#### After pairing: one checklist

Extend Android's readiness checklist (`ReadinessScreen` / `ReadinessRecords.kt`, skippable, revisitable, remembers choices) to all four platforms. Add four items before the phone-side items, all reading the readiness endpoint:

1. **Computer**: ✓ for a confirmed online computer; automatically ✓ if hub itself is a computer. If none, show "Add computer"; if pending, "Confirm this is my computer."
2. **Claude Code**: installed or not.
3. **Model**: logged in / key works (detection and three routes in 4.3).
4. **Try a message**: run a real main-chat turn (send "Hello" through normal main-chat creation), and display its reply; on failure, show the reason and fix.
5. Phone-side items (notifications, background activity, local network, cellular-network test…; already exist).

The checklist does not block entry: users can enter the main UI at any time, and the main-chat guidance card is a compact version of it. "Later" persists in Postgres (dismiss in 5.1); the full-page checklist then stops appearing, but the card still returns if readiness worsens.

#### Rewriting empty states

| Location | Today | Replace with |
| --- | --- | --- |
| Overview (`home.json:31`, `all_set`) | "Everything is ready" with zero computers | "One step left: add your computer" + button |
| Machines (`machines.json:25`, `MachinesView.swift:98`, equivalent Android text) | "Once hub connects, this will show…" | "No computers yet. Add one so Persona can do work for you on it" + "Add computer" |
| Agents | "Start Claude in herdr on any computer…" | With zero computers, same as Machines; otherwise "New session" |
| New Agent | Empty computer field; disabled "Start" with no reason | Explain why + entry point |
| Inbox (iOS, web) | "All done" even with no sources connected | "No message sources connected yet" + only sources that take a few taps to connect (email, WeChat) |
| Assistant settings "Runs on" | "Each chat runs in the background in [blank] on [blank]" | If no machine, "No computer selected to run on yet" |
| Model configuration | Exposes `legacy.kb` / `legacy.triage` | Show only localized names |
| Official extension marketplace unreadable | Red text + "Enter token" | Muted "Official extensions are temporarily unavailable. Try again later." Do not ask beginners for a token here. With no token, a subtle link below leads to "Manage marketplaces" (where it can be entered). When GitHub rate-limits hub, try a fallback address first (extensions.md, "GitHub rate limits") |
| Avatar empty state (web) | "Avatar packages belong in ~/.persona/personas/ on the hub machine" | Remove the path |
| iOS launcher fallback | Uses `"ccc"` if configuration cannot be read | Show "Loading / Settings unavailable," not a nonexistent name |

Web implementation (#4485, #4480; details in `docs/web/web.md`, "Zero computers, zero sources") differs from or adds to the table:

- When there are zero computers, no Agents, and none starting, replace the entire Agents page with the Machines-style "No computers yet" + "Add computer"; keep the sidebar Agents section unchanged. With no computers, New Agent contains only the Computer field, the reason, and "Add computer" (close New Agent before opening Add computer; do not stack dialogs).
- Show "No message sources connected yet" in Inbox only if it has never received any items, is neither searching nor filtering, and hub has no email accounts, Lark is unconfigured (ask `/lark/status` if hub has `lark`), and no logged-in connector contributes to Inbox. If any check cannot be read, retain "All done." List email, Lark (when hub has `lark` but is unconfigured), and up to three connectors that can log in here (`login` capability, supply data, not logged in); all open **Settings › Connectors**.
- The sentence in **Settings › General** gives only the computer name, not the working directory (the path remains in **Settings › Assistant**). Likewise for the assistant profile subtitle; with no computer, say "No computer connected yet."
- Pressing Enter while readiness blocks main chat shows one line in the composer: "Not sent: add a computer first. Your text will stay here." (disappears after 4 seconds). The Send replacement shakes, and the readiness card flashes a border then shakes. Touchscreen Enter already inserts a newline, so no notice. "How do I connect a model?" ends by explaining that main chat still runs on a computer today; entering a key alone is not enough. With zero computers (step action `add_computer`), the primary button becomes "Add computer." If a computer exists but is offline / pending / outdated, use that step's own action (wake / confirm / update) and say "Your computer is not ready yet; fix this first: …," rather than asking users to add another. Enter with an empty composer or only attachments gives the same notice.
- If adding a 4th computer is rejected with `too_many_pending`, the dialog lists the blocking computers. If none has ever connected, say "None has connected yet. Delete the ones you do not need," with "Delete" on each; deleting one returns to the form. Hub's `too_many_pending` text also stops saying "Waiting for confirmation." Computers that have never connected (`error_code: never_connected`, or pending without `last_seen`) show "Not connected yet" in the sidebar and machine cards, not "Offline."
- Disabled secondary buttons lose their border and bright text in dark mode (light mode unchanged). If all viewed marketplaces are unreadable, the extension list says "Extensions will appear here once loaded," not also "This marketplace has no extensions yet." With multiple marketplaces and none selected individually (some unreadable), say "No extensions in the loaded marketplaces yet."

### 4.6 Error copy

Principles:

1. Every failure = an error code + one plain-language sentence + one button. Put original text and request ID in expandable "Details" for screenshots when asking for help.
2. Clients use `code` for their own copy and buttons; do not implement four separate literal-text matchers. Hub's `error` also provides plain-language text localized to the request language, so old clients can at least understand it.
3. Say which computer, what is missing, and where to tap. Do not say "Operation failed, please retry," or describe an offline computer as inability to connect to hub.
4. Automatically retry self-healing failures (assistant process restart) without interruption.

| code | When | Plain-language copy | Action |
| --- | --- | --- | --- |
| `no_machine` | Main chat has no usable computer (currently 502 `internal`, `assistant/start.go:29,157-158`; memory at `profile.go:465` uses the same English sentence) | "Persona needs one of your computers to do work. No computer is connected yet." | `add_computer` |
| `node_unreachable` (existing) | Computer offline | "<computer-name> is offline, possibly asleep or with Persona Link closed." | `wake_computer` (instructions page) |
| `assistant_offline` (existing, 503) | Assistant process just restarted | "The assistant is restarting. It will recover in a few seconds." | Automatic retry |
| `machine_pending` | Dispatching work to a pending computer or selecting it as the assistant machine | "You have not confirmed this computer is yours yet." | `confirm_computer` |
| `claude_missing` | Exit code 127 with command `claude`, or `claudeExecutable` cannot find it | "Claude Code is not installed on <computer-name> yet." | `install_claude` (instructions page this round) / `connect_model` |
| `launcher_missing` | 127 for a missing custom launcher from `~/.zshrc` | "The model launcher '<name>' was not found on <computer-name>. It may have been deleted." | `connect_model` |
| `not_logged_in` | `auth status` reports no login, or the reply begins "Not logged in" | "Claude Code on <computer-name> is not logged in yet." | `login_claude` (illustrated) / `connect_model` |
| `auth_failed` | "Invalid API key", "Failed to authenticate", etc. (already in markers) | "The model's key is incorrect or expired." | `edit_provider` |
| `model_route` / `rate_limited` | Wrong model name / 429 (health-check categories already exist) | "This model is currently unavailable / Requests are too frequent. It will retry later." | `connect_model` |
| `tls_required` (existing) | Adding a computer when hub has no TLS | "Hub has not enabled secure connections, so computers cannot join." | `enable_tls` |
| `mesh_member` | Adding a computer from a device on a member hub | "This hub is a group member. Add the computer on the primary hub." | — |
| `pair_code_expired` / `pair_code_used` | Pairing code expired / used (iOS currently shows English `invalid token`) | "This code expired / has already been used. Generate another." | Generate a new code |
| `hub_identity_changed` | Pinned fingerprint mismatch | "Hub's identity changed (reinstalled or moved to another computer). If you reinstalled it, scan a new code." No "Ignore and continue" option | Scan again |
| `node_service_down` (machine `error_code`) | Link works, persistent node is not running (current source text `…node-attach.sock 连不上：persona-link node install…`, meaning cannot connect to node-attach.sock) | "Persona's background service is not running on this computer. Open Persona Link and let it reinstall the service." | — |
| `link_offline` (machine `error_code`) | Persona Link disconnected (current text `link closed: failed to get reader…`) | "This computer is offline: Persona Link is not running, or the computer is asleep." | — |
| `db_missing` (CLI) | Empty `database_url` | "Hub's database is not set up yet. Run personad hub setup to create it." | — |
| `port_in_use` (CLI) | Another program occupies the port (today `hub pair` may hit another hub and return 401) | "Port <n> is in use by another program, possibly another Persona." | Installer chooses another port automatically |

HTTP status: new codes use `rpc.Error{Code}`. `apierr.Call` returns 409 for codes other than `internal` / `node_unreachable` / `timeout` (`apierr/apierr.go:117-131`), so `no_machine` and similar errors must not reuse those three names. Note that existing coded `node_unreachable` is 502; only `ErrOffline` is 503.

### 4.7 Defaults

| Item | Today | Beginner default |
| --- | --- | --- |
| Main-chat model | App-stored value > `assistant.launcher` in the manager's `manager.json` > `claude` (`assistant/profile.go:139-150`) | Unchanged. Use working Claude Code when detected; a key connected during onboarding is assigned through `assign` only while the choice is still default |
| Main-chat permissions | Default mode: approve Bash and similar tools on the phone each time; Persona's own tools are already allowed through `--allowedTools` (`docs/assistant/assistant.md`, "Tools that need no approval") | Unchanged. Improve approval-card explanations; onboarding says "It will ask before doing work on your computer." "Read without asking" cannot safely be expressed for Bash (read-only commands cannot be distinguished). First count approvals during a real beginner setup, then decide (decision 14) |
| Writes on the user's behalf | Hub asks first for email sending, phone writes, and Lark writes | Unchanged (hard boundary) |
| New Agent sessions | User-selected | Default "Manual approval"; no beginner preset uses "Allow all" |
| Launcher health checks | Off by default; when enabled, makes a real request every 30 minutes (`launcher_health.go:58`, costs money). Repair Agents default to bypass (`:68`, user decision 2026-11-05) | Unchanged. Readiness uses only free checks, independent of health checks |
| Scheduled tasks | Morning briefing enabled by default | Skip before readiness (4.4) |
| Example questions | Web has 4, iOS 6, maintained separately | Hub supplies one set in readiness (`suggestions`), selected by connected capabilities; all four clients only render it |
| Built-in providers | None | Built-in presets in 4.3 |

## 5. Protocol (client contract; iOS / macOS implement this)

General rules:

- All new fields are optional and ignored by old clients. Register new endpoints in `features` of `hello` / `GET /api/v1/health` (`Features` in `proto/proto.go`; hub adds extension features in `hub/extensions_link.go`). Clients enter new flows only when advertised; on old hubs, fall back to current pages. New registrations in this section: `assistant_readiness`, `machines_add`, `machine_trust`, `builtin_presets`.
- Enums are closed: unknown `state` displays "One step left + `title`"; unknown `action.id` displays text without a button; unknown `code` displays `error`.
- The server localizes `title` / `detail` / `error` using `Accept-Language` (`server/internal/i18n/locales/`). Clients may override with their own copy, but must work using server text alone.
- Never include secrets; keys return only `has_key`.

### 5.1 Main-chat readiness

```
GET /api/v1/assistant/readiness            Logged-in full device (not answer-only watches or extension tokens)
GET /api/v1/assistant/readiness?probe=1    Same, plus a real one-word check (costs money; repeated calls within 30 seconds return the previous result)
```

Lives under the assistant service, which already knows machine, launcher, and model configuration (`assistant/profile.go`, `models.go`); hub only forwards `/api/v1/assistant/*`. If the assistant process is disconnected, return 503 `assistant_offline`, like other assistant endpoints.

```json
{
  "version": 1,
  "mode": "setup",
  "ready": false,
  "next": "model",
  "machine": {"id": "mbp", "name": "Your MacBook"},
  "launcher": "claude",
  "steps": [
    {"id": "computer", "state": "ok", "machine": "mbp"},
    {"id": "claude", "state": "ok", "version": "2.1.308"},
    {"id": "model", "state": "problem", "code": "not_logged_in", "machine": "mbp", "launcher": "claude",
     "title": "Claude Code is not logged in yet", "detail": "Sign in on your MacBook, or use an API key.",
     "action": {"id": "connect_model"}, "alt_actions": [{"id": "login_claude"}]},
    {"id": "first_message", "state": "todo", "action": {"id": "test_assistant"}}
  ],
  "suggestions": [
    {"id": "what_can_you_do", "title": "What can you help me with?", "local": true},
    {"id": "today", "title": "What's on today", "prompt": "What's on my calendar today?", "needs": ["phone_calendar"]}
  ],
  "probe": {"ok": false, "millis": 2140, "error": "…"},
  "dismissed": false,
  "checked_at": "2026-11-06T09:00:00Z"
}
```

| Field | Values |
| --- | --- |
| `mode` | `full` / `setup` / `chat_only` (only appears in P1) |
| `ready` | `mode == "full"` |
| `next` | ID of the first step not `ok` / `skipped`; `null` if all ready |
| `machine` | Computer main chat will use; `null` if none. Pending computers never appear here |
| `launcher` | Effective launcher name (after `effective()`, overridden by main-chat purpose in model configuration) |
| `steps[].id` | Fixed order: `computer`, `claude`, `model`, `first_message` |
| `steps[].state` | `ok` / `todo` / `problem` / `checking` / `unknown` / `skipped` |
| `steps[].code` | Supplied for `todo` / `problem`, using codes in 4.6. `computer`: `no_machine`, `node_unreachable`, `machine_pending`, `node_outdated`; `claude`: `claude_missing`, `launcher_missing`; `model`: `not_logged_in`, `auth_failed`, `model_route`, `rate_limited`, `model_unverified`; `first_message`: `never_tried`, or the failure code |
| `steps[].action.id` / `alt_actions[].id` | `add_computer`, `confirm_computer`, `wake_computer`, `install_claude`, `login_claude`, `connect_model`, `edit_provider`, `test_assistant`, `enable_tls`, `open_admin` |
| `suggestions[]` | `local: true` opens client-local help without sending to a model; `needs` lists system permissions requested only on tap (client explains beforehand) |
| `probe` | Present only with `?probe=1`; the `{ok, model, answer, millis, error}` from `POST /api/v1/launchers/{name}/check` (`assistant/models_launchers_routes.go:242`) |
| `dismissed` | User tapped "Later" |

Evaluation order and data sources (all existing, plus one new node RPC):

1. `computer`: device list, online status, trust state (5.3). Later steps are `unknown` until the computer is `ok`.
2. `claude`: `claude-version` from node self-check (`node/doctor.go:454`), cached by the assistant service for 10 minutes.
3. `model`: new node RPC `claude.auth_status` (4.3, runs `claude auth status --json` in the launcher's actual environment, cached 10 minutes); the most recent `?probe=1` or "Try a message" result; `turn_error_kind` of main chat's last turn. **No proactive token spending** by default. If never verified, use `unknown` + `model_unverified`, not `problem`; `mode` can still be `full`.
4. `first_message`: whether main chat has ever had a successful reply. `todo` does not affect `mode`.

Storage: the latest check result and `dismissed` live in Postgres settings (one key, `assistant.readiness`), not files.

```
POST /api/v1/assistant/readiness/dismiss   {"dismissed": true}  →  {"ok": true}
```

`POST /api/v1/assistant/warm` keeps returning `{"ok": true}` and adds `readiness` (same structure, no probe). Clients refetch on entering the Persona tab, successful pairing, adding / confirming a computer, saving a provider, and receiving a `machines` change. No new push event in the first version.

On old hubs, this endpoint returns 404: clients fall back to current behavior (no first-screen card; failures show cards by `code`).

Implementation (#3710, `server/internal/assistant/readiness.go`; endpoint details are authoritative in `docs/protocol/protocol.md`, "Main-chat readiness") differs from or adds to the design:

- Full devices only: probing costs money, and hub does not expose launcher-running endpoints to answer-only devices; Watch does not need this currently.
- `model` takes the newest evidence from the last probe, main chat's last turn, launcher health records, and `claude.auth_status` (#4048). An unclassified failure is also `unknown` + `model_unverified`, not `problem`.
- `claude.auth_status` (#4048): node runs `claude auth status --json` with the same command as chat creation (`launcherCommand`: the default is `claude` in the login shell, with this computer's `claude_start_dir`; Persona-managed launchers use `personad launch <name>`) and the same `--settings` (a managed launcher's gateway overrides env in the user's global settings). Put `--settings` before the subcommand (`auth status` itself does not accept it). Version gate: 2.1.41, when `claude auth` first appeared; do not run on older versions. Codex / runtime launchers, unreadable JSON, or no answer within 20 seconds return `unknown` + `reason`. Return only `state`, `auth_method`, `api_provider`, and the variable name identifying the key source; account email and organization never leave the computer. Default-launcher claude discovery is unchanged (#2854).
- Readiness interpretation: no login means `problem` + `not_logged_in`. A `claude.ai` login only disproves "not logged in": mark the model `ok` only with no other evidence or if the latest evidence said not logged in. It does not override main-chat, probe, or health-check `auth_failed` / `model_route` / `rate_limited` (being logged in does not mean the token is unexpired or the plan includes the model). Keys, gateway tokens, and Bedrock / Vertex (`api_key`, `api_key_helper`, `oauth_token`, `third_party`) are not proof; keep `unknown` + `model_unverified`, with `detail` saying "Using a configured account or key; not tested yet." Unlike the design, logged-in results cache for 10 minutes; not-logged-in or unanswered results (RPC failure, `timeout`, `unreadable`) cache only 1 minute. Old-node `unknown method` caches 10 minutes without logging (in `setup`, Send is replaced, so no other action will reveal that the user has signed in). Request alongside version self-check, sharing GET's 12-second / warm's 2-second budget; incomplete work still caches for the next request. Old nodes without this method behave as before.
- `rate_limited` is `problem`, but does not change `mode` to `setup` (self-healing; should not replace Send).
- `claude` also reads two things: the code from the last rejected chat creation (`claude_missing` / `launcher_missing`, cleared after one successful open, a post-rejection self-check finds the version, or the launcher reappears—`setup` prevents sending, so clearing cannot depend only on success), and whether the launcher still exists on this computer. Codex or runtime launchers use `skipped` for this step. If node self-check does not finish in time, use `checking`.
- `node_outdated` checks only whether node understands model configuration: every main-chat creation includes `model_fingerprint` (`finishModel`), which old nodes cannot open.
- Warm computes readiness for at most 2.5 seconds; on timeout, return the last recorded result (omit `readiness` if none), and finish / persist in the background. Opening the tab does not slow down for this.
- `machine_pending` appears after "Confirm computer" (5.3, item 9 in 7.2) merges; `node_outdated` (node does not understand model configuration) has no button.
- `?probe=1` does not run unless computer is `ok`, and the response omits `probe`. A probe with no computer reply (offline, timeout) returns only `error`, is not recorded, and is not evidence.
- The probe takes up to 100 seconds (`healthCheckTimeout`) plus GET's 12-second self-check. It does not stop when the client stops waiting: its result is recorded and the next try within 30 seconds gets it (#1892). Clients wait past that limit (web: `pastHub(PROBE_HUB_MS)`), and a try that gets no answer says so beside the button instead of showing an earlier try's result.
- `dismiss` accepts `{"dismissed": false}` to undo; an empty body means `true`.
- Once `first_message` succeeds, it stays `ok` (stored in `assistant.readiness`, even after main chat changes).

### 5.2 Structured main-chat errors

On failure from `POST /api/v1/assistant/conversations` or a session-send endpoint:

```json
{"code": "no_machine", "error": "Persona needs one of your computers to do work. No computer is connected yet.",
 "detail": "assistant machine is not configured", "action": "add_computer", "request_id": "…"}
```

- `no_machine`, `claude_missing`, `launcher_missing`, and `machine_pending` return 409; `node_unreachable` is unchanged (coded 502, `ErrOffline` 503); `assistant_offline` is unchanged (503).
- `detail` contains the original text; clients show it only in "Details."
- For turns with `turn_error_kind: "user"`, sessions also carry `turn_error_code` (`not_logged_in` / `auth_failed` / `model_route` / `rate_limited`), which clients use to render cards.
- `errNoAssistantMachine` (`start.go:29`) and `memory()` (`profile.go:465`) use the same code; do not let them drift apart. Update voice-prompt goldens (`server/internal/voice/testdata/prompts/refused.failed.*.golden`, all three languages) too.
- Old clients still display `error`, now localized rather than raw English. Check that iOS `isQuiet` does not treat 409 as a silent error.

### 5.3 Logged-in devices add and confirm computers

Extend the existing pairing-code endpoint, rather than creating another system:

```
POST /api/v1/auth/pair-codes      Full device (not a temporary pairing-code identity), or loopback + main token
{"add": {"name": "Your MacBook", "os": "mac", "icon": "laptop"}}
or {"machine": "example-macbook"}            Regenerate a code for an existing link computer
```

```json
{
  "token": "jp_…", "expires_at": "…",
  "machine": "example-macbook", "trust": "pending",
  "pair_url": "persona://link?url=…&token=…&machine=example-macbook&hub=…&fp=…&tls=…",
  "command": "persona-link pair '<pair_url>'",
  "install": {"linux": "curl -fsSL <site>/install.sh | sh -s -- --node --pair '<pair_url>'"}
}
```

- `add.name` may be Chinese; generate the ID from the name (ASCII conversion + deduplication, satisfying `/admin` ID rules). Do not ask users for an ID that "cannot be changed later." `add.os` only determines which installation instructions the client shows.
- With `add`, hub writes hub.json and calls `addMachineLive` using the link branch of `POST /api/v1/admin/machines`, while writing a `pending` trust record to Postgres. Loopback + main token writes `confirmed` (or no record).
- Reuse an existing same-name link computer that has never connected, generating a new code.
- `<site>` comes from configuration, not a hardcoded value.
- Errors: 403 (not a full device, or a device obtained from a pairing code and not yet confirmed); 409 `tls_required` (`action: "enable_tls"`); 409 `mesh_member`; 429 `too_many_pending` (at most 3 computers with unredeemed codes or pending confirmation at once).
- Remove the device restriction at `pairing.go:132-135`, but allow only `add` / existing link computers.
- Implementation (#3735) differs from the design; `docs/protocol/protocol.md` ("Computer pairing codes") is authoritative. Devices may generate codes only for **pending** computers (`machine` or same-name reuse); confirmed ones return 409 `not_pending`.
  Thus device-generated codes always go through "pending → confirmed." Re-pair confirmed computers in the admin UI or on the hub machine (client M3 cannot regenerate codes for confirmed computers in the app). This is not a security boundary: full devices can already use
  `POST /api/v1/auth/tokens` (`kind: link`) to issue credentials for any link computer; tightening that is separate work. Before regenerating a pending computer's code, revoke its existing Persona Link credentials (credentials redeemed from the old code must not become confirmed alongside the new ones). `add` may include `id` (for `hub pair --machine <id> --add`). The response always includes `trust`
  (`pending` / `confirmed`), plus `name` and `added`. If hub does not know its release website (local build, Docker image), omit `install`. A main-token request with nonexistent `machine` returns 400 `unknown_machine`.
  Receipt notification text includes the computer name: "<device> generated a pairing code for computer '<name>'." The notification's "Not you? Revoke" button is not implemented yet (requires notification categories on each client).

- Android implementation (#4297): follow web's add-computer flow, with confirmation on the "Waiting for it to connect" screen. Add through the selected hub group; regenerate, confirm, and reject through the machine's group by machine ID. Without `machines_add`, the add-computer page only explains how to add on web or in the admin UI (no longer a dialog). The web login URL in **Settings › Devices › Connect new device** uses the address this phone uses to connect to hub. "Regenerate link" is available only for computers with `error_code: never_connected` (regeneration invalidates existing pairing; sleeping computers only need waking). Entering from "Confirm computer" with none awaiting confirmation shows "No computers awaiting confirmation," not the add form. The Machines "+" and empty-state "Add computer" open the add page only with `machines_add`; old hubs retain the explanation dialog. If `expires_at` is unreadable, already expired relative to the phone clock, or more than 10 minutes ahead, count 10 minutes from now using the phone clock.

- iOS / macOS implementation (#4196): follow web's flow and copy, with confirmation on the same waiting screen. "Regenerate link" likewise appears only for pending computers that have never connected. Without `machines_add`, show no add entry point (Machines retains its original empty-state copy); without `machine_trust`, show no confirm / reject buttons. Other pages (readiness-card `add_computer` / `confirm_computer`) open through `AppModel.openAddComputer()` / `openConfirmComputer(_:)`; see `docs/apple/ios.md` ("Adding and confirming computers").

Add two optional fields to every machine in the list (`GET /api/v1/machines` and hello machines):

| Field | Values |
| --- | --- |
| `trust` | `pending` / omitted (= confirmed) |
| `error_code` | `node_service_down`, `link_offline`, `never_connected`, `node_outdated`; retain original `error` text |

```
POST /api/v1/machines/{id}/confirm    Full device   →  {"ok": true}       pending → confirmed, then push keys
POST /api/v1/machines/{id}/reject     Full device   →  {"ok": true}       Delete hub.json entry and record; revoke link credentials
```

Pending computers: skipped by `pushManaged`, `defaultAssistantMachine`, and assistant machine candidates; absent from computer pickers for new Agents, dispatch, and to-dos; automatically rejected after 24 hours without confirmation. Code generation, redemption, confirmation, rejection, and expiry deletion all create Postgres audit records and notify other paired devices ("<device> generated a computer pairing code," "New computer <name> wants to join").

`personad hub pair --machine <id> [--add] [--name <name>]`: uses loopback + main token; `--add` creates a missing computer directly as confirmed.

### 5.4 Providers: adding from presets and `assign`

`GET /api/v1/launchers/presets`: returns "built-in + plugins," sorted by request language. Add optional fields to `LauncherPreset`:

| Field | Description |
| --- | --- |
| `plugin` | `"builtin"` for built-ins |
| `signup_url` | Page for signing up / buying a key; clients open it in the in-app browser (set only for built-in presets) |
| `display_name` | Localized display name (such as Zhipu GLM in Chinese) |

Two additions to existing `POST /api/v1/launchers`:

```json
{"preset": "builtin/deepseek", "api_key": "…", "name": "", "display_name": "",
 "assign": ["assistant", "agent.new"]}
→ {"launcher": {…, "name": "deepseek", "display_name": "DeepSeek", "has_key": true},
   "assigned": ["assistant"], "kept": [{"purpose": "agent.new", "reason": "customized"}]}
```

- With `preset`, `name` may be empty: use the vendor slug, adding `-2` on collision, satisfying `managedLauncherName` (`models_launchers.go:98`, ASCII only). If `display_name` is empty, use the preset's value. Without `preset`, `display_name` may also be Chinese.
- `assign`: after saving, PATCH each listed purpose, **only if that purpose remains at its untouched default**. Split results into `assigned` and `kept`. Clients do not need their own "read revision, then PATCH" sequence.
- With `preset`, existing `POST /api/v1/launchers/test` uses the preset's default model if `model` is omitted.

### 5.5 Command line

| Command | Output |
| --- | --- |
| `personad hub setup [--json]` | One localized line per step; with `--json`, one `{"step","ok","detail","next"}` per line, followed by `{"done":true,"pair_url","admin_url","admin_local_only","setup_code","expires_at"}` for Persona Link / all-in-one Mac callers |
| `personad hub pair --machine <id> --add` | Same as today, with an extra "Added: <name>" line |

## 6. Do / defer / decisions needed

### 6.1 In this round

- Server: structured main-chat error codes and localized copy; add `Not logged in` to `APIErrorMarkers`; readiness endpoint and readiness in warm; node reports Claude Code installation / login; built-in presets, localized display names, empty names when adding from presets, `assign`; pending computers + keys only to confirmed ones + receipt notifications; logged-in devices add computers, `hub pair --machine --add`; machine `error_code`; skip scheduled tasks before readiness; `personad hub setup` (TLS, LAN `public_url`, `admin.first_setup` in new files); localized next steps in `db.go`, the end of `install.sh`, and startup logs; degradation when the extension marketplace is unreadable.
- Web: main-chat guidance and failure cards; command-free login, device deduplication; empty states; post-pairing checklist; model wizard starts from presets; add and confirm computers.
- Android: same as web; add Connect new device, Test connection, Get model list, built-in presets; `operationFeedback` chooses copy by code (today everything becomes "Operation failed").
- Deployment: fill `.env` at Docker first startup; map the TLS port.
- Documentation and website: this document; a "Get started" page for ordinary users (website + in-app, demo-mode screenshots); split the README top into "I want to use it" and "I want to develop"; rewrite pairing steps in `../operations/persona-link-install.md` and `../operations/link.md` around "Add computer in the app"; put actual download options on the website's first screen, moving curl into "Command-line installation."
- iOS / macOS / Persona Link: open issues (7.3), with protocol in section 5.

Documentation implementation (#4046): "Get started" lives in `docs/operations/getting-started.md`. The README top is split into "I want to use it" and "I want to develop"; pairing in `../operations/persona-link-install.md` and `../operations/link.md` primarily uses web **Machines › Add computer**, with admin UI and command line under "Advanced."
Differences from the plan: the website was not changed (first screen and FAQ await decision 15), and the app still has no entry point to this page. No screenshots were added; demo-mode screenshots will accompany the website version.
Until iOS / Android add-computer, preset wizard, and readiness cards merge, the page honestly says "Use the web for now"; update it as those items merge.
When Claude Code is installed after the hub, rerunning `hub setup` adds this machine (`kind: local`) while hub.json has `"machines": []` and no manager keeps a computer, and restarts the running hub so it picks it up (#4083); no hand edit of hub.json is needed. A manager that keeps its own list takes the computers out of hub.json and leaves `"machines": []` there ([managers.md](managers.md) "Each manager's computers"), so setup reads the hub's record of who keeps what (`hub.machines_kept`, in the hub's database, migrating nothing) after the database step and adds nothing while it names a computer kept by a manager still on the roster. When it can't read the record, it adds nothing either and says why.

### 6.2 Deferred

| Item | Reason |
| --- | --- |
| "Chat only" mode | Separate P1 issue: a new run mode with memory and permissions separate from main chat on node; needs decision 9 |
| One-tap Claude Code installation in the app | P1: executes a downloaded installer on the user's computer; needs confirmation, verification, and fallback for networks in China |
| Claude subscription sign-in without a terminal | First spike whether `claude setup-token` / `claude auth login` can finish without a terminal; write conclusions in `docs/lessons/agents.md`; also needs decision 10 |
| mDNS hub discovery in the app | P1: after default TLS; discovery only, initial trust still requires a one-time code |
| Computer-initiated, phone-approved pairing | P1: separate security review; copying / sharing links is sufficient this round |
| Phone displays a short code, computer enters it | mDNS can be spoofed on the same network, allowing code theft |
| All-in-one Mac package, Persona Link installer | Requires Developer ID (decision 1) |
| Embedded Postgres / SQLite | Repository policy puts all state in Postgres; bundling third-party Postgres binaries needs evaluation (decision 2) |
| Health checks / automatic repair enabled for beginners | Real requests every 30 minutes cost money; repair-Agent bypass is the user's 2026-11-05 decision, not changed in onboarding |
| "Allow all" / "Read without asking" in beginner presets | The former's risk is unacceptable; the latter cannot safely be expressed for Bash (decision 14) |
| Windows hub, Windows tray app | #1179, #299 tracked by others |
| One-click cloud templates, relay tunnels for away-from-home connections | Requires external service accounts or operated infrastructure (decisions 6, 7) |
| Hub migration tool (Mac to NAS) | Make first installation work first |
| Web demo mode | `docs/web/web.md` already decides against it; hub hosts the web app itself |
| Linked hubs, passive mode (hub connects), ssh, or Lark setup in onboarding | Beginners always have one hub; Lark setup does not work for ordinary users today |
| Publisher-provided Gmail OAuth | Requires Google restricted-scope verification (decision 12); onboarding recommends only IMAP / app passwords |

### 6.3 User decisions needed

| # | Decision | Why it needs a decision | Fallback without a decision |
| --- | --- | --- | --- |
| 1 | Developer ID signing + notarization for Persona Link and the future all-in-one Mac package | Paid developer account and notarization process | Mac installation only from source; say so on the website; acceptance tier B (8.3) is unavailable |
| 2 | Database on machines without Docker: bundled Postgres binary or embedded-postgres | New dependency, package size (tens of MB), data-directory management during upgrades | Require Docker Desktop / OrbStack or local Postgres |
| 3 | Combine Mac app (`PersonaMac`) and Persona Link into one download | Product form | Two apps, with the difference explained on the first screen |
| 4 | Make the Docker image (ghcr) public | Public release policy | NAS users need `docker login` |
| 5 | Public signed Android APK download page, or Google Play | Signing key, store registration fee | Hub-hosted APK (`hub pair --android`) |
| 6 | One-click cloud-server templates | Cloud marketplace account registration | Run the same one-line command on a cloud server |
| 7 | Away-from-home direct connection: publisher-operated relay tunnels | New traffic costs and operations | LAN works; use VPN or IPv6 away from home |
| 8 | New-install TLS binds only LAN IPv4 (recommended), or opens all addresses at `:18662` | On machines with public IPv6, all-address binding directly exposes the port publicly (device tokens and rate limits provide protection, `docs/operations/network.md`) | Follow the recommendation: LAN only; ask "Use away from home too?" at the end of onboarding |
| 9 | Whether to build "Chat only" mode (P1) | Product decision: Docker / NAS and key-only users no longer get a blank screen; cost is two main-chat systems | These users see only a guidance card and must add a computer |
| 10 | Node performs headless Claude subscription sign-in and stores the token in hub | Compliance with Anthropic terms | API key is the only terminal-free path |
| 11 | Which providers "Where to get a model" recommends, their order, affiliate links, and whether official Anthropic is first (may be unreachable directly in China) | Business and compliance judgment | List providers by language, without endorsements or prices |
| 12 | Publisher-provided verified Gmail OAuth client | Google restricted-scope verification / CASA, potentially paid | Recommend only IMAP / app passwords |
| 13 | Hub's UI name: "hub", "host", or "center" | Global terminology | Keep "hub," explaining it on first use |
| 14 | Whether beginners get a "Read without asking" permission tier | Looser security default; read-only Bash commands cannot be distinguished | Default mode; count approvals in real testing first |
| 15 | Reorient the website and TestFlight description toward ordinary users (remove "Write to us" from FAQ, publish links) | Public positioning | Change only README and documentation |
| 16 | Real-user testing: who recruits, screen-recording consent template | Requires external participants | Automated tests and internal walkthroughs only |

## 7. Ownership and staged plan

### 7.1 Platform ownership

| Platform | Work this round | Where |
| --- | --- | --- |
| Server | Error codes, readiness, node Claude Code status, presets and `assign`, pending computers and key filtering, add computers, receipt notifications, scheduled-task gating, `hub setup`, CLI copy | Linux development machine, tests against real Postgres |
| Web | Guidance cards, failure cards, empty states, login page, device deduplication, checklist, model wizard, add / confirm computer | Linux development machine, Playwright screenshots |
| Android | Four checklist items, coded copy, first screen, Connect new device, Test connection, built-in presets, add / confirm computer | Linux development machine; state in the PR when screenshots were not taken due to lack of KVM |
| iOS / macOS / Persona Link | Issues in 7.3 | Mac |
| Deployment | Docker entrypoint and compose | Linux development machine |
| Documentation / website | Get started, README, Persona Link pairing steps, website first screen and FAQ (FAQ awaits decision 15) | Linux development machine; publish per `docs/operations/site.md` |

### 7.2 PR order

Create an issue for each PR first (`Refs #287`), one issue per PR, in the order below. Earlier PRs are useful independently.

| # | Title (for PR) | Platform | Content | Value when merged alone |
| --- | --- | --- | --- | --- |
| 1 | fix(assistant): say why the main conversation can't start, with a code | server | Codes and localized copy from 5.2; add `Not logged in` to markers as `user`, with scheduled-task / to-do tests; unify `start.go` and `memory()`; update goldens | Old clients immediately get a localized 409 instead of raw English 502; later clients show cards by code |
| 2 | test: guard onboarding copy and add readiness scenarios to the mock hub | scripts, mock | Copy guard from 8.1; seven readiness states and scenarios for every error code in `mock_hub.py` / `mock_scenarios.py` | Every later UI PR has copy guards and screenshot data |
| 3 | feat(node): report whether Claude Code is installed and signed in | server (node) | `claude.auth_status` RPC (actual launcher environment, version check, `unknown` fallback); prefer absolute paths for default launcher (coordinate in #2854 first) | Readiness can detect login for free |
| 4 | feat(assistant): readiness of the main conversation | server | All of 5.1; `readiness` in warm; `features` registration; endpoint in `docs/protocol/protocol.md` | Single data source for all four clients and iOS issues |
| 5 | feat(web): show what the main conversation still needs | web | Guidance card, Send button, failure cards instead of toasts, empty states in 4.5 | New web users no longer see "looks ready, fails only after sending" |
| 6 | fix(web): sign in without commands and keep one device per browser | web | Rewrite login page; reuse / replace old device on sign-in | Web's first screen stops teaching `jq` |
| 7 | feat(assistant): built-in provider presets and assign-if-default | server | `presets_builtin.json` (test every provider), `signup_url`, `display_name`, empty names allowed, `assign`, default model for tests | Key-only users connect in three steps |
| 8 | feat(web): provider wizard starts from presets | web | Provider list first, paste only a key, automatic test, `assign`; move "Import from ~/.zshrc" into Advanced | Web model wizard becomes usable for beginners |
| 9 | feat(hub): pending machines get no keys until confirmed | server | Trust table (no record = confirmed), `pushManaged` / assistant-machine / Agent-list filters, confirm / reject, 24-hour cleanup, machine `trust` / `error_code` | Security prerequisite for allowing devices to add computers; no behavior change when merged alone |
| 10 | feat(hub): a signed-in device can add a computer | server | `add` in pair-codes (extract admin add function), `hub pair --machine --add`, receipt notifications, `mesh_member`, `too_many_pending` | Adding computers no longer requires /admin or a setup code |
| 11 | feat(web): add and confirm a computer from the machines page | web | Pages in 4.2, reuse countdown and polling from `connect-device.ts`; confirmation on the waiting screen | Web users never need /admin |
| 12 | feat(assistant): routines wait until the main conversation is ready | server | Skip before readiness, record "Skipped," no failure push | New hubs with zero computers stop failing at scheduled times |
| 13 | feat(hub): personad hub setup | server, scripts | All of 4.1; called by `install.sh --hub`; copy at `db.go:37`; `newInstallConfig()` and old-file loading goldens | Website's "one command" actually installs hub for the first time; can run alongside 5–12 |
| 14 | feat(docker): first start fills .env and maps the TLS port | deploy | Entrypoint, compose | Two fewer manual steps for NAS |
| 15 | feat(android): onboarding checklist, coded errors and first screen | Android | Four checklist items, coded `operationFeedback`, remove first-screen commands, empty states, guidance card | Android matches web |
| 16 | feat(android): presets, connection test, connect a device, add a computer | Android | Model wizard, Connect new device, add / confirm computer | Same |
| 17 | docs: getting started for people who don't use a terminal | docs, site | Get started, README, Persona Link pairing, website first screen | A readable explanation for people arriving from an ad |

Separate P1 issues: "Chat only" mode (after decision 9), node `claude.install`, headless sign-in spike, in-app mDNS discovery, computer-initiated pairing, all-in-one Mac (after decisions 1 and 2).

### 7.3 iOS / macOS work assigned to a Mac

Each issue references the protocol in section 5 and states "The Linux development machine cannot compile Swift; note in the PR if Xcode 26.0 was not verified." UI uses only the design system (`.surface`, `EmptyStateView(action:)`, `Theme.*`); do not raise the `TokenRatchetTests` baseline. Single-line buttons; light and dark screenshots.

| # | Title | Changes | Acceptance |
| --- | --- | --- | --- |
| M1 | iOS / Mac: show missing main-chat setup by readiness, use cards for failures | Read `GET /api/v1/assistant/readiness` (depends on PR 4; check `assistant_readiness` in `features`); guidance card above composer, "Connect a model first" Send replacement, preserve drafts without queueing; cards for 5.2 codes, original text in "Details"; replace `turn_error_code` bubbles with cards; remove `"ccc"` fallback | Light / dark screenshots for seven states (mock hub): zero computers, offline, pending, missing Claude Code, not logged in, wrong key, all ready. No raw English errors on first screen; 409 not swallowed by `isQuiet`; `AssistantView` model has FakeHub unit tests; old hub (404) behaves as before |
| M2 | iOS / Mac: first screen and post-pairing checklist | WelcomeView uses 4.5 structure, command line under "Other ways"; after pairing, reuse `pairingEngineMachine` insertion to add computer / Claude Code / model / test message, skippable and revisitable; "Later" calls dismiss; exiting demo goes to "No hub yet?" | No `personad`, token, hub.json on first screen or checklist (copy guard includes iOS strings); screenshots of each state for all four items; "N steps left" at top of Settings after skipping; Mac paste-link pairing still works |
| M3 | iOS / Mac: add and confirm computers | Machines, Agents empty state, checklist "Add computer" → choose OS → name (may be Chinese) → pair-codes with `add` (depends on PR 10) → copy Linux command / share Mac link → countdown waiting for online → "This is my computer" calls confirm on the same screen. Machine list shows localized `trust` and `error_code`; "New computer wants to join" push allows direct confirm / reject | 0 to 1 confirmed online computer without hub.json, TLS, or node; regenerate expired codes; pending computers absent from New Agent; rejection removes it from list; no entry point on old hubs without `add` |
| M4 | iOS / Mac: model wizard starts from presets | ProviderWizard first step reads vendors from `GET /api/v1/launchers/presets` (remove hardcoded "Official Anthropic"), key only, automatic test, save with `assign`; "Use for main chat" defaults off when readiness finds working Claude Code; "I already use Claude Code" uses `?probe=1`; "Where to get a model" opens `signup_url` in-app | Screenshots for all three routes; 401 / 403 / 404 / 429 / timeout copy matches `assistant-models_probe.json`; Chinese display name saves; customized main-chat model is not overwritten on save (listed in `kept`) |
| M5 | Persona Link: pair by opening a link, no command required | Register `persona` and `persona` URL schemes; `persona://link?…` opens and prefills pairing window; pairing copy becomes "Copy this from Add computer in the phone app"; first-screen plain-language states: unpaired / connected / waiting for phone confirmation / background service not running | Links from Notes, AirDrop, email open pairing; pending shows "Confirm on your phone"; document unsigned-build behavior in the PR |
| M6 | iOS / Mac: pairing and connection receipt notifications | "<device> generated a pairing code" and "New device / computer connected" notifications (depends on PR 10 pushes), with "Not you? Revoke" | Tapping Revoke after arrival revokes the device or rejects the computer; notification text contains no IDs or addresses |

## 8. Validation

### 8.1 Automation

- **Server unit tests** (Linux development machine, real Postgres, `go test -p 1 ./...`):
  - Loading old hub.json gives the same configuration as before (goldens include db.sh's database_url-only file and a handwritten file without `public`); `newInstallConfig()` includes TLS, LAN `public_url`, `admin.first_setup`.
  - Pending: device codes add pending computers; no keys, no assistant selection, absent from New Agent, cleanup after 24 hours. Pre-upgrade machines and `/admin` / loopback additions are confirmed; Watch, link, and pairing-code identities cannot generate computer codes.
  - `assign` changes only untouched-default purposes; preset additions allow empty names and suffix collisions; built-in presets have complete fields.
  - Readiness: a case for each state; no model request by default; `?probe=1` reuses results within 30 seconds.
  - After adding `Not logged in` to markers, test scheduled tasks, to-dos, and session-list `TurnError` behavior.
  - Codes such as `no_machine` return 409 with localized `error`.
- **Copy guard**: add test_onboarding_copy.py under `scripts/test/`, scanning onboarding, empty states, and error-code copy (web i18n, Android `values-zh`, iOS strings, server i18n). Prohibit `hub.json`, `token` ("pairing code" does not count), `jq`, `personad`, `TLS`, `node`, `herdr`, `launcher`, `localhost`, `~/.`. Whitelist keys under "Advanced" / "Details." Include it in tests like `privacy_check.py`.
- **Failure scenarios**: add seven readiness states (zero computers, offline, pending, missing Claude Code, not logged in, wrong key, all ready) and every code in 4.6 to `mock_hub.py` / `mock_scenarios.py`. Web Playwright, Android `android.sh shots`, and iOS screenshots share them.
- Implementation (#4033; usage in `docs/operations/testing.md`, "Onboarding copy guard" and "Readiness scenarios") differs from or adds to the design:
  - Alongside the whitelist (`ALLOW`, with reasons), the copy guard has `PENDING` for existing issues being fixed (Android first screen / empty states in #4032; iOS / Mac in M2–M4 of 7.3). Remove entries once fixed; never add new ones. iOS has no catalogs yet, so scan Chinese literals in a few Swift files.
  - The pending scenario's readiness is `no_machine`, not `machine_pending`: hub does not expose pending computers to the assistant (`devices.list` excludes them), matching 5.1's implementation notes. `machine_pending` appears when starting an Agent on one, and with `MOCK_START_ERROR=machine_pending`.
  - Additional scenarios: `demo`, `unverified`, `launcher_missing`, `model_route`, `rate_limited`, `node_outdated`, `assistant_offline`. `server/internal/assistant/readiness_scenarios_test.go` generates goldens for readiness and chat-creation rejection from real code; mock tests compare each rather than copying by hand.
- **Fresh-user end-to-end** (Linux development machine): on an isolated test hub (own port, data directory, database, `PERSONA_LOCAL_NODE=0`), Playwright follows web login → guidance card → add a preset provider through a fake gateway → `assign` takes effect → readiness `model` changes. Assert the first screen and onboarding contain no prohibited copy-guard terms.
- **Upgrade regression**: upgrade using a handwritten hub.json, `~/.zshrc` launchers, and a database with Persona-managed providers. Keys still distribute, all machines are confirmed, main chat works as before; `./scripts/test/test-server.sh` passes completely.

### 8.2 Screenshots

For every UI PR: a light and dark screenshot for each state above; web also at width 390. Take Android screenshots on a KVM-capable machine; if unavailable on the Linux development machine, state that in the PR. iOS / Mac screenshots follow 7.3 acceptance on a Mac.

### 8.3 Testing with nontechnical users

**Two acceptance tiers**:

- **Tier A (achievable this round)**: an always-on Linux mini PC or NAS + phone. The tester pastes one command using the website screenshots (the only command on the hub machine); everything else happens in the app.
- **Tier B (after decisions 1 and 2)**: clean Mac + iPhone, downloads apps only, no terminal. The original "10 minutes" acceptance in #287 belongs to this tier.

**Steps**:

1. **Paper prototype first**: turn onboarding screenshots into a clickable prototype (Wizard of Oz); have 2 people try it, fixing copy before writing the UI.
2. **Recruit 5 people**: never used a terminal, not developers, product managers, or testers, with their own phones. Use a clean OS user and an uninstalled / reinstalled app.
3. **Give goals, not steps**: the scenario card says "Have Persona tell you the largest folders on this computer." Tier A also asks "Connect your laptop." Provide a funded test key or let them sign up themselves (testing whether "Where to get a model" works).
4. **Two networks**: home Wi-Fi; at the end, switch to cellular and ask another question (checking whether away-from-home guidance is clear).
5. **Inject failures**: randomly assign 1 failure per person, prepared by the observer: computer sleep, expired pairing code, wrong key, Claude Code not logged in, phone denied local network permission. See whether the error card lets them fix it themselves.
6. **Observer watches without helping**: if asked, say only "What do you think you should do?" Give a hint only after more than 3 minutes stuck; record one help request.
7. **Record**: completion, total time, time per step, help requests, every error card seen, and confusing terms spoken ("What is hub?"). Screen recording requires signed consent and is not committed; anonymized conclusions go in `docs/archive/onboarding-usability-<date>.md`.
8. **Count one more thing**: approvals prompted when main chat first acts on the computer (for decision 14).

**Passing threshold**: at least 4 of 5 independently finish tier A within 15 minutes; no one sees raw English diagnostics; at most 3 help requests in total; every failure scenario has someone fix it using card buttons alone. Return failed steps to the paper prototype and revise.

## 9. Compatibility

- New defaults go only into newly generated hub.json; no field changes in existing files, and no new defaults in `defaultConfig()` (4.1).
- `deploy.sh`, `db.sh`, `hub-systemd.sh`, socket activation, and handwritten hub.json work as before. The database container created by `hub setup` has the same name and volume as `db.sh`.
- `~/.zshrc` launchers are still discovered and selectable; onboarding simply does not mention them.
- The `personad` command, `persona://pair` / `persona://link` links, and `PERSONA_*` environment variables remain; Persona Link recognizes both schemes.
- Errors change from 502 `internal` to 409 + code: old clients already display non-2xx `error` as text; only the language changes.
- Old node and Persona Link do not need to understand pending; filtering lives entirely in hub and the assistant service.
- New endpoints are registered in `features`; new clients fall back to current pages on old hubs.
- Check all worktrees before choosing migration numbers in Postgres PRs (the trust table needs one; the readiness key needs no new table).
