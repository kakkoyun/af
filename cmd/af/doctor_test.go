package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/testutil"
)

func TestDoctor_LocalReportsMissingRequiredTools(t *testing.T) {
	// Empty PATH means everything is missing.
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("HOME", t.TempDir())

	stdout, _, err := executeCommand(t, newRootCmd(), "doctor")
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want missing-tool error")
	}
	for _, want := range []string{"Local environment:", "✗ git", "Status: "} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q; full output:\n%s", want, stdout)
		}
	}
}

func TestDoctor_LocalPassesWhenAllMustToolsPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bin := filepath.Join(home, "bin")
	for _, name := range []string{"git", "tmux", "pi"} {
		testutil.WriteExecutable(t, bin, name, "echo "+name+" version test")
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	stdout, _, err := executeCommand(t, newRootCmd(), "doctor")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v; stdout:\n%s", err, stdout)
	}
	for _, want := range []string{"✓ git", "✓ tmux", "✓ pi", "all required tools present"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q; full output:\n%s", want, stdout)
		}
	}
}

func TestDoctor_RemoteUsesSSHHostInHeading(t *testing.T) {
	// Default ssh is shadowed by the testscript fakebin in our testscript
	// scenarios. For this unit test we just want to confirm the heading
	// reflects the host and that an error surfaces when ssh probes fail.
	t.Setenv("HOME", t.TempDir())

	// Send to a clearly bogus host. ssh will fail; the doctor reports
	// every probe as missing, which is the expected behaviour.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = ctx
	stdout, _, err := executeCommand(t, newRootCmd(), "--config", filepath.Join(t.TempDir(), "missing.toml"), "doctor", "--remote", "test-host-xyz")
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want missing-tool error")
	}
	if !strings.Contains(stdout, "Remote environment (test-host-xyz):") {
		t.Fatalf("doctor --remote heading missing host; output:\n%s", stdout)
	}
}

func TestDoctor_LocalReportsObsidianVaultAccessibility(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	bin := filepath.Join(home, "bin")
	for _, name := range []string{"git", "tmux", "pi"} {
		testutil.WriteExecutable(t, bin, name, "echo "+name+" version test")
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	goodVault := filepath.Join(home, "Vaults", "personal")
	missingVault := filepath.Join(home, "Vaults", "missing")
	err := os.MkdirAll(goodVault, 0o750)
	if err != nil {
		t.Fatalf("mkdir good vault: %v", err)
	}
	configPath := filepath.Join(home, ".config", "af", "config.toml")
	err = os.MkdirAll(filepath.Dir(configPath), 0o750)
	if err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	body := "schema_version = 1\n\n[obsidian.vaults]\npersonal = \"" + goodVault + "\"\nmissing = \"" + missingVault + "\"\n"
	err = os.WriteFile(configPath, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\nstdout:\n%s", err, stdout)
	}
	for _, want := range []string{
		"✓ obsidian:personal",
		goodVault,
		"⚠ obsidian:missing",
		"update [obsidian.vaults].missing",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output missing %q; full output:\n%s", want, stdout)
		}
	}
}
