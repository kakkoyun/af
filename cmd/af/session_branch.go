package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/session"
	"github.com/kakkoyun/af/internal/workstream"
)

const sessionDirPerm = 0o750

func newSessionBranchCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "session-branch",
		Short: "Create an ad-hoc workstream branch in the current checkout",
		Long:  "session-branch creates a branch in the current checkout and a lightweight state.toml under the sessions dir. No worktree, no tmux, no agent \u2014 useful for quick scratch work without forking off a full workstream.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionBranch(cmd, opts)
		},
	}
}

func runSessionBranch(cmd *cobra.Command, opts *rootOptions) error {
	ctx := cmd.Context()
	root, cfg, err := sessionBranchRepoContext(ctx, opts)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	name := workstream.AutoSessionName(filepath.Base(root), now)
	branchName := workstream.BranchName(workstream.BranchOptions{
		Name:             name,
		Prefix:           cfg.Branch.Prefix,
		PrefixOnForkOnly: cfg.Branch.PrefixOnForkOnly,
	})

	_, err = git.NewExecRunner().Run(ctx, root, "checkout", "-b", branchName)
	if err != nil {
		return fmt.Errorf("session-branch: git checkout -b: %w", err)
	}

	err = persistSessionBranchState(root, name, branchName, now)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "created session-branch %s on %s\n", name, branchName)
	if err != nil {
		return fmt.Errorf("session-branch write: %w", err)
	}
	return nil
}

func sessionBranchRepoContext(ctx context.Context, opts *rootOptions) (string, config.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", config.Config{}, fmt.Errorf("session-branch: getwd: %w", err)
	}
	cfg, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: opts.configPath,
		RepoDir:        cwd,
	})
	if err != nil {
		return "", config.Config{}, fmt.Errorf("session-branch: load config: %w", err)
	}
	out, err := git.NewExecRunner().Run(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", config.Config{}, fmt.Errorf("session-branch: git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), cfg, nil
}

func persistSessionBranchState(root, name, branchName string, now time.Time) error {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return fmt.Errorf("session-branch: %w", err)
	}
	sessionDir := filepath.Join(stateDir, name)
	err = os.MkdirAll(sessionDir, sessionDirPerm)
	if err != nil {
		return fmt.Errorf("session-branch: mkdir state dir: %w", err)
	}
	statePath := filepath.Join(sessionDir, "state.toml")
	state := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        workstream.SessionID(filepath.Base(root), branchName, "primary", now).String(),
			Name:      name,
			Status:    "active",
			CreatedAt: now,
		},
		Worktree: session.WorktreeState{
			Path:       root,
			Branch:     branchName,
			BaseBranch: "HEAD",
			GitRoot:    root,
			RepoSlug:   filepath.Base(root),
		},
		Execution: session.ExecutionState{
			Mode:        "local",
			Multiplexer: "none",
		},
	}
	err = session.WriteState(statePath, state)
	if err != nil {
		return fmt.Errorf("session-branch: write state: %w", err)
	}
	return nil
}
