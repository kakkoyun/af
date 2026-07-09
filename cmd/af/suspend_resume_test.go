package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
)

// writeTestSessionStateWithTmux is writeTestSessionState plus an
// Execution.TmuxSession, needed by the resume-attach tests below since
// attach targets state.Execution.TmuxSession.
func writeTestSessionStateWithTmux(t *testing.T, home, name, branch, status, tmuxSession string) {
	t.Helper()
	writeTestSessionState(t, home, name, branch, status)
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name, "state.toml")
	err := session.Update(statePath, func(s *session.State) error {
		s.Execution.TmuxSession = tmuxSession
		return nil
	})
	if err != nil {
		t.Fatalf("set tmux session: %v", err)
	}
}

// installFakeResumeMux replaces newResumeMux with a fresh
// *mux.FakeMultiplexer and restores the real constructor on cleanup.
func installFakeResumeMux(t *testing.T) *mux.FakeMultiplexer {
	t.Helper()
	fake := mux.NewFakeMultiplexer()
	orig := newResumeMux
	newResumeMux = func() mux.Multiplexer { return fake }
	t.Cleanup(func() { newResumeMux = orig })
	return fake
}

// sessionAttached reports whether name is attached in fake, per
// mux.FakeMultiplexer's Session.Attached bookkeeping.
func sessionAttached(t *testing.T, fake *mux.FakeMultiplexer, name string) bool {
	t.Helper()
	sessions, err := fake.ListSessions(t.Context())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, s := range sessions {
		if s.Name == name {
			return s.Attached
		}
	}
	return false
}

func TestSuspend_TransitionsActiveToSuspended(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "mywork", "feat/mywork", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "suspend", "mywork")
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if !strings.Contains(stdout, "suspended") {
		t.Fatalf("expected stdout to mention 'suspended'; got:\n%s", stdout)
	}
}

func TestResume_TransitionsSuspendedToActive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "mywork", "feat/mywork", "suspended")

	stdout, _, err := executeCommand(t, newRootCmd(), "resume", "mywork", "--bare")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(stdout, "active") {
		t.Fatalf("expected stdout to mention 'active'; got:\n%s", stdout)
	}
}

// TestResume_SuspendedNonBareAttaches pins that a normal (non-bare)
// resume of a suspended workstream attaches to its tmux session via the
// shared attach mechanism, once it is back to active.
func TestResume_SuspendedNonBareAttaches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithTmux(t, home, "mywork", "feat/mywork", "suspended", "af-mywork")
	fake := installFakeResumeMux(t)
	err := fake.CreateSession(t.Context(), "af-mywork", home)
	if err != nil {
		t.Fatalf("pre-create fake tmux session: %v", err)
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "resume", "mywork")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(stdout, "active") {
		t.Fatalf("expected stdout to mention 'active'; got:\n%s", stdout)
	}
	if !sessionAttached(t, fake, "af-mywork") {
		t.Fatal("resume (non-bare) should attach to the respawned tmux session")
	}
}

// TestResume_ActiveSessionAttachesInsteadOfErroring is the issue #23
// regression pin: `af resume` on an already-active workstream must not
// hit the lifecycle FSM's "invalid transition" error. It should print a
// no-op notice on stderr and attach.
func TestResume_ActiveSessionAttachesInsteadOfErroring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithTmux(t, home, "demo", "feat/demo", "active", "af-demo")
	fake := installFakeResumeMux(t)
	err := fake.CreateSession(t.Context(), "af-demo", home)
	if err != nil {
		t.Fatalf("pre-create fake tmux session: %v", err)
	}

	_, stderr, err := executeCommand(t, newRootCmd(), "resume", "demo")
	if err != nil {
		t.Fatalf("resume on active session should not error, got: %v", err)
	}
	if !strings.Contains(stderr, "session 'demo' is already active") {
		t.Fatalf("stderr = %q, want the already-active notice", stderr)
	}
	if !sessionAttached(t, fake, "af-demo") {
		t.Fatal("resume on an active session should attach to its tmux session")
	}
}

// TestResume_ActiveSessionRespawnsDeadTmuxBeforeAttach is the issue #33
// Fix 2 regression pin: `af resume` on an already-active workstream
// whose tmux session died out from under it (tmux server restarted, or
// the session was killed out-of-band) must respawn the session before
// attaching, instead of attaching directly into a session that no
// longer exists.
func TestResume_ActiveSessionRespawnsDeadTmuxBeforeAttach(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithTmux(t, home, "demo", "feat/demo", "active", "af-demo")
	fake := installFakeResumeMux(t)
	// No pre-created fake tmux session: simulates a dead tmux server.

	_, stderr, err := executeCommand(t, newRootCmd(), "resume", "demo")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(stderr, "session 'demo' is already active") {
		t.Fatalf("stderr = %q, want the already-active notice", stderr)
	}
	exists, existsErr := fake.SessionExists(t.Context(), "af-demo")
	if existsErr != nil {
		t.Fatalf("SessionExists: %v", existsErr)
	}
	if !exists {
		t.Fatal("resume on an active workstream with a dead tmux session must respawn it")
	}
	if !sessionAttached(t, fake, "af-demo") {
		t.Fatal("resume on an active workstream must attach after respawning the dead tmux session")
	}
}

// TestResume_ActiveSessionLiveSessionSkipsRespawn pins the other half of
// issue #33 Fix 2: when the tmux session is still alive, resume must
// attach only — it must not recreate (and thereby reset) the session.
func TestResume_ActiveSessionLiveSessionSkipsRespawn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithTmux(t, home, "demo", "feat/demo", "active", "af-demo")
	fake := installFakeResumeMux(t)
	err := fake.CreateSession(t.Context(), "af-demo", home)
	if err != nil {
		t.Fatalf("pre-create fake tmux session: %v", err)
	}
	err = fake.SetEnv(t.Context(), "af-demo", "AF_MARK", "original")
	if err != nil {
		t.Fatalf("mark original session: %v", err)
	}

	_, _, err = executeCommand(t, newRootCmd(), "resume", "demo")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	mark, err := fake.GetEnv(t.Context(), "af-demo", "AF_MARK")
	if err != nil {
		t.Fatalf("GetEnv: %v", err)
	}
	if mark != "original" {
		t.Fatal("resume on an active workstream with a live tmux session must not recreate it")
	}
	if !sessionAttached(t, fake, "af-demo") {
		t.Fatal("resume on an active workstream must attach to the live tmux session")
	}
}

// TestResume_ActiveSessionBareIsNoOp covers the --bare branch of issue
// #23: no attach, just the notice plus a manual-attach hint, and still
// exit 0 (not an error).
func TestResume_ActiveSessionBareIsNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithTmux(t, home, "demo", "feat/demo", "active", "af-demo")
	fake := installFakeResumeMux(t)
	err := fake.CreateSession(t.Context(), "af-demo", home)
	if err != nil {
		t.Fatalf("pre-create fake tmux session: %v", err)
	}

	_, stderr, err := executeCommand(t, newRootCmd(), "resume", "demo", "--bare")
	if err != nil {
		t.Fatalf("resume --bare on active session should not error, got: %v", err)
	}
	if !strings.Contains(stderr, "session 'demo' is already active") {
		t.Fatalf("stderr = %q, want the already-active notice", stderr)
	}
	if !strings.Contains(stderr, "to attach: tmux attach -t af-demo") {
		t.Fatalf("stderr = %q, want the manual attach hint", stderr)
	}
	if sessionAttached(t, fake, "af-demo") {
		t.Fatal("resume --bare on an active session must not attach")
	}
}

// TestResume_TerminalStatesStillError pins that completed/abandoned
// sessions keep erroring on resume (only the active-session case
// becomes a no-op per issue #23).
func TestResume_TerminalStatesStillError(t *testing.T) {
	for _, status := range []string{"completed", "abandoned"} {
		t.Run(status, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeTestSessionState(t, home, "demo", "feat/demo", status)
			installFakeResumeMux(t)

			_, _, err := executeCommand(t, newRootCmd(), "resume", "demo")
			if !errors.Is(err, lifecycle.ErrLifecycleTransition) {
				t.Fatalf("resume on %s = %v, want ErrLifecycleTransition", status, err)
			}
		})
	}
}

func TestSuspend_LeaseRefusal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithLease(t, home, "leased-ws", session.SlicerWTLeaseHeldByVM)
	installNoopSlicerFactory(t)

	_, _, err := executeCommand(t, newRootCmd(), "suspend", "leased-ws")
	if err == nil {
		t.Fatal("expected error when workstream is leased to VM")
	}
	if !errors.Is(err, lifecycle.ErrSuspendLeasedToVM) {
		t.Errorf("want ErrSuspendLeasedToVM, got %v", err)
	}
}

func TestSuspend_ForceAllowsWithLease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithLease(t, home, "leased-ws2", session.SlicerWTLeaseHeldByVM)
	installNoopSlicerFactory(t)

	_, _, err := executeCommand(t, newRootCmd(), "suspend", "leased-ws2", "--force")
	if err != nil {
		t.Fatalf("suspend --force: %v", err)
	}
}

// installNoopSlicerFactory replaces sessiondataSlicerFactory with a
// FakeSlicer whose Source is an empty temp directory. The fake's
// Inventory returns no entries, so the auto-sync runs to completion
// with zero imports and no conflicts — the test can then exercise
// ADR-065 lease behaviour without depending on a real slicer binary.
func installNoopSlicerFactory(t *testing.T) {
	t.Helper()
	orig := sessiondataSlicerFactory
	t.Cleanup(func() { sessiondataSlicerFactory = orig })
	empty := t.TempDir()
	sessiondataSlicerFactory = func() sessiondata.Slicer { return &sessiondata.FakeSlicer{Source: empty} }
}

// TestSuspend_WaitsForHeldSessionLock verifies mutating commands
// serialize behind the per-session flock instead of interleaving with a
// concurrent af process.
func TestSuspend_WaitsForHeldSessionLock(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "locked-ws", "feat/locked", "active")

	lockPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "locked-ws", session.LockFileName)
	lock, err := session.LockFile(lockPath, session.LockExclusive)
	if err != nil {
		t.Fatalf("pre-hold lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, _, execErr := executeCommand(t, newRootCmd(), "suspend", "locked-ws")
		done <- execErr
	}()

	select {
	case <-done:
		t.Fatal("suspend completed while the session lock was held")
	case <-time.After(200 * time.Millisecond):
	}

	err = lock.Unlock()
	if err != nil {
		t.Fatalf("unlock: %v", err)
	}

	select {
	case execErr := <-done:
		if execErr != nil {
			t.Fatalf("suspend after lock release: %v", execErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("suspend did not complete after the lock was released")
	}
}
