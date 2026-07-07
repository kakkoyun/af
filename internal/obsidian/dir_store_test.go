package obsidian_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kakkoyun/af/internal/obsidian"
)

func dirStoreNote() obsidian.Note {
	return obsidian.Note{
		Frontmatter: obsidian.Frontmatter{
			Session: "demo",
			Branch:  "feat/demo",
			Status:  "active",
			Schema:  1,
		},
		Body: "# demo\n\nnotes body\n",
	}
}

func TestDirStore_WriteReadRoundTrip(t *testing.T) {
	store := obsidian.NewDirStore()
	path := filepath.Join(t.TempDir(), "vault", "af", "demo.md")

	err := store.Write(context.Background(), path, dirStoreNote())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read(context.Background(), path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Frontmatter.Session != "demo" {
		t.Fatalf("Frontmatter.Session = %q, want demo", got.Frontmatter.Session)
	}
	if got.Body != dirStoreNote().Body {
		t.Fatalf("Body = %q, want %q", got.Body, dirStoreNote().Body)
	}
}

func TestDirStore_WriteCreatesParentDirs(t *testing.T) {
	store := obsidian.NewDirStore()
	path := filepath.Join(t.TempDir(), "deeply", "nested", "vault", "demo.md")

	err := store.Write(context.Background(), path, dirStoreNote())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written note: %v", err)
	}
	if info.IsDir() {
		t.Fatal("written note is a directory")
	}
}

func TestDirStore_ReadMissingReturnsErrNoteNotFound(t *testing.T) {
	store := obsidian.NewDirStore()

	_, err := store.Read(context.Background(), filepath.Join(t.TempDir(), "missing.md"))
	if !errors.Is(err, obsidian.ErrNoteNotFound) {
		t.Fatalf("Read(missing) error = %v, want ErrNoteNotFound", err)
	}
}

func TestDirStore_WriteIsAtomicOverExisting(t *testing.T) {
	store := obsidian.NewDirStore()
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.md")

	err := store.Write(context.Background(), path, dirStoreNote())
	if err != nil {
		t.Fatalf("first Write: %v", err)
	}
	updated := dirStoreNote()
	updated.Body = "# demo\n\nupdated\n"
	err = store.Write(context.Background(), path, updated)
	if err != nil {
		t.Fatalf("second Write: %v", err)
	}
	got, err := store.Read(context.Background(), path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Body != updated.Body {
		t.Fatalf("Body = %q, want %q", got.Body, updated.Body)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want 1 (no tmp litter)", len(entries))
	}
}

func TestDirStore_ContextCancellationRejected(t *testing.T) {
	store := obsidian.NewDirStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := store.Write(ctx, filepath.Join(t.TempDir(), "demo.md"), dirStoreNote())
	if err == nil {
		t.Fatal("Write with cancelled context succeeded, want error")
	}
	_, err = store.Read(ctx, filepath.Join(t.TempDir(), "demo.md"))
	if err == nil {
		t.Fatal("Read with cancelled context succeeded, want error")
	}
}
