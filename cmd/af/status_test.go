package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/session"
)

func TestStatus_DefaultExcludesCompleted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "live-work", "feat/live", "active")
	writeTestSessionState(t, home, "done-work", "feat/done", "completed")

	stdout, _, err := executeCommand(t, newRootCmd(), "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(stdout, "live-work") {
		t.Fatalf("status missing active workstream; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "done-work") {
		t.Fatalf("status must not include completed workstream by default; got:\n%s", stdout)
	}
}

func TestStatus_AllIncludesCompleted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "live-work", "feat/live", "active")
	writeTestSessionState(t, home, "done-work", "feat/done", "completed")

	stdout, _, err := executeCommand(t, newRootCmd(), "status", "--all")
	if err != nil {
		t.Fatalf("status --all: %v", err)
	}
	for _, want := range []string{"live-work", "done-work"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("status --all missing %q; got:\n%s", want, stdout)
		}
	}
}

func TestStatus_JSONEmitsValidJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "live-work", "feat/live", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "status", "--json")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}

	var rows []map[string]any
	err = json.Unmarshal([]byte(strings.TrimSpace(stdout)), &rows)
	if err != nil {
		t.Fatalf("status --json not valid JSON: %v\noutput:\n%s", err, stdout)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d; output:\n%s", len(rows), stdout)
	}
	for _, key := range []string{"name", "status", "branch"} {
		if _, ok := rows[0][key]; !ok {
			t.Fatalf("JSON row missing key %q; row: %v", key, rows[0])
		}
	}
}

func TestStatus_ShowsLease(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionStateWithLease(t, home, "leased-status", session.SlicerWTLeaseHeldByVM)

	stdout, _, err := executeCommand(t, newRootCmd(), "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(stdout, "sbox-abc") {
		t.Errorf("status output missing VM name; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "held_by_vm") {
		t.Errorf("status output missing lease state; got:\n%s", stdout)
	}
}
