package main

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootHelpIncludesVersionAndPersistentFlags(t *testing.T) {
	stdout, stderr, err := executeCommand(t, newRootCmd(), "--help")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v, want nil", err)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty output", stderr)
	}

	for _, want := range []string{"version", "--config", "--session", "--verbose"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help output %q does not contain %q", stdout, want)
		}
	}
}

func TestRootPersistentFlagsParse(t *testing.T) {
	opts := &rootOptions{}
	_, _, err := executeCommand(t, newRootCmdWithOptions(opts), "--verbose", "--config", "af.toml", "--session", "demo", "version")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v, want nil", err)
	}

	if !opts.verbose {
		t.Fatal("verbose = false, want true")
	}
	if opts.configPath != "af.toml" {
		t.Fatalf("configPath = %q, want %q", opts.configPath, "af.toml")
	}
	if opts.sessionName != "demo" {
		t.Fatalf("sessionName = %q, want %q", opts.sessionName, "demo")
	}
}

func TestVersionCommandWrapsWriteError(t *testing.T) {
	want := errNilOutput
	cmd := newVersionCmd()
	cmd.SetOut(failingWriter{err: want})
	cmd.SetErr(io.Discard)

	err := cmd.ExecuteContext(t.Context())
	if !errors.Is(err, want) {
		t.Fatalf("ExecuteContext() error = %v, want wrapped %v", err, want)
	}
}

func TestRootHelpWrapsWriteError(t *testing.T) {
	want := errNilOutput
	cmd := newRootCmd()
	cmd.SetOut(failingWriter{err: want})
	cmd.SetErr(io.Discard)

	err := cmd.ExecuteContext(t.Context())
	if !errors.Is(err, want) {
		t.Fatalf("ExecuteContext() error = %v, want wrapped %v", err, want)
	}
}

// TestNoCommandHelpTextMentionsADRs pins issue #25 Part 4.2: user-facing
// cobra Short/Long/Example strings must read in plain language, not cite
// internal ADR numbers (git history and code comments still may). Any
// command that regresses to an "(ADR-NNN)" aside fails this test.
func TestNoCommandHelpTextMentionsADRs(t *testing.T) {
	root := newRootCmd()
	walkCommandsForADRText(t, root)
}

func walkCommandsForADRText(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	for _, field := range []struct {
		name  string
		value string
	}{
		{"Short", cmd.Short},
		{"Long", cmd.Long},
		{"Example", cmd.Example},
	} {
		if adrMentionPattern.MatchString(field.value) {
			t.Errorf("%s.%s mentions an ADR number: %q", cmd.CommandPath(), field.name, field.value)
		}
	}
	for _, child := range cmd.Commands() {
		walkCommandsForADRText(t, child)
	}
}

var adrMentionPattern = regexp.MustCompile(`ADR-\d+`)

func executeCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()

	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd.SetOut(&stdoutBuffer)
	cmd.SetErr(&stderrBuffer)
	cmd.SetArgs(args)

	err := cmd.ExecuteContext(t.Context())
	return stdoutBuffer.String(), stderrBuffer.String(), err
}
