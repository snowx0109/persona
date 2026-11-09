package node

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nwverse/persona/server/internal/lazyre"
	"github.com/nwverse/persona/server/internal/prompts"
	"github.com/nwverse/persona/server/internal/proto"
)

// launcherCheckTimeout bounds Settings › Models' "Test": no answer by then is a failure (a variable for the tests).
var launcherCheckTimeout = 90 * time.Second

// launcherCheck answers Settings › Models' "Test": runs the launcher once, headless, with a one-word prompt in an empty
// directory, and says whether an answer came back, how long it took and which model answered. It is a real request
// (a few tokens).
func (n *Node) launcherCheck(ctx context.Context, name string) proto.LauncherCheck {
	if name != "claude" && !nameRe.MatchString(name) {
		return proto.LauncherCheck{Error: fmt.Sprintf("invalid launcher %q", name)}
	}
	agent := "claude"
	if m, managed := n.managedLauncher(name); managed {
		agent = m.AgentType()
	} else if name != "claude" {
		if res, ok := n.runtimeLauncherModels(name); ok && res.Agent != "" {
			agent = res.Agent
		} else if d := n.launcherProbeCached(ctx, name); d.Agent != "" {
			agent = d.Agent
		}
	}
	if agent == "claude" {
		// The headless probe below answers before Claude Code's first-run setup is finished, while an interactive
		// start then stops on the first-run screens: fail with the cause instead of marking the launcher healthy
		// (claudeonboarding.go). Unknown verdicts do not block; the real probe runs.
		if ready, known := n.claudeOnboardingReady(ctx); known && !ready {
			return proto.LauncherCheck{Agent: agent, Error: claudeOnboardingNotReadyMsg}
		}
	}
	dir, err := os.MkdirTemp("", "persona-check-")
	if err != nil {
		return proto.LauncherCheck{Error: err.Error()}
	}
	defer os.RemoveAll(dir)
	prompt := prompts.RenderLauncherCheck()
	args := ` -p ` + shellQuote(prompt) + ` --output-format json --max-turns 1`
	if agent == "agy" || agent == "omp" {
		args = ` -p ` + shellQuote(prompt)
	} else if agent != "claude" { // Codex and compatible variants: one non-interactive turn
		args = ` exec --skip-git-repo-check ` + shellQuote(prompt)
	}
	if agent == "claude" {
		// No hooks: the check is no session of the user's, and Persona's own hooks can hold it past the timeout (#3412;
		// the model list does the same, launcher_models.go). claudeSettings merges this with what the launcher needs from
		// --settings: a managed launcher's env (else the user's global settings env, another gateway, would win), an
		// alias's own --settings (Claude Code reads only the last one).
		ours := filepath.Join(dir, "check-settings.json")
		if err := os.WriteFile(ours, []byte(`{"disableAllHooks":true}`), 0o600); err != nil {
			return proto.LauncherCheck{Error: err.Error()}
		}
		args += ` --settings ` + shellQuote(n.claudeSettings(ctx, name, ours, dir))
	}
	ctx, cancel := context.WithTimeout(ctx, launcherCheckTimeout)
	defer cancel()
	cmd := loginShell(ctx, "cd "+shellQuote(dir)+" && "+n.launcherCommand(name)+args+" </dev/null")
	killGroupOnCancel(cmd)
	start := time.Now()
	out, err := cmd.CombinedOutput()
	res := proto.LauncherCheck{Agent: agent, Millis: int(time.Since(start).Milliseconds())}
	text := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		// What it printed before the time ran out usually names the cause (an upstream 401 or 402, a proxy that
		// refuses, a host that does not resolve); with nothing printed, how the command ended (#5951).
		res.Error = fmt.Sprintf("%d 秒内没有回答", int(launcherCheckTimeout.Seconds()))
		if tail := checkOutput(text); tail != "" {
			res.Error += "；最后的输出：" + tail
		} else if err != nil {
			res.Error += "；" + clipText(redactCheck(err.Error()), 200)
		}
		return res
	}
	if agent == "claude" {
		if r, ok := claudeResult(text); ok {
			res.Model, res.Answer = r.model, clipText(r.result, 200)
			if r.isError {
				res.Error = clipText(redactCheck(r.result), 400)
				// A result that is only its kind (error_during_execution…) says nothing of why; what claude printed
				// besides its result line often does.
				if r.bare {
					if tail := checkOutput(withoutResultLines(text)); tail != "" {
						res.Error += "；" + tail
					}
				}
			} else {
				res.OK = true
			}
			if r.millis > 0 {
				res.Millis = r.millis
			}
			return res
		}
	} else if err == nil && text != "" {
		lines := strings.Split(text, "\n")
		res.OK, res.Answer = true, clipText(lines[len(lines)-1], 200)
		return res
	}
	res.Error = checkOutput(text)
	if res.Error == "" && err != nil {
		res.Error = clipText(redactCheck(err.Error()), 400)
	}
	return res
}

// checkOutput is the end of what a check printed, as its error says it: the last lines, without keys, URLs or IPs.
func checkOutput(text string) string { return clipText(redactCheck(lastLines(text, 6)), 400) }

var (
	checkURL = lazyre.New(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	checkIP  = lazyre.New(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`)
)

// redactCheck keeps keys, gateway URLs and addresses out of a check's error (it is shown and may be logged).
func redactCheck(s string) string {
	return checkIP.ReplaceAllString(checkURL.ReplaceAllString(redactSecrets(s), "<url>"), "<ip>")
}

// withoutResultLines is what claude printed besides its JSON result.
func withoutResultLines(text string) string {
	var keep []string
	for _, l := range strings.Split(text, "\n") {
		if !strings.Contains(l, `"is_error"`) {
			keep = append(keep, l)
		}
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

type claudeOut struct {
	result  string
	isError bool
	bare    bool // an error whose result was empty: it says only its subtype
	model   string
	millis  int
}

// claudeResult reads `claude -p --output-format json`'s answer: the last line that holds a JSON object, from its first
// "{" that starts one (an interactive zsh may print banners first, and a shell integration's escape codes on the same
// line, #3412).
func claudeResult(out string) (claudeOut, bool) {
	lines := strings.Split(out, "\n")
	for l := len(lines) - 1; l >= 0; l-- {
		line := lines[l]
		for i := strings.IndexByte(line, '{'); i >= 0; {
			if r, ok := claudeResultJSON(line[i:]); ok {
				return r, true
			}
			next := strings.IndexByte(line[i+1:], '{')
			if next < 0 {
				break
			}
			i += 1 + next
		}
	}
	return claudeOut{}, false
}

func claudeResultJSON(body string) (claudeOut, bool) {
	var r struct {
		Result     string                     `json:"result"`
		IsError    bool                       `json:"is_error"`
		Subtype    string                     `json:"subtype"`
		DurationMS int                        `json:"duration_ms"`
		ModelUsage map[string]json.RawMessage `json:"modelUsage"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(body)), &r) != nil || (r.Result == "" && r.Subtype == "" && r.ModelUsage == nil) {
		return claudeOut{}, false
	}
	c := claudeOut{result: r.Result, isError: r.IsError || (r.Subtype != "" && r.Subtype != "success"), millis: r.DurationMS}
	if c.isError && c.result == "" {
		c.result, c.bare = r.Subtype, true
	}
	for m := range r.ModelUsage {
		if c.model == "" || len(m) > len(c.model) { // the main model; the small one is usually a prefix-free other name
			c.model = m
		}
	}
	return c, true
}

func clipText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
