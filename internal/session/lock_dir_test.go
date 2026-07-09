package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/session"
)

// TestWithDirLock_CreatesLockFileInDir verifies the base primitive locks
// directly on a directory (not derived from a state.toml path), which is
// what the state-root lock and any future non-session-state lock needs.
func TestWithDirLock_CreatesLockFileInDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ran := false

	err := session.WithDirLock(dir, func() error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithDirLock() error = %v", err)
	}
	if !ran {
		t.Fatal("WithDirLock() did not run fn")
	}
	_, err = os.Stat(filepath.Join(dir, session.LockFileName))
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
}

// TestWithDirLock_FailsWhenDirMissing pins the ghost-directory guard:
// WithDirLock must refuse to materialize a directory that does not
// already exist, exactly like WithLock does for session directories.
func TestWithDirLock_FailsWhenDirMissing(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "gone")

	err := session.WithDirLock(dir, func() error { return nil })
	if err == nil {
		t.Fatal("WithDirLock() on missing dir succeeded, want error")
	}
	if !errors.Is(err, session.ErrSessionDirNotFound) {
		t.Fatalf("WithDirLock() on missing dir = %v, want ErrSessionDirNotFound", err)
	}
	_, statErr := os.Stat(dir)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dir was recreated by WithDirLock: stat err = %v", statErr)
	}
}

// TestWithDirLock_PropagatesFnError checks fn's error surfaces unwrapped
// through errors.Is, matching WithLock's contract.
func TestWithDirLock_PropagatesFnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := session.WithDirLock(dir, func() error { return errLockBoom })
	if !errors.Is(err, errLockBoom) {
		t.Fatalf("WithDirLock() = %v, want %v", err, errLockBoom)
	}
}

// TestWithLock_DelegatesToWithDirLock pins that WithLock is now a thin
// wrapper: it locks filepath.Dir(statePath), so passing a dir with the
// same basename layout produces the same lock file location and error
// text as before the refactor.
func TestWithLock_DelegatesToWithDirLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.toml")

	err := session.WithLock(statePath, func() error { return nil })
	if err != nil {
		t.Fatalf("WithLock() error = %v", err)
	}
	_, err = os.Stat(filepath.Join(dir, session.LockFileName))
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
}

func TestWithDirLock_LockAcquisitionFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, session.LockFileName)
	err := os.MkdirAll(blocker, 0o750)
	if err != nil {
		t.Fatalf("seed blocker dir: %v", err)
	}

	err = session.WithDirLock(dir, func() error { return nil })
	if err == nil || !strings.Contains(err.Error(), "session lock") {
		t.Fatalf("WithDirLock() error = %v, want session lock context", err)
	}
}
