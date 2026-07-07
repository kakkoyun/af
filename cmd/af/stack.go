package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/session"
	"github.com/kakkoyun/af/internal/workstream"
)

var (
	errStackNoState        = errors.New("workstream state not found")
	errStackParentRequired = errors.New("--parent required")
	errSyncNoParent        = errors.New("sync requires Stack.ParentSession to be set (use af stack <name> --parent <other>)")
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
		Short: "Rebase this workstream's branch onto its stack parent",
		Long:  "sync fetches the parent branch and rebases the current workstream branch onto it per ADR-059. On conflict git is left mid-rebase; resolve the conflict, run 'git rebase --continue', then re-run 'af sync'.",
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
	err := workstream.ValidateSessionName(parent)
	if err != nil {
		return fmt.Errorf("stack: %w", err)
	}
	state, statePath, err := loadStackState(cmd, name)
	if err != nil {
		return err
	}
	err = session.Update(statePath, func(s *session.State) error {
		now := time.Now().UTC()
		s.Stack.ParentSession = parent
		s.Stack.LinkedAt = &now
		state = *s
		return nil
	})
	if err != nil {
		return fmt.Errorf("stack: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "stacked %s onto %s\n", state.Session.Name, parent)
	if err != nil {
		return fmt.Errorf("stack write: %w", err)
	}
	return nil
}

func runUnstack(cmd *cobra.Command, name string) error {
	state, statePath, err := loadStackState(cmd, name)
	if err != nil {
		return err
	}
	err = session.Update(statePath, func(s *session.State) error {
		s.Stack.ParentSession = ""
		s.Stack.ParentBranch = ""
		s.Stack.LinkedAt = nil
		state = *s
		return nil
	})
	if err != nil {
		return fmt.Errorf("unstack: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "unstacked %s\n", state.Session.Name)
	if err != nil {
		return fmt.Errorf("unstack write: %w", err)
	}
	return nil
}

func runSync(cmd *cobra.Command, name string) error {
	state, _, err := loadStackState(cmd, name)
	if err != nil {
		return err
	}
	if state.Stack.ParentSession == "" {
		return fmt.Errorf("sync: %w", errSyncNoParent)
	}

	parentState, parentStatePath, err := loadStackStateByName(state.Stack.ParentSession)
	if err != nil {
		return fmt.Errorf("sync: read parent state: %w", err)
	}
	if parentState.PR.Number != 0 {
		err = withSessionLock(parentStatePath, func() error {
			return refreshPRCacheForState(cmd.Context(), parentStatePath, &parentState, prCacheRefreshOptions{
				Command: "sync",
				Force:   true,
			})
		})
		if err != nil {
			return fmt.Errorf("sync: refresh parent PR state for %s: %w", parentState.Session.Name, err)
		}
	}

	result, err := lifecycle.Sync(
		cmd.Context(),
		lifecycle.SyncDeps{Git: git.NewExecRunner()},
		lifecycle.SyncOptions{
			SessionName: state.Session.Name,
			Worktree:    state.Worktree.Path,
			Branch:      state.Worktree.Branch,
			ParentRef:   parentState.Worktree.Branch,
		},
	)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	if result.FetchWarning != "" {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), //nolint:errcheck // Informational warning.
			"warning: could not fetch origin %s: %s (rebasing against possibly-stale parent)\n",
			result.ParentRef, result.FetchWarning)
	}
	if result.Rebased {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "sync: rebased %s onto %s (%s..%s)\n",
			result.Branch, result.ParentRef, shortSHA(result.BaseBefore), shortSHA(result.BaseAfter))
	} else {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "sync: %s already up to date with %s\n",
			result.Branch, result.ParentRef)
	}
	if err != nil {
		return fmt.Errorf("sync write: %w", err)
	}
	return nil
}

const shortSHALen = 7

// shortSHA returns the first 7 characters of a SHA for display, or the
// full string if it is shorter.
func shortSHA(s string) string {
	if len(s) > shortSHALen {
		return s[:shortSHALen]
	}
	return s
}

func loadStackState(cmd *cobra.Command, name string) (session.State, string, error) {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return session.State{}, "", err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, "", fmt.Errorf("stack: %w: %v", errStackNoState, err) //nolint:errorlint // primary sentinel is errStackNoState; underlying read error is informational.
	}
	return state, statePath, nil
}

func loadStackStateByName(name string) (session.State, string, error) {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return session.State{}, "", fmt.Errorf("stack: %w", err)
	}
	statePath, err := statePathForSessionName(stateDir, name)
	if err != nil {
		return session.State{}, "", fmt.Errorf("stack: %w", err)
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, "", fmt.Errorf("stack: %w: %v", errStackNoState, err) //nolint:errorlint // primary sentinel is errStackNoState; underlying read error is informational.
	}
	return state, statePath, nil
}
