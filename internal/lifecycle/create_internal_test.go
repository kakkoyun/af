package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kakkoyun/af/internal/workstream"
)

// TestCheckNameCollision_RefusesEscapingCandidate covers the
// belt-and-suspenders containment check for callers that bypass
// validateCreateOpts.
func TestCheckNameCollision_RefusesEscapingCandidate(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	err := os.MkdirAll(filepath.Join(root, "victim"), 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err = checkNameCollision(stateDir, "", "../victim")
	if !errors.Is(err, workstream.ErrInvalidSessionName) {
		t.Fatalf("want ErrInvalidSessionName, got %v", err)
	}
}
