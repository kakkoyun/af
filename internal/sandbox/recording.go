package sandbox

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

// NewRecordingRunner returns an empty recording runner.
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

// Run records command and returns queued output, if any.
func (runner *RecordingRunner) Run(ctx context.Context, command Command) ([]byte, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("record sandbox command %s: %w", command.Name, err)
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
