package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
)

// TestWritebackSessionExport_DoesNotClobberConcurrentWrites verifies the
// post-sync writeback re-reads state under the lock: the sync can take
// minutes, and a stale pre-sync snapshot must not revert fields
// committed meanwhile.
func TestWritebackSessionExport_DoesNotClobberConcurrentWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "vmws", "feat/vmws", "active")
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "vmws", "state.toml")

	snapshot, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState snapshot: %v", err)
	}

	concurrent := snapshot
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	concurrent.Stack.ParentSession = "parent"
	concurrent.Stack.LinkedAt = &now
	err = session.WriteState(statePath, concurrent)
	if err != nil {
		t.Fatalf("WriteState concurrent: %v", err)
	}

	err = writebackSessionExport(statePath, snapshot, sessiondata.SyncResult{
		StagingPath: filepath.Join(home, "staging"),
	})
	if err != nil {
		t.Fatalf("writebackSessionExport: %v", err)
	}

	final, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState final: %v", err)
	}
	if final.Stack.ParentSession != "parent" {
		t.Fatalf("Stack.ParentSession = %q, want parent (clobbered by stale snapshot)", final.Stack.ParentSession)
	}
	if final.SessionExport.LastSyncStatus != session.ExportSyncOK {
		t.Fatalf("SessionExport.LastSyncStatus = %q, want %q", final.SessionExport.LastSyncStatus, session.ExportSyncOK)
	}
}
