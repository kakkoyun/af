package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/obsidian"
)

// writeRetroArchiveNote creates one archived note under archiveDir/<name>/note.md.
func writeRetroArchiveNote(t *testing.T, archiveDir, name string, note obsidian.Note) {
	t.Helper()
	entryDir := filepath.Join(archiveDir, name)
	err := os.MkdirAll(entryDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir archive entry %s: %v", name, err)
	}
	noteBytes, err := obsidian.EmitNote(note)
	if err != nil {
		t.Fatalf("EmitNote: %v", err)
	}
	err = os.WriteFile(filepath.Join(entryDir, "note.md"), noteBytes, 0o600)
	if err != nil {
		t.Fatalf("write note: %v", err)
	}
}

// defaultRetroNote returns a basic archived note for use in tests.
func defaultRetroNote(name string) obsidian.Note {
	return obsidian.Note{
		Frontmatter: obsidian.Frontmatter{
			Schema:    1,
			Session:   name,
			Branch:    "feat/" + name,
			Status:    "completed",
			StartedAt: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		},
		Body: "## Notes\n\nsome archived note content for " + name + "\n",
	}
}

func TestRetro_EmptyArchiveReportsNoMatches(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

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
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	writeRetroArchiveNote(t, archiveDir, "some-name", defaultRetroNote("some-name"))

	stdout, _, err := executeCommand(t, newRootCmd(), "retro")
	if err != nil {
		t.Fatalf("retro: %v", err)
	}
	if !strings.Contains(stdout, "some-name") {
		t.Fatalf("retro output missing 'some-name'; got:\n%s", stdout)
	}
}

// TestRetro_AIErrorsWhenNoNotesMatch verifies that --ai fails with
// errRetroAINoNotes when the archive exists but all notes are filtered out.
func TestRetro_AIErrorsWhenNoNotesMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	// Seed a note that will be filtered out by --tag.
	writeRetroArchiveNote(t, archiveDir, "some-name", defaultRetroNote("some-name"))

	orig := retroAIBodyFunc
	t.Cleanup(func() { retroAIBodyFunc = orig })
	retroAIBodyFunc = func(_ context.Context, _ agent.Agent, _ agent.BodyOpts, _ string) (string, error) {
		return "SHOULD NOT REACH", nil
	}

	_, _, err := executeCommand(t, newRootCmd(), "retro", "--ai", "--tag", "nonexistent-tag")
	if err == nil {
		t.Fatal("retro --ai with no matching notes returned nil, want error")
	}
	if !errors.Is(err, errRetroAINoNotes) {
		t.Fatalf("error %v does not wrap errRetroAINoNotes", err)
	}
}

// TestRetro_AIUsesAgentNarrative verifies that a matching note causes the
// injected seam to be called and its output appears under "## Narrative".
func TestRetro_AIUsesAgentNarrative(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	writeRetroArchiveNote(t, archiveDir, "ai-note", defaultRetroNote("ai-note"))

	orig := retroAIBodyFunc
	t.Cleanup(func() { retroAIBodyFunc = orig })
	retroAIBodyFunc = func(_ context.Context, _ agent.Agent, _ agent.BodyOpts, _ string) (string, error) {
		return "FAKE NARRATIVE", nil
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "retro", "--ai")
	if err != nil {
		t.Fatalf("retro --ai: %v", err)
	}
	if !strings.Contains(stdout, "## Narrative") {
		t.Fatalf("output missing '## Narrative'; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "FAKE NARRATIVE") {
		t.Fatalf("output missing 'FAKE NARRATIVE'; got:\n%s", stdout)
	}
}

// TestRetro_AIErrorsOnEmptyAgentOutput verifies that an agent returning
// only whitespace is treated as errRetroAIEmpty.
func TestRetro_AIErrorsOnEmptyAgentOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	writeRetroArchiveNote(t, archiveDir, "empty-note", defaultRetroNote("empty-note"))

	orig := retroAIBodyFunc
	t.Cleanup(func() { retroAIBodyFunc = orig })
	retroAIBodyFunc = func(_ context.Context, _ agent.Agent, _ agent.BodyOpts, _ string) (string, error) {
		return "   ", nil
	}

	_, _, err := executeCommand(t, newRootCmd(), "retro", "--ai")
	if err == nil {
		t.Fatal("retro --ai with empty agent output returned nil, want error")
	}
	if !errors.Is(err, errRetroAIEmpty) {
		t.Fatalf("error %v does not wrap errRetroAIEmpty", err)
	}
}

// TestRetro_AIPromptContainsNoteContent verifies the prompt passed to the
// agent contains the note session name and body text.
func TestRetro_AIPromptContainsNoteContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	note := defaultRetroNote("prompt-check")
	note.Body = "## Notes\n\nUNIQUE_BODY_CONTENT\n"
	writeRetroArchiveNote(t, archiveDir, "prompt-check", note)

	var capturedPrompt string
	orig := retroAIBodyFunc
	t.Cleanup(func() { retroAIBodyFunc = orig })
	retroAIBodyFunc = func(_ context.Context, _ agent.Agent, _ agent.BodyOpts, p string) (string, error) {
		capturedPrompt = p
		return "narrative", nil
	}

	_, _, err := executeCommand(t, newRootCmd(), "retro", "--ai")
	if err != nil {
		t.Fatalf("retro --ai: %v", err)
	}
	if !strings.Contains(capturedPrompt, "prompt-check") {
		t.Fatalf("prompt missing session name 'prompt-check'; prompt:\n%s", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "UNIQUE_BODY_CONTENT") {
		t.Fatalf("prompt missing note body; prompt:\n%s", capturedPrompt)
	}
}

// TestRetro_AIModelPassedToAgent verifies --ai-model is forwarded to the seam.
func TestRetro_AIModelPassedToAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archiveDir := filepath.Join(home, ".local", "share", "af", "v1", "archive")

	writeRetroArchiveNote(t, archiveDir, "model-note", defaultRetroNote("model-note"))

	var capturedModel string
	orig := retroAIBodyFunc
	t.Cleanup(func() { retroAIBodyFunc = orig })
	retroAIBodyFunc = func(_ context.Context, _ agent.Agent, opts agent.BodyOpts, _ string) (string, error) {
		capturedModel = opts.Model
		return "narrative with model=" + opts.Model, nil
	}

	_, _, err := executeCommand(t, newRootCmd(), "retro", "--ai", "--ai-model", "sonnet-4-5")
	if err != nil {
		t.Fatalf("retro --ai --ai-model: %v", err)
	}
	if capturedModel != "sonnet-4-5" {
		t.Fatalf("capturedModel = %q, want %q", capturedModel, "sonnet-4-5")
	}
}
