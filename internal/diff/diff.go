// Package diff implements opinionated diff rendering per ADR-064.
//
// The dispatch order is:
//
//   - --web: run diffity with an explicit base..head range (fails fast if diffity
//     is not on PATH).
//   - non-interactive stdout: git diff --stat (compact summary, safe for piping).
//   - interactive terminal with hunk on PATH: pipe git diff --no-color to hunk.
//   - interactive terminal without hunk: git diff (plain output).
//
// All external binary invocations are isolated behind the Executor interface so
// tests never require real binaries.
package diff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// Mode selects the diff rendering strategy.
type Mode int

const (
	// ModeAuto selects hunk-or-git for interactive stdout, git-stat for non-interactive.
	ModeAuto Mode = iota
	// ModeWeb uses diffity to open a browser diff.
	ModeWeb
)

var (
	// ErrDiffityMissing is returned when --web is requested but diffity is not on PATH.
	ErrDiffityMissing = errors.New("diff: diffity not found on PATH; install with: npm i -g diffity")
	// ErrEmptyOptions is returned when required fields are missing.
	ErrEmptyOptions = errors.New("diff: Worktree, Base, and Head are required")
)

// Options configures a diff rendering operation.
// Field order follows govet fieldalignment (pointers/interfaces first, then scalars).
type Options struct { //nolint:govet // Field grouping by semantic section improves readability.
	// Worktree is the working directory passed to every external command.
	Worktree string
	// Base is the base ref (e.g. "main" or "HEAD~1").
	Base string
	// Head is the head ref (e.g. a branch name or "HEAD").
	Head string
	// Stdout receives rendered output.
	Stdout io.Writer
	// Stderr receives error output from child processes.
	Stderr io.Writer
	// Mode selects the rendering strategy.
	Mode Mode
	// Interactive reports whether Stdout is a live terminal. When false the
	// non-interactive git-stat path is chosen regardless of Mode.
	Interactive bool
}

// Executor runs external commands. Real code uses ExecExecutor; tests inject fakes.
type Executor interface {
	// Execute runs argv[0] argv[1:] in dir, streaming stdout/stderr.
	Execute(ctx context.Context, dir string, stdout, stderr io.Writer, argv ...string) error
	// ExecutePipe pipes cmd1's stdout into cmd2's stdin. Both run in dir.
	ExecutePipe(ctx context.Context, dir string, stdout, stderr io.Writer, cmd1, cmd2 []string) error
}

// Deps wires external dependencies for Render.
type Deps struct {
	// LookPath resolves binary names to absolute paths.
	// Defaults to exec.LookPath when nil.
	LookPath func(string) (string, error)
	// Exec runs external commands.
	Exec Executor
}

// Render dispatches the diff according to opts.
func Render(ctx context.Context, deps Deps, opts Options) error {
	if opts.Worktree == "" || opts.Base == "" || opts.Head == "" {
		return fmt.Errorf("%w", ErrEmptyOptions)
	}
	lookPath := deps.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	switch opts.Mode {
	case ModeWeb:
		return renderWeb(ctx, deps.Exec, opts, lookPath)
	case ModeAuto:
		fallthrough //nolint:gocritic // explicit exhaustive case; fallthrough to shared interactive logic.
	default:
		if !opts.Interactive {
			return renderStat(ctx, deps.Exec, opts)
		}
		return renderTerminal(ctx, deps.Exec, opts, lookPath)
	}
}

// renderWeb opens the diff in a browser via diffity. Fails fast when diffity is absent.
func renderWeb(ctx context.Context, ex Executor, opts Options, lookPath func(string) (string, error)) error {
	_, err := lookPath("diffity")
	if err != nil {
		return fmt.Errorf("%w", ErrDiffityMissing)
	}
	rangeArg := opts.Base + ".." + opts.Head
	execErr := ex.Execute(ctx, opts.Worktree, opts.Stdout, opts.Stderr, "diffity", rangeArg)
	if execErr != nil {
		return fmt.Errorf("diff web: %w", execErr)
	}
	return nil
}

// renderStat prints a compact git diff --stat summary (non-interactive path).
func renderStat(ctx context.Context, ex Executor, opts Options) error {
	rangeArg := opts.Base + "..." + opts.Head
	execErr := ex.Execute(ctx, opts.Worktree, opts.Stdout, opts.Stderr, "git", "diff", "--stat", rangeArg)
	if execErr != nil {
		return fmt.Errorf("diff stat: %w", execErr)
	}
	return nil
}

// renderTerminal dispatches to hunk (if installed) or plain git diff.
func renderTerminal(ctx context.Context, ex Executor, opts Options, lookPath func(string) (string, error)) error {
	rangeArg := opts.Base + "..." + opts.Head
	_, err := lookPath("hunk")
	if err != nil {
		// hunk not found: fall back to plain git diff.
		execErr := ex.Execute(ctx, opts.Worktree, opts.Stdout, opts.Stderr, "git", "diff", rangeArg)
		if execErr != nil {
			return fmt.Errorf("diff terminal fallback: %w", execErr)
		}
		return nil
	}
	// Pipe git diff --no-color into hunk patch -.
	cmd1 := []string{"git", "diff", "--no-color", rangeArg}
	cmd2 := []string{"hunk", "patch", "-"}
	pipeErr := ex.ExecutePipe(ctx, opts.Worktree, opts.Stdout, opts.Stderr, cmd1, cmd2)
	if pipeErr != nil {
		return fmt.Errorf("diff hunk pipe: %w", pipeErr)
	}
	return nil
}

// ExecExecutor is the production Executor that shells out via os/exec.
type ExecExecutor struct{}

// Execute runs argv[0] with argv[1:] in dir.
func (ExecExecutor) Execute(ctx context.Context, dir string, stdout, stderr io.Writer, argv ...string) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv constructed by typed callers, not from shell input.
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("diff execute %s: %w", argv[0], err)
	}
	return nil
}

// ExecutePipe runs cmd1 | cmd2 in dir using an os/exec pipe.
func (ExecExecutor) ExecutePipe(ctx context.Context, dir string, stdout, stderr io.Writer, cmd1, cmd2 []string) error {
	c1 := exec.CommandContext(ctx, cmd1[0], cmd1[1:]...) //nolint:gosec // argv from typed callers.
	c1.Dir = dir

	c2 := exec.CommandContext(ctx, cmd2[0], cmd2[1:]...) //nolint:gosec // argv from typed callers.
	c2.Dir = dir
	c2.Stdout = stdout
	c2.Stderr = stderr

	pipe, err := c1.StdoutPipe()
	if err != nil {
		return fmt.Errorf("diff pipe setup: %w", err)
	}
	c2.Stdin = pipe

	err = c1.Start()
	if err != nil {
		return fmt.Errorf("diff cmd1 start: %w", err)
	}
	err = c2.Start()
	if err != nil {
		_ = c1.Process.Kill() //nolint:errcheck // best-effort kill during error recovery.
		return fmt.Errorf("diff cmd2 start: %w", err)
	}
	// c2 inherited its own copy of the pipe's read end; close ours or
	// cmd1 never receives EPIPE when cmd2 (a pager) exits early and
	// both Waits deadlock.
	_ = pipe.Close() //nolint:errcheck // double-close via c1.Wait is harmless.
	// Wait for c1 first; broken-pipe from c2 exiting early is non-fatal.
	_ = c1.Wait() //nolint:errcheck // broken-pipe is expected when c2 exits early.
	err = c2.Wait()
	if err != nil {
		return fmt.Errorf("diff cmd2 wait: %w", err)
	}
	return nil
}
