package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kakkoyun/af/internal/session"
)

// TestReadLedgerTail_SkipsCorruptLines verifies one malformed JSON line
// no longer poisons the whole read: valid events around it are returned.
func TestReadLedgerTail_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ledger.jsonl")
	content := `{"ts":"2026-07-01T00:00:00Z","event":"created","session":"a"}
{not json at all
{"ts":"2026-07-02T00:00:00Z","event":"suspended","session":"a"}

{"ts":"2026-07-03T00:00:00Z","event":"resumed","session":"a"}
`
	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	events, err := session.ReadLedgerTail(t.Context(), path, 0)
	if err != nil {
		t.Fatalf("ReadLedgerTail: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 3 (corrupt + blank lines skipped)", len(events))
	}
	if events[0].Type != "created" || events[2].Type != "resumed" {
		t.Fatalf("unexpected event order: %q ... %q", events[0].Type, events[2].Type)
	}
}
