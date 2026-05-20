package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/obsidian"
)

func TestRetro_EmptyArchiveReportsNoMatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No archive directory created — expect an error mentioning the archive.

	_, _, err := executeCommand(t, newRootCmd(), "retro")
	if err == nil {
		t.Fatal("retro with no archive returned nil, want error")
	}
	if !strings.Contains(err.Error(), "archive directory does not exist") {
		t.Fatalf("error %v does not mention 'archive directory does not exist'", err)
	}
}

func TestRetro_ListsArchivedNotes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create an archive entry with a valid Obsidian note.
	archiveEntryDir := filepath.Join(home, ".local", "share", "af", "v1", "archive", "some-name")
	err := os.MkdirAll(archiveEntryDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir archive entry: %v", err)
	}

	note := obsidian.Note{
		Frontmatter: obsidian.Frontmatter{
			Schema:    1,
			Session:   "some-name",
			Branch:    "feat/some-name",
			Status:    "completed",
			StartedAt: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		},
		Body: "## Notes\n\nsome archived note content\n",
	}
	noteBytes, err := obsidian.EmitNote(note)
	if err != nil {
		t.Fatalf("EmitNote: %v", err)
	}
	notePath := filepath.Join(archiveEntryDir, "note.md")
	err = os.WriteFile(notePath, noteBytes, 0o600)
	if err != nil {
		t.Fatalf("write note: %v", err)
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "retro")
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if !strings.Contains(stdout, "some-name") {
		t.Fatalf("retro output missing 'some-name'; got:\n%s", stdout)
	}
}
