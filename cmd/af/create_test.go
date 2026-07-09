package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/obsidian"
	sandboxpkg "github.com/kakkoyun/af/internal/sandbox"
)

// installFakeCreatePipeline wires a fake git runner (answering exactly
// the queries the create pipeline issues) and a fake multiplexer through
// newCreateContextOverride, and chdirs into a fresh repo-shaped temp
// directory so loadCreateConfig's os.Getwd() lands somewhere hermetic.
// It returns the fake multiplexer for attach/session assertions.
func installFakeCreatePipeline(t *testing.T) *mux.FakeMultiplexer {
	t.Helper()
	repoDir := t.TempDir()
	t.Chdir(repoDir)

	gitFake := git.NewFakeRunner()
	gitFake.SetResponse([]string{"rev-parse", "--show-toplevel"}, git.FakeResponse{Output: repoDir + "\n"})

	muxFake := mux.NewFakeMultiplexer()

	orig := newCreateContextOverride
	newCreateContextOverride = func(*rootOptions) *createContext {
		return &createContext{
			git:   gitFake,
			mux:   muxFake,
			getwd: func() (string, error) { return repoDir, nil },
		}
	}
	t.Cleanup(func() { newCreateContextOverride = orig })

	return muxFake
}

// forceInteractiveCreate overrides isInteractiveCreateFunc for the
// duration of the test, letting the interactive-attach branch of
// shouldAttachAfterCreate run without a real pty.
func forceInteractiveCreate(t *testing.T, interactive bool) {
	t.Helper()
	orig := isInteractiveCreateFunc
	isInteractiveCreateFunc = func(*cobra.Command) bool { return interactive }
	t.Cleanup(func() { isInteractiveCreateFunc = orig })
}

// TestCreate_NonInteractivePrintsFooterAndDoesNotAttach pins issue #21's
// default-safe behaviour: without a real TTY (every test invocation, and
// every CI/pipe invocation in production), create must print the
// next-steps footer instead of attaching.
func TestCreate_NonInteractivePrintsFooterAndDoesNotAttach(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	muxFake := installFakeCreatePipeline(t)

	stdout, _, err := executeCommand(t, newRootCmd(), "create", "demo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(stdout, "to attach:   af resume demo") {
		t.Fatalf("stdout missing next-steps footer; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "to check in: af status") {
		t.Fatalf("stdout missing next-steps footer; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "to finish:   af done demo") {
		t.Fatalf("stdout missing next-steps footer; got:\n%s", stdout)
	}
	if sessionAttached(t, muxFake, "af-demo") {
		t.Fatal("non-interactive create must not attach")
	}
}

// TestCreate_NoAttachFlagPrintsFooterAndDoesNotAttach pins the --no-attach
// flag itself: even if the invocation were interactive, --no-attach must
// force the footer path.
func TestCreate_NoAttachFlagPrintsFooterAndDoesNotAttach(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	muxFake := installFakeCreatePipeline(t)
	forceInteractiveCreate(t, true)

	stdout, _, err := executeCommand(t, newRootCmd(), "create", "demo", "--no-attach")
	if err != nil {
		t.Fatalf("create --no-attach: %v", err)
	}
	if !strings.Contains(stdout, "to attach:   af resume demo") {
		t.Fatalf("stdout missing next-steps footer; got:\n%s", stdout)
	}
	if sessionAttached(t, muxFake, "af-demo") {
		t.Fatal("create --no-attach must not attach even when interactive")
	}
}

// TestCreate_InteractiveAttachesToTmuxSession pins the issue #21 default:
// a real interactive create attaches via the shared mux.Attach mechanism
// instead of printing the footer.
func TestCreate_InteractiveAttachesToTmuxSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	muxFake := installFakeCreatePipeline(t)
	forceInteractiveCreate(t, true)

	stdout, _, err := executeCommand(t, newRootCmd(), "create", "demo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if strings.Contains(stdout, "to attach:") {
		t.Fatalf("interactive create should attach, not print the footer; got:\n%s", stdout)
	}
	if !sessionAttached(t, muxFake, "af-demo") {
		t.Fatal("interactive create should attach to the new tmux session")
	}
}

// TestCreate_BareImpliesNoAttach pins that --bare never attaches, even
// when forced interactive, since --bare skips the tmux+agent launch
// entirely (no session exists to attach to).
func TestCreate_BareImpliesNoAttach(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	muxFake := installFakeCreatePipeline(t)
	forceInteractiveCreate(t, true)

	stdout, _, err := executeCommand(t, newRootCmd(), "create", "demo", "--bare")
	if err != nil {
		t.Fatalf("create --bare: %v", err)
	}
	if !strings.Contains(stdout, "to attach:   af resume demo") {
		t.Fatalf("stdout missing next-steps footer; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "or: tmux attach") {
		t.Fatalf("--bare has no tmux session; footer must omit the tmux alternative; got:\n%s", stdout)
	}
	sessions, err := muxFake.ListSessions(t.Context())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("--bare create must not create (or attach to) any tmux session; got %+v", sessions)
	}
}

// TestCreate_TmuxLineIncludesAttachCommand pins issue #24 Option C: the
// tmux summary line names the usable `af resume` command, not just the
// raw tmux session name (which is what confused users into passing the
// tmux name to af commands in the first place).
func TestCreate_TmuxLineIncludesAttachCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installFakeCreatePipeline(t)

	stdout, _, err := executeCommand(t, newRootCmd(), "create", "demo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(stdout, "tmux:      af-demo   (attach: af resume demo)") {
		t.Fatalf("stdout missing the attach-annotated tmux line; got:\n%s", stdout)
	}
}

func TestCreate_SandboxFlagRejectsSBX(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// af create with --sandbox sbx must fail before touching git.
	_, _, err := executeCommand(t, newRootCmd(), "create", "--sandbox", "sbx", "--bare", "demo")
	if err == nil {
		t.Fatal("create --sandbox sbx: error = nil, want errSandboxFlagUnsupported")
	}
	if !errors.Is(err, errSandboxFlagUnsupported) {
		t.Fatalf("create --sandbox sbx: error = %v, want errSandboxFlagUnsupported", err)
	}
}

func TestCreate_SandboxFlagRejectsDocker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "create", "--sandbox", "docker", "--bare", "demo")
	if err == nil {
		t.Fatal("create --sandbox docker: error = nil, want error")
	}
	if !errors.Is(err, errSandboxFlagUnsupported) {
		t.Fatalf("create --sandbox docker: error = %v, want errSandboxFlagUnsupported", err)
	}
}

// TestCreate_SandboxProviderFactory_RejectsSBX exercises sandbox.NewProvider
// directly so the CLI plumbing has a unit-level anchor.
func TestCreate_SandboxProviderFactory_RejectsSBX(t *testing.T) {
	_, err := sandboxpkg.NewProvider("sbx")
	if err == nil {
		t.Fatal("NewProvider(sbx) error = nil, want ErrUnsupportedProvider")
	}
	if !errors.Is(err, sandboxpkg.ErrUnsupportedProvider) {
		t.Fatalf("NewProvider(sbx) error = %v, want ErrUnsupportedProvider", err)
	}
}

// TestDefaultCreateContext_WiresDiskNoteStore guards the ADR-047 wiring:
// production creates must carry a real note store, not nil, or
// note-on-create silently becomes a no-op.
func TestDefaultCreateContext_WiresDiskNoteStore(t *testing.T) {
	cc := defaultCreateContext(&rootOptions{})
	if cc.notes == nil {
		t.Fatal("defaultCreateContext().notes = nil, want obsidian.DirStore")
	}
	if _, ok := cc.notes.(obsidian.DirStore); !ok {
		t.Fatalf("defaultCreateContext().notes = %T, want obsidian.DirStore", cc.notes)
	}
}
