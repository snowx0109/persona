package node

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A headless `claude -p` probe answers even on a machine where Claude Code's first-run setup is unfinished, but an
// interactive start then stops on the first-run screens (style picker, sign-in) and fails with a generic readiness
// timeout. The health check therefore reads hasCompletedOnboarding from Claude Code's global config first and fails
// with the cause instead of an answer that would mark the launcher healthy.

// claudeOnboardingNotReadyMsg is the check's error when the first-run setup is unfinished: it says what is wrong and
// what the person must do, since the probe command itself would still pass.
const claudeOnboardingNotReadyMsg = "Claude Code 还没完成首次引导（hasCompletedOnboarding 未置为 true）：请在这台机器的终端里运行一次 claude，" +
	"过完主题选择和登录等引导页面；在那之前探针能回答，但交互启动会卡在引导页"

// onboardingShellEnvTimeout bounds the login-shell question asked only when the default config file is absent.
const onboardingShellEnvTimeout = 5 * time.Second

// claudeOnboardingReady reports whether Claude Code's first-run setup is finished on this machine. known is false when
// the state cannot be judged (a legacy config path, a file that cannot be read or parsed, or another profile that has
// finished setup while the default one has not): the caller then runs the real probe instead of failing on a guess.
func (n *Node) claudeOnboardingReady(ctx context.Context) (ready, known bool) {
	path := n.claudeOnboardingConfig(ctx)
	if !modernClaudeConfig(path) {
		// The legacy .config.json belongs to old Claude Code builds with another onboarding flow; only the modern
		// .claude.json (and the custom-OAuth variant) carries hasCompletedOnboarding.
		return true, false
	}
	ready, known = claudeOnboardingVerdict(path)
	if !known || ready {
		return ready, known
	}
	// This config has not finished setup, but a launcher can pin another CLAUDE_CONFIG_DIR (claudedirs.go): when any
	// profile the node knows has finished its, do not block on the default config's verdict — that launcher may be
	// fully ready while the default profile is fresh.
	name := filepath.Base(path)
	for _, dir := range n.claudeConfigDirs() {
		if !filepath.IsAbs(dir) {
			continue // a node without a home reports a relative placeholder; it is not a real profile
		}
		if r, k := claudeOnboardingVerdict(filepath.Join(dir, name)); k && r {
			return true, false
		}
	}
	return false, true
}

// claudeOnboardingConfig locates Claude Code's global config as the running claude sees it. The node daemon may have
// been started outside the interactive shell, so when the default file is absent the login shell gets one question:
// a CLAUDE_CONFIG_DIR exported only from ~/.zshrc still redirects where claude looks.
func (n *Node) claudeOnboardingConfig(ctx context.Context) string {
	path := claudeGlobalConfig(os.Environ())
	if _, err := os.Stat(path); err == nil {
		return path
	}
	if dir := shellClaudeConfigDir(ctx); dir != "" {
		// envValue reads the first match, so drop an empty CLAUDE_CONFIG_DIR the process already carries before
		// appending the shell's value.
		env := append(withoutEnv(os.Environ(), []string{"CLAUDE_CONFIG_DIR"}), "CLAUDE_CONFIG_DIR="+dir)
		return claudeGlobalConfig(env)
	}
	return path
}

// shellClaudeConfigDir reads CLAUDE_CONFIG_DIR from the interactive login shell. It is a variable so tests can stay
// shell-free.
var shellClaudeConfigDir = func(ctx context.Context) string {
	pctx, cancel := context.WithTimeout(ctx, onboardingShellEnvTimeout)
	defer cancel()
	out, err := loginShell(pctx, `printf '%s\n' "$CLAUDE_CONFIG_DIR" </dev/null`).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// modernClaudeConfig reports whether path is a global config that carries hasCompletedOnboarding.
func modernClaudeConfig(path string) bool {
	base := filepath.Base(path)
	return base == ".claude.json" || base == ".claude-custom-oauth.json"
}

// claudeOnboardingVerdict reads one global config file. A missing file means claude has never run there: the
// first-run screens are certain. Any other read failure, or malformed JSON, is unknown rather than a verdict.
func claudeOnboardingVerdict(path string) (ready, known bool) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, true
	}
	if err != nil {
		return true, false
	}
	var cfg struct {
		HasCompletedOnboarding bool `json:"hasCompletedOnboarding"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return true, false
	}
	return cfg.HasCompletedOnboarding, true
}

// onboardingBlockHint names an unfinished first-run setup when the global config says so, else "". It turns the
// interactive start's generic readiness timeout into an error that says what to do.
func (n *Node) onboardingBlockHint(ctx context.Context) string {
	ready, known := n.claudeOnboardingReady(ctx)
	if known && !ready {
		return "this machine's Claude Code has not completed its first-run setup (hasCompletedOnboarding is unset): " +
			"run claude once in a terminal on that machine and finish its theme and sign-in screens"
	}
	return ""
}
