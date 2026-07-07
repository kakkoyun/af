package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

type agentOptions struct {
	root           *rootOptions
	session        string
	slot           string
	provider       string
	removeWorktree bool
}

var (
	errAgentSlotRequired    = errors.New("--slot is required")
	errAgentNameRequired    = errors.New("--agent is required")
	errAgentNoWorkstreamCwd = errors.New("no workstream detected in cwd")
)

func newAgentCmd(opts *rootOptions) *cobra.Command {
	aOpts := &agentOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage multi-agent slots on a workstream",
		Long:  "agent subcommands add, list, and stop named agent slots per ADR-039.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
			if err != nil {
				return fmt.Errorf("agent: usage: %w", err)
			}
			return nil
		},
	}
	cmd.PersistentFlags().StringVar(&aOpts.session, "session", "", "target workstream (default: cwd discovery)")

	cmd.AddCommand(newAgentListCmd(aOpts))
	cmd.AddCommand(newAgentAddCmd(aOpts))
	cmd.AddCommand(newAgentStopCmd(aOpts))
	return cmd
}

func newAgentListCmd(opts *agentOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List agent slots for the workstream",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentList(cmd, opts)
		},
	}
}

func newAgentAddCmd(opts *agentOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new agent slot (creates a sub-worktree for non-primary slots)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentAdd(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.slot, "slot", "", "slot name (required)")
	cmd.Flags().StringVar(&opts.provider, "agent", "", "agent provider (pi, claude, codex)")
	return cmd
}

func newAgentStopCmd(opts *agentOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <slot>",
		Short: "Mark an agent slot stopped",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.slot = args[0]
			return runAgentStop(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.removeWorktree, "remove-worktree", false, "also remove the sub-worktree and delete the sub-branch")
	return cmd
}

func runAgentList(cmd *cobra.Command, opts *agentOptions) error {
	statePath, err := resolveAgentStatePath(opts)
	if err != nil {
		return err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return fmt.Errorf("agent list: %w", err)
	}
	w := cmd.OutOrStdout()
	agents := lifecycle.SortedAgents(state)
	for i := range agents {
		_, err = fmt.Fprintf(w, "%-12s %-10s %s\n", agents[i].Slot, agents[i].Status, agents[i].Provider)
		if err != nil {
			return fmt.Errorf("agent list write: %w", err)
		}
	}
	return nil
}

func runAgentAdd(cmd *cobra.Command, opts *agentOptions) error {
	if opts.slot == "" {
		return fmt.Errorf("agent add: %w", errAgentSlotRequired)
	}
	if opts.provider == "" {
		return fmt.Errorf("agent add: %w", errAgentNameRequired)
	}
	statePath, err := resolveAgentStatePath(opts)
	if err != nil {
		return err
	}
	err = withSessionLock(statePath, func() error {
		_, _, lockedErr := lifecycle.AgentAdd(cmd.Context(), lifecycle.AgentAddDeps{
			Git: git.NewExecRunner(),
			Mux: mux.NewTmux(),
		}, lifecycle.AgentAddOptions{
			StatePath: statePath,
			Slot:      opts.slot,
			Provider:  opts.provider,
		})
		if lockedErr != nil {
			return fmt.Errorf("agent add: %w", lockedErr)
		}
		return nil
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "added agent slot %s (%s)\n", opts.slot, opts.provider)
	if err != nil {
		return fmt.Errorf("agent add write: %w", err)
	}
	return nil
}

func runAgentStop(cmd *cobra.Command, opts *agentOptions) error {
	statePath, err := resolveAgentStatePath(opts)
	if err != nil {
		return err
	}
	err = withSessionLock(statePath, func() error {
		lockedErr := lifecycle.AgentStop(cmd.Context(), git.NewExecRunner(), lifecycle.AgentStopOptions{
			StatePath:      statePath,
			Slot:           opts.slot,
			RemoveWorktree: opts.removeWorktree,
		})
		if lockedErr != nil {
			return fmt.Errorf("agent stop: %w", lockedErr)
		}
		return nil
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "stopped agent slot %s\n", opts.slot)
	if err != nil {
		return fmt.Errorf("agent stop write: %w", err)
	}
	return nil
}

func resolveAgentStatePath(opts *agentOptions) (string, error) {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return "", fmt.Errorf("resolve state path: %w", err)
	}
	if opts.session != "" {
		return filepath.Join(stateDir, opts.session, "state.toml"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve state path: getwd: %w", err)
	}
	discovered, err := session.DiscoverStatePath(session.DiscoverOptions{
		Cwd:         cwd,
		SessionsDir: stateDir,
	})
	if err != nil {
		return "", fmt.Errorf("resolve state path: %w", err)
	}
	if discovered == "" {
		return "", fmt.Errorf("resolve state path: %w (cwd=%s)", errAgentNoWorkstreamCwd, cwd)
	}
	return discovered, nil
}
