package sessiondata_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
)

// TestSync_ContinueHost_RenamesAndRewritesThenMerges is the end-to-end
// Sync path for ADR-066 §Host continuation: the staged Claude project
// directory is renamed from the VM slug to the host slug and its cwd
// fields rewritten BEFORE the merge/dedup step, so the host destination
// ends up under the host-slug directory with host-referencing content.
func TestSync_ContinueHost_RenamesAndRewritesThenMerges(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".claude/projects/" + normalizeVMSlug + "/session1.jsonl": string(readTestdata(t, "claude_input.jsonl")),
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "ch1", VM: "vm1", HomeDir: hostHome,
		Kinds:        []sessiondata.AgentKind{sessiondata.KindClaude},
		ContinueHost: true,
		VMPath:       normalizeVMPath,
		HostPath:     normalizeHostPath,
		Now:          fixedTime,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Merge.Imported != 1 {
		t.Fatalf("Imported = %d, want 1; report=%+v", res.Merge.Imported, res.Merge)
	}

	dest := filepath.Join(hostHome, ".claude", "projects", normalizeHostSlug, "session1.jsonl")
	got, readErr := os.ReadFile(dest) //nolint:gosec // path under t.TempDir().
	if readErr != nil {
		t.Fatalf("read host dest %s: %v", dest, readErr)
	}
	want := readTestdata(t, "claude_want.jsonl")
	if !bytes.Equal(got, want) {
		t.Errorf("host dest content mismatch:\ngot:  %q\nwant: %q", got, want)
	}
	// The VM-slug destination must not exist on the host.
	_, statErr := os.Stat(filepath.Join(hostHome, ".claude", "projects", normalizeVMSlug))
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("VM-slug host dir should not exist; statErr=%v", statErr)
	}
	if res.Normalize.RewrittenFiles[sessiondata.KindClaude] != 1 {
		t.Errorf("Normalize.RewrittenFiles[claude] = %d, want 1", res.Normalize.RewrittenFiles[sessiondata.KindClaude])
	}
	if len(res.Normalize.RenamedDirs) != 1 {
		t.Errorf("Normalize.RenamedDirs = %v, want 1 entry", res.Normalize.RenamedDirs)
	}
}

// TestSync_ContinueHost_IdempotentSecondSyncImportsZero is the ADR-066
// idempotency requirement: normalization runs against the staging tree
// before the merge step computes content hashes, so a second sync
// against unchanged VM content — even with --continue-host — produces
// byte-identical normalized output and the merge step reports it as
// already imported rather than writing a new file or a conflict.
func TestSync_ContinueHost_IdempotentSecondSyncImportsZero(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".claude/projects/" + normalizeVMSlug + "/session1.jsonl": string(readTestdata(t, "claude_input.jsonl")),
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	// Two distinct staging timestamps, as two real `af session-data sync`
	// invocations would produce (time.Now advances between runs).
	clockCalls := 0
	clock := func() time.Time {
		clockCalls++
		return fixedTime().Add(time.Duration(clockCalls) * time.Minute)
	}
	opts := sessiondata.SyncOptions{
		Session: "ch-idem", VM: "vm1", HomeDir: hostHome,
		Kinds:        []sessiondata.AgentKind{sessiondata.KindClaude},
		ContinueHost: true,
		VMPath:       normalizeVMPath,
		HostPath:     normalizeHostPath,
		Now:          clock,
	}

	first, err := sessiondata.Sync(context.Background(), fake, opts)
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if first.Merge.Imported != 1 || first.Merge.Conflicts != 0 {
		t.Fatalf("first sync: Imported=%d Conflicts=%d, want 1/0; report=%+v", first.Merge.Imported, first.Merge.Conflicts, first.Merge)
	}

	// Second sync: the VM's fake $HOME is unchanged; staging re-runs
	// under a fresh timestamped directory, normalizes identically, and
	// the merge step must see the SHA-256 match and skip.
	second, err := sessiondata.Sync(context.Background(), fake, opts)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if second.Merge.Imported != 0 {
		t.Errorf("second sync Imported = %d, want 0", second.Merge.Imported)
	}
	if second.Merge.Skipped != 1 {
		t.Errorf("second sync Skipped = %d, want 1", second.Merge.Skipped)
	}
	if second.Merge.Conflicts != 0 {
		t.Errorf("second sync Conflicts = %d, want 0", second.Merge.Conflicts)
	}
}

// TestSync_ContinueHost_MissingPathsFailFast asserts Sync's fail-fast
// validation for host continuation: a live (non-dry-run) --continue-host
// sync with an empty VMPath or HostPath would silently skip
// normalization (NormalizeForHost's documented no-op guard), leaving
// transcripts unusable for host resume — so Sync must refuse instead.
// Dry-run stays exempt: it only reports manifest candidate counts and
// never uses the paths.
func TestSync_ContinueHost_MissingPathsFailFast(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, vmPath, hostPath string
	}{
		{"empty-vm-path", "", normalizeHostPath},
		{"empty-host-path", normalizeVMPath, ""},
		{"both-empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &sessiondata.FakeSlicer{Source: t.TempDir()}
			_, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
				Session: "ch-nopath", VM: "vm1", HomeDir: t.TempDir(),
				ContinueHost: true,
				VMPath:       tc.vmPath,
				HostPath:     tc.hostPath,
				Now:          fixedTime,
			})
			if !errors.Is(err, sessiondata.ErrSyncFailed) {
				t.Fatalf("err = %v, want ErrSyncFailed", err)
			}
			if len(fake.Calls) != 0 {
				t.Errorf("Sync must fail before touching the VM; got calls %+v", fake.Calls)
			}
		})
	}

	t.Run("dry-run-allows-empty-paths", func(t *testing.T) {
		t.Parallel()
		home := fakeVM(t, map[string]string{
			".claude/projects/" + normalizeVMSlug + "/session1.jsonl": string(readTestdata(t, "claude_input.jsonl")),
		})
		fake := &sessiondata.FakeSlicer{Source: home}
		res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
			Session: "ch-nopath-dry", VM: "vm1", HomeDir: t.TempDir(),
			DryRun:       true,
			ContinueHost: true,
		})
		if err != nil {
			t.Fatalf("dry-run Sync: %v", err)
		}
		if res.NormalizePreview.CandidateFiles[sessiondata.KindClaude] != 1 {
			t.Errorf("CandidateFiles[claude] = %d, want 1", res.NormalizePreview.CandidateFiles[sessiondata.KindClaude])
		}
	})
}

// TestSync_DryRunContinueHost_ReportsCandidatesWithoutCopying asserts
// that `--dry-run --continue-host` reports per-kind candidate counts
// from the manifest (ADR-066: dry-run "prints ... without copying") —
// no Slicer.Copy call is made, no staging directory is created, and no
// host file is touched.
func TestSync_DryRunContinueHost_ReportsCandidatesWithoutCopying(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".claude/projects/" + normalizeVMSlug + "/session1.jsonl": string(readTestdata(t, "claude_input.jsonl")),
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "ch-dry", VM: "vm1", HomeDir: hostHome,
		Kinds:        []sessiondata.AgentKind{sessiondata.KindClaude},
		DryRun:       true,
		ContinueHost: true,
		VMPath:       normalizeVMPath,
		HostPath:     normalizeHostPath,
	})
	if err != nil {
		t.Fatalf("Sync dry-run: %v", err)
	}
	if !res.DryRun {
		t.Fatal("DryRun = false, want true")
	}
	if res.NormalizePreview.CandidateFiles[sessiondata.KindClaude] != 1 {
		t.Errorf("NormalizePreview.CandidateFiles[claude] = %d, want 1", res.NormalizePreview.CandidateFiles[sessiondata.KindClaude])
	}
	for _, call := range fake.Calls {
		if call.Method == "Copy" {
			t.Errorf("Copy must not be invoked on --dry-run --continue-host; got call %+v", call)
		}
	}
	_, statErr := os.Stat(filepath.Join(hostHome, ".local", "share", "af", "v1", "session-import"))
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("staging dir should not exist on dry-run; statErr=%v", statErr)
	}
}
