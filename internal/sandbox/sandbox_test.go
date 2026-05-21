package sandbox_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

func TestKnownProviders_SlicerOnly(t *testing.T) {
	got := sandbox.KnownProviders()
	want := []string{"slicer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownProviders() = %#v, want %#v", got, want)
	}
}

func TestNewProvider_AcceptsSlicer(t *testing.T) {
	p, err := sandbox.NewProvider("slicer")
	if err != nil {
		t.Fatalf("NewProvider(\"slicer\") error = %v", err)
	}
	if p == nil {
		t.Fatal("NewProvider(\"slicer\") returned nil provider")
	}
}

func TestNewProvider_RejectsSBX(t *testing.T) {
	_, err := sandbox.NewProvider("sbx")
	if err == nil {
		t.Fatal("NewProvider(\"sbx\") error = nil, want ErrUnsupportedProvider")
	}
	if !errors.Is(err, sandbox.ErrUnsupportedProvider) {
		t.Fatalf("NewProvider(\"sbx\") error = %v, want ErrUnsupportedProvider", err)
	}
}

func TestNewProvider_RejectsUnknown(t *testing.T) {
	_, err := sandbox.NewProvider("docker")
	if err == nil {
		t.Fatal("NewProvider(\"docker\") error = nil, want ErrUnsupportedProvider")
	}
	if !errors.Is(err, sandbox.ErrUnsupportedProvider) {
		t.Fatalf("NewProvider(\"docker\") error = %v, want ErrUnsupportedProvider", err)
	}
}

func TestSlicer_LaunchBuildsCommandAndHandle(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	runner.QueueOutput("vm-session\n")
	provider := sandbox.NewSlicerWithRunner(runner)

	handle, err := provider.Launch(ctx, sandbox.LaunchOpts{Workstream: "session", Worktree: "/repo", AgentArgv: []string{"pi"}})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	if handle.ID != "vm-session" {
		t.Fatalf("handle ID = %q, want vm-session", handle.ID)
	}
	if !reflect.DeepEqual(handle.AttachCmd, []string{"slicer", "vm", "shell", "vm-session"}) {
		t.Fatalf("AttachCmd = %#v", handle.AttachCmd)
	}

	want := []sandbox.Command{{Name: "slicer", Args: []string{"vm", "run", "--name", "session", "--mount", "/repo", "--", "pi"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestNewSlicerWithOptions_PassesGroupToArgv(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	runner.QueueOutput("vm-id\n")
	provider := sandbox.NewSlicerProvider(sandbox.SlicerOptions{
		Group:     "af-myrepo-tight",
		Resources: sandbox.SlicerResources{VCPU: 2, RAMGB: 4},
	}, runner)

	_, err := provider.Launch(ctx, sandbox.LaunchOpts{
		Workstream: "session",
		Worktree:   "/repo",
		AgentArgv:  []string{"pi"},
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	commands := runner.Commands()
	if len(commands) != 1 {
		t.Fatalf("want 1 command, got %d", len(commands))
	}
	args := commands[0].Args
	// Expect: vm run --name session --group af-myrepo-tight --mount /repo -- pi
	want := []string{"vm", "run", "--name", "session", "--group", "af-myrepo-tight", "--mount", "/repo", "--", "pi"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("launch args = %#v, want %#v", args, want)
	}
}

func TestFakeSandbox_LaunchHealthTeardownAndList(t *testing.T) {
	ctx := context.Background()
	fake := sandbox.NewFake("slicer")

	handle, err := fake.Launch(ctx, sandbox.LaunchOpts{Workstream: "session", Worktree: "/repo", AgentArgv: []string{"pi"}})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	healthy, err := fake.IsHealthy(ctx, handle)
	if err != nil {
		t.Fatalf("IsHealthy() error = %v", err)
	}
	if !healthy {
		t.Fatal("IsHealthy() = false, want true")
	}

	handles, err := fake.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(handles) != 1 || handles[0].ID != handle.ID {
		t.Fatalf("List() = %#v, want launched handle", handles)
	}

	err = fake.Teardown(ctx, handle)
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	healthy, err = fake.IsHealthy(ctx, handle)
	if err != nil {
		t.Fatalf("IsHealthy(after teardown) error = %v", err)
	}
	if healthy {
		t.Fatal("IsHealthy(after teardown) = true, want false")
	}
}
