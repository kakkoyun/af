package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// LockFileName is the flock file kept beside state.toml that serializes
// read-modify-write sequences on a session's state and ledger.
const LockFileName = ".af.lock"

// WithLock runs fn while holding the exclusive flock at
// <dir(statePath)>/.af.lock. Every command that reads state.toml,
// mutates it, and writes it back must run the whole sequence under this
// lock so concurrent af processes cannot interleave. Acquisition blocks
// until the holder releases; flock is released by the kernel if the
// holding process dies, so a crash cannot wedge the session.
func WithLock(statePath string, fn func() error) error {
	dir := filepath.Dir(statePath)
	// Refuse to materialize a ghost session dir: LockFile would
	// MkdirAll, so locking a session that a concurrent `af done`
	// already archived must fail instead of recreating an empty
	// directory that the ADR-069 collision check then treats as live.
	_, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("session lock: session directory %s: %w", dir, err)
	}
	lockPath := filepath.Join(dir, LockFileName)
	lock, err := LockFile(lockPath, LockExclusive)
	if err != nil {
		return fmt.Errorf("session lock %s: %w", lockPath, err)
	}
	defer func() { _ = lock.Unlock() }() //nolint:errcheck // Best-effort unlock on return.
	return fn()
}
