//go:build integration

package mux_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/mux"
)

// TestIntegrationTmux_SessionLifecycle drives a real tmux server through
// the Multiplexer surface the hermetic suite fakes: create, existence
// probe, environment round-trip, listing, and teardown. Runs only under
// -tags integration on hosts with tmux installed.
func TestIntegrationTmux_SessionLifecycle(t *testing.T) {
	_, err := exec.LookPath("tmux")
	if err != nil {
		t.Skipf("tmux not installed: %v", err)
	}

	tmux := mux.NewTmux()
	ctx := t.Context()
	if !tmux.IsAvailable(ctx) {
		t.Fatal("IsAvailable() = false with tmux on PATH")
	}

	name := fmt.Sprintf("af-integration-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { _ = tmux.KillSession(ctx, name) }) //nolint:errcheck // Best-effort teardown of the CI tmux session.

	err = tmux.CreateSession(ctx, name, t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	exists, err := tmux.SessionExists(ctx, name)
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if !exists {
		t.Fatalf("SessionExists(%s) = false after CreateSession", name)
	}

	assertSessionEnvRoundTrip(t, tmux, name)
	assertSessionListedThenKilled(t, tmux, name)
}

func assertSessionEnvRoundTrip(t *testing.T, tmux mux.Tmux, name string) {
	t.Helper()
	ctx := t.Context()
	err := tmux.SetEnv(ctx, name, "AF_INTEGRATION_PROBE", "probe-value")
	if err != nil {
		t.Fatalf("SetEnv: %v", err)
	}
	value, err := tmux.GetEnv(ctx, name, "AF_INTEGRATION_PROBE")
	if err != nil {
		t.Fatalf("GetEnv: %v", err)
	}
	if value != "probe-value" {
		t.Fatalf("GetEnv = %q, want probe-value", value)
	}
}

func assertSessionListedThenKilled(t *testing.T, tmux mux.Tmux, name string) {
	t.Helper()
	ctx := t.Context()
	sessions, err := tmux.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	found := false
	for i := range sessions {
		if sessions[i].Name == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListSessions missing %s: %+v", name, sessions)
	}

	err = tmux.KillSession(ctx, name)
	if err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	exists, err := tmux.SessionExists(ctx, name)
	if err != nil {
		t.Fatalf("SessionExists after kill: %v", err)
	}
	if exists {
		t.Fatalf("SessionExists(%s) = true after KillSession", name)
	}
}
