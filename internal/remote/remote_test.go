package remote_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/remote"
)

func TestSSH_RunBuildsArgvWithOptionsHostAndCommand(t *testing.T) {
	ctx := context.Background()
	executor := remote.NewFakeExecutor()
	ssh := remote.NewSSHWithExecutor("devbox", []string{"-A", "-o", "ControlMaster=auto"}, executor)

	_, err := ssh.Run(ctx, "tmux attach -t kakkoyun--issue-42")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := executor.Commands()
	want := []remote.Command{{Name: "ssh", Args: []string{"-A", "-o", "ControlMaster=auto", "devbox", "tmux attach -t kakkoyun--issue-42"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestRemoteClonePath_MapsRepoAndBranchUnderAFClones(t *testing.T) {
	got := remote.ClonePath("kakkoyun/af", "kakkoyun/issue-42")
	want := "~/af-clones/kakkoyun/af/kakkoyun/issue-42"
	if got != want {
		t.Fatalf("ClonePath() = %q, want %q", got, want)
	}
}

func TestProbeCommand_UsesWhichForAllTools(t *testing.T) {
	got := remote.ProbeCommand("tmux", "pi")
	want := "which tmux pi"
	if got != want {
		t.Fatalf("ProbeCommand() = %q, want %q", got, want)
	}
}

func TestNewSSH_BuildsSSHArgvWithOptionsHostAndCommand(t *testing.T) {
	ssh := remote.NewSSH("devbox", []string{"-A"})

	got := ssh.Command("uptime")
	want := remote.Command{Name: "ssh", Args: []string{"-A", "devbox", "uptime"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Command() = %#v, want %#v", got, want)
	}
}

func TestNewSSHWithExecutor_NilExecutorDefaultsAndCopiesOptions(t *testing.T) {
	options := []string{"-A"}
	ssh := remote.NewSSHWithExecutor("devbox", options, nil)
	options[0] = "-X"

	got := ssh.Command("uptime")
	want := remote.Command{Name: "ssh", Args: []string{"-A", "devbox", "uptime"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Command() = %#v, want %#v", got, want)
	}
}

func TestSSH_RunWrapsExecutorErrorWithHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executor := remote.NewFakeExecutor()
	ssh := remote.NewSSHWithExecutor("devbox", nil, executor)

	_, err := ssh.Run(ctx, "uptime")
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if !strings.Contains(err.Error(), "ssh devbox") {
		t.Fatalf("Run() error = %q, want host in message", err)
	}
}

func TestProbeCommand_NoToolsReturnsTrue(t *testing.T) {
	got := remote.ProbeCommand()
	want := "true"
	if got != want {
		t.Fatalf("ProbeCommand() = %q, want %q", got, want)
	}
}

func TestExecExecutor_RunReturnsCombinedOutputAndHonorsDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello\n"), 0o600)
	if err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	out, err := remote.ExecExecutor{}.Run(ctx, remote.Command{Name: "cat", Dir: dir, Args: []string{"note.txt"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(out) != "hello\n" {
		t.Fatalf("Run() output = %q, want %q", out, "hello\n")
	}
}

func TestExecExecutor_RunWrapsExitErrorAndKeepsOutput(t *testing.T) {
	ctx := context.Background()

	out, err := remote.ExecExecutor{}.Run(ctx, remote.Command{Name: "sh", Args: []string{"-c", "echo boom; exit 3"}})
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Fatalf("Run() exit code = %d, want 3", exitErr.ExitCode())
	}
	if !strings.Contains(string(out), "boom") {
		t.Fatalf("Run() output = %q, want it to contain boom", out)
	}
	if !strings.Contains(err.Error(), "run sh -c") {
		t.Fatalf("Run() error = %q, want command in message", err)
	}
}

func TestExecExecutor_RunReturnsErrNotFoundForMissingBinary(t *testing.T) {
	ctx := context.Background()

	_, err := remote.ExecExecutor{}.Run(ctx, remote.Command{Name: "af-definitely-missing-binary"})
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Run() error = %v, want exec.ErrNotFound", err)
	}
}

func TestFakeExecutor_RunReturnsErrorForCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executor := remote.NewFakeExecutor()

	_, err := executor.Run(ctx, remote.Command{Name: "ssh"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := executor.Commands(); len(got) != 0 {
		t.Fatalf("commands = %#v, want none recorded", got)
	}
}

func TestFakeExecutor_QueuesOutputAndRecordsCommands(t *testing.T) {
	ctx := context.Background()
	executor := remote.NewFakeExecutor()
	executor.QueueOutput("ok\n")

	out, err := executor.Run(ctx, remote.Command{Name: "ssh", Args: []string{"host", "echo ok"}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(out) != "ok\n" {
		t.Fatalf("Run() output = %q, want ok", out)
	}

	want := []remote.Command{{Name: "ssh", Args: []string{"host", "echo ok"}}}
	if got := executor.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}
