package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/duration"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/session"
)

// errCleanSyncFailed reports that the ADR-067 automatic session-data
// sync failed for at least one VM-leased target during `af clean`.
// Targets whose sync failed are skipped (their session dir is kept so
// the user can recover); every other target in the same run is still
// reaped. The command exits non-zero whenever this sentinel fires so
// the failure cannot be missed.
var errCleanSyncFailed = errors.New("clean: automatic sync failed for one or more workstreams")

type cleanOptions struct {
	root             *rootOptions
	maxAge           string
	dryRun           bool
	includeAbandoned bool
	force            bool
	discard          bool
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
	cmd.Flags().BoolVar(&cOpts.discard, "discard", false, "discard agent session transcripts; skip ADR-067 automatic sync before VM teardown")
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
	return executeClean(cmd, targets, opts.dryRun, opts.discard)
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

func executeClean(cmd *cobra.Command, targets []sessionSummary, dryRun, discard bool) error {
	w := cmd.OutOrStdout()
	if len(targets) == 0 {
		_, err := fmt.Fprintln(w, "nothing to clean")
		if err != nil {
			return fmt.Errorf("clean write: %w", err)
		}
		return nil
	}
	if dryRun {
		return executeCleanDryRun(w, targets)
	}
	return executeCleanRemoval(cmd, w, targets, discard)
}

// executeCleanDryRun reports what a real run would do without touching
// any state. Per ADR-067, VM-leased targets are called out separately
// ("would sync + remove") since reaping them also runs the automatic
// session-data sync. It reads straight off the already-loaded summaries
// rather than re-reading state.toml: dry-run is informational only, so
// the tiny staleness window that matters for the real removal path
// (see cleanRemoveTarget) is immaterial here.
func executeCleanDryRun(w io.Writer, targets []sessionSummary) error {
	for i := range targets {
		name := targets[i].state.Session.Name
		verb := "would remove"
		if isVMLeaseHeld(targets[i].state) {
			verb = "would sync + remove"
		}
		_, err := fmt.Fprintf(w, "%s %s\n", verb, name)
		if err != nil {
			return fmt.Errorf("clean write: %w", err)
		}
	}
	return nil
}

// executeCleanRemoval reaps every target, one at a time. A target whose
// ADR-067 automatic sync fails is skipped (its session dir is kept) but
// does not abort the run: every other target is still processed. If any
// target's sync failed, the function returns errCleanSyncFailed after
// the loop so the command exits non-zero.
func executeCleanRemoval(cmd *cobra.Command, w io.Writer, targets []sessionSummary, discard bool) error {
	syncFailed := false
	for i := range targets {
		name := targets[i].state.Session.Name
		err := cleanRemoveTarget(cmd, targets[i].statePath, discard)
		if err != nil {
			if errors.Is(err, errSessionDataAutoSyncFailed) {
				syncFailed = true
				_, printErr := fmt.Fprintf(w, "skipped %s: automatic sync failed\n", name)
				if printErr != nil {
					return fmt.Errorf("clean write: %w", printErr)
				}
				continue
			}
			return fmt.Errorf("clean: remove %s: %w", name, err)
		}
		_, err = fmt.Fprintf(w, "removed %s\n", name)
		if err != nil {
			return fmt.Errorf("clean write: %w", err)
		}
	}
	if syncFailed {
		return fmt.Errorf("clean: %w", errCleanSyncFailed)
	}
	return nil
}

// cleanRemoveTarget removes one target's session dir under its session
// lock. It re-reads state.toml fresh (the pre-lock sessionSummary is
// stale by design: PR refresh and other targets' processing happen
// between the initial scan and this point) and, for a workstream whose
// worktree is still held by a slicer VM, runs the ADR-067 automatic
// sync before RemoveAll. A sync error (wrapping
// errSessionDataAutoSyncFailed) aborts only this target's removal; the
// recovery hint is already printed by autoSyncBeforeTeardown.
func cleanRemoveTarget(cmd *cobra.Command, statePath string, discard bool) error {
	return withSessionLock(statePath, func() error {
		fresh, readErr := session.ReadState(statePath)
		if readErr != nil {
			return fmt.Errorf("read state %s: %w", statePath, readErr)
		}
		if isVMLeaseHeld(fresh) {
			syncErr := autoSyncBeforeTeardown(cmd, fresh, statePath, discard)
			if syncErr != nil {
				return syncErr
			}
		}
		return os.RemoveAll(filepath.Dir(statePath))
	})
}

// isVMLeaseHeld reports whether a workstream's worktree is currently
// held by a slicer VM (ADR-065), meaning the ADR-067 automatic
// session-data sync must run before teardown can proceed.
func isVMLeaseHeld(state session.State) bool {
	return state.SlicerWT.VM != "" && state.SlicerWT.LeaseState == session.SlicerWTLeaseHeldByVM
}

func refreshCleanTargetPRs(cmd *cobra.Command, targets []sessionSummary) error {
	for i := range targets {
		if targets[i].state.PR.Number == 0 {
			continue
		}
		err := withSessionLock(targets[i].statePath, func() error {
			return refreshPRCacheForState(cmd.Context(), targets[i].statePath, &targets[i].state, prCacheRefreshOptions{
				Command: "clean",
				Force:   true,
			})
		})
		if err != nil {
			return fmt.Errorf("clean: refresh PR state for %s: %w", targets[i].state.Session.Name, err)
		}
	}
	return nil
}
