package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletions_BashEmitsScript(t *testing.T) {
	stdout, stderr, err := executeCommand(t, newRootCmd(), "completions", "bash")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v; stderr=%q", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "bash completion") && !strings.Contains(stdout, "_af_") {
		t.Fatalf("bash completion script unexpected; stdout head:\n%s", head(stdout))
	}
}

func TestCompletions_ZshEmitsScript(t *testing.T) {
	stdout, _, err := executeCommand(t, newRootCmd(), "completions", "zsh")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout, "#compdef") {
		t.Fatalf("zsh completion missing #compdef header; head:\n%s", head(stdout))
	}
}

func TestCompletions_FishEmitsScript(t *testing.T) {
	stdout, _, err := executeCommand(t, newRootCmd(), "completions", "fish")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout, "complete -c af") {
		t.Fatalf("fish completion missing 'complete -c af'; head:\n%s", head(stdout))
	}
}

func TestCompletions_PowerShellEmitsScript(t *testing.T) {
	stdout, _, err := executeCommand(t, newRootCmd(), "completions", "powershell")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout, "Register-ArgumentCompleter") {
		t.Fatalf("powershell completion missing Register-ArgumentCompleter; head:\n%s", head(stdout))
	}
}

func TestCompletions_UnknownShellReturnsError(t *testing.T) {
	_, _, err := executeCommand(t, newRootCmd(), "completions", "tcsh")
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want unknown-shell error")
	}
	if !strings.Contains(err.Error(), "tcsh") {
		t.Fatalf("error %q does not mention the bad shell", err)
	}
}

func TestCompletions_RequiresShellArg(t *testing.T) {
	_, _, err := executeCommand(t, newRootCmd(), "completions")
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want missing-arg error")
	}
}

func head(s string) string {
	if len(s) <= 240 {
		return s
	}
	return s[:240] + "..."
}

// TestCompletions_InstallWritesExpectedDestination pins the exact
// per-shell install path (issue #22) and checks the installed content
// is byte-identical to the plain stdout-mode script.
func TestCompletions_InstallWritesExpectedDestination(t *testing.T) {
	tests := []struct {
		shell    string
		relPath  string
		hintWant string
	}{
		{shell: "zsh", relPath: filepath.Join(".zfunc", "_af"), hintWant: "fpath=(~/.zfunc $fpath)"},
		{shell: "bash", relPath: filepath.Join(".bash_completion.d", "af"), hintWant: "~/.bash_completion.d/af"},
		{shell: "fish", relPath: filepath.Join(".config", "fish", "completions", "af.fish"), hintWant: "no action needed"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			wantScript, _, err := executeCommand(t, newRootCmd(), "completions", tt.shell)
			if err != nil {
				t.Fatalf("completions %s: %v", tt.shell, err)
			}

			stdout, _, err := executeCommand(t, newRootCmd(), "completions", tt.shell, "--install")
			if err != nil {
				t.Fatalf("completions %s --install: %v", tt.shell, err)
			}

			wantPath := filepath.Join(home, tt.relPath)
			if !strings.Contains(stdout, "installed "+tt.shell+" completions: "+wantPath) {
				t.Fatalf("stdout = %q, want it to report installing to %q", stdout, wantPath)
			}
			if !strings.Contains(stdout, tt.hintWant) {
				t.Fatalf("stdout = %q, want activation hint containing %q", stdout, tt.hintWant)
			}

			got, err := os.ReadFile(wantPath) //nolint:gosec // Test reads a file it just installed under a temp HOME.
			if err != nil {
				t.Fatalf("ReadFile(%s): %v", wantPath, err)
			}
			if string(got) != wantScript {
				t.Fatalf("installed content does not match stdout-mode script for %s", tt.shell)
			}

			info, err := os.Stat(wantPath)
			if err != nil {
				t.Fatalf("Stat(%s): %v", wantPath, err)
			}
			if info.Mode().Perm() != 0o644 {
				t.Fatalf("file mode = %v, want 0o644", info.Mode().Perm())
			}
		})
	}
}

// TestCompletions_InstallSecondRunIsUpToDate verifies the idempotent
// no-op path: installing twice in a row leaves the file untouched and
// reports "already up to date" instead of re-writing it.
func TestCompletions_InstallSecondRunIsUpToDate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "completions", "zsh", "--install")
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	path := filepath.Join(home, ".zfunc", "_af")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat before second run: %v", err)
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "completions", "zsh", "--install")
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if !strings.Contains(stdout, "already up to date: "+path) {
		t.Fatalf("stdout = %q, want 'already up to date: %s'", stdout, path)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat after second run: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("mtime changed on a no-op install: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

// TestCompletions_InstallOverwritesStaleContent verifies that a
// destination file with different (stale) content is silently
// replaced, and the command reports it as an install rather than
// "already up to date".
func TestCompletions_InstallOverwritesStaleContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".bash_completion.d", "af")
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = os.WriteFile(path, []byte("# stale completions from a previous af version\n"), 0o644) //nolint:gosec // Test fixture; 0o644 matches production file mode.
	if err != nil {
		t.Fatalf("WriteFile stale content: %v", err)
	}

	wantScript, _, err := executeCommand(t, newRootCmd(), "completions", "bash")
	if err != nil {
		t.Fatalf("completions bash: %v", err)
	}

	stdout, _, err := executeCommand(t, newRootCmd(), "completions", "bash", "--install")
	if err != nil {
		t.Fatalf("completions bash --install: %v", err)
	}
	if !strings.Contains(stdout, "installed bash completions: "+path) {
		t.Fatalf("stdout = %q, want 'installed bash completions: %s'", stdout, path)
	}

	got, err := os.ReadFile(path) //nolint:gosec // Test reads a file it just installed under a temp HOME.
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != wantScript {
		t.Fatalf("stale content was not overwritten with the fresh script")
	}
}

// TestCompletions_InstallAutoDetectsShellFromEnv covers the $SHELL
// auto-detection contract used when --install is passed without a
// positional shell argument: a recognized basename picks that shell,
// while an empty or unrecognized $SHELL is a domain usage error.
func TestCompletions_InstallAutoDetectsShellFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		shellEnv  string
		wantPath  string
		errSubstr string
		wantErr   bool
	}{
		{name: "zsh detected", shellEnv: "/bin/zsh", wantPath: filepath.Join(".zfunc", "_af")},
		{name: "empty SHELL errors", shellEnv: "", wantErr: true, errSubstr: "cannot detect shell"},
		{name: "unsupported shell errors", shellEnv: "/bin/ksh", wantErr: true, errSubstr: "cannot detect shell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SHELL", tt.shellEnv)

			stdout, _, err := executeCommand(t, newRootCmd(), "completions", "--install")
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want auto-detect failure")
				}
				if !errors.Is(err, errCannotDetectShell) {
					t.Fatalf("error = %v, want wrapped errCannotDetectShell", err)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("completions --install (auto-detect): %v", err)
			}
			wantPath := filepath.Join(home, tt.wantPath)
			if !strings.Contains(stdout, wantPath) {
				t.Fatalf("stdout = %q, want it to mention %q", stdout, wantPath)
			}
			_, statErr := os.Stat(wantPath)
			if statErr != nil {
				t.Fatalf("Stat(%s): %v", wantPath, statErr)
			}
		})
	}
}

// TestCompletions_DryRunInstallWritesNothing verifies --dry-run
// --install writes no file and creates no parent directory, while
// still reporting what would happen and printing the activation hint.
func TestCompletions_DryRunInstallWritesNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stdout, _, err := executeCommand(t, newRootCmd(), "completions", "zsh", "--install", "--dry-run")
	if err != nil {
		t.Fatalf("completions zsh --install --dry-run: %v", err)
	}
	wantPath := filepath.Join(home, ".zfunc", "_af")
	if !strings.Contains(stdout, "would install zsh completions to "+wantPath) {
		t.Fatalf("stdout = %q, want 'would install zsh completions to %s'", stdout, wantPath)
	}
	if !strings.Contains(stdout, "fpath=(~/.zfunc $fpath)") {
		t.Fatalf("stdout = %q, want the zsh activation hint", stdout)
	}
	_, statErr := os.Stat(wantPath)
	if statErr == nil || !os.IsNotExist(statErr) {
		t.Fatalf("Stat(%s) error = %v, want IsNotExist", wantPath, statErr)
	}
	_, parentStatErr := os.Stat(filepath.Dir(wantPath))
	if parentStatErr == nil || !os.IsNotExist(parentStatErr) {
		t.Fatalf("Stat(%s) error = %v, want IsNotExist (parent dir must not be created)", filepath.Dir(wantPath), parentStatErr)
	}
}

// TestCompletions_DryRunWithoutInstallErrors verifies --dry-run alone
// (without --install) is a domain usage error, not a silent no-op.
func TestCompletions_DryRunWithoutInstallErrors(t *testing.T) {
	_, _, err := executeCommand(t, newRootCmd(), "completions", "zsh", "--dry-run")
	if err == nil {
		t.Fatal("error = nil, want --dry-run-without--install error")
	}
	if !errors.Is(err, errDryRunRequiresInstall) {
		t.Fatalf("error = %v, want wrapped errDryRunRequiresInstall", err)
	}
}

// TestCompletions_InstallRejectsPowerShell verifies powershell (which
// has no standard user-local completions convention) is rejected by
// --install with a message listing the shells that do support it.
func TestCompletions_InstallRejectsPowerShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "completions", "powershell", "--install")
	if err == nil {
		t.Fatal("error = nil, want unsupported-shell error for powershell --install")
	}
	if !errors.Is(err, errUnsupportedShell) {
		t.Fatalf("error = %v, want wrapped errUnsupportedShell", err)
	}
}
