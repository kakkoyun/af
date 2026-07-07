package setup_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/setup"
)

// errGit is a GitConfigurer whose calls fail or return canned values,
// for exercising the git error branches in updateGitignore.
type errGit struct {
	getVal   string
	getErr   error
	setErr   error
	setCalls int
}

func (g *errGit) GetGlobal(_ context.Context, _ string) (string, error) {
	return g.getVal, g.getErr
}

func (g *errGit) SetGlobal(_ context.Context, _, _ string) error {
	g.setCalls++
	return g.setErr
}

func TestRun_StateDirErrors(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, home string) string
		wantSub string
	}{
		{
			name: "state root is a regular file",
			prepare: func(t *testing.T, home string) string {
				t.Helper()
				root := filepath.Join(home, "state-file")
				mustWrite(t, root, "not a dir\n")
				return root
			},
			wantSub: "create state subdir",
		},
		{
			name: "secrets path is a regular file",
			prepare: func(t *testing.T, home string) string {
				t.Helper()
				root := filepath.Join(home, "state")
				for _, sub := range []string{"sessions", "archive"} {
					mkdirAll(t, filepath.Join(root, "v1", sub))
				}
				mustWrite(t, filepath.Join(root, "v1", "secrets"), "not a dir\n")
				return root
			},
			wantSub: "create secrets dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			root := tt.prepare(t, home)

			_, err := setup.Run(context.Background(), io.Discard, setup.Options{
				HomeDir:         home,
				StateDirRoot:    root,
				SkipGitignore:   true,
				SkipCompletions: true,
			})
			if err == nil {
				t.Fatal("Run succeeded, want state-dir error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestRun_ConfigStatErrorWhenParentIsFile(t *testing.T) {
	home := t.TempDir()
	blocker := filepath.Join(home, "blocker")
	mustWrite(t, blocker, "file\n")

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		UserConfigPath:  filepath.Join(blocker, "config.toml"),
		SkipGitignore:   true,
		SkipCompletions: true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want stat error")
	}
	if !strings.Contains(err.Error(), "stat user config") {
		t.Fatalf("error = %v, want substring %q", err, "stat user config")
	}
}

func TestRun_ForceRemoveErrorWhenConfigIsNonEmptyDir(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "af", "config.toml")
	mustWrite(t, filepath.Join(configDir, "occupant"), "keeps dir non-empty\n")

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		Force:           true,
		SkipGitignore:   true,
		SkipCompletions: true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want remove error")
	}
	if !strings.Contains(err.Error(), "remove existing config") {
		t.Fatalf("error = %v, want substring %q", err, "remove existing config")
	}
}

func TestRun_WriteConfigErrorOnTrailingSlashPath(t *testing.T) {
	home := t.TempDir()
	// A trailing separator makes stat return ErrNotExist but the final
	// open fail with EISDIR, exercising the write-error branch.
	configPath := filepath.Join(home, ".config", "af", "config.toml") + string(os.PathSeparator)

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		UserConfigPath:  configPath,
		SkipGitignore:   true,
		SkipCompletions: true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want write error")
	}
	if !strings.Contains(err.Error(), "write user config") {
		t.Fatalf("error = %v, want substring %q", err, "write user config")
	}
}

func TestRun_GitSetGlobalErrorAborts(t *testing.T) {
	home := t.TempDir()
	git := &errGit{setErr: errors.New("boom")}

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		Git:             git,
		SkipCompletions: true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want set git config error")
	}
	if !strings.Contains(err.Error(), "set git core.excludesfile") {
		t.Fatalf("error = %v, want substring %q", err, "set git core.excludesfile")
	}
}

func TestRun_GitGetGlobalErrorFallsBackToDefaultPath(t *testing.T) {
	home := t.TempDir()
	git := &errGit{getErr: errors.New("git unavailable")}

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		Git:             git,
		SkipCompletions: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filepath.Join(home, ".config", "git", "ignore")
	if res.GitignorePath != want {
		t.Fatalf("GitignorePath = %q, want %q", res.GitignorePath, want)
	}
	if git.setCalls != 0 {
		t.Fatalf("SetGlobal called %d times after GetGlobal error, want 0", git.setCalls)
	}
}

func TestRun_ExpandsTildeInExcludesfile(t *testing.T) {
	home := t.TempDir()
	git := &errGit{getVal: "~/custom-ignore"}

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		Git:             git,
		SkipCompletions: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filepath.Join(home, "custom-ignore")
	if res.GitignorePath != want {
		t.Fatalf("GitignorePath = %q, want %q", res.GitignorePath, want)
	}
	if !res.GitignoreAdded {
		t.Fatal("GitignoreAdded = false, want true for fresh tilde-expanded path")
	}
}

func TestRun_GitignoreParentCreateErrorWhenComponentIsFile(t *testing.T) {
	home := t.TempDir()
	blocker := filepath.Join(home, "blocker")
	mustWrite(t, blocker, "file\n")

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		GitignorePath:   filepath.Join(blocker, "sub", "ignore"),
		SkipCompletions: true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want mkdir error")
	}
	if !strings.Contains(err.Error(), "create gitignore parent") {
		t.Fatalf("error = %v, want substring %q", err, "create gitignore parent")
	}
}

func TestRun_GitignoreReadErrorWhenPathIsDirectory(t *testing.T) {
	home := t.TempDir()
	dirAsIgnore := filepath.Join(home, "ignore-dir")
	mkdirAll(t, dirAsIgnore)

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		GitignorePath:   dirAsIgnore,
		SkipCompletions: true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want read error")
	}
	if !strings.Contains(err.Error(), "read gitignore") {
		t.Fatalf("error = %v, want substring %q", err, "read gitignore")
	}
}

func TestRun_GitignoreAppendsNewlineBeforeEntry(t *testing.T) {
	home := t.TempDir()
	ignorePath := filepath.Join(home, ".config", "git", "ignore")
	mustWrite(t, ignorePath, "existing") // no trailing newline

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		SkipCompletions: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.GitignoreAdded {
		t.Fatal("GitignoreAdded = false, want true")
	}
	body, readErr := os.ReadFile(ignorePath) //nolint:gosec // ignorePath is built under t.TempDir().
	if readErr != nil {
		t.Fatalf("read gitignore: %v", readErr)
	}
	if got, want := string(body), "existing\n.af/\n"; got != want {
		t.Fatalf("gitignore body = %q, want %q", got, want)
	}
}

func TestRun_UnsupportedShellReturnsSentinel(t *testing.T) {
	home := t.TempDir()

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:       home,
		Shell:         "tcsh",
		SkipGitignore: true,
	})
	if !errors.Is(err, setup.ErrUnsupportedShell) {
		t.Fatalf("error = %v, want errors.Is ErrUnsupportedShell", err)
	}
	if res.Shell != "" {
		t.Fatalf("Result.Shell = %q, want empty on aborted completion step", res.Shell)
	}
}

func TestRun_PowershellSkipsFileInstall(t *testing.T) {
	home := t.TempDir()
	var buf bytes.Buffer

	res, err := setup.Run(context.Background(), &buf, setup.Options{
		HomeDir:       home,
		Shell:         "powershell",
		SkipGitignore: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Shell != "powershell" {
		t.Fatalf("Shell = %q, want %q", res.Shell, "powershell")
	}
	if res.CompletionPath != "" {
		t.Fatalf("CompletionPath = %q, want empty for powershell", res.CompletionPath)
	}
	if !strings.Contains(buf.String(), "$PROFILE") {
		t.Fatalf("summary missing powershell hint; got:\n%s", buf.String())
	}
}

func TestRun_InstallsCompletionPerShell(t *testing.T) {
	tests := []struct {
		name     string
		shell    string
		wantPath func(home string) string
	}{
		{
			name:  "zsh",
			shell: "zsh",
			wantPath: func(home string) string {
				return filepath.Join(home, ".config", "zsh", "completions", "_af")
			},
		},
		{
			name:  "fish",
			shell: "fish",
			wantPath: func(home string) string {
				return filepath.Join(home, ".config", "fish", "completions", "af.fish")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			marker := "# " + tt.shell + " content"

			res, err := setup.Run(context.Background(), io.Discard, setup.Options{
				HomeDir:       home,
				Shell:         tt.shell,
				GenerateBash:  stubGenerator("wrong"),
				GenerateZsh:   stubGenerator(marker),
				GenerateFish:  stubGenerator(marker),
				SkipGitignore: true,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			want := tt.wantPath(home)
			if res.CompletionPath != want {
				t.Fatalf("CompletionPath = %q, want %q", res.CompletionPath, want)
			}
			body, readErr := os.ReadFile(res.CompletionPath)
			if readErr != nil {
				t.Fatalf("read completion: %v", readErr)
			}
			if !strings.Contains(string(body), marker) {
				t.Fatalf("completion missing %q; body:\n%s", marker, body)
			}
		})
	}
}

func TestRun_NilGeneratorSkipsCompletionWrite(t *testing.T) {
	home := t.TempDir()

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:       home,
		Shell:         "bash",
		GenerateBash:  nil,
		SkipGitignore: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filepath.Join(home, ".local", "share", "bash-completion", "completions", "af")
	if res.CompletionPath != want {
		t.Fatalf("CompletionPath = %q, want %q", res.CompletionPath, want)
	}
	if _, statErr := os.Stat(want); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("stat completion = %v, want ErrNotExist (nothing written)", statErr)
	}
}

func TestRun_CompletionDirCreateErrorWhenComponentIsFile(t *testing.T) {
	home := t.TempDir()
	blocker := filepath.Join(home, "blocker")
	mustWrite(t, blocker, "file\n")

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:        home,
		Shell:          "bash",
		GenerateBash:   stubGenerator("# bash"),
		BashCompletion: filepath.Join(blocker, "completions", "af"),
		SkipGitignore:  true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want mkdir error")
	}
	if !strings.Contains(err.Error(), "create completion dir for bash") {
		t.Fatalf("error = %v, want substring %q", err, "create completion dir for bash")
	}
}

func TestRun_CompletionOpenErrorWhenPathIsDirectory(t *testing.T) {
	home := t.TempDir()
	dirAsCompletion := filepath.Join(home, "completions", "af")
	mkdirAll(t, dirAsCompletion)

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:        home,
		Shell:          "bash",
		GenerateBash:   stubGenerator("# bash"),
		BashCompletion: dirAsCompletion,
		SkipGitignore:  true,
	})
	if err == nil {
		t.Fatal("Run succeeded, want open error")
	}
	if !strings.Contains(err.Error(), "open completion file") {
		t.Fatalf("error = %v, want substring %q", err, "open completion file")
	}
}

func TestRun_GeneratorErrorIsWrapped(t *testing.T) {
	home := t.TempDir()
	genErr := errors.New("generator broke")

	_, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir: home,
		Shell:   "zsh",
		GenerateZsh: func(io.Writer) error {
			return genErr
		},
		SkipGitignore: true,
	})
	if !errors.Is(err, genErr) {
		t.Fatalf("error = %v, want errors.Is generator error", err)
	}
	if !strings.Contains(err.Error(), "generate zsh completion") {
		t.Fatalf("error = %v, want substring %q", err, "generate zsh completion")
	}
}

func TestRun_DetectsShellFromEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SHELL", "/usr/bin/zsh")

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:       home,
		GenerateZsh:   stubGenerator("# zsh"),
		SkipGitignore: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Shell != "zsh" {
		t.Fatalf("Shell = %q, want %q (detected from $SHELL)", res.Shell, "zsh")
	}
}

func TestRun_DefaultsHomeDirFromUserHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		SkipGitignore:   true,
		SkipCompletions: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := filepath.Join(home, ".local", "share", "af", "v1")
	if res.StateDir != want {
		t.Fatalf("StateDir = %q, want %q", res.StateDir, want)
	}
}

func TestRun_ObsidianHintFalseWhenVaultsConfigured(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "af", "config.toml")
	mustWrite(t, configPath, "schema_version = 1\n\n[obsidian.vaults]\npersonal = \"/vaults/personal\"\n")

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		SkipGitignore:   true,
		SkipCompletions: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ObsidianHint {
		t.Fatal("ObsidianHint = true with configured vaults, want false")
	}
}

func TestRun_ObsidianHintFalseWhenConfigUnparsable(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "af", "config.toml")
	mustWrite(t, configPath, "this is not [ valid toml\n")

	res, err := setup.Run(context.Background(), io.Discard, setup.Options{
		HomeDir:         home,
		SkipGitignore:   true,
		SkipCompletions: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ObsidianHint {
		t.Fatal("ObsidianHint = true on unparsable config, want false")
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	err := os.MkdirAll(path, 0o750)
	if err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
