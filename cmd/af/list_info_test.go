package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

func writeTestSessionState(t *testing.T, home, name, branch, status string) {
	t.Helper()
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name)
	err := os.MkdirAll(stateDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	statePath := filepath.Join(stateDir, "state.toml")
	state := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-000000000000",
			Name:      name,
			Status:    status,
			CreatedAt: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
		},
		Worktree: session.WorktreeState{
			Path:       filepath.Join(home, "wt", name),
			Branch:     branch,
			BaseBranch: "main",
			RepoSlug:   "github.com/owner/repo",
		},
	}
	err = session.WriteState(statePath, state)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

func TestList_EmptyDirectoryPrintsNoWorkstreams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout, _, err := executeCommand(t, newRootCmd(), "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, "no workstreams") {
		t.Fatalf("expected 'no workstreams'; got %q", stdout)
	}
}

func TestList_PrintsOneLinePerWorkstream(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "alpha", "kakkoyun/alpha", "active")
	writeTestSessionState(t, home, "beta", "kakkoyun/beta", "suspended")

	stdout, _, err := executeCommand(t, newRootCmd(), "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"alpha", "beta", "kakkoyun/alpha", "kakkoyun/beta", "active", "suspended"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("list output missing %q; got:\n%s", want, stdout)
		}
	}
}

func TestInfo_BySessionName_PrintsTextSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "alpha", "kakkoyun/alpha", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "info", "alpha")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	for _, want := range []string{"Session:   alpha", "Status:    active", "Branch:    kakkoyun/alpha"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("info text missing %q; got:\n%s", want, stdout)
		}
	}
}

func TestInfo_JSONMode_EmitsParsableJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "alpha", "kakkoyun/alpha", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "info", "alpha", "--json")
	if err != nil {
		t.Fatalf("info --json: %v", err)
	}
	if !strings.Contains(stdout, `"Name": "alpha"`) {
		t.Fatalf("info --json missing alpha name; got:\n%s", stdout)
	}
}

func TestInfo_MissingSessionReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "info", "nope")
	if err == nil {
		t.Fatal("info missing session returned nil, want error")
	}
}
