package main

import (
	"github.com/kakkoyun/af/internal/session"
)

// withSessionLock serializes a read-modify-write sequence on a session's
// state.toml/ledger against concurrent af processes. Every command that
// mutates session state must run the whole sequence under this lock.
func withSessionLock(statePath string, fn func() error) error {
	return session.WithLock(statePath, fn) //nolint:wrapcheck // Lock errors are wrapped by session.WithLock; fn errors carry command context.
}
