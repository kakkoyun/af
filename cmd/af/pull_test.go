package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/session"
)

// writeTestSessionStateWithLease creates a session directory under the fake
// home with a SlicerWT lease set. Returns the sessions root so the test can
// set HOME.
func writeTestSessionStateWithLease(t *testing.T, home, name string, lease session.SlicerWTLeaseState) {
	t.Helper()
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name)
	err := os.MkdirAll(stateDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	statePath := filepath.Join(stateDir, "state.toml")
	s := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-000000000001",
			Name:      name,
			Status:    "active",
			CreatedAt: time.Now().UTC(),
		},
		Worktree: session.WorktreeState{
			Path:       home,
			Branch:     "feat/test",
			BaseBranch: "main",
			RepoSlug:   "github.com/owner/repo",
		},
		SlicerWT: session.SlicerWTState{
			VM:         "sbox-abc",
			Path:       home,
			PushedAt:   time.Now().UTC(),
			LeaseState: lease,
		},
	}
	err = session.WriteState(statePath, s)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

func writeTestSessionStateNoLease(t *testing.T, home, name string) {
	t.Helper()
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name)
	err := os.MkdirAll(stateDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	statePath := filepath.Join(stateDir, "state.toml")
	s := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-000000000002",
			Name:      name,
			Status:    "active",
			CreatedAt: time.Now().UTC(),
		},
		Worktree: session.WorktreeState{
			Path: home, Branch: "feat/x", BaseBranch: "main", RepoSlug: "github.com/owner/repo",
		},
	}
	err = session.WriteState(statePath, s)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

func TestPull_NoLeaseReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateNoLease(t, home, "no-lease-ws")

	_, _, err := executeCommand(t, newRootCmd(), "pull", "no-lease-ws")
	if err == nil {
		t.Fatal("expected error for workstream with no lease")
	}
	if !errors.Is(err, lifecycle.ErrPullNoLease) {
		t.Errorf("want ErrPullNoLease, got %v", err)
	}
}

func TestPull_AlreadyPulledReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithLease(t, home, "pulled-ws", session.SlicerWTLeasePulled)

	_, _, err := executeCommand(t, newRootCmd(), "pull", "pulled-ws")
	if err == nil {
		t.Fatal("expected error for already-pulled workstream")
	}
	if !errors.Is(err, lifecycle.ErrPullAlreadyPulled) {
		t.Errorf("want ErrPullAlreadyPulled, got %v", err)
	}
}
