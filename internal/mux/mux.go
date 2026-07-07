package mux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const splitFieldLimit = 2

// ErrSessionNotFound reports a missing multiplexer session.
var ErrSessionNotFound = errors.New("mux session not found")

// ErrPaneNotFound reports a missing multiplexer pane.
var ErrPaneNotFound = errors.New("mux pane not found")

// Session describes a multiplexer session.
type Session struct {
	Name     string
	Attached bool
}

// Pane describes a multiplexer pane.
type Pane struct {
	ID  string
	CWD string
}

// Multiplexer controls long-running agent terminal sessions.
type Multiplexer interface { //nolint:interfacebloat // ADR-040 deliberately keeps the tmux seam explicit for test fakes.
	IsAvailable(ctx context.Context) bool
	InsideSession(ctx context.Context) (string, bool, error)
	CreateSession(ctx context.Context, name, cwd string) error
	KillSession(ctx context.Context, name string) error
	SessionExists(ctx context.Context, name string) (bool, error)
	Attach(ctx context.Context, name string) error
	SendKeys(ctx context.Context, session, pane, keys string) error
	SetEnv(ctx context.Context, session, key, value string) error
	GetEnv(ctx context.Context, session, key string) (string, error)
	SetOption(ctx context.Context, session, key, value string) error
	ListSessions(ctx context.Context) ([]Session, error)
	SplitVertical(ctx context.Context, session, cwd string) (string, error)
	KillPane(ctx context.Context, session, pane string) error
	ListPanes(ctx context.Context, session string) ([]Pane, error)
}

// Command is one external command invocation.
type Command struct {
	Name string
	Dir  string
	Args []string
}

// Runner executes external commands for Tmux.
type Runner interface {
	Run(ctx context.Context, command Command) ([]byte, error)
}

// ExecRunner runs commands through os/exec.
type ExecRunner struct{}

// Run executes command and returns combined stdout/stderr.
func (ExecRunner) Run(ctx context.Context, command Command) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...) //nolint:gosec // Command argv is constructed by typed provider methods, not shell input.
	cmd.Dir = command.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run %s %s: %w", command.Name, strings.Join(command.Args, " "), err)
	}

	return output, nil
}

// Tmux implements Multiplexer with the tmux CLI.
type Tmux struct {
	runner Runner
	binary string
}

// NewTmux returns a tmux multiplexer using os/exec.
func NewTmux() Tmux {
	return NewTmuxWithRunner(ExecRunner{})
}

// NewTmuxWithRunner returns a tmux multiplexer using runner.
func NewTmuxWithRunner(runner Runner) Tmux {
	if runner == nil {
		runner = ExecRunner{}
	}

	return Tmux{runner: runner, binary: "tmux"}
}

// IsAvailable reports whether tmux can be found on PATH.
func (tmux Tmux) IsAvailable(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	_, err := exec.LookPath(tmux.binary)

	return err == nil
}

// InsideSession reports whether the current process is already inside tmux.
func (Tmux) InsideSession(ctx context.Context) (string, bool, error) {
	err := ctx.Err()
	if err != nil {
		return "", false, fmt.Errorf("check tmux session: %w", err)
	}
	if os.Getenv("TMUX") == "" {
		return "", false, nil
	}

	return os.Getenv("TMUX_PANE"), true, nil
}

// CreateSession creates a detached tmux session and marks it as af-managed.
func (tmux Tmux) CreateSession(ctx context.Context, name, cwd string) error {
	_, err := tmux.run(ctx, "new-session", "-d", "-s", name, "-c", cwd)
	if err != nil {
		return fmt.Errorf("create tmux session %s: %w", name, err)
	}
	err = tmux.SetOption(ctx, name, "@AF_SESSION", "1")
	if err != nil {
		return fmt.Errorf("mark tmux session %s: %w", name, err)
	}

	return nil
}

// KillSession kills a tmux session.
func (tmux Tmux) KillSession(ctx context.Context, name string) error {
	_, err := tmux.run(ctx, "kill-session", "-t", name)
	if err != nil {
		return fmt.Errorf("kill tmux session %s: %w", name, err)
	}

	return nil
}

// SessionExists reports whether a tmux session exists.
func (tmux Tmux) SessionExists(ctx context.Context, name string) (bool, error) {
	_, err := tmux.run(ctx, "has-session", "-t", name)
	if err == nil {
		return true, nil
	}
	// has-session exits non-zero both for "can't find session" and for
	// "no server running" — either way the session does not exist.
	// Only non-exit failures (missing binary, cancelled context) are
	// real errors.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, fmt.Errorf("check tmux session %s: %w", name, err)
}

// Attach attaches the user's terminal to a tmux session.
func (tmux Tmux) Attach(ctx context.Context, name string) error {
	_, err := tmux.run(ctx, "attach-session", "-t", name)
	if err != nil {
		return fmt.Errorf("attach tmux session %s: %w", name, err)
	}

	return nil
}

// SendKeys sends keys to a tmux target.
func (tmux Tmux) SendKeys(ctx context.Context, session, pane, keys string) error {
	target := session
	if pane != "" {
		target = pane
	}
	_, err := tmux.run(ctx, "send-keys", "-t", target, keys, "C-m")
	if err != nil {
		return fmt.Errorf("send keys to tmux target %s: %w", target, err)
	}

	return nil
}

// SetEnv sets a tmux session environment variable.
func (tmux Tmux) SetEnv(ctx context.Context, session, key, value string) error {
	_, err := tmux.run(ctx, "set-environment", "-t", session, key, value)
	if err != nil {
		return fmt.Errorf("set tmux env %s: %w", key, err)
	}

	return nil
}

// GetEnv returns a tmux session environment variable.
func (tmux Tmux) GetEnv(ctx context.Context, session, key string) (string, error) {
	output, err := tmux.run(ctx, "show-environment", "-t", session, key)
	if err != nil {
		return "", fmt.Errorf("get tmux env %s: %w", key, err)
	}
	line := strings.TrimSpace(string(output))
	prefix := key + "="
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("get tmux env %s: %w", key, ErrSessionNotFound)
	}

	return strings.TrimPrefix(line, prefix), nil
}

// SetOption sets a tmux session option.
func (tmux Tmux) SetOption(ctx context.Context, session, key, value string) error {
	_, err := tmux.run(ctx, "set-option", "-t", session, key, value)
	if err != nil {
		return fmt.Errorf("set tmux option %s: %w", key, err)
	}

	return nil
}

// ListSessions returns af-managed tmux sessions.
func (tmux Tmux) ListSessions(ctx context.Context) ([]Session, error) {
	// The space-free attached count leads: tmux sanitizes control
	// characters (including tab) in format output to underscores, so a
	// tab separator never survives a real server.
	output, err := tmux.run(ctx, "list-sessions", "-F", "#{session_attached} #{session_name}")
	if err != nil {
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}

	return parseSessions(string(output)), nil
}

// SplitVertical splits a session vertically and returns the new pane id.
func (tmux Tmux) SplitVertical(ctx context.Context, session, cwd string) (string, error) {
	output, err := tmux.run(ctx, "split-window", "-v", "-P", "-F", "#{pane_id}", "-t", session, "-c", cwd)
	if err != nil {
		return "", fmt.Errorf("split tmux session %s: %w", session, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// KillPane kills one pane in a tmux session.
func (tmux Tmux) KillPane(ctx context.Context, _, pane string) error {
	_, err := tmux.run(ctx, "kill-pane", "-t", pane)
	if err != nil {
		return fmt.Errorf("kill tmux pane %s: %w", pane, err)
	}

	return nil
}

// ListPanes returns panes for a tmux session.
func (tmux Tmux) ListPanes(ctx context.Context, session string) ([]Pane, error) {
	// pane_id (%N) is space-free and leads; the path may contain spaces.
	output, err := tmux.run(ctx, "list-panes", "-t", session, "-F", "#{pane_id} #{pane_current_path}")
	if err != nil {
		return nil, fmt.Errorf("list tmux panes for %s: %w", session, err)
	}

	return parsePanes(string(output)), nil
}

func (tmux Tmux) run(ctx context.Context, args ...string) ([]byte, error) {
	output, err := tmux.runner.Run(ctx, Command{Name: tmux.binary, Args: args})
	if err != nil {
		return output, fmt.Errorf("tmux %s: %w", strings.Join(args, " "), err)
	}

	return output, nil
}

func parseSessions(output string) []Session {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	sessions := make([]Session, 0, len(lines))
	for _, line := range lines {
		fields := strings.SplitN(line, " ", splitFieldLimit)
		if len(fields) != splitFieldLimit {
			continue
		}
		sessions = append(sessions, Session{Name: fields[1], Attached: fields[0] != "0"})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Name < sessions[j].Name })

	return sessions
}

func parsePanes(output string) []Pane {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	panes := make([]Pane, 0, len(lines))
	for _, line := range lines {
		fields := strings.SplitN(line, " ", splitFieldLimit)
		pane := Pane{ID: fields[0]}
		if len(fields) == splitFieldLimit {
			pane.CWD = fields[1]
		}
		panes = append(panes, pane)
	}

	return panes
}
