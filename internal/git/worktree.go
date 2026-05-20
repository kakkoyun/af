package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kakkoyun/af/internal/workstream"
)

const discoveryDirPerm = 0o750

var (
	// ErrInvalidWorktreePlan reports missing inputs for worktree planning.
	ErrInvalidWorktreePlan = errors.New("invalid worktree plan")
	// ErrConflictingStateLink reports an existing .af/state.toml link to another state file.
	ErrConflictingStateLink = errors.New("conflicting state symlink")
)

// WorktreeOptions are the stable layout inputs for a primary worktree.
type WorktreeOptions struct {
	Root   string
	Repo   string
	Branch string
}

// WorktreePlan describes a primary git worktree path and branch.
type WorktreePlan struct {
	Path   string
	Repo   string
	Branch string
}

// SubWorktreePlan describes a sibling sub-worktree for a non-primary slot.
type SubWorktreePlan struct {
	Path   string
	Branch string
	Slot   string
}

// CleanupOptions describes safe worktree and branch cleanup planning inputs.
type CleanupOptions struct {
	MergedBranches map[string]bool
	Primary        WorktreePlan
	SubWorktrees   []SubWorktreePlan
	Force          bool
}

// CleanupPlan is a dry, executable plan for later git cleanup code.
type CleanupPlan struct {
	WorktreeRemovals []string
	BranchDeletions  []string
	SkippedBranches  []string
}

// PlanPrimaryWorktree returns the stable primary worktree layout.
func PlanPrimaryWorktree(opts WorktreeOptions) (WorktreePlan, error) {
	if opts.Root == "" || opts.Repo == "" || opts.Branch == "" {
		return WorktreePlan{}, fmt.Errorf("primary worktree requires root, repo, and branch: %w", ErrInvalidWorktreePlan)
	}

	return WorktreePlan{
		Path:   filepath.Join(opts.Root, opts.Repo, filepath.FromSlash(opts.Branch)),
		Repo:   opts.Repo,
		Branch: opts.Branch,
	}, nil
}

// PlanSubWorktree returns the sibling sub-worktree layout for slot.
func PlanSubWorktree(primary WorktreePlan, slot string) SubWorktreePlan {
	sanitizedSlot := workstream.Sanitize(slot)
	return SubWorktreePlan{
		Path:   primary.Path + "--" + sanitizedSlot,
		Branch: workstream.SubBranchName(primary.Branch, slot),
		Slot:   slot,
	}
}

// EnsureStateSymlink creates or verifies worktree/.af/state.toml.
func EnsureStateSymlink(worktreePath, statePath string) error {
	link := filepath.Join(worktreePath, ".af", "state.toml")
	err := os.MkdirAll(filepath.Dir(link), discoveryDirPerm)
	if err != nil {
		return fmt.Errorf("create discovery directory %s: %w", filepath.Dir(link), err)
	}

	matches, err := existingSymlinkMatches(link, statePath)
	if err != nil {
		return err
	}
	if matches {
		return nil
	}
	err = os.Symlink(statePath, link)
	if err != nil {
		return fmt.Errorf("create state symlink %s: %w", link, err)
	}

	return nil
}

// PlanCleanup plans worktree removal and safe branch deletion.
func PlanCleanup(opts CleanupOptions) CleanupPlan {
	plan := CleanupPlan{
		WorktreeRemovals: make([]string, 0, len(opts.SubWorktrees)+1),
	}
	for _, sub := range opts.SubWorktrees {
		plan.WorktreeRemovals = append(plan.WorktreeRemovals, sub.Path)
		if opts.Force || opts.MergedBranches[sub.Branch] {
			plan.BranchDeletions = append(plan.BranchDeletions, sub.Branch)
			continue
		}
		plan.SkippedBranches = append(plan.SkippedBranches, sub.Branch)
	}
	if opts.Primary.Path != "" {
		plan.WorktreeRemovals = append(plan.WorktreeRemovals, opts.Primary.Path)
	}

	return plan
}

func existingSymlinkMatches(link, want string) (bool, error) {
	info, err := os.Lstat(link)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat state symlink %s: %w", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, fmt.Errorf("state discovery path %s exists and is not a symlink: %w", link, ErrConflictingStateLink)
	}

	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		return false, fmt.Errorf("resolve state symlink %s: %w", link, err)
	}
	wantResolved, err := filepath.EvalSymlinks(want)
	if err != nil {
		return false, fmt.Errorf("resolve target state %s: %w", want, err)
	}
	if got != wantResolved {
		return false, fmt.Errorf("state symlink %s points to %s, want %s: %w", link, got, wantResolved, ErrConflictingStateLink)
	}

	return true, nil
}
