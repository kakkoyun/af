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
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

// writeFinishState writes sessions/demo/state.toml in the given status
// with the supplied agents and tmux session name, returning the state path.
func writeFinishState(t *testing.T, sessionsDir, status, tmuxSession string, agents []session.AgentState) string {
	t.Helper()
	sessionDir := filepath.Join(sessionsDir, demoName)
	err := os.MkdirAll(sessionDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	path := filepath.Join(sessionDir, "state.toml")
	st := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "sess-done-test",
			Name:      demoName,
			Status:    status,
			CreatedAt: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		},
		Worktree: session.WorktreeState{
			Path:    filepath.Join(sessionsDir, "wt", demoName),
			Branch:  demoName,
			GitRoot: filepath.Join(sessionsDir, "repo"),
		},
		Execution: session.ExecutionState{
			Mode:        "local",
			Multiplexer: "tmux",
			TmuxSession: tmuxSession,
		},
		Agents: agents,
	}
	err = session.WriteState(path, st)
	if err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

func TestFinishWorkstream_CompletesAndArchives(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archiveDir := filepath.Join(home, "archive")
	path := writeFinishState(t, sessionsDir, "active", "af-demo", nil)

	runner := git.NewFakeRunner()
	muxFake := mux.NewFakeMultiplexer()
	err := muxFake.CreateSession(context.Background(), "af-demo", home)
	if err != nil {
		t.Fatalf("pre-create tmux session: %v", err)
	}

	state, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: runner, Mux: muxFake},
		lifecycle.DoneOptions{
			StatePath:  path,
			ArchiveDir: archiveDir,
			Now:        time.Date(2026, 5, 24, 16, 0, 0, 0, time.UTC),
		})
	if err != nil {
		t.Fatalf("FinishWorkstream: %v", err)
	}
	if state.Session.Status != string(lifecycle.Completed) {
		t.Fatalf("status = %q, want completed", state.Session.Status)
	}

	// The tmux session must be gone.
	exists, err := muxFake.SessionExists(context.Background(), "af-demo")
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if exists {
		t.Fatal("tmux session still alive after done")
	}

	// The primary worktree removal must have been requested.
	commands := strings.Join(runner.CommandStrings(), "\n")
	if !strings.Contains(commands, "worktree remove "+filepath.Join(sessionsDir, "wt", "demo")+" --force") {
		t.Fatalf("missing primary worktree removal; commands:\n%s", commands)
	}

	// The session dir must have moved into the archive with its ledger.
	_, statErr := os.Stat(filepath.Dir(path))
	if !os.IsNotExist(statErr) {
		t.Fatalf("session dir still present after archive (stat: %v)", statErr)
	}
	archivedLedger, err := os.ReadFile(filepath.Join(archiveDir, "demo", "ledger.jsonl")) //nolint:gosec // test path under t.TempDir.
	if err != nil {
		t.Fatalf("read archived ledger: %v", err)
	}
	if !strings.Contains(string(archivedLedger), "completed") {
		t.Fatal("archived ledger missing completed event")
	}
}

func TestFinishWorkstream_ForceAbandons(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	path := writeFinishState(t, sessionsDir, "active", "", nil)

	state, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner()},
		lifecycle.DoneOptions{StatePath: path, Force: true})
	if err != nil {
		t.Fatalf("FinishWorkstream --force: %v", err)
	}
	if state.Session.Status != string(lifecycle.Abandoned) {
		t.Fatalf("status = %q, want abandoned", state.Session.Status)
	}
	if !strings.Contains(ledgerText(t, path), "abandoned") {
		t.Fatal("ledger missing abandoned event")
	}
}

func TestFinishWorkstream_RejectsTerminalStates(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"completed", "abandoned"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			path := writeFinishState(t, filepath.Join(home, "sessions"), status, "", nil)

			_, err := lifecycle.FinishWorkstream(context.Background(),
				lifecycle.DoneDeps{Git: git.NewFakeRunner()},
				lifecycle.DoneOptions{StatePath: path})
			if !errors.Is(err, lifecycle.ErrDoneAlreadyTerminal) {
				t.Fatalf("want ErrDoneAlreadyTerminal, got %v", err)
			}
		})
	}
}

func TestFinishWorkstream_ReadStateError(t *testing.T) {
	t.Parallel()
	_, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner()},
		lifecycle.DoneOptions{StatePath: filepath.Join(t.TempDir(), "missing", "state.toml")})
	if err == nil {
		t.Fatal("expected error for missing state path")
	}
}

func TestFinishWorkstream_RemovesSubWorktrees(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	subA := filepath.Join(home, "wt", "demo--a")
	subB := filepath.Join(home, "wt", "demo--b")
	path := writeFinishState(t, sessionsDir, "active", "", []session.AgentState{
		{Slot: "primary"}, // no sub-worktree — must be skipped
		{Slot: "a", SubWorktree: subA, SubBranch: "demo--a"},
		{Slot: "b", SubWorktree: subB, SubBranch: "demo--b"},
	})
	runner := git.NewFakeRunner()

	_, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: runner},
		lifecycle.DoneOptions{StatePath: path})
	if err != nil {
		t.Fatalf("FinishWorkstream: %v", err)
	}
	commands := strings.Join(runner.CommandStrings(), "\n")
	for _, sub := range []string{subA, subB} {
		if !strings.Contains(commands, "worktree remove "+sub+" --force") {
			t.Fatalf("missing sub-worktree removal for %s; commands:\n%s", sub, commands)
		}
	}
}

func TestFinishWorkstream_SubWorktreeRemoveErrorAborts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	sub := filepath.Join(home, "wt", "demo--a")
	path := writeFinishState(t, sessionsDir, "active", "", []session.AgentState{
		{Slot: "a", SubWorktree: sub, SubBranch: "demo--a"},
	})
	runner := git.NewFakeRunner()
	runner.SetResponse([]string{"worktree", "remove", sub, "--force"}, git.FakeResponse{Err: errTestGitFailed})

	_, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: runner},
		lifecycle.DoneOptions{StatePath: path})
	if !errors.Is(err, errTestGitFailed) {
		t.Fatalf("want wrapped git error, got %v", err)
	}

	// State must remain active on failure.
	persisted, readErr := session.ReadState(path)
	if readErr != nil {
		t.Fatalf("re-read state: %v", readErr)
	}
	if persisted.Session.Status != "active" {
		t.Fatalf("status = %q, want active (unchanged on failure)", persisted.Session.Status)
	}
}

func TestFinishWorkstream_PrimaryWorktreeRemoveErrorAborts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	path := writeFinishState(t, sessionsDir, "active", "", nil)
	runner := git.NewFakeRunner()
	primary := filepath.Join(sessionsDir, "wt", "demo")
	runner.SetResponse([]string{"worktree", "remove", primary, "--force"}, git.FakeResponse{Err: errTestGitFailed})

	_, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: runner},
		lifecycle.DoneOptions{StatePath: path})
	if !errors.Is(err, errTestGitFailed) {
		t.Fatalf("want wrapped git error, got %v", err)
	}
	if !strings.Contains(err.Error(), "remove primary worktree") {
		t.Fatalf("err = %v, want primary-worktree context", err)
	}
}

func TestFinishWorkstream_ForceIgnoresGitRemovalErrors(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	sub := filepath.Join(home, "wt", "demo--a")
	path := writeFinishState(t, sessionsDir, "active", "", []session.AgentState{
		{Slot: "a", SubWorktree: sub, SubBranch: "demo--a"},
	})
	runner := git.NewFakeRunner()
	primary := filepath.Join(sessionsDir, "wt", "demo")
	runner.SetResponse([]string{"worktree", "remove", sub, "--force"}, git.FakeResponse{Err: errTestGitFailed})
	runner.SetResponse([]string{"worktree", "remove", primary, "--force"}, git.FakeResponse{Err: errTestGitFailed})

	state, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: runner},
		lifecycle.DoneOptions{StatePath: path, Force: true})
	if err != nil {
		t.Fatalf("FinishWorkstream --force with git failures: %v", err)
	}
	if state.Session.Status != string(lifecycle.Abandoned) {
		t.Fatalf("status = %q, want abandoned", state.Session.Status)
	}
}

func TestFinishWorkstream_EmptyArchiveDirSkipsArchive(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	path := writeFinishState(t, sessionsDir, "active", "", nil)

	_, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner()},
		lifecycle.DoneOptions{StatePath: path})
	if err != nil {
		t.Fatalf("FinishWorkstream: %v", err)
	}
	_, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("state.toml missing without ArchiveDir: %v", statErr)
	}
}

func TestFinishWorkstream_ArchiveRootCreateFails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	path := writeFinishState(t, sessionsDir, "active", "", nil)

	// A regular file blocks MkdirAll for any path nested beneath it.
	blocker := filepath.Join(home, "blocker")
	err := os.WriteFile(blocker, []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	_, err = lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner()},
		lifecycle.DoneOptions{StatePath: path, ArchiveDir: filepath.Join(blocker, "archive")})
	if err == nil {
		t.Fatal("expected archive error when archive root cannot be created")
	}
	if !strings.Contains(err.Error(), "archive") {
		t.Fatalf("err = %v, want archive context", err)
	}
}

func TestFinishWorkstream_AppendEventFailure(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	path := writeFinishState(t, sessionsDir, "active", "", nil)
	// A directory at ledger.jsonl makes session.AppendEvent fail.
	err := os.MkdirAll(filepath.Join(filepath.Dir(path), "ledger.jsonl"), 0o750)
	if err != nil {
		t.Fatalf("block ledger: %v", err)
	}

	_, err = lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner()},
		lifecycle.DoneOptions{StatePath: path})
	if err == nil || !strings.Contains(err.Error(), "append event") {
		t.Fatalf("err = %v, want append event failure", err)
	}
}

func TestFinishWorkstream_ArchiveRenameFails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	archiveDir := filepath.Join(home, "archive")
	path := writeFinishState(t, sessionsDir, "active", "", nil)

	// Pre-create a non-empty archive/<name> so os.Rename fails.
	err := os.MkdirAll(filepath.Join(archiveDir, "demo"), 0o750)
	if err != nil {
		t.Fatalf("mkdir archived name: %v", err)
	}
	err = os.WriteFile(filepath.Join(archiveDir, "demo", "keep"), []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("write keep file: %v", err)
	}

	_, err = lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner()},
		lifecycle.DoneOptions{StatePath: path, ArchiveDir: archiveDir})
	if err == nil {
		t.Fatal("expected archive error when target dir is occupied")
	}
	if !strings.Contains(err.Error(), "archive") {
		t.Fatalf("err = %v, want archive context", err)
	}
}
