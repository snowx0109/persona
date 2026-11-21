package proto

import "time"

// Read-only machine file browser RPC methods and bounds (Phase 6A).
// Write operations will build on the same canonical path/symlink boundary helpers in a follow-up phase.
const (
	MethodMachineFilesRoots   = "machine.files.roots"
	MethodMachineFilesList    = "machine.files.list"
	MethodMachineFilesPreview = "machine.files.preview"
	MethodMachineFilesTail    = "machine.files.tail"
	MethodMachineFilesChunk   = "machine.files.chunk"

	MachineFileListDefaultLimit    = 2000
	MachineFileListMaxLimit        = 2000
	MachineFilePreviewDefaultBytes = 256 << 10 // 256 KiB default bounded text preview
	MachineFilePreviewMaxBytes     = 2 << 20   // 2 MiB hard preview cap
	MachineFileTailDefaultLines    = 200
	MachineFileTailMaxLines        = 5000
	MachineFileTailDefaultBytes    = 128 << 10 // 128 KiB default log tail window
	MachineFileTailMaxBytes        = 2 << 20   // 2 MiB hard tail byte cap
	MachineFileDownloadChunkBytes  = FileDownloadChunkBytes

	// CodeMachineFileBoundary indicates a path traversal, symlink escape, or off-limits secret boundary refusal.
	CodeMachineFileBoundary = "machine_file_boundary"
	// CodeMachineFileBinary indicates a binary file where raw text preview/tail is refused.
	CodeMachineFileBinary = "machine_file_binary"
)

// MachineFileRoot describes a quick root shortcut surfaced in the Web file browser.
type MachineFileRoot struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Kind   string `json:"kind"` // home, workspace, config
	Exists bool   `json:"exists"`
}

// MachineFileRootsResponse lists quick root shortcuts for a machine.
type MachineFileRootsResponse struct {
	Home  string            `json:"home"`
	OS    string            `json:"os"`
	Roots []MachineFileRoot `json:"roots"`
}

// MachineFileListRequest requests a strictly single-level directory listing.
type MachineFileListRequest struct {
	Path  string `json:"path"`
	Root  string `json:"root,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// MachineFileEntry describes one entry in a single-level directory listing.
type MachineFileEntry struct {
	Name            string    `json:"name"`
	Path            string    `json:"path"`
	Kind            string    `json:"kind"` // dir, file, symlink, other
	Size            int64     `json:"size"`
	Mode            string    `json:"mode"`
	Perm            uint32    `json:"perm"`
	ModTime         time.Time `json:"mtime"`
	Symlink         bool      `json:"symlink,omitempty"`
	LinkTarget      string    `json:"link_target,omitempty"`
	LinkBroken      bool      `json:"link_broken,omitempty"`
	TargetKind      string    `json:"target_kind,omitempty"` // dir, file, other when symlink resolves safely
	Sensitive       bool      `json:"sensitive,omitempty"`
	SensitiveReason string    `json:"sensitive_reason,omitempty"`
	Binary          bool      `json:"binary,omitempty"`
}

// MachineFileListResponse returns a single-level directory listing with canonical path metadata.
type MachineFileListResponse struct {
	Path          string             `json:"path"`
	CanonicalPath string             `json:"canonical_path"`
	Parent        string             `json:"parent,omitempty"`
	Entries       []MachineFileEntry `json:"entries"`
	Total         int                `json:"total"`
	Truncated     bool               `json:"truncated,omitempty"`
}

// MachineFilePreviewRequest requests a bounded text preview of a single regular file.
type MachineFilePreviewRequest struct {
	Path     string `json:"path"`
	Root     string `json:"root,omitempty"`
	MaxBytes int    `json:"max_bytes,omitempty"`
}

// MachineFilePreviewResponse returns bounded UTF-8 text or binary refusal metadata.
type MachineFilePreviewResponse struct {
	Path            string    `json:"path"`
	CanonicalPath   string    `json:"canonical_path"`
	Name            string    `json:"name"`
	Size            int64     `json:"size"`
	Mode            string    `json:"mode"`
	Perm            uint32    `json:"perm"`
	ModTime         time.Time `json:"mtime"`
	Mime            string    `json:"mime"`
	Revision        string    `json:"revision"`
	Symlink         bool      `json:"symlink,omitempty"`
	LinkTarget      string    `json:"link_target,omitempty"`
	Sensitive       bool      `json:"sensitive,omitempty"`
	SensitiveReason string    `json:"sensitive_reason,omitempty"`
	Binary          bool      `json:"binary,omitempty"`
	Text            string    `json:"text,omitempty"`
	BytesRead       int64     `json:"bytes_read"`
	Truncated       bool      `json:"truncated,omitempty"`
	MaxBytes        int       `json:"max_bytes"`
}

// MachineFileTailRequest requests the last N lines (bounded by MaxBytes) of a text/log file.
type MachineFileTailRequest struct {
	Path     string `json:"path"`
	Root     string `json:"root,omitempty"`
	Lines    int    `json:"lines,omitempty"`
	MaxBytes int    `json:"max_bytes,omitempty"`
}

// MachineFileTailResponse returns the bounded trailing lines of a file.
type MachineFileTailResponse struct {
	Path            string    `json:"path"`
	CanonicalPath   string    `json:"canonical_path"`
	Name            string    `json:"name"`
	Size            int64     `json:"size"`
	ModTime         time.Time `json:"mtime"`
	Mime            string    `json:"mime"`
	Revision        string    `json:"revision"`
	Sensitive       bool      `json:"sensitive,omitempty"`
	SensitiveReason string    `json:"sensitive_reason,omitempty"`
	Binary          bool      `json:"binary,omitempty"`
	Text            string    `json:"text,omitempty"`
	Lines           int       `json:"lines"`
	RequestedLines  int       `json:"requested_lines"`
	BytesRead       int64     `json:"bytes_read"`
	Offset          int64     `json:"offset"`
	Truncated       bool      `json:"truncated,omitempty"`
}

// MachineFileChunkRequest requests a bounded byte chunk for streaming single-file downloads.
type MachineFileChunkRequest struct {
	Path     string `json:"path"`
	Root     string `json:"root,omitempty"`
	Offset   int64  `json:"offset"`
	Length   int    `json:"length"`
	Revision string `json:"revision,omitempty"`
}

// MachineFileChunkResponse returns file metadata and (when Length > 0) one chunk of bytes.
type MachineFileChunkResponse struct {
	Path          string    `json:"path"`
	CanonicalPath string    `json:"canonical_path"`
	Name          string    `json:"name"`
	Size          int64     `json:"size"`
	Mime          string    `json:"mime"`
	ModTime       time.Time `json:"mtime"`
	Revision      string    `json:"revision"`
	Offset        int64     `json:"offset"`
	Data          []byte    `json:"data,omitempty"`
}
