package session_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

// TestLockFile_TimesOutWhenBusy pins the bounded-acquisition contract
// added for ADR-068 §4/SPEC §15.2: a contended lock must not block
// forever. AF_LOCK_TIMEOUT shortens the deadline so the test doesn't
// wait out the real default.
func TestLockFile_TimesOutWhenBusy(t *testing.T) {
	t.Setenv("AF_LOCK_TIMEOUT", "50ms")
	path := filepath.Join(t.TempDir(), "state.toml.lock")

	holder, err := session.LockFile(path, session.LockExclusive)
	if err != nil {
		t.Fatalf("LockFile(holder) error = %v", err)
	}
	defer func() { _ = holder.Unlock() }() //nolint:errcheck // Best-effort unlock at test teardown.

	start := time.Now()
	_, err = session.LockFile(path, session.LockExclusive)
	elapsed := time.Since(start)

	if !errors.Is(err, session.ErrLockBusy) {
		t.Fatalf("LockFile(contended) error = %v, want ErrLockBusy", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("LockFile(contended) error = %v, want it to mention %q", err, path)
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("LockFile(contended) returned after %v, want it to have waited out the timeout", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("LockFile(contended) took %v, want it bounded near the 50ms timeout", elapsed)
	}
}

// TestLockFile_InvalidTimeoutFallsBackToDefault verifies a malformed
// AF_LOCK_TIMEOUT does not collapse the deadline to zero (which would
// make every acquisition fail instantly); it falls back to the
// package default, which comfortably outlasts this test's short hold.
func TestLockFile_InvalidTimeoutFallsBackToDefault(t *testing.T) {
	t.Setenv("AF_LOCK_TIMEOUT", "not-a-duration")
	path := filepath.Join(t.TempDir(), "state.toml.lock")

	holder, err := session.LockFile(path, session.LockExclusive)
	if err != nil {
		t.Fatalf("LockFile(holder) error = %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = holder.Unlock() //nolint:errcheck // Best-effort unlock from the releasing goroutine.
		close(released)
	}()

	_, err = session.LockFile(path, session.LockExclusive)
	<-released
	if err != nil {
		t.Fatalf("LockFile(waiter) error = %v, want success once default timeout tolerates a 150ms hold", err)
	}
}

// TestLockFile_DefaultTimeoutUnaffectedByExistingContentionWindows
// pins that ordinary uncontended acquisition (the common case, and the
// case exercised by TestWithLock_SerializesReadModifyWrite) is
// unaffected by the bounded-acquisition change.
func TestLockFile_DefaultTimeoutUnaffectedByExistingContentionWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml.lock")
	lock, err := session.LockFile(path, session.LockExclusive)
	if err != nil {
		t.Fatalf("LockFile() error = %v", err)
	}
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
}
