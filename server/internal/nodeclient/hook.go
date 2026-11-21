package nodeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/herdr"
	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/procinfo"
)

// The environment the engines give a pane's processes, naming the pane (node/host*.go set and read the same names).
const (
	HerdrPaneEnv      = "HERDR_PANE_ID"     // classic herdr
	HerdrSessionEnv   = "HERDR_SESSION"     // classic herdr: the named session the pane is in (none: the default server)
	HerdrSocketEnv    = "HERDR_SOCKET_PATH" // classic herdr: its server's socket, in the session's directory
	HerdrCorePaneEnv  = "HC_TERMINAL_ID"    // herdr-core (Herdr App)
	OrcaPaneEnv       = "ORCA_PANE_KEY"     // Orca
	OrcaLegacyPaneEnv = "ORCA_AGENT_PANE"   // older Orca
	TernPaneEnv       = "TERN_PANE"         // Tern
	RexPaneEnv        = "REX_BLOCK"         // Rex
)

// GlobalHookFlag marks the report-only entries the node puts in the user's own Claude Code settings
// (node/globalhooks.go): `<binary> hook --persona-global`.
const GlobalHookFlag = "--persona-global"

// HomeFlag names the data directory, and so the node, of a hook in exec form (`<binary> hook --persona-home <dir>`,
// the live replies plugin's: node/doctor.go displayHooks). Claude Code starts that one without a shell, so it cannot
// have PERSONA_HOME in front as the shell-form hooks do.
const HomeFlag = "--persona-home"

// HookPane is the pane a hook runs in, from its environment, as a stored PaneRef: herdr's HERDR_PANE_ID as it always
// was in the default server, qualified with its session in a named one ("@<session>:w4:p1", herdr.PaneOf, #2196),
// else herdr-core's HC_TERMINAL_ID ("herdr-core:t7"), else Orca's ORCA_PANE_KEY (or its older ORCA_AGENT_PANE:
// "orca:<tabId>:<leafId>"), else Tern's TERN_PANE ("tern:<block>"); "" in none. A node whose own herdr server is a
// named session writes its panes the other way round and translates (node ownPane).
func HookPane(getenv func(string) string) string {
	if p := getenv(HerdrPaneEnv); p != "" {
		return herdr.PaneOf(p, herdr.EnvSession(getenv), "")
	}
	if t := getenv(HerdrCorePaneEnv); t != "" {
		return host.PaneRef{Kind: host.HerdrCore, ID: t}.String()
	}
	for _, v := range []string{OrcaPaneEnv, OrcaLegacyPaneEnv} {
		if k := getenv(v); k != "" {
			return host.PaneRef{Kind: host.Orca, ID: k}.String()
		}
	}
	if b := getenv(TernPaneEnv); b != "" {
		return host.PaneRef{Kind: host.Tern, ID: b}.String()
	}
	if b := getenv(RexPaneEnv); b != "" {
		return host.PaneRef{Kind: host.Rex, ID: b}.String()
	}
	return ""
}

// RunHook is `<binary> hook [--persona-home <dir>] [--persona-global]`: a Claude Code hook's input on stdin, what the
// hook prints on stdout (nothing: let the TUI decide). The node writes that command into the settings of the Agents it
// starts and, with GlobalHookFlag, into the user's own (node/doctor.go, node/globalhooks.go).
func RunHook(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) {
	if len(args) >= 2 && args[0] == HomeFlag {
		if args[1] != "" {
			_ = os.Setenv("PERSONA_HOME", args[1]) // read by SocketPath, as from a shell-form hook's environment
		}
		args = args[2:]
	}
	input, err := io.ReadAll(stdin)
	if err != nil || !json.Valid(input) {
		return
	}
	if len(args) > 0 && args[0] == GlobalHookFlag { // an entry in the user's settings.json: report only
		ReportHook(ctx, input, HookPane(os.Getenv))
		return
	}
	if len(args) > 0 && args[0] == CuaOnlyFlag && !runsCua(input) {
		return // an Agent's Bash command that does not run cua-driver: nothing to ask the node
	}
	if isDisplay(input) {
		Display(ctx, input)
		return
	}
	out := Hook(ctx, input, HookPane(os.Getenv))
	if len(out) == 0 {
		return
	}
	_, _ = stdout.Write(append(out, '\n'))
}

// CuaOnlyFlag marks the Bash hook of the Agents Persona starts on a Mac (node/doctor.go): only a command that
// mentions cua-driver goes to the node's gate (node/cua_gate.go, #4349); any other returns at once, so the Agent's
// Bash never waits on the node, not even while it restarts.
const CuaOnlyFlag = "--cua-only"

// runsCua: the hook is a Bash PreToolUse whose command mentions cua-driver.
func runsCua(input json.RawMessage) bool {
	var in struct {
		Event     string `json:"hook_event_name"`
		Tool      string `json:"tool_name"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	return json.Unmarshal(input, &in) == nil && in.Event == "PreToolUse" && in.Tool == "Bash" &&
		strings.Contains(in.ToolInput.Command, "cua-driver")
}

// Hook hands a Claude Code hook's input to the node and returns what the hook should print (nil: let the TUI decide).
// When the node goes away while the phone is deciding (a hub or node restart) it reconnects to the next node with the
// same request for up to ResumeWait: the interaction keeps its id, and the hub keeps it pending through the gap.
// No node at all: nil at once, the TUI asks as usual.
func Hook(ctx context.Context, input json.RawMessage, pane string) json.RawMessage {
	req := Request{Kind: "hook", Input: input, Pane: pane, Attempt: NewAttempt()}
	req.AgentPID, req.AgentStart = AgentOf(input)
	// A live node answers by the interaction's expiry; a socket that is there but never answers must not hold Claude.
	ctx, cancel := context.WithTimeout(ctx, HookTimeout+ResumeWait)
	defer cancel()
	line, err := DialResumable(ctx, Dial, req)
	if err != nil {
		return nil
	}
	var resp struct {
		Output json.RawMessage `json:"output"`
	}
	_ = json.Unmarshal(line, &resp)
	if len(resp.Output) == 0 || string(resp.Output) == "null" {
		return nil
	}
	return resp.Output
}

// reportTimeout is how long a report-only hook waits for the node; it asks for nothing, so a node that does not
// answer at once is not worth holding the process for.
const reportTimeout = 3 * time.Second

// ReportHook hands a global report-only hook's input to the node and returns at once when there is nothing to report
// to: not in a pane Persona knows, or no node socket. It prints nothing either way (the entries are async).
func ReportHook(ctx context.Context, input json.RawMessage, pane string) {
	if pane == "" {
		return
	}
	if _, err := os.Stat(SocketPath()); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, reportTimeout)
	defer cancel()
	req := Request{Kind: "hook", Input: input, Pane: pane, ReportOnly: true}
	req.AgentPID, req.AgentStart = AgentOf(input) // the counters need it as much as a plugin hook's do
	_, _ = Dial(ctx, req)
}

// DisplayEvent is the hook event that carries a reply's lines as Claude Code shows them: only the Agents Persona starts
// have it, in their live replies plugin while the machine's setting is on (node/hooksplugin.go, node/livereply.go).
const DisplayEvent = "MessageDisplay"

// displayWait bounds a MessageDisplay hook. Claude Code shows the lines only once its MessageDisplay hooks are done,
// "async" or not (2.1.319), so the hook hands them to the node and goes: a node that does not take them at once costs
// the terminal no more than this. A node answers before it does anything with them, within a millisecond; a hook that
// gave up after writing them still had them taken.
const displayWait = 150 * time.Millisecond

// isDisplay says whether a hook's input is a MessageDisplay one.
func isDisplay(input json.RawMessage) bool {
	if !bytes.Contains(input, []byte(DisplayEvent)) {
		return false
	}
	var in struct {
		Event string `json:"hook_event_name"`
	}
	return json.Unmarshal(input, &in) == nil && in.Event == DisplayEvent
}

// Display hands a MessageDisplay hook's input to the node, which answers as soon as it has it, and prints nothing: what
// the hook prints would replace the lines on the terminal. No node, or one that does not answer in displayWait: the
// lines are not streamed, nothing else changes.
func Display(ctx context.Context, input json.RawMessage) {
	if _, err := os.Stat(SocketPath()); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, displayWait)
	defer cancel()
	_, _ = Dial(ctx, Request{Kind: "hook.display", Input: input})
}

// HookAncestors bounds how far up from a hook's parent AgentOf looks for its claude (a launcher, a shell, a wrapper).
const HookAncestors = 6

// AgentOf finds the claude this hook process runs under (FindAgent), by the session id in its input.
func AgentOf(input json.RawMessage) (pid int, start string) {
	var who struct {
		SessionID string `json:"session_id"`
	}
	_ = json.Unmarshal(input, &who)
	home, _ := os.UserHomeDir()
	return FindAgent(who.SessionID, os.Getppid(), ClaudeDir(home), procinfo.Parent)
}

// FindAgent finds the claude a hook runs under: walking up from the hook's parent, the first process whose
// sessions/<pid>.json names this session. That file is Claude Code's own, so a shell, a launcher or a wrapper in between
// is never taken for it. The process' start comes with it where the platform can tell (start is "" otherwise). Not
// finding it (the file is written a few seconds after claude starts, or this is not a claude) is pid 0.
func FindAgent(sessionID string, from int, dir string, parent func(int) (int, error)) (pid int, start string) {
	if sessionID == "" {
		return 0, ""
	}
	for i, p := 0, from; i < HookAncestors && p > 1; i++ {
		if ReadClaudeSessionID(filepath.Join(dir, "sessions", strconv.Itoa(p)+".json")) == sessionID {
			return p, procinfo.StartMark(p)
		}
		next, err := parent(p)
		if err != nil {
			return 0, ""
		}
		p = next
	}
	return 0, ""
}

// ClaudeDir is the default Claude Code directory for home: $CLAUDE_CONFIG_DIR, else ~/.claude.
func ClaudeDir(home string) string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(home, ".claude")
}

// ValidSessionID says whether id looks like a Claude Code session id (a UUID; the node's validID too): 8 to 64 hex
// digits and hyphens, as `^[0-9a-fA-F-]{8,64}$` says. Every character it allows is one byte, so counting bytes is
// counting characters. A loop, not that regexp: the session list checks each row's id every second, and the match
// was about 8% of a resting node's CPU.
func ValidSessionID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		switch c := id[i]; {
		case '0' <= c && c <= '9', 'a' <= c && c <= 'f', 'A' <= c && c <= 'F', c == '-':
		default:
			return false
		}
	}
	return true
}

// ReadClaudeSessionID reads the session id out of a claude's sessions/<pid>.json ("" when missing or invalid).
func ReadClaudeSessionID(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var f struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(b, &f) != nil || !ValidSessionID(f.SessionID) {
		return ""
	}
	return f.SessionID
}
