package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/git"
)

func TestPlanPrimaryWorktree_UsesStableRootRepoBranchLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	plan, err := git.PlanPrimaryWorktree(git.WorktreeOptions{Root: root, Repo: "af", Branch: "kakkoyun/issue-42"})
	if err != nil {
		t.Fatalf("PlanPrimaryWorktree() error = %v", err)
	}

	want := filepath.Join(root, "af", "kakkoyun", "issue-42")
	if plan.Path != want || plan.Branch != "kakkoyun/issue-42" || plan.Repo != "af" {
		t.Fatalf("PlanPrimaryWorktree() = %#v, want path %q", plan, want)
	}
}

func TestPlanSubWorktree_UsesSiblingPathAndSubBranch(t *testing.T) {
	root := filepath.Join(string(os.PathSeparator), "tmp", "worktrees")
	primary, err := git.PlanPrimaryWorktree(git.WorktreeOptions{Root: root, Repo: "af", Branch: "kakkoyun/issue-42"})
	if err != nil {
		t.Fatalf("PlanPrimaryWorktree() error = %v", err)
	}

	sub := git.PlanSubWorktree(primary, "review.bot")
	wantPath := filepath.Join(root, "af", "kakkoyun", "issue-42--review--bot")
	wantBranch := "kakkoyun/issue-42--review--bot"
	if sub.Path != wantPath || sub.Branch != wantBranch || sub.Slot != "review.bot" {
		t.Fatalf("PlanSubWorktree() = %#v, want path %q branch %q", sub, wantPath, wantBranch)
	}
}

func TestEnsureStateSymlink_CreatesIdempotentDiscoveryLink(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	state := filepath.Join(root, "sessions", "one", "state.toml")
	writeFile(t, state, "schema_version = 1\n")

	err := git.EnsureStateSymlink(worktree, state)
	if err != nil {
		t.Fatalf("EnsureStateSymlink() error = %v", err)
	}
	err = git.EnsureStateSymlink(worktree, state)
	if err != nil {
		t.Fatalf("EnsureStateSymlink() second call error = %v", err)
	}

	link := filepath.Join(worktree, ".af", "state.toml")
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("resolve symlink: %v", err)
	}
	want, err := filepath.EvalSymlinks(state)
	if err != nil {
		t.Fatalf("resolve state: %v", err)
	}
	if target != want {
		t.Fatalf("symlink target = %q, want %q", target, want)
	}
}

func TestEnsureStateSymlink_RejectsConflictingLink(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	state := filepath.Join(root, "sessions", "one", "state.toml")
	other := filepath.Join(root, "sessions", "two", "state.toml")
	writeFile(t, state, "schema_version = 1\n")
	writeFile(t, other, "schema_version = 1\n")
	err := git.EnsureStateSymlink(worktree, other)
	if err != nil {
		t.Fatalf("EnsureStateSymlink(other) error = %v", err)
	}

	err = git.EnsureStateSymlink(worktree, state)
	if err == nil {
		t.Fatal("EnsureStateSymlink(conflict) error = nil, want error")
	}
}

func TestPlanPrimaryWorktree_RejectsMissingInputs(t *testing.T) {
	tests := []struct {
		name string
		opts git.WorktreeOptions
	}{
		{name: "missing root", opts: git.WorktreeOptions{Repo: "af", Branch: "main"}},
		{name: "missing repo", opts: git.WorktreeOptions{Root: "/tmp/worktrees", Branch: "main"}},
		{name: "missing branch", opts: git.WorktreeOptions{Root: "/tmp/worktrees", Repo: "af"}},
		{name: "all missing", opts: git.WorktreeOptions{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := git.PlanPrimaryWorktree(tt.opts)
			if !errors.Is(err, git.ErrInvalidWorktreePlan) {
				t.Fatalf("PlanPrimaryWorktree(%#v) error = %v, want ErrInvalidWorktreePlan", tt.opts, err)
			}
			if plan != (git.WorktreePlan{}) {
				t.Fatalf("PlanPrimaryWorktree(%#v) plan = %#v, want zero value", tt.opts, plan)
			}
		})
	}
}

func TestEnsureStateSymlink_FailsWhenWorktreePathIsFile(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	writeFile(t, worktree, "not a directory\n")
	state := filepath.Join(root, "state.toml")
	writeFile(t, state, "schema_version = 1\n")

	err := git.EnsureStateSymlink(worktree, state)
	if err == nil {
		t.Fatal("EnsureStateSymlink() error = nil, want directory creation error")
	}
	if !strings.Contains(err.Error(), "create discovery directory") {
		t.Fatalf("EnsureStateSymlink() error = %q, want create discovery directory wrap", err)
	}
}

func TestEnsureStateSymlink_RejectsRegularFileAtLinkPath(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	state := filepath.Join(root, "state.toml")
	writeFile(t, state, "schema_version = 1\n")
	writeFile(t, filepath.Join(worktree, ".af", "state.toml"), "stale copy\n")

	err := git.EnsureStateSymlink(worktree, state)
	if !errors.Is(err, git.ErrConflictingStateLink) {
		t.Fatalf("EnsureStateSymlink() error = %v, want ErrConflictingStateLink", err)
	}
}

func TestEnsureStateSymlink_FailsOnDanglingExistingLink(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	state := filepath.Join(root, "state.toml")
	writeFile(t, state, "schema_version = 1\n")

	link := filepath.Join(worktree, ".af", "state.toml")
	mkdirAll(t, filepath.Dir(link))
	err := os.Symlink(filepath.Join(root, "missing.toml"), link)
	if err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}

	err = git.EnsureStateSymlink(worktree, state)
	if err == nil {
		t.Fatal("EnsureStateSymlink() error = nil, want resolve error")
	}
	if !strings.Contains(err.Error(), "resolve state symlink") {
		t.Fatalf("EnsureStateSymlink() error = %q, want resolve state symlink wrap", err)
	}
}

func TestEnsureStateSymlink_FailsWhenDesiredTargetMissing(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	existing := filepath.Join(root, "existing.toml")
	writeFile(t, existing, "schema_version = 1\n")
	err := git.EnsureStateSymlink(worktree, existing)
	if err != nil {
		t.Fatalf("EnsureStateSymlink(existing) error = %v", err)
	}

	err = git.EnsureStateSymlink(worktree, filepath.Join(root, "missing.toml"))
	if err == nil {
		t.Fatal("EnsureStateSymlink(missing target) error = nil, want resolve error")
	}
	if !strings.Contains(err.Error(), "resolve target state") {
		t.Fatalf("EnsureStateSymlink() error = %q, want resolve target state wrap", err)
	}
}

func TestEnsureStateSymlink_ConflictErrorIsSentinel(t *testing.T) {
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	state := filepath.Join(root, "sessions", "one", "state.toml")
	other := filepath.Join(root, "sessions", "two", "state.toml")
	writeFile(t, state, "schema_version = 1\n")
	writeFile(t, other, "schema_version = 1\n")
	err := git.EnsureStateSymlink(worktree, other)
	if err != nil {
		t.Fatalf("EnsureStateSymlink(other) error = %v", err)
	}

	err = git.EnsureStateSymlink(worktree, state)
	if !errors.Is(err, git.ErrConflictingStateLink) {
		t.Fatalf("EnsureStateSymlink(conflict) error = %v, want ErrConflictingStateLink", err)
	}
}

func TestPlanCleanup_ForceDeletesUnmergedBranches(t *testing.T) {
	plan := git.PlanCleanup(git.CleanupOptions{
		Primary: git.WorktreePlan{Path: "/tmp/primary", Branch: "kakkoyun/issue-42"},
		SubWorktrees: []git.SubWorktreePlan{
			{Path: "/tmp/primary--review", Branch: "kakkoyun/issue-42--review", Slot: "review"},
			{Path: "/tmp/primary--tests", Branch: "kakkoyun/issue-42--tests", Slot: "tests"},
		},
		Force: true,
	})

	wantDeletions := []string{"kakkoyun/issue-42--review", "kakkoyun/issue-42--tests"}
	if !equalStringSlices(plan.BranchDeletions, wantDeletions) {
		t.Fatalf("BranchDeletions = %#v, want %#v", plan.BranchDeletions, wantDeletions)
	}
	if len(plan.SkippedBranches) != 0 {
		t.Fatalf("SkippedBranches = %#v, want empty", plan.SkippedBranches)
	}
}

func TestPlanCleanup_OmitsEmptyPrimaryPath(t *testing.T) {
	plan := git.PlanCleanup(git.CleanupOptions{
		SubWorktrees: []git.SubWorktreePlan{
			{Path: "/tmp/primary--review", Branch: "kakkoyun/issue-42--review", Slot: "review"},
		},
		MergedBranches: map[string]bool{"kakkoyun/issue-42--review": true},
	})

	if !equalStringSlices(plan.WorktreeRemovals, []string{"/tmp/primary--review"}) {
		t.Fatalf("WorktreeRemovals = %#v, want sub worktree only", plan.WorktreeRemovals)
	}
	if !equalStringSlices(plan.BranchDeletions, []string{"kakkoyun/issue-42--review"}) {
		t.Fatalf("BranchDeletions = %#v", plan.BranchDeletions)
	}
}

func TestPlanCleanup_EmptyOptionsYieldEmptyPlan(t *testing.T) {
	plan := git.PlanCleanup(git.CleanupOptions{})
	if len(plan.WorktreeRemovals) != 0 || len(plan.BranchDeletions) != 0 || len(plan.SkippedBranches) != 0 {
		t.Fatalf("PlanCleanup(empty) = %#v, want empty plan", plan)
	}
}

func TestPlanCleanup_RemovesAllWorktreesButDeletesOnlySafeBranches(t *testing.T) {
	plan := git.PlanCleanup(git.CleanupOptions{
		Primary: git.WorktreePlan{Path: "/tmp/primary", Branch: "kakkoyun/issue-42"},
		SubWorktrees: []git.SubWorktreePlan{
			{Path: "/tmp/primary--review", Branch: "kakkoyun/issue-42--review", Slot: "review"},
			{Path: "/tmp/primary--tests", Branch: "kakkoyun/issue-42--tests", Slot: "tests"},
		},
		MergedBranches: map[string]bool{"kakkoyun/issue-42--review": true},
	})

	wantRemovals := []string{"/tmp/primary--review", "/tmp/primary--tests", "/tmp/primary"}
	if !equalStringSlices(plan.WorktreeRemovals, wantRemovals) {
		t.Fatalf("WorktreeRemovals = %#v, want %#v", plan.WorktreeRemovals, wantRemovals)
	}
	if !equalStringSlices(plan.BranchDeletions, []string{"kakkoyun/issue-42--review"}) {
		t.Fatalf("BranchDeletions = %#v", plan.BranchDeletions)
	}
	if !equalStringSlices(plan.SkippedBranches, []string{"kakkoyun/issue-42--tests"}) {
		t.Fatalf("SkippedBranches = %#v", plan.SkippedBranches)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	err := os.MkdirAll(path, 0o750)
	if err != nil {
		t.Fatalf("create directory %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}

	return true
}
