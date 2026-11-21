//go:build darwin

package node

import (
	"context"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/nwverse/persona/server/internal/proto"
)

const darwinProcCmdTimeout = 6 * time.Second

func osListListeningPorts(ctx context.Context, protocol string) ([]proto.ListeningPort, bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, darwinProcCmdTimeout)
	defer cancel()

	// Collect redacted command lines from ps so listening sockets include redacted arguments.
	cmdByPID := map[int]string{}
	if psOut, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,uid=,state=,rss=,pcpu=,etime=,args=").Output(); err == nil {
		for _, rp := range parseDarwinPS(psOut, nil) {
			cmdByPID[rp.PID] = redactCmdline(rp.Args)
		}
	}

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

	var args []string
	switch protocol {
	case "tcp":
		args = []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-F", "pcuPtTn"}
	case "udp":
		args = []string{"-nP", "-iUDP", "-F", "pcuPtTn"}
	default:
		args = []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-iUDP", "-F", "pcuPtTn"}
	}
	lsofPath := "/usr/sbin/lsof"
	if _, err := exec.LookPath(lsofPath); err != nil {
		if p, err2 := exec.LookPath("lsof"); err2 == nil {
			lsofPath = p
		} else {
			return nil, false, "lsof is not available on this computer", nil
		}
	}
	out, err := exec.CommandContext(ctx, lsofPath, args...).Output()
	if err != nil && len(out) == 0 {
		// lsof exits 1 when no sockets match; treat empty output as zero listening ports.
		var ee *exec.ExitError
		if errorsAsExit(err, &ee) && ee.ExitCode() == 1 {
			return []proto.ListeningPort{}, true, "", nil
		}
		return nil, false, "could not inspect listening ports with lsof", nil
	}
	return parseLsofPorts(out, cmdByPID, lookupUID), true, "", nil
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

func osListProcesses(ctx context.Context) ([]rawProcess, int64, bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, darwinProcCmdTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,ppid=,uid=,state=,rss=,pcpu=,etime=,args=").Output()
	if err != nil {
		return nil, 0, false, "could not inspect processes with ps", err
	}
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
	var memTotal int64
	if v, err := unix.SysctlUint64("hw.memsize"); err == nil {
		memTotal = int64(v)
	}
	return parseDarwinPS(out, lookupUID), memTotal, true, "", nil
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
