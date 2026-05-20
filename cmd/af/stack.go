package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/session"
)

var (
	errStackNoState        = errors.New("workstream state not found")
	errStackParentRequired = errors.New("--parent required")
)

func newStackCmd(_ *rootOptions) *cobra.Command {
	var parent string
	cmd := &cobra.Command{
		Use:   "stack [session]",
		Short: "Link this workstream as a child of --parent in the stack model (ADR-059)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runStack(cmd, name, parent)
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "parent session name")
	return cmd
}

func newUnstackCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "unstack [session]",
		Short: "Remove the stack parent link from this workstream",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runUnstack(cmd, name)
		},
	}
}

func newSyncCmd(_ *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "sync [session]",
		Short: "Sync this workstream's branch onto its stack parent (placeholder)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runSync(cmd, name)
		},
	}
}

func runStack(cmd *cobra.Command, name, parent string) error {
	if parent == "" {
		return fmt.Errorf("stack: %w", errStackParentRequired)
	}
	state, statePath, err := loadStackState(name)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	state.Stack.ParentSession = parent
	state.Stack.LinkedAt = &now
	err = session.WriteState(statePath, state)
	if err != nil {
		return fmt.Errorf("stack: write state: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "stacked %s onto %s\n", state.Session.Name, parent)
	if err != nil {
		return fmt.Errorf("stack write: %w", err)
	}
	return nil
}

func runUnstack(cmd *cobra.Command, name string) error {
	state, statePath, err := loadStackState(name)
	if err != nil {
		return err
	}
	state.Stack.ParentSession = ""
	state.Stack.ParentBranch = ""
	state.Stack.LinkedAt = nil
	err = session.WriteState(statePath, state)
	if err != nil {
		return fmt.Errorf("unstack: write state: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "unstacked %s\n", state.Session.Name)
	if err != nil {
		return fmt.Errorf("unstack write: %w", err)
	}
	return nil
}

func runSync(cmd *cobra.Command, name string) error {
	state, _, err := loadStackState(name)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "sync: stack parent for %s is %q (sync logic deferred to ADR-059 implementation)\n", state.Session.Name, state.Stack.ParentSession)
	if err != nil {
		return fmt.Errorf("sync write: %w", err)
	}
	return nil
}

func loadStackState(name string) (session.State, string, error) {
	statePath, err := resolveLifecycleStatePath(name)
	if err != nil {
		return session.State{}, "", err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, "", fmt.Errorf("stack: %w: %v", errStackNoState, err) //nolint:errorlint // primary sentinel is errStackNoState; underlying read error is informational.
	}
	return state, statePath, nil
}
