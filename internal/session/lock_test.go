package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

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
					return err
				}
				state.Session.MaxAgents++
				return session.WriteState(statePath, state)
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
	sentinel := errors.New("boom")

	err := session.WithLock(statePath, func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithLock = %v, want %v", err, sentinel)
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
