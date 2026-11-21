package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rexcli"
)

type fakeRex struct {
	mu      sync.Mutex
	ls      string
	views   map[string]string
	procs   map[string]string
	titles  map[string]string
	clients map[string]string
	capture string
	errs    map[string]error
	calls   []fakeRexCall
}

type fakeRexCall struct {
	args  []string
	stdin string
}

func (f *fakeRex) client() *rexcli.Client {
	return &rexcli.Client{Bin: func() string { return "rex" }, Run: f.run}
}

func (f *fakeRex) run(_ context.Context, _ string, stdin []byte, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeRexCall{args: append([]string(nil), args...), stdin: string(stdin)})
	if len(args) > 0 && args[0] == "version" {
		return []byte("rex 0.1.0-preview.21\ncommit: 202b1b3\n"), nil
	}
	rest := args
	if len(rest) > 0 && rest[0] == "--autostart=false" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return nil, errors.New("empty args")
	}
	if err := f.errs[rest[0]]; err != nil {
		return nil, err
	}
	switch rest[0] {
	case "ls":
		return []byte(f.ls), nil
	case "capture":
		return []byte(f.capture), nil
	case "send", "send-key":
		return nil, nil
	case "block":
		// block call [-s S] -b B METHOD
		var bid, method string
		for i := 2; i < len(rest); i++ {
			if rest[i] == "-b" && i+1 < len(rest) {
				bid = rest[i+1]
				i++
			} else if rest[i] == "-s" && i+1 < len(rest) {
				i++
			} else {
				method = rest[i]
			}
		}
		if err := f.errs[method]; err != nil {
			return nil, err
		}
		switch method {
		case "process":
			if p, ok := f.procs[bid]; ok {
				return []byte(p), nil
			}
			return nil, fmt.Errorf(`exit status 1: Error: block target %q not found`, bid)
		case "title":
			if t, ok := f.titles[bid]; ok {
				return []byte(t), nil
			}
			return []byte(`{"title":""}`), nil
		}
	case "api":
		// api call [-s S] METHOD [-]
		var sid, method string
		for i := 2; i < len(rest); i++ {
			if rest[i] == "-s" && i+1 < len(rest) {
				sid = rest[i+1]
				i++
			} else if rest[i] != "-" {
				method = rest[i]
			}
		}
		if err := f.errs[method]; err != nil {
			return nil, err
		}
		switch method {
		case "server.status":
			return []byte(`{"pid":123,"api_revision":18}`), nil
		case "session.view":
			if v, ok := f.views[sid]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("exit status 1: session_not_found")
		case "session.create":
			return []byte(`{"session_id":"session:new","initial_windows":[{"window_id":"window:new","block_ids":["block:new"],"revision":1}]}`), nil
		case "session.new_window":
			return []byte(`{"window_id":"window:tab2","block_ids":["block:tab2"],"revision":2}`), nil
		case "session.new_split":
			return []byte(`{"block_ids":["block:split1"],"revision":3}`), nil
		case "session.list_clients":
			if c, ok := f.clients[sid]; ok {
				return []byte(c), nil
			}
			return []byte(`{"clients":[]}`), nil
		default:
			return []byte(`{}`), nil
		}
	}
	return nil, nil
}

func (f *fakeRex) calledMethod(method string) []fakeRexCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []fakeRexCall
	for _, c := range f.calls {
		if slices.Contains(c.args, method) {
			out = append(out, c)
		}
	}
	return out
}

func sampleRex() *fakeRex {
	return &fakeRex{
		ls: `{"sessions":[{"session_id":"session:s1","label":"workspace-1"}]}`,
		views: map[string]string{
			"session:s1": `{"revision":1,"session_id":"session:s1","label":"workspace-1","active_window_id":"window:w1","windows":[
				{"window_id":"window:w1","label":"Window 1","focused_block_id":"block:b1","layers":[
					{"layer_id":"layer:l1","kind":"tiled","blocks":[
						{"block_id":"block:b1","label":"shell","creator_name":"com.superlogical.terminal","flavor":"com.superlogical.terminal.shell"},
						{"block_id":"block:onb","label":"welcome","creator_name":"com.superlogical.onboarding","flavor":"com.superlogical.onboarding.onboarding"}
					]}
				]},
				{"window_id":"window:w2","label":"api","focused_block_id":"block:b2","layers":[
					{"layer_id":"layer:l2","kind":"tiled","blocks":[
						{"block_id":"block:b2","label":"shell","creator_name":"com.superlogical.terminal","flavor":"com.superlogical.terminal.shell"},
						{"block_id":"block:b3","label":"shell","creator_name":"com.superlogical.terminal","flavor":"com.superlogical.terminal.shell"},
						{"block_id":"block:b4","label":"shell","creator_name":"com.superlogical.terminal","flavor":"com.superlogical.terminal.shell"}
					]}
				]}
			]}`,
		},
		procs: map[string]string{
			"block:b1": `{"child":{"pid":100,"name":"login","path":"/usr/bin/login"},
				"foreground":{"pid":101,"name":"2.1.329","path":"/home/me/.local/share/claude/versions/2.1.329","argv0":"claude","cwd":"/home/me/app"},
				"ancestors":[{"pid":102,"name":"zsh","path":"/bin/zsh","argv0":"-zsh","cwd":"/home/me/app"}]}`,
			"block:b2": `{"child":{"pid":200,"name":"login"},
				"foreground":{"pid":201,"name":"zsh","argv0":"-zsh","cwd":"/home/me/api"}}`,
			"block:b3": `{"foreground":{"pid":301,"name":"node","argv0":"node","cwd":"/home/me/api"}}`,
			"block:b4": `{"foreground":{"pid":401,"name":"herdr","argv0":"herdr","cwd":"/home/me"}}`,
		},
		titles: map[string]string{
			"block:b1": `{"title":"✳ Fix the build"}`,
			"block:b2": `{"title":"zsh"}`,
			"block:b3": `{"title":"codex"}`,
			"block:b4": `{"title":"herdr"}`,
		},
	}
}

func TestRexHostSnapshotAndProcesses(t *testing.T) {
	oldRead := readRexProcArgv
	readRexProcArgv = func(pid int) []string {
		if pid == 301 {
			return []string{"node", "/usr/lib/node_modules/@openai/codex/bin/codex.js"}
		}
		return nil
	}
	defer func() { readRexProcArgv = oldRead }()

	f := sampleRex()
	h := newRexHost(f.client())
	ctx := context.Background()

	if v := h.Version(ctx); v != "0.1.0-preview.21" {
		t.Fatalf("version = %q", v)
	}
	layout, err := h.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Spaces) != 1 || layout.Spaces[0].ID != "session:s1" || layout.Spaces[0].Label != "workspace-1" {
		t.Fatalf("spaces = %+v", layout.Spaces)
	}
	if len(layout.Slots) != 2 || layout.Slots[0].ID != "window:w1" || layout.Slots[1].Label != "api" {
		t.Fatalf("slots = %+v", layout.Slots)
	}
	// Onboarding block is skipped: 4 terminal blocks total.
	if len(layout.Panes) != 4 {
		t.Fatalf("panes = %d, want 4", len(layout.Panes))
	}
	// b1 (claude) and b3 (codex behind node) are agents; b2 (zsh) and b4 (herdr) are not.
	if len(layout.Agents) != 2 {
		t.Fatalf("agents = %+v, want 2", layout.Agents)
	}
	if layout.Agents[0].PaneID != "rex:block:b1" || layout.Agents[0].Agent != "claude" || layout.Agents[0].AgentStatus != proto.StatusUnknown || layout.Agents[0].Title != "Fix the build" {
		t.Errorf("agent[0] = %+v", layout.Agents[0])
	}
	if layout.Agents[1].PaneID != "rex:block:b3" || layout.Agents[1].Agent != "codex" {
		t.Errorf("agent[1] = %+v", layout.Agents[1])
	}

	pids, err := h.PaneProcesses(ctx, host.PaneRef{Kind: host.Rex, ID: "block:b1"})
	if err != nil || !slices.Equal(pids, []int{101, 102, 100}) {
		t.Errorf("PaneProcesses = %v, %v", pids, err)
	}
	fg, err := h.PaneForeground(ctx, host.PaneRef{Kind: host.Rex, ID: "block:b1"})
	if err != nil || !slices.Equal(fg, []string{"2.1.329", "claude"}) {
		t.Errorf("PaneForeground = %v, %v", fg, err)
	}
}

func TestRexHostReadPromptAndNotDelivered(t *testing.T) {
	f := sampleRex()
	f.capture = "line 1\n\x1b[32mline 2\x1b[0m\n\x1b[0m   \n\n"
	h := newRexHost(f.client())
	ctx := context.Background()
	if _, err := h.Snapshot(ctx); err != nil {
		t.Fatal(err)
	}

	pane := host.PaneRef{Kind: host.Rex, ID: "block:b1"}
	got, err := h.Read(ctx, pane, host.ReadOpts{ANSI: true, Lines: 1})
	if err != nil || got != "\x1b[32mline 2\x1b[0m\r\n" {
		t.Fatalf("Read ANSI = %q, %v", got, err)
	}

	// Prompt into shell pane (block:b2) is refused with ErrNotDelivered, nothing pasted.
	shellPane := host.PaneRef{Kind: host.Rex, ID: "block:b2"}
	if err := h.Prompt(ctx, shellPane, "hi"); !errors.Is(err, host.ErrNotDelivered) {
		t.Fatalf("Prompt into shell = %v, want ErrNotDelivered", err)
	}
	if len(f.calledMethod("block.event")) != 0 {
		t.Fatalf("block.event was called for shell pane")
	}

	// Prompt into claude pane pastes via block.event then presses enter.
	if err := h.Prompt(ctx, pane, "hello\x1bworld"); err != nil {
		t.Fatalf("Prompt into claude = %v", err)
	}
	events := f.calledMethod("block.event")
	if len(events) != 1 || strings.Contains(events[0].stdin, "\x1b") {
		t.Fatalf("paste calls = %+v", events)
	}
	if len(f.calledMethod("send-key")) != 1 {
		t.Fatalf("send-key calls = %+v", f.calledMethod("send-key"))
	}
}

func TestRexHostLaunchRenameSplitClose(t *testing.T) {
	f := sampleRex()
	h := newRexHost(f.client())
	ctx := context.Background()

	cr, err := h.CreateSpace(ctx, "/tmp/demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if cr.SpaceID != "session:new" || cr.SlotID != "window:new" || cr.Pane.String() != "rex:block:new" {
		t.Fatalf("CreateSpace = %+v", cr)
	}
	if err := h.Run(ctx, cr.Pane, "claude"); err != nil {
		t.Fatalf("Run = %v", err)
	}

	slot, err := h.CreateSlot(ctx, "session:s1", "/tmp/demo", "tests")
	if err != nil {
		t.Fatal(err)
	}
	if slot.SlotID != "window:tab2" || slot.Pane.String() != "rex:block:tab2" {
		t.Fatalf("CreateSlot = %+v", slot)
	}

	splitPane, err := h.Split(ctx, slot.Pane, "down", "/tmp/demo")
	if err != nil {
		t.Fatal(err)
	}
	if splitPane.String() != "rex:block:split1" {
		t.Fatalf("Split = %s", splitPane)
	}
	splits := f.calledMethod("session.new_split")
	var splitReq map[string]any
	if err := json.Unmarshal([]byte(splits[0].stdin), &splitReq); err != nil || splitReq["direction"] != "vertical" || splitReq["focus"] != false {
		t.Fatalf("split req = %s", splits[0].stdin)
	}

	if err := h.RenameSpace(ctx, "session:s1", "renamed-space"); err != nil {
		t.Fatal(err)
	}
	if err := h.RenameSlot(ctx, "window:tab2", "renamed-tab"); err != nil {
		t.Fatal(err)
	}

	// Close last block in an unattached empty session destroys the session.
	f.views["session:empty"] = `{"revision":2,"session_id":"session:empty","windows":[]}`
	if err := h.Close(ctx, host.PaneRef{Kind: host.Rex, ID: "block:last"}, "session:empty", "window:last"); err != nil {
		t.Fatal(err)
	}
	if len(f.calledMethod("session.destroy")) != 1 {
		t.Fatalf("expected session.destroy for empty unattached session")
	}

	if err := h.CloseSlot(ctx, "window:tab2"); err != nil {
		t.Fatal(err)
	}
	if len(f.calledMethod("session.close_window")) != 1 {
		t.Fatalf("expected session.close_window")
	}
}

func TestRexEngineDetection(t *testing.T) {
	f := sampleRex()
	e := rexEngine(context.Background(), f.client())
	if e.ID != proto.EngineRex || e.Name != "Rex" || !e.Available || e.Version != "0.1.0-preview.21" {
		t.Fatalf("rexEngine available = %+v", e)
	}

	f.errs = map[string]error{"server.status": errors.New("exit status 1: Error: no Rex server is running, and autostart is disabled")}
	e2 := rexEngine(context.Background(), f.client())
	if e2.Available || e2.Version != "0.1.0-preview.21" {
		t.Fatalf("rexEngine stopped = %+v", e2)
	}
}

// TestRexEventChange: the host's own rex calls (each a client of its own) never set off a new look, or every look would
// set off the next; blocks, windows and sessions changing do, and a bell or desktop notification is urgent.
func TestRexEventChange(t *testing.T) {
	for _, c := range []struct {
		ev              rexcli.Event
		refresh, urgent bool
	}{
		{rexcli.Event{Type: "client_joined"}, false, false},
		{rexcli.Event{Type: "client_left"}, false, false},
		{rexcli.Event{Type: "tailscale_status_changed"}, false, false},
		{rexcli.Event{Type: "session_event", Kind: "client_connected"}, false, false},
		{rexcli.Event{Type: "session_event", Kind: "client_disconnected"}, false, false},
		{rexcli.Event{Type: "session_event", Kind: "block_owner_changed"}, false, false},
		{rexcli.Event{Type: "session_event", Kind: "block_event", Name: "size_changed"}, false, false},
		{rexcli.Event{Type: "session_event", Kind: "block_event", Name: "selection_copied"}, false, false},
		{rexcli.Event{Type: "session_created"}, true, false},
		{rexcli.Event{Type: "session_destroyed"}, true, false},
		{rexcli.Event{Type: "session_event", Kind: "window_created"}, true, false},
		{rexcli.Event{Type: "session_event", Kind: "block_closed"}, true, false},
		{rexcli.Event{Type: "session_event", Kind: "session_label_changed"}, true, false},
		{rexcli.Event{Type: "session_event", Kind: "block_event", Name: "process_changed"}, true, false},
		{rexcli.Event{Type: "session_event", Kind: "block_event", Name: "title_changed"}, true, false},
		{rexcli.Event{Type: "session_event", Kind: "block_event", Name: "bell"}, true, true},
		{rexcli.Event{Type: "session_event", Kind: "block_event", Name: "desktop_notification"}, true, true},
	} {
		if refresh, urgent := rexEventChange(c.ev); refresh != c.refresh || urgent != c.urgent {
			t.Errorf("%+v: refresh %v urgent %v, want %v %v", c.ev, refresh, urgent, c.refresh, c.urgent)
		}
	}
}
