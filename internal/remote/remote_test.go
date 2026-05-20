package remote_test

import (
	"context"
	"reflect"
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
