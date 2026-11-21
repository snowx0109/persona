// Package rexcli drives Rex, a multiplexing terminal (docs/integrations/engines.md "Rex"), through its `rex` command
// line and control API: listing sessions and window layouts, inspecting a terminal block's foreground process and
// title, capturing colored screens, sending text/pastes/keys, opening/splitting/renaming/closing blocks and sessions,
// and streaming server events.
//
// Every command passes --autostart=false so probing or snapshotting never starts a Rex server behind the user's back.
package rexcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ErrNotInstalled: no rex binary on this machine.
var ErrNotInstalled = errors.New("rex is not installed")

// ErrNotRunning: rex is installed but its server is not running.
var ErrNotRunning = errors.New("rex is not running")

// ErrNotFound: the session, window or block named does not exist (any more).
var ErrNotFound = errors.New("rex: not found")

// callTimeout bounds one command; a capture of a long scrollback is the slowest.
const callTimeout = 5 * time.Second

// streamLiveAfter: a `rex events` that has not failed this long after starting is live.
var streamLiveAfter = 300 * time.Millisecond

const (
	// TerminalCreator is Rex's block creator for terminal blocks.
	TerminalCreator = "com.superlogical.terminal"
	// ShellFlavor is the terminal flavor for an interactive shell block.
	ShellFlavor = "com.superlogical.terminal.shell"
)

// Client runs the rex binary. The zero value is not usable: New sets the binary lookup and the runner.
type Client struct {
	// Bin finds the binary each call ("" when Rex is not installed), so installing Rex later needs no restart.
	Bin func() string
	// Run runs the binary with optional stdin and args, returning stdout; a failure carries stderr text (tests swap it
	// for a fake rex).
	Run func(ctx context.Context, bin string, stdin []byte, args ...string) ([]byte, error)
	// Stream starts the binary with args and returns its stdout as a line stream (`rex events`).
	Stream func(ctx context.Context, bin string, args ...string) (stream, error)
	Log    func(format string, args ...any)
}

// New is a client of the rex binary FindBin finds.
func New() *Client { return &Client{Bin: FindBin, Run: run, Stream: startStream} }

// FindBin is rex on PATH, else ~/.local/bin/rex, else, on macOS, the managed RexServer binary inside Application
// Support or the app bundle: a resident node (launchd, systemd) has only the system's PATH.
func FindBin() string {
	if p, err := exec.LookPath("rex"); err == nil {
		return p
	}
	var cands []string
	home, homeErr := os.UserHomeDir()
	if homeErr == nil {
		cands = append(cands, filepath.Join(home, ".local", "bin", "rex"))
	}
	if runtime.GOOS == "darwin" {
		if homeErr == nil {
			cands = append(cands,
				filepath.Join(home, "Library", "Application Support", "com.superlogical.rex", "RexServer", "Versions", "Current", "Rex Server.app", "Contents", "MacOS", "rex"),
			)
		}
		cands = append(cands,
			"/Applications/Rex.app/Contents/MacOS/rex",
			"/Applications/Rex Beta.app/Contents/MacOS/rex",
		)
		if homeErr == nil {
			cands = append(cands,
				filepath.Join(home, "Applications", "Rex.app", "Contents", "MacOS", "rex"),
				filepath.Join(home, "Applications", "Rex Beta.app", "Contents", "MacOS", "rex"),
			)
		}
	}
	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// run is the real runner: the binary's stdout, or an error carrying its stderr.
func run(ctx context.Context, bin string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("%w: %s", err, msg)
		}
		return out, err
	}
	return out, nil
}

// classify turns rex's error text into this package's sentinel errors.
func classify(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "no rex server is running"),
		strings.Contains(low, "autostart is disabled"),
		strings.Contains(low, "connection refused"):
		return fmt.Errorf("%w: %v", ErrNotRunning, err)
	case strings.Contains(low, "not found"),
		strings.Contains(low, "session_not_found"):
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}

// call runs one rex command with --autostart=false and returns its stdout.
func (c *Client) call(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	bin := c.Bin()
	if bin == "" {
		return nil, ErrNotInstalled
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	full := make([]string, 0, len(args)+1)
	full = append(full, "--autostart=false")
	full = append(full, args...)
	out, err := c.Run(ctx, bin, stdin, full...)
	return out, classify(err)
}

// apiCall invokes a server control API method via `rex --autostart=false api call [-s session] method [-]`.
func (c *Client) apiCall(ctx context.Context, sessionID, method string, body any) ([]byte, error) {
	args := []string{"api", "call"}
	if sessionID != "" {
		args = append(args, "-s", sessionID)
	}
	args = append(args, method)
	var stdin []byte
	if body != nil {
		var err error
		stdin, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("rex %s: %w", method, err)
		}
		args = append(args, "-")
	}
	return c.call(ctx, stdin, args...)
}

// Version is the binary's version ("0.1.0-preview.21"), "" when Rex is not installed or does not answer. It needs no
// running server.
func (c *Client) Version(ctx context.Context) string {
	bin := c.Bin()
	if bin == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	out, err := c.Run(ctx, bin, nil, "version")
	if err != nil {
		return ""
	}
	return parseVersion(string(out))
}

// parseVersion takes "rex 0.1.0-preview.21\ncommit: ..." to "0.1.0-preview.21".
func parseVersion(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	f := strings.Fields(line)
	if len(f) >= 2 && f[0] == "rex" {
		return f[1]
	}
	if len(f) == 1 {
		return f[0]
	}
	return ""
}

// Status verifies that the Rex server is reachable without autostarting it.
func (c *Client) Status(ctx context.Context) error {
	_, err := c.apiCall(ctx, "", "server.status", nil)
	return err
}

// SessionRef is one entry of `rex ls --json`.
type SessionRef struct {
	SessionID string `json:"session_id"`
	Label     string `json:"label"`
}

// Sessions lists all sessions on the server.
func (c *Client) Sessions(ctx context.Context) ([]SessionRef, error) {
	out, err := c.call(ctx, nil, "ls", "--json")
	if err != nil {
		return nil, err
	}
	var res struct {
		Sessions []SessionRef `json:"sessions"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("rex ls: %w", err)
	}
	return res.Sessions, nil
}

// SessionView is the result of `session.view`: one session's windows, layers and blocks.
type SessionView struct {
	Revision       int      `json:"revision"`
	SessionID      string   `json:"session_id"`
	Label          string   `json:"label"`
	ActiveWindowID string   `json:"active_window_id"`
	Windows        []Window `json:"windows"`
}

// Window is a tab within a Rex session.
type Window struct {
	WindowID       string  `json:"window_id"`
	Label          string  `json:"label"`
	FocusedBlockID string  `json:"focused_block_id"`
	Layers         []Layer `json:"layers"`
}

// Layer holds placed blocks in a window.
type Layer struct {
	LayerID string  `json:"layer_id"`
	Kind    string  `json:"kind"`
	Blocks  []Block `json:"blocks"`
}

// Block is one pane inside a window layer.
type Block struct {
	BlockID     string `json:"block_id"`
	Label       string `json:"label"`
	CreatorName string `json:"creator_name"`
	Flavor      string `json:"flavor"`
	HostID      string `json:"host_id"`
	Owner       string `json:"owner"`
}

// Terminal reports whether the block is a terminal block.
func (b Block) Terminal() bool { return b.CreatorName == TerminalCreator }

// SessionView reads one session's windows and blocks.
func (c *Client) SessionView(ctx context.Context, sessionID string) (*SessionView, error) {
	out, err := c.apiCall(ctx, sessionID, "session.view", nil)
	if err != nil {
		return nil, err
	}
	var v SessionView
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("rex session.view: %w", err)
	}
	return &v, nil
}

// Proc is a process reported by `com.superlogical.terminal.process`.
type Proc struct {
	Pid         int    `json:"pid"`
	UserID      int    `json:"user_id"`
	User        string `json:"user"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	InvokedPath string `json:"invoked_path"`
	Argv0       string `json:"argv0"`
	Cwd         string `json:"cwd"`
}

// Process is the result of `rex block call process`.
type Process struct {
	Child      *Proc  `json:"child"`
	Foreground *Proc  `json:"foreground"`
	Ancestors  []Proc `json:"ancestors"`
	Home       string `json:"home"`
}

func blockScopeArgs(sessionID, blockID string) []string {
	var args []string
	if sessionID != "" {
		args = append(args, "-s", sessionID)
	}
	args = append(args, "-b", blockID)
	return args
}

// Process returns the processes running in a terminal block.
func (c *Client) Process(ctx context.Context, sessionID, blockID string) (*Process, error) {
	args := append([]string{"block", "call"}, blockScopeArgs(sessionID, blockID)...)
	args = append(args, "process")
	out, err := c.call(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	var p Process
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, fmt.Errorf("rex process: %w", err)
	}
	return &p, nil
}

// Title returns the terminal title reported by a terminal block ("" when unset).
func (c *Client) Title(ctx context.Context, sessionID, blockID string) (string, error) {
	args := append([]string{"block", "call"}, blockScopeArgs(sessionID, blockID)...)
	args = append(args, "title")
	out, err := c.call(ctx, nil, args...)
	if err != nil {
		return "", err
	}
	var t struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &t); err != nil {
		return "", fmt.Errorf("rex title: %w", err)
	}
	return t.Title, nil
}

// Capture reads a block's screen and scrollback as plain text or VT escape sequences.
func (c *Client) Capture(ctx context.Context, sessionID, blockID string, ansi, unwrap bool) (string, error) {
	args := append([]string{"capture"}, blockScopeArgs(sessionID, blockID)...)
	if ansi {
		args = append(args, "--format=vt")
	} else {
		args = append(args, "--format=text")
	}
	args = append(args, "--trim")
	if unwrap {
		args = append(args, "--unwrap")
	}
	out, err := c.call(ctx, nil, args...)
	return string(out), err
}

// SendText writes text directly into a block without pressing enter.
func (c *Client) SendText(ctx context.Context, sessionID, blockID, text string) error {
	if text == "" {
		return nil
	}
	args := append([]string{"send"}, blockScopeArgs(sessionID, blockID)...)
	_, err := c.call(ctx, []byte(text), args...)
	return err
}

// Paste sends a paste event to a block (bracketed paste when the foreground app enabled it), without pressing enter.
func (c *Client) Paste(ctx context.Context, sessionID, blockID, text string) error {
	body := map[string]any{
		"session_id": sessionID,
		"block_id":   blockID,
		"event": map[string]any{
			"type":    "paste",
			"payload": map[string]any{"text": text},
		},
	}
	_, err := c.apiCall(ctx, sessionID, "block.event", body)
	return err
}

// SendKeys presses keys in a block using Rex's key syntax (which matches Persona's key names: "enter", "esc",
// "shift+tab", "ctrl+u", "up", "backspace"…).
func (c *Client) SendKeys(ctx context.Context, sessionID, blockID string, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	args := append([]string{"send-key"}, blockScopeArgs(sessionID, blockID)...)
	args = append(args, keys...)
	_, err := c.call(ctx, nil, args...)
	return err
}

// Created is what CreateSession or NewWindow opened.
type Created struct {
	SessionID string
	WindowID  string
	BlockID   string
}

func shellLayout(cwd string) map[string]any {
	block := map[string]any{"flavor": ShellFlavor}
	if cwd != "" {
		block["options"] = map[string]any{"cwd": cwd}
	}
	return map[string]any{"block": block}
}

// CreateSession opens a new session with one shell window in cwd.
func (c *Client) CreateSession(ctx context.Context, label, cwd string) (*Created, error) {
	body := map[string]any{
		"initial_windows": []map[string]any{
			{"layout": shellLayout(cwd)},
		},
	}
	if label != "" {
		body["label"] = label
	}
	out, err := c.apiCall(ctx, "", "session.create", body)
	if err != nil {
		return nil, err
	}
	var res struct {
		SessionID      string `json:"session_id"`
		InitialWindows []struct {
			WindowID string   `json:"window_id"`
			BlockIDs []string `json:"block_ids"`
		} `json:"initial_windows"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("rex session.create: %w", err)
	}
	if res.SessionID == "" || len(res.InitialWindows) == 0 || len(res.InitialWindows[0].BlockIDs) == 0 {
		return nil, fmt.Errorf("rex session.create: no block in %q", bytes.TrimSpace(out))
	}
	return &Created{
		SessionID: res.SessionID,
		WindowID:  res.InitialWindows[0].WindowID,
		BlockID:   res.InitialWindows[0].BlockIDs[0],
	}, nil
}

// NewWindow opens a new window with one shell block in sessionID.
func (c *Client) NewWindow(ctx context.Context, sessionID, label, cwd string, focus bool) (*Created, error) {
	body := map[string]any{
		"session_id": sessionID,
		"focus":      focus,
		"layout":     shellLayout(cwd),
	}
	if label != "" {
		body["window_label"] = label
	}
	out, err := c.apiCall(ctx, sessionID, "session.new_window", body)
	if err != nil {
		return nil, err
	}
	var res struct {
		WindowID string   `json:"window_id"`
		BlockIDs []string `json:"block_ids"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("rex session.new_window: %w", err)
	}
	if len(res.BlockIDs) == 0 || res.BlockIDs[0] == "" {
		return nil, fmt.Errorf("rex session.new_window: no block in %q", bytes.TrimSpace(out))
	}
	return &Created{
		SessionID: sessionID,
		WindowID:  res.WindowID,
		BlockID:   res.BlockIDs[0],
	}, nil
}

// NewSplit splits anchorBlockID in direction ("horizontal" or "vertical"), placing the new shell block after it.
func (c *Client) NewSplit(ctx context.Context, sessionID, anchorBlockID, direction, cwd string, focus bool) (string, error) {
	body := map[string]any{
		"session_id": sessionID,
		"anchor_id":  anchorBlockID,
		"direction":  direction,
		"side":       "after",
		"focus":      focus,
		"layout":     shellLayout(cwd),
	}
	out, err := c.apiCall(ctx, sessionID, "session.new_split", body)
	if err != nil {
		return "", err
	}
	var res struct {
		BlockIDs []string `json:"block_ids"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("rex session.new_split: %w", err)
	}
	if len(res.BlockIDs) == 0 || res.BlockIDs[0] == "" {
		return "", fmt.Errorf("rex session.new_split: no block in %q", bytes.TrimSpace(out))
	}
	return res.BlockIDs[0], nil
}

// SetSessionLabel renames a session.
func (c *Client) SetSessionLabel(ctx context.Context, sessionID, label string) error {
	_, err := c.apiCall(ctx, sessionID, "session.set_label", map[string]any{
		"session_id": sessionID,
		"label":      label,
	})
	return err
}

// SetWindowLabel renames a window (tab) within a session.
func (c *Client) SetWindowLabel(ctx context.Context, sessionID, windowID, label string) error {
	_, err := c.apiCall(ctx, sessionID, "session.set_window_label", map[string]any{
		"session_id": sessionID,
		"window_id":  windowID,
		"label":      label,
	})
	return err
}

// CloseBlock closes one block.
func (c *Client) CloseBlock(ctx context.Context, sessionID, blockID string) error {
	_, err := c.apiCall(ctx, sessionID, "block.close", map[string]any{
		"session_id": sessionID,
		"block_id":   blockID,
	})
	return err
}

// CloseWindow closes a window and all blocks in it.
func (c *Client) CloseWindow(ctx context.Context, sessionID, windowID string) error {
	_, err := c.apiCall(ctx, sessionID, "session.close_window", map[string]any{
		"session_id": sessionID,
		"window_id":  windowID,
	})
	return err
}

// DestroySession destroys a session and all blocks in it.
func (c *Client) DestroySession(ctx context.Context, sessionID string) error {
	_, err := c.apiCall(ctx, sessionID, "session.destroy", map[string]any{
		"session_id": sessionID,
	})
	return err
}

// ListClients returns the number of clients currently attached to sessionID.
func (c *Client) ListClients(ctx context.Context, sessionID string) (int, error) {
	out, err := c.apiCall(ctx, sessionID, "session.list_clients", map[string]any{
		"session_id": sessionID,
	})
	if err != nil {
		return 0, err
	}
	var res struct {
		Clients []json.RawMessage `json:"clients"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return 0, fmt.Errorf("rex session.list_clients: %w", err)
	}
	return len(res.Clients), nil
}

// Event is one line of `rex events --all --json`, flattened from the server's envelope. A server event is
// {"type", "payload"}; a session's own events come as "session_event" with the session and its event in the payload,
// and a block's events as that session event's "block_event" with the block and the event's name:
//
//	{"type":"session_event","payload":{"session_id":"session:…","event":{"type":"block_event",
//	  "payload":{"block_id":"block:…","creator_name":"com.superlogical.terminal","name":"bell","payload":{}}}}}
type Event struct {
	// Type is the server event: "session_event", "session_created", "session_destroyed", "client_joined"…
	Type string
	// SessionID is the session a "session_event" is about.
	SessionID string
	// Kind is a "session_event"'s own event: "block_event", "window_created", "client_connected"…
	Kind string
	// BlockID is the block a "block_event" comes from.
	BlockID string
	// Name is a "block_event"'s name: "bell", "title_changed", "process_changed"…
	Name string
}

// parseEvent reads one line of `rex events --json`; ok is false for a line that is not an event. Parts of the
// envelope that do not decode are left empty rather than dropping the event.
func parseEvent(line []byte) (Event, bool) {
	var outer struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(line, &outer) != nil || outer.Type == "" {
		return Event{}, false
	}
	ev := Event{Type: outer.Type}
	if outer.Type != "session_event" {
		return ev, true
	}
	var session struct {
		SessionID string          `json:"session_id"`
		Event     json.RawMessage `json:"event"`
	}
	if json.Unmarshal(outer.Payload, &session) != nil {
		return ev, true
	}
	ev.SessionID = session.SessionID
	var inner struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(session.Event, &inner) != nil {
		return ev, true
	}
	ev.Kind = inner.Type
	if inner.Type != "block_event" {
		return ev, true
	}
	var block struct {
		BlockID string `json:"block_id"`
		Name    string `json:"name"`
	}
	if json.Unmarshal(inner.Payload, &block) == nil {
		ev.BlockID, ev.Name = block.BlockID, block.Name
	}
	return ev, true
}

// stream is a running `rex events`: its output scanner, and the wait for its end.
type stream struct {
	out  *bufio.Scanner
	wait func() error
}

func startStream(ctx context.Context, bin string, args ...string) (stream, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return stream{}, err
	}
	if err := cmd.Start(); err != nil {
		return stream{}, err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return stream{out: sc, wait: func() error {
		err := cmd.Wait()
		if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}}, nil
}

// Events streams server events as JSON lines until ctx ends or rex stops. live is called once the stream has started.
func (c *Client) Events(ctx context.Context, live func(), fn func(Event)) error {
	bin := c.Bin()
	if bin == "" {
		return ErrNotInstalled
	}
	s, err := c.Stream(ctx, bin, "--autostart=false", "events", "--all", "--json")
	if err != nil {
		return err
	}
	var once sync.Once
	goLive := func() { once.Do(live) }
	timer := time.AfterFunc(streamLiveAfter, goLive)
	defer timer.Stop()
	for s.out.Scan() {
		goLive()
		if ev, ok := parseEvent(s.out.Bytes()); ok {
			fn(ev)
		}
	}
	err = s.wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return classify(err)
}
