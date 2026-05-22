package sessiondata_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
)

// errTestBoom is a sentinel used by tests asserting error propagation.
var errTestBoom = errors.New("sessiondata test: boom")

// fakeVM builds a temp directory simulating a VM $HOME and populates it
// with the given file map. Returns the absolute path to the fake $HOME.
func fakeVM(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, rel)
		err := os.MkdirAll(filepath.Dir(abs), 0o750)
		if err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
		}
		err = os.WriteFile(abs, []byte(content), 0o600)
		if err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}
	return root
}

// TestParseKindFlag asserts the --agent value parsing rules.
func TestParseKindFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want []sessiondata.AgentKind
	}{
		{"", sessiondata.AllKinds()},
		{"all", sessiondata.AllKinds()},
		{"claude", []sessiondata.AgentKind{sessiondata.KindClaude}},
		{"pi,codex", []sessiondata.AgentKind{sessiondata.KindCodex, sessiondata.KindPi}}, // AllKinds order, not input order.
		{"claude,claude,claude", []sessiondata.AgentKind{sessiondata.KindClaude}},
		{"  pi  ,  harness ", []sessiondata.AgentKind{sessiondata.KindPi, sessiondata.KindHarness}},
	}
	for _, tt := range tests {
		got, err := sessiondata.ParseKindFlag(tt.in)
		if err != nil {
			t.Errorf("ParseKindFlag(%q): unexpected error %v", tt.in, err)
			continue
		}
		if !equalKinds(got, tt.want) {
			t.Errorf("ParseKindFlag(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseKindFlag_RejectsUnknown(t *testing.T) {
	t.Parallel()
	_, err := sessiondata.ParseKindFlag("unknown,claude")
	if !errors.Is(err, sessiondata.ErrUnknownAgent) {
		t.Errorf("want ErrUnknownAgent, got %v", err)
	}
}

func TestSourceRoots(t *testing.T) {
	t.Parallel()
	tests := map[sessiondata.AgentKind][]string{
		sessiondata.KindClaude:  {".claude/projects", ".claude/sessions"},
		sessiondata.KindCodex:   {".codex/sessions"},
		sessiondata.KindPi:      {".pi/agent/sessions"},
		sessiondata.KindHarness: {".pi/agent/teams"},
	}
	for kind, want := range tests {
		got := sessiondata.SourceRoots(kind)
		if !equalStrings(got, want) {
			t.Errorf("SourceRoots(%q) = %v, want %v", kind, got, want)
		}
	}
}

func TestFetchManifest_GroupsByKind(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".claude/projects/foo/transcript.jsonl":      `{"role":"user"}`,
		".claude/sessions/foo.json":                  `{}`,
		".codex/sessions/2026/05/22/rollout-x.jsonl": `{}`,
		".pi/agent/sessions/abc.jsonl":               `{}`,
		".pi/agent/teams/teamA/state.json":           `{}`,
		"random/unallowlisted.txt":                   `nope`,
	})
	fake := &sessiondata.FakeSlicer{Source: home}

	manifest, err := sessiondata.FetchManifest(context.Background(), fake, "vm-test", sessiondata.AllKinds())
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if manifest.VM != "vm-test" {
		t.Errorf("VM = %q, want vm-test", manifest.VM)
	}
	wantCount := map[sessiondata.AgentKind]int{
		sessiondata.KindClaude:  2,
		sessiondata.KindCodex:   1,
		sessiondata.KindPi:      1,
		sessiondata.KindHarness: 1,
	}
	for kind, n := range wantCount {
		if got := len(manifest.Items[kind]); got != n {
			t.Errorf("manifest[%q] = %d files, want %d", kind, got, n)
		}
	}
	if total := manifest.Count(); total != 5 {
		t.Errorf("Count = %d, want 5", total)
	}
	// Ensure the random unallowlisted file did not leak in.
	for _, kind := range sessiondata.AllKinds() {
		for _, e := range manifest.Items[kind] {
			if strings.Contains(e.Path, "unallowlisted") {
				t.Errorf("manifest leaked unallowlisted file: %s", e.Path)
			}
		}
	}
}

func TestFetchManifest_OnlyRequestedKinds(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".claude/projects/a.jsonl": "x",
		".codex/sessions/b.jsonl":  "y",
	})
	fake := &sessiondata.FakeSlicer{Source: home}
	manifest, err := sessiondata.FetchManifest(context.Background(), fake, "vm", []sessiondata.AgentKind{sessiondata.KindClaude})
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if len(manifest.Items[sessiondata.KindClaude]) != 1 {
		t.Errorf("claude items = %d, want 1", len(manifest.Items[sessiondata.KindClaude]))
	}
	if _, has := manifest.Items[sessiondata.KindCodex]; has {
		t.Errorf("manifest must not include codex when not requested")
	}
}

func TestPull_DryRunDoesNotCopy(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/r.jsonl": "x",
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds:  []sessiondata.AgentKind{sessiondata.KindCodex},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("Pull dry-run: %v", err)
	}
	if !res.DryRun {
		t.Errorf("Result.DryRun = false, want true")
	}
	if res.StagingPath != "" {
		t.Errorf("StagingPath should be empty on dry-run; got %q", res.StagingPath)
	}
	// No staging directory must exist.
	_, statErr := os.Stat(filepath.Join(hostHome, ".local", "share", "af", "v1", "session-import"))
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("staging dir should not exist on dry-run; statErr=%v", statErr)
	}
	// No host destination must have been touched.
	_, destStatErr := os.Stat(filepath.Join(hostHome, ".codex", "sessions", "r.jsonl"))
	if !errors.Is(destStatErr, fs.ErrNotExist) {
		t.Errorf("host destination should not exist on dry-run; destStatErr=%v", destStatErr)
	}
	// Only Inventory was called.
	for _, call := range fake.Calls {
		if call.Method == "Copy" {
			t.Errorf("Copy should not be invoked on dry-run; got call %+v", call)
		}
	}
}

func TestPull_NewFileIsImported(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/2026/05/22/rollout-x.jsonl": "ROLLOUT-CONTENT",
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s1", VM: "vm1", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Merge.Imported != 1 {
		t.Errorf("Imported = %d, want 1", res.Merge.Imported)
	}
	if res.Merge.Skipped != 0 || res.Merge.Conflicts != 0 {
		t.Errorf("unexpected Skipped/Conflicts on fresh import: %+v", res.Merge)
	}
	dest := filepath.Join(hostHome, ".codex", "sessions", "2026", "05", "22", "rollout-x.jsonl")
	data, err := os.ReadFile(dest) //nolint:gosec // test reading test fixture.
	if err != nil {
		t.Fatalf("read host dest: %v", err)
	}
	if string(data) != "ROLLOUT-CONTENT" {
		t.Errorf("dest contents = %q, want ROLLOUT-CONTENT", string(data))
	}
	// Verify file mode is 0o600.
	info, statErr := os.Stat(dest)
	if statErr != nil {
		t.Fatalf("stat dest: %v", statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("dest mode = %v, want 0o600", info.Mode().Perm())
	}
}

func TestPull_IdenticalContentIsSkipped(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/dup.jsonl": "SAME-CONTENT",
	})
	hostHome := t.TempDir()
	// Pre-populate the host with identical content.
	dest := filepath.Join(hostHome, ".codex", "sessions", "dup.jsonl")
	err := os.MkdirAll(filepath.Dir(dest), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(dest, []byte("SAME-CONTENT"), 0o600)
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
		t.Fatalf("Pull: %v", err)
	}
	if res.Merge.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1; report=%+v", res.Merge.Skipped, res.Merge)
	}
	if res.Merge.Imported != 0 || res.Merge.Conflicts != 0 {
		t.Errorf("unexpected Imported/Conflicts on identical content: %+v", res.Merge)
	}
}

func TestPull_DifferentContentIsQuarantined(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/conflict.jsonl": "FROM-VM",
	})
	hostHome := t.TempDir()
	dest := filepath.Join(hostHome, ".codex", "sessions", "conflict.jsonl")
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
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Merge.Conflicts != 1 {
		t.Errorf("Conflicts = %d, want 1; report=%+v", res.Merge.Conflicts, res.Merge)
	}
	// Host destination must remain untouched (FROM-HOST).
	got, readErr := os.ReadFile(dest) //nolint:gosec // test path under hostHome.
	if readErr != nil {
		t.Fatalf("read host dest: %v", readErr)
	}
	if string(got) != "FROM-HOST" {
		t.Errorf("host dest mutated: got %q, want FROM-HOST", got)
	}
	// VM content must be in the conflicts/ subtree under StagingPath.
	want := filepath.Join(res.StagingPath, "conflicts", ".codex", "sessions", "conflict.jsonl")
	conflict, err := os.ReadFile(want) //nolint:gosec // conflict path under StagingPath.
	if err != nil {
		t.Fatalf("read quarantined %s: %v", want, err)
	}
	if string(conflict) != "FROM-VM" {
		t.Errorf("quarantined content = %q, want FROM-VM", conflict)
	}
}

func TestPull_StagingPathHasExpectedShape(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/x.jsonl": "x",
	})
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{Source: home}

	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "alpha", VM: "vmZ", HomeDir: hostHome,
		Kinds: []sessiondata.AgentKind{sessiondata.KindCodex},
		Now:   fixedTime,
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	want := filepath.Join(hostHome, ".local", "share", "af", "v1", "session-import", "alpha", "vmZ", "20260522T120000Z")
	if res.StagingPath != want {
		t.Errorf("StagingPath = %q, want %q", res.StagingPath, want)
	}
}

func TestPull_PropagatesInventoryError(t *testing.T) {
	t.Parallel()
	hostHome := t.TempDir()
	fake := &sessiondata.FakeSlicer{InventoryErr: errTestBoom}

	_, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "s", VM: "v", HomeDir: hostHome,
	})
	if !errors.Is(err, errTestBoom) {
		t.Errorf("want wrapped %v, got %v", errTestBoom, err)
	}
	if !errors.Is(err, sessiondata.ErrSyncFailed) {
		t.Errorf("want ErrSyncFailed wrap, got %v", err)
	}
}

func TestPull_RejectsEmptyArgs(t *testing.T) {
	t.Parallel()
	fake := &sessiondata.FakeSlicer{}
	cases := []sessiondata.SyncOptions{
		{Session: "", VM: "v", HomeDir: "/h"},
		{Session: "s", VM: "", HomeDir: "/h"},
		{Session: "s", VM: "v", HomeDir: ""},
	}
	for _, opts := range cases {
		_, err := sessiondata.Sync(context.Background(), fake, opts)
		if !errors.Is(err, sessiondata.ErrSyncFailed) {
			t.Errorf("Pull(%+v): want ErrSyncFailed, got %v", opts, err)
		}
	}
}

func TestList_DelegatesToFetchManifest(t *testing.T) {
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".pi/agent/sessions/abc.jsonl":     "x",
		".pi/agent/teams/teamA/state.json": "y",
	})
	fake := &sessiondata.FakeSlicer{Source: home}
	manifest, err := sessiondata.List(context.Background(), fake, "vm", nil) // nil → AllKinds.
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(manifest.Items[sessiondata.KindPi]) != 1 {
		t.Errorf("pi items = %d, want 1", len(manifest.Items[sessiondata.KindPi]))
	}
	if len(manifest.Items[sessiondata.KindHarness]) != 1 {
		t.Errorf("harness items = %d, want 1", len(manifest.Items[sessiondata.KindHarness]))
	}
}

func TestManifestSummary(t *testing.T) {
	t.Parallel()
	manifest := sessiondata.Manifest{
		Items: map[sessiondata.AgentKind][]sessiondata.FileEntry{
			sessiondata.KindClaude: {{Path: "a"}, {Path: "b"}},
			sessiondata.KindCodex:  {{Path: "c"}},
		},
	}
	got := sessiondata.ManifestSummary(manifest)
	if got != "claude=2 codex=1" {
		t.Errorf("ManifestSummary = %q, want %q", got, "claude=2 codex=1")
	}
	empty := sessiondata.ManifestSummary(sessiondata.Manifest{})
	if empty != "(empty)" {
		t.Errorf("ManifestSummary(empty) = %q, want (empty)", empty)
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
}

func equalKinds(a, b []sessiondata.AgentKind) bool {
	if len(a) != len(b) {
		return false
	}
	aa := make([]string, len(a))
	bb := make([]string, len(b))
	for i := range a {
		aa[i] = string(a[i])
		bb[i] = string(b[i])
	}
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSync_RecordsSourceCursors asserts that Sync populates per-file
// SourceRecords with kind, dest path, size, hash, mtime, and status.
// Required by ADR-067 for state.toml writeback.
func TestSync_RecordsSourceCursors(t *testing.T) { //nolint:cyclop // Test asserts per-status fields across three records; splitting hurts coverage of the per-record invariants.
	t.Parallel()
	home := fakeVM(t, map[string]string{
		".codex/sessions/2026/05/22/r-fresh.jsonl": "FRESH",
		".codex/sessions/2026/05/22/r-same.jsonl":  "SAME",
		".pi/agent/sessions/conflict.jsonl":        "VM-VERSION",
	})
	hostHome := t.TempDir()
	// Pre-populate one identical-content host file (Skipped).
	sameDest := filepath.Join(hostHome, ".codex", "sessions", "2026", "05", "22", "r-same.jsonl")
	err := os.MkdirAll(filepath.Dir(sameDest), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(sameDest, []byte("SAME"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	// Pre-populate one different-content host file (Conflict).
	confDest := filepath.Join(hostHome, ".pi", "agent", "sessions", "conflict.jsonl")
	err = os.MkdirAll(filepath.Dir(confDest), 0o700)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(confDest, []byte("HOST-VERSION"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	fake := &sessiondata.FakeSlicer{Source: home}
	res, err := sessiondata.Sync(context.Background(), fake, sessiondata.SyncOptions{
		Session: "src-test", VM: "vm1", HomeDir: hostHome,
		Now: fixedTime,
	})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// One imported, one skipped, one conflict → three source records.
	if got := len(res.Merge.Sources); got != 3 {
		t.Fatalf("len(Sources) = %d, want 3; report=%+v", got, res.Merge)
	}
	byStatus := map[sessiondata.SourceStatus]int{}
	for _, src := range res.Merge.Sources {
		byStatus[src.Status]++
		if src.Hash == "" || !strings.HasPrefix(src.Hash, "sha256:") {
			t.Errorf("Source %s missing sha256 hash; got %q", src.VMRelPath, src.Hash)
		}
		if src.Mode != "copy" {
			t.Errorf("Source %s Mode = %q, want copy", src.VMRelPath, src.Mode)
		}
		if src.Size == 0 {
			t.Errorf("Source %s Size = 0", src.VMRelPath)
		}
	}
	if byStatus[sessiondata.SourceStatusOK] != 1 || byStatus[sessiondata.SourceStatusSkipped] != 1 || byStatus[sessiondata.SourceStatusConflict] != 1 {
		t.Errorf("status histogram %v, want ok=1 skipped=1 conflict=1", byStatus)
	}
}
