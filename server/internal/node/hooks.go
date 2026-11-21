package node

import (
	"bufio"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/acceptloop"
	"github.com/nwverse/persona/server/internal/assistantwire"
	"github.com/nwverse/persona/server/internal/host"
	"github.com/nwverse/persona/server/internal/i18n"
	"github.com/nwverse/persona/server/internal/identity"
	"github.com/nwverse/persona/server/internal/nodeclient"
	"github.com/nwverse/persona/server/internal/osproc"
	"github.com/nwverse/persona/server/internal/prompts"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

func (n *Node) listenHookSocket(ctx context.Context) (net.Listener, error) {
	ln, err := n.listenOnce(ctx)
	if err != nil {
		return nil, err
	}
	// A node from an earlier hub connection may still be shutting down; if its cleanup (or anything else) removes
	// our socket file, hooks silently fall back to the terminal. Re-create it whenever it disappears, or when another
	// file took its place that nothing answers on (a node that started, listened and exited without unlinking: every
	// `personad launch` then failed with "connection refused" and the assistant's messages stayed undelivered).
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			if why := n.sockGone(); why != "" {
				n.logger.Printf("hook socket %s, re-creating", why)
				if _, err := n.listenOnce(ctx); err != nil {
					n.logger.Printf("hook socket: %v", err)
				}
			}
		}
	}()
	return ln, nil
}

// sockGone says why the node socket needs re-creating, "" when it does not: the file is gone, or it is another file
// than the one this node listens on and nothing answers there. A live one is left alone: a newer node owns it.
func (n *Node) sockGone() string {
	fi, err := os.Stat(nodeclient.SocketPath())
	if os.IsNotExist(err) {
		return "vanished"
	}
	if err != nil {
		return ""
	}
	n.sockMu.Lock()
	mine := n.sock != nil && os.SameFile(fi, n.sock)
	n.sockMu.Unlock()
	if mine {
		return ""
	}
	c, err := net.DialTimeout("unix", nodeclient.SocketPath(), time.Second)
	if err != nil {
		return "replaced by one nothing answers on"
	}
	c.Close()
	return ""
}

func (n *Node) listenOnce(ctx context.Context) (net.Listener, error) {
	path := nodeclient.SocketPath()
	_ = os.Remove(path)
	ln, err := osproc.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// Never unlink on close: by then the path may belong to a newer node.
	ln.(*osproc.UnixListener).SetUnlinkOnClose(false)
	_ = os.Chmod(path, 0o600)
	if _, err := ensureLaunchKey(nodeclient.LaunchKeyPath()); err != nil { // before any personad launch reads it
		n.logf("launch key: %v", err)
	}
	if fi, err := os.Stat(path); err == nil {
		n.sockMu.Lock()
		n.sock = fi
		n.sockMu.Unlock()
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	go acceptloop.Run(ctx, ln, n.logf, func(c net.Conn) { go n.serveSock(ctx, c) })
	return ln, nil
}

func (n *Node) serveSock(ctx context.Context, c net.Conn) {
	defer c.Close()
	if ctx.Err() != nil { // this node is going away (its listener closes): it answers nothing, not even a ping
		return
	}
	r := bufio.NewReader(c)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	if isHTTPRequestLine(line) { // Persona's Claude Code plugin (ccplugin.go)
		n.serveCCPluginHTTP(ctx, c, line, r)
		return
	}
	var req sockRequest
	if json.Unmarshal(line, &req) != nil {
		return
	}
	if req.Kind == "hook.display" {
		// A reply's lines as Claude shows them (livereply.go): Claude shows them only once the hook is done, so it is
		// answered before anything else is done with them.
		_, _ = c.Write([]byte(`{"ok":true}` + "\n"))
		n.displayHook(req.Input)
		return
	}
	sent := req // as it came: what the spool keeps of a hook event the connection could not send (hook_outbox.go)
	req.SessionID = n.resolveAssistantKey(req.SessionID)
	req.Pane = n.ownPane(req.Pane)
	// The caller going away (hook killed because the user answered in the terminal) cancels the request.
	ctx, cancel := context.WithCancel(withHookCall(ctx, sent))
	defer cancel()
	go func() {
		_, _ = r.ReadByte()
		cancel()
	}()

	switch req.Kind {
	case "hook", "ask", "notify": // after what standby took while the manager was away (spool_replay.go)
		if n.replayed != nil {
			// Not for ever: a report-only hook gives up after 3 s (nodeclient.ReportHook), and a replay that takes long
			// (a manager slow to answer) must not hold the others past what they wait for. Out of order beats lost.
			wait := replayGateMost
			if req.ReportOnly {
				wait = 2 * time.Second
			}
			late := time.After(wait)
			select {
			case <-n.replayed:
			case <-late:
			case <-ctx.Done():
				return
			}
			if req.Kind == "hook" {
				n.ahead.note(req) // before the replay is done: it leaves what this one moved on alone (spool_replay.go)
			}
		}
	}

	var resp any
	var waitID string
	var wait *pendingMCPWait
	switch req.Kind {
	case "hook":
		out, err := n.handleHook(ctx, req)
		if errors.Is(err, errNodeGone) {
			// This node is going away under a pending question: the hook asks the next one (runHook).
			resp = map[string]any{"retry": true}
		} else {
			resp = map[string]any{"output": out}
		}
	case "hook.decided": // another PermissionRequest hook answered a call (hookdecided.go)
		var in hookInput
		if json.Unmarshal(req.Input, &in) == nil && in.HookEventName == "PermissionRequest" && validID(in.SessionID) {
			n.decided.decide(in, time.Now())
		}
		resp = map[string]any{"ok": true}
	case "claude-plugin.status": // which sessions' Persona plugin polls this node (ccplugin.go): installers, doctor
		resp = n.ccPluginStatusNow()
	case "ping": // personad node doctor, persona-link node switch: a round trip that changes nothing
		resp = map[string]any{"ok": true, "program": n.Program, "version": n.Version}
	case "update.inventory":
		var target nodeclient.UpdateInventoryTarget
		if err := json.Unmarshal(req.Input, &target); err != nil {
			resp = map[string]any{"error": "invalid explicit update inventory target"}
		} else if inventory, err := n.updateInventory(ctx, target); err != nil {
			resp = map[string]any{"error": err.Error()}
		} else {
			resp = inventory
		}
	case "preflight": // personad node doctor's path-access check, read with this node's permissions (preflight.go)
		resp = n.preflightIn(ctx, i18n.Resolve(req.Locale).Or(i18n.Default))
	case "ask": // personad ask (scriptcli.go)
		resp = n.handleAsk(ctx, req)
	case "notify": // personad notify (scriptcli.go)
		resp = n.handleNotify(ctx, req)
	case assistantwire.MethodDecisionRaise, assistantwire.MethodDecisionGet, assistantwire.MethodDecisionWithdraw:
		resp = n.handleDecision(ctx, req) // personad decision: non-blocking, never spooled
	case "launcher.env": // personad launch <name>
		resp = n.launcherEnvFor(req)
	case "launcher.pi": // pi's Persona extension: the providers pi can use here (pi_providers.go)
		resp = n.piProvidersFor(ctx, req)
	case "secret.env": // personad launch --ticket (secretenv.go)
		resp = n.secretEnvFor(req)
	case "mcp.tools":
		var res map[string]any
		// The assistant's only when the node says so too (clicall.go mcpSession).
		session, assistant := n.mcpSession(c, req)
		if session == "" && !assistant {
			session = n.paneCaller(req.Pane) // an agent's: the hub tells a task leader's tools from any other's by it
		}
		// A guest's (its MCP server says so, or this node knows it is one): the hub gives it only its own tools.
		guest := req.Guest || n.guestOf(session) != nil
		params := mcpToolsParams(req.Input, session, assistant, guest)
		// A supervision round's process asks for its narrower set (assistant_round.go); a manager that does not know
		// it gives the whole list.
		if req.ToolSet != "" {
			params["tool_set"] = req.ToolSet
		}
		if err := n.peer.Call(ctx, "mcp.tools", params, &res); err != nil {
			resp = map[string]any{"tools": []proto.Tool{}, "error": err.Error()}
		} else {
			if res == nil {
				res = map[string]any{}
			}
			res["session"] = session
			resp = res
		}
	case "cli.call": // personad tg: the node, not the caller, says whether it is the assistant (clicall.go)
		resp = n.cliCall(ctx, c, req)
	case "mcp.call":
		var res proto.ToolResult
		session, assistant := n.mcpSession(c, req)
		sid := n.mcpCaller(req.Name, session, req.Pane, assistant)
		if req.Name == toolMailSaveAttachment { // writes on this machine (mailattachment.go)
			resp = n.saveMailAttachment(ctx, req.Args, sid, assistant, req.Guest || n.guestOf(sid) != nil)
			break
		}
		params := map[string]any{"name": req.Name, "args": req.Args, "session_id": sid,
			"assistant": assistant, "guest": req.Guest || n.guestOf(sid) != nil}
		n.putSourcePermission(c, params, sid, req.Name, req.Args)
		if origin := n.assistantOrigin(sid); origin != "" {
			params["origin"] = origin
		}
		if req.WaitUpdates {
			waitID, wait = n.beginMCPWait(c)
			if wait != nil {
				params["wait_updates"], params["call_id"] = true, waitID
			}
		}
		if err := n.peer.Call(ctx, "mcp.call", params, &res); err != nil {
			res = proto.TextResult("Persona hub error: "+err.Error(), true)
		}
		resp = res
	default:
		resp = map[string]any{"error": "unknown kind"}
	}
	if wait != nil {
		n.finishMCPWait(waitID, wait, resp)
		return
	}
	b, _ := json.Marshal(resp)
	_, _ = c.Write(append(b, '\n'))
}

type hookInput struct {
	HookEventName        string          `json:"hook_event_name"`
	Source               string          `json:"source"`
	SessionID            string          `json:"session_id"`
	TranscriptPath       string          `json:"transcript_path"`
	Cwd                  string          `json:"cwd"`
	PermissionMode       string          `json:"permission_mode"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	ToolUseID            string          `json:"tool_use_id"`
	Message              string          `json:"message"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	StopHookActive       bool            `json:"stop_hook_active"`
	Prompt               string          `json:"prompt"`
	PromptID             string          `json:"prompt_id"` // the turn's user line (its promptId in the transcript)
	AgentID              string          `json:"agent_id"`
	AgentType            string          `json:"agent_type"`
	MCPServerName        string          `json:"mcp_server_name"`
	MCPConfigSource      string          `json:"mcp_config_source"`
	MCPServer            *struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	} `json:"mcp_server"`
	Mode            string          `json:"mode"`
	URL             string          `json:"url"`
	ElicitationID   string          `json:"elicitation_id"`
	RequestedSchema json.RawMessage `json:"requested_schema"`
}

// errNodeGone: the hub connection ended while a hook waited (the node is going away).
var errNodeGone = errors.New("node going away")

// interactionID is the same for every attempt of one hook call, so a hook that reconnects after a restart finds
// its interaction again: the tool call it is about, else the hook process's own id.
func interactionID(in hookInput, attempt string) string {
	key := in.SessionID + "\x00" + in.HookEventName + "\x00" + in.ToolName + "\x00"
	switch {
	case in.HookEventName == "Elicitation":
		// One tool can ask multiple questions; only this hook attempt identifies a form request.
		if attempt == "" {
			return "ix_" + newUUID()[:12]
		}
		key += in.MCPServerName + "\x00" + in.ElicitationID + "\x00attempt:" + attempt
	case in.ToolUseID != "":
		key += in.ToolUseID
	case attempt != "":
		key += "attempt:" + attempt
	default: // an older hook that does not reconnect
		return "ix_" + newUUID()[:12]
	}
	sum := sha256.Sum256([]byte(key))
	return "ix_" + hex.EncodeToString(sum[:])[:16]
}

// questionInteraction reads the AskUserQuestion call shared by PreToolUse and PermissionRequest.
func questionInteraction(in hookInput, ix *proto.Interaction) bool {
	var q struct {
		Questions []struct {
			Question    string                 `json:"question"`
			Header      string                 `json:"header"`
			MultiSelect bool                   `json:"multiSelect"`
			Options     []proto.QuestionOption `json:"options"`
		} `json:"questions"`
	}
	if json.Unmarshal(in.ToolInput, &q) != nil || len(q.Questions) == 0 {
		return false
	}
	ix.Kind, ix.CanCancel = proto.IxQuestion, true
	for _, qq := range q.Questions {
		ix.Questions = append(ix.Questions, proto.Question{Question: qq.Question, Header: qq.Header, MultiSelect: qq.MultiSelect, Options: qq.Options})
	}
	return true
}

// questionInputWithoutAnswers matches PostToolUse's answered input to PermissionRequest's original input.
func questionInputWithoutAnswers(raw json.RawMessage) json.RawMessage {
	var input map[string]any
	if json.Unmarshal(raw, &input) != nil {
		return raw
	}
	delete(input, "answers")
	normalized, err := json.Marshal(input)
	if err != nil {
		return raw
	}
	return normalized
}

// handleHook returns the JSON Claude Code should read from the hook's stdout, or nil to let the TUI decide;
// errNodeGone when this node lost the hub before the phone answered.
func (n *Node) handleHook(ctx context.Context, req sockRequest) (any, error) {
	var in hookInput
	if json.Unmarshal(req.Input, &in) != nil {
		return nil, nil
	}
	if hookCallOf(ctx) == nil { // not from node.sock (serveSock): as it is
		ctx = withHookCall(ctx, req)
	}
	defer n.hookFlights.enter(in.SessionID)() // its process waits on this call: not idle (model_restore.go)
	if n.sessionBlocked(in.SessionID, in.Cwd) {
		return nil, nil // a blocked Agent folder (agentdirs.go): nothing reported, no card; Claude Code asks itself
	}
	if req.ReportOnly && !n.takesReport(in) {
		return nil, nil // before the counters: a session the plugin reports for would count each prompt twice
	}
	n.ultracodeHook(req, in)
	if repair := n.assistantViewStop(ctx, req, in); repair != nil {
		return repair, nil
	}
	if hold := n.closeoutStop(ctx, req, in); hold != nil {
		return hold, nil
	}
	stopSig := n.recordHookTurn(req, in, time.Now())
	if !req.ReportOnly && !req.Resume {
		n.liveTurnHook(in) // a prompt or a Stop ends the live text of the turn before (livereply.go)
	}
	if n.peer == nil {
		return nil, nil
	}
	if req.ReportOnly {
		n.rememberTranscript(in)
		n.notifyHookEvent(ctx, req, in, stopSig)
		return nil, nil
	}
	// An Orca pane is bound only on a machine that enables Orca: elsewhere nothing could drive it, and the hook
	// reported no pane for it before Orca was a host (a herdr machine behaves as it did).
	if paneKindOK(n, req.Pane) {
		n.bindPane(req.Pane, in.SessionID)
		if in.HookEventName == "SessionStart" {
			n.noteSessionStart(req.Pane, in.SessionID) // rule 2 of a session's owner (ownership.go)
		}
	}
	if in.HookEventName == "SessionStart" {
		n.resumeOnSessionStart(req.Pane, in.SessionID) // herdr's restore follows it to the new session (herdrresume.go)
	}
	n.rememberTranscript(in)
	if !req.Resume { // the event was reported on the first attempt
		n.notifyHookEvent(ctx, req, in, stopSig)
	}
	// Every Bash call of every agent waits on this hook. Only the assistant's go to larkGate, and another agent's
	// needs only the cua-driver check below: one that does not name cua-driver goes on here, before the card's
	// title is looked up (a re-read of the transcript's tail) and its claude is watched (#2474).
	if in.HookEventName == "PreToolUse" && in.ToolName == "Bash" && !bashNamesCua(in.ToolInput) && !n.isAssistantSession(in.SessionID) {
		return nil, nil
	}

	ix := proto.Interaction{
		ID: interactionID(in, req.Attempt), SessionID: in.SessionID, Cwd: in.Cwd, CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(nodeclient.HookTimeout).UTC(), ToolName: in.ToolName, ToolUseID: in.ToolUseID, ToolInput: in.ToolInput,
		Resumable: req.Attempt != "",
		AgentID:   in.AgentID, AgentType: in.AgentType, MCPServerName: in.MCPServerName, MCPConfigSource: in.MCPConfigSource,
	}
	if in.MCPServer != nil {
		ix.MCPServerName, ix.MCPConfigSource = in.MCPServer.Name, in.MCPServer.Source
	}
	// A card of a headless conversation is placed in its directory, where it started, not where the model's last
	// `cd` left the hook: a health conversation's card in health/reports or /tmp is still the health space's (#2368).
	n.mu.Lock()
	proc := n.procs[in.SessionID]
	n.mu.Unlock()
	if proc != nil {
		ix.Cwd = identity.CardCwd(proc.cwd, in.Cwd)
	}
	if p := n.transcriptPath(in.SessionID); p != "" {
		if st, err := os.Stat(p); err == nil {
			ix.SessionTitle = n.meta(p, st).title
		}
	}
	if ix.SessionTitle == "" {
		ix.SessionTitle = filepath.Base(ix.Cwd)
	}
	if asksPhone(in) {
		var stopWatch func()
		ctx, stopWatch = n.watchAgent(ctx, req)
		defer stopWatch()
	}

	switch {
	case in.HookEventName == "SessionStart":
		// Reported and bound above; what is left is the closeout's form as its starting context (closeout_start.go).
		if brief := n.closeoutStart(ctx, req, in); brief != nil {
			return brief, nil
		}
		return nil, nil

	case in.HookEventName == "Elicitation":
		if !elicitationInteraction(in, &ix) {
			return nil, nil
		}
		res, ok, err := n.ask(ctx, ix)
		if err != nil || !ok {
			return nil, err
		}
		return elicitationOutput(ix, res), nil

	case in.HookEventName == "PreToolUse" && cuaToolOf(in.ToolName) != "":
		// An Agent's own Cua Driver: never System Settings or a permission prompt (cua_gate.go, #4349).
		if why := cuaVerdict(ctx, cuaToolOf(in.ToolName), in.ToolInput); why != "" {
			n.logf("refused %s for %s: protected app, or the screen not checked", in.ToolName, in.SessionID)
			return preToolDecision("deny", why), nil
		}
		return nil, nil

	case in.HookEventName == "PreToolUse" && in.ToolName == "Bash":
		var c struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(in.ToolInput, &c) == nil {
			if why := cuaBashVerdict(ctx, c.Command); why != "" { // its cua-driver calls, the same way (cua_gate.go)
				n.logf("refused a cua-driver call in Bash for %s: protected app, or not checked", in.SessionID)
				return preToolDecision("deny", why), nil
			}
		}
		if !n.isAssistantSession(in.SessionID) {
			return nil, nil
		}
		return n.larkGateWithEdit(ctx, in, ix)

	case in.HookEventName == "PostToolUse" && in.ToolName == "AskUserQuestion":
		// A terminal answer includes answers; PermissionRequest's original input did not. Complete only the same
		// tool_use_id, whether its waiter is already present or its completion arrives first (#2382).
		in.ToolInput = questionInputWithoutAnswers(in.ToolInput)
		n.questions.complete(in, time.Now())
		return nil, nil

	case in.HookEventName == "PreToolUse" && in.ToolName == "AskUserQuestion":
		if !questionInteraction(in, &ix) {
			return nil, nil
		}
		// Answers are keyed by question text: refuse alike-worded questions instead of silently dropping one answer.
		if proto.HasDuplicateQuestionText(ix.Questions) {
			return preToolDecision("deny", duplicateQuestionReason), nil
		}
		// Let Claude draw its own dialog. PermissionRequest below carries the phone card and its answer (#2382).
		n.questions.before(in, time.Now())
		return nil, nil

	case in.HookEventName == "PermissionRequest" && in.ToolName == "ExitPlanMode":
		var p struct {
			Plan string `json:"plan"`
		}
		_ = json.Unmarshal(in.ToolInput, &p)
		ix.Kind, ix.Plan, ix.PlanThen = proto.IxPlan, p.Plan, n.planThen(in.SessionID)
		// The terminal's own dialog may decide the plan while the card still waits (the card is not always
		// withdrawn then): the footer is watched from now, until the card is answered.
		decided := &terminalDecided{}
		askCtx, endAsk := context.WithCancel(context.WithValue(ctx, terminalDecidedKey{}, decided))
		defer endAsk()
		stopWatch := n.planAfterTerminal(in.SessionID, func() {
			decided.Store(true) // the card is withdrawn as decided on the computer, not as a hook that went away
			endAsk()
		})
		res, ok, err := n.ask(askCtx, ix)
		if err != nil || !ok || res.Approve == nil {
			return nil, err // the watch goes on: the terminal decides, and with it the mode that follows
		}
		stopWatch()
		if !*res.Approve {
			msg := "用户在手机上打回了这个计划。"
			if strings.TrimSpace(res.Feedback) != "" {
				msg += "修改意见：" + res.Feedback
			}
			return permissionDecision(map[string]any{"behavior": "deny", "message": msg}), nil
		}
		mode := cmp.Or(proto.PlanAfterMode(ix.PlanThen), res.Mode, proto.ModeBypass)
		if n.peer != nil && res.ModeActor != nil {
			_ = n.peer.Notify(proto.EventModeSet, proto.ModeSetEvent{SessionID: in.SessionID, From: in.PermissionMode, To: mode, Actor: res.ModeActor})
		}
		var input map[string]any
		_ = json.Unmarshal(in.ToolInput, &input)
		// A bare allow does not close the plan dialog; it needs updatedInput plus a setMode update (2.1.310).
		return permissionDecision(map[string]any{
			"behavior": "allow", "updatedInput": input,
			"updatedPermissions": []any{map[string]any{"type": "setMode", "mode": mode, "destination": "session"}},
		}), nil

	case in.HookEventName == "PermissionRequest" && in.ToolName == "AskUserQuestion":
		if !questionInteraction(in, &ix) {
			return nil, nil
		}
		if proto.HasDuplicateQuestionText(ix.Questions) {
			return permissionDecision(map[string]any{"behavior": "deny", "message": duplicateQuestionReason}), nil
		}
		if req.Resume && n.restoreQuestionFromTranscript(in, req.Attempt, time.Now()) {
			// Several identical calls fit no exact PostToolUse. Hand this attempt back to the terminal and withdraw
			// its restored card instead of waiting for a completion that may never come (#2382).
			_ = n.peer.Notify("interaction.cancel", map[string]any{"id": ix.ID})
			return nil, nil
		}
		ctx, otherAnswered, stopOther := n.decided.watch(ctx, permissionKey(in))
		defer stopOther()
		ctx, questionAnswered, stopQuestion, toolUseID := n.questions.watch(ctx, in, req.Attempt, req.Resume)
		defer stopQuestion()
		answered := func() bool { return otherAnswered() || questionAnswered() }
		ix.ToolUseID = toolUseID
		if n.decided.grace(ctx, answered) {
			if questionAnswered() {
				// No new ask means askOnce cannot withdraw the resumable card retained by the hub (#2382).
				_ = n.peer.Notify("interaction.cancel", map[string]any{"id": ix.ID})
			}
			return nil, nil
		}
		res, ok, err := n.ask(ctx, ix)
		if answered() {
			return nil, nil
		}
		if err == nil && ok && res.Cancel {
			return permissionDecision(map[string]any{"behavior": "deny", "message": prompts.RenderQuestionCancelled()}), nil
		}
		if err != nil || !ok || len(res.Answers) == 0 {
			return nil, err
		}
		var input map[string]any
		_ = json.Unmarshal(in.ToolInput, &input)
		input["answers"] = res.Answers
		return permissionDecision(map[string]any{"behavior": "allow", "updatedInput": input}), nil

	case in.HookEventName == "PermissionRequest":
		ix.Kind, ix.Summary = proto.IxPermission, toolSummary(in.ToolName, in.ToolInput)
		// Assistant grants live at the hub so revocation applies to the next request. File grants cover
		// only the file's directory; terminal Agents retain Claude's own whole-tool session rules.
		assistant := n.isAssistantSession(in.SessionID)
		rule := map[string]any{"toolName": in.ToolName}
		if assistant {
			alwaysKey := "tool:" + in.ToolName
			if scope := fileRuleScope(in.ToolName, in.ToolInput); scope != "" {
				// Claude Code matches path rules on Read and Edit only; Edit(path) covers every file-editing tool,
				// a Write(path) rule is never matched.
				tool := "Edit"
				if in.ToolName == "Read" {
					tool = "Read"
				}
				alwaysKey = "tool:" + tool + "(" + scope + ")"
			}
			// Keep the existing keys compatible with grants made by earlier nodes.
			ix.AlwaysKeys = []string{alwaysKey}
		}
		// Another hook may answer it first (hookdecided.go): then nothing is shown, or what was is withdrawn.
		ctx, answered, stop := n.decided.watch(ctx, permissionKey(in))
		defer stop()
		if n.decided.grace(ctx, answered) {
			return nil, nil
		}
		res, ok, err := n.ask(ctx, ix)
		if answered() {
			return nil, nil
		}
		if err != nil || !ok || res.Allow == nil {
			return nil, err
		}
		if !*res.Allow {
			msg := res.Message
			if msg == "" {
				msg = "用户在手机上拒绝了这次调用。"
			}
			return permissionDecision(map[string]any{"behavior": "deny", "message": msg}), nil
		}
		var input map[string]any
		_ = json.Unmarshal(in.ToolInput, &input)
		d := map[string]any{"behavior": "allow", "updatedInput": input}
		if res.Always && !assistant {
			d["updatedPermissions"] = []any{map[string]any{
				"type": "addRules", "rules": []any{rule},
				"behavior": "allow", "destination": "session",
			}}
		}
		if !assistant { // Claude may not take it: its safety checks keep their dialog (allowwatch.go, #3752)
			n.watchAllow(in.SessionID, n.allowPane(req, in.SessionID))
		}
		return permissionDecision(d), nil
	}
	return nil, nil
}

// recordHookTurn feeds the session's turn counters (hookturn.go) from a hook event and returns the signature of the
// reply a Stop ended the turn with. A subagent's events and a reconnecting hook's repeat do not count: they are not
// the main agent's turns.
func (n *Node) recordHookTurn(req sockRequest, in hookInput, now time.Time) (stopSig string) {
	if req.Resume || in.AgentID != "" || !validID(in.SessionID) {
		return ""
	}
	n.hooks.agent(in.SessionID, req.AgentPID, req.AgentStart, now) // before counting: a new process starts from zero
	switch in.HookEventName {
	case "UserPromptSubmit":
		n.hooks.prompt(in.SessionID, now)
	case "Stop":
		n.hooks.stop(in.SessionID, now)
		return stopSignature(in.LastAssistantMessage)
	}
	return ""
}

// rememberTranscript keeps the transcript path a hook names for its session.
func (n *Node) rememberTranscript(in hookInput) {
	if in.TranscriptPath == "" || !validID(in.SessionID) {
		return
	}
	n.setPath(in.SessionID, in.TranscriptPath)
}

// notifyHookEvent tells the hub what a hook saw. A report-only request marks an event from a global hook
// (globalhooks.go), so that the hub does not take the session for one with Persona's hooks, which would hold its screen
// cards back for a hook card that never comes (hub/machine.go, hasHooks). stopSig lets the hub tell a second completion
// of the same reply from a new one.
//
// A resident node's connection may be waiting for the manager to resume it: its events go through the outbox
// (hook_outbox.go), in order, and the hook waits for its own at most hookEventWait, not at all while the transport is
// lost. A card it asks for afterwards waits for it all the same (afterHookEvent).
func (n *Node) notifyHookEvent(ctx context.Context, req sockRequest, in hookInput, stopSig string) {
	params := hookEventParams(in, stopSig, req.ReportOnly)
	if n.resumeWindow <= 0 { // its connection ends with its transport: written here, as it always was
		_ = n.peer.Notify("hook.event", params)
		return
	}
	hc := hookCallOf(ctx) // handleHook made one if serveSock did not
	if hc == nil {
		hc = &hookCall{sent: req}
	}
	e := &hookOut{req: hc.sent, in: in, at: time.Now(), params: params, sent: make(chan struct{})}
	hc.event.Store(e)
	n.hookOut.add(n, e)
	if n.peer.Lost() {
		return // written once the manager resumes the connection, or kept for the next one
	}
	select {
	case <-e.sent:
	case <-time.After(hookEventWait):
	}
}

// paneKindOK: a hook's pane is one this machine's hosts can drive (an Orca or Tern pane only where the set enables
// that host: both name the pane in every terminal of theirs, driven or not).
func paneKindOK(n *Node, pane string) bool {
	switch host.ParsePane(pane).Kind {
	case host.Orca:
		return n.hostEnabled(proto.EngineOrca)
	case host.Tern:
		return n.hostEnabled(proto.EngineTern)
	case host.Rex:
		return n.hostEnabled(proto.EngineRex)
	}
	return true
}

// hookEventParams is hook.event's params for what a hook saw.
func hookEventParams(in hookInput, stopSig string, reportOnly bool) map[string]any {
	p := map[string]any{
		"session_id": in.SessionID, "event": in.HookEventName, "tool_name": in.ToolName,
		"message": in.Message, "last_assistant_message": snippet(in.LastAssistantMessage), "cwd": in.Cwd,
		"stop_sig": stopSig,
	}
	if reportOnly {
		p["report_only"] = true
	}
	if in.HookEventName == "UserPromptSubmit" {
		p["prompt_source"] = promptSource(in.Prompt) // never the prompt itself: the apps see the event too
	}
	return p
}

// promptSource says who a UserPromptSubmit's prompt is from, for the manager (a meeting's batch is not the user's own
// message: docs/assistant/meeting-minutes.md "Feeding the attending session"): the kind of message source it carries
// (meeting, conversation, task…), "notification" for a background task reporting back, else "user".
func promptSource(prompt string) string {
	if src := prompts.ItemSource(proto.Item{Role: "user", Kind: proto.KindText, Text: prompt}); src != nil && src.Kind != "" {
		return src.Kind
	}
	if strings.Contains(prompt, "<task-notification>") {
		return "notification"
	}
	return "user"
}

// reportedEvent: what a global report-only hook (globalhooks.go) may tell the hub; the tools are the cards.
func reportedEvent(in hookInput) bool {
	switch in.HookEventName {
	case "SessionStart", "UserPromptSubmit", "Stop", "Notification":
		return true
	case "PreToolUse", "PostToolUse":
		return in.ToolName == "AskUserQuestion" || in.ToolName == "ExitPlanMode"
	}
	return false
}

// takesReport says whether an event from a global report-only hook is for the hub and the counters. A session Persona
// started or the assistant reports through its own hooks (hooksplugin.go), so it is not taken twice. A report-only
// event never binds a pane (a claude that a tmux server started inherits the pane's variable without being in it, and
// would take the pane from the claude that is; the node finds the session of a pane by itself, panes_bind.go).
func (n *Node) takesReport(in hookInput) bool {
	if !validID(in.SessionID) || !reportedEvent(in) {
		return false
	}
	_, ours := n.state.get(in.SessionID)
	return !ours && !n.isAssistantSession(in.SessionID)
}

func permissionDecision(d map[string]any) any {
	return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PermissionRequest", "decision": d}}
}

// duplicateQuestionReason goes back to the model when two questions share one text, telling it how to ask again.
const duplicateQuestionReason = "这次提问里有两道题的题干完全一样，我没法把两道题的答案分别交给你。" +
	"请把每道题改成不同的问法，或者合并成一道多选题，然后重新调用 AskUserQuestion。"

// agentWatch is how often a hook waiting on the phone looks whether the claude it runs under still lives.
var agentWatch = 2 * time.Second

// agentGone says the claude seen with that start is surely gone (procGone); a variable so tests can say who lives.
var agentGone = procGone

// asksPhone: the hook events that may wait on the phone (the cases of handleHook that call ask).
// bashNamesCua is whether a Bash call's input may hold a cua-driver call for cuaBashVerdict to check: its command
// names cua-driver (cuaCalls finds none otherwise). An input not read as a command is not checked there either.
func bashNamesCua(input json.RawMessage) bool {
	var c struct {
		Command string `json:"command"`
	}
	return json.Unmarshal(input, &c) == nil && strings.Contains(c.Command, "cua-driver")
}

func asksPhone(in hookInput) bool {
	switch in.HookEventName {
	case "Elicitation":
		return true
	case "PermissionRequest":
		return true
	case "PreToolUse":
		return in.ToolName == "Bash"
	}
	return false
}

// watchAgent ends ctx once the claude the hook runs under is gone (killed, crashed, out of memory). Claude Code starts
// its hooks in their own process group on Linux and macOS, so a claude that dies without cleaning up leaves its hook
// waiting, and the card on the phone, until nodeclient.HookTimeout (#4344). Ending ctx withdraws the card the way the hook going
// away does (ask sends interaction.cancel). Gone takes two looks in a row, so one odd read does not withdraw the card
// of a claude still waiting. Not watched: a hook whose claude was not found (AgentPID 0), or one already gone when it
// asks (a pid from another pid namespace reads as gone). stop also waits for the watcher.
func (n *Node) watchAgent(ctx context.Context, req sockRequest) (_ context.Context, stop func()) {
	if req.AgentPID <= 0 || agentGone(req.AgentPID, req.AgentStart) {
		return ctx, func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		t := time.NewTicker(agentWatch)
		defer t.Stop()
		misses := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			if !agentGone(req.AgentPID, req.AgentStart) {
				misses = 0
				continue
			}
			if misses++; misses >= 2 {
				cancel()
				return
			}
		}
	}()
	return ctx, func() { cancel(); <-exited }
}

// ask forwards an interaction to the hub and blocks until the phone answers, it expires, or the hook is cancelled.
// It fails with errNodeGone when the hub connection ends first: a resumable hook then asks the next node.
func (n *Node) ask(ctx context.Context, ix proto.Interaction) (proto.Resolution, bool, error) {
	afterHookEvent(ctx) // a hook's card goes after its event
	if c, ok := ctx.Value(codexApprovalOwner{}).(*codexConn); ok {
		return c.askResumable(ctx, ix)
	}
	return n.askOnce(ctx, ix)
}

func (n *Node) askOnce(ctx context.Context, ix proto.Interaction) (proto.Resolution, bool, error) {
	if n.peer == nil { // no hub: nobody to ask
		return proto.Resolution{}, false, nil
	}
	ctx, cancel := context.WithDeadline(ctx, ix.ExpiresAt)
	defer cancel()
	var res proto.Resolution
	err := n.peer.Call(ctx, "interaction.create", map[string]any{"interaction": ix}, &res)
	if err != nil {
		select {
		case <-n.peer.Done():
			if ix.Resumable {
				return res, false, errNodeGone
			}
			return res, false, nil
		default:
		}
		if errors.Is(err, rpc.ErrClosed) {
			return res, false, errNodeGone
		}
		if d, _ := ctx.Value(terminalDecidedKey{}).(*terminalDecided); d != nil && d.Load() {
			_ = n.peer.Notify("interaction.dismiss", map[string]any{"id": ix.ID}) // answered in the terminal (planthen.go)
			return res, false, nil
		}
		_ = n.peer.Notify("interaction.cancel", map[string]any{"id": ix.ID})
		return res, false, nil
	}
	if res.Cancel && (ix.Kind != proto.IxQuestion || !ix.CanCancel || !res.IsQuestionCancel()) {
		return proto.Resolution{}, false, fmt.Errorf("invalid question cancellation")
	}
	if res.Dismiss {
		return res, false, nil
	}
	if ix.EditableText != "" && res.EditedText != "" {
		n.editedAnswers.Store(ix.ID, res.EditedText)
	}
	return res, true, nil
}

// fileRuleScope is the permission rule content for a file tool's call: its file's directory and below
// ("//abs/dir/**"), or "" for another tool or a path that is not absolute.
func fileRuleScope(name string, input json.RawMessage) string {
	switch name {
	case "Read", "Edit", "Write", "MultiEdit", "NotebookEdit":
	default:
		return ""
	}
	var a struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	_ = json.Unmarshal(input, &a)
	p := a.FilePath
	if p == "" {
		p = a.NotebookPath
	}
	if p == "" || !filepath.IsAbs(p) {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(p))
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir // a Windows drive path
	}
	return "/" + strings.TrimSuffix(dir, "/") + "/**"
}

func toolSummary(name string, input json.RawMessage) string {
	var m map[string]any
	_ = json.Unmarshal(input, &m)
	pick := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := m[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	switch name {
	case "Bash":
		return pick("command")
	case "Edit", "Write", "Read", "MultiEdit", "NotebookEdit":
		return pick("file_path", "notebook_path")
	case "WebFetch":
		return pick("url")
	case "WebSearch":
		return pick("query")
	}
	if s := pick("description", "command", "path", "url", "query", "prompt"); s != "" {
		return s
	}
	// The summary is a push's body, shown on the lock screen: never an argument named like a credential (#4734).
	if s := proto.ArgGist(input); s != "" {
		return snippet(s)
	}
	if s := proto.RedactArgs(input); s != "" && s != "{}" && s != "null" {
		return snippet(s)
	}
	return name
}

// callerTools are the agents' own tools that act for the agent calling them: the manager's
// (assistantwire.AgentToolNames: reporting to its supervisor; the decisions, thread, repositories and links of the
// todo it works on), handing a subtask to another computer and reading it back (assistant/dispatch.go), and a task
// leader's task_plan and task_review (assistant/task_leader.go) and task_spawn. The manager's come from the list it
// reads itself, so a tool it adds there gets its caller here too (#9089: the repository tools were added to one list
// only, and every agent's call of them reached the hub with no session).
var callerTools = slices.Concat(assistantwire.AgentToolNames, []string{"dispatch_start", "dispatch_read", "task_plan", "task_review", "task_spawn"})

// mcpCaller is the session an MCP call of tool name goes to the hub with: the one it names, else, for an agent's
// tools that need to know whose call it is (callerTools), the session its pane runs. The rest go on as before, with
// none (a connector's card is not hung on the agent).
func (n *Node) mcpCaller(name, session, pane string, assistant bool) string {
	if session == "" && !assistant && slices.Contains(callerTools, name) {
		return n.paneCaller(pane)
	}
	return session
}

// paneCaller is the agent session running in pane (a hook's PaneRef): the one the host shows there now, else the one
// its hooks bound it to (an agent the host does not recognise); "" when none is known.
func (n *Node) paneCaller(pane string) string {
	if pane == "" {
		return ""
	}
	if id := n.sessionOfPane(pane); id != "" {
		return id
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.panes[pane]
}
