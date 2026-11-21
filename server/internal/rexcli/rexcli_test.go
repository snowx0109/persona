package rexcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"rex 0.1.0-preview.21\ncommit: 202b1b3\nbuilt: 2026-11-17T18:23:18Z\n", "0.1.0-preview.21"},
		{"rex 1.2.3\n", "1.2.3"},
		{"0.5.0\n", "0.5.0"},
		{"", ""},
	} {
		if got := parseVersion(tc.in); got != tc.want {
			t.Errorf("parseVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestClassify(t *testing.T) {
	if err := classify(errors.New("exit status 1: Error: no Rex server is running at /tmp/x.sock, and autostart is disabled")); !errors.Is(err, ErrNotRunning) {
		t.Errorf("not running: %v", err)
	}
	if err := classify(errors.New(`exit status 1: Error: block target "block:0" not found`)); !errors.Is(err, ErrNotFound) {
		t.Errorf("not found: %v", err)
	}
	if err := classify(errors.New("exit status 1: session_not_found")); !errors.Is(err, ErrNotFound) {
		t.Errorf("session_not_found: %v", err)
	}
}

func TestAPICallAndAutostartDisabled(t *testing.T) {
	var gotArgs []string
	var gotStdin []byte
	c := &Client{
		Bin: func() string { return "/bin/rex" },
		Run: func(_ context.Context, _ string, stdin []byte, args ...string) ([]byte, error) {
			gotArgs = args
			gotStdin = stdin
			return []byte(`{"session_id":"session:1","initial_windows":[{"window_id":"window:1","block_ids":["block:1"]}]}`), nil
		},
	}
	cr, err := c.CreateSession(context.Background(), "work", "/tmp/proj")
	if err != nil {
		t.Fatal(err)
	}
	if cr.SessionID != "session:1" || cr.WindowID != "window:1" || cr.BlockID != "block:1" {
		t.Fatalf("created = %+v", cr)
	}
	if len(gotArgs) == 0 || gotArgs[0] != "--autostart=false" {
		t.Fatalf("missing --autostart=false: %v", gotArgs)
	}
	var body map[string]any
	if err := json.Unmarshal(gotStdin, &body); err != nil {
		t.Fatalf("stdin json: %v", err)
	}
	if body["label"] != "work" {
		t.Errorf("label = %v", body["label"])
	}
}

func TestEventsStream(t *testing.T) {
	old := streamLiveAfter
	streamLiveAfter = 10 * time.Millisecond
	defer func() { streamLiveAfter = old }()

	// Lines as rex 0.1.0-preview.21 writes them (ids shortened).
	lines := strings.Join([]string{
		`{"type":"client_joined","payload":{"client":{"client_id":"client:c1","info":{"kind":"cli","name":"rex"}}}}`,
		`{"type":"session_event","payload":{"session_id":"session:1","event":{"type":"block_event","payload":{"block_id":"block:1","creator_name":"com.superlogical.terminal","name":"bell","payload":{}}}}}`,
		`not json`,
		`{"type":"session_event","payload":{"session_id":"session:1","event":{"type":"client_connected","payload":{"client_id":"client:c2"}}}}`,
		`{"type":"session_created","payload":{"session_id":"session:2"}}`,
	}, "\n") + "\n"

	c := &Client{
		Bin: func() string { return "/bin/rex" },
		Stream: func(_ context.Context, _ string, args ...string) (stream, error) {
			if len(args) == 0 || args[0] != "--autostart=false" {
				t.Errorf("stream missing --autostart=false: %v", args)
			}
			return stream{
				out:  bufio.NewScanner(strings.NewReader(lines)),
				wait: func() error { return nil },
			}, nil
		},
	}
	var lived int
	var got []Event
	if err := c.Events(context.Background(), func() { lived++ }, func(ev Event) { got = append(got, ev) }); err != nil {
		t.Fatal(err)
	}
	if lived != 1 {
		t.Errorf("live called %d times, want 1", lived)
	}
	want := []Event{
		{Type: "client_joined"},
		{Type: "session_event", SessionID: "session:1", Kind: "block_event", BlockID: "block:1", Name: "bell"},
		{Type: "session_event", SessionID: "session:1", Kind: "client_connected"},
		{Type: "session_created"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}
