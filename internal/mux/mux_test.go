package mux_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/kakkoyun/af/internal/mux"
)

// wrapErr adds op context to err for table-test closures, preserving nil
// results and error identity for errors.Is assertions.
func wrapErr(op string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func TestTmux_CreateSessionRunsExpectedCommands(t *testing.T) {
	ctx := context.Background()
	runner := mux.NewRecordingRunner()
	tmux := mux.NewTmuxWithRunner(runner)

	err := tmux.CreateSession(ctx, "kakkoyun--issue-42", "/repo")
	requireNoError(t, err)

	got := runner.Commands()
	want := []mux.Command{
		{Name: "tmux", Args: []string{"new-session", "-d", "-s", "kakkoyun--issue-42", "-c", "/repo"}},
		{Name: "tmux", Args: []string{"set-option", "-t", "kakkoyun--issue-42", "@AF_SESSION", "1"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestTmux_SplitVerticalParsesPaneID(t *testing.T) {
	ctx := context.Background()
	runner := mux.NewRecordingRunner()
	runner.QueueOutput("%5\n")
	tmux := mux.NewTmuxWithRunner(runner)

	pane, err := tmux.SplitVertical(ctx, "session", "/repo/sub")
	requireNoError(t, err)
	if pane != "%5" {
		t.Fatalf("SplitVertical() = %q, want %%5", pane)
	}

	want := []mux.Command{{Name: "tmux", Args: []string{"split-window", "-v", "-P", "-F", "#{pane_id}", "-t", "session", "-c", "/repo/sub"}}}
	if got := runner.Commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestFakeMultiplexer_TracksSessionsPanesAndEnv(t *testing.T) {
	ctx := context.Background()
	fake := mux.NewFakeMultiplexer()

	err := fake.CreateSession(ctx, "session", "/repo")
	requireNoError(t, err)
	exists, err := fake.SessionExists(ctx, "session")
	requireNoError(t, err)
	if !exists {
		t.Fatal("SessionExists() = false, want true")
	}

	pane, err := fake.SplitVertical(ctx, "session", "/repo/sub")
	requireNoError(t, err)
	if pane != "%1" {
		t.Fatalf("SplitVertical() = %q, want %%1", pane)
	}

	err = fake.SetEnv(ctx, "session", "AF_SESSION", "session")
	requireNoError(t, err)
	value, err := fake.GetEnv(ctx, "session", "AF_SESSION")
	requireNoError(t, err)
	if value != "session" {
		t.Fatalf("GetEnv() = %q, want session", value)
	}

	panes, err := fake.ListPanes(ctx, "session")
	requireNoError(t, err)
	wantPanes := []mux.Pane{{ID: "%0", CWD: "/repo"}, {ID: "%1", CWD: "/repo/sub"}}
	if !reflect.DeepEqual(panes, wantPanes) {
		t.Fatalf("ListPanes() = %#v, want %#v", panes, wantPanes)
	}

	err = fake.KillPane(ctx, "session", pane)
	requireNoError(t, err)
	err = fake.KillSession(ctx, "session")
	requireNoError(t, err)
}

func requireNoError(tb testing.TB, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatalf("unexpected error: %v", err)
	}
}
