package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/version"
)

func TestRunExecutesVersionCommand(t *testing.T) {
	withBuildInfo(t, "1.2.3", "abc1234", "2026-05-09")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(t.Context(), []string{"af", "version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}

	want := "af 1.2.3 (abc1234, 2026-05-09)\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}

	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty output", got)
	}
}

func TestRunReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := run(ctx, []string{"af", "version"}, io.Discard, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
}

func TestRunRequiresOutputWriters(t *testing.T) {
	tests := []struct {
		stdout io.Writer
		stderr io.Writer
		name   string
	}{
		{name: "nil stdout", stdout: nil, stderr: io.Discard},
		{name: "nil stderr", stdout: io.Discard, stderr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(t.Context(), []string{"af", "version"}, tt.stdout, tt.stderr)
			if !errors.Is(err, errNilOutput) {
				t.Fatalf("run() error = %v, want %v", err, errNilOutput)
			}
		})
	}
}

func TestRunReturnsCommandError(t *testing.T) {
	err := run(t.Context(), []string{"af", "--definitely-not-a-flag"}, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("run() error = nil, want an unknown-flag error")
	}

	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("run() error = %v, want unknown flag", err)
	}
}

func withBuildInfo(t *testing.T, v, commit, date string) {
	t.Helper()

	oldVersion := version.Version
	oldCommit := version.Commit
	oldDate := version.Date
	t.Cleanup(func() {
		version.Version = oldVersion
		version.Commit = oldCommit
		version.Date = oldDate
	})

	version.Version = v
	version.Commit = commit
	version.Date = date
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
