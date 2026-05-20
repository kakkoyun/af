// Package proxy executes user-configured diff/pr/editor commands per
// ADR-048. The package handles token interpolation, argv-vs-shell
// dispatch, and child-process invocation through an injectable Runner
// so tests do not touch a real shell.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Tokens collects the substitutions accepted by [diff].cmd and [pr].cmd.
type Tokens map[string]string

// Command describes one expanded proxy invocation.
type Command struct {
	Name string
	Dir  string
	Args []string
	// Shell, when true, indicates the command should run via `sh -c`
	// with Args[0] as a single string. Argv mode is the default.
	Shell bool
}

// Runner executes a Command and returns its combined output.
type Runner interface {
	Run(ctx context.Context, command Command) ([]byte, error)
}

// ErrEmptyArgv reports an argv-mode command with no arguments.
var ErrEmptyArgv = errors.New("proxy command has no arguments")

// Expand applies token substitution to argv elements. Each element is
// independently processed via strings.ReplaceAll so multi-word tokens
// survive the split.
func Expand(argv []string, tokens Tokens) []string {
	if len(argv) == 0 {
		return nil
	}
	out := make([]string, 0, len(argv))
	for _, raw := range argv {
		expanded := raw
		for k, v := range tokens {
			expanded = strings.ReplaceAll(expanded, "{"+k+"}", v)
		}
		out = append(out, expanded)
	}
	return out
}

// ExpandString applies token substitution to a single shell-mode string,
// shell-quoting each value before substitution to defang shell-meta
// characters carried into the substitution.
func ExpandString(template string, tokens Tokens) string {
	out := template
	for k, v := range tokens {
		out = strings.ReplaceAll(out, "{"+k+"}", shellQuote(v))
	}
	return out
}

// BuildArgvCommand returns a Command that runs argv[0] with argv[1:] in dir.
func BuildArgvCommand(argv []string, dir string) (Command, error) {
	if len(argv) == 0 {
		return Command{}, ErrEmptyArgv
	}
	return Command{
		Name:  argv[0],
		Args:  append([]string{}, argv[1:]...),
		Dir:   dir,
		Shell: false,
	}, nil
}

// BuildShellCommand returns a Command that runs script via sh -c in dir.
func BuildShellCommand(script, dir string) Command {
	return Command{
		Name:  "sh",
		Args:  []string{"-c", script},
		Dir:   dir,
		Shell: true,
	}
}

// ExecRunner executes Commands through os/exec.
type ExecRunner struct{}

// Run invokes the command and returns the combined stdout+stderr.
func (ExecRunner) Run(ctx context.Context, command Command) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...) //nolint:gosec // Argv is constructed by typed helpers in this package.
	cmd.Dir = command.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("proxy run %s: %w", command.Name, err)
	}
	return out, nil
}

// shellQuote wraps value in POSIX-safe single quotes, escaping any '.
func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
