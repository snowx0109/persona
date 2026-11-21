package proto

// Terminal engines: the terminal host(s) a machine's node drives, chosen per machine. EngineHerdrBoth runs classic
// herdr and herdr-core side by side (docs/integrations/engines.md "Host sets"): a machine used both locally and through the Herdr
// app's remote sessions.
const (
	EngineHerdr     = "herdr"      // the classic herdr CLI
	EngineHerdrCore = "herdr-core" // the runtime of the Herdr desktop app
	EngineHerdrBoth = "herdr-both" // classic herdr and herdr-core together
	EngineOrca      = "orca"       // the Orca desktop app's runtime
	// EngineTern is Tern, Stencil's multiplexing terminal, through its `tern` command line (node/hosttern.go,
	// docs/integrations/engines.md "Tern").
	EngineTern = "tern"
	// EngineRex is Rex, a multiplexing terminal, through its `rex` command line and control API (node/hostrex.go,
	// docs/integrations/engines.md "Rex").
	EngineRex = "rex"
	// EnginePaseo is not a terminal: the Paseo daemon's agents, through its runtime (node/runtimecontrol.go,
	// docs/integrations/paseo.md). New sessions and sessions to continue go to Paseo.
	EnginePaseo = "paseo"
	// EngineCindy is Cindy's agents, through its runtime bridge (node/cindybridge.go, docs/integrations/engines.md).
	EngineCindy = "cindy"
)

// RuntimeEngine: engine is one that is a runtime, not a terminal (Paseo, Cindy): its own providers start every session
// that opens in it. Whether it runs only those (a launcher it lacks gets one of the same kind, or a refusal) its node
// says per start host (launchers.list's host_runtime, #8140).
func RuntimeEngine(engine string) bool { return engine == EnginePaseo || engine == EngineCindy }

// EngineNames are the engines a machine may use, with the names the app shows.
var EngineNames = map[string]string{
	EngineHerdr:     "herdr",
	EngineHerdrCore: "Herdr App",
	EngineHerdrBoth: "herdr + Herdr App",
	EngineOrca:      "Orca",
	EngineTern:      "Tern",
	EngineRex:       "Rex",
	EnginePaseo:     "Paseo",
	EngineCindy:     "Cindy",
}

// Engine is one engine as found on a machine: Available when its socket is reachable (herdr: or its binary is there).
// Note says why one that is there cannot be used (Orca too old or too new, Paseo's bridge not connected) or asks to
// update it (Paseo), in Chinese; the hub words the node-engine catalog's notes in each reader's language
// (i18n.EngineText), the others reach the app as they are.
type Engine struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Note      string `json:"note,omitempty"`
}
