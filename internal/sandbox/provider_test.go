package sandbox_test

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

func TestProvider_Name(t *testing.T) {
	provider := sandbox.NewSlicer()
	if got := provider.Name(); got != "slicer" {
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
	if got := provider.Name(); got != "slicer" {
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
	sentinel := errors.New("runner boom")
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
			return err
		}},
		{name: "teardown", run: func(provider sandbox.Provider) error {
			return provider.Teardown(ctx, handle)
		}},
		{name: "list", run: func(provider sandbox.Provider) error {
			handles, err := provider.List(ctx)
			if handles != nil {
				t.Errorf("List() = %#v on runner error, want nil", handles)
			}
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := sandbox.NewSlicerWithRunner(&fakeRunner{err: sentinel})
			err := tt.run(provider)
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, want wrapped %v", err, sentinel)
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
