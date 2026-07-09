package lifecycle_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/session"
)

// writeAgentTestState writes a minimal active state.toml carrying the
// supplied agent slots and returns its path. The worktree layout is
// deterministic: <dir>/wt/demo on branch "demo" with git root <dir>/repo.
func writeAgentTestState(t *testing.T, dir string, agents []session.AgentState) string {
	t.Helper()
	path := filepath.Join(dir, "state.toml")
	st := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "sess-agent-test",
			Name:      "demo",
			Status:    "active",
			CreatedAt: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC),
		},
		Worktree: session.WorktreeState{
			Path:    filepath.Join(dir, "wt", "demo"),
			Branch:  "demo",
			GitRoot: filepath.Join(dir, "repo"),
		},
		Agents: agents,
	}
	err := session.WriteState(path, st)
	if err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

// ledgerText returns the raw ledger.jsonl content next to statePath.
func ledgerText(t *testing.T, statePath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(filepath.Dir(statePath), "ledger.jsonl"))
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return string(data)
}

func TestAgentAdd_PrimarySlotSkipsGitWorktree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, nil)
	runner := git.NewFakeRunner()

	state, _, err := lifecycle.AgentAdd(context.Background(),
		lifecycle.AgentAddDeps{Git: runner},
		lifecycle.AgentAddOptions{
			StatePath: path,
			Slot:      "primary",
			Provider:  "pi",
			Now:       time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
		})
	if err != nil {
		t.Fatalf("AgentAdd: %v", err)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("primary slot ran git commands: %v", runner.CommandStrings())
	}
	if len(state.Agents) != 1 || state.Agents[0].Slot != "primary" {
		t.Fatalf("agents = %+v, want single primary slot", state.Agents)
	}
	if !strings.Contains(ledgerText(t, path), "agent_added") {
		t.Fatal("ledger missing agent_added event")
	}
}

func TestAgentAdd_NonPrimaryCreatesSubWorktree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, []session.AgentState{{Slot: "primary", Provider: "pi"}})
	runner := git.NewFakeRunner()

	wtPath := filepath.Join(dir, "wt", "demo")
	wantPlan := git.PlanSubWorktree(git.WorktreePlan{Path: wtPath, Branch: "demo"}, "reviewer")

	state, plan, err := lifecycle.AgentAdd(context.Background(),
		lifecycle.AgentAddDeps{Git: runner},
		lifecycle.AgentAddOptions{
			StatePath: path,
			Slot:      "reviewer",
			Provider:  "claude",
			Now:       time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
		})
	if err != nil {
		t.Fatalf("AgentAdd: %v", err)
	}
	if plan.Path != wantPlan.Path || plan.Branch != wantPlan.Branch {
		t.Fatalf("plan = %+v, want %+v", plan, wantPlan)
	}
	wantCmd := strings.Join([]string{"worktree", "add", "-b", wantPlan.Branch, wantPlan.Path, "demo"}, " ")
	if got := strings.Join(runner.CommandStrings(), "\n"); !strings.Contains(got, wantCmd) {
		t.Fatalf("git commands = %q, want %q", got, wantCmd)
	}

	// Both the returned and the persisted state must carry the new slot.
	if len(state.Agents) != 2 {
		t.Fatalf("returned agents = %d, want 2", len(state.Agents))
	}
	assertPersistedReviewerSlot(t, path, wantPlan)
}

// assertPersistedReviewerSlot re-reads the state at path and verifies
// the reviewer slot was persisted with its sub-worktree plan.
func assertPersistedReviewerSlot(t *testing.T, path string, wantPlan git.SubWorktreePlan) {
	t.Helper()
	persisted, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if len(persisted.Agents) != 2 {
		t.Fatalf("persisted agents = %d, want 2", len(persisted.Agents))
	}
	got := persisted.Agents[1]
	if got.Slot != "reviewer" || got.Provider != "claude" || got.SubWorktree != wantPlan.Path || got.SubBranch != wantPlan.Branch {
		t.Fatalf("persisted agent = %+v", got)
	}
}

func TestAgentAdd_DuplicateSlotRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, []session.AgentState{{Slot: "reviewer", Provider: "pi"}})

	_, _, err := lifecycle.AgentAdd(context.Background(),
		lifecycle.AgentAddDeps{Git: git.NewFakeRunner()},
		lifecycle.AgentAddOptions{StatePath: path, Slot: "reviewer", Provider: "pi"})
	if !errors.Is(err, lifecycle.ErrAgentSlotExists) {
		t.Fatalf("want ErrAgentSlotExists, got %v", err)
	}
}

func TestAgentAdd_GitWorktreeAddFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, nil)
	runner := git.NewFakeRunner()

	wtPath := filepath.Join(dir, "wt", "demo")
	plan := git.PlanSubWorktree(git.WorktreePlan{Path: wtPath, Branch: "demo"}, "helper")
	runner.SetResponse(
		[]string{"worktree", "add", "-b", plan.Branch, plan.Path, "demo"},
		git.FakeResponse{Err: errTestGitFailed},
	)

	_, _, err := lifecycle.AgentAdd(context.Background(),
		lifecycle.AgentAddDeps{Git: runner},
		lifecycle.AgentAddOptions{StatePath: path, Slot: "helper", Provider: "pi"})
	if !errors.Is(err, errTestGitFailed) {
		t.Fatalf("want wrapped git error, got %v", err)
	}
	if !strings.Contains(err.Error(), "git worktree add") {
		t.Fatalf("err = %v, want git worktree add context", err)
	}
}

func TestAgentAdd_ReadStateError(t *testing.T) {
	t.Parallel()
	_, _, err := lifecycle.AgentAdd(context.Background(),
		lifecycle.AgentAddDeps{Git: git.NewFakeRunner()},
		lifecycle.AgentAddOptions{
			StatePath: filepath.Join(t.TempDir(), "missing", "state.toml"),
			Slot:      "primary",
		})
	if err == nil {
		t.Fatal("expected error for missing state path")
	}
}

func TestAgentAdd_ZeroNowDefaultsToWallClock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, nil)

	state, _, err := lifecycle.AgentAdd(context.Background(),
		lifecycle.AgentAddDeps{Git: git.NewFakeRunner()},
		lifecycle.AgentAddOptions{StatePath: path, Slot: "primary", Provider: "pi"})
	if err != nil {
		t.Fatalf("AgentAdd: %v", err)
	}
	if state.Agents[0].CreatedAt.IsZero() {
		t.Fatal("CreatedAt zero, want wall-clock default")
	}
}

func TestAgentStop_MarksSlotStopped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, []session.AgentState{{Slot: "primary", Provider: "pi", Status: "active"}})
	runner := git.NewFakeRunner()

	err := lifecycle.AgentStop(context.Background(), runner, lifecycle.AgentStopOptions{
		StatePath: path,
		Slot:      "primary",
		Now:       time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("AgentStop: %v", err)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("stop without RemoveWorktree ran git: %v", runner.CommandStrings())
	}
	persisted, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if persisted.Agents[0].Status != "stopped" {
		t.Fatalf("status = %q, want stopped", persisted.Agents[0].Status)
	}
	if !strings.Contains(ledgerText(t, path), "agent_stopped") {
		t.Fatal("ledger missing agent_stopped event")
	}
}

func TestAgentStop_MissingSlot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, []session.AgentState{{Slot: "primary"}})

	err := lifecycle.AgentStop(context.Background(), git.NewFakeRunner(), lifecycle.AgentStopOptions{
		StatePath: path,
		Slot:      "nope",
	})
	if !errors.Is(err, lifecycle.ErrAgentSlotMissing) {
		t.Fatalf("want ErrAgentSlotMissing, got %v", err)
	}
}

func TestAgentStop_RemoveWorktreeRunsGitCleanup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "wt", "demo--reviewer")
	path := writeAgentTestState(t, dir, []session.AgentState{{
		Slot: "reviewer", Provider: "claude", Status: "active",
		SubWorktree: sub, SubBranch: "demo--reviewer",
	}})
	runner := git.NewFakeRunner()

	err := lifecycle.AgentStop(context.Background(), runner, lifecycle.AgentStopOptions{
		StatePath:      path,
		Slot:           "reviewer",
		RemoveWorktree: true,
		Now:            time.Date(2026, 5, 22, 11, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("AgentStop: %v", err)
	}
	commands := strings.Join(runner.CommandStrings(), "\n")
	if !strings.Contains(commands, "worktree remove "+sub+" --force") {
		t.Fatalf("missing worktree remove; commands:\n%s", commands)
	}
	if !strings.Contains(commands, "branch -D demo--reviewer") {
		t.Fatalf("missing branch delete; commands:\n%s", commands)
	}

	// The removed worktree/branch must also be cleared from state:
	// a later `af done` iterates agents with SubWorktree != "" and
	// re-runs `git worktree remove` — against real git that fatals on
	// the already-removed path and aborts the whole done (found by the
	// docs/EXAMPLES.md vet run: add → stop --remove-worktree → done).
	state, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("ReadState after stop: %v", err)
	}
	if state.Agents[0].SubWorktree != "" || state.Agents[0].SubBranch != "" {
		t.Fatalf("SubWorktree/SubBranch not cleared after removal: %+v", state.Agents[0])
	}
}

// TestAgentStop_WithoutRemoveWorktreeKeepsPaths pins the counterpart:
// stopping WITHOUT --remove-worktree must keep the sub-worktree fields
// so `af done` still knows to clean the surviving worktree up.
func TestAgentStop_WithoutRemoveWorktreeKeepsPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "wt", "demo--reviewer")
	path := writeAgentTestState(t, dir, []session.AgentState{{
		Slot: "reviewer", Provider: "claude", Status: "active",
		SubWorktree: sub, SubBranch: "demo--reviewer",
	}})

	err := lifecycle.AgentStop(context.Background(), git.NewFakeRunner(), lifecycle.AgentStopOptions{
		StatePath: path,
		Slot:      "reviewer",
	})
	if err != nil {
		t.Fatalf("AgentStop: %v", err)
	}
	state, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("ReadState after stop: %v", err)
	}
	if state.Agents[0].SubWorktree != sub || state.Agents[0].SubBranch != "demo--reviewer" {
		t.Fatalf("sub-worktree fields must survive a stop without removal: %+v", state.Agents[0])
	}
}

func TestAgentStop_RemoveWorktreeGitFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "wt", "demo--reviewer")
	path := writeAgentTestState(t, dir, []session.AgentState{{
		Slot: "reviewer", Status: "active", SubWorktree: sub, SubBranch: "demo--reviewer",
	}})
	runner := git.NewFakeRunner()
	runner.SetResponse([]string{"worktree", "remove", sub, "--force"}, git.FakeResponse{Err: errTestGitFailed})

	err := lifecycle.AgentStop(context.Background(), runner, lifecycle.AgentStopOptions{
		StatePath:      path,
		Slot:           "reviewer",
		RemoveWorktree: true,
	})
	if !errors.Is(err, errTestGitFailed) {
		t.Fatalf("want wrapped git error, got %v", err)
	}

	// Failure must abort before any state write.
	persisted, readErr := session.ReadState(path)
	if readErr != nil {
		t.Fatalf("re-read state: %v", readErr)
	}
	if persisted.Agents[0].Status != "active" {
		t.Fatalf("status = %q, want active (unchanged on failure)", persisted.Agents[0].Status)
	}
}

func TestAgentStop_ReadStateError(t *testing.T) {
	t.Parallel()
	err := lifecycle.AgentStop(context.Background(), git.NewFakeRunner(), lifecycle.AgentStopOptions{
		StatePath: filepath.Join(t.TempDir(), "missing", "state.toml"),
		Slot:      "primary",
	})
	if err == nil {
		t.Fatal("expected error for missing state path")
	}
}

func TestAgentStop_ZeroNowDefaultsToWallClock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, []session.AgentState{{Slot: "primary", Status: "active"}})

	err := lifecycle.AgentStop(context.Background(), git.NewFakeRunner(), lifecycle.AgentStopOptions{
		StatePath: path,
		Slot:      "primary",
	})
	if err != nil {
		t.Fatalf("AgentStop: %v", err)
	}
	if !strings.Contains(ledgerText(t, path), "agent_stopped") {
		t.Fatal("ledger missing agent_stopped event")
	}
}

func TestAgentAdd_AppendEventFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, nil)
	// A directory at ledger.jsonl makes session.AppendEvent fail.
	err := os.MkdirAll(filepath.Join(dir, "ledger.jsonl"), 0o750)
	if err != nil {
		t.Fatalf("block ledger: %v", err)
	}

	_, _, err = lifecycle.AgentAdd(context.Background(),
		lifecycle.AgentAddDeps{Git: git.NewFakeRunner()},
		lifecycle.AgentAddOptions{StatePath: path, Slot: "primary", Provider: "pi"})
	if err == nil || !strings.Contains(err.Error(), "append event") {
		t.Fatalf("err = %v, want append event failure", err)
	}
}

func TestAgentStop_AppendEventFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAgentTestState(t, dir, []session.AgentState{{Slot: "primary", Status: "active"}})
	err := os.MkdirAll(filepath.Join(dir, "ledger.jsonl"), 0o750)
	if err != nil {
		t.Fatalf("block ledger: %v", err)
	}

	err = lifecycle.AgentStop(context.Background(), git.NewFakeRunner(), lifecycle.AgentStopOptions{
		StatePath: path,
		Slot:      "primary",
	})
	if err == nil || !strings.Contains(err.Error(), "append event") {
		t.Fatalf("err = %v, want append event failure", err)
	}
}

func TestSortedAgents_SortsBySlotWithoutMutatingInput(t *testing.T) {
	t.Parallel()
	state := session.State{Agents: []session.AgentState{
		{Slot: "zeta"},
		{Slot: "alpha"},
		{Slot: "mid"},
	}}

	got := lifecycle.SortedAgents(state)

	wantOrder := []string{"alpha", "mid", "zeta"}
	for i, want := range wantOrder {
		if got[i].Slot != want {
			t.Fatalf("got[%d].Slot = %q, want %q", i, got[i].Slot, want)
		}
	}
	if state.Agents[0].Slot != "zeta" {
		t.Fatalf("input mutated: %+v", state.Agents)
	}
}

func TestSortedAgents_EmptyState(t *testing.T) {
	t.Parallel()
	got := lifecycle.SortedAgents(session.State{})
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}
