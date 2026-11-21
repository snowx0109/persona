package hub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// machineFileRoutes registers human-only read-only machine file browser endpoints (Phase 6A).
// These routes require full credentials (no extension scope) and expose zero MCP tools.
func (h *Hub) machineFileRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /api/v1/machines/{mid}/files/roots", h.withMachine(h.machineFileRoots))
	api.HandleFunc("GET /api/v1/machines/{mid}/files/list", h.withMachine(h.machineFileList))
	api.HandleFunc("GET /api/v1/machines/{mid}/files/preview", h.withMachine(h.machineFilePreview))
	api.HandleFunc("GET /api/v1/machines/{mid}/files/tail", h.withMachine(h.machineFileTail))
	api.HandleFunc("GET /api/v1/machines/{mid}/files/download", h.withMachine(h.machineFileDownload))
}

func (h *Hub) machineFileCallErr(w http.ResponseWriter, r *http.Request, err error) {
	if h.nodeOutdated(w, r, err, "hub-download.outdated_file", nil) {
		return
	}
	var re *rpc.Error
	if errors.As(err, &re) {
		switch re.Code {
		case proto.CodeMachineFileBoundary:
			writeCode(w, http.StatusForbidden, proto.CodeMachineFileBoundary, re.Message, nil)
			return
		case proto.CodeMachineFileBinary:
			writeCode(w, http.StatusUnsupportedMediaType, proto.CodeMachineFileBinary, re.Message, nil)
			return
		case proto.CodeFileChanged:
			writeCode(w, http.StatusConflict, proto.CodeFileChanged, re.Message, nil)
			return
		}
	}
	callErr(w, err)
}

func (h *Hub) machineFileRoots(w http.ResponseWriter, r *http.Request, m *Machine) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var out proto.MachineFileRootsResponse
	if err := m.call(ctx, proto.MethodMachineFilesRoots, map[string]any{}, &out); err != nil {
		h.machineFileCallErr(w, r, err)
		return
	}
	writeJSON(w, out)
}

func (h *Hub) machineFileList(w http.ResponseWriter, r *http.Request, m *Machine) {
	q := r.URL.Query()
	req := proto.MachineFileListRequest{
		Path:  q.Get("path"),
		Root:  q.Get("root"),
		Limit: atoi(q.Get("limit")),
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var out proto.MachineFileListResponse
	if err := m.call(ctx, proto.MethodMachineFilesList, req, &out); err != nil {
		h.machineFileCallErr(w, r, err)
		return
	}
	writeJSON(w, out)
}

func (h *Hub) machineFilePreview(w http.ResponseWriter, r *http.Request, m *Machine) {
	q := r.URL.Query()
	req := proto.MachineFilePreviewRequest{
		Path:     q.Get("path"),
		Root:     q.Get("root"),
		MaxBytes: atoi(q.Get("max_bytes")),
	}
	if strings.TrimSpace(req.Path) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("path is required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var out proto.MachineFilePreviewResponse
	if err := m.call(ctx, proto.MethodMachineFilesPreview, req, &out); err != nil {
		h.machineFileCallErr(w, r, err)
		return
	}
	writeJSON(w, out)
}

func (h *Hub) machineFileTail(w http.ResponseWriter, r *http.Request, m *Machine) {
	q := r.URL.Query()
	req := proto.MachineFileTailRequest{
		Path:     q.Get("path"),
		Root:     q.Get("root"),
		Lines:    atoi(q.Get("lines")),
		MaxBytes: atoi(q.Get("max_bytes")),
	}
	if strings.TrimSpace(req.Path) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("path is required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var out proto.MachineFileTailResponse
	if err := m.call(ctx, proto.MethodMachineFilesTail, req, &out); err != nil {
		h.machineFileCallErr(w, r, err)
		return
	}
	writeJSON(w, out)
}

// machineFileDownload streams a single regular file chunk-by-chunk over HTTP without buffering the whole file in memory.
func (h *Hub) machineFileDownload(w http.ResponseWriter, r *http.Request, m *Machine) {
	q := r.URL.Query()
	req := proto.MachineFileChunkRequest{
		Path: q.Get("path"),
		Root: q.Get("root"),
	}
	if strings.TrimSpace(req.Path) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("path is required"))
		return
	}
	fetch := func(p proto.MachineFileChunkRequest) (*proto.MachineFileChunkResponse, error) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		var out proto.MachineFileChunkResponse
		if err := m.call(ctx, proto.MethodMachineFilesChunk, p, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	info, err := fetch(req)
	if err != nil {
		h.machineFileCallErr(w, r, err)
		return
	}
	if info.Size < 0 || info.Revision == "" {
		writeErr(w, http.StatusBadGateway, errors.New("invalid file metadata"))
		return
	}
	req.Path = info.Path
	req.Revision = info.Revision

	contentType := info.Mime
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", fmt.Sprintf("%q", info.Revision))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")

	reader := &machineFileStreamReader{
		fetch:   fetch,
		request: req,
		size:    info.Size,
		back:    func(pause time.Duration, until time.Time) bool { return awaitMachine(r.Context(), m, pause, until) },
	}
	http.ServeContent(w, r, info.Name, info.ModTime, reader)
	if failed := reader.failed.Load(); failed != nil {
		if r.Context().Err() == nil {
			h.logf("[%s] machine file download from %s aborted at byte %d of %d: %v",
				w.Header().Get("X-Request-Id"), m.cfg.ID, failed.at, info.Size, failed.err)
		}
		panic(http.ErrAbortHandler)
	}
}

type machineFileStreamReader struct {
	fetch   func(proto.MachineFileChunkRequest) (*proto.MachineFileChunkResponse, error)
	back    func(pause time.Duration, until time.Time) bool
	request proto.MachineFileChunkRequest
	size    int64
	pos     int64
	buf     []byte
	failed  atomic.Pointer[readFailure]
}

func (f *machineFileStreamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if f.pos >= f.size {
		return 0, io.EOF
	}
	if len(f.buf) == 0 {
		q := f.request
		q.Offset = f.pos
		q.Length = int(min(int64(proto.MachineFileDownloadChunkBytes), f.size-f.pos))
		chunk, err := f.readChunk(q)
		if err == nil && (chunk.Offset != q.Offset || chunk.Size != f.size || chunk.Revision != q.Revision || len(chunk.Data) != q.Length) {
			err = errors.New("file chunk does not match the download")
		}
		if err != nil {
			f.failed.Store(&readFailure{at: q.Offset, err: err})
			return 0, err
		}
		f.buf = chunk.Data
	}
	n := copy(p, f.buf)
	f.buf = f.buf[n:]
	f.pos += int64(n)
	return n, nil
}

func (f *machineFileStreamReader) readChunk(q proto.MachineFileChunkRequest) (*proto.MachineFileChunkResponse, error) {
	var until time.Time
	pause := downloadRetryPoll
	for {
		c, err := f.fetch(q)
		if err == nil || f.back == nil || !nodeAway(err) {
			return c, err
		}
		if until.IsZero() {
			until = time.Now().Add(downloadRetryWindow)
		}
		if !f.back(pause, until) {
			return nil, err
		}
		pause = min(2*pause, downloadRetryMax)
	}
}

func (f *machineFileStreamReader) Seek(offset int64, whence int) (int64, error) {
	var target int64
	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = f.pos + offset
	case io.SeekEnd:
		target = f.size + offset
	default:
		return 0, errors.New("invalid seek origin")
	}
	if target < 0 || target > f.size {
		return 0, errors.New("seek outside the file")
	}
	if target != f.pos {
		f.pos, f.buf = target, nil
	}
	return f.pos, nil
}
