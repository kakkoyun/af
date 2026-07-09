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
	events, err := session.ReadLedgerTail(t.Context(), ledgerPath, 5)
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

// TestStatePathForSessionName_MissingDirHintsTmuxSessionName pins issue
// #24 Option B: when the session directory doesn't exist and the given
// name starts with "af-", the error suggests the corresponding
// `af resume` invocation with the prefix stripped.
func TestStatePathForSessionName_MissingDirHintsTmuxSessionName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "info", "af-demo")
	if err == nil {
		t.Fatal("info af-demo: error = nil, want not-found error")
	}
	if !strings.Contains(err.Error(), "session 'af-demo' not found") {
		t.Fatalf("error = %v, want the friendly not-found lead", err)
	}
	if !strings.Contains(err.Error(), "did you mean: af resume demo") {
		t.Fatalf("error = %v, want the did-you-mean hint", err)
	}
}

// TestStatePathForSessionName_MissingDirWithDoubleDashSkipsDidYouMean
// covers the fallback branch of issue #24 Option B: a stripped remainder
// containing "--" (workstream.Sanitize's path-separator encoding) is
// ambiguous to reverse, so the hint must not guess a workstream name —
// it only points at `af list`.
func TestStatePathForSessionName_MissingDirWithDoubleDashSkipsDidYouMean(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "info", "af-team--feature")
	if err == nil {
		t.Fatal("info af-team--feature: error = nil, want not-found error")
	}
	if !strings.Contains(err.Error(), "session 'af-team--feature' not found") {
		t.Fatalf("error = %v, want the friendly not-found lead", err)
	}
	if strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error = %v, must not guess a did-you-mean for an ambiguous '--' remainder", err)
	}
	if !strings.Contains(err.Error(), "see 'af list'") {
		t.Fatalf("error = %v, want the generic af-list hint", err)
	}
}

// TestStatePathForSessionName_MissingDirWithoutAFPrefixHasNoHint checks
// that ordinary workstream-name typos (no af- prefix) get the plain
// not-found message without any tmux-session-name hint.
func TestStatePathForSessionName_MissingDirWithoutAFPrefixHasNoHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "info", "nope")
	if err == nil {
		t.Fatal("info nope: error = nil, want not-found error")
	}
	if !strings.Contains(err.Error(), "session 'nope' not found") {
		t.Fatalf("error = %v, want the friendly not-found lead", err)
	}
	if strings.Contains(err.Error(), "hint:") {
		t.Fatalf("error = %v, must not add a tmux-session-name hint for a plain workstream name", err)
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

// TestSessionResolution_RejectsTraversalNames verifies session names from
// positional args, --session, and AF_SESSION cannot escape the state
// root — validation must hold at the resolve chokepoint, not only in
// create (ADR-069 containment).
func TestSessionResolution_RejectsTraversalNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "safe", "feat/safe", "active")

	t.Run("positional", func(t *testing.T) {
		_, _, err := executeCommand(t, newRootCmd(), "info", "../../../etc/passwd")
		if err == nil || !strings.Contains(err.Error(), "invalid session name") {
			t.Fatalf("info with traversal positional = %v, want invalid session name", err)
		}
	})
	t.Run("session flag", func(t *testing.T) {
		_, _, err := executeCommand(t, newRootCmd(), "--session", "../evil", "info")
		if err == nil || !strings.Contains(err.Error(), "invalid session name") {
			t.Fatalf("info with traversal --session = %v, want invalid session name", err)
		}
	})
	t.Run("env", func(t *testing.T) {
		t.Setenv("AF_SESSION", "../evil")
		_, _, err := executeCommand(t, newRootCmd(), "info")
		if err == nil || !strings.Contains(err.Error(), "invalid session name") {
			t.Fatalf("info with traversal AF_SESSION = %v, want invalid session name", err)
		}
	})
	t.Run("stack parent", func(t *testing.T) {
		_, _, err := executeCommand(t, newRootCmd(), "stack", "safe", "--parent", "../evil")
		if err == nil || !strings.Contains(err.Error(), "invalid session name") {
			t.Fatalf("stack with traversal --parent = %v, want invalid session name", err)
		}
	})
}
