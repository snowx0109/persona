package node

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nwverse/persona/sdk/go/wire"
	"github.com/nwverse/persona/server/internal/closeout"
	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/identity"
	"github.com/nwverse/persona/server/internal/proto"
)

// hostTextSlack: the transcript's words this long after the host's record of the answer still belong to that turn.
const hostTextSlack = 10 * time.Minute

// hostNotices applies what the host says beyond the state: a failure notice (FailureReason, the state stays the
// turn's), and an attention the agent is past (a message came in after it: the turn's state, no reason). A host only
// clears its notices when a message comes through it; one sent another way leaves them standing.
func (n *Node) hostNotices(id string, s *proto.Session, a host.Agent) {
	if a.Failure == "" && a.AttentionAt.IsZero() {
		return
	}
	var promptAt time.Time
	if p := n.transcriptPath(id); p != "" {
		if st, err := os.Stat(p); err == nil {
			promptAt = n.meta(p, st).lastPromptAt
		}
	}
	if a.Failure != "" && (a.FailureAt.IsZero() || !promptAt.After(a.FailureAt)) {
		s.FailureReason = a.Failure
	}
	if !a.AttentionAt.IsZero() && promptAt.After(a.AttentionAt) && s.Status == proto.StatusBlocked {
		s.Status = a.TurnStatus
	}
}

// sessionList returns live herdr sessions plus recent offline transcripts. One lookup pass (codexListPass) serves the
// whole rebuild: the rows' decoration and the Codex threads take each rollout's stat and the assistant's project name
// from it, instead of once per row every second.
func (n *Node) sessionList(ctx context.Context, refreshAgents bool) []proto.Session {
	return n.sessionListIn(ctx, refreshAgents, newCodexListPass())
}

// sessionListIn is sessionList looking up through pass (nil: each time).
func (n *Node) sessionListIn(ctx context.Context, refreshAgents bool, pass *codexListPass) []proto.Session {
	if refreshAgents {
		_ = n.refreshHosts(ctx)
	}
	n.mu.Lock()
	agents := make(map[string]host.Agent, len(n.agents))
	agyRuntimeIDs := map[string]string{}
	for id, agent := range n.agents {
		agents[id] = agent
		if agent.Agent == agyAgent && agent.AgentStatus != proto.StatusOffline && agent.AgentStatus != proto.StatusInterrupted {
			if conversation := n.agyConversationLocked(id, agent.PaneID); conversation != "" {
				agyRuntimeIDs[id] = agyAgent + "-" + conversation
			}
		}
	}
	onList := n.onAgentsList // replaced whole with n.agents (setAgentsAt), never changed in place
	n.mu.Unlock()

	// One slice for the whole list, sized from the last one, filtered where it is (#2826): grown from nil and copied again
	// into the rows kept, with the runtimes' rows in a third, a list of 300 rows (1,520 bytes each) took about 4.8 MB.
	size := int(n.listRows.Load())
	out := make([]proto.Session, 0, size+4) // room for a few new sessions; more regrow it once
	seen := make(map[string]bool, size)
	livePanes := map[string]bool{} // the resting Claudes' panes, whose screens are read for a dialog
	now := time.Now()
	// In id order, not the map's: the list is sorted by UpdatedAt with a stable sort, and rows that tie on it (live panes
	// with no transcript yet all have none) came out in a new order on every pass, each one a whole new list for the hub.
	for _, id := range slices.Sorted(maps.Keys(agents)) {
		a := agents[id]
		if !onList[id] { // a pane on Claude's agents list stays idle: the transcript is its parked session's (claudebg.go)
			a.AgentStatus = n.liveStatus(id, a, now)
		}
		s := proto.Session{
			SessionID: id, Cwd: a.Cwd, Status: a.AgentStatus, PaneID: a.PaneID, TabID: a.TabID,
			WorkspaceID: a.WorkspaceID, AgentName: a.Name, Title: a.Title, Agent: a.Agent,
			ExternalURL: a.ExternalURL, Chat: a.Chat, Topic: a.Topic, Dispatched: a.Dispatched,
		}
		if a.Agent == "codex" && isCodexWaitID(id) { // opened with no first message: no thread, no transcript yet (codex_waiting.go)
			n.placeIn(&s, a)
			codexWaitRow(&s, a)
			if w := n.codexWaitByID(id); w != nil {
				s.UpdatedAt = w.began.UTC()
				if w.manual && s.HerdrTabName() != "" {
					s.Title = s.HerdrTabName()
				}
			}
			n.decorateIn(&s, pass)
			out = append(out, s)
			seen[id] = true
			continue
		}
		viaBotmux := host.ParsePane(a.PaneID).Kind == host.Botmux
		switch {
		case viaBotmux && (a.Agent == "codex" || a.Agent == traexAgent):
			// The thread's name, else botmux's title of the topic; messages go through botmux, neither route.
			if t := n.codexTitle(id); a.Agent == "codex" && t != "" {
				s.Title = t
			} else if t := n.traexTitle(id); a.Agent == traexAgent && t != "" {
				s.Title = t
			}
		case a.Agent == "codex":
			s.Title = n.codexTitle(id)                        // the terminal title is "dots | codex | Ready"
			s.CodexRoute, s.CodexRouteNote = n.codexRoute(id) // codex_daemon.go
		case a.Agent == traexAgent:
			s.Title = n.traexTitle(id) // the thread's name; the terminal title adds " | <directory>"
			s.CodexRoute, s.CodexRouteNote = codexRouteTerminal, "TRAEX 没有共享的 app-server"
		}
		n.placeIn(&s, a)
		s.Ports = n.portsOf(a.PaneID)
		if a.Agent == "codex" && s.Status == proto.StatusWorking && n.codexTurnOver(id, time.Now()) {
			s.Status = proto.StatusDone
		}
		n.hostNotices(id, &s, a)
		n.mu.Lock()
		if n.driving[id] && s.Status == proto.StatusBlocked {
			s.Status = proto.StatusIdle
		}
		n.mu.Unlock()
		if onList[id] { // its pane is on Claude's agents list: shown in its tab, picked from in its terminal (claudebg.go)
			s.Capabilities, s.ReadOnlyReason = &proto.SessionCapabilities{Read: true, Terminal: true}, claudeAgentsList
		}
		if s.Status == proto.StatusBlocked { // only while it still waits (not once the phone drives it)
			s.AttentionReason = a.Attention
		}
		// Resting is what its host says, before a lost first message marks the row blocked: the dialog that kept the
		// message out is read and named, not only the generic sentence.
		resting := restingClaude(a, s.Status)
		if why := n.lostFirstPrompt(id, s.Status); why != "" { // its first message did not go in (firstprompt.go)
			s.Status, s.AttentionReason = proto.StatusBlocked, why
		}
		if resting && !onList[id] && !n.isDriving(id) {
			livePanes[a.PaneID] = true
			if head := n.dialogOn(id, a.PaneID); head != "" { // a dialog holds its terminal (dialogstatus.go)
				s.Status, s.AttentionReason = proto.StatusBlocked, head
			}
		}
		// An allow its Claude did not take: the dialog it left holds the terminal, whatever herdr says (allowwatch.go).
		if head := n.allowDialog(id); head != "" && !onList[id] && !n.isDriving(id) {
			s.Status, s.AttentionReason = proto.StatusBlocked, head
		}
		if !onList[id] { // the list's prompt is no suggestion of its
			s.Suggestion = n.noteStatus(id, a.Agent, a.PaneID, s.Status) // suggestion.go
		}
		// textAt: when the transcript last had the agent's words; rollout: a Codex pane's rollout meta, for its effort.
		var textAt, promptAt time.Time
		var rollout quickMeta
		var rolloutPath string
		if p := n.transcriptPath(id); p != "" {
			if st, err := os.Stat(p); err == nil {
				m := n.meta(p, st)
				if identity.InHealthSpace(m.cwd) {
					s.Space = identity.SpaceHealth
				}
				rolloutPath = p
				textAt = m.lastTextAt
				rollout = m
				if m.title != "" && a.Agent != "codex" && a.Agent != traexAgent {
					s.Title = m.title
				}
				if s.Title == "" || s.Title == "Claude Code" && !m.titleManual {
					s.Title = ""
					untitled(&s, m)
				}
				s.Model, s.LastText, s.UpdatedAt, s.Rev = m.model, snippet(m.lastText), lastUsed(m, st), st.Size()
				s.AgentSurface = m.surface
				s.Asks, s.AsksRead = m.asks, true // the list's last_text is only the reply's start (#5096)
				promptAt = m.lastPromptAt
				if a.Agent != "codex" && a.Agent != traexAgent {
					s.Model = n.shownModel(id, m.model) // the pick made in the app until a reply shows the real one (model_switch.go)
				}
				m.turnErr.set(&s)
				s.Activity, s.ActivityAt = m.activityOf()
				n.setSessionContext(&s, m)
				s.ModelFallback = m.modelFallback
				s.WokeByTask = m.wake
				if s.Status == proto.StatusWorking || s.Status == proto.StatusBlocked {
					s.Queued = m.queue // an idle agent has taken everything; stale entries are from outside the tail
				}
				// Background shells keep running after the turn ends; background subagents are listed as subagents.
				n.setBackground(&s, m)
				s.BackgroundWaiting = n.backgroundWaits(&s) // bgwait.go (#6045)
				s.Loop, s.Goal, s.Plan = m.loop, m.goal, m.plan
				s.Subagents = n.subagents(id)
			}
		}
		if a.Agent != "codex" && a.Agent != traexAgent {
			n.terminalModesRow(&s, rollout)
		}
		blockedSince := n.noteBlockedAt(id, s.Status == proto.StatusBlocked, now)
		if a.Agent == "codex" && host.ParsePane(a.PaneID).Kind != host.Botmux && s.Status == proto.StatusBlocked &&
			n.codexRolloutWorkingOverBlocked(ctx, id, a.PaneID, rolloutPath, rollout, blockedSince, now) {
			s.Status = proto.StatusWorking
			s.AttentionReason = ""
		}
		// The host's record of the answer (botmux's reply in the topic), unless the transcript has words well after it:
		// the session went on outside the host (a `claude --resume` in a terminal). Text written in the same turn
		// right after the reply (the sentinel, a closing line) comes within minutes.
		if a.LastText != "" && (a.LastTextAt.IsZero() || !textAt.After(a.LastTextAt.Add(hostTextSlack))) {
			s.LastText = snippet(a.LastText)
			// A message to the agent after the host's answer answered its question (#5096); a host that gives no time
			// for its answer is taken as the latest word.
			if !a.LastTextAt.IsZero() && promptAt.After(a.LastTextAt) {
				s.Asks = ""
			} else {
				s.Asks = closeout.AskOf(a.LastText)
			}
			s.AsksRead = true
		}
		if a.Agent == "codex" {
			if _, thread, err := n.codexOwner(id); err == nil && thread.model != "" {
				s.Model = thread.model
			}
			n.codexRewindRow(id, &s)
			if c, thread, driven := n.codexEffortThread(id); driven {
				s.Model = n.codexModelNow(id, c, thread, s.Model)
				s.Capabilities.Model = proto.Bool(true)
			}
			n.codexEffortRow(id, &s.Efforts, &s.Effort, rollout) // joined on its app-server: its effort (codex_effort.go)
			n.codexSpeedRow(&s)
		}
		if piFamily(a.Agent) {
			n.mergePiRow(&s) // its conversation is the extension's (pi.go)
		}
		switch {
		case a.Agent == opencodeAgent:
			s.Title = opencodeTitle(a.Title)
			n.mergeOpencodeRow(&s) // its conversation is the plugin's (opencode.go)
		case a.Agent == cursorAgent || a.Agent == agyAgent:
			// Herdr drives the pane (agent.prompt, the keys). Agy can also read its verified conversation (workagent.go).
			if runtimeID := agyRuntimeIDs[id]; runtimeID != "" {
				seen[runtimeID] = true
				if s.Model == "" {
					if runtimeRow, _, ok := n.runtimes.rowOf(runtimeID); ok {
						s.Model = runtimeRow.Model
					}
				}
			}
		case otherAgent(a):
			s.ReadOnlyReason = otherNoReader(a.Agent)
		}
		if n.codexBlocked(id) {
			markCodexConflict(&s)
		}
		n.decorateIn(&s, pass)
		out = append(out, s)
		seen[id] = true
	}

	for _, s := range n.starts.listed(seen) { // starting.go
		n.decorateIn(&s, pass)
		out = append(out, s)
		seen[s.SessionID] = true
	}
	for _, s := range n.switches().listed(seen) { // an agent moving to another launcher, between its two (switch_share.go)
		n.decorateIn(&s, pass)
		out = append(out, s)
		seen[s.SessionID] = true
	}
	// Runtimes (runtimes.go): listed as they say, appended straight into the list and dropped there when already seen.
	from := len(out)
	out = n.runtimes.appendSessions(out)
	if pass != nil {
		pass.runtimes = n.runtimes.codexThreads() // conversations that are Codex threads here: listed once (runtimecodex.go)
	}
	peak := len(out)
	j := from
	var rowIDs map[string]bool // the runtimes' rows, where the line of a fork stops (aliasesOf)
	for i := from; i < len(out); i++ {
		s := &out[i]
		if seen[s.SessionID] {
			continue
		}
		n.decorateState(s, false)
		if n.runtimeAssistant(s.SessionID) { // the assistant keeps its mode (assistant_runtime.go)
			if m := n.assistantModeOf(s.SessionID); m != "" {
				s.PermissionMode = m
			}
		}
		// A conversation in a Claude transcript here (runtimetranscript.go): what it says, and its own row goes. Under
		// an id of its own, the ids it went by go too, named on the row for the manager's archive (runtimealiases.go).
		if t := n.mergeRuntimeTranscript(s); t != "" {
			seen[t] = true
			if t != s.SessionID {
				if rowIDs == nil {
					rowIDs = make(map[string]bool, len(out)-from)
					for k := from; k < len(out); k++ {
						rowIDs[out[k].SessionID] = true
					}
				}
				aliases := n.aliasesOf(s.SessionID, t, func(id string) bool { return rowIDs[id] })
				for _, a := range aliases {
					seen[a] = true
				}
				s.SetAliases(aliases)
			}
		}
		seen[s.SessionID] = true
		if j != i {
			out[j] = *s
		}
		j++
	}
	clear(out[j:])
	out = out[:j]
	for _, s := range n.assistantSessions() {
		if !seen[s.SessionID] {
			out = append(out, s)
			seen[s.SessionID] = true
		}
	}
	for _, s := range n.claudeSessions(n.liveClaudes(), seen, livePanes) { // claudes outside Persona's panes (claudebg.go)
		out = append(out, s)
		seen[s.SessionID] = true
	}
	for _, s := range n.traexSessions(seen) {
		out = append(out, s)
		seen[s.SessionID] = true
	}
	n.forgetDialogs(livePanes) // after every row that reads a pane's dialog, the tmux claudes' too
	for _, s := range n.codexSessionsIn(seen, pass) {
		out = append(out, s)
	}
	for _, s := range n.codexAssistantsAtRest(seen) {
		out = append(out, s)
		seen[s.SessionID] = true
	}
	from = len(out)
	out = n.appendOffline(out, seen)
	n.markInterrupted(out[from:], seen) // the restart cut these off (#1917)
	// omp's stopped sessions (ompdisk.go)
	out = append(out, n.ompOfflineRows(seen)...)
	for i := from; i < len(out); i++ {
		n.decorateIn(&out[i], pass)
		n.markStopped(&out[i]) // read-only (stopped.go, #4754)
	}
	n.listRows.Store(int32(max(peak, len(out))))
	// Terminal-owned Codex parents were already seen before history was added: over the whole list, before any row goes.
	resolveCodexHierarchy(out)
	// A conversation that moved between Claude Code and Codex is one row, the session running it now: the sessions it
	// ran in before are left out (engine_switch.go).
	var earlier map[string]bool
	if chains := n.state.engineHistories(); chains != nil {
		earlier = joinStretches(out, chains)
	}
	// The rows kept move down over the dropped ones, in order.
	j = 0
	for i := range out {
		s := &out[i]
		if !s.Assistant && s.PaneID != "" && s.Status != proto.StatusOffline && s.Status != proto.StatusInterrupted {
			n.observedRunning(s.SessionID)
			n.noteGatewayRunning(s)
		}
		if earlier[s.SessionID] || !n.sessionScopeAllows(s.Cwd) {
			continue
		}
		if n.listBlocked(s.SessionID, s.Cwd) {
			continue
		}
		if s.Status == proto.StatusWorking && (s.Agent == "" || s.Agent == "claude" || s.Agent == "codex") {
			s.ToolLoop = n.toolLoopOf(s.SessionID)
		}
		s.WorkspaceCwd = n.sessionWorkspaceCwd(s.Cwd)
		fillGitPlace(s)            // project, worktree and branch for the apps' tree (gitplace.go)
		s.CanFork = n.forkable(*s) // "Branch to New Chat" (fork.go)
		if j != i {
			out[j] = *s
		}
		j++
	}
	clear(out[j:])
	if j == 0 {
		// No row kept is nil, as the list always was: sessions.changed and sessions.list say null, not [], and the manager
		// takes [] from a sessions.list as a list emptied (devices.go), null as none given.
		return nil
	}
	out = out[:j]
	parents, origins := n.offlineDesktop()
	linkSideSessions(out, parents)           // the desktop app's side sessions under the sessions that started them (#5935)
	linkHerdrChildren(out, agents)           // herdr's sub-agent terminals under the session of the terminal that started them
	placeClaudeDesktop(out, origins, n.home) // and in their projects (#8521)
	sortNewestFirst(out)
	return out
}

// sortNewestFirst sorts list by UpdatedAt, newest first, rows that tie in the order they came (a stable sort): it sorts
// the rows' positions, then moves each row once. A stable sort of the rows themselves moved each 1,520-byte row over
// and over: 250 us of a 300-row list, against 50 us.
func sortNewestFirst(list []proto.Session) {
	order := make([]int32, len(list))
	for i := range order {
		order[i] = int32(i)
	}
	slices.SortStableFunc(order, func(a, b int32) int { return list[b].UpdatedAt.Compare(list[a].UpdatedAt) })
	// Row i takes the row at order[i]: each cycle of the permutation is followed once, its first row held aside.
	for i := range order {
		if order[i] < 0 || int(order[i]) == i {
			continue
		}
		held := list[i]
		j := i
		for {
			k := int(order[j])
			order[j] = -1 // in place
			if k == i {
				list[j] = held
				break
			}
			list[j] = list[k]
			j = k
		}
	}
}

func (n *Node) decorate(s *proto.Session) { n.decorateIn(s, nil) }

// decorateIn is decorate looking up through a list pass (codexListPass; nil: each time).
func (n *Node) decorateIn(s *proto.Session, pass *codexListPass) {
	s.AlsoOpenIn = n.alsoOpenIn(s.SessionID) // live in another host too (ownership.go)
	if s.PaneID != "" && s.Host == "" {
		s.Host = host.ParsePane(s.PaneID).Kind // the host it runs in (#2576)
	}
	if st, ok := n.decorateState(s, true); ok {
		s.Compaction = st.Compaction
		s.PendingSwitch = st.PendingSwitch   // launcher_switch.go
		s.SetEngineHistory(st.EngineHistory) // engine_switch.go
	}
	n.decorateModelRecovery(s) // node-owned recovery marker, and the shipped fallback-card projection (#7824)
	if s.PendingSwitch == nil {
		s.PendingSwitch = n.unrecordedSwitch(s.SessionID)
	}
	if ps := s.PendingSwitch; ps != nil && time.Since(ps.At) > switchMaxAge {
		s.PendingSwitch = nil // dropped by the next message that would carry it out: not shown as waiting meanwhile
	}
	if s.Launcher == "" {
		s.Launcher = n.inferLauncher(s.SessionID)
	}
	n.normalizeSessionRowModel(s) // keep the echoed model for display, clearing stand-in markers ("<synthetic>")
	n.applyLauncherContextWindow(s)
	if s.Title == "" && s.HerdrTabName() == "" {
		s.Title = filepath.Base(s.Cwd)
	}
	s.BypassAvailable = n.hooks.canBypass(s.SessionID) // modes.go claudeCanBypass
	n.modeRestartRow(s)                                // mode_restart.go
	n.ultracodeRow(s, pass)
	n.wakeEffortChange(s)
}

// decorateState restores metadata recorded when Persona started the session.
func (n *Node) decorateState(s *proto.Session, managesModel bool) (sessionState, bool) {
	st, ok := n.state.get(s.SessionID)
	if ok && isCodexWaitID(s.SessionID) && st.Launcher == "" {
		return sessionState{}, false // a manually discovered waiting row's pane binding is not an app start
	}
	if ok {
		s.StartedByPersona = true
		s.Launcher = st.Launcher
		s.OffLauncher = n.offLauncher(s.SessionID) // its claude not started by Persona's command (offlauncher.go)
		if st.TaskExecution {
			classifyTaskTurnFailure(s)
		}
		if managesModel {
			model := st.Model
			s.RequestedModel = &model
		}
		// A row without a pane is listed where its record says it started, when that is an assistant or health
		// directory: not where a `cd` in its transcript left it (#2368). A pane's directory is its host's.
		if identity.InHealthSpace(st.Cwd) {
			s.Space = identity.SpaceHealth
		}
		if s.Cwd == "" || s.PaneID == "" && identity.IsAssistantDir(st.Cwd) {
			s.Cwd = st.Cwd
		}
	}
	return st, ok
}

func snippet(s string) string {
	const most = 200
	s = wire.AssistantCardText(s)
	// One pass instead of strings.Fields, Join and []rune over the whole reply: runs of white space become one space,
	// leading and trailing space go, and the walk stops once there is more than most runes to show.
	var out strings.Builder
	out.Grow(min(len(s), most*utf8.UTFMax))
	runes, space := 0, false
	for i, r := range s {
		if unicode.IsSpace(r) {
			space = out.Len() > 0
			continue
		}
		if space {
			if runes == most {
				return truncatedSnippet(out.String())
			}
			out.WriteByte(' ')
			runes++
			space = false
		}
		if runes == most {
			return truncatedSnippet(out.String())
		}
		_, n := utf8.DecodeRuneInString(s[i:])
		out.WriteString(s[i : i+n])
		runes++
	}
	return out.String()
}

// truncatedSnippet ends a cut snippet with "…". Like the string([]rune) it replaces, a cut snippet has each
// invalid UTF-8 byte as U+FFFD; an uncut one keeps the bytes it had.
func truncatedSnippet(s string) string {
	if !utf8.ValidString(s) {
		s = string([]rune(s))
	}
	return s + "…"
}

const (
	pollFailBackoffFirst = 2 * time.Second
	pollFailBackoffMax   = 30 * time.Second
)

// hostFailBackoff throttles repeated snapshot attempts for a host that is failing to answer (for example herdr on a
// computer where it is not installed or not running). A watcher kick or a successful snapshot resets the host at once.
type hostFailBackoff struct {
	gap     map[string]time.Duration
	retryAt map[string]time.Time
}

func newHostFailBackoff() *hostFailBackoff {
	return &hostFailBackoff{gap: map[string]time.Duration{}, retryAt: map[string]time.Time{}}
}

func (b *hostFailBackoff) skip(kind string, now time.Time, kicked bool) bool {
	if kicked {
		delete(b.gap, kind)
		delete(b.retryAt, kind)
		return false
	}
	until, ok := b.retryAt[kind]
	return ok && now.Before(until)
}

func (b *hostFailBackoff) record(kind string, failed bool, now time.Time) {
	if !failed {
		delete(b.gap, kind)
		delete(b.retryAt, kind)
		return
	}
	next := min(max(2*b.gap[kind], pollFailBackoffFirst), pollFailBackoffMax)
	b.gap[kind] = next
	b.retryAt[kind] = now.Add(next)
}

// pollAgents refreshes the hosts' state every second (only the ones due) and pushes sessions.changed when anything
// visible changes. Each watcher host is snapshotted on its own stream's changes and at most every slowSnapshot while
// its stream is live, independently of the other hosts (the combined engine): a herdr-core event never snapshots
// herdr and vice versa.
func (n *Node) pollAgents(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	last := map[string]time.Time{} // host kind → its last snapshot
	backoff := newHostFailBackoff()
	var listed time.Time
	for {
		sigs := n.signalSnapshot()
		kicked, ok := nextChange(ctx, t.C, sigs, listed)
		if !ok {
			return
		}
		now := time.Now()
		var due []host.Host
		for _, h := range n.hosts.All() {
			if backoff.skip(h.Kind(), now, kicked[h.Kind()]) {
				continue
			}
			if n.hostDue(h.Kind(), last[h.Kind()], kicked[h.Kind()]) {
				due = append(due, h)
			}
		}
		// The transcripts (revs) are checked every tick regardless; a host that does not answer (not installed or not
		// running) does not stop the list: runtimes, the assistant and the transcripts do not need it. Its agents stay
		// as last seen (refreshHosts).
		if len(due) > 0 {
			failed, _ := n.refreshHostsWithFailures(ctx, due...)
			now = time.Now()
			for _, h := range due {
				last[h.Kind()] = now
				backoff.record(h.Kind(), failed[h.Kind()], now)
			}
		}
		list := n.sessionList(ctx, false)
		listed = time.Now()
		if revs := n.publish(list); len(revs) > 0 {
			n.kickPorts() // a tool call ran: it may have started (or stopped) a server (ports.go)
		}
	}
}

// publish pushes a rebuilt list to the hub: the whole list when anything it shows changed, else only the revs that
// moved (a transcript grew: a few bytes instead of the list, so the app can pull those sessions). It returns the
// revs that moved. Runtimes (runtimes.go) publish through it too when their list changes. Nothing goes while the hub
// has yet to say whether botmux's sessions are listed (botmuxPending), nor a list the manager holds already, byte for
// byte (sessions_sent.go).
func (n *Node) publish(list []proto.Session) map[string]int64 {
	if n.botmuxPending() {
		return nil
	}
	n.mu.Lock()
	size := len(n.lastList)
	n.mu.Unlock()
	key := listKeySized(list, size)
	n.mu.Lock()
	changed := key != n.lastList
	n.lastList = key
	revs := n.revsChanged(list)
	n.mu.Unlock()
	if n.peer == nil {
		return revs
	}
	if changed {
		n.sendSessions(list)
	} else if len(revs) > 0 {
		n.sendRevs(revs)
	}
	return revs
}

// revsChanged records the list's revs and returns those that differ from the last pushed. Callers hold n.mu.
func (n *Node) revsChanged(list []proto.Session) map[string]int64 {
	var out map[string]int64
	next := make(map[string]int64, len(list))
	for _, s := range list {
		next[s.SessionID] = s.Rev
		if s.Rev != 0 && n.lastRevs[s.SessionID] != s.Rev {
			if out == nil {
				out = map[string]int64{}
			}
			out[s.SessionID] = s.Rev
		}
	}
	n.lastRevs = next
	return out
}

// listBlocked: the live list leaves session sid out for its working folder cwd: a blocked Agent folder or Claude config
// folder, except the assistant's own conversations and the sessions the hub keeps.
func (n *Node) listBlocked(sid, cwd string) bool {
	return cwd != "" && n.cwdBlocked(cwd) && !n.isAssistantSession(sid) && !n.keptSessions()[sid]
}
