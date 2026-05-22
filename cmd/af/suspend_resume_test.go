package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
)

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
