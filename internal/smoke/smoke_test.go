package smoke_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/smoke"
)

// fakeExec returns canned results keyed by the joined argv; repeated
// keys consume a queue. Unmatched commands succeed with empty output.
type fakeExec struct {
	responses map[string][]fakeResult
	calls     []string
}

type fakeResult struct {
	err    error
	stdout string
	stderr string
	exit   int
}

func (f *fakeExec) queue(key string, res fakeResult) {
	if f.responses == nil {
		f.responses = map[string][]fakeResult{}
	}
	f.responses[key] = append(f.responses[key], res)
}

func (f *fakeExec) run(_ context.Context, _ string, _ []string, name string, args ...string) ([]byte, []byte, int, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, key)
	q := f.responses[key]
	if len(q) == 0 {
		return nil, nil, 0, nil
	}
	res := q[0]
	f.responses[key] = q[1:]
	return []byte(res.stdout), []byte(res.stderr), res.exit, res.err
}

// compliantExec cans the outputs a healthy af binary would produce for
// every step with a content expectation.
func compliantExec() *fakeExec {
	f := &fakeExec{}
	f.queue("af version", fakeResult{stdout: "af dev (commit none)\n"})
	f.queue("af --help", fakeResult{stdout: "Usage:\n  af [command]\n"})
	f.queue("af list", fakeResult{stdout: "no workstreams\n"})  // list-empty
	f.queue("af list", fakeResult{stdout: "smoke-ws active\n"}) // list after create
	for key, out := range map[string]string{
		"af create smoke-ws --bare --from main": "created workstream smoke-ws\n",
		"af status":                             "smoke-ws active\n",
		"af info smoke-ws --ledger 5":           "Session:   smoke-ws\ncreated\n",
		"af suspend smoke-ws":                   "workstream smoke-ws -> suspended\n",
		"af resume smoke-ws --bare":             "workstream smoke-ws -> active\n",
		"af done smoke-ws":                      "workstream smoke-ws -> completed\n",
	} {
		f.queue(key, fakeResult{stdout: out})
	}
	// Invalid session names exit 64 (EX_USAGE per ADR-068 §2); the
	// other rejections stay on the generic exit 1.
	for key, msg := range map[string]string{
		"af create ../evil --bare":           "invalid session name",
		"af stack smoke-ws --parent ../evil": "invalid session name",
	} {
		f.queue(key, fakeResult{stderr: msg, exit: smoke.ExitUsageForTest})
	}
	f.queue("af sync smoke-ws", fakeResult{stderr: "sync requires Stack.ParentSession to be set", exit: 1})
	// Unknown sessions exit 66 (EX_NOINPUT per ADR-068 §2).
	f.queue("af info ghost-session-does-not-exist", fakeResult{stderr: "session directory missing", exit: smoke.ExitNoInputForTest})
	return f
}

var errToolMissing = errors.New("not found")

func allToolsPresent(string) (string, error) { return "/usr/bin/fake", nil }

func newOptions(t *testing.T, exec *fakeExec) smoke.Options {
	t.Helper()
	return smoke.Options{
		Binary:   "af",
		Root:     t.TempDir(),
		Exec:     exec.run,
		LookPath: allToolsPresent,
		Now:      func() time.Time { return time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC) },
	}
}

func TestRun_AllStepsPassWithCompliantBinary(t *testing.T) {
	exec := compliantExec()

	report, err := smoke.Run(context.Background(), newOptions(t, exec))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	pass, fail, skip := report.Counts()
	if fail != 0 || skip != 0 {
		for _, s := range report.Steps {
			if s.Status != smoke.StatusPass {
				t.Logf("%s: %s (%s)", s.Name, s.Status, s.Reason)
			}
		}
		t.Fatalf("Counts() = pass %d fail %d skip %d, want all pass", pass, fail, skip)
	}
	if report.Env.AFVersion == "" {
		t.Fatal("Env.AFVersion empty; version step output must be captured")
	}
}

func TestRun_FailingStepIsReportedWithDetail(t *testing.T) {
	exec := compliantExec()
	exec.responses["af setup --shell bash"] = []fakeResult{{stderr: "boom: disk full", exit: 1}}

	report, err := smoke.Run(context.Background(), newOptions(t, exec))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_, fail, _ := report.Counts()
	if fail == 0 {
		t.Fatal("want at least one failure")
	}
	var found bool
	for _, s := range report.Failures() {
		if s.Name == "setup" {
			found = true
			if !strings.Contains(s.Stderr, "disk full") {
				t.Fatalf("failure Stderr = %q, want captured output", s.Stderr)
			}
			if s.Reason == "" {
				t.Fatal("failure Reason empty; must explain the expectation miss")
			}
		}
	}
	if !found {
		t.Fatalf("setup not in Failures(): %+v", report.Failures())
	}
}

func TestRun_MissingToolSkipsDependentSteps(t *testing.T) {
	exec := compliantExec()
	opts := newOptions(t, exec)
	opts.LookPath = func(name string) (string, error) {
		if name == "git" {
			return "", errToolMissing
		}
		return "/usr/bin/fake", nil
	}

	report, err := smoke.Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	_, fail, skip := report.Counts()
	if fail != 0 {
		t.Fatalf("missing git must SKIP repo steps, not fail them: %+v", report.Failures())
	}
	if skip == 0 {
		t.Fatal("want git-dependent steps skipped")
	}
	for _, call := range exec.calls {
		if strings.HasPrefix(call, "af create smoke-ws") {
			t.Fatal("create ran despite missing git")
		}
	}
}

func TestRenderMarkdown_FailureFirstAndActionable(t *testing.T) {
	report := smoke.Report{
		GeneratedAt: time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
		Env:         smoke.Env{AFVersion: "af dev", GOOS: "darwin", GOARCH: "arm64", Tools: map[string]string{"git": "git version 2.55.0", "tmux": "missing"}},
		Steps: []smoke.StepResult{
			{Name: "version", Argv: []string{"af", "version"}, Status: smoke.StatusPass, Expect: "exit 0"},
			{Name: "create", Argv: []string{"af", "create", "smoke-ws", "--bare"}, Status: smoke.StatusFail, ExitCode: 1, Stderr: "create workstream failed: boom", Expect: "exit 0 and state.toml exists", Reason: "exit code 1, want 0"},
			{Name: "pull", Argv: []string{"af", "pull"}, Status: smoke.StatusSkip, Reason: "slicer not installed"},
		},
	}

	md := smoke.RenderMarkdown(report)
	failIdx := strings.Index(md, "## Failures")
	passIdx := strings.Index(md, "## Passed")
	if failIdx == -1 || passIdx == -1 || failIdx > passIdx {
		t.Fatalf("markdown must list failures before passes:\n%s", md)
	}
	for _, want := range []string{
		"af dev", "darwin/arm64",
		"af create smoke-ws --bare",
		"create workstream failed: boom",
		"exit code 1, want 0",
		"## Skipped",
		"slicer not installed",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRenderJSON_RoundTrips(t *testing.T) {
	report := smoke.Report{
		GeneratedAt: time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
		Env:         smoke.Env{AFVersion: "af dev", GOOS: "linux", GOARCH: "amd64"},
		Steps:       []smoke.StepResult{{Name: "version", Status: smoke.StatusPass}},
	}
	data, err := smoke.RenderJSON(report)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var back smoke.Report
	err = json.Unmarshal(data, &back)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.Steps[0].Name != "version" || back.Env.GOOS != "linux" {
		t.Fatalf("round trip lost data: %+v", back)
	}
}

func TestTruncateOutput_CapsLongStreams(t *testing.T) {
	exec := compliantExec()
	exec.responses["af version"] = []fakeResult{{stdout: "af " + strings.Repeat("x", 64*1024)}}

	report, err := smoke.Run(context.Background(), newOptions(t, exec))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, s := range report.Steps {
		if s.Name == "version" {
			if len(s.Stdout) > 8*1024+64 {
				t.Fatalf("stdout not truncated: %d bytes", len(s.Stdout))
			}
			if !strings.Contains(s.Stdout, "[truncated") {
				t.Fatal("truncation marker missing")
			}
		}
	}
}
