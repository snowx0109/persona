package machdiag

import (
	"bufio"
	"bytes"
	"cmp"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/proto"
)

// process is one running process as the diagnosis sees it: its executable's name and what it uses. Nothing else of it
// is read.
type process struct {
	name string
	rss  int64    // bytes
	cpu  *float64 // percent of one core; nil when unknown
}

// The largest groups a report keeps: by resident memory and, added to those, by CPU.
const (
	topByRSS = 15
	topByCPU = 5
)

// kinds tags the executables a user knows by product. Names compare lowercased; anything else is "other". Node
// processes cannot be told apart by name (an Agent's CLI, a dev server and an editor's helper are all "node"), so
// they stay "other" rather than be guessed.
var kinds = map[string]string{
	"claude": proto.ProcessKindAgent, "codex": proto.ProcessKindAgent, "opencode": proto.ProcessKindAgent,
	"pi": proto.ProcessKindAgent, "gemini": proto.ProcessKindAgent,
	// The engines (proto.Engine*) and the terminal multiplexer Agents may run in.
	proto.EngineHerdr: proto.ProcessKindEngine, proto.EngineOrca: proto.ProcessKindEngine,
	proto.EngineTern: proto.ProcessKindEngine, proto.EngineRex: proto.ProcessKindEngine,
	proto.EnginePaseo: proto.ProcessKindEngine,
	proto.EngineCindy: proto.ProcessKindEngine, "tmux": proto.ProcessKindEngine,
	"personad": proto.ProcessKindPersona, "persona-node": proto.ProcessKindPersona,
	"persona-link": proto.ProcessKindPersona, "persona": proto.ProcessKindPersona,
}

func kindOf(name string) string {
	if k, ok := kinds[strings.ToLower(name)]; ok {
		return k
	}
	return proto.ProcessKindOther
}

// report groups processes by name and keeps the union of the largest by memory and by CPU, sorted by memory. Kernel
// threads (no resident memory) are counted but not grouped.
func report(procs []process) proto.ProcessReport {
	byName := map[string]*proto.ProcessGroup{}
	for _, p := range procs {
		if p.rss <= 0 || p.name == "" {
			continue
		}
		g := byName[p.name]
		if g == nil {
			g = &proto.ProcessGroup{Name: p.name, Kind: kindOf(p.name)}
			byName[p.name] = g
		}
		g.Count++
		g.RSS += p.rss
		if p.cpu != nil {
			v := *p.cpu
			if g.CPU != nil {
				v += *g.CPU
			}
			g.CPU = &v
		}
	}
	groups := make([]proto.ProcessGroup, 0, len(byName))
	for _, g := range byName {
		if g.CPU != nil {
			v := math.Round(*g.CPU*10) / 10
			g.CPU = &v
		}
		groups = append(groups, *g)
	}
	byRSS := func(a, b proto.ProcessGroup) int {
		return cmp.Or(cmp.Compare(b.RSS, a.RSS), strings.Compare(a.Name, b.Name))
	}
	slices.SortFunc(groups, byRSS)
	keep := map[string]bool{}
	for _, g := range groups[:min(topByRSS, len(groups))] {
		keep[g.Name] = true
	}
	byCPU := slices.Clone(groups)
	slices.SortStableFunc(byCPU, func(a, b proto.ProcessGroup) int { return cmp.Compare(cpuOf(b), cpuOf(a)) })
	for _, g := range byCPU[:min(topByCPU, len(byCPU))] {
		if g.CPU != nil && *g.CPU > 0 {
			keep[g.Name] = true
		}
	}
	out := make([]proto.ProcessGroup, 0, len(keep))
	for _, g := range groups {
		if keep[g.Name] {
			out = append(out, g)
		}
	}
	return proto.ProcessReport{Count: len(procs), Groups: out}
}

func cpuOf(g proto.ProcessGroup) float64 {
	if g.CPU == nil {
		return -1
	}
	return *g.CPU
}

// procStat is what /proc/<pid>/stat says of a process: its name (comm, at most 15 bytes), CPU time in clock ticks
// (user + system) and resident pages.
type procStat struct {
	name     string
	ticks    uint64
	rssPages int64
}

// parseProcStat reads /proc/<pid>/stat (proc(5)). The name sits in parentheses and may itself hold spaces and
// parentheses, so it ends at the last ')'; the fields after it start with field 3, the state.
func parseProcStat(b []byte) (procStat, bool) {
	open, end := bytes.IndexByte(b, '('), bytes.LastIndexByte(b, ')')
	if open < 0 || end < open {
		return procStat{}, false
	}
	f := strings.Fields(string(b[end+1:]))
	// utime is field 14, stime 15 and rss 24: indices 11, 12 and 21 after the name.
	if len(f) < 22 {
		return procStat{}, false
	}
	utime, err1 := strconv.ParseUint(f[11], 10, 64)
	stime, err2 := strconv.ParseUint(f[12], 10, 64)
	rss, err3 := strconv.ParseInt(f[21], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return procStat{}, false
	}
	return procStat{name: string(b[open+1 : end]), ticks: utime + stime, rssPages: max(rss, 0)}, true
}

// parsePS reads `ps -axo pid=,rss=,pcpu=,comm=` (macOS): RSS in KiB, CPU as ps measures it (a decaying average), and
// the executable, of which only the last path element is kept. comm is the executable's path, not the command line,
// and may hold spaces ("Google Chrome Helper"), so it is the rest of the line.
func parsePS(b []byte) []process {
	var out []process
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		if _, err := strconv.Atoi(f[0]); err != nil {
			continue
		}
		kib, err1 := strconv.ParseInt(f[1], 10, 64)
		cpu, err2 := strconv.ParseFloat(strings.Replace(f[2], ",", ".", 1), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		// The comm column starts after the third field's text.
		rest := line
		for range 3 {
			rest = strings.TrimLeft(rest, " \t")
			rest = rest[strings.IndexAny(rest, " \t"):]
		}
		// A login shell's comm starts with "-" ("-zsh").
		name := strings.TrimPrefix(path.Base(strings.TrimSpace(rest)), "-")
		if name == "" || name == "." || name == "/" {
			continue
		}
		out = append(out, process{name: name, rss: kib << 10, cpu: &cpu})
	}
	return out
}

// clockTicks is USER_HZ, the unit of /proc's CPU times: 100 on every architecture Linux builds for.
const clockTicks = 100

// sampleCPU makes processes from two readings dt apart: a process in both has its CPU share of that window; one that
// started in between has none.
func sampleCPU(first, second map[int]procStat, dt time.Duration, pageSize int) []process {
	out := make([]process, 0, len(second))
	for pid, st := range second {
		p := process{name: st.name, rss: st.rssPages * int64(pageSize)}
		if before, ok := first[pid]; ok && before.name == st.name && st.ticks >= before.ticks && dt > 0 {
			v := float64(st.ticks-before.ticks) / clockTicks / dt.Seconds() * 100
			p.cpu = &v
		}
		out = append(out, p)
	}
	return out
}
