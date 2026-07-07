package sessiondata_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
)

const (
	normalizeVMPath   = "/root/workspace/proj"
	normalizeHostPath = "/home/user/proj"
	// normalizeVMSlug and normalizeHostSlug are the Claude Code
	// project-directory slugs for normalizeVMPath / normalizeHostPath
	// ("/" and "." replaced by "-").
	normalizeVMSlug   = "-root-workspace-proj"
	normalizeHostSlug = "-home-user-proj"
)

// readTestdata reads a fixture file under testdata/normalize/.
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "normalize", name)) //nolint:gosec // fixed test fixture path.
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// writeStaged writes content at stagingRoot/relPath, creating parent
// directories as needed.
func writeStaged(t *testing.T, stagingRoot, relPath string, content []byte) {
	t.Helper()
	abs := filepath.Join(stagingRoot, relPath)
	err := os.MkdirAll(filepath.Dir(abs), 0o700)
	if err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
	}
	err = os.WriteFile(abs, content, 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", abs, err)
	}
}

// TestNormalizeForHost_Claude_RenamesProjectDirAndRewritesContent
// asserts the Claude Code rewriter: the staged VM-slug project
// directory is renamed to the host slug, and cwd fields inside its
// *.jsonl files are rewritten from vmPath to hostPath byte-for-byte,
// while unrelated fields, records with no path fields, and non-JSON
// lines round-trip untouched.
func TestNormalizeForHost_Claude_RenamesProjectDirAndRewritesContent(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	relIn := filepath.Join(".claude", "projects", normalizeVMSlug, "session1.jsonl")
	writeStaged(t, staging, relIn, readTestdata(t, "claude_input.jsonl"))

	result, err := sessiondata.NormalizeForHost(staging, []sessiondata.AgentKind{sessiondata.KindClaude}, normalizeVMPath, normalizeHostPath)
	if err != nil {
		t.Fatalf("NormalizeForHost: %v", err)
	}

	// The VM-slug directory must be gone.
	_, vmDirStatErr := os.Stat(filepath.Join(staging, ".claude", "projects", normalizeVMSlug))
	if !os.IsNotExist(vmDirStatErr) {
		t.Errorf("VM-slug dir should no longer exist; statErr=%v", vmDirStatErr)
	}

	relOut := filepath.Join(".claude", "projects", normalizeHostSlug, "session1.jsonl")
	got, err := os.ReadFile(filepath.Join(staging, relOut)) //nolint:gosec // path under t.TempDir().
	if err != nil {
		t.Fatalf("read renamed file: %v", err)
	}
	want := readTestdata(t, "claude_want.jsonl")
	if !bytes.Equal(got, want) {
		t.Errorf("content mismatch:\ngot:  %q\nwant: %q", got, want)
	}

	if result.RewrittenFiles[sessiondata.KindClaude] != 1 {
		t.Errorf("RewrittenFiles[claude] = %d, want 1", result.RewrittenFiles[sessiondata.KindClaude])
	}
	if len(result.RewrittenPaths) != 1 || result.RewrittenPaths[0] != filepath.ToSlash(relOut) {
		t.Errorf("RewrittenPaths = %v, want [%s]", result.RewrittenPaths, filepath.ToSlash(relOut))
	}
	wantRename := filepath.ToSlash(filepath.Join(".claude", "projects", normalizeVMSlug)) + " -> " + filepath.ToSlash(filepath.Join(".claude", "projects", normalizeHostSlug))
	if len(result.RenamedDirs) != 1 || result.RenamedDirs[0] != wantRename {
		t.Errorf("RenamedDirs = %v, want [%s]", result.RenamedDirs, wantRename)
	}
}

// TestNormalizeForHost_Claude_MergesWhenHostSlugDirAlreadyExists covers
// the fallback path in moveClaudeDir: when the host-slug directory is
// already present in staging (e.g. two syncs landed on the same
// second-resolution staging timestamp and a prior pass already
// renamed), NormalizeForHost merges the VM-slug directory's files into
// it instead of failing the whole sync.
func TestNormalizeForHost_Claude_MergesWhenHostSlugDirAlreadyExists(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	relIn := filepath.Join(".claude", "projects", normalizeVMSlug, "session1.jsonl")
	writeStaged(t, staging, relIn, readTestdata(t, "claude_input.jsonl"))
	// Pre-existing host-slug directory with an unrelated file, as if a
	// prior same-timestamp pass already normalized a different session.
	preexisting := filepath.Join(".claude", "projects", normalizeHostSlug, "session0.jsonl")
	writeStaged(t, staging, preexisting, []byte(`{"cwd":"/home/user/proj","type":"user"}`+"\n"))

	result, err := sessiondata.NormalizeForHost(staging, []sessiondata.AgentKind{sessiondata.KindClaude}, normalizeVMPath, normalizeHostPath)
	if err != nil {
		t.Fatalf("NormalizeForHost: %v", err)
	}

	_, vmDirStatErr := os.Stat(filepath.Join(staging, ".claude", "projects", normalizeVMSlug))
	if !os.IsNotExist(vmDirStatErr) {
		t.Errorf("VM-slug dir should be removed after merge; statErr=%v", vmDirStatErr)
	}
	// Both the pre-existing file and the merged-in file must be present.
	_, preexistingStatErr := os.Stat(filepath.Join(staging, preexisting))
	if preexistingStatErr != nil {
		t.Errorf("pre-existing host file lost during merge: %v", preexistingStatErr)
	}
	movedIn := filepath.Join(staging, ".claude", "projects", normalizeHostSlug, "session1.jsonl")
	got, readErr := os.ReadFile(movedIn) //nolint:gosec // path under t.TempDir().
	if readErr != nil {
		t.Fatalf("read merged-in file: %v", readErr)
	}
	want := readTestdata(t, "claude_want.jsonl")
	if !bytes.Equal(got, want) {
		t.Errorf("merged-in content mismatch:\ngot:  %q\nwant: %q", got, want)
	}
	if result.RewrittenFiles[sessiondata.KindClaude] != 1 {
		t.Errorf("RewrittenFiles[claude] = %d, want 1 (only the merged-in file needed rewriting)", result.RewrittenFiles[sessiondata.KindClaude])
	}
}

// TestNormalizeForHost_Codex_RewritesSessionMetaCwdInPlace asserts the
// Codex rewriter operates in place (no directory rename) and only
// rewrites the cwd-bearing line, leaving the event line and the
// non-JSON line untouched.
func TestNormalizeForHost_Codex_RewritesSessionMetaCwdInPlace(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	rel := filepath.Join(".codex", "sessions", "2026", "05", "22", "rollout-x.jsonl")
	writeStaged(t, staging, rel, readTestdata(t, "codex_input.jsonl"))

	result, err := sessiondata.NormalizeForHost(staging, []sessiondata.AgentKind{sessiondata.KindCodex}, normalizeVMPath, normalizeHostPath)
	if err != nil {
		t.Fatalf("NormalizeForHost: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(staging, rel)) //nolint:gosec // path under t.TempDir().
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	want := readTestdata(t, "codex_want.jsonl")
	if !bytes.Equal(got, want) {
		t.Errorf("content mismatch:\ngot:  %q\nwant: %q", got, want)
	}
	if result.RewrittenFiles[sessiondata.KindCodex] != 1 {
		t.Errorf("RewrittenFiles[codex] = %d, want 1", result.RewrittenFiles[sessiondata.KindCodex])
	}
	if len(result.RenamedDirs) != 0 {
		t.Errorf("Codex normalization must not rename directories; got %v", result.RenamedDirs)
	}
}

// TestNormalizeForHost_Pi_RewritesExactVMPathStringOccurrences asserts
// the documented pi fallback: exact vmPath string occurrences inside
// JSON string values are rewritten, in place, without assuming a
// specific field name.
func TestNormalizeForHost_Pi_RewritesExactVMPathStringOccurrences(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	rel := filepath.Join(".pi", "agent", "sessions", "index.jsonl")
	writeStaged(t, staging, rel, readTestdata(t, "pi_input.jsonl"))

	result, err := sessiondata.NormalizeForHost(staging, []sessiondata.AgentKind{sessiondata.KindPi}, normalizeVMPath, normalizeHostPath)
	if err != nil {
		t.Fatalf("NormalizeForHost: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(staging, rel)) //nolint:gosec // path under t.TempDir().
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	want := readTestdata(t, "pi_want.jsonl")
	if !bytes.Equal(got, want) {
		t.Errorf("content mismatch:\ngot:  %q\nwant: %q", got, want)
	}
	if result.RewrittenFiles[sessiondata.KindPi] != 1 {
		t.Errorf("RewrittenFiles[pi] = %d, want 1", result.RewrittenFiles[sessiondata.KindPi])
	}
}

// TestNormalizeForHost_HarnessIsNoOp asserts ADR-066 silence on harness
// host-continuation: NormalizeForHost must not touch harness files.
func TestNormalizeForHost_HarnessIsNoOp(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	rel := filepath.Join(".pi", "agent", "teams", "teamA", "state.json")
	content := []byte(`{"cwd":"/root/workspace/proj"}` + "\n")
	writeStaged(t, staging, rel, content)

	result, err := sessiondata.NormalizeForHost(staging, []sessiondata.AgentKind{sessiondata.KindHarness}, normalizeVMPath, normalizeHostPath)
	if err != nil {
		t.Fatalf("NormalizeForHost: %v", err)
	}
	if len(result.RewrittenFiles) != 0 {
		t.Errorf("RewrittenFiles = %v, want empty (harness has no rewriter)", result.RewrittenFiles)
	}
	got, err := os.ReadFile(filepath.Join(staging, rel)) //nolint:gosec // path under t.TempDir().
	if err != nil {
		t.Fatalf("read harness file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("harness file was mutated: got %q, want %q", got, content)
	}
}

// TestNormalizeForHost_NoOpWhenPathsEqualOrEmpty asserts the documented
// no-op guard: when vmPath == hostPath, or either is empty,
// NormalizeForHost must not touch the staging tree at all.
func TestNormalizeForHost_NoOpWhenPathsEqualOrEmpty(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, vm, host string
	}{
		{"equal", normalizeVMPath, normalizeVMPath},
		{"empty-vm", "", normalizeHostPath},
		{"empty-host", normalizeVMPath, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			staging := t.TempDir()
			rel := filepath.Join(".codex", "sessions", "r.jsonl")
			content := readTestdata(t, "codex_input.jsonl")
			writeStaged(t, staging, rel, content)

			result, err := sessiondata.NormalizeForHost(staging, sessiondata.AllKinds(), tc.vm, tc.host)
			if err != nil {
				t.Fatalf("NormalizeForHost: %v", err)
			}
			if len(result.RewrittenFiles) != 0 || len(result.RewrittenPaths) != 0 || len(result.RenamedDirs) != 0 {
				t.Errorf("expected zero-value result, got %+v", result)
			}
			got, readErr := os.ReadFile(filepath.Join(staging, rel)) //nolint:gosec // path under t.TempDir().
			if readErr != nil {
				t.Fatalf("read file: %v", readErr)
			}
			if !bytes.Equal(got, content) {
				t.Errorf("file was mutated: got %q, want %q", got, content)
			}
		})
	}
}

// TestNormalizeForHost_IdempotentOnSecondPass asserts that running
// NormalizeForHost a second time (simulating a second sync against a
// freshly-staged, still-VM-slugged copy) against already-normalized
// content is a no-op: the host slug directory already holds
// hostPath-referencing content, so nothing changes.
func TestNormalizeForHost_IdempotentOnSecondPass(t *testing.T) {
	t.Parallel()
	staging := t.TempDir()
	relIn := filepath.Join(".claude", "projects", normalizeVMSlug, "session1.jsonl")
	writeStaged(t, staging, relIn, readTestdata(t, "claude_input.jsonl"))

	_, err := sessiondata.NormalizeForHost(staging, []sessiondata.AgentKind{sessiondata.KindClaude}, normalizeVMPath, normalizeHostPath)
	if err != nil {
		t.Fatalf("first NormalizeForHost: %v", err)
	}

	// Second pass over the now-host-slugged tree: nothing left to rewrite.
	result, err := sessiondata.NormalizeForHost(staging, []sessiondata.AgentKind{sessiondata.KindClaude}, normalizeVMPath, normalizeHostPath)
	if err != nil {
		t.Fatalf("second NormalizeForHost: %v", err)
	}
	if len(result.RewrittenFiles) != 0 {
		t.Errorf("second pass RewrittenFiles = %v, want empty", result.RewrittenFiles)
	}
	if len(result.RenamedDirs) != 0 {
		t.Errorf("second pass RenamedDirs = %v, want empty (VM-slug dir no longer present)", result.RenamedDirs)
	}
}

// TestCandidateNormalizeCounts_CountsAllowlistedExtensionsPerKind
// asserts the dry-run preview counts manifest entries per kind by
// allowlisted-root + extension match, skips harness entirely, and does
// not touch any filesystem state.
func TestCandidateNormalizeCounts_CountsAllowlistedExtensionsPerKind(t *testing.T) {
	t.Parallel()
	manifest := sessiondata.Manifest{
		VM: "vm1",
		Items: map[sessiondata.AgentKind][]sessiondata.FileEntry{
			sessiondata.KindClaude: {
				{Path: ".claude/projects/-root-proj/a.jsonl"},
				{Path: ".claude/projects/-root-proj/b.jsonl"},
			},
			sessiondata.KindCodex: {
				{Path: ".codex/sessions/2026/05/22/rollout-x.jsonl"},
			},
			sessiondata.KindPi: {
				{Path: ".pi/agent/sessions/index.jsonl"},
			},
			sessiondata.KindHarness: {
				{Path: ".pi/agent/teams/teamA/state.json"},
			},
		},
	}

	preview := sessiondata.CandidateNormalizeCounts(manifest, sessiondata.AllKinds())
	if preview.CandidateFiles[sessiondata.KindClaude] != 2 {
		t.Errorf("CandidateFiles[claude] = %d, want 2", preview.CandidateFiles[sessiondata.KindClaude])
	}
	if preview.CandidateFiles[sessiondata.KindCodex] != 1 {
		t.Errorf("CandidateFiles[codex] = %d, want 1", preview.CandidateFiles[sessiondata.KindCodex])
	}
	if preview.CandidateFiles[sessiondata.KindPi] != 1 {
		t.Errorf("CandidateFiles[pi] = %d, want 1", preview.CandidateFiles[sessiondata.KindPi])
	}
	if _, ok := preview.CandidateFiles[sessiondata.KindHarness]; ok {
		t.Errorf("CandidateFiles must not include harness; got %v", preview.CandidateFiles)
	}
	if len(preview.CandidatePaths) != 4 {
		t.Errorf("len(CandidatePaths) = %d, want 4", len(preview.CandidatePaths))
	}
}
