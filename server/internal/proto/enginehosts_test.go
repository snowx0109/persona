package proto

import (
	"slices"
	"testing"
)

func TestNormalizeEngineChoice(t *testing.T) {
	for _, c := range []struct {
		hosts     []string
		def       string
		wantHosts []string
		wantDef   string
		bad       bool
	}{
		{hosts: []string{"orca", "herdr", "orca"}, wantHosts: []string{"herdr", "orca"}, wantDef: "herdr"}, // canonical order, once each
		{hosts: []string{"orca", "herdr"}, def: "orca", wantHosts: []string{"herdr", "orca"}, wantDef: "orca"},
		{hosts: []string{"paseo"}, wantHosts: []string{"paseo"}, wantDef: "paseo"},
		{hosts: []string{"paseo", "tern", "herdr"}, def: "tern", wantHosts: []string{"herdr", "tern", "paseo"}, wantDef: "tern"}, // Tern after Orca, before Paseo
		{hosts: []string{"paseo", "rex", "tern"}, def: "rex", wantHosts: []string{"tern", "rex", "paseo"}, wantDef: "rex"},       // Rex after Tern, before Paseo
		{hosts: []string{"cindy"}, wantHosts: []string{"cindy"}, wantDef: "cindy"},
		{hosts: []string{"cindy", "herdr"}, def: "cindy", wantHosts: []string{"herdr", "cindy"}, wantDef: "cindy"},
		{hosts: []string{"cindy", "tern"}, def: "tern", wantHosts: []string{"tern", "cindy"}, wantDef: "tern"}, // Tern before Cindy
		{hosts: nil, wantHosts: []string{}, wantDef: ""},                                                       // the empty set: no default
		{hosts: []string{}, def: "herdr", bad: true},
		{hosts: []string{"herdr"}, def: "orca", bad: true}, // default not enabled
		{hosts: []string{"tmux"}, bad: true},
		{hosts: []string{"herdr-both"}, bad: true}, // a set, not a member
	} {
		got, err := NormalizeEngineChoice(c.hosts, c.def)
		if c.bad {
			if err == nil {
				t.Errorf("%v/%q: no error, got %v", c.hosts, c.def, got)
			}
			continue
		}
		if err != nil || got.Hosts == nil || !slices.Equal(got.Hosts, c.wantHosts) || got.Default != c.wantDef {
			t.Errorf("%v/%q = %v, %v; want %v/%q", c.hosts, c.def, got, err, c.wantHosts, c.wantDef)
		}
	}
}

// Every single engine maps to a set and back to itself; sets map to the closest single engine.
func TestLegacyEngineRoundTrip(t *testing.T) {
	for engine := range EngineNames {
		c := EngineHostsOf(engine)
		if got := LegacyEngine(c); got != engine {
			t.Errorf("%s → %v → %s", engine, c, got)
		}
		if !LegacyExpresses(c) {
			t.Errorf("%s: its own set is not expressible", engine)
		}
	}
	both := EngineHostsOf(EngineHerdrBoth)
	if !slices.Equal(both.Hosts, []string{"herdr", "herdr-core"}) || both.Default != "herdr" {
		t.Errorf("herdr-both = %v", both)
	}
	if got := EngineHostsOf("tmux"); !slices.Equal(got.Hosts, []string{"herdr"}) {
		t.Errorf("unknown engine = %v", got)
	}
	for _, c := range []struct {
		hosts     []string
		def       string
		legacy    string
		expresses bool
	}{
		{[]string{"herdr", "orca"}, "herdr", "herdr", false},
		{[]string{"herdr", "orca"}, "orca", "orca", false},
		{[]string{"herdr", "herdr-core"}, "herdr-core", "herdr-both", false}, // herdr-both defaults to herdr
		{[]string{}, "", "herdr", false},
	} {
		n, err := NormalizeEngineChoice(c.hosts, c.def)
		if err != nil {
			t.Fatal(err)
		}
		if got := LegacyEngine(n); got != c.legacy || LegacyExpresses(n) != c.expresses {
			t.Errorf("%v/%s: legacy %s expresses %v", c.hosts, c.def, got, LegacyExpresses(n))
		}
	}
}

func TestSuggestEngineHosts(t *testing.T) {
	found := []Engine{
		{ID: "herdr", Available: false}, {ID: "herdr-core", Available: true}, {ID: "herdr-both", Available: false},
		{ID: "orca", Available: true}, {ID: "paseo", Available: false}, {ID: "future", Available: true},
	}
	if got := SuggestEngineHosts(found); !slices.Equal(got.Hosts, []string{"herdr-core", "orca"}) || got.Default != "herdr-core" {
		t.Errorf("suggested %v", got)
	}
	found[0].Available = true
	if got := SuggestEngineHosts(found); got.Default != "herdr" {
		t.Errorf("herdr found: default %q", got.Default)
	}
	for _, none := range [][]Engine{nil, {{ID: "orca"}}} {
		if got := SuggestEngineHosts(none); !slices.Equal(got.Hosts, []string{"herdr"}) || got.Default != "herdr" {
			t.Errorf("nothing found %v: %v", none, got)
		}
	}
}
