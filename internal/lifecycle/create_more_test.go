package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
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

// makeCreateOpts returns a fully-populated bare CreateOptions rooted at home.
func makeCreateOpts(t *testing.T, home string) lifecycle.CreateOptions {
	t.Helper()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)
	return lifecycle.CreateOptions{
		Name:         "demo",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     repoSlug(),
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     filepath.Join(home, "sessions"),
		AgentName:    "pi",
		Bare:         true,
		Now:          time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC),
	}
}

// worktreeAddFailRunner fails every `git worktree add` invocation and
// delegates everything else to the wrapped FakeRunner.
type worktreeAddFailRunner struct {
	inner *git.FakeRunner
	err   error
}

func (r worktreeAddFailRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
		return nil, r.err
	}
	out, err := r.inner.Run(ctx, dir, args...)
	if err != nil {
		return out, fmt.Errorf("fake git run: %w", err)
	}
	return out, nil
}

// errNoteStoreWrite is returned by failingNoteStore for every operation.
var errNoteStoreWrite = errors.New("note store write failed")

// failingNoteStore is an obsidian.Store whose operations always fail.
type failingNoteStore struct{}

func (failingNoteStore) Read(_ context.Context, _ string) (obsidian.Note, error) {
	return obsidian.Note{}, errNoteStoreWrite
}

func (failingNoteStore) Write(_ context.Context, _ string, _ obsidian.Note) error {
	return errNoteStoreWrite
}

// failMux injects failures into selected FakeMultiplexer operations.
var errMuxBoom = errors.New("mux boom")

type failMux struct {
	*mux.FakeMultiplexer

	failCreate   bool
	failSetEnv   bool
	failSendKeys bool
}

func (m *failMux) CreateSession(ctx context.Context, name, cwd string) error {
	if m.failCreate {
		return errMuxBoom
	}
	err := m.FakeMultiplexer.CreateSession(ctx, name, cwd)
	if err != nil {
		return fmt.Errorf("fake create session: %w", err)
	}
	return nil
}

func (m *failMux) SetEnv(ctx context.Context, sessionName, key, value string) error {
	if m.failSetEnv {
		return errMuxBoom
	}
	err := m.FakeMultiplexer.SetEnv(ctx, sessionName, key, value)
	if err != nil {
		return fmt.Errorf("fake set env: %w", err)
	}
	return nil
}

func (m *failMux) SendKeys(ctx context.Context, sessionName, pane, keys string) error {
	if m.failSendKeys {
		return errMuxBoom
	}
	err := m.FakeMultiplexer.SendKeys(ctx, sessionName, pane, keys)
	if err != nil {
		return fmt.Errorf("fake send keys: %w", err)
	}
	return nil
}

// emptyLaunchAgent is an agent whose LaunchCmd is empty, so create must
// skip the agent-launch SendKeys.
type emptyLaunchAgent struct {
	*agent.Fake
}

func (emptyLaunchAgent) LaunchCmd(_ agent.LaunchOpts) []string { return nil }

func TestCreate_WritesResolvedControlToState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		wantMode  string
		mode      agent.ApprovalMode
		maxAgents int
	}{
		{"auto", "auto", agent.ApprovalAuto, 3},
		{"yolo", "yolo", agent.ApprovalYolo, 0},
		{"default", "", agent.ApprovalDefault, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			opts := makeCreateOpts(t, home)
			opts.Control = lifecycle.ControlContext{
				ApprovalMode: tt.mode,
				MaxAgents:    tt.maxAgents,
			}

			res, err := lifecycle.Create(context.Background(),
				lifecycle.CreateDeps{Git: git.NewFakeRunner()}, opts)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			state, err := session.ReadState(res.StatePath)
			if err != nil {
				t.Fatalf("ReadState: %v", err)
			}
			if state.Session.ApprovalMode != tt.wantMode {
				t.Errorf("ApprovalMode = %q, want %q", state.Session.ApprovalMode, tt.wantMode)
			}
			if state.Session.MaxAgents != tt.maxAgents {
				t.Errorf("MaxAgents = %d, want %d", state.Session.MaxAgents, tt.maxAgents)
			}
		})
	}
}

func TestCreate_AutoGeneratesNameAndTimestamp(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	opts.Name = ""
	opts.Now = time.Time{}

	res, err := lifecycle.Create(context.Background(),
		lifecycle.CreateDeps{Git: git.NewFakeRunner()}, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.SessionName == "" {
		t.Fatal("SessionName empty, want auto-generated name")
	}
	if res.Branch == "" {
		t.Fatal("Branch empty, want derived branch")
	}
	state, err := session.ReadState(res.StatePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.Session.CreatedAt.IsZero() {
		t.Fatal("CreatedAt zero, want wall-clock default")
	}
}

func TestCreate_ValidatesOptionsAndDeps(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	base := makeCreateOpts(t, home)

	cases := []struct {
		mutate func(*lifecycle.CreateOptions)
		deps   lifecycle.CreateDeps
		name   string
	}{
		{func(o *lifecycle.CreateOptions) { o.GitRoot = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}, "empty git root"},
		{func(o *lifecycle.CreateOptions) { o.RepoSlug = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}, "empty repo slug"},
		{func(o *lifecycle.CreateOptions) { o.WorktreeRoot = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}, "empty worktree root"},
		{func(o *lifecycle.CreateOptions) { o.StateDir = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}, "empty state dir"},
		{func(o *lifecycle.CreateOptions) { o.FromBranch = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}, "empty from-branch"},
		{func(*lifecycle.CreateOptions) {}, lifecycle.CreateDeps{}, "nil git runner"},
		{
			func(o *lifecycle.CreateOptions) { o.Bare = false },
			lifecycle.CreateDeps{Git: git.NewFakeRunner()},
			"nil mux for non-bare",
		},
		{
			func(o *lifecycle.CreateOptions) { o.Bare = false },
			lifecycle.CreateDeps{Git: git.NewFakeRunner(), Mux: mux.NewFakeMultiplexer()},
			"nil agent for non-bare",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := base
			tt.mutate(&opts)
			_, err := lifecycle.Create(context.Background(), tt.deps, opts)
			if !errors.Is(err, lifecycle.ErrCreate) {
				t.Fatalf("want ErrCreate, got %v", err)
			}
		})
	}
}

// TestCreate_HostAgentlessAllowsNilAgent is the issue #33 Fix 3
// regression pin: a --sandbox create passes deps.Agent=nil (the agent
// launches inside the VM instead, via `slicer wt push --launch`), but
// Create must not reject that the way it rejects an accidental nil
// agent for an ordinary non-bare create. The tmux session is still
// created, and the primary agent slot in state.toml is still recorded
// from AgentName (never from deps.Agent).
func TestCreate_HostAgentlessAllowsNilAgent(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	opts.Bare = false
	opts.HostAgentless = true
	muxFake := mux.NewFakeMultiplexer()

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git: git.NewFakeRunner(),
		Mux: muxFake,
	}, opts)
	if err != nil {
		t.Fatalf("Create with HostAgentless and nil Agent: %v", err)
	}
	if res.TmuxSession == "" {
		t.Fatal("TmuxSession empty, want a session even for a host-agentless sandbox create")
	}

	state, err := session.ReadState(res.StatePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if len(state.Agents) != 1 || state.Agents[0].Provider != "pi" {
		t.Fatalf("state.Agents = %#v, want primary slot recorded from AgentName (pi)", state.Agents)
	}
}

// TestCreate_NilAgentStillRejectedWithoutHostAgentless pins that
// HostAgentless is required to opt out of the nil-agent guard — an
// ordinary non-bare, non-sandbox create with a nil Agent must still
// fail the way it always has.
func TestCreate_NilAgentStillRejectedWithoutHostAgentless(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	opts.Bare = false

	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git: git.NewFakeRunner(),
		Mux: mux.NewFakeMultiplexer(),
	}, opts)
	if !errors.Is(err, lifecycle.ErrCreate) {
		t.Fatalf("want ErrCreate, got %v", err)
	}
}

func TestCreate_GitWorktreeAddFailureAborts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	runner := worktreeAddFailRunner{inner: git.NewFakeRunner(), err: errTestGitFailed}

	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{Git: runner}, opts)
	if !errors.Is(err, errTestGitFailed) {
		t.Fatalf("want wrapped git error, got %v", err)
	}
	if !strings.Contains(err.Error(), "git worktree add") {
		t.Fatalf("err = %v, want git worktree add context", err)
	}
	// No session dir may be left behind after the aborted create.
	_, statErr := os.Stat(filepath.Join(opts.StateDir, "demo"))
	if !os.IsNotExist(statErr) {
		t.Fatalf("session dir created despite git failure (stat: %v)", statErr)
	}
}

func TestCreate_StateRootLockFailureAborts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	// A regular file at StateDir makes the .af.lock MkdirAll fail.
	err := os.WriteFile(opts.StateDir, []byte("not a dir"), 0o600)
	if err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	_, err = lifecycle.Create(context.Background(), lifecycle.CreateDeps{Git: git.NewFakeRunner()}, opts)
	if err == nil {
		t.Fatal("expected error when state root cannot be locked")
	}
	if !strings.Contains(err.Error(), "lock state root") {
		t.Fatalf("err = %v, want lock state root context", err)
	}
}

// TestCreate_StateRootLockFileExistsAfterCreate pins that provisionWorkstream
// still serializes through a session.LockFileName flock directly under
// StateDir (now via the shared session.WithDirLock primitive instead of
// a duplicated lifecycle-local helper), and that StateDir itself is
// still auto-created on first use rather than requiring the caller to
// pre-create it.
func TestCreate_StateRootLockFileExistsAfterCreate(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	_, statErr := os.Stat(opts.StateDir)
	if !os.IsNotExist(statErr) {
		t.Fatalf("StateDir must not pre-exist for this test, stat err = %v", statErr)
	}

	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{Git: git.NewFakeRunner()}, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err = os.Stat(filepath.Join(opts.StateDir, session.LockFileName))
	if err != nil {
		t.Fatalf("state-root lock file missing: %v", err)
	}
}

func TestCreate_NoteWriteFailureAborts(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	opts.NotesDir = filepath.Join(home, "notes")

	_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Notes: failingNoteStore{},
	}, opts)
	if !errors.Is(err, errNoteStoreWrite) {
		t.Fatalf("want wrapped note-store error, got %v", err)
	}
}

func TestCreate_MuxFailuresAbort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mux  *failMux
		name string
	}{
		{&failMux{FakeMultiplexer: mux.NewFakeMultiplexer(), failCreate: true}, "create session fails"},
		{&failMux{FakeMultiplexer: mux.NewFakeMultiplexer(), failSetEnv: true}, "set env fails"},
		{&failMux{FakeMultiplexer: mux.NewFakeMultiplexer(), failSendKeys: true}, "send keys fails"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			opts := makeCreateOpts(t, home)
			opts.Bare = false

			_, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
				Git:   git.NewFakeRunner(),
				Mux:   tt.mux,
				Agent: agent.NewFake("pi"),
			}, opts)
			if !errors.Is(err, errMuxBoom) {
				t.Fatalf("want wrapped mux error, got %v", err)
			}
		})
	}
}

func TestCreate_SessionDirBlockedByFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	// A regular file at sessions/demo passes the dir-only collision
	// check but blocks session-dir creation.
	mkdirT(t, opts.StateDir)
	err := os.WriteFile(filepath.Join(opts.StateDir, "demo"), []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	_, err = lifecycle.Create(context.Background(), lifecycle.CreateDeps{Git: git.NewFakeRunner()}, opts)
	if err == nil || !strings.Contains(err.Error(), "create session dir") {
		t.Fatalf("err = %v, want create session dir failure", err)
	}
}

func TestCreate_WorktreeParentBlockedByFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	// A regular file at the worktree root blocks MkdirAll for the
	// planned worktree parent directory.
	err := os.WriteFile(opts.WorktreeRoot, []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	_, err = lifecycle.Create(context.Background(), lifecycle.CreateDeps{Git: git.NewFakeRunner()}, opts)
	if err == nil || !strings.Contains(err.Error(), "create worktree parent") {
		t.Fatalf("err = %v, want create worktree parent failure", err)
	}
}

func TestCreate_StateSymlinkBlockedByFile(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	// A regular file at the planned worktree path (wt/<slug>/demo, since
	// the empty prefix keeps branch == name) blocks the .af discovery dir.
	plannedWorktree := filepath.Join(opts.WorktreeRoot, opts.RepoSlug, "demo")
	mkdirT(t, filepath.Dir(plannedWorktree))
	err := os.WriteFile(plannedWorktree, []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	_, err = lifecycle.Create(context.Background(), lifecycle.CreateDeps{Git: git.NewFakeRunner()}, opts)
	if err == nil || !strings.Contains(err.Error(), "symlink .af/state.toml") {
		t.Fatalf("err = %v, want state symlink failure", err)
	}
}

func TestCreate_EmptyLaunchCmdSkipsAgentLaunch(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := makeCreateOpts(t, home)
	opts.Bare = false
	// SendKeys failure would surface as an error, proving SendKeys is
	// never reached for an empty launch command.
	muxFake := &failMux{FakeMultiplexer: mux.NewFakeMultiplexer(), failSendKeys: true}

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Mux:   muxFake,
		Agent: emptyLaunchAgent{Fake: agent.NewFake("pi")},
	}, opts)
	if err != nil {
		t.Fatalf("Create with empty launch cmd: %v", err)
	}
	if res.TmuxSession == "" {
		t.Fatal("TmuxSession empty, want session name even without agent launch")
	}
}
