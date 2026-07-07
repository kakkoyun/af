package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

// writeLifecycleState writes a state.toml in status with the given tmux
// session name and returns its path.
func writeLifecycleState(t *testing.T, dir, status, tmuxSession string) string {
	t.Helper()
	path := filepath.Join(dir, "state.toml")
	st := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "sess-sr-test",
			Name:      "demo",
			Status:    status,
			CreatedAt: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC),
		},
		Worktree: session.WorktreeState{
			Path:    dir,
			Branch:  "demo",
			GitRoot: dir,
		},
		Execution: session.ExecutionState{
			Mode:        "local",
			Multiplexer: "tmux",
			TmuxSession: tmuxSession,
		},
	}
	err := session.WriteState(path, st)
	if err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

// trackingMux wraps FakeMultiplexer to observe respawn behaviour and to
// inject SessionExists failures.
type trackingMux struct {
	*mux.FakeMultiplexer

	existsErr   error
	createCalls int
}

func (m *trackingMux) SessionExists(ctx context.Context, name string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	exists, err := m.FakeMultiplexer.SessionExists(ctx, name)
	if err != nil {
		return exists, fmt.Errorf("fake session exists: %w", err)
	}
	return exists, nil
}

func (m *trackingMux) CreateSession(ctx context.Context, name, cwd string) error {
	m.createCalls++
	err := m.FakeMultiplexer.CreateSession(ctx, name, cwd)
	if err != nil {
		return fmt.Errorf("fake create session: %w", err)
	}
	return nil
}

func TestSuspendWorkstream_TransitionsToSuspended(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "active", "af-demo")
	now := time.Date(2026, 5, 22, 14, 0, 0, 0, time.UTC)

	state, err := lifecycle.SuspendWorkstream(context.Background(), lifecycle.SuspendOptions{
		StatePath: path,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("SuspendWorkstream: %v", err)
	}
	if state.Session.Status != string(lifecycle.Suspended) {
		t.Fatalf("status = %q, want suspended", state.Session.Status)
	}
	if state.Session.SuspendedAt == nil || !state.Session.SuspendedAt.Equal(now) {
		t.Fatalf("SuspendedAt = %v, want %v", state.Session.SuspendedAt, now)
	}

	persisted, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if persisted.Session.Status != string(lifecycle.Suspended) {
		t.Fatalf("persisted status = %q, want suspended", persisted.Session.Status)
	}
	if !strings.Contains(ledgerText(t, path), "suspended") {
		t.Fatal("ledger missing suspended event")
	}
}

func TestSuspendWorkstream_ZeroNowDefaultsToWallClock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "active", "")

	state, err := lifecycle.SuspendWorkstream(context.Background(), lifecycle.SuspendOptions{StatePath: path})
	if err != nil {
		t.Fatalf("SuspendWorkstream: %v", err)
	}
	if state.Session.SuspendedAt == nil || state.Session.SuspendedAt.IsZero() {
		t.Fatalf("SuspendedAt = %v, want wall-clock default", state.Session.SuspendedAt)
	}
}

func TestSuspendWorkstream_RejectsInvalidSourceStates(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"suspended", "completed", "abandoned"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := writeLifecycleState(t, dir, status, "")

			_, err := lifecycle.SuspendWorkstream(context.Background(), lifecycle.SuspendOptions{StatePath: path})
			if !errors.Is(err, lifecycle.ErrLifecycleTransition) {
				t.Fatalf("want ErrLifecycleTransition, got %v", err)
			}
		})
	}
}

func TestSuspendWorkstream_ReadStateError(t *testing.T) {
	t.Parallel()
	_, err := lifecycle.SuspendWorkstream(context.Background(), lifecycle.SuspendOptions{
		StatePath: filepath.Join(t.TempDir(), "missing", "state.toml"),
	})
	if err == nil {
		t.Fatal("expected error for missing state path")
	}
}

func TestResumeWorkstream_RespawnsDeadTmuxSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "suspended", "af-demo")
	muxFake := mux.NewFakeMultiplexer()
	now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)

	state, err := lifecycle.ResumeWorkstream(context.Background(),
		lifecycle.ResumeDeps{Mux: muxFake},
		lifecycle.ResumeOptions{StatePath: path, Now: now})
	if err != nil {
		t.Fatalf("ResumeWorkstream: %v", err)
	}
	if state.Session.Status != string(lifecycle.Active) {
		t.Fatalf("status = %q, want active", state.Session.Status)
	}
	if state.Session.SuspendedAt != nil {
		t.Fatalf("SuspendedAt = %v, want nil after resume", state.Session.SuspendedAt)
	}
	exists, err := muxFake.SessionExists(context.Background(), "af-demo")
	if err != nil {
		t.Fatalf("SessionExists: %v", err)
	}
	if !exists {
		t.Fatal("tmux session not respawned on resume")
	}
	if !strings.Contains(ledgerText(t, path), "resumed") {
		t.Fatal("ledger missing resumed event")
	}
}

func TestResumeWorkstream_BareSkipsMultiplexer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "suspended", "af-demo")
	tracker := &trackingMux{FakeMultiplexer: mux.NewFakeMultiplexer()}

	_, err := lifecycle.ResumeWorkstream(context.Background(),
		lifecycle.ResumeDeps{Mux: tracker},
		lifecycle.ResumeOptions{StatePath: path, Bare: true})
	if err != nil {
		t.Fatalf("ResumeWorkstream --bare: %v", err)
	}
	if tracker.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0 for --bare", tracker.createCalls)
	}
}

func TestResumeWorkstream_KeepsAliveTmuxSession(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "suspended", "af-demo")
	tracker := &trackingMux{FakeMultiplexer: mux.NewFakeMultiplexer()}
	err := tracker.FakeMultiplexer.CreateSession(context.Background(), "af-demo", dir)
	if err != nil {
		t.Fatalf("pre-create session: %v", err)
	}

	_, err = lifecycle.ResumeWorkstream(context.Background(),
		lifecycle.ResumeDeps{Mux: tracker},
		lifecycle.ResumeOptions{StatePath: path})
	if err != nil {
		t.Fatalf("ResumeWorkstream: %v", err)
	}
	if tracker.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0 for an alive session", tracker.createCalls)
	}
}

func TestResumeWorkstream_RespawnsWhenExistsCheckFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "suspended", "af-demo")
	existsErr := errors.New("tmux server gone") //nolint:err113 // test-only sentinel
	tracker := &trackingMux{FakeMultiplexer: mux.NewFakeMultiplexer(), existsErr: existsErr}

	_, err := lifecycle.ResumeWorkstream(context.Background(),
		lifecycle.ResumeDeps{Mux: tracker},
		lifecycle.ResumeOptions{StatePath: path})
	if err != nil {
		t.Fatalf("ResumeWorkstream: %v", err)
	}
	if tracker.createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1 when exists-check fails", tracker.createCalls)
	}
}

func TestResumeWorkstream_NilMuxAndEmptySessionAreTolerated(t *testing.T) {
	t.Parallel()
	t.Run("nil mux", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := writeLifecycleState(t, dir, "suspended", "af-demo")
		_, err := lifecycle.ResumeWorkstream(context.Background(),
			lifecycle.ResumeDeps{}, lifecycle.ResumeOptions{StatePath: path})
		if err != nil {
			t.Fatalf("ResumeWorkstream with nil mux: %v", err)
		}
	})
	t.Run("empty tmux session", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := writeLifecycleState(t, dir, "suspended", "")
		tracker := &trackingMux{FakeMultiplexer: mux.NewFakeMultiplexer()}
		_, err := lifecycle.ResumeWorkstream(context.Background(),
			lifecycle.ResumeDeps{Mux: tracker}, lifecycle.ResumeOptions{StatePath: path})
		if err != nil {
			t.Fatalf("ResumeWorkstream with empty tmux session: %v", err)
		}
		if tracker.createCalls != 0 {
			t.Fatalf("createCalls = %d, want 0 for empty tmux session name", tracker.createCalls)
		}
	})
}

func TestResumeWorkstream_RejectsInvalidSourceStates(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"active", "completed", "abandoned"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := writeLifecycleState(t, dir, status, "")

			_, err := lifecycle.ResumeWorkstream(context.Background(),
				lifecycle.ResumeDeps{}, lifecycle.ResumeOptions{StatePath: path})
			if !errors.Is(err, lifecycle.ErrLifecycleTransition) {
				t.Fatalf("want ErrLifecycleTransition, got %v", err)
			}
		})
	}
}

// blockLedger occupies <dir>/ledger.jsonl with a directory so that
// session.AppendEvent fails with EISDIR.
func blockLedger(t *testing.T, dir string) {
	t.Helper()
	err := os.MkdirAll(filepath.Join(dir, "ledger.jsonl"), 0o750)
	if err != nil {
		t.Fatalf("block ledger: %v", err)
	}
}

func TestSuspendWorkstream_AppendEventFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "active", "")
	blockLedger(t, dir)

	_, err := lifecycle.SuspendWorkstream(context.Background(), lifecycle.SuspendOptions{StatePath: path})
	if err == nil || !strings.Contains(err.Error(), "append event") {
		t.Fatalf("err = %v, want append event failure", err)
	}
}

func TestResumeWorkstream_AppendEventFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeLifecycleState(t, dir, "suspended", "")
	blockLedger(t, dir)

	_, err := lifecycle.ResumeWorkstream(context.Background(),
		lifecycle.ResumeDeps{}, lifecycle.ResumeOptions{StatePath: path})
	if err == nil || !strings.Contains(err.Error(), "append event") {
		t.Fatalf("err = %v, want append event failure", err)
	}
}

func TestResumeWorkstream_ReadStateError(t *testing.T) {
	t.Parallel()
	_, err := lifecycle.ResumeWorkstream(context.Background(),
		lifecycle.ResumeDeps{},
		lifecycle.ResumeOptions{StatePath: filepath.Join(t.TempDir(), "missing", "state.toml")})
	if err == nil {
		t.Fatal("expected error for missing state path")
	}
}
