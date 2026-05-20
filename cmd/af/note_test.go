package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNote_AppendsEventToLedger(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "notework", "feat/notework", "active")

	_, _, err := executeCommand(t, newRootCmd(), "note", "notework", "--append", "hello")
	if err != nil {
		t.Fatalf("note: %v", err)
	}

	ledgerPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "notework", "ledger.jsonl")
	data, err := os.ReadFile(ledgerPath) //nolint:gosec // path under t.TempDir()
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Fatalf("ledger does not contain 'hello'; got:\n%s", data)
	}
}
