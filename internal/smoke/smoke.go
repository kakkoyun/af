// Package smoke implements the `af doctor --all` host self-smoke
// (ADR-074): it re-executes the running af binary against an isolated
// temporary environment, records every step's argv/exit/output, and
// renders a report designed to be pasted to an AI assistant (or filed
// as a GitHub issue) for diagnosis. The temp root is owned by the
// caller, so a run leaves the machine clean.
package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Status classifies one step's outcome.
type Status string

// Step outcomes.
const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// maxCapturedBytes bounds captured stdout/stderr per step so reports
// stay pasteable.
const maxCapturedBytes = 8 * 1024

// scratchDirPerm is the mode for directories inside the scratch root.
const scratchDirPerm = 0o750

// exitUsage mirrors cmd/af's EX_USAGE (ADR-068 §2): af's own
// argument validation failures, e.g. an invalid session name.
const exitUsage = 64

// exitNoInput mirrors cmd/af's EX_NOINPUT (ADR-068 §2): a session,
// branch, or file that does not exist.
const exitNoInput = 66

// skippedExitCode marks steps that never executed in the JSON report,
// disambiguating them from a successful exit 0.
const skippedExitCode = -1

// StepResult records one executed (or skipped) smoke step.
type StepResult struct {
	Name     string        `json:"name"`
	Dir      string        `json:"dir,omitempty"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	Expect   string        `json:"expect"`
	Status   Status        `json:"status"`
	Reason   string        `json:"reason,omitempty"`
	Argv     []string      `json:"argv,omitempty"`
	Duration time.Duration `json:"duration_ns"`
	ExitCode int           `json:"exit_code"`
}

// Env captures the host environment the smoke ran against.
type Env struct {
	Tools     map[string]string `json:"tools,omitempty"`
	AFVersion string            `json:"af_version"`
	GOOS      string            `json:"goos"`
	GOARCH    string            `json:"goarch"`
}

// Report is the full outcome of one self-smoke run.
type Report struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Env         Env          `json:"env"`
	Steps       []StepResult `json:"steps"`
}

// Counts tallies step outcomes as (pass, fail, skip).
func (r Report) Counts() (int, int, int) {
	var pass, fail, skip int
	for i := range r.Steps {
		switch r.Steps[i].Status {
		case StatusPass:
			pass++
		case StatusFail:
			fail++
		case StatusSkip:
			skip++
		}
	}
	return pass, fail, skip
}

// Failures returns the failing steps in run order.
func (r Report) Failures() []StepResult {
	out := make([]StepResult, 0)
	for i := range r.Steps {
		if r.Steps[i].Status == StatusFail {
			out = append(out, r.Steps[i])
		}
	}
	return out
}

// ExecFunc runs one external command and returns its captured streams
// and exit code. Implementations must not return an error for plain
// non-zero exits — only for failures to run at all.
type ExecFunc func(ctx context.Context, dir string, env []string, name string, args ...string) (stdout, stderr []byte, exitCode int, err error)

// Options wires Run to the host (or to fakes in tests).
type Options struct {
	// Exec runs commands; LookPath probes required tools; Now stamps
	// the report.
	Exec     ExecFunc
	LookPath func(name string) (string, error)
	Now      func() time.Time
	// Binary is the af executable to re-invoke (os.Executable in prod).
	Binary string
	// Root is the isolated scratch directory. The caller creates and
	// removes it; Run only populates it.
	Root string
}

// layout derives the isolated environment paths from the root.
type layout struct {
	root string
	home string
	repo string
}

func newLayout(root string) layout {
	return layout{root: root, home: filepath.Join(root, "home"), repo: filepath.Join(root, "repo")}
}

// step describes one smoke step declaratively.
type step struct {
	name           string
	stdoutContains string
	stderrContains string
	expect         string   // human-readable expectation for the report
	requires       []string // external binaries that must be on PATH
	args           []string // af arguments
	wantExit       int
	inRepo         bool // run with the scratch repo as cwd
}

// steps is the local command-surface suite. Ordering matters: later
// steps consume state created by earlier ones.
func steps() []step {
	return []step{
		{name: "version", args: []string{"version"}, expect: "exit 0, version string on stdout", stdoutContains: "af"},
		{name: "help", args: []string{"--help"}, expect: "exit 0", stdoutContains: "Usage"},
		{name: "setup", args: []string{"setup", "--shell", "bash"}, expect: "exit 0; config, state dirs, and bash completions created in isolated HOME"},
		{name: "config-show", args: []string{"config", "show"}, expect: "exit 0, merged TOML on stdout"},
		{name: "doctor-probe", args: []string{"doctor"}, expect: "runs; tool inventory captured for the report (exit ignored)", wantExit: -1},
		{name: "list-empty", args: []string{"list"}, expect: "exit 0, reports no workstreams", stdoutContains: "no workstreams"},
		{name: "create", requires: []string{"git"}, args: []string{"create", "smoke-ws", "--bare", "--from", "main"}, inRepo: true, expect: "exit 0, workstream created", stdoutContains: "created workstream"},
		{name: "create-traversal-rejected", requires: []string{"git"}, args: []string{"create", "../evil", "--bare"}, inRepo: true, wantExit: exitUsage, stderrContains: "invalid session name", expect: "traversal names rejected (ADR-069) with EX_USAGE (ADR-068)"},
		{name: "list", requires: []string{"git"}, args: []string{"list"}, expect: "exit 0, lists smoke-ws", stdoutContains: "smoke-ws"},
		{name: "status", requires: []string{"git"}, args: []string{"status"}, expect: "exit 0, dashboard lists smoke-ws", stdoutContains: "smoke-ws"},
		{name: "info", requires: []string{"git"}, args: []string{"info", "smoke-ws", "--ledger", "5"}, expect: "exit 0, state summary with ledger tail", stdoutContains: "smoke-ws"},
		{name: "note", requires: []string{"git"}, args: []string{"note", "smoke-ws", "--append", "doctor self-smoke probe"}, expect: "exit 0, ledger event appended"},
		{name: "stack-parent-validation", requires: []string{"git"}, args: []string{"stack", "smoke-ws", "--parent", "../evil"}, wantExit: exitUsage, stderrContains: "invalid session name", expect: "stack parent names validated with EX_USAGE (ADR-068)"},
		{name: "sync-without-parent", requires: []string{"git"}, args: []string{"sync", "smoke-ws"}, wantExit: 1, stderrContains: "ParentSession", expect: "sync without a stack parent fails with guidance"},
		{name: "suspend", requires: []string{"git"}, args: []string{"suspend", "smoke-ws"}, expect: "exit 0, status flips to suspended", stdoutContains: "suspended"},
		{name: "resume", requires: []string{"git"}, args: []string{"resume", "smoke-ws", "--bare"}, expect: "exit 0, status flips to active", stdoutContains: "active"},
		{name: "done", requires: []string{"git"}, args: []string{"done", "smoke-ws"}, expect: "exit 0, workstream archived", stdoutContains: "completed"},
		{name: "unknown-session-rejected", args: []string{"info", "ghost-session-does-not-exist"}, wantExit: exitNoInput, expect: "unknown session names error cleanly with EX_NOINPUT (ADR-068)"},
		{name: "clean-dry-run", args: []string{"clean", "--dry-run"}, expect: "exit 0, dry run reaps nothing"},
		{name: "retro", args: []string{"retro"}, expect: "exit 0, archive mined (may be empty)", wantExit: -1},
		{name: "completions", args: []string{"completions", "bash"}, expect: "exit 0, completion script on stdout"},
	}
}

// probedTools is the inventory captured into the report environment.
func probedTools() []string {
	return []string{"git", "tmux", "fzf", "gh", "pi", "claude", "codex", "slicer"}
}

// Run executes the self-smoke suite against the isolated root and
// returns the report. It never returns an error for step failures —
// those live in the report — only for being unable to run at all.
func Run(ctx context.Context, opts Options) (Report, error) {
	lay := newLayout(opts.Root)
	err := os.MkdirAll(lay.home, scratchDirPerm)
	if err != nil {
		return Report{}, fmt.Errorf("smoke: create isolated home: %w", err)
	}

	report := Report{
		GeneratedAt: opts.Now(),
		Env:         Env{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Tools: map[string]string{}},
	}
	probeTools(ctx, opts, lay, &report)

	gitReady := prepareRepo(ctx, opts, lay, &report)

	suite := steps()
	for i := range suite {
		st := &suite[i]
		if reason := missingRequirement(opts, st, gitReady); reason != "" {
			report.Steps = append(report.Steps, StepResult{Name: st.name, Expect: st.expect, Status: StatusSkip, Reason: reason, ExitCode: skippedExitCode})
			continue
		}
		res := runStep(ctx, opts, lay, st)
		if st.name == "version" && res.Status == StatusPass {
			report.Env.AFVersion = strings.TrimSpace(res.Stdout)
		}
		report.Steps = append(report.Steps, res)
	}
	return report, nil
}

func missingRequirement(opts Options, st *step, gitReady bool) string {
	for _, tool := range st.requires {
		_, err := opts.LookPath(tool)
		if err != nil {
			return tool + " not installed"
		}
		if tool == "git" && !gitReady {
			return "scratch git repository could not be prepared"
		}
	}
	return ""
}

// probeTools records tool availability/version lines for the report.
func probeTools(ctx context.Context, opts Options, lay layout, report *Report) {
	for _, tool := range probedTools() {
		_, err := opts.LookPath(tool)
		if err != nil {
			report.Env.Tools[tool] = "missing"
			continue
		}
		stdout, _, exit, execErr := opts.Exec(ctx, lay.root, isolatedEnv(lay), tool, "--version")
		line := firstLine(string(stdout))
		if execErr != nil || exit != 0 || line == "" {
			line = "present (version probe failed)"
		}
		report.Env.Tools[tool] = line
	}
}

// prepareRepo initialises the scratch git repository the create/lifecycle
// steps run in. Returns false (and records nothing fatal) when git is
// unavailable or the init fails — dependent steps then skip.
func prepareRepo(ctx context.Context, opts Options, lay layout, report *Report) bool {
	_, err := opts.LookPath("git")
	if err != nil {
		return false
	}
	err = os.MkdirAll(lay.repo, scratchDirPerm)
	if err != nil {
		return false
	}
	cmds := [][]string{
		{"git", "init", "-q", "-b", "main"},
		{"git", "commit", "--allow-empty", "-q", "-m", "smoke seed"},
	}
	for _, argv := range cmds {
		_, stderr, exit, execErr := opts.Exec(ctx, lay.repo, isolatedEnv(lay), argv[0], argv[1:]...)
		if execErr != nil || exit != 0 {
			report.Steps = append(report.Steps, StepResult{
				Name:     "prepare-scratch-repo",
				Argv:     argv,
				Dir:      lay.repo,
				ExitCode: exit,
				Stderr:   truncateOutput(string(stderr)),
				Expect:   "scratch git repo initialises",
				Status:   StatusSkip,
				Reason:   "git repo preparation failed; repo-dependent steps skipped",
			})
			return false
		}
	}
	return true
}

// isolatedEnv builds the child environment: the host PATH with HOME
// (and git identity) redirected into the scratch root so no real state
// is touched.
func isolatedEnv(lay layout) []string {
	env := []string{
		"HOME=" + lay.home,
		"PATH=" + os.Getenv("PATH"),
		"GIT_AUTHOR_NAME=af-doctor", "GIT_AUTHOR_EMAIL=doctor@af.invalid",
		"GIT_COMMITTER_NAME=af-doctor", "GIT_COMMITTER_EMAIL=doctor@af.invalid",
		"GIT_CONFIG_GLOBAL=" + filepath.Join(lay.home, ".gitconfig"),
		"NO_COLOR=1",
	}
	if term := os.Getenv("TERM"); term != "" {
		env = append(env, "TERM="+term)
	}
	return env
}

func runStep(ctx context.Context, opts Options, lay layout, st *step) StepResult {
	dir := lay.root
	if st.inRepo {
		dir = lay.repo
	}
	argv := append([]string{opts.Binary}, st.args...)
	started := opts.Now()
	stdout, stderr, exit, execErr := opts.Exec(ctx, dir, isolatedEnv(lay), opts.Binary, st.args...)
	res := StepResult{
		Name:     st.name,
		Argv:     argv,
		Dir:      dir,
		ExitCode: exit,
		Duration: opts.Now().Sub(started),
		Stdout:   truncateOutput(string(stdout)),
		Stderr:   truncateOutput(string(stderr)),
		Expect:   st.expect,
	}
	res.Status, res.Reason = evaluate(st, exit, string(stdout), string(stderr), execErr)
	return res
}

// evaluate applies the step expectations to the captured outcome. A
// non-nil execErr always fails: per the ExecFunc contract it means the
// command could not run at all, which wantExit=-1 (ignore the exit
// code) must not mask.
func evaluate(st *step, exit int, stdout, stderr string, execErr error) (Status, string) {
	if execErr != nil {
		return StatusFail, fmt.Sprintf("could not execute: %v", execErr)
	}
	if st.wantExit >= 0 && exit != st.wantExit {
		return StatusFail, fmt.Sprintf("exit code %d, want %d", exit, st.wantExit)
	}
	if st.stdoutContains != "" && !strings.Contains(stdout, st.stdoutContains) {
		return StatusFail, fmt.Sprintf("stdout missing %q", st.stdoutContains)
	}
	if st.stderrContains != "" && !strings.Contains(stderr, st.stderrContains) {
		return StatusFail, fmt.Sprintf("stderr missing %q", st.stderrContains)
	}
	return StatusPass, ""
}

func truncateOutput(s string) string {
	if len(s) <= maxCapturedBytes {
		return s
	}
	return s[:maxCapturedBytes] + fmt.Sprintf("\n... [truncated %d bytes]", len(s)-maxCapturedBytes)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// RenderJSON serialises the report for tooling.
func RenderJSON(r Report) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("smoke: marshal report: %w", err)
	}
	return data, nil
}

// RenderMarkdown renders the paste-to-assistant report: environment
// block first, then failures (most actionable), skips, and a compact
// pass list.
func RenderMarkdown(r Report) string {
	var b strings.Builder
	pass, fail, skip := r.Counts()
	fmt.Fprintf(&b, "# af doctor self-smoke report\n\n")
	fmt.Fprintf(&b, "- generated: %s\n- af: %s\n- host: %s/%s\n- result: %d pass, %d fail, %d skip\n\n",
		r.GeneratedAt.Format(time.RFC3339), r.Env.AFVersion, r.Env.GOOS, r.Env.GOARCH, pass, fail, skip)

	b.WriteString("## Environment\n\n")
	tools := make([]string, 0, len(r.Env.Tools))
	for tool := range r.Env.Tools {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		fmt.Fprintf(&b, "- %s: %s\n", tool, r.Env.Tools[tool])
	}
	b.WriteString("\n")

	if fail > 0 {
		b.WriteString("## Failures\n\n")
		failures := r.Failures()
		for i := range failures {
			renderFailure(&b, &failures[i])
		}
	}
	if skip > 0 {
		b.WriteString("## Skipped\n\n")
		for i := range r.Steps {
			if r.Steps[i].Status == StatusSkip {
				fmt.Fprintf(&b, "- `%s`: %s\n", r.Steps[i].Name, r.Steps[i].Reason)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## Passed\n\n")
	for i := range r.Steps {
		if r.Steps[i].Status == StatusPass {
			fmt.Fprintf(&b, "- `%s` (%s)\n", r.Steps[i].Name, r.Steps[i].Duration.Round(time.Millisecond))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderFailure(b *strings.Builder, s *StepResult) {
	fmt.Fprintf(b, "### %s\n\n", s.Name)
	fmt.Fprintf(b, "- expectation: %s\n- verdict: %s\n- exit code: %d\n\n", s.Expect, s.Reason, s.ExitCode)
	fmt.Fprintf(b, "Command (run in an isolated `$HOME`; `dir` is inside the scratch root):\n\n```\ncd %s\n%s\n```\n\n", s.Dir, strings.Join(s.Argv, " "))
	if s.Stdout != "" {
		fmt.Fprintf(b, "stdout:\n\n```\n%s\n```\n\n", s.Stdout)
	}
	if s.Stderr != "" {
		fmt.Fprintf(b, "stderr:\n\n```\n%s\n```\n\n", s.Stderr)
	}
}
