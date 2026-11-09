package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nwverse/persona/server/internal/apierr"
	"github.com/nwverse/persona/server/internal/assistantwire"
	"github.com/nwverse/persona/server/internal/i18n"
	"github.com/nwverse/persona/server/internal/lazyre"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// The main conversation's readiness (docs/assistant/onboarding.md 4.4, 5.1): whether it can answer now and, when not, the first
// thing missing, for the clients' setup card and checklist. Four steps in a fixed order: the computer it runs on,
// Claude Code there, the model, and a first answer. Everything comes from what the assistant already knows (the
// machines, the launcher in effect, the main conversation's last turn, the launcher checks) plus two free questions to
// the node, both cached: its self-check for Claude Code's version, and claude.auth_status for whether Claude Code is
// signed in as the launcher runs it. No tokens are spent unless the client asks for a probe. The last result and the
// user's "Later" are kept in the settings (readinessKey).

// readinessKey is the setting the readiness record is kept in.
const readinessKey = "assistant.readiness"

var (
	// readinessProbeReuse: a probe asked for again this soon gets the last one's result (each one spends tokens).
	readinessProbeReuse = 30 * time.Second
	// readinessClaudeFresh: how long a machine's Claude Code version (node.doctor) is used before it is asked again;
	// one that read no version (a failed ask, Claude Code not there yet) is asked again after readinessClaudeRetry.
	readinessClaudeFresh = 10 * time.Minute
	readinessClaudeRetry = time.Minute
	// readinessAuthFresh: how long a launcher's sign-in status (claude.auth_status) is used before it is asked again
	// (also an old node's unknown method). Signed out, or not answered (a failed ask, a timeout, unreadable output), it
	// is asked again after readinessClaudeRetry: in setup mode nothing else comes to say the user has signed in since.
	readinessAuthFresh = 10 * time.Minute
	// How long a readiness waits for the node's self-check before saying checking (the check goes on and is cached):
	// GET waits for it, the warm-up answers quickly.
	readinessWaitGet  = 12 * time.Second
	readinessWaitWarm = 2 * time.Second
	// readinessQuickSlack: what warm waits past readinessWaitWarm for the rest of the work before it answers the
	// last readiness kept; readinessQuickMax bounds that work going on in the background.
	readinessQuickSlack = 500 * time.Millisecond
	readinessQuickMax   = 30 * time.Second
)

// Readiness modes, step ids and states (docs/assistant/onboarding.md 5.1).
const (
	ReadinessFull     = "full"
	ReadinessSetup    = "setup"
	ReadinessChatOnly = "chat_only" // P1: not answered yet

	StepComputer     = "computer"
	StepClaude       = "claude"
	StepModel        = "model"
	StepFirstMessage = "first_message"

	StepOK       = "ok"
	StepTodo     = "todo"
	StepProblem  = "problem"
	StepChecking = "checking"
	StepUnknown  = "unknown"
	StepSkipped  = "skipped"
)

// Step codes the readiness has beyond proto's (docs/assistant/onboarding.md 4.6).
const (
	codeNodeUnreachable = "node_unreachable"
	codeNodeOutdated    = "node_outdated"
	codeModelUnverified = "model_unverified"
	codeNeverTried      = "never_tried"
)

// Readiness is GET /api/v1/assistant/readiness's answer.
type Readiness struct {
	Version     int                   `json:"version"`
	Mode        string                `json:"mode"`
	Ready       bool                  `json:"ready"`
	Next        *string               `json:"next"`    // the first step not ok or skipped; null when there is none
	Machine     *ReadinessMachine     `json:"machine"` // the main conversation's machine; null when there is none
	Launcher    string                `json:"launcher"`
	Steps       []ReadinessStep       `json:"steps"`
	Suggestions []ReadinessSuggestion `json:"suggestions"`
	// Probe is the one-word check's result, only with ?probe=1 (and a machine to run it on).
	Probe     *proto.LauncherCheck `json:"probe,omitempty"`
	Dismissed bool                 `json:"dismissed"`
	CheckedAt time.Time            `json:"checked_at"`
}

// ReadinessMachine names the main conversation's machine.
type ReadinessMachine struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ReadinessStep is one step: its state, and when it is todo or a problem the code, the words and what to do.
type ReadinessStep struct {
	ID         string            `json:"id"`
	State      string            `json:"state"`
	Code       string            `json:"code,omitempty"`
	Machine    string            `json:"machine,omitempty"`
	Launcher   string            `json:"launcher,omitempty"`
	Version    string            `json:"version,omitempty"` // Claude Code's, when known
	Title      string            `json:"title,omitempty"`
	Detail     string            `json:"detail,omitempty"`
	Action     *ReadinessAction  `json:"action,omitempty"`
	AltActions []ReadinessAction `json:"alt_actions,omitempty"`
}

// ReadinessAction is a button: its id names what the client opens (add_computer, connect_model…).
type ReadinessAction struct {
	ID string `json:"id"`
}

// ReadinessSuggestion is an example question for the main conversation's first screen: a local one opens the
// client's own help and is not sent to the model; needs are the system permissions sending it will ask for.
type ReadinessSuggestion struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Prompt string   `json:"prompt,omitempty"`
	Local  bool     `json:"local,omitempty"`
	Needs  []string `json:"needs,omitempty"`
}

// readinessRecord is what readinessKey holds.
type readinessRecord struct {
	Dismissed   bool       `json:"dismissed"`
	DismissedAt *time.Time `json:"dismissed_at,omitempty"`
	// Probe is the last probe's result, for the machine and launcher it ran with.
	Probe *readinessProbe `json:"probe,omitempty"`
	// Start is the last coded refusal of a start (claude_missing, launcher_missing), cleared by a start that went.
	Start *readinessStart `json:"start,omitempty"`
	// AnsweredAt: the main conversation answered once (first_message stays ok after it).
	AnsweredAt *time.Time `json:"answered_at,omitempty"`
	// Last is the last readiness worked out (without suggestions and probe).
	Last *Readiness `json:"last,omitempty"`
	// RoutinesWaiting: since then scheduled runs were skipped for not being ready; the first one that goes again
	// clears it and notes it in the main conversation (schedule_ready.go).
	RoutinesWaiting *time.Time `json:"routines_waiting,omitempty"`
}

type readinessProbe struct {
	Machine  string              `json:"machine"`
	Launcher string              `json:"launcher"`
	At       time.Time           `json:"at"`
	Check    proto.LauncherCheck `json:"check"`
}

type readinessStart struct {
	Machine  string    `json:"machine"`
	Launcher string    `json:"launcher"`
	Code     string    `json:"code"`
	Detail   string    `json:"detail,omitempty"`
	At       time.Time `json:"at"`
}

// claudeSeen is a machine's Claude Code as its node's self-check found it.
type claudeSeen struct {
	known   bool   // the node answered with a claude-version check
	version string // "" when it could not read one
	at      time.Time
}

// claudeAuth is a launcher's sign-in status on a machine as its node's claude.auth_status found it.
type claudeAuth struct {
	known bool // the node answered (an old one has no such method)
	old   bool // the node has no claude.auth_status: asked again only after readinessAuthFresh
	st    proto.ClaudeAuthStatus
	at    time.Time
}

// readinessBook keeps the record (read once from the settings), the machines' Claude Code versions and the
// launchers' sign-in status.
type readinessBook struct {
	mu      sync.Mutex
	loaded  bool
	rec     readinessRecord
	probing chan struct{} // closed when the probe on its way is done

	claude   map[string]claudeSeen    // by machine
	claudeIn map[string]chan struct{} // the self-checks on their way, closed when done

	auth   map[string]claudeAuth    // by machine + "/" + launcher
	authIn map[string]chan struct{} // the questions on their way, closed when done
}

// readinessLoadLocked reads the record once (with no database, or one that does not answer, it starts empty and
// is read again next time).
func (s *Service) readinessLoadLocked(ctx context.Context) {
	b := &s.ready
	if b.loaded {
		return
	}
	st := s.store()
	if st == nil {
		b.loaded = true
		return
	}
	var rec readinessRecord
	if _, err := st.Setting(ctx, readinessKey, &rec); err != nil {
		s.logf("assistant: readiness: %v", err)
		return
	}
	b.rec, b.loaded = rec, true
}

// readinessUpdate changes the record with fn and saves it.
func (s *Service) readinessUpdate(ctx context.Context, fn func(*readinessRecord)) error {
	b := &s.ready
	b.mu.Lock()
	defer b.mu.Unlock()
	s.readinessLoadLocked(ctx)
	st := s.store()
	if !b.loaded && st != nil {
		// The record could not be read: writing fn's change over an empty one would wipe what is kept.
		return errors.New("assistant readiness: the record could not be read")
	}
	fn(&b.rec)
	if st != nil {
		return st.PutSetting(ctx, readinessKey, b.rec)
	}
	return nil
}

func (s *Service) readinessRecordNow(ctx context.Context) readinessRecord {
	b := &s.ready
	b.mu.Lock()
	defer b.mu.Unlock()
	s.readinessLoadLocked(ctx)
	return b.rec
}

// startRefused: a start on machine with launcher was refused with err (nil: it went). A coded refusal that says
// Claude Code or the launcher is missing is remembered for the claude step; a start that went clears it.
func (s *Service) startRefused(ctx context.Context, machine, launcher string, err error) {
	var re *rpc.Error
	coded := errors.As(err, &re) && (re.Code == proto.CodeClaudeMissing || re.Code == proto.CodeLauncherMissing)
	if err != nil && !coded {
		return
	}
	cur := s.readinessRecordNow(ctx).Start
	if err == nil && (cur == nil || cur.Machine != machine || cur.Launcher != launcher) {
		return // nothing to clear: no write on every start
	}
	if uerr := s.readinessUpdate(ctx, func(r *readinessRecord) {
		r.Start = nil
		if coded {
			r.Start = &readinessStart{Machine: machine, Launcher: launcher, Code: re.Code, Detail: re.Message, At: time.Now().UTC()}
		}
	}); uerr != nil {
		s.logf("assistant: readiness: %v", uerr)
	}
}

// clearStart forgets the remembered refusal st (when it is still the one kept).
func (s *Service) clearStart(ctx context.Context, st readinessStart) {
	if err := s.readinessUpdate(ctx, func(r *readinessRecord) {
		if r.Start != nil && r.Start.At.Equal(st.At) {
			r.Start = nil
		}
	}); err != nil {
		s.logf("assistant: readiness: %v", err)
	}
}

// ---- the routes ----

func (s *Service) readinessRoutes() {
	s.handle("GET /api/v1/assistant/readiness", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("probe")
		// A probe spends tokens: only a full device starts one (the hub relays this route for full devices alone).
		probe := (q == "1" || q == "true") && fullRole(r)
		apierr.WriteJSON(w, s.readiness(r.Context(), probe, readinessWaitGet))
	})
	s.handle("POST /api/v1/assistant/readiness/dismiss", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Dismissed *bool `json:"dismissed"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				apierr.Write(w, http.StatusBadRequest, err)
				return
			}
		}
		on := in.Dismissed == nil || *in.Dismissed // an empty body dismisses
		if err := s.readinessUpdate(r.Context(), func(rec *readinessRecord) {
			rec.Dismissed, rec.DismissedAt = on, nil
			if on {
				now := time.Now().UTC()
				rec.DismissedAt = &now
			}
		}); err != nil {
			apierr.Write(w, http.StatusInternalServerError, err)
			return
		}
		apierr.WriteJSON(w, map[string]any{"ok": true})
	})
}

// ---- working it out ----

// readinessEvidence is what something said of the model lately: it answered (ok), or failed with text (or, said
// outright, code).
type readinessEvidence struct {
	at   time.Time
	ok   bool
	text string
	code string
}

// readiness works out the main conversation's readiness; probe runs the one-word check first (or reuses one made in
// the last readinessProbeReuse); wait bounds how long it waits for the node's self-check.
func (s *Service) readiness(ctx context.Context, probe bool, wait time.Duration) Readiness {
	lang := s.requestCtxLang(ctx)
	v := s.settings(ctx)
	launcher := s.launcherNow(ctx)
	out := Readiness{Version: 1, Launcher: launcher, CheckedAt: time.Now().UTC().Truncate(time.Second)}

	d, has := s.device(ctx, v.MachineID)
	computer := ReadinessStep{ID: StepComputer, State: StepOK}
	switch {
	case !has || v.MachineID == "":
		computer = s.readinessStep(lang, StepComputer, StepTodo, proto.CodeNoMachine, "", launcher, "add_computer")
	case !d.Online && !d.Reachable:
		computer = s.readinessStep(lang, StepComputer, StepProblem, codeNodeUnreachable, d.Name, launcher, "wake_computer")
	case d.Version != "" && !d.ModelConfiguration: // a node from before Model configuration cannot run the main conversation's model
		computer = s.readinessStep(lang, StepComputer, StepProblem, codeNodeOutdated, d.Name, launcher, "")
	}
	if has {
		out.Machine = &ReadinessMachine{ID: d.ID, Name: d.Name}
		computer.Machine = d.ID
	}
	out.Steps = append(out.Steps, computer)

	if computer.State != StepOK {
		for _, id := range []string{StepClaude, StepModel, StepFirstMessage} {
			out.Steps = append(out.Steps, ReadinessStep{ID: id, State: StepUnknown})
		}
	} else {
		s.askAhead(d.ID, launcher)
		if probe {
			out.Probe = s.readinessProbe(ctx, d.ID, launcher)
		}
		deadline := time.Now().Add(wait) // the node's two answers are waited for together, not one after the other
		rec := s.readinessRecordNow(ctx)
		main, hasMain := s.readinessMain(ctx, d.ID, v.Main)
		auth, _ := s.claudeAuthStatus(ctx, d.ID, launcher, time.Until(deadline))
		model, modelOK := s.readinessModel(ctx, lang, d, launcher, rec, main, hasMain, auth)
		first := s.readinessFirst(ctx, lang, d, rec, v, main, hasMain)
		// One button for the try: the first message's step offers it, and every client shows the try's spinner and
		// answer there, so an unverified model step that would offer the same leaves it to that step (#6317). Once the
		// first message is ok, the model step keeps its own.
		if model.Action != nil && model.Action.ID == "test_assistant" && first.Action != nil && first.Action.ID == "test_assistant" {
			model.Action = nil
		}
		out.Steps = append(out.Steps, s.readinessClaude(ctx, lang, d, launcher, rec, modelOK, time.Until(deadline)), model, first)
	}

	out.Mode = ReadinessFull
	for _, st := range out.Steps {
		if st.ID != StepFirstMessage && readinessBlocks(st) {
			out.Mode = ReadinessSetup
		}
		if out.Next == nil && st.State != StepOK && st.State != StepSkipped {
			id := st.ID
			out.Next = &id
		}
	}
	out.Ready = out.Mode == ReadinessFull
	out.Suggestions = readinessSuggestions(lang, out.Mode)
	rec := s.readinessRecordNow(ctx)
	out.Dismissed = rec.Dismissed
	s.readinessKeep(ctx, out, rec.Last)
	return out
}

// readinessQuick is readiness without a probe within about within: when working it out takes longer (a slow
// machine, the database) it goes on in the background and the last one kept is answered, or nil when none is.
func (s *Service) readinessQuick(ctx context.Context, within time.Duration) *Readiness {
	done := make(chan Readiness, 1)
	go func() {
		bctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), readinessQuickMax)
		defer cancel()
		done <- s.readiness(bctx, false, within)
	}()
	t := time.NewTimer(within + readinessQuickSlack)
	defer t.Stop()
	select {
	case r := <-done:
		return &r
	case <-t.C:
	}
	b := &s.ready
	b.mu.Lock()
	var last *Readiness
	if b.loaded && b.rec.Last != nil {
		r := *b.rec.Last
		r.Dismissed = b.rec.Dismissed
		last = &r
	}
	b.mu.Unlock()
	if last != nil {
		last.Suggestions = readinessSuggestions(s.requestCtxLang(ctx), last.Mode)
	}
	return last
}

// readinessBlocks: the step keeps the main conversation from answering (mode setup). A rate limit passes by itself.
func readinessBlocks(st ReadinessStep) bool {
	switch st.State {
	case StepTodo:
		return true
	case StepProblem:
		return st.Code != proto.TurnErrorRateLimited
	}
	return false
}

// readinessKeep saves the result when it differs from the last one (the time aside), so that a client opening the
// Persona tab does not write the database each time.
func (s *Service) readinessKeep(ctx context.Context, r Readiness, last *Readiness) {
	r.Suggestions, r.Probe = nil, nil
	if last != nil {
		a, b := *last, r
		a.CheckedAt, b.CheckedAt, a.Dismissed, b.Dismissed = time.Time{}, time.Time{}, false, false
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) == string(jb) {
			return
		}
	}
	if err := s.readinessUpdate(ctx, func(rec *readinessRecord) { rec.Last = &r }); err != nil {
		s.logf("assistant: readiness: %v", err)
	}
}

// readinessMain is the main conversation as its machine lists it (decorated: its turn error read off its last words).
func (s *Service) readinessMain(ctx context.Context, machine, main string) (proto.Session, bool) {
	if main == "" {
		return proto.Session{}, false
	}
	var list []proto.Session
	if s.fleetOn() {
		if m, ok := s.fleetMachine(machine); ok {
			if sess, found := s.nodeSession(m.ID, main); found && m.isAssistant(sess) {
				// Readiness needs one headless conversation, not a decorated
				// and serialized copy of every coding session on the machine.
				list = s.decorateInventory(m.ID, []proto.Session{sess}, false)
			}
		}
	}
	if list == nil {
		list, _ = s.sessions(ctx, machine)
	}
	for _, sess := range list {
		if sess.SessionID == main {
			if text, kind := turnError(sess); text != "" && sess.TurnError == "" {
				sess.TurnError, sess.TurnErrorKind = text, kind
			}
			if sess.TurnError != "" && sess.TurnErrorCode == "" {
				sess.TurnErrorCode = turnErrorCode(sess.TurnError)
			}
			return sess, true
		}
	}
	return proto.Session{}, false
}

// answeredWell: the session's latest turn ended with an answer, not an error.
func answeredWell(sess proto.Session) bool {
	return (sess.State == proto.StateIdle || sess.State == proto.StateUnread) && sess.TurnError == "" &&
		strings.TrimSpace(sess.LastText) != ""
}

// readinessModel is the model step from the newest of what is known: the last probe, the main conversation's last
// turn, the launcher's health record, Claude Code's sign-in status. ok reports that the newest said it answered (or
// that Claude Code is signed in to claude.ai). Signed in with a key or a gateway's token says nothing of whether the
// key works: no evidence, but an unknown step says what it runs on.
func (s *Service) readinessModel(ctx context.Context, lang i18n.Lang, d Device, launcher string, rec readinessRecord,
	main proto.Session, hasMain bool, auth claudeAuth) (ReadinessStep, bool) {
	var ev []readinessEvidence
	if p := rec.Probe; p != nil && p.Machine == d.ID && p.Launcher == launcher {
		ev = append(ev, readinessEvidence{at: p.At, ok: p.Check.OK, text: p.Check.Error})
	}
	if hasMain && (main.Launcher == "" || main.Launcher == launcher) {
		switch {
		case main.TurnError != "":
			ev = append(ev, readinessEvidence{at: main.UpdatedAt, text: main.TurnError})
		case answeredWell(main):
			ev = append(ev, readinessEvidence{at: main.UpdatedAt, ok: true})
		}
	}
	// A launcher only the terminal cannot run says nothing of the main conversation, which runs in the login shell.
	if h, ok := s.launcherHealth(ctx, d.ID, launcher); ok && h.State != "" && h.Kind != errKindNotInTerminal {
		switch {
		case h.Kind == errKindUsageLimit:
			// The subscription's limit, used up: it recovers on its own, and says nothing once it reset.
			if h.unusable() {
				ev = append(ev, readinessEvidence{at: h.CheckedAt, code: proto.TurnErrorRateLimited})
			}
		case time.Since(h.CheckedAt) >= healthStale:
		case h.State == healthOK:
			ev = append(ev, readinessEvidence{at: h.CheckedAt, ok: true})
		case missingCode(h.Error) == "": // a start that found no claude is the claude step's
			ev = append(ev, readinessEvidence{at: h.CheckedAt, text: h.Error})
		}
	}
	newestOf := func() *readinessEvidence {
		var n *readinessEvidence
		for i := range ev {
			if n == nil || ev[i].at.After(n.at) {
				n = &ev[i]
			}
		}
		return n
	}
	// The sign-in status: signed out is evidence like a turn that said so. Signed in to claude.ai only disproves
	// signed out: it is the model's evidence when nothing else is known, or when the newest says not logged in; an
	// expired token, a model the plan lacks or a rate limit that a turn met still stands.
	keyed := false
	if auth.known {
		switch {
		case auth.st.State == proto.ClaudeNotLoggedIn:
			ev = append(ev, readinessEvidence{at: auth.at, code: proto.TurnErrorNotLoggedIn})
		case auth.st.State == proto.ClaudeLoggedIn && auth.st.AuthMethod == "claude.ai":
			if n := newestOf(); n == nil || n.code == proto.TurnErrorNotLoggedIn ||
				n.code == "" && turnErrorCode(n.text) == proto.TurnErrorNotLoggedIn {
				ev = append(ev, readinessEvidence{at: auth.at, ok: true})
			}
		case auth.st.State == proto.ClaudeLoggedIn:
			keyed = true
		}
	}
	newest := newestOf()
	if newest != nil && newest.ok {
		return ReadinessStep{ID: StepModel, State: StepOK, Machine: d.ID, Launcher: launcher}, true
	}
	if newest != nil {
		code := newest.code
		if code == "" {
			code = turnErrorCode(newest.text)
		}
		if code != "" {
			action, alt := "connect_model", ""
			switch code {
			case proto.TurnErrorNotLoggedIn:
				alt = "login_claude"
			case proto.TurnErrorAuthFailed:
				action = "edit_provider"
			}
			st := s.readinessStep(lang, StepModel, StepProblem, code, d.Name, launcher, action)
			st.Machine = d.ID
			if alt != "" {
				st.AltActions = []ReadinessAction{{ID: alt}}
			}
			return st, false
		}
	}
	// Never verified, or it failed in a way that does not say what is wrong: unknown, not a problem.
	st := s.readinessStep(lang, StepModel, StepUnknown, codeModelUnverified, d.Name, launcher, "test_assistant")
	st.Machine = d.ID
	if keyed && newest == nil {
		st.Detail = i18n.T(lang, "assistant-readiness.model_key_unverified_detail", i18n.Args{"machine": d.Name})
	}
	return st, false
}

// readinessClaude is the claude step: the launcher is there (a runtime's or Codex's is skipped), Claude Code is
// installed (the node's self-check). The model having answered says it is there whatever the check found.
func (s *Service) readinessClaude(ctx context.Context, lang i18n.Lang, d Device, launcher string, rec readinessRecord,
	modelOK bool, wait time.Duration) ReadinessStep {
	problem := func(code, action string) ReadinessStep {
		st := s.readinessStep(lang, StepClaude, StepProblem, code, d.Name, launcher, action)
		st.Machine = d.ID
		if code == proto.CodeClaudeMissing {
			st.AltActions = []ReadinessAction{{ID: "connect_model"}}
		}
		return st
	}
	if st := rec.Start; st != nil && st.Machine == d.ID && st.Launcher == launcher && !modelOK {
		// What was looked at after the refusal says whether it still holds: the user may have installed Claude Code
		// or added the launcher since, and in setup mode no start comes to clear it.
		gone := false
		if st.Code == proto.CodeClaudeMissing {
			seen, done := s.claudeVersion(ctx, d.ID, wait, st.At)
			gone = done && seen.version != "" && !seen.at.Before(st.At)
		} else {
			_, found := s.readinessLauncher(ctx, d.ID, launcher)
			gone = found == launcherThere
		}
		if !gone {
			action := "connect_model"
			if st.Code == proto.CodeClaudeMissing {
				action = "install_claude"
			}
			return problem(st.Code, action)
		}
		s.clearStart(ctx, *st)
	}
	if h, ok := s.launcherUnusable(ctx, d.ID, launcher); ok && !modelOK && h.Kind != errKindNotInTerminal {
		switch missingCode(h.Error) { // the launcher's health record: its start or check found no such command
		case proto.CodeClaudeMissing:
			return problem(proto.CodeClaudeMissing, "install_claude")
		case proto.CodeLauncherMissing:
			return problem(proto.CodeLauncherMissing, "connect_model")
		}
	}
	plain := launcher == "" || launcher == "claude"
	if !plain {
		switch agent, found := s.readinessLauncher(ctx, d.ID, launcher); {
		case found == launcherGone && !modelOK:
			return problem(proto.CodeLauncherMissing, "connect_model")
		case found == launcherThere && agent != "" && agent != "claude":
			return ReadinessStep{ID: StepClaude, State: StepSkipped, Launcher: launcher} // Codex, a runtime: not Claude Code
		}
	}
	seen, done := s.claudeVersion(ctx, d.ID, wait, time.Time{})
	switch {
	case modelOK:
		return ReadinessStep{ID: StepClaude, State: StepOK, Version: seen.version}
	case !done:
		return ReadinessStep{ID: StepClaude, State: StepChecking}
	case seen.version != "":
		return ReadinessStep{ID: StepClaude, State: StepOK, Version: seen.version}
	case seen.known && plain:
		return problem(proto.CodeClaudeMissing, "install_claude")
	}
	// An old node with no self-check, or a launcher of its own that may find claude where the login shell does not.
	return ReadinessStep{ID: StepClaude, State: StepUnknown}
}

// readinessFirst is the first_message step: the main conversation answered once (a main conversation was replaced:
// it did long ago), or its one turn failed.
func (s *Service) readinessFirst(ctx context.Context, lang i18n.Lang, d Device, rec readinessRecord, v Settings,
	main proto.Session, hasMain bool) ReadinessStep {
	if rec.AnsweredAt != nil || len(v.Former) > 0 {
		return ReadinessStep{ID: StepFirstMessage, State: StepOK}
	}
	if hasMain && answeredWell(main) {
		now := time.Now().UTC()
		if err := s.readinessUpdate(ctx, func(r *readinessRecord) { r.AnsweredAt = &now }); err != nil {
			s.logf("assistant: readiness: %v", err)
		}
		return ReadinessStep{ID: StepFirstMessage, State: StepOK}
	}
	if hasMain && main.TurnErrorCode != "" {
		return s.readinessStep(lang, StepFirstMessage, StepProblem, main.TurnErrorCode, d.Name, "", "test_assistant")
	}
	return s.readinessStep(lang, StepFirstMessage, StepTodo, codeNeverTried, d.Name, "", "test_assistant")
}

// readinessStep is a step with its code's words in lang and its action.
func (s *Service) readinessStep(lang i18n.Lang, id, state, code, machine, launcher, action string) ReadinessStep {
	args := i18n.Args{"machine": machine, "launcher": launcher}
	if machine == "" {
		args["machine"] = i18n.T(lang, "assistant-readiness.this_computer")
	}
	st := ReadinessStep{ID: id, State: state, Code: code, Launcher: launcher,
		Title: i18n.T(lang, "assistant-readiness."+code+"_title", args), Detail: i18n.T(lang, "assistant-readiness."+code+"_detail", args)}
	if id == StepComputer || id == StepFirstMessage {
		st.Launcher = ""
	}
	if action != "" {
		st.Action = &ReadinessAction{ID: action}
	}
	return st
}

// readinessSuggestions are the first screen's example questions: while something is missing only the local ones
// (what Persona does, how to connect a model), which never reach the model.
func readinessSuggestions(lang i18n.Lang, mode string) []ReadinessSuggestion {
	t := func(key string) string { return i18n.T(lang, "assistant-readiness.suggest_"+key) }
	out := []ReadinessSuggestion{{ID: "what_can_you_do", Title: t("what_can_you_do"), Local: true}}
	if mode != ReadinessFull {
		return append(out, ReadinessSuggestion{ID: "how_to_connect_model", Title: t("how_to_connect_model"), Local: true})
	}
	return append(out,
		ReadinessSuggestion{ID: "largest_folders", Title: t("largest_folders"), Prompt: t("largest_folders_prompt")},
		ReadinessSuggestion{ID: "today", Title: t("today"), Prompt: t("today_prompt"), Needs: []string{"phone_calendar"}},
		ReadinessSuggestion{ID: "connect_mail", Title: t("connect_mail"), Prompt: t("connect_mail_prompt")})
}

// ---- what it asks ----

// readinessProbe runs the one-word check of launcher on machine (launchers.check, what "Test" runs), or answers
// the last one's result when it ran this launcher there within readinessProbeReuse; one at a time.
func (s *Service) readinessProbe(ctx context.Context, machine, launcher string) *proto.LauncherCheck {
	b := &s.ready
	for {
		b.mu.Lock()
		s.readinessLoadLocked(ctx)
		if p := b.rec.Probe; p != nil && p.Machine == machine && p.Launcher == launcher && time.Since(p.At) < readinessProbeReuse {
			c := p.Check
			b.mu.Unlock()
			return &c
		}
		wait := b.probing
		if wait == nil {
			b.probing = make(chan struct{})
			b.mu.Unlock()
			break
		}
		b.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return &proto.LauncherCheck{Error: ctx.Err().Error()}
		}
	}
	defer func() {
		b.mu.Lock()
		close(b.probing)
		b.probing = nil
		b.mu.Unlock()
	}()
	// The check goes on when the client stops waiting (its own time ran out, the page closed): its result is kept, and
	// a try within readinessProbeReuse gets it instead of starting another check that is cut off at the same point (#1892).
	ctx = context.WithoutCancel(ctx)
	cctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()
	var res proto.LauncherCheck
	if err := s.call(cctx, machine, assistantwire.DeviceLaunchersCheck, map[string]any{"name": launcher}, &res); err != nil {
		// Not the model's answer (the machine could not be asked): said, not kept.
		return &proto.LauncherCheck{Error: i18n.Localize(err, s.requestCtxLang(ctx))}
	}
	if err := s.readinessUpdate(ctx, func(r *readinessRecord) {
		r.Probe = &readinessProbe{Machine: machine, Launcher: launcher, At: time.Now().UTC(), Check: res}
	}); err != nil {
		s.logf("assistant: readiness: %v", err)
	}
	return &res
}

// What the machine's launcher list says of a launcher.
const (
	launcherUnknownHere = iota // the list could not be read
	launcherThere
	launcherGone
)

// readinessLauncher looks launcher up among what machine can start: the agent it starts ("" Claude Code; a runtime's
// assistant launcher counts as not Claude Code) and whether it is there.
func (s *Service) readinessLauncher(ctx context.Context, machine, launcher string) (string, int) {
	// A managed launcher runs Claude Code through personad launch, and so does a direct API provider speaking Messages
	// (its gateway, #2455); other direct API protocols cannot start Claude.
	if slices.ContainsFunc(s.storedLaunchers(ctx), func(l storedLauncher) bool {
		return l.Name == launcher && !(l.Kind == "api" && !messagesAPI(l.API))
	}) {
		return "", launcherThere
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	list, err := s.machineLaunchers(cctx, machine)
	if err != nil {
		return "", launcherUnknownHere
	}
	i := slices.IndexFunc(list, func(l proto.Launcher) bool { return l.Name == launcher })
	if i < 0 {
		return "", launcherGone
	}
	if list[i].Assistant {
		return "runtime", launcherThere
	}
	return list[i].Agent, launcherThere
}

// claudeVersion is machine's Claude Code as its node's self-check (node.doctor's claude-version) found it, cached
// readinessClaudeFresh when it read a version and readinessClaudeRetry when not (it may be installed any moment);
// a check made before since is not used. done false when the check did not finish within wait (it goes on, and is
// cached).
func (s *Service) claudeVersion(ctx context.Context, machine string, wait time.Duration, since time.Time) (claudeSeen, bool) {
	b := &s.ready
	b.mu.Lock()
	if c, ok := b.claude[machine]; ok && !c.at.Before(since) &&
		(c.version != "" && time.Since(c.at) < readinessClaudeFresh || c.version == "" && time.Since(c.at) < readinessClaudeRetry) {
		b.mu.Unlock()
		return c, true
	}
	ch := b.claudeIn[machine]
	if ch == nil {
		ch = make(chan struct{})
		if b.claudeIn == nil {
			b.claudeIn = map[string]chan struct{}{}
		}
		b.claudeIn[machine] = ch
		go s.askClaudeVersion(machine, ch)
	}
	b.mu.Unlock()
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
		return claudeSeen{}, false
	case <-ctx.Done():
		return claudeSeen{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.claude[machine], true
}

// askClaudeVersion asks machine's node for its self-check and keeps what it says of Claude Code. The path-access check
// is left out ("preflight": false): it is not needed here, takes up to about 20 s, and would read the user's folders
// on every ask (older nodes ignore the parameter and have no such check).
func (s *Service) askClaudeVersion(machine string, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var rep struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	seen := claudeSeen{at: time.Now()}
	if err := s.call(ctx, machine, deviceNodeDoctor, map[string]any{"preflight": false}, &rep); err != nil {
		s.logf("assistant: readiness: node.doctor on %s: %v", machine, err) // an old node has none: unknown
	} else {
		for _, c := range rep.Checks {
			if c.Name == "claude-version" {
				seen.known = true
				if m := doctorVersion.FindStringSubmatch(c.Detail); m != nil {
					seen.version = m[1]
				}
			}
		}
	}
	b := &s.ready
	b.mu.Lock()
	if b.claude == nil {
		b.claude = map[string]claudeSeen{}
	}
	b.claude[machine] = seen
	delete(b.claudeIn, machine)
	b.mu.Unlock()
	close(done)
}

// askAhead starts the node's two free answers the readiness waits for (Claude Code's version, the launcher's sign-in
// status) when they are not cached, so that they run at the same time.
func (s *Service) askAhead(machine, launcher string) {
	s.claudeVersion(context.Background(), machine, 0, time.Time{})
	s.claudeAuthStatus(context.Background(), machine, launcher, 0)
}

// claudeAuthStatus is launcher's sign-in status on machine (claude.auth_status), cached readinessAuthFresh, and
// readinessClaudeRetry when signed out or not answered (readinessAuthFresh for an old node). done false when the node did not answer within wait (the
// question goes on, and is cached).
func (s *Service) claudeAuthStatus(ctx context.Context, machine, launcher string, wait time.Duration) (claudeAuth, bool) {
	key := machine + "/" + launcher
	b := &s.ready
	b.mu.Lock()
	if a, ok := b.auth[key]; ok {
		fresh := readinessAuthFresh
		if !a.known && !a.old || a.st.State == proto.ClaudeNotLoggedIn ||
			a.st.State == proto.ClaudeAuthUnknown && (a.st.Reason == proto.ClaudeAuthTimeout || a.st.Reason == proto.ClaudeAuthUnreadable) {
			fresh = readinessClaudeRetry
		}
		if time.Since(a.at) < fresh {
			b.mu.Unlock()
			return a, true
		}
	}
	ch := b.authIn[key]
	if ch == nil {
		ch = make(chan struct{})
		if b.authIn == nil {
			b.authIn = map[string]chan struct{}{}
		}
		b.authIn[key] = ch
		go s.askClaudeAuth(machine, launcher, key, ch)
	}
	b.mu.Unlock()
	if wait <= 0 {
		return claudeAuth{}, false
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ch:
	case <-t.C:
		return claudeAuth{}, false
	case <-ctx.Done():
		return claudeAuth{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.auth[key], true
}

// askClaudeAuth asks machine's node whether Claude Code is signed in as launcher runs it, and keeps the answer.
func (s *Service) askClaudeAuth(machine, launcher, key string, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a := claudeAuth{at: time.Now()}
	if err := s.call(ctx, machine, assistantwire.DeviceClaudeAuthStatus, map[string]any{"name": launcher}, &a.st); err != nil {
		// An old node has none: unknown, and not asked (or logged) again every minute.
		a.old = strings.Contains(err.Error(), "unknown method")
		if !a.old {
			s.logf("assistant: readiness: claude.auth_status on %s: %v", machine, err)
		}
	} else {
		a.known = true
	}
	b := &s.ready
	b.mu.Lock()
	if b.auth == nil {
		b.auth = map[string]claudeAuth{}
	}
	b.auth[key] = a
	delete(b.authIn, key)
	b.mu.Unlock()
	close(done)
}

// deviceNodeDoctor is the node's self-check (node/doctor_rpc.go), read-only.
const deviceNodeDoctor = "node.doctor"

// doctorVersion is the version at the start of the claude-version check's detail ("2.1.308, supports --plugin-dir");
// none when it could not read one.
var doctorVersion = lazyre.New(`^\s*v?(\d+\.\d+\.\d+[0-9A-Za-z.+-]*)`)

// missingCode is the start refusal's code a launcher error reads as (Claude Code or the command missing), "" else.
func missingCode(text string) string {
	t := strings.ToLower(text)
	switch {
	case strings.Contains(t, "command not found: claude") || strings.Contains(t, "claude: command not found") ||
		strings.Contains(t, "claude not found"):
		return proto.CodeClaudeMissing
	case strings.Contains(t, "command not found"):
		return proto.CodeLauncherMissing
	}
	return ""
}
