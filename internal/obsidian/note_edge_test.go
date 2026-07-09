package obsidian_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/obsidian"
)

func TestParseNote_MissingFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "no leading marker", content: "# just a body\n"},
		{name: "empty content", content: ""},
		{name: "unterminated frontmatter", content: "---\naf_schema: 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := obsidian.ParseNote([]byte(tt.content))
			if !errors.Is(err, obsidian.ErrMissingFrontmatter) {
				t.Fatalf("ParseNote() error = %v, want ErrMissingFrontmatter", err)
			}
		})
	}
}

func TestParseNote_InvalidYAMLFrontmatter(t *testing.T) {
	content := []byte("---\naf_schema: [unclosed\n---\nbody\n")

	_, err := obsidian.ParseNote(content)
	if err == nil {
		t.Fatal("ParseNote() error = nil, want YAML parse error")
	}
	if !strings.Contains(err.Error(), "parse obsidian frontmatter") {
		t.Fatalf("ParseNote() error = %v, want parse obsidian frontmatter context", err)
	}
}

func TestEmitNote_ParseNoteRoundTrip(t *testing.T) {
	note := obsidian.Note{
		Frontmatter: obsidian.Frontmatter{Schema: 1, Session: "roundtrip", Status: "active"},
		Body:        "# roundtrip\n\nbody\n",
	}

	content, err := obsidian.EmitNote(note)
	if err != nil {
		t.Fatalf("EmitNote() error = %v", err)
	}
	got, err := obsidian.ParseNote(content)
	if err != nil {
		t.Fatalf("ParseNote(EmitNote()) error = %v", err)
	}
	if got.Frontmatter.Session != "roundtrip" || got.Frontmatter.Schema != 1 {
		t.Fatalf("Frontmatter = %#v", got.Frontmatter)
	}
	if got.Body != note.Body {
		t.Fatalf("Body = %q, want %q", got.Body, note.Body)
	}
}

func TestResolveNotePath_SingleVaultAutoSelected(t *testing.T) {
	vaultPath := filepath.Join(string(filepath.Separator)+"vaults", "solo")
	cfg := obsidian.PathConfig{Vaults: map[string]string{"solo": vaultPath}, SubfolderMode: obsidian.SubfolderModeFlat}

	got, err := obsidian.ResolveNotePath(cfg, "session-1", "", "")
	if err != nil {
		t.Fatalf("ResolveNotePath() error = %v", err)
	}
	want := filepath.Join(vaultPath, "session-1.md")
	if got != want {
		t.Fatalf("ResolveNotePath() = %q, want %q", got, want)
	}
}

func TestResolveNotePath_Errors(t *testing.T) {
	tests := []struct {
		cfg  obsidian.PathConfig
		name string
	}{
		{name: "no vaults configured", cfg: obsidian.PathConfig{}},
		{name: "multiple vaults without selection", cfg: obsidian.PathConfig{
			Vaults: map[string]string{"a": "/a", "b": "/b"},
		}},
		{name: "selected vault has no path", cfg: obsidian.PathConfig{
			Vaults:     map[string]string{"other": "/other"},
			NotesVault: "personal",
		}},
		{name: "single vault with empty path", cfg: obsidian.PathConfig{
			Vaults: map[string]string{"solo": ""},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := obsidian.ResolveNotePath(tt.cfg, "session-1", "", "")
			if !errors.Is(err, obsidian.ErrVaultNotConfigured) {
				t.Fatalf("ResolveNotePath() error = %v, want ErrVaultNotConfigured", err)
			}
		})
	}
}

func TestMemoryStore_CancelledContextRejected(t *testing.T) {
	store := obsidian.NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	note := obsidian.Note{Frontmatter: obsidian.Frontmatter{Schema: 1}, Body: "body\n"}

	err := store.Write(ctx, "note.md", note)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Write(cancelled) error = %v, want context.Canceled", err)
	}
	_, err = store.Read(ctx, "note.md")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Read(cancelled) error = %v, want context.Canceled", err)
	}
}
