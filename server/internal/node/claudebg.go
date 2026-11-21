package node

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/prompts"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// Claude sessions that run where Persona has no terminal pane: a background session Claude's own daemon runs (/fork,
// /background: kind "bg"), a session open in Claude's desktop app (entrypoint claude-desktop…), claude in a terminal
// that herdr, Orca, Tern or botmux do not run (entrypoint cli). Each writes sessions/<pid>.json in its config directory with
// the session it has open, its status (busy / idle / shell / waiting) and, since Claude Code 2.1.29x, its peer inbox
// (claudepeer.go). They listed as offline while they worked, and a message from the phone was refused (a second
// claude --resume would write the same transcript, openelsewhere.go). The node lists every live one as running, with
// its status from the file and its transcript as for any session; a message goes through its inbox when the node can
// use it (send) and is refused, with why, when it cannot — never by starting a second claude.
//
// What the phone cannot do there: answer a question or a permission prompt (those are on the computer: no hooks of
// Persona's run in such a claude, and its screen is not Persona's to read), stop a turn (nothing to send Esc to), or
// open a terminal. A claude in a pane of Persona's engines lists there as before (its pane comes first: seen). One
// whose pane variable names a pane no engine in use lists with a claude in it (Orca's terminals on a node without
// Orca, a nested tmux, a claude the engine's process scan missed) lists here as running, after a short grace that
// lets an engine report a claude just started in its pane first (claudeInPane).
//
// An interactive claude that sent its own session to the background keeps that session in its sessions/<pid>.json
// with parkedJobId set, and shows Claude's agents view in its pane until it comes back. herdr still names the pane
// after the session its SessionStart hook reported and reads its state off the terminal title — a background
// session's — so the parked session looked busy with another one's work, and a message to it went into the agents
// view. As in Claude Code (a parked claude does not hold its session), such a pane is not taken for that session.
//
// The agents view opens the job the session went to (Claude's autoOpenJobId), and the pane then is that job's: its
// conversation, its prompt, its name as the terminal title. Such a pane is the background session's (the job's
// sessions/<pid>.json has jobId = the parkedJobId, and its name is the title), so it lists in its herdr tab and takes
// what is typed (2026-11-04: tab 4 of a herdr workspace was missing on the phone, its session listed apart, read-only).
//
// On the agents list (the job ended or stopped, or another one is picked) the pane is still the parked session's tab:
// it lists there, idle (herdr's state is a background session's title), with its terminal: the phone picks a session
// from the list as on the computer (arrows, enter), and the pane is then that one's. A message is not typed into it —
// the list's prompt starts a new background session. Before, such a pane listed nowhere (2026-11-05: tabs 2 and 4
// missing on the phone after their jobs ended).

const sourceClaudeBg = proto.SourceClaudeBg

const claudeBgReadOnly = proto.ReadOnlyClaudeBg

const claudeAgentsList = "这个标签停在 Claude Code 的 agents 列表上：打开终端，用方向键选中一个会话、回车进去后再发消息"

// claudeBgStatus maps a claude's sessions/<pid>.json status to the node's: waiting is a question or dialog of its
// own, answered on the computer. shell (Claude Code 2.1.31x and later) is a turn that ended while a background command
// or Monitor still runs, which older builds wrote as idle; the commands come from the transcript (#6363). Anything else
// is unknown.
var claudeBgStatus = map[string]string{
	"busy": proto.StatusWorking, "idle": proto.StatusIdle, "shell": proto.StatusIdle, "waiting": proto.StatusBlocked,
}

// parkedClaudeSessions are the sessions only parked claudes have open, each with the background job it went to.
func parkedClaudeSessions(procs []claudeProc) map[string]parkedJob {
	parked, held := map[string]parkedJob{}, map[string]bool{}
	jobs := map[string]claudeProc{}
	for _, c := range procs {
		if c.Kind == "bg" && c.JobID != "" {
			jobs[c.JobID] = c
		}
	}
	for _, c := range procs {
		if c.ParkedJobID != "" {
			j := jobs[c.ParkedJobID]
			parked[c.SessionID] = parkedJob{SessionID: j.SessionID, Name: j.Name}
		} else {
			held[c.SessionID] = true
		}
	}
	for id := range held {
		delete(parked, id)
	}
	return parked
}

// parkedJob is the live background session a parked session went to: its id and name ("" when it is not running).
type parkedJob struct{ SessionID, Name string }

// shownIn is the job's session when the pane shows it (its name is the pane's title), else "".
func (j parkedJob) shownIn(a host.Agent) string {
	if j.SessionID == "" || !validID(j.SessionID) || j.Name == "" || a.Title != j.Name {
		return ""
	}
	return j.SessionID
}

// onAgentsList is a parked session's pane on the agents list, idle: the state herdr reads off the title is not its.
func onAgentsList(a host.Agent) host.Agent {
	a.AgentStatus = proto.StatusIdle
	return a
}

// agentsListRefusal refuses a message to a parked session whose pane is on the agents list, nil for any other.
func (n *Node) agentsListRefusal(id string) error {
	n.mu.Lock()
	listed := n.onAgentsList[id]
	n.mu.Unlock()
	if !listed {
		return nil
	}
	return refusal(openElsewhere, claudeAgentsList+"。这次没有发送。", map[string]any{"where": whereAgentsList})
}

// outsideSource is the source a live claude outside Persona's panes lists under, "" for one the node leaves alone: a
// warm background process no session has claimed, a parked one (its session is the job's now), one that runs from an
// SDK or another program (entrypoint sdk-…: its own host drives it), one inside a terminal of Persona's engines.
func (n *Node) outsideSource(c claudeProc) string {
	switch {
	case c.Spare || c.ParkedJobID != "" || !validID(c.SessionID):
		return ""
	case c.Kind == "bg":
		return sourceClaudeBg
	case c.Kind != "" && c.Kind != "interactive":
		return ""
	case desktopEntrypoint(c.Entrypoint):
		return proto.SourceClaudeDesktop
	case c.Entrypoint == "cli" && !n.claudeInPane(c):
		return proto.SourceClaudeTerminal
	}
	return ""
}

// claudeSessions lists the live claudes outside Persona's panes that are not in seen and have a transcript: the
// background ones, the desktop app's, the ones in other terminals.
func (n *Node) claudeSessions(procs []claudeProc, seen map[string]bool, livePanes map[string]bool) []proto.Session {
	var out []proto.Session
	listed := map[string]bool{}
	var desktop *claudeDesktopIndex // read once, when a desktop session rests
	for _, c := range procs {
		id := c.SessionID
		if seen[id] || listed[id] {
			continue // a pane lists it (and a cli claude's environment is not read for nothing)
		}
		source := n.outsideSource(c)
		if source == "" {
			continue
		}
		p := n.transcriptPath(id)
		if p == "" {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		listed[id] = true
		m := n.meta(p, st)
		status, ok := claudeBgStatus[c.Status]
		if !ok {
			status = proto.StatusUnknown
		}
		if status == proto.StatusIdle && source == proto.SourceClaudeDesktop {
			// Its last turn ended while nobody looked: Unread until it is seen (claudedesktop_unread.go).
			if desktop == nil {
				d := readClaudeDesktopIndex(n.home)
				desktop = &d
			}
			if n.desktopTurnUnread(id, *desktop) {
				status = proto.StatusDone
			}
		}
		s := proto.Session{
			SessionID: id, Cwd: c.Cwd, Status: status, Agent: "claude", Title: m.title, SourceKind: source,
			Model: n.shownModel(id, m.model), LastText: snippet(m.lastText), UpdatedAt: lastUsed(m, st), Rev: st.Size(), AgentSurface: m.surface,
		}
		s.Capabilities, s.ReadOnlyReason = outsideCapabilities(c)
		if s.Capabilities != nil && s.Capabilities.Send && n.ccPluginInput(id, c.pid) {
			s.Capabilities.PluginInput = true // the owner's messages go in as their input (ccplugin.go)
			if n.ccPluginSettingsCapable(id, c.pid) {
				s.Capabilities.Model = proto.Bool(true)
				s.Efforts = slices.Clone(proto.EffortLevels)
			}
		}
		if s.Capabilities != nil && c.Status == "busy" && n.ccPlugins.interruptCapable(id, c.pid) {
			s.Capabilities.Interrupt = true
		}
		// A claude in a tmux pane says where it runs ("tmux":"orc:@11.%11") and no hooks of Persona's run there: the
		// pane's screen is what its approval shows on, so it gets that screen and the keys into it (#2562).
		if pane := outsidePane(c); pane != "" {
			s.PaneID = pane
			if s.Capabilities == nil {
				s.Capabilities = &proto.SessionCapabilities{Read: true}
			}
			s.Capabilities.Terminal = true
			s.Capabilities.Send = true         // the owner's message can be typed here even with no inbox (#5965)
			s.Capabilities.PluginInput = false // this pane, not the plugin, carries the owner's input
			s.Capabilities.Model = proto.Bool(true)
			s.Capabilities.Modes = proto.CanModes(true)
			s.Efforts = slices.Clone(proto.EffortLevels)
			s.ReadOnlyReason = ""
			livePanes[pane] = true // its dialog read is kept while the claude lives (forgetDialogs)
			// Its own status file says waiting when a dialog has the keyboard, but a build that does not: a resting
			// claude's screen is read, as for a pane (dialogstatus.go).
			if status == proto.StatusIdle || status == proto.StatusDone || status == proto.StatusUnknown {
				if head := n.dialogOn(id, pane); head != "" {
					s.Status, s.AttentionReason = proto.StatusBlocked, head
				}
			} else if status == proto.StatusBlocked {
				// The phone answers the dialog on the pane's screen: the row names it once it is read (#5965).
				s.AttentionReason = n.dialogOn(id, pane)
			}
		}
		if status == proto.StatusBlocked && s.AttentionReason == "" {
			s.AttentionReason = proto.AttentionClaudeComputer // no dialog read from a pane (yet)
			if s.Capabilities != nil && s.Capabilities.Send && s.PaneID == "" && asking(c, m) {
				s.AttentionReason = proto.AttentionClaudeAnswer // a message answers it (sendOutside)
				// Its question is a card the apps answer (claudeoutside_ask.go): the answer goes as that message.
				s.Capabilities.Approve = true
			}
		}
		if s.Cwd == "" {
			s.Cwd = m.cwd
		}
		// The transcript's title first, then a name something chose for this claude, then its first message (untitled)
		// as for any session. The name Claude derives itself only stands in while there is nothing else: it is the
		// directory's base name and two hex digits, so the sessions of one directory differed by those alone.
		if s.Title == "" && c.NameSource != "derived" {
			s.Title = c.Name
		}
		untitled(&s, m)
		if s.Title == "" {
			s.Title = c.Name
		}
		n.setSessionContext(&s, m)
		s.ModelFallback = m.modelFallback
		s.Activity, s.ActivityAt = m.activityOf()
		s.PermissionMode, s.Effort = n.currentModes(id, m)
		m.turnErr.set(&s)
		if status == proto.StatusWorking || status == proto.StatusBlocked {
			s.Queued = m.queue
		}
		s.WokeByTask = m.wake
		n.setBackground(&s, m) // as for a pane's session
		s.BackgroundWaiting = n.backgroundWaits(&s)
		s.Loop, s.Goal, s.Plan = m.loop, m.goal, m.plan
		s.Subagents = n.subagents(id)
		n.decorate(&s)
		out = append(out, s)
	}
	return out
}

// outsideCapabilities: read always; send when its peer inbox is one the node can use, with a reason when not. Never
// modes: the peer inbox carries messages only, and the session has no pane to drive the TUI's pickers in (setModes).
func outsideCapabilities(c claudeProc) (*proto.SessionCapabilities, string) {
	p, has := peerOf(c)
	switch {
	case has && peerReachable(p):
		// A turn it runs, or a question it waits on, stops at a message of priority now (interruptOutside).
		stoppable := c.Status == "busy" || c.Status == "waiting"
		return &proto.SessionCapabilities{Read: true, Send: true, Interrupt: stoppable, Modes: proto.CanModes(false)}, ""
	case c.Kind == "bg":
		return &proto.SessionCapabilities{Read: true, Modes: proto.CanModes(false)}, claudeBgReadOnly
	case !has:
		return &proto.SessionCapabilities{Read: true, Modes: proto.CanModes(false)}, proto.ReadOnlyClaudeNoInbox
	}
	return &proto.SessionCapabilities{Read: true, Modes: proto.CanModes(false)}, proto.ReadOnlyClaudeInbox
}

// outsidePane is the tmux pane an outside claude runs in, "" when it runs in none (#2562): Claude Code writes the pane
// down in its own session file, and the process's TMUX names the server that pane belongs to ("" for the default one).
func outsidePane(c claudeProc) string {
	socket := ""
	if paneEnvOfPID != nil {
		socket = tmuxSocket(paneEnvOfPID(c.pid)[tmuxEnv])
	}
	return tmuxPaneOf(c.Tmux, socket)
}

// outsidePaneOf is the tmux pane a session outside Persona's panes runs in, "" for any other session: where a card's
// answer goes when no host lists an agent for it (send.go paneFor, panescreen.go screen).
func (n *Node) outsidePaneOf(id string) string {
	c, ok := n.outsideClaude(id)
	if !ok {
		return ""
	}
	return outsidePane(c)
}

// outsideClaude is the live claude outside Persona's panes that has session id open (outsideSource), ok false when
// none has.
func (n *Node) outsideClaude(id string) (claudeProc, bool) {
	if !validID(id) {
		return claudeProc{}, false
	}
	for _, c := range n.liveClaudes() {
		if c.SessionID == id && n.outsideSource(c) != "" {
			return c, true
		}
	}
	return claudeProc{}, false
}

// sendOutside sends text to a session a claude outside Persona's panes has open, through its peer inbox. A verified
// owner's ordinary message to a claude in tmux is left for send's usual pane path instead; anyone else's message there
// still needs the inbox and is refused without one, never typed. With no pane and no inbox, handled is false and the
// caller refuses the session as open elsewhere (openElsewhereRefusal). Nothing starts a second claude --resume on the
// same transcript. A send counts once the session wrote that it took the message (peerTaken); one it did not take in
// time — held for the person at the computer, dropped as a duplicate — is reported as not confirmed (delivered
// unknown), so the app neither shows it as sent nor resends it by itself.
func (n *Node) sendOutside(ctx context.Context, id, text string, images []proto.Image) (handled bool, err error) {
	return n.sendOutsidePriority(ctx, id, text, images, "")
}

// sendOutsidePriority is sendOutside with a priority the caller has already established from the transcript's full
// items. An empty priority uses the quick-metadata question check below.
func (n *Node) sendOutsidePriority(ctx context.Context, id, text string, images []proto.Image, forced string) (handled bool, err error) {
	c, ok := n.outsideClaude(id)
	if !ok {
		return false, nil
	}
	pane := outsidePane(c)
	if pane != "" && strings.HasPrefix(strings.TrimSpace(text), "/") {
		// Keep the inbox's command refusal on the tmux route too: exitAgent knows only Persona's own agents.
		return true, refusal(peerCommand, peerCommandMsg, nil)
	}
	if forced == "" && pane != "" && ownerDeliveryOf(ctx) != nil {
		return false, nil // the owner's own words go through send's ordinary tmux-pane checks and confirmation (#5965)
	}
	p, has := peerOf(c)
	if !has {
		if pane != "" {
			return true, refusal(peerUnsupported, peerNoInboxMsg, nil) // nobody else's words may be typed into the pane
		}
		return false, nil
	}
	if strings.HasPrefix(strings.TrimSpace(text), "/") {
		// The inbox runs no commands (Claude Code takes a peer's message with skipSlashCommands): /exit, /model or
		// /clear would reach the model as words. Refused, so nothing looks done that was not.
		return true, refusal(peerCommand, peerCommandMsg, nil)
	}
	if len(images) > 0 {
		paths, err := n.saveImages(images)
		if err != nil {
			return true, err
		}
		text = attachImages(text, paths)
	}
	// A session waiting on a question it asked takes the message as its answer: at priority now it ends that wait (the
	// question's call ends unanswered) and runs the message; queued, it would wait behind the question until someone
	// answered it on the computer. One waiting on a permission prompt or another dialog gets it queued as before: at
	// now it would decline the prompt, which is the computer's to approve.
	var offset int64
	priority := forced
	if priority == "" {
		priority = peerNext
	}
	if path := n.transcriptPath(id); path != "" {
		if st, err := os.Stat(path); err == nil {
			offset = st.Size()
			if forced == "" && asking(c, n.metaNow(path, st)) {
				priority = peerNow
			}
		}
	}
	// The owner's own message goes in as their input through Persona's plugin when the session has it (ccplugin.go),
	// whether idle or queued behind a running turn; otherwise, and when the plugin does not take it, through the inbox.
	if owner := ownerDeliveryOf(ctx); owner != nil && forced == "" && priority == peerNext {
		if ok, why := n.ccPluginSend(ctx, c, id, text); ok {
			log.Printf("send %s: claude %d (%s) took it as the user's input through Persona's plugin", id, c.pid, c.Entrypoint)
			if c.Status == "busy" {
				markSendQueued(ctx)
			}
			owner.as, owner.reason = deliveredUserInput, ""
			return true, nil
		} else {
			owner.as, owner.reason = deliveredRelayed, why
		}
	}
	p.fromMode = n.peerAttest(ctx, c, id)
	_, p.source = ctx.Value(sourceDeliveryKey{}).(string)
	err = peerSendAt(ctx, p, id, text, priority)
	var pe *errPeer
	if asPeer(err, &pe) {
		log.Printf("send %s: claude %d's inbox refused: %v", id, c.pid, err)
		return true, refusal(pe.code, pe.msg, map[string]any{"pid": c.pid})
	}
	if peerTaken(ctx, func() string { return n.transcriptPath(id) }, offset, text, peerConfirmWait) {
		log.Printf("send %s: claude %d (%s) took it through its inbox (%s)", id, c.pid, c.Entrypoint, priority)
		// Taken into its queue while a permission prompt or another dialog holds it: Claude reads the message only once
		// that is answered at the computer, which the answer says (waits_on_computer). The status is read once, after
		// the take: a prompt that opened just after a busy session queued the message marks it too, and rightly, since
		// it still waits on the computer. A message at priority now waits on nothing: it ended the question it answers
		// (the status file may say waiting a moment longer).
		if now, ok := n.outsideClaude(id); ok && priority == peerNext && claudeBgStatus[now.Status] == proto.StatusBlocked {
			if w, ok := ctx.Value(computerWaitKey{}).(*bool); ok {
				*w = true
			}
		}
		return true, nil
	}
	if err != nil {
		log.Printf("send %s: claude %d's inbox: %v", id, c.pid, err)
		return true, uncertain(err)
	}
	if peerHeldSeen(n.transcriptPath(id), offset) {
		log.Printf("send %s: written to claude %d's inbox, not taken within %s: it holds it for approval at the computer", id, c.pid, peerConfirmWait)
		return true, heldOnComputer()
	}
	log.Printf("send %s: written to claude %d's inbox, not taken within %s (held or dropped there)", id, c.pid, peerConfirmWait)
	return true, unconfirmed()
}

// peerAttest is the from-mode the node states in a message to session id, which claude c has open (claudepeer.go). It
// is the class of whoever's words the message carries, bound to evidence taken when the message is sent, and stated
// only where that evidence is reliable (#9607):
//   - the owner's own message (ownerInputExact), card answer or Stop (the Devices manager's owner-control RPC): it goes in as the session's own user's words, the one authorization the node acts on. The
//     person who owns the session is the one its permission prompts and held messages wait on, and what they type at its
//     terminal goes in whatever its mode. bypass when live receiver evidence puts the session in that class
//     (peerSessionClassAt); nothing otherwise (a session that prompts takes a message that states none). Uncertain
//     evidence can only make the session hold the message, never take more than its owner could.
//   - an Agent message on the source-permission delivery RPC: the source node captured its class when the words were produced, bound to the source process and call. It never becomes owner input.
//   - anything without reliable source evidence (including older peers and non-owner controls): nothing.
func (n *Node) peerAttest(ctx context.Context, c claudeProc, id string) string {
	if ownerDeliveryOf(ctx) != nil {
		if peerSessionClassAt(ctx, n, c, id) == peerModeBypass {
			return peerModeBypass
		}
		return ""
	}
	mode, _ := ctx.Value(sourceDeliveryKey{}).(string)
	return mode
}

// peerSessionClass is the classifier without a send context. Keep its signature for callers that only inspect a class.
func peerSessionClass(n *Node, c claudeProc, id string) string {
	return peerSessionClassAt(context.Background(), n, c, id)
}

// peerSessionClassAt also accepts plan mode for owner delivery if the destination can switch to bypass.
func peerSessionClassAt(ctx context.Context, n *Node, c claudeProc, id string) string {
	mode := peerSessionModeAt(ctx, n, c, id)
	if mode == "plan" && claudeCanBypass(c.pid) {
		return peerModeBypass
	}
	return permissionClass(mode)
}

// peerSessionModeAt reads the current permission mode. A readable external tmux footer is the freshest mode:
// shift+tab changes it before Claude writes the transcript. A visible composer without a recognized mode is prompting:
// bypass and plan keep their indicators when a draft or dialog hides default's hint. Without live evidence, read the newest
// transcript copy in claude c's own config directory, bypassing the ordinary metadata cache.
func peerSessionModeAt(ctx context.Context, n *Node, c claudeProc, id string) string {
	mode := ""
	if pane := outsidePane(c); pane != "" {
		if text, err := n.readPane(ctx, pane, "visible", 60); err == nil {
			mode = tuiMode(text)
			if mode == "" && tuiPromptingScreen(text) {
				return "default"
			}
		}
	}
	if mode == "" {
		matches, _ := filepath.Glob(filepath.Join(filepath.Dir(c.dir), "projects", "*", id+".jsonl"))
		var path string
		var newest os.FileInfo
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && (newest == nil || st.ModTime().After(newest.ModTime())) {
				path, newest = m, st
			}
		}
		var m quickMeta
		if newest != nil {
			m = n.metaNow(path, newest)
		}
		mode = m.mode
		if c.pid > 0 && c.StartedAt > 0 && (mode == "" || (!m.modeAt.IsZero() && m.modeAt.UnixMilli() < c.StartedAt)) {
			if args, err := processArgv(c.pid); err == nil {
				if argMode := argsPermissionMode(args); argMode != "" {
					mode = argMode
				}
			}
		}
	}
	return mode
}

// computerWaitKey is where sendOutside leaves, for session.send's answer, that the session took the message while
// waiting on a dialog only the computer can answer (withComputerWait).
type computerWaitKey struct{}

// withComputerWait gives sendOutside somewhere to say the message waits behind a dialog on the computer.
func withComputerWait(ctx context.Context) (context.Context, *bool) {
	w := new(bool)
	return context.WithValue(ctx, computerWaitKey{}, w), w
}

// askTool is Claude Code's question tool.
const askTool = "AskUserQuestion"

// asking: claude c waits on a question it asked (its status file says waiting, and its transcript's latest call, m,
// is the question), not on a permission prompt, a plan to approve or another dialog only the computer answers.
func asking(c claudeProc, m quickMeta) bool {
	return c.Status == "waiting" && m.tool == askTool
}

// interruptOutside stops the turn a claude outside Persona's panes runs, through its inbox: a message at priority now
// cuts the turn (or ends a question or permission prompt it waits on) and asks it to stop (prompts.PeerStop). handled false for any other
// session, and one a tmux pane runs (its Esc goes to the pane). A session at rest has nothing to stop: done, nothing sent.
func (n *Node) interruptOutside(ctx context.Context, id string) (handled bool, err error) {
	c, ok := n.outsideClaude(id)
	if !ok || outsidePane(c) != "" {
		return false, nil
	}
	if ownerDeliveryOf(ctx) != nil && c.Status == "busy" && n.ccPluginInterrupt(ctx, c, id) {
		return true, nil
	}
	p, has := peerOf(c)
	if !has {
		return true, refusal(peerUnsupported, peerNoInboxMsg, nil)
	}
	if c.Status != "busy" && c.Status != "waiting" {
		return true, nil // at rest already (a Stop from a list a moment old): nothing to stop, nothing sent
	}
	var offset int64
	if st, err := os.Stat(n.transcriptPath(id)); err == nil {
		offset = st.Size()
	}
	stop := prompts.PeerStop()
	p.fromMode = n.peerAttest(ctx, c, id)
	err = peerSendAt(ctx, p, id, stop, peerNow)
	var pe *errPeer
	if asPeer(err, &pe) {
		return true, refusal(pe.code, pe.msg, map[string]any{"pid": c.pid})
	}
	if peerTaken(ctx, func() string { return n.transcriptPath(id) }, offset, stop, peerConfirmWait) {
		log.Printf("interrupt %s: claude %d took the stop through its inbox", id, c.pid)
		return true, nil
	}
	if err != nil {
		return true, uncertain(err)
	}
	if peerHeldSeen(n.transcriptPath(id), offset) {
		return true, heldOnComputer()
	}
	return true, unconfirmed()
}

// heldOnComputer is the outcome of a message the session did not take and wrote that it holds for approval at the
// computer (peerHeldSeen). Still delivered unknown: it goes in once approved there, and nothing resends it.
func heldOnComputer() error {
	b, _ := json.Marshal(map[string]any{"delivered": deliveredUnknown})
	return &rpc.Error{Code: peerHeld, Message: peerHeldMsg, Data: b}
}

// unconfirmed is the outcome of a message the inbox read but the session has not taken: delivery unknown.
func unconfirmed() error {
	b, _ := json.Marshal(map[string]any{"delivered": deliveredUnknown})
	return &rpc.Error{Code: peerUnconfirmed, Message: peerUnconfirmedMsg, Data: b}
}

// asPeer: err is a refusal of the inbox's (*errPeer).
func asPeer(err error, target **errPeer) bool { return errors.As(err, target) }

// claudeInPane: the claude process runs in a pane that one of the engines in use lists with a claude in it, so that
// engine lists it, not this. A pane variable alone is not enough: an engine the machine does not enable (Orca's
// terminals while the node uses herdr alone), or one whose process scan missed the claude (a nested pty, a launcher
// that renamed it), lists nothing there, and a running claude must not fall through to the offline list. A claude
// the node first saw less than outsidePaneGrace ago still counts as its pane's: an engine reports a claude typed into
// its pane a moment after it starts, and until then it should not flip to an outside row and back.
func (n *Node) claudeInPane(c claudeProc) bool {
	if paneEnvOfPID == nil {
		return false
	}
	refs := n.enginePaneRefs(paneEnvOfPID(c.pid))
	if len(refs) == 0 {
		return false
	}
	now := time.Now()
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, agents := range n.hostAgents {
		for _, a := range agents {
			if a.Agent == "claude" && slices.Contains(refs, a.PaneID) {
				return true
			}
		}
	}
	if n.paneClaudes == nil {
		n.paneClaudes = map[int]paneClaudeSeen{}
	}
	for pid, e := range n.paneClaudes { // gone (the list is rebuilt every second): forget it
		if now.Sub(e.last) > paneClaudeForget {
			delete(n.paneClaudes, pid)
		}
	}
	e, ok := n.paneClaudes[c.pid]
	if !ok || e.start != c.ProcStart { // first seen, or the pid now belongs to another process
		e = paneClaudeSeen{start: c.ProcStart, first: now}
	}
	e.last = now
	n.paneClaudes[c.pid] = e
	return now.Sub(e.first) < outsidePaneGrace
}

// paneClaudeSeen is when claudeInPane first and last saw a claude process (start: its procStart, telling a reused pid
// apart).
type paneClaudeSeen struct {
	start       string
	first, last time.Time
}

// outsidePaneGrace: how long a claude in an engine pane that no engine lists yet is left to its engine (a var: tests
// set it to 0). paneClaudeForget: an entry not looked at for this long is dropped.
var outsidePaneGrace = 5 * time.Second

const paneClaudeForget = time.Minute

// enginePaneRefs are the panes a process's engine pane variables (enginePaneEnvs) name, as the hosts write PaneIDs.
func (n *Node) enginePaneRefs(env map[string]string) []string {
	var out []string
	add := func(kind, id string) {
		if id != "" {
			out = append(out, host.PaneRef{Kind: kind, ID: id}.String())
		}
	}
	if p := n.envPane(func(name string) string { return env[name] }); p != "" {
		out = append(out, p) // herdr's pane ids are stored as they are, a named session's qualified (#2196)
	}
	add(host.HerdrCore, env[herdrCorePaneEnv])
	add(host.Orca, env[orcaPaneEnv])
	add(host.Orca, env[orcaLegacyPaneEnv])
	add(host.Botmux, env[botmuxPaneEnv])
	add(host.Tern, env[ternPaneEnv])
	add(host.Rex, env[rexPaneEnv])
	return out
}

// enginePaneEnvs are the variables Persona's engines set in their panes' processes.
// herdrSessionEnv and herdrSocketEnv come along to tell which herdr server's pane HERDR_PANE_ID is.
var enginePaneEnvs = []string{herdrPaneEnv, herdrSessionEnv, herdrSocketEnv, herdrCorePaneEnv, orcaPaneEnv, orcaLegacyPaneEnv,
	botmuxPaneEnv, ternPaneEnv, rexPaneEnv}

// tmuxEnv is tmux's own variable in a pane's processes: "<socket>,<server pid>,<session index>". Its socket names the
// server the pane belongs to, which a pane id alone does not (tmuxHost).
const tmuxEnv = "TMUX"

// paneEnvKeys are the variables read from a process's environment: the engines' pane variables, and the tmux server.
var paneEnvKeys = append(append([]string{}, enginePaneEnvs...), tmuxEnv)
