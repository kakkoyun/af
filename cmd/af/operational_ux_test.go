package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

var errOperationalUXBoom = errors.New("boom")

func TestExitCodeForErrorMapsKnownClasses(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{err: errSessionResolutionNoInput, want: exitNoInput},
		{err: errPRRefreshNoPR, want: exitDataErr},
		{err: errSessionPickerInterrupted, want: exitInterrupted},
		{err: cobra.ExactArgs(1)(nil, []string{}), want: exitUsage},
		{err: errOperationalUXBoom, want: exitGeneral},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			if got := exitCodeForError(tt.err); got != tt.want {
				t.Fatalf("exitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestNote_CreatesSessionLockFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "locked-note", "feat/locked", "active")

	_, _, err := executeCommand(t, newRootCmd(), "note", "locked-note", "--append", "lock me")
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	lockPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "locked-note", ".af.lock")
	_, err = os.Stat(lockPath)
	if err != nil {
		t.Fatalf("lock file %s missing: %v", lockPath, err)
	}
}

func TestCompletionSessionFlagCompletesWorkstreams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "alpha", "feat/alpha", "active")
	writeTestSessionState(t, home, "beta", "feat/beta", "suspended")

	comps, directive := completeSessionNames(newRootCmd(), nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %v, want NoFileComp", directive)
	}
	for _, want := range []string{"alpha", "beta"} {
		if !slices.Contains(comps, want) {
			t.Fatalf("completions missing %q: %v", want, comps)
		}
	}
}
