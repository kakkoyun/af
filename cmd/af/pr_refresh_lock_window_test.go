package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/pr"
	"github.com/kakkoyun/af/internal/session"
)

// concurrentWaitTimeout bounds how long a concurrent operation is
// allowed to take in these tests. It is generous compared to the
// deterministic in-process blocking these tests use, but still catches
// a regression where the session lock is held across the fake network
// fetch (which would otherwise block until AF_LOCK_TIMEOUT, ~30s).
const concurrentWaitTimeout = 5 * time.Second

// TestRefreshPRCacheForState_DoesNotHoldLockAcrossNetworkFetch is the
// issue #3 acceptance test: refreshPRCacheForState must not hold the
// session flock while prRefreshFunc (the gh pr view network seam) is
// in flight. A fake prRefreshFunc signals "fetch started" and then
// blocks; while it is blocked, a concurrent session.Update on the same
// statePath must complete quickly. Once the fake is released, the
// refresh finishes and both the refreshed PR field and the concurrent
// update's field must be present in the final state (no clobber).
func TestRefreshPRCacheForState_DoesNotHoldLockAcrossNetworkFetch(t *testing.T) { //nolint:cyclop,funlen // Test exercises the full fetch-blocks/concurrent-update/release/no-clobber sequence in one deterministic scenario.
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "lockwin", "feat/lockwin", "active")
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "lockwin", "state.toml")

	onDisk, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	onDisk.PR.Number = 11
	err = session.WriteState(statePath, onDisk)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	fetchStarted := make(chan struct{})
	release := make(chan struct{})
	restore := prRefreshFunc
	prRefreshFunc = func(_ context.Context, prState *session.PRState, _ pr.Options) (pr.Result, error) {
		close(fetchStarted)
		<-release
		old := prState.State
		prState.State = pr.StateMerged
		return pr.Result{Old: old, New: pr.StateMerged, Changed: old != pr.StateMerged}, nil
	}
	t.Cleanup(func() { prRefreshFunc = restore })

	state := onDisk
	refreshDone := make(chan error, 1)
	go func() {
		refreshDone <- refreshPRCacheForState(context.Background(), statePath, &state, prCacheRefreshOptions{
			Command: "test",
			Force:   true,
		})
	}()

	select {
	case <-fetchStarted:
	case <-time.After(concurrentWaitTimeout):
		t.Fatal("prRefreshFunc was never invoked")
	}

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- session.Update(statePath, func(s *session.State) error {
			s.Stack.ParentSession = "concurrent-parent"
			return nil
		})
	}()

	select {
	case updateErr := <-updateDone:
		if updateErr != nil {
			t.Fatalf("concurrent session.Update failed: %v", updateErr)
		}
	case <-time.After(concurrentWaitTimeout):
		t.Fatal("concurrent session.Update did not complete while the PR refresh was blocked in prRefreshFunc — the session lock is held across the network fetch")
	}

	close(release)

	select {
	case refreshErr := <-refreshDone:
		if refreshErr != nil {
			t.Fatalf("refreshPRCacheForState: %v", refreshErr)
		}
	case <-time.After(concurrentWaitTimeout):
		t.Fatal("refreshPRCacheForState did not finish after prRefreshFunc was released")
	}

	final, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState final: %v", err)
	}
	if final.PR.State != pr.StateMerged {
		t.Fatalf("PR.State = %q, want %q (refresh fields must land)", final.PR.State, pr.StateMerged)
	}
	if final.Stack.ParentSession != "concurrent-parent" {
		t.Fatalf("Stack.ParentSession = %q, want concurrent-parent (concurrent Update must not be clobbered)", final.Stack.ParentSession)
	}
}

// TestRefreshPRCacheForState_SessionArchivedDuringFetchFailsWithoutResurrecting
// pins the race semantics required when release-call-reacquire is used:
// if a racing `af done` archives the session between the network fetch
// and the merge-back re-read, the refresh must fail instead of
// recreating the session directory.
func TestRefreshPRCacheForState_SessionArchivedDuringFetchFailsWithoutResurrecting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "archme", "feat/archme", "active")
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "archme", "state.toml")
	sessionDir := filepath.Dir(statePath)

	onDisk, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	onDisk.PR.Number = 22
	err = session.WriteState(statePath, onDisk)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	restore := prRefreshFunc
	prRefreshFunc = func(_ context.Context, prState *session.PRState, _ pr.Options) (pr.Result, error) {
		// Simulate a racing `af done` archiving the session mid-fetch,
		// i.e. after the network call started but before the
		// merge-back critical section re-reads state.
		removeErr := os.RemoveAll(sessionDir)
		if removeErr != nil {
			t.Fatalf("simulate concurrent archive: %v", removeErr)
		}
		prState.State = pr.StateMerged
		return pr.Result{Old: pr.StateOpen, New: pr.StateMerged, Changed: true}, nil
	}
	t.Cleanup(func() { prRefreshFunc = restore })

	state := onDisk
	err = refreshPRCacheForState(context.Background(), statePath, &state, prCacheRefreshOptions{
		Command: "test",
		Force:   true,
	})
	if err == nil {
		t.Fatal("refreshPRCacheForState should fail when the session is archived mid-fetch")
	}
	_, statErr := os.Stat(sessionDir)
	if !os.IsNotExist(statErr) {
		t.Fatalf("session directory should stay gone (no resurrection); stat err = %v", statErr)
	}
}
