package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/kakkoyun/af/internal/session"
)

var (
	errSessionResolutionNoInput = errors.New("no session specified and none could be inferred")
	errSessionPickerInterrupted = errors.New("session picker interrupted")
)

const hoursPerDay = 24

type sessionPickerOptions struct {
	Stdin    io.Reader
	Stderr   io.Writer
	StateDir string
}

type sessionPickerFn func(context.Context, sessionPickerOptions) (string, error)

//nolint:gochecknoglobals // Test seam for ADR-070 fzf picker behaviour.
var sessionPickerFunc sessionPickerFn = defaultSessionPicker

func resolveLifecycleStatePathForCommand(cmd *cobra.Command, positional string) (string, error) {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return "", fmt.Errorf("resolve state path: %w", err)
	}
	if statePath := explicitSessionStatePath(cmd, stateDir, positional); statePath != "" {
		return statePath, nil
	}
	if statePath := envSessionStatePath(stateDir); statePath != "" {
		return statePath, nil
	}
	statePath, err := cwdSessionStatePath(stateDir)
	if err != nil {
		return "", err
	}
	if statePath != "" {
		return statePath, nil
	}
	return pickerSessionStatePath(cmd, stateDir)
}

func explicitSessionStatePath(cmd *cobra.Command, stateDir, positional string) string {
	name := strings.TrimSpace(positional)
	flagName := rootSessionFlag(cmd)
	if flagName != "" {
		if name != "" {
			warnSessionOverride(cmd, name, flagName)
		}
		name = flagName
	}
	if name == "" {
		return ""
	}
	return statePathForSessionName(stateDir, name)
}

func envSessionStatePath(stateDir string) string {
	envName := strings.TrimSpace(os.Getenv("AF_SESSION"))
	if envName == "" {
		return ""
	}
	return statePathForSessionName(stateDir, envName)
}

func cwdSessionStatePath(stateDir string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve state path: getwd: %w", err)
	}
	discovered, err := session.DiscoverStatePath(session.DiscoverOptions{Cwd: cwd, SessionsDir: stateDir})
	if err == nil {
		return discovered, nil
	}
	if !errors.Is(err, session.ErrNoCurrentWorkstream) {
		return "", fmt.Errorf("resolve state path: discover cwd: %w", err)
	}
	return "", nil
}

func pickerSessionStatePath(cmd *cobra.Command, stateDir string) (string, error) {
	if !canUseSessionPicker(cmd) {
		return "", noInputSessionError()
	}
	selected, err := sessionPickerFunc(cmd.Context(), sessionPickerOptions{
		Stdin:    cmd.InOrStdin(),
		Stderr:   cmd.ErrOrStderr(),
		StateDir: stateDir,
	})
	if err != nil {
		return "", fmt.Errorf("resolve state path: %w", err)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", noInputSessionError()
	}
	return statePathForSessionName(stateDir, selected), nil
}

func noInputSessionError() error {
	return fmt.Errorf("resolve state path: %w. pass [session], set --session NAME, set AF_SESSION, or run inside a workstream worktree (cwd contains a .af/state.toml symlink)", errSessionResolutionNoInput)
}

func statePathForSessionName(stateDir, name string) string {
	return filepath.Join(stateDir, name, "state.toml")
}

func rootSessionFlag(cmd *cobra.Command) string {
	if cmd == nil || cmd.Root() == nil {
		return ""
	}
	flag := cmd.Root().PersistentFlags().Lookup("session")
	if flag == nil {
		return ""
	}
	return strings.TrimSpace(flag.Value.String())
}

func warnSessionOverride(cmd *cobra.Command, positional, flagName string) {
	if cmd == nil {
		return
	}
	writef(cmd.ErrOrStderr(), "warning: --session %q overrides positional session %q\n", flagName, positional)
}

func canUseSessionPicker(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if !isTerminalReader(cmd.InOrStdin()) || !isTerminalWriter(cmd.ErrOrStderr()) {
		return false
	}
	_, err := exec.LookPath("fzf")
	return err == nil
}

func isTerminalReader(r io.Reader) bool {
	file, ok := r.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

type sessionPickerRow struct {
	touched time.Time
	state   session.State
}

func defaultSessionPicker(ctx context.Context, opts sessionPickerOptions) (string, error) {
	rows, err := sessionPickerRows(opts.StateDir)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	input := renderSessionPickerRows(rows)
	fzf := exec.CommandContext(ctx, "fzf", "--prompt", "af> session ", "--delimiter", "\t", "--with-nth", "1,2,3,4,5")
	fzf.Stdin = strings.NewReader(input)
	fzf.Stderr = opts.Stderr
	out, err := fzf.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("%w: %w", errSessionPickerInterrupted, ctx.Err())
		}
		return "", fmt.Errorf("%w: %w", errSessionPickerInterrupted, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

func sessionPickerRows(stateDir string) ([]sessionPickerRow, error) {
	summaries, err := readAllStates(stateDir)
	if err != nil {
		return nil, err
	}
	rows := make([]sessionPickerRow, 0, len(summaries))
	for i := range summaries {
		ledgerPath := filepath.Join(filepath.Dir(summaries[i].statePath), "ledger.jsonl")
		touched, touchErr := session.LastTouchedAt(ledgerPath)
		if touchErr != nil {
			touched = summaries[i].state.Session.CreatedAt
		}
		rows = append(rows, sessionPickerRow{state: summaries[i].state, touched: touched})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].touched.Equal(rows[j].touched) {
			return rows[i].state.Session.Name < rows[j].state.Session.Name
		}
		return rows[i].touched.After(rows[j].touched)
	})
	return rows, nil
}

func renderSessionPickerRows(rows []sessionPickerRow) string {
	now := time.Now().UTC()
	var b strings.Builder
	for i := range rows {
		st := rows[i].state
		age := humanAge(now.Sub(rows[i].touched))
		_, _ = fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", st.Session.Name, st.Session.Status, st.Worktree.RepoSlug, st.Worktree.Branch, age)
	}
	return b.String()
}

func humanAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < hoursPerDay*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/hoursPerDay))
	}
}
