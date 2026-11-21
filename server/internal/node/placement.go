package node

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/proto"
)

// errEngineChanged: the engine was switched while a refresh ran; its result is the old engine's and is dropped.
var errEngineChanged = errors.New("the engine changed during the refresh")

// errNoHost: the node has no terminal host to look at.
var errNoHost = errors.New("no terminal host")

// layout remembers a host's space and slot names from its last snapshot.
type layout struct {
	spaces map[string]host.Space
	slots  map[string]host.Slot
	// panes is every pane of the snapshot, agent or not: the terminal browser lists shells from it (panes.go).
	panes []host.Agent
	// paneIndex is each pane's place in its slot (host.Layout.PaneIndex), nil when the host does not say.
	paneIndex map[string]int
	at        time.Time
}

// refreshHosts takes one snapshot of every host, or of only the hosts given: agents plus the space / slot names shown
// as space / slot. The hosts are asked concurrently, each bounded by its own client's timeouts, so a slow host never
// holds up the other host of a combined engine. It fails only when no host answered; a host that did not answer keeps
// the agents it gave last. A host that is not running (host.ErrNotRunning: herdr-core without its socket) answered: it
// has no agents.
func (n *Node) refreshHosts(ctx context.Context, only ...host.Host) error {
	return n.refreshHostInventoryWithFailures(ctx, false, nil, only...)
}

// refreshHostsWithFailures refreshes only (or every host when empty) and reports which host kinds failed to answer.
// host.ErrNotRunning counts as an answer (not a failure).
func (n *Node) refreshHostsWithFailures(ctx context.Context, only ...host.Host) (map[string]bool, error) {
	failed := map[string]bool{}
	err := n.refreshHostInventoryWithFailures(ctx, false, failed, only...)
	return failed, err
}

// refreshHostInventory retains the display's tolerant refresh by default. An
// update evidence producer must request every host and reject cached fallbacks.
func (n *Node) refreshHostInventory(ctx context.Context, strict bool, only ...host.Host) error {
	return n.refreshHostInventoryWithFailures(ctx, strict, nil, only...)
}

func (n *Node) refreshHostInventoryWithFailures(ctx context.Context, strict bool, failed map[string]bool, only ...host.Host) error {
	if n.sessionScope != nil {
		if strict {
			return errors.New("scoped node cannot attest complete host inventory")
		}
		return nil
	}
	n.mu.Lock()
	epoch := n.engEpoch // a refresh that outlives an engine switch must not bring the old engine's agents back
	n.mu.Unlock()
	all := n.hosts.All()
	if len(all) == 0 {
		return errNoHost
	}
	targets := only
	if len(targets) == 0 {
		targets = all
	}
	type outcome struct {
		kind   string
		agents []host.Agent
		full   bool
		err    error
	}
	outs := make([]outcome, len(targets))
	var wg sync.WaitGroup
	for i, h := range targets {
		wg.Add(1)
		go func(i int, h host.Host) {
			defer wg.Done()
			agents, full, err := n.lookAt(ctx, h, epoch)
			outs[i] = outcome{kind: h.Kind(), agents: agents, full: full, err: err}
		}(i, h)
	}
	wg.Wait()
	if strict {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, o := range outs {
			if o.err != nil && !errors.Is(o.err, host.ErrNotRunning) {
				return fmt.Errorf("incomplete host inventory %s: %w", o.kind, o.err)
			}
			if o.err == nil && !o.full {
				return fmt.Errorf("incomplete host inventory %s: full snapshot unavailable", o.kind)
			}
		}
		if len(targets) != len(all) {
			return errors.New("partial host refresh cannot attest complete inventory")
		}
		for _, h := range all {
			found := false
			for _, target := range targets {
				found = found || h == target
			}
			if !found {
				return errors.New("host inventory target set differs from configured hosts")
			}
		}
	}

	n.mu.Lock()
	if n.engEpoch != epoch {
		n.mu.Unlock()
		return errEngineChanged
	}
	if n.hostAgents == nil {
		n.hostAgents = map[string][]host.Agent{}
	}
	answered, snapped, firstErr := false, false, error(nil)
	heard := map[string]bool{} // the hosts that answered: the merge asks only their panes what they run (setAgentsAt)
	for _, o := range outs {
		if o.err != nil && errors.Is(o.err, host.ErrNotRunning) {
			o.err, o.agents = nil, nil // an answer: nothing runs there
		}
		if o.err != nil {
			if firstErr == nil {
				firstErr = o.err
			}
			if failed != nil {
				failed[o.kind] = true
			}
			o.agents = n.hostAgents[o.kind]
		} else {
			answered, snapped = true, snapped || o.full
			n.hostAgents[o.kind] = o.agents
			heard[o.kind] = true
		}
	}
	union := make([]host.Agent, 0)
	for _, agents := range n.hostAgents { // hosts not in this partial refresh keep what they gave last
		union = append(union, agents...)
	}
	n.mu.Unlock()

	if !answered {
		return firstErr
	}
	if !n.setAgentsAt(union, epoch, heard) {
		return errEngineChanged
	}
	n.backfillResume(union) // herdrresume.go
	if snapped {
		n.panesChanged()
	}
	return nil
}

// lookAt is one host's agents: from a snapshot (full), which also gives the space / slot names; a host that cannot
// snapshot (older herdr without `api snapshot`) gives its agent list instead.
func (n *Node) lookAt(ctx context.Context, h host.Host, epoch uint64) (agents []host.Agent, full bool, err error) {
	snap, err := h.Snapshot(ctx)
	if err != nil {
		agents, aerr := h.Agents(ctx)
		if aerr != nil {
			return nil, false, err
		}
		return agents, false, nil
	}
	l := layout{spaces: map[string]host.Space{}, slots: map[string]host.Slot{}, panes: snap.Panes, paneIndex: snap.PaneIndex, at: time.Now()}
	for _, w := range snap.Spaces {
		l.spaces[w.ID] = w
	}
	for _, t := range snap.Slots {
		l.slots[t.ID] = t
	}
	n.mu.Lock()
	if n.engEpoch != epoch {
		n.mu.Unlock()
		return nil, false, errEngineChanged
	}
	if n.layouts == nil {
		n.layouts = map[string]layout{}
	}
	n.layouts[h.Kind()] = l
	n.mu.Unlock()
	// Panes running an agent the host does not detect (a compatible CLI) become agents of our own.
	agents = append(snap.Agents, n.traexAgents(ctx, snap.Panes, snap.Agents)...)
	// Retain active Claude agents when Herdr temporarily drops agent detection during tool or subshell execution (#1641).
	return append(agents, n.claudeGraceAgents(snap.Panes, agents)...), true, nil
}

// claudeGraceAgents recovers live Claude agents whose terminal panes are still open in snap.Panes
// but were temporarily omitted from snap.Agents because the host's foreground process detection
// flapped to a child tool process or subshell (#1641).
func (n *Node) claudeGraceAgents(panes, agents []host.Agent) []host.Agent {
	if len(panes) == 0 {
		return nil
	}
	hasAgent := make(map[string]bool, len(agents))
	claimedSessions := make(map[string]bool, len(agents))
	for _, a := range agents {
		if a.PaneID != "" {
			hasAgent[a.PaneID] = true
		}
		if id := a.SessionID(); id != "" {
			claimedSessions[id] = true
		}
	}

	panesByID := make(map[string]host.Agent, len(panes))
	for _, p := range panes {
		if p.PaneID != "" {
			panesByID[p.PaneID] = p
		}
	}

	claudes := n.liveClaudes()
	liveSessions := make(map[string]bool, len(claudes))
	for _, c := range claudes {
		if c.SessionID != "" && c.Kind != "bg" {
			liveSessions[c.SessionID] = true
		}
	}
	if len(liveSessions) == 0 {
		return nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	var out []host.Agent
	for id, prev := range n.agents {
		if prev.Agent != "claude" || prev.PaneID == "" {
			continue
		}
		if hasAgent[prev.PaneID] || claimedSessions[id] {
			continue
		}
		if !liveSessions[id] {
			continue
		}
		p, ok := panesByID[prev.PaneID]
		if !ok {
			continue
		}
		retained := prev
		retained.WorkspaceID = p.WorkspaceID
		retained.TabID = p.TabID
		if p.Cwd != "" {
			retained.Cwd = p.Cwd
		}
		if p.Title != "" {
			retained.Title = p.Title
			retained.RawTitle = p.RawTitle
		}
		if p.Tokens != nil {
			retained.Tokens = p.Tokens
		}
		if retained.AgentSession == nil {
			retained.AgentSession = &host.AgentSession{Kind: "id", Value: id}
		}
		if retained.AgentStatus == "" {
			retained.AgentStatus = proto.StatusWorking
		}
		out = append(out, retained)
		hasAgent[prev.PaneID] = true
		claimedSessions[id] = true
	}
	return out
}

// placeIn fills the space / slot fields of a live session.
func (n *Node) placeIn(s *proto.Session, a host.Agent) {
	kind := n.hosts.Ref(a.PaneID).Kind
	n.mu.Lock()
	l := n.layouts[kind]
	w, wok := l.spaces[a.WorkspaceID]
	t, tok := l.slots[a.TabID]
	pi, piok := l.paneIndex[a.PaneID]
	n.mu.Unlock()
	s.SpaceKind, s.SpaceID, s.SlotID = kind, a.WorkspaceID, a.TabID
	if wok {
		s.SpaceName, s.SpaceIndex, s.ParentSpaceID = w.Label, int32(w.Number), w.ParentID
	}
	if tok {
		s.SlotName, s.SlotIndex = t.Label, int32(t.Number)
	}
	s.PaneIndex = nil
	if piok {
		s.PaneIndex = &pi
	}
	if a.Tokens != nil && a.Tokens["persona_from"] != "" {
		s.SpawnFrom = a.Tokens["persona_from"]
	}
}

// unplaced: a claude in a terminal (not a background session, not parked) runs a session no pane is named after.
func unplaced(procs []claudeProc, agents []host.Agent) bool {
	named := map[string]bool{}
	for _, a := range agents {
		if id := a.SessionID(); id != "" {
			named[id] = true
		}
	}
	for _, c := range procs {
		if c.Kind == "interactive" && c.ParkedJobID == "" && validID(c.SessionID) && !named[c.SessionID] {
			return true
		}
	}
	return false
}

// setAgents maps live claude agents to session ids: the host's own report when it knows (herdr's claude integration),
// else what our hooks told us about the pane, else the newest transcript in the agent's project directory.
func (n *Node) setAgents(agents []host.Agent) {
	n.mu.Lock()
	epoch := n.engEpoch
	n.mu.Unlock()
	n.setAgentsAt(agents, epoch, nil)
}

// setAgentsAt is setAgents for a refresh of engine epoch: false (nothing set) when the engine changed since. heard is
// the hosts that answered the refresh (nil: every host): only their panes' processes are asked for. The merge covers
// every host's agents, and asking all of them tied each host's refresh to the others' I/O (#5816): Orca's poll every
// second asked herdr, and herdr's refresh ran Orca's process scan (three ps runs, about 0.2 s on a Mac). A pane of
// another host keeps the pid its own refresh found (sessionFromKnownProcess), and is looked at again only by that
// host's refresh. Ownership's rule 1 still asks every candidate's host (resolve).
func (n *Node) setAgentsAt(agents []host.Agent, epoch uint64, heard map[string]bool) bool {
	fromProcess := func(pane string) string {
		if heard == nil || heard[host.ParsePane(pane).Kind] {
			return n.sessionFromProcess(pane)
		}
		return n.sessionFromKnownProcess(pane)
	}
	m, onList := map[string]host.Agent{}, map[string]bool{}
	var unknown, codex, family, opencode, botmux []host.Agent
	other := map[string]host.Agent{} // terminal-only agents by their row id (opencode.go)
	contested := contest{}           // sessions live in two hosts at once: an owner is picked below (ownership.go)
	procs := n.liveClaudes()
	parked := parkedClaudeSessions(procs)    // claudebg.go
	botmuxThreads := n.botmuxThreads(agents) // hostbotmux.go
	// What the claude in each pane says it runs, for the panes herdr names a session for: herdr takes the session from
	// a SessionStart hook that reported HERDR_PANE_ID, and a claude elsewhere that inherited the variable (botmux's
	// tmux server, first started in a herdr pane, 2026-11-04) reports into the pane too. The pane's own claude wins.
	// Asked only while a terminal claude runs a session herdr names for no pane (the pane's own one, then): one
	// process lookup per pane, kept a minute.
	own := map[string]string{}
	if unplaced(procs, agents) {
		for _, a := range agents {
			if a.Agent == "claude" && a.SessionID() != "" && a.PaneID != "" {
				if id := fromProcess(a.PaneID); id != "" {
					own[a.PaneID] = id
				}
			}
		}
	}
	n.mu.Lock()
	for _, a := range agents {
		if (a.Agent == "codex" || a.Agent == traexAgent) && host.ParsePane(a.PaneID).Kind == host.Botmux {
			botmux = append(botmux, a) // after the panes that run their thread themselves
			continue
		}
		if a.Agent == "codex" {
			codex = append(codex, a)
			continue
		}
		if a.Agent == cursorAgent || a.Agent == agyAgent { // a work agent herdr detects (workagent.go): its own row
			if id := otherSession(a); id != "" {
				other[id] = a
			}
			continue
		}
		if piFamily(a.Agent) { // its session: herdr's integration, or the extension in it (pi.go, pifamily.go)
			family = append(family, a)
			continue
		}
		if a.Agent == opencodeAgent { // its session: herdr's opencode plugin, or the Persona plugin in the server (opencode.go)
			opencode = append(opencode, a)
			continue
		}
		if a.Agent == traexAgent { // found by its process, which names its session (the compatible CLI integration)
			if id := a.SessionID(); id != "" {
				m[id] = a
			}
			continue
		}
		if a.Agent != "claude" {
			// An agent nothing here reads is still an agent the user runs: listed with its terminal (opencode.go).
			if otherAgent(a) {
				if id := otherSession(a); id != "" {
					other[id] = a
				}
			}
			continue
		}
		if id := a.SessionID(); id != "" {
			if p := own[a.PaneID]; p != "" && p != id {
				id = p // reported by a claude outside the pane (above)
			}
			if j, ok := parked[id]; ok {
				// its claude sent the session to the background: the pane shows Claude's agents view, or the job
				if bg := j.shownIn(a); bg != "" {
					if !liveTaken(m, bg) {
						m[bg] = a
					}
				} else if !liveTaken(m, id) {
					m[id], onList[id] = onAgentsList(a), true
				}
				continue
			}
			if liveTaken(m, id) && a.AgentStatus == proto.StatusOffline {
				continue // the same session runs in another host's pane (botmux's parked row, herdr's resume)
			}
			if crossHost(m, id, a) {
				contested.add(id, m[id], a) // live in another host too: its owner is picked below
				continue
			}
			m[id] = a
			continue
		}
		unknown = append(unknown, a)
	}
	n.notePanes(unknown)
	n.mu.Unlock()
	var omps []familyNote
	for _, a := range family {
		id := n.piSession(a)
		if a.Agent == ompAgent && a.PaneID != "" {
			omps = append(omps, familyNote{pane: a.PaneID, session: id}) // the panes that may write a file ompdisk.go lists
		}
		if id == "" {
			id = otherSession(a) // nothing reported its session yet: the pane is the row, terminal only (pi.go)
		}
		if id != "" {
			if _, taken := m[id]; !taken {
				m[id] = a
			}
		}
	}
	n.noteFamilyPanes(omps)
	for _, a := range opencode {
		id := n.opencodeSession(a)
		if id == "" {
			id = otherSession(a) // herdr reported no session id (an older integration): the pane is the row, terminal only
		}
		if id != "" {
			if _, taken := m[id]; !taken {
				m[id] = a
			}
		}
	}
	for id, a := range other {
		if _, taken := m[id]; !taken {
			m[id] = a
		}
	}
	for _, a := range unknown {
		// Without herdr's claude integration the pane's claude process says which session it runs. A pane binding
		// (our start, a hook) comes after it: the bindings of sessions Persona started are kept in the hub's
		// database and come back when node restarts, long after another claude took the pane (2026-10-31: after a
		// redeploy two panes showed sessions they had run days earlier). A guess (title, then newest transcript in
		// the directory) is the last resort — six agents in one repo got each other's tab names that way.
		id := fromProcess(a.PaneID)
		if j, ok := parked[id]; ok { // the same, as the pane's own claude tells it
			if bg := j.shownIn(a); bg != "" {
				if !liveTaken(m, bg) {
					m[bg] = a
				}
			} else if !liveTaken(m, id) {
				m[id], onList[id] = onAgentsList(a), true
			}
			continue
		}
		if id != "" && crossHost(m, id, a) { // its own claude runs a session another host's pane lists
			contested.add(id, m[id], a)
			continue
		}
		if id == "" {
			n.mu.Lock()
			id = n.panes[a.PaneID]
			n.mu.Unlock()
			if isCodexWaitID(id) {
				id = "" // a Codex that waited for its first message ran here (codex_waiting.go); this agent is another
			}
		}
		if id == "" || liveTaken(m, id) {
			if n.freshPane(a.PaneID) {
				continue // its claude has not written sessions/<pid>.json yet; the newest transcript is an older session
			}
			id = n.guessSession(a, m)
		}
		if id != "" {
			m[id] = a
		}
	}
	n.resolve(m, contested) // the owner of every session live in two hosts (ownership.go)
	// Codex panes: the thread named in the title (what the pane shows now: a /resume in Codex changes it), else what
	// herdr's Codex integration or our own start said. An unidentified Codex in a herdr tab has its own waiting row.
	var unnamed, manual []host.Agent
	for _, a := range codex {
		if w := n.codexWaitOf(a.PaneID); w != nil {
			// Opened with no first message (codex_waiting.go): its row until its thread is attached, which only the look
			// does. What herdr's integration and the pane's title name are noted for it, never taken as is.
			named, titled := "", n.codexTitleThread(a)
			if sid := a.SessionID(); sid != "" {
				home := n.codexPaneHome(a.PaneID)
				named = n.codexPublicID(home, mainThread(home, sid))
			}
			n.mu.Lock()
			w.herdr, w.titled = named, titled
			n.mu.Unlock()
			m[w.id] = a
			continue
		}
		home := n.codexPaneHome(a.PaneID)
		reported := a.SessionID() // what the host says the pane runs now, not what the node remembers of the pane
		id := reported
		if id != "" {
			id = mainThread(home, id) // a subagent's id stands for its conversation
		}
		sub := id != reported
		var bound string
		if id == "" {
			n.mu.Lock()
			bound = n.panes[a.PaneID]
			n.mu.Unlock()
			if isCodexWaitID(bound) {
				m[bound] = a // a concurrent refresh just opened the row; its watcher identifies the thread
				continue
			}
			id = bound
		}
		if named, claimed := n.codexByTitle(a, id, m); named != "" {
			id = named
		} else if claimed {
			id = "" // the title names another pane's thread: keeping the stale reported one would misroute this row
		} else if _, match, ok := codexTitleName(a.Title); ok && (reported == "" ||
			(!sub && threadTitle(home, id) != "" && !threadFormerlyNamed(home, id, match))) {
			// An unrecognised current title must not reuse a stale pane binding either, nor a thread the host named
			// that the index knows under another name (a /resume in Codex since). A reported thread the index has no
			// name for is not contradicted by a name the index lacks, so the host's word stands: left out, the pane
			// was listed nowhere and its thread only as its app-server's row, with no terminal (#7538). Nor is a
			// reported subagent contradicted by its conversation's name, which is not the name of the thread reported,
			// nor a renamed thread by the name the index gave it before (a rename on the app-server the title missed).
			id = ""
		}
		if id != "" {
			id = n.codexPublicID(home, id)
		}
		if _, taken := m[id]; id == "" || taken {
			kind := host.ParsePane(a.PaneID).Kind
			if kind == host.Herdr || kind == host.HerdrCore {
				if a.PaneID != "" && reported == "" && bound == "" && a.AgentStatus != proto.StatusOffline {
					manual = append(manual, a)
				}
			} else {
				unnamed = append(unnamed, a)
			}
			continue
		}
		m[id] = a
	}
	// botmux's Codex and compatible variants sessions, by the thread their rollout names (botmuxThreads), before any guess could take
	// that rollout for another pane. A parked one gives way to a pane that runs the same thread.
	for _, a := range botmux {
		id := botmuxThreads[a.PaneID]
		if id == "" || (a.AgentStatus == proto.StatusOffline && liveTaken(m, id)) {
			continue
		}
		m[id] = a
	}
	for _, a := range unnamed {
		n.mu.Lock()
		starting := n.codexStarting[a.PaneID]
		n.mu.Unlock()
		if starting {
			continue // a start of ours: its thread is not on disk yet, and an older one in the directory is not it
		}
		if id := n.codexSession(a, m); id != "" {
			m[id] = a
		}
	}
	// Session state moved to a pane's real id is written once mu is released: a write tells the hub (stateFile.persist).
	var moved []func()
	defer func() {
		for _, put := range moved {
			put()
		}
	}()
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.engEpoch != epoch {
		return false
	}
	for id, a := range m {
		if isCodexWaitID(id) && n.panes[a.PaneID] != id {
			// The look for threads won a concurrent refresh. Follow the current binding, including a pane that left.
			delete(m, id)
			if bound := n.panes[a.PaneID]; bound != "" && !liveTaken(m, bound) {
				m[bound] = a
			}
		}
	}
	for _, a := range manual {
		if n.codexStarting[a.PaneID] || n.state.isLoading() {
			continue // the app's start, or the manager's stored binding, has the identity for this pane
		}
		if bound := n.panes[a.PaneID]; bound != "" {
			if !liveTaken(m, bound) {
				m[bound] = a // another refresh already created its row or attached its thread
			}
			continue
		}
		w := n.watchManualCodexLocked(a)
		m[w.id] = a
		moved = append(moved, func() { n.saveManualCodexWait(w) })
	}
	// A pane that now runs another session than it did: rare, and the first thing to look at when a session drops off
	// the live list while its agent runs (#5463: a tmux worker took its manager's pane).
	was := map[string]string{}
	for id, a := range n.agents {
		if a.PaneID != "" {
			was[a.PaneID] = id
		}
	}
	for id, a := range m {
		if old := was[a.PaneID]; a.PaneID != "" && old != "" && old != id {
			log.Printf("agents: pane %s runs session %s now, not %s", a.PaneID, id, old)
		}
	}
	// A pane first listed by its fallback id (omp-w1-p2), then by its session: a client holding the fallback row keeps
	// working (pifamily.go).
	if n.idAlias == nil {
		n.idAlias = map[string]paneAlias{}
	}
	live := map[string]bool{}
	for id, a := range m {
		if a.PaneID == "" {
			continue
		}
		live[a.PaneID] = true
		if old := was[a.PaneID]; old != "" && old != id && !validID(old) && piFamily(a.Agent) {
			n.idAlias[old] = paneAlias{id: id, pane: a.PaneID}
			// The pane is the session's now: herdr's restore and a later send find the real id (herdrresume.go stays
			// out of a fallback id it cannot resume).
			n.panes[a.PaneID] = id
			if st, ok := n.state.get(old); ok {
				st.Pane = a.PaneID
				moved = append(moved, func() { n.state.put(id, st) })
			}
			if l, ok := n.launches[a.PaneID]; ok && l.ID == old {
				l.ID = id
				n.launches[a.PaneID] = l
			}
		}
	}
	for pane, thread := range n.codexManual {
		if m[thread].PaneID != pane {
			delete(n.codexManual, pane)
			if n.panes[pane] == thread {
				delete(n.panes, pane)
			}
		}
	}
	for fallback, alias := range n.idAlias {
		if !live[alias.pane] {
			delete(n.idAlias, fallback) // its pane is gone: nothing is left to point at
		}
	}
	// herdr's omp integration reports the session file: what resuming a stopped one needs (ompresume.go).
	if n.ompFiles == nil {
		n.ompFiles = map[string]string{}
	}
	for _, a := range agents {
		if a.Agent == ompAgent {
			if p := a.SessionPath(); p != "" {
				if id := a.SessionID(); id != "" {
					n.ompFiles[id] = p
				}
			}
		}
	}
	n.agents = m
	n.onAgentsList = onList
	if n.lastPanes == nil {
		n.lastPanes = map[string]string{}
	}
	for id, a := range m {
		n.lastPanes[id] = a.PaneID
	}
	return true
}

// liveTaken: a running agent already has session id in m. An offline one (a parked botmux session) gives way to the
// pane that runs it.
func liveTaken(m map[string]host.Agent, id string) bool {
	a, ok := m[id]
	return ok && a.AgentStatus != proto.StatusOffline
}
