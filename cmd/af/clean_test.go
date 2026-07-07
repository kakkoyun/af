package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
)

func TestClean_DryRunListsCompletedWorkstreams(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "done-work", "feat/done", "completed")
	writeTestSessionState(t, home, "active-work", "feat/active", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "clean", "--dry-run")
	if err != nil {
		t.Fatalf("clean --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "done-work") {
		t.Fatalf("stdout missing 'done-work'; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "would remove") {
		t.Fatalf("stdout missing 'would remove'; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "active-work") {
		t.Fatalf("stdout must not contain active workstream 'active-work'; got:\n%s", stdout)
	}
}

func TestClean_RemovesCompletedSessionDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "done-work", "feat/done", "completed")

	sessionDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "done-work")

	_, statErr := os.Stat(sessionDir)
	if statErr != nil {
		t.Fatalf("expected session dir to exist before clean: %v", statErr)
	}

	_, _, err := executeCommand(t, newRootCmd(), "clean")
	if err != nil {
		t.Fatalf("clean: %v", err)
	}

	_, statErr = os.Stat(sessionDir)
	if statErr == nil {
		t.Fatal("expected session dir to be removed after clean, but it still exists")
	}
}

// TestClean_NonVMTargetOutputFormatUnchanged is a regression test for
// ADR-067: a workstream with no slicer VM must produce the exact same
// "removed <name>" output as before automatic sync was wired into clean.
func TestClean_NonVMTargetOutputFormatUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "plain-work", "feat/plain-work", "completed")

	stdout, _, err := executeCommand(t, newRootCmd(), "clean")
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if stdout != "removed plain-work\n" {
		t.Errorf("stdout = %q, want %q", stdout, "removed plain-work\n")
	}
}

// TestClean_LeasedTargetSyncsBeforeRemoval asserts that a VM-leased
// workstream runs the ADR-067 automatic sync before its session dir is
// removed. --force is required here because the fixture is "active",
// not "completed" -- exercising the realistic ADR-067 scenario of
// forcibly reaping a workstream that still holds a slicer VM lease.
func TestClean_LeasedTargetSyncsBeforeRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "leased-work", "sbox-clean-1")

	vmHome := t.TempDir()
	err := os.MkdirAll(filepath.Join(vmHome, ".pi", "agent", "sessions"), 0o750)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(vmHome, ".pi", "agent", "sessions", "leased.jsonl"), []byte("LINE\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	withSessionDataSlicer(t, &sessiondata.FakeSlicer{Source: vmHome})

	stdout, stderr, err := executeCommand(t, newRootCmd(), "clean", "--force")
	if err != nil {
		t.Fatalf("clean --force: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "removed leased-work") {
		t.Errorf("stdout missing 'removed leased-work'; got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "auto-synced before teardown") {
		t.Errorf("stderr should report auto-sync; got: %s", stderr)
	}
	dest := filepath.Join(home, ".pi", "agent", "sessions", "leased.jsonl")
	_, statErr := os.Stat(dest)
	if statErr != nil {
		t.Errorf("host destination should exist after sync: %v", statErr)
	}
	sessionDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "leased-work")
	_, statErr = os.Stat(sessionDir)
	if !os.IsNotExist(statErr) {
		t.Errorf("session dir should be removed after clean; statErr=%v", statErr)
	}
}

// TestClean_DiscardSkipsSyncBeforeRemoval asserts that --discard bypasses
// the ADR-067 sync entirely (the slicer factory is never invoked) while
// still removing the target.
func TestClean_DiscardSkipsSyncBeforeRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "discard-work", "sbox-clean-2")

	called := 0
	orig := sessiondataSlicerFactory
	t.Cleanup(func() { sessiondataSlicerFactory = orig })
	sessiondataSlicerFactory = func() sessiondata.Slicer {
		called++
		return &sessiondata.FakeSlicer{Source: t.TempDir()}
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "clean", "--force", "--discard")
	if err != nil {
		t.Fatalf("clean --force --discard: %v", err)
	}
	if called != 0 {
		t.Errorf("slicer factory should not be invoked with --discard; called=%d", called)
	}
	if !strings.Contains(stdout, "removed discard-work") {
		t.Errorf("stdout missing 'removed discard-work'; got:\n%s", stdout)
	}
}

// TestClean_SyncFailureSkipsTargetButOthersStillRemoved asserts that a
// sync failure on one VM-leased target blocks only that target's
// removal (recovery hint printed, non-zero exit) while an unrelated,
// non-leased target in the same run is still reaped.
func TestClean_SyncFailureSkipsTargetButOthersStillRemoved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "bad-leased", "sbox-clean-3")
	writeTestSessionState(t, home, "good-plain", "feat/good", "completed")

	// Pre-populate a divergent host file so the VM copy is not a prefix,
	// forcing a merge conflict per ADR-067 §Latest-sync merge rules.
	hostDest := filepath.Join(home, ".pi", "agent", "sessions", "conflict.jsonl")
	err := os.MkdirAll(filepath.Dir(hostDest), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(hostDest, []byte("HOST"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
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

	stdout, stderr, err := executeCommand(t, newRootCmd(), "clean", "--force")
	if !errors.Is(err, errCleanSyncFailed) {
		t.Fatalf("want errCleanSyncFailed, got %v", err)
	}
	assertSyncFailureOutcome(t, home, stdout, stderr)
}

// assertSyncFailureOutcome checks the shared post-conditions of
// TestClean_SyncFailureSkipsTargetButOthersStillRemoved, split out to
// keep the test function's cyclomatic complexity within lint limits.
func assertSyncFailureOutcome(t *testing.T, home, stdout, stderr string) {
	t.Helper()
	if !strings.Contains(stderr, "--discard") {
		t.Errorf("stderr should mention --discard recovery hint; got: %s", stderr)
	}
	if !strings.Contains(stdout, "removed good-plain") {
		t.Errorf("stdout should still report removal of good-plain; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "removed bad-leased") {
		t.Errorf("stdout must not report bad-leased as removed; got:\n%s", stdout)
	}
	badDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "bad-leased")
	_, statErr := os.Stat(badDir)
	if statErr != nil {
		t.Errorf("bad-leased session dir should remain after failed sync: %v", statErr)
	}
	goodDir := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "good-plain")
	_, statErr = os.Stat(goodDir)
	if !os.IsNotExist(statErr) {
		t.Errorf("good-plain session dir should be removed; statErr=%v", statErr)
	}
}

// TestClean_DryRunShowsSyncWordingForLeasedTarget asserts that --dry-run
// distinguishes VM-leased targets ("would sync + remove") from plain
// targets ("would remove") and never touches the slicer.
func TestClean_DryRunShowsSyncWordingForLeasedTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSlicerBackedState(t, home, "leased-dry", "sbox-clean-4")
	writeTestSessionState(t, home, "plain-dry", "feat/plain", "completed")

	called := 0
	orig := sessiondataSlicerFactory
	t.Cleanup(func() { sessiondataSlicerFactory = orig })
	sessiondataSlicerFactory = func() sessiondata.Slicer {
		called++
		return &sessiondata.FakeSlicer{Source: t.TempDir()}
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "clean", "--dry-run", "--force")
	if err != nil {
		t.Fatalf("clean --dry-run --force: %v", err)
	}
	if !strings.Contains(stdout, "would sync + remove leased-dry") {
		t.Errorf("stdout missing sync wording for leased target; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "would remove plain-dry") {
		t.Errorf("stdout missing plain wording for non-leased target; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "would sync + remove plain-dry") {
		t.Errorf("plain-dry must not get sync wording; got:\n%s", stdout)
	}
	if called != 0 {
		t.Errorf("dry-run must not invoke the slicer; called=%d", called)
	}
}
