package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/duration"
	"github.com/kakkoyun/af/internal/lifecycle"
)

type cleanOptions struct {
	root             *rootOptions
	maxAge           string
	dryRun           bool
	includeAbandoned bool
	force            bool
}

func newCleanCmd(opts *rootOptions) *cobra.Command {
	cOpts := &cleanOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Reap completed (and optionally abandoned) workstreams",
		Long:  "clean enumerates workstreams in a terminal lifecycle state and removes their state dirs. --dry-run only reports. --max-age D restricts reaping to entries older than the given duration (e.g. 7d, 24h).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClean(cmd, cOpts)
		},
	}
	cmd.Flags().BoolVar(&cOpts.dryRun, "dry-run", false, "list what would be removed without removing")
	cmd.Flags().BoolVar(&cOpts.includeAbandoned, "include-abandoned", false, "also reap workstreams marked abandoned")
	cmd.Flags().StringVar(&cOpts.maxAge, "max-age", "", "only reap entries older than this duration (e.g. 7d, 24h)")
	cmd.Flags().BoolVar(&cOpts.force, "force", false, "ignore lifecycle status and reap anyway")
	return cmd
}

func runClean(cmd *cobra.Command, opts *cleanOptions) error {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return fmt.Errorf("clean: %w", err)
	}
	summaries, err := readAllStates(stateDir)
	if err != nil {
		return fmt.Errorf("clean: %w", err)
	}

	cutoff, err := cleanCutoff(opts.maxAge)
	if err != nil {
		return err
	}

	targets := selectCleanTargets(summaries, opts, cutoff)
	err = refreshCleanTargetPRs(cmd, targets)
	if err != nil {
		return err
	}
	return executeClean(cmd, targets, opts.dryRun)
}

func cleanCutoff(maxAge string) (time.Time, error) {
	if maxAge == "" {
		return time.Time{}, nil
	}
	d, err := duration.Parse(maxAge)
	if err != nil {
		return time.Time{}, fmt.Errorf("clean: parse --max-age: %w", err)
	}
	return time.Now().UTC().Add(-d), nil
}

func selectCleanTargets(summaries []sessionSummary, opts *cleanOptions, cutoff time.Time) []sessionSummary {
	out := make([]sessionSummary, 0, len(summaries))
	for i := range summaries {
		if !cleanTargetMatches(summaries[i], opts, cutoff) {
			continue
		}
		out = append(out, summaries[i])
	}
	return out
}

func cleanTargetMatches(s sessionSummary, opts *cleanOptions, cutoff time.Time) bool {
	state := lifecycle.State(s.state.Session.Status)
	if !opts.force {
		switch {
		case state == lifecycle.Completed:
		case state == lifecycle.Abandoned && opts.includeAbandoned:
		default:
			return false
		}
	}
	if !cutoff.IsZero() && s.state.Session.CreatedAt.After(cutoff) {
		return false
	}
	return true
}

func executeClean(cmd *cobra.Command, targets []sessionSummary, dryRun bool) error {
	w := cmd.OutOrStdout()
	if len(targets) == 0 {
		_, err := fmt.Fprintln(w, "nothing to clean")
		if err != nil {
			return fmt.Errorf("clean write: %w", err)
		}
		return nil
	}
	for i := range targets {
		name := targets[i].state.Session.Name
		if dryRun {
			_, err := fmt.Fprintf(w, "would remove %s\n", name)
			if err != nil {
				return fmt.Errorf("clean write: %w", err)
			}
			continue
		}
		err := os.RemoveAll(filepath.Dir(targets[i].statePath))
		if err != nil {
			return fmt.Errorf("clean: remove %s: %w", name, err)
		}
		_, err = fmt.Fprintf(w, "removed %s\n", name)
		if err != nil {
			return fmt.Errorf("clean write: %w", err)
		}
	}
	return nil
}

func refreshCleanTargetPRs(cmd *cobra.Command, targets []sessionSummary) error {
	for i := range targets {
		if targets[i].state.PR.Number == 0 {
			continue
		}
		err := refreshPRCacheForState(cmd.Context(), targets[i].statePath, &targets[i].state, prCacheRefreshOptions{
			Command: "clean",
			Force:   true,
		})
		if err != nil {
			return fmt.Errorf("clean: refresh PR state for %s: %w", targets[i].state.Session.Name, err)
		}
	}
	return nil
}
