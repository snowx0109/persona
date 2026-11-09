package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// The errors seen on broken machines (placeholders for the machines; the texts as the CLIs and gateways wrote them).
func TestClassifyLauncherError(t *testing.T) {
	for text, want := range map[string]string{
		`API Error: 400 {"error":{"message":"Bare Claude model name cannot be routed"}}`:        errKindModelRoute,
		`API Error: 400 no account match for model speech_o50`:                                  errKindModelRoute,
		`session.start: context deadline exceeded`:                                              errKindTimeout,
		`90 秒内没有回答`:                                                                             errKindTimeout,
		`API Error: 401 {"error":"invalid or missing API key"}`:                                 errKindAuth,
		`Not logged in · Please run /login`:                                                     errKindAuth,
		"`model_provider = \"trae\"` requires Trae auth. Please run `traecli auth login --sso`": errKindAuth,
		`ERROR codex_models_manager: No Trae auth token available for Trae model fetch`:         errKindAuth,
		`error: unknown option '--plugin-dir'`:                                                  errKindCLIOutdated,
		`Claude Code version 2.0.1 is no longer supported, please update`:                       errKindCLIOutdated,
		`Claude Code 还没完成首次引导（hasCompletedOnboarding 未置为 true）：请在这台机器的终端里运行一次 claude`:                                                                             errKindOnboarding,
		`claude did not become ready within 1m0s of starting: this machine's Claude Code has not completed its first-run setup (hasCompletedOnboarding is unset)`: errKindOnboarding,
		`zsh: command not found: work`: errKindOther,
	} {
		if got := classifyLauncherError(text); got != want {
			t.Errorf("%q: %s, want %s", text, got, want)
		}
	}
}

// First-run setup is a person's job: an onboarding failure never starts an automatic repair, however many probes
// failed in a row; periodic and on-demand checks clear it once the setup is finished.
func TestOnboardingFailureNeverRepairs(t *testing.T) {
	r := LauncherHealth{Kind: errKindOnboarding, Fails: 5}
	if r.repairNow() {
		t.Fatal("an onboarding failure must never enter auto repair")
	}
	if (LauncherHealth{Kind: errKindOnboarding}).repairNow() {
		t.Fatal("an onboarding failure must not repair on its first failure either")
	}
}

// launcherHealthHub is a hub with three machines (the assistant's "server", a broken "l4", a healthy "box") that all run the
// launcher "work", which model settings › Agents starts with.
func launcherHealthHub(t *testing.T) (*Service, *fakeHub) {
	return launcherHealthHubOn(t, newTestService)
}

// launcherHealthHubOn is launcherHealthHub with the service made by mk (newStorelessService keeps plans in memory;
// with no database the model settings stay as it is, so tasks name the launcher).
func launcherHealthHubOn(t *testing.T, mk func(*testing.T, *fakeHub) *Service) (*Service, *fakeHub) {
	t.Helper()
	f := newFakeHub().withMachine()
	f.sessions["l4"], f.sessions["box"] = nil, nil
	f.devices = []Device{
		{ID: "server", Name: "server", Home: "/home/me", Online: true, Reachable: true},
		{ID: "l4", Name: "l4", Home: "/home/me", SSHHost: "l4-ssh", Online: true, Reachable: true},
		{ID: "box", Name: "box", Home: "/Users/me", Online: true, Reachable: true},
	}
	f.launcherList = []proto.Launcher{{Name: "work"}}
	f.checks = map[string]proto.LauncherCheck{
		"l4/work": {Agent: "claude", Error: `API Error: 400 Bare Claude model name cannot be routed`},
	}
	f.hello.Agents.LauncherHealth.Enabled = new(true) // off unless hub.json says so
	s := mk(t, f)
	if s.store() != nil {
		setModel(t, s, "agent.new", ModelBinding{Provider: "launcher.work"})
	}
	return s, f
}

func healthState(s *Service, machine, launcher string) LauncherHealth {
	r, _ := s.launcherHealth(context.Background(), machine, launcher)
	return r
}

// A round checks every launcher once per interval; one that fails a known way, with no repair configured, is blocked;
// agents_list shows it, and agent_start goes to a machine where it works (the directory follows the home).
func TestLauncherHealthRoundAndDetour(t *testing.T) {
	s, f := launcherHealthHub(t)
	f.checks["server/work"] = proto.LauncherCheck{Error: "90 秒内没有回答"} // a first timeout: not repaired yet, not used either
	ctx := context.Background()
	s.healthRound(ctx, true)
	r := healthState(s, "l4", "work")
	if r.State != healthBlocked || r.Kind != errKindModelRoute || !strings.Contains(r.BlockedReason, "no repair agent") || r.Source != "check" {
		t.Fatalf("l4: %+v", r)
	}
	if ok := healthState(s, "box", "work"); ok.State != healthOK || ok.Model != "m" {
		t.Fatalf("box: %+v", ok)
	}
	if r := healthState(s, "server", "work"); r.State != healthFailing || r.Kind != errKindTimeout || len(f.starts) != 0 {
		t.Fatalf("server: %+v (starts %v)", r, f.starts)
	}
	n := len(f.checked)
	s.healthRound(ctx, true)
	if len(f.checked) != n {
		t.Fatalf("checked again within the interval: %v", f.checked[n:])
	}
	if len(f.pushes) != 0 {
		t.Fatalf("pushed: %+v", f.pushes)
	}

	list := s.describeAgents(ctx, "all", "", "")
	if !strings.Contains(list, "machine=l4 launcher=work · blocked · model_route") || !strings.Contains(list, "machine=server launcher=work · failing · timeout") ||
		strings.Contains(list, "machine=box") {
		t.Fatalf("agents_list:\n%s", list)
	}
	res, err := s.runTool(ctx, "server", "conv9", "agent_start", json.RawMessage(`{"machine":"l4","cwd":"/home/me/code/app","prompt":"修 CI"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.starts) != 1 || f.starts[0]["cwd"] != "/Users/me/code/app" || !strings.Contains(res, "runs on box instead") {
		t.Fatalf("start %v: %s", f.starts, res)
	}
	var route struct {
		Launchers []launcherWithHealth `json:"launchers"`
	}
	body := getJSON(t, s, "/api/v1/machines/l4/launchers")
	if err := json.Unmarshal(body, &route); err != nil || len(route.Launchers) == 0 || route.Launchers[0].Health == nil || route.Launchers[0].Health.State != healthBlocked {
		t.Fatalf("picker: %s", body)
	}
	if body := getJSON(t, s, "/api/v1/launchers/health?state=blocked"); !strings.Contains(string(body), `"machine":"l4"`) || strings.Contains(string(body), `"machine":"box"`) {
		t.Fatalf("health: %s", body)
	}

	// Fixed by hand: the next check (forced here) makes it usable again.
	delete(f.checks, "l4/work")
	postJSON(t, s, "/api/v1/launchers/health/check?machine=l4&launcher=work")
	if r := healthState(s, "l4", "work"); r.State != healthOK || r.BlockedReason != "" || r.Fails != 0 {
		t.Fatalf("after the fix: %+v", r)
	}
}

// Off unless hub.json turns it on: no checks, no repairs; a failed start still counts.
func TestLauncherHealthOffByDefault(t *testing.T) {
	s, f := launcherHealthHub(t)
	f.hello.Agents.LauncherHealth.Enabled = nil
	f.hello.Agents.LauncherHealth.Repair.Skill = "agent-cli-setup"
	s.healthRound(context.Background(), true)
	if len(f.checked) != 0 || len(f.starts) != 0 || len(s.healthRecords(context.Background(), "", "")) != 0 {
		t.Fatalf("checked %v, starts %v", f.checked, f.starts)
	}
	f.mu.Lock()
	f.checks["box/work"] = proto.LauncherCheck{Error: "90 秒内没有回答"} // the follow-up check fails too
	f.failStart = errors.New("session.start: context deadline exceeded")
	f.mu.Unlock()
	if _, err := s.runTool(context.Background(), "server", "conv9", "agent_start", json.RawMessage(`{"machine":"box","cwd":"/Users/me/x","prompt":"hi"}`)); err == nil {
		t.Fatal("started")
	}
	if r := healthState(s, "box", "work"); r.Fails == 0 || r.State != healthFailing {
		t.Fatalf("not recorded: %+v", r)
	}
}

// A CLI too old is the node's to update; the launcher checks again and works.
func TestLauncherHealthUpgradesTheCLI(t *testing.T) {
	s, f := launcherHealthHub(t)
	f.checks["l4/work"] = proto.LauncherCheck{Agent: "claude", Error: "error: unknown option '--plugin-dir'"}
	f.repairRes = &proto.LauncherRepair{OK: true, Output: "Successfully updated"}
	f.onRepair = func(f *fakeHub) { delete(f.checks, "l4/work") }
	s.healthRound(context.Background(), true)
	r := healthState(s, "l4", "work")
	if r.State != healthOK || r.Repair == nil || r.Repair.Action != launcherRepairUpgrade || !r.Repair.OK || len(f.repairs) != 1 || f.repairs[0]["action"] != "upgrade_cli" {
		t.Fatalf("%+v (repairs %v)", r, f.repairs)
	}
}

// Without a node that can update it, and with a repair skill configured, an agent on the assistant's machine gets the
// broken launchers (on a quiet todo); once it stops they are checked again: fixed, or blocked with what it said.
func TestLauncherHealthRepairAgent(t *testing.T) {
	for _, fixed := range []bool{true, false} {
		s, f := launcherHealthHub(t)
		f.hello.Agents.LauncherHealth.Repair.Skill = "agent-cli-setup"
		ctx := context.Background()
		s.healthRound(ctx, true)
		r := healthState(s, "l4", "work")
		if r.State != healthRepairing || r.Repair == nil || r.Repair.Action != "agent" || len(f.starts) != 1 {
			t.Fatalf("repairing: %+v, starts %v", r, f.starts)
		}
		start := f.starts[0]
		prompt, _ := start["prompt"].(string)
		if start["cwd"] != "/home/me" || start["launcher"] != "work" || start["permission_mode"] != "bypassPermissions" || !strings.Contains(prompt, "agent-cli-setup") ||
			!strings.Contains(prompt, "Bare Claude model name") || !strings.Contains(prompt, "ssh l4-ssh") {
			t.Fatalf("repair start: %v", start)
		}
		todo := onlyTodo(s)
		if !todo.Quiet || todo.Origin != "" || len(todo.Agents) != 1 || todo.Agents[0].MachineID != "server" {
			t.Fatalf("todo: %+v", todo)
		}
		// Not before it has had a moment, nor while it works.
		f.mu.Lock()
		f.sessions["server"] = []proto.Session{{SessionID: "s1", MachineID: "server", State: proto.StateWorking, Status: proto.StatusWorking}}
		f.mu.Unlock()
		s.followRepairs(ctx, s.healthConfig(ctx))
		if healthState(s, "l4", "work").State != healthRepairing {
			t.Fatal("followed too early")
		}
		s.health.mu.Lock()
		s.health.repairs["l4"].StartedAt = time.Now().Add(-3 * time.Minute)
		s.health.mu.Unlock()
		s.followRepairs(ctx, s.healthConfig(ctx))
		if healthState(s, "l4", "work").State != healthRepairing {
			t.Fatal("followed while it works")
		}
		f.mu.Lock()
		f.sessions["server"] = []proto.Session{{SessionID: "s1", MachineID: "server", State: proto.StateUnread, Status: proto.StatusIdle, LastText: "reconfigured the gateway"}}
		if fixed {
			delete(f.checks, "l4/work")
		}
		f.mu.Unlock()
		s.followRepairs(ctx, s.healthConfig(ctx))
		r = healthState(s, "l4", "work")
		if fixed && (r.State != healthOK || r.Repair == nil || !r.Repair.OK) {
			t.Fatalf("fixed: %+v", r)
		}
		if !fixed && (r.State != healthBlocked || !strings.Contains(r.BlockedReason, "reconfigured the gateway")) {
			t.Fatalf("not fixed: %+v", r)
		}
		if len(s.health.repairs) != 0 || len(f.pushes) != 0 {
			t.Fatalf("repairs left %v, pushes %v", s.health.repairs, f.pushes)
		}
	}
}

// A start that fails counts against the launcher there (a check follows to confirm); a dropped link does not.
func TestLauncherHealthRecordsFailedStarts(t *testing.T) {
	s, f := launcherHealthHub(t)
	delete(f.checks, "l4/work")
	f.checks["box/work"] = proto.LauncherCheck{Error: "90 秒内没有回答"}
	f.failStart = errors.New("session.start: context deadline exceeded")
	ctx := context.Background()
	if _, err := s.runTool(ctx, "server", "conv9", "agent_start", json.RawMessage(`{"machine":"box","cwd":"/Users/me/x","prompt":"hi"}`)); err == nil {
		t.Fatal("started")
	}
	waitFor(t, "the follow-up check", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.checked) > 0
	})
	waitFor(t, "the record", func() bool { r := healthState(s, "box", "work"); return r.Source == "check" && r.Fails == 2 })
	if r := healthState(s, "box", "work"); r.State != healthFailing || r.Kind != errKindTimeout {
		t.Fatalf("%+v", r)
	}
	s.recordStartFailure(ctx, "l4", "work", fmt.Errorf("session.start: %w", rpc.ErrClosed))
	if _, ok := s.launcherHealth(ctx, "l4", "work"); ok {
		t.Fatal("a dropped link counted")
	}
}

// A task planned on a machine whose launcher does not work there is put on one where it does, and says why.
func TestLauncherHealthMovesPlannedTasks(t *testing.T) {
	s, f := launcherHealthHubOn(t, newStorelessService)
	f.checks["server/work"] = proto.LauncherCheck{Error: "90 秒内没有回答"}
	ctx := context.Background()
	s.healthRound(ctx, true)
	res, err := s.toolTaskPlan(ctx, "conv9", json.RawMessage(`{"tasks":[{"title":"修 CI","prompt":"修 CI","machine":"l4","cwd":"~/code/app","launcher":"work",
		"readonly":false,"user_requested":false,"machine_policy":"flexible","why":"代码在那"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	id := regexp.MustCompile(`plan_[0-9a-f]+`).FindString(res)
	p, err := s.loadPlan(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if task := p.Tasks[0]; task.Machine != "box" || task.Cwd != "/Users/me/code/app" || !strings.Contains(task.Why, "failed its health check on l4") {
		t.Fatalf("task: %+v", task)
	}
}

// A start aimed at a machine whose launcher has a failing record probes that machine once on the spot: when the
// launcher works now (fixed since the record), the record is cleared and the start stays on the asked machine (#2018).
func TestLauncherHealthRecheckClearsStaleRecord(t *testing.T) {
	s, f := launcherHealthHub(t)
	ctx := context.Background()
	s.healthRound(ctx, true) // l4/work fails and is blocked
	if r := healthState(s, "l4", "work"); !r.unusable() {
		t.Fatalf("seed record: %+v", r)
	}
	delete(f.checks, "l4/work") // the launcher was fixed after the record was written
	n := len(f.checked)
	res, err := s.runTool(ctx, "server", "conv9", "agent_start", json.RawMessage(`{"machine":"l4","cwd":"/home/me/code/app","prompt":"修 CI"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.checked) != n+1 || f.checked[n] != "l4/work" {
		t.Fatalf("on-demand check: %v", f.checked[n:])
	}
	if r := healthState(s, "l4", "work"); r.State != healthOK || r.Fails != 0 || r.Source != "check" {
		t.Fatalf("record not cleared: %+v", r)
	}
	if len(f.starts) != 1 || f.starts[0]["cwd"] != "/home/me/code/app" {
		t.Fatalf("the start did not stay on l4: %v", f.starts)
	}
	if strings.Contains(res, "runs on") {
		t.Fatalf("the start was rerouted: %s", res)
	}
}

// An on-demand probe that still fails leaves the record untouched and the start reroutes as before; a second start
// within healthRecheckThrottle trusts that answer and does not probe again. A machine that cannot answer the probe
// gives no result and reroutes the same way.
func TestLauncherHealthRecheckThrottleAndFailure(t *testing.T) {
	s, f := launcherHealthHub(t)
	ctx := context.Background()
	f.checks["server/work"] = proto.LauncherCheck{Error: "90 秒内没有回答"} // box is the only healthy alternative
	s.healthRound(ctx, true)
	before := healthState(s, "l4", "work")
	n := len(f.checked)
	res, err := s.runTool(ctx, "server", "conv9", "agent_start", json.RawMessage(`{"machine":"l4","cwd":"/home/me/code/app","prompt":"修 CI"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.checked) != n+1 || f.checked[n] != "l4/work" {
		t.Fatalf("on-demand check: %v", f.checked[n:])
	}
	if len(f.starts) != 1 || f.starts[0]["cwd"] != "/Users/me/code/app" || !strings.Contains(res, "runs on box instead") {
		t.Fatalf("not rerouted: starts %v, answer %s", f.starts, res)
	}
	if r := healthState(s, "l4", "work"); r.CheckedAt != before.CheckedAt || r.Fails != before.Fails || r.State != before.State {
		t.Fatalf("the failed probe changed the record: before %+v, after %+v", before, r)
	}

	// Throttled: the next start does not probe again but still reroutes.
	n = len(f.checked)
	if _, err := s.runTool(ctx, "server", "conv10", "agent_start", json.RawMessage(`{"machine":"l4","cwd":"/home/me/code/app","prompt":"再修"}`)); err != nil {
		t.Fatal(err)
	}
	if len(f.checked) != n {
		t.Fatalf("probed again within the throttle: %v", f.checked[n:])
	}
	if len(f.starts) != 2 {
		t.Fatalf("starts: %v", f.starts)
	}

	// The node can no longer answer: the probe gives no result and the detour still moves the start to a healthy one.
	l4 := f.devices[1]
	f.mu.Lock()
	delete(f.sessions, "l4")
	f.mu.Unlock()
	s.health.mu.Lock()
	delete(s.health.recheckAt, healthKey("l4", "work"))
	s.health.mu.Unlock()
	if _, got, checked := s.recheckLauncherNow(ctx, "l4", "work"); !checked || got {
		t.Fatalf("want no answer from the gone machine, got checked=%v got=%v", checked, got)
	}
	alt, _, moved := s.healthDetour(ctx, l4, "work")
	if !moved || alt.ID != "box" {
		t.Fatalf("unreachable detour: moved=%v alt=%s", moved, alt.ID)
	}
}

// Records a probe cannot settle are not rechecked on the spot: a subscription usage limit (the request would be
// refused too) and a launcher missing from the terminal's shell (the probe's login shell runs it fine).
func TestLauncherHealthRecheckSkipsSpecialRecords(t *testing.T) {
	s, f := launcherHealthHub(t)
	ctx := context.Background()
	s.recordHealth(ctx, "l4", "work", healthResult{err: "usage limit reached", source: "check"})
	if r := healthState(s, "l4", "work"); r.Kind != errKindUsageLimit {
		t.Fatalf("seed usage_limit: %+v", r)
	}
	n := len(f.checked)
	if _, note, moved := s.healthDetour(ctx, f.devices[1], "work"); !moved || !strings.Contains(note, "usage limit") {
		t.Fatalf("usage limit detour: moved=%v note=%q", moved, note)
	}
	if len(f.checked) != n {
		t.Fatalf("a usage limit was probed: %v", f.checked[n:])
	}

	editHealth(s, "l4", "work", func(r *LauncherHealth) {
		r.State, r.Kind, r.BlockedReason = healthBlocked, errKindNotInTerminal, "the new tab's shell (bash) has no command work"
	})
	n = len(f.checked)
	if _, note, moved := s.healthDetour(ctx, f.devices[1], "work"); !moved || !strings.Contains(note, errKindNotInTerminal) {
		t.Fatalf("terminal detour: moved=%v note=%q", moved, note)
	}
	if len(f.checked) != n {
		t.Fatalf("a terminal-shell record was probed: %v", f.checked[n:])
	}
}

// Two starts for the same failing launcher at once run one on-demand probe; the second waits for its answer.
func TestLauncherHealthRecheckSingleFlight(t *testing.T) {
	s, f := launcherHealthHub(t)
	ctx := context.Background()
	s.recordHealth(ctx, "l4", "work", healthResult{err: "broken", source: "check"})
	delete(f.checks, "l4/work") // the probe now succeeds; the stale record is what is wrong
	release, entered := make(chan struct{}), make(chan struct{})
	f.onCheck = func(*fakeHub) {
		close(entered)
		<-release
	}
	type answer struct {
		res     healthResult
		got     bool
		checked bool
	}
	answers := make(chan answer, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, g, c := s.recheckLauncherNow(context.Background(), "l4", "work")
		answers <- answer{r, g, c}
	}()
	<-entered
	waitFor(t, "the in-flight probe", func() bool {
		s.health.mu.Lock()
		n := len(s.health.recheck)
		s.health.mu.Unlock()
		return n == 1
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, g, c := s.recheckLauncherNow(context.Background(), "l4", "work")
		answers <- answer{r, g, c}
	}()
	// Both callers must be parked in recheckLauncherNow before the probe is allowed to finish, or the second one
	// could arrive after the cleanup and take the throttle branch instead of joining the probe.
	waitFor(t, "the second caller to join the probe", func() bool {
		s.health.mu.Lock()
		rc := s.health.recheck[healthKey("l4", "work")]
		joined := rc != nil && rc.waiters >= 1
		s.health.mu.Unlock()
		return joined
	})
	close(release)
	wg.Wait()
	for i := 0; i < 2; i++ {
		a := <-answers
		if !a.checked || !a.got || !a.res.ok {
			t.Fatalf("caller %d got %+v", i, a)
		}
	}
	s.health.mu.Lock()
	left := len(s.health.recheck)
	s.health.mu.Unlock()
	if left != 0 {
		t.Fatalf("in-flight entries left: %d", left)
	}
	f.mu.Lock()
	probes := 0
	for _, c := range f.checked {
		if c == "l4/work" {
			probes++
		}
	}
	f.mu.Unlock()
	if probes != 1 {
		t.Fatalf("probes ran: %d", probes)
	}
}

func getJSON(t *testing.T, s *Service, path string) []byte {
	t.Helper()
	w := serve(s, "GET", path, "")
	if w.Code != 200 {
		t.Fatalf("GET %s: %d %s", path, w.Code, w.Body)
	}
	return w.Body.Bytes()
}

func postJSON(t *testing.T, s *Service, path string) []byte {
	t.Helper()
	w := serve(s, "POST", path, "")
	if w.Code != 200 {
		t.Fatalf("POST %s: %d %s", path, w.Code, w.Body)
	}
	return w.Body.Bytes()
}

// A start the node turned down before running the launcher (a directory that is not there, a coded refusal of the
// request) says nothing about the launcher: it is not recorded, and a planned task is not moved off the machine for it
// (#3790). A failure of the run itself, or the machine not able to run it now, still counts.
func TestLauncherHealthIgnoresRefusedStarts(t *testing.T) {
	s, f := launcherHealthHubOn(t, newStorelessService)
	delete(f.checks, "l4/work")
	ctx := context.Background()
	for _, err := range []error{
		&rpc.Error{Message: "directory /home/me/nope does not exist"},
		fmt.Errorf("session.start: %w", &rpc.Error{Message: `unknown permission mode "yolo"`}),
		&rpc.Error{Message: "invalid model"},
		&rpc.Error{Code: "codex_needs_prompt", Message: "新建 Codex 会话要连第一条消息一起发"},
		&rpc.Error{Code: proto.CodeNodeOutdated, Message: "node too old"},
		&rpc.Error{Code: "ultracode_unsupported", Message: "Claude Code 版本太旧"},
	} {
		s.recordStartFailure(ctx, "l4", "work", err)
		if r, ok := s.launcherHealth(ctx, "l4", "work"); ok {
			t.Fatalf("%v counted against the launcher: %+v", err, r)
		}
	}
	for _, err := range []error{
		&rpc.Error{Message: "wait for claude session: context deadline exceeded"},
		&rpc.Error{Code: "engine_missing", Message: "herdr is not running"},
		&rpc.Error{Code: "runtime_away", Message: "runtime not connected"},
		errors.New("session.start: context deadline exceeded"),
	} {
		if startRefused(err) {
			t.Fatalf("%v read as a refused request", err)
		}
	}

	f.mu.Lock()
	f.failStart = &rpc.Error{Message: "directory /home/me/nope does not exist"}
	f.mu.Unlock()
	s.startPlanTask("plan_x", "conv1", 0, PlanTask{Title: "修 CI", Prompt: "修 CI", Machine: "l4", Cwd: "/home/me/nope", Launcher: "work"}, false)
	f.mu.Lock()
	starts := append([]map[string]any(nil), f.starts...)
	checked := len(f.checked)
	f.mu.Unlock()
	if len(starts) != 1 || checked != 0 {
		t.Fatalf("a bad directory was retried elsewhere or checked: starts %v, checks %d", starts, checked)
	}
	if r, ok := s.launcherHealth(ctx, "l4", "work"); ok {
		t.Fatalf("recorded: %+v", r)
	}
}
