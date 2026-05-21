package config_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/config"
)

func TestRender_EmitsSchemaVersionAndAllSections(t *testing.T) {
	cfg := config.Defaults()
	cfg.General.WorktreeRoot = "/abs/worktrees"
	cfg.Obsidian.Vaults = map[string]string{
		"personal": "/Users/owner/Vaults/personal",
		"work":     "/Users/owner/Vaults/work",
	}

	got := config.Render(cfg)

	mustContain(t, got, "schema_version = 1")
	for _, section := range []string{
		"[general]",
		"[branch]",
		"[editor]",
		"[diff]",
		"[pr]",
		"[remote]",
		"[sandbox]",
		"[sandbox.slicer]",
		"[obsidian]",
		"[obsidian.vaults]",
		"[doctor]",
		"[secret]",
		"[status]",
		"[lifecycle]",
	} {
		mustContain(t, got, section)
	}
}

func TestRender_EmitsArgvCommandsAsTomlArrays(t *testing.T) {
	cfg := config.Defaults()

	got := config.Render(cfg)

	mustContain(t, got, `cmd = ["git", "diff", "{base}...HEAD"]`)
	mustContain(t, got, `cmd = ["gh", "pr", "create", "--base", "{base}", "--head", "{head}"]`)
	mustContain(t, got, "shell = false")
}

func TestRender_EmitsShellCommandsAsQuotedStrings(t *testing.T) {
	cfg := config.Defaults()
	cfg.PR.Command = config.ProxyCommandConfig{
		Script: `gh pr create --title "{title}" --body {body}`,
		Shell:  true,
	}

	got := config.Render(cfg)

	mustContain(t, got, `shell = true`)
	mustContain(t, got, `cmd = "gh pr create --title \"{title}\" --body {body}"`)
}

func TestRender_EmitsObsidianVaultsSortedByKey(t *testing.T) {
	cfg := config.Defaults()
	cfg.Obsidian.Vaults = map[string]string{
		"work":     "/vault/work",
		"personal": "/vault/personal",
		"side":     "/vault/side",
	}

	got := config.Render(cfg)

	personal := strings.Index(got, `personal = "/vault/personal"`)
	side := strings.Index(got, `side = "/vault/side"`)
	work := strings.Index(got, `work = "/vault/work"`)

	if personal < 0 || side < 0 || work < 0 {
		t.Fatalf("missing vault entries in:\n%s", got)
	}
	if personal >= side || side >= work {
		t.Fatalf("vault entries not sorted; offsets personal=%d side=%d work=%d in:\n%s", personal, side, work, got)
	}
}

func TestRender_RoundTrips_ThroughLoad(t *testing.T) {
	cfg := config.Defaults()
	cfg.General.DefaultAgent = "claude"
	cfg.General.WorktreeRoot = "/tmp/worktrees"
	cfg.Branch.Prefix = "kakkoyun"
	cfg.Branch.PrefixOnForkOnly = false
	cfg.Obsidian.NotesVault = "personal"
	cfg.Obsidian.Vaults = map[string]string{"personal": "/vault/personal"}
	cfg.Doctor.ExtraTools = []string{"jq", "fzf"}
	cfg.Secret.RedactKeys = []string{"slack_webhook"}

	rendered := config.Render(cfg)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, rendered)

	roundTripped, err := config.LoadWithOptions(context.Background(), config.LoadOptions{
		UserConfigPath: path,
	})
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v; rendered:\n%s", err, rendered)
	}

	assertRoundTrip(t, cfg, roundTripped)
}

func assertRoundTrip(t *testing.T, want, got config.Config) {
	t.Helper()

	if want.SchemaVersion != got.SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, want.SchemaVersion)
	}
	if want.General.DefaultAgent != got.General.DefaultAgent {
		t.Fatalf("DefaultAgent = %q, want %q", got.General.DefaultAgent, want.General.DefaultAgent)
	}
	if want.General.WorktreeRoot != got.General.WorktreeRoot {
		t.Fatalf("WorktreeRoot = %q, want %q", got.General.WorktreeRoot, want.General.WorktreeRoot)
	}
	if want.Branch.Prefix != got.Branch.Prefix {
		t.Fatalf("Branch.Prefix = %q, want %q", got.Branch.Prefix, want.Branch.Prefix)
	}
	if want.Branch.PrefixOnForkOnly != got.Branch.PrefixOnForkOnly {
		t.Fatalf("PrefixOnForkOnly = %v, want %v", got.Branch.PrefixOnForkOnly, want.Branch.PrefixOnForkOnly)
	}
	if want.Obsidian.NotesVault != got.Obsidian.NotesVault {
		t.Fatalf("NotesVault = %q, want %q", got.Obsidian.NotesVault, want.Obsidian.NotesVault)
	}
	if want.Obsidian.Vaults["personal"] != got.Obsidian.Vaults["personal"] {
		t.Fatalf("Vaults[personal] = %q, want %q", got.Obsidian.Vaults["personal"], want.Obsidian.Vaults["personal"])
	}
	if strings.Join(want.Doctor.ExtraTools, ",") != strings.Join(got.Doctor.ExtraTools, ",") {
		t.Fatalf("ExtraTools = %v, want %v", got.Doctor.ExtraTools, want.Doctor.ExtraTools)
	}
	if strings.Join(want.Secret.RedactKeys, ",") != strings.Join(got.Secret.RedactKeys, ",") {
		t.Fatalf("RedactKeys = %v, want %v", got.Secret.RedactKeys, want.Secret.RedactKeys)
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output does not contain %q\noutput:\n%s", needle, haystack)
	}
}
