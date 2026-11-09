package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/assistantwire"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// getReadiness is GET /api/v1/assistant/readiness (query: its query string) as the hub relays it, in lang.
func getReadiness(t *testing.T, s *Service, query, lang string) Readiness {
	t.Helper()
	header := map[string][]string{}
	if lang != "" {
		header["Accept-Language"] = []string{lang}
	}
	res, err := serveHTTP(context.Background(), s, assistantwire.HTTPRequest{Method: "GET", Path: "/api/v1/assistant/readiness",
		Query: query, Header: header}, s.lang)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 {
		t.Fatalf("readiness: %d %s", res.Status, res.Body)
	}
	var out Readiness
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("readiness: %s", res.Body)
	}
	return out
}

func readinessStep(t *testing.T, r Readiness, id string) ReadinessStep {
	t.Helper()
	for _, st := range r.Steps {
		if st.ID == id {
			return st
		}
	}
	t.Fatalf("no step %s in %+v", id, r.Steps)
	return ReadinessStep{}
}

func nextOf(r Readiness) string {
	if r.Next == nil {
		return ""
	}
	return *r.Next
}

// readyMachine is a hub with the assistant's machine "server" on Claude Code 2.1.308, its main conversation m1.
func readyMachine(t *testing.T, main ...proto.Session) (*fakeHub, *Service, *memStore) {
	t.Helper()
	f := newFakeHub().withMachine()
	f.doctor = map[string]string{"server": "2.1.308，支持 --plugin-dir"}
	f.settings = &Settings{Main: "m1"}
	if len(main) > 0 {
		f.setSessions("server", main...)
	}
	st := &memStore{}
	return f, newServiceOn(t, f, st), st
}

// With no computer the first missing step is the computer, said in plain words with what to do; what comes after it
// cannot be told, and the suggestions are the local ones only (docs/assistant/onboarding.md 5.1).
func TestReadinessWithoutComputer(t *testing.T) {
	s := newStorelessService(t, newFakeHub())
	r := getReadiness(t, s, "", "")
	c := readinessStep(t, r, StepComputer)
	if r.Version != 1 || r.Mode != ReadinessSetup || r.Ready || nextOf(r) != StepComputer || r.Machine != nil || r.Launcher != "claude" ||
		c.State != StepTodo || c.Code != proto.CodeNoMachine || c.Action == nil || c.Action.ID != "add_computer" ||
		c.Title != "添加你的电脑" || c.Detail != "Persona 要在你的一台电脑上干活，现在还没接上电脑。" {
		t.Fatalf("readiness = %+v", r)
	}
	if len(r.Steps) != 4 || r.Steps[1].ID != StepClaude || r.Steps[2].ID != StepModel || r.Steps[3].ID != StepFirstMessage {
		t.Fatalf("steps = %+v", r.Steps)
	}
	for _, st := range r.Steps[1:] {
		if st.State != StepUnknown {
			t.Fatalf("after the computer: %+v", st)
		}
	}
	for _, sg := range r.Suggestions {
		if !sg.Local {
			t.Fatalf("setup suggests %+v", sg)
		}
	}
	if len(r.Suggestions) != 2 || r.Probe != nil || r.Dismissed || r.CheckedAt.IsZero() {
		t.Fatalf("readiness = %+v", r)
	}
	// In English when the request says so.
	if c := readinessStep(t, getReadiness(t, s, "", "en"), StepComputer); c.Title != "Add your computer" {
		t.Fatalf("en: %+v", c)
	}
	// warm carries the same.
	status, body := serveJSON(t, s, "POST", "/api/v1/assistant/warm", "", `{}`)
	rd, _ := body["readiness"].(map[string]any)
	if status != 200 || body["ok"] != true || rd == nil || rd["mode"] != ReadinessSetup || rd["next"] != StepComputer {
		t.Fatalf("warm: %d %v", status, body)
	}
}

// A computer that is not online is named, and said to be asleep rather than the hub unreachable.
func TestReadinessComputerOffline(t *testing.T) {
	f := newFakeHub().withMachine()
	f.devices = []Device{{ID: "server", Name: "小林的 MacBook"}}
	r := getReadiness(t, newStorelessService(t, f), "", "")
	c := readinessStep(t, r, StepComputer)
	if r.Mode != ReadinessSetup || c.State != StepProblem || c.Code != "node_unreachable" || c.Action.ID != "wake_computer" ||
		c.Title != "小林的 MacBook 不在线" || r.Machine == nil || r.Machine.Name != "小林的 MacBook" || c.Machine != "server" {
		t.Fatalf("readiness = %+v", r)
	}
}

// A computer with Claude Code and a model never tried: the main conversation can go (full), the model is unknown
// rather than a problem, and nothing spent tokens to find out.
func TestReadinessFullButUnverified(t *testing.T) {
	f, s, _ := readyMachine(t)
	r := getReadiness(t, s, "", "")
	cl, m, first := readinessStep(t, r, StepClaude), readinessStep(t, r, StepModel), readinessStep(t, r, StepFirstMessage)
	if r.Mode != ReadinessFull || !r.Ready || nextOf(r) != StepModel || r.Machine == nil || r.Machine.ID != "server" ||
		cl.State != StepOK || cl.Version != "2.1.308" ||
		m.State != StepUnknown || m.Code != "model_unverified" || m.Action != nil || // the try is the first message's (#6317)
		first.State != StepTodo || first.Code != "never_tried" || first.Action == nil || first.Action.ID != "test_assistant" {
		t.Fatalf("readiness = %+v", r)
	}
	if len(f.checked) != 0 {
		t.Fatalf("checked without a probe: %v", f.checked)
	}
	// The self-check is asked for Claude Code's version only: without the path-access reads (#5944).
	f.mu.Lock()
	asked := 0
	for _, c := range f.calls {
		if c.Method == deviceNodeDoctor {
			asked++
			if string(c.Params) != `{"preflight":false}` {
				t.Errorf("node.doctor params %s", c.Params)
			}
		}
	}
	f.mu.Unlock()
	if asked == 0 {
		t.Error("node.doctor not asked")
	}
	if len(r.Suggestions) < 3 || r.Suggestions[2].ID != "today" || len(r.Suggestions[2].Needs) != 1 || r.Suggestions[2].Prompt == "" {
		t.Fatalf("suggestions = %+v", r.Suggestions)
	}
}

// The node's self-check found no claude for the plain launcher: Claude Code is missing.
func TestReadinessClaudeMissing(t *testing.T) {
	f, s, _ := readyMachine(t)
	f.doctor["server"] = "读不到 claude 的版本"
	r := getReadiness(t, s, "", "")
	cl := readinessStep(t, r, StepClaude)
	if r.Mode != ReadinessSetup || nextOf(r) != StepClaude || cl.State != StepProblem || cl.Code != proto.CodeClaudeMissing ||
		cl.Action.ID != "install_claude" || len(cl.AltActions) != 1 || cl.AltActions[0].ID != "connect_model" ||
		cl.Detail != "server 上还没装 Claude Code。装好它，或者改用 API Key。" {
		t.Fatalf("claude = %+v (%s)", cl, r.Mode)
	}
}

// A node with no self-check (an old one) leaves Claude Code unknown, which does not stop the main conversation.
func TestReadinessOldNodeUnknown(t *testing.T) {
	f, s, _ := readyMachine(t)
	f.doctor = nil
	r := getReadiness(t, s, "", "")
	if cl := readinessStep(t, r, StepClaude); cl.State != StepUnknown || r.Mode != ReadinessFull {
		t.Fatalf("readiness = %+v", r)
	}
}

// A start refused because Claude Code is not there is the claude step's, until a start goes or a self-check made
// after it reads a version (installed since: in setup mode no start comes to clear it).
func TestReadinessStartRefusal(t *testing.T) {
	f, s, st := readyMachine(t)
	f.doctor["server"] = "读不到 claude 的版本"
	refuse := func() {
		t.Helper()
		data, _ := json.Marshal(map[string]string{"delivered": "none", "launcher": "claude"})
		f.mu.Lock()
		f.failAssistant = &rpc.Error{Code: proto.CodeClaudeMissing, Message: "zsh:1: command not found: claude", Data: data}
		f.mu.Unlock()
		if status, _ := serveJSON(t, s, "POST", "/api/v1/assistant/conversations", "", `{"prompt":"你好","main":true}`); status != 409 {
			t.Fatalf("start: %d", status)
		}
		f.mu.Lock()
		f.failAssistant = nil
		f.mu.Unlock()
	}
	refuse()
	if cl := readinessStep(t, getReadiness(t, s, "", ""), StepClaude); cl.Code != proto.CodeClaudeMissing || cl.Action.ID != "install_claude" {
		t.Fatalf("claude = %+v", cl)
	}
	// Claude Code installed: the next self-check reads a version and the refusal is forgotten.
	old := readinessClaudeRetry
	readinessClaudeRetry = 0
	t.Cleanup(func() { readinessClaudeRetry = old })
	f.mu.Lock()
	f.doctor["server"] = "2.1.308，支持 --plugin-dir"
	f.mu.Unlock()
	if cl := readinessStep(t, getReadiness(t, s, "", ""), StepClaude); cl.State != StepOK || cl.Version != "2.1.308" {
		t.Fatalf("after installing: %+v", cl)
	}
	var rec readinessRecord
	if _, err := st.Setting(context.Background(), readinessKey, &rec); err != nil || rec.Start != nil {
		t.Fatalf("record = %+v (%v)", rec, err)
	}
	// Refused again, and the self-check after it reads no version (the version read before does not count): the
	// refusal holds until a start goes.
	readinessClaudeRetry = time.Hour
	f.mu.Lock()
	f.doctor["server"] = "读不到 claude 的版本"
	f.mu.Unlock()
	refuse()
	if cl := readinessStep(t, getReadiness(t, s, "", ""), StepClaude); cl.Code != proto.CodeClaudeMissing {
		t.Fatalf("refused again: %+v", cl)
	}
	if status, body := serveJSON(t, s, "POST", "/api/v1/assistant/conversations", "", `{"prompt":"你好","main":true}`); status != 200 {
		t.Fatalf("start: %d %v", status, body)
	}
	if _, err := st.Setting(context.Background(), readinessKey, &rec); err != nil || rec.Start != nil {
		t.Fatalf("after a start: %+v (%v)", rec, err)
	}
}

// The main conversation's last turn was Claude Code's "Not logged in": the model step says sign in (or use a key),
// and so does the first message.
func TestReadinessNotLoggedIn(t *testing.T) {
	_, s, _ := readyMachine(t, proto.Session{SessionID: "m1", State: proto.StateIdle, LastText: "Not logged in · Please run /login",
		UpdatedAt: time.Now()})
	r := getReadiness(t, s, "", "")
	m, first := readinessStep(t, r, StepModel), readinessStep(t, r, StepFirstMessage)
	if r.Mode != ReadinessSetup || nextOf(r) != StepModel || m.State != StepProblem || m.Code != proto.TurnErrorNotLoggedIn ||
		m.Action.ID != "connect_model" || len(m.AltActions) != 1 || m.AltActions[0].ID != "login_claude" ||
		m.Title != "Claude Code 还没登录" || m.Detail != "在 server 上登录一次，或者改用 API Key。" || m.Launcher != "claude" ||
		first.State != StepProblem || first.Code != proto.TurnErrorNotLoggedIn {
		t.Fatalf("readiness = %+v", r)
	}
}

// An answer in the main conversation: all ready, and the first message stays done after a later failure.
func TestReadinessAnswered(t *testing.T) {
	f, s, _ := readyMachine(t, proto.Session{SessionID: "m1", State: proto.StateIdle, LastText: "你好，我是 Persona。",
		UpdatedAt: time.Now()})
	r := getReadiness(t, s, "", "")
	if r.Mode != ReadinessFull || r.Next != nil || readinessStep(t, r, StepModel).State != StepOK || readinessStep(t, r, StepFirstMessage).State != StepOK {
		t.Fatalf("readiness = %+v", r)
	}
	f.setSessions("server", proto.Session{SessionID: "m1", State: proto.StateIdle, LastText: "API Error: 429 rate limit",
		UpdatedAt: time.Now().Add(time.Second)})
	r = getReadiness(t, s, "", "")
	m := readinessStep(t, r, StepModel)
	if m.State != StepProblem || m.Code != proto.TurnErrorRateLimited || r.Mode != ReadinessFull || // a rate limit passes by itself
		readinessStep(t, r, StepFirstMessage).State != StepOK {
		t.Fatalf("after a rate limit: %+v", r)
	}
}

// ?probe=1 runs the one-word check once; asked again within 30 seconds it answers the same result. A key that does
// not work is the model step's problem, with editing the provider to do.
func TestReadinessProbe(t *testing.T) {
	f, s, _ := readyMachine(t)
	f.checks = map[string]proto.LauncherCheck{"server/claude": {Error: "Invalid API key · Please run /login", Millis: 900}}
	r := getReadiness(t, s, "probe=1", "")
	m := readinessStep(t, r, StepModel)
	if r.Probe == nil || r.Probe.OK || r.Probe.Error == "" || m.Code != proto.TurnErrorAuthFailed || m.Action.ID != "edit_provider" ||
		r.Mode != ReadinessSetup {
		t.Fatalf("readiness = %+v probe %+v", r, r.Probe)
	}
	if r2 := getReadiness(t, s, "probe=1", ""); r2.Probe == nil || r2.Probe.Millis != 900 || len(f.checked) != 1 {
		t.Fatalf("again: %+v, checked %v", r2.Probe, f.checked)
	}
	if r3 := getReadiness(t, s, "", ""); r3.Probe != nil || readinessStep(t, r3, StepModel).Code != proto.TurnErrorAuthFailed {
		t.Fatalf("without probe: %+v", r3)
	}
	// Past the reuse window it runs again; this time it answers.
	old := readinessProbeReuse
	readinessProbeReuse = 0
	t.Cleanup(func() { readinessProbeReuse = old })
	f.mu.Lock()
	f.checks = nil
	f.mu.Unlock()
	if r4 := getReadiness(t, s, "probe=1", ""); r4.Probe == nil || !r4.Probe.OK || readinessStep(t, r4, StepModel).State != StepOK ||
		len(f.checked) != 2 || r4.Mode != ReadinessFull {
		t.Fatalf("again later: %+v, checked %v", r4, f.checked)
	}
}

// A client that stops waiting for its probe (its own time ran out, the page closed) does not cut the check off: the
// result is kept, and the next try within the reuse window answers it instead of starting the check again (#1892).
func TestReadinessProbeOutlivesTheClient(t *testing.T) {
	f, s, _ := readyMachine(t)
	started, release := make(chan struct{}, 1), make(chan struct{})
	var asked atomic.Int32 // the checks the node was asked for, the one cut off too
	f.mu.Lock()
	f.checkWait = func(ctx context.Context) error {
		asked.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = serveHTTP(ctx, s, assistantwire.HTTPRequest{Method: "GET", Path: "/api/v1/assistant/readiness", Query: "probe=1"}, s.lang)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the check never started")
	}
	cancel() // the client hangs up while the check runs
	time.Sleep(50 * time.Millisecond)
	close(release)
	<-done
	r := getReadiness(t, s, "probe=1", "")
	if r.Probe == nil || !r.Probe.OK || asked.Load() != 1 {
		t.Fatalf("the try after: %+v, checks asked for %d (want the first one's result, 1)", r.Probe, asked.Load())
	}
}

// "Later" is kept in the settings (assistant.readiness), with the last result.
func TestReadinessDismiss(t *testing.T) {
	f, s, st := readyMachine(t)
	if status, body := serveJSON(t, s, "POST", "/api/v1/assistant/readiness/dismiss", "", `{"dismissed":true}`); status != 200 || body["ok"] != true {
		t.Fatalf("dismiss: %d %v", status, body)
	}
	if r := getReadiness(t, s, "", ""); !r.Dismissed {
		t.Fatalf("readiness = %+v", r)
	}
	var rec readinessRecord
	if ok, err := st.Setting(context.Background(), readinessKey, &rec); !ok || err != nil || !rec.Dismissed || rec.Last == nil ||
		rec.Last.Mode != ReadinessFull {
		t.Fatalf("record = %+v (%v %v)", rec, ok, err)
	}
	// A new assistant on the same database reads it back.
	if r := getReadiness(t, newServiceOn(t, f, st), "", ""); !r.Dismissed {
		t.Fatalf("after a restart: %+v", r)
	}
	// The same result again is not written again.
	puts := st.puts
	getReadiness(t, s, "", "")
	if st.puts != puts {
		t.Fatalf("written again: %d → %d", puts, st.puts)
	}
	if status, _ := serveJSON(t, s, "POST", "/api/v1/assistant/readiness/dismiss", "", `{"dismissed":false}`); status != 200 {
		t.Fatal(status)
	}
	if r := getReadiness(t, s, "", ""); r.Dismissed {
		t.Fatalf("undone: %+v", r)
	}
}

// A launcher of the user's own that the machine no longer has: launcher_missing; one that runs Codex: skipped.
func TestReadinessLauncher(t *testing.T) {
	f, s, _ := readyMachine(t)
	f.hello.Assistant.Launcher = "cc_gone"
	f.zshrc = map[string][]proto.Launcher{"server": {{Name: "cx", Agent: "codex"}}}
	r := getReadiness(t, s, "", "")
	if cl := readinessStep(t, r, StepClaude); r.Launcher != "cc_gone" || cl.Code != proto.CodeLauncherMissing || cl.Action.ID != "connect_model" ||
		cl.Detail != "在 server 上找不到「cc_gone」这个模型入口，可能被删了。" || r.Mode != ReadinessSetup {
		t.Fatalf("readiness = %+v", r)
	}
	f.mu.Lock()
	f.hello.Assistant.Launcher = "cx"
	f.mu.Unlock()
	if cl := readinessStep(t, getReadiness(t, s, "", ""), StepClaude); cl.State != StepSkipped {
		t.Fatalf("codex: %+v", cl)
	}
}

// A direct API provider added in Persona that speaks Messages starts Claude Code on its gateway (#2455): its launcher
// is there, not launcher_missing, right after the key is added.
func TestReadinessManagedAPILauncher(t *testing.T) {
	f, s, _ := readyMachine(t)
	key, err := s.sealKey("sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.putSetting(context.Background(), launchersManagedKey, []storedLauncher{{ManagedLauncher: proto.ManagedLauncher{Name: "direct",
		Kind: "api", API: "messages", BaseURL: "https://api.example.com", Model: "m"}, KeySealed: key}}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.hello.Assistant.Launcher = "direct"
	f.mu.Unlock()
	r := getReadiness(t, s, "", "")
	if cl := readinessStep(t, r, StepClaude); r.Launcher != "direct" || cl.State == StepProblem || r.Mode != ReadinessFull {
		t.Fatalf("readiness = %+v", r)
	}
}

// A record that cannot be read is not written over: what was kept ("Later") survives a database hiccup.
func TestReadinessUnreadRecordKept(t *testing.T) {
	_, s, st := readyMachine(t)
	if status, _ := serveJSON(t, s, "POST", "/api/v1/assistant/readiness/dismiss", "", `{}`); status != 200 {
		t.Fatal(status)
	}
	s.ready.mu.Lock()
	s.ready.loaded, s.ready.rec = false, readinessRecord{}
	s.ready.mu.Unlock()
	st.mu.Lock()
	st.failSetting = errors.New("database hiccup")
	st.mu.Unlock()
	getReadiness(t, s, "", "")
	if status, _ := serveJSON(t, s, "POST", "/api/v1/assistant/readiness/dismiss", "", `{"dismissed":false}`); status != 500 {
		t.Fatalf("dismiss over an unread record: %d", status)
	}
	st.mu.Lock()
	st.failSetting = nil
	st.mu.Unlock()
	var rec readinessRecord
	if _, err := st.Setting(context.Background(), readinessKey, &rec); err != nil || !rec.Dismissed {
		t.Fatalf("record = %+v (%v)", rec, err)
	}
}

// Claude Code signed out as the launcher runs it (claude.auth_status, free): the model step says sign in before any
// message is sent. The answer is kept; once the user signs in, the next question after the short retry says so.
func TestReadinessSignedOutStatus(t *testing.T) {
	f, s, _ := readyMachine(t)
	f.auth = map[string]proto.ClaudeAuthStatus{"server/claude": {State: proto.ClaudeNotLoggedIn, AuthMethod: "none"}}
	r := getReadiness(t, s, "", "")
	m := readinessStep(t, r, StepModel)
	if r.Mode != ReadinessSetup || nextOf(r) != StepModel || m.State != StepProblem || m.Code != proto.TurnErrorNotLoggedIn ||
		m.Action.ID != "connect_model" || len(m.AltActions) != 1 || m.AltActions[0].ID != "login_claude" || m.Title != "Claude Code 还没登录" {
		t.Fatalf("readiness = %+v", r)
	}
	getReadiness(t, s, "", "")
	f.mu.Lock()
	asked := append([]string(nil), f.authAsked...)
	f.mu.Unlock()
	if len(asked) != 1 || asked[0] != "server/claude" {
		t.Fatalf("asked %v", asked)
	}
	// Signed in to claude.ai since: that is the model's evidence, and the main conversation can go.
	old := readinessClaudeRetry
	readinessClaudeRetry = 0
	t.Cleanup(func() { readinessClaudeRetry = old })
	f.mu.Lock()
	f.auth["server/claude"] = proto.ClaudeAuthStatus{State: proto.ClaudeLoggedIn, AuthMethod: "claude.ai"}
	f.mu.Unlock()
	r = getReadiness(t, s, "", "")
	if m := readinessStep(t, r, StepModel); r.Mode != ReadinessFull || m.State != StepOK {
		t.Fatalf("after signing in: %+v", r)
	}
	if len(f.checked) != 0 {
		t.Fatalf("spent tokens: %v", f.checked)
	}
}

// Signed in with a key (or a gateway's token) says nothing of whether the key works: the model stays unverified, and
// says what it runs on.
func TestReadinessKeyStatusUnverified(t *testing.T) {
	f, s, _ := readyMachine(t)
	f.auth = map[string]proto.ClaudeAuthStatus{"server/claude": {State: proto.ClaudeLoggedIn, AuthMethod: "api_key", KeySource: "ANTHROPIC_API_KEY"}}
	r := getReadiness(t, s, "", "")
	m := readinessStep(t, r, StepModel)
	if r.Mode != ReadinessFull || m.State != StepUnknown || m.Code != "model_unverified" ||
		m.Detail != "server 上的 Claude Code 用的是配好的账号或 Key，还没试过。发一句试试它能不能用。" {
		t.Fatalf("readiness = %+v", r)
	}
	// One button for the try, on the first message's step, where clients show how it went (#6317).
	if first := readinessStep(t, r, StepFirstMessage); m.Action != nil || first.State != StepTodo || first.Action == nil || first.Action.ID != "test_assistant" {
		t.Fatalf("the try's button: model %+v, first message %+v", m.Action, first)
	}
	// A main conversation replaced since counts as answered: the model step keeps the try then.
	f3 := newFakeHub().withMachine()
	f3.doctor = map[string]string{"server": "2.1.308，支持 --plugin-dir"}
	f3.settings = &Settings{Main: "m1", Former: []FormerMain{{MachineID: "server", SessionID: "m0"}}}
	f3.auth = f.auth
	r3 := getReadiness(t, newServiceOn(t, f3, &memStore{}), "", "")
	if m3, first3 := readinessStep(t, r3, StepModel), readinessStep(t, r3, StepFirstMessage); first3.State != StepOK || m3.Action == nil || m3.Action.ID != "test_assistant" {
		t.Fatalf("first message ok: model %+v, first message %+v", m3, first3)
	}
	// An unknown status (an old Claude Code, a launcher that drops the arguments) is no evidence either way.
	f2, s2, _ := readyMachine(t)
	f2.auth = map[string]proto.ClaudeAuthStatus{"server/claude": {State: proto.ClaudeAuthUnknown, Reason: proto.ClaudeAuthOld}}
	if m := readinessStep(t, getReadiness(t, s2, "", ""), StepModel); m.State != StepUnknown || m.Detail != "发一句试试，确认模型能回答。" {
		t.Fatalf("unknown status: %+v", m)
	}
}

// The newest evidence wins: an answer in the main conversation after a signed-out status (signed in since, within
// the cache) is the model working.
func TestReadinessAnswerAfterSignedOutStatus(t *testing.T) {
	f, s, _ := readyMachine(t)
	f.auth = map[string]proto.ClaudeAuthStatus{"server/claude": {State: proto.ClaudeNotLoggedIn}}
	if m := readinessStep(t, getReadiness(t, s, "", ""), StepModel); m.Code != proto.TurnErrorNotLoggedIn {
		t.Fatalf("model = %+v", m)
	}
	f.setSessions("server", proto.Session{SessionID: "m1", State: proto.StateIdle, LastText: "你好。", UpdatedAt: time.Now().Add(time.Second)})
	if r := getReadiness(t, s, "", ""); readinessStep(t, r, StepModel).State != StepOK || r.Mode != ReadinessFull {
		t.Fatalf("after an answer: %+v", r)
	}
}

// Signed in to claude.ai only disproves signed out: a failure a turn met (a model the plan lacks, a rate limit) stays,
// even when the sign-in status was asked after it.
func TestReadinessSignedInKeepsTurnFailure(t *testing.T) {
	for _, c := range []struct{ text, code string }{
		{"There's an issue with the selected model (claude-x). It may not exist or you may not have access to it.", proto.TurnErrorModelRoute},
		{"API Error: 429 rate limit", proto.TurnErrorRateLimited},
	} {
		f, s, _ := readyMachine(t, proto.Session{SessionID: "m1", State: proto.StateIdle, TurnError: c.text,
			UpdatedAt: time.Now().Add(-time.Minute)})
		f.auth = map[string]proto.ClaudeAuthStatus{"server/claude": {State: proto.ClaudeLoggedIn, AuthMethod: "claude.ai"}}
		r := getReadiness(t, s, "", "")
		if m := readinessStep(t, r, StepModel); m.State != StepProblem || m.Code != c.code {
			t.Fatalf("%s: model = %+v", c.code, m)
		}
	}
	// With nothing else known it is the model's evidence.
	f, s, _ := readyMachine(t)
	f.auth = map[string]proto.ClaudeAuthStatus{"server/claude": {State: proto.ClaudeLoggedIn, AuthMethod: "claude.ai"}}
	if m := readinessStep(t, getReadiness(t, s, "", ""), StepModel); m.State != StepOK {
		t.Fatalf("nothing else: %+v", m)
	}
}

// A status the node could not read in time is asked again after the short retry; an old node's unknown method waits
// the long one.
func TestReadinessAuthStatusRetry(t *testing.T) {
	old := readinessClaudeRetry
	readinessClaudeRetry = 0
	t.Cleanup(func() { readinessClaudeRetry = old })
	f, s, _ := readyMachine(t)
	f.auth = map[string]proto.ClaudeAuthStatus{"server/claude": {State: proto.ClaudeAuthUnknown, Reason: proto.ClaudeAuthTimeout}}
	getReadiness(t, s, "", "")
	getReadiness(t, s, "", "")
	f.mu.Lock()
	asked := len(f.authAsked)
	f.mu.Unlock()
	if asked != 2 {
		t.Fatalf("timeout asked %d times", asked)
	}
	f2, s2, _ := readyMachine(t) // f2.auth nil: unknown method
	getReadiness(t, s2, "", "")
	getReadiness(t, s2, "", "")
	f2.mu.Lock()
	asked = len(f2.authAsked)
	f2.mu.Unlock()
	if asked != 1 {
		t.Fatalf("old node asked %d times", asked)
	}
}
