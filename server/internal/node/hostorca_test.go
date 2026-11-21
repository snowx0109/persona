package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/nodeclient"
	"github.com/nwverse/persona/server/internal/orcart"
	"github.com/nwverse/persona/server/internal/orcart/orcarttest"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rexcli"
	"github.com/nwverse/persona/server/internal/rpc"
	"github.com/nwverse/persona/server/internal/terncli"
)

// Orca as the machine's engine (hostorca.go, engine.go), driven through the node's requests against a fake Orca
// runtime socket (orcarttest) and a fake process table. No test here talks to a real Orca.

func TestMain(m *testing.M) {
	if os.Getenv("NODE_TEST_LAUNCH") == "1" { // the real managed-launch path, before the test harness clears its environment
		if err := Launch(context.Background(), os.Args[2], os.Args[3:], ""); err != nil {
			panic(err)
		}
		os.Exit(0)
	}
	if mode := os.Getenv("NODE_TEST_PLUGIN"); mode != "" { // the test binary as a plugin's program (plugins_test.go)
		runTestPlugin(mode)
	}
	if os.Getenv("NODE_TEST_PREFLIGHT") == "1" { // the test binary as the preflight's child (preflight_test.go)
		// NODE_TEST_PREFLIGHT_SLOW: start that much later, as on a busy machine; NODE_TEST_PREFLIGHT_STARTED: a file
		// to write the time to once running, for a test that times the run without the start.
		if d, err := time.ParseDuration(os.Getenv("NODE_TEST_PREFLIGHT_SLOW")); err == nil {
			time.Sleep(d)
		}
		if f := os.Getenv("NODE_TEST_PREFLIGHT_STARTED"); f != "" {
			_ = os.WriteFile(f, []byte(time.Now().Format(time.RFC3339Nano)), 0o600)
		}
		preflightChild(os.Args[1:], os.Stdout)
		os.Exit(0)
	}
	// A plain run of the whole package: its tests in concurrent processes of this binary (partition_test.go, #5734).
	if code, ran := runPartitioned(); ran {
		os.Exit(code)
	}
	// Nor `node doctor` as the preflight's child: the test binary again, in that mode, given long enough to come up
	// under the whole server suite's load (#1110); only a child that never starts waits that long.
	preflightChildCmd = func(string) ([]string, []string) {
		self, _ := os.Executable()
		return []string{self}, []string{"NODE_TEST_PREFLIGHT=1"}
	}
	preflightStart = 30 * time.Second
	// No test may reach the user's running Orca: the default metadata path points nowhere.
	os.Setenv("ORCA_USER_DATA_PATH", "/tmp/persona-test-no-orca")
	// Nor the user's botmux: a node registers no botmux host unless a test gives it one.
	botmuxBin = func() string { return "" }
	// Nor the user's Tern: no tern binary is found (a test that wants one gives its client a fake).
	ternClient = func() *terncli.Client { return &terncli.Client{Bin: func() string { return "" }} }
	// Nor the user's Rex: no rex binary is found (a test that wants one gives its client a fake).
	rexClient = func() *rexcli.Client { return &rexcli.Client{Bin: func() string { return "" }} }
	// Nor the user's gh outside the test's PATH (ghpath.go).
	ghFallbackDirs = func() []string { return nil }
	// Nor the user's Codex catalog: a model recovery on a Codex list runs no `codex debug models` (a test that wants
	// windows gives its own, assistant_fallback_window.go).
	codexDebugModels = func(context.Context, *Node, string) ([]byte, error) { return nil, errors.New("no codex in tests") }
	// Subscription usage tests provide their own app-server instead of polling a real account.
	codexUsageCLI = func() (string, error) { return "", errors.New("no codex in tests") }
	// Nor the ChatGPT app's bundled codex: a test that wants one sets its own (codex_bundled_test.go).
	bundledCodexCLI = func() string { return "" }
	// Nor the user's Paseo: its home points nowhere (a test that wants one sets its own).
	os.Setenv("PASEO_HOME", "/tmp/persona-test-no-paseo")
	// Nor the user's Cindy: its home points nowhere (a test that wants one sets its own).
	os.Setenv("CINDY_HOME", "/tmp/persona-test-no-cindy")
	// Nor the user's Claude config directory (claude under a profile, or these tests run from inside a Claude Code
	// session): the node's own default is then ~/.claude of the test's home (#2140; a test that wants one sets its own).
	os.Unsetenv("CLAUDE_CONFIG_DIR")
	// Nor the user's state directory (these tests run from inside a Persona conversation, or a shell that set it): the
	// state directory of a test's node is the one under its own home. A PERSONA_HOME, or its former name, left in the
	// environment won over it, and the health uploads' tests wrote their fake reports into the user's real health
	// space. A test that wants one sets its own.
	for _, k := range []string{"PERSONA_HOME", legacyEnv("PERSONA_HOME")} {
		os.Unsetenv(k)
	}
	// Nor the user's login shell: herdr's panes run zsh, whatever the machine's is (a test that wants another sets it).
	herdrPaneShell = func() string { return "/bin/zsh" }
	// A send looks at the box once after typing and confirms nothing: most fakes never write what was typed into the
	// transcript, and would wait out the window (#6428). sendconfirm_test.go turns it on (withSubmitWindow).
	submitWindow = 0
	os.Exit(m.Run())
}

const orcaCwd = "/tmp/orcaproj"

// fakeProcs is the process table the Orca host scans: agent processes by pane key.
type fakeProcs struct {
	mu    sync.Mutex
	procs map[string][]orcaProc
	err   error
	scans int
}

func (p *fakeProcs) scan() (map[string][]orcaProc, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scans++
	out := map[string][]orcaProc{}
	for k, v := range p.procs {
		out[k] = slices.Clone(v)
	}
	return out, p.err
}

func (p *fakeProcs) set(pane string, procs ...orcaProc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.procs == nil {
		p.procs = map[string][]orcaProc{}
	}
	if len(procs) == 0 {
		delete(p.procs, pane)
		return
	}
	p.procs[pane] = procs
}

// orcaNode is a Node whose engine is Orca, on a fake runtime and process table; the fake herdr it also gets
// (herdrNode's) is not registered, so a call to it would show in its trace.
func orcaNode(t *testing.T) (*Node, *fakeHerdr, *orcarttest.Fake, *fakeProcs, string) {
	t.Helper()
	n, f, home := herdrNode(t)
	o := orcarttest.New(t)
	procs := &fakeProcs{}
	h := newOrcaHost(&orcart.Client{MetaPath: o.MetaPath})
	h.scan = procs.scan
	n.hosts = host.NewRegistry(h)
	n.eng.name = proto.EngineOrca
	t.Cleanup(n.starts.close) // before the fake Orca runtime closes
	return n, f, o, procs, home
}

func orcaTerm(handle, tab, leaf, identity string) map[string]any {
	return orcarttest.Terminal(handle, tab, leaf, "repo::"+orcaCwd, orcaCwd, "refs/heads/feature-x", "✳ "+handle, identity)
}

// The claude terminals are live sessions, each named by its claude process's sessions/<pid>.json (found by the pane key
// in its environment); a terminal whose claude exited (Orca still says claude), a shell and another machine's terminal
// are not; spaces are worktrees named by branch, slots their tabs; states come from worktree.ps (waiting → blocked).
func TestOrcaListsClaudeTerminals(t *testing.T) {
	n, f, o, procs, home := orcaNode(t)
	for _, id := range []string{sidA, sidB, sidC} {
		writeTranscript(t, home, orcaCwd, id, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	}
	writeClaudePID(t, home, 4242, sidA)
	writeClaudePID(t, home, 4343, sidB)
	writeClaudePID(t, home, 4444, sidC)
	remote := orcaTerm("term_r", "tabR", "leafR", "claude")
	remote["executionHostId"] = "ssh-test-host"
	o.SetTerminals(
		orcaTerm("term_1", "tab1", "leaf1", "claude"),
		orcaTerm("term_2", "tab1", "leaf2", "claude"),
		orcaTerm("term_3", "tab2", "leaf3", "claude"), // no row yet: unknown
		orcaTerm("term_4", "tab2", "leaf4", "claude"), // its claude exited
		orcaTerm("term_5", "tab3", "leaf5", ""),       // a shell
		remote,
	)
	o.SetWorktrees(orcarttest.Worktree("repo::"+orcaCwd, orcaCwd, "refs/heads/feature-x",
		orcarttest.Agent("tab1:leaf1", "claude", "working"),
		orcarttest.Agent("tab1:leaf2", "claude", "waiting"),
		orcarttest.Agent("tab2:leaf4", "claude", "done"),
	))
	procs.set("tab1:leaf1", orcaProc{pid: 4242, agent: "claude"})
	procs.set("tab1:leaf2", orcaProc{pid: 4343, agent: "claude"})
	procs.set("tab2:leaf3", orcaProc{pid: 4444, agent: "claude"})
	procs.set("tabR:leafR", orcaProc{pid: 4545, agent: "claude"}) // cannot happen, but must not matter

	list := listSessions(t, n)
	m := byID(list)
	a, b, c := m[sidA], m[sidB], m[sidC]
	if a.PaneID != "orca:tab1:leaf1" || a.Status != proto.StatusWorking || a.SpaceKind != host.Orca ||
		a.SpaceID != "repo::"+orcaCwd || a.SpaceName != "feature-x" || a.SpaceIndex != 1 || a.SlotID != "tab1" ||
		a.SlotName != "tab tab1" || a.SlotIndex != 1 {
		t.Errorf("term_1: %+v", a)
	}
	if b.PaneID != "orca:tab1:leaf2" || b.Status != proto.StatusBlocked {
		t.Errorf("term_2: %+v", b)
	}
	if c.PaneID != "orca:tab2:leaf3" || c.Status != proto.StatusUnknown || c.SlotIndex != 2 {
		t.Errorf("term_3: %+v", c)
	}
	live := 0
	for _, s := range list {
		if s.PaneID != "" {
			live++
		}
	}
	if live != 3 {
		t.Errorf("%d live sessions: %v", live, ids(list))
	}
	// Listing reads; it neither types nor reads screens: only the claude without a row is asked about (Orca's own
	// reading of the terminal, the state when its hooks have none); herdr is not asked.
	if tr := o.Trace(); strings.Join(tr, "|") != `terminal.agentStatus {"terminal":"term_3"}` {
		t.Errorf("orca calls beyond listing: %v", tr)
	}
	if inv := o.Invalid(); len(inv) != 0 {
		t.Errorf("params Orca would refuse: %v", inv)
	}
	if tr := f.trace(); len(tr) != 0 {
		t.Errorf("herdr called: %v", tr)
	}
}

// When the processes cannot be listed, Orca's own word decides: its agent identity and a live agent row.
func TestOrcaWithoutProcessScan(t *testing.T) {
	v := &orcaView{rows: map[string]orcart.AgentRow{
		"t:a": {PaneKey: "t:a", AgentType: "claude", State: "done", HasLivePty: true},
		"t:b": {PaneKey: "t:b", AgentType: "claude", State: "working"},
	}}
	term := func(leaf, identity string) orcart.Terminal {
		return orcart.Terminal{Handle: "h", TabID: "t", LeafID: leaf, AgentIdentity: identity}
	}
	if a := v.agent(term("a", "claude")); a.Agent != "claude" || a.AgentStatus != proto.StatusDone {
		t.Errorf("a: %+v", a)
	}
	for _, tm := range []orcart.Terminal{term("b", "claude"), term("a", ""), term("c", "claude")} {
		if a := v.agent(tm); a.Agent != "" {
			t.Errorf("%s: an agent %+v", tm.LeafID, a)
		}
	}
	// With the scan, a codex process is codex; its row's state counts only when the row is codex's.
	v = &orcaView{procsKnown: true, procs: map[string][]orcaProc{"t:a": {{pid: 1, agent: "codex"}}},
		rows: map[string]orcart.AgentRow{"t:a": {AgentType: "claude", State: "working"}}}
	if a := v.agent(term("a", "codex")); a.Agent != "codex" || a.AgentStatus != proto.StatusUnknown {
		t.Errorf("codex: %+v", a)
	}
	// The row folds in its subagents (working); the pane's own claude finished its turn.
	row := orcart.AgentRow{AgentType: "claude", State: "working"}
	row.MainAgent = &struct {
		State string `json:"state"`
	}{State: "done"}
	v = &orcaView{procsKnown: true, procs: map[string][]orcaProc{"t:a": {{pid: 1, agent: "claude"}}},
		rows: map[string]orcart.AgentRow{"t:a": row}}
	if a := v.agent(term("a", "claude")); a.AgentStatus != proto.StatusDone {
		t.Errorf("main agent done, children working: %+v", a)
	}
}

// orcaSession is a node with one claude session in Orca's terminal term_1 (pane tab1:leaf1).
func orcaSession(t *testing.T) (*Node, *orcarttest.Fake, *fakeProcs) {
	t.Helper()
	n, _, o, procs, home := orcaNode(t)
	writeTranscript(t, home, orcaCwd, sidA, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	writeClaudePID(t, home, 4242, sidA)
	o.SetTerminals(orcaTerm("term_1", "tab1", "leaf1", "claude"))
	o.SetWorktrees(orcarttest.Worktree("repo::"+orcaCwd, orcaCwd, "refs/heads/feature-x",
		orcarttest.Agent("tab1:leaf1", "claude", "done")))
	procs.set("tab1:leaf1", orcaProc{pid: 4242, agent: "claude"})
	if m := byID(listSessions(t, n)); m[sidA].PaneID != "orca:tab1:leaf1" {
		t.Fatalf("not listed: %v", m)
	}
	o.Reset()
	return n, o, procs
}

// A send reads the plain screen once (only a dialog refuses it: no colors to tell a draft from the placeholder),
// checks the claude is in front, pastes the text (our own bracketed paste, no enter), checks again and submits with a
// separate carriage return; nothing after.
func TestOrcaSendChecksForADialogThenPrompts(t *testing.T) {
	withSubmitGrace(t, 50*time.Millisecond)
	n, o, _ := orcaSession(t)
	o.SetScreen("term_1", screenFixture(t, "claude-2.1.312-typed-162.txt"))
	if _, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond) // past the grace: nothing colored to check, no enter pressed after
	assertTrace(t, o.Trace(), []string{
		`terminal.read {"screen":true,"terminal":"term_1"}`,
		`terminal.agentStatus {"terminal":"term_1"}`,
		orcarttest.SendCall("term_1", map[string]any{"text": "\x1b[200~hello\x1b[201~"}),
		`terminal.agentStatus {"terminal":"term_1"}`,
		orcarttest.SendCall("term_1", map[string]any{"text": "\r"}),
	})
	if w := o.Writes(); len(w) != 2 || w[0].Enter || w[0].Pasted || w[1].Text != "\r" {
		t.Errorf("writes %+v", w)
	}
	if inv := o.Invalid(); len(inv) != 0 {
		t.Errorf("params Orca would refuse: %v", inv)
	}

	o.Reset()
	o.SetScreen("term_1", "Do you want to proceed?\n❯ 1. Yes\n  2. No\n\nEsc to cancel\n")
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
	if code, _ := refusalOf(t, err); code != "dialog_open" {
		t.Fatalf("send into a dialog: %v", err)
	}
	if tr := o.Trace("terminal.send"); len(tr) != 0 {
		t.Errorf("typed into a dialog: %v", tr)
	}
}

func TestOrcaSendsToForegroundTraex(t *testing.T) {
	n, _, o, procs, home := orcaNode(t)
	const pane = "tab1:leaf1"
	const command = "/opt/bin/traex"
	pid := os.Getpid() // the writer lock must name a live process owned by this user
	writeTraexOwner(t, home, threadMain, pid)
	traexCache.Lock()
	previous, present := traexCache.panes[pid]
	traexCache.panes[pid] = "orca:" + pane
	traexCache.Unlock()
	t.Cleanup(func() {
		traexCache.Lock()
		defer traexCache.Unlock()
		if present {
			traexCache.panes[pid] = previous
		} else {
			delete(traexCache.panes, pid)
		}
	})
	o.SetTerminals(orcaTerm("term_1", "tab1", "leaf1", traexAgent))
	o.SetScreen("term_1", "› Ask Codex to do anything\n")
	setProcess := func(state string) {
		args := fmt.Sprintf("%d %s 01:00 %s", pid, state, command)
		env := fmt.Sprintf("%d %s ORCA_PANE_KEY=%s", pid, command, pane)
		procs.set(pane, parseOrcaPSEnv(env, parseOrcaPSArgs(args, time.Now()), nil)[pane]...)
	}
	setProcess("S+")
	if s := byID(listSessions(t, n))[threadMain]; s.Agent != traexAgent || s.PaneID != "orca:"+pane {
		t.Fatalf("TraeX conversation is not bound to its terminal: %+v", s)
	}
	o.Reset()

	req := map[string]any{"session_id": threadMain, "text": "hello"}
	if _, err := request(t, n, "session.send", req); err != nil {
		t.Fatalf("send to foreground TraeX: %v", err)
	}
	if writes := o.Writes(); len(writes) != 2 || writes[0].Text != "\x1b[200~hello\x1b[201~" || writes[1].Text != "\r" {
		t.Fatalf("expected a bracketed paste and a separate submit, got %+v", writes)
	}

	o.Reset()
	setProcess("S") // the same process and pane still exist, but TraeX no longer owns the foreground group
	if _, err := request(t, n, "session.send", req); err == nil {
		t.Fatal("the background TraeX process accepted a send")
	} else if code, data := refusalOf(t, err); code != "not_delivered" || data["delivered"] != "none" {
		t.Fatalf("expected a refusal before typing, got %v", err)
	}
	if writes := o.Writes(); len(writes) != 0 {
		t.Fatalf("wrote into the terminal without TraeX in front: %+v", writes)
	}
}

// Orca turning the input down (nothing written) is a refusal the app can retry: a permission dialog is dialog_open,
// no agent in front / a gone or read-only terminal is not_delivered.
func TestOrcaSendRefusedByOrca(t *testing.T) {
	n, o, _ := orcaSession(t)
	o.SetScreen("term_1", "❯ \n")
	for _, c := range []struct {
		answer any
		code   string
	}{
		{map[string]any{"send": map[string]any{"accepted": false, "bytesWritten": 0, "refusedReason": "permission"}}, "dialog_open"},
		{map[string]any{"send": map[string]any{"accepted": false, "bytesWritten": 0, "refusedReason": "no-agent"}}, "not_delivered"},
		{orcarttest.Fail{Code: "terminal_not_writable"}, "not_delivered"},
	} {
		o.Handle("terminal.send", func(map[string]any) any { return c.answer })
		_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
		if got, data := refusalOf(t, err); got != c.code || data["delivered"] != "none" {
			t.Errorf("%v: %v %v", c.answer, got, data)
		}
	}
	// An error that leaves open whether Orca wrote: not a clean refusal.
	o.Handle("terminal.send", func(map[string]any) any { return orcarttest.Fail{Code: "runtime_error"} })
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
	var re *rpc.Error
	if err == nil || errors.As(err, &re) && re.Code == "not_delivered" {
		t.Errorf("runtime_error: %v", err)
	}
}

// The claude exited and the pane is back at its shell (Orca may still name claude): the shell does not get the
// message as a command — refused before anything is typed, and the session is no longer live.
func TestOrcaSendAfterTheAgentExited(t *testing.T) {
	n, o, procs := orcaSession(t)
	o.SetScreen("term_1", "$ \n")
	procs.set("tab1:leaf1")
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "rm -rf build"})
	if code, _ := refusalOf(t, err); code != "not_delivered" {
		t.Fatalf("send to an exited agent: %v", err)
	}
	if tr := o.Trace("terminal.send"); len(tr) != 0 {
		t.Errorf("typed: %v", tr)
	}
	time.Sleep(orcaProcsMaxAge)
	if m := byID(listSessions(t, n)); m[sidA].PaneID != "" {
		t.Errorf("an exited agent still listed live: %+v", m[sidA])
	}
}

// Keys go as their bytes, one write each; text is typed as is, then its keys; an unknown key is refused before
// anything is pressed.
func TestOrcaKeysAndText(t *testing.T) {
	n, o, _ := orcaSession(t)
	if _, err := request(t, n, "session.keys", map[string]any{"session_id": sidA, "keys": []string{"esc", "shift+tab", "ctrl+u", "up"}}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, o.Trace(), []string{
		orcarttest.SendCall("term_1", map[string]any{"text": "\x1b"}),
		orcarttest.SendCall("term_1", map[string]any{"text": "\x1b[Z"}),
		orcarttest.SendCall("term_1", map[string]any{"text": "\x15"}),
		orcarttest.SendCall("term_1", map[string]any{"text": "\x1b[A"}),
	})
	o.Reset()
	if _, err := request(t, n, "session.keys", map[string]any{"session_id": sidA, "text": "yes please", "keys": []string{"enter"}}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, o.Trace(), []string{
		`terminal.agentStatus {"terminal":"term_1"}`,
		orcarttest.SendCall("term_1", map[string]any{"text": "yes please"}),
		orcarttest.SendCall("term_1", map[string]any{"text": "\r"}),
	})
	o.Reset()
	if _, err := request(t, n, "session.keys", map[string]any{"session_id": sidA, "keys": []string{"enter", "hyper+q"}}); err == nil {
		t.Error("an unknown key was taken")
	}
	if tr := o.Trace(); len(tr) != 0 {
		t.Errorf("pressed before refusing: %v", tr)
	}
	for k, want := range map[string]string{"enter": "\r", "backspace": "\x7f", "ctrl+c": "\x03", "tab": "\t", "1": "1",
		"y": "y", "down": "\x1b[B", "space": " "} {
		if got, ok := orcaKeyBytes(k); !ok || got != want {
			t.Errorf("%s = %q", k, got)
		}
	}
}

// A colored screen read gets the plain rows (CRLF-ended like colored ones); "recent" reads the screen too.
func TestOrcaScreenIsPlain(t *testing.T) {
	n, o, _ := orcaSession(t)
	o.SetScreen("term_1", "line one\nline two\n")
	res, err := request(t, n, "session.screen", map[string]any{"session_id": sidA, "format": "ansi"})
	if err != nil {
		t.Fatal(err)
	}
	if res["text"] != "line one\r\nline two\r\n" || res["status"] != proto.StatusDone {
		t.Errorf("screen = %q", res)
	}
	if _, err := request(t, n, "session.screen", map[string]any{"session_id": sidA, "source": "recent", "lines": 500}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, o.Trace(), []string{
		`terminal.read {"screen":true,"terminal":"term_1"}`,
		`terminal.read {"screen":true,"terminal":"term_1"}`,
	})
	if _, err := request(t, n, "terminal.watch", map[string]any{"session_id": sidA}); err == nil ||
		!strings.HasPrefix(err.Error(), "unsupported") {
		t.Errorf("terminal.watch = %v", err)
	}
}

// A handle Orca no longer knows (the terminal came back under a new one) is looked up again by the pane key.
func TestOrcaStaleHandle(t *testing.T) {
	n, o, _ := orcaSession(t)
	o.SetTerminals(orcaTerm("term_1b", "tab1", "leaf1", "claude"))
	o.SetScreen("term_1b", "hi\n")
	res, err := request(t, n, "session.screen", map[string]any{"session_id": sidA})
	if err != nil || res["text"] != "hi\n" {
		t.Fatalf("screen = %v, %v", res, err)
	}
	assertTrace(t, o.Trace("terminal.read"), []string{
		`terminal.read {"screen":true,"terminal":"term_1"}`,
		`terminal.read {"screen":true,"terminal":"term_1b"}`,
	})
}

// session.close: ctrl+c twice, then terminal.close (Orca closes the pane, its tab when it was the only one). A
// terminal already gone counts as closed.
func TestOrcaClose(t *testing.T) {
	n, o, _ := orcaSession(t)
	if _, err := request(t, n, "session.close", map[string]any{"session_id": sidA}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, o.Trace(), []string{
		orcarttest.SendCall("term_1", map[string]any{"text": "\x03"}),
		orcarttest.SendCall("term_1", map[string]any{"text": "\x03"}),
		`terminal.close {"terminal":"term_1"}`,
	})

	n, o, _ = orcaSession(t)
	o.SetTerminals() // gone before the close
	if _, err := request(t, n, "session.close", map[string]any{"session_id": sidA}); err != nil {
		t.Errorf("close of a gone terminal: %v", err)
	}
	if tr := o.Trace("terminal.close"); len(tr) != 0 {
		t.Errorf("closed a terminal that is not there: %v", tr)
	}
}

// Container and Agent renames use different runtime operations and exact scoped identities.
func TestOrcaRenameTargetsTheWorktreeOrAgent(t *testing.T) {
	n, o, _ := orcaSession(t)
	if _, err := request(t, n, "worktree.rename", map[string]any{"kind": "orca", "workspace_id": "repo::" + orcaCwd, "name": "  Backend work  "}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, o.Trace("worktree.set"), []string{`worktree.set {"displayName":"Backend work","worktree":"id:repo::` + orcaCwd + `"}`})
	if len(o.Trace("terminal.rename", "terminal.send")) != 0 {
		t.Fatal("renaming a worktree touched its Agent")
	}
	o.Reset()
	if _, err := request(t, n, "session.rename", map[string]any{"session_id": sidA, "tab_name": "Agent review"}); err != nil {
		t.Fatal(err)
	}
	assertTrace(t, o.Trace("terminal.rename"), []string{`terminal.rename {"terminal":"term_1","title":"Agent review"}`})
	if len(o.Trace("worktree.set")) != 0 {
		t.Fatal("renaming an Agent touched its worktree")
	}
	o.Reset()
	for _, body := range []map[string]any{
		{"kind": "orca", "workspace_id": "missing", "name": "Other"},
		{"kind": "orca", "workspace_id": "repo::" + orcaCwd, "name": " "},
		{"kind": "orca", "workspace_id": "repo::" + orcaCwd, "name": strings.Repeat("x", maxLabel+1)},
	} {
		if _, err := request(t, n, "worktree.rename", body); err == nil {
			t.Fatal("invalid worktree rename accepted")
		}
	}
	o.SetTerminals()
	if _, err := request(t, n, "session.rename", map[string]any{"session_id": sidA, "tab_name": "Gone"}); err == nil {
		t.Fatal("renamed a missing terminal")
	}
	if len(o.Trace("worktree.set", "terminal.rename", "terminal.send")) != 0 {
		t.Fatal("a refused rename reached another target")
	}
}

// Orca not running (no metadata, or its socket gone): no sessions at once, nothing logged, the refresh succeeds.
func TestOrcaAbsentOrGone(t *testing.T) {
	n, _, o, procs, home := orcaNode(t)
	writeClaudePID(t, home, 4242, sidB)
	writeTranscript(t, home, orcaCwd, sidB, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	o.SetTerminals(orcaTerm("term_1", "tab1", "leaf1", "claude"))
	procs.set("tab1:leaf1", orcaProc{pid: 4242, agent: "claude"})
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := n.agent(sidB); !ok {
		t.Fatal("Orca's session not found")
	}
	o.Stop()
	time.Sleep(time.Second) // past the failed list kept for Agents
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := n.agent(sidB); ok {
		t.Error("Orca quit, its session is still live")
	}

	var logged []string
	var mu sync.Mutex
	c := &orcart.Client{MetaPath: "/tmp/persona-no-such-dir/orca-runtime.json", Log: func(format string, args ...any) {
		mu.Lock()
		logged = append(logged, fmt.Sprintf(format, args...))
		mu.Unlock()
	}}
	h := newOrcaHost(c)
	h.scan = procs.scan
	n.hosts.Set(h)
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

// Orca pushes nothing on its socket: with Orca as the engine no change stream starts, and the poll takes a snapshot
// every second (not herdr's slow snapshot, which relies on its events).
func TestOrcaPollsEverySecond(t *testing.T) {
	n, _, o, _, _ := orcaNode(t)
	_, stop := pollNode(t, n)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n.watchEngine(ctx)
	time.Sleep(3200 * time.Millisecond)
	stop()
	if got := o.Count("terminal.list"); got < 3 {
		t.Errorf("%d snapshots in 3 s", got)
	}
	if n.events.isLive() {
		t.Error("a change stream counts as live for Orca")
	}
}

// node.info finds Orca when its runtime answers with a protocol Persona speaks; an incompatible one is there, not
// available, with a note.
func TestOrcaEngineDetection(t *testing.T) {
	o := orcarttest.New(t)
	e := orcaEngine(context.Background(), &orcart.Client{MetaPath: o.MetaPath})
	if e != (proto.Engine{ID: "orca", Name: "Orca", Available: true, Version: "1.4.245"}) {
		t.Errorf("engine %+v", e)
	}
	o.SetStatus(map[string]any{"runtimeProtocolVersion": 2})
	e = orcaEngine(context.Background(), &orcart.Client{MetaPath: o.MetaPath})
	if e.Available || e.Note == "" || e.Version != "1.4.245" {
		t.Errorf("incompatible: %+v", e)
	}
	e = orcaEngine(context.Background(), &orcart.Client{MetaPath: "/tmp/persona-no-such-dir/orca-runtime.json"})
	if e != (proto.Engine{ID: "orca", Name: "Orca"}) {
		t.Errorf("absent: %+v", e)
	}
	if hs := engineHosts(proto.EngineOrca, nil); len(hs) != 1 || hs[0].Kind() != host.Orca {
		t.Errorf("orca hosts %v", hs)
	}
}

// engine.use orca swaps the hosts for Orca's and lists its sessions; back to herdr drops them.
func TestOrcaEngineUse(t *testing.T) {
	n, _, home := herdrNode(t)
	o := orcarttest.New(t)
	t.Setenv("ORCA_USER_DATA_PATH", filepath.Dir(o.MetaPath))
	writeClaudePID(t, home, 4242, sidB)
	writeTranscript(t, home, orcaCwd, sidB, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	o.SetTerminals(orcaTerm("term_1", "tab1", "leaf1", "claude"))
	o.SetWorktrees(orcarttest.Worktree("repo::"+orcaCwd, orcaCwd, "", orcarttest.Agent("tab1:leaf1", "claude", "working")))
	procs := &fakeProcs{}
	procs.set("tab1:leaf1", orcaProc{pid: 4242, agent: "claude"})
	old := orcaScan
	orcaScan = procs.scan
	t.Cleanup(func() { orcaScan = old })
	if _, err := request(t, n, "engine.use", map[string]any{"engine": "orca"}); err != nil {
		t.Fatal(err)
	}
	h, ok := n.hosts.Get(host.Orca).(*orcaHost)
	if !ok || len(n.hosts.All()) != 2 || n.hosts.All()[1].Kind() != host.Tmux {
		t.Fatalf("hosts %v", hostKinds(n))
	}
	if m := byID(listSessions(t, n)); m[sidB].PaneID != "orca:tab1:leaf1" || m[sidB].Status != proto.StatusWorking ||
		m[sidB].SpaceName != "orcaproj" {
		t.Errorf("sessions %v", m)
	}
	if n.defaultHost() != host.Host(h) {
		t.Error("Orca is not the default host")
	}
	if _, err := request(t, n, "engine.use", map[string]any{"engine": "herdr"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := n.agent(sidB); ok {
		t.Error("Orca's session kept after switching back")
	}
}

// The process scan keeps the claude / codex processes in front (not suspended, not in the background), only the pane
// key out of their environment (never the hook token), skips agents inside a herdr / herdr-core nested in an Orca
// terminal, finds claude and codex under node too, falls back on ORCA_AGENT_PANE, and lists claude first.
func TestOrcaProcessParsing(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	args := strings.Join([]string{
		"  101 S+   01:00 /usr/local/bin/claude --resume x",
		"  102 S+   1-02:03:04 node /opt/lib/node_modules/@anthropic-ai/claude-code/cli.js",
		"  103 R+   00:05 /opt/bin/codex",
		"  104 Ss+  00:05 -zsh",
		"  105 T+   00:09 /usr/local/bin/claude", // suspended (ctrl+z)
		"  106 S    00:09 /usr/local/bin/claude", // in the background (claude -p … &)
		"  107 S+   00:09 /usr/local/bin/claude", // inside a herdr nested in the Orca terminal
		"  108 S+   00:09 /usr/local/bin/claude", // inside herdr-core
		"  100 S+   00:30 node /opt/lib/node_modules/@openai/codex/bin/codex.js",
		"garbage",
	}, "\n")
	cands := parseOrcaPSArgs(args, now)
	if _, ok := cands[105]; ok {
		t.Error("a suspended claude counts")
	}
	if _, ok := cands[106]; ok {
		t.Error("a background claude counts")
	}
	if p := cands[102]; p.start != now.Add(-(26*time.Hour + 3*time.Minute + 4*time.Second)) {
		t.Errorf("start %v", p.start)
	}
	env := strings.Join([]string{
		"  101 /usr/local/bin/claude --resume x TERM=xterm ORCA_AGENT_HOOK_TOKEN=secret ORCA_PANE_KEY=tab1:leaf1 HOME=/h",
		"  102 node /opt/lib/node_modules/@anthropic-ai/claude-code/cli.js ORCA_AGENT_PANE=tab2:leaf2",
		"  103 /opt/bin/codex ORCA_PANE_KEY=tab1:leaf1",
		"  107 /usr/local/bin/claude ORCA_PANE_KEY=tab5:leaf5 HERDR_PANE_ID=w1:p1",
		"  108 /usr/local/bin/claude ORCA_PANE_KEY=tab5:leaf5 HC_TERMINAL_ID=t3",
		"  100 node /opt/lib/node_modules/@openai/codex/bin/codex.js ORCA_PANE_KEY=tab4:leaf4",
		"  999 /usr/local/bin/claude ORCA_PANE_KEY=tab9:leaf9", // not a candidate
	}, "\n")
	got := parseOrcaPSEnv(env, cands, nil) // the pty owners: TestOrcaNestedPtysAreNotThePanes
	pids := map[string][]int{}
	for pane, ps := range got {
		for _, p := range ps {
			pids[pane] = append(pids[pane], p.pid)
		}
	}
	want := map[string][]int{"tab1:leaf1": {101, 103}, "tab2:leaf2": {102}, "tab4:leaf4": {100}}
	if fmt.Sprint(pids) != fmt.Sprint(want) {
		t.Errorf("parsed %v\nwant %v", pids, want)
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "secret") {
		t.Error("the hook token was kept")
	}
	for stat, front := range map[string]bool{
		"42 (claude) S 1 42 42 34816 42 4194304":      true,
		"42 (claude code) S 1 42 42 34816 77 4194304": false, // background: another group has the terminal
		"42 (claude) T 1 42 42 34816 42 4194304":      false, // stopped
	} {
		if procInFront(stat) != front {
			t.Errorf("procInFront(%q) = %v", stat, !front)
		}
	}
	for s, want := range map[string]time.Duration{"00:05": 5 * time.Second, "01:02:03": time.Hour + 2*time.Minute + 3*time.Second,
		"2-00:00:01": 48*time.Hour + time.Second} {
		if d, ok := parseEtime(s); !ok || d != want {
			t.Errorf("parseEtime(%q) = %v", s, d)
		}
	}
}

func TestHookPaneOrca(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	for _, c := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"ORCA_PANE_KEY": "tab1:leaf1"}, "orca:tab1:leaf1"},
		{map[string]string{"ORCA_AGENT_PANE": "tab1:leaf1"}, "orca:tab1:leaf1"},
		{map[string]string{"HERDR_PANE_ID": "w1:p2", "ORCA_PANE_KEY": "tab1:leaf1"}, "w1:p2"},
		{map[string]string{"HC_TERMINAL_ID": "t7", "ORCA_PANE_KEY": "tab1:leaf1"}, "herdr-core:t7"},
	} {
		if got := nodeclient.HookPane(env(c.env)); got != c.want {
			t.Errorf("%v: HookPane = %q, want %q", c.env, got, c.want)
		}
	}
	if r := host.ParsePane("orca:tab1:leaf1"); r != (host.PaneRef{Kind: host.Orca, ID: "tab1:leaf1"}) {
		t.Errorf("ParsePane = %+v", r)
	}
}

// Text goes in only when an agent is in front: Orca writes anything else raw into whatever is there (a shell would run
// it). Not in front — our scan finds none (suspended, background, nested: TestOrcaProcessParsing), or the scan failed
// — is refused before anything is typed, for prompts, typed text and keys that type; enter / esc / ctrl+c still go
// (a stuck pane can be interrupted and closed). Orca's own "no agent" is not a refusal (TestOrcaSendsWhileClaudeHas
// BackgroundTasks).
func TestOrcaRefusesTextWithoutAnAgentInFront(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(o *orcarttest.Fake, p *fakeProcs)
	}{
		{"no agent process in front", func(_ *orcarttest.Fake, p *fakeProcs) { p.set("tab1:leaf1") }},
		{"the process scan failed", func(_ *orcarttest.Fake, p *fakeProcs) {
			p.mu.Lock()
			p.err = errors.New("ps: exit status 1")
			p.mu.Unlock()
		}},
	} {
		n, o, procs := orcaSession(t)
		o.SetScreen("term_1", "❯ \n")
		c.setup(o, procs)
		for _, req := range []map[string]any{
			{"session_id": sidA, "text": "rm -rf build"},
		} {
			_, err := request(t, n, "session.send", req)
			if code, data := refusalOf(t, err); code != "not_delivered" || data["delivered"] != "none" {
				t.Errorf("%s: send: %v", c.name, err)
			}
		}
		for _, req := range []map[string]any{
			{"session_id": sidA, "text": "yes", "keys": []string{"enter"}},
			{"session_id": sidA, "keys": []string{"y"}},
		} {
			if _, err := request(t, n, "session.keys", req); err == nil {
				t.Errorf("%s: keys %v went in", c.name, req)
			}
		}
		if w := o.Writes(); len(w) != 0 {
			t.Errorf("%s: wrote %+v", c.name, w)
		}
		if _, err := request(t, n, "session.keys", map[string]any{"session_id": sidA, "keys": []string{"ctrl+c"}}); err != nil {
			t.Errorf("%s: ctrl+c: %v", c.name, err)
		}
		if w := o.Writes(); len(w) != 1 || w[0].Text != "\x03" {
			t.Errorf("%s: ctrl+c writes %+v", c.name, w)
		}
	}
}

// A permission dialog in front (Orca's agentStatus): a prompt would answer it — refused as dialog_open; the dialog's
// own answer keys go through.
func TestOrcaPermissionDialog(t *testing.T) {
	n, o, _ := orcaSession(t)
	o.SetScreen("term_1", "❯ \n") // our screen check sees nothing: Orca knows better
	o.SetAgentStatus("term_1", true, "permission")
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
	if code, data := refusalOf(t, err); code != "dialog_open" || data["delivered"] != "none" {
		t.Fatalf("prompt into a permission dialog: %v", err)
	}
	if _, err := request(t, n, "session.keys", map[string]any{"session_id": sidA, "keys": []string{"1"}}); err != nil {
		t.Fatalf("answer key: %v", err)
	}
	if w := o.Writes(); len(w) != 1 || w[0].Text != "1" {
		t.Errorf("writes %+v", w)
	}
}

// The terminal behind the pane gets another process between the check that an agent is in front and the write (a new
// incarnation): the write, fenced to the process that was checked, is refused, nothing typed — it is not retried
// against the new one.
func TestOrcaNewIncarnationRefusesWrites(t *testing.T) {
	n, o, _ := orcaSession(t)
	o.SetScreen("term_1", "❯ \n")
	o.Handle("terminal.agentStatus", func(map[string]any) any {
		t2 := orcaTerm("term_1", "tab1", "leaf1", "claude")
		t2["incarnationId"] = "inc-new"
		o.SetTerminals(t2)
		return map[string]any{"agentStatus": map[string]any{"handle": "term_1", "isRunningAgent": true, "status": nil}}
	})
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
	if code, _ := refusalOf(t, err); code != "not_delivered" {
		t.Fatalf("send to a replaced process: %v", err)
	}
	if w := o.Writes(); len(w) != 0 {
		t.Errorf("wrote %+v", w)
	}
	// A read follows the pane to its new process.
	o.SetScreen("term_1", "new\n")
	if res, err := request(t, n, "session.screen", map[string]any{"session_id": sidA}); err != nil || res["text"] != "new\n" {
		t.Errorf("read after the change: %v %v", res, err)
	}
}

// A multi-line message goes in whole: one bracketed paste (its newlines stay in the input instead of each
// submitting, which Orca's raw path for a fresh agent would do), an ESC inside it made inert, then one carriage return.
func TestOrcaMultiLinePrompt(t *testing.T) {
	n, o, _ := orcaSession(t)
	o.SetScreen("term_1", "❯ \n")
	text := "first line\nsecond \x1b[201~ line\nthird"
	if _, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": text}); err != nil {
		t.Fatal(err)
	}
	w := o.Writes()
	if len(w) != 2 || w[0].Text != "\x1b[200~first line\nsecond <ESC>[201~ line\nthird\x1b[201~" || w[0].Enter || w[1].Text != "\r" {
		t.Errorf("writes %+v", w)
	}
	if strings.Count(w[0].Text, "\x1b") != 2 {
		t.Errorf("an ESC from the text reached the terminal: %q", w[0].Text)
	}
	if inv := o.Invalid(); len(inv) != 0 {
		t.Errorf("params Orca would refuse: %v", inv)
	}
}

// Once the paste went in, a failure (the submit refused, the agent no longer in front) leaves the text possibly in
// the input: delivered unknown, not a clean refusal.
func TestOrcaPromptSubmitFails(t *testing.T) {
	for _, name := range []string{"submit refused", "agent gone before the submit"} {
		n, o, procs := orcaSession(t)
		o.SetScreen("term_1", "❯ \n")
		o.Handle("terminal.send", func(p map[string]any) any {
			if p["text"] == "\r" {
				return orcarttest.Fail{Code: "terminal_not_writable"}
			}
			if name == "agent gone before the submit" {
				o.SetAgentStatus("term_1", false, "")
				procs.set("tab1:leaf1")
			}
			return map[string]any{"send": map[string]any{"handle": "term_1", "accepted": true, "bytesWritten": 10}}
		})
		_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
		var re *rpc.Error
		if !errors.As(err, &re) || !maybeDelivered(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A session that is not running that Orca does not resume (no Claude transcript) is refused, nothing typed, with a
// reason: its claude not in front when its pane is still there (suspended), else not running — not "unsupported".
// A message to a stopped agent is refused (#4754); the user's Resume (session.start) brings back a Claude session
// whose pane is gone on Orca (#5548), and on Herdr App as herdr does (#2532).
func TestNotRunningOnAnEngineWithoutResume(t *testing.T) {
	n, o, procs := orcaSession(t)
	n.claudeStart, n.claudeReady = 200*time.Millisecond, 200*time.Millisecond
	procs.set("tab1:leaf1") // suspended: no claude in front
	time.Sleep(orcaProcsMaxAge)
	listSessions(t, n)
	_, err := request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
	if code, data := refusalOf(t, err); code != "agent_not_in_front" || data["delivered"] != "none" {
		t.Fatalf("suspended: %v", err)
	}
	if !strings.Contains(err.Error(), "不在前台") {
		t.Errorf("message %q", err)
	}
	if w := o.Writes(); len(w) != 0 {
		t.Errorf("wrote %+v", w)
	}
	o.SetTerminals() // the terminal closed: a message is refused, Resume brings it back in a new one with its launcher
	n.state.mu.Lock()
	st := n.state.Sessions[sidA]
	st.Launcher = "claude-gpt"
	n.state.Sessions[sidA] = st
	n.state.mu.Unlock()
	_, err = request(t, n, "session.send", map[string]any{"session_id": sidA, "text": "hello"})
	if code, _ := refusalOf(t, err); code != agentStopped {
		t.Fatalf("a message to a stopped agent: %v", err)
	}
	starts := orcaTypedStarts(o)
	_, _ = request(t, n, "session.start", map[string]any{"resume_session_id": sidA})
	if s := strings.Join(starts(), "\n"); !strings.Contains(s, "--resume "+sidA) || !strings.Contains(s, "claude-gpt") {
		t.Fatalf("gone: not resumed with its launcher; calls %v", o.Trace())
	}
	const unknown = "0e000000-0000-4000-8000-0000000000ff" // no Claude transcript: not Orca's to resume
	_, err = request(t, n, "session.send", map[string]any{"session_id": unknown, "text": "hello"})
	if code, data := refusalOf(t, err); code != "not_running" || data["delivered"] != "none" || !strings.Contains(err.Error(), "Orca") {
		t.Fatalf("no transcript: %v", err)
	}
	const cx = "0e000000-0000-4000-8000-0000000000fe" // a Codex Persona started, exited: its rollout is no Claude transcript
	writeRollout(t, n.home, cx, orcaCwd, time.Now(), time.Now().Add(-time.Minute))
	n.state.mu.Lock()
	n.state.Sessions[cx] = sessionState{Launcher: "codex", Cwd: orcaCwd}
	n.state.mu.Unlock()
	o.Reset()
	_, err = request(t, n, "session.send", map[string]any{"session_id": cx, "text": "hello"})
	if code, _ := refusalOf(t, err); code != agentStopped {
		t.Fatalf("codex: %v", err)
	}
	if c := o.Trace("terminal.create", "terminal.send"); len(c) != 0 {
		t.Errorf("codex resumed in Orca: %v", c)
	}

	// Herdr App resumes it as herdr does (#2532): a new tab, its launcher with --resume through the start file.
	hn, hc := hcSession(t)
	hn.claudeStart, hn.claudeReady = 200*time.Millisecond, 200*time.Millisecond // the fake shell never runs claude
	hc.doc = appDoc(hcCwd)
	var typed string
	hc.handle("terminal.send", func(p map[string]any) any {
		typed, _ = startFileCommand(str(p["text"]))
		return map[string]any{}
	})
	hc.setTerminals()
	time.Sleep(time.Second) // past the failed-list cache
	listSessions(t, hn)
	_, err = request(t, hn, "session.start", map[string]any{"resume_session_id": sidA})
	if isRefusal(err, "not_running") {
		t.Fatalf("herdr-core: %v", err)
	}
	if r := hc.trace("terminal.run"); len(r) != 1 || !strings.Contains(typed, "--resume "+sidA) {
		t.Fatalf("herdr-core: not resumed in a new tab: runs %v, typed %q (%v)", r, typed, err)
	}
}

// States: a pane without a worktree.ps row takes Orca's agentStatus (working / permission → blocked / idle → done);
// a row older than the pane's agent process is the previous agent's and does not count.
func TestOrcaStatusFallbackAndStaleRows(t *testing.T) {
	n, _, o, procs, home := orcaNode(t)
	writeTranscript(t, home, orcaCwd, sidA, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	writeTranscript(t, home, orcaCwd, sidB, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	writeClaudePID(t, home, 4242, sidA)
	writeClaudePID(t, home, 4343, sidB)
	o.SetTerminals(orcaTerm("term_1", "tab1", "leaf1", "claude"), orcaTerm("term_2", "tab2", "leaf2", "claude"))
	old := orcarttest.Agent("tab2:leaf2", "claude", "working")
	old["updatedAt"] = time.Now().Add(-time.Hour).UnixMilli()
	o.SetWorktrees(orcarttest.Worktree("repo::"+orcaCwd, orcaCwd, "", old))
	o.SetAgentStatus("term_1", true, "permission")
	o.SetAgentStatus("term_2", true, "idle")
	procs.set("tab1:leaf1", orcaProc{pid: 4242, agent: "claude", start: time.Now().Add(-time.Minute)})
	procs.set("tab2:leaf2", orcaProc{pid: 4343, agent: "claude", start: time.Now().Add(-time.Minute)})
	m := byID(listSessions(t, n))
	if m[sidA].Status != proto.StatusBlocked || m[sidB].Status != proto.StatusDone {
		t.Errorf("states %s %s", m[sidA].Status, m[sidB].Status)
	}
	// The fallback is cached: the next poll does not ask again.
	o.Reset()
	listSessions(t, n)
	if c := o.Count("terminal.agentStatus"); c != 0 {
		t.Errorf("asked again %d times", c)
	}
}

// Polling is light: the layouts (costly for Orca) come only when the terminals or a topology changed; worktree.ps
// answers "unchanged" to its snapshot; Agents after a Snapshot reuses its look.
func TestOrcaPollingCost(t *testing.T) {
	n, o, _ := orcaSession(t)
	h := n.hosts.Get(host.Orca).(*orcaHost)
	for range 3 {
		listSessions(t, n)
	}
	layouts := func() int {
		c := 0
		for _, l := range o.Trace("terminal.list") {
			if strings.Contains(l, `"includeVisualLayouts":true`) {
				c++
			}
		}
		return c
	}
	if c := layouts(); c != 0 {
		t.Errorf("%d layout reads without a change", c)
	}
	if tr := o.Trace("worktree.ps"); len(tr) != 3 || strings.Contains(tr[1], "null") {
		t.Errorf("worktree.ps %v", tr)
	}
	o.SetRevision("repo::"+orcaCwd, 2)
	listSessions(t, n)
	if c := layouts(); c != 1 {
		t.Errorf("%d layout reads after a topology change", c)
	}
	o.Reset()
	if _, err := h.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Agents(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c := o.Count("terminal.list"); c != 1 {
		t.Errorf("Snapshot then Agents listed %d times", c)
	}
}

// Keys become xterm bytes, modifiers included; keys that only move or interrupt do not count as typing.
func TestOrcaKeyBytes(t *testing.T) {
	for k, want := range map[string]string{
		"enter": "\r", "backspace": "\x7f", "ctrl+c": "\x03", "tab": "\t", "1": "1", "y": "y", "down": "\x1b[B",
		"space": " ", "ctrl+space": "\x00", "alt+b": "\x1bb", "alt+left": "\x1b[1;3D", "ctrl+left": "\x1b[1;5D",
		"shift+right": "\x1b[1;2C", "ctrl+up": "\x1b[1;5A", "f1": "\x1bOP", "f5": "\x1b[15~", "f12": "\x1b[24~",
		"shift+f1": "\x1b[1;2P", "delete": "\x1b[3~", "insert": "\x1b[2~", "pageup": "\x1b[5~", "home": "\x1b[H",
		"alt+backspace": "\x1b\x7f", "alt+space": "\x1b ", "alt+tab": "\x1b\t", "shift+a": "A", "alt+2": "\x1b2",
		"shift+enter": "\x1b\r", "ctrl+enter": "\x1b[13;5u", "ctrl+a": "\x01", "esc": "\x1b",
	} {
		if got, ok := orcaKeyBytes(k); !ok || got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	for _, k := range []string{"hyper+q", "f0", "ctrl+tab", ""} {
		if b, ok := orcaKeyBytes(k); ok {
			t.Errorf("%q has bytes %q", k, b)
		}
	}
	for b, typing := range map[string]bool{"\r": false, "\x1b": false, "\x1b[1;3D": false, "\x1bOP": false, "\x03": false,
		"y": true, " ": true, "\x1bb": true, "\x1b[15~": false} {
		if orcaTypes(b) != typing {
			t.Errorf("orcaTypes(%q) = %v", b, !typing)
		}
	}
}

// Pressing keys stops between keys when the caller gives up.
func TestOrcaKeysHonourContext(t *testing.T) {
	_, o, _ := orcaSession(t)
	h := newOrcaHost(&orcart.Client{MetaPath: o.MetaPath})
	if _, _, err := h.terminals(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := h.SendKeys(ctx, host.PaneRef{Kind: host.Orca, ID: "tab1:leaf1"}, "esc", "esc", "esc")
	if err == nil || len(o.Writes()) > 1 {
		t.Errorf("keys after cancel: %v, %d writes", err, len(o.Writes()))
	}
}

// A hook reporting an Orca pane is bound only on a machine whose engine is Orca; a herdr machine stays as it was.
func TestOrcaHookPaneBinding(t *testing.T) {
	n, _, _ := herdrNode(t)
	_, _ = hubPipe(t, n), 0
	in, _ := json.Marshal(map[string]any{"session_id": sidA, "hook_event_name": "Notification"})
	_, _ = n.handleHook(context.Background(), sockRequest{Kind: "hook", Input: in, Pane: "orca:tab1:leaf1"})
	n.mu.Lock()
	_, bound := n.panes["orca:tab1:leaf1"]
	n.mu.Unlock()
	if bound {
		t.Error("an Orca pane bound on a herdr machine")
	}
	n.eng.name = proto.EngineOrca
	_, _ = n.handleHook(context.Background(), sockRequest{Kind: "hook", Input: in, Pane: "orca:tab1:leaf1"})
	n.mu.Lock()
	_, bound = n.panes["orca:tab1:leaf1"]
	n.mu.Unlock()
	if !bound {
		t.Error("an Orca pane not bound on an Orca machine")
	}
}

// A child worktree (Orca's "N children") carries its parent's id as its space's parent, on the space and on the sessions
// in it; a top-level one, and one that names itself, has none (#2073).
func TestOrcaChildWorktreeNamesItsParent(t *testing.T) {
	n, _, o, procs, home := orcaNode(t)
	writeTranscript(t, home, orcaCwd, sidA, time.Now().Add(-time.Minute), `{"type":"custom-title","customTitle":"x"}`)
	writeClaudePID(t, home, 4242, sidA)
	o.SetTerminals(orcaTerm("term_1", "tab1", "leaf1", "claude"))
	parent := orcarttest.Worktree("repo::/code/main", "/code/main", "refs/heads/main")
	child := orcarttest.Worktree("repo::"+orcaCwd, orcaCwd, "refs/heads/feature-x", orcarttest.Agent("tab1:leaf1", "claude", "working"))
	child["parentWorktreeId"] = "repo::/code/main"
	self := orcarttest.Worktree("repo::/code/self", "/code/self", "refs/heads/self")
	self["parentWorktreeId"] = "repo::/code/self"
	o.SetWorktrees(parent, child, self)
	procs.set("tab1:leaf1", orcaProc{pid: 4242, agent: "claude"})

	if s := byID(listSessions(t, n))[sidA]; s.SpaceID != "repo::"+orcaCwd || s.ParentSpaceID != "repo::/code/main" {
		t.Errorf("child session: space %q parent %q", s.SpaceID, s.ParentSpaceID)
	}
	h := n.hosts.Get(host.Orca)
	snap, err := h.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := h.(interface {
		Spaces(context.Context) ([]host.Space, error)
	}).Spaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, spaces := range map[string][]host.Space{"snapshot": snap.Spaces, "catalog": catalog} {
		got := map[string]string{}
		for _, sp := range spaces {
			got[sp.ID] = sp.ParentID
		}
		want := map[string]string{"repo::/code/main": "", "repo::" + orcaCwd: "repo::/code/main", "repo::/code/self": ""}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s parents %v, want %v", name, got, want)
		}
	}
}

// #3828: a row blocked by a subagent waiting on the user is blocked, though its main agent works on (it waits for the
// subagent) or has settled: the pane needs the user. A row working only because of its background subagents is not.
func TestOrcaSubagentWaitingNeedsYou(t *testing.T) {
	term := orcart.Terminal{Handle: "h", TabID: "t", LeafID: "a", AgentIdentity: "claude"}
	for _, c := range []struct{ row, main, want string }{
		{"blocked", "working", proto.StatusBlocked},
		{"waiting", "done", proto.StatusBlocked},
		{"working", "done", proto.StatusDone},
		{"blocked", "", proto.StatusBlocked},
	} {
		row := orcart.AgentRow{AgentType: "claude", State: c.row}
		if c.main != "" {
			row.MainAgent = &struct {
				State string `json:"state"`
			}{State: c.main}
		}
		v := &orcaView{procsKnown: true, procs: map[string][]orcaProc{"t:a": {{pid: 1, agent: "claude"}}},
			rows: map[string]orcart.AgentRow{"t:a": row}}
		if a := v.agent(term); a.AgentStatus != c.want {
			t.Errorf("row %s, main %s: %s, want %s", c.row, c.main, a.AgentStatus, c.want)
		}
		if got := orcaChatStatus(row); got != c.want {
			t.Errorf("chat: row %s, main %s: %s, want %s", c.row, c.main, got, c.want)
		}
	}
}
