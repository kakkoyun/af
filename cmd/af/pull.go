package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/sandbox"
)

func newPullCmd(opts *rootOptions) *cobra.Command {
	_ = opts
	return &cobra.Command{
		Use:   "pull [session]",
		Short: "Pull a slicer-wt workstream's VM commits back to the host worktree",
		Long: "pull runs `slicer wt pull` for the named workstream (or the one detected from cwd), " +
			"imports VM branches under refs/slicer/<vm>/*, fast-forwards the host branch, and " +
			"releases the host-worktree lease. The workstream must have lease_state=held_by_vm. " +
			"After pull, the host branch contains VM commits and can be pushed normally.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runPull(cmd, name)
		},
	}
}

func runPull(cmd *cobra.Command, name string) error {
	statePath, err := resolveLifecycleStatePath(name)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	res, err := lifecycle.Pull(cmd.Context(), lifecycle.PullDeps{
		Runner: sandbox.ExecRunner{},
	}, lifecycle.PullOptions{
		StatePath: statePath,
	})
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(),
		"pull: %s: pulled commits from VM %s (lease released)\n",
		res.SessionName, res.VM,
	)
	if err != nil {
		return fmt.Errorf("pull write: %w", err)
	}
	return nil
}
