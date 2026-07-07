package sessiondata_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
)

// stubSlicer returns scripted inventory entries; Copy is a no-op. Used
// to exercise manifest bucketing with entries FakeSlicer cannot produce
// (exact-root paths, near-miss prefixes).
type stubSlicer struct {
	entries []sessiondata.FileEntry
}

func (s *stubSlicer) Inventory(_ context.Context, _ string, _ []string) ([]sessiondata.FileEntry, error) {
	return s.entries, nil
}

func (*stubSlicer) Copy(_ context.Context, _, _, _ string) error { return nil }

func TestAllKinds_StableOrder(t *testing.T) {
	t.Parallel()
	want := []sessiondata.AgentKind{
		sessiondata.KindClaude,
		sessiondata.KindCodex,
		sessiondata.KindPi,
		sessiondata.KindHarness,
	}
	got := sessiondata.AllKinds()
	if len(got) != len(want) {
		t.Fatalf("AllKinds() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllKinds()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSourceRoots_UnknownKindIsNil(t *testing.T) {
	t.Parallel()
	if got := sessiondata.SourceRoots("nope"); got != nil {
		t.Errorf("SourceRoots(unknown) = %v, want nil", got)
	}
}

func TestHostDestination(t *testing.T) {
	t.Parallel()
	tests := map[sessiondata.AgentKind]string{
		sessiondata.KindClaude:  ".claude",
		sessiondata.KindCodex:   ".codex",
		sessiondata.KindPi:      ".pi/agent",
		sessiondata.KindHarness: ".pi/agent",
		"nope":                  "",
	}
	for kind, want := range tests {
		if got := sessiondata.HostDestination(kind); got != want {
			t.Errorf("HostDestination(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestSourceRootRelToHostHome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind   sessiondata.AgentKind
		vmRoot string
		want   string
	}{
		{sessiondata.KindClaude, ".claude/projects", ".claude/projects"},
		{sessiondata.KindClaude, ".claude/sessions", ".claude/sessions"},
		{sessiondata.KindClaude, ".codex/sessions", ""},
		{"nope", ".claude/projects", ""},
	}
	for _, tt := range tests {
		got := sessiondata.SourceRootRelToHostHome(tt.kind, tt.vmRoot)
		if got != tt.want {
			t.Errorf("SourceRootRelToHostHome(%q, %q) = %q, want %q", tt.kind, tt.vmRoot, got, tt.want)
		}
	}
}

func TestSortedRoots(t *testing.T) {
	t.Parallel()
	want := []string{
		".claude/projects",
		".claude/sessions",
		".codex/sessions",
		".pi/agent/sessions",
		".pi/agent/teams",
	}
	got := sessiondata.SortedRoots()
	if !equalStrings(got, want) {
		t.Errorf("SortedRoots() = %v, want %v", got, want)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("SortedRoots() is not sorted: %v", got)
	}
}

func TestParseKindFlag_EmptyListAfterSplit(t *testing.T) {
	t.Parallel()
	_, err := sessiondata.ParseKindFlag(" , , ")
	if !errors.Is(err, sessiondata.ErrUnknownAgent) {
		t.Fatalf("ParseKindFlag(blank list) error = %v, want ErrUnknownAgent", err)
	}
}

func TestFetchManifest_EmptyVMErrors(t *testing.T) {
	t.Parallel()
	fake := &sessiondata.FakeSlicer{Source: t.TempDir()}
	_, err := sessiondata.FetchManifest(context.Background(), fake, "", sessiondata.AllKinds())
	if !errors.Is(err, sessiondata.ErrInventoryFailed) {
		t.Fatalf("FetchManifest(empty vm) error = %v, want ErrInventoryFailed", err)
	}
}

func TestFetchManifest_NoKindsReturnsEmptyManifest(t *testing.T) {
	t.Parallel()
	fake := &sessiondata.FakeSlicer{Source: t.TempDir()}
	manifest, err := sessiondata.FetchManifest(context.Background(), fake, "vm1", nil)
	if err != nil {
		t.Fatalf("FetchManifest(no kinds) error = %v", err)
	}
	if manifest.VM != "vm1" {
		t.Errorf("VM = %q, want vm1", manifest.VM)
	}
	if manifest.Count() != 0 {
		t.Errorf("Count = %d, want 0", manifest.Count())
	}
	if kinds := manifest.NonEmptyKinds(); len(kinds) != 0 {
		t.Errorf("NonEmptyKinds = %v, want empty", kinds)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("slicer invoked %d times, want 0", len(fake.Calls))
	}
}

func TestFetchManifest_BucketsSortsAndDropsNearMisses(t *testing.T) {
	t.Parallel()
	stub := &stubSlicer{entries: []sessiondata.FileEntry{
		{Path: ".codex/sessions/z.jsonl", Size: 1},
		{Path: ".codex/sessionsX/nope.jsonl", Size: 1}, // prefix without separator must not match.
		{Path: "unrelated/file.txt", Size: 1},
		{Path: ".codex/sessions", Size: 1}, // exact root match.
		{Path: ".codex/sessions/a.jsonl", Size: 1},
	}}

	// Duplicate kinds exercise the union-roots dedup path.
	manifest, err := sessiondata.FetchManifest(context.Background(), stub, "vm1", []sessiondata.AgentKind{sessiondata.KindCodex, sessiondata.KindCodex})
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	got := manifest.Items[sessiondata.KindCodex]
	wantPaths := []string{".codex/sessions", ".codex/sessions/a.jsonl", ".codex/sessions/z.jsonl"}
	if len(got) != len(wantPaths) {
		t.Fatalf("codex items = %+v, want paths %v", got, wantPaths)
	}
	for i, want := range wantPaths {
		if got[i].Path != want {
			t.Errorf("items[%d].Path = %q, want %q (sorted)", i, got[i].Path, want)
		}
	}
}

func TestSync_CopyErrorFailsSync(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/r.jsonl": "x",
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home, CopyErr: errTestBoom}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:   fixedTime,
	})
	if !errors.Is(err, sessiondata.ErrSyncFailed) {
		t.Fatalf("Sync error = %v, want ErrSyncFailed", err)
	}
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("Sync error = %v, want wrapped copy error", err)
	}
	if res.StagingPath == "" {
		t.Error("StagingPath empty; want partial result carrying the staging dir")
	}
}

func TestSync_StagingRootOverride(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/x.jsonl": "x",
	})
	hostHome := t.TempDir()
	stagingRoot := filepath.Join(t.TempDir(), "stage")
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		StagingRoot: stagingRoot,
		Kinds:       []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:         fixedTime,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	want := filepath.Join(stagingRoot, "s1", "vm1", "20260522T120000Z")
	if res.StagingPath != want {
		t.Errorf("StagingPath = %q, want %q", res.StagingPath, want)
	}
	_, statErr := os.Stat(filepath.Join(hostHome, ".local", "share", "af", "v1", "session-import"))
	if statErr == nil {
		t.Error("default staging dir created despite StagingRoot override")
	}
}

func TestSync_MakeStagingFailure(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/x.jsonl": "x",
	})
	blocker := filepath.Join(t.TempDir(), "blocker")
	err := os.WriteFile(blocker, []byte("not a directory\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	fake := &sessiondata.FakeSlicer{Source: home}

	_, err = sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: t.TempDir(),
		StagingRoot: filepath.Join(blocker, "nested"),
		Kinds:       []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:         fixedTime,
	})
	if !errors.Is(err, sessiondata.ErrSyncFailed) {
		t.Fatalf("Sync error = %v, want ErrSyncFailed", err)
	}
	if !strings.Contains(err.Error(), "mkdir staging") {
		t.Fatalf("Sync error = %v, want mkdir staging context", err)
	}
}

func TestSync_DefaultClock(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/x.jsonl": "x",
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
	})
	if err != nil {
		t.Fatalf("Sync with default clock: %v", err)
	}
	if res.Merge.Imported != 1 {
		t.Errorf("Imported = %d, want 1", res.Merge.Imported)
	}
}

func TestSync_DestIsDirectoryQuarantines(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/blocked.jsonl": "FROM-VM",
	})
	hostHome := t.TempDir()
	// Pre-create the host destination as a directory: content cannot be
	// compared, so the file must be quarantined.
	err := os.MkdirAll(filepath.Join(hostHome, ".codex", "sessions", "blocked.jsonl"), 0o700)
	if err != nil {
		t.Fatal(err)
	}

	fake := &sessiondata.FakeSlicer{Source: home}
	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Merge.Conflicts != 1 {
		t.Fatalf("Conflicts = %d, want 1; report=%+v", res.Merge.Conflicts, res.Merge)
	}
	quarantined := filepath.Join(res.StagingPath, "conflicts", ".codex", "sessions", "blocked.jsonl")
	got, readErr := os.ReadFile(quarantined) //nolint:gosec // test path under StagingPath.
	if readErr != nil {
		t.Fatalf("read quarantined file: %v", readErr)
	}
	if string(got) != "FROM-VM" {
		t.Errorf("quarantined content = %q, want FROM-VM", got)
	}
}

func TestSync_NonJSONLConflictQuarantines(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".claude/sessions/meta.json": "FROM-VM",
	})
	hostHome := t.TempDir()
	dest := filepath.Join(hostHome, ".claude", "sessions", "meta.json")
	err := os.MkdirAll(filepath.Dir(dest), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(dest, []byte("FROM-HOST"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	fake := &sessiondata.FakeSlicer{Source: home}
	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindClaude},
		Now:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// Non-JSONL files never take the append path: straight to quarantine.
	if res.Merge.Conflicts != 1 || res.Merge.Imported != 0 {
		t.Errorf("report = %+v, want exactly one conflict", res.Merge)
	}
	got, _ := os.ReadFile(dest) //nolint:gosec,errcheck // test inspection.
	if string(got) != "FROM-HOST" {
		t.Errorf("host dest mutated: %q", got)
	}
}

func TestSync_ConflictPathsAreSorted(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/b.jsonl": "VM-B",
		".codex/sessions/a.jsonl": "VM-A",
	})
	hostHome := t.TempDir()
	for name, content := range map[string]string{"a.jsonl": "HOST-A", "b.jsonl": "HOST-B"} {
		dest := filepath.Join(hostHome, ".codex", "sessions", name)
		err := os.MkdirAll(filepath.Dir(dest), 0o700)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(dest, []byte(content), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	fake := &sessiondata.FakeSlicer{Source: home}
	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.Merge.ConflictPaths) != 2 {
		t.Fatalf("ConflictPaths = %v, want 2 entries", res.Merge.ConflictPaths)
	}
	if !sort.StringsAreSorted(res.Merge.ConflictPaths) {
		t.Errorf("ConflictPaths not sorted: %v", res.Merge.ConflictPaths)
	}
}

func TestFakeSlicer_CopyMissingSourceIsNoop(t *testing.T) {
	t.Parallel()
	fake := &sessiondata.FakeSlicer{Source: t.TempDir()}
	out := filepath.Join(t.TempDir(), "out")

	err := fake.Copy(context.Background(), "vm1", ".claude/projects", out)
	if err != nil {
		t.Fatalf("Copy(missing source) error = %v, want nil", err)
	}
	_, statErr := os.Stat(out)
	if statErr == nil {
		t.Error("Copy(missing source) created the destination; want no-op")
	}
}

func TestFakeSlicer_CopySingleFile(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/x.jsonl": "SINGLE",
	})
	fake := &sessiondata.FakeSlicer{Source: home}
	out := filepath.Join(t.TempDir(), "nested", "x.jsonl")

	err := fake.Copy(context.Background(), "vm1", ".codex/sessions/x.jsonl", out)
	if err != nil {
		t.Fatalf("Copy(single file): %v", err)
	}
	got, err := os.ReadFile(out) //nolint:gosec // test path.
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(got) != "SINGLE" {
		t.Errorf("copied content = %q, want SINGLE", got)
	}
}

func TestFakeSlicer_CopyBlockedDestinationFails(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/x.jsonl": "x",
	})
	blocker := filepath.Join(t.TempDir(), "blocker")
	err := os.WriteFile(blocker, []byte("not a directory\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	fake := &sessiondata.FakeSlicer{Source: home}

	err = fake.Copy(context.Background(), "vm1", ".codex/sessions", filepath.Join(blocker, "sub"))
	if err == nil {
		t.Fatal("Copy into blocked destination error = nil, want mkdir failure")
	}
}

func TestManifest_CountAndNonEmptyKinds(t *testing.T) {
	t.Parallel()
	manifest := sessiondata.Manifest{
		Items: map[sessiondata.AgentKind][]sessiondata.FileEntry{
			sessiondata.KindClaude: {},
			sessiondata.KindPi:     {{Path: "x"}},
		},
	}
	if got := manifest.Count(); got != 1 {
		t.Errorf("Count = %d, want 1", got)
	}
	kinds := manifest.NonEmptyKinds()
	if len(kinds) != 1 || kinds[0] != sessiondata.KindPi {
		t.Errorf("NonEmptyKinds = %v, want [pi]", kinds)
	}
}

func TestSync_DestParentNotDirectoryFailsMerge(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/x.jsonl": "x",
	})
	hostHome := t.TempDir()
	// A regular file where the .codex directory should be makes the
	// destination stat fail with ENOTDIR (not fs.ErrNotExist).
	err := os.WriteFile(filepath.Join(hostHome, ".codex"), []byte("not a directory\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	fake := &sessiondata.FakeSlicer{Source: home}
	_, err = sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:   fixedTime,
	})
	if !errors.Is(err, sessiondata.ErrSyncFailed) {
		t.Fatalf("Sync error = %v, want ErrSyncFailed", err)
	}
	if !strings.Contains(err.Error(), "stat dest") {
		t.Fatalf("Sync error = %v, want stat dest context", err)
	}
}

func TestSync_StagedRootThatIsAFileIsSkipped(t *testing.T) {
	t.Parallel()
	// .claude/sessions is a regular file inside the VM: Inventory skips
	// it (not a directory) and the merge step must skip the staged copy
	// rather than walking it.
	home := fakeVM(t, map[string]string{
		".claude/projects/p.jsonl": "PROJECT",
		".claude/sessions":         "not a directory",
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindClaude},
		Now:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.Merge.Imported != 1 || res.Merge.Conflicts != 0 {
		t.Errorf("report = %+v, want exactly one import", res.Merge)
	}
	if len(res.Merge.Sources) != 1 || res.Merge.Sources[0].VMRelPath != ".claude/projects/p.jsonl" {
		t.Errorf("Sources = %+v, want only the projects file", res.Merge.Sources)
	}
}
