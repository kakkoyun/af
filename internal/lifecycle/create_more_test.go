package lifecycle_test

import (
	"context"
	"errors"
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
	return r.inner.Run(ctx, dir, args...)
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
	return m.FakeMultiplexer.CreateSession(ctx, name, cwd)
}

func (m *failMux) SetEnv(ctx context.Context, sessionName, key, value string) error {
	if m.failSetEnv {
		return errMuxBoom
	}
	return m.FakeMultiplexer.SetEnv(ctx, sessionName, key, value)
}

func (m *failMux) SendKeys(ctx context.Context, sessionName, pane, keys string) error {
	if m.failSendKeys {
		return errMuxBoom
	}
	return m.FakeMultiplexer.SendKeys(ctx, sessionName, pane, keys)
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
		mode      agent.ApprovalMode
		wantMode  string
		maxAgents int
	}{
		{"auto", agent.ApprovalAuto, "auto", 3},
		{"yolo", agent.ApprovalYolo, "yolo", 0},
		{"default", agent.ApprovalDefault, "", 0},
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
		name   string
		mutate func(*lifecycle.CreateOptions)
		deps   lifecycle.CreateDeps
	}{
		{"empty git root", func(o *lifecycle.CreateOptions) { o.GitRoot = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}},
		{"empty repo slug", func(o *lifecycle.CreateOptions) { o.RepoSlug = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}},
		{"empty worktree root", func(o *lifecycle.CreateOptions) { o.WorktreeRoot = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}},
		{"empty state dir", func(o *lifecycle.CreateOptions) { o.StateDir = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}},
		{"empty from-branch", func(o *lifecycle.CreateOptions) { o.FromBranch = "" }, lifecycle.CreateDeps{Git: git.NewFakeRunner()}},
		{"nil git runner", func(*lifecycle.CreateOptions) {}, lifecycle.CreateDeps{}},
		{
			"nil mux for non-bare",
			func(o *lifecycle.CreateOptions) { o.Bare = false },
			lifecycle.CreateDeps{Git: git.NewFakeRunner()},
		},
		{
			"nil agent for non-bare",
			func(o *lifecycle.CreateOptions) { o.Bare = false },
			lifecycle.CreateDeps{Git: git.NewFakeRunner(), Mux: mux.NewFakeMultiplexer()},
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
	if _, statErr := os.Stat(filepath.Join(opts.StateDir, "demo")); !os.IsNotExist(statErr) {
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
		name string
		mux  *failMux
	}{
		{"create session fails", &failMux{FakeMultiplexer: mux.NewFakeMultiplexer(), failCreate: true}},
		{"set env fails", &failMux{FakeMultiplexer: mux.NewFakeMultiplexer(), failSetEnv: true}},
		{"send keys fails", &failMux{FakeMultiplexer: mux.NewFakeMultiplexer(), failSendKeys: true}},
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
