// Package host is what the node needs from a terminal host — where the terminals run (herdr, later Orca): its panes,
// reading their screens, typing into them, opening and closing them. What runs inside a pane (Claude Code, Codex,
// a compatible CLI) is the agent; telling agents apart, finding their sessions and judging their state stay in the node, on top
// of any host (docs/plans/plan-orca-support.md).
//
// A host implements Host; everything else is an optional capability the node finds with a type assertion.
package host

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// Host kinds. Herdr panes are stored and sent without a prefix (see PaneRef).
const (
	Herdr = "herdr"
	// Orca is the Orca desktop app's runtime; its pane ids are Orca's pane keys "<tabId>:<leafId>".
	Orca = "orca"
	// HerdrCore is herdr-core (hc), the runtime behind the Herdr desktop app: not classic herdr, a host of its own.
	HerdrCore = "herdr-core"
	// Botmux is botmux, the Feishu topic ↔ AI CLI bridge: a pane is a botmux session, its id the session id.
	Botmux = "botmux"
	// Tern is Tern, Stencil's multiplexing terminal: a pane is a Tern block, its id the block's number (what Tern puts in
	// the block's processes as TERN_PANE).
	Tern = "tern"
	// Rex is Rex, a multiplexing terminal: a pane is a terminal block, its id the block's full ID ("block:<uuid>", what
	// Rex puts in the block's processes as REX_BLOCK).
	Rex = "rex"
	// Tmux is a tmux server the user opened: a pane is tmux's own id ("%11"), read and typed into through the tmux
	// command. No space, slot or agent comes from it: a session running there is listed by claudebg.go (#2562).
	Tmux = "tmux"
)

// Kinds are the host kinds a stored pane may be prefixed with; the list is fixed, not what a machine registers, so
// a stored pane means the same everywhere.
var Kinds = []string{Herdr, Orca, HerdrCore, Botmux, Tern, Rex, Tmux}

// Host is what every terminal host provides. Pane ids in what it returns (Agent.PaneID, Created.Pane) are PaneRefs
// of its own kind; the PaneRefs it is given are always of its kind.
type Host interface {
	// Kind names the host ("herdr"); it is the prefix of its PaneRefs.
	Kind() string
	// Version is the host's version, "" when unknown or not running.
	Version(ctx context.Context) string
	// Snapshot is the whole layout at once: spaces, slots, the panes running an agent the host detects, every pane.
	Snapshot(ctx context.Context) (*Layout, error)
	// Agents lists only the panes running an agent the host detects (a lighter call than Snapshot; herdr: hosts that
	// cannot snapshot).
	Agents(ctx context.Context) ([]Agent, error)
	// Agent is one pane as an agent pane (its agent and state, which the host may not know: Agent "", or ErrNoAgent
	// from a host that answers with an error, herdr).
	Agent(ctx context.Context, pane PaneRef) (*Agent, error)
	// Read reads a pane's screen.
	Read(ctx context.Context, pane PaneRef, opts ReadOpts) (string, error)
	// SendText types text into a pane as is (no enter).
	SendText(ctx context.Context, pane PaneRef, text string) error
	// SendKeys presses keys in a pane, by Persona's key names (herdr's: "enter", "esc", "shift+tab", "ctrl+u", "up"…);
	// each host translates them.
	SendKeys(ctx context.Context, pane PaneRef, keys ...string) error
	// Prompt submits text to the agent in a pane the way the host does it best (herdr: agent.prompt, a paste and a
	// submit). ErrNoAgent (errors.Is) when the host does not know an agent there: the caller types it instead.
	Prompt(ctx context.Context, pane PaneRef, text string) error
	// PaneProcesses are the pids of a pane's foreground processes, claude first.
	PaneProcesses(ctx context.Context, pane PaneRef) ([]int, error)
	// PaneEnv is the environment variable that names the pane in the processes running in it ("HERDR_PANE_ID"): its
	// value is the host's pane id (PaneRef.ID) — for classic herdr, in its own server; a named session's panes are
	// qualified with the session the process's HERDR_SESSION names (herdr.PaneOf, node envPane) first.
	PaneEnv() string
}

// Watcher pushes changes: the node takes a snapshot when one comes instead of polling every second.
type Watcher interface {
	// Watch calls changed for every change until ctx ends or the host ends the stream, and returns why it stopped.
	// panes are the agent panes whose state changes to report (a different set needs a new Watch); the first call of
	// changed means the stream is live (snapshot then, to cover the gap before it). urgent: an agent started or
	// stopped waiting for the user, so a card comes or goes and the change skips the list's pacing (#751).
	Watch(ctx context.Context, panes []PaneRef, changed func(urgent bool)) error
}

// PlainScreen is a host whose screens may come without colors: when PlainScreen is true, Read with ReadOpts.ANSI
// fails (ErrUnsupported) and the node judges a screen by its text alone (the dialog check before a send, no draft
// check: a draft and Claude's grey placeholder look the same without colors). A host without it keeps colors.
type PlainScreen interface {
	PlainScreen() bool
}

// ScreenDriver is a host that may not drive its panes' terminals at all: when CanDriveScreen is false, Read, SendText
// and SendKeys fail (ErrUnsupported) and only Prompt reaches the agent, as a message of its own (botmux). The node
// then offers nothing that has to read a screen back or type a command (ultracode). A host without it can.
type ScreenDriver interface {
	CanDriveScreen() bool
}

// TextTyper is a host that may not type text without submitting it: when CanTypeText is false, SendText fails
// (ErrUnsupported) and the node submits text followed by enter with Prompt instead, and refuses other typing before
// pressing anything. A host without it can type.
type TextTyper interface {
	CanTypeText() bool
}

// ForegroundNamer is a host that can name the programs a pane runs in front (herdr: pane.process_info), a login
// shell's with its leading "-". The node types a message into a pane whose agent the host does not know only while
// one of them is not a shell: once the agent exits, the shell would run it as a command (#1185).
type ForegroundNamer interface {
	PaneForeground(ctx context.Context, pane PaneRef) ([]string, error)
}

// SpaceLister lists spaces without requiring an agent or permission to launch one.
type SpaceLister interface {
	Spaces(ctx context.Context) ([]Space, error)
}

// Launcher opens new panes running a command.
type Launcher interface {
	// Spaces lists the spaces (herdr: workspaces) new panes can go into.
	Spaces(ctx context.Context) ([]Space, error)
	// CreateSpace opens a new space in cwd, with one slot holding one pane.
	CreateSpace(ctx context.Context, cwd, label string) (*Created, error)
	// CreateSlot opens a new slot (herdr: tab) with one pane in a space.
	CreateSlot(ctx context.Context, space, cwd, label string) (*Created, error)
	// Run types command into a pane and presses enter.
	Run(ctx context.Context, pane PaneRef, command string) error
	// NameAgent names the agent running in a pane (shown by the host).
	NameAgent(ctx context.Context, pane PaneRef, name string) error
}

// EnvLauncher is a Launcher that can start the new pane's shell with extra environment ("KEY=VALUE"): herdr's
// workspace.create / tab.create env, the Herdr App's terminal.run env. The node opens an Agent's terminal this way so
// the shell's own startup files can tell it from one the user opened (PERSONA_AGENT, #10111) and skip the questions
// that would stop the start (#10365).
type EnvLauncher interface {
	CreateSpaceEnv(ctx context.Context, cwd, label string, env []string) (*Created, error)
	CreateSlotEnv(ctx context.Context, space, cwd, label string, env []string) (*Created, error)
}

// Organizer renames and closes.
type Organizer interface {
	RenameSpace(ctx context.Context, space, name string) error
	RenameSlot(ctx context.Context, slot, name string) error
	// Close closes a pane in the given space / slot: the whole slot when the pane is its only one.
	Close(ctx context.Context, pane PaneRef, space, slot string) error
}

// SlotCloser closes a whole slot (herdr: a tab) with every pane in it, whatever runs there.
type SlotCloser interface {
	CloseSlot(ctx context.Context, slot string) error
}

// Splitter splits a pane in two; direction is "right" (side by side) or "down" (stacked). The new pane starts in cwd
// (the host's choice when empty) and takes no focus. It returns the new pane.
type Splitter interface {
	Split(ctx context.Context, pane PaneRef, direction, cwd string) (PaneRef, error)
}

// Worktreer is a host that makes Git worktree checkouts itself and opens them as spaces grouped under the repository's
// (herdr: `worktree create`). A host without it, or one that says CanWorktree is false (an older herdr), gets the
// checkout from git and a plain CreateSpace.
type Worktreer interface {
	CanWorktree() bool
	// WorktreesDir is the directory the host puts checkouts under (`<dir>/<repo>/<branch-slug>`); home is the user's.
	WorktreesDir(home string) string
	// CreateWorktree checks branch out at path (an existing local branch, else a new one from the space's HEAD) and opens
	// it as a space, taking no focus. It never forces anything.
	CreateWorktree(ctx context.Context, space, branch, path string) (*Created, error)
}

// Streamer streams a pane's terminal as it changes, at the pane's own size, without resizing it.
type Streamer interface {
	// CanObserve reports whether this host can stream (herdr: a version with `terminal session observe`).
	CanObserve() bool
	// PaneSize is the grid the pane's programs see.
	PaneSize(ctx context.Context, pane PaneRef) (cols, rows int, err error)
	// Observe streams frames to fn until ctx ends or the host stops, and returns why.
	Observe(ctx context.Context, pane PaneRef, cols, rows int, fn func(Frame)) error
}

// Controller lets a client drive a pane's terminal at its own size (input, resize, scroll, click).
type Controller interface {
	CanControl() bool
	// Control takes the pane's terminal at cols × rows, streaming frames to fn. ErrAttachBusy (errors.Is) when another
	// client holds it and takeover is false.
	Control(ctx context.Context, pane PaneRef, cols, rows int, takeover bool, fn func(Frame)) (Control, error)
}

// FullClienter runs the host's own interactive client in a terminal of the node's, for the web's full herdr view: the
// host's sidebar, tabs and splits instead of one pane (docs/integrations/terminal.md "Full herdr UI").
type FullClienter interface {
	// CanFullClient reports whether this host can, on this machine (herdr on Unix, with sessions to attach to).
	CanFullClient() bool
	// StartFullClient runs the client at cols × rows and gives what it draws to onData, in order, from one goroutine;
	// onData may block, which stops the client drawing (flow control), and must not keep the slice. With a pane, the
	// host first shows that pane's tab (the host's focus is shared by all its clients).
	StartFullClient(ctx context.Context, opts FullClientOptions, onData func([]byte)) (FullClient, error)
}

// FullClientOptions says how to run a full client.
type FullClientOptions struct {
	Cols, Rows int
	Pane       *PaneRef
}

// FullClient is a running full client (FullClienter.StartFullClient).
type FullClient interface {
	// Write types into the client.
	Write(p []byte) error
	Resize(cols, rows int) error
	// Done closes when the client is gone; Err then says why (nil after Close).
	Done() <-chan struct{}
	Err() error
	// Close ends the client and waits for it to be gone.
	Close()
}

// FocusState is what a host shows now: its focused space, slot and pane (shared by every client of the host).
type FocusState struct {
	Space, Slot, Pane string
}

// Focuser moves and reports the focus a host shares among its clients (herdr: pane.focus, and the focus events with
// the snapshot's focused_* ids). The web's full herdr view follows it and moves it (docs/integrations/terminal.md "Focus synchronization").
type Focuser interface {
	// Focused is what the host shows now; Pane is empty when the host does not say.
	Focused(ctx context.Context) (FocusState, error)
	// WatchFocus calls changed when the stream starts and for each focus event, until ctx ends or the host ends the
	// stream, and returns why it stopped. The events say nothing about who moved the focus.
	WatchFocus(ctx context.Context, changed func()) error
	// FocusPane makes the pane the focused one (its slot and space too).
	FocusPane(ctx context.Context, pane PaneRef) error
}

// MetadataReporter reports and clears pane metadata tokens (docs/assistant/todos.md, #762).
type MetadataReporter interface {
	ReportMetadata(ctx context.Context, pane PaneRef, source string, tokens map[string]*string) error
}

// Control is a terminal being driven (Controller.Control).
type Control interface {
	Input(text string) error
	Resize(cols, rows int) error
	Scroll(lines int, up bool, column, row int) error
	Click(column, row int) error
	// Done closes when the control ended (Close, the host ending it); Err then says why (ErrTakenOver: someone took it).
	Done() <-chan struct{}
	Err() error
	Close()
}

// MouseEvent is one event of the pointer on a driven terminal: Action down | up | drag (moving with the button held) |
// move (no button), Button left | middle | right, a cell of the screen (0-based) and the modifier bits (shift 1, ctrl 2,
// alt 4). The host gives it to a program that takes the mouse and drops it for one that does not.
type MouseEvent struct {
	Action    string
	Button    string
	Column    int
	Row       int
	Modifiers int
}

// Mouser is a Control that takes mouse events beyond Click (herdr: terminal.mouse; docs/integrations/terminal.md "Mouse").
type Mouser interface {
	Mouse(MouseEvent) error
}

// Releaser is a Control that can end at a size: the pane resized to cols × rows, then given back (herdr: CloseAt). herdr
// lays a released pane out at its own client's size only while one is attached, so the node gives back the size the
// pane had before it took it (docs/integrations/terminal.md section 11).
type Releaser interface {
	CloseAt(cols, rows int)
}

// Redrawer is a Control that can be asked for a whole frame of the screen as it is, the programs seeing no change
// (herdr: a resize to the size it has). The node asks when a slow link falls behind (node/terminal.go frameGate).
type Redrawer interface {
	Redraw() error
}

// Errors a host marks its own errors with (errors.Is); the error text stays the host's.
var (
	// ErrNoAgent: Prompt or Agent found no agent the host knows in the pane.
	ErrNoAgent = errors.New("no agent in the pane")
	// ErrAgentBlocked: Prompt refused, nothing typed, because the host takes the agent to be waiting on the user (herdr
	// says so of a Codex with an async question open, whose input box still takes a message).
	ErrAgentBlocked = errors.New("agent blocked")
	// ErrAttachBusy: another client holds the terminal (Controller.Control without takeover).
	ErrAttachBusy = errors.New("attach_busy")
	// ErrTakenOver: the terminal being driven was taken by another client.
	ErrTakenOver = errors.New("taken_over")
	// ErrNotRunning: the host is not running on this machine (no socket): it has no panes, as opposed to panes it
	// cannot be asked about right now.
	ErrNotRunning = errors.New("not running")
	// ErrNotDelivered: the host refused the input, nothing was typed (safe to send again).
	ErrNotDelivered = errors.New("not delivered")
	// ErrDialogOpen: the host refused the input because the program says a dialog has the keyboard (with
	// ErrNotDelivered).
	ErrDialogOpen = errors.New("dialog open")
	// ErrUnsupported: the host cannot do this (a capability it lacks). Errors carrying it start with "unsupported",
	// the prefix the hub falls back on.
	ErrUnsupported = errors.New("unsupported")
)

// Mark makes err also match kind with errors.Is, keeping its text and what it already matches.
func Mark(err, kind error) error {
	if err == nil {
		return nil
	}
	return marked{err, kind}
}

type marked struct{ err, kind error }

func (m marked) Error() string   { return m.err.Error() }
func (m marked) Unwrap() []error { return []error{m.err, m.kind} }

// ReadOpts says what of a pane's screen to read.
type ReadOpts struct {
	// Source: "visible" (the screen), "recent" (back into the scrollback, wrapped as shown) or "recent-unwrapped".
	Source string
	// Lines: at most this many lines, from the end.
	Lines int
	// ANSI keeps colors and styles (SGR escapes; rows end in \r\n).
	ANSI bool
}

// AgentSession is what an agent integration in the host reported about the pane's session. Source identifies the
// reporting integration and Agent names the agent. Kind "id" carries the session ID in Value. Kind "path" carries
// the session's file, as herdr's pi integration reports it.
type AgentSession struct {
	Source string
	Agent  string
	Kind   string
	Value  string
}

// Agent is one pane as the node sees it: where it is, and the agent running in it with its state and session when the
// host (or the node) knows them.
type Agent struct {
	// Agent is the kind of agent: "claude", "codex", "traex"…; "" when nothing the host detects runs there.
	Agent        string
	AgentSession *AgentSession
	// Name is the agent's name in the host (NameAgent).
	Name string
	// AgentStatus is the host's state for the agent (proto.Status*), "" when it has none.
	AgentStatus   string
	Cwd           string
	ForegroundCwd string
	// PaneID is the pane as a PaneRef string (String): a herdr pane id as is ("@<session>:<id>" for a named herdr
	// session's, herdr.QualifyID).
	PaneID string
	// ParentPaneID is the pane that started this one as a PaneRef string (String), "" when the host says it belongs to
	// none. A host that knows its panes' parent (herdr-core's terminal.parent) reports it: the node draws the session
	// under its parent's while both are listed (linkHerdrChildren).
	ParentPaneID string
	// TabID, WorkspaceID: the host's slot and space ids.
	TabID       string
	WorkspaceID string
	// Title is the terminal title without escapes; RawTitle as set (with the spinner a busy TUI puts in front).
	Title    string
	RawTitle string
	// Tokens are metadata tokens reported by herdr (#762).
	Tokens map[string]string
	// Attention is why the host says the agent waits for the user (botmux's attention reason), "" when it says nothing;
	// AttentionAt when it said so, and TurnStatus the agent's state from its turn alone (what AgentStatus is once the
	// agent took a message after the attention: the attention is out of date).
	Attention   string
	AttentionAt time.Time
	TurnStatus  string
	// Failure is the host's notice that the agent's turn failed and it stopped there (botmux: recovery exhausted, not
	// safe to continue on its own), FailureAt when; nothing the user answers, so not a wait: AgentStatus is the turn's.
	Failure   string
	FailureAt time.Time
	// ExternalURL opens the agent where the host shows it (a botmux session's Feishu topic), "" when there is none.
	ExternalURL string
	// Chat and Topic are the Feishu chat and the topic's root message a botmux session answers in; Dispatched the
	// topics it opened for other bots with `botmux dispatch` (its children's Topic). "" and nil elsewhere.
	Chat, Topic string
	Dispatched  []string
	// LastText is the agent's last answer as the host recorded it (the last reply botmux posted in the topic), "" when
	// the host keeps none: the transcript's last words then.
	LastText string
	// LastTextAt is when the host recorded LastText, zero when it does not say.
	LastTextAt time.Time
}

// SessionID is the session id the host reported, "" when none. A reported file names its session: pi's are
// <time>_<session id>.jsonl, so the id is what follows the last "_" of the file's name (the whole name without one).
func (a Agent) SessionID() string {
	if a.AgentSession == nil {
		return ""
	}
	switch a.AgentSession.Kind {
	case "id":
		return a.AgentSession.Value
	case "path":
		name := strings.TrimSuffix(filepath.Base(a.AgentSession.Value), filepath.Ext(a.AgentSession.Value))
		if i := strings.LastIndexByte(name, '_'); i >= 0 {
			name = name[i+1:]
		}
		if name == "." || name == "/" {
			return ""
		}
		return name
	}
	return ""
}

// SessionPath is the session file the host reported (Kind "path"), "" when it reported none.
func (a Agent) SessionPath() string {
	if a.AgentSession != nil && a.AgentSession.Kind == "path" {
		return a.AgentSession.Value
	}
	return ""
}

// Space is a group of slots (herdr: a workspace).
type Space struct {
	ID     string
	Label  string
	Number int
	// Cwd and Branch come from the host; empty means the host did not report them.
	Cwd    string
	Branch string
	// ParentID is the space this one is nested under in the host (Orca: a child worktree's parent); empty at the top.
	ParentID string
	// DisplayName is the host's user-visible title; Label retains its existing session/pin identity.
	DisplayName string
	// Project placement is separate from workspace lineage: several roots can share one project.
	ProjectID, ProjectName, ProjectPath string
	IsMain                              *bool
	Unread                              *bool
	SidebarStatus, WorkspaceKind        string
	Head                                string
}

// Slot holds panes inside a space (herdr: a tab).
type Slot struct {
	ID      string
	SpaceID string
	Label   string
	Number  int
}

// Layout is a host's whole state at one moment.
type Layout struct {
	Spaces []Space
	Slots  []Slot
	// Agents are the panes running an agent the host detects.
	Agents []Agent
	// Panes is every pane, agent or not (Agent ""): where agents the host does not detect (a compatible CLI) are found.
	Panes []Agent
	// PaneIndex is each pane's 0-based place in its slot by layout, top left first (herdr: its split tree), keyed by
	// PaneID; nil when the host does not say where its panes sit (docs/protocol/agent-states.md "Several panes in one tab").
	PaneIndex map[string]int
}

// Created is what a Launcher opened.
type Created struct {
	SpaceID string // set by CreateSpace
	SlotID  string
	Pane    PaneRef
	// Shell is the shell actually opened by the host, when reported by its launch API.
	Shell string
	// Path and Branch: the checkout CreateWorktree made, when the host chose them rather than taking the asked ones
	// (Orca puts it under its own workspace directory); empty when they are the asked ones.
	Path, Branch string
}

// Frame is one frame of a terminal stream: the grid redrawn as VT sequences — the whole screen when Full, else only
// what changed. Data is base64.
type Frame struct {
	Seq    int64
	Full   bool
	Width  int
	Height int
	Data   string
}
