package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
)

func newSuspendCmd(_ *rootOptions) *cobra.Command {
	var (
		force   bool
		discard bool
	)
	cmd := &cobra.Command{
		Use:   "suspend [session]",
		Short: "Suspend a workstream (state.toml records suspension; tmux stays alive)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
			if err != nil {
				return err
			}
			preState, err := readStateForAutoSync(cmd.Context(), statePath)
			if err != nil {
				return fmt.Errorf("suspend: %w", err)
			}
			err = autoSyncBeforeTeardown(cmd, preState, statePath, discard)
			if err != nil {
				return fmt.Errorf("suspend: %w", err)
			}
			state, err := lifecycle.SuspendWorkstream(cmd.Context(), lifecycle.SuspendOptions{
				StatePath: statePath,
				Force:     force,
			})
			if err != nil {
				return fmt.Errorf("suspend: %w", err)
			}
			if state.SlicerWT.VM != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: slicer VM %s lease is %s\n", state.SlicerWT.VM, state.SlicerWT.LeaseState) //nolint:errcheck // Informational only.
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "workstream %s -> %s\n", state.Session.Name, state.Session.Status)
			if err != nil {
				return fmt.Errorf("suspend write: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force suspend even when worktree is leased to a slicer VM (sets lease_state=discarded)")
	cmd.Flags().BoolVar(&discard, "discard", false, "discard agent session transcripts; skip ADR-067 automatic sync before VM teardown")
	return cmd
}

func newResumeCmd(_ *rootOptions) *cobra.Command {
	var bare bool
	cmd := &cobra.Command{
		Use:   "resume [session]",
		Short: "Resume a suspended workstream",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
			if err != nil {
				return err
			}
			state, err := lifecycle.ResumeWorkstream(cmd.Context(), lifecycle.ResumeDeps{Mux: mux.NewTmux()}, lifecycle.ResumeOptions{
				StatePath: statePath,
				Bare:      bare,
			})
			if err != nil {
				return fmt.Errorf("resume: %w", err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "workstream %s -> %s\n", state.Session.Name, state.Session.Status)
			if err != nil {
				return fmt.Errorf("resume write: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&bare, "bare", false, "skip tmux respawn")
	return cmd
}
