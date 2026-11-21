package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/nwverse/persona/server/internal/i18n"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// Each machine's node drives a set of terminal hosts side by side (#2576, proto/enginehosts.go): any of herdr,
// herdr-core, orca and paseo, or none, with one of them the default — picked by the user in the app and kept here.
// Two settings keys: machine_engine_hosts (machine id → {hosts, default}) is the choice; machine_engines (machine id →
// a single engine) is written beside it with the single engine closest to the set, read for machines chosen before
// sets and by an older hub after a rollback. A machine in neither is herdr. The hub tells the node with engine.use
// when it connects (before the first session list) and when the user changes it (docs/protocol/protocol.md "Engine").

// enginesKey is the settings key holding map[machineID]engine (the single engine, before sets and for rollbacks).
const enginesKey = "machine_engines"

// engineHostsKey is the settings key holding map[machineID]proto.EngineChoice.
const engineHostsKey = "machine_engine_hosts"

// engineChoices is the choice of each machine, loaded from settings on first use.
type engineChoices struct {
	mu       sync.Mutex
	loaded   bool
	failedAt time.Time // the last load that failed: not tried again for engineRetry
	byID     map[string]string
	sets     map[string]proto.EngineChoice
}

// engineRetry: after settings could not be read, how long engineOf answers from what it has (herdr) before trying
// again — it is called while machine lists are built.
const engineRetry = 30 * time.Second

func (d *Devices) loadEnginesLocked(ctx context.Context) {
	e := &d.engines
	if e.loaded || !e.failedAt.IsZero() && time.Since(e.failedAt) < engineRetry {
		return
	}
	e.byID, e.sets = map[string]string{}, map[string]proto.EngineChoice{}
	if d.opts.Store == nil {
		e.loaded = true
		return
	}
	var m map[string]string
	var sets map[string]proto.EngineChoice
	_, err := d.opts.Store.Setting(ctx, enginesKey, &m)
	if err == nil {
		_, err = d.opts.Store.Setting(ctx, engineHostsKey, &sets)
	}
	if err != nil {
		d.logf("engines: %v (retried in %s)", err, engineRetry)
		e.failedAt = time.Now()
		return
	}
	for id, engine := range m {
		e.byID[id] = engine
	}
	for id, c := range sets {
		// Written by this hub in canonical form; a value it cannot take (edited by hand) falls back to the single engine.
		if n, err := proto.NormalizeEngineChoice(c.Hosts, c.Default); err == nil {
			e.sets[id] = n
		} else {
			d.logf("engines: machine %s: %v (using its single engine)", id, err)
		}
	}
	e.loaded = true
}

// engineOf is the machine's single engine (proto.Machine.Engine): the one closest to its set, herdr when never set.
func (d *Devices) engineOf(machine string) string {
	if c, ok := d.storedChoice(machine); ok {
		return proto.LegacyEngine(c)
	}
	return proto.EngineHerdr
}

// engineChoiceOf is the machine's set: what the user chose, else herdr alone.
func (d *Devices) engineChoiceOf(machine string) proto.EngineChoice {
	if c, ok := d.storedChoice(machine); ok {
		return c
	}
	return proto.EngineHostsOf(proto.EngineHerdr)
}

// storedChoice is the set the user chose for the machine — chosen as a set, or as a single engine before sets; false
// when never chosen (or settings unreadable).
func (d *Devices) storedChoice(machine string) (proto.EngineChoice, bool) {
	d.engines.mu.Lock()
	defer d.engines.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.loadEnginesLocked(ctx)
	if c, ok := d.engines.sets[machine]; ok {
		return c, true
	}
	if engine := d.engines.byID[machine]; engine != "" {
		return proto.EngineHostsOf(engine), true
	}
	return proto.EngineChoice{}, false
}

// engineChoiceToMove is the machine's stored set for a move to another manager (devicewire.MethodEngineGet): nil when
// never chosen, an error while settings can't be read (not "never chosen", which would let the move drop it).
func (d *Devices) engineChoiceToMove(machine string) (*proto.EngineChoice, error) {
	c, ok := d.storedChoice(machine)
	d.engines.mu.Lock()
	loaded := d.engines.loaded
	d.engines.mu.Unlock()
	switch {
	case !loaded:
		return nil, errors.New("engines: settings not readable")
	case !ok:
		return nil, nil
	}
	return &c, nil
}

// errUnknownEngine: not one of proto.EngineNames, or a set that is not valid.
var errUnknownEngine = errors.New("unknown engine")

// setEngine stores the machine's single engine (PUT with {"engine"}): the set it stands for.
func (d *Devices) setEngine(ctx context.Context, machine, engine string) error {
	if _, ok := proto.EngineNames[engine]; !ok {
		return fmt.Errorf("%w %q (herdr, herdr-core, herdr-both, orca, tern, rex, paseo, cindy)", errUnknownEngine, engine)
	}
	return d.setEngineChoice(ctx, machine, proto.EngineHostsOf(engine))
}

// setEngineHosts stores the machine's set (PUT with {"hosts", "default"}).
func (d *Devices) setEngineHosts(ctx context.Context, machine string, hosts []string, def string) error {
	c, err := proto.NormalizeEngineChoice(hosts, def)
	if err != nil {
		return fmt.Errorf("%w: %v", errUnknownEngine, err)
	}
	return d.setEngineChoice(ctx, machine, c)
}

// setEngineChoice writes the set, then the single engine closest to it (for older hubs after a rollback).
func (d *Devices) setEngineChoice(ctx context.Context, machine string, c proto.EngineChoice) error {
	e := &d.engines
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failedAt = time.Time{} // the user is waiting: try now
	d.loadEnginesLocked(ctx)
	if !e.loaded {
		return errors.New("engines: settings not readable")
	}
	sets := map[string]proto.EngineChoice{}
	for id, v := range e.sets {
		sets[id] = v
	}
	sets[machine] = c
	byID := map[string]string{}
	for id, v := range e.byID {
		byID[id] = v
	}
	byID[machine] = proto.LegacyEngine(c)
	if d.opts.Store != nil {
		if err := d.opts.Store.PutSetting(ctx, engineHostsKey, sets); err != nil {
			return err
		}
		e.sets = sets // the choice is stored: read it even if the rollback copy below fails
		if err := d.opts.Store.PutSetting(ctx, enginesKey, byID); err != nil {
			return err
		}
	}
	e.sets, e.byID = sets, byID
	return nil
}

// engineUse is what engine.use carries: the single engine (all an older node reads) and the set with its default
// (read by nodes that report proto.NodeInfo.EngineHosts). Hosts is never nil: [] is the empty set, not "no set".
type engineUse struct {
	Engine  string   `json:"engine"`
	Hosts   []string `json:"hosts"`
	Default string   `json:"default"`
}

// useEngine tells a connected node the set the user chose, if any: a machine never set is left alone (its node
// starts on herdr, and an old node never sees a method it does not know). An older node takes the single engine and
// ignores the set (proto.Machine.EngineSetsUnsupported says so when that is not the same). Calls take turns per machine and each sends
// the value stored when it runs, so the node ends on the latest choice. A node that refuses it (too old) keeps herdr:
// logged, the stored choice stands.
func (m *Machine) useEngine(ctx context.Context, peer *rpc.Peer) error {
	m.engineMu.Lock()
	defer m.engineMu.Unlock()
	c, ok := m.d.storedChoice(m.cfg.ID)
	if !ok {
		return nil
	}
	engine := proto.LegacyEngine(c)
	err := peer.Call(ctx, "engine.use", engineUse{Engine: engine, Hosts: c.Hosts, Default: c.Default}, nil)
	if err != nil && !restartingError(err) {
		m.d.logf("[%s] the node did not take engine %s %v (it stays on its own engine until updated): %v", m.cfg.ID, engine, c.Hosts, err)
	}
	return err
}

// putEngine is PUT /api/v1/machines/{mid}/engine, with a set {"hosts": ["herdr", "orca"], "default": "herdr"}
// ("default" may be left out: the first host; "hosts": [] enables none) or, from older clients, a single engine
// {"engine": "herdr-core"}; a body with hosts is a set whatever else it says. Stored even while the machine is
// offline (applied when it connects), sent to the node at once when online; answers the machine as listed.
func (d *Devices) putEngine(w http.ResponseWriter, r *http.Request, m *Machine) {
	var in struct {
		Engine  string    `json:"engine"`
		Hosts   *[]string `json:"hosts"`
		Default string    `json:"default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("bad body: %w", err))
		return
	}
	var err error
	if in.Hosts != nil {
		err = d.setEngineHosts(r.Context(), m.cfg.ID, *in.Hosts, in.Default)
	} else {
		err = d.setEngine(r.Context(), m.cfg.ID, in.Engine)
	}
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, errUnknownEngine) {
			code = http.StatusBadRequest
		}
		writeErr(w, code, err)
		return
	}
	m.mu.Lock()
	peer := m.peer
	m.mu.Unlock()
	if peer != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		m.useEngine(ctx, peer)
		cancel()
	}
	d.reportNow()
	// The app takes this machine as it is: the notes on its engines in the reader's language, as in the hub's machine
	// list (hub locale_deploy.go).
	s := d.decorate(m, m.snapshot())
	s.Engines = i18n.EngineNotes(i18n.FromRequest(r), s.Engines)
	writeJSON(w, s)
}
