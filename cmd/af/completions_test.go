package main

import (
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
