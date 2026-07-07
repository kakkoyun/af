package diff_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/diff"
)

// errExecFailed is the sentinel injected into fakeExecutor for error-path tests.
var errExecFailed = errors.New("exec failed")

// requireBinary skips the test when name is not installed on the host.
func requireBinary(t *testing.T, name string) {
	t.Helper()
	_, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("binary %q not available: %v", name, err)
	}
}

// TestRender_ExecutorErrorsWrapped verifies that each dispatch branch wraps the
// executor error with a branch-specific message while preserving errors.Is.
func TestRender_ExecutorErrorsWrapped(t *testing.T) {
	t.Parallel()

	cases := []struct { //nolint:govet // Readability over field alignment in test table.
		name        string
		mode        diff.Mode
		interactive bool
		missing     []string
		wantMsg     string
	}{
		{
			name:    "web branch wraps as diff web",
			mode:    diff.ModeWeb,
			wantMsg: "diff web: ",
		},
		{
			name:    "stat branch wraps as diff stat",
			mode:    diff.ModeAuto,
			wantMsg: "diff stat: ",
		},
		{
			name:        "terminal fallback wraps as diff terminal fallback",
			mode:        diff.ModeAuto,
			interactive: true,
			missing:     []string{"hunk"},
			wantMsg:     "diff terminal fallback: ",
		},
		{
			name:        "hunk pipe wraps as diff hunk pipe",
			mode:        diff.ModeAuto,
			interactive: true,
			wantMsg:     "diff hunk pipe: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex := &fakeExecutor{err: errExecFailed}
			deps := diff.Deps{LookPath: fakeLookPath(tc.missing...), Exec: ex}
			err := diff.Render(t.Context(), deps, makeOpts(tc.mode, tc.interactive))
			if !errors.Is(err, errExecFailed) {
				t.Fatalf("want errExecFailed via errors.Is, got: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("want message containing %q, got %q", tc.wantMsg, err.Error())
			}
		})
	}
}

// TestRender_UnknownModeUsesAutoDispatch verifies that an out-of-range Mode
// value falls into the default branch and behaves like ModeAuto.
func TestRender_UnknownModeUsesAutoDispatch(t *testing.T) {
	t.Parallel()

	ex := &fakeExecutor{}
	deps := diff.Deps{LookPath: fakeLookPath(), Exec: ex}
	err := diff.Render(t.Context(), deps, makeOpts(diff.Mode(99), false))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(ex.calls))
	}
	argv := ex.calls[0].argv
	if argv[0] != gitBin || argv[1] != diffArgLiteral || argv[2] != statArg {
		t.Fatalf("want 'git diff --stat' for unknown mode, got %v", argv)
	}
}

// TestRender_NilLookPathDefaultsToExecLookPath verifies that a nil
// Deps.LookPath falls back to exec.LookPath without panicking. The
// non-interactive stat path never consults LookPath, so the default is safe
// regardless of what binaries exist on the host.
func TestRender_NilLookPathDefaultsToExecLookPath(t *testing.T) {
	t.Parallel()

	ex := &fakeExecutor{}
	err := diff.Render(t.Context(), diff.Deps{Exec: ex}, makeOpts(diff.ModeAuto, false))
	if err != nil {
		t.Fatalf("Render with nil LookPath: %v", err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(ex.calls))
	}
}

// TestExecExecutor_Execute verifies the production Execute happy path streams
// stdout from a real process.
func TestExecExecutor_Execute(t *testing.T) {
	t.Parallel()
	requireBinary(t, "sh")

	var out, errBuf bytes.Buffer
	ex := diff.ExecExecutor{}
	err := ex.Execute(t.Context(), t.TempDir(), &out, &errBuf, "sh", "-c", "printf hello; printf world >&2")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := out.String(); got != "hello" {
		t.Fatalf("want stdout %q, got %q", "hello", got)
	}
	if got := errBuf.String(); got != "world" {
		t.Fatalf("want stderr %q, got %q", "world", got)
	}
}

// TestExecExecutor_ExecuteErrors verifies Execute wraps both spawn failures
// and non-zero exits.
func TestExecExecutor_ExecuteErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		need string
		argv []string
	}{
		{name: "missing binary", argv: []string{"af-definitely-not-a-binary-xyz"}},
		{name: "non-zero exit", argv: []string{"sh", "-c", "exit 3"}, need: "sh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.need != "" {
				requireBinary(t, tc.need)
			}
			ex := diff.ExecExecutor{}
			err := ex.Execute(t.Context(), t.TempDir(), io.Discard, io.Discard, tc.argv...)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), "diff execute "+tc.argv[0]) {
				t.Fatalf("want wrapped 'diff execute %s' message, got %q", tc.argv[0], err.Error())
			}
		})
	}
}

// TestExecExecutor_ExecutePipe verifies the production pipeline wires cmd1
// stdout into cmd2 stdin.
func TestExecExecutor_ExecutePipe(t *testing.T) {
	t.Parallel()
	requireBinary(t, "sh")
	requireBinary(t, "cat")

	var out bytes.Buffer
	ex := diff.ExecExecutor{}
	err := ex.ExecutePipe(t.Context(), t.TempDir(), &out, io.Discard,
		[]string{"sh", "-c", "printf piped"}, []string{"cat"})
	if err != nil {
		t.Fatalf("ExecutePipe: %v", err)
	}
	if got := out.String(); got != "piped" {
		t.Fatalf("want %q through pipe, got %q", "piped", got)
	}
}

// TestExecExecutor_ExecutePipeErrors verifies each reachable error branch of
// the production pipeline: cmd1 spawn failure, cmd2 spawn failure, and cmd2
// non-zero exit.
func TestExecExecutor_ExecutePipeErrors(t *testing.T) {
	t.Parallel()

	cases := []struct { //nolint:govet // Readability over field alignment in test table.
		name    string
		cmd1    []string
		cmd2    []string
		needs   []string
		wantMsg string
	}{
		{
			name:    "cmd1 start failure",
			cmd1:    []string{"af-definitely-not-a-binary-xyz"},
			cmd2:    []string{"cat"},
			needs:   []string{"cat"},
			wantMsg: "diff cmd1 start: ",
		},
		{
			name:    "cmd2 start failure kills cmd1",
			cmd1:    []string{"cat"},
			cmd2:    []string{"af-definitely-not-a-binary-xyz"},
			needs:   []string{"cat"},
			wantMsg: "diff cmd2 start: ",
		},
		{
			name:    "cmd2 non-zero exit",
			cmd1:    []string{"sh", "-c", "printf x"},
			cmd2:    []string{"sh", "-c", "cat >/dev/null; exit 4"},
			needs:   []string{"sh", "cat"},
			wantMsg: "diff cmd2 wait: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, n := range tc.needs {
				requireBinary(t, n)
			}
			ex := diff.ExecExecutor{}
			err := ex.ExecutePipe(t.Context(), t.TempDir(), io.Discard, io.Discard, tc.cmd1, tc.cmd2)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("want message containing %q, got %q", tc.wantMsg, err.Error())
			}
		})
	}
}

// TestExecExecutor_ExecutePipeBrokenPipeTolerated verifies that cmd2 exiting
// before consuming all of cmd1's output is non-fatal (broken pipe on cmd1 is
// deliberately ignored).
func TestExecExecutor_ExecutePipeBrokenPipeTolerated(t *testing.T) {
	t.Parallel()
	requireBinary(t, "sh")
	requireBinary(t, "head")

	var out bytes.Buffer
	ex := diff.ExecExecutor{}
	cmd1 := []string{"sh", "-c", "i=0; while [ $i -lt 100000 ]; do echo line; i=$((i+1)); done"}
	cmd2 := []string{"head", "-n", "1"}
	err := ex.ExecutePipe(t.Context(), t.TempDir(), &out, io.Discard, cmd1, cmd2)
	if err != nil {
		t.Fatalf("ExecutePipe with early-exiting cmd2: %v", err)
	}
	if got := out.String(); got != "line\n" {
		t.Fatalf("want single line through head, got %q", got)
	}
}

// TestExecExecutor_ExecuteContextCancel verifies that a cancelled context
// terminates the child process and Execute reports an error.
func TestExecExecutor_ExecuteContextCancel(t *testing.T) {
	t.Parallel()
	requireBinary(t, "sh")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ex := diff.ExecExecutor{}
	err := ex.Execute(ctx, t.TempDir(), io.Discard, io.Discard, "sh", "-c", "sleep 10")
	if err == nil {
		t.Fatal("want error from cancelled context, got nil")
	}
}
