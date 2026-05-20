package main

import (
	"strings"
	"testing"
)

func TestSuspend_TransitionsActiveToSuspended(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "mywork", "feat/mywork", "active")

	stdout, _, err := executeCommand(t, newRootCmd(), "suspend", "mywork")
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if !strings.Contains(stdout, "suspended") {
		t.Fatalf("expected stdout to mention 'suspended'; got:\n%s", stdout)
	}
}

func TestResume_TransitionsSuspendedToActive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "mywork", "feat/mywork", "suspended")

	stdout, _, err := executeCommand(t, newRootCmd(), "resume", "mywork", "--bare")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(stdout, "active") {
		t.Fatalf("expected stdout to mention 'active'; got:\n%s", stdout)
	}
}
