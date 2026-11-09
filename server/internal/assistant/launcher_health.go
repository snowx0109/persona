package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nwverse/persona/server/internal/compat"
	"net/http"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nwverse/persona/server/internal/apierr"
	"github.com/nwverse/persona/server/internal/assistantwire"
	"github.com/nwverse/persona/server/internal/lazyre"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
	"github.com/nwverse/persona/server/internal/transcript"
)

// Launcher health (docs/server/launcher-health.md): every launcher on every machine is checked now and then with the same
// one-word request "Test availability" sends (launchers.check); a failed start counts too. A launcher whose latest result is a
// failure is not used for a new agent while another machine has a healthy one; known errors are repaired (the node
// updates its CLI, else an agent with the configured skill), and what stays broken is blocked for the user to see.
// Nothing is pushed.

const launchersHealthKey = "launchers.health"

// The launcher's states.
const (
	healthOK        = "ok"
	healthFailing   = "failing"   // failed, not repaired yet
	healthRepairing = "repairing" // a repair agent is on it
	healthBlocked   = "blocked"   // repairs did not help (or there is none): the user's to fix
)

// The classes of a launcher's error (classifyLauncherError).
const (
	errKindModelRoute  = "model_route"
	errKindAuth        = "auth"
	errKindCLIOutdated = "cli_outdated"
	errKindTimeout     = "timeout"
	errKindOther       = "other"
	// errKindUsageLimit: the subscription's usage limit is used up (LauncherHealth.ResetsAt, when known). Unusable until
	// it resets, then checked again; never repaired: nothing on the machine is broken.
	errKindUsageLimit = "usage_limit"
	// errKindNotInTerminal: a start's new terminal tab could not run the launcher, or its claude (node/shellmiss.go,
	// #1642): the terminal reads other startup files than the login shell the launchers are listed and checked in, so
	// a check passing says nothing of it, and no repair the manager has reaches it. Blocked at once; a start with it
	// that works clears it (or it goes stale).
	errKindNotInTerminal = "not_in_terminal"
)

var (
	// healthTick is how often the loop looks at the machines; a launcher is checked again after the interval.
	healthTick = 5 * time.Minute
	// healthOnlineRecheck: a machine that comes online gets its launchers checked unless checked this recently (the
	// hub's restarts would otherwise check everything each time).
	healthOnlineRecheck = 10 * time.Minute
	// healthStale: a failure older than this no longer keeps a launcher from being used (a usage limit with a known
	// reset: until the reset instead).
	healthStale = 6 * time.Hour
	// healthLimitMax: a usage limit said to reset further ahead than this was misread (a weekly limit resets within 7
	// days): the record counts as one without a reset.
	healthLimitMax = 8 * 24 * time.Hour
	// healthMaxRepairs: repair rounds for one run of failures before the launcher is blocked.
	healthMaxRepairs = 2
	// repairSettle and repairDeadline: a repair agent that stopped is rechecked once it has run this long; one that
	// has not stopped by the deadline is rechecked anyway.
	repairSettle   = 2 * time.Minute
	repairDeadline = 45 * time.Minute
	// healthCheckTimeout bounds one check (the node's own is 90 seconds).
	healthCheckTimeout = 100 * time.Second
	// healthRecheckThrottle: how long a failing launcher keeps the on-demand probe's answer instead of probing again,
	// so a burst of starts does not turn into a burst of probes.
	healthRecheckThrottle = 30 * time.Second
)

const defaultHealthInterval = 30 * time.Minute

// defaultRepairPermission: the repair agent runs ssh and edits CLI settings; under the default mode it would stop on
// an approval card for each step (2026-11-05 user decision).
const defaultRepairPermission = "bypassPermissions"

// LauncherHealth is one launcher on one machine as last checked.
type LauncherHealth struct {
	Machine   string    `json:"machine"`
	Launcher  string    `json:"launcher"`
	Agent     string    `json:"agent,omitempty"` // claude, codex… (as the check ran it)
	State     string    `json:"state"`
	Kind      string    `json:"kind,omitempty"` // the error's class; "" when it works
	Error     string    `json:"error,omitempty"`
	Model     string    `json:"model,omitempty"` // which model answered
	Millis    int       `json:"millis,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	Source    string    `json:"source"`          // check | start (an agent's start failed) | session (an agent's turn met a usage limit)
	Fails     int       `json:"fails,omitempty"` // failures in a row
	// Repairs are this run of failures' repair rounds; BlockedReason says why it was given up on.
	Repairs       int            `json:"repairs,omitempty"`
	Repair        *LauncherFixed `json:"repair,omitempty"`
	BlockedReason string         `json:"blocked_reason,omitempty"`
	// ResetsAt: Kind is errKindUsageLimit and this is when the limit resets (read off the check's words when they name a
	// time zone, or an agent's own record of the limit): the launcher counts as unusable until then, and no longer after.
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

// LauncherFixed is the latest repair of a launcher.
type LauncherFixed struct {
	Action    string    `json:"action"` // upgrade_cli | agent
	At        time.Time `json:"at"`
	OK        bool      `json:"ok"`
	Output    string    `json:"output,omitempty"`
	Machine   string    `json:"machine,omitempty"` // the repair agent's
	SessionID string    `json:"session_id,omitempty"`
	TodoID    string    `json:"todo_id,omitempty"`
}

// machineRepair is a repair agent at work on one machine's launchers.
type machineRepair struct {
	Machine      string    `json:"machine"` // the broken one
	Launchers    []string  `json:"launchers"`
	AgentMachine string    `json:"agent_machine"`
	SessionID    string    `json:"session_id"`
	TodoID       string    `json:"todo_id,omitempty"`
	StartedAt    time.Time `json:"started_at"`
}

// healthStore is what launchersHealthKey holds.
type healthStore struct {
	Records []LauncherHealth `json:"records"`
	Repairs []machineRepair  `json:"repairs,omitempty"`
	// Counted are the app's failed starts already counted (launcher_health_starts.go), kept across a restart while the
	// node may still list them.
	Counted []countedStart `json:"counted_starts,omitempty"`
}

type healthBook struct {
	mu       sync.Mutex
	loaded   bool
	recs     map[string]*LauncherHealth // by healthKey
	repairs  map[string]*machineRepair  // by broken machine
	online   map[string]bool            // reachable at the last look
	checking map[string]bool            // machines being checked now
	// The app's starts answered at once (launcher_health_starts.go): those seen starting, by machine and session, and
	// the failed ones already counted, when (kept with the records: healthStore.Counted).
	starting  map[[2]string]startWatch
	counted   map[[2]string]time.Time
	recheck   map[string]*healthRecheck // on-demand probes in flight, by healthKey
	recheckAt map[string]time.Time      // last on-demand probe, by healthKey
}

// healthRecheck is an on-demand launcher probe one start is running and concurrent starts wait for.
type healthRecheck struct {
	done    chan struct{}
	res     healthResult
	got     bool
	waiters int // starts that joined this probe instead of starting their own
}

func healthKey(machine, launcher string) string { return machine + "/" + launcher }

// healthConfig is manager.json agents.launcher_health with its defaults.
type healthConfig struct {
	on       bool
	interval time.Duration
	repair   assistantwire.LauncherRepairConfig
}

func (s *Service) healthConfig(ctx context.Context) healthConfig {
	c := s.hello(ctx).Agents.LauncherHealth
	out := healthConfig{on: c.Enabled != nil && *c.Enabled, interval: defaultHealthInterval, repair: c.Repair}
	if out.repair.Permission == "" {
		out.repair.Permission = defaultRepairPermission
	}
	if c.IntervalMinutes > 0 {
		out.interval = time.Duration(c.IntervalMinutes) * time.Minute
	}
	return out
}

// ---- what is stored ----

// healthLoadLocked reads the records once (b.mu held).
func (s *Service) healthLoadLocked(ctx context.Context) {
	b := &s.health
	if b.loaded {
		return
	}
	b.recs, b.repairs = map[string]*LauncherHealth{}, map[string]*machineRepair{}
	if b.online == nil {
		b.online, b.checking = map[string]bool{}, map[string]bool{}
	}
	if b.recheck == nil {
		b.recheck, b.recheckAt = map[string]*healthRecheck{}, map[string]time.Time{}
	}
	var stored healthStore
	if st := s.store(); st != nil {
		// Bounded: b.mu is held, and every start waits for it (#5467).
		rctx, cancel := context.WithTimeout(ctx, historyTimeout)
		_, err := st.Setting(rctx, launchersHealthKey, &stored)
		cancel()
		if err != nil {
			s.logf("launcher health: read: %v", err)
			return // read again next time
		}
	}
	for i := range stored.Records {
		r := stored.Records[i]
		b.recs[healthKey(r.Machine, r.Launcher)] = &r
	}
	for i := range stored.Repairs {
		r := stored.Repairs[i]
		b.repairs[r.Machine] = &r
	}
	for _, c := range stored.Counted {
		if b.counted == nil {
			b.counted = map[[2]string]time.Time{}
		}
		if key := [2]string{c.Machine, c.Session}; b.counted[key].IsZero() {
			b.counted[key] = c.At
		}
	}
	b.loaded = true
}

// healthSaveLocked writes the records (b.mu held); no database keeps them in memory only.
func (s *Service) healthSaveLocked(ctx context.Context) {
	st := s.store()
	b := &s.health
	if st == nil || !b.loaded {
		return // not over what could not be read
	}
	out := healthStore{Records: make([]LauncherHealth, 0, len(b.recs))}
	for _, r := range b.recs {
		out.Records = append(out.Records, *r)
	}
	sort.Slice(out.Records, func(i, j int) bool {
		return healthKey(out.Records[i].Machine, out.Records[i].Launcher) < healthKey(out.Records[j].Machine, out.Records[j].Launcher)
	})
	for _, r := range b.repairs {
		out.Repairs = append(out.Repairs, *r)
	}
	sort.Slice(out.Repairs, func(i, j int) bool { return out.Repairs[i].Machine < out.Repairs[j].Machine })
	for key, at := range b.counted {
		if time.Since(at) <= startFollowFor {
			out.Counted = append(out.Counted, countedStart{Machine: key[0], Session: key[1], At: at})
		}
	}
	sort.Slice(out.Counted, func(i, j int) bool {
		return out.Counted[i].Machine+"/"+out.Counted[i].Session < out.Counted[j].Machine+"/"+out.Counted[j].Session
	})
	// Bounded: b.mu is held, and every start waits for it; the records stay in memory either way (#5467).
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), historyTimeout)
	defer cancel()
	if err := st.PutSetting(wctx, launchersHealthKey, out); err != nil {
		s.logf("launcher health: save: %v", err)
	}
}

// healthRecords are the records, by machine and launcher; state and machine filter them when set.
func (s *Service) healthRecords(ctx context.Context, state, machine string) []LauncherHealth {
	b := &s.health
	b.mu.Lock()
	s.healthLoadLocked(ctx)
	out := []LauncherHealth{}
	for _, r := range b.recs {
		if (state == "" || r.State == state) && (machine == "" || r.Machine == machine) {
			out = append(out, *r)
		}
	}
	b.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		return healthKey(out[i].Machine, out[i].Launcher) < healthKey(out[j].Machine, out[j].Launcher)
	})
	return out
}

// launcherHealth is launcher's record on machine.
func (s *Service) launcherHealth(ctx context.Context, machine, launcher string) (LauncherHealth, bool) {
	b := &s.health
	b.mu.Lock()
	defer b.mu.Unlock()
	s.healthLoadLocked(ctx)
	if r := b.recs[healthKey(machine, launcher)]; r != nil {
		return *r, true
	}
	return LauncherHealth{}, false
}

// unusable: the record says the launcher does not work (its latest result, failed within healthStale). A usage limit
// whose reset is known holds until that reset, however far off, and no longer.
func (r LauncherHealth) unusable() bool {
	if r.State == "" || r.State == healthOK {
		return false
	}
	if reset, ok := r.limitReset(); ok {
		return time.Now().Before(reset)
	}
	return time.Since(r.CheckedAt) < healthStale
}

// limitReset is when the usage limit r records resets: r failed on the subscription's usage limit, and its reset is
// known and not past healthLimitMax from when it was recorded.
func (r LauncherHealth) limitReset() (time.Time, bool) {
	if r.Kind != errKindUsageLimit || r.ResetsAt == nil || r.State == healthOK || r.ResetsAt.Sub(r.CheckedAt) > healthLimitMax {
		return time.Time{}, false
	}
	return *r.ResetsAt, true
}

// resetNote is how agents_list's lines and a detour's reason say when r's usage limit resets ("" without one):
// machine-readable, and how far off.
func (r LauncherHealth) resetNote() string {
	reset, ok := r.limitReset()
	if !ok {
		return ""
	}
	return fmt.Sprintf("resets_at=%s, %s", reset.UTC().Format(time.RFC3339), until(reset))
}

// launcherUnusable is launcher's record on machine when it does not work.
func (s *Service) launcherUnusable(ctx context.Context, machine, launcher string) (LauncherHealth, bool) {
	r, ok := s.launcherHealth(ctx, machine, launcher)
	return r, ok && r.unusable()
}

// ---- recording ----

// healthResult is one check's or start's outcome.
type healthResult struct {
	ok           bool
	agent, model string
	millis       int
	err          string
	source       string
	kind         string // the error's class when the outcome says it (a start's coded refusal); "": classifyLauncherError
	// resetsAt: a usage limit's reset as the outcome's source knew it (an agent's own record of the limit); nil: read
	// off err's words when they name a time zone.
	resetsAt *time.Time
}

// Where a result comes from (LauncherHealth.Source).
const (
	healthSourceCheck = "check" // launchers.check, in the node's login shell
	healthSourceStart = "start" // an agent's start, in a terminal tab
	// healthSourceSession: an agent's own turn with the launcher ended on the subscription's usage limit, its reset
	// known (launcherLimitTurnEnded). Nothing was run for it: no check, no repair.
	healthSourceSession = "session"
)

// terminalBlockedReason is why a launcher the terminal cannot run is blocked: no repair reaches the terminal's startup files.
const terminalBlockedReason = "not available in this computer's terminal: its shell reads other startup files than the login shell " +
	"Persona lists and checks launchers in (another shell, a container, an rc file that returns early). Define the launcher where " +
	"the terminal's shell reads it, or add it to Persona as a model provider; a start with it that works clears this"

// recordHealth keeps r as launcher's latest on machine; the apps are told when its state changes.
func (s *Service) recordHealth(ctx context.Context, machine, launcher string, r healthResult) LauncherHealth {
	b := &s.health
	b.mu.Lock()
	s.healthLoadLocked(ctx)
	key := healthKey(machine, launcher)
	rec := b.recs[key]
	if rec == nil {
		rec = &LauncherHealth{Machine: machine, Launcher: launcher}
		b.recs[key] = rec
	}
	before := rec.State
	if r.source != healthSourceStart && rec.Kind == errKindNotInTerminal && rec.unusable() {
		// The check runs in the login shell, where the launcher works; the terminal is what cannot run it. Only a
		// start says otherwise (#1642): a session already running says nothing of a new terminal tab.
		out := *rec
		b.mu.Unlock()
		return out
	}
	rec.CheckedAt, rec.Source, rec.Millis = time.Now().UTC(), r.source, r.millis
	if r.agent != "" {
		rec.Agent = r.agent
	}
	rec.ResetsAt = nil
	if r.ok {
		rec.State, rec.Kind, rec.Error, rec.Fails, rec.Repairs, rec.BlockedReason = healthOK, "", "", 0, 0, ""
		rec.Model = r.model
	} else {
		rec.Fails++
		rec.Error = clip(oneLine(r.err), 400)
		rec.Kind = r.kind
		if rec.Kind == "" {
			rec.Kind = classifyLauncherError(r.err)
		}
		switch {
		case rec.Kind == errKindNotInTerminal:
			rec.State, rec.BlockedReason = healthBlocked, terminalBlockedReason // no repair round (repairMachine takes failing ones)
		case rec.Kind == errKindUsageLimit:
			// The request got through to the subscription, whose limit is used up: nothing is broken, so no repair
			// is due (repairNow) and a block for an earlier error no longer stands. The reset clears it.
			rec.State, rec.BlockedReason, rec.ResetsAt = healthFailing, "", limitResetOf(r, rec.CheckedAt)
		case rec.State == "" || rec.State == healthOK:
			rec.State = healthFailing
		}
	}
	out := *rec
	s.healthSaveLocked(ctx)
	b.mu.Unlock()
	if before != out.State {
		s.healthChanged(ctx, out, before)
	}
	return out
}

// setHealthState moves launcher on machine to state ("": it stays; why: the blocked reason) and keeps fixed as its
// latest repair, one more round of this run of failures. Nothing changes once a check passed meanwhile, nor for a
// usage limit met meanwhile: it is neither repaired nor blocked, its reset clears it.
func (s *Service) setHealthState(ctx context.Context, machine, launcher, state, why string, fixed *LauncherFixed) {
	b := &s.health
	b.mu.Lock()
	s.healthLoadLocked(ctx)
	rec := b.recs[healthKey(machine, launcher)]
	if rec == nil || rec.State == healthOK || rec.Kind == errKindUsageLimit {
		b.mu.Unlock()
		return
	}
	before := rec.State
	if state != "" {
		rec.State = state
	}
	if state == healthBlocked {
		rec.BlockedReason = clip(oneLine(why), 600)
	}
	if fixed != nil {
		f := *fixed
		rec.Repair = &f
		rec.Repairs++
	}
	out := *rec
	s.healthSaveLocked(ctx)
	b.mu.Unlock()
	if before != out.State {
		s.healthChanged(ctx, out, before)
	}
}

// repairWorked marks launcher's latest repair on machine as the one that fixed it (its check passed after).
func (s *Service) repairWorked(ctx context.Context, machine, launcher string, fixed LauncherFixed) {
	b := &s.health
	b.mu.Lock()
	defer b.mu.Unlock()
	s.healthLoadLocked(ctx)
	if rec := b.recs[healthKey(machine, launcher)]; rec != nil && rec.State == healthOK {
		fixed.OK = true
		rec.Repair = &fixed
		s.healthSaveLocked(ctx)
	}
}

// healthChanged logs a launcher's new state and tells the apps (an event, not a push).
func (s *Service) healthChanged(ctx context.Context, r LauncherHealth, before string) {
	if before == "" {
		before = "new"
	}
	if r.State == healthOK {
		s.logf("launcher health: %s on %s: %s → ok", r.Launcher, r.Machine, before)
	} else {
		s.logf("launcher health: %s on %s: %s → %s (%s: %s)", r.Launcher, r.Machine, before, r.State, r.Kind, clip(r.Error, 160))
	}
	s.event(ctx, map[string]any{"type": "launcher_health", "record": r})
}

// recordStartFailure: an agent did not start with launcher on machine. A dropped link says nothing about the
// launcher, nor does a start the node turned down before running it (startRefused); anything else counts, and a
// check follows at once to tell a blink from a broken launcher. Not after a start the terminal could not run
// (errKindNotInTerminal): the check runs where it works.
func (s *Service) recordStartFailure(ctx context.Context, machine, launcher string, err error) {
	if err == nil || launcher == "" || transientStartErr(err) || errors.Is(err, ErrNoDevice) || startRefused(err) {
		return
	}
	if terminalMissing(err) {
		s.recordHealth(ctx, machine, launcher, healthResult{err: err.Error(), source: healthSourceStart, kind: errKindNotInTerminal})
		return
	}
	s.recordHealth(ctx, machine, launcher, healthResult{err: err.Error(), source: healthSourceStart})
	go func() {
		ctx := context.WithoutCancel(ctx)
		if res, ok := s.probeLauncherHealth(ctx, machine, launcher); ok {
			s.recordHealth(ctx, machine, launcher, res)
		}
	}()
}

// recordStartSuccess: an agent started with launcher on machine, so the terminal there runs it: a record saying it
// does not (errKindNotInTerminal) is cleared. Any other record stays: a start that came up says nothing of the model
// route or the key a check found broken.
func (s *Service) recordStartSuccess(ctx context.Context, machine, launcher string) {
	if launcher == "" {
		launcher = "claude"
	}
	if r, ok := s.launcherHealth(ctx, machine, launcher); !ok || r.Kind != errKindNotInTerminal || r.State == healthOK {
		return
	}
	s.recordHealth(ctx, machine, launcher, healthResult{ok: true, source: healthSourceStart})
}

// limitResetOf is when the usage limit r failed on resets, as r's source knew it or as its words say (at: when it
// was recorded); nil when unknown or further off than healthLimitMax.
func limitResetOf(r healthResult, at time.Time) *time.Time {
	reset := time.Time{}
	if r.resetsAt != nil {
		reset = *r.resetsAt
	} else if t, ok := transcript.UsageLimitReset(r.err, at); ok {
		reset = t
	}
	if reset.IsZero() || reset.Sub(at) > healthLimitMax {
		return nil
	}
	reset = reset.UTC()
	return &reset
}

// launcherLimitTurnEnded: a turn of machine's session sid ended. When it ended on the subscription's usage limit,
// with a reset still ahead, the launcher the session names counts as usage-limited on machine until then (source
// healthSourceSession): an agent started with it there would be refused at once too. Nothing runs for it, no check
// (it would spend a request to be refused) and no repair. The assistant's conversations count as well: the main
// chat runs the machine's launcher under the same sign-in, so its limit is the launcher's.
func (s *Service) launcherLimitTurnEnded(ctx context.Context, machine, sid string) {
	// Once the turn stayed still, as a task's end is judged (settleDelay): right after a write the node's list may not
	// have read the transcript yet (its meta is read again at most every few seconds), and says no error meanwhile.
	time.Sleep(s.settleDelay(ctx))
	// As the node lists it, when the fleet is here: the fields below are the node's, and the decorated copy
	// session.get answers with costs a pass over the machine's whole list.
	sess, ok := s.nodeSession(machine, sid)
	if !s.fleetOn() {
		sess, ok = s.session(ctx, machine, sid)
	}
	if !ok || sess.Launcher == "" {
		return
	}
	lim := limitOfTurn(sess)
	reset, ok := usageLimitReset(lim)
	if now := time.Now(); !ok || !reset.After(now) || reset.Sub(now) > healthLimitMax {
		return
	}
	if r, found := s.launcherHealth(ctx, machine, sess.Launcher); found {
		if at, limited := r.limitReset(); limited && !at.Before(reset) {
			return // known already (each turn a background task wakes meets it again)
		}
	}
	s.recordHealth(ctx, machine, sess.Launcher, healthResult{err: lim.TurnError, source: healthSourceSession, kind: errKindUsageLimit, resetsAt: &reset})
}

// limitOfTurn is sess's latest turn error decorated with what it says of a usage limit (decorateLimit): the node's
// word, else Claude Code's last words when they are the error. Whatever sess's state now, unlike the lists'
// turnError: a background task wakes an agent its limit stopped again and again, and it counts as working meanwhile.
func limitOfTurn(sess proto.Session) proto.Session {
	out := proto.Session{TurnErrorResetsAt: sess.TurnErrorResetsAt, UpdatedAt: sess.UpdatedAt}
	out.TurnError, out.TurnErrorKind = latestTurnError(sess)
	decorateLimit(&out)
	return out
}

// terminalMissing: err is the node's refusal of a start whose new terminal tab could not run the launcher or its claude
// (launcher_missing / claude_missing with where "terminal", node/shellmiss.go, #1642).
func terminalMissing(err error) bool {
	var re *rpc.Error
	if !errors.As(err, &re) || (re.Code != proto.CodeLauncherMissing && re.Code != proto.CodeClaudeMissing) {
		return false
	}
	var data struct {
		Where string `json:"where"`
	}
	return json.Unmarshal(re.Data, &data) == nil && data.Where == "terminal"
}

// startRequestErr are the node's answers to a session.start it would not run, whatever the launcher (node/control.go
// checkStart and start, node/model_config.go check): a directory that is not there, a bad name, mode, effort or model.
var startRequestErr = lazyre.New(`^directory .* does not exist$|^invalid launcher |^invalid session id |^invalid model$|^unknown permission mode |^unknown effort level |^unsupported agent type |^permission mode and effort are only supported for Claude starts|^普通 Agent 的启动只支持|^新建 Codex 的思考强度由其启动器管理`)

// startRefusalCodes are the node's (and the hub's) coded refusals of the request itself (node/control.go,
// node/codex_start.go, node/openelsewhere.go…). Codes about the machine's state (engine_missing, runtime_away,
// not_running, orca_not_registered) are not here: the launcher cannot run there now, and a task may go elsewhere.
var startRefusalCodes = []string{"codex_needs_prompt", "codex_resume", "codex_read_only", "orca_resume", "open_elsewhere", "runtime_session",
	"identity_conflict", "ultracode_unsupported", proto.CodeNodeOutdated}

// startRefused: the start failed on the request, not the launcher: the node checked it and said no before running
// anything (one of startRefusalCodes, or of startRequestErr).
func startRefused(err error) bool {
	var re *rpc.Error
	if !errors.As(err, &re) {
		return false
	}
	if re.Code != "" {
		return slices.Contains(startRefusalCodes, re.Code)
	}
	return startRequestErr.MatchString(re.Message)
}

// ---- classifying ----

var launcherErrorKinds = []struct {
	kind string
	re   *lazyre.Regexp
}{
	// First: the limit's words name no other cause, and its line may carry a prefix ("Error: …"). Claude Code's own
	// line ("You've hit your session limit · resets 4:20pm (…)", an older "Claude AI usage limit reached|…") and
	// Codex's (its message, its error kinds in a rollout and the app-server). A gateway's 429, "rate limit" or "quota"
	// is not a subscription's limit and stays in its class.
	{errKindUsageLimit, lazyre.New(`(?i)\byou(?:'|’)ve hit your [^·\n.]{0,40}\blimit\b|usage limit reached|usage_limit_exceeded|usageLimitExceeded`)},
	{errKindModelRoute, lazyre.New(`(?i)bare claude model name|cannot be routed|no account match|model[^.\n]{0,60}(not found|not supported|not available|does not exist|unknown)|unknown model|invalid model`)},
	{errKindAuth, lazyre.New(`(?i)\b40[13]\b|invalid (or missing )?(api[ _-]?key|x-api-key|token|bearer)|authentication_error|unauthori[sz]ed|not logged in|please run /login|token (has )?expired|permission_error|forbidden|requires \w+ auth|no \w+ auth token|auth login`)},
	{errKindCLIOutdated, lazyre.New(`(?i)unknown option|unrecognized option|unexpected argument|please (update|upgrade)|(update|upgrade) (claude|codex|your)|version[^.\n]{0,40}(too old|no longer supported|not supported|outdated)|minimum (required )?version|requires (a )?newer`)},
	{errKindTimeout, lazyre.New(`(?i)context deadline exceeded|deadline exceeded|timed? ?out|秒内没有回答`)},
}

// classifyLauncherError names the class of a launcher's error, the first that matches.
func classifyLauncherError(text string) string {
	for _, k := range launcherErrorKinds {
		if k.re.MatchString(text) {
			return k.kind
		}
	}
	return errKindOther
}

// repairNow: a failure the repairs know right away; the others wait for a second one in a row. Never a usage limit,
// however many in a row: nothing on the machine is broken, it waits for the reset.
func (r LauncherHealth) repairNow() bool {
	switch r.Kind {
	case errKindUsageLimit:
		return false
	case errKindModelRoute, errKindAuth, errKindCLIOutdated:
		return true
	}
	return r.Fails >= 2
}

// ---- checking ----

// probeLauncherHealth runs launchers.check for launcher on machine; false when the machine could not be asked (no result).
func (s *Service) probeLauncherHealth(ctx context.Context, machine, launcher string) (healthResult, bool) {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()
	var res proto.LauncherCheck
	if err := s.call(ctx, machine, assistantwire.DeviceLaunchersCheck, map[string]any{"name": launcher}, &res); err != nil {
		s.logf("launcher health: check %s on %s: %v", launcher, machine, err)
		return healthResult{}, false
	}
	return healthResult{ok: res.OK, agent: res.Agent, model: res.Model, millis: res.Millis, err: res.Error, source: healthSourceCheck}, true
}

// healthLaunchers are the launchers to check on machine: those it can start (its ~/.zshrc's, the managed ones) and
// the default one; not a runtime's (they are not run with a prompt) nor an API provider's.
func (s *Service) healthLaunchers(ctx context.Context, machine string) ([]string, error) {
	list, err := s.machineLaunchers(ctx, machine)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range list {
		if l.Source == "" || machineSource(l.Source) || compat.Source(l.Source) == "persona" {
			out = append(out, l.Name)
		}
	}
	if d := s.startLauncher(ctx, ""); !slices.Contains(out, d) {
		out = append(out, d)
	}
	return out, nil
}

// startLauncher is the launcher a start with launcher runs: itself, else model settings › Agents', else claude.
func (s *Service) startLauncher(ctx context.Context, launcher string) string {
	if launcher != "" {
		return launcher
	}
	if b, err := s.resolveModel(ctx, "agent.new"); err == nil {
		if name, ok := strings.CutPrefix(b.Provider, "launcher."); ok && name != "" {
			return name
		}
	}
	return "claude"
}

// healthRound is one look at the machines: the launchers due on each reachable one are checked (each machine's one
// after another, the machines side by side), then repaired; the repair agents at work are followed. wait: return
// when it is all done (tests, a forced check), else it goes on in the background.
func (s *Service) healthRound(ctx context.Context, wait bool) {
	cfg := s.healthConfig(ctx)
	devices := s.devices(ctx)
	b := &s.health
	var wg sync.WaitGroup
	for _, d := range devices {
		b.mu.Lock()
		s.healthLoadLocked(ctx)
		cameOnline := d.Reachable && !b.online[d.ID]
		b.online[d.ID] = d.Reachable
		busy := b.checking[d.ID]
		if d.Reachable && cfg.on && !busy {
			b.checking[d.ID] = true
		}
		b.mu.Unlock()
		if !d.Reachable || !cfg.on || busy {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				b.mu.Lock()
				delete(b.checking, d.ID)
				b.mu.Unlock()
			}()
			s.checkMachine(ctx, d, cfg, cameOnline, "")
			s.repairMachine(ctx, d, cfg)
		}()
	}
	if wait {
		wg.Wait()
	}
	s.followRepairs(ctx, cfg)
}

// checkMachine checks machine d's launchers that are due (only: one by that name, at once); the records of launchers
// it no longer has go.
func (s *Service) checkMachine(ctx context.Context, d Device, cfg healthConfig, cameOnline bool, only string) []LauncherHealth {
	names, err := s.healthLaunchers(ctx, d.ID)
	if err != nil {
		s.logf("launcher health: %s: launchers: %v", d.ID, err)
		return nil
	}
	b := &s.health
	b.mu.Lock()
	for key, r := range b.recs {
		if r.Machine == d.ID && !slices.Contains(names, r.Launcher) {
			delete(b.recs, key)
			delete(b.recheckAt, key)
		}
	}
	b.mu.Unlock()
	var out []LauncherHealth
	for _, name := range names {
		if ctx.Err() != nil {
			break
		}
		if only != "" && name != only {
			continue
		}
		if only == "" {
			r, ok := s.launcherHealth(ctx, d.ID, name)
			since := time.Since(r.CheckedAt)
			if reset, limited := r.limitReset(); limited {
				// A check before the reset is refused too, and spends a request for it; the first round after it
				// checks, whatever the interval. From a minute after, as the manager continues a turn its limit
				// stopped (limitContinueAfter): one right at the reset may still be refused.
				if time.Now().Before(reset.Add(limitContinueAfter)) {
					continue
				}
			} else if ok && since < cfg.interval && !(cameOnline && since >= healthOnlineRecheck) {
				continue
			}
			if ok && r.Kind == errKindNotInTerminal && r.unusable() {
				continue // the login shell's check cannot tell (recordHealth): it waits for a start, or goes stale
			}
		}
		if res, ok := s.probeLauncherHealth(ctx, d.ID, name); ok {
			out = append(out, s.recordHealth(ctx, d.ID, name, res))
		}
	}
	return out
}

// ---- picking a machine ----

// healthyMachine is another reachable machine that runs launcher and whose check of it did not fail: one where it
// passed first, then one where it was never checked.
func (s *Service) healthyMachine(ctx context.Context, launcher, except string) (Device, bool) {
	best, rank := Device{}, 3
	for _, d := range s.devices(ctx) {
		if d.ID == except || !d.Reachable {
			continue
		}
		r, ok := s.launcherHealth(ctx, d.ID, launcher)
		n := 1
		switch {
		case ok && r.unusable():
			continue
		case ok && r.State == healthOK:
			n = 0
		}
		if n >= rank || !s.launcherOn(ctx, d.ID, launcher) {
			continue
		}
		best, rank = d, n
	}
	return best, rank < 3
}

// launcherOn: machine has launcher (a managed one is on every machine; a failed list says no).
func (s *Service) launcherOn(ctx context.Context, machine, launcher string) bool {
	if launcher == "claude" || slices.ContainsFunc(s.storedLaunchers(ctx), func(l storedLauncher) bool { return l.Name == launcher && l.Kind != "api" }) {
		return true
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	list, err := s.machineLaunchers(cctx, machine)
	return err == nil && slices.ContainsFunc(list, func(l proto.Launcher) bool { return l.Name == launcher })
}

// rehome moves dir from one machine's home to another's (a path outside the home stays).
func rehome(dir string, from, to Device) string {
	if from.Home == "" || to.Home == "" || from.Home == to.Home {
		return dir
	}
	if dir == from.Home {
		return to.Home
	}
	if rest, ok := strings.CutPrefix(dir, from.Home+"/"); ok {
		return path.Join(to.Home, rest)
	}
	return dir
}

// recheckLauncherNow probes machine's launcher on the spot when its record says the launcher fails, so a fix since
// the last round (a CLI installed, a PATH corrected) clears the record and the start stays on the asked machine.
// A probe already running is joined; a probe within healthRecheckThrottle is not repeated (checked=false), and the
// caller keeps the usual reroute behavior. got=false means the machine could not be asked.
func (s *Service) recheckLauncherNow(ctx context.Context, machine, launcher string) (res healthResult, got, checked bool) {
	b := &s.health
	key := healthKey(machine, launcher)
	b.mu.Lock()
	s.healthLoadLocked(ctx)
	if rc := b.recheck[key]; rc != nil {
		rc.waiters++
		b.mu.Unlock()
		select {
		case <-rc.done:
			return rc.res, rc.got, true
		case <-ctx.Done():
			return healthResult{}, false, false
		}
	}
	if at := b.recheckAt[key]; !at.IsZero() && time.Since(at) < healthRecheckThrottle {
		b.mu.Unlock()
		return healthResult{}, false, false
	}
	rc := &healthRecheck{done: make(chan struct{})}
	b.recheck[key] = rc
	b.recheckAt[key] = time.Now()
	b.mu.Unlock()
	defer func() { // a panic must not leave waiters blocked on a channel that never closes
		close(rc.done)
		b.mu.Lock()
		delete(b.recheck, key)
		b.mu.Unlock()
	}()

	res, got = s.probeLauncherHealth(ctx, machine, launcher)
	rc.res, rc.got = res, got
	return res, got, true
}

// healthDetour is where a start with launcher on machine m goes instead: another machine when its last check failed
// there and another has it healthy (moved), with a line saying so for whoever started it; no other: m, and the line
// says it may fail. A usage limit with a known reset says when that is.
func (s *Service) healthDetour(ctx context.Context, m Device, launcher string) (Device, string, bool) {
	r, bad := s.launcherUnusable(ctx, m.ID, launcher)
	if !bad {
		return m, "", false
	}
	// The record can lag behind a fix: probe the machine once on the spot before moving the start elsewhere. A pass
	// clears the record and the start stays on the asked machine (#2018). Two records cannot be settled by a probe:
	// a subscription usage limit (the request would be refused too) and a launcher the terminal's shell cannot run
	// (the probe's login shell runs it fine); only a real start clears those.
	if r.Kind != errKindUsageLimit && r.Kind != errKindNotInTerminal {
		if res, got, checked := s.recheckLauncherNow(ctx, m.ID, launcher); checked && got && res.ok {
			s.recordHealth(ctx, m.ID, launcher, res)
			return m, "", false
		}
	}
	alt, moved := s.healthyMachine(ctx, launcher, m.ID)
	if note := r.resetNote(); note != "" {
		what := fmt.Sprintf("launcher %s is at its subscription usage limit on %s (%s)", launcher, m.ID, note)
		if moved {
			return alt, fmt.Sprintf("%s, so this runs on %s instead", what, alt.ID), true
		}
		return m, what + " and no other machine has it healthy; started there anyway", false
	}
	why := fmt.Sprintf("%s: %s", r.Kind, clip(r.Error, 160))
	if moved {
		return alt, fmt.Sprintf("launcher %s failed its health check on %s (%s), so this runs on %s instead", launcher, m.ID, why, alt.ID), true
	}
	return m, fmt.Sprintf("launcher %s failed its last health check on %s (%s) and no other machine has it healthy; started there anyway", launcher, m.ID, why), false
}

// healthLines are agents_list's lines about the launchers that do not work (machine: only there).
func (s *Service) healthLines(ctx context.Context, machine string) string {
	var b strings.Builder
	for _, r := range s.healthRecords(ctx, "", machine) {
		if !r.unusable() {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("Launchers that do not work now (agent_start / task_plan move off them):\n")
		}
		fmt.Fprintf(&b, "- machine=%s launcher=%s · %s · %s: %s · checked %s", r.Machine, r.Launcher, r.State, r.Kind, clip(r.Error, 200), ago(r.CheckedAt))
		if note := r.resetNote(); note != "" {
			b.WriteString(" · " + note)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// launcherWithHealth is a launcher of the "New Agent" picker with how its last check went on that machine.
type launcherWithHealth struct {
	ConfiguredLauncher
	Health *launcherHealthView `json:"health,omitempty"`
}

type launcherHealthView struct {
	State     string     `json:"state"`
	Kind      string     `json:"kind,omitempty"`
	Error     string     `json:"error,omitempty"`
	CheckedAt time.Time  `json:"checked_at"`
	ResetsAt  *time.Time `json:"resets_at,omitempty"` // a usage limit's reset: unusable until then
}

// withHealth puts machine's health records on its launchers (none: never checked).
func (s *Service) withHealth(ctx context.Context, machine string, list []ConfiguredLauncher) []launcherWithHealth {
	out := make([]launcherWithHealth, 0, len(list))
	for _, l := range list {
		v := launcherWithHealth{ConfiguredLauncher: l}
		if r, ok := s.launcherHealth(ctx, machine, l.Name); ok && (r.State == healthOK || r.unusable()) { // not a failure too old to count
			v.Health = &launcherHealthView{State: r.State, Kind: r.Kind, Error: r.Error, CheckedAt: r.CheckedAt}
			if reset, limited := r.limitReset(); limited {
				v.Health.ResetsAt = &reset
			}
		}
		out = append(out, v)
	}
	return out
}

// ---- the loop and the routes ----

// healthLoop checks the launchers every healthTick until ctx ends (Run).
func (s *Service) healthLoop(ctx context.Context) {
	t := time.NewTicker(healthTick)
	defer t.Stop()
	for {
		if !s.hello(ctx).Assistant.Standby { // a member hub's machines are its center's to check
			s.healthRound(ctx, false)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) launcherHealthRoutes() {
	s.handle("GET /api/v1/launchers/health", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		apierr.WriteJSON(w, map[string]any{"records": s.healthRecords(r.Context(), q.Get("state"), q.Get("machine"))})
	})
	// A check now: of one launcher on one machine, all of a machine's, or every reachable machine's.
	s.handle("POST /api/v1/launchers/health/check", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()
		q := r.URL.Query()
		machine, launcher := q.Get("machine"), q.Get("launcher")
		cfg := s.healthConfig(ctx)
		cfg.interval = 0 // everything is due
		var out []LauncherHealth
		for _, d := range s.devices(ctx) {
			if (machine != "" && d.ID != machine) || !d.Reachable {
				continue
			}
			out = append(out, s.checkMachine(ctx, d, cfg, false, launcher)...)
		}
		if out == nil {
			out = []LauncherHealth{}
		}
		apierr.WriteJSON(w, map[string]any{"records": out})
	})
}
