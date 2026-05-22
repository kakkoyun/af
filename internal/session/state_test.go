package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

const (
	testDirPerm  = 0o750
	testFilePerm = 0o600
)

func TestStateRoundTrip_WritesAtomicTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	want := sampleState()

	err := session.WriteState(path, want)
	if err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}
	_, err = os.Stat(path + ".tmp")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary state file exists after WriteState: %v", err)
	}

	got, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	assertSampleState(t, got)
}

func TestReadState_RejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml")
	writeFile(t, path, `schema_version = 2

[session]
name = "future"
`)

	_, err := session.ReadState(path)
	if err == nil {
		t.Fatal("ReadState() error = nil, want schema error")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("ReadState() error = %v, want schema_version context", err)
	}
}

func TestLedgerAppendAndLastTouchedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	first := time.Date(2026, time.May, 20, 10, 0, 0, 123_000_000, time.UTC)
	second := first.Add(2 * time.Hour)

	err := session.AppendEvent(path, session.Event{Timestamp: first, Type: "session_created", Fields: map[string]any{"branch": "kakkoyun/issue-42"}})
	if err != nil {
		t.Fatalf("AppendEvent(first) error = %v", err)
	}
	err = session.AppendEvent(path, session.Event{Timestamp: second, Type: "agent_launched", Fields: map[string]any{"slot": "primary", "agent": "pi"}})
	if err != nil {
		t.Fatalf("AppendEvent(second) error = %v", err)
	}

	got, err := session.LastTouchedAt(path)
	if err != nil {
		t.Fatalf("LastTouchedAt() error = %v", err)
	}
	if !got.Equal(second) {
		t.Fatalf("LastTouchedAt() = %s, want %s", got, second)
	}

	content := readFile(t, path)
	if !strings.HasSuffix(content, "\n") || strings.Count(content, "\n") != 2 {
		t.Fatalf("ledger content %q, want two newline-terminated JSONL records", content)
	}
}

// TestLedger_EventTypeRoundTrip asserts that AppendEvent + ReadLedgerTail
// preserve the Event.Type field. Regression test for a parser/writer key
// mismatch (writer used "event", parser only looked at "type").
func TestLedger_EventTypeRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "ledger.jsonl")
	ts := time.Date(2026, time.May, 22, 12, 0, 0, 0, time.UTC)

	err := session.AppendEvent(path, session.Event{
		Timestamp: ts,
		Type:      "agent_sessions_pulled",
		Fields:    map[string]any{"vm": "sbox-abc", "imported": 3},
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	events, err := session.ReadLedgerTail(path, 0)
	if err != nil {
		t.Fatalf("ReadLedgerTail: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	if events[0].Type != "agent_sessions_pulled" {
		t.Errorf("Type = %q, want agent_sessions_pulled", events[0].Type)
	}
	if !events[0].Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", events[0].Timestamp, ts)
	}
	if events[0].Fields["vm"] != "sbox-abc" {
		t.Errorf("Fields[vm] = %v, want sbox-abc", events[0].Fields["vm"])
	}
}

func TestRepoSlugFromRemote(t *testing.T) {
	tests := map[string]string{
		"git@github.com:kakkoyun/af.git":       "kakkoyun/af",
		"https://github.com/kakkoyun/af":       "kakkoyun/af",
		"https://github.com/kakkoyun/af.git":   "kakkoyun/af",
		"ssh://git@github.com/kakkoyun/af.git": "kakkoyun/af",
		"https://gitlab.com/kakkoyun/af.git":   "",
		"not a url":                            "",
	}

	for remote, want := range tests {
		t.Run(remote, func(t *testing.T) {
			got := session.RepoSlugFromRemote(remote)
			if got != want {
				t.Fatalf("RepoSlugFromRemote() = %q, want %q", got, want)
			}
		})
	}
}

func TestDiscoverStatePath_UsesExplicitSessionThenWorktreeSymlink(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	explicitState := filepath.Join(sessionsDir, "explicit", "state.toml")
	writeFile(t, explicitState, "schema_version = 1\n")

	got, err := session.DiscoverStatePath(session.DiscoverOptions{SessionName: "explicit", SessionsDir: sessionsDir})
	if err != nil {
		t.Fatalf("DiscoverStatePath(explicit) error = %v", err)
	}
	if got != explicitState {
		t.Fatalf("DiscoverStatePath(explicit) = %q, want %q", got, explicitState)
	}

	worktree := filepath.Join(root, "worktrees", "repo", "branch", "nested")
	state := filepath.Join(sessionsDir, "from-cwd", "state.toml")
	writeFile(t, state, "schema_version = 1\n")
	err = os.MkdirAll(filepath.Join(filepath.Dir(worktree), ".af"), testDirPerm)
	if err != nil {
		t.Fatalf("create .af dir: %v", err)
	}
	err = os.MkdirAll(worktree, testDirPerm)
	if err != nil {
		t.Fatalf("create nested worktree dir: %v", err)
	}
	link := filepath.Join(filepath.Dir(worktree), ".af", "state.toml")
	err = os.Symlink(state, link)
	if err != nil {
		t.Fatalf("create state symlink: %v", err)
	}

	got, err = session.DiscoverStatePath(session.DiscoverOptions{Cwd: worktree, SessionsDir: sessionsDir})
	if err != nil {
		t.Fatalf("DiscoverStatePath(cwd) error = %v", err)
	}
	wantState, err := filepath.EvalSymlinks(state)
	if err != nil {
		t.Fatalf("resolve expected state symlink path: %v", err)
	}
	if got != wantState {
		t.Fatalf("DiscoverStatePath(cwd) = %q, want %q", got, wantState)
	}
}

func TestLockFile_CreatesLockFileAndUnlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.toml.lock")
	lock, err := session.LockFile(path, session.LockExclusive)
	if err != nil {
		t.Fatalf("LockFile() error = %v", err)
	}
	_, err = os.Stat(path)
	if err != nil {
		t.Fatalf("lock file was not created: %v", err)
	}
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
}

// TestState_RoundTrip_PreservesControlFields verifies that the ADR-061
// additive fields (ApprovalMode, MaxAgents, RemoteControl) survive a
// WriteState → ReadState round-trip.
func TestState_RoundTrip_PreservesControlFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	created := time.Date(2026, time.May, 22, 10, 0, 0, 0, time.UTC)

	want := session.State{
		SchemaVersion: 1,
		Session: session.Info{
			Name:         "ctrl-test",
			ID:           "ctrl-uuid",
			CreatedAt:    created,
			Status:       "active",
			ApprovalMode: "yolo",
			MaxAgents:    4,
		},
		Worktree: session.WorktreeState{
			Path: "/tmp/ctrl", Branch: "ctrl-branch",
			BaseBranch: "main", GitRoot: "/tmp/repo", RepoSlug: "owner/repo",
		},
		Execution: session.ExecutionState{
			Mode: "local", Multiplexer: "tmux",
			TmuxSession:   "af-ctrl-test",
			RemoteControl: "superterm",
		},
		Versions: session.VersionsState{AF: "dev", AgentVersions: map[string]string{}},
	}

	err := session.WriteState(path, want)
	if err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}
	got, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}

	if got.Session.ApprovalMode != "yolo" {
		t.Errorf("Session.ApprovalMode = %q, want yolo", got.Session.ApprovalMode)
	}
	if got.Session.MaxAgents != 4 {
		t.Errorf("Session.MaxAgents = %d, want 4", got.Session.MaxAgents)
	}
	if got.Execution.RemoteControl != "superterm" {
		t.Errorf("Execution.RemoteControl = %q, want superterm", got.Execution.RemoteControl)
	}
}

func sampleState() session.State {
	created := time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC)
	return session.State{
		SchemaVersion: 1,
		Session: session.Info{
			Name:      "kakkoyun--issue-42",
			ID:        "session-uuid",
			CreatedAt: created,
			Status:    "active",
		},
		Worktree: session.WorktreeState{
			Path:       "/tmp/worktree",
			Branch:     "kakkoyun/issue-42",
			BaseBranch: "upstream/main",
			GitRoot:    "/tmp/repo",
			RepoSlug:   "kakkoyun/af",
		},
		Execution: session.ExecutionState{Mode: "local", Multiplexer: "tmux", TmuxSession: "kakkoyun--issue-42"},
		Agents: []session.AgentState{{
			Slot:       "primary",
			Provider:   "pi",
			SessionIDs: []string{"agent-uuid"},
			Pane:       "%0",
			Status:     "running",
			CreatedAt:  created,
		}},
		PR:       session.PRState{},
		Stack:    session.StackState{},
		Versions: session.VersionsState{AF: "dev", AgentVersions: map[string]string{"pi": "1.0.0"}},
	}
}

func assertSampleState(t *testing.T, got session.State) {
	t.Helper()
	if got.SchemaVersion != 1 || got.Session.Name != "kakkoyun--issue-42" || got.Session.Status != "active" {
		t.Fatalf("state session = %#v", got.Session)
	}
	if got.Worktree.Branch != "kakkoyun/issue-42" || got.Worktree.RepoSlug != "kakkoyun/af" {
		t.Fatalf("state worktree = %#v", got.Worktree)
	}
	if len(got.Agents) != 1 || got.Agents[0].Slot != "primary" || got.Agents[0].SessionIDs[0] != "agent-uuid" {
		t.Fatalf("state agents = %#v", got.Agents)
	}
	if got.Versions.AgentVersions["pi"] != "1.0.0" {
		t.Fatalf("state versions = %#v", got.Versions)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), testDirPerm)
	if err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	err = os.WriteFile(path, []byte(content), testFilePerm)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path) //nolint:gosec // Test helper reads files created by the test.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func TestState_RoundTrip_PreservesSlicerResources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")

	original := session.State{
		SchemaVersion: 1,
		Session:       session.Info{ID: "test-id", Name: "test"},
		Execution: session.ExecutionState{
			Mode:                       "local",
			SandboxResourceProfile:     "tight",
			SandboxResourceVCPU:        2,
			SandboxResourceRAMGB:       4,
			SandboxResourceStorageSize: "25G",
			SandboxResourceGPUCount:    1,
			SandboxResourceImage:       "ubuntu-22-04",
			SandboxResourceHypervisor:  "firecracker",
			SandboxManagedGroup:        "af-myrepo-tight",
		},
		Versions: session.VersionsState{AgentVersions: map[string]string{}},
	}
	err := session.WriteState(path, original)
	if err != nil {
		t.Fatalf("WriteState() error = %v", err)
	}
	got, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	assertSlicerResources(t, got.Execution)
}

func assertSlicerResources(t *testing.T, ex session.ExecutionState) {
	t.Helper()
	if ex.SandboxResourceProfile != "tight" {
		t.Errorf("SandboxResourceProfile = %q, want tight", ex.SandboxResourceProfile)
	}
	if ex.SandboxResourceVCPU != 2 {
		t.Errorf("SandboxResourceVCPU = %d, want 2", ex.SandboxResourceVCPU)
	}
	if ex.SandboxResourceRAMGB != 4 {
		t.Errorf("SandboxResourceRAMGB = %d, want 4", ex.SandboxResourceRAMGB)
	}
	if ex.SandboxResourceStorageSize != "25G" {
		t.Errorf("SandboxResourceStorageSize = %q, want 25G", ex.SandboxResourceStorageSize)
	}
	if ex.SandboxResourceGPUCount != 1 {
		t.Errorf("SandboxResourceGPUCount = %d, want 1", ex.SandboxResourceGPUCount)
	}
	if ex.SandboxResourceImage != "ubuntu-22-04" {
		t.Errorf("SandboxResourceImage = %q, want ubuntu-22-04", ex.SandboxResourceImage)
	}
	if ex.SandboxResourceHypervisor != "firecracker" {
		t.Errorf("SandboxResourceHypervisor = %q, want firecracker", ex.SandboxResourceHypervisor)
	}
	if ex.SandboxManagedGroup != "af-myrepo-tight" {
		t.Errorf("SandboxManagedGroup = %q, want af-myrepo-tight", ex.SandboxManagedGroup)
	}
}

func TestState_RoundTrip_PreservesSlicerWT(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	now := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	pulledAt := now.Add(time.Hour)

	in := session.State{
		SchemaVersion: 1,
		Session:       session.Info{Name: "demo", Status: "active", CreatedAt: now},
		SlicerWT: session.SlicerWTState{
			VM:         "sbox-abc123",
			Path:       "/path/to/wt",
			PushedAt:   now,
			PulledAt:   &pulledAt,
			LeaseState: session.SlicerWTLeasePulled,
		},
	}
	err := session.WriteState(path, in)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	out, err := session.ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if out.SlicerWT.VM != "sbox-abc123" {
		t.Errorf("VM = %q, want sbox-abc123", out.SlicerWT.VM)
	}
	if out.SlicerWT.Path != "/path/to/wt" {
		t.Errorf("Path = %q, want /path/to/wt", out.SlicerWT.Path)
	}
	if out.SlicerWT.LeaseState != session.SlicerWTLeasePulled {
		t.Errorf("LeaseState = %q, want pulled", out.SlicerWT.LeaseState)
	}
	if out.SlicerWT.PulledAt == nil {
		t.Fatal("PulledAt is nil")
	}
	if !out.SlicerWT.PulledAt.Equal(pulledAt) {
		t.Errorf("PulledAt = %v, want %v", out.SlicerWT.PulledAt, pulledAt)
	}
}

func TestSlicerWTLeaseState_Constants(t *testing.T) {
	t.Parallel()
	if session.SlicerWTLeaseHeldByVM != "held_by_vm" {
		t.Errorf("HeldByVM = %q", session.SlicerWTLeaseHeldByVM)
	}
	if session.SlicerWTLeasePulled != "pulled" {
		t.Errorf("Pulled = %q", session.SlicerWTLeasePulled)
	}
	if session.SlicerWTLeaseDiscarded != "discarded" {
		t.Errorf("Discarded = %q", session.SlicerWTLeaseDiscarded)
	}
}

func TestState_IsLeasedToVM(t *testing.T) {
	t.Parallel()
	cases := []struct {
		lease session.SlicerWTLeaseState
		want  bool
	}{
		{session.SlicerWTLeaseHeldByVM, true},
		{session.SlicerWTLeasePulled, false},
		{session.SlicerWTLeaseDiscarded, false},
		{"", false},
	}
	for _, tc := range cases {
		s := session.State{SlicerWT: session.SlicerWTState{LeaseState: tc.lease}}
		if got := s.IsLeasedToVM(); got != tc.want {
			t.Errorf("lease=%q: IsLeasedToVM()=%v, want %v", tc.lease, got, tc.want)
		}
	}
}
