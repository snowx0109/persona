package assistant

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/proto"
)

// A direction's lead conversation is held to assistant.main_compact_window as the main conversation is (#15318). Its
// messages carry the window on every path the manager sends them by: the apps' messages and the node's own
// (sendModel, assistant.model), a relay, and the manager's own messages such as a supervision digest (session.send,
// session.send_once). The value is 400000 while the setting is unset and the saved value otherwise (0 too: the node then
// follows the launcher), and a value a caller set is replaced by it: the node applies whatever window comes with a
// conversation. Another side conversation and the background supervisor carry none, a value a caller set for them
// included, and a lead that no longer leads carries none from its next message.
func TestMainCompactWindowGoesWithLeaderConversations(t *testing.T) {
	ctx := context.Background()
	f := newFakeHub().withMachine()
	s := newTestService(t, f)
	start := func(p StartParams) string {
		t.Helper()
		sess, err := s.start(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		return sess.SessionID
	}
	main := start(StartParams{Main: true, Text: "hello"})
	lead, side, background := start(StartParams{Text: "lead"}), start(StartParams{Text: "side"}), start(StartParams{Text: "background"})
	f.setSessions("server", proto.Session{SessionID: main, Assistant: true}, proto.Session{SessionID: lead, Assistant: true},
		proto.Session{SessionID: side, Assistant: true}, proto.Session{SessionID: background, Assistant: true})
	lastCall := func(method string) map[string]any {
		t.Helper()
		f.mu.Lock()
		defer f.mu.Unlock()
		for i := len(f.calls) - 1; i >= 0; i-- {
			if f.calls[i].Method == method {
				var p map[string]any
				if err := json.Unmarshal(f.calls[i].Params, &p); err != nil {
					t.Fatal(err)
				}
				return p
			}
		}
		t.Fatalf("no %s", method)
		return nil
	}
	// window is what the manager sends with a message into conversation id on each of its paths, by path.
	window := func(t *testing.T, id string) map[string]any {
		t.Helper()
		got := map[string]any{}
		params, err := s.SendParams(ctx, SendParams{MachineID: "server", SessionID: id, Params: map[string]any{"text": "next", "main_compact_window": 1}})
		if err != nil {
			t.Fatal(err)
		}
		got["message"] = params["main_compact_window"]
		raw, _ := json.Marshal(map[string]string{"session_id": id})
		out, handled, err := s.NodeAnswer(ctx, "server", "assistant.model", raw)
		m, _ := out.(map[string]any)
		if err != nil || !handled {
			t.Fatalf("assistant.model: %v %v %v", out, handled, err)
		}
		got["assistant.model"] = m["main_compact_window"]
		if err := s.deliverRelay(ctx, "server", id, relayMsg{Tool: noticeTool, Text: "A task finished.", At: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
		got["relay"] = f.lastSend(t)["main_compact_window"]
		for _, method := range []string{DeviceSessionSend, DeviceSessionSendOnce} {
			if err := s.call(ctx, "server", method, map[string]any{"session_id": id, "text": "A task needs your review.", "main_compact_window": 1}, nil); err != nil {
				t.Fatal(err)
			}
			got[method] = lastCall(method)["main_compact_window"]
		}
		return got
	}
	for _, tt := range []struct {
		name string
		set  *int
		want int
	}{
		{"unset", nil, 400_000},
		{"off", ptr(0), 0},
		{"saved", ptr(250_000), 250_000},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setSettings(t, s, func(v *Settings) {
				v.Main, v.Leaders, v.BackgroundSupervisor, v.MainCompactWindow = main, []string{lead}, background, tt.set
			})
			for path, got := range window(t, lead) {
				// What SendParams and assistant.model answer is the map itself; what went to the node was JSON.
				if got != tt.want && got != float64(tt.want) {
					t.Errorf("lead, %s: main_compact_window %v, want %d", path, got, tt.want)
				}
			}
			for _, id := range []string{side, background} {
				for path, got := range window(t, id) {
					if got != nil {
						t.Errorf("%s, %s: main_compact_window %v, want none", id, path, got)
					}
				}
			}
		})
	}
	// Unpinned, it is an ordinary side conversation again: its next message carries no window.
	setSettings(t, s, func(v *Settings) { v.Leaders, v.MainCompactWindow = nil, nil })
	for path, got := range window(t, lead) {
		if got != nil {
			t.Errorf("a conversation that no longer leads, %s: main_compact_window %v, want none", path, got)
		}
	}
}
