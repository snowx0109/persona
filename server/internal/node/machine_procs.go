package node

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/lazyre"
	"github.com/nwverse/persona/server/internal/proto"
)

const (
	defaultProcessLimit = 100
	maxProcessLimit     = 300
	maxCmdlineLen       = 512
	staleTestUptimeSecs = 30 * 60 // 30 minutes
)

// machineProcsOps holds OS hooks so unit tests can exercise RPC validation,
// protection rules, orphan heuristics, sorting, and kill guards deterministically.
type machineProcsOps struct {
	now           func() time.Time
	currentUID    func() int
	currentPID    func() int
	currentUser   func() string
	listPorts     func(ctx context.Context, protocol string) ([]proto.ListeningPort, bool, string, error)
	listProcesses func(ctx context.Context) ([]rawProcess, int64, bool, string, error)
	sendSignal    func(pid int, sig string) error
}

type rawProcess struct {
	PID           int
	PPID          int
	UID           int
	User          string
	Name          string
	Args          []string
	CPUPercent    float64
	RSSBytes      int64
	UptimeSeconds int64
	State         string
}

var defaultMachineProcsOps = machineProcsOps{
	now:           time.Now,
	currentUID:    os.Getuid,
	currentPID:    os.Getpid,
	currentUser:   lookupCurrentUser,
	listPorts:     osListListeningPorts,
	listProcesses: osListProcesses,
	sendSignal:    osSendProcessSignal,
}

func (n *Node) procsOps() machineProcsOps {
	if n != nil && n.machineProcs != nil {
		return *n.machineProcs
	}
	return defaultMachineProcsOps
}

func lookupCurrentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return strconv.Itoa(os.Getuid())
}

// machinePorts handles the read-only machine.ports Node RPC.
func (n *Node) machinePorts(ctx context.Context, params json.RawMessage) (any, error) {
	var p proto.MachinePortsParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, errors.New("bad machine.ports params")
		}
	}
	protocol := strings.ToLower(strings.TrimSpace(p.Protocol))
	switch protocol {
	case "", "tcp", "udp":
	default:
		return nil, fmt.Errorf("invalid protocol %q: expected tcp or udp", p.Protocol)
	}
	ops := n.procsOps()
	now := ops.now().Unix()
	ports, supported, unsupported, err := ops.listPorts(ctx, protocol)
	if err != nil {
		return nil, err
	}
	if !supported {
		return proto.MachinePortsResult{
			At:          now,
			Supported:   false,
			Unsupported: unsupported,
			Ports:       []proto.ListeningPort{},
		}, nil
	}
	ports = normalizeListeningPorts(ports, protocol)
	return proto.MachinePortsResult{
		At:        now,
		Supported: true,
		Ports:     ports,
	}, nil
}

// machineProcesses handles the read-only machine.processes Node RPC.
func (n *Node) machineProcesses(ctx context.Context, params json.RawMessage) (any, error) {
	var p proto.MachineProcessesParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, errors.New("bad machine.processes params")
		}
	}
	sortBy := strings.ToLower(strings.TrimSpace(p.Sort))
	switch sortBy {
	case "":
		sortBy = "cpu"
	case "cpu", "memory", "uptime":
	default:
		return nil, fmt.Errorf("invalid sort %q: expected cpu, memory, or uptime", p.Sort)
	}
	limit := p.Limit
	if limit <= 0 {
		limit = defaultProcessLimit
	}
	if limit > maxProcessLimit {
		limit = maxProcessLimit
	}

	ops := n.procsOps()
	now := ops.now().Unix()
	raw, memTotal, supported, unsupported, err := ops.listProcesses(ctx)
	if err != nil {
		return nil, err
	}
	if !supported {
		return proto.MachineProcessesResult{
			At:          now,
			Supported:   false,
			Unsupported: unsupported,
			CurrentUser: ops.currentUser(),
			Processes:   []proto.MachineProcess{},
		}, nil
	}

	curUID := ops.currentUID()
	curPID := ops.currentPID()
	byPID := make(map[int]rawProcess, len(raw))
	for _, rp := range raw {
		byPID[rp.PID] = rp
	}
	ancestors := ancestorPIDSet(curPID, byPID)

	all := make([]proto.MachineProcess, 0, len(raw))
	orphanCount := 0
	for _, rp := range raw {
		cmdline := redactCmdline(rp.Args)
		if cmdline == "" {
			cmdline = rp.Name
		}
		owned := curUID >= 0 && rp.UID == curUID
		protReason := protectedProcessReason(rp, curPID, ancestors)
		mp := proto.MachineProcess{
			PID:             rp.PID,
			PPID:            rp.PPID,
			UID:             rp.UID,
			User:            rp.User,
			Name:            rp.Name,
			Cmdline:         cmdline,
			CPUPercent:      math.Round(rp.CPUPercent*10) / 10,
			RSSBytes:        max(rp.RSSBytes, 0),
			UptimeSeconds:   max(rp.UptimeSeconds, 0),
			State:           rp.State,
			Owned:           owned,
			Protected:       protReason != "",
			ProtectedReason: protReason,
		}
		if reasons := detectOrphanReasons(mp, byPID); len(reasons) > 0 {
			mp.LikelyOrphan = true
			mp.OrphanReasons = reasons
			orphanCount++
		}
		all = append(all, mp)
	}

	query := strings.ToLower(strings.TrimSpace(p.Query))
	filtered := make([]proto.MachineProcess, 0, len(all))
	for _, mp := range all {
		if p.OrphanOnly && !mp.LikelyOrphan {
			continue
		}
		if query != "" && !matchesProcessQuery(mp, query) {
			continue
		}
		filtered = append(filtered, mp)
	}

	sortMachineProcesses(filtered, sortBy)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return proto.MachineProcessesResult{
		At:          now,
		Supported:   true,
		CurrentUser: ops.currentUser(),
		MemTotal:    memTotal,
		TotalCount:  len(all),
		OrphanCount: orphanCount,
		Processes:   filtered,
	}, nil
}

// machineProcessKill handles machine.process.kill with strict ownership and
// Persona self-protection guardrails.
func (n *Node) machineProcessKill(ctx context.Context, params json.RawMessage) (any, error) {
	var p proto.MachineProcessKillParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.New("bad machine.process.kill params")
	}
	sig := strings.ToUpper(strings.TrimSpace(p.Signal))
	switch sig {
	case "SIGTERM", "SIGKILL":
	default:
		return nil, refusal(proto.CodeInvalidSignal, "signal must be SIGTERM or SIGKILL", map[string]any{"signal": p.Signal})
	}
	if p.PID <= 1 {
		return nil, refusal(proto.CodeProcessProtected, "refusing to signal system or invalid PID", map[string]any{"pid": p.PID, "reason": "system_pid"})
	}

	ops := n.procsOps()
	raw, _, supported, unsupported, err := ops.listProcesses(ctx)
	if err != nil {
		return nil, err
	}
	if !supported {
		return nil, refusal(proto.CodeProcessUnsupported, unsupported, nil)
	}

	byPID := make(map[int]rawProcess, len(raw))
	for _, rp := range raw {
		byPID[rp.PID] = rp
	}
	target, ok := byPID[p.PID]
	if !ok {
		return nil, refusal(proto.CodeProcessNotFound, fmt.Sprintf("process %d was not found", p.PID), map[string]any{"pid": p.PID})
	}

	curUID := ops.currentUID()
	if curUID < 0 || target.UID != curUID {
		return nil, refusal(proto.CodeProcessNotOwned, fmt.Sprintf("process %d is owned by another user", p.PID), map[string]any{"pid": p.PID})
	}

	curPID := ops.currentPID()
	ancestors := ancestorPIDSet(curPID, byPID)
	if reason := protectedProcessReason(target, curPID, ancestors); reason != "" {
		return nil, refusal(proto.CodeProcessProtected, fmt.Sprintf("process %d is protected (%s)", p.PID, reason), map[string]any{"pid": p.PID, "reason": reason})
	}

	if err := ops.sendSignal(p.PID, sig); err != nil {
		if errors.Is(err, os.ErrProcessDone) || strings.Contains(strings.ToLower(err.Error()), "no such process") {
			return nil, refusal(proto.CodeProcessNotFound, fmt.Sprintf("process %d has already exited", p.PID), map[string]any{"pid": p.PID})
		}
		if errors.Is(err, os.ErrPermission) {
			return nil, refusal(proto.CodeProcessNotOwned, fmt.Sprintf("permission denied signaling process %d", p.PID), map[string]any{"pid": p.PID})
		}
		return nil, err
	}
	return proto.MachineProcessKillResult{
		PID:    p.PID,
		Signal: sig,
		Sent:   true,
	}, nil
}

func normalizeListeningPorts(ports []proto.ListeningPort, protocolFilter string) []proto.ListeningPort {
	out := make([]proto.ListeningPort, 0, len(ports))
	seen := make(map[string]bool, len(ports))
	for _, p := range ports {
		if p.Port <= 0 || p.Port > 65535 {
			continue
		}
		p.Protocol = strings.ToLower(strings.TrimSpace(p.Protocol))
		if protocolFilter == "tcp" && !strings.HasPrefix(p.Protocol, "tcp") {
			continue
		}
		if protocolFilter == "udp" && !strings.HasPrefix(p.Protocol, "udp") {
			continue
		}
		p.BindAddr = normalizeBindAddr(p.BindAddr)
		p.Scope = classifyPortScope(p.BindAddr)
		key := fmt.Sprintf("%s|%s|%d|%d", p.Protocol, p.BindAddr, p.Port, p.PID)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b proto.ListeningPort) int {
		return cmp.Or(
			cmp.Compare(a.Port, b.Port),
			strings.Compare(a.Protocol, b.Protocol),
			strings.Compare(a.BindAddr, b.BindAddr),
			cmp.Compare(a.PID, b.PID),
		)
	})
	return out
}

func normalizeBindAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimPrefix(addr, "[")
	addr = strings.TrimSuffix(addr, "]")
	if addr == "" || addr == "*" {
		return "0.0.0.0"
	}
	if ip, err := netip.ParseAddr(addr); err == nil {
		return ip.Unmap().String()
	}
	return addr
}

func classifyPortScope(addr string) string {
	if addr == "" || addr == "*" {
		return proto.PortScopeAll
	}
	if ip, err := netip.ParseAddr(addr); err == nil {
		ip = ip.Unmap()
		if ip.IsLoopback() {
			return proto.PortScopeLoopback
		}
		if ip.IsUnspecified() {
			return proto.PortScopeAll
		}
		return proto.PortScopeInterface
	}
	return proto.PortScopeInterface
}

func matchesProcessQuery(p proto.MachineProcess, q string) bool {
	return strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Cmdline), q) ||
		strings.Contains(strings.ToLower(p.User), q) ||
		strings.Contains(strconv.Itoa(p.PID), q)
}

func sortMachineProcesses(list []proto.MachineProcess, sortBy string) {
	slices.SortFunc(list, func(a, b proto.MachineProcess) int {
		switch sortBy {
		case "memory":
			return cmp.Or(cmp.Compare(b.RSSBytes, a.RSSBytes), cmp.Compare(b.CPUPercent, a.CPUPercent), cmp.Compare(a.PID, b.PID))
		case "uptime":
			return cmp.Or(cmp.Compare(b.UptimeSeconds, a.UptimeSeconds), cmp.Compare(b.RSSBytes, a.RSSBytes), cmp.Compare(a.PID, b.PID))
		default: // cpu
			return cmp.Or(cmp.Compare(b.CPUPercent, a.CPUPercent), cmp.Compare(b.RSSBytes, a.RSSBytes), cmp.Compare(a.PID, b.PID))
		}
	})
}

func ancestorPIDSet(pid int, byPID map[int]rawProcess) map[int]bool {
	out := map[int]bool{}
	cur := pid
	for range 64 {
		rp, ok := byPID[cur]
		if !ok || rp.PPID <= 1 || rp.PPID == cur || out[rp.PPID] {
			break
		}
		out[rp.PPID] = true
		cur = rp.PPID
	}
	return out
}

// protectedProcessReason returns a non-empty reason if terminating rp could disrupt
// the active Persona node/hub/link runtime or system init.
func protectedProcessReason(rp rawProcess, currentPID int, ancestors map[int]bool) string {
	if rp.PID <= 1 {
		return "system_pid"
	}
	if rp.PID == currentPID {
		return "persona_node"
	}
	if ancestors[rp.PID] {
		return "persona_ancestor"
	}
	base := strings.ToLower(strings.TrimPrefix(path.Base(rp.Name), "-"))
	switch base {
	case "personad", "persona-node", "persona-link", "persona-hub":
		if strings.HasSuffix(base, ".test") {
			return ""
		}
		// Classify hub vs node when cmdline reveals the subcommand.
		lowerCmd := strings.ToLower(strings.Join(rp.Args, " "))
		if strings.Contains(lowerCmd, " hub") || base == "persona-hub" {
			return "persona_hub"
		}
		return "persona_node"
	}
	if len(rp.Args) > 0 {
		arg0 := strings.ToLower(path.Base(rp.Args[0]))
		switch arg0 {
		case "personad", "persona-node", "persona-link", "persona-hub":
			lowerCmd := strings.ToLower(strings.Join(rp.Args, " "))
			if strings.Contains(lowerCmd, " hub") || arg0 == "persona-hub" {
				return "persona_hub"
			}
			return "persona_node"
		}
	}
	return ""
}

// detectOrphanReasons flags likely stale/orphan test or temporary processes without
// falsely flagging normal user services or protected Persona processes.
func detectOrphanReasons(p proto.MachineProcess, byPID map[int]rawProcess) []string {
	if !p.Owned || p.Protected {
		return nil
	}
	var reasons []string
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(p.State)), "Z") {
		reasons = append(reasons, "zombie")
	}

	lowerName := strings.ToLower(p.Name)
	lowerCmd := strings.ToLower(p.Cmdline)
	reparented := p.PPID == 1 || (p.PPID > 1 && !hasPID(byPID, p.PPID))
	isTest := isLikelyTestProcess(lowerName, lowerCmd)
	isTemp := isTempBinary(lowerCmd)

	if reparented && isTest {
		reasons = append(reasons, "reparented_to_init", "test_process")
	} else if reparented && isTemp {
		reasons = append(reasons, "reparented_to_init", "temp_binary")
	} else if isTest && p.UptimeSeconds >= staleTestUptimeSecs {
		reasons = append(reasons, "long_running_test")
	}
	return reasons
}

func hasPID(byPID map[int]rawProcess, pid int) bool {
	_, ok := byPID[pid]
	return ok
}

var testBinaryRe = lazyre.New(`(^|/)[^/\s]+\.test(\s|$)`)

func isLikelyTestProcess(lowerName, lowerCmd string) bool {
	if strings.HasSuffix(lowerName, ".test") || testBinaryRe.MatchString(lowerCmd) {
		return true
	}
	if strings.Contains(lowerCmd, "go-build") && strings.Contains(lowerCmd, ".test") {
		return true
	}
	for _, marker := range []string{
		"go test",
		"vitest",
		"jest",
		"pytest",
		"playwright",
		"cypress",
		"mocha",
		"cargo test",
		"xcodebuild test",
		"chromedriver",
		"geckodriver",
		"headless_shell",
	} {
		if strings.Contains(lowerCmd, marker) || strings.Contains(lowerName, marker) {
			return true
		}
	}
	// Detached test tmux servers using temporary sockets (e.g. tmux -L test-123 or -S /tmp/...).
	if (lowerName == "tmux" || strings.HasPrefix(lowerCmd, "tmux ")) &&
		(strings.Contains(lowerCmd, "test") || strings.Contains(lowerCmd, "/tmp/") || strings.Contains(lowerCmd, "/var/folders/")) {
		return true
	}
	return false
}

func isTempBinary(lowerCmd string) bool {
	fields := strings.Fields(lowerCmd)
	if len(fields) == 0 {
		return false
	}
	exe := fields[0]
	return strings.HasPrefix(exe, "/tmp/") ||
		strings.HasPrefix(exe, "/var/tmp/") ||
		strings.HasPrefix(exe, "/var/folders/") ||
		strings.Contains(exe, "/go-build")
}

// ── Command-line secret redaction ────────────────────────────────────────────

const redactedPlaceholder = "[REDACTED]"

var (
	secretTokenRe = lazyre.New(`(?i)\b(sk-(?:ant-|proj-)?[a-z0-9_-]{12,}|gh[pousr]_[a-z0-9]{16,}|github_pat_[a-z0-9_]{20,}|glpat-[a-z0-9_-]{16,}|xox[baprs]-[a-z0-9-]{12,}|AKIA[0-9A-Z]{16}|eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})`)
	bearerRe      = lazyre.New(`(?i)\b(Bearer|Basic)\s+\S+`)
	urlCredsRe    = lazyre.New(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^/\s:@]+):([^/\s@]+)@`)
	queryParamRe  = lazyre.New(`([?&])([^=&\s]+)=([^&#\s]*)`)
)

// sensitiveKeyName reports whether an env var or CLI flag name likely holds a secret.
func sensitiveKeyName(name string) bool {
	k := strings.ToLower(strings.TrimLeft(strings.TrimSpace(name), "-"))
	k = strings.ReplaceAll(k, "-", "_")
	if k == "" {
		return false
	}
	if k == "pwd" || k == "key" || k == "secret" || k == "token" || k == "pass" || k == "password" || k == "passwd" || k == "auth" || k == "cookie" {
		return true
	}
	for _, sub := range []string{
		"token", "secret", "passw", "apikey", "api_key", "auth",
		"cookie", "cred", "private_key", "access_key", "signing_key",
		"master_key", "encryption_key", "session_key", "bearer", "jwt",
		"webhook", "dsn", "client_key", "pat",
	} {
		if strings.Contains(k, sub) {
			return true
		}
	}
	return false
}

// sensitiveNextValueFlag reports whether a standalone flag expects a secret in the next argv element.
func sensitiveNextValueFlag(arg string) bool {
	if !strings.HasPrefix(arg, "-") || strings.Contains(arg, "=") {
		return false
	}
	// Avoid false positives on common short flags like -p (port) or -k (insecure).
	if len(arg) == 2 {
		return false
	}
	return sensitiveKeyName(arg)
}

func redactCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := make([]string, 0, len(args))
	redactNext := false
	headerNext := false
	userPassNext := false
	for _, raw := range args {
		if raw == "" {
			continue
		}
		if redactNext {
			out = append(out, redactedPlaceholder)
			redactNext = false
			continue
		}
		if headerNext {
			out = append(out, redactHeaderArg(raw))
			headerNext = false
			continue
		}
		if userPassNext {
			out = append(out, redactUserPassArg(raw))
			userPassNext = false
			continue
		}
		lower := strings.ToLower(raw)
		if lower == "-h" || lower == "--header" {
			out = append(out, raw)
			headerNext = true
			continue
		}
		if lower == "-u" || lower == "--user" || lower == "--proxy-user" {
			out = append(out, raw)
			userPassNext = true
			continue
		}
		if sensitiveNextValueFlag(raw) {
			out = append(out, raw)
			redactNext = true
			continue
		}
		out = append(out, redactSingleArg(raw))
	}
	joined := strings.Join(out, " ")
	if len(joined) > maxCmdlineLen {
		return joined[:maxCmdlineLen-1] + "…"
	}
	return joined
}

func redactHeaderArg(v string) string {
	idx := strings.IndexByte(v, ':')
	if idx <= 0 {
		return redactSingleArg(v)
	}
	name := strings.TrimSpace(v[:idx])
	if sensitiveKeyName(name) || strings.HasPrefix(strings.ToLower(name), "x-api-") || strings.HasPrefix(strings.ToLower(name), "x-auth") {
		return name + ": " + redactedPlaceholder
	}
	return redactSingleArg(v)
}

func redactUserPassArg(v string) string {
	if idx := strings.IndexByte(v, ':'); idx >= 0 {
		return v[:idx+1] + redactedPlaceholder
	}
	return redactSingleArg(v)
}

func redactSingleArg(arg string) string {
	// Flag or env assignment: --token=abc, API_KEY=abc, --header=Authorization: Bearer xyz.
	// Do not split standalone URLs whose query string happens to contain '='.
	if eq := strings.IndexByte(arg, '='); eq > 0 {
		lhs := arg[:eq]
		if !strings.Contains(lhs, "://") && !strings.ContainsAny(lhs, "?/") {
			rhs := arg[eq+1:]
			lowerLHS := strings.ToLower(lhs)
			if lowerLHS == "--header" || lowerLHS == "-h" {
				return lhs + "=" + redactHeaderArg(rhs)
			}
			if lowerLHS == "--user" || lowerLHS == "--proxy-user" {
				return lhs + "=" + redactUserPassArg(rhs)
			}
			if sensitiveKeyName(lhs) {
				return lhs + "=" + redactedPlaceholder
			}
			// Still inspect RHS for URL credentials or embedded secrets.
			return lhs + "=" + redactFreeText(rhs)
		}
	}
	return redactFreeText(arg)
}

func redactFreeText(s string) string {
	// Redact URL credentials and sensitive query parameters.
	if strings.Contains(s, "://") {
		s = urlCredsRe.ReplaceAllString(s, "${1}${2}:"+redactedPlaceholder+"@")
		if strings.Contains(s, "?") {
			s = queryParamRe.ReplaceAllStringFunc(s, func(match string) string {
				parts := queryParamRe.FindStringSubmatch(match)
				if len(parts) == 4 && sensitiveKeyName(parts[2]) {
					return parts[1] + parts[2] + "=" + redactedPlaceholder
				}
				return match
			})
		}
	}
	s = bearerRe.ReplaceAllString(s, "${1} "+redactedPlaceholder)
	s = secretTokenRe.ReplaceAllString(s, redactedPlaceholder)
	return s
}

// ── Cross-platform pure parsers (tested on every OS) ─────────────────────────

type procNetEntry struct {
	Protocol string
	BindAddr string
	Port     int
	UID      int
	Inode    string
}

// parseMachineProcNet parses Linux /proc/net/{tcp,tcp6,udp,udp6} for listening sockets.
// TCP listens in state 0A (TCP_LISTEN). UDP listens when state is 07 (TCP_CLOSE)
// and remote port is 0000 with a non-zero local port.
func parseMachineProcNet(b []byte, protocol string) []procNetEntry {
	isTCP := strings.HasPrefix(protocol, "tcp")
	isIPv6 := strings.HasSuffix(protocol, "6")
	var out []procNetEntry
	sc := bufio.NewScanner(bytes.NewReader(b))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			first = false
			continue // header
		}
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		state := strings.ToUpper(f[3])
		if isTCP {
			if state != "0A" {
				continue
			}
		} else {
			// Bound/listening UDP socket: state 07 and remote port 0000.
			if state != "07" || !strings.HasSuffix(f[2], ":0000") {
				continue
			}
		}
		addr, port, ok := parseProcNetAddr(f[1], isIPv6)
		if !ok || port <= 0 {
			continue
		}
		uid, _ := strconv.Atoi(f[7])
		inode := f[9]
		out = append(out, procNetEntry{
			Protocol: protocol,
			BindAddr: addr,
			Port:     port,
			UID:      uid,
			Inode:    inode,
		})
	}
	return out
}

func parseProcNetAddr(hexAddrPort string, ipv6 bool) (string, int, bool) {
	hexIP, hexPort, ok := strings.Cut(hexAddrPort, ":")
	if !ok {
		return "", 0, false
	}
	portVal, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return "", 0, false
	}
	if !ipv6 {
		if len(hexIP) != 8 {
			return "", 0, false
		}
		v, err := strconv.ParseUint(hexIP, 16, 32)
		if err != nil {
			return "", 0, false
		}
		var ip [4]byte
		binary.NativeEndian.PutUint32(ip[:], uint32(v))
		return net.IP(ip[:]).String(), int(portVal), true
	}
	if len(hexIP) != 32 {
		return "", 0, false
	}
	var ip [16]byte
	for i := range 4 {
		v, err := strconv.ParseUint(hexIP[i*8:(i+1)*8], 16, 32)
		if err != nil {
			return "", 0, false
		}
		binary.NativeEndian.PutUint32(ip[i*4:(i+1)*4], uint32(v))
	}
	return netip.AddrFrom16(ip).Unmap().String(), int(portVal), true
}

// linuxProcStat is the parsed subset of /proc/<pid>/stat used for process inspection.
type linuxProcStat struct {
	Name       string
	State      string
	PPID       int
	Ticks      uint64
	StartTicks uint64
	RSSPages   int64
}

func parseLinuxProcStat(b []byte) (linuxProcStat, bool) {
	open, end := bytes.IndexByte(b, '('), bytes.LastIndexByte(b, ')')
	if open < 0 || end < open {
		return linuxProcStat{}, false
	}
	f := strings.Fields(string(b[end+1:]))
	if len(f) < 22 {
		return linuxProcStat{}, false
	}
	ppid, err0 := strconv.Atoi(f[1])
	utime, err1 := strconv.ParseUint(f[11], 10, 64)
	stime, err2 := strconv.ParseUint(f[12], 10, 64)
	startTicks, err3 := strconv.ParseUint(f[19], 10, 64)
	rss, err4 := strconv.ParseInt(f[21], 10, 64)
	if err0 != nil || err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return linuxProcStat{}, false
	}
	return linuxProcStat{
		Name:       string(b[open+1 : end]),
		State:      f[0],
		PPID:       ppid,
		Ticks:      utime + stime,
		StartTicks: startTicks,
		RSSPages:   max(rss, 0),
	}, true
}

// parseProcStatusUID extracts the real UID from Linux /proc/<pid>/status.
func parseProcStatusUID(b []byte) (int, bool) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if rest, ok := strings.CutPrefix(line, "Uid:"); ok {
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				if uid, err := strconv.Atoi(fields[0]); err == nil {
					return uid, true
				}
			}
			return 0, false
		}
	}
	return 0, false
}

// parseLsofPorts parses `lsof -nP -F pcuPtTn` output (macOS) into listening ports.
func parseLsofPorts(b []byte, cmdByPID map[int]string, userLookup func(uid int) string) []proto.ListeningPort {
	var out []proto.ListeningPort
	var curPID, curUID int
	var curCmd string
	var curType, curProto, curName, curState string

	flushFD := func() {
		defer func() { curType, curProto, curName, curState = "", "", "", "" }()
		if curName == "" || curProto == "" {
			return
		}
		protoLower := strings.ToLower(curProto)
		if protoLower == "tcp" {
			if curState != "" && !strings.EqualFold(curState, "LISTEN") {
				return
			}
		} else if protoLower == "udp" {
			// Connected UDP entries contain "->" (peer endpoint); keep unbound listeners.
			if strings.Contains(curName, "->") {
				return
			}
		} else {
			return
		}
		bind, port, ok := splitHostPortLoose(curName)
		if !ok || port <= 0 {
			return
		}
		fullProto := protoLower
		if strings.EqualFold(curType, "IPv6") {
			fullProto += "6"
		}
		if bind == "*" {
			if fullProto == "tcp6" || fullProto == "udp6" {
				bind = "::"
			} else {
				bind = "0.0.0.0"
			}
		}
		username := ""
		if curUID >= 0 && userLookup != nil {
			username = userLookup(curUID)
		}
		out = append(out, proto.ListeningPort{
			Protocol: fullProto,
			BindAddr: bind,
			Port:     port,
			Scope:    classifyPortScope(bind),
			PID:      curPID,
			User:     username,
			Process:  curCmd,
			Cmdline:  cmdByPID[curPID],
		})
	}

	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64<<10), 2<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		tag, val := line[0], line[1:]
		switch tag {
		case 'p':
			flushFD()
			curPID, _ = strconv.Atoi(val)
			curUID = -1
			curCmd = ""
		case 'c':
			curCmd = val
		case 'u':
			curUID, _ = strconv.Atoi(val)
		case 'f':
			flushFD()
		case 't':
			curType = val
		case 'P':
			curProto = val
		case 'n':
			curName = val
		case 'T':
			if st, ok := strings.CutPrefix(val, "ST="); ok {
				curState = st
			}
		}
	}
	flushFD()
	return out
}

func splitHostPortLoose(s string) (string, int, bool) {
	s = strings.TrimSpace(s)
	// Strip lsof annotations such as " (LISTEN)".
	if idx := strings.IndexByte(s, ' '); idx > 0 {
		s = s[:idx]
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		idx := strings.LastIndexByte(s, ':')
		if idx < 0 {
			return "", 0, false
		}
		host, portStr = s[:idx], s[idx+1:]
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	return host, port, true
}

// parseDarwinPS parses `ps -axo pid=,ppid=,uid=,state=,rss=,pcpu=,etime=,args=`.
func parseDarwinPS(b []byte, userLookup func(uid int) string) []rawProcess {
	var out []rawProcess
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields, rest := takeFields(line, 7)
		if len(fields) < 7 {
			continue
		}
		pid, err0 := strconv.Atoi(fields[0])
		ppid, err1 := strconv.Atoi(fields[1])
		uid, err2 := strconv.Atoi(fields[2])
		state := fields[3]
		rssKiB, err3 := strconv.ParseInt(fields[4], 10, 64)
		cpuPct, err4 := strconv.ParseFloat(strings.Replace(fields[5], ",", ".", 1), 64)
		uptime := parsePSElapsed(fields[6])
		if err0 != nil || err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		args := strings.Fields(rest)
		name := ""
		if len(args) > 0 {
			name = strings.TrimPrefix(path.Base(args[0]), "-")
		}
		if name == "" {
			continue
		}
		username := strconv.Itoa(uid)
		if userLookup != nil {
			if u := userLookup(uid); u != "" {
				username = u
			}
		}
		out = append(out, rawProcess{
			PID:           pid,
			PPID:          ppid,
			UID:           uid,
			User:          username,
			Name:          name,
			Args:          args,
			CPUPercent:    cpuPct,
			RSSBytes:      max(rssKiB, 0) << 10,
			UptimeSeconds: uptime,
			State:         state,
		})
	}
	return out
}

func takeFields(s string, n int) ([]string, string) {
	out := make([]string, 0, n)
	rest := strings.TrimSpace(s)
	for range n {
		if rest == "" {
			break
		}
		idx := strings.IndexAny(rest, " \t")
		if idx < 0 {
			out = append(out, rest)
			rest = ""
			break
		}
		out = append(out, rest[:idx])
		rest = strings.TrimLeft(rest[idx:], " \t")
	}
	return out, rest
}

// parsePSElapsed parses BSD/POSIX ps etime format: [[dd-]hh:]mm:ss.
func parsePSElapsed(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var days int64
	if d, rest, ok := strings.Cut(s, "-"); ok {
		days, _ = strconv.ParseInt(d, 10, 64)
		s = rest
	}
	parts := strings.Split(s, ":")
	var h, m, sec int64
	switch len(parts) {
	case 2:
		m, _ = strconv.ParseInt(parts[0], 10, 64)
		sec, _ = strconv.ParseInt(parts[1], 10, 64)
	case 3:
		h, _ = strconv.ParseInt(parts[0], 10, 64)
		m, _ = strconv.ParseInt(parts[1], 10, 64)
		sec, _ = strconv.ParseInt(parts[2], 10, 64)
	default:
		return 0
	}
	return max(days*86400+h*3600+m*60+sec, 0)
}
