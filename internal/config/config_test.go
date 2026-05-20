package config_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/config"
)

func TestLoad_UsesDefaults_WhenFilesAreMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: filepath.Join(home, ".config", "af", "missing.toml"),
		RepoDir:        filepath.Join(home, "repo"),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}

	if cfg.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", cfg.SchemaVersion)
	}
	if cfg.General.DefaultAgent != "pi" {
		t.Fatalf("DefaultAgent = %q, want pi", cfg.General.DefaultAgent)
	}
	if cfg.General.Multiplexer != "tmux" {
		t.Fatalf("Multiplexer = %q, want tmux", cfg.General.Multiplexer)
	}
	wantWorktreeRoot := filepath.Join(home, "Workspace", ".worktrees")
	if cfg.General.WorktreeRoot != wantWorktreeRoot {
		t.Fatalf("WorktreeRoot = %q, want %q", cfg.General.WorktreeRoot, wantWorktreeRoot)
	}
	if got := strings.Join(cfg.Diff.Command.Argv, " "); got != "git diff {base}...HEAD" {
		t.Fatalf("Diff.Command.Argv = %q, want default git diff", got)
	}
	if got := strings.Join(cfg.PR.Command.Argv, " "); got != "gh pr create --base {base} --head {head}" {
		t.Fatalf("PR.Command.Argv = %q, want default gh pr create", got)
	}
	if got := strings.Join(cfg.PR.FlagTemplate["title"], " "); got != "--title {title}" {
		t.Fatalf("PR.FlagTemplate[title] = %q, want --title token", got)
	}
	if cfg.Secret.KeyringService != "af" {
		t.Fatalf("Secret.KeyringService = %q, want af", cfg.Secret.KeyringService)
	}
}

func TestLoad_MergesUserAndRepoLayers_WhenRepoOverridesFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	repoDir := filepath.Join(home, "repo")
	writeFile(t, userPath, `
schema_version = 1

[general]
default_agent = "claude"
worktree_root = "~/custom-worktrees"

[branch]
prefix = "user"

[diff]
cmd = ["git", "diff", "user-base"]
`)
	writeFile(t, filepath.Join(repoDir, ".af", "config.toml"), `
schema_version = 1

[general]
max_sessions = 3

[branch]
prefix = "repo"

[diff]
cmd = ["delta", "{base}"]

[pr]
shell = true
cmd = "gh pr create --fill --head {head}"
`)

	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: userPath,
		RepoDir:        repoDir,
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}

	assertMergedUserRepoConfig(t, cfg, home)
}

func assertMergedUserRepoConfig(t *testing.T, cfg config.Config, home string) {
	t.Helper()
	if cfg.General.DefaultAgent != "claude" {
		t.Fatalf("DefaultAgent = %q, want user layer", cfg.General.DefaultAgent)
	}
	if cfg.General.MaxSessions != 3 {
		t.Fatalf("MaxSessions = %d, want repo layer", cfg.General.MaxSessions)
	}
	if cfg.Branch.Prefix != "repo" {
		t.Fatalf("Branch.Prefix = %q, want repo layer", cfg.Branch.Prefix)
	}
	wantWorktreeRoot := filepath.Join(home, "custom-worktrees")
	if cfg.General.WorktreeRoot != wantWorktreeRoot {
		t.Fatalf("WorktreeRoot = %q, want %q", cfg.General.WorktreeRoot, wantWorktreeRoot)
	}
	if got := strings.Join(cfg.Diff.Command.Argv, " "); got != "delta {base}" {
		t.Fatalf("Diff.Command.Argv = %q, want repo command", got)
	}
	if !cfg.PR.Command.Shell || cfg.PR.Command.Script != "gh pr create --fill --head {head}" {
		t.Fatalf("PR.Command = %#v, want shell command from repo", cfg.PR.Command)
	}
}

func TestLoad_IgnoresGlobalOnlySections_WhenTheyAppearInRepoConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	repoDir := filepath.Join(home, "repo")
	writeFile(t, userPath, `
schema_version = 1

[obsidian.vaults]
personal = "~/Vaults/personal"

[secret]
keyring_service = "user-service"
redact_keys = ["user_token"]
`)
	writeFile(t, filepath.Join(repoDir, ".af", "config.toml"), `
schema_version = 1

[obsidian]
notes_folder = "repo notes"

[obsidian.vaults]
personal = "/tmp/repo-override"
work = "/tmp/work"

[secret]
keyring_service = "repo-service"
redact_keys = ["repo_token"]
`)

	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: userPath,
		RepoDir:        repoDir,
		Logger:         slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}

	if cfg.Obsidian.NotesFolder != "repo notes" {
		t.Fatalf("NotesFolder = %q, want repo override", cfg.Obsidian.NotesFolder)
	}
	wantVault := filepath.Join(home, "Vaults", "personal")
	if cfg.Obsidian.Vaults["personal"] != wantVault {
		t.Fatalf("Vault personal = %q, want %q", cfg.Obsidian.Vaults["personal"], wantVault)
	}
	if _, ok := cfg.Obsidian.Vaults["work"]; ok {
		t.Fatal("repo-only Obsidian vault was merged, want global-only section ignored")
	}
	if cfg.Secret.KeyringService != "user-service" {
		t.Fatalf("KeyringService = %q, want user value", cfg.Secret.KeyringService)
	}
	if got := strings.Join(cfg.Secret.RedactKeys, ","); got != "user_token" {
		t.Fatalf("RedactKeys = %q, want user-only value", got)
	}
}

func TestLoad_RejectsHigherSchemaVersion(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `schema_version = 2`)

	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want unsupported schema error")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("LoadWithOptions() error = %v, want schema_version context", err)
	}
}

func TestLoad_RejectsProxyCommandShape_WhenShellModeAndCmdTypeDisagree(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[diff]
shell = true
cmd = ["git", "diff"]
`)

	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want proxy command shape error")
	}
	if !strings.Contains(err.Error(), "diff.cmd") {
		t.Fatalf("LoadWithOptions() error = %v, want diff.cmd context", err)
	}
}

func writeFile(t *testing.T, path, content string) {
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
