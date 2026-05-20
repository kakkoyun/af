package lifecycle_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/obsidian"
	"github.com/kakkoyun/af/internal/session"
)

// repoSlug is a fixed logical identifier used across the create-test
// suite. It is path-shaped (host/owner/repo) so primary-worktree
// planning produces a recognisable filesystem layout under the host's
// path separator.
func repoSlug() string {
	return filepath.Join("github.com", "owner", "repo")
}

func mkdirT(t *testing.T, path string) {
	t.Helper()
	err := os.MkdirAll(path, 0o750)
	if err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestCreate_ProducesStateLedgerWorktreeAndTmuxSession(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)

	gitRunner := git.NewFakeRunner()
	muxFake := mux.NewFakeMultiplexer()
	noteStore := obsidian.NewMemoryStore()

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   gitRunner,
		Mux:   muxFake,
		Agent: agent.NewFake("pi"),
		Notes: noteStore,
	}, lifecycle.CreateOptions{
		Name:         "demo",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     repoSlug(),
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     filepath.Join(home, "sessions"),
		NotesDir:     filepath.Join(home, "notes"),
		AgentName:    "pi",
		Now:          time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	assertCreateResult(t, res, gitRoot, gitRunner, muxFake)
}

func assertCreateResult(t *testing.T, res lifecycle.CreateResult, gitRoot string, runner *git.FakeRunner, muxFake *mux.FakeMultiplexer) {
	t.Helper()
	assertCreateResultIdentity(t, res)
	assertCreateResultState(t, res, gitRoot)
	assertCreateResultEffects(t, res, runner, muxFake)
}

func assertCreateResultIdentity(t *testing.T, res lifecycle.CreateResult) {
	t.Helper()
	if res.SessionName != "demo" {
		t.Fatalf("SessionName = %q, want demo", res.SessionName)
	}
	if !strings.HasSuffix(res.WorktreePath, filepath.Join(repoSlug(), "demo")) {
		t.Fatalf("WorktreePath = %q", res.WorktreePath)
	}
	if !strings.Contains(res.TmuxSession, "af-demo") {
		t.Fatalf("TmuxSession = %q, want af-demo prefix", res.TmuxSession)
	}
}

func assertCreateResultState(t *testing.T, res lifecycle.CreateResult, gitRoot string) {
	t.Helper()
	state, err := session.ReadState(res.StatePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.Session.Name != "demo" {
		t.Fatalf("state.session.name = %q, want demo", state.Session.Name)
	}
	if state.Worktree.Branch == "" || state.Worktree.GitRoot != gitRoot {
		t.Fatalf("state.worktree unexpected: %+v", state.Worktree)
	}
	_, ledgerStatErr := os.Stat(res.LedgerPath)
	if ledgerStatErr != nil {
		t.Fatalf("ledger missing: %v", ledgerStatErr)
	}
	if res.NotePath == "" {
		t.Fatal("NotePath empty, want a note path")
	}
}

func assertCreateResultEffects(t *testing.T, _ lifecycle.CreateResult, runner *git.FakeRunner, muxFake *mux.FakeMultiplexer) {
	t.Helper()
	gotCommands := strings.Join(runner.CommandStrings(), "\n")
	if !strings.Contains(gotCommands, "worktree add -b") {
		t.Fatalf("did not invoke worktree add; commands:\n%s", gotCommands)
	}

	sessions, err := muxFake.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}
}

func TestCreate_BareSkipsMuxAndAgent(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)

	gitRunner := git.NewFakeRunner()

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git: gitRunner,
	}, lifecycle.CreateOptions{
		Name:         "bare-demo",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     repoSlug(),
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     filepath.Join(home, "sessions"),
		AgentName:    "pi",
		Bare:         true,
		Now:          time.Now(),
	})
	if err != nil {
		t.Fatalf("Create bare: %v", err)
	}
	if res.TmuxSession != "" {
		t.Fatalf("TmuxSession = %q on --bare, want empty", res.TmuxSession)
	}
}

func TestCreate_RejectsEmptyRequiredFields(t *testing.T) {
	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{Git: git.NewFakeRunner()}, lifecycle.CreateOptions{
		Bare: true,
	})
	if err == nil {
		t.Fatal("Create with empty opts returned nil, want error")
	}
}
