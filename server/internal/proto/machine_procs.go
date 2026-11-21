package proto

// Listening port inspection and process management for a computer (#12352).
// Read-only inspection (machine.ports, machine.processes) redacts sensitive command-line
// arguments; process termination (machine.process.kill) is exposed only on the user-authenticated
// Web/App HTTP route with ownership and Persona self-protection guardrails.

const (
	MethodMachinePorts       = "machine.ports"        // MachinePortsParams -> MachinePortsResult
	MethodMachineProcesses   = "machine.processes"    // MachineProcessesParams -> MachineProcessesResult
	MethodMachineProcessKill = "machine.process.kill" // MachineProcessKillParams -> MachineProcessKillResult
)

// Refusal codes returned by machine.process.kill and mapped by the hub.
const (
	CodeProcessNotFound    = "process_not_found"
	CodeProcessNotOwned    = "process_not_owned"
	CodeProcessProtected   = "process_protected"
	CodeProcessUnsupported = "process_unsupported"
	CodeInvalidSignal      = "invalid_signal"
)

// Listening port bind scopes.
const (
	PortScopeLoopback  = "loopback"  // 127.0.0.1, ::1, 127.0.0.0/8
	PortScopeAll       = "all"       // 0.0.0.0, ::, *
	PortScopeInterface = "interface" // specific non-loopback address
)

// MachinePortsParams are the parameters for machine.ports.
type MachinePortsParams struct {
	Protocol string `json:"protocol,omitempty"` // "", "tcp", "udp"
}

// ListeningPort describes one active listening TCP or UDP socket.
type ListeningPort struct {
	Protocol string `json:"protocol"`          // "tcp", "tcp6", "udp", "udp6"
	BindAddr string `json:"bind_addr"`         // e.g. "127.0.0.1", "0.0.0.0", "::1", "::"
	Port     int    `json:"port"`              // 1..65535
	Scope    string `json:"scope"`             // PortScope*
	PID      int    `json:"pid,omitempty"`     // 0 when hidden or inaccessible
	User     string `json:"user,omitempty"`    // OS username when resolved
	Process  string `json:"process,omitempty"` // executable name
	Cmdline  string `json:"cmdline,omitempty"` // redacted command line
}

// MachinePortsResult is the response of machine.ports.
type MachinePortsResult struct {
	At          int64           `json:"at"`
	Supported   bool            `json:"supported"`
	Unsupported string          `json:"unsupported,omitempty"`
	Ports       []ListeningPort `json:"ports"`
}

// MachineProcessesParams are the parameters for machine.processes.
type MachineProcessesParams struct {
	Sort       string `json:"sort,omitempty"`        // "cpu" (default), "memory", "uptime"
	Query      string `json:"query,omitempty"`       // case-insensitive filter on name, cmdline, user, or PID
	OrphanOnly bool   `json:"orphan_only,omitempty"` // return only likely stale/orphan test processes
	Limit      int    `json:"limit,omitempty"`       // default 100, capped at 300
}

// MachineProcess describes one running process on the computer.
type MachineProcess struct {
	PID             int      `json:"pid"`
	PPID            int      `json:"ppid"`
	UID             int      `json:"uid,omitempty"`
	User            string   `json:"user,omitempty"`
	Name            string   `json:"name"`
	Cmdline         string   `json:"cmdline"`                    // redacted command line
	CPUPercent      float64  `json:"cpu_percent"`                // percent of one core
	RSSBytes        int64    `json:"rss_bytes"`                  // resident memory in bytes
	UptimeSeconds   int64    `json:"uptime_seconds"`             // elapsed wall seconds
	State           string   `json:"state,omitempty"`            // OS state code (R, S, Z, ...)
	Owned           bool     `json:"owned"`                      // owned by the node's current OS user
	Protected       bool     `json:"protected,omitempty"`        // self-protected Persona/system process
	ProtectedReason string   `json:"protected_reason,omitempty"` // persona_node, persona_hub, persona_ancestor, system_pid
	LikelyOrphan    bool     `json:"likely_orphan,omitempty"`    // heuristic stale/orphan test process
	OrphanReasons   []string `json:"orphan_reasons,omitempty"`   // heuristic reason codes
}

// MachineProcessesResult is the response of machine.processes.
type MachineProcessesResult struct {
	At          int64            `json:"at"`
	Supported   bool             `json:"supported"`
	Unsupported string           `json:"unsupported,omitempty"`
	CurrentUser string           `json:"current_user,omitempty"`
	MemTotal    int64            `json:"mem_total,omitempty"`
	TotalCount  int              `json:"total_count"`
	OrphanCount int              `json:"orphan_count"`
	Processes   []MachineProcess `json:"processes"`
}

// MachineProcessKillParams are the parameters for machine.process.kill.
type MachineProcessKillParams struct {
	PID    int    `json:"pid"`
	Signal string `json:"signal"` // "SIGTERM" or "SIGKILL"
}

// MachineProcessKillResult is the response of machine.process.kill.
type MachineProcessKillResult struct {
	PID    int    `json:"pid"`
	Signal string `json:"signal"`
	Sent   bool   `json:"sent"`
}
