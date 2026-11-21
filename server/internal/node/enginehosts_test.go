package node

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/proto"
)

// setHost is a host for the engine-set tests: its agents (or an error), the pids in each pane, and what it was sent.
type setHost struct {
	bareHost
	mu      sync.Mutex
	agents  []host.Agent
	err     error
	pids    map[string]int // pane ref → the claude's pid
	sent    []string       // pane ref + ": " + text
	calls   int
	looks   int   // PaneProcesses calls
	lookErr error // what PaneProcesses fails with, when set
}

func newSetHost(kind string, agents ...host.Agent) *setHost {
	return &setHost{bareHost: bareHost{kind: kind}, agents: agents, pids: map[string]int{}}
}

func (h *setHost) Snapshot(context.Context) (*host.Layout, error) {
	return nil, errors.New("no snapshot") // lookAt falls back to Agents
}

func (h *setHost) Agents(context.Context) ([]host.Agent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	if h.err != nil {
		return nil, h.err
	}
	return slices.Clone(h.agents), nil
}

func (h *setHost) PaneProcesses(_ context.Context, pane host.PaneRef) ([]int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.looks++
	if h.lookErr != nil {
		return nil, h.lookErr
	}
	if pid := h.pids[pane.String()]; pid != 0 {
		return []int{pid}, nil
	}
	return nil, nil
}

func (h *setHost) SendText(_ context.Context, pane host.PaneRef, text string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent = append(h.sent, pane.String()+": "+text)
	return nil
}

func (h *setHost) looked() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.looks
}

func (h *setHost) sentText() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.sent)
}

const (
	sidShared = "0d000000-0000-4000-8000-000000000001" // live in a herdr pane and an Orca pane at once
	sidOnly   = "0d000000-0000-4000-8000-000000000002" // live in the Orca pane alone
	herdrPane = "w1:p1"
	orcaPane  = "orca:t1:l1"
)

func claudeIn(pane, id, status string) host.Agent {
	return host.Agent{Agent: "claude", AgentSession: &host.AgentSession{Kind: "id", Value: id}, AgentStatus: status,
		PaneID: pane, Cwd: "/work/repo"}
}

// setNode is a node driving the set {herdr, orca} (default herdr) through two setHosts.
func setNode(t *testing.T) (*Node, *setHost, *setHost, string) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	n, home := testNode(t)
	hh := newSetHost(host.Herdr, claudeIn(herdrPane, sidShared, proto.StatusWorking))
	oh := newSetHost(host.Orca, claudeIn(orcaPane, sidShared, proto.StatusIdle), claudeIn("orca:t2:l1", sidOnly, proto.StatusIdle))
	n.hosts, n.panes, n.panePIDs = host.NewRegistry(hh, oh), map[string]string{}, map[string]panePID{}
	c := proto.EngineChoice{Hosts: []string{"herdr", "orca"}, Default: "herdr"}
	n.eng.name, n.eng.set = proto.LegacyEngine(c), &c
	return n, hh, oh, home
}

// writePID makes pid's ~/.claude/sessions file say it runs session id.
func writePID(t *testing.T, home string, pid int, id string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"pid": pid, "sessionId": id})
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(pid)+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// engineHostsFor registers the set's terminal hosts in canonical order; Paseo and the empty set register none.
func TestEngineHostsForSets(t *testing.T) {
	for _, c := range []struct {
		hosts []string
		kinds []string
	}{
		{[]string{"orca", "herdr"}, []string{host.Herdr, host.Orca}},
		{[]string{"herdr", "herdr-core", "orca", "paseo", "cindy"}, []string{host.Herdr, host.HerdrCore, host.Orca}},
		{[]string{"tern", "paseo", "herdr"}, []string{host.Herdr, host.Tern}},
		{[]string{"rex", "tern", "paseo"}, []string{host.Tern, host.Rex}},
		{[]string{"paseo"}, nil},
		{[]string{"cindy"}, nil},
		{[]string{}, nil},
	} {
		ch, err := proto.NormalizeEngineChoice(c.hosts, "")
		if err != nil {
			t.Fatal(err)
		}
		var kinds []string
		for _, h := range engineHostsFor(ch, nil) {
			kinds = append(kinds, h.Kind())
		}
		if !slices.Equal(kinds, c.kinds) {
			t.Errorf("%v: hosts %v, want %v", c.hosts, kinds, c.kinds)
		}
	}
}

// engine.use with a set: the hosts in it are registered, the reply and node.info say the set, the single engine is
// the closest one; an older hub's {engine} still works; a bad set is refused and changes nothing.
func TestEngineUseTakesASet(t *testing.T) {
	n, _ := testNode(t)
	n.hosts = host.NewRegistry(nodeHosts(proto.EngineHerdr, false, nil)...)
	out, err := n.handle(context.Background(), "engine.use", json.RawMessage(`{"engine":"herdr","hosts":["orca","herdr"],"default":"orca"}`))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(out); string(b) != `{"default":"orca","engine":"orca","hosts":["herdr","orca"]}` {
		t.Errorf("reply %s", b)
	}
	if n.hosts.Get(host.Herdr) == nil || n.hosts.Get(host.Orca) == nil || n.defaultKind() != "orca" || !n.hostEnabled("orca") {
		t.Errorf("hosts %v default %q", n.hosts.All(), n.defaultKind())
	}
	if c := n.info(context.Background()).EngineHosts; c == nil || !slices.Equal(c.Hosts, []string{"herdr", "orca"}) || c.Default != "orca" {
		t.Errorf("node.info engine_hosts %v", c)
	}
	if _, err := n.handle(context.Background(), "engine.use", json.RawMessage(`{"hosts":["tmux"]}`)); err == nil {
		t.Error("an unknown host was taken")
	}
	if n.defaultKind() != "orca" {
		t.Errorf("a refused set changed the default: %q", n.defaultKind())
	}
	if _, err := n.handle(context.Background(), "engine.use", json.RawMessage(`{"engine":"herdr-core"}`)); err != nil {
		t.Fatal(err)
	}
	if c := n.engineChoice(); !slices.Equal(c.Hosts, []string{"herdr-core"}) || n.engine() != "herdr-core" {
		t.Errorf("single engine: %v", c)
	}
}

// Changing only the default host keeps every host and its agents: no reset, no ended terminal streams.
func TestEngineUseDefaultOnlyKeepsHosts(t *testing.T) {
	n, hh, oh, _ := setNode(t)
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	epoch := n.engEpoch
	if err := n.useEngineChoice(context.Background(), proto.EngineChoice{Hosts: []string{"herdr", "orca"}, Default: "orca"}); err != nil {
		t.Fatal(err)
	}
	if n.engEpoch != epoch || n.hosts.Get(host.Herdr) != hh || n.hosts.Get(host.Orca) != oh {
		t.Errorf("hosts were replaced: epoch %d → %d", epoch, n.engEpoch)
	}
	if _, ok := n.agent(sidOnly); !ok {
		t.Error("agents were dropped")
	}
	if n.defaultKind() != "orca" || n.engine() != "orca" {
		t.Errorf("default %q engine %q", n.defaultKind(), n.engine())
	}
}

// The empty set: no terminal host, nothing starts (the phone hears why), and the list still has what it can show.
func TestEmptySetStartsNothing(t *testing.T) {
	n, _ := testNode(t)
	n.hosts = host.NewRegistry(nodeHosts(proto.EngineHerdr, false, nil)...)
	if err := n.useEngineChoice(context.Background(), proto.EngineChoice{Hosts: []string{}}); err != nil {
		t.Fatal(err)
	}
	// tmux stays: an empty set still reads and answers the dialogs of a claude the user started in tmux (#2562).
	if len(n.hosts.All()) != 1 || n.hosts.All()[0].Kind() != host.Tmux || n.defaultKind() != "" || n.defaultHost() != nil || n.engine() != "herdr" {
		t.Fatalf("hosts %v default %q", n.hosts.All(), n.defaultKind())
	}
	_, err := hostCap[host.Launcher](n)
	if code, _ := refusalOf(t, err); code != engineMissing || !strings.Contains(err.Error(), "这台电脑没启用终端引擎") {
		t.Errorf("hostCap = %v", err)
	}
	_, err = n.startAsync("", t.TempDir(), "", "hi", "", "", "", "")
	if code, _ := refusalOf(t, err); code != engineMissing {
		t.Errorf("startAsync = %v", err)
	}
	if n.hostEnabled(proto.EngineHerdr) {
		t.Error("herdr enabled in the empty set")
	}
	if _, err := n.startHerdrServer(context.Background()); err == nil {
		t.Error("herdr started on a machine that does not enable it")
	}
}

// Rule 3: two live panes, neither one's process or hook says more: the default host's pane owns the session; the
// session is listed once and names the other host.
func TestOwnerIsTheDefaultHostAtLast(t *testing.T) {
	n, _, _, _ := setNode(t)
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, _ := n.agent(sidShared); a.PaneID != herdrPane {
		t.Errorf("owner %q, want the herdr pane", a.PaneID)
	}
	if got := n.alsoOpenIn(sidShared); !slices.Equal(got, []string{host.Orca}) {
		t.Errorf("also open in %v", got)
	}
	if got := n.alsoOpenIn(sidOnly); got != nil {
		t.Errorf("a session in one host is flagged: %v", got)
	}
	s := proto.Session{SessionID: sidShared}
	n.decorate(&s)
	if !slices.Equal(s.AlsoOpenIn, []string{"orca"}) {
		t.Errorf("row also_open_in %v", s.AlsoOpenIn)
	}

	// The other default: the Orca pane.
	c := proto.EngineChoice{Hosts: []string{"herdr", "orca"}, Default: "orca"}
	n.eng.set = &c
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, _ := n.agent(sidShared); a.PaneID != orcaPane {
		t.Errorf("default orca: owner %q", a.PaneID)
	}
	if got := n.alsoOpenIn(sidShared); !slices.Equal(got, []string{host.Herdr}) {
		t.Errorf("also open in %v", got)
	}
}

// Rule 2 beats rule 3: the pane whose SessionStart hook named the session last owns it.
func TestOwnerIsTheLastSessionStart(t *testing.T) {
	n, _, _, _ := setNode(t)
	n.noteSessionStart(herdrPane, sidShared)
	time.Sleep(2 * time.Millisecond)
	n.noteSessionStart(orcaPane, sidShared) // resumed in Orca after herdr
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, _ := n.agent(sidShared); a.PaneID != orcaPane {
		t.Errorf("owner %q, want the pane that started it last", a.PaneID)
	}
	// A SessionStart that named another session (the pane moved on) does not count.
	n.noteSessionStart(orcaPane, sidOnly)
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, _ := n.agent(sidShared); a.PaneID != herdrPane {
		t.Errorf("owner %q after the Orca pane started another session", a.PaneID)
	}
}

// Rule 1 beats both: the pane whose own claude process runs the session.
func TestOwnerIsThePaneWhoseProcessRunsIt(t *testing.T) {
	n, _, oh, home := setNode(t)
	n.noteSessionStart(herdrPane, sidShared) // the hook and the default both say herdr
	oh.pids[orcaPane] = 5151
	writePID(t, home, 5151, sidShared)
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, _ := n.agent(sidShared); a.PaneID != orcaPane {
		t.Errorf("owner %q, want the pane whose claude runs it", a.PaneID)
	}
}

// Sends and pane lookups go to the owner only; the other host gets nothing.
func TestSendGoesToTheOwnerOnly(t *testing.T) {
	n, hh, oh, _ := setNode(t)
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	pane, err := n.paneFor(context.Background(), sidShared)
	if err != nil || pane != herdrPane {
		t.Fatalf("paneFor = %q, %v", pane, err)
	}
	if err := n.sendText(context.Background(), pane, "hello"); err != nil {
		t.Fatal(err)
	}
	if got := hh.sentText(); !slices.Equal(got, []string{herdrPane + ": hello"}) {
		t.Errorf("herdr got %v", got)
	}
	if got := oh.sentText(); len(got) != 0 {
		t.Errorf("orca got %v", got)
	}
}

// claudeOutsidePanes is a claude in a plain terminal (this test's own pid, so it is alive) running a session no pane
// runs: the merge then also checks what the panes whose sessions their host names run (setAgentsAt's own).
func claudeOutsidePanes(t *testing.T, home string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "sessionId": "0d000000-0000-4000-8000-0000000000ff", "kind": "interactive"})
	if err := os.WriteFile(filepath.Join(home, ".claude", "sessions", strconv.Itoa(os.Getpid())+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A refresh asks only the hosts that answered it what their panes run (#5816). The session merge covers every host's
// agents and asked each pane's host for its processes (sessionFromProcess): Orca's poll every second asked herdr, and
// herdr's refresh ran Orca's process scan (three ps runs, about 0.2 s on a Mac) before it could finish. A pane of
// another host keeps the pid its own refresh found: its session stays listed in that pane. Each host has a claude it
// names no session for and one it names, checked because a claude outside every pane runs another session.
func TestARefreshAsksOnlyItsOwnHostsWhatTheirPanesRun(t *testing.T) {
	const (
		herdrNamed = "0d000000-0000-4000-8000-000000000003"
		orcaNamed  = "0d000000-0000-4000-8000-000000000004"
	)
	n, hh, oh, home := setNode(t)
	unnamed := func(pane string) host.Agent {
		return host.Agent{Agent: "claude", PaneID: pane, Cwd: "/work/repo", AgentStatus: proto.StatusIdle}
	}
	hh.agents = []host.Agent{unnamed(herdrPane), claudeIn("w1:p2", herdrNamed, proto.StatusIdle)}
	oh.agents = []host.Agent{unnamed(orcaPane), claudeIn("orca:t2:l1", orcaNamed, proto.StatusIdle)}
	hh.pids[herdrPane], hh.pids["w1:p2"], oh.pids[orcaPane], oh.pids["orca:t2:l1"] = 4101, 4102, 4201, 4202
	writePID(t, home, 4101, sidShared)
	writePID(t, home, 4102, herdrNamed)
	writePID(t, home, 4201, sidOnly)
	writePID(t, home, 4202, orcaNamed)
	claudeOutsidePanes(t, home)
	listed := func(when string) {
		t.Helper()
		for id, pane := range map[string]string{sidShared: herdrPane, herdrNamed: "w1:p2", sidOnly: orcaPane, orcaNamed: "orca:t2:l1"} {
			if a, _ := n.agent(id); a.PaneID != pane {
				t.Errorf("%s: %s in %q, want %s", when, id, a.PaneID, pane)
			}
		}
	}
	ctx := context.Background()
	if err := n.refreshHosts(ctx); err != nil {
		t.Fatal(err)
	}
	listed("every host refreshed")
	if hh.looked() != 2 || oh.looked() != 2 {
		t.Fatalf("a refresh of every host asked herdr %d times, Orca %d times, want each pane once", hh.looked(), oh.looked())
	}
	for _, c := range []struct {
		name             string
		refreshed, other *setHost
	}{{"Orca's poll", oh, hh}, {"herdr's refresh", hh, oh}} {
		dueLooks(n, host.Herdr) // a minute on: every pane is due for another look
		dueLooks(n, host.Orca)
		own, other := c.refreshed.looked(), c.other.looked()
		if err := n.refreshHosts(ctx, c.refreshed); err != nil {
			t.Fatal(err)
		}
		if got := c.other.looked() - other; got != 0 {
			t.Errorf("%s asked the other host what its panes run (%d times)", c.name, got)
		}
		if got := c.refreshed.looked() - own; got != 2 {
			t.Errorf("%s looked at its own panes %d times, want 2", c.name, got)
		}
		listed("after " + c.name)
	}
}

// A session live in two hosts has the same owner whichever host refreshed (ownership's rule 1 asks every candidate's
// host, #5816). herdr names the session in its pane and was never asked what runs there; then the same session is
// resumed in Orca. Both panes' claudes run it and no hook said which started it last, so the default host's pane owns it
// (rule 3), after Orca's poll as after herdr's refresh.
func TestAContestedSessionsOwnerDoesNotDependOnWhichHostRefreshed(t *testing.T) {
	n, hh, oh, home := setNode(t)
	hh.agents, oh.agents = []host.Agent{claudeIn(herdrPane, sidShared, proto.StatusIdle)}, nil
	hh.pids[herdrPane], oh.pids[orcaPane] = 4101, 4201
	writePID(t, home, 4101, sidShared)
	writePID(t, home, 4201, sidShared)
	ctx := context.Background()
	if err := n.refreshHosts(ctx); err != nil {
		t.Fatal(err)
	}
	oh.agents = []host.Agent{{Agent: "claude", PaneID: orcaPane, Cwd: "/work/repo", AgentStatus: proto.StatusIdle}}
	for _, c := range []struct {
		name string
		h    *setHost
	}{{"Orca's poll", oh}, {"herdr's refresh", hh}, {"Orca's next poll", oh}} {
		if err := n.refreshHosts(ctx, c.h); err != nil {
			t.Fatal(err)
		}
		if a, _ := n.agent(sidShared); a.PaneID != herdrPane {
			t.Errorf("after %s: owner %q, want herdr's pane", c.name, a.PaneID)
		}
		if got := n.alsoOpenIn(sidShared); !slices.Equal(got, []string{host.Orca}) {
			t.Errorf("after %s: also open in %v", c.name, got)
		}
	}
}

// A process lookup that fails keeps the pid found before (#5816): the other host's refreshes read that pid
// (sessionFromKnownProcess), and the pane's own refresh must not drop the session they list.
func TestAFailedLookupKeepsThePidFoundBefore(t *testing.T) {
	n, hh, oh, home := setNode(t)
	hh.agents = nil
	oh.agents = []host.Agent{{Agent: "claude", PaneID: orcaPane, Cwd: "/work/repo", AgentStatus: proto.StatusIdle}}
	oh.pids[orcaPane] = 4201
	writePID(t, home, 4201, sidOnly)
	ctx := context.Background()
	if err := n.refreshHosts(ctx); err != nil {
		t.Fatal(err)
	}
	oh.mu.Lock()
	oh.lookErr = errors.New("ps: interrupted")
	oh.mu.Unlock()
	for _, c := range []struct {
		name string
		h    *setHost
	}{{"herdr's refresh", hh}, {"Orca's poll", oh}} {
		dueLooks(n, host.Orca)
		if err := n.refreshHosts(ctx, c.h); err != nil {
			t.Fatal(err)
		}
		if a, ok := n.agent(sidOnly); !ok || a.PaneID != orcaPane {
			t.Errorf("after %s: listed %v in %q", c.name, ok, a.PaneID)
		}
	}
}

// Same-host duplicates behave as before sets (the later pane listed) and are not flagged as a conflict; an offline
// row gives way to the live one as before.
func TestNoConflictWithinOneHostOrWithOffline(t *testing.T) {
	n, hh, oh, _ := setNode(t)
	hh.agents = []host.Agent{claudeIn("w1:p1", sidShared, proto.StatusIdle), claudeIn("w1:p2", sidShared, proto.StatusIdle)}
	oh.agents = []host.Agent{claudeIn(orcaPane, sidOnly, proto.StatusOffline), claudeIn("orca:t9:l1", sidShared, proto.StatusOffline)}
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, _ := n.agent(sidShared); a.PaneID != "w1:p2" {
		t.Errorf("same host: owner %q", a.PaneID)
	}
	if got := n.alsoOpenIn(sidShared); got != nil {
		t.Errorf("flagged %v", got)
	}
}

// One host down: the other's sessions are listed and driven as before; the host that came back is listed again.
func TestOneHostDownLeavesTheOther(t *testing.T) {
	n, hh, oh, _ := setNode(t)
	hh.err = errors.New("herdr: connection refused")
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatalf("refresh with one host down: %v", err)
	}
	if a, ok := n.agent(sidOnly); !ok || a.PaneID != "orca:t2:l1" {
		t.Errorf("orca's own session: %v %v", a, ok)
	}
	if a, _ := n.agent(sidShared); a.PaneID != orcaPane {
		t.Errorf("the shared session: owner %q, want orca (herdr gave nothing yet)", a.PaneID)
	}
	if err := n.sendText(context.Background(), "orca:t2:l1", "still here"); err != nil || len(oh.sentText()) != 1 {
		t.Errorf("send to orca: %v %v", err, oh.sentText())
	}
	// herdr not running is an answer: nothing there, orca untouched.
	hh.err = host.ErrNotRunning
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := n.agent(sidOnly); !ok {
		t.Error("orca's session went with herdr")
	}
	hh.err = nil
	if err := n.refreshHosts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, _ := n.agent(sidShared); a.PaneID != herdrPane {
		t.Errorf("herdr back: owner %q", a.PaneID)
	}
}

// Pane-less operations follow the default host.
func TestDefaultHostDecides(t *testing.T) {
	n, hh, _, _ := setNode(t)
	if h, _ := n.hostOfKind(""); h != hh {
		t.Errorf("hostOfKind default = %v", h)
	}
	if n.defaultEngineName() != "herdr" {
		t.Errorf("name %q", n.defaultEngineName())
	}
	c := proto.EngineChoice{Hosts: []string{"herdr", "orca"}, Default: "orca"}
	n.eng.set = &c
	if n.defaultEngineName() != "Orca" || n.defaultHost().Kind() != host.Orca {
		t.Errorf("default orca: %q", n.defaultEngineName())
	}
	if !n.hostEnabled(proto.EngineHerdr) {
		t.Error("herdr not enabled")
	}
}
