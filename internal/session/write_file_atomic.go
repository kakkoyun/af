package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic atomically writes data to path: it creates a unique
// temporary file in path's directory (via os.CreateTemp, so concurrent
// writers never share one fixed temp name), writes and fsyncs it,
// chmods it to perm, renames it onto path, and fsyncs the parent
// directory so the rename itself is durable across a crash. The
// caller is responsible for ensuring path's directory already exists.
// On any failure after the temp file is created, the temp file is
// removed on a best-effort basis.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	err = writeSyncCloseTemp(tmp, data)
	if err != nil {
		removeTempBestEffort(tmpPath)
		return fmt.Errorf("write temp file %s: %w", tmpPath, err)
	}
	err = os.Chmod(tmpPath, perm)
	if err != nil {
		removeTempBestEffort(tmpPath)
		return fmt.Errorf("chmod temp file %s: %w", tmpPath, err)
	}
	err = os.Rename(tmpPath, path)
	if err != nil {
		removeTempBestEffort(tmpPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}

	return syncDir(dir)
}

// writeSyncCloseTemp writes data to file, fsyncs it, and closes it so
// the subsequent rename installs fully-persisted bytes.
func writeSyncCloseTemp(file *os.File, data []byte) error {
	_, err := file.Write(data)
	if err != nil {
		_ = file.Close() //nolint:errcheck // Write error takes precedence.
		return fmt.Errorf("write: %w", err)
	}
	err = file.Sync()
	if err != nil {
		_ = file.Close() //nolint:errcheck // Sync error takes precedence.
		return fmt.Errorf("sync: %w", err)
	}
	err = file.Close()
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}

	return nil
}

// removeTempBestEffort removes a temp file left behind by a failed
// atomic write. Failure to clean up is not itself an error the caller
// needs to see: the original write failure is what matters.
func removeTempBestEffort(tmpPath string) {
	_ = os.Remove(tmpPath) //nolint:errcheck // Best-effort cleanup on write failure.
}
