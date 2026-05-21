package main

import (
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/session"
)

func TestInfo_ShowsLease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithLease(t, home, "leased-info", session.SlicerWTLeaseHeldByVM)

	stdout, _, err := executeCommand(t, newRootCmd(), "info", "leased-info")
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if !strings.Contains(stdout, "Slicer worktree:") {
		t.Errorf("info output missing Slicer worktree section; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "sbox-abc") {
		t.Errorf("info output missing VM name; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "held_by_vm") {
		t.Errorf("info output missing lease state; got:\n%s", stdout)
	}
}
