package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LockFileName is the flock file kept beside state.toml that serializes
// read-modify-write sequences on a session's state and ledger.
const LockFileName = ".af.lock"

// ErrSessionDirNotFound reports that a workstream's session directory
// does not exist. cmd/af's resolution chokepoint (statePathForSessionName)
// checks for this ahead of time so commands fail with a friendly
// "session '<name>' not found" message (issue #24/#25); WithDirLock's own
// check below is the same guard kept as defence-in-depth against races.
var ErrSessionDirNotFound = errors.New("session directory not found")

// WithLock runs fn while holding the exclusive flock at
// <dir(statePath)>/.af.lock. Every command that reads state.toml,
// mutates it, and writes it back must run the whole sequence under this
// lock so concurrent af processes cannot interleave. Acquisition blocks
// until the holder releases or AF_LOCK_TIMEOUT elapses; flock is
// released by the kernel if the holding process dies, so a crash cannot
// wedge the session.
func WithLock(statePath string, fn func() error) error {
	return WithDirLock(filepath.Dir(statePath), fn)
}

// WithDirLock runs fn while holding the exclusive flock at
// <dir>/.af.lock. It is the base primitive behind WithLock (which locks
// the directory containing a state.toml) and the state-root lock used
// by workstream creation (ADR-069 §3 collision semantics). Acquisition
// blocks until the holder releases or AF_LOCK_TIMEOUT elapses; flock is
// released by the kernel if the holding process dies, so a crash cannot
// wedge the directory.
func WithDirLock(dir string, fn func() error) error {
	// Refuse to materialize a ghost directory: LockFile would MkdirAll,
	// so locking a session that a concurrent `af done` already archived
	// must fail instead of recreating an empty directory that the
	// ADR-069 collision check then treats as live.
	_, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("session lock: session directory %s: %w: %w", dir, ErrSessionDirNotFound, err)
	}
	lockPath := filepath.Join(dir, LockFileName)
	lock, err := LockFile(lockPath, LockExclusive)
	if err != nil {
		return fmt.Errorf("session lock %s: %w", lockPath, err)
	}
	defer func() { _ = lock.Unlock() }() //nolint:errcheck // Best-effort unlock on return.
	return fn()
}
