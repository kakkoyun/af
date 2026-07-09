package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/obsidian"
)

// writeFile writes content to path, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	err = os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// newFakeCreateContext wires a createContext whose git seam returns just
// enough canned output for `af create` to reach lifecycle.Create, and
// whose mux/notes seams are in-memory fakes so no tmux/filesystem
// side effects escape the test.
func newFakeCreateContext(t *testing.T, gitRoot string) *createContext {
	t.Helper()

	fakeGit := git.NewFakeRunner()
	fakeGit.SetResponse([]string{"rev-parse", "--show-toplevel"}, git.FakeResponse{Output: gitRoot})

	return &createContext{
		git:   fakeGit,
		mux:   mux.NewFakeMultiplexer(),
		notes: obsidian.NewMemoryStore(),
		getwd: func() (string, error) { return gitRoot, nil },
	}
}

// withFakeCreateContext overrides newCreateContextOverride for the
// duration of the test and restores it on cleanup.
func withFakeCreateContext(t *testing.T, cc *createContext) {
	t.Helper()

	prior := newCreateContextOverride
	newCreateContextOverride = func(*rootOptions) *createContext { return cc }
	t.Cleanup(func() { newCreateContextOverride = prior })
}

// TestCreate_WarnsOnStderrOnce_WhenNotesVaultEmpty guards issue #17
// Option 2: `af create` must not silently skip the Obsidian note step
// when [obsidian] notes_vault is unset. It must warn on stderr exactly
// once and must not change the command's success/failure outcome.
func TestCreate_WarnsOnStderrOnce_WhenNotesVaultEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	gitRoot := t.TempDir()
	withFakeCreateContext(t, newFakeCreateContext(t, gitRoot))

	_, stderr, err := executeCommand(t, newRootCmd(), "create", "--bare", "demo")
	if err != nil {
		t.Fatalf("create --bare demo: error = %v, want nil", err)
	}

	count := strings.Count(stderr, obsidianDisabledWarning)
	if count != 1 {
		t.Fatalf("stderr warning occurrences = %d, want 1; stderr:\n%s", count, stderr)
	}
}

// TestCreate_NoWarning_WhenNotesVaultConfigured is the negative case:
// once notes_vault is set, the warning must not print at all.
func TestCreate_NoWarning_WhenNotesVaultConfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	vaultDir := t.TempDir()
	configDir := filepath.Join(home, ".config", "af")
	writeFile(t, filepath.Join(configDir, "config.toml"), `schema_version = 1

[obsidian]
notes_vault = "personal"

[obsidian.vaults]
personal = "`+filepath.ToSlash(vaultDir)+`"
`)

	gitRoot := t.TempDir()
	withFakeCreateContext(t, newFakeCreateContext(t, gitRoot))

	_, stderr, err := executeCommand(t, newRootCmd(), "create", "--bare", "demo")
	if err != nil {
		t.Fatalf("create --bare demo: error = %v, want nil", err)
	}

	if strings.Contains(stderr, obsidianDisabledWarning) {
		t.Fatalf("stderr contains the disabled-Obsidian warning despite a configured vault; stderr:\n%s", stderr)
	}
}
