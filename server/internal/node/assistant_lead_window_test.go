package node

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/nwverse/persona/server/internal/proto"
)

// A leader conversation (a direction's lead, #15318) is held to the window the manager sends with it as the main
// conversation is, though it is not the main conversation: Claude's compact window and Codex's limit on a managed
// launcher that declares a larger one, with the idle /compact at 75% of it. A conversation the manager sends no window
// with keeps what the launcher declares.
func TestAssistantLeadCompactWindowPolicy(t *testing.T) {
	fakeClaudeHomeBash(t, "exit 0\n") // runs without zsh too
	t.Setenv("CODEX_HOME", t.TempDir())
	big := []proto.ProviderModel{{ID: "big", ContextWindow: 1_000_000}}
	n := codexTestNode(t)
	n.dir = t.TempDir()
	n.setManaged([]proto.ManagedLauncher{
		{Name: "gw1m", Model: "big", Models: big},
		{Name: "codex1m", Agent: "codex", Model: "big", Models: big},
	})
	cwd := t.TempDir()
	for _, tt := range []struct {
		name, launcher string
		limit          int
		env            string
		codex, idle    int
	}{
		{"leader conversation", "gw1m", 400_000, "CLAUDE_CODE_AUTO_COMPACT_WINDOW=400000 CLAUDE_CODE_MAX_CONTEXT_TOKENS=1000000", 400_000, 300_000},
		{"leader Codex conversation", "codex1m", 400_000, "CLAUDE_CODE_AUTO_COMPACT_WINDOW=400000 CLAUDE_CODE_MAX_CONTEXT_TOKENS=1000000", 400_000, 300_000},
		{"no window sent", "gw1m", 0, "CLAUDE_CODE_AUTO_COMPACT_WINDOW=1000000 CLAUDE_CODE_MAX_CONTEXT_TOKENS=1000000", 1_000_000, 750_000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := n.assistantCompactionPolicy(tt.launcher, cwd, "", modelExecution{MainCompactWindow: tt.limit})
			if got := strings.Join(c.env(), " "); got != tt.env || c.codexLimit() != tt.codex || c.idle() != tt.idle {
				t.Fatalf("policy %+v: env %q codex %d idle %d, want %q %d %d", c, got, c.codexLimit(), c.idle(), tt.env, tt.codex, tt.idle)
			}
			args := strings.Join(codexArgs("personad", "test", c.codexLimit()), " ")
			if !strings.Contains(args, "model_auto_compact_token_limit="+strconv.Itoa(tt.codex)) {
				t.Fatalf("codex args %q, want limit %d", args, tt.codex)
			}
		})
	}
}

// Through the node's RPC (Node.handle): a leader conversation's assistant.send and session.send carry
// main_compact_window without is_main. Its process starts with the window, and keeps it for a session.send with the
// same window and for a start with no options (the conversation's record, which a manual compaction uses). The context
// meter's auto-compact threshold follows it. A message without it (an older manager, or the conversation no longer
// leads) starts it again on the launcher's window.
func TestAssistantLeadCompactWindowRPC(t *testing.T) {
	f := cliTestBash(t) // runs without zsh too
	f.fakeLaunch(t)
	f.n.setManaged([]proto.ManagedLauncher{{Name: "gw", Model: "big", Models: []proto.ProviderModel{{ID: "big", ContextWindow: 1_000_000}}}})
	send := func(method string, p map[string]any) string {
		t.Helper()
		p["text"], p["is_main"], p["model_fingerprint"] = "hello", false, "fp"
		raw, err := nodeCall(t, f.n, method, p)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		var out struct {
			SessionID string `json:"session_id"`
		}
		_ = json.Unmarshal(raw, &out)
		return out.SessionID
	}
	id := send("assistant.send", map[string]any{"launcher": "gw", "cwd": t.TempDir(), "main_compact_window": 400_000})
	first := f.proc(id)
	if first == nil {
		t.Fatal("the leader conversation did not start")
	}
	if env := first.cmd.Env; envValue(env, "CLAUDE_CODE_AUTO_COMPACT_WINDOW") != "400000" || envValue(env, "CLAUDE_CODE_MAX_CONTEXT_TOKENS") != "1000000" || first.compactIdleAt != 300_000 {
		t.Fatalf("a leader conversation's start: AUTO %q MAX %q, idle %d", envValue(env, "CLAUDE_CODE_AUTO_COMPACT_WINDOW"),
			envValue(env, "CLAUDE_CODE_MAX_CONTEXT_TOKENS"), first.compactIdleAt)
	}
	if st, _ := f.n.state.get(id); st.IsMain || st.MainCompactWindow != 400_000 {
		t.Fatalf("recorded IsMain %v, window %d", st.IsMain, st.MainCompactWindow)
	}
	if got := f.n.sessionAutoCompactThreshold(id, "gw", 1_000_000); got != 400_000 {
		t.Fatalf("the context meter's auto-compact threshold: %d, want 400000", got)
	}
	f.idle(t, id)
	send("session.send", map[string]any{"session_id": id, "launcher": "gw", "main_compact_window": 400_000})
	if f.proc(id) != first {
		t.Fatal("session.send with the same window restarted the leader conversation")
	}
	f.idle(t, id)
	if _, err := f.n.assistantSend(context.Background(), id, "", "", "again", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if f.proc(id) != first {
		t.Fatal("a send with no options restarted the leader conversation without its window")
	}
	f.idle(t, id)
	send("session.send", map[string]any{"session_id": id, "launcher": "gw"})
	if p := f.proc(id); p == first || p.compactIdleAt != 750_000 {
		t.Fatalf("session.send without a window: same process %v, idle %d", p == first, p.compactIdleAt)
	}
	if st, _ := f.n.state.get(id); st.MainCompactWindow != 0 {
		t.Fatalf("recorded window %d after a message without one", st.MainCompactWindow)
	}
}

// While no process runs, the context meter's auto-compact threshold is the window the conversation last ran with,
// a leader conversation's as the main conversation's.
func TestAssistantLeadCompactWindowThresholdWithoutProcess(t *testing.T) {
	n := assistantNode(t)
	const id = "11111111-2222-4333-8444-555555555555"
	n.state.put(id, sessionState{Launcher: "claude", MainCompactWindow: 400_000})
	if got := n.sessionAutoCompactThreshold(id, "", 1_000_000); got != 400_000 {
		t.Fatalf("threshold %d, want 400000", got)
	}
	n.state.put(id, sessionState{Launcher: "claude"})
	if got := n.sessionAutoCompactThreshold(id, "", 1_000_000); got != 0 {
		t.Fatalf("threshold %d with no window, want 0 (not known)", got)
	}
}
