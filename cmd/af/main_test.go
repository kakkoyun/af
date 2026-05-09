package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func TestRunReturnsScaffoldError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run(t.Context(), []string{"af"}, &stdout, &stderr)
	if !errors.Is(err, errScaffoldOnly) {
		t.Fatalf("run() error = %v, want %v", err, errScaffoldOnly)
	}

	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty output", got)
	}

	if got := stderr.String(); got == "" {
		t.Fatal("stderr is empty, want a diagnostic for the scaffold-only binary")
	}
}

func TestRunReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := run(ctx, []string{"af"}, io.Discard, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
}

func TestRunRequiresOutputWriters(t *testing.T) {
	tests := []struct {
		name   string
		stdout io.Writer
		stderr io.Writer
	}{
		{name: "nil stdout", stdout: nil, stderr: io.Discard},
		{name: "nil stderr", stdout: io.Discard, stderr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(t.Context(), []string{"af"}, tt.stdout, tt.stderr)
			if !errors.Is(err, errNilOutput) {
				t.Fatalf("run() error = %v, want %v", err, errNilOutput)
			}
		})
	}
}

func TestRunWrapsDiagnosticWriteError(t *testing.T) {
	want := errors.New("write failed")

	err := run(t.Context(), []string{"af"}, io.Discard, failingWriter{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("run() error = %v, want wrapped %v", err, want)
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
