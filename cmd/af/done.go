package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
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
	preState, err := readStateForAutoSync(cmd.Context(), statePath)
	if err != nil {
		return fmt.Errorf("done: %w", err)
	}
	err = autoSyncBeforeTeardown(cmd, preState, statePath, opts.discard)
	if err != nil {
		return fmt.Errorf("done: %w", err)
	}
	stateForRefresh, err := readStateForAutoSync(cmd.Context(), statePath)
	if err != nil {
		return fmt.Errorf("done: %w", err)
	}
	if stateForRefresh.PR.Number != 0 {
		err = refreshPRCacheForState(cmd.Context(), statePath, &stateForRefresh, prCacheRefreshOptions{
			Command: "done",
			Force:   true,
		})
		if err != nil {
			return fmt.Errorf("done: refresh PR state: %w", err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	state, finishErr := lifecycle.FinishWorkstream(cmd.Context(), lifecycle.DoneDeps{
		Git: git.NewExecRunner(),
		Mux: mux.NewTmux(),
	}, lifecycle.DoneOptions{
		StatePath:  statePath,
		ArchiveDir: archiveDir,
		Force:      opts.force,
	})
	if finishErr != nil {
		return fmt.Errorf("done: %w", finishErr)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "workstream %s -> %s\n", state.Session.Name, state.Session.Status)
	if err != nil {
		return fmt.Errorf("done write: %w", err)
	}
	return nil
}

func resolveDoneStatePath(cmd *cobra.Command, name string) (string, error) {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return "", fmt.Errorf("done: %w", err)
	}
	_ = stateDir
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return "", fmt.Errorf("done: %w", err)
	}
	return statePath, nil
}
