package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/pr"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

type prCacheRefreshOptions struct {
	Command   string
	Force     bool
	RequirePR bool
}

// refreshPRCacheForState applies the ADR-071 PR state cache policy to one
// state.toml. The passed State is mutated in place and written back whenever a
// refresh attempt changes persistent fields (successful fetch, flip, or
// last_refresh_error update). Callers decide whether refresh errors are soft
// (status/info) or hard (clean/sync/done).
func refreshPRCacheForState(ctx context.Context, statePath string, state *session.State, opts prCacheRefreshOptions) error {
	if state.PR.Number == 0 {
		if opts.RequirePR {
			return pr.ErrNoPR
		}
		return nil
	}
	cfg, err := loadConfigForPRRefresh(ctx, state.Worktree.Path)
	if err != nil {
		return err
	}
	result, refreshErr := prRefreshFunc(ctx, &state.PR, pr.Options{
		Runner:   sandbox.ExecRunner{},
		RepoSlug: state.Worktree.RepoSlug,
		TTL:      cfg.PR.RefreshTTL,
		Force:    opts.Force,
		Now:      time.Now,
	})
	if refreshErr != nil || !result.Skipped {
		writeErr := session.WriteState(statePath, *state)
		if writeErr != nil {
			return fmt.Errorf("write refreshed PR state: %w", writeErr)
		}
	}
	if refreshErr != nil {
		return refreshErr
	}
	if result.Changed {
		emitErr := emitPRStateChangedEvent(statePath, state, result)
		if emitErr != nil {
			return emitErr
		}
	}
	return nil
}

func loadConfigForPRRefresh(ctx context.Context, repoDir string) (config.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config.Config{}, fmt.Errorf("resolve home: %w", err)
	}
	loaded, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: filepath.Join(home, ".config", "af", "config.toml"),
		RepoDir:        repoDir,
	})
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return loaded, nil
}

func warnPRRefreshOnce(ctx context.Context, warned *bool, command string, err error) {
	if *warned {
		return
	}
	*warned = true
	slog.WarnContext(ctx, "PR state refresh failed; rendering stale PR state as ?", "command", command, "error", err)
}
