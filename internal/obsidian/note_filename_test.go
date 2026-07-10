package obsidian_test

import (
	"path/filepath"
	"testing"

	"github.com/kakkoyun/af/internal/obsidian"
)

// TestNoteFileName covers the issue #34 filename derivation rules:
// plain names, the auto-name slug-prefix + timestamp-reformat
// convention, nested user-chosen names, and a slug-prefixed name whose
// remainder is not a timestamp.
func TestNoteFileName(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		repoSlug    string
		want        string
	}{
		{
			name:        "plain name is untouched",
			sessionName: "demo",
			repoSlug:    "github.com/kakkoyun/af",
			want:        "demo.md",
		},
		{
			name:        "auto name strips slug prefix and reformats the timestamp",
			sessionName: "github.com/kakkoyun/dotfiles-20260709-085045",
			repoSlug:    "github.com/kakkoyun/dotfiles",
			want:        "2026-07-09-085045.md",
		},
		{
			name:        "nested user name replaces slashes, no slug prefix present",
			sessionName: "team/x",
			repoSlug:    "github.com/kakkoyun/af",
			want:        "team-x.md",
		},
		{
			name:        "backslashes sanitised too (Windows path separator)",
			sessionName: `team\x`,
			repoSlug:    "github.com/kakkoyun/af",
			want:        "team-x.md",
		},
		{
			name:        "slug prefix present but remainder is not a timestamp",
			sessionName: "github.com/kakkoyun/af-feature/sub",
			repoSlug:    "github.com/kakkoyun/af",
			want:        "feature-sub.md",
		},
		{
			name:        "empty repo slug never strips a prefix",
			sessionName: "20260709-085045",
			repoSlug:    "",
			want:        "20260709-085045.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := obsidian.NoteFileName(tt.sessionName, tt.repoSlug)
			if got != tt.want {
				t.Fatalf("NoteFileName(%q, %q) = %q, want %q", tt.sessionName, tt.repoSlug, got, tt.want)
			}
		})
	}
}

// TestComposeNotePath covers the issue #34 directory-composition
// rules: default "repo" mode nests under a per-repo subfolder derived
// from the repo slug, empty mode behaves the same as "repo" (config
// callers that predate the key), "flat" keeps the old layout, and a
// nested session name never creates a subdirectory of its own.
func TestComposeNotePath(t *testing.T) {
	notesDir := filepath.Join(string(filepath.Separator)+"vault", "00 - workstreams")

	tests := []struct {
		name        string
		mode        string
		sessionName string
		repoSlug    string
		gitRoot     string
		want        string
	}{
		{
			name:        "repo mode nests under the repo subfolder",
			mode:        obsidian.SubfolderModeRepo,
			sessionName: "fix-parser",
			repoSlug:    "github.com/kakkoyun/af",
			want:        filepath.Join(notesDir, "af", "fix-parser.md"),
		},
		{
			name:        "empty mode defaults to repo nesting",
			mode:        "",
			sessionName: "fix-parser",
			repoSlug:    "github.com/kakkoyun/af",
			want:        filepath.Join(notesDir, "af", "fix-parser.md"),
		},
		{
			name:        "flat mode keeps the pre-issue-34 layout",
			mode:        obsidian.SubfolderModeFlat,
			sessionName: "fix-parser",
			repoSlug:    "github.com/kakkoyun/af",
			want:        filepath.Join(notesDir, "fix-parser.md"),
		},
		{
			name:        "empty repo slug falls back to the git root basename",
			mode:        obsidian.SubfolderModeRepo,
			sessionName: "fix-parser",
			repoSlug:    "",
			gitRoot:     filepath.Join(string(filepath.Separator)+"home", "owner", "scratch"),
			want:        filepath.Join(notesDir, "scratch", "fix-parser.md"),
		},
		{
			name:        "nested session name never creates its own subdirectory",
			mode:        obsidian.SubfolderModeRepo,
			sessionName: "team/x",
			repoSlug:    "github.com/kakkoyun/af",
			want:        filepath.Join(notesDir, "af", "team-x.md"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := obsidian.ComposeNotePath(notesDir, tt.mode, tt.sessionName, tt.repoSlug, tt.gitRoot)
			if got != tt.want {
				t.Fatalf("ComposeNotePath() = %q, want %q", got, tt.want)
			}
		})
	}
}
