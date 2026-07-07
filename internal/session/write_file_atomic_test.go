package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/session"
)

// TestWriteFileAtomic_RoundTrips verifies the happy path: data lands at
// path with the requested permission bits and no temp file survives.
func TestWriteFileAtomic_RoundTrips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")

	err := session.WriteFileAtomic(path, []byte("hello atomic"), 0o600)
	if err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}

	got, err := os.ReadFile(path) //nolint:gosec // Test reads a file it just wrote to a temp dir.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "hello atomic" {
		t.Fatalf("content = %q, want %q", got, "hello atomic")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries after write, want 1 (no leftover temp file): %v", len(entries), entries)
	}
}

// TestWriteFileAtomic_SurvivesHostileFixedTmpName pins that a directory
// squatting the legacy fixed "<path>.tmp" name cannot break a write:
// os.CreateTemp always picks a fresh unique name.
func TestWriteFileAtomic_SurvivesHostileFixedTmpName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")
	err := os.Mkdir(path+".tmp", 0o750)
	if err != nil {
		t.Fatalf("seed squatted tmp dir: %v", err)
	}

	err = session.WriteFileAtomic(path, []byte("data"), 0o600)
	if err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
}

// TestWriteFileAtomic_MissingDirFails checks the create-temp-file
// failure path and that it surfaces a "create temp file" context.
func TestWriteFileAtomic_MissingDirFails(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing", "payload.bin")

	err := session.WriteFileAtomic(path, []byte("data"), 0o600)
	if err == nil || !strings.Contains(err.Error(), "create temp file") {
		t.Fatalf("WriteFileAtomic() error = %v, want create temp file context", err)
	}
}

// TestWriteFileAtomic_RenameOntoDirectoryFails checks the replace
// failure path and that the temp file is cleaned up.
func TestWriteFileAtomic_RenameOntoDirectoryFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")
	err := os.Mkdir(path, 0o750)
	if err != nil {
		t.Fatalf("seed destination directory: %v", err)
	}

	err = session.WriteFileAtomic(path, []byte("data"), 0o600)
	if err == nil || !strings.Contains(err.Error(), "replace") {
		t.Fatalf("WriteFileAtomic() error = %v, want replace context", err)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir() error = %v", readErr)
	}
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries after failed write, want 1 (temp file left behind): %v", len(entries), entries)
	}
}

// TestWriteFileAtomic_DirDoesNotExistPropagatesNotExist confirms the
// underlying os error is preserved through errors.Is so callers can
// distinguish "no such directory" from other failures if they need to.
func TestWriteFileAtomic_DirDoesNotExistPropagatesNotExist(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing", "payload.bin")

	err := session.WriteFileAtomic(path, []byte("data"), 0o600)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("WriteFileAtomic() error = %v, want wrapping os.ErrNotExist", err)
	}
}
