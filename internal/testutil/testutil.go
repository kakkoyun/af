package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const (
	privateDirPerm     = 0o750
	privateFilePerm    = 0o600
	executableFilePerm = 0o700
)

// BuildBinary builds packagePath into output for integration-style tests.
func BuildBinary(tb testing.TB, ctx context.Context, packagePath, output string) {
	tb.Helper()

	MustMkdirAll(tb, filepath.Dir(output))
	cmd := exec.CommandContext(ctx, "go", "build", "-o", output, packagePath)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("build %s: %v\n%s", packagePath, err, combined)
	}
}

// MustMkdirAll creates path or fails the test.
func MustMkdirAll(tb testing.TB, path string) {
	tb.Helper()

	err := os.MkdirAll(path, privateDirPerm)
	if err != nil {
		tb.Fatalf("create directory %s: %v", path, err)
	}
}

// PrependPath returns pathList with dir prepended using the host path separator.
func PrependPath(dir, pathList string) string {
	if pathList == "" {
		return dir
	}
	return dir + string(os.PathListSeparator) + pathList
}

// WriteExecutable writes a POSIX shell executable into dir for fake-command tests.
func WriteExecutable(tb testing.TB, dir, name, body string) string {
	tb.Helper()

	MustMkdirAll(tb, dir)
	path := filepath.Join(dir, name)
	content := fmt.Sprintf("#!/bin/sh\n%s\n", body)
	err := os.WriteFile(path, []byte(content), privateFilePerm)
	if err != nil {
		tb.Fatalf("write executable %s: %v", path, err)
	}
	err = os.Chmod(path, executableFilePerm)
	if err != nil {
		tb.Fatalf("mark executable %s: %v", path, err)
	}
	return path
}
