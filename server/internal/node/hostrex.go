package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/lazyre"
	"github.com/nwverse/persona/server/internal/nodeclient"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rexcli"
)

// rexHost is Rex, a multiplexing terminal (docs/integrations/engines.md "Rex"), driven through its `rex` command line
// and control API. A pane is a terminal block (`com.superlogical.terminal`), its id the block's full ID
// ("block:<uuid>" — what Rex puts in the block's processes as REX_BLOCK). Spaces are Rex sessions and slots their
// windows (tabs).
//
// Listing, colored reads, typing, keys, prompts, process → session, events, starting, splitting, renaming and closing.
// No live terminal stream and no native worktrees (Persona falls back to git). Rex does not know which agent runs in a
// block: the node finds it from the block's foreground process (`block call process`), and its state from Claude
// Code's sessions file, transcript and hooks, as on Tern.
type rexHost struct {
	c *rexcli.Client

	mu       sync.Mutex
	failedAt time.Time // when the last look failed (failErr), for Agents right after a failed Snapshot
	failErr  error
	view     *rexView // the last look, reused for a moment (rexViewReuse) and for resolving session ids
	viewAt   time.Time
}

var (
	_ host.Host            = (*rexHost)(nil)
	_ host.Watcher         = (*rexHost)(nil)
	_ host.Launcher        = (*rexHost)(nil)
	_ host.Organizer       = (*rexHost)(nil)
	_ host.SlotCloser      = (*rexHost)(nil)
	_ host.Splitter        = (*rexHost)(nil)
	_ host.ForegroundNamer = (*rexHost)(nil)
)

func newRexHost(c *rexcli.Client) *rexHost { return &rexHost{c: c} }

// rexPaneEnv is the variable Rex sets in every terminal block's processes: the block's full ID ("block:<uuid>").
const rexPaneEnv = nodeclient.RexPaneEnv

const (
	// rexViewReuse: Agents right after a Snapshot, and session lookups, reuse the last look this long.
	rexViewReuse = 500 * time.Millisecond
	// rexProcsParallel: how many `rex block call` requests a look runs concurrently.
	rexProcsParallel = 4
)

// readRexProcArgv reads a process's full argv when Rex reports only argv0 ("node"/"bun") for a wrapper process;
// tests replace it for deterministic unit tests.
var readRexProcArgv = rexProcArgv

func rexProcArgv(pid int) []string {
	if pid <= 0 {
		return nil
	}
	switch runtime.GOOS {
	case "linux":
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil || len(b) == 0 {
			return nil
		}
		var out []string
		for _, part := range strings.Split(string(b), "\x00") {
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	case "windows":
		return nil
	default:
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "ps", "-ww", "-p", strconv.Itoa(pid), "-o", "args=").Output()
		if err != nil {
			return nil
		}
		return strings.Fields(string(out))
	}
}

func (h *rexHost) Kind() string    { return host.Rex }
func (h *rexHost) PaneEnv() string { return rexPaneEnv }

func (h *rexHost) Version(ctx context.Context) string { return h.c.Version(ctx) }

// rexErr marks Rex not installed or not running as host.ErrNotRunning: it then has no blocks.
func rexErr(err error) error {
	if errors.Is(err, rexcli.ErrNotRunning) || errors.Is(err, rexcli.ErrNotInstalled) {
		return host.Mark(err, host.ErrNotRunning)
	}
	return err
}

// rexInputErr marks the input refusals that typed nothing (host.ErrNotDelivered): Rex gone, or the block gone.
func rexInputErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, rexcli.ErrNotFound):
		return host.Mark(err, host.ErrNotDelivered)
	case errors.Is(err, rexcli.ErrNotRunning), errors.Is(err, rexcli.ErrNotInstalled):
		return host.Mark(rexErr(err), host.ErrNotDelivered)
	}
	return err
}

// rexView is one snapshot of Rex: every session's layout, plus each terminal block's processes and title.
type rexView struct {
	layout *host.Layout
	// session maps each window id and block id to its owning session id.
	session map[string]string
	// procs maps block id to its process info (nil when reading it failed).
	procs map[string]*rexcli.Process
	// titles maps block id to its terminal title.
	titles map[string]string
	// views keeps each session's SessionView by session id.
	views map[string]*rexcli.SessionView
}

// look lists all sessions, reads each session's view, and fetches every terminal block's process and title.
func (h *rexHost) look(ctx context.Context, fresh bool) (*rexView, error) {
	h.mu.Lock()
	if h.failErr != nil && time.Since(h.failedAt) < time.Second {
		err := h.failErr
		h.mu.Unlock()
		return nil, err
	}
	if !fresh && h.view != nil && time.Since(h.viewAt) < rexViewReuse {
		v := h.view
		h.mu.Unlock()
		return v, nil
	}
	h.mu.Unlock()

	refs, err := h.c.Sessions(ctx)
	if err != nil {
		err = rexErr(err)
		h.mu.Lock()
		h.failedAt, h.failErr = time.Now(), err
		h.mu.Unlock()
		return nil, err
	}
	var views []*rexcli.SessionView
	for _, ref := range refs {
		if ref.SessionID == "" {
			continue
		}
		sv, err := h.c.SessionView(ctx, ref.SessionID)
		if err != nil {
			if errors.Is(err, rexcli.ErrNotFound) {
				continue // session closed meanwhile
			}
			if errors.Is(err, rexcli.ErrNotRunning) || errors.Is(err, rexcli.ErrNotInstalled) {
				err = rexErr(err)
				h.mu.Lock()
				h.failedAt, h.failErr = time.Now(), err
				h.mu.Unlock()
				return nil, err
			}
			continue
		}
		if sv.Label == "" {
			sv.Label = ref.Label
		}
		views = append(views, sv)
	}
	v := h.build(ctx, views)
	h.mu.Lock()
	h.failErr = nil
	h.view, h.viewAt = v, time.Now()
	h.mu.Unlock()
	return v, nil
}

func (h *rexHost) build(ctx context.Context, views []*rexcli.SessionView) *rexView {
	v := &rexView{
		layout:  &host.Layout{},
		session: map[string]string{},
		views:   map[string]*rexcli.SessionView{},
	}
	type place struct {
		s *rexcli.SessionView
		w rexcli.Window
		b rexcli.Block
	}
	var blocks []place
	seenBlock := map[string]bool{}
	for _, sv := range views {
		if sv == nil || sv.SessionID == "" {
			continue
		}
		if _, dup := v.views[sv.SessionID]; dup {
			continue
		}
		v.views[sv.SessionID] = sv
		spaceLabel := sv.Label
		if spaceLabel == "" {
			spaceLabel = sv.SessionID
		}
		v.layout.Spaces = append(v.layout.Spaces, host.Space{
			ID:     sv.SessionID,
			Label:  spaceLabel,
			Number: len(v.layout.Spaces) + 1,
		})
		for j, w := range sv.Windows {
			v.session[w.WindowID] = sv.SessionID
			num := j + 1
			wLabel := w.Label
			if wLabel == "" {
				wLabel = strconv.Itoa(num)
			}
			v.layout.Slots = append(v.layout.Slots, host.Slot{
				ID:      w.WindowID,
				SpaceID: sv.SessionID,
				Label:   wLabel,
				Number:  num,
			})
			for _, layer := range w.Layers {
				for _, b := range layer.Blocks {
					if b.BlockID == "" || seenBlock[b.BlockID] {
						continue
					}
					seenBlock[b.BlockID] = true
					v.session[b.BlockID] = sv.SessionID
					if b.Terminal() {
						blocks = append(blocks, place{s: sv, w: w, b: b})
					}
				}
			}
		}
	}

	v.procs = make(map[string]*rexcli.Process, len(blocks))
	v.titles = make(map[string]string, len(blocks))
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, rexProcsParallel)
	)
	for _, p := range blocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(sid, bid string) {
			defer func() { <-sem; wg.Done() }()
			pr, err := h.c.Process(ctx, sid, bid)
			if err != nil {
				pr = nil
			}
			title, _ := h.c.Title(ctx, sid, bid)
			mu.Lock()
			v.procs[bid] = pr
			v.titles[bid] = title
			mu.Unlock()
		}(p.s.SessionID, p.b.BlockID)
	}
	wg.Wait()

	for _, p := range blocks {
		title := v.titles[p.b.BlockID]
		if title == "" {
			title = p.b.Label
		}
		a := rexAgent(p.s.SessionID, p.w.WindowID, p.b.BlockID, title, v.procs[p.b.BlockID])
		v.layout.Panes = append(v.layout.Panes, a)
		if a.Agent != "" {
			v.layout.Agents = append(v.layout.Agents, a)
		}
	}
	fillSpaceDirectories(v.layout.Spaces, v.layout.Panes)
	return v
}

// rexCwd picks the best working directory reported for a block: foreground's cwd, else the nearest ancestor with a cwd.
func rexCwd(p *rexcli.Process) string {
	if p == nil {
		return ""
	}
	if p.Foreground != nil && p.Foreground.Cwd != "" {
		return p.Foreground.Cwd
	}
	for _, anc := range p.Ancestors {
		if anc.Cwd != "" {
			return anc.Cwd
		}
	}
	if p.Child != nil && p.Child.Cwd != "" {
		return p.Child.Cwd
	}
	return ""
}

// rexShellOrHerdr reports whether the foreground process is a shell or classic herdr's client.
func rexShellOrHerdr(fg *rexcli.Proc) bool {
	if fg == nil {
		return true
	}
	return shellProgram(fg.Name) || shellProgram(fg.Argv0) ||
		fg.Name == "herdr" || filepath.Base(fg.Argv0) == "herdr" || filepath.Base(fg.Path) == "herdr"
}

// rexAgentKind identifies the agent running in front of a Rex block (claude, codex, traex), "" when none.
func rexAgentKind(fg *rexcli.Proc) string {
	if fg == nil || rexShellOrHerdr(fg) {
		return ""
	}
	for _, cand := range []string{fg.Argv0, fg.Path, fg.Name} {
		if cand == "" {
			continue
		}
		if k := agentOfCmd([]string{cand}); k != "" {
			return k
		}
	}
	for _, cand := range []string{fg.Argv0, fg.Path, fg.Name} {
		if cand == "" {
			continue
		}
		switch progName(cand) {
		case "node", "bun":
			if fg.Pid > 0 {
				if k := agentOfCmd(readRexProcArgv(fg.Pid)); k != "" {
					return k
				}
			}
			return ""
		}
	}
	return ""
}

// rexAgent builds a host.Agent row for one Rex terminal block.
func rexAgent(sessionID, windowID, blockID, rawTitle string, p *rexcli.Process) host.Agent {
	cwd := rexCwd(p)
	a := host.Agent{
		Cwd:         cwd,
		PaneID:      host.PaneRef{Kind: host.Rex, ID: blockID}.String(),
		WorkspaceID: sessionID,
		TabID:       windowID,
		Title:       stripSpinner(rawTitle),
		RawTitle:    rawTitle,
	}
	if p == nil || p.Foreground == nil {
		return a
	}
	a.ForegroundCwd = p.Foreground.Cwd
	if k := rexAgentKind(p.Foreground); k != "" {
		a.Agent, a.AgentStatus = k, proto.StatusUnknown
	}
	return a
}

// sessionOf returns the session ID owning a window or block ID, refreshing the view once if unknown.
func (h *rexHost) sessionOf(ctx context.Context, id string) (string, error) {
	h.mu.Lock()
	if h.view != nil {
		if sid := h.view.session[id]; sid != "" {
			h.mu.Unlock()
			return sid, nil
		}
	}
	h.mu.Unlock()
	v, err := h.look(ctx, true)
	if err != nil {
		return "", err
	}
	if sid := v.session[id]; sid != "" {
		return sid, nil
	}
	return "", host.Mark(fmt.Errorf("rex: no session for %s", id), rexcli.ErrNotFound)
}

func (h *rexHost) Snapshot(ctx context.Context) (*host.Layout, error) {
	v, err := h.look(ctx, true)
	if err != nil {
		return nil, err
	}
	return v.layout, nil
}

func (h *rexHost) Spaces(ctx context.Context) ([]host.Space, error) {
	v, err := h.look(ctx, true)
	if err != nil {
		return nil, err
	}
	return v.layout.Spaces, nil
}

func (h *rexHost) Agents(ctx context.Context) ([]host.Agent, error) {
	v, err := h.look(ctx, false)
	if err != nil {
		return nil, err
	}
	return v.layout.Agents, nil
}

// Agent reads one block's current process and title afresh.
func (h *rexHost) Agent(ctx context.Context, pane host.PaneRef) (*host.Agent, error) {
	v, err := h.look(ctx, false)
	if err != nil {
		return nil, err
	}
	for _, a := range v.layout.Panes {
		if a.PaneID == pane.String() {
			p, err := h.c.Process(ctx, a.WorkspaceID, pane.ID)
			if err != nil {
				return nil, rexErr(err)
			}
			title, err := h.c.Title(ctx, a.WorkspaceID, pane.ID)
			if err != nil || title == "" {
				title = a.RawTitle
			}
			out := rexAgent(a.WorkspaceID, a.TabID, pane.ID, title, p)
			return &out, nil
		}
	}
	return nil, fmt.Errorf("rex: no terminal block %s", pane.ID)
}

// rexSGR matches SGR escapes, to tell a row that only resets colors from one with text.
var rexSGR = lazyre.New(`\x1b\[[0-9;:]*m`)

// Read captures a block's screen (with VT escapes when o.ANSI is set), drops trailing blank rows, slices to o.Lines,
// and joins with \r\n when ANSI is requested.
func (h *rexHost) Read(ctx context.Context, pane host.PaneRef, o host.ReadOpts) (string, error) {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return "", rexErr(err)
	}
	text, err := h.c.Capture(ctx, sid, pane.ID, o.ANSI, o.Source == "recent-unwrapped")
	if err != nil {
		return "", rexErr(err)
	}
	rows := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(rows) > 0 && strings.TrimSpace(rexSGR.ReplaceAllString(rows[len(rows)-1], "")) == "" {
		rows = rows[:len(rows)-1]
	}
	if o.Lines > 0 && len(rows) > o.Lines {
		rows = rows[len(rows)-o.Lines:]
	}
	if len(rows) == 0 {
		return "", nil
	}
	eol := "\n"
	if o.ANSI {
		eol = "\r\n"
	}
	return strings.Join(rows, eol) + eol, nil
}

// SendText types text directly into a block without pressing enter.
func (h *rexHost) SendText(ctx context.Context, pane host.PaneRef, text string) error {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return rexInputErr(err)
	}
	return rexInputErr(h.c.SendText(ctx, sid, pane.ID, text))
}

// SendKeys presses keys in a block using Persona's key names.
func (h *rexHost) SendKeys(ctx context.Context, pane host.PaneRef, keys ...string) error {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return rexInputErr(err)
	}
	return rexInputErr(h.c.SendKeys(ctx, sid, pane.ID, keys...))
}

// front verifies an agent is running in front of pane before pasting or pressing enter.
func (h *rexHost) front(ctx context.Context, sid string, pane host.PaneRef) error {
	p, err := h.c.Process(ctx, sid, pane.ID)
	if err != nil {
		return rexInputErr(err)
	}
	if rexAgentKind(p.Foreground) == "" {
		return host.Mark(fmt.Errorf("rex: no agent runs in front of block %s", pane.ID), host.ErrNotDelivered)
	}
	return nil
}

// Prompt pastes text into the block and submits with enter after a short pause, verifying an agent is in front before
// each step.
func (h *rexHost) Prompt(ctx context.Context, pane host.PaneRef, text string) error {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return rexInputErr(err)
	}
	if err := h.front(ctx, sid, pane); err != nil {
		return err
	}
	if err := h.c.Paste(ctx, sid, pane.ID, strings.ReplaceAll(text, "\x1b", inertEscape)); err != nil {
		return rexInputErr(err)
	}
	afterPaste := func(err error) error {
		return fmt.Errorf("rex: the text was pasted into block %s but not submitted: %v", pane.ID, err)
	}
	select {
	case <-ctx.Done():
		return afterPaste(ctx.Err())
	case <-time.After(orcaPasteGap(text)):
	}
	if err := h.front(ctx, sid, pane); err != nil {
		return afterPaste(err)
	}
	if err := h.c.SendKeys(ctx, sid, pane.ID, "enter"); err != nil {
		return afterPaste(err)
	}
	return nil
}

// PaneProcesses returns the foreground PID first (Claude's sessions/<pid>.json), followed by ancestors and child PIDs.
func (h *rexHost) PaneProcesses(ctx context.Context, pane host.PaneRef) ([]int, error) {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return nil, rexErr(err)
	}
	p, err := h.c.Process(ctx, sid, pane.ID)
	if err != nil {
		return nil, rexErr(err)
	}
	var pids []int
	add := func(pid int) {
		if pid <= 0 {
			return
		}
		for _, existing := range pids {
			if existing == pid {
				return
			}
		}
		pids = append(pids, pid)
	}
	if p.Foreground != nil {
		add(p.Foreground.Pid)
	}
	for _, anc := range p.Ancestors {
		add(anc.Pid)
	}
	if p.Child != nil {
		add(p.Child.Pid)
	}
	return pids, nil
}

// PaneForeground names the program in front of a block.
func (h *rexHost) PaneForeground(ctx context.Context, pane host.PaneRef) ([]string, error) {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return nil, rexErr(err)
	}
	p, err := h.c.Process(ctx, sid, pane.ID)
	if err != nil {
		return nil, rexErr(err)
	}
	if p.Foreground == nil {
		return nil, nil
	}
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		for _, x := range out {
			if x == s {
				return
			}
		}
		out = append(out, s)
	}
	add(p.Foreground.Name)
	add(p.Foreground.Argv0)
	return out, nil
}

// Watch streams server events from `rex events --all --json` (block events come only with --all) and reports those
// that can change what the node lists (rexEventChange).
func (h *rexHost) Watch(ctx context.Context, _ []host.PaneRef, changed func(urgent bool)) error {
	return rexErr(h.c.Events(ctx, func() { changed(false) }, func(ev rexcli.Event) {
		if refresh, urgent := rexEventChange(ev); refresh {
			changed(urgent)
		}
	}))
}

// rexBlockEvents are the terminal block events that can change what the node lists: the program in front starting or
// exiting, the title an agent sets, its directory, and a bell or desktop notification (urgent: the agent may be asking
// for the user). Size, selection, clipboard, port and progress events change nothing listed.
var rexBlockEvents = map[string]bool{
	"process_changed": true, "child_exited": true, "title_changed": true, "pwd_changed": true,
	"bell": true, "desktop_notification": true,
}

// rexEventChange tells whether a Rex event calls for a new look, and whether it is urgent. Every rex command is a
// client of its own, so the host's own calls raise "client_*" server events and "client_connected" /
// "client_disconnected" session events; those, and block ownership following a client, are left out, or each look
// would set off the next one.
func rexEventChange(ev rexcli.Event) (refresh, urgent bool) {
	switch {
	case strings.HasPrefix(ev.Type, "client_"), ev.Type == "tailscale_status_changed":
		return false, false
	case ev.Type != "session_event":
		return true, false // sessions created, destroyed, moved
	case strings.HasPrefix(ev.Kind, "client_"), ev.Kind == "block_owner_changed":
		return false, false
	case ev.Kind == "block_event":
		if !rexBlockEvents[ev.Name] {
			return false, false
		}
		return true, ev.Name == "bell" || ev.Name == "desktop_notification"
	}
	return true, false // windows, blocks, layouts and labels
}

// CreateSpace opens a Rex session named label (the directory's basename when empty) with one shell window in cwd.
func (h *rexHost) CreateSpace(ctx context.Context, cwd, label string) (*host.Created, error) {
	if label == "" && cwd != "" {
		label = filepath.Base(cwd)
	}
	cr, err := h.c.CreateSession(ctx, label, cwd)
	if err != nil {
		return nil, rexErr(err)
	}
	h.remember(cr.SessionID, cr.WindowID, cr.BlockID)
	return &host.Created{
		SpaceID: cr.SessionID,
		SlotID:  cr.WindowID,
		Pane:    host.PaneRef{Kind: host.Rex, ID: cr.BlockID},
	}, nil
}

// CreateSlot opens a new window in space (session) running a shell in cwd, without stealing focus.
func (h *rexHost) CreateSlot(ctx context.Context, space, cwd, label string) (*host.Created, error) {
	cr, err := h.c.NewWindow(ctx, space, label, cwd, false)
	if err != nil {
		return nil, rexErr(err)
	}
	h.remember(space, cr.WindowID, cr.BlockID)
	return &host.Created{
		SpaceID: space,
		SlotID:  cr.WindowID,
		Pane:    host.PaneRef{Kind: host.Rex, ID: cr.BlockID},
	}, nil
}

// remember records a newly created window/block in the cached session map so immediate follow-up calls (Run, Read)
// do not need an extra Snapshot round-trip.
func (h *rexHost) remember(sessionID, windowID, blockID string) {
	if sessionID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.view == nil {
		h.view = &rexView{layout: &host.Layout{}, session: map[string]string{}, views: map[string]*rexcli.SessionView{}}
	}
	if windowID != "" {
		h.view.session[windowID] = sessionID
	}
	if blockID != "" {
		h.view.session[blockID] = sessionID
	}
}

// Run types command into a block and presses enter.
func (h *rexHost) Run(ctx context.Context, pane host.PaneRef, command string) error {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return rexInputErr(err)
	}
	if err := h.c.SendText(ctx, sid, pane.ID, command); err != nil {
		return rexInputErr(err)
	}
	return rexInputErr(h.c.SendKeys(ctx, sid, pane.ID, "enter"))
}

// NameAgent: Rex shows the title the agent sets itself; the window keeps the label it was opened with.
func (h *rexHost) NameAgent(context.Context, host.PaneRef, string) error { return nil }

// RenameSpace renames a Rex session.
func (h *rexHost) RenameSpace(ctx context.Context, space, name string) error {
	return rexErr(h.c.SetSessionLabel(ctx, space, name))
}

// RenameSlot renames a Rex window (tab).
func (h *rexHost) RenameSlot(ctx context.Context, slot, name string) error {
	sid, err := h.sessionOf(ctx, slot)
	if err != nil {
		return rexErr(err)
	}
	return rexErr(h.c.SetWindowLabel(ctx, sid, slot, name))
}

// Close closes a block. If the session is left empty and has no clients attached, it is destroyed so Persona does not
// leave orphaned empty sessions behind. A block already gone counts as closed.
func (h *rexHost) Close(ctx context.Context, pane host.PaneRef, space, _ string) error {
	sid := space
	if sid == "" {
		var err error
		sid, err = h.sessionOf(ctx, pane.ID)
		if err != nil {
			if errors.Is(err, rexcli.ErrNotFound) {
				return nil
			}
			return rexErr(err)
		}
	}
	if err := h.c.CloseBlock(ctx, sid, pane.ID); err != nil && !errors.Is(err, rexcli.ErrNotFound) {
		return rexErr(err)
	}
	if sid == "" {
		return nil
	}
	sv, err := h.c.SessionView(ctx, sid)
	if err != nil || !rexSessionEmpty(sv) {
		return nil
	}
	clients, err := h.c.ListClients(ctx, sid)
	if err == nil && clients == 0 {
		if err := h.c.DestroySession(ctx, sid); err != nil && !errors.Is(err, rexcli.ErrNotFound) {
			h.logf("rex: destroying empty session %s: %v", sid, err)
		}
	}
	return nil
}

// CloseSlot closes a whole window (tab) and every block in it.
func (h *rexHost) CloseSlot(ctx context.Context, slot string) error {
	sid, err := h.sessionOf(ctx, slot)
	if err != nil {
		if errors.Is(err, rexcli.ErrNotFound) {
			return nil
		}
		return rexErr(err)
	}
	if err := h.c.CloseWindow(ctx, sid, slot); err != nil && !errors.Is(err, rexcli.ErrNotFound) {
		return rexErr(err)
	}
	return nil
}

// Split splits pane side by side ("right") or stacked ("down"), keeping focus unchanged.
func (h *rexHost) Split(ctx context.Context, pane host.PaneRef, direction, cwd string) (host.PaneRef, error) {
	sid, err := h.sessionOf(ctx, pane.ID)
	if err != nil {
		return host.PaneRef{}, rexErr(err)
	}
	dir := "horizontal"
	if direction == "down" {
		dir = "vertical"
	}
	bid, err := h.c.NewSplit(ctx, sid, pane.ID, dir, cwd, false)
	if err != nil {
		return host.PaneRef{}, rexErr(err)
	}
	h.remember(sid, "", bid)
	return host.PaneRef{Kind: host.Rex, ID: bid}, nil
}

func rexSessionEmpty(sv *rexcli.SessionView) bool {
	if sv == nil {
		return true
	}
	for _, w := range sv.Windows {
		for _, l := range w.Layers {
			if len(l.Blocks) > 0 {
				return false
			}
		}
	}
	return true
}

func (h *rexHost) logf(format string, args ...any) {
	if h.c.Log != nil {
		h.c.Log(format, args...)
	}
}
