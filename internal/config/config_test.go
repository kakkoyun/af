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

// wantNotesSubfolderModeRepo is the compiled [obsidian]
// notes_subfolder_mode default (issue #34).
const wantNotesSubfolderModeRepo = "repo"

// TestObsidianConfig_NotesSubfolderModeDefaultsToRepo guards issue #34:
// an absent notes_subfolder_mode key must resolve to the compiled
// "repo" default, both with no user config at all and with a user
// config that omits the key.
func TestObsidianConfig_NotesSubfolderModeDefaultsToRepo(t *testing.T) {
	home := t.TempDir()

	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: filepath.Join(home, ".config", "af", "missing.toml"),
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	if cfg.Obsidian.NotesSubfolderMode != wantNotesSubfolderModeRepo {
		t.Fatalf("NotesSubfolderMode = %q, want repo (no config file)", cfg.Obsidian.NotesSubfolderMode)
	}

	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[obsidian]
notes_vault = "personal"
`)
	cfg, err = config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	if cfg.Obsidian.NotesSubfolderMode != wantNotesSubfolderModeRepo {
		t.Fatalf("NotesSubfolderMode = %q, want repo (key absent from config)", cfg.Obsidian.NotesSubfolderMode)
	}
}

// TestObsidianConfig_NotesSubfolderModeAcceptsFlat guards the issue #34
// opt-out: an explicit "flat" value must round-trip unchanged.
func TestObsidianConfig_NotesSubfolderModeAcceptsFlat(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[obsidian]
notes_subfolder_mode = "flat"
`)

	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	if cfg.Obsidian.NotesSubfolderMode != "flat" {
		t.Fatalf("NotesSubfolderMode = %q, want flat", cfg.Obsidian.NotesSubfolderMode)
	}
}

// TestObsidianConfig_RejectsBadNotesSubfolderMode guards issue #34: any
// value other than "repo"/"flat" is a config validation error,
// consistent with how the config package reports other bad enum
// values (e.g. sandbox.default_provider).
func TestObsidianConfig_RejectsBadNotesSubfolderMode(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[obsidian]
notes_subfolder_mode = "nested"
`)

	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want error for bad notes_subfolder_mode")
	}
	if !strings.Contains(err.Error(), "notes_subfolder_mode") {
		t.Fatalf("error %q does not mention notes_subfolder_mode", err)
	}
}

func TestSandboxConfig_RejectsSBXProvider(t *testing.T) {
	home := t.TempDir()
	userCfgPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userCfgPath, `
schema_version = 1
[sandbox]
default_provider = "sbx"
`)

	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: userCfgPath,
	})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want error for sbx provider")
	}
	if !strings.Contains(err.Error(), "sbx") {
		t.Fatalf("error %q does not mention sbx", err)
	}
}

func TestSandboxConfig_AcceptsSlicerProvider(t *testing.T) {
	home := t.TempDir()
	userCfgPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userCfgPath, `
schema_version = 1
[sandbox]
default_provider = "slicer"
`)

	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: userCfgPath,
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v, want no error for slicer provider", err)
	}
	if cfg.Sandbox.DefaultProvider != "slicer" {
		t.Fatalf("DefaultProvider = %q, want slicer", cfg.Sandbox.DefaultProvider)
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

// ---------------------------------------------------------------------------
// ADR-062: [sandbox.slicer.resources] config tests
// ---------------------------------------------------------------------------

func TestSlicerResources_AcceptsValid(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[sandbox.slicer.resources]
name         = "tight"
vcpu         = 2
ram_gb       = 4
storage_size = "25G"
gpu_count    = 0
hypervisor   = "firecracker"
`)
	cfg, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}
	got := cfg.Sandbox.Slicer.Resources
	if got.Name != "tight" {
		t.Errorf("Name = %q, want tight", got.Name)
	}
	if got.VCPU != 2 {
		t.Errorf("VCPU = %d, want 2", got.VCPU)
	}
	if got.RAMGB != 4 {
		t.Errorf("RAMGB = %d, want 4", got.RAMGB)
	}
	if got.StorageSize != "25G" {
		t.Errorf("StorageSize = %q, want 25G", got.StorageSize)
	}
	if got.GPUCount != 0 {
		t.Errorf("GPUCount = %d, want 0", got.GPUCount)
	}
	if got.Hypervisor != "firecracker" {
		t.Errorf("Hypervisor = %q, want firecracker", got.Hypervisor)
	}
}

func TestSlicerResources_RejectsNegativeVCPU(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[sandbox.slicer.resources]
vcpu = -1
`)
	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want error for negative vcpu")
	}
	if !strings.Contains(err.Error(), "vcpu") && !strings.Contains(err.Error(), ">= 0") {
		t.Fatalf("error %q does not mention vcpu or >= 0", err)
	}
}

func TestSlicerResources_RejectsBadStorageSize(t *testing.T) {
	for _, bad := range []string{"25GB", "abc", "25 G"} {
		t.Run(bad, func(t *testing.T) {
			home := t.TempDir()
			userPath := filepath.Join(home, ".config", "af", "config.toml")
			writeFile(t, userPath, "schema_version = 1\n\n[sandbox.slicer.resources]\nstorage_size = \""+bad+"\"\n")
			_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
			if err == nil {
				t.Fatalf("LoadWithOptions() error = nil, want storage_size error for %q", bad)
			}
			if !strings.Contains(err.Error(), "storage_size") {
				t.Fatalf("error %q does not mention storage_size", err)
			}
		})
	}
}

func TestSlicerResources_RejectsUnknownHypervisor(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[sandbox.slicer.resources]
hypervisor = "xhyve"
`)
	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want error for unknown hypervisor")
	}
	if !strings.Contains(err.Error(), "hypervisor") {
		t.Fatalf("error %q does not mention hypervisor", err)
	}
}

func TestSlicerResources_RejectsGroupAndResourcesCombined(t *testing.T) {
	home := t.TempDir()
	userPath := filepath.Join(home, ".config", "af", "config.toml")
	writeFile(t, userPath, `
schema_version = 1

[sandbox.slicer]
group = "my-existing-group"

[sandbox.slicer.resources]
vcpu = 4
`)
	_, err := config.LoadWithOptions(context.Background(), config.LoadOptions{UserConfigPath: userPath})
	if err == nil {
		t.Fatal("LoadWithOptions() error = nil, want error for group + resources conflict")
	}
	if !strings.Contains(err.Error(), "group") && !strings.Contains(err.Error(), "resource") {
		t.Fatalf("error %q does not mention group/resource conflict", err)
	}
}
