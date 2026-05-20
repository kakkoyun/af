package main

import (
	"testing"
)

func TestEditor_FailsWithoutConfiguredEditor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EDITOR", "")
	writeTestSessionState(t, home, "editwork", "feat/editwork", "active")

	// No config file exists, so defaults apply: terminal = "$EDITOR".
	// With EDITOR unset, the resolved target expands to "", which causes
	// exec.Command to fail. The test verifies that af editor returns a
	// non-nil error when no usable editor binary is available.
	_, _, err := executeCommand(t, newRootCmd(), "editor", "editwork")
	if err == nil {
		t.Fatal("editor without configured editor returned nil, want error")
	}
}
