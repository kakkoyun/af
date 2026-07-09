package obsidian_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/obsidian"
)

func TestParseNote_ReadsFrontmatterAndBody(t *testing.T) {
	content := []byte(`---
af_schema: 1
af_session: kakkoyun--issue-42
af_repo: af
af_branch: kakkoyun/issue-42
af_base_branch: upstream/main
af_status: active
af_agents:
  - slot: primary
    provider: pi
    status: running
af_started_at: 2026-05-06T12:00:00Z
af_completed_at: null
af_pr_number: 0
af_pr_url: ""
af_pr_state: ""
tags: [af]
af_tags: [infra]
---
# kakkoyun--issue-42

## Log

body stays opaque
`)

	note, err := obsidian.ParseNote(content)
	if err != nil {
		t.Fatalf("ParseNote() error = %v", err)
	}
	if note.Frontmatter.Session != "kakkoyun--issue-42" {
		t.Fatalf("Session = %q", note.Frontmatter.Session)
	}
	if note.Frontmatter.StartedAt.Format(time.RFC3339) != "2026-05-06T12:00:00Z" {
		t.Fatalf("StartedAt = %s", note.Frontmatter.StartedAt.Format(time.RFC3339))
	}
	if note.Frontmatter.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil", note.Frontmatter.CompletedAt)
	}
	if len(note.Frontmatter.Agents) != 1 || note.Frontmatter.Agents[0].Provider != "pi" {
		t.Fatalf("Agents = %#v", note.Frontmatter.Agents)
	}
	if note.Body != "# kakkoyun--issue-42\n\n## Log\n\nbody stays opaque\n" {
		t.Fatalf("Body = %q", note.Body)
	}
}

func TestEmitNote_WritesFrontmatterAndPreservesBody(t *testing.T) {
	started := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	note := obsidian.Note{
		Frontmatter: obsidian.Frontmatter{
			Schema:     1,
			Session:    "kakkoyun--issue-42",
			Repo:       "af",
			Branch:     "kakkoyun/issue-42",
			BaseBranch: "upstream/main",
			Status:     "active",
			Agents: []obsidian.Agent{{
				Slot:     "primary",
				Provider: "pi",
				Status:   "running",
			}},
			StartedAt: started,
			PRNumber:  0,
			Tags:      []string{"af"},
			AFTags:    []string{},
		},
		Body: "# kakkoyun--issue-42\n\n## Log\n",
	}

	content, err := obsidian.EmitNote(note)
	if err != nil {
		t.Fatalf("EmitNote() error = %v", err)
	}
	text := string(content)
	for _, want := range []string{"---\n", "af_schema: 1", "af_session: kakkoyun--issue-42", "af_completed_at: null", "tags:\n    - af", "# kakkoyun--issue-42\n\n## Log\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("EmitNote() missing %q in:\n%s", want, text)
		}
	}
}

func TestResolveNotePath_UsesConfiguredVaultAndFolder(t *testing.T) {
	cfg := obsidian.PathConfig{
		Vaults:        map[string]string{"personal": string(filepath.Separator) + filepath.Join("vaults", "personal")},
		NotesVault:    "personal",
		NotesFolder:   filepath.Join("00 - workstreams", "extra"),
		SubfolderMode: obsidian.SubfolderModeFlat,
	}

	got, err := obsidian.ResolveNotePath(cfg, "kakkoyun--issue-42", "", "")
	if err != nil {
		t.Fatalf("ResolveNotePath() error = %v", err)
	}
	want := filepath.Join(string(filepath.Separator)+filepath.Join("vaults", "personal"), "00 - workstreams", "extra", "kakkoyun--issue-42.md")
	if got != want {
		t.Fatalf("ResolveNotePath() = %q, want %q", got, want)
	}
}

// TestResolveNotePath_RepoModeNestsUnderRepoSubfolder guards issue #34:
// the default "repo" subfolder mode nests the note under a folder
// named after the last path element of the repo slug.
func TestResolveNotePath_RepoModeNestsUnderRepoSubfolder(t *testing.T) {
	vaultPath := string(filepath.Separator) + filepath.Join("vaults", "personal")
	cfg := obsidian.PathConfig{
		Vaults:      map[string]string{"personal": vaultPath},
		NotesVault:  "personal",
		NotesFolder: "00 - workstreams",
	}

	got, err := obsidian.ResolveNotePath(cfg, "github.com/kakkoyun/dotfiles-20260709-085045", "github.com/kakkoyun/dotfiles", "")
	if err != nil {
		t.Fatalf("ResolveNotePath() error = %v", err)
	}
	want := filepath.Join(vaultPath, "00 - workstreams", "dotfiles", "2026-07-09-085045.md")
	if got != want {
		t.Fatalf("ResolveNotePath() = %q, want %q", got, want)
	}
}

func TestMemoryStore_WriteReadAndMissing(t *testing.T) {
	ctx := context.Background()
	store := obsidian.NewMemoryStore()
	path := filepath.Join("vault", "note.md")
	note := obsidian.Note{
		Frontmatter: obsidian.Frontmatter{Schema: 1, Session: "note"},
		Body:        "# note\n",
	}

	err := store.Write(ctx, path, note)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got, err := store.Read(ctx, path)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.Frontmatter.Session != "note" || got.Body != "# note\n" {
		t.Fatalf("Read() = %#v", got)
	}

	_, err = store.Read(ctx, filepath.Join("vault", "missing.md"))
	if !errors.Is(err, obsidian.ErrNoteNotFound) {
		t.Fatalf("Read(missing) error = %v, want ErrNoteNotFound", err)
	}
}
