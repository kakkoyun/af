package mux

import (
	"context"
	"fmt"
	"sync"
)

// RecordingRunner records commands and returns queued outputs.
type RecordingRunner struct {
	commands []Command
	outputs  [][]byte
	mu       sync.Mutex
}

// NewRecordingRunner returns an empty recording command runner.
func NewRecordingRunner() *RecordingRunner {
	return &RecordingRunner{}
}

// QueueOutput queues output for the next Run call.
func (runner *RecordingRunner) QueueOutput(output string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.outputs = append(runner.outputs, []byte(output))
}

// Commands returns recorded commands.
func (runner *RecordingRunner) Commands() []Command {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	commands := make([]Command, 0, len(runner.commands))
	for _, command := range runner.commands {
		commands = append(commands, copyCommand(command))
	}

	return commands
}

// Run records command and returns the next queued output, if any.
func (runner *RecordingRunner) Run(ctx context.Context, command Command) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("record command %s: %w", command.Name, err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.commands = append(runner.commands, copyCommand(command))
	if len(runner.outputs) == 0 {
		return nil, nil
	}
	output := append([]byte(nil), runner.outputs[0]...)
	runner.outputs = runner.outputs[1:]

	return output, nil
}

func copyCommand(command Command) Command {
	copied := Command{Name: command.Name, Dir: command.Dir}
	copied.Args = append([]string(nil), command.Args...)

	return copied
}

// RecordingInteractiveRunner records InteractiveRunner calls. Tests use
// it to assert that Tmux.Attach ran a command through the caller's real
// terminal (issue #33 Fix 0) instead of the captured Runner, or vice
// versa, without ever touching a real tty.
type RecordingInteractiveRunner struct {
	commands []Command
	mu       sync.Mutex
}

// NewRecordingInteractiveRunner returns an empty recording interactive runner.
func NewRecordingInteractiveRunner() *RecordingInteractiveRunner {
	return &RecordingInteractiveRunner{}
}

// Commands returns recorded interactive commands.
func (runner *RecordingInteractiveRunner) Commands() []Command {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	commands := make([]Command, 0, len(runner.commands))
	for _, command := range runner.commands {
		commands = append(commands, copyCommand(command))
	}

	return commands
}

// RunInteractive records command and returns nil.
func (runner *RecordingInteractiveRunner) RunInteractive(ctx context.Context, command Command) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("record interactive command %s: %w", command.Name, err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.commands = append(runner.commands, copyCommand(command))

	return nil
}
