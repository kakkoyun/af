package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

// writeTestSessionStateWithWorktree is like writeTestSessionState but accepts
// a custom worktreePath and baseBranch so tests can point to real git repos.
func writeTestSessionStateWithWorktree(t *testing.T, home, name, worktreePath, branch, baseBranch, status string) {
	t.Helper()
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name)
	err := os.MkdirAll(stateDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	state := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-000000000001",
			Name:      name,
			Status:    status,
			CreatedAt: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		},
		Worktree: session.WorktreeState{
			Path:       worktreePath,
			Branch:     branch,
			BaseBranch: baseBranch,
			RepoSlug:   "github.com/owner/repo",
		},
	}
	err = session.WriteState(filepath.Join(stateDir, "state.toml"), state)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

// initGitRepo initialises a bare git repository in dir with a single
// empty commit on "main". Returns the branch name.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	type step struct{ args []string }
	steps := []step{
		{[]string{"init", "-b", "main"}},
		{[]string{"config", "user.email", "test@af.test"}},
		{[]string{"config", "user.name", "AF Test"}},
		{[]string{"commit", "--allow-empty", "-m", "initial"}},
	}
	for _, s := range steps {
		cmd := exec.CommandContext(t.Context(), "git", s.args...) //nolint:gosec // Test helper; args are literal string constants.
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", s.args, err, out)
		}
	}
	return "main"
}

// writePRTestConfig writes a minimal af config that uses "echo" as the PR
// command so tests can run without a real GitHub remote.
func writePRTestConfig(t *testing.T, home string) {
	t.Helper()
	cfgDir := filepath.Join(home, ".config", "af")
	err := os.MkdirAll(cfgDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	const cfgContent = "schema_version = 1\n\n[pr]\nshell = false\ncmd   = [\"echo\", \"--body\", \"{body}\"]\n"
	err = os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(cfgContent), 0o600)
	if err != nil {
		t.Fatalf("write pr test config: %v", err)
	}
}

// TestPR_AIRejectsWebFlag verifies that combining --ai and --web is rejected
// immediately (before any I/O) with errPRAIWebIncompatible.
func TestPR_AIRejectsWebFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "pr", "--ai", "--web")
	if !errors.Is(err, errPRAIWebIncompatible) {
		t.Fatalf("want errPRAIWebIncompatible, got: %v", err)
	}
}

// TestPR_AIErrorsOnEmptyDiff verifies that --ai returns errPRAIEmptyDiff when
// the worktree's base branch and head branch are identical (no diff).
func TestPR_AIErrorsOnEmptyDiff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoDir := t.TempDir()
	branch := initGitRepo(t, repoDir)
	writeTestSessionStateWithWorktree(t, home, "ai-diff-test", repoDir, branch, branch, "active")

	_, _, err := executeCommand(t, newRootCmd(), "pr", "--ai", "ai-diff-test")
	if !errors.Is(err, errPRAIEmptyDiff) {
		t.Fatalf("want errPRAIEmptyDiff, got: %v", err)
	}
}

// TestPR_AIUsesBodyFromAgent verifies that when prAIBodyFunc returns a body,
// that body is passed through to the configured PR command.
func TestPR_AIUsesBodyFromAgent(t *testing.T) {
	orig := prAIBodyFunc
	t.Cleanup(func() { prAIBodyFunc = orig })
	prAIBodyFunc = func(_ context.Context, _ session.State, _ string) (string, error) {
		return "AI-GENERATED BODY", nil
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	// Use a real directory for the worktree path so the proxy runner's chdir succeeds.
	worktreeDir := t.TempDir()
	writeTestSessionStateWithWorktree(t, home, "ai-body-test", worktreeDir, "feat/ai-test", "main", "active")
	writePRTestConfig(t, home)

	stdout, _, err := executeCommand(t, newRootCmd(), "pr", "--ai", "--title", "Test PR", "ai-body-test")
	if err != nil {
		t.Fatalf("pr --ai: %v", err)
	}
	if !strings.Contains(stdout, "AI-GENERATED BODY") {
		t.Fatalf("stdout %q does not contain AI-GENERATED BODY", stdout)
	}
}

// TestPR_AIErrorsOnEmptyAgentOutput verifies that a whitespace-only body
// returned by the agent is treated as empty and returns errPRAIEmptyBody.
func TestPR_AIErrorsOnEmptyAgentOutput(t *testing.T) {
	orig := prAIBodyFunc
	t.Cleanup(func() { prAIBodyFunc = orig })
	prAIBodyFunc = func(_ context.Context, _ session.State, _ string) (string, error) {
		return "   ", nil
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "ai-empty-test", "feat/ai-empty", "active")

	_, _, err := executeCommand(t, newRootCmd(), "pr", "--ai", "ai-empty-test")
	if !errors.Is(err, errPRAIEmptyBody) {
		t.Fatalf("want errPRAIEmptyBody, got: %v", err)
	}
}
