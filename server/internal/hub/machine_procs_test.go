package hub

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nwverse/persona/server/internal/extwire"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

func callMachineProcRoute(h *Hub, role, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.SetPathValue("mid", "m1")
	if strings.Contains(path, "/processes/") && strings.HasSuffix(path, "/kill") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			r.SetPathValue("pid", parts[len(parts)-2])
		}
	}
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, principal{Role: role, ID: "dev-web"}))
	w := httptest.NewRecorder()
	switch {
	case method == http.MethodGet && strings.Contains(path, "/ports"):
		h.withMachine(h.handleMachinePorts)(w, r)
	case method == http.MethodGet && strings.Contains(path, "/processes"):
		h.withMachine(h.handleMachineProcesses)(w, r)
	case method == http.MethodPost && strings.HasSuffix(path, "/kill"):
		h.withMachine(h.handleMachineProcessKill)(w, r)
	}
	return w
}

func TestMachineProcRoutesRequireFullDevice(t *testing.T) {
	tn := newTermNode(t)
	h := tn.hub

	for _, role := range []string{roleAnswer, roleExtension, roleLink, roleScoped} {
		for _, tc := range []struct {
			method string
			path   string
			body   string
		}{
			{http.MethodGet, "/api/v1/machines/m1/ports", ""},
			{http.MethodGet, "/api/v1/machines/m1/processes", ""},
			{http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGTERM","confirm":true}`},
		} {
			w := callMachineProcRoute(h, role, tc.method, tc.path, tc.body)
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s with role %s: got status %d, want 403", tc.method, tc.path, role, w.Code)
			}
		}
	}
	if got := tn.await(t, 0); len(got) != 0 {
		t.Fatalf("node heard refused requests: %q", got)
	}

	// Verify no extension scope opens these routes.
	for _, pattern := range []string{
		"GET /api/v1/machines/{mid}/ports",
		"GET /api/v1/machines/{mid}/processes",
		"POST /api/v1/machines/{mid}/processes/{pid}/kill",
	} {
		if scope := extwire.RouteScopes[pattern]; scope != "" {
			t.Errorf("route %s must not grant extension scope %q", pattern, scope)
		}
	}
}

func TestMachineProcRoutesForwardParamsAndConfirmationGates(t *testing.T) {
	tn := newTermNode(t)
	h := tn.hub
	tn.answers = map[string]any{
		proto.MethodMachinePorts:       proto.MachinePortsResult{At: 1, Supported: true, Ports: []proto.ListeningPort{}},
		proto.MethodMachineProcesses:   proto.MachineProcessesResult{At: 1, Supported: true, Processes: []proto.MachineProcess{}},
		proto.MethodMachineProcessKill: proto.MachineProcessKillResult{PID: 4321, Signal: "SIGTERM", Sent: true},
	}

	// Bad query validation happens before node call.
	if w := callMachineProcRoute(h, roleFull, http.MethodGet, "/api/v1/machines/m1/ports?protocol=icmp", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid protocol status = %d, want 400", w.Code)
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodGet, "/api/v1/machines/m1/processes?sort=invalid", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid sort status = %d, want 400", w.Code)
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodGet, "/api/v1/machines/m1/processes?limit=0", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want 400", w.Code)
	}

	// Kill confirmation gates.
	if w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/1/kill", `{"signal":"SIGTERM","confirm":true}`); w.Code != http.StatusBadRequest {
		t.Fatalf("pid 1 status = %d, want 400", w.Code)
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGTERM","confirm":false}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "confirmation_required") {
		t.Fatalf("unconfirmed SIGTERM: status=%d body=%s", w.Code, w.Body.String())
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGKILL","confirm":true}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "confirmation_required") {
		t.Fatalf("SIGKILL without confirm_pid: status=%d body=%s", w.Code, w.Body.String())
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGKILL","confirm":true,"confirm_pid":9999}`); w.Code != http.StatusBadRequest {
		t.Fatalf("SIGKILL with mismatched confirm_pid: status=%d", w.Code)
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGHUP","confirm":true}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid signal status = %d, want 400", w.Code)
	}
	if got := tn.await(t, 0); len(got) != 0 {
		t.Fatalf("validation failures should not reach node: %q", got)
	}

	// Valid requests reach node with expected RPC params.
	if w := callMachineProcRoute(h, roleFull, http.MethodGet, "/api/v1/machines/m1/ports?protocol=tcp", ""); w.Code != http.StatusOK {
		t.Fatalf("GET ports status = %d: %s", w.Code, w.Body.String())
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodGet, "/api/v1/machines/m1/processes?sort=memory&q=vitest&orphan_only=1&limit=25", ""); w.Code != http.StatusOK {
		t.Fatalf("GET processes status = %d: %s", w.Code, w.Body.String())
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGTERM","confirm":true}`); w.Code != http.StatusOK {
		t.Fatalf("POST SIGTERM status = %d: %s", w.Code, w.Body.String())
	}
	if w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGKILL","confirm":true,"confirm_pid":4321}`); w.Code != http.StatusOK {
		t.Fatalf("POST SIGKILL status = %d: %s", w.Code, w.Body.String())
	}

	got := tn.await(t, 4)
	if len(got) != 4 || got[0] != proto.MethodMachinePorts || got[1] != proto.MethodMachineProcesses || got[2] != proto.MethodMachineProcessKill || got[3] != proto.MethodMachineProcessKill {
		t.Fatalf("unexpected node call sequence: %q", got)
	}

	tn.mu.Lock()
	procArgs := tn.args[proto.MethodMachineProcesses]
	killArgs := tn.args[proto.MethodMachineProcessKill]
	tn.mu.Unlock()
	var pp proto.MachineProcessesParams
	if err := json.Unmarshal([]byte(procArgs), &pp); err != nil {
		t.Fatalf("unmarshal procArgs: %v", err)
	}
	if pp.Sort != "memory" || pp.Query != "vitest" || !pp.OrphanOnly || pp.Limit != 25 {
		t.Errorf("unexpected forwarded process params: %+v", pp)
	}
	var kp proto.MachineProcessKillParams
	if err := json.Unmarshal([]byte(killArgs), &kp); err != nil {
		t.Fatalf("unmarshal killArgs: %v", err)
	}
	if kp.PID != 4321 || kp.Signal != "SIGKILL" {
		t.Errorf("unexpected forwarded kill params: %+v", kp)
	}
}

func newFailingProcHub(t *testing.T, rpcErr *rpc.Error) *Hub {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h2nR, h2nW := io.Pipe()
	n2hR, n2hW := io.Pipe()
	hubSide := rpc.New("h", h2nW)
	nodeSide := rpc.New("n", n2hW)
	nodeSide.OnRequest = func(_ context.Context, _ string, _ json.RawMessage) (any, error) {
		return nil, rpcErr
	}
	go func() { _ = hubSide.Serve(ctx, n2hR) }()
	go func() { _ = nodeSide.Serve(ctx, h2nR) }()
	t.Cleanup(func() { cancel(); _ = h2nW.Close(); _ = n2hW.Close() })
	h := &Hub{byID: map[string]*Machine{}, logger: log.New(io.Discard, "", 0)}
	m := newMachine(h, MachineConfig{ID: "m1", Name: "m1"})
	m.peer = hubSide
	h.byID["m1"] = m
	return h
}

func TestMachineProcessKillMapsNodeRefusalCodes(t *testing.T) {
	for _, tc := range []struct {
		code       string
		wantStatus int
	}{
		{proto.CodeProcessNotFound, http.StatusNotFound},
		{proto.CodeProcessNotOwned, http.StatusForbidden},
		{proto.CodeProcessProtected, http.StatusForbidden},
		{proto.CodeInvalidSignal, http.StatusBadRequest},
		{proto.CodeProcessUnsupported, http.StatusNotImplemented},
	} {
		t.Run(tc.code, func(t *testing.T) {
			h := newFailingProcHub(t, &rpc.Error{Code: tc.code, Message: "refused: " + tc.code})
			w := callMachineProcRoute(h, roleFull, http.MethodPost, "/api/v1/machines/m1/processes/4321/kill", `{"signal":"SIGTERM","confirm":true}`)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.code) {
				t.Errorf("response body missing code %q: %s", tc.code, w.Body.String())
			}
		})
	}
}
