package node

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeOnboardingVerdict(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "missing.json")
	if ready, known := claudeOnboardingVerdict(missing); !known || ready {
		t.Fatalf("missing config: ready=%v known=%v, want not ready, known", ready, known)
	}

	writeConfig := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	cases := []struct {
		name      string
		file      string
		wantReady bool
		wantKnown bool
	}{
		{"flag true", writeConfig(t.Name()+"-true.json", `{"hasCompletedOnboarding":true}`), true, true},
		{"flag false", writeConfig(t.Name()+"-false.json", `{"hasCompletedOnboarding":false}`), false, true},
		{"flag absent", writeConfig(t.Name()+"-absent.json", `{}`), false, true},
		{"flag among other keys", writeConfig(t.Name()+"-other.json", `{"theme":"dark","hasCompletedOnboarding":true,"foo":1}`), true, true},
		{"malformed json", writeConfig(t.Name()+"-bad.json", `{not json`), true, false},
		{"empty file", writeConfig(t.Name()+"-empty.json", ``), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ready, known := claudeOnboardingVerdict(tc.file)
			if ready != tc.wantReady || known != tc.wantKnown {
				t.Fatalf("got ready=%v known=%v, want ready=%v known=%v", ready, known, tc.wantReady, tc.wantKnown)
			}
		})
	}

	// A directory where a file is expected is an unreadable-for-another-reason case: unknown, not a verdict.
	if ready, known := claudeOnboardingVerdict(t.TempDir()); known || !ready {
		t.Fatalf("directory path: ready=%v known=%v, want unknown", ready, known)
	}
}

func TestModernClaudeConfig(t *testing.T) {
	for _, p := range []string{
		"/home/me/.claude.json",
		"/home/me/.claude/.claude-custom-oauth.json",
	} {
		if !modernClaudeConfig(p) {
			t.Errorf("%s should carry hasCompletedOnboarding", p)
		}
	}
	for _, p := range []string{
		"/home/me/.claude/.config.json",
		"/home/me/settings.json",
	} {
		if modernClaudeConfig(p) {
			t.Errorf("%s is not a modern onboarding config", p)
		}
	}
}

// With the config present the readiness lookup never asks a shell, so a zero-value Node is enough.
func TestClaudeOnboardingReadyFromConfig(t *testing.T) {
	prev := shellClaudeConfigDir
	shellClaudeConfigDir = func(context.Context) string { return "" }
	t.Cleanup(func() { shellClaudeConfigDir = prev })

	n := &Node{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")

	// No config yet: claude has never run, so first-run screens are certain.
	if ready, known := n.claudeOnboardingReady(context.Background()); !known || ready {
		t.Fatalf("no config: ready=%v known=%v, want not ready, known", ready, known)
	}

	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, known := n.claudeOnboardingReady(context.Background()); !known || !ready {
		t.Fatalf("onboarded config: ready=%v known=%v, want ready, known", ready, known)
	}

	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, known := n.claudeOnboardingReady(context.Background()); !known || ready {
		t.Fatalf("flag absent: ready=%v known=%v, want not ready, known", ready, known)
	}
}

// The default file is absent but the login shell exports CLAUDE_CONFIG_DIR: readiness is read where claude looks.
func TestClaudeOnboardingReadyFromShellConfigDir(t *testing.T) {
	home := t.TempDir()
	cfgDir := t.TempDir()
	prev := shellClaudeConfigDir
	shellClaudeConfigDir = func(context.Context) string { return cfgDir }
	t.Cleanup(func() { shellClaudeConfigDir = prev })

	n := &Node{}
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")

	if ready, known := n.claudeOnboardingReady(context.Background()); !known || ready {
		t.Fatalf("no config anywhere: ready=%v known=%v, want not ready, known", ready, known)
	}

	if err := os.WriteFile(filepath.Join(cfgDir, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, known := n.claudeOnboardingReady(context.Background()); !known || !ready {
		t.Fatalf("shell-directed config: ready=%v known=%v, want ready, known", ready, known)
	}
}

func TestClaudeOnboardingReadyCustomOAuth(t *testing.T) {
	n := &Node{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "https://example.invalid")

	path := filepath.Join(home, ".claude-custom-oauth.json")
	if err := os.WriteFile(path, []byte(`{"hasCompletedOnboarding":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, known := n.claudeOnboardingReady(context.Background()); !known || !ready {
		t.Fatalf("custom oauth config: ready=%v known=%v, want ready, known", ready, known)
	}
}

// A legacy .config.json is not judged: old builds keep another onboarding flow there.
func TestClaudeOnboardingSkipsLegacyConfig(t *testing.T) {
	n := &Node{}
	cfgDir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")

	legacy := filepath.Join(cfgDir, ".config.json")
	if err := os.WriteFile(legacy, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, known := n.claudeOnboardingReady(context.Background()); known || !ready {
		t.Fatalf("legacy config: ready=%v known=%v, want unknown (not judged)", ready, known)
	}
}

// A launcher can pin another CLAUDE_CONFIG_DIR: an onboarded profile keeps the default config's missing flag from
// blocking the check; a profile that is itself unfinished still blocks.
func TestClaudeOnboardingMultipleProfiles(t *testing.T) {
	prev := shellClaudeConfigDir
	shellClaudeConfigDir = func(context.Context) string { return "" }
	t.Cleanup(func() { shellClaudeConfigDir = prev })

	home := t.TempDir()
	profile := t.TempDir()
	n := &Node{home: home, cdirs: newClaudeDirs()}
	n.cdirs.scan = func() map[int]procEnv { return nil } // no real process scan in a unit test
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")

	// The default profile never finished setup.
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	n.setKnownClaudeDirs([]string{profile})

	// The pinned profile is fresh too: block.
	if ready, known := n.claudeOnboardingReady(context.Background()); !known || ready {
		t.Fatalf("two fresh profiles: ready=%v known=%v, want not ready, known", ready, known)
	}

	// Another profile finished its setup: do not block on the default's verdict.
	if err := os.WriteFile(filepath.Join(profile, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if ready, known := n.claudeOnboardingReady(context.Background()); known || !ready {
		t.Fatalf("an onboarded profile exists: ready=%v known=%v, want unknown (not blocking)", ready, known)
	}
}

func TestOnboardingBlockHint(t *testing.T) {
	prev := shellClaudeConfigDir
	shellClaudeConfigDir = func(context.Context) string { return "" }
	t.Cleanup(func() { shellClaudeConfigDir = prev })

	n := &Node{}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_CUSTOM_OAUTH_URL", "")

	if hint := n.onboardingBlockHint(context.Background()); hint == "" {
		t.Fatal("an unfinished setup should name the first-run cause")
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"hasCompletedOnboarding":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if hint := n.onboardingBlockHint(context.Background()); hint != "" {
		t.Fatalf("onboarded setup should give no hint, got %q", hint)
	}
}
