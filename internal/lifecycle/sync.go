// Package lifecycle orchestrates workstream create, suspend, resume,
// done, and stack-sync operations per ADR-038, ADR-046, and ADR-059.
package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kakkoyun/af/internal/git"
)

// ErrSync is the top-level sentinel for sync failures not covered by a
// more specific error.
var ErrSync = errors.New("sync failed")

// ErrSyncNoParent reports that the workstream has no stack parent
// configured.  Use `af stack <session> --parent <parent>` to set one.
var ErrSyncNoParent = errors.New("no stack parent configured")

// ErrSyncConflict reports that `git rebase` encountered a merge
// conflict.  The caller should instruct the user to resolve the
// conflict, run `git rebase --continue`, then retry `af sync`.
var ErrSyncConflict = errors.New("rebase conflict")

// ErrSyncDirtyWorktree reports that the worktree has uncommitted
// changes that must be committed or stashed before sync can proceed.
var ErrSyncDirtyWorktree = errors.New("worktree has uncommitted changes; commit or stash first")

// SyncOptions configures a rebase-onto-parent sync operation per ADR-059.
type SyncOptions struct {
	// SessionName is the name of the workstream being synced.
	SessionName string
	// Worktree is the absolute path to the worktree directory.
	Worktree string
	// Branch is the head branch to rebase.
	Branch string
	// ParentRef is the parent branch name (resolved from the parent
	// workstream's state.toml Worktree.Branch field).
	ParentRef string
}

// SyncDeps wires the sync orchestrator to its external collaborators.
type SyncDeps struct {
	// Git executes git sub-commands; use git.NewExecRunner() in
	// production and git.NewFakeRunner() in tests.
	Git git.Runner
}

// SyncResult records the outcome of a successful Sync call.
type SyncResult struct {
	// SessionName is the name of the synced workstream.
	SessionName string
	// Branch is the head branch that was (potentially) rebased.
	Branch string
	// ParentRef is the parent branch that Branch was rebased onto.
	ParentRef string
	// BaseBefore is the HEAD commit SHA before the rebase.
	BaseBefore string
	// BaseAfter is the HEAD commit SHA after the rebase (equals
	// BaseBefore when Rebased is false).
	BaseAfter string
	// Rebased is true when commits were actually replayed.
	Rebased bool
}

// Sync rebases Branch onto ParentRef per ADR-059 §Commands "af sync":
//
//  1. Validate all options are non-empty.
//  2. Reject dirty worktrees (`git status --porcelain` non-empty).
//  3. Attempt `git fetch origin <parentRef>` (errors silently dropped
//     for local-only stacks with no remote configured).
//  4. Capture the pre-rebase HEAD SHA.
//  5. Compute `git merge-base HEAD <parentRef>` and `git rev-parse
//     <parentRef>`.  If they are equal, Branch already contains all of
//     ParentRef — return Rebased=false immediately.
//  6. Run `git rebase --onto <parentRef> <mergeBase> <branch>`.  On
//     conflict exit the caller receives ErrSyncConflict; on any other
//     non-zero exit ErrSync is returned.
//  7. Capture the post-rebase HEAD SHA and return Rebased=true.
func Sync(ctx context.Context, deps SyncDeps, opts SyncOptions) (SyncResult, error) {
	err := validateSyncOptions(opts)
	if err != nil {
		return SyncResult{}, err
	}

	err = detectDirtyWorktree(ctx, deps.Git, opts.Worktree)
	if err != nil {
		return SyncResult{}, err
	}

	tryFetchParent(ctx, deps.Git, opts.Worktree, opts.ParentRef)

	baseBefore, err := captureHEADSHA(ctx, deps.Git, opts.Worktree)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: capture HEAD: %w", ErrSync, err)
	}

	mergeBase, err := findMergeBase(ctx, deps.Git, opts.Worktree, opts.ParentRef)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: merge-base: %w", ErrSync, err)
	}

	parentSHA, err := revParseSHA(ctx, deps.Git, opts.Worktree, opts.ParentRef)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: rev-parse parent: %w", ErrSync, err)
	}

	if mergeBase == parentSHA {
		return SyncResult{
			SessionName: opts.SessionName,
			Branch:      opts.Branch,
			ParentRef:   opts.ParentRef,
			Rebased:     false,
			BaseBefore:  baseBefore,
			BaseAfter:   baseBefore,
		}, nil
	}

	rebaseErr := runRebase(ctx, deps.Git, opts.Worktree, opts.ParentRef, mergeBase, opts.Branch)
	if rebaseErr != nil {
		return SyncResult{}, rebaseErr
	}

	baseAfter, err := captureHEADSHA(ctx, deps.Git, opts.Worktree)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: capture HEAD after rebase: %w", ErrSync, err)
	}

	return SyncResult{
		SessionName: opts.SessionName,
		Branch:      opts.Branch,
		ParentRef:   opts.ParentRef,
		Rebased:     true,
		BaseBefore:  baseBefore,
		BaseAfter:   baseAfter,
	}, nil
}

// validateSyncOptions returns ErrSync when any required field is empty.
func validateSyncOptions(opts SyncOptions) error {
	switch {
	case opts.SessionName == "":
		return fmt.Errorf("%w: empty session name", ErrSync)
	case opts.Worktree == "":
		return fmt.Errorf("%w: empty worktree path", ErrSync)
	case opts.Branch == "":
		return fmt.Errorf("%w: empty branch", ErrSync)
	case opts.ParentRef == "":
		return fmt.Errorf("%w: empty parent ref", ErrSync)
	}
	return nil
}

// detectDirtyWorktree runs `git status --porcelain` and returns
// ErrSyncDirtyWorktree when the output is non-empty.
func detectDirtyWorktree(ctx context.Context, runner git.Runner, worktree string) error {
	out, err := runner.Run(ctx, worktree, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("%w: git status: %w", ErrSync, err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("sync: %w", ErrSyncDirtyWorktree)
	}
	return nil
}

// tryFetchParent attempts `git fetch origin <parentRef>`.  Errors are
// silently dropped because local-only stacks have no remote configured.
func tryFetchParent(ctx context.Context, runner git.Runner, worktree, parentRef string) {
	fetchOut, fetchErr := runner.Run(ctx, worktree, "fetch", "origin", parentRef)
	_ = fetchOut
	_ = fetchErr
}

// captureHEADSHA returns the current HEAD commit SHA via `git rev-parse HEAD`.
func captureHEADSHA(ctx context.Context, runner git.Runner, worktree string) (string, error) {
	out, err := runner.Run(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// revParseSHA resolves ref to a commit SHA via `git rev-parse <ref>`.
func revParseSHA(ctx context.Context, runner git.Runner, worktree, ref string) (string, error) {
	out, err := runner.Run(ctx, worktree, "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// findMergeBase returns the common ancestor of HEAD and parentRef via
// `git merge-base HEAD <parentRef>`.
func findMergeBase(ctx context.Context, runner git.Runner, worktree, parentRef string) (string, error) {
	out, err := runner.Run(ctx, worktree, "merge-base", "HEAD", parentRef)
	if err != nil {
		return "", fmt.Errorf("merge-base HEAD %s: %w", parentRef, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// runRebase executes `git rebase --onto <parentRef> <mergeBase>
// <branch>`.  On conflict it returns ErrSyncConflict; on any other
// failure it returns ErrSync.
func runRebase(ctx context.Context, runner git.Runner, worktree, parentRef, mergeBase, branch string) error {
	out, err := runner.Run(ctx, worktree, "rebase", "--onto", parentRef, mergeBase, branch)
	if err != nil {
		if bytes.Contains(out, []byte("CONFLICT")) || bytes.Contains(out, []byte("could not apply")) {
			return fmt.Errorf("%w: %s", ErrSyncConflict, strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("%w: rebase onto %s: %w", ErrSync, parentRef, err)
	}
	return nil
}
