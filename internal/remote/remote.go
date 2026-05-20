package remote

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"sync"
)

const sshHostAndCommandArgCount = 2

// Command is one external command invocation.
type Command struct {
	Name string
	Dir  string
	Args []string
}

// Executor runs external commands for SSH remotes.
type Executor interface {
	Run(ctx context.Context, command Command) ([]byte, error)
}

// ExecExecutor runs commands through os/exec.
type ExecExecutor struct{}

// Run executes command and returns combined stdout/stderr.
func (ExecExecutor) Run(ctx context.Context, command Command) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...) //nolint:gosec // SSH argv is constructed without shell interpolation.
	cmd.Dir = command.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run %s %s: %w", command.Name, strings.Join(command.Args, " "), err)
	}

	return output, nil
}

// SSH executes commands on an opaque ssh host string.
type SSH struct {
	host     string
	binary   string
	executor Executor
	options  []string
}

// NewSSH returns an SSH remote using os/exec.
func NewSSH(host string, options []string) SSH {
	return NewSSHWithExecutor(host, options, ExecExecutor{})
}

// NewSSHWithExecutor returns an SSH remote using executor.
func NewSSHWithExecutor(host string, options []string, executor Executor) SSH {
	if executor == nil {
		executor = ExecExecutor{}
	}

	return SSH{host: host, binary: "ssh", options: append([]string(nil), options...), executor: executor}
}

// Command builds the local ssh argv for remoteCommand.
func (ssh SSH) Command(remoteCommand string) Command {
	args := make([]string, 0, len(ssh.options)+sshHostAndCommandArgCount)
	args = append(args, ssh.options...)
	args = append(args, ssh.host, remoteCommand)

	return Command{Name: ssh.binary, Args: args}
}

// Run executes remoteCommand over ssh.
func (ssh SSH) Run(ctx context.Context, remoteCommand string) ([]byte, error) {
	command := ssh.Command(remoteCommand)
	output, err := ssh.executor.Run(ctx, command)
	if err != nil {
		return output, fmt.Errorf("ssh %s: %w", ssh.host, err)
	}

	return output, nil
}

// ClonePath returns the remote plain-clone path for repo and branch.
func ClonePath(repo, branch string) string {
	return path.Join("~/af-clones", repo, branch)
}

// ProbeCommand returns the remote availability probe for tools.
func ProbeCommand(tools ...string) string {
	if len(tools) == 0 {
		return "true"
	}

	return "which " + strings.Join(tools, " ")
}

// FakeExecutor records commands and returns queued outputs.
type FakeExecutor struct {
	commands []Command
	outputs  [][]byte
	mu       sync.Mutex
}

// NewFakeExecutor returns an empty fake executor.
func NewFakeExecutor() *FakeExecutor {
	return &FakeExecutor{}
}

// QueueOutput queues output for the next Run call.
func (executor *FakeExecutor) QueueOutput(output string) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.outputs = append(executor.outputs, []byte(output))
}

// Commands returns recorded commands.
func (executor *FakeExecutor) Commands() []Command {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	commands := make([]Command, 0, len(executor.commands))
	for _, command := range executor.commands {
		commands = append(commands, copyCommand(command))
	}

	return commands
}

// Run records command and returns queued output, if any.
func (executor *FakeExecutor) Run(ctx context.Context, command Command) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("record remote command %s: %w", command.Name, err)
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.commands = append(executor.commands, copyCommand(command))
	if len(executor.outputs) == 0 {
		return nil, nil
	}
	output := append([]byte(nil), executor.outputs[0]...)
	executor.outputs = executor.outputs[1:]

	return output, nil
}

func copyCommand(command Command) Command {
	copied := Command{Name: command.Name, Dir: command.Dir}
	copied.Args = append([]string(nil), command.Args...)

	return copied
}
