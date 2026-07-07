package obsidian_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/obsidian"
)

func TestDirStore_ReadDirectoryFails(t *testing.T) {
	store := obsidian.NewDirStore()
	dir := t.TempDir()

	_, err := store.Read(context.Background(), dir)
	if err == nil {
		t.Fatal("Read(directory) error = nil, want read error")
	}
	if errors.Is(err, obsidian.ErrNoteNotFound) {
		t.Fatalf("Read(directory) error = %v, want non-ErrNoteNotFound read error", err)
	}
}

func TestDirStore_ReadInvalidNoteFails(t *testing.T) {
	store := obsidian.NewDirStore()
	path := filepath.Join(t.TempDir(), "garbage.md")
	err := os.WriteFile(path, []byte("no frontmatter here\n"), 0o600)
	if err != nil {
		t.Fatalf("seed garbage note: %v", err)
	}

	_, err = store.Read(context.Background(), path)
	if !errors.Is(err, obsidian.ErrMissingFrontmatter) {
		t.Fatalf("Read(garbage) error = %v, want ErrMissingFrontmatter", err)
	}
}

func TestDirStore_WriteParentIsFileFails(t *testing.T) {
	store := obsidian.NewDirStore()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	err := os.WriteFile(blocker, []byte("file, not dir\n"), 0o600)
	if err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}

	err = store.Write(context.Background(), filepath.Join(blocker, "note.md"), dirStoreNote())
	if err == nil {
		t.Fatal("Write(under file) error = nil, want create parent error")
	}
	if !strings.Contains(err.Error(), "create parent") {
		t.Fatalf("Write(under file) error = %v, want create parent context", err)
	}
}

func TestDirStore_WriteTmpPathIsDirectoryFails(t *testing.T) {
	store := obsidian.NewDirStore()
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	err := os.Mkdir(path+".tmp", 0o750)
	if err != nil {
		t.Fatalf("seed tmp directory: %v", err)
	}

	err = store.Write(context.Background(), path, dirStoreNote())
	if err == nil {
		t.Fatal("Write(tmp blocked) error = nil, want write error")
	}
}

func TestDirStore_WriteRenameOntoDirectoryFails(t *testing.T) {
	store := obsidian.NewDirStore()
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	err := os.Mkdir(path, 0o750)
	if err != nil {
		t.Fatalf("seed destination directory: %v", err)
	}

	err = store.Write(context.Background(), path, dirStoreNote())
	if err == nil {
		t.Fatal("Write(onto directory) error = nil, want replace error")
	}
	if !strings.Contains(err.Error(), "replace") {
		t.Fatalf("Write(onto directory) error = %v, want replace context", err)
	}
	if _, statErr := os.Stat(path + ".tmp"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("tmp file left behind after failed rename: stat err = %v", statErr)
	}
}
