package sandbox_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

// slicerName is the provider binary name asserted throughout these tests.
const slicerName = "slicer"

// errRunnerBoom is a sentinel runner failure used by provider tests.
var errRunnerBoom = errors.New("runner boom")

func TestProvider_Name(t *testing.T) {
	provider := sandbox.NewSlicer()
	if got := provider.Name(); got != slicerName {
		t.Fatalf("Name() = %q, want slicer", got)
	}
}

func TestProvider_IsAvailable_MatchesLookPath(t *testing.T) {
	provider := sandbox.NewSlicer()
	_, lookErr := exec.LookPath("slicer")
	want := lookErr == nil
	if got := provider.IsAvailable(context.Background()); got != want {
		t.Fatalf("IsAvailable() = %v, want %v (LookPath err = %v)", got, want, lookErr)
	}
}

func TestProvider_IsAvailable_CancelledContext(t *testing.T) {
	provider := sandbox.NewSlicer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if provider.IsAvailable(ctx) {
		t.Fatal("IsAvailable(cancelled) = true, want false")
	}
}

func TestNewSlicerProvider_NilRunnerDefaultsToExec(t *testing.T) {
	provider := sandbox.NewSlicerProvider(sandbox.SlicerOptions{}, nil)
	if got := provider.Name(); got != slicerName {
		t.Fatalf("Name() = %q, want slicer", got)
	}
}

func TestProvider_AttachBuildsCommand(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	provider := sandbox.NewSlicerWithRunner(runner)

	err := provider.Attach(ctx, &sandbox.Handle{ID: "vm-1"})
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	want := []sandbox.Command{{Name: "slicer", Args: []string{"vm", "shell", "vm-1"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestProvider_IsHealthyBuildsCommand(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	provider := sandbox.NewSlicerWithRunner(runner)

	healthy, err := provider.IsHealthy(ctx, &sandbox.Handle{ID: "vm-1"})
	if err != nil {
		t.Fatalf("IsHealthy() error = %v", err)
	}
	if !healthy {
		t.Fatal("IsHealthy() = false, want true")
	}
	want := []sandbox.Command{{Name: "slicer", Args: []string{"vm", "status", "vm-1"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestProvider_TeardownBuildsCommand(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	provider := sandbox.NewSlicerWithRunner(runner)

	err := provider.Teardown(ctx, &sandbox.Handle{ID: "vm-1"})
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	want := []sandbox.Command{{Name: "slicer", Args: []string{"vm", "delete", "vm-1"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

// TestProvider_AttachCommandExported pins that AttachCommand (issue #33
// Fix 3) is the single source of truth for the argv used to attach to a
// running sandbox, so cmd/af can build the exact same shell command it
// sends into the host tmux pane.
func TestProvider_AttachCommandExported(t *testing.T) {
	provider := sandbox.NewSlicer()
	want := []string{"slicer", "vm", "shell", "vm-1"}
	if got := provider.AttachCommand("vm-1"); !reflect.DeepEqual(got, want) {
		t.Fatalf("AttachCommand() = %#v, want %#v", got, want)
	}
}

// TestProvider_Launch_HandleAttachCmdMatchesAttachCommand pins that
// slicerWTLaunch builds Handle.AttachCmd via the exported AttachCommand
// method rather than a second, potentially-drifting inline copy of the
// same argv shape.
func TestProvider_Launch_HandleAttachCmdMatchesAttachCommand(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	runner.QueueOutput("Launched VM vm-123\n")
	provider := sandbox.NewSlicerWithRunner(runner)

	handle, err := provider.Launch(ctx, sandbox.LaunchOpts{Worktree: "/repo"})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	want := provider.AttachCommand(handle.VMName)
	if !reflect.DeepEqual(handle.AttachCmd, want) {
		t.Fatalf("Handle.AttachCmd = %#v, want %#v (from AttachCommand)", handle.AttachCmd, want)
	}
}

func TestProvider_ListSortsHandlesAndSetsAttachCmd(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	runner.QueueOutput("vm-b vm-a\n")
	provider := sandbox.NewSlicerWithRunner(runner)

	handles, err := provider.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []sandbox.Handle{
		{ID: "vm-a", AttachCmd: []string{"slicer", "vm", "shell", "vm-a"}},
		{ID: "vm-b", AttachCmd: []string{"slicer", "vm", "shell", "vm-b"}},
	}
	if !reflect.DeepEqual(handles, want) {
		t.Fatalf("List() = %#v, want %#v", handles, want)
	}
	wantCommands := []sandbox.Command{{Name: "slicer", Args: []string{"vm", "list"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", got, wantCommands)
	}
}

func TestProvider_OperationsWrapRunnerErrors(t *testing.T) {
	ctx := context.Background()
	handle := &sandbox.Handle{ID: "vm-1"}

	tests := []struct {
		run  func(provider sandbox.Provider) error
		name string
	}{
		{name: "attach", run: func(provider sandbox.Provider) error {
			return provider.Attach(ctx, handle)
		}},
		{name: "health", run: func(provider sandbox.Provider) error {
			healthy, err := provider.IsHealthy(ctx, handle)
			if healthy {
				t.Error("IsHealthy() = true on runner error, want false")
			}
			if err != nil {
				return fmt.Errorf("is healthy: %w", err)
			}
			return nil
		}},
		{name: "teardown", run: func(provider sandbox.Provider) error {
			return provider.Teardown(ctx, handle)
		}},
		{name: "list", run: func(provider sandbox.Provider) error {
			handles, err := provider.List(ctx)
			if handles != nil {
				t.Errorf("List() = %#v on runner error, want nil", handles)
			}
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := sandbox.NewSlicerWithRunner(&fakeRunner{err: errRunnerBoom})
			err := tt.run(provider)
			if !errors.Is(err, errRunnerBoom) {
				t.Fatalf("error = %v, want wrapped %v", err, errRunnerBoom)
			}
		})
	}
}

func TestProvider_LaunchEmptyWorktreeFails(t *testing.T) {
	ctx := context.Background()
	provider := sandbox.NewSlicerWithRunner(sandbox.NewRecordingRunner())

	_, err := provider.Launch(ctx, sandbox.LaunchOpts{Workstream: "session"})
	if !errors.Is(err, sandbox.ErrSlicerWTPushFailed) {
		t.Fatalf("Launch() error = %v, want ErrSlicerWTPushFailed", err)
	}
}

func TestProvider_LaunchOmitsSessionTagWithoutWorkstream(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	runner.QueueOutput("Launched VM vm-anon\n")
	provider := sandbox.NewSlicerWithRunner(runner)

	handle, err := provider.Launch(ctx, sandbox.LaunchOpts{Worktree: "/repo", Tags: []string{"extra"}})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	if handle.VMName != "vm-anon" {
		t.Fatalf("VMName = %q, want vm-anon", handle.VMName)
	}
	want := []sandbox.Command{{Name: "slicer", Args: []string{"wt", "push", "--launch", "--tag", "af", "--tag", "extra", "/repo"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestExecRunner_RunReturnsOutput(t *testing.T) {
	runner := sandbox.ExecRunner{}

	output, err := runner.Run(context.Background(), sandbox.Command{
		Name: "sh",
		Dir:  t.TempDir(),
		Args: []string{"-c", "printf ok"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(output) != "ok" {
		t.Fatalf("Run() output = %q, want ok", output)
	}
}

func TestExecRunner_RunWrapsFailure(t *testing.T) {
	runner := sandbox.ExecRunner{}

	output, err := runner.Run(context.Background(), sandbox.Command{
		Name: "sh",
		Args: []string{"-c", "printf oops >&2; exit 3"},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}
	if !strings.Contains(err.Error(), "run sh") {
		t.Fatalf("Run() error = %v, want run context in message", err)
	}
	if !strings.Contains(string(output), "oops") {
		t.Fatalf("Run() output = %q, want stderr captured", output)
	}
}

// TestExecRunner_RunErrorIncludesStderr covers issue #19: af swallowed the
// failing command's stderr, reporting only a bare exit status. The error
// returned by Run must now embed the stderr text so callers (and users) can
// see what the invoked tool actually said.
func TestExecRunner_RunErrorIncludesStderr(t *testing.T) {
	runner := sandbox.ExecRunner{}

	_, err := runner.Run(context.Background(), sandbox.Command{
		Name: "sh",
		Args: []string{"-c", "echo boom disk full >&2; exit 1"},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}
	if !strings.Contains(err.Error(), "boom disk full") {
		t.Fatalf("Run() error = %v, want stderr text embedded", err)
	}
}

// TestExecRunner_RunErrorTruncatesLongStderr covers the 512-byte cap on the
// stderr snippet embedded in the error, per issue #19. The 2000-byte stderr
// payload is generated entirely inside the shell script (not passed as an
// argv value) so the argv portion of the error message stays short and
// doesn't itself trip the "not truncated" assertion.
func TestExecRunner_RunErrorTruncatesLongStderr(t *testing.T) {
	runner := sandbox.ExecRunner{}

	_, err := runner.Run(context.Background(), sandbox.Command{
		Name: "sh",
		Args: []string{"-c", "head -c 2000 /dev/zero | tr '\\0' 'x' >&2; exit 1"},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}
	if !strings.Contains(err.Error(), "…") {
		t.Fatalf("Run() error = %v, want truncation marker …", err)
	}
	if strings.Contains(err.Error(), strings.Repeat("x", 600)) {
		t.Fatalf("Run() error contains more than 512 bytes of stderr, want truncated")
	}
}

// TestExecRunner_RunErrorKeepsPlainMessageWhenStderrEmpty preserves the
// original bare-exit-status message when the failing command wrote nothing
// to stderr.
func TestExecRunner_RunErrorKeepsPlainMessageWhenStderrEmpty(t *testing.T) {
	runner := sandbox.ExecRunner{}

	_, err := runner.Run(context.Background(), sandbox.Command{
		Name: "sh",
		Args: []string{"-c", "exit 1"},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}
	if err.Error() != "run sh -c exit 1: exit status 1" {
		t.Fatalf("Run() error = %q, want plain exit-status message", err.Error())
	}
}

func TestWrapCommandError_NilErrReturnsNil(t *testing.T) {
	err := sandbox.WrapCommandError("slicer", []string{"wt", "push"}, nil, []byte("ignored"))
	if err != nil {
		t.Fatalf("WrapCommandError() = %v, want nil", err)
	}
}

func TestWrapCommandError_TruncatesLongStderr(t *testing.T) {
	long := strings.Repeat("y", 2000)
	err := sandbox.WrapCommandError("slicer", []string{"wt", "push"}, errRunnerBoom, []byte(long))
	if err == nil {
		t.Fatal("WrapCommandError() = nil, want error")
	}
	if !strings.Contains(err.Error(), "…") {
		t.Fatalf("WrapCommandError() = %v, want truncation marker", err)
	}
	if strings.Contains(err.Error(), strings.Repeat("y", 600)) {
		t.Fatalf("WrapCommandError() = %v, want stderr snippet truncated to <= 512 bytes", err)
	}
	if !errors.Is(err, errRunnerBoom) {
		t.Fatalf("WrapCommandError() = %v, want wrapped %v", err, errRunnerBoom)
	}
}
