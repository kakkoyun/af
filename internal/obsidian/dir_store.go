package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kakkoyun/af/internal/session"
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
// The temp-file-plus-rename-plus-parent-fsync mechanics are shared
// with session.WriteState via session.WriteFileAtomic, so both stores
// give the same atomicity and durability guarantees.
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
	err = session.WriteFileAtomic(path, content, noteFilePerm)
	if err != nil {
		return fmt.Errorf("write obsidian note %s: %w", path, err)
	}
	return nil
}
