package config_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/config"
)

const agentClaude = "claude"

// TestControlConfig_AcceptsValid verifies that all valid control field values
// load without error.
func TestControlConfig_AcceptsValid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeControlFile(t, userPath, `
schema_version = 1

[control]
agent          = "claude"
approval_mode  = "auto"
sandbox        = "slicer"
remote         = "work-mini"
remote_control = "superterm"
max_agents     = 2
`)
	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v, want nil", err)
	}
	if cfg.Control.Agent != agentClaude {
		t.Errorf("Control.Agent = %q, want claude", cfg.Control.Agent)
	}
	if cfg.Control.ApprovalMode != "auto" {
		t.Errorf("Control.ApprovalMode = %q, want auto", cfg.Control.ApprovalMode)
	}
	if cfg.Control.Sandbox != "slicer" {
		t.Errorf("Control.Sandbox = %q, want slicer", cfg.Control.Sandbox)
	}
	if cfg.Control.Remote != "work-mini" {
		t.Errorf("Control.Remote = %q, want work-mini", cfg.Control.Remote)
	}
	if cfg.Control.RemoteControl != "superterm" {
		t.Errorf("Control.RemoteControl = %q, want superterm", cfg.Control.RemoteControl)
	}
	if cfg.Control.MaxAgents != 2 {
		t.Errorf("Control.MaxAgents = %d, want 2", cfg.Control.MaxAgents)
	}
}

// TestControlConfig_RejectsUnknownSandbox verifies that an invalid sandbox
// value produces a parse error.
func TestControlConfig_RejectsUnknownSandbox(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeControlFile(t, userPath, `
schema_version = 1

[control]
sandbox = "sbx"
`)
	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want sandbox validation error")
	}
	if !strings.Contains(err.Error(), "control.sandbox") {
		t.Fatalf("LoadWithOptions() error = %v, want control.sandbox in message", err)
	}
}

// TestControlConfig_RejectsUnknownRemoteControl verifies that an invalid
// remote_control value produces a parse error.
func TestControlConfig_RejectsUnknownRemoteControl(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeControlFile(t, userPath, `
schema_version = 1

[control]
remote_control = "vnc"
`)
	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want remote_control validation error")
	}
	if !strings.Contains(err.Error(), "control.remote_control") {
		t.Fatalf("LoadWithOptions() error = %v, want control.remote_control in message", err)
	}
}

// TestControlConfig_RejectsRemoteWithShellMeta verifies that shell
// metacharacters in control.remote are rejected at parse time.
func TestControlConfig_RejectsRemoteWithShellMeta(t *testing.T) {
	for _, meta := range []string{";", "|", "&", "`", "$", "<", ">"} {
		t.Run("meta="+meta, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			userPath := filepath.Join(home, ".config", "af", "config.toml")
			writeControlFile(t, userPath, "schema_version = 1\n\n[control]\nremote = \"host"+meta+"evil\"\n")
			_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
			if err == nil {
				t.Fatalf("LoadWithOptions() error = nil for meta %q, want shell-meta error", meta)
			}
			if !strings.Contains(err.Error(), "control.remote") {
				t.Errorf("LoadWithOptions() error = %v, want control.remote in message", err)
			}
		})
	}
}

// TestControlConfig_RepoOverridesUser verifies that repo [control] wins over
// user [control] per ADR-061 precedence.
func TestControlConfig_RepoOverridesUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	repoDir := filepath.Join(home, "repo")

	writeControlFile(t, userPath, `
schema_version = 1

[control]
agent = "pi"
max_agents = 1
`)
	writeControlFile(t, filepath.Join(repoDir, ".af", "config.toml"), `
schema_version = 1

[control]
agent = "claude"
max_agents = 3
`)
	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: userPath,
		RepoDir:        repoDir,
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v, want nil", err)
	}
	if cfg.Control.Agent != agentClaude {
		t.Errorf("Control.Agent = %q, want claude (repo overrides user)", cfg.Control.Agent)
	}
	if cfg.Control.MaxAgents != 3 {
		t.Errorf("Control.MaxAgents = %d, want 3 (repo overrides user)", cfg.Control.MaxAgents)
	}
}

// TestControlConfig_RejectsUnknownApprovalMode verifies that an invalid
// approval_mode value produces a parse error.
func TestControlConfig_RejectsUnknownApprovalMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeControlFile(t, userPath, `
schema_version = 1

[control]
approval_mode = "unsafe"
`)
	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want approval_mode validation error")
	}
	if !strings.Contains(err.Error(), "control.approval_mode") {
		t.Fatalf("LoadWithOptions() error = %v, want control.approval_mode in message", err)
	}
}

// writeControlFile is a local helper that creates a config TOML at path.
func writeControlFile(t *testing.T, path, content string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	err = os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write test file %s: %v", path, err)
	}
}
