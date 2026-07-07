package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/pr"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

type prCacheRefreshOptions struct {
	Command   string
	Force     bool
	RequirePR bool
}

// refreshPRCacheForState applies the ADR-071 PR state cache policy to
// one state.toml using release-call-reacquire (issue #3): callers must
// NOT hold the session lock when calling this function. The gh pr view
// network call (via prRefreshFunc) runs entirely outside any lock, on
// a copy of the cached PR state, so a slow GitHub round trip never
// stalls a concurrent af command on this session. Only the short
// merge-back (see mergeBackPRRefresh) — re-read state, merge in the
// refreshed PR fields, write, and emit the ledger event on a flip —
// runs under session.WithLock. If a racing `af done` archives the
// session between the fetch and the merge-back, the re-read fails and
// this function returns that error without writing anything back (an
// archived session is never resurrected). On success the passed
// *session.State is updated in place with the final merged state so
// callers keep displaying current data. Callers decide whether refresh
// errors are soft (status/info) or hard (clean/sync/done).
//
// af done's teardown does not call this function: releasing the lock
// mid-done would let a concurrent command observe or write into a
// session that is being archived, so it stays on refreshPRCacheLocked
// instead (see the comment in done.go).
func refreshPRCacheForState(ctx context.Context, statePath string, state *session.State, opts prCacheRefreshOptions) error {
	current, err := session.ReadState(statePath)
	if err != nil {
		return fmt.Errorf("read state before PR refresh: %w", err)
	}
	if current.PR.Number == 0 {
		*state = current
		if opts.RequirePR {
			return pr.ErrNoPR
		}
		return nil
	}
	cfg, err := loadConfigForPRRefresh(ctx, current.Worktree.Path)
	if err != nil {
		return err
	}
	refreshedPR := current.PR
	result, refreshErr := prRefreshFunc(ctx, &refreshedPR, pr.Options{
		Runner:   sandbox.ExecRunner{},
		RepoSlug: current.Worktree.RepoSlug,
		TTL:      cfg.PR.RefreshTTL,
		Force:    opts.Force,
		Now:      time.Now,
	})
	return mergeBackPRRefresh(statePath, state, refreshedPR, result, refreshErr)
}

// mergeBackPRRefresh runs the short critical section a
// release-call-reacquire refresh needs once its network call has
// already completed: re-acquire the session lock, re-read state.toml
// (failing rather than resurrecting the session if it was archived
// meanwhile), merge only the refreshed PR fields into that fresh read,
// persist the outcome (write + ledger emit share this same critical
// section), and copy the final state back into the caller's pointer.
// Shared by refreshPRCacheForState and `af pr --refresh`.
func mergeBackPRRefresh(statePath string, state *session.State, refreshedPR session.PRState, result pr.Result, refreshErr error) error {
	return session.WithLock(statePath, func() error { //nolint:wrapcheck // Errors below already carry their own context; WithLock's own errors are lock-acquisition failures callers expect verbatim (session.ErrLockBusy mapping).
		fresh, readErr := session.ReadState(statePath)
		if readErr != nil {
			return fmt.Errorf("reread state after PR refresh: %w", readErr)
		}
		// A skipped refresh made no network call and mutated nothing, so
		// the re-read is the freshest truth (a concurrent refresh may have
		// landed since the pre-fetch read) — merge the fetched copy only
		// when the refresh actually ran, or errored (persisting
		// LastRefreshError). Mirrors persistPRRefreshOutcome's write gate.
		if refreshErr != nil || !result.Skipped {
			fresh.PR = refreshedPR
		}
		outcomeErr := persistPRRefreshOutcome(statePath, &fresh, result, refreshErr)
		*state = fresh
		return outcomeErr
	})
}

// refreshPRCacheLocked applies the same ADR-071 PR state cache policy
// as refreshPRCacheForState but for callers that already hold the
// session lock end-to-end and must keep the network call inside that
// same critical section — currently only `af done`'s teardown (see
// the comment in done.go). Calling refreshPRCacheForState there
// instead would try to re-acquire a flock this process already holds
// and block until AF_LOCK_TIMEOUT.
func refreshPRCacheLocked(ctx context.Context, statePath string, state *session.State, opts prCacheRefreshOptions) error {
	fresh, err := session.ReadState(statePath)
	if err != nil {
		return fmt.Errorf("reread state before PR refresh: %w", err)
	}
	*state = fresh
	if state.PR.Number == 0 {
		if opts.RequirePR {
			return pr.ErrNoPR
		}
		return nil
	}
	cfg, err := loadConfigForPRRefresh(ctx, state.Worktree.Path)
	if err != nil {
		return err
	}
	result, refreshErr := prRefreshFunc(ctx, &state.PR, pr.Options{
		Runner:   sandbox.ExecRunner{},
		RepoSlug: state.Worktree.RepoSlug,
		TTL:      cfg.PR.RefreshTTL,
		Force:    opts.Force,
		Now:      time.Now,
	})
	return persistPRRefreshOutcome(statePath, state, result, refreshErr)
}

// persistPRRefreshOutcome writes the refreshed state back when the
// refresh attempt changed persistent fields and emits the
// pr_state_changed ledger event on a flip. Callers must already hold
// the session lock: the ledger append has to share the same critical
// section as the write, and this is called both from inside
// mergeBackPRRefresh's re-acquired lock (network call already ran
// outside any lock) and from refreshPRCacheLocked's caller-held lock
// (network call ran inside that lock, by design for af done).
func persistPRRefreshOutcome(statePath string, state *session.State, result pr.Result, refreshErr error) error {
	if refreshErr != nil || !result.Skipped {
		writeErr := session.WriteState(statePath, *state) //nolint:forbidigo // Ledger emit below must share this critical section with the write; the gh network call itself no longer runs inside any lock (see mergeBackPRRefresh), so this can't collapse into session.Update.
		if writeErr != nil {
			return fmt.Errorf("write refreshed PR state: %w", writeErr)
		}
	}
	if refreshErr != nil {
		return refreshErr
	}
	if result.Changed {
		emitErr := emitPRStateChangedEvent(statePath, state, result)
		if emitErr != nil {
			return emitErr
		}
	}
	return nil
}

func loadConfigForPRRefresh(ctx context.Context, repoDir string) (config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Config{}, fmt.Errorf("resolve home: %w", err)
	}
	loaded, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: filepath.Join(home, ".config", "af", "config.toml"),
		RepoDir:        repoDir,
	})
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return loaded, nil
}

func warnPRRefreshOnce(ctx context.Context, warned *bool, command string, err error) {
	if *warned {
		return
	}
	*warned = true
	slog.WarnContext(ctx, "PR state refresh failed; rendering stale PR state as ?", "command", command, "error", err)
}
