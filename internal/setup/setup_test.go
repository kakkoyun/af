package setup_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/setup"
)

type fakeGit struct {
	store map[string]string
	set   map[string]string
}

func newFakeGit(initial map[string]string) *fakeGit {
	return &fakeGit{store: copyMap(initial), set: make(map[string]string)}
}

func (f *fakeGit) GetGlobal(_ context.Context, key string) (string, error) {
	return f.store[key], nil
}

func (f *fakeGit) SetGlobal(_ context.Context, key, value string) error {
	f.store[key] = value
	f.set[key] = value
	return nil
}

func TestRun_CreatesStateDirAndConfig(t *testing.T) {
	home := t.TempDir()
	gen := stubGenerator("# completion")

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		Shell:           "bash",
		GenerateBash:    gen,
		GenerateZsh:     gen,
		GenerateFish:    gen,
		ShellDetectEnv:  "/bin/bash",
		Git:             newFakeGit(nil),
		SkipGitignore:   false,
		SkipCompletions: false,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, sub := range []string{"sessions", "archive", "secrets"} {
		path := filepath.Join(home, ".local", "share", "af", "v1", sub)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("state subdir %s missing: %v", sub, statErr)
		}
		if !info.IsDir() {
			t.Fatalf("state subdir %s not a dir", sub)
		}
	}

	if !res.ConfigCreated {
		t.Fatal("ConfigCreated = false, want true on first run")
	}
	_, statErr := os.Stat(filepath.Join(home, ".config", "af", "config.toml"))
	if statErr != nil {
		t.Fatalf("user config not written: %v", statErr)
	}
}

func TestRun_IsIdempotentOnSecondInvocation(t *testing.T) {
	home := t.TempDir()
	gen := stubGenerator("# completion v1")

	opts := setup.Options{
		HomeDir:        home,
		Shell:          "bash",
		GenerateBash:   gen,
		GenerateZsh:    gen,
		GenerateFish:   gen,
		ShellDetectEnv: "/bin/bash",
		Git:            newFakeGit(nil),
	}

	_, err := setup.Run(context.Background(), io.Discard, opts)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second, err := setup.Run(context.Background(), io.Discard, opts)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second.ConfigCreated {
		t.Fatal("ConfigCreated = true on second run, want false (idempotent)")
	}
	if second.GitignoreAdded {
		t.Fatal("GitignoreAdded = true on second run, want false (idempotent)")
	}
}

func TestRun_ForceOverwritesExistingConfig(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "af", "config.toml")
	mustWrite(t, configPath, "schema_version = 1\n# stale\n")

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		Force:           true,
		SkipCompletions: true,
		Git:             newFakeGit(nil),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.ConfigCreated {
		t.Fatal("ConfigCreated = false on --force, want true")
	}
	body, readErr := os.ReadFile(configPath) //nolint:gosec // configPath is built under t.TempDir().
	if readErr != nil {
		t.Fatalf("read overwritten config: %v", readErr)
	}
	if strings.Contains(string(body), "# stale") {
		t.Fatalf("stale config not overwritten; body:\n%s", body)
	}
}

func TestRun_AppendsAfEntryToGitignoreOnce(t *testing.T) {
	home := t.TempDir()
	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		SkipCompletions: true,
		Git:             newFakeGit(nil),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	body, readErr := os.ReadFile(res.GitignorePath)
	if readErr != nil {
		t.Fatalf("read gitignore: %v", readErr)
	}
	if !strings.Contains(string(body), ".af/\n") {
		t.Fatalf("gitignore missing .af/ entry; body:\n%s", body)
	}

	_, err = setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		SkipCompletions: true,
		Git:             newFakeGit(nil),
	})
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	body, secondReadErr := os.ReadFile(res.GitignorePath)
	if secondReadErr != nil {
		t.Fatalf("re-read gitignore: %v", secondReadErr)
	}
	if got := strings.Count(string(body), ".af/\n"); got != 1 {
		t.Fatalf(".af/ entry appears %d times, want exactly 1; body:\n%s", got, body)
	}
}

func TestRun_HonoursExistingGitExcludesfile(t *testing.T) {
	home := t.TempDir()
	customIgnore := filepath.Join(home, "custom-ignore")
	mustWrite(t, customIgnore, "existing\n")

	git := newFakeGit(map[string]string{"core.excludesfile": customIgnore})

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		SkipCompletions: true,
		Git:             git,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.GitignorePath != customIgnore {
		t.Fatalf("GitignorePath = %q, want %q", res.GitignorePath, customIgnore)
	}
	body, readErr := os.ReadFile(customIgnore) //nolint:gosec // customIgnore is built under t.TempDir().
	if readErr != nil {
		t.Fatalf("read custom ignore: %v", readErr)
	}
	if !strings.Contains(string(body), ".af/\n") {
		t.Fatalf("custom ignore missing .af/; body:\n%s", body)
	}
	if _, ok := git.set["core.excludesfile"]; ok {
		t.Fatal("git config was set despite existing core.excludesfile")
	}
}

func TestRun_InstallsBashCompletionToExpectedPath(t *testing.T) {
	home := t.TempDir()
	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:        home,
		Shell:          "bash",
		GenerateBash:   stubGenerator("# bash content"),
		GenerateZsh:    stubGenerator("ignore"),
		GenerateFish:   stubGenerator("ignore"),
		ShellDetectEnv: "/bin/bash",
		SkipGitignore:  true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filepath.Join(home, ".local", "share", "bash-completion", "completions", "af")
	if res.CompletionPath != want {
		t.Fatalf("CompletionPath = %q, want %q", res.CompletionPath, want)
	}
	body, readErr := os.ReadFile(res.CompletionPath)
	if readErr != nil {
		t.Fatalf("read completion: %v", readErr)
	}
	if !strings.Contains(string(body), "# bash content") {
		t.Fatalf("completion missing expected content; body:\n%s", body)
	}
}

func TestRun_PrintsObsidianHintWhenVaultsEmpty(t *testing.T) {
	home := t.TempDir()
	var buf bytes.Buffer
	res, err := setup.Run(context.Background(), &buf, setup.Options{
		HomeDir:         home,
		SkipCompletions: true,
		SkipGitignore:   true,
		Git:             newFakeGit(nil),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.ObsidianHint {
		t.Fatal("ObsidianHint = false, want true (empty vaults)")
	}
	if !strings.Contains(buf.String(), "[obsidian.vaults]") {
		t.Fatalf("summary missing obsidian hint; got:\n%s", buf.String())
	}
}

func stubGenerator(body string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, body)
		if err != nil {
			return fmt.Errorf("stubGenerator: %w", err)
		}
		return nil
	}
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	err = os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
