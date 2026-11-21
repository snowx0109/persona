package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nwverse/persona/server/internal/identity"
	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

// machinePathOptions controls canonical path and symlink boundary resolution.
// AllowMissingLeaf is designed so Phase 2 write operations (create, mkdir, upload)
// can reuse the exact same canonical ancestor + boundary validation helpers.
type machinePathOptions struct {
	AllowMissingLeaf bool
}

// machinePathResolution is the validated result of resolving an operator-supplied path.
type machinePathResolution struct {
	Input           string
	DisplayPath     string
	CanonicalPath   string
	RootPath        string
	Symlink         bool
	LinkTarget      string
	Sensitive       bool
	SensitiveReason string
}

// handleMachineFiles dispatches read-only machine file browser RPCs.
func (n *Node) handleMachineFiles(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case proto.MethodMachineFilesRoots:
		return n.machineFileRoots(ctx), nil
	case proto.MethodMachineFilesList:
		var req proto.MachineFileListRequest
		if len(params) > 0 {
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, fmt.Errorf("bad params: %w", err)
			}
		}
		return n.machineFileList(req)
	case proto.MethodMachineFilesPreview:
		var req proto.MachineFilePreviewRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("bad params: %w", err)
		}
		return n.machineFilePreview(req)
	case proto.MethodMachineFilesTail:
		var req proto.MachineFileTailRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("bad params: %w", err)
		}
		return n.machineFileTail(req)
	case proto.MethodMachineFilesChunk:
		var req proto.MachineFileChunkRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("bad params: %w", err)
		}
		return n.machineFileChunk(req)
	default:
		return nil, fmt.Errorf("unknown method %s", method)
	}
}

func machineBoundaryError(msg string) error {
	return &rpc.Error{Code: proto.CodeMachineFileBoundary, Message: msg}
}

// validateRawMachinePath rejects empty or control-character-injected paths before any filesystem touch.
func validateRawMachinePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	for _, r := range trimmed {
		if r == 0 || r < 0x20 || r == 0x7f {
			return "", machineBoundaryError("path contains invalid control characters")
		}
	}
	return trimmed, nil
}

// expandMachinePath expands ~ / ~/ against n.home and optionally anchors a relative path to boundaryRoot.
func (n *Node) expandMachinePath(raw, boundaryRoot string) (string, error) {
	p, err := validateRawMachinePath(raw)
	if err != nil {
		return "", err
	}
	switch {
	case p == "~":
		if n.home == "" {
			return "", fmt.Errorf("home directory is unavailable")
		}
		p = n.home
	case strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`):
		if n.home == "" {
			return "", fmt.Errorf("home directory is unavailable")
		}
		p = filepath.Join(n.home, p[2:])
	case !filepath.IsAbs(p):
		if strings.TrimSpace(boundaryRoot) == "" {
			return "", machineBoundaryError("path must be absolute or start with ~/")
		}
		p = filepath.Join(boundaryRoot, p)
	}
	return filepath.Clean(p), nil
}

// isWithinCanonicalRoot reports whether target lies at or strictly beneath canonical root.
// Both paths must be absolute and cleaned; prefix collisions like /home/alice2 vs /home/alice return false.
func isWithinCanonicalRoot(target, root string) bool {
	t := filepath.Clean(target)
	r := filepath.Clean(root)
	if !filepath.IsAbs(t) || !filepath.IsAbs(r) {
		return false
	}
	if !strings.EqualFold(filepath.VolumeName(t), filepath.VolumeName(r)) {
		return false
	}
	rel, err := filepath.Rel(r, t)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// resolveCanonicalBoundary validates an optional root boundary directory and returns its canonical path.
func (n *Node) resolveCanonicalBoundary(boundaryRoot string) (string, error) {
	expanded, err := n.expandMachinePath(boundaryRoot, "")
	if err != nil {
		return "", err
	}
	realRoot, err := n.realPath(absPath(expanded))
	if err != nil {
		if errors.Is(err, errOffLimits) || errors.Is(err, errICloud) || errors.Is(err, errTooManyLinks) {
			return "", machineBoundaryError(err.Error())
		}
		return "", explainDenied(expanded, n.home, err)
	}
	st, err := os.Stat(realRoot)
	if err != nil {
		return "", explainDenied(expanded, n.home, err)
	}
	if !st.IsDir() {
		return "", machineBoundaryError("root boundary must be a directory")
	}
	return filepath.Clean(realRoot), nil
}

// resolveMachinePath performs canonical path and symlink-boundary validation.
// It resolves symlinks component-by-component through n.realPath (enforcing off-limits secrets,
// iCloud, /proc & /sys protections, and symlink loop bounds) and enforces containment within
// boundaryRoot when provided.
func (n *Node) resolveMachinePath(asked, boundaryRoot string, opts machinePathOptions) (*machinePathResolution, error) {
	var realRoot string
	var expandedRoot string
	if strings.TrimSpace(boundaryRoot) != "" {
		var err error
		expandedRoot, err = n.expandMachinePath(boundaryRoot, "")
		if err != nil {
			return nil, err
		}
		realRoot, err = n.resolveCanonicalBoundary(boundaryRoot)
		if err != nil {
			return nil, err
		}
	}

	display, err := n.expandMachinePath(asked, expandedRoot)
	if err != nil {
		return nil, err
	}

	absDisplay := absPath(display)
	canonical, err := n.realPath(absDisplay)
	if err != nil {
		switch {
		case errors.Is(err, errOffLimits), errors.Is(err, errICloud), errors.Is(err, errTooManyLinks):
			return nil, machineBoundaryError(err.Error())
		case opts.AllowMissingLeaf && errors.Is(err, fs.ErrNotExist):
			canonical, err = n.resolveMissingLeaf(absDisplay)
			if err != nil {
				if errors.Is(err, errOffLimits) || errors.Is(err, errICloud) || errors.Is(err, errTooManyLinks) {
					return nil, machineBoundaryError(err.Error())
				}
				return nil, explainDenied(display, n.home, err)
			}
		default:
			return nil, explainDenied(display, n.home, err)
		}
	}
	canonical = filepath.Clean(canonical)

	if realRoot != "" && !isWithinCanonicalRoot(canonical, realRoot) {
		return nil, machineBoundaryError("path escapes the allowed root boundary")
	}

	res := &machinePathResolution{
		Input:         asked,
		DisplayPath:   display,
		CanonicalPath: canonical,
		RootPath:      realRoot,
	}

	// Inspect whether the leaf itself is a symlink without re-traversing off-limits paths.
	parent := filepath.Dir(absDisplay)
	if parent != absDisplay {
		if realParent, perr := n.realPath(parent); perr == nil {
			leaf := filepath.Join(realParent, filepath.Base(absDisplay))
			if lst, lerr := os.Lstat(leaf); lerr == nil && lst.Mode()&fs.ModeSymlink != 0 {
				res.Symlink = true
				if target, rerr := os.Readlink(leaf); rerr == nil {
					res.LinkTarget = target
				}
			}
		}
	}
	if !res.Symlink && fold(canonical) != fold(absDisplay) {
		res.Symlink = true
	}

	res.Sensitive, res.SensitiveReason = classifyMachineSensitivePath(display, canonical)
	return res, nil
}

// resolveMissingLeaf resolves the deepest existing ancestor through realPath and validates the
// missing relative suffix against off-limits rules so Phase 2 writes cannot escape via ancestor symlinks.
func (n *Node) resolveMissingLeaf(absDisplay string) (string, error) {
	cur := filepath.Clean(absDisplay)
	var tail []string
	for {
		realCur, err := n.realPath(cur)
		if err == nil {
			slices.Reverse(tail)
			joined := filepath.Clean(filepath.Join(append([]string{realCur}, tail...)...))
			if !isWithinCanonicalRoot(joined, realCur) {
				return "", machineBoundaryError("path traversal outside resolved ancestor")
			}
			if err := n.offLimitsResolved(joined); err != nil {
				return "", err
			}
			return joined, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

// openSafeMachineRegularFile opens a resolved regular file and verifies against TOCTOU swaps.
func (n *Node) openSafeMachineRegularFile(res *machinePathResolution) (*os.File, os.FileInfo, error) {
	before, err := os.Stat(res.CanonicalPath)
	if err != nil {
		return nil, nil, explainDenied(res.DisplayPath, n.home, err)
	}
	if before.IsDir() {
		return nil, nil, fmt.Errorf("path is a directory, not a regular file")
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("only regular files can be read")
	}
	f, err := os.Open(res.CanonicalPath)
	if err != nil {
		return nil, nil, explainDenied(res.DisplayPath, n.home, err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, nil, fmt.Errorf("only regular files can be read")
	}
	if !sameFileRead(before, st) {
		f.Close()
		return nil, nil, errFileChanged
	}
	// Re-verify the original display path still resolves to the exact same canonical file.
	rechecked, err := n.realPath(absPath(res.DisplayPath))
	if err != nil {
		f.Close()
		if errors.Is(err, errOffLimits) || errors.Is(err, errICloud) || errors.Is(err, errTooManyLinks) {
			return nil, nil, machineBoundaryError(err.Error())
		}
		return nil, nil, explainDenied(res.DisplayPath, n.home, err)
	}
	if res.RootPath != "" && !isWithinCanonicalRoot(rechecked, res.RootPath) {
		f.Close()
		return nil, nil, machineBoundaryError("path escapes the allowed root boundary")
	}
	after, err := os.Stat(rechecked)
	if err != nil || !sameFileRead(st, after) {
		f.Close()
		return nil, nil, errFileChanged
	}
	return f, st, nil
}

// classifyMachineSensitivePath flags sensitive config/secret files so the UI warns operators clearly.
func classifyMachineSensitivePath(paths ...string) (bool, string) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		slashed := fold(filepath.Clean(p))
		base := filepath.Base(slashed)

		// Hub / Persona configuration and keys.
		if base == "hub.json" || strings.HasPrefix(base, "hub.json.") ||
			base == "manager.json" || base == "link.json" ||
			base == "secret.key" || base == "hub.key" || base == "handles.key" {
			return true, "hub_config"
		}

		// Environment files (.env, .env.local, .env.production, etc.), excluding sample templates.
		if base == ".env" || (strings.HasPrefix(base, ".env.") &&
			!strings.HasSuffix(base, ".example") &&
			!strings.HasSuffix(base, ".sample") &&
			!strings.HasSuffix(base, ".template") &&
			!strings.HasSuffix(base, ".dist")) {
			return true, "env"
		}

		// Private keys and keystores.
		if keyNames[base] ||
			strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") ||
			strings.HasSuffix(base, ".p8") || strings.HasSuffix(base, ".p12") ||
			strings.HasSuffix(base, ".pfx") || strings.HasSuffix(base, ".jks") ||
			strings.HasSuffix(base, ".kdbx") ||
			strings.Contains(slashed, "/.ssh/") || strings.HasSuffix(slashed, "/.ssh") ||
			strings.Contains(slashed, "/.gnupg/") || strings.HasSuffix(slashed, "/.gnupg") {
			return true, "private_key"
		}

		// Common credential files and secret directories.
		if base == ".netrc" || base == ".git-credentials" || base == ".npmrc" ||
			base == ".pypirc" || base == ".pgpass" ||
			base == ".credentials.json" || base == "credentials.json" ||
			base == "auth.json" || base == "secrets.json" ||
			base == "secrets.yaml" || base == "secrets.yml" ||
			strings.HasSuffix(base, ".secret") ||
			strings.Contains(slashed, "/.aws/") || strings.HasSuffix(slashed, "/.aws") ||
			strings.Contains(slashed, "/.kube/") || strings.HasSuffix(slashed, "/.kube") ||
			strings.Contains(slashed, "/.docker/") || strings.HasSuffix(slashed, "/.docker") {
			return true, "credentials"
		}
	}
	return false, ""
}

// binaryExtensionHint is a lightweight listing-time hint (exact binary refusal uses byte sniffing).
var binaryExtensionHint = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true, ".ico": true, ".heic": true, ".avif": true,
	".pdf": true, ".zip": true, ".gz": true, ".tgz": true, ".tar": true, ".bz2": true, ".xz": true, ".zst": true, ".7z": true, ".rar": true,
	".mp3": true, ".wav": true, ".flac": true, ".m4a": true, ".ogg": true, ".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".wasm": true, ".bin": true, ".o": true, ".a": true, ".class": true, ".jar": true,
	".dmg": true, ".pkg": true, ".iso": true, ".deb": true, ".rpm": true, ".apk": true, ".ipa": true,
	".sqlite": true, ".db": true, ".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
	".docx": true, ".xlsx": true, ".pptx": true, ".doc": true, ".xls": true, ".ppt": true,
}

func isBinaryExtension(name string) bool {
	return binaryExtensionHint[strings.ToLower(filepath.Ext(name))]
}

// isBinaryPreviewContent detects binary payloads so raw text rendering is refused.
func isBinaryPreviewContent(path string, data []byte, trimLeadingUTF8, trimTrailingUTF8 bool) (bool, string) {
	sniff := data[:min(len(data), sniffBytes)]
	mime := fileMime(path, sniff)
	if len(data) == 0 {
		return false, mime
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return true, mime
	}
	if streamedMime(mime) {
		return true, mime
	}
	check := data
	if trimLeadingUTF8 {
		check = trimLeadingPartialRune(check)
	}
	if trimTrailingUTF8 {
		check = trimPartialRune(check)
	}
	if !utf8.Valid(check) {
		return true, mime
	}
	// Reject payloads with a high density of non-text C0 control bytes (allow \t, \n, \v, \f, \r, ESC).
	badCtrl := 0
	for _, b := range check {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\v' && b != '\f' && b != '\r' && b != 0x1b {
			badCtrl++
		}
	}
	if badCtrl > 0 && (badCtrl*50 > len(check) || badCtrl > 16) {
		return true, mime
	}
	return false, mime
}

// trimLeadingPartialRune skips up to 3 leading UTF-8 continuation bytes when a slice begins mid-rune.
func trimLeadingPartialRune(b []byte) []byte {
	for i := 0; i < 3 && len(b) > 0; i++ {
		if (b[0] & 0xc0) != 0x80 {
			break
		}
		b = b[1:]
	}
	return b
}

// machineFileRoots returns quick root shortcuts: $HOME, workspaces, and common config directories.
func (n *Node) machineFileRoots(ctx context.Context) proto.MachineFileRootsResponse {
	resp := proto.MachineFileRootsResponse{
		Home:  n.home,
		OS:    runtime.GOOS,
		Roots: []proto.MachineFileRoot{},
	}
	seen := map[string]bool{}
	add := func(id, label, path, kind string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		res, err := n.resolveMachinePath(path, "", machinePathOptions{})
		if err != nil {
			return
		}
		st, err := os.Stat(res.CanonicalPath)
		if err != nil || !st.IsDir() {
			return
		}
		key := fold(res.CanonicalPath)
		if seen[key] {
			return
		}
		seen[key] = true
		resp.Roots = append(resp.Roots, proto.MachineFileRoot{
			ID:     id,
			Label:  label,
			Path:   res.DisplayPath,
			Kind:   kind,
			Exists: true,
		})
	}

	if n.home != "" {
		add("home", "$HOME", n.home, "home")
	}

	// Active workspaces from terminal/runtime hosts (if available).
	if n.hosts != nil {
		if ws, err := n.listWorkspaces(ctx); err == nil {
			wsCount := 0
			for _, w := range ws.Workspaces {
				if w.Cwd == "" {
					continue
				}
				label := strings.TrimSpace(w.Title)
				if label == "" {
					label = filepath.Base(w.Cwd)
				}
				add("ws:"+w.ID, label, w.Cwd, "workspace")
				wsCount++
				if wsCount >= 8 {
					break
				}
			}
		}
	}

	// Recently used agent directories as workspace shortcuts.
	for i, d := range n.recentDirs() {
		if i >= 6 {
			break
		}
		add(fmt.Sprintf("recent:%d", i), d.Name, d.Path, "workspace")
	}

	// Config directories (existing only).
	if n.home != "" {
		dataDir := identity.DataDir(n.home)
		if dataDir != "" {
			add("cfg:persona", filepath.Base(dataDir), dataDir, "config")
		}
		add("cfg:config", ".config", filepath.Join(n.home, ".config"), "config")
		add("cfg:claude", ".claude", filepath.Join(n.home, ".claude"), "config")
		add("cfg:codex", ".codex", filepath.Join(n.home, ".codex"), "config")
	}
	if runtime.GOOS != "windows" {
		add("cfg:etc", "/etc", "/etc", "config")
	}

	return resp
}

// machineFileList performs a strictly single-level os.ReadDir listing (never recursive).
func (n *Node) machineFileList(req proto.MachineFileListRequest) (*proto.MachineFileListResponse, error) {
	path := strings.TrimSpace(req.Path)
	if path == "" {
		if strings.TrimSpace(req.Root) != "" {
			path = req.Root
		} else {
			path = "~"
		}
	}
	res, err := n.resolveMachinePath(path, req.Root, machinePathOptions{})
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(res.CanonicalPath)
	if err != nil {
		return nil, explainDenied(res.DisplayPath, n.home, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}
	ents, err := os.ReadDir(res.CanonicalPath)
	if err != nil {
		return nil, explainDenied(res.DisplayPath, n.home, err)
	}

	limit := req.Limit
	if limit <= 0 || limit > proto.MachineFileListMaxLimit {
		limit = proto.MachineFileListDefaultLimit
	}

	type sortable struct {
		entry proto.MachineFileEntry
		key   string
		dir   bool
	}
	all := make([]sortable, 0, len(ents))
	for _, e := range ents {
		name := e.Name()
		entryDisplay := filepath.Join(res.DisplayPath, name)
		entryReal := filepath.Join(res.CanonicalPath, name)
		isSymlink := e.Type()&fs.ModeSymlink != 0

		item := proto.MachineFileEntry{
			Name:    name,
			Path:    entryDisplay,
			Symlink: isSymlink,
		}
		if info, ierr := e.Info(); ierr == nil {
			item.Size = info.Size()
			item.Mode = info.Mode().String()
			item.Perm = uint32(info.Mode().Perm())
			item.ModTime = info.ModTime().UTC()
		}

		isDir := e.IsDir()
		switch {
		case isSymlink:
			item.Kind = "symlink"
			if target, rerr := os.Readlink(entryReal); rerr == nil {
				item.LinkTarget = target
			}
			resolvedTarget, tst, serr := n.statSafe(entryReal)
			switch {
			case serr == nil:
				if res.RootPath != "" && !isWithinCanonicalRoot(resolvedTarget, res.RootPath) {
					// Symlink points outside boundary root: keep metadata minimal and do not advertise traversal.
					item.TargetKind = "other"
				} else if tst.IsDir() {
					item.TargetKind = "dir"
					isDir = true
				} else if tst.Mode().IsRegular() {
					item.TargetKind = "file"
					item.Size = tst.Size()
				} else {
					item.TargetKind = "other"
				}
			case errors.Is(serr, fs.ErrNotExist), errors.Is(serr, errTooManyLinks):
				item.LinkBroken = true
			}
		case isDir:
			item.Kind = "dir"
		case e.Type().IsRegular():
			item.Kind = "file"
		default:
			item.Kind = "other"
		}

		sensitive, reason := classifyMachineSensitivePath(entryDisplay, entryReal)
		if !sensitive && n.offLimitsResolved(entryReal) != nil {
			sensitive = true
			reason = "protected"
		}
		item.Sensitive = sensitive
		item.SensitiveReason = reason
		if item.Kind == "file" || (item.Symlink && item.TargetKind == "file") {
			item.Binary = isBinaryExtension(name)
		}

		all = append(all, sortable{entry: item, key: strings.ToLower(name), dir: isDir})
	}

	slices.SortFunc(all, func(a, b sortable) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return strings.Compare(a.entry.Name, b.entry.Name)
	})

	total := len(all)
	truncated := total > limit
	kept := min(total, limit)
	out := make([]proto.MachineFileEntry, kept)
	for i := 0; i < kept; i++ {
		out[i] = all[i].entry
	}

	parent := filepath.Dir(res.DisplayPath)
	if parent == res.DisplayPath {
		parent = ""
	}
	if res.RootPath != "" && parent != "" {
		if pres, perr := n.resolveMachinePath(parent, req.Root, machinePathOptions{}); perr != nil || !isWithinCanonicalRoot(pres.CanonicalPath, res.RootPath) {
			parent = ""
		}
	}

	return &proto.MachineFileListResponse{
		Path:          res.DisplayPath,
		CanonicalPath: res.CanonicalPath,
		Parent:        parent,
		Entries:       out,
		Total:         total,
		Truncated:     truncated,
	}, nil
}

// machineFilePreview returns a bounded UTF-8 text preview (hard cap 2 MiB) or refuses raw text for binaries.
func (n *Node) machineFilePreview(req proto.MachineFilePreviewRequest) (*proto.MachineFilePreviewResponse, error) {
	res, err := n.resolveMachinePath(req.Path, req.Root, machinePathOptions{})
	if err != nil {
		return nil, err
	}
	f, st, err := n.openSafeMachineRegularFile(res)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = proto.MachineFilePreviewDefaultBytes
	}
	if maxBytes > proto.MachineFilePreviewMaxBytes {
		maxBytes = proto.MachineFilePreviewMaxBytes
	}

	size := st.Size()
	want := min(size, int64(maxBytes))
	buf := make([]byte, want)
	var nr int
	if want > 0 {
		nr, err = f.ReadAt(buf, 0)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		buf = buf[:nr]
	}

	more := size > int64(len(buf))
	// Handle virtual zero-size regular files (e.g. /proc/cpuinfo) safely within maxBytes.
	if size == 0 {
		vbuf, verr := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
		if verr == nil && len(vbuf) > 0 {
			if len(vbuf) > maxBytes {
				more = true
				vbuf = vbuf[:maxBytes]
			}
			buf = vbuf
			size = int64(len(buf))
		}
	}

	binary, mime := isBinaryPreviewContent(res.DisplayPath, buf, false, more)
	resp := &proto.MachineFilePreviewResponse{
		Path:            res.DisplayPath,
		CanonicalPath:   res.CanonicalPath,
		Name:            filepath.Base(res.DisplayPath),
		Size:            size,
		Mode:            st.Mode().String(),
		Perm:            uint32(st.Mode().Perm()),
		ModTime:         st.ModTime().UTC(),
		Mime:            mime,
		Revision:        fileRevision(st),
		Symlink:         res.Symlink,
		LinkTarget:      res.LinkTarget,
		Sensitive:       res.Sensitive,
		SensitiveReason: res.SensitiveReason,
		Binary:          binary,
		BytesRead:       int64(len(buf)),
		Truncated:       more,
		MaxBytes:        maxBytes,
	}
	if binary {
		return resp, nil
	}

	text := buf
	if more {
		text = trimPartialRune(text)
		if idx := bytes.LastIndexByte(text, '\n'); idx >= 0 {
			text = text[:idx+1]
		}
	}
	resp.Text = string(text)
	resp.BytesRead = int64(len(text))
	if !strings.HasPrefix(resp.Mime, "text/") && !strings.Contains(resp.Mime, "json") && !strings.Contains(resp.Mime, "xml") {
		resp.Mime = "text/plain; charset=utf-8"
	}
	return resp, nil
}

// machineFileTail reads the last N lines (bounded by MaxBytes) from the end of a file without reading the whole file.
func (n *Node) machineFileTail(req proto.MachineFileTailRequest) (*proto.MachineFileTailResponse, error) {
	res, err := n.resolveMachinePath(req.Path, req.Root, machinePathOptions{})
	if err != nil {
		return nil, err
	}
	f, st, err := n.openSafeMachineRegularFile(res)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lines := req.Lines
	if lines <= 0 {
		lines = proto.MachineFileTailDefaultLines
	}
	if lines > proto.MachineFileTailMaxLines {
		lines = proto.MachineFileTailMaxLines
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = proto.MachineFileTailDefaultBytes
	}
	if maxBytes > proto.MachineFileTailMaxBytes {
		maxBytes = proto.MachineFileTailMaxBytes
	}

	size := st.Size()
	resp := &proto.MachineFileTailResponse{
		Path:            res.DisplayPath,
		CanonicalPath:   res.CanonicalPath,
		Name:            filepath.Base(res.DisplayPath),
		Size:            size,
		ModTime:         st.ModTime().UTC(),
		Revision:        fileRevision(st),
		Sensitive:       res.Sensitive,
		SensitiveReason: res.SensitiveReason,
		RequestedLines:  lines,
	}

	if size == 0 {
		resp.Mime = fileMime(res.DisplayPath, nil)
		return resp, nil
	}

	// Probe file head first so binary files with trailing text headers/comments are still refused.
	headLen := min(size, int64(sniffBytes))
	head := make([]byte, headLen)
	if hn, herr := f.ReadAt(head, 0); herr == nil || errors.Is(herr, io.EOF) {
		head = head[:hn]
		if bin, mime := isBinaryPreviewContent(res.DisplayPath, head, false, size > int64(len(head))); bin {
			resp.Binary = true
			resp.Mime = mime
			return resp, nil
		}
	}

	// Walk backwards from EOF in bounded chunks until we have lines+1 newlines or hit maxBytes.
	const blockSize int64 = 32 << 10
	readStart := max(int64(0), size-int64(maxBytes))
	pos := size
	var chunks [][]byte
	totalRead := int64(0)
	newlineCount := 0
	// If the file ends with '\n', that trailing newline terminates the last line, so we need lines+1 newlines to find
	// the start of the N-th line from the end.
	targetNewlines := lines

	for pos > readStart {
		step := min(blockSize, pos-readStart)
		pos -= step
		chunk := make([]byte, step)
		nr, rerr := f.ReadAt(chunk, pos)
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return nil, rerr
		}
		chunk = chunk[:nr]
		if len(chunks) == 0 && len(chunk) > 0 && chunk[len(chunk)-1] == '\n' {
			targetNewlines = lines + 1
		}
		chunks = append(chunks, chunk)
		totalRead += int64(len(chunk))
		newlineCount += bytes.Count(chunk, []byte{'\n'})
		if newlineCount >= targetNewlines {
			break
		}
	}

	// Assemble collected chunks in forward order.
	buf := make([]byte, 0, totalRead)
	for i := len(chunks) - 1; i >= 0; i-- {
		buf = append(buf, chunks[i]...)
	}
	offset := pos

	// Trim to exact last N lines when we found enough newlines.
	trimmedByLines := false
	if newlineCount >= targetNewlines {
		seen := 0
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				seen++
				if seen == targetNewlines {
					offset += int64(i + 1)
					buf = buf[i+1:]
					trimmedByLines = true
					break
				}
			}
		}
	}

	// If we stopped mid-file due to maxBytes (not on an exact line boundary), advance to the next full line.
	if offset > 0 && !trimmedByLines {
		if nl := bytes.IndexByte(buf, '\n'); nl >= 0 && nl+1 < len(buf) {
			offset += int64(nl + 1)
			buf = buf[nl+1:]
		} else {
			trimmed := trimLeadingPartialRune(buf)
			offset += int64(len(buf) - len(trimmed))
			buf = trimmed
		}
	}

	binary, mime := isBinaryPreviewContent(res.DisplayPath, buf, offset > 0, false)
	resp.Mime = mime
	if binary {
		resp.Binary = true
		return resp, nil
	}

	text := string(buf)
	lineCount := strings.Count(text, "\n")
	if len(text) > 0 && !strings.HasSuffix(text, "\n") {
		lineCount++
	}
	resp.Text = text
	resp.Lines = lineCount
	resp.BytesRead = int64(len(buf))
	resp.Offset = offset
	resp.Truncated = offset > 0
	if !strings.HasPrefix(resp.Mime, "text/") && !strings.Contains(resp.Mime, "json") && !strings.Contains(resp.Mime, "xml") {
		resp.Mime = "text/plain; charset=utf-8"
	}
	return resp, nil
}

// machineFileChunk serves one bounded chunk for streaming downloads without buffering whole files.
func (n *Node) machineFileChunk(req proto.MachineFileChunkRequest) (*proto.MachineFileChunkResponse, error) {
	if req.Offset < 0 || req.Length < 0 || req.Length > proto.MachineFileDownloadChunkBytes {
		return nil, fmt.Errorf("invalid file chunk range")
	}
	res, err := n.resolveMachinePath(req.Path, req.Root, machinePathOptions{})
	if err != nil {
		return nil, err
	}
	f, st, err := n.openSafeMachineRegularFile(res)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	revision := fileRevision(st)
	if req.Revision != "" && req.Revision != revision {
		return nil, errFileChanged
	}
	if req.Offset > st.Size() {
		return nil, fmt.Errorf("offset is outside the file")
	}

	out := &proto.MachineFileChunkResponse{
		Path:          res.DisplayPath,
		CanonicalPath: res.CanonicalPath,
		Name:          filepath.Base(res.DisplayPath),
		Size:          st.Size(),
		ModTime:       st.ModTime().UTC(),
		Revision:      revision,
		Offset:        req.Offset,
	}
	if req.Length == 0 {
		out.Mime = fileMime(res.DisplayPath, readHead(f))
		return out, nil
	}
	out.Data = make([]byte, min(int64(req.Length), st.Size()-req.Offset))
	nr, err := f.ReadAt(out.Data, req.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if nr != len(out.Data) {
		return nil, io.ErrUnexpectedEOF
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != st.Size() || !after.ModTime().Equal(st.ModTime()) {
		return nil, errFileChanged
	}
	return out, nil
}
