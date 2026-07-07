package mux_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/mux"
)

var errRunFailed = errors.New("run failed")

// failingRunner always fails, exercising Tmux error wrapping.
type failingRunner struct{}

func (failingRunner) Run(context.Context, mux.Command) ([]byte, error) {
	return nil, errRunFailed
}

// flakyRunner succeeds for the first succeedFor calls, then fails.
type flakyRunner struct {
	succeedFor int
	calls      int
}

func (runner *flakyRunner) Run(context.Context, mux.Command) ([]byte, error) {
	runner.calls++
	if runner.calls > runner.succeedFor {
		return nil, errRunFailed
	}

	return nil, nil
}

// tmuxArgvCase pairs a Tmux call with the tmux argv it must produce.
type tmuxArgvCase struct {
	call func(ctx context.Context, tmux mux.Tmux) error
	name string
	want []string
}

// tmuxArgvCases enumerates the Tmux methods checked by TestTmux_CommandArgv.
func tmuxArgvCases() []tmuxArgvCase {
	return []tmuxArgvCase{
		{
			name: "kill session",
			call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.KillSession(ctx, "work") },
			want: []string{"kill-session", "-t", "work"},
		},
		{
			name: "has session",
			call: func(ctx context.Context, tmux mux.Tmux) error {
				_, err := tmux.SessionExists(ctx, "work")
				return wrapErr("session exists", err)
			},
			want: []string{"has-session", "-t", "work"},
		},
		{
			name: "attach session",
			call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.Attach(ctx, "work") },
			want: []string{"attach-session", "-t", "work"},
		},
		{
			name: "send keys to session",
			call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.SendKeys(ctx, "work", "", "claude") },
			want: []string{"send-keys", "-t", "work", "claude", "C-m"},
		},
		{
			name: "send keys to pane target",
			call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.SendKeys(ctx, "work", "%3", "claude") },
			want: []string{"send-keys", "-t", "%3", "claude", "C-m"},
		},
	}
}

// tmuxArgvCasesMore continues tmuxArgvCases; split keeps both under funlen.
func tmuxArgvCasesMore() []tmuxArgvCase {
	return []tmuxArgvCase{
		{
			name: "set environment",
			call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.SetEnv(ctx, "work", "AF_WS", "issue-42") },
			want: []string{"set-environment", "-t", "work", "AF_WS", "issue-42"},
		},
		{
			name: "set option",
			call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.SetOption(ctx, "work", "@AF_SESSION", "1") },
			want: []string{"set-option", "-t", "work", "@AF_SESSION", "1"},
		},
		{
			name: "kill pane ignores session",
			call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.KillPane(ctx, "work", "%3") },
			want: []string{"kill-pane", "-t", "%3"},
		},
		{
			name: "list panes",
			call: func(ctx context.Context, tmux mux.Tmux) error {
				_, err := tmux.ListPanes(ctx, "work")
				return wrapErr("list panes", err)
			},
			want: []string{"list-panes", "-t", "work", "-F", "#{pane_id}\t#{pane_current_path}"},
		},
		{
			name: "list sessions",
			call: func(ctx context.Context, tmux mux.Tmux) error {
				_, err := tmux.ListSessions(ctx)
				return wrapErr("list sessions", err)
			},
			want: []string{"list-sessions", "-F", "#{session_name}\t#{session_attached}"},
		},
	}
}

func TestTmux_CommandArgv(t *testing.T) {
	for _, test := range append(tmuxArgvCases(), tmuxArgvCasesMore()...) {
		t.Run(test.name, func(t *testing.T) {
			runner := mux.NewRecordingRunner()
			tmux := mux.NewTmuxWithRunner(runner)
			err := test.call(context.Background(), tmux)
			requireNoError(t, err)
			commands := runner.Commands()
			if len(commands) != 1 {
				t.Fatalf("recorded %d commands, want 1", len(commands))
			}
			if commands[0].Name != "tmux" {
				t.Fatalf("command name = %q, want tmux", commands[0].Name)
			}
			if !reflect.DeepEqual(commands[0].Args, test.want) {
				t.Fatalf("args = %#v, want %#v", commands[0].Args, test.want)
			}
		})
	}
}

func TestTmux_RunnerFailuresWrapSentinel(t *testing.T) {
	tests := []struct {
		call func(ctx context.Context, tmux mux.Tmux) error
		name string
	}{
		{name: "create session", call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.CreateSession(ctx, "work", "/repo") }},
		{name: "kill session", call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.KillSession(ctx, "work") }},
		{name: "session exists", call: func(ctx context.Context, tmux mux.Tmux) error {
			_, err := tmux.SessionExists(ctx, "work")
			return wrapErr("session exists", err)
		}},
		{name: "attach", call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.Attach(ctx, "work") }},
		{name: "send keys", call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.SendKeys(ctx, "work", "", "claude") }},
		{name: "set env", call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.SetEnv(ctx, "work", "AF_WS", "v") }},
		{name: "get env", call: func(ctx context.Context, tmux mux.Tmux) error {
			_, err := tmux.GetEnv(ctx, "work", "AF_WS")
			return wrapErr("get env", err)
		}},
		{name: "set option", call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.SetOption(ctx, "work", "@k", "v") }},
		{name: "list sessions", call: func(ctx context.Context, tmux mux.Tmux) error {
			_, err := tmux.ListSessions(ctx)
			return wrapErr("list sessions", err)
		}},
		{name: "split vertical", call: func(ctx context.Context, tmux mux.Tmux) error {
			_, err := tmux.SplitVertical(ctx, "work", "/repo")
			return wrapErr("split vertical", err)
		}},
		{name: "kill pane", call: func(ctx context.Context, tmux mux.Tmux) error { return tmux.KillPane(ctx, "work", "%3") }},
		{name: "list panes", call: func(ctx context.Context, tmux mux.Tmux) error {
			_, err := tmux.ListPanes(ctx, "work")
			return wrapErr("list panes", err)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmux := mux.NewTmuxWithRunner(failingRunner{})
			err := test.call(context.Background(), tmux)
			requireErrorIs(t, err, errRunFailed)
		})
	}
}

func TestTmux_CreateSessionMarkFailure(t *testing.T) {
	tmux := mux.NewTmuxWithRunner(&flakyRunner{succeedFor: 1})

	err := tmux.CreateSession(context.Background(), "work", "/repo")
	requireErrorIs(t, err, errRunFailed)
	if !strings.Contains(err.Error(), "mark tmux session work") {
		t.Fatalf("error = %q, want mark tmux session wrapping", err)
	}
}

func TestTmux_SessionExistsReportsFalseOnError(t *testing.T) {
	tmux := mux.NewTmuxWithRunner(failingRunner{})

	exists, err := tmux.SessionExists(context.Background(), "gone")
	requireErrorIs(t, err, errRunFailed)
	if exists {
		t.Fatal("SessionExists() = true, want false on error")
	}
}

func TestTmux_GetEnvParsesValue(t *testing.T) {
	runner := mux.NewRecordingRunner()
	runner.QueueOutput("AF_WORKSTREAM=issue-42\n")
	tmux := mux.NewTmuxWithRunner(runner)

	value, err := tmux.GetEnv(context.Background(), "work", "AF_WORKSTREAM")
	requireNoError(t, err)
	if value != "issue-42" {
		t.Fatalf("GetEnv() = %q, want issue-42", value)
	}

	want := []mux.Command{{Name: "tmux", Args: []string{"show-environment", "-t", "work", "AF_WORKSTREAM"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestTmux_GetEnvUnsetVariable(t *testing.T) {
	runner := mux.NewRecordingRunner()
	runner.QueueOutput("-AF_WORKSTREAM\n")
	tmux := mux.NewTmuxWithRunner(runner)

	_, err := tmux.GetEnv(context.Background(), "work", "AF_WORKSTREAM")
	requireErrorIs(t, err, mux.ErrSessionNotFound)
}

func TestTmux_ListSessionsParsesAndSorts(t *testing.T) {
	runner := mux.NewRecordingRunner()
	runner.QueueOutput("beta\t0\nalpha\t2\nsolo\n")
	tmux := mux.NewTmuxWithRunner(runner)

	sessions, err := tmux.ListSessions(context.Background())
	requireNoError(t, err)
	want := []mux.Session{
		{Name: "alpha", Attached: true},
		{Name: "beta", Attached: false},
		{Name: "solo", Attached: false},
	}
	if !reflect.DeepEqual(sessions, want) {
		t.Fatalf("ListSessions() = %#v, want %#v", sessions, want)
	}
}

func TestTmux_ListSessionsEmptyOutput(t *testing.T) {
	runner := mux.NewRecordingRunner()
	runner.QueueOutput("")
	tmux := mux.NewTmuxWithRunner(runner)

	sessions, err := tmux.ListSessions(context.Background())
	requireNoError(t, err)
	if sessions != nil {
		t.Fatalf("ListSessions() = %#v, want nil", sessions)
	}
}

func TestTmux_ListPanesParsesOptionalCWD(t *testing.T) {
	runner := mux.NewRecordingRunner()
	runner.QueueOutput("%0\t/repo\n%1\n")
	tmux := mux.NewTmuxWithRunner(runner)

	panes, err := tmux.ListPanes(context.Background(), "work")
	requireNoError(t, err)
	want := []mux.Pane{{ID: "%0", CWD: "/repo"}, {ID: "%1"}}
	if !reflect.DeepEqual(panes, want) {
		t.Fatalf("ListPanes() = %#v, want %#v", panes, want)
	}
}

func TestTmux_ListPanesEmptyOutput(t *testing.T) {
	runner := mux.NewRecordingRunner()
	runner.QueueOutput("")
	tmux := mux.NewTmuxWithRunner(runner)

	panes, err := tmux.ListPanes(context.Background(), "work")
	requireNoError(t, err)
	if panes != nil {
		t.Fatalf("ListPanes() = %#v, want nil", panes)
	}
}

func TestTmux_IsAvailableFindsBinaryOnPath(t *testing.T) {
	t.Setenv("PATH", writeFakeTmux(t))

	if !mux.NewTmux().IsAvailable(context.Background()) {
		t.Fatal("IsAvailable() = false, want true with tmux on PATH")
	}
}

func TestTmux_IsAvailableMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if mux.NewTmux().IsAvailable(context.Background()) {
		t.Fatal("IsAvailable() = true, want false with empty PATH dir")
	}
}

func TestTmux_IsAvailableCanceledContext(t *testing.T) {
	t.Setenv("PATH", writeFakeTmux(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if mux.NewTmux().IsAvailable(ctx) {
		t.Fatal("IsAvailable() = true, want false with canceled context")
	}
}

func TestTmux_InsideSession(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	t.Setenv("TMUX_PANE", "%3")

	pane, inside, err := mux.NewTmux().InsideSession(context.Background())
	requireNoError(t, err)
	if !inside {
		t.Fatal("InsideSession() inside = false, want true")
	}
	if pane != "%3" {
		t.Fatalf("InsideSession() pane = %q, want %%3", pane)
	}
}

func TestTmux_InsideSessionOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")

	pane, inside, err := mux.NewTmux().InsideSession(context.Background())
	requireNoError(t, err)
	if inside {
		t.Fatal("InsideSession() inside = true, want false")
	}
	if pane != "" {
		t.Fatalf("InsideSession() pane = %q, want empty", pane)
	}
}

func TestTmux_InsideSessionCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := mux.NewTmux().InsideSession(ctx)
	requireErrorIs(t, err, context.Canceled)
}

func TestTmux_NilRunnerFallsBackToExec(t *testing.T) {
	t.Setenv("PATH", "")
	tmux := mux.NewTmuxWithRunner(nil)

	err := tmux.KillSession(context.Background(), "nope")
	if err == nil {
		t.Fatal("KillSession() = nil, want exec lookup error with empty PATH")
	}
	if !strings.Contains(err.Error(), "kill tmux session nope") {
		t.Fatalf("error = %q, want kill tmux session wrapping", err)
	}
}

func TestExecRunner_RunReturnsCombinedOutput(t *testing.T) {
	dir := t.TempDir()
	runner := mux.ExecRunner{}

	output, err := runner.Run(context.Background(), mux.Command{Name: "sh", Dir: dir, Args: []string{"-c", "pwd"}})
	requireNoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	requireNoError(t, err)
	// Resolve the reported path too: on macOS the temp dir sits under
	// the /var -> /private/var symlink and the shell may report either
	// form depending on how it derives its working directory.
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	requireNoError(t, err)
	if got != want {
		t.Fatalf("Run() output = %q, want %q", got, want)
	}
}

func TestExecRunner_RunFailureKeepsOutput(t *testing.T) {
	runner := mux.ExecRunner{}

	output, err := runner.Run(context.Background(), mux.Command{Name: "sh", Args: []string{"-c", "echo boom >&2; exit 3"}})
	if err == nil {
		t.Fatal("Run() = nil, want error for exit 3")
	}
	if !strings.Contains(err.Error(), "run sh") {
		t.Fatalf("error = %q, want run sh wrapping", err)
	}
	if !strings.Contains(string(output), "boom") {
		t.Fatalf("output = %q, want stderr captured", output)
	}
}

func TestRecordingRunner_RunCanceledContext(t *testing.T) {
	runner := mux.NewRecordingRunner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner.Run(ctx, mux.Command{Name: "tmux"})
	requireErrorIs(t, err, context.Canceled)
	if commands := runner.Commands(); len(commands) != 0 {
		t.Fatalf("recorded %d commands, want 0 after canceled context", len(commands))
	}
}

func writeFakeTmux(tb testing.TB) string {
	tb.Helper()
	dir := tb.TempDir()
	path := filepath.Join(dir, "tmux")
	err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700) //nolint:gosec // fixture must be executable for exec.LookPath
	requireNoError(tb, err)

	return dir
}

func requireErrorIs(tb testing.TB, err, target error) {
	tb.Helper()
	if !errors.Is(err, target) {
		tb.Fatalf("error = %v, want %v", err, target)
	}
}
