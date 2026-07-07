package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	noteDirPerm  = 0o750
	noteFilePerm = 0o600
)

// DirStore is the production filesystem-backed Store (ADR-047). Notes
// are written atomically (tmp + rename) so a crash mid-write never
// leaves a truncated note in the vault.
type DirStore struct{}

// NewDirStore returns a Store persisting notes to the local filesystem.
func NewDirStore() DirStore {
	return DirStore{}
}

// Read parses the note at path.
func (DirStore) Read(ctx context.Context, path string) (Note, error) {
	err := ctx.Err()
	if err != nil {
		return Note{}, fmt.Errorf("read obsidian note %s: %w", path, err)
	}
	content, err := os.ReadFile(path) //nolint:gosec // Note paths are derived from af's own config.
	if errors.Is(err, os.ErrNotExist) {
		return Note{}, fmt.Errorf("read obsidian note %s: %w", path, ErrNoteNotFound)
	}
	if err != nil {
		return Note{}, fmt.Errorf("read obsidian note %s: %w", path, err)
	}
	note, err := ParseNote(content)
	if err != nil {
		return Note{}, fmt.Errorf("read obsidian note %s: %w", path, err)
	}
	return note, nil
}

// Write emits note to path atomically, creating parent directories.
func (DirStore) Write(ctx context.Context, path string, note Note) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("write obsidian note %s: %w", path, err)
	}
	content, err := EmitNote(note)
	if err != nil {
		return fmt.Errorf("write obsidian note %s: %w", path, err)
	}
	err = os.MkdirAll(filepath.Dir(path), noteDirPerm)
	if err != nil {
		return fmt.Errorf("write obsidian note %s: create parent: %w", path, err)
	}
	// A unique temp name keeps concurrent writers from interleaving in
	// a shared .tmp file, and the fsync before rename makes the atomic
	// claim hold across power loss, matching session.WriteState.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write obsidian note %s: create temp: %w", path, err)
	}
	tmpPath := tmp.Name()
	err = writeAndSyncNote(tmp, content)
	if err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // Best-effort tmp cleanup on write failure.
		return fmt.Errorf("write obsidian note %s: %w", path, err)
	}
	err = os.Chmod(tmpPath, noteFilePerm)
	if err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // Best-effort tmp cleanup on chmod failure.
		return fmt.Errorf("write obsidian note %s: chmod: %w", path, err)
	}
	err = os.Rename(tmpPath, path)
	if err != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // Best-effort tmp cleanup on rename failure.
		return fmt.Errorf("write obsidian note %s: replace: %w", path, err)
	}
	return nil
}

// writeAndSyncNote writes content and fsyncs before close so the
// subsequent rename installs fully-persisted bytes.
func writeAndSyncNote(file *os.File, content []byte) error {
	_, err := file.Write(content)
	if err != nil {
		_ = file.Close() //nolint:errcheck // Write error takes precedence.
		return fmt.Errorf("write temp: %w", err)
	}
	err = file.Sync()
	if err != nil {
		_ = file.Close() //nolint:errcheck // Sync error takes precedence.
		return fmt.Errorf("sync: %w", err)
	}
	err = file.Close()
	if err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	return nil
}
