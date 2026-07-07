package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/session"
)

func TestSessionResolution_SessionFlagOverridesPositional(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "alpha", "feat/alpha", "active")
	writeTestSessionState(t, home, "beta", "feat/beta", "active")

	stdout, stderr, err := executeCommand(t, newRootCmd(), "--session", "beta", "info", "alpha")
	if err != nil {
		t.Fatalf("info --session beta alpha: %v", err)
	}
	if !strings.Contains(stdout, "Session:   beta") {
		t.Fatalf("--session should override positional session; got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "--session") || !strings.Contains(stderr, "overrides positional") {
		t.Fatalf("stderr should warn that --session overrides positional arg; got %q", stderr)
	}
}

func TestSessionResolution_UsesAFSessionEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AF_SESSION", "env-work")
	writeTestSessionState(t, home, "env-work", "feat/env", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "info")
	if err != nil {
		t.Fatalf("info via AF_SESSION: %v", err)
	}
	if !strings.Contains(stdout, "Session:   env-work") {
		t.Fatalf("AF_SESSION should resolve env-work; got:\n%s", stdout)
	}
}

func TestSessionResolution_WalksUpCwdDiscoverySymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "nested-work", "feat/nested", "active")
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "nested-work", "state.toml")
	worktree := filepath.Join(home, "worktree")
	nested := filepath.Join(worktree, "a", "b")
	err := os.MkdirAll(filepath.Join(worktree, ".af"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(nested, 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(statePath, filepath.Join(worktree, ".af", "state.toml"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	_, _, err = executeCommand(t, newRootCmd(), "note", "--append", "from nested cwd")
	if err != nil {
		t.Fatalf("note from nested cwd: %v", err)
	}
	ledgerPath := filepath.Join(filepath.Dir(statePath), "ledger.jsonl")
	events, err := session.ReadLedgerTail(ledgerPath, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[len(events)-1].Type != "note" {
		t.Fatalf("note should append to inferred session ledger; got %+v", events)
	}
}

func TestSessionResolution_NoInputReturnsHelpfulError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AF_SESSION", "")

	_, _, err := executeCommand(t, newRootCmd(), "info")
	if !errors.Is(err, errSessionResolutionNoInput) {
		t.Fatalf("want errSessionResolutionNoInput, got %v", err)
	}
	if !strings.Contains(err.Error(), "pass [session]") || !strings.Contains(err.Error(), "AF_SESSION") {
		t.Fatalf("error should include resolution hints, got %v", err)
	}
}

// errFakeFzfExit simulates a non-zero fzf exit (e.g. Esc) in tests.
var errFakeFzfExit = errors.New("exit status 130")

// TestDefaultSessionPicker_ParsesFzfSelection drives the picker through
// the fzf command seam: rows are rendered to fzf stdin and the selected
// row's first field becomes the session name.
func TestDefaultSessionPicker_ParsesFzfSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "picked", "feat/picked", "active")
	writeTestSessionState(t, home, "other", "feat/other", "active")
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions")

	var sawInput string
	restore := fzfCommandFunc
	fzfCommandFunc = func(_ context.Context, input string, _ io.Writer) ([]byte, error) {
		sawInput = input
		return []byte("picked\tactive\tfeat/picked\n"), nil
	}
	t.Cleanup(func() { fzfCommandFunc = restore })

	selected, err := defaultSessionPicker(context.Background(), sessionPickerOptions{StateDir: stateDir})
	if err != nil {
		t.Fatalf("defaultSessionPicker: %v", err)
	}
	if selected != "picked" {
		t.Fatalf("selected = %q, want picked", selected)
	}
	if !strings.Contains(sawInput, "other") {
		t.Fatalf("fzf input missing candidate rows; got %q", sawInput)
	}
}

// TestDefaultSessionPicker_FzfFailureIsInterrupted maps a non-zero fzf
// exit (e.g. Esc) to errSessionPickerInterrupted.
func TestDefaultSessionPicker_FzfFailureIsInterrupted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "one", "feat/one", "active")
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions")

	restore := fzfCommandFunc
	fzfCommandFunc = func(context.Context, string, io.Writer) ([]byte, error) {
		return nil, errFakeFzfExit
	}
	t.Cleanup(func() { fzfCommandFunc = restore })

	_, err := defaultSessionPicker(context.Background(), sessionPickerOptions{StateDir: stateDir})
	if !errors.Is(err, errSessionPickerInterrupted) {
		t.Fatalf("err = %v, want errSessionPickerInterrupted", err)
	}
}
