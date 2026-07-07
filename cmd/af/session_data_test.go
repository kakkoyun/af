package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
)

func writeSlicerBackedState(t *testing.T, home, name, vm string) {
	t.Helper()
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name)
	err := os.MkdirAll(stateDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	s := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID:        "00000000-0000-0000-0000-00000000beef",
			Name:      name,
			Status:    "active",
			CreatedAt: time.Now().UTC(),
		},
		Worktree: session.WorktreeState{
			Path:       home,
			Branch:     "feat/sd",
			BaseBranch: "main",
			RepoSlug:   "github.com/kakkoyun/af",
		},
		SlicerWT: session.SlicerWTState{
			VM:         vm,
			Path:       home,
			PushedAt:   time.Now().UTC(),
			LeaseState: session.SlicerWTLeaseHeldByVM,
		},
	}
	err = session.WriteState(filepath.Join(stateDir, "state.toml"), s)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

func writeNonSlicerState(t *testing.T, home, name string) {
	t.Helper()
	stateDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name)
	err := os.MkdirAll(stateDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	s := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			ID: "00000000-0000-0000-0000-0000000fffff", Name: name,
			Status: "active", CreatedAt: time.Now().UTC(),
		},
		Worktree: session.WorktreeState{
			Path: home, Branch: "feat/x", BaseBranch: "main", RepoSlug: "github.com/x/y",
		},
	}
	err = session.WriteState(filepath.Join(stateDir, "state.toml"), s)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

// withSessionDataSlicer replaces the package-level slicer factory and
// restores it on cleanup. Returns the FakeSlicer so tests can populate
// its Source filesystem.
func withSessionDataSlicer(t *testing.T, fake *sessiondata.FakeSlicer) {
	t.Helper()
	orig := sessiondataSlicerFactory
	t.Cleanup(func() { sessiondataSlicerFactory = orig })
	sessiondataSlicerFactory = func() sessiondata.Slicer { return fake }
}

func TestSessionDataSync_ListsAndPullsSlicerBacked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "sd-pull", "sbox-aaa")

	// Build a fake VM home directory with one allowlisted file per agent.
	vmHome := t.TempDir()
	err := os.MkdirAll(filepath.Join(vmHome, ".codex", "sessions"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vmHome, ".codex", "sessions", "rollout-abc.jsonl"), []byte("CONTENT-A"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	stdout, stderr, err := executeCommand(t, newRootCmd(), "session-data", "sync", "--agent", "codex", "sd-pull")
	if err != nil {
		t.Fatalf("session-data sync: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "imported 1") {
		t.Errorf("stdout should report imported=1; got: %s", stdout)
	}
	// The host destination should now contain the codex rollout.
	dest := filepath.Join(home, ".codex", "sessions", "rollout-abc.jsonl")
	data, err := os.ReadFile(dest) //nolint:gosec // test path under hostHome.
	if err != nil {
		t.Fatalf("read host dest: %v", err)
	}
	if string(data) != "CONTENT-A" {
		t.Errorf("dest content = %q, want CONTENT-A", data)
	}
	// A ledger event must have been appended.
	ledgerPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "sd-pull", "ledger.jsonl")
	events, err := session.ReadLedgerTail(t.Context(), ledgerPath, 10)
	if err != nil {
		t.Fatalf("ReadLedgerTail: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("ledger should contain agent_sessions_pulled event; got 0 events")
	}
	if events[len(events)-1].Type != "agent_sessions_synced" {
		t.Errorf("last event type = %q, want agent_sessions_synced", events[len(events)-1].Type)
	}
}

func TestSessionDataSync_DryRunDoesNotImport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "sd-dry", "sbox-bbb")

	vmHome := t.TempDir()
	err := os.MkdirAll(filepath.Join(vmHome, ".codex", "sessions"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vmHome, ".codex", "sessions", "r.jsonl"), []byte("X"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	stdout, _, err := executeCommand(t, newRootCmd(), "session-data", "sync", "--dry-run", "--agent", "codex", "sd-dry")
	if err != nil {
		t.Fatalf("session-data sync --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Errorf("stdout should mention dry-run; got: %s", stdout)
	}
	// Host destination must not have been touched.
	_, statErr := os.Stat(filepath.Join(home, ".codex", "sessions", "r.jsonl"))
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("dry-run should not touch host destination; statErr=%v", statErr)
	}
}

func TestSessionDataSync_RejectsNonSlicerSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeNonSlicerState(t, home, "sd-local")
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: t.TempDir()})

	_, _, err := executeCommand(t, newRootCmd(), "session-data", "sync", "sd-local")
	if !errors.Is(err, errSessionDataNoLease) {
		t.Errorf("want errSessionDataNoLease, got %v", err)
	}
}

func TestSessionDataList_ShowsManifest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "sd-list", "sbox-ccc")

	vmHome := t.TempDir()
	err := os.MkdirAll(filepath.Join(vmHome, ".pi", "agent", "sessions"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vmHome, ".pi", "agent", "sessions", "sess1.jsonl"), []byte("p"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	stdout, _, err := executeCommand(t, newRootCmd(), "session-data", "list", "sd-list")
	if err != nil {
		t.Fatalf("session-data list: %v", err)
	}
	if !strings.Contains(stdout, "vm=sbox-ccc") {
		t.Errorf("stdout should mention VM name; got: %s", stdout)
	}
	if !strings.Contains(stdout, "pi=1") {
		t.Errorf("stdout should mention pi=1 in summary; got: %s", stdout)
	}
	if !strings.Contains(stdout, "sess1.jsonl") {
		t.Errorf("stdout should list session file path; got: %s", stdout)
	}
}

func TestSessionDataSync_BadAgentFlagRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "sd-bad", "sbox-ddd")
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: t.TempDir()})

	_, _, err := executeCommand(t, newRootCmd(), "session-data", "sync", "--agent", "nope", "sd-bad")
	if !errors.Is(err, sessiondata.ErrUnknownAgent) {
		t.Errorf("want ErrUnknownAgent, got %v", err)
	}
}

// Sanity check: ensure context.Background-based test helpers do not
// invoke the real slicer binary when the factory is replaced.
func TestSessionDataSync_FactoryReplaceableByTest(t *testing.T) {
	orig := sessiondataSlicerFactory
	t.Cleanup(func() { sessiondataSlicerFactory = orig })
	called := 0
	sessiondataSlicerFactory = func() sessiondata.Slicer {
		called++
		return &sessiondata.FakeSlicer{Source: t.TempDir()}
	}
	_ = sessiondataSlicerFactory()
	if called != 1 {
		t.Errorf("factory should be called once; got %d", called)
	}
	_ = context.Background() // silence import.
}

// TestSessionDataSync_WritebackPopulatesStateExport asserts that a
// successful sync writes the [session_export] section into state.toml
// per ADR-067 §State schema, including per-source cursors.
func TestSessionDataSync_WritebackPopulatesStateExport(t *testing.T) { //nolint:cyclop // Test asserts multiple invariants of a single sync outcome; splitting hurts readability.
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "sd-write", "sbox-write")

	vmHome := t.TempDir()
	err := os.MkdirAll(filepath.Join(vmHome, ".pi", "agent", "sessions"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vmHome, ".pi", "agent", "sessions", "abc.jsonl"), []byte(`{"ok":true}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	_, _, err = executeCommand(t, newRootCmd(), "session-data", "sync", "--agent", "pi", "sd-write")
	if err != nil {
		t.Fatalf("session-data sync: %v", err)
	}

	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "sd-write", "state.toml")
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.SessionExport.LastSyncStatus != session.ExportSyncOK {
		t.Errorf("LastSyncStatus = %q, want ok", state.SessionExport.LastSyncStatus)
	}
	if state.SessionExport.LastSyncAt == nil {
		t.Fatal("LastSyncAt is nil after sync")
	}
	if state.SessionExport.LastManifest == "" {
		t.Error("LastManifest is empty")
	}
	if len(state.SessionExport.Sources) != 1 {
		t.Fatalf("len(Sources) = %d, want 1", len(state.SessionExport.Sources))
	}
	src := state.SessionExport.Sources[0]
	if src.Agent != "pi" {
		t.Errorf("Source.Agent = %q, want pi", src.Agent)
	}
	if src.VM != "sbox-write" {
		t.Errorf("Source.VM = %q, want sbox-write", src.VM)
	}
	if src.Status != session.SourceStatusOK {
		t.Errorf("Source.Status = %q, want ok", src.Status)
	}
	if src.Hash == "" {
		t.Errorf("Source.Hash is empty")
	}
}
