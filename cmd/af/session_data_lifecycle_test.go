package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
	"github.com/kakkoyun/af/internal/testutil"
)

// TestSuspend_AutoSyncRunsForSlicerBacked asserts that suspending a
// slicer-backed workstream first runs the ADR-067 automatic sync, then
// writes back the session_export state, before lifecycle teardown.
func TestSuspend_AutoSyncRunsForSlicerBacked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "ls-auto", "sbox-auto")
	// Clear the lease so suspend doesn't block on ADR-065 (this test
	// is about ADR-067 only).
	clearLease(t, home, "ls-auto")

	// Fake VM has one transcript file that should be imported.
	vmHome := t.TempDir()
	err := os.MkdirAll(filepath.Join(vmHome, ".pi", "agent", "sessions"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vmHome, ".pi", "agent", "sessions", "auto.jsonl"), []byte("LINE\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	_, stderr, err := executeCommand(t, newRootCmd(), "suspend", "ls-auto")
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if !strings.Contains(stderr, "auto-synced before teardown") {
		t.Errorf("stderr should report auto-sync; got: %s", stderr)
	}
	dest := filepath.Join(home, ".pi", "agent", "sessions", "auto.jsonl")
	_, statErr := os.Stat(dest)
	if errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("host destination should have been written by auto-sync")
	}
}

// TestSuspend_AutoSyncBlocksTeardownOnConflict asserts that a sync
// producing conflicts blocks suspend per ADR-067 §Lifecycle rule. The
// user is expected to re-run with --discard.
func TestSuspend_AutoSyncBlocksTeardownOnConflict(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "ls-conf", "sbox-conf")
	clearLease(t, home, "ls-conf")

	// Pre-populate host with a divergent file.
	hostDest := filepath.Join(home, ".pi", "agent", "sessions", "conflict.jsonl")
	err := os.MkdirAll(filepath.Dir(hostDest), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(hostDest, []byte("HOST"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// VM has different content (not a prefix).
	vmHome := t.TempDir()
	err = os.MkdirAll(filepath.Join(vmHome, ".pi", "agent", "sessions"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vmHome, ".pi", "agent", "sessions", "conflict.jsonl"), []byte("VM-DIFFERENT"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	_, stderr, err := executeCommand(t, newRootCmd(), "suspend", "ls-conf")
	if !errors.Is(err, errSessionDataAutoSyncFailed) {
		t.Fatalf("want errSessionDataAutoSyncFailed, got %v", err)
	}
	if !strings.Contains(stderr, "--discard") {
		t.Errorf("stderr should mention --discard recovery hint; got: %s", stderr)
	}
}

// TestSuspend_DiscardSkipsAutoSync asserts that --discard bypasses the
// ADR-067 sync entirely and records last_sync_status=discarded.
func TestSuspend_DiscardSkipsAutoSync(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "ls-disc", "sbox-disc")
	clearLease(t, home, "ls-disc")

	// Stub slicer: if anything called it, the test would fail because
	// the fake records calls.
	called := 0
	orig := sessiondataSlicerFactory
	t.Cleanup(func() { sessiondataSlicerFactory = orig })
	sessiondataSlicerFactory = func() sessiondata.Slicer {
		called++
		return &sessiondata.FakeSlicer{Source: t.TempDir()}
	}

	_, _, err := executeCommand(t, newRootCmd(), "suspend", "ls-disc", "--discard")
	if err != nil {
		t.Fatalf("suspend --discard: %v", err)
	}
	if called != 0 {
		t.Errorf("slicer factory should not be invoked when --discard is set; called=%d", called)
	}
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "ls-disc", "state.toml")
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.SessionExport.LastSyncStatus != session.ExportSyncDiscarded {
		t.Errorf("LastSyncStatus = %q, want discarded", state.SessionExport.LastSyncStatus)
	}
}

// TestSuspend_AutoSyncSkipsForNonSlicer asserts that workstreams with
// no slicer VM bypass the auto-sync entirely.
func TestSuspend_AutoSyncSkipsForNonSlicer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeNonSlicerState(t, home, "ls-local")

	called := 0
	orig := sessiondataSlicerFactory
	t.Cleanup(func() { sessiondataSlicerFactory = orig })
	sessiondataSlicerFactory = func() sessiondata.Slicer {
		called++
		return &sessiondata.FakeSlicer{Source: t.TempDir()}
	}

	_, _, err := executeCommand(t, newRootCmd(), "suspend", "ls-local")
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if called != 0 {
		t.Errorf("slicer factory should not be invoked when no VM; called=%d", called)
	}
}

// clearLease zeroes the lease state on an existing slicer-backed
// workstream fixture so the auto-sync hook is reachable without ADR-065
// refusal.
func clearLease(t *testing.T, home, name string) {
	t.Helper()
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name, "state.toml")
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	state.SlicerWT.LeaseState = ""
	err = session.WriteState(statePath, state)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
}

// Silence unused-import warnings if/when imports change.
var _ = context.Background

// TestDone_ArchiveContainsNoLockFile verifies ADR-068 §4: the flock
// file must not travel into the archive with the session directory.
func TestDone_ArchiveContainsNoLockFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, "bin")
	for _, name := range []string{"git", "tmux"} {
		testutil.WriteExecutable(t, bin, name, "exit 0")
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	writeTestSessionState(t, home, "locked-arch", "feat/locked-arch", "active")

	_, _, err := executeCommand(t, newRootCmd(), "done", "locked-arch")
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	archived := filepath.Join(home, ".local", "share", "af", "v1", "archive", "locked-arch")
	_, statErr := os.Stat(filepath.Join(archived, "state.toml"))
	if statErr != nil {
		t.Fatalf("archived state.toml missing: %v", statErr)
	}
	_, lockStatErr := os.Stat(filepath.Join(archived, session.LockFileName))
	if !errors.Is(lockStatErr, os.ErrNotExist) {
		t.Fatalf("archive contains stray %s: stat err = %v", session.LockFileName, lockStatErr)
	}
}
