package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/session"
	"github.com/kakkoyun/af/internal/workstream"
)

// errExitCodesBoom is a test-only sentinel for the exitGeneral fallback row.
var errExitCodesBoom = errors.New("boom")

// TestExitCodeForError_MapsEveryADR068Row table-tests every row of the
// ADR-068 §2 / SPEC.md §15.3 exit-code contract.
func TestExitCodeForError_MapsEveryADR068Row(t *testing.T) {
	cobraUsageErr := cobra.ExactArgs(1)(nil, []string{})

	tests := []struct {
		err  error
		name string
		want int
	}{
		{name: "nil", err: nil, want: exitOK},
		{name: "generic unclassified", err: errExitCodesBoom, want: exitGeneral},
		{name: "cobra unknown-command/flag/argcount", err: cobraUsageErr, want: exitUsageCobra},
		{name: "af domain usage: note --append", err: fmt.Errorf("note: %w", errNoteAppendRequired), want: exitUsage},
		{name: "af domain usage: stack --parent", err: fmt.Errorf("stack: %w", errStackParentRequired), want: exitUsage},
		{name: "af domain usage: unsupported shell", err: fmt.Errorf("completions: %w", errUnsupportedShell), want: exitUsage},
		{name: "af domain usage: invalid session name", err: fmt.Errorf("AF_SESSION: %w", workstream.ErrInvalidSessionName), want: exitUsage},
		{name: "bad state.toml", err: fmt.Errorf("stack: %w", errStackNoState), want: exitNoInput},
		{name: "session resolution no input", err: errSessionResolutionNoInput, want: exitNoInput},
		{name: "proxy no state", err: errProxyNoState, want: exitNoInput},
		{name: "pr --refresh no PR", err: errPRRefreshNoPR, want: exitDataErr},
		{name: "pr --ai empty diff", err: errPRAIEmptyDiff, want: exitDataErr},
		{name: "review empty diff", err: errReviewEmptyDiff, want: exitDataErr},
		{name: "review empty body", err: errReviewEmptyBody, want: exitDataErr},
		{name: "external tool missing", err: fmt.Errorf("run gh: %w", exec.ErrNotFound), want: exitUnavailable},
		{name: "lock busy", err: fmt.Errorf("note: %w", session.ErrLockBusy), want: exitTempFail},
		{name: "permission denied", err: fmt.Errorf("open state: %w", os.ErrPermission), want: exitNoPerm},
		{name: "session picker interrupted", err: errSessionPickerInterrupted, want: exitInterrupted},
		{name: "context canceled", err: context.Canceled, want: exitInterrupted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exitCodeForError(tt.err); got != tt.want {
				t.Fatalf("exitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// TestExitCodeForError_LockBusyEndToEnd pre-holds a session's lock file
// in-process, then runs a mutating command (note --append, which uses
// withSessionLock) through the real cobra dispatch with a tiny
// AF_LOCK_TIMEOUT. The returned error must map to EX_TEMPFAIL (75) and
// carry the ADR-068 §4 retry hint.
func TestExitCodeForError_LockBusyEndToEnd(t *testing.T) {
	t.Setenv("AF_LOCK_TIMEOUT", "30ms")
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "busy", "feat/busy", "active")

	lockPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "busy", session.LockFileName)
	holder, err := session.LockFile(lockPath, session.LockExclusive)
	if err != nil {
		t.Fatalf("pre-hold LockFile: %v", err)
	}
	defer func() { _ = holder.Unlock() }() //nolint:errcheck // Best-effort unlock at test teardown.

	_, _, cmdErr := executeCommand(t, newRootCmd(), "note", "busy", "--append", "hello")
	if cmdErr == nil {
		t.Fatal("note --append with a pre-held lock returned nil, want a lock-busy error")
	}
	if !errors.Is(cmdErr, session.ErrLockBusy) {
		t.Fatalf("error = %v, want wrapped session.ErrLockBusy", cmdErr)
	}
	if got := exitCodeForError(cmdErr); got != exitTempFail {
		t.Fatalf("exitCodeForError(%v) = %d, want exitTempFail (%d)", cmdErr, got, exitTempFail)
	}
	if !strings.Contains(cmdErr.Error(), "retry shortly") {
		t.Fatalf("error = %v, want it to carry the retry hint", cmdErr)
	}
}

// TestHandlePanicOutput_FormatsAndReturnsExitSoftware pins the panic
// recovery contract added for ADR-068 §2 EX_SOFTWARE (70): a caught
// panic prints "panic: <value>" plus a stack trace to the given
// writer and reports exitSoftware for main to exit with.
func TestHandlePanicOutput_FormatsAndReturnsExitSoftware(t *testing.T) {
	var buf strings.Builder
	got := handlePanicOutput(&buf, "invariant violated")
	if got != exitSoftware {
		t.Fatalf("handlePanicOutput() = %d, want exitSoftware (%d)", got, exitSoftware)
	}
	output := buf.String()
	if !strings.HasPrefix(output, "panic: invariant violated\n") {
		t.Fatalf("output = %q, want it to start with %q", output, "panic: invariant violated\n")
	}
	if !strings.Contains(output, "goroutine") {
		t.Fatalf("output = %q, want it to contain a stack trace", output)
	}
}
