package main

import (
	"fmt"
	"path/filepath"

	"github.com/kakkoyun/af/internal/session"
)

func withSessionLock(statePath string, fn func() error) error {
	lockPath := filepath.Join(filepath.Dir(statePath), ".af.lock")
	lock, err := session.LockFile(lockPath, session.LockExclusive)
	if err != nil {
		return fmt.Errorf("session lock %s: %w", lockPath, err)
	}
	defer func() { _ = lock.Unlock() }() //nolint:errcheck // Best-effort unlock on command exit.
	err = fn()
	if err != nil {
		return err
	}
	return nil
}
