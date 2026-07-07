package smoke

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// ExecCommand is the production ExecFunc: it runs the command with the
// given working directory and environment, captures both streams, and
// reports non-zero exits via the exit code rather than an error.
func ExecCommand(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode(), nil
		}
		return stdout.Bytes(), stderr.Bytes(), -1, fmt.Errorf("run %s: %w", name, err)
	}
	return stdout.Bytes(), stderr.Bytes(), 0, nil
}
