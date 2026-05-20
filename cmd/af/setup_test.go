package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetup_RunsEndToEndAgainstTempHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	stdout, stderr, err := executeCommand(t, newRootCmd(), "setup", "--skip-gitignore")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v; stderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	for _, want := range []string{
		"af setup complete:",
		"state dir:",
		"user config:",
		"completions:",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("setup output missing %q; full output:\n%s", want, stdout)
		}
	}

	stateDir := filepath.Join(home, ".local", "share", "af", "v1")
	for _, sub := range []string{"sessions", "archive", "secrets"} {
		_, statErr := os.Stat(filepath.Join(stateDir, sub))
		if statErr != nil {
			t.Fatalf("expected state dir %s missing: %v", sub, statErr)
		}
	}

	configPath := filepath.Join(home, ".config", "af", "config.toml")
	_, configStatErr := os.Stat(configPath)
	if configStatErr != nil {
		t.Fatalf("user config not written: %v", configStatErr)
	}
}

func TestSetup_SkipFlagsHonoured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	_, stderr, err := executeCommand(t, newRootCmd(), "setup", "--skip-gitignore", "--skip-completions")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v; stderr=%q", err, stderr)
	}

	_, gitignoreStatErr := os.Stat(filepath.Join(home, ".config", "git", "ignore"))
	if gitignoreStatErr == nil {
		t.Fatal("--skip-gitignore did not skip gitignore write")
	}
	zshCompletion := filepath.Join(home, ".config", "zsh", "completions", "_af")
	_, zshStatErr := os.Stat(zshCompletion)
	if zshStatErr == nil {
		t.Fatal("--skip-completions did not skip completion install")
	}
}
