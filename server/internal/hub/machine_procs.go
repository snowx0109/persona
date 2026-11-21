package hub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// Listening port inspection and confirmation-gated process management (#12352).
// All routes require a full authenticated user device (Web/App); extension tokens,
// answer-only devices, and link tokens are refused before the node hears of the request.
// No default MCP tools are exposed for these routes.

func (h *Hub) machineProcRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/v1/machines/{mid}/ports", h.withMachine(h.handleMachinePorts))
	api.HandleFunc("GET /api/v1/machines/{mid}/processes", h.withMachine(h.handleMachineProcesses))
	api.HandleFunc("POST /api/v1/machines/{mid}/processes/{pid}/kill", h.withMachine(h.handleMachineProcessKill))
}

func (h *Hub) handleMachinePorts(w http.ResponseWriter, r *http.Request, m *Machine) {
	if principalOf(r.Context()).Role != roleFull {
		writeErr(w, http.StatusForbidden, errors.New("listening port inspection requires a full device"))
		return
	}
	protocol := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("protocol")))
	switch protocol {
	case "", "tcp", "udp":
	default:
		writeErrCode(w, http.StatusBadRequest, "bad_request", errors.New("protocol must be tcp or udp"))
		return
	}
	params := proto.MachinePortsParams{Protocol: protocol}
	out, ok := h.callNode(w, r, m, proto.MethodMachinePorts, params, 20*time.Second, "", nil)
	if !ok {
		return
	}
	writeRawJSON(w, out)
}

func (h *Hub) handleMachineProcesses(w http.ResponseWriter, r *http.Request, m *Machine) {
	if principalOf(r.Context()).Role != roleFull {
		writeErr(w, http.StatusForbidden, errors.New("process inspection requires a full device"))
		return
	}
	q := r.URL.Query()
	sortBy := strings.ToLower(strings.TrimSpace(q.Get("sort")))
	switch sortBy {
	case "", "cpu", "memory", "uptime":
	default:
		writeErrCode(w, http.StatusBadRequest, "bad_request", errors.New("sort must be cpu, memory, or uptime"))
		return
	}
	query := strings.TrimSpace(q.Get("query"))
	if query == "" {
		query = strings.TrimSpace(q.Get("q"))
	}
	orphanOnly := q.Get("orphan_only") == "1" || strings.EqualFold(q.Get("orphan_only"), "true")
	var limit int
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			writeErrCode(w, http.StatusBadRequest, "bad_request", errors.New("limit must be a positive integer"))
			return
		}
		limit = v
	}
	params := proto.MachineProcessesParams{
		Sort:       sortBy,
		Query:      query,
		OrphanOnly: orphanOnly,
		Limit:      limit,
	}
	out, ok := h.callNode(w, r, m, proto.MethodMachineProcesses, params, 20*time.Second, "", nil)
	if !ok {
		return
	}
	writeRawJSON(w, out)
}

func (h *Hub) handleMachineProcessKill(w http.ResponseWriter, r *http.Request, m *Machine) {
	if principalOf(r.Context()).Role != roleFull {
		writeErr(w, http.StatusForbidden, errors.New("process termination requires a full device"))
		return
	}
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 1 {
		writeErrCode(w, http.StatusBadRequest, "bad_request", errors.New("pid must be an integer greater than 1"))
		return
	}
	var body struct {
		Signal     string `json:"signal"`
		Confirm    bool   `json:"confirm"`
		ConfirmPID int    `json:"confirm_pid"`
	}
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 16<<10))
	if err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", err)
		return
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		writeErrCode(w, http.StatusBadRequest, "bad_request", errors.New("bad JSON: "+err.Error()))
		return
	}
	sig := strings.ToUpper(strings.TrimSpace(body.Signal))
	switch sig {
	case "SIGTERM", "SIGKILL":
	default:
		writeErrCode(w, http.StatusBadRequest, proto.CodeInvalidSignal, errors.New("signal must be SIGTERM or SIGKILL"))
		return
	}
	if !body.Confirm {
		writeErrCode(w, http.StatusBadRequest, "confirmation_required", errors.New("explicit confirmation is required to terminate a process"))
		return
	}
	if sig == "SIGKILL" && body.ConfirmPID != pid {
		writeErrCode(w, http.StatusBadRequest, "confirmation_required", errors.New("SIGKILL requires confirm_pid matching the target PID"))
		return
	}

	params := proto.MachineProcessKillParams{PID: pid, Signal: sig}
	out, ok := h.callNode(w, r, m, proto.MethodMachineProcessKill, params, 15*time.Second, "", nil, func(err error) bool {
		var re *rpc.Error
		if !errors.As(err, &re) {
			return false
		}
		switch re.Code {
		case proto.CodeProcessNotFound:
			writeErrCode(w, http.StatusNotFound, re.Code, errors.New(re.Message))
			return true
		case proto.CodeProcessNotOwned:
			writeErrCode(w, http.StatusForbidden, re.Code, errors.New(re.Message))
			return true
		case proto.CodeProcessProtected:
			writeErrCode(w, http.StatusForbidden, re.Code, errors.New(re.Message))
			return true
		case proto.CodeInvalidSignal:
			writeErrCode(w, http.StatusBadRequest, re.Code, errors.New(re.Message))
			return true
		case proto.CodeProcessUnsupported:
			writeErrCode(w, http.StatusNotImplemented, re.Code, errors.New(re.Message))
			return true
		default:
			return false
		}
	})
	if !ok {
		return
	}
	h.logf("machine %q: sent %s to pid %d (device %q)", m.cfg.ID, sig, pid, principalOf(r.Context()).ID)
	writeRawJSON(w, out)
}
