package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
)

var errLifecycleNoState = errors.New("no .af/state.toml in current directory")

func newSuspendCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "suspend [session]",
		Short: "Suspend a workstream (state.toml records suspension; tmux stays alive)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			statePath, err := resolveLifecycleStatePath(name)
			if err != nil {
				return err
			}
			state, err := lifecycle.SuspendWorkstream(cmd.Context(), lifecycle.SuspendOptions{StatePath: statePath})
			if err != nil {
				return fmt.Errorf("suspend: %w", err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "workstream %s -> %s\n", state.Session.Name, state.Session.Status)
			if err != nil {
				return fmt.Errorf("suspend write: %w", err)
			}
			return nil
		},
	}
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
			statePath, err := resolveLifecycleStatePath(name)
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

func resolveLifecycleStatePath(name string) (string, error) {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return "", fmt.Errorf("resolve state path: %w", err)
	}
	if name != "" {
		return filepath.Join(stateDir, name, "state.toml"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve state path: getwd: %w", err)
	}
	statePath := filepath.Join(cwd, ".af", "state.toml")
	_, err = os.Stat(statePath)
	if err != nil {
		return "", fmt.Errorf("resolve state path: %w (cwd=%s)", errLifecycleNoState, cwd)
	}
	return statePath, nil
}
