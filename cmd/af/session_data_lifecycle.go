package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
)

// errSessionDataAutoSyncFailed reports a teardown-blocking sync failure
// per ADR-067 §Lifecycle rule.
var errSessionDataAutoSyncFailed = errors.New("session-data: automatic sync blocked teardown")

// autoSyncBeforeTeardown runs an ADR-067 automatic session-data sync
// for a slicer-backed workstream just before a destructive lifecycle
// step (suspend or done). The sync is skipped when:
//
//   - the workstream has no slicer VM (state.SlicerWT.VM == "");
//   - the caller passed --discard (acknowledging transcript loss);
//   - the lease is already discarded or pulled (no live VM expected).
//
// On a successful sync the state is written back via the same path used
// by `af session-data sync`. On a sync error or any conflicts, the
// function returns errSessionDataAutoSyncFailed so the caller can abort
// teardown and print the recovery hint.
func autoSyncBeforeTeardown(cmd *cobra.Command, state session.State, statePath string, discard bool) error {
	if state.SlicerWT.VM == "" {
		return nil
	}
	if discard {
		return recordDiscardedSync(state, statePath)
	}
	if state.SlicerWT.LeaseState == session.SlicerWTLeaseDiscarded ||
		state.SlicerWT.LeaseState == session.SlicerWTLeasePulled {
		// VM was already pulled or explicitly discarded — sync is
		// either a no-op or impossible. Skip without blocking.
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("auto-sync: resolve home: %w", err)
	}
	slicer := sessiondataSlicerFactory()
	result, err := sessiondata.Sync(cmd.Context(), slicer, sessiondata.SyncOptions{
		Session: state.Session.Name,
		VM:      state.SlicerWT.VM,
		HomeDir: home,
		Now:     time.Now,
	})
	if err != nil {
		printAutoSyncRecoveryHint(cmd, state, err)
		return fmt.Errorf("%w: %w", errSessionDataAutoSyncFailed, err)
	}
	// Auto-sync before teardown never requests --continue-host: it is an
	// analysis-only safety net (ADR-066/ADR-067), not a user-directed
	// resume-on-host request.
	err = emitSyncEvent(state, result, false)
	if err != nil {
		return fmt.Errorf("auto-sync: emit ledger event: %w", err)
	}
	err = writebackSessionExport(statePath, state, result)
	if err != nil {
		return fmt.Errorf("auto-sync: writeback state: %w", err)
	}
	if result.Merge.Conflicts > 0 {
		printAutoSyncRecoveryHint(cmd, state, nil)
		return fmt.Errorf("%w: %d conflict(s) quarantined under %s",
			errSessionDataAutoSyncFailed, result.Merge.Conflicts, result.StagingPath)
	}
	writef(cmd.ErrOrStderr(),
		"session-data: auto-synced before teardown (imported=%d skipped=%d staging=%s)\n",
		result.Merge.Imported, result.Merge.Skipped, result.StagingPath)
	return nil
}

// recordDiscardedSync sets last_sync_status=discarded so the audit
// trail reflects the deliberate skip per ADR-067.
func recordDiscardedSync(state session.State, statePath string) error {
	now := time.Now().UTC()
	state.SessionExport = session.ExportState{
		LastSyncAt:     &now,
		LastSyncStatus: session.ExportSyncDiscarded,
	}
	err := session.WriteState(statePath, state)
	if err != nil {
		return fmt.Errorf("auto-sync: record discarded sync: %w", err)
	}
	return nil
}

func printAutoSyncRecoveryHint(cmd *cobra.Command, state session.State, syncErr error) {
	writef(cmd.ErrOrStderr(),
		"session-data: automatic sync failed for %s (vm=%s). Recovery:\n"+
			"  - run `af session-data sync %s` and address conflicts under <staging>/conflicts/, or\n"+
			"  - re-run this command with --discard to acknowledge transcript loss.\n",
		state.Session.Name, state.SlicerWT.VM, state.Session.Name)
	if syncErr != nil {
		writef(cmd.ErrOrStderr(), "  reason: %v\n", syncErr)
	}
}

// readStateForAutoSync loads state.toml for the auto-sync helper. The
// suspend/done commands resolve their own statePath; this thin wrapper
// keeps the read+error wrapping consistent.
func readStateForAutoSync(_ context.Context, statePath string) (session.State, error) {
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, fmt.Errorf("read state %s: %w", statePath, err)
	}
	return state, nil
}
