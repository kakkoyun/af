package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/session"
)

func TestStack_RequiresParentFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "beta", "feat/beta", "active")

	_, _, err := executeCommand(t, newRootCmd(), "stack", "beta")
	if err == nil {
		t.Fatal("stack without --parent returned nil, want error")
	}
	if !errors.Is(err, errStackParentRequired) {
		if !strings.Contains(err.Error(), "--parent required") {
			t.Fatalf("error %v does not mention --parent required", err)
		}
	}
}

func TestStack_SetsParentInState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "alpha", "feat/alpha", "active")
	writeTestSessionState(t, home, "beta", "feat/beta", "active")

	_, _, err := executeCommand(t, newRootCmd(), "stack", "beta", "--parent", "alpha")
	if err != nil {
		t.Fatalf("stack: %v", err)
	}

	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "beta", "state.toml")
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if state.Stack.ParentSession != "alpha" {
		t.Fatalf("Stack.ParentSession = %q, want %q", state.Stack.ParentSession, "alpha")
	}
}

func TestUnstack_ClearsParent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "alpha", "feat/alpha", "active")
	writeTestSessionState(t, home, "beta", "feat/beta", "active")

	_, _, err := executeCommand(t, newRootCmd(), "stack", "beta", "--parent", "alpha")
	if err != nil {
		t.Fatalf("stack: %v", err)
	}

	_, _, err = executeCommand(t, newRootCmd(), "unstack", "beta")
	if err != nil {
		t.Fatalf("unstack: %v", err)
	}

	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "beta", "state.toml")
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState after unstack: %v", err)
	}
	if state.Stack.ParentSession != "" {
		t.Fatalf("Stack.ParentSession = %q after unstack, want empty", state.Stack.ParentSession)
	}
}
