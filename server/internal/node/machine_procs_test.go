package node

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nwverse/persona/server/internal/proto"
	"github.com/nwverse/persona/server/internal/rpc"
)

func TestRedactCmdline(t *testing.T) {
	args := []string{
		"node",
		"server.js",
		"-p", "3000",
		"--port=8080",
		"--token", "my-super-secret-tok",
		"--api-key=secret-key-val",
		"OPENAI_API_KEY=sk-proj-1234567890abcdef",
		"-H", "Authorization: Bearer topsecretbearer",
		"-H", "Accept: application/json",
		"-u", "alice:s3cr3tpass",
		"postgres://dbuser:dbpass@127.0.0.1:5432/app?sslmode=disable&token=abc123",
		"ghp_abcdefghijklmnopqrstuvwxyz012345",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signaturepart12345",
	}
	got := redactCmdline(args)

	for _, leaked := range []string{
		"my-super-secret-tok",
		"secret-key-val",
		"sk-proj-1234567890abcdef",
		"topsecretbearer",
		"s3cr3tpass",
		"dbpass",
		"abc123",
		"ghp_abcdefghijklmnopqrstuvwxyz012345",
		"signaturepart12345",
	} {
		if strings.Contains(got, leaked) {
			t.Errorf("redacted cmdline leaked %q: %s", leaked, got)
		}
	}
	for _, kept := range []string{
		"node server.js",
		"-p 3000",
		"--port=8080",
		"--token [REDACTED]",
		"--api-key=[REDACTED]",
		"OPENAI_API_KEY=[REDACTED]",
		"Authorization: [REDACTED]",
		"Accept: application/json",
		"-u alice:[REDACTED]",
		"postgres://dbuser:[REDACTED]@127.0.0.1:5432/app",
		"sslmode=disable",
	} {
		if !strings.Contains(got, kept) {
			t.Errorf("redacted cmdline missing expected %q: %s", kept, got)
		}
	}
}

func TestParseProcNetAndStat(t *testing.T) {
	tcpSample := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 11111 1 0 100 0 0 10 0
   1: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 22222 1 0 100 0 0 10 0
   2: 0100007F:9999 0100007F:0050 01 00000000:00000000 00:00000000 00000000  1000        0 33333 1 0 100 0 0 10 0
`
	tcp := parseMachineProcNet([]byte(tcpSample), "tcp")
	if len(tcp) != 2 {
		t.Fatalf("expected 2 listening tcp sockets, got %d: %+v", len(tcp), tcp)
	}
	if tcp[0].BindAddr != "127.0.0.1" || tcp[0].Port != 3000 || tcp[0].Inode != "11111" {
		t.Errorf("unexpected tcp[0]: %+v", tcp[0])
	}
	if tcp[1].BindAddr != "0.0.0.0" || tcp[1].Port != 8080 || tcp[1].Inode != "22222" {
		t.Errorf("unexpected tcp[1]: %+v", tcp[1])
	}

	tcp6Sample := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:48E4 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 44444
   1: 00000000000000000000000000000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 55555
`
	tcp6 := parseMachineProcNet([]byte(tcp6Sample), "tcp6")
	if len(tcp6) != 2 {
		t.Fatalf("expected 2 listening tcp6 sockets, got %d: %+v", len(tcp6), tcp6)
	}
	if tcp6[0].BindAddr != "::1" || tcp6[0].Port != 18660 {
		t.Errorf("unexpected tcp6[0]: %+v", tcp6[0])
	}
	if tcp6[1].BindAddr != "::" || tcp6[1].Port != 8080 {
		t.Errorf("unexpected tcp6[1]: %+v", tcp6[1])
	}

	udpSample := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
   0: 0100007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000   100        0 66666 2 0 0
   1: 0100007F:CA56 0100007F:23A3 01 00000000:00000000 00:00000000 00000000   100        0 77777 2 0 0
`
	udp := parseMachineProcNet([]byte(udpSample), "udp")
	if len(udp) != 1 || udp[0].Port != 53 || udp[0].BindAddr != "127.0.0.1" {
		t.Fatalf("unexpected udp sockets: %+v", udp)
	}

	statLine := "4242 (my (test) proc) S 1 4242 4242 0 -1 4194304 100 0 0 0 150 50 0 0 20 0 1 0 123456 1048576 512 18446744073709551615"
	st, ok := parseLinuxProcStat([]byte(statLine))
	if !ok {
		t.Fatal("parseLinuxProcStat failed")
	}
	if st.Name != "my (test) proc" || st.State != "S" || st.PPID != 1 || st.Ticks != 200 || st.StartTicks != 123456 || st.RSSPages != 512 {
		t.Fatalf("unexpected stat: %+v", st)
	}

	uid, ok := parseProcStatusUID([]byte("Name:\tnode\nUid:\t1001\t1001\t1001\t1001\n"))
	if !ok || uid != 1001 {
		t.Fatalf("parseProcStatusUID = (%d, %v), want (1001, true)", uid, ok)
	}
}

func TestParseDarwinLsofAndPS(t *testing.T) {
	lsofOut := strings.Join([]string{
		"p1234",
		"cnode",
		"u501",
		"f20",
		"tIPv4",
		"PTCP",
		"n127.0.0.1:3000",
		"TST=LISTEN",
		"f21",
		"tIPv6",
		"PTCP",
		"n*:8080",
		"TST=LISTEN",
		"f22",
		"tIPv4",
		"PTCP",
		"n127.0.0.1:50000->127.0.0.1:3000",
		"TST=ESTABLISHED",
		"f23",
		"tIPv4",
		"PUDP",
		"n*:5353",
		"f24",
		"tIPv4",
		"PUDP",
		"n127.0.0.1:5555->127.0.0.1:53",
	}, "\n")
	ports := parseLsofPorts([]byte(lsofOut), map[int]string{1234: "node dev.js --token=[REDACTED]"}, func(uid int) string {
		if uid == 501 {
			return "alice"
		}
		return ""
	})
	if len(ports) != 3 {
		t.Fatalf("expected 3 listening ports from lsof, got %d: %+v", len(ports), ports)
	}
	if ports[0].Protocol != "tcp" || ports[0].BindAddr != "127.0.0.1" || ports[0].Port != 3000 || ports[0].Scope != proto.PortScopeLoopback || ports[0].User != "alice" {
		t.Errorf("unexpected lsof port[0]: %+v", ports[0])
	}
	if ports[1].Protocol != "tcp6" || ports[1].BindAddr != "::" || ports[1].Port != 8080 || ports[1].Scope != proto.PortScopeAll {
		t.Errorf("unexpected lsof port[1]: %+v", ports[1])
	}
	if ports[2].Protocol != "udp" || ports[2].BindAddr != "0.0.0.0" || ports[2].Port != 5353 {
		t.Errorf("unexpected lsof port[2]: %+v", ports[2])
	}

	psOut := "  1234     1   501 S    40960  12.5 01:02:03 /usr/local/bin/node /app/server.js --token=abc\n" +
		"  5678  1234   501 R   102400   0.2 2-03:04:05 /tmp/go-build123/b001/hub.test -test.v\n"
	procs := parseDarwinPS([]byte(psOut), func(int) string { return "alice" })
	if len(procs) != 2 {
		t.Fatalf("expected 2 procs from darwin ps, got %d", len(procs))
	}
	if procs[0].PID != 1234 || procs[0].PPID != 1 || procs[0].UptimeSeconds != 3723 || procs[0].RSSBytes != 40960<<10 {
		t.Errorf("unexpected darwin ps proc[0]: %+v", procs[0])
	}
	if procs[1].PID != 5678 || procs[1].UptimeSeconds != 2*86400+3*3600+4*60+5 || procs[1].Name != "hub.test" {
		t.Errorf("unexpected darwin ps proc[1]: %+v", procs[1])
	}
}

func TestMachineProcsRPCsAndGuardrails(t *testing.T) {
	fixedNow := time.Unix(1_760_000_000, 0)
	var killed []struct {
		pid int
		sig string
	}

	sampleProcs := []rawProcess{
		{PID: 1, PPID: 0, UID: 0, User: "root", Name: "systemd", Args: []string{"/sbin/init"}, CPUPercent: 0.1, RSSBytes: 10 << 20, UptimeSeconds: 100000, State: "S"},
		{PID: 100, PPID: 1, UID: 1000, User: "dev", Name: "systemd-user", Args: []string{"/lib/systemd/systemd", "--user"}, CPUPercent: 0.2, RSSBytes: 20 << 20, UptimeSeconds: 90000, State: "S"},
		{PID: 200, PPID: 100, UID: 1000, User: "dev", Name: "personad", Args: []string{"personad", "node", "--token=secret123"}, CPUPercent: 1.5, RSSBytes: 80 << 20, UptimeSeconds: 50000, State: "S"},
		{PID: 201, PPID: 1, UID: 1000, User: "dev", Name: "personad", Args: []string{"personad", "hub"}, CPUPercent: 2.0, RSSBytes: 120 << 20, UptimeSeconds: 50000, State: "S"},
		{PID: 300, PPID: 1, UID: 1000, User: "dev", Name: "hub.test", Args: []string{"/tmp/go-build999/b001/hub.test", "-test.run=TestSlow"}, CPUPercent: 45.2, RSSBytes: 256 << 20, UptimeSeconds: 4000, State: "R"},
		{PID: 301, PPID: 250, UID: 1000, User: "dev", Name: "node", Args: []string{"node", "node_modules/.bin/vitest", "--token=shouldhide"}, CPUPercent: 10.0, RSSBytes: 512 << 20, UptimeSeconds: 3600, State: "S"},
		{PID: 302, PPID: 100, UID: 1000, User: "dev", Name: "python3", Args: []string{"python3", "app.py"}, CPUPercent: 5.0, RSSBytes: 64 << 20, UptimeSeconds: 600, State: "S"},
		{PID: 400, PPID: 1, UID: 0, User: "root", Name: "nginx", Args: []string{"nginx: master process"}, CPUPercent: 0.5, RSSBytes: 30 << 20, UptimeSeconds: 80000, State: "S"},
	}

	n := &Node{
		machineProcs: &machineProcsOps{
			now:         func() time.Time { return fixedNow },
			currentUID:  func() int { return 1000 },
			currentPID:  func() int { return 200 },
			currentUser: func() string { return "dev" },
			listPorts: func(_ context.Context, protocol string) ([]proto.ListeningPort, bool, string, error) {
				return []proto.ListeningPort{
					{Protocol: "tcp", BindAddr: "0.0.0.0", Port: 8080, PID: 302, User: "dev", Process: "python3", Cmdline: "python3 app.py"},
					{Protocol: "tcp", BindAddr: "0.0.0.0", Port: 8080, PID: 302, User: "dev", Process: "python3", Cmdline: "python3 app.py"}, // duplicate
					{Protocol: "tcp", BindAddr: "127.0.0.1", Port: 3000, PID: 301, User: "dev", Process: "node", Cmdline: "node vitest"},
					{Protocol: "udp", BindAddr: "127.0.0.1", Port: 5353, PID: 400, User: "root", Process: "nginx"},
				}, true, "", nil
			},
			listProcesses: func(context.Context) ([]rawProcess, int64, bool, string, error) {
				return sampleProcs, 16 << 30, true, "", nil
			},
			sendSignal: func(pid int, sig string) error {
				killed = append(killed, struct {
					pid int
					sig string
				}{pid: pid, sig: sig})
				return nil
			},
		},
	}

	// 1. machine.ports: deduplication, sorting, scope classification, and protocol filter.
	out, err := n.handle(context.Background(), proto.MethodMachinePorts, json.RawMessage(`{"protocol":"tcp"}`))
	if err != nil {
		t.Fatalf("machine.ports failed: %v", err)
	}
	portsRes := out.(proto.MachinePortsResult)
	if !portsRes.Supported || len(portsRes.Ports) != 2 {
		t.Fatalf("unexpected ports result: %+v", portsRes)
	}
	if portsRes.Ports[0].Port != 3000 || portsRes.Ports[0].Scope != proto.PortScopeLoopback {
		t.Errorf("unexpected ports[0]: %+v", portsRes.Ports[0])
	}
	if portsRes.Ports[1].Port != 8080 || portsRes.Ports[1].Scope != proto.PortScopeAll {
		t.Errorf("unexpected ports[1]: %+v", portsRes.Ports[1])
	}
	if _, err := n.handle(context.Background(), proto.MethodMachinePorts, json.RawMessage(`{"protocol":"icmp"}`)); err == nil {
		t.Fatal("expected error for invalid protocol")
	}

	// 2. machine.processes: sorting, redaction, protection flags, and orphan detection.
	out, err = n.handle(context.Background(), proto.MethodMachineProcesses, json.RawMessage(`{"sort":"memory"}`))
	if err != nil {
		t.Fatalf("machine.processes failed: %v", err)
	}
	procRes := out.(proto.MachineProcessesResult)
	if !procRes.Supported || procRes.TotalCount != len(sampleProcs) || procRes.OrphanCount != 2 {
		t.Fatalf("unexpected processes summary: total=%d orphans=%d", procRes.TotalCount, procRes.OrphanCount)
	}
	if procRes.Processes[0].PID != 301 {
		t.Errorf("expected PID 301 first when sorted by memory, got %d", procRes.Processes[0].PID)
	}
	if strings.Contains(procRes.Processes[0].Cmdline, "shouldhide") {
		t.Errorf("cmdline was not redacted: %s", procRes.Processes[0].Cmdline)
	}
	byPID := map[int]proto.MachineProcess{}
	for _, p := range procRes.Processes {
		byPID[p.PID] = p
	}
	if !byPID[200].Protected || byPID[200].ProtectedReason != "persona_node" {
		t.Errorf("node process should be protected: %+v", byPID[200])
	}
	if !byPID[100].Protected || byPID[100].ProtectedReason != "persona_ancestor" {
		t.Errorf("ancestor process should be protected: %+v", byPID[100])
	}
	if !byPID[201].Protected || byPID[201].ProtectedReason != "persona_hub" {
		t.Errorf("hub process should be protected: %+v", byPID[201])
	}
	if !byPID[300].LikelyOrphan || !byPID[301].LikelyOrphan {
		t.Errorf("expected PID 300 and 301 flagged as likely orphans: 300=%+v 301=%+v", byPID[300], byPID[301])
	}
	if byPID[302].LikelyOrphan || byPID[400].LikelyOrphan || byPID[200].LikelyOrphan {
		t.Errorf("unexpected orphan flags on normal/protected/root processes")
	}

	// Orphan-only filter.
	out, err = n.handle(context.Background(), proto.MethodMachineProcesses, json.RawMessage(`{"orphan_only":true,"sort":"cpu"}`))
	if err != nil {
		t.Fatalf("machine.processes orphan_only failed: %v", err)
	}
	orphanRes := out.(proto.MachineProcessesResult)
	if len(orphanRes.Processes) != 2 || orphanRes.Processes[0].PID != 300 {
		t.Fatalf("unexpected orphan_only result: %+v", orphanRes.Processes)
	}

	// 3. machine.process.kill guardrails.
	expectRefusal := func(params, wantCode string) {
		t.Helper()
		_, err := n.handle(context.Background(), proto.MethodMachineProcessKill, json.RawMessage(params))
		var re *rpc.Error
		if !errors.As(err, &re) || re.Code != wantCode {
			t.Fatalf("kill(%s): got err=%v, want rpc code %q", params, err, wantCode)
		}
	}
	expectRefusal(`{"pid":302,"signal":"SIGHUP"}`, proto.CodeInvalidSignal)
	expectRefusal(`{"pid":1,"signal":"SIGTERM"}`, proto.CodeProcessProtected)
	expectRefusal(`{"pid":99999,"signal":"SIGTERM"}`, proto.CodeProcessNotFound)
	expectRefusal(`{"pid":400,"signal":"SIGTERM"}`, proto.CodeProcessNotOwned)
	expectRefusal(`{"pid":200,"signal":"SIGTERM"}`, proto.CodeProcessProtected)
	expectRefusal(`{"pid":100,"signal":"SIGTERM"}`, proto.CodeProcessProtected)
	expectRefusal(`{"pid":201,"signal":"SIGKILL"}`, proto.CodeProcessProtected)
	if len(killed) != 0 {
		t.Fatalf("refused kills still sent signals: %+v", killed)
	}

	// Allowed SIGTERM & SIGKILL on owned non-protected process.
	out, err = n.handle(context.Background(), proto.MethodMachineProcessKill, json.RawMessage(`{"pid":300,"signal":"SIGTERM"}`))
	if err != nil {
		t.Fatalf("allowed SIGTERM failed: %v", err)
	}
	if res := out.(proto.MachineProcessKillResult); !res.Sent || res.PID != 300 || res.Signal != "SIGTERM" {
		t.Fatalf("unexpected kill result: %+v", res)
	}
	out, err = n.handle(context.Background(), proto.MethodMachineProcessKill, json.RawMessage(`{"pid":301,"signal":"SIGKILL"}`))
	if err != nil {
		t.Fatalf("allowed SIGKILL failed: %v", err)
	}
	if res := out.(proto.MachineProcessKillResult); !res.Sent || res.PID != 301 || res.Signal != "SIGKILL" {
		t.Fatalf("unexpected kill result: %+v", res)
	}
	if len(killed) != 2 || killed[0].pid != 300 || killed[0].sig != "SIGTERM" || killed[1].pid != 301 || killed[1].sig != "SIGKILL" {
		t.Fatalf("unexpected sent signals: %+v", killed)
	}
}

func TestMachineProcsUnsupportedFallback(t *testing.T) {
	n := &Node{
		machineProcs: &machineProcsOps{
			now:         time.Now,
			currentUID:  func() int { return -1 },
			currentPID:  func() int { return 10 },
			currentUser: func() string { return "winuser" },
			listPorts: func(context.Context, string) ([]proto.ListeningPort, bool, string, error) {
				return nil, false, "unsupported os", nil
			},
			listProcesses: func(context.Context) ([]rawProcess, int64, bool, string, error) {
				return nil, 0, false, "unsupported os", nil
			},
			sendSignal: func(int, string) error { return errors.New("unsupported") },
		},
	}

	portsOut, err := n.handle(context.Background(), proto.MethodMachinePorts, nil)
	if err != nil {
		t.Fatalf("unsupported machine.ports returned error: %v", err)
	}
	if res := portsOut.(proto.MachinePortsResult); res.Supported || res.Unsupported == "" || res.Ports == nil {
		t.Fatalf("unexpected unsupported ports result: %+v", res)
	}

	procsOut, err := n.handle(context.Background(), proto.MethodMachineProcesses, nil)
	if err != nil {
		t.Fatalf("unsupported machine.processes returned error: %v", err)
	}
	if res := procsOut.(proto.MachineProcessesResult); res.Supported || res.Unsupported == "" || res.Processes == nil {
		t.Fatalf("unexpected unsupported processes result: %+v", res)
	}

	_, err = n.handle(context.Background(), proto.MethodMachineProcessKill, json.RawMessage(`{"pid":500,"signal":"SIGTERM"}`))
	var re *rpc.Error
	if !errors.As(err, &re) || re.Code != proto.CodeProcessUnsupported {
		t.Fatalf("expected process_unsupported refusal, got %v", err)
	}
}

func TestMachineProcsLiveLinuxSmoke(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux live /proc smoke test")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	n := &Node{}
	portsOut, err := n.handle(context.Background(), proto.MethodMachinePorts, json.RawMessage(`{"protocol":"tcp"}`))
	if err != nil {
		t.Fatalf("live machine.ports failed: %v", err)
	}
	portsRes := portsOut.(proto.MachinePortsResult)
	if !portsRes.Supported {
		t.Fatalf("expected Supported=true on linux")
	}
	foundPort := false
	for _, p := range portsRes.Ports {
		if p.Port == port && p.BindAddr == "127.0.0.1" && p.Scope == proto.PortScopeLoopback && p.PID == os.Getpid() {
			foundPort = true
			break
		}
	}
	if !foundPort {
		t.Fatalf("did not find live test listener on 127.0.0.1:%d (pid %d)", port, os.Getpid())
	}

	procsOut, err := n.handle(context.Background(), proto.MethodMachineProcesses, json.RawMessage(`{"sort":"cpu","limit":50}`))
	if err != nil {
		t.Fatalf("live machine.processes failed: %v", err)
	}
	procsRes := procsOut.(proto.MachineProcessesResult)
	if !procsRes.Supported || procsRes.TotalCount == 0 || len(procsRes.Processes) == 0 {
		t.Fatalf("unexpected live processes result: supported=%v total=%d", procsRes.Supported, procsRes.TotalCount)
	}
}
