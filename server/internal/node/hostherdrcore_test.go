package node

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/herdrcore"
	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/nodeclient"
	"github.com/nwverse/persona/server/internal/proto"
)

// herdr-core as the machine's engine (hostherdrcore.go, engine.go), driven through the node's requests against a fake
// herdr-core socket.

const hcCwd = "/tmp/hcproj"

// The claude terminals are live sessions, each named by its foreground claude's sessions/<pid>.json; other programs,
// another machine's terminals and finished ones are not; places come from the Herdr app's labels and document.
func TestHerdrCoreListsClaudeTerminals(t *testing.T) {
	n, f, hc, home := hcNode(t)
	for _, id := range []string{sidA, sidB, sidC} {
		writeTranscript(t, home, hcCwd, id, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	}
	writeClaudePID(t, home, 4242, sidA)
	writeClaudePID(t, home, 4343, sidB)
	writeClaudePID(t, home, 4444, sidC)
	writeClaudePID(t, home, 4545, "0d000000-0000-4000-8000-0000000000ff") // another machine's: never asked about
	finished := hcTerminal("t6", "claude-code", "done", 4646, "claude", hcCwd, "", "", "")
	finished["state"] = "finished"
	hc.setTerminals(
		hcTerminal("t1", "claude-code", "working", 4242, "claude", hcCwd, "S1", "T1", "proj / 1"),
		hcTerminal("t2", "grok-build", "idle", 5000, "grok", hcCwd, "S1", "T2", "proj / 2"),
		hcTerminal("t1b", "", "", 4343, "/usr/local/bin/claude", hcCwd, "", "", ""), // has not reported yet
		hcTerminal("t3", "claude-code", "error", 4444, "claude", hcCwd, "S1", "T2", "proj / 2"),
		hcTerminal("m/t7", "claude-code", "working", 4545, "claude", hcCwd, "", "", ""),
		finished,
	)
	hc.doc = map[string]any{"spaces": []any{map[string]any{"id": "S1", "name": "", "folder": hcCwd,
		"tabs": []any{map[string]any{"id": "T1", "name": ""}, map[string]any{"id": "T2", "name": "review"}}}}}

	m := byID(listSessions(t, n))
	if len(m) != 3 {
		t.Fatalf("listed %v", ids(listSessions(t, n)))
	}
	a, b, c := m[sidA], m[sidB], m[sidC]
	if a.PaneID != "herdr-core:t1" || a.Status != proto.StatusWorking || a.SpaceKind != host.HerdrCore ||
		a.SpaceID != "S1" || a.SpaceName != "proj" || a.SpaceIndex != 1 || a.SlotID != "T1" || a.SlotName != "1" || a.SlotIndex != 1 {
		t.Errorf("t1: %+v", a)
	}
	if b.PaneID != "herdr-core:t1b" || b.Status != proto.StatusUnknown || b.SpaceID != dirSpace+hcCwd || b.SpaceName != "hcproj" || b.SlotID != "" {
		t.Errorf("t1b: %+v", b)
	}
	if c.PaneID != "herdr-core:t3" || c.Status != proto.StatusUnknown || c.SlotName != "review" || c.SlotIndex != 2 {
		t.Errorf("t3: %+v", c)
	}
	// Only the terminals that could be claude's are asked about, and only for their processes; herdr is not asked.
	for _, call := range hc.trace() {
		if !strings.HasPrefix(call, "terminal.get ") || strings.Contains(call, "m/t7") || strings.Contains(call, `"t6"`) {
			t.Errorf("herdr-core call %s", call)
		}
	}
	if tr := f.trace(); len(tr) != 0 {
		t.Errorf("herdr called: %v", tr)
	}
}

// A terminal another terminal started is a side session under the session of its nearest listed ancestor terminal
// (terminal.parent): it keeps its own state, and the manager draws it under that row.
func TestHerdrCoreLinksStartedTerminals(t *testing.T) {
	n, _, hc, home := hcNode(t)
	for _, id := range []string{sidA, sidB, sidC} {
		writeTranscript(t, home, hcCwd, id, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	}
	writeClaudePID(t, home, 4242, sidA)
	writeClaudePID(t, home, 4343, sidB)
	writeClaudePID(t, home, 4444, sidC)
	lead := hcTerminal("t1", "claude-code", "idle", 4242, "claude", hcCwd, "", "", "")
	shell := hcTerminal("t2", "", "", 5000, "zsh", hcCwd, "", "", "")
	shell["parent"] = "t1"
	child := hcTerminal("t3", "claude-code", "working", 4343, "claude", hcCwd, "", "", "")
	child["parent"] = "t2"
	grandchild := hcTerminal("t4", "claude-code", "done", 4444, "claude", hcCwd, "", "", "")
	grandchild["parent"] = "t3"
	hc.setTerminals(lead, shell, child, grandchild)

	m := byID(listSessions(t, n))
	leadRow, childRow, grandchildRow := m[sidA], m[sidB], m[sidC]
	if got := childRow.ParentAgentID(); got != sidA {
		t.Errorf("child parent_agent_id = %q, want %q", got, sidA)
	}
	if got := grandchildRow.ParentAgentID(); got != sidB {
		t.Errorf("grandchild parent_agent_id = %q, want %q", got, sidB)
	}
	if got := leadRow.ParentAgentID(); got != "" {
		t.Errorf("lead parent_agent_id = %q", got)
	}
}

func TestHerdrCoreStatusAndAgent(t *testing.T) {
	cases := []struct {
		app, state, fg string // fg: the foreground's argv[0], "" for none
		agent, status  string
	}{
		{"claude-code", "working", "claude", "claude", proto.StatusWorking},
		{"claude-code", "blocked", "claude", "claude", proto.StatusBlocked},
		{"claude-code", "done", "claude", "claude", proto.StatusDone},
		{"claude-code", "idle", "claude", "claude", proto.StatusIdle},
		{"claude-code", "error", "claude", "claude", proto.StatusUnknown},
		{"codex", "working", "codex", "codex", proto.StatusWorking},
		{"grok-build", "idle", "grok", "", ""},            // not a program the node drives: left for a compatible CLI
		{"", "", "claude", "claude", proto.StatusUnknown}, // has not reported yet
		{"", "", "/opt/bin/codex", "codex", proto.StatusUnknown},
		{"", "", "zsh", "", ""},
		{"claude-code", "done", "zsh", "", ""},      // claude exited: its last state stays on the terminal
		{"claude-code", "done", "-zsh", "", ""},     // a login shell
		{"claude-code", "working", "", "", ""},      // nothing in the foreground
		{"claude-code", "working", "herdr", "", ""}, // classic herdr inside
	}
	for _, c := range cases {
		var tm herdrcore.Terminal
		tm.Address, tm.Cwd = "t1", hcCwd
		if c.app != "" {
			tm.Status = &herdrcore.Status{App: c.app, State: c.state}
		}
		if c.fg != "" {
			tm.Foreground = &herdrcore.Foreground{Pid: 1, Name: "2.1.0", Argv: []string{c.fg}}
		}
		a := fromTerminal(tm)
		if a.Agent != c.agent || a.AgentStatus != c.status || a.PaneID != "herdr-core:t1" {
			t.Errorf("%s/%s/%s: agent %q status %q pane %q", c.app, c.state, c.fg, a.Agent, a.AgentStatus, a.PaneID)
		}
	}
	if got := stripSpinner("✳ fix the build"); got != "fix the build" {
		t.Errorf("stripSpinner = %q", got)
	}
	if got := stripSpinner("a fix"); got != "a fix" {
		t.Errorf("stripSpinner = %q", got)
	}
}

// hcSession is a node with one claude session in herdr-core terminal t1.
func hcSession(t *testing.T) (*Node, *fakeHC) {
	t.Helper()
	n, _, hc, home := hcNode(t)
	writeTranscript(t, home, hcCwd, sidA, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	writeClaudePID(t, home, 4242, sidA)
	hc.setTerminals(hcTerminal("t1", "claude-code", "idle", 4242, "claude", hcCwd, "", "", ""))
	if m := byID(listSessions(t, n)); m[sidA].PaneID != "herdr-core:t1" {
		t.Fatalf("not listed: %v", m)
	}
	hc.reset()
	return n, hc
}

// A send reads the plain screen once (no colors: only a dialog refuses it, a draft cannot be told from the
// placeholder), looks that the agent still runs there, and pastes and submits in one terminal.send; nothing is pressed
// after it.
func TestHerdrCoreSendChecksForADialogThenSends(t *testing.T) {
	withSubmitGrace(t, 50*time.Millisecond)
	n, hc := hcSession(t)
	hc.setScreen("t1", screenFixture(t, "claude-2.1.312-typed-162.txt"))
	if _, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond) // past the grace: ensureSubmitted has nothing colored to read
	assertTrace(t, hc.trace(), []string{
		call("terminal.read", map[string]any{"terminal": "t1", "lines": 0}),
		call("terminal.get", map[string]any{"terminal": "t1"}),
		call("terminal.send", map[string]any{"terminal": "t1", "text": "hello"}),
	})

	hc.reset()
	hc.setScreen("t1", "Do you want to proceed?\n❯ 1. Yes\n  2. No\n\nEsc to cancel\n")
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
	if code, _ := refusalOf(t, err); code != "dialog_open" {
		t.Fatalf("send into a dialog: %v", err)
	}
	assertTrace(t, hc.trace(), []string{call("terminal.read", map[string]any{"terminal": "t1", "lines": 0})})
}

// terminal.send pastes the text: an escape in it that would end the paste and press keys after it goes in inert, from a
// send and from any other prompt.
func TestHerdrCoreSendCannotEndItsPaste(t *testing.T) {
	withSubmitGrace(t, 50*time.Millisecond)
	n, hc := hcSession(t)
	hc.setScreen("t1", screenFixture(t, "claude-2.1.312-typed-162.txt"))
	if _, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello\x1b[201~\x1b[Z"}); err != nil {
		t.Fatal(err)
	}
	if err := n.prompt(context.Background(), "herdr-core:t1", "go on\x1b[201~\x1b[Z\u009b"); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, hc.trace("terminal.send"), []string{
		call("terminal.send", map[string]any{"terminal": "t1", "text": "hello<ESC>[201~<ESC>[Z"}),
		call("terminal.send", map[string]any{"terminal": "t1", "text": "go on<ESC>[201~<ESC>[Z<U+009B>"}),
	})
}

// herdr-core refusing the text (nothing typed) is a refusal the app can retry: a dialog for blocked, else not_delivered.
func TestHerdrCoreSendRefusedByHerdrCore(t *testing.T) {
	n, hc := hcSession(t)
	for kind, code := range map[string]string{"blocked": "dialog_open", "unavailable": "not_delivered",
		"status_mismatch": "not_delivered", "finished": "not_delivered"} {
		hc.handle("terminal.send", func(map[string]any) any { return hcFail{kind} })
		_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
		if got, data := refusalOf(t, err); got != code || data["delivered"] != "none" {
			t.Errorf("%s: %v %v", kind, got, data)
		}
	}
}

// The claude exited and the terminal is back at its shell, its last state still on it: the shell does not get the
// message as a command — refused before anything is typed.
func TestHerdrCoreSendAfterTheAgentExited(t *testing.T) {
	n, hc := hcSession(t)
	exited := hcTerminal("t1", "claude-code", "done", 4242, "claude", hcCwd, "", "", "")
	exited["foreground"] = map[string]any{"pid": 4241, "name": "zsh", "argv": []string{"-zsh"}, "cwd": hcCwd}
	hc.setTerminals(exited)
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "rm -rf build"})
	if code, _ := refusalOf(t, err); code != "not_delivered" {
		t.Fatalf("send to an exited agent: %v", err)
	}
	if tr := hc.trace("terminal.send"); len(tr) != 0 {
		t.Errorf("typed: %v", tr)
	}
	if m := byID(listSessions(t, n)); m[sidA].PaneID != "" {
		t.Errorf("an exited agent still listed live: %+v", m[sidA])
	}
}

// Keys go through as named; text and enter go as a prompt; any other text is refused before a key is pressed —
// including a free-text dialog answer, whose keys would open the field first.
func TestHerdrCoreKeysAndText(t *testing.T) {
	n, hc := hcSession(t)
	if _, err := request(t, n, "session.keys", map[string]any{"session_id": sidA, "keys": []string{"esc", "shift+tab"}}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, hc.trace(), []string{call("terminal.keys", map[string]any{"terminal": "t1", "keys": []any{"esc", "shift+tab"}})})
	hc.reset()
	if _, err := request(t, n, "session.keys", map[string]any{"session_id": sidA, "text": "yes please", "keys": []string{"enter"}}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, hc.trace(), []string{
		call("terminal.get", map[string]any{"terminal": "t1"}),
		call("terminal.send", map[string]any{"terminal": "t1", "text": "yes please"}),
	})
	for _, params := range []map[string]any{
		{"session_id": sidA, "text": "/effort"},
		{"session_id": sidA, "text": "use the other one", "keys": []string{"enter"}, "pre_keys": []string{"3"}},
	} {
		hc.reset()
		_, err := request(t, n, "session.keys", params)
		if code, _ := refusalOf(t, err); code != "cannot_type" {
			t.Errorf("%v: %v", params, err)
		}
		if tr := hc.trace(); len(tr) != 0 {
			t.Errorf("%v pressed %v", params, tr)
		}
	}
}

// A colored screen read gets the plain rows (CRLF-ended like colored ones); "recent" reads the screen too (asking for
// history would scroll a full-screen program).
func TestHerdrCoreScreenIsPlain(t *testing.T) {
	n, hc := hcSession(t)
	hc.setScreen("t1", "line one\nline two\n")
	res, err := request(t, n, "session.screen", map[string]any{"session_id": sidA, "format": "ansi"})
	if err != nil {
		t.Fatal(err)
	}
	if res["text"] != "line one\r\nline two\r\n" || res["status"] != proto.StatusIdle {
		t.Errorf("screen = %q", res)
	}
	if _, err := request(t, n, "session.screen", map[string]any{"session_id": sidA, "source": "recent", "lines": 500}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, hc.trace(), []string{
		call("terminal.read", map[string]any{"terminal": "t1", "lines": 0}),
		call("terminal.read", map[string]any{"terminal": "t1", "lines": 0}),
	})
}

// A live stream is not available: the hub falls back to screen reads.
func TestHerdrCoreHasNoTerminalStream(t *testing.T) {
	n, _ := hcSession(t)
	// herdr can stream here (the process-wide cache terminal_test.go sets the same way): only herdr-core cannot.
	observeSupport.once.Do(func() { observeSupport.ok = true })
	_, err := request(t, n, "terminal.watch", map[string]any{"session_id": sidA})
	if err == nil || !strings.HasPrefix(err.Error(), "unsupported") {
		t.Fatalf("terminal.watch = %v", err)
	}
}

// Events from herdr-core take a snapshot at once: the subscription's start, then each change but size / keep and
// another machine's.
func TestHerdrCoreEventsKickARefresh(t *testing.T) {
	n, _, hc, _ := hcNode(t)
	sig := newWatcherSignals()
	n.hostSig = map[string]*watcherSignals{host.HerdrCore: sig}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.watchHost(ctx, n.hosts.Get(host.HerdrCore).(host.Watcher), sig)
	select {
	case <-hc.subbed:
	case <-time.After(3 * time.Second):
		t.Fatal("never subscribed")
	}
	kicked := func() bool {
		select {
		case <-sig.kick:
			return true
		case <-time.After(300 * time.Millisecond):
			return false
		}
	}
	if !kicked() {
		t.Fatal("no snapshot once the stream started")
	}
	hc.push("event", map[string]any{"type": "size", "terminal": "t1", "seq": 1, "data": map[string]any{}})
	hc.push("event", map[string]any{"type": "status", "terminal": "m/t1", "seq": 2, "data": map[string]any{}})
	hc.push("heartbeat", map[string]any{"seq": 2})
	if kicked() {
		t.Fatal("a size / another machine's / heartbeat event took a snapshot")
	}
	hc.push("event", map[string]any{"type": "status", "terminal": "t1", "seq": 3, "data": map[string]any{}})
	if !kicked() {
		t.Fatal("a status event took no snapshot")
	}
	hc.push("missed", map[string]any{"seq": 9, "terminals": []any{}})
	if !kicked() {
		t.Fatal("missed events took no snapshot")
	}
	// A status into or out of blocked is a card's: urgent (#751); working → done is not.
	sig.urgent.Store(false)
	status := func(seq int, terminal, state string) bool {
		hc.push("event", map[string]any{"type": "status", "terminal": terminal, "seq": seq,
			"data": map[string]any{"status": map[string]any{"app": "claude-code", "state": state}}})
		if !kicked() {
			t.Fatalf("status %s took no snapshot", state)
		}
		return sig.urgent.Swap(false)
	}
	// t1 said a status (none) above; t2 is new.
	if status(10, "t1", "working") || status(11, "t1", "done") || !status(12, "t1", "blocked") || status(13, "t1", "blocked") ||
		!status(14, "t1", "working") || !status(15, "t2", "idle") {
		t.Fatal("urgent only into or out of blocked, and on a terminal's first status")
	}
}

// herdr-core not running: no sessions, at once, nothing logged (and the refresh succeeds: nothing runs there); one
// that went away takes its sessions with it.
func TestHerdrCoreAbsentOrGone(t *testing.T) {
	n, _, hc, home := hcNode(t)
	writeClaudePID(t, home, 4242, sidB)
	writeTranscript(t, home, hcCwd, sidB, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	hc.setTerminals(hcTerminal("t1", "claude-code", "idle", 4242, "claude", hcCwd, "", "", ""))
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := n.agent(sidB); !ok {
		t.Fatal("herdr-core's session not found")
	}
	if err := os.Remove(hc.sock); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second) // past the failed list kept for Agents
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := n.agent(sidB); ok {
		t.Error("herdr-core stopped, its session is still live")
	}

	var logged []string
	var mu sync.Mutex
	c := &herdrcore.Client{Socket: "/tmp/persona-no-such-dir/c.sock", Log: func(format string, args ...any) {
		mu.Lock()
		logged = append(logged, fmt.Sprintf(format, args...))
		mu.Unlock()
	}}
	n.hosts.Set(&herdrCoreHost{c: c})
	start := time.Now()
	for range 3 {
		if err := n.refreshHosts(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("3 refreshes took %s", took)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(logged) != 0 {
		t.Errorf("logged %q", logged)
	}
}

// With herdr-core as the engine, a terminal running classic herdr's client is not claude's, whatever state passes
// through it.
func TestHerdrCoreSkipsANestedHerdr(t *testing.T) {
	n, _, hc, home := hcNode(t)
	writeClaudePID(t, home, 4242, sidA)
	writeTranscript(t, home, hcCwd, sidA, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	nested := hcTerminal("t1", "claude-code", "working", 4242, "/usr/local/bin/herdr", hcCwd, "", "", "")
	hc.setTerminals(nested)
	if list := listSessions(t, n); len(byID(list)) != 0 && byID(list)[sidA].Status != proto.StatusOffline {
		t.Errorf("a nested herdr became a session: %+v", byID(list)[sidA])
	}
	if _, ok := n.agent(sidA); ok {
		t.Error("a nested herdr is an agent")
	}
	for _, fg := range []herdrcore.Foreground{{Pid: 1, Name: "herdr"}, {Pid: 1, Name: "x", Argv: []string{"herdr"}}} {
		tm := herdrcore.Terminal{Address: "t2", Status: &herdrcore.Status{App: "claude-code", State: "idle"}, Foreground: &fg}
		if a := fromTerminal(tm); a.Agent != "" {
			t.Errorf("%+v: agent %q", fg, a.Agent)
		}
	}
}

// herdr-core opens Windows PowerShell by default on Windows: its terminal is a shell to start in and to type into.
func TestHerdrCoreWindowsPowerShellIsAShell(t *testing.T) {
	for _, name := range []string{"powershell.exe", `C:\WINDOWS\System32\WindowsPowerShell\v1.0\powershell.exe`, "pwsh.exe", "PowerShell.EXE", "zsh", "-bash"} {
		if !hcShell(name) {
			t.Errorf("hcShell(%q) = false", name)
		}
	}
	for _, name := range []string{"claude.exe", "node", "cmd.exe", ""} {
		if hcShell(name) {
			t.Errorf("hcShell(%q) = true", name)
		}
	}
	fg := herdrcore.Foreground{Pid: 1, Name: "powershell.exe", Argv: []string{`C:\WINDOWS\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoExit"}}
	if !isShell(&fg) {
		t.Errorf("isShell(%+v) = false", fg)
	}
}

// On Windows herdr-core names the first program below the shell as in front: a start's claude runs three bash deep.
// The nearest agent below it is found in the process table, and its terminal is claude's (#11214).
func TestHerdrCoreAgentBelowTheReportedForeground(t *testing.T) {
	procs := map[int]proc{
		10: {cmd: "powershell.exe"},
		20: {ppid: 10, cmd: "bash.exe"}, 21: {ppid: 20, cmd: "bash.exe"}, 22: {ppid: 21, cmd: "bash.exe"},
		30: {ppid: 22, cmd: "claude.exe"}, 31: {ppid: 30, cmd: "claude.exe"}, 32: {ppid: 30, cmd: "bash.exe"},
		40: {ppid: 10, cmd: "node.exe"},
	}
	children := procChildren(procs)
	if got := agentBelow(procs, children, 20); got != 30 {
		t.Errorf("agentBelow(20) = %d, want 30 (the nearest claude)", got)
	}
	if got := agentBelow(procs, children, 40); got != 0 {
		t.Errorf("agentBelow(40) = %d, want none", got)
	}
	cycle := map[int]proc{1: {ppid: 2, cmd: "bash.exe"}, 2: {ppid: 1, cmd: "bash.exe"}}
	if got := agentBelow(cycle, procChildren(cycle), 1); got != 0 {
		t.Errorf("agentBelow in a cycle = %d", got)
	}
	tm := herdrcore.Terminal{Address: "t3", State: "live", Foreground: &herdrcore.Foreground{Pid: 30, Name: "claude.exe", Argv: []string{"claude.exe"}}}
	if a := fromTerminal(tm); a.Agent != "claude" {
		t.Errorf("claude.exe in front: agent %q", a.Agent)
	}
}

func TestHookPane(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i+1 < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, c := range []struct {
		env  func(string) string
		want string
	}{
		{env("HERDR_PANE_ID", "w1:p2", "HC_TERMINAL_ID", "t7"), "w1:p2"},
		{env("HC_TERMINAL_ID", "t7"), "herdr-core:t7"},
		{env("TERN_PANE", "42"), "tern:42"},
		{env("HERDR_PANE_ID", "w1:p2", "TERN_PANE", "42"), "w1:p2"}, // herdr's client in a Tern tab: the agent is herdr's
		{env("REX_BLOCK", "block:b1"), "rex:block:b1"},
		{env("HERDR_PANE_ID", "w1:p2", "REX_BLOCK", "block:b1"), "w1:p2"}, // herdr's client in a Rex pane: the agent is herdr's
		{env(), ""},
	} {
		if got := nodeclient.HookPane(c.env); got != c.want {
			t.Errorf("HookPane = %q, want %q", got, c.want)
		}
	}
}

// session.close: ctrl+c twice, then terminal.kill — what closing a pane in the Herdr app does (the app then drops the
// pane). A terminal already finished or gone counts as closed; one still live after the kill is an error.
func TestHerdrCoreClose(t *testing.T) {
	n, hc := hcSession(t)
	hc.handle("terminal.kill", func(p map[string]any) any {
		k := hcTerminal("t1", "", "", 0, "", hcCwd, "", "", "")
		k["state"] = "finished"
		return k
	})
	if _, err := request(t, n, "session.close", map[string]any{"session_id": sidA}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, hc.trace(), []string{
		call("terminal.keys", map[string]any{"terminal": "t1", "keys": []any{"ctrl+c"}}),
		call("terminal.keys", map[string]any{"terminal": "t1", "keys": []any{"ctrl+c"}}),
		call("terminal.kill", map[string]any{"terminal": "t1"}),
	})

	// Already gone: the keys fail, the kill finds nothing; closed all the same.
	for _, kind := range []string{"finished", "not_found"} {
		n, hc := hcSession(t)
		hc.handle("terminal.keys", func(map[string]any) any { return hcFail{"finished"} })
		if kind == "not_found" {
			hc.handle("terminal.kill", func(map[string]any) any { return hcFail{"not_found"} })
		} else {
			hc.handle("terminal.kill", func(map[string]any) any {
				k := hcTerminal("t1", "", "", 0, "", hcCwd, "", "", "")
				k["state"] = "finished"
				return k
			})
		}
		if _, err := request(t, n, "session.close", map[string]any{"session_id": sidA}); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
		if tr := hc.trace("terminal.kill"); len(tr) != 1 {
			t.Errorf("%s: kills %v", kind, tr)
		}
	}

	// Still live after the kill (the core did not end it): not closed.
	n, hc = hcSession(t)
	hc.handle("terminal.kill", func(map[string]any) any { return hcTerminal("t1", "", "", 0, "", hcCwd, "", "", "") })
	if _, err := request(t, n, "session.close", map[string]any{"session_id": sidA}); err == nil {
		t.Error("closed a terminal that still runs")
	}
}

// A session herdr-core never had cannot be closed; the refusal does not name an engine.
func TestHerdrCoreCloseUnknown(t *testing.T) {
	n, hc := hcSession(t)
	_, err := request(t, n, "session.close", map[string]any{"session_id": "0d000000-0000-4000-8000-0000000000ff"})
	if err == nil || strings.Contains(err.Error(), "herdr") {
		t.Fatalf("close of an unknown session: %v", err)
	}
	if tr := hc.trace(); len(tr) != 0 {
		t.Errorf("herdr-core called: %v", tr)
	}
}
