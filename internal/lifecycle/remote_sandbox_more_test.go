package lifecycle_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/remote"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/secret"
)

// secretEnvelopeAtDir returns an Envelope whose Path is a directory, so
// Envelope.Write must fail with "is a directory".
func secretEnvelopeAtDir(t *testing.T) secret.Envelope {
	t.Helper()
	return secret.Envelope{Path: t.TempDir(), Entries: map[string]string{"K": "v"}}
}

// errSSHBoom simulates a failing SSH executor.
var errSSHBoom = errors.New("ssh boom")

type errSSHExecutor struct{}

func (errSSHExecutor) Run(_ context.Context, _ remote.Command) ([]byte, error) {
	return nil, errSSHBoom
}

// remoteCommandsOf extracts the remote command strings (the last ssh arg)
// from a FakeExecutor's recorded invocations.
func remoteCommandsOf(executor *remote.FakeExecutor) []string {
	recorded := executor.Commands()
	out := make([]string, 0, len(recorded))
	for _, command := range recorded {
		if len(command.Args) == 0 {
			continue
		}
		out = append(out, command.Args[len(command.Args)-1])
	}
	return out
}

func TestPrepareRemote_DefaultRootAndShellSafeArgsUnquoted(t *testing.T) {
	t.Parallel()
	executor := remote.NewFakeExecutor()
	rc := lifecycle.RemoteContext{Host: "worker", SSHExecutor: executor}

	remotePath, err := lifecycle.PrepareRemoteWorkstream(
		context.Background(), rc, "github.com/owner/repo", "feat/x", "main")
	if err != nil {
		t.Fatalf("PrepareRemoteWorkstream: %v", err)
	}
	want := "$HOME/Workspace/.worktrees/github.com/owner/repo/feat/x"
	if remotePath != want {
		t.Fatalf("remotePath = %q, want %q", remotePath, want)
	}
	commands := remoteCommandsOf(executor)
	if len(commands) != 3 {
		t.Fatalf("len(commands) = %d, want 3: %q", len(commands), commands)
	}
	// Shell-safe paths must be spliced without quoting.
	if commands[0] != "mkdir -p "+want {
		t.Fatalf("commands[0] = %q", commands[0])
	}
	if !strings.Contains(commands[2], "git checkout -b feat/x main") {
		t.Fatalf("commands[2] = %q, want unquoted checkout", commands[2])
	}
}

func TestPrepareRemote_QuotesUnsafeArgs(t *testing.T) {
	t.Parallel()
	executor := remote.NewFakeExecutor()
	rc := lifecycle.RemoteContext{
		Host:        "worker",
		RemoteRoot:  "/srv/wt",
		SSHExecutor: executor,
	}

	remotePath, err := lifecycle.PrepareRemoteWorkstream(
		context.Background(), rc, "owner/repo", "feat/it's risky", "")
	if err != nil {
		t.Fatalf("PrepareRemoteWorkstream: %v", err)
	}
	if !strings.HasPrefix(remotePath, "/srv/wt/") {
		t.Fatalf("remotePath = %q, want custom root prefix", remotePath)
	}
	joined := strings.Join(remoteCommandsOf(executor), "\n")
	// The apostrophe in the branch must be escaped with the POSIX '\''
	// idiom, and the empty from-branch must be spliced as ''.
	if !strings.Contains(joined, `'\''`) {
		t.Fatalf("commands missing escaped single quote:\n%s", joined)
	}
	if !strings.Contains(joined, " ''") {
		t.Fatalf("commands missing quoted empty from-branch:\n%s", joined)
	}
}

func TestPrepareRemote_SSHFailureWrapsErrRemoteSetup(t *testing.T) {
	t.Parallel()
	rc := lifecycle.RemoteContext{Host: "worker", SSHExecutor: errSSHExecutor{}}

	_, err := lifecycle.PrepareRemoteWorkstream(
		context.Background(), rc, "owner/repo", "feat/x", "main")
	if !errors.Is(err, lifecycle.ErrRemoteSetup) {
		t.Fatalf("want ErrRemoteSetup, got %v", err)
	}
	if !errors.Is(err, errSSHBoom) {
		t.Fatalf("want wrapped executor error, got %v", err)
	}
}

func TestPrepareRemote_EnvelopeWriteErrorAborts(t *testing.T) {
	t.Parallel()
	executor := remote.NewFakeExecutor()
	rc := lifecycle.RemoteContext{
		Host:        "worker",
		SSHExecutor: executor,
		// A directory as the envelope path makes os.WriteFile fail.
		Envelope: secretEnvelopeAtDir(t),
	}

	_, err := lifecycle.PrepareRemoteWorkstream(
		context.Background(), rc, "owner/repo", "feat/x", "main")
	if err == nil {
		t.Fatal("expected error when envelope write fails")
	}
	if errors.Is(err, lifecycle.ErrRemoteSetup) {
		t.Fatalf("envelope write failure must not wrap ErrRemoteSetup; got %v", err)
	}
	if len(executor.Commands()) != 0 {
		t.Fatalf("SSH commands ran despite envelope failure: %v", executor.Commands())
	}
}

func TestLaunchSandbox_LaunchFailureWrapsErrSandboxSetup(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the fake sandbox fails Launch on a canceled context

	_, err := lifecycle.LaunchSandboxWorkstream(ctx,
		lifecycle.SandboxContext{Provider: sandbox.NewFake("noop")},
		sandbox.LaunchOpts{Workstream: "ws"})
	if !errors.Is(err, lifecycle.ErrSandboxSetup) {
		t.Fatalf("want ErrSandboxSetup, got %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want wrapped context.Canceled, got %v", err)
	}
}
