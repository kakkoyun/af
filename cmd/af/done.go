package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

type doneOptions struct {
	root    *rootOptions
	force   bool
	discard bool
}

func newDoneCmd(opts *rootOptions) *cobra.Command {
	dOpts := &doneOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "done [session]",
		Short: "Complete (or --force abandon) a workstream and archive its state",
		Long:  "done tears down the tmux session, removes the git worktree(s), records a terminal lifecycle event, and moves the session dir into the archive. --force marks the workstream Abandoned and skips merged-into-base checks.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runDone(cmd, dOpts, name)
		},
	}
	cmd.Flags().BoolVar(&dOpts.force, "force", false, "abandon rather than complete; skip safety checks")
	cmd.Flags().BoolVar(&dOpts.discard, "discard", false, "discard agent session transcripts; skip ADR-067 automatic sync before VM teardown")
	return cmd
}

func runDone(cmd *cobra.Command, opts *doneOptions, name string) error {
	statePath, err := resolveDoneStatePath(cmd, name)
	if err != nil {
		return err
	}
	var state session.State
	err = withSessionLock(statePath, func() error {
		var lockedErr error
		state, lockedErr = finishWorkstreamLocked(cmd, opts, statePath)
		return lockedErr
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "workstream %s -> %s\n", state.Session.Name, state.Session.Status)
	if err != nil {
		return fmt.Errorf("done write: %w", err)
	}
	return nil
}

// finishWorkstreamLocked runs the done pipeline (auto-sync, PR cache
// refresh, teardown + archive) and must be called under the session
// lock.
func finishWorkstreamLocked(cmd *cobra.Command, opts *doneOptions, statePath string) (session.State, error) {
	preState, err := readStateForAutoSync(cmd.Context(), statePath)
	if err != nil {
		return session.State{}, fmt.Errorf("done: %w", err)
	}
	err = autoSyncBeforeTeardown(cmd, preState, statePath, opts.discard)
	if err != nil {
		return session.State{}, fmt.Errorf("done: %w", err)
	}
	stateForRefresh, err := readStateForAutoSync(cmd.Context(), statePath)
	if err != nil {
		return session.State{}, fmt.Errorf("done: %w", err)
	}
	if stateForRefresh.PR.Number != 0 {
		// Deliberately runs the gh pr view network call inside this
		// already-held session lock (refreshPRCacheLocked), unlike
		// status/info/clean/sync which use the release-call-reacquire
		// refreshPRCacheForState (issue #3). Releasing the lock
		// mid-done would let a concurrent command read stale state or
		// write into a session that is in the middle of being torn
		// down and archived; done's whole pipeline is meant to be one
		// atomic critical section, so the PR refresh stays inside it.
		err = refreshPRCacheLocked(cmd.Context(), statePath, &stateForRefresh, prCacheRefreshOptions{
			Command: "done",
			Force:   true,
		})
		if err != nil {
			return session.State{}, fmt.Errorf("done: refresh PR state: %w", err)
		}
	}
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		home = ""
	}
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	state, err := lifecycle.FinishWorkstream(cmd.Context(), lifecycle.DoneDeps{
		Git: git.NewExecRunner(),
		Mux: mux.NewTmux(),
	}, lifecycle.DoneOptions{
		StatePath:  statePath,
		ArchiveDir: archiveDir,
		Force:      opts.force,
	})
	if err != nil {
		return session.State{}, fmt.Errorf("done: %w", err)
	}
	// ADR-068 §4: the lock file must not persist in the archive. The
	// rename moved it along with the session dir; the held fd keeps the
	// flock valid, so unlinking the archived name is safe.
	_ = os.Remove(filepath.Join(archiveDir, state.Session.Name, session.LockFileName)) //nolint:errcheck // Best-effort cleanup per ADR-068.
	return state, nil
}

func resolveDoneStatePath(cmd *cobra.Command, name string) (string, error) {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return "", fmt.Errorf("done: %w", err)
	}
	return statePath, nil
}
