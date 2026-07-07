package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
)

// writeSlicerBackedStateWithPaths is writeSlicerBackedState but lets the
// caller set distinct host (Worktree.Path) and VM (SlicerWT.Path) paths,
// as ADR-066 §Host continuation normalization needs to be exercised.
func writeSlicerBackedStateWithPaths(t *testing.T, home, name, vm, hostPath, vmPath string) {
	t.Helper()
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name)
	err := os.MkdirAll(stateDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	s := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-00000000c0de",
			Name:      name,
			Status:    "active",
			CreatedAt: time.Now().UTC(),
		},
		Worktree: session.WorktreeState{
			Path:       hostPath,
			Branch:     "feat/sd",
			BaseBranch: "main",
			RepoSlug:   "github.com/kakkoyun/af",
		},
		SlicerWT: session.SlicerWTState{
			VM:         vm,
			Path:       vmPath,
			PushedAt:   time.Now().UTC(),
			LeaseState: session.SlicerWTLeaseHeldByVM,
		},
	}
	err = session.WriteState(filepath.Join(stateDir, "state.toml"), s)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

// TestSessionDataSync_ContinueHostRewritesClaudeTranscript is the
// end-to-end CLI path for ADR-066 §Host continuation: with distinct
// state.SlicerWT.Path (VM) and state.Worktree.Path (host), `session-data
// sync --continue-host` renames the staged Claude project directory to
// the host slug and rewrites its cwd fields before merging, the CLI no
// longer prints the "not yet implemented" notice, and the ledger event
// records continueHost=true.
func TestSessionDataSync_ContinueHostRewritesClaudeTranscript(t *testing.T) { //nolint:cyclop // Test asserts CLI stdout, host content, and ledger fields for a single sync outcome; splitting hurts readability.
	home := t.TempDir()
	t.Setenv("HOME", home)
	vmPath := "/root/workspace/proj"
	writeSlicerBackedStateWithPaths(t, home, "sd-ch", "sbox-ch", home, vmPath)

	vmHome := t.TempDir()
	vmSlug := "-root-workspace-proj"
	claudeDir := filepath.Join(vmHome, ".claude", "projects", vmSlug)
	err := os.MkdirAll(claudeDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(claudeDir, "session1.jsonl"),
		[]byte(`{"cwd":"`+vmPath+`","type":"user"}`+"\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	stdout, stderr, err := executeCommand(t, newRootCmd(), "session-data", "sync", "--agent", "claude", "--continue-host", "sd-ch")
	if err != nil {
		t.Fatalf("session-data sync --continue-host: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if strings.Contains(stdout, "not yet implemented") || strings.Contains(stderr, "not yet implemented") {
		t.Errorf("CLI must no longer print the not-yet-implemented notice; stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stdout, "continue-host rewrote") {
		t.Errorf("stdout should report continue-host rewrite counts; got: %s", stdout)
	}

	// Mirrors sessiondata.claudeProjectSlug: "/" and "." become "-".
	hostSlug := strings.NewReplacer("/", "-", ".", "-").Replace(home)
	dest := filepath.Join(home, ".claude", "projects", hostSlug, "session1.jsonl")
	data, readErr := os.ReadFile(dest) //nolint:gosec // test path under hostHome.
	if readErr != nil {
		t.Fatalf("read host dest %s: %v", dest, readErr)
	}
	if !strings.Contains(string(data), `"cwd":"`+home+`"`) {
		t.Errorf("host dest content should reference host path %s; got: %s", home, data)
	}

	ledgerPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "sd-ch", "ledger.jsonl")
	events, err := session.ReadLedgerTail(t.Context(), ledgerPath, 10)
	if err != nil {
		t.Fatalf("ReadLedgerTail: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("ledger should contain an agent_sessions_synced event")
	}
	last := events[len(events)-1]
	if got, ok := last.Fields["continueHost"].(bool); !ok || !got {
		t.Errorf("ledger continueHost field = %v, want true", last.Fields["continueHost"])
	}
}

// TestSessionDataSync_ContinueHostDryRunReportsCandidates asserts that
// `--dry-run --continue-host` reports per-kind candidate counts and does
// not touch the host destination.
func TestSessionDataSync_ContinueHostDryRunReportsCandidates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	vmPath := "/root/workspace/proj"
	writeSlicerBackedStateWithPaths(t, home, "sd-ch-dry", "sbox-ch-dry", home, vmPath)

	vmHome := t.TempDir()
	claudeDir := filepath.Join(vmHome, ".claude", "projects", "-root-workspace-proj")
	err := os.MkdirAll(claudeDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(claudeDir, "session1.jsonl"), []byte(`{"cwd":"`+vmPath+`"}`+"\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	stdout, _, err := executeCommand(t, newRootCmd(), "session-data", "sync", "--dry-run", "--agent", "claude", "--continue-host", "sd-ch-dry")
	if err != nil {
		t.Fatalf("session-data sync --dry-run --continue-host: %v", err)
	}
	if !strings.Contains(stdout, "would inspect for rewriting") {
		t.Errorf("stdout should mention continue-host candidate preview; got: %s", stdout)
	}
	if !strings.Contains(stdout, "claude=1") {
		t.Errorf("stdout should report claude=1 candidate; got: %s", stdout)
	}
	_, statErr := os.Stat(filepath.Join(home, ".claude", "projects"))
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("dry-run must not touch the host destination; statErr=%v", statErr)
	}
}
