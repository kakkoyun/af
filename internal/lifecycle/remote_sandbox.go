package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kakkoyun/af/internal/remote"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/secret"
)

// ErrRemoteSetup reports a remote-clone failure during Create.
var ErrRemoteSetup = errors.New("remote workstream setup failed")

// ErrSandboxSetup reports a sandbox-launch failure during Create.
var ErrSandboxSetup = errors.New("sandbox launch failed")

// RemoteContext bundles remote-mode inputs for Create.
type RemoteContext struct { //nolint:govet // Field grouping prioritises readability.
	Host       string
	SSHOptions []string
	// RemoteRoot is the directory on the remote host where worktrees are
	// created (typically ~/Workspace/.worktrees). Empty means "the
	// remote's $HOME/Workspace/.worktrees".
	RemoteRoot string
	// Envelope is an optional ephemeral env-file written locally before the
	// SSH commands run and deleted on return (best-effort). The SCP
	// transport that delivers the envelope to the remote host is deferred to
	// the runtime layer per ADR-041.
	Envelope secret.Envelope
	// SSHExecutor overrides the SSH executor used for remote commands.
	// Nil means the default os/exec-backed executor. This field exists
	// solely as a test seam.
	SSHExecutor remote.Executor
}

// SandboxContext bundles sandbox-mode inputs for Create.
type SandboxContext struct {
	Provider sandbox.Sandbox
	// Envelope holds the temporary secret file written before launch and
	// deleted after the agent process has sourced it. Place the path inside
	// the worktree root so the sandbox provider can mount and source it.
	Envelope secret.Envelope
}

// PrepareRemoteWorkstream creates the remote worktree directory and
// clones the repo onto it. It returns the absolute remote worktree path.
//
// If RemoteContext.Envelope.Path is non-empty, the envelope file is
// written locally before the SSH commands execute and deleted on return
// (best-effort). The SCP transport responsible for copying the envelope
// to the remote host is deferred to the runtime layer per ADR-041.
// Token mapping (cwd <-> remote path) is also deferred to the runtime.
func PrepareRemoteWorkstream(ctx context.Context, rc RemoteContext, repoSlug, branch, fromBranch string) (string, error) {
	if rc.Host == "" {
		return "", fmt.Errorf("%w: empty host", ErrRemoteSetup)
	}
	if rc.Envelope.Path != "" {
		defer func() { _ = rc.Envelope.Delete() }() //nolint:errcheck // teardown best-effort
		err := rc.Envelope.Write()
		if err != nil {
			return "", fmt.Errorf("remote envelope write: %w", err)
		}
	}
	var ssh remote.SSH
	if rc.SSHExecutor != nil {
		ssh = remote.NewSSHWithExecutor(rc.Host, rc.SSHOptions, rc.SSHExecutor)
	} else {
		ssh = remote.NewSSH(rc.Host, rc.SSHOptions)
	}
	root := rc.RemoteRoot
	if root == "" {
		root = "$HOME/Workspace/.worktrees"
	}
	remotePath := root + "/" + repoSlug + "/" + branch
	commands := []string{
		"mkdir -p " + shellQuote(remotePath),
		"cd " + shellQuote(remotePath) + " && git init --quiet || true",
		"cd " + shellQuote(remotePath) + " && git checkout -b " + shellQuote(branch) + " " + shellQuote(fromBranch) + " || true",
	}
	for _, command := range commands {
		_, err := ssh.Run(ctx, command)
		if err != nil {
			return "", fmt.Errorf("%w: %s: %w", ErrRemoteSetup, command, err)
		}
	}
	return remotePath, nil
}

// LaunchSandboxWorkstream writes the Envelope (if Path is non-empty),
// calls sandbox.Sandbox.Launch with the supplied LaunchOpts, then
// deletes the Envelope on return (best-effort). Failures from Launch
// wrap ErrSandboxSetup.
//
// Place Envelope.Path inside the worktree root so the provider can
// mount and source it as part of its agent-start sequence.
func LaunchSandboxWorkstream(ctx context.Context, sc SandboxContext, opts sandbox.LaunchOpts) (*sandbox.Handle, error) {
	if sc.Provider == nil {
		return nil, fmt.Errorf("%w: nil provider", ErrSandboxSetup)
	}
	if sc.Envelope.Path != "" {
		defer func() { _ = sc.Envelope.Delete() }() //nolint:errcheck // teardown best-effort
		err := sc.Envelope.Write()
		if err != nil {
			return nil, fmt.Errorf("sandbox envelope write: %w", err)
		}
	}
	handle, err := sc.Provider.Launch(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSandboxSetup, err)
	}
	return handle, nil
}

// shellQuote returns arg safe for splicing into a shell command (POSIX).
func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if isShellSafe(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

const shellSafeSymbols = "-_./${}"

func isShellSafe(s string) bool {
	for _, r := range s {
		if !isShellSafeRune(r) {
			return false
		}
	}
	return true
}

func isShellSafeRune(r rune) bool {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
		return true
	}
	return strings.ContainsRune(shellSafeSymbols, r)
}
