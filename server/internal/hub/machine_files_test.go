package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/extwire"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

type machineFilesTestNode struct {
	data          []byte
	chunkRequests []proto.MachineFileChunkRequest
	m             *Machine
	closeFn       func()
}

func (n *machineFilesTestNode) connect(t *testing.T) {
	t.Helper()
	toNode, nodeIn := io.Pipe()
	toHub, hubIn := io.Pipe()
	hubPeer, nodePeer := rpc.New("h", nodeIn), rpc.New("n", hubIn)
	nodePeer.OnRequest = n.handle
	ctx, cancel := context.WithCancel(context.Background())
	go hubPeer.Serve(ctx, toHub)
	go nodePeer.Serve(ctx, toNode)
	n.closeFn = func() {
		cancel()
		_ = toNode.Close()
		_ = toHub.Close()
		hubPeer.Close(rpc.ErrClosed)
	}
	n.m.mu.Lock()
	n.m.peer = hubPeer
	n.m.mu.Unlock()
	t.Cleanup(n.closeFn)
}

func (n *machineFilesTestNode) handle(_ context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case proto.MethodMachineFilesRoots:
		return proto.MachineFileRootsResponse{
			Home: "/home/dev",
			OS:   "linux",
			Roots: []proto.MachineFileRoot{
				{ID: "home", Label: "$HOME", Path: "/home/dev", Kind: "home", Exists: true},
			},
		}, nil
	case proto.MethodMachineFilesList:
		var req proto.MachineFileListRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if req.Path == "/etc/shadow" {
			return nil, &rpc.Error{Code: proto.CodeMachineFileBoundary, Message: "path escapes the allowed root boundary"}
		}
		return proto.MachineFileListResponse{
			Path:          "/home/dev",
			CanonicalPath: "/home/dev",
			Entries: []proto.MachineFileEntry{
				{Name: ".env", Path: "/home/dev/.env", Kind: "file", Size: 16, Mode: "-rw-------", Perm: 0o600, Sensitive: true, SensitiveReason: "env"},
			},
			Total: 1,
		}, nil
	case proto.MethodMachineFilesPreview:
		var req proto.MachineFilePreviewRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if req.Path == "/home/dev/image.png" {
			return proto.MachineFilePreviewResponse{
				Path:     req.Path,
				Name:     "image.png",
				Size:     1024,
				Mime:     "image/png",
				Revision: "r1",
				Binary:   true,
				MaxBytes: proto.MachineFilePreviewDefaultBytes,
			}, nil
		}
		return proto.MachineFilePreviewResponse{
			Path:      req.Path,
			Name:      "app.log",
			Size:      12,
			Mime:      "text/plain; charset=utf-8",
			Revision:  "r1",
			Text:      "hello world\n",
			BytesRead: 12,
			MaxBytes:  proto.MachineFilePreviewDefaultBytes,
		}, nil
	case proto.MethodMachineFilesTail:
		var req proto.MachineFileTailRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return proto.MachineFileTailResponse{
			Path:           req.Path,
			Name:           "app.log",
			Size:           10000,
			Mime:           "text/plain; charset=utf-8",
			Revision:       "r1",
			Text:           "last line\n",
			Lines:          1,
			RequestedLines: req.Lines,
			BytesRead:      10,
			Offset:         9990,
			Truncated:      true,
		}, nil
	case proto.MethodMachineFilesChunk:
		var req proto.MachineFileChunkRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		n.chunkRequests = append(n.chunkRequests, req)
		if req.Path == "/home/dev/.ssh/id_rsa" {
			return nil, &rpc.Error{Code: proto.CodeMachineFileBoundary, Message: "this path holds secrets and is not readable remotely"}
		}
		out := proto.MachineFileChunkResponse{
			Path:          req.Path,
			CanonicalPath: req.Path,
			Name:          "bundle.bin",
			Size:          int64(len(n.data)),
			Mime:          "application/octet-stream",
			ModTime:       time.Unix(1700000000, 0).UTC(),
			Revision:      "rev-1",
			Offset:        req.Offset,
		}
		if req.Length > 0 {
			end := min(int64(len(n.data)), req.Offset+int64(req.Length))
			out.Data = n.data[req.Offset:end]
		}
		return out, nil
	default:
		return nil, errors.New("unexpected method " + method)
	}
}

func machineFilesFixture(t *testing.T, data []byte) (*http.ServeMux, *machineFilesTestNode) {
	t.Helper()
	h := newTestHub(t)
	node := &machineFilesTestNode{data: data}
	node.m = newMachine(h, MachineConfig{ID: "m1", Name: "devbox"})
	h.byID["m1"] = node.m
	node.connect(t)
	mux := http.NewServeMux()
	h.machineFileRoutes(mux)
	return mux, node
}

func TestMachineFilesHubRoutesAndBoundaryError(t *testing.T) {
	mux, _ := machineFilesFixture(t, []byte("payload"))

	// Roots
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/machines/m1/files/roots", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("roots status=%d body=%s", rec.Code, rec.Body.String())
	}
	var roots proto.MachineFileRootsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &roots); err != nil || len(roots.Roots) != 1 {
		t.Fatalf("roots decode: %v %+v", err, roots)
	}

	// List
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/machines/m1/files/list?path=/home/dev", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Boundary error maps to 403 machine_file_boundary
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/machines/m1/files/list?path=/etc/shadow", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("boundary status=%d want 403 body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(proto.CodeMachineFileBoundary)) {
		t.Fatalf("expected boundary code in body: %s", rec.Body.String())
	}

	// Preview (text & binary) and Tail
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/machines/m1/files/preview?path=/home/dev/image.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("binary preview status=%d", rec.Code)
	}
	var prev proto.MachineFilePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &prev); err != nil || !prev.Binary || prev.Text != "" {
		t.Fatalf("binary preview mismatch: %v %+v", err, prev)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/machines/m1/files/tail?path=/home/dev/app.log&lines=50", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("tail status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Missing path validation
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/machines/m1/files/preview", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing path status=%d want 400", rec.Code)
	}
}

func TestMachineFilesStreamingDownloadMultiChunkAndRange(t *testing.T) {
	// > 2 chunks so download must stream multiple bounded RPC chunks rather than one full buffer.
	data := bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, (proto.MachineFileDownloadChunkBytes/2)+17)
	mux, node := machineFilesFixture(t, data)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/machines/m1/files/download?path=/home/dev/bundle.bin", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("downloaded bytes length=%d want=%d", rec.Body.Len(), len(data))
	}
	if rec.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("missing sandbox CSP header")
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing nosniff header")
	}
	// Verify chunked streaming: 1 metadata call + >= 3 chunk calls, each <= MachineFileDownloadChunkBytes.
	if len(node.chunkRequests) < 4 {
		t.Fatalf("expected multi-chunk streaming, got %d requests", len(node.chunkRequests))
	}
	for _, q := range node.chunkRequests {
		if q.Length > proto.MachineFileDownloadChunkBytes {
			t.Fatalf("chunk request exceeded bound: %d", q.Length)
		}
	}

	// Range request (206 Partial Content).
	req := httptest.NewRequest("GET", "/api/v1/machines/m1/files/download?path=/home/dev/bundle.bin", nil)
	req.Header.Set("Range", "bytes=10-25")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range status=%d want 206", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), data[10:26]) {
		t.Fatalf("range body mismatch")
	}
	wantRange := "bytes 10-25/" + strconv.Itoa(len(data))
	if got := rec.Header().Get("Content-Range"); got != wantRange {
		t.Fatalf("Content-Range=%q want %q", got, wantRange)
	}
}

func TestMachineFilesRoutesRequireFullCredentialsOnly(t *testing.T) {
	patterns := routePatterns(t)
	for _, p := range []string{
		"GET /api/v1/machines/{mid}/files/roots",
		"GET /api/v1/machines/{mid}/files/list",
		"GET /api/v1/machines/{mid}/files/preview",
		"GET /api/v1/machines/{mid}/files/tail",
		"GET /api/v1/machines/{mid}/files/download",
	} {
		if !slices.Contains(patterns, p) {
			t.Errorf("missing machine files route %s", p)
		}
		if scope := extwire.RouteScopes[p]; scope != "" {
			t.Errorf("machine files route %s must not grant extension scope %q", p, scope)
		}
	}
}
