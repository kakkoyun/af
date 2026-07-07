package session_test

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

// errUpdateBoom is a test-only sentinel propagated through Update's mutate.
var errUpdateBoom = errors.New("update boom")

// TestUpdate_AppliesMutationAtomically verifies Update composes
// WithLock + ReadState + mutate + WriteState into a single call, so
// callers no longer hand-roll the read-modify-write closure.
func TestUpdate_AppliesMutationAtomically(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")
	writeUpdateTestState(t, statePath, 0)

	err := session.Update(statePath, func(state *session.State) error {
		state.Session.MaxAgents = 7
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if got.Session.MaxAgents != 7 {
		t.Fatalf("MaxAgents = %d, want 7", got.Session.MaxAgents)
	}
}

// TestUpdate_SerializesConcurrentMutations pins the same lost-update
// guarantee as TestWithLock_SerializesReadModifyWrite, now through the
// higher-level Update API.
func TestUpdate_SerializesConcurrentMutations(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")
	writeUpdateTestState(t, statePath, 0)

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- session.Update(statePath, func(state *session.State) error {
				state.Session.MaxAgents++
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
	}

	got, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if got.Session.MaxAgents != workers {
		t.Fatalf("MaxAgents = %d, want %d (lost update)", got.Session.MaxAgents, workers)
	}
}

// TestUpdate_MutateErrorAbortsWithoutWriting verifies a mutate error
// both propagates and skips WriteState, so a failed mutation cannot
// half-apply.
func TestUpdate_MutateErrorAbortsWithoutWriting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")
	writeUpdateTestState(t, statePath, 3)

	err := session.Update(statePath, func(state *session.State) error {
		state.Session.MaxAgents = 999
		return errUpdateBoom
	})
	if !errors.Is(err, errUpdateBoom) {
		t.Fatalf("Update() error = %v, want %v", err, errUpdateBoom)
	}

	got, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if got.Session.MaxAgents != 3 {
		t.Fatalf("MaxAgents = %d, want unchanged 3 after mutate error", got.Session.MaxAgents)
	}
}

// TestUpdate_ReadStateErrorPropagates verifies a missing/corrupt state
// file surfaces its ReadState error rather than a nil State reaching
// mutate.
func TestUpdate_ReadStateErrorPropagates(t *testing.T) {
	t.Parallel()
	statePath := filepath.Join(t.TempDir(), "state.toml")

	called := false
	err := session.Update(statePath, func(*session.State) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("Update() error = nil, want ReadState error for missing file")
	}
	if called {
		t.Fatal("Update() called mutate despite ReadState failure")
	}
}

// TestUpdate_LockAcquisitionFailurePropagates checks Update surfaces
// WithLock's ghost-directory guard.
func TestUpdate_LockAcquisitionFailurePropagates(t *testing.T) {
	t.Parallel()
	statePath := filepath.Join(t.TempDir(), "gone", "state.toml")

	err := session.Update(statePath, func(*session.State) error { return nil })
	if err == nil {
		t.Fatal("Update() error = nil, want lock acquisition error")
	}
}

func writeUpdateTestState(t *testing.T, path string, maxAgents int) {
	t.Helper()
	state := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-000000000001",
			Name:      "update-demo",
			Status:    "active",
			CreatedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
			MaxAgents: maxAgents,
		},
	}
	err := session.WriteState(path, state)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}
