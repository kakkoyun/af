package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigHelpListsInitAndShowSubcommands(t *testing.T) {
	stdout, stderr, err := executeCommand(t, newRootCmd(), "config", "--help")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v, want nil", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty output", stderr)
	}
	for _, want := range []string{"init", "show"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help output %q missing subcommand %q", stdout, want)
		}
	}
}

func TestConfigInit_WritesUserTemplate_ToDefaultPathUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout, stderr, err := executeCommand(t, newRootCmd(), "config", "init")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v\nstderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	wantPath := filepath.Join(home, ".config", "af", "config.toml")
	if !strings.Contains(stdout, wantPath) {
		t.Fatalf("stdout %q does not mention default path %q", stdout, wantPath)
	}

	body, err := os.ReadFile(wantPath) //nolint:gosec // wantPath is constructed under t.TempDir().
	if err != nil {
		t.Fatalf("read default config: %v", err)
	}
	if !strings.Contains(string(body), "schema_version = 1") {
		t.Fatalf("default config missing schema_version; got:\n%s", body)
	}
}

func TestConfigInit_HonorsConfigFlag_ForExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom", "config.toml")

	_, stderr, err := executeCommand(t, newRootCmd(), "--config", path, "config", "init")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v\nstderr=%q", err, stderr)
	}

	_, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat custom config: %v", statErr)
	}
}

func TestConfigInit_RefusesToOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	seedErr := os.WriteFile(path, []byte("schema_version = 1\n"), 0o600)
	if seedErr != nil {
		t.Fatalf("seed config: %v", seedErr)
	}

	_, _, err := executeCommand(t, newRootCmd(), "--config", path, "config", "init")
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want overwrite refusal")
	}
	if !strings.Contains(err.Error(), "config init") {
		t.Fatalf("error %q does not mention command context", err)
	}
}

func TestConfigShow_PrintsEffectiveMergedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	mkErr := os.MkdirAll(filepath.Dir(userPath), 0o750)
	if mkErr != nil {
		t.Fatalf("mkdir user config dir: %v", mkErr)
	}
	userBody := `
schema_version = 1

[general]
default_agent = "claude"
worktree_root = "/tmp/owner/worktrees"

[obsidian.vaults]
personal = "/tmp/owner/Vaults/personal"
`
	writeErr := os.WriteFile(userPath, []byte(userBody), 0o600)
	if writeErr != nil {
		t.Fatalf("write user config: %v", writeErr)
	}

	stdout, stderr, err := executeCommand(t, newRootCmd(), "config", "show")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v\nstderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	for _, marker := range []string{
		"schema_version = 1",
		`default_agent = "claude"`,
		`worktree_root = "/tmp/owner/worktrees"`,
		`personal = "/tmp/owner/Vaults/personal"`,
	} {
		if !strings.Contains(stdout, marker) {
			t.Fatalf("config show missing %q; full output:\n%s", marker, stdout)
		}
	}
}

func TestConfigShow_FallsBackToDefaults_WhenNoConfigFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout, _, err := executeCommand(t, newRootCmd(), "config", "show")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	for _, marker := range []string{
		"schema_version = 1",
		`default_agent = "pi"`,
		`multiplexer = "tmux"`,
	} {
		if !strings.Contains(stdout, marker) {
			t.Fatalf("config show missing default %q; full output:\n%s", marker, stdout)
		}
	}
}
