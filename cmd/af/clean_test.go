package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClean_DryRunListsCompletedWorkstreams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "done-work", "feat/done", "completed")
	writeTestSessionState(t, home, "active-work", "feat/active", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "clean", "--dry-run")
	if err != nil {
		t.Fatalf("clean --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "done-work") {
		t.Fatalf("stdout missing 'done-work'; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "would remove") {
		t.Fatalf("stdout missing 'would remove'; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "active-work") {
		t.Fatalf("stdout must not contain active workstream 'active-work'; got:\n%s", stdout)
	}
}

func TestClean_RemovesCompletedSessionDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "done-work", "feat/done", "completed")

	sessionDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "done-work")

	_, statErr := os.Stat(sessionDir)
	if statErr != nil {
		t.Fatalf("expected session dir to exist before clean: %v", statErr)
	}

	_, _, err := executeCommand(t, newRootCmd(), "clean")
	if err != nil {
		t.Fatalf("clean: %v", err)
	}

	_, statErr = os.Stat(sessionDir)
	if statErr == nil {
		t.Fatal("expected session dir to be removed after clean, but it still exists")
	}
}
