package lifecycle_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/session"
)

// TestPull_ZeroNowUsesSlicerTimestamp covers the Now-defaulting branch:
// with a zero Options.Now, PulledAt comes from the slicer wt pull result.
func TestPull_ZeroNowUsesSlicerTimestamp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writePullState(t, dir, session.SlicerWTLeaseHeldByVM)
	before := time.Now().UTC().Add(-time.Minute)

	res, err := lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{output: []byte("pull ok")}},
		lifecycle.PullOptions{StatePath: path},
	)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.PulledAt.IsZero() || res.PulledAt.Before(before) {
		t.Fatalf("PulledAt = %v, want recent slicer timestamp", res.PulledAt)
	}

	updated, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("re-read state: %v", err)
	}
	if updated.SlicerWT.PulledAt == nil || !updated.SlicerWT.PulledAt.Equal(res.PulledAt) {
		t.Fatalf("persisted PulledAt = %v, want %v", updated.SlicerWT.PulledAt, res.PulledAt)
	}
}

// TestPull_ReturnsSessionNameFromState pins the result identity fields.
func TestPull_ReturnsSessionNameFromState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writePullState(t, dir, session.SlicerWTLeaseHeldByVM)

	res, err := lifecycle.Pull(context.Background(),
		lifecycle.PullDeps{Runner: fakeSandboxRunner{}},
		lifecycle.PullOptions{StatePath: path},
	)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.SessionName != "demo" {
		t.Errorf("SessionName = %q, want demo", res.SessionName)
	}
	if !strings.HasPrefix(res.VM, "sbox-") {
		t.Errorf("VM = %q, want sbox- prefix", res.VM)
	}
}
