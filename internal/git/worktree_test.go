package git_test

import (
	"os"
	"path/filepath"
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	err = os.WriteFile(path, []byte(content), 0o600)
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
