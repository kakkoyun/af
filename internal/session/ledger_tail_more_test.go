package session_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

func TestReadLedgerTail_MissingFileReturnsEmpty(t *testing.T) {
	t.Parallel()
	events, err := session.ReadLedgerTail(filepath.Join(t.TempDir(), "absent.jsonl"), 0)
	if err != nil {
		t.Fatalf("ReadLedgerTail(missing) error = %v, want nil", err)
	}
	if len(events) != 0 {
		t.Fatalf("ReadLedgerTail(missing) = %d events, want 0", len(events))
	}
}

func TestReadLedgerTail_TailLimit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	base := time.Date(2026, time.July, 3, 8, 0, 0, 0, time.UTC)
	types := []string{"one", "two", "three", "four", "five"}
	for i, typ := range types {
		err := session.AppendEvent(path, session.Event{Timestamp: base.Add(time.Duration(i) * time.Minute), Type: typ})
		if err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", typ, err)
		}
	}

	events, err := session.ReadLedgerTail(path, 2)
	if err != nil {
		t.Fatalf("ReadLedgerTail(n=2) error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ReadLedgerTail(n=2) = %d events, want 2", len(events))
	}
	if events[0].Type != "four" || events[1].Type != "five" {
		t.Fatalf("tail types = %q, %q; want four, five", events[0].Type, events[1].Type)
	}
}

func TestReadLedgerTail_ParsesAlternateAndPartialKeys(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	content := `{"type":"alt","ts":"2026-07-01T00:00:00Z"}
{"event":123,"ts":456}
{"event":"badts","ts":"not-a-time","extra":"kept"}
`
	writeFile(t, path, content)

	events, err := session.ReadLedgerTail(path, 0)
	if err != nil {
		t.Fatalf("ReadLedgerTail() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(events))
	}
	if events[0].Type != "alt" {
		t.Errorf("events[0].Type = %q, want alt (accepted via \"type\" key)", events[0].Type)
	}
	if events[1].Type != "" || !events[1].Timestamp.IsZero() {
		t.Errorf("events[1] = %+v, want empty type and zero timestamp for non-string values", events[1])
	}
	if events[2].Type != "badts" || !events[2].Timestamp.IsZero() {
		t.Errorf("events[2] = %+v, want type badts with zero timestamp", events[2])
	}
	if events[2].Fields["extra"] != "kept" {
		t.Errorf("events[2].Fields[extra] = %v, want kept", events[2].Fields["extra"])
	}
}

func TestReadLedgerTail_DirectoryPathFailsScan(t *testing.T) {
	t.Parallel()
	_, err := session.ReadLedgerTail(t.TempDir(), 0)
	if err == nil || !strings.Contains(err.Error(), "scan ledger") {
		t.Fatalf("ReadLedgerTail(directory) error = %v, want scan ledger context", err)
	}
}

func TestReadLedgerTail_ParentNotDirectoryFailsOpen(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFile(t, blocker, "not a directory\n")

	_, err := session.ReadLedgerTail(filepath.Join(blocker, "ledger.jsonl"), 0)
	if err == nil || !strings.Contains(err.Error(), "open ledger") {
		t.Fatalf("ReadLedgerTail(ENOTDIR) error = %v, want open ledger context", err)
	}
}
