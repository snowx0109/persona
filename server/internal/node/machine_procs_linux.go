//go:build linux

package node

import (
	"bytes"
	"context"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nwverse/persona/server/internal/proto"
)

const (
	linuxClockTicks = 100
	linuxCPUWindow  = 250 * time.Millisecond
)

func osListListeningPorts(ctx context.Context, protocol string) ([]proto.ListeningPort, bool, string, error) {
	tables := []struct {
		proto string
		path  string
	}{
		{"tcp", "/proc/net/tcp"},
		{"tcp6", "/proc/net/tcp6"},
		{"udp", "/proc/net/udp"},
		{"udp6", "/proc/net/udp6"},
	}

	var entries []procNetEntry
	for _, tbl := range tables {
		if protocol == "tcp" && !strings.HasPrefix(tbl.proto, "tcp") {
			continue
		}
		if protocol == "udp" && !strings.HasPrefix(tbl.proto, "udp") {
			continue
		}
		b, err := os.ReadFile(tbl.path)
		if err != nil {
			continue
		}
		entries = append(entries, parseMachineProcNet(b, tbl.proto)...)
	}

	inodes := map[string]bool{}
	for _, e := range entries {
		if e.Inode != "" && e.Inode != "0" {
			inodes[e.Inode] = true
		}
	}
	inodeOwner := mapSocketInodesToPID(ctx, inodes)
	uidCache := map[int]string{}
	lookupUID := func(uid int) string {
		if s, ok := uidCache[uid]; ok {
			return s
		}
		s := strconv.Itoa(uid)
		if u, err := user.LookupId(s); err == nil && u.Username != "" {
			s = u.Username
		}
		uidCache[uid] = s
		return s
	}

	procCache := map[int]struct {
		name    string
		cmdline string
	}{}
	out := make([]proto.ListeningPort, 0, len(entries))
	for _, e := range entries {
		pid := inodeOwner[e.Inode]
		var procName, cmdline string
		if pid > 0 {
			info, ok := procCache[pid]
			if !ok {
				info.name = readLinuxProcName(pid)
				info.cmdline = redactCmdline(readLinuxProcArgs(pid))
				if info.cmdline == "" {
					info.cmdline = info.name
				}
				procCache[pid] = info
			}
			procName, cmdline = info.name, info.cmdline
		}
		out = append(out, proto.ListeningPort{
			Protocol: e.Protocol,
			BindAddr: e.BindAddr,
			Port:     e.Port,
			Scope:    classifyPortScope(e.BindAddr),
			PID:      pid,
			User:     lookupUID(e.UID),
			Process:  procName,
			Cmdline:  cmdline,
		})
	}
	return out, true, "", nil
}

func mapSocketInodesToPID(ctx context.Context, wanted map[string]bool) map[string]int {
	out := make(map[string]int, len(wanted))
	if len(wanted) == 0 {
		return out
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			break
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fds, err := os.ReadDir("/proc/" + e.Name() + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink("/proc/" + e.Name() + "/fd/" + fd.Name())
			if err != nil {
				continue
			}
			if rest, ok := strings.CutPrefix(target, "socket:["); ok {
				inode := strings.TrimSuffix(rest, "]")
				if wanted[inode] {
					if _, exists := out[inode]; !exists {
						out[inode] = pid
					}
				}
			}
		}
	}
	return out
}

func osListProcesses(ctx context.Context) ([]rawProcess, int64, bool, string, error) {
	first, err := readAllLinuxProcStats()
	if err != nil {
		return nil, 0, false, err.Error(), err
	}
	start := time.Now()
	select {
	case <-ctx.Done():
		return nil, 0, true, "", ctx.Err()
	case <-time.After(linuxCPUWindow):
	}
	second, err := readAllLinuxProcStats()
	if err != nil {
		return nil, 0, false, err.Error(), err
	}
	dt := time.Since(start)
	uptimeSecs := readLinuxSystemUptime()
	pageSize := int64(os.Getpagesize())

	uidCache := map[int]string{}
	lookupUID := func(uid int) string {
		if s, ok := uidCache[uid]; ok {
			return s
		}
		s := strconv.Itoa(uid)
		if u, err := user.LookupId(s); err == nil && u.Username != "" {
			s = u.Username
		}
		uidCache[uid] = s
		return s
	}

	out := make([]rawProcess, 0, len(second))
	for pid, st := range second {
		var cpuPct float64
		if prev, ok := first[pid]; ok && prev.Name == st.Name && st.Ticks >= prev.Ticks && dt > 0 {
			cpuPct = float64(st.Ticks-prev.Ticks) / linuxClockTicks / dt.Seconds() * 100
		}
		uid := -1
		if sb, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status"); err == nil {
			if u, ok := parseProcStatusUID(sb); ok {
				uid = u
			}
		}
		args := readLinuxProcArgs(pid)
		var procUptime int64
		if uptimeSecs > 0 {
			procUptime = max(int64(uptimeSecs)-int64(st.StartTicks/linuxClockTicks), 0)
		}
		username := ""
		if uid >= 0 {
			username = lookupUID(uid)
		}
		out = append(out, rawProcess{
			PID:           pid,
			PPID:          st.PPID,
			UID:           uid,
			User:          username,
			Name:          st.Name,
			Args:          args,
			CPUPercent:    cpuPct,
			RSSBytes:      st.RSSPages * pageSize,
			UptimeSeconds: procUptime,
			State:         st.State,
		})
	}
	return out, readLinuxMemTotal(), true, "", nil
}

func readAllLinuxProcStats() (map[int]linuxProcStat, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	out := make(map[int]linuxProcStat, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		if st, ok := parseLinuxProcStat(b); ok {
			out[pid] = st
		}
	}
	return out, nil
}

func readLinuxProcName(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readLinuxProcArgs(pid int) []string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil || len(b) == 0 {
		return nil
	}
	parts := bytes.Split(bytes.TrimRight(b, "\x00"), []byte{0})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			out = append(out, string(part))
		}
	}
	return out
}

func readLinuxSystemUptime() float64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[0], 64)
	return v
}

func readLinuxMemTotal() int64 {
	var si syscall.Sysinfo_t
	if syscall.Sysinfo(&si) != nil {
		return 0
	}
	return int64(si.Totalram) * int64(si.Unit)
}

func osSendProcessSignal(pid int, sig string) error {
	switch sig {
	case "SIGTERM":
		return syscall.Kill(pid, syscall.SIGTERM)
	case "SIGKILL":
		return syscall.Kill(pid, syscall.SIGKILL)
	default:
		return syscall.EINVAL
	}
}
