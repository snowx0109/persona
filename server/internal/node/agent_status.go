package node

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/screen"
)

// agent is the live agent of session id, with its status as liveStatus says.
func (n *Node) agent(id string) (host.Agent, bool) {
	n.mu.Lock()
	a, ok := n.agents[id]
	n.mu.Unlock()
	if ok {
		a.AgentStatus = n.liveStatus(id, a, time.Now())
	}
	return a, ok
}

// liveStatus is an agent's status with its own signs of work over its host's: a Claude's as claudeStatus says, a
// Codex's as codexStatus says; any other agent's is its host's.
func (n *Node) liveStatus(id string, a host.Agent, now time.Time) string {
	switch a.Agent {
	case "claude":
		return n.claudeStatus(id, a, now)
	case "codex":
		return n.codexStatus(id, a)
	case cursorAgent, agyAgent:
		return n.workAgentStatus(a, now)
	}
	return a.AgentStatus
}

// codexStatus is a Codex's status with its own signs of a running turn over its host's (#4865). herdr's word for a
// Codex can be wrong for a whole turn: a reporter holding the pane decides it instead of the screen (#4657), and herdr
// said idle through a turn of a Codex joined on its app-server while its chat page showed the turn's live words. When
// the host says done, idle or does not know, a Codex is working while
//
//   - the app-server the node is connected to runs a turn in its thread (codex_remote.go): turn/started, the reply to
//     the node's own turn/start, or the status its read every 5 seconds gives; or
//   - its terminal title shows the spinner Codex animates there while a turn runs (codexSpinnerTitle).
//
// blocked and working stay the host's, and so does a botmux session's whole state (botmux reports it). The list
// still corrects a working whose rollout says the turn ended a while ago (codexTurnOver).
func (n *Node) codexStatus(id string, a host.Agent) string {
	if a.Agent != "codex" || host.ParsePane(a.PaneID).Kind == host.Botmux ||
		a.AgentStatus != proto.StatusDone && a.AgentStatus != proto.StatusIdle && a.AgentStatus != proto.StatusUnknown {
		return a.AgentStatus
	}
	if _, t, err := n.codexOwner(id); err == nil && t.status == proto.StatusWorking {
		return proto.StatusWorking
	}
	if codexSpinnerTitle(a.RawTitle) {
		return proto.StatusWorking
	}
	return a.AgentStatus
}

// codexBlockedStaleGrace: how long after herdr reports blocked a rollout's advancing turn activity must continue before
// herdr's blocked status is treated as stale.
const codexBlockedStaleGrace = 10 * time.Second

// codexFreshWorking: how recently a Codex rollout must have had turn activity to override a stale host blocked status.
const codexFreshWorking = 45 * time.Second

// noteBlockedAt records when session id entered StatusBlocked (or clears it once it leaves), returning the entry time.
func (n *Node) noteBlockedAt(id string, blocked bool, now time.Time) time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !blocked {
		delete(n.blockedAt, id)
		return time.Time{}
	}
	if n.blockedAt == nil {
		n.blockedAt = map[string]time.Time{}
	}
	since, ok := n.blockedAt[id]
	if !ok {
		since = now
		n.blockedAt[id] = now
	}
	return since
}

// codexRolloutWorkingOverBlocked reports whether a Codex session whose host reports StatusBlocked is provably running
// a turn in its rollout: an open turn in the current tail with fresh turn-scoped activity advancing well after
// blocked began, neither its app-server thread, screen approval, nor an open blocking rollout ask is waiting, and a
// fresh screen read confirms no approval or terminal dialog is visible.
func (n *Node) codexRolloutWorkingOverBlocked(ctx context.Context, id, pane, path string, m quickMeta, blockedSince, now time.Time) bool {
	if blockedSince.IsZero() || !m.turnOpen || m.turnClosed {
		return false
	}
	latest := m.turnActivityAt
	if latest.IsZero() || !latest.After(blockedSince.Add(codexBlockedStaleGrace)) || now.Sub(latest) >= codexFreshWorking {
		return false
	}
	if _, t, err := n.codexOwner(id); err == nil && t.status == proto.StatusBlocked {
		return false
	}
	if n.cx.hasApproval(id) || n.cx.hasBlockingAsk(id, path) {
		return false
	}
	if pane == "" || n.hosts == nil {
		return false
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	text, err := n.dialogScreen(rctx, pane)
	if err != nil {
		return false
	}
	if d := screen.ReadCodex(text); d.Approval != nil || d.Question != nil || d.View == screen.CodexTranscript {
		if d.Approval != nil {
			n.cx.setApproval(id, true)
		}
		return false
	}
	return screen.ParseDialog(text) == nil && !screen.Choosing(text)
}

// codexSpinner: the frames of the spinner Codex animates in its terminal title while a turn runs, as herdr's own Codex
// detection reads them (its osc_title_working rule).
const codexSpinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

// codexSpinnerTitle: a word of the title is one spinner frame ("⠦ fix the build | repo"). A title asking for the
// user (Codex's "Action Required" prefix) is not a running turn, whatever else it shows.
func codexSpinnerTitle(title string) bool {
	if strings.Contains(title, "Action Required") {
		return false
	}
	for _, w := range strings.Fields(title) {
		if r, size := utf8.DecodeRuneInString(w); size == len(w) && strings.ContainsRune(codexSpinner, r) {
			return true
		}
	}
	return false
}

// claudeFresh: how recently a Claude transcript with an open turn must have been written to count as at work.
const claudeFresh = 45 * time.Second

// claudeStatus is a Claude's status with its own signs of work over its host's (#2069: herdr 0.9.3 said done through a
// whole turn started from the phone, so the app showed it as finished while it searched code). When the host says
// done, idle or does not know:
//
//   - a session that runs Persona's hooks is judged by their turn counters first (judgeHookTurn): a prompt no Stop has
//     answered is working, a Stop with no prompt after it is not;
//   - otherwise, or while the hooks have said nothing yet, it is working while its terminal title shows Claude Code's
//     spinner, or its transcript has a turn started and not finished (no turn_duration, no interrupt) written to
//     within claudeFresh. A transcript whose turn has finished keeps the host's word: a title read just before Claude
//     redrew it says nothing then.
//
// Tern reports no state at all: there the Claude's own word (ternClaudeStatus) comes before both.
//
// Hooks whose claude process is gone (or whose pid now belongs to another program) are forgotten.
func (n *Node) claudeStatus(id string, a host.Agent, now time.Time) string {
	if a.Agent != "claude" || a.AgentStatus != proto.StatusDone && a.AgentStatus != proto.StatusIdle && a.AgentStatus != proto.StatusUnknown {
		return a.AgentStatus
	}
	var m quickMeta
	var mod time.Time
	hasFile := false
	if p := n.transcriptPath(id); p != "" {
		if st, err := os.Stat(p); err == nil {
			m, mod, hasFile = n.meta(p, st), st.ModTime(), true
		}
	}
	if a.AgentStatus == proto.StatusUnknown {
		if k := host.ParsePane(a.PaneID).Kind; k == host.Tern || k == host.Rex {
			if s := n.ternClaudeStatus(id, a.PaneID, hasFile); s != "" {
				return s
			}
		}
	}
	// When the transcript last said something: its newest timed line, not its mtime (#5967); the mtime when the tail has
	// no time at all.
	if !m.contentAt.IsZero() {
		mod = m.contentAt
	}
	fresh := hasFile && now.Sub(mod) < claudeFresh
	spinner := claudeSpinnerTitle(a.RawTitle)
	if t, ok := n.hooks.get(id); ok {
		if t.pid > 0 && !processAlive(t.pid, t.start) {
			n.hooks.forget(id) // a dead process' turn is over, and a reused pid is not it
		} else {
			switch judgeHookTurn(hookSignals{turn: t, spinner: spinner, waiting: claudeWaitingTitle(a.RawTitle), hasFile: hasFile,
				fileMod: mod, turnOpen: m.claudeTurn, turnClosed: m.turnClosed, now: now}) {
			case hookWorking:
				return proto.StatusWorking
			case hookResting:
				return a.AgentStatus
			}
		}
	}
	switch {
	case m.turnClosed:
		return a.AgentStatus
	case spinner, m.claudeTurn && fresh:
		return proto.StatusWorking
	}
	return a.AgentStatus
}

// ternClaudeStatus is what a Claude in a Tern or Rex block says of itself, "" when it says nothing. Neither reports agent
// state (ternAgent, rexAgent), so a Claude there was judged by its transcript alone (#6312): a question it asked read as working
// while the transcript was fresh, then as unknown, which the overview does not list, and it never needed the user.
// Claude Code keeps its own state in its sessions/<pid>.json (the pid of the block's claude, the file that names its
// session): busy, idle (or shell: idle with background commands running), or waiting while a dialog of its own has the
// keyboard (a question, a permission, a picker; claudebg.go reads the same field for the claudes no engine shows).
// Waiting is blocked, and its screen card is made as for herdr's. Idle is a turn that ended, done as herdr calls a
// finished turn nobody has looked at, once there is a transcript (hasFile); before its first message it is idle.
// Nothing to go by (no pid found for the block yet, a file that names another session after a /clear, an older Claude
// Code that writes no status) is "".
func (n *Node) ternClaudeStatus(id, pane string, hasFile bool) string {
	n.mu.Lock()
	pid := n.panePIDs[pane].pid
	n.mu.Unlock()
	if pid == 0 {
		return ""
	}
	b, err := os.ReadFile(n.claudeSessionFile(pid))
	if err != nil {
		return ""
	}
	var f struct {
		SessionID string `json:"sessionId"`
		Status    string `json:"status"`
	}
	if json.Unmarshal(b, &f) != nil || f.SessionID != id {
		return ""
	}
	s := claudeBgStatus[f.Status]
	if s == proto.StatusIdle && hasFile {
		return proto.StatusDone
	}
	return s
}

// restingClaude: a Claude Code in a herdr or Tern pane whose host says its turn is over (or does not know), so a
// dialog on its screen is what it waits on (dialogstatus.go). herdr: where it was seen (#2553). Tern: a Claude Code
// that does not mark a dialog of its own waiting (ternClaudeStatus) shows it only there. Orca reports its own
// permission state, and botmux sessions have no screen of theirs to read.
func restingClaude(a host.Agent, status string) bool {
	if a.Agent != "claude" || a.PaneID == "" {
		return false
	}
	if k := host.ParsePane(a.PaneID).Kind; k != host.Herdr && k != host.Tern && k != host.Rex {
		return false
	}
	return status == proto.StatusDone || status == proto.StatusIdle || status == proto.StatusUnknown
}

// isDriving: Persona is working the session's TUI itself (modes.go): a picker it opened is not the user's to answer.
func (n *Node) isDriving(id string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.driving[id]
}

// claudeSpinnerTitle: the terminal title starts with the spinner Claude Code shows in it while it works — a quarter
// circle (◐◓◑◒, 2.1.3xx) or, in older versions, a braille dot; ✳ when it waits.
func claudeSpinnerTitle(title string) bool {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(title))
	return r >= '◐' && r <= '◓' || r >= 0x2800 && r <= 0x28FF
}

// claudeWaitingTitle: the title starts with the mark Claude Code shows in it while it waits for input (✳).
func claudeWaitingTitle(title string) bool {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(title))
	return r == '✳'
}
