package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/obsidian"
	"github.com/kakkoyun/af/internal/session"
	"github.com/kakkoyun/af/internal/workstream"
)

// demoName is the canonical session name used across the create-test
// suite.
const demoName = "demo"

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
		Name:         demoName,
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
	if res.SessionName != demoName {
		t.Fatalf("SessionName = %q, want demo", res.SessionName)
	}
	if !strings.HasSuffix(res.WorktreePath, filepath.Join(repoSlug(), demoName)) {
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
	if state.Session.Name != demoName {
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

func assertCreateResultEffects(t *testing.T, res lifecycle.CreateResult, runner *git.FakeRunner, muxFake *mux.FakeMultiplexer) {
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
	gotSession, err := muxFake.GetEnv(context.Background(), res.TmuxSession, "AF_SESSION")
	if err != nil {
		t.Fatalf("GetEnv(AF_SESSION): %v", err)
	}
	if gotSession != res.SessionName {
		t.Fatalf("AF_SESSION = %q, want %q", gotSession, res.SessionName)
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

func TestCreate_PreservesSlicerResourcesInState(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)

	gitRunner := git.NewFakeRunner()
	gitRunner.SetResponse([]string{"worktree", "add"}, git.FakeResponse{})

	wantResources := lifecycle.SandboxResourceProfile{
		ProfileName:  "tight",
		VCPU:         2,
		RAMGB:        4,
		StorageSize:  "25G",
		GPUCount:     0,
		Hypervisor:   "firecracker",
		ManagedGroup: "af-owner-repo-tight",
	}

	result, err := lifecycle.Create(
		context.Background(),
		lifecycle.CreateDeps{Git: gitRunner},
		lifecycle.CreateOptions{
			Name:             "tight-test",
			FromBranch:       "main",
			GitRoot:          gitRoot,
			RepoSlug:         repoSlug(),
			WorktreeRoot:     filepath.Join(home, "worktrees"),
			StateDir:         filepath.Join(home, "state"),
			Bare:             true,
			SandboxGroup:     wantResources.ManagedGroup,
			SandboxResources: wantResources,
		},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	state, err := session.ReadState(result.StatePath)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	ex := state.Execution
	if ex.SandboxResourceProfile != "tight" {
		t.Errorf("SandboxResourceProfile = %q, want tight", ex.SandboxResourceProfile)
	}
	if ex.SandboxResourceVCPU != 2 {
		t.Errorf("SandboxResourceVCPU = %d, want 2", ex.SandboxResourceVCPU)
	}
	if ex.SandboxManagedGroup != "af-owner-repo-tight" {
		t.Errorf("SandboxManagedGroup = %q, want af-owner-repo-tight", ex.SandboxManagedGroup)
	}
}

// TestCreate_RejectsActiveNameCollision verifies the ADR-069 §3 strict
// name collision check fires when a sessions/<name> dir already
// exists.
func TestCreate_RejectsActiveNameCollision(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)
	stateDir := filepath.Join(home, "sessions")
	// Pre-create the colliding active dir.
	mkdirT(t, filepath.Join(stateDir, "dup"))

	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Mux:   mux.NewFakeMultiplexer(),
		Agent: agent.NewFake("pi"),
		Notes: obsidian.NewMemoryStore(),
	}, lifecycle.CreateOptions{
		Name:         "dup",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     repoSlug(),
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     stateDir,
		AgentName:    "pi",
		Now:          time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, lifecycle.ErrNameCollision) {
		t.Fatalf("want ErrNameCollision, got %v", err)
	}
}

// TestCreate_RejectsArchivedNameCollision verifies the collision check
// also rejects names that exist only under the archive directory.
func TestCreate_RejectsArchivedNameCollision(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)
	stateDir := filepath.Join(home, "sessions")
	archiveDir := filepath.Join(home, "archive")
	// Pre-create the colliding archived dir.
	mkdirT(t, filepath.Join(archiveDir, "old"))

	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Mux:   mux.NewFakeMultiplexer(),
		Agent: agent.NewFake("pi"),
		Notes: obsidian.NewMemoryStore(),
	}, lifecycle.CreateOptions{
		Name:         "old",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     repoSlug(),
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     stateDir,
		ArchiveDir:   archiveDir,
		AgentName:    "pi",
		Now:          time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, lifecycle.ErrNameCollision) {
		t.Fatalf("want ErrNameCollision, got %v", err)
	}
}

// TestCreate_AllowsFreshNameWithEmptyArchiveDir verifies an empty
// ArchiveDir disables the archive check (back-compat).
func TestCreate_AllowsFreshNameWithEmptyArchiveDir(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)

	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Mux:   mux.NewFakeMultiplexer(),
		Agent: agent.NewFake("pi"),
		Notes: obsidian.NewMemoryStore(),
	}, lifecycle.CreateOptions{
		Name:         "fresh-name",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     repoSlug(),
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     filepath.Join(home, "sessions"),
		// ArchiveDir intentionally empty.
		AgentName: "pi",
		Now:       time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create with empty ArchiveDir: %v", err)
	}
}

// TestCreate_RejectsTraversalNames verifies ADR-069 collision checks
// cannot be bypassed with names that escape the state root or would
// produce malformed git refs.
func TestCreate_RejectsTraversalNames(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)
	stateDir := filepath.Join(home, "sessions")

	for _, name := range []string{"../evil", "a/../../b", "/abs", "-rf", "a b"} {
		t.Run(name, func(t *testing.T) {
			_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
				Git:   git.NewFakeRunner(),
				Mux:   mux.NewFakeMultiplexer(),
				Agent: agent.NewFake("pi"),
				Notes: obsidian.NewMemoryStore(),
			}, lifecycle.CreateOptions{
				Name:         name,
				FromBranch:   "main",
				GitRoot:      gitRoot,
				RepoSlug:     repoSlug(),
				WorktreeRoot: filepath.Join(home, "wt"),
				StateDir:     stateDir,
				AgentName:    "pi",
				Now:          time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
			})
			if !errors.Is(err, lifecycle.ErrCreate) {
				t.Fatalf("want ErrCreate, got %v", err)
			}
			if !errors.Is(err, workstream.ErrInvalidSessionName) {
				t.Fatalf("want ErrInvalidSessionName, got %v", err)
			}
			// Nothing may leak outside (or inside) the state root.
			entries, readErr := os.ReadDir(home)
			if readErr != nil {
				t.Fatalf("read home: %v", readErr)
			}
			for _, entry := range entries {
				if entry.Name() != "repo" {
					t.Fatalf("unexpected entry %q created under home", entry.Name())
				}
			}
		})
	}
}

// TestCreate_ConcurrentSameNameExactlyOneWins verifies the ADR-069
// collision check and session-dir creation run under the state-root
// lock, so two racing creates cannot both claim one name.
func TestCreate_ConcurrentSameNameExactlyOneWins(t *testing.T) {
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)
	stateDir := filepath.Join(home, "sessions")

	run := func(worktreeRoot string) error {
		_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
			Git:   git.NewFakeRunner(),
			Mux:   mux.NewFakeMultiplexer(),
			Agent: agent.NewFake("pi"),
			Notes: obsidian.NewMemoryStore(),
		}, lifecycle.CreateOptions{
			Name:         "raced",
			FromBranch:   "main",
			GitRoot:      gitRoot,
			RepoSlug:     repoSlug(),
			WorktreeRoot: worktreeRoot,
			StateDir:     stateDir,
			AgentName:    "pi",
			Now:          time.Date(2026, 7, 3, 1, 0, 0, 0, time.UTC),
		})
		if err != nil {
			return fmt.Errorf("create: %w", err)
		}
		return nil
	}

	const attempts = 4
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- run(filepath.Join(home, fmt.Sprintf("wt%d", i)))
		}()
	}
	wg.Wait()
	close(errs)

	var wins, collisions int
	for err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, lifecycle.ErrNameCollision):
			collisions++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || collisions != attempts-1 {
		t.Fatalf("wins = %d, collisions = %d; want exactly 1 win and %d collisions", wins, collisions, attempts-1)
	}
}
