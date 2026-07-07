package diff_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/diff"
)

// errBinaryNotFound is the sentinel returned by fakeLookPath for absent binaries.
var errBinaryNotFound = errors.New("binary not found")

const (
	// diffArgLiteral satisfies goconst for the repeated "diff" arg string.
	diffArgLiteral = "diff"
	// gitBin satisfies goconst for the repeated "git" arg string.
	gitBin = "git"
	// statArg satisfies goconst for the repeated "--stat" arg string.
	statArg = "--stat"
)

// fakeExecutor records calls and optionally returns an error.
type fakeExecutor struct { //nolint:govet // Readability over field alignment in test helper.
	calls []fakeCall
	err   error
}

type fakeCall struct {
	argv []string
	pipe bool // true when the call came from ExecutePipe
}

func (f *fakeExecutor) Execute(_ context.Context, _ string, stdout, _ io.Writer, argv ...string) error {
	f.calls = append(f.calls, fakeCall{argv: argv})
	_, _ = stdout.Write([]byte("fake: " + strings.Join(argv, " "))) //nolint:errcheck // test fake.
	return f.err
}

func (f *fakeExecutor) ExecutePipe(_ context.Context, _ string, stdout, _ io.Writer, cmd1, cmd2 []string) error {
	f.calls = append(f.calls, fakeCall{argv: cmd1, pipe: true}, fakeCall{argv: cmd2, pipe: true})
	_, _ = stdout.Write([]byte("fake-pipe: " + strings.Join(cmd1, " ") + " | " + strings.Join(cmd2, " "))) //nolint:errcheck // test fake.
	return f.err
}

// fakeLookPath returns an error when the name is in the missing set.
func fakeLookPath(missing ...string) func(string) (string, error) {
	set := make(map[string]bool, len(missing))
	for _, m := range missing {
		set[m] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "", fmt.Errorf("%w: %s", errBinaryNotFound, name)
		}
		return "/fake/bin/" + name, nil
	}
}

func makeOpts(mode diff.Mode, interactive bool) diff.Options {
	return diff.Options{
		Worktree:    "/repo",
		Base:        "main",
		Head:        "feat/x",
		Mode:        mode,
		Stdout:      io.Discard,
		Stderr:      io.Discard,
		Interactive: interactive,
	}
}

// TestRender_WebRequiresDiffity verifies that ModeWeb returns ErrDiffityMissing
// when diffity is absent.
func TestRender_WebRequiresDiffity(t *testing.T) {
	t.Parallel()

	ex := &fakeExecutor{}
	deps := diff.Deps{LookPath: fakeLookPath("diffity"), Exec: ex}
	err := diff.Render(t.Context(), deps, makeOpts(diff.ModeWeb, false))
	if !errors.Is(err, diff.ErrDiffityMissing) {
		t.Fatalf("want ErrDiffityMissing, got: %v", err)
	}
	if len(ex.calls) != 0 {
		t.Fatalf("no external command should run when diffity is missing, got %d calls", len(ex.calls))
	}
}

// TestRender_WebHappyPath verifies that ModeWeb invokes diffity with base..head.
func TestRender_WebHappyPath(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	ex := &fakeExecutor{}
	deps := diff.Deps{LookPath: fakeLookPath(), Exec: ex}
	opts := diff.Options{
		Worktree: "/repo", Base: "main", Head: "feat/x",
		Mode: diff.ModeWeb, Stdout: &out, Stderr: io.Discard,
	}
	err := diff.Render(t.Context(), deps, opts)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(ex.calls))
	}
	got := ex.calls[0].argv
	if got[0] != "diffity" {
		t.Fatalf("want diffity, got %s", got[0])
	}
	want := "main..feat/x"
	if got[1] != want {
		t.Fatalf("want range %q, got %q", want, got[1])
	}
}

// TestRender_NonInteractiveUsesStat verifies that non-interactive mode runs
// git diff --stat with the three-dot range.
func TestRender_NonInteractiveUsesStat(t *testing.T) {
	t.Parallel()

	ex := &fakeExecutor{}
	deps := diff.Deps{LookPath: fakeLookPath(), Exec: ex}
	err := diff.Render(t.Context(), deps, makeOpts(diff.ModeAuto, false))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(ex.calls))
	}
	argv := ex.calls[0].argv
	if argv[0] != gitBin || argv[1] != diffArgLiteral || argv[2] != statArg {
		t.Fatalf("want 'git diff --stat', got %v", argv)
	}
	// Three-dot range.
	if !strings.HasSuffix(argv[3], "...feat/x") {
		t.Fatalf("want three-dot range ending ...feat/x, got %q", argv[3])
	}
}

// TestRender_HunkUsedWhenInstalled verifies the interactive path chooses the
// git | hunk pipeline when hunk is on PATH.
func TestRender_HunkUsedWhenInstalled(t *testing.T) {
	t.Parallel()

	ex := &fakeExecutor{}
	deps := diff.Deps{LookPath: fakeLookPath(), Exec: ex}
	err := diff.Render(t.Context(), deps, makeOpts(diff.ModeAuto, true))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// ExecutePipe records two calls (cmd1 + cmd2).
	if len(ex.calls) != 2 {
		t.Fatalf("want 2 pipe calls, got %d: %+v", len(ex.calls), ex.calls)
	}
	if !ex.calls[0].pipe {
		t.Fatal("want pipe call, got plain Execute")
	}
	// cmd1 is git diff --no-color.
	cmd1 := ex.calls[0].argv
	if cmd1[0] != gitBin || cmd1[1] != diffArgLiteral || cmd1[2] != "--no-color" {
		t.Fatalf("want 'git diff --no-color ...', got %v", cmd1)
	}
	// cmd2 is hunk patch -.
	cmd2 := ex.calls[1].argv
	if cmd2[0] != "hunk" || cmd2[1] != "patch" || cmd2[2] != "-" {
		t.Fatalf("want 'hunk patch -', got %v", cmd2)
	}
}

// TestRender_FallsBackToGitWhenHunkMissing verifies the interactive path falls
// back to plain git diff when hunk is absent.
func TestRender_FallsBackToGitWhenHunkMissing(t *testing.T) {
	t.Parallel()

	ex := &fakeExecutor{}
	deps := diff.Deps{LookPath: fakeLookPath("hunk"), Exec: ex}
	err := diff.Render(t.Context(), deps, makeOpts(diff.ModeAuto, true))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(ex.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(ex.calls))
	}
	argv := ex.calls[0].argv
	if ex.calls[0].pipe {
		t.Fatal("want plain Execute, got pipe call")
	}
	if argv[0] != gitBin || argv[1] != diffArgLiteral {
		t.Fatalf("want 'git diff', got %v", argv)
	}
	// Must NOT contain --no-color (that's only for the hunk pipeline).
	for _, a := range argv {
		if a == "--no-color" {
			t.Fatal("--no-color must not appear in plain git diff fallback")
		}
	}
	// Must NOT contain --stat (that's the non-interactive path).
	for _, a := range argv {
		if a == statArg {
			t.Fatal("--stat must not appear in interactive plain git diff")
		}
	}
}

// TestRender_EmptyOptionsRejected verifies that missing required fields return
// ErrEmptyOptions before any external command is invoked.
func TestRender_EmptyOptionsRejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts diff.Options
	}{
		{"missing Worktree", diff.Options{Base: "main", Head: "feat/x"}},
		{"missing Base", diff.Options{Worktree: "/r", Head: "feat/x"}},
		{"missing Head", diff.Options{Worktree: "/r", Base: "main"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ex := &fakeExecutor{}
			err := diff.Render(t.Context(), diff.Deps{LookPath: fakeLookPath(), Exec: ex}, tc.opts)
			if !errors.Is(err, diff.ErrEmptyOptions) {
				t.Fatalf("want ErrEmptyOptions, got: %v", err)
			}
			if len(ex.calls) != 0 {
				t.Fatal("no external command should run when options are invalid")
			}
		})
	}
}
