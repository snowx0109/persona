package node

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

func TestMachineFilesPathAndSymlinkBoundarySafety(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	root := filepath.Join(home, "workspace")
	siblingPrefix := filepath.Join(home, "workspace2")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{
		filepath.Join(root, "sub"),
		siblingPrefix,
		outside,
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".persona"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeFile := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(filepath.Join(root, "sub", "hello.txt"), []byte("ok\n"))
	writeFile(filepath.Join(siblingPrefix, "secret.txt"), []byte("sibling\n"))
	writeFile(filepath.Join(outside, "escape.txt"), []byte("outside\n"))
	writeFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("PRIVATE KEY"))
	writeFile(filepath.Join(home, ".persona", "hub.json"), []byte(`{"token":"nope"}`))

	if err := os.Symlink(filepath.Join(outside, "escape.txt"), filepath.Join(root, "escape-link.txt")); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "sub", "hello.txt"), filepath.Join(root, "inside-link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "loop-b"), filepath.Join(root, "loop-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "loop-a"), filepath.Join(root, "loop-b")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, ".ssh", "id_ed25519"), filepath.Join(root, "ssh-link")); err != nil {
		t.Fatal(err)
	}

	n := &Node{home: home}

	assertBoundary := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected boundary refusal, got nil", name)
		}
		var re *rpc.Error
		if !errors.As(err, &re) || re.Code != proto.CodeMachineFileBoundary {
			t.Fatalf("%s: got %v, want rpc.Error with code %q", name, err, proto.CodeMachineFileBoundary)
		}
	}

	// Prefix collision (/workspace2 vs /workspace) must be rejected.
	if isWithinCanonicalRoot(siblingPrefix, root) {
		t.Fatalf("isWithinCanonicalRoot accepted prefix-colliding sibling %q under %q", siblingPrefix, root)
	}
	if !isWithinCanonicalRoot(filepath.Join(root, "sub"), root) {
		t.Fatalf("isWithinCanonicalRoot rejected child under root")
	}

	// Control characters & NUL byte rejection.
	_, err := n.resolveMachinePath(filepath.Join(root, "bad\x00name"), root, machinePathOptions{})
	assertBoundary("nul byte", err)
	_, err = n.resolveMachinePath("relative/no-root.txt", "", machinePathOptions{})
	assertBoundary("relative without root", err)

	// Path traversal out of root via .. and sibling prefix.
	_, err = n.resolveMachinePath(filepath.Join(root, "..", "workspace2", "secret.txt"), root, machinePathOptions{})
	assertBoundary("dotdot traversal to sibling", err)
	_, err = n.resolveMachinePath(filepath.Join(siblingPrefix, "secret.txt"), root, machinePathOptions{})
	assertBoundary("sibling prefix outside root", err)

	// Symlink pointing outside root (leaf and directory ancestor).
	_, err = n.resolveMachinePath(filepath.Join(root, "escape-link.txt"), root, machinePathOptions{})
	assertBoundary("leaf symlink escape", err)
	_, err = n.resolveMachinePath(filepath.Join(root, "escape-dir", "escape.txt"), root, machinePathOptions{})
	assertBoundary("ancestor symlink escape", err)

	// Symlink loop.
	_, err = n.resolveMachinePath(filepath.Join(root, "loop-a"), root, machinePathOptions{})
	assertBoundary("symlink loop", err)

	// Off-limits secrets refused even without a workspace boundary root.
	_, err = n.resolveMachinePath("~/.ssh/id_ed25519", "", machinePathOptions{})
	assertBoundary("direct ssh key", err)
	_, err = n.resolveMachinePath("~/.persona/hub.json", "", machinePathOptions{})
	assertBoundary("direct hub.json", err)
	_, err = n.resolveMachinePath(filepath.Join(root, "ssh-link"), "", machinePathOptions{})
	assertBoundary("symlink to ssh key", err)

	// Symlink inside root pointing inside root is allowed and recorded.
	res, err := n.resolveMachinePath(filepath.Join(root, "inside-link.txt"), root, machinePathOptions{})
	if err != nil {
		t.Fatalf("inside symlink should succeed: %v", err)
	}
	if !res.Symlink || res.LinkTarget == "" {
		t.Fatalf("expected symlink metadata on inside link, got %+v", res)
	}

	// Reusable Phase-2 write helper: AllowMissingLeaf resolves missing targets under existing ancestors
	// and rejects ancestor symlink escapes or off-limits targets.
	newTarget, err := n.resolveMachinePath(filepath.Join(root, "sub", "new-dir", "created.txt"), root, machinePathOptions{AllowMissingLeaf: true})
	if err != nil {
		t.Fatalf("missing leaf inside root should succeed: %v", err)
	}
	if !isWithinCanonicalRoot(newTarget.CanonicalPath, newTarget.RootPath) {
		t.Fatalf("resolved missing leaf %q outside root %q", newTarget.CanonicalPath, newTarget.RootPath)
	}
	_, err = n.resolveMachinePath(filepath.Join(root, "escape-dir", "new-file.txt"), root, machinePathOptions{AllowMissingLeaf: true})
	assertBoundary("missing leaf through escaping symlink dir", err)
	_, err = n.resolveMachinePath("~/.persona/hub.json", "", machinePathOptions{AllowMissingLeaf: true})
	assertBoundary("missing leaf targeting hub.json", err)
}

func TestMachineFilesSingleLevelReadDirAndLabels(t *testing.T) {
	home := t.TempDir()
	ws := filepath.Join(home, "project")
	nested := filepath.Join(ws, "nested")
	if err := os.MkdirAll(filepath.Join(nested, "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep", "hidden.txt"), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".env"), []byte("API_KEY=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".env.example"), []byte("API_KEY=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "tls.key"), []byte("KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "hub.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "archive.zip"), []byte("PK\x03\x04"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "README.md"), []byte("# Hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hasSymlinks := os.Symlink(filepath.Join(ws, "README.md"), filepath.Join(ws, "readme-link")) == nil
	if hasSymlinks {
		_ = os.Symlink(filepath.Join(ws, "missing-target"), filepath.Join(ws, "broken-link"))
	}

	n := &Node{home: home}
	list, err := n.machineFileList(proto.MachineFileListRequest{Path: ws})
	if err != nil {
		t.Fatalf("machineFileList: %v", err)
	}

	// Strictly single-level: deep/hidden.txt must never appear in ws listing.
	byName := map[string]proto.MachineFileEntry{}
	for _, e := range list.Entries {
		if strings.Contains(e.Name, "deep") || strings.Contains(e.Name, "hidden.txt") {
			t.Fatalf("listing leaked nested entry %q", e.Name)
		}
		byName[e.Name] = e
	}

	// Directory sorted first.
	if len(list.Entries) == 0 || list.Entries[0].Name != "nested" || list.Entries[0].Kind != "dir" {
		t.Fatalf("expected directory 'nested' first, got %+v", list.Entries)
	}
	if !byName[".env"].Sensitive || byName[".env"].SensitiveReason != "env" {
		t.Errorf(".env should be labeled sensitive/env, got %+v", byName[".env"])
	}
	if byName[".env.example"].Sensitive {
		t.Errorf(".env.example should not be labeled sensitive, got %+v", byName[".env.example"])
	}
	if !byName["tls.key"].Sensitive || byName["tls.key"].SensitiveReason != "private_key" {
		t.Errorf("tls.key should be labeled sensitive/private_key, got %+v", byName["tls.key"])
	}
	if !byName["hub.json"].Sensitive || byName["hub.json"].SensitiveReason != "hub_config" {
		t.Errorf("hub.json should be labeled sensitive/hub_config, got %+v", byName["hub.json"])
	}
	if !byName["archive.zip"].Binary {
		t.Errorf("archive.zip should be hinted binary, got %+v", byName["archive.zip"])
	}
	if byName["README.md"].Mode == "" || byName["README.md"].ModTime.IsZero() {
		t.Errorf("README.md missing mode/mtime: %+v", byName["README.md"])
	}
	if hasSymlinks {
		if !byName["readme-link"].Symlink || byName["readme-link"].Kind != "symlink" || byName["readme-link"].TargetKind != "file" {
			t.Errorf("readme-link symlink metadata wrong: %+v", byName["readme-link"])
		}
		if !byName["broken-link"].Symlink || !byName["broken-link"].LinkBroken {
			t.Errorf("broken-link should be marked broken: %+v", byName["broken-link"])
		}
	}

	// Limit truncation check.
	limited, err := n.machineFileList(proto.MachineFileListRequest{Path: ws, Limit: 2})
	if err != nil {
		t.Fatalf("limited list: %v", err)
	}
	if len(limited.Entries) != 2 || !limited.Truncated || limited.Total != len(list.Entries) {
		t.Fatalf("expected 2 truncated entries out of %d, got len=%d truncated=%v total=%d", len(list.Entries), len(limited.Entries), limited.Truncated, limited.Total)
	}
}

func TestMachineFilesPreviewBoundsAndBinaryRefusal(t *testing.T) {
	home := t.TempDir()
	n := &Node{home: home}

	// Large text file exceeding 2 MiB hard cap.
	line := strings.Repeat("a", 1023) + "\n"
	bigData := strings.Repeat(line, 2500) // ~2.5 MiB
	bigPath := filepath.Join(home, "big.log")
	if err := os.WriteFile(bigPath, []byte(bigData), 0o644); err != nil {
		t.Fatal(err)
	}

	// Requesting above 2 MiB is clamped to hard 2 MiB cap.
	preview, err := n.machineFilePreview(proto.MachineFilePreviewRequest{
		Path:     bigPath,
		MaxBytes: 10 << 20,
	})
	if err != nil {
		t.Fatalf("preview big text: %v", err)
	}
	if preview.Binary {
		t.Fatalf("text file misclassified as binary")
	}
	if !preview.Truncated || preview.MaxBytes != proto.MachineFilePreviewMaxBytes {
		t.Fatalf("expected truncated preview capped at %d, got truncated=%v max=%d", proto.MachineFilePreviewMaxBytes, preview.Truncated, preview.MaxBytes)
	}
	if len(preview.Text) == 0 || len(preview.Text) > proto.MachineFilePreviewMaxBytes {
		t.Fatalf("preview text length out of bounds: %d", len(preview.Text))
	}
	if preview.Revision == "" {
		t.Fatalf("expected non-empty revision for Phase 2 CAS reuse")
	}

	// Binary file (NUL bytes / PNG header) refuses raw text render.
	binPath := filepath.Join(home, "blob.dat")
	binPayload := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x0d}, bytes.Repeat([]byte{0x00, 0xff, 0x01}, 64)...)
	if err := os.WriteFile(binPath, binPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	binPreview, err := n.machineFilePreview(proto.MachineFilePreviewRequest{Path: binPath})
	if err != nil {
		t.Fatalf("preview binary: %v", err)
	}
	if !binPreview.Binary || binPreview.Text != "" {
		t.Fatalf("binary preview must refuse raw text render, got binary=%v textLen=%d", binPreview.Binary, len(binPreview.Text))
	}
}

func TestMachineFilesTailLargeLog(t *testing.T) {
	home := t.TempDir()
	n := &Node{home: home}

	var b strings.Builder
	for i := 1; i <= 5000; i++ {
		fmt.Fprintf(&b, "line-%05d payload-中文-log-entry\n", i)
	}
	logPath := filepath.Join(home, "app.log")
	if err := os.WriteFile(logPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := n.machineFileTail(proto.MachineFileTailRequest{
		Path:  logPath,
		Lines: 10,
	})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if res.Binary {
		t.Fatalf("log misclassified as binary")
	}
	if res.Lines != 10 || !res.Truncated || res.Offset <= 0 {
		t.Fatalf("expected 10 truncated trailing lines, got lines=%d truncated=%v offset=%d", res.Lines, res.Truncated, res.Offset)
	}
	gotLines := strings.Split(strings.TrimSuffix(res.Text, "\n"), "\n")
	if len(gotLines) != 10 || !strings.HasPrefix(gotLines[0], "line-04991 ") || !strings.HasPrefix(gotLines[9], "line-05000 ") {
		t.Fatalf("unexpected tail lines: first=%q last=%q count=%d", gotLines[0], gotLines[len(gotLines)-1], len(gotLines))
	}

	// Byte-bounded tail must stay within MaxBytes and start cleanly on a valid UTF-8 line boundary.
	capped, err := n.machineFileTail(proto.MachineFileTailRequest{
		Path:     logPath,
		Lines:    500,
		MaxBytes: 512,
	})
	if err != nil {
		t.Fatalf("byte-capped tail: %v", err)
	}
	if capped.BytesRead > 512 || !capped.Truncated || !strings.HasPrefix(capped.Text, "line-") {
		t.Fatalf("byte-capped tail invalid: bytes=%d truncated=%v prefix=%q", capped.BytesRead, capped.Truncated, capped.Text[:min(20, len(capped.Text))])
	}

	// Binary file refused by tail too.
	binPath := filepath.Join(home, "dump.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02, 'a', 'b', '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	binTail, err := n.machineFileTail(proto.MachineFileTailRequest{Path: binPath, Lines: 5})
	if err != nil {
		t.Fatalf("tail binary: %v", err)
	}
	if !binTail.Binary || binTail.Text != "" {
		t.Fatalf("tail must refuse raw text for binary file, got %+v", binTail)
	}
}

func TestMachineFilesRootsAndChunkedDownload(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("0123456789abcdef"), 1024) // 16 KiB
	filePath := filepath.Join(home, "artifact.txt")
	if err := os.WriteFile(filePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	n := &Node{home: home}
	roots := n.machineFileRoots(context.Background())
	if roots.Home != home || len(roots.Roots) < 2 {
		t.Fatalf("expected home and .config roots, got %+v", roots)
	}
	if roots.Roots[0].Kind != "home" || roots.Roots[0].Path != home {
		t.Fatalf("expected $HOME first, got %+v", roots.Roots[0])
	}

	// Chunk metadata call (Length=0) followed by streaming reads.
	head, err := n.machineFileChunk(proto.MachineFileChunkRequest{Path: filePath})
	if err != nil {
		t.Fatalf("chunk head: %v", err)
	}
	if head.Size != int64(len(payload)) || head.Revision == "" || len(head.Data) != 0 {
		t.Fatalf("unexpected chunk head: %+v", head)
	}
	var assembled []byte
	for off := int64(0); off < head.Size; off += 4096 {
		c, err := n.machineFileChunk(proto.MachineFileChunkRequest{
			Path:     head.Path,
			Offset:   off,
			Length:   4096,
			Revision: head.Revision,
		})
		if err != nil {
			t.Fatalf("chunk at %d: %v", off, err)
		}
		assembled = append(assembled, c.Data...)
	}
	if !bytes.Equal(assembled, payload) {
		t.Fatalf("reassembled chunk payload mismatch")
	}

	// Revision mismatch returns errFileChanged.
	_, err = n.machineFileChunk(proto.MachineFileChunkRequest{
		Path:     head.Path,
		Offset:   0,
		Length:   64,
		Revision: "stale-rev",
	})
	if !errors.Is(err, errFileChanged) {
		t.Fatalf("expected errFileChanged on stale revision, got %v", err)
	}
}
