package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/workstream"
)

// TestRootCommand_RejectsInvalidAFSessionEnv pins the issue #15 fix:
// a malformed AF_SESSION must fail EVERY af invocation up front with
// "invalid session name", not just the commands that happen to consume
// the variable. Before the fix, `AF_SESSION='../../etc' af list`
// silently ignored the traversal name and exited 0.
func TestRootCommand_RejectsInvalidAFSessionEnv(t *testing.T) {
	for _, name := range []string{"../../etc", "/abs/path", "a/../b", "-flag"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AF_SESSION", name)
			t.Setenv("HOME", t.TempDir())

			_, _, err := executeCommand(t, newRootCmd(), "list")
			if !errors.Is(err, workstream.ErrInvalidSessionName) {
				t.Fatalf("err = %v, want ErrInvalidSessionName", err)
			}
			if !strings.Contains(err.Error(), "AF_SESSION") {
				t.Fatalf("error should name the offending source (AF_SESSION): %v", err)
			}
		})
	}
}

// TestRootCommand_AcceptsValidAFSessionEnv guards against the #15 fix
// over-rejecting: a well-formed AF_SESSION must not break commands that
// do not consume it.
func TestRootCommand_AcceptsValidAFSessionEnv(t *testing.T) {
	t.Setenv("AF_SESSION", "perfectly-legal-name")
	t.Setenv("HOME", t.TempDir())

	stdout, _, err := executeCommand(t, newRootCmd(), "list")
	if err != nil {
		t.Fatalf("af list with valid AF_SESSION: %v", err)
	}
	if !strings.Contains(stdout, "no workstreams") {
		t.Fatalf("unexpected list output:\n%s", stdout)
	}
}
