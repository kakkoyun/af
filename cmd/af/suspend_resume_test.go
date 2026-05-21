package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/lifecycle"
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

	_, _, err := executeCommand(t, newRootCmd(), "suspend", "leased-ws2", "--force")
	if err != nil {
		t.Fatalf("suspend --force: %v", err)
	}
}
