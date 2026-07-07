package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/pr"
	"github.com/kakkoyun/af/internal/session"
)

const (
	staleTestParent = "parent"
	staleTestMerged = "merged"
)

// TestRefreshPRCache_DoesNotClobberConcurrentWrites verifies the PR
// cache refresh re-reads state under the caller's lock instead of
// writing back a stale pre-lock snapshot, which would silently revert
// fields committed by a concurrent command.
func TestRefreshPRCache_DoesNotClobberConcurrentWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "racy", "feat/racy", "active")
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "racy", "state.toml")

	// Give the session a PR so the refresh path engages.
	onDisk, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	onDisk.PR.Number = 5
	err = session.WriteState(statePath, onDisk)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	// Caller takes its snapshot (as info/status/clean do before locking).
	snapshot, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState snapshot: %v", err)
	}

	// A concurrent command commits a stack link after the snapshot.
	concurrent := onDisk
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	concurrent.Stack.ParentSession = staleTestParent
	concurrent.Stack.LinkedAt = &now
	err = session.WriteState(statePath, concurrent)
	if err != nil {
		t.Fatalf("WriteState concurrent: %v", err)
	}

	restore := prRefreshFunc
	prRefreshFunc = func(_ context.Context, state *session.PRState, _ pr.Options) (pr.Result, error) {
		state.State = staleTestMerged
		return pr.Result{Old: "open", New: staleTestMerged, Changed: true}, nil
	}
	t.Cleanup(func() { prRefreshFunc = restore })

	err = refreshPRCacheForState(context.Background(), statePath, &snapshot, prCacheRefreshOptions{
		Command: "test",
		Force:   true,
	})
	if err != nil {
		t.Fatalf("refreshPRCacheForState: %v", err)
	}

	final, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState final: %v", err)
	}
	if final.Stack.ParentSession != staleTestParent {
		t.Fatalf("Stack.ParentSession = %q, want %q (concurrent write clobbered by stale snapshot)", final.Stack.ParentSession, staleTestParent)
	}
	if final.PR.State != staleTestMerged {
		t.Fatalf("PR.State = %q, want merged (refresh fields must still land)", final.PR.State)
	}
}
