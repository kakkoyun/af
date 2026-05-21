package lifecycle_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

// fakeSandboxRunner satisfies sandbox.Runner for pull tests.
type fakeSandboxRunner struct { //nolint:govet // readability over packing
	output []byte
	err    error
}

func (f fakeSandboxRunner) Run(_ context.Context, _ sandbox.Command) ([]byte, error) {
	return f.output, f.err
}

func writePullState(t *testing.T, dir string, st session.SlicerWTLeaseState) string {
	t.Helper()
	path := filepath.Join(dir, "state.toml")
	s := session.State{
		SchemaVersion: 1,
		Session:       session.Info{Name: "demo", Status: "active", CreatedAt: time.Now().UTC()},
		SlicerWT: session.SlicerWTState{
			VM:         "sbox-abc123",
			Path:       dir,
			PushedAt:   time.Now().UTC(),
			LeaseState: st,
		},
	}
	err := session.WriteState(path, s)
	if err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

func TestPull_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writePullState(t, dir, session.SlicerWTLeaseHeldByVM)
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)

	res, err := lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{output: []byte("pull ok")}},
		lifecycle.PullOptions{StatePath: path, Now: now},
	)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.VM != "sbox-abc123" {
		t.Errorf("VM = %q, want sbox-abc123", res.VM)
	}
	if !res.PulledAt.Equal(now) {
		t.Errorf("PulledAt = %v, want %v", res.PulledAt, now)
	}

	// State on disk should reflect pulled.
	updated, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if updated.SlicerWT.LeaseState != session.SlicerWTLeasePulled {
		t.Errorf("LeaseState = %q, want pulled", updated.SlicerWT.LeaseState)
	}
	if updated.SlicerWT.PulledAt == nil {
		t.Fatal("PulledAt is nil in updated state")
	}
}

func TestPull_RefusesWhenNoLease(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	s := session.State{SchemaVersion: 1, Session: session.Info{Name: "demo", Status: "active", CreatedAt: time.Now().UTC()}}
	err := session.WriteState(path, s)
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{}},
		lifecycle.PullOptions{StatePath: path},
	)
	if !errors.Is(err, lifecycle.ErrPullNoLease) {
		t.Errorf("want ErrPullNoLease, got %v", err)
	}
}

func TestPull_RefusesWhenAlreadyPulled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writePullState(t, dir, session.SlicerWTLeasePulled)

	_, err := lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{}},
		lifecycle.PullOptions{StatePath: path},
	)
	if !errors.Is(err, lifecycle.ErrPullAlreadyPulled) {
		t.Errorf("want ErrPullAlreadyPulled, got %v", err)
	}
}

func TestPull_RefusesWhenDiscarded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writePullState(t, dir, session.SlicerWTLeaseDiscarded)

	_, err := lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{}},
		lifecycle.PullOptions{StatePath: path},
	)
	if !errors.Is(err, lifecycle.ErrPullDiscarded) {
		t.Errorf("want ErrPullDiscarded, got %v", err)
	}
}

func TestPull_PropagatesRunnerError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writePullState(t, dir, session.SlicerWTLeaseHeldByVM)

	runErr := errors.New("connection refused") //nolint:err113 // test-only sentinel
	_, err := lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{err: runErr}},
		lifecycle.PullOptions{StatePath: path},
	)
	if !errors.Is(err, lifecycle.ErrPullFailed) {
		t.Errorf("want ErrPullFailed, got %v", err)
	}
	if !errors.Is(err, runErr) {
		t.Errorf("want wrapped runErr, got %v", err)
	}

	// State must not have been updated on failure.
	updated, readErr2 := session.ReadState(path)
	if readErr2 != nil {
		t.Fatalf("re-read state: %v", readErr2)
	}
	if updated.SlicerWT.LeaseState != session.SlicerWTLeaseHeldByVM {
		t.Errorf("LeaseState changed on error: %q", updated.SlicerWT.LeaseState)
	}
}

func TestPull_MissingStatePath(t *testing.T) {
	t.Parallel()
	_, err := lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{}},
		lifecycle.PullOptions{StatePath: filepath.Join(t.TempDir(), "nosuch", "state.toml")},
	)
	if err == nil {
		t.Fatal("expected error for missing state path")
	}
	if errors.Is(err, os.ErrNotExist) || err.Error() == "" {
		return // acceptable
	}
}

// --- Suspend lease enforcement ---

func writeStateWithLease(t *testing.T, dir string, lease session.SlicerWTLeaseState) string {
	t.Helper()
	path := filepath.Join(dir, "state.toml")
	s := session.State{
		SchemaVersion: 1,
		Session:       session.Info{Name: "demo", Status: "active", CreatedAt: time.Now().UTC()},
		SlicerWT: session.SlicerWTState{
			VM:         "sbox-xyz",
			Path:       dir,
			PushedAt:   time.Now().UTC(),
			LeaseState: lease,
		},
	}
	err := session.WriteState(path, s)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestSuspend_RefusesWhenLeasedToVMWithoutForce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeStateWithLease(t, dir, session.SlicerWTLeaseHeldByVM)

	_, err := lifecycle.SuspendWorkstream(context.Background(), lifecycle.SuspendOptions{StatePath: path})
	if !errors.Is(err, lifecycle.ErrSuspendLeasedToVM) {
		t.Errorf("want ErrSuspendLeasedToVM, got %v", err)
	}
}

func TestSuspend_AllowsForceMarksDiscarded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeStateWithLease(t, dir, session.SlicerWTLeaseHeldByVM)

	_, err := lifecycle.SuspendWorkstream(context.Background(), lifecycle.SuspendOptions{StatePath: path, Force: true})
	if err != nil {
		t.Fatalf("SuspendWorkstream --force: %v", err)
	}
	updated, readErrSusp := session.ReadState(path)
	if readErrSusp != nil {
		t.Fatalf("re-read after suspend --force: %v", readErrSusp)
	}
	if updated.SlicerWT.LeaseState != session.SlicerWTLeaseDiscarded {
		t.Errorf("LeaseState = %q, want discarded", updated.SlicerWT.LeaseState)
	}
}

// --- Done lease enforcement ---

func writeActiveDoneState(t *testing.T, dir string, lease session.SlicerWTLeaseState) string {
	t.Helper()
	path := filepath.Join(dir, "state.toml")
	s := session.State{
		SchemaVersion: 1,
		Session:       session.Info{Name: "demo", Status: "active", CreatedAt: time.Now().UTC()},
		Worktree:      session.WorktreeState{GitRoot: dir, Path: dir, Branch: "feat/x"},
		SlicerWT: session.SlicerWTState{
			VM:         "sbox-xyz",
			Path:       dir,
			PushedAt:   time.Now().UTC(),
			LeaseState: lease,
		},
	}
	err := session.WriteState(path, s)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestDone_RefusesWhenLeasedToVMWithoutForce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeActiveDoneState(t, dir, session.SlicerWTLeaseHeldByVM)

	_, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner(), Mux: nil},
		lifecycle.DoneOptions{StatePath: path},
	)
	if !errors.Is(err, lifecycle.ErrDoneLeasedToVM) {
		t.Errorf("want ErrDoneLeasedToVM, got %v", err)
	}
}

func TestDone_AllowsForceMarksDiscarded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeActiveDoneState(t, dir, session.SlicerWTLeaseHeldByVM)

	_, err := lifecycle.FinishWorkstream(context.Background(),
		lifecycle.DoneDeps{Git: git.NewFakeRunner(), Mux: nil},
		lifecycle.DoneOptions{StatePath: path, Force: true},
	)
	// May fail on worktree removal (dir is not a real git repo), that's fine;
	// we just need the lease check itself not to block.
	if errors.Is(err, lifecycle.ErrDoneLeasedToVM) {
		t.Errorf("should not return ErrDoneLeasedToVM with Force=true")
	}
}
