package node

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nwverse/persona/server/internal/herdr"
	"github.com/nwverse/persona/server/internal/herdrcore"
	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/orcart"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rexcli"
	"github.com/nwverse/persona/server/internal/rpc"
	"github.com/nwverse/persona/server/internal/terncli"
)

// A machine's node drives a set of terminal hosts side by side (#2576, proto/enginehosts.go): any of herdr (the
// default, as every node did before), herdr-core (the Herdr desktop app), orca (the Orca desktop app) and paseo (a
// runtime, not a terminal), or none — with one of them the default, where pane-less operations (new sessions,
// continuing one, renaming a space) go. The user picks them per machine in the app; the hub stores them and tells
// the node with engine.use when it connects and when they change (docs/protocol/protocol.md "Engines"). Only the hosts in the
// set are registered, and each runs isolated from the others: refreshes in parallel, each watcher refreshing only its
// own host (placement.go, herdrevents.go), so one host that is slow or down does not hold up another.

// engineState is the engine in use and the change stream watching it.
type engineState struct {
	mu   sync.Mutex
	name string              // the single engine closest to set ("" until engine.use: herdr)
	set  *proto.EngineChoice // the hosts and the default host; nil: the set name stands for (tests set only name)
	base context.Context     // Serve's context, nil before it
	stop context.CancelFunc  // ends the current engine's watcher

	use   sync.Mutex    // one engine.use at a time, applied whole (the last one to arrive wins)
	ready chan struct{} // closed once the hub said which engine (configured)
	once  sync.Once

	// botmux: whether botmux's sessions are listed next to the engine's (useBotmux); nil until the hub said, or
	// listed sessions without saying. botmuxSaid is the same "not nil", read without the lock (botmuxPending).
	botmux     *bool
	botmuxSaid atomic.Bool
}

// configWait is how long a new connection waits for the hub to say which engine before it starts on herdr: a hub
// that stores a choice sends engine.use first; one without a choice (or an old hub) lists sessions, which counts too.
var configWait = 3 * time.Second

func (n *Node) readyCh() chan struct{} {
	n.eng.mu.Lock()
	defer n.eng.mu.Unlock()
	if n.eng.ready == nil {
		n.eng.ready = make(chan struct{})
	}
	return n.eng.ready
}

// configured: the engine is settled (engine.use, or the hub listing sessions without it): polling may start.
func (n *Node) configured() {
	ch := n.readyCh()
	n.eng.once.Do(func() { close(ch) })
}

// startWhenConfigured starts polling and the engine's change stream once the engine is settled, or after configWait:
// a node never polls or watches herdr on a machine whose engine is herdr-core, not even for the first seconds.
func (n *Node) startWhenConfigured(ctx context.Context) {
	select {
	case <-n.readyCh():
	case <-time.After(configWait):
	case <-ctx.Done():
		return
	}
	go n.herdrAtStart(ctx)
	go n.pollAgents(ctx)
	go n.watchCodexCards(ctx)
	n.watchEngine(ctx)
}

// engineHosts are the hosts of a single engine (the set it stands for, engineHostsFor).
func engineHosts(engine string, logf func(format string, args ...any)) []host.Host {
	return engineHostsFor(proto.EngineHostsOf(engine), logf)
}

// engineHostsFor are the terminal hosts of a set, in its canonical order: herdr at its default socket (what every
// node had before engines), herdr-core at the socket hc would use, Orca's runtime, Tern's command line. Paseo is no terminal host: its
// sessions are its runtime's (runtimecontrol.go), run while the set holds it (engine.use in node.go). The empty set has
// none. The hosts stay isolated from each other — their refreshes run in parallel and each watcher only refreshes its
// own host (placement.go, herdrevents.go).
func engineHostsFor(c proto.EngineChoice, logf func(format string, args ...any)) []host.Host {
	var hs []host.Host
	for _, kind := range c.Hosts {
		switch kind {
		case proto.EngineHerdr:
			hc := herdr.New()
			hc.Log = logf
			if own := herdr.OwnSession(); logf != nil && own != "" {
				if hc.Session == own {
					logf("herdr: session %s (%s): this node's data directory has a herdr server of its own", own, hc.Socket)
				} else { // HERDR_SESSION / HERDR_SOCKET_PATH in the node's environment (a node started in a herdr pane has them)
					logf("herdr: this node's data directory has its own herdr session (%s), but its environment points at %s: using that, so its Agents belong to whichever node started that server", own, hc.Socket)
				}
			}
			hs = append(hs, newHerdrHost(hc, logf))
		case proto.EngineHerdrCore:
			cc := herdrcore.New()
			cc.Log = logf
			hs = append(hs, &herdrCoreHost{c: cc})
		case proto.EngineOrca:
			oc := orcart.New()
			oc.Log = logf
			hs = append(hs, newOrcaHost(oc))
		case proto.EngineTern:
			tc := ternClient()
			tc.Log = logf
			hs = append(hs, newTernHost(tc))
		case proto.EngineRex:
			rc := rexClient()
			rc.Log = logf
			hs = append(hs, newRexHost(rc))
		}
	}
	return hs
}

// defaultHostKind is the default host of a single engine: the engine itself, except for the combined engine, whose
// full-capability host is classic herdr.
func defaultHostKind(engine string) string { return proto.EngineHostsOf(engine).Default }

// engine is the single engine closest to the set in use (node.info's engine, user-facing names).
func (n *Node) engine() string {
	n.eng.mu.Lock()
	defer n.eng.mu.Unlock()
	return n.engineLocked()
}

func (n *Node) engineLocked() string {
	if n.eng.set != nil {
		return proto.LegacyEngine(*n.eng.set)
	}
	if n.eng.name == "" {
		return proto.EngineHerdr
	}
	return n.eng.name
}

// engineChoice is the set in use: what engine.use said, herdr alone before it.
func (n *Node) engineChoice() proto.EngineChoice {
	n.eng.mu.Lock()
	defer n.eng.mu.Unlock()
	return n.engineChoiceLocked()
}

func (n *Node) engineChoiceLocked() proto.EngineChoice {
	if n.eng.set != nil {
		return *n.eng.set
	}
	return proto.EngineHostsOf(n.engineLocked())
}

// hostEnabled: the set in use holds the host (or runtime engine) kind.
func (n *Node) hostEnabled(kind string) bool { return slices.Contains(n.engineChoice().Hosts, kind) }

// defaultKind is the default host of the set in use: where pane-less operations go; "" when the set is empty.
func (n *Node) defaultKind() string { return n.engineChoice().Default }

// noEngineRefusal is why nothing can start on a machine that enables no terminal engine (the empty set).
func noEngineRefusal() error {
	return refusal(engineMissing, "这台电脑没启用终端引擎，不能从手机新开或接着会话，这次没有发送。"+
		"在 设置 › 机器 › 这台电脑 › 引擎 里启用一个（herdr、Herdr App、Orca、Paseo 或 Cindy）再试。", nil)
}

// watchEngine starts watching the engine in use for as long as ctx lasts (Serve); engine.use moves it to the new one.
func (n *Node) watchEngine(ctx context.Context) {
	n.eng.mu.Lock()
	defer n.eng.mu.Unlock()
	n.eng.base = ctx
	n.startWatchLocked()
}

// startWatchLocked ends the current engine's change stream and starts the one of the engine in use: herdr's own
// (watchHerdr, as before engines), or the stream of a host that pushes changes (watchHost). n.eng.mu is held.
func (n *Node) startWatchLocked() {
	if n.eng.stop != nil {
		n.eng.stop()
		n.eng.stop = nil
	}
	if n.eng.base == nil {
		return // not serving (tests drive the node without Serve)
	}
	ctx, cancel := context.WithCancel(n.eng.base)
	n.eng.stop = cancel
	// Each registered watcher gets its own stream: herdr's pane-aware subscription, every other host's whole-host
	// stream (watchHost). The combined engine starts both; one host's events never refresh the other host.
	for _, h := range n.hosts.All() {
		w, ok := h.(host.Watcher)
		if !ok {
			continue
		}
		if h.Kind() == host.Herdr {
			go n.watchHerdr(ctx)
			go n.watchFocus(ctx)
		} else {
			go n.watchHost(ctx, w, n.hostSig[h.Kind()]) // resetSignals built one for every Watcher host
		}
	}
}

// useEngine switches the node to engine: the old host's agents, places and caches go, its change stream stops, the
// new engine's starts, and a fresh session list goes to the hub once built (in the background: the reply does not wait
// for it). The engine in use already: nothing happens.
//
// Switches take turns (n.eng.use). A refresh running meanwhile drops its result (n.engEpoch), and every terminal
// stream of the old engine ends with terminal.end, so the phone does not hold one of its panes.
func (n *Node) useEngine(ctx context.Context, engine string) error {
	if _, ok := proto.EngineNames[engine]; !ok {
		return fmt.Errorf("unknown engine %q", engine)
	}
	return n.useEngineChoice(ctx, proto.EngineHostsOf(engine))
}

// useEngineChoice switches the node to a set of hosts (useEngine). Only the default changed: nothing is reset, the
// pane-less operations just go elsewhere from now on. The hosts changed: as a switch of engine.
func (n *Node) useEngineChoice(ctx context.Context, c proto.EngineChoice) error {
	c, err := proto.NormalizeEngineChoice(c.Hosts, c.Default)
	if err != nil {
		return err
	}
	n.eng.use.Lock()
	defer n.eng.use.Unlock()
	cur := n.engineChoice()
	if proto.SameEngineChoice(cur, c) {
		return nil
	}
	if slices.Equal(cur.Hosts, c.Hosts) {
		n.eng.mu.Lock()
		n.eng.set, n.eng.name = &c, proto.LegacyEngine(c)
		n.eng.mu.Unlock()
		n.logf("engine: default host now %q", c.Default)
		return nil
	}
	engine := proto.LegacyEngine(c)
	n.eng.mu.Lock()
	n.hosts.Set(nodeHostsFor(c, n.eng.botmux != nil && *n.eng.botmux, n.logf)...)
	n.resetSignals()
	n.eng.name, n.eng.set = engine, &c
	n.mu.Lock()
	n.engEpoch++
	epoch := n.engEpoch
	n.hostAgents, n.layouts, n.agents, n.panePIDs, n.lastList = nil, nil, map[string]host.Agent{}, map[string]panePID{}, ""
	n.lastPanes, n.onAgentsList = nil, nil
	n.mu.Unlock()
	n.sent.forget() // the new engine's list goes once even when the old one's was the same (sessions_sent.go)
	n.endTermStreams("the machine's terminal engine changed")
	traexCache.Lock()
	traexCache.panes, traexCache.status = map[int]string{}, map[string]traexState{}
	traexCache.Unlock()
	n.startWatchLocked()
	n.eng.mu.Unlock()
	n.logf("engine: now %v (default %q)", c.Hosts, c.Default)

	// The new engine's list goes to the hub when it is ready, not before the reply: building it asks the engine, scans
	// processes and reads every agent's transcript, which on a machine with dozens of agents and cold caches took
	// longer than the hub's 30 s for engine.use, and the app's engine picker waits on that call (#635). Built outside
	// n.eng.use, so a list finished after a later switch is dropped: it would cover the newer engine's until the next
	// poll (the hub would see sessions vanish and finish).
	// On the connection's ctx, not engine.use's: that one ends with the reply (#2645), which is before the list is built.
	n.engLists.Add(1)
	go func() {
		defer n.engLists.Done()
		list := n.sessionList(rpc.ConnContext(ctx), true)
		n.mu.Lock()
		current := n.engEpoch == epoch
		n.mu.Unlock()
		if current {
			n.publish(list)
		}
	}()
	return nil
}

// endTermStreams stops every terminal stream and tells the hub why (terminal.end), as when one ends on its own.
func (n *Node) endTermStreams(why string) {
	n.mu.Lock()
	terms := n.terms
	n.terms = nil
	n.mu.Unlock()
	for id, s := range terms {
		s.mu.Lock()
		control := s.ctl != nil
		s.mu.Unlock()
		s.cancel()
		if n.peer != nil {
			_ = n.peer.Notify("terminal.end", map[string]any{"session_id": id, "error": why, "control": control})
		}
	}
}

// herdrCLIVersion is `herdr --version` ("" without herdr): the version without talking to a running herdr.
func herdrCLIVersion(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "herdr", "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "herdr"))
}

func (n *Node) logf(format string, args ...any) {
	if n.logger != nil {
		n.logger.Printf(format, args...)
	}
}

// engines is what node.info reports of each engine on this machine. herdr is available when it answers, its socket
// file is there or its binary is on PATH — when herdr is not a registered host it is not asked: its version comes from
// `herdr --version`. herdr-core when its socket answers hello (the one herdr-core call a node without it makes, once
// per connection). The combined engine when both are there. Orca when its runtime metadata is there and its socket
// answers status.get with a protocol Persona speaks (Note says why not when it answers with another). herdrVersion is
// what the registered herdr host already said ("" when not registered).
func (n *Node) engines(ctx context.Context, herdrVersion string) []proto.Engine {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	hv := herdrVersion
	if hv == "" && n.hosts.Get(host.Herdr) == nil {
		hv = herdrCLIVersion(ctx)
	}
	_, pathErr := exec.LookPath("herdr")
	hcv := herdrcore.New().Version(ctx)
	herdrAvailable := hv != "" || herdr.SocketUp() || pathErr == nil
	coreAvailable := hcv != ""
	return []proto.Engine{
		{ID: proto.EngineHerdr, Name: proto.EngineNames[proto.EngineHerdr], Available: herdrAvailable, Version: hv},
		{ID: proto.EngineHerdrCore, Name: proto.EngineNames[proto.EngineHerdrCore], Available: coreAvailable, Version: hcv},
		// The combined choice is selectable only when both runtimes are there; its version is left out (the picker
		// already shows each runtime's version on its own row).
		{ID: proto.EngineHerdrBoth, Name: proto.EngineNames[proto.EngineHerdrBoth], Available: herdrAvailable && coreAvailable},
		orcaEngine(ctx, orcaClient()),
		ternEngine(ctx, ternClient()),
		rexEngine(ctx, rexClient()),
		n.paseoEngine(ctx),
		n.cindyEngine(ctx),
	}
}

// ternClient is the tern command line the Tern host drives and node.info asks; tests point it at a fake one.
var ternClient = terncli.New

// ternEngine is Tern as found on this machine: available when its binary is there and its session daemon answers
// (`tern inspect`); the version is the binary's, also while Tern is not running.
func ternEngine(ctx context.Context, c *terncli.Client) proto.Engine {
	e := proto.Engine{ID: proto.EngineTern, Name: proto.EngineNames[proto.EngineTern]}
	if c.Bin() == "" {
		return e
	}
	e.Version = c.Version(ctx)
	if _, err := c.Inspect(ctx, ""); err == nil {
		e.Available = true
	}
	return e
}

// rexClient is the rex command line the Rex host drives and node.info asks; tests point it at a fake one.
var rexClient = rexcli.New

// rexEngine is Rex as found on this machine: available when its binary is there and its server answers
// (`server.status` with --autostart=false); the version is the binary's, also while Rex is not running.
func rexEngine(ctx context.Context, c *rexcli.Client) proto.Engine {
	e := proto.Engine{ID: proto.EngineRex, Name: proto.EngineNames[proto.EngineRex]}
	if c.Bin() == "" {
		return e
	}
	e.Version = c.Version(ctx)
	if err := c.Status(ctx); err == nil {
		e.Available = true
	}
	return e
}

// orcaClient is the Orca runtime node.info asks; tests point it at a fake one.
var orcaClient = orcart.New

// orcaEngine is Orca as found on this machine: available when its runtime answers with a protocol Persona speaks.
func orcaEngine(ctx context.Context, c *orcart.Client) proto.Engine {
	e := proto.Engine{ID: proto.EngineOrca, Name: proto.EngineNames[proto.EngineOrca]}
	st, err := c.Probe(ctx)
	if st != nil {
		e.Version = st.AppVersion
	}
	switch {
	case err == nil:
		e.Available = true
	case errors.Is(err, orcart.ErrIncompatible):
		e.Note = "这个版本的 Orca 和 Persona 不兼容，请把两边都更新到最新版"
	}
	return e
}
