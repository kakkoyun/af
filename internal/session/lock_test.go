package session_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

// errLockBoom is a test-only sentinel propagated through WithLock.
var errLockBoom = errors.New("boom")

func TestWithLock_SerializesReadModifyWrite(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")
	writeLockTestState(t, statePath, 0)

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- session.WithLock(statePath, func() error {
				state, err := session.ReadState(statePath)
				if err != nil {
					return fmt.Errorf("read state: %w", err)
				}
				state.Session.MaxAgents++
				err = session.WriteState(statePath, state)
				if err != nil {
					return fmt.Errorf("write state: %w", err)
				}
				return nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("WithLock: %v", err)
		}
	}

	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.Session.MaxAgents != workers {
		t.Fatalf("MaxAgents = %d, want %d (lost update)", state.Session.MaxAgents, workers)
	}
}

func TestWithLock_CreatesLockFileBesideState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")

	err := session.WithLock(statePath, func() error { return nil })
	if err != nil {
		t.Fatalf("WithLock: %v", err)
	}
	_, err = os.Stat(filepath.Join(dir, ".af.lock"))
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
}

func TestWithLock_PropagatesFnError(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")

	err := session.WithLock(statePath, func() error { return errLockBoom })
	if !errors.Is(err, errLockBoom) {
		t.Fatalf("WithLock = %v, want %v", err, errLockBoom)
	}
}

func writeLockTestState(t *testing.T, path string, maxAgents int) {
	t.Helper()
	state := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-000000000000",
			Name:      "lock-demo",
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

// TestWithLock_FailsWhenSessionDirMissing verifies WithLock refuses to
// materialize a ghost session directory: locking a session that a
// concurrent `af done` already archived must fail, not recreate the
// directory with only a lock file inside (which would satisfy the
// ADR-069 collision check forever).
func TestWithLock_FailsWhenSessionDirMissing(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "gone", "state.toml")

	err := session.WithLock(statePath, func() error { return nil })
	if err == nil {
		t.Fatal("WithLock on missing session dir succeeded, want error")
	}
	_, statErr := os.Stat(filepath.Dir(statePath))
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("session dir was recreated by WithLock: stat err = %v", statErr)
	}
}
