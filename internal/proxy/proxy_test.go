package proxy

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
)

// expandCase drives one TestExpand table entry.
type expandCase struct {
	name   string
	argv   []string
	tokens Tokens
	want   []string
}

// expandCases enumerates the argv expansion table for TestExpand.
func expandCases() []expandCase {
	return []expandCase{
		{
			name:   "nil_argv",
			argv:   nil,
			tokens: Tokens{"a": "b"},
			want:   nil,
		},
		{
			name:   "empty_argv",
			argv:   []string{},
			tokens: Tokens{"a": "b"},
			want:   nil,
		},
		{
			name:   "no_tokens",
			argv:   []string{"git", "diff", "{base}"},
			tokens: nil,
			want:   []string{"git", "diff", "{base}"},
		},
		{
			name:   "single_token",
			argv:   []string{"git", "diff", "{base}"},
			tokens: Tokens{"base": "main"},
			want:   []string{"git", "diff", "main"},
		},
		{
			name:   "token_repeated_in_element",
			argv:   []string{"{ref}..{ref}"},
			tokens: Tokens{"ref": "HEAD"},
			want:   []string{"HEAD..HEAD"},
		},
		{
			name:   "multiple_tokens_in_one_element",
			argv:   []string{"{base}..{head}"},
			tokens: Tokens{"base": "main", "head": "feature"},
			want:   []string{"main..feature"},
		},
		{
			name:   "multi_word_value_stays_one_element",
			argv:   []string{"--message", "{msg}"},
			tokens: Tokens{"msg": "hello world"},
			want:   []string{"--message", "hello world"},
		},
		{
			name:   "unknown_token_left_verbatim",
			argv:   []string{"{missing}"},
			tokens: Tokens{"present": "x"},
			want:   []string{"{missing}"},
		},
		{
			name:   "empty_value",
			argv:   []string{"--flag={v}"},
			tokens: Tokens{"v": ""},
			want:   []string{"--flag="},
		},
	}
}

func TestExpand(t *testing.T) {
	for _, tt := range expandCases() {
		t.Run(tt.name, func(t *testing.T) {
			got := Expand(tt.argv, tt.tokens)
			if len(got) != len(tt.want) {
				t.Fatalf("Expand(%q) = %q, want %q", tt.argv, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Expand(%q)[%d] = %q, want %q", tt.argv, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExpandDoesNotAliasInput(t *testing.T) {
	argv := []string{"git", "{cmd}"}
	got := Expand(argv, Tokens{"cmd": "diff"})
	argv[0] = "mutated"
	if got[0] != "git" {
		t.Fatalf("Expand result aliases input argv: got[0] = %q, want %q", got[0], "git")
	}
}

func TestExpandString(t *testing.T) {
	tests := []struct {
		name     string
		template string
		tokens   Tokens
		want     string
	}{
		{
			name:     "plain_value_quoted",
			template: "git diff {base}",
			tokens:   Tokens{"base": "main"},
			want:     "git diff 'main'",
		},
		{
			name:     "empty_value_quoted",
			template: "echo {v}",
			tokens:   Tokens{"v": ""},
			want:     "echo ''",
		},
		{
			name:     "single_quote_escaped",
			template: "echo {v}",
			tokens:   Tokens{"v": "it's"},
			want:     `echo 'it'\''s'`,
		},
		{
			name:     "shell_metacharacters_defanged",
			template: "echo {v}",
			tokens:   Tokens{"v": "$(rm -rf /); `id` && | ;"},
			want:     "echo '$(rm -rf /); `id` && | ;'",
		},
		{
			name:     "unknown_token_left_verbatim",
			template: "echo {missing}",
			tokens:   Tokens{"present": "x"},
			want:     "echo {missing}",
		},
		{
			name:     "no_tokens",
			template: "echo hello",
			tokens:   nil,
			want:     "echo hello",
		},
		{
			name:     "token_repeated",
			template: "{v} {v}",
			tokens:   Tokens{"v": "a"},
			want:     "'a' 'a'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandString(tt.template, tt.tokens)
			if got != tt.want {
				t.Fatalf("ExpandString(%q, %v) = %q, want %q", tt.template, tt.tokens, got, tt.want)
			}
		})
	}
}

func TestBuildArgvCommand(t *testing.T) {
	tests := []struct {
		name    string
		argv    []string
		dir     string
		wantErr error
		want    Command
	}{
		{
			name: "name_only",
			argv: []string{"true"},
			dir:  "/tmp",
			want: Command{Name: "true", Args: []string{}, Dir: "/tmp", Shell: false},
		},
		{
			name: "name_and_args",
			argv: []string{"git", "diff", "main"},
			dir:  "/repo",
			want: Command{Name: "git", Args: []string{"diff", "main"}, Dir: "/repo", Shell: false},
		},
		{
			name:    "empty_argv",
			argv:    nil,
			dir:     "/tmp",
			wantErr: ErrEmptyArgv,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildArgvCommand(tt.argv, tt.dir)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("BuildArgvCommand(%q) error = %v, want %v", tt.argv, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildArgvCommand(%q) error = %v", tt.argv, err)
			}
			assertCommandEqual(t, got, tt.want)
		})
	}
}

func TestBuildArgvCommandCopiesArgs(t *testing.T) {
	argv := []string{"git", "diff"}
	got, err := BuildArgvCommand(argv, "")
	if err != nil {
		t.Fatalf("BuildArgvCommand(%q) error = %v", argv, err)
	}
	argv[1] = "mutated"
	if got.Args[0] != "diff" {
		t.Fatalf("Command.Args aliases input argv: Args[0] = %q, want %q", got.Args[0], "diff")
	}
}

func TestBuildShellCommand(t *testing.T) {
	got := BuildShellCommand("echo hi", "/work")
	want := Command{Name: "sh", Args: []string{"-c", "echo hi"}, Dir: "/work", Shell: true}
	assertCommandEqual(t, got, want)
}

func assertCommandEqual(t *testing.T, got, want Command) {
	t.Helper()
	if got.Name != want.Name || got.Dir != want.Dir || got.Shell != want.Shell {
		t.Fatalf("Command = %+v, want %+v", got, want)
	}
	if len(got.Args) != len(want.Args) {
		t.Fatalf("Command.Args = %q, want %q", got.Args, want.Args)
	}
	for i := range got.Args {
		if got.Args[i] != want.Args[i] {
			t.Fatalf("Command.Args[%d] = %q, want %q", i, got.Args[i], want.Args[i])
		}
	}
}

func TestExecRunnerRunSuccess(t *testing.T) {
	out, err := ExecRunner{}.Run(context.Background(), BuildShellCommand("printf hi", t.TempDir()))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(out) != "hi" {
		t.Fatalf("Run() output = %q, want %q", out, "hi")
	}
}

func TestExecRunnerRunCombinesStdoutAndStderr(t *testing.T) {
	out, err := ExecRunner{}.Run(context.Background(), BuildShellCommand("echo out; echo err 1>&2", t.TempDir()))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(string(out), "out") || !strings.Contains(string(out), "err") {
		t.Fatalf("Run() output = %q, want both stdout and stderr", out)
	}
}

func TestExecRunnerRunHonorsDir(t *testing.T) {
	dir := t.TempDir()
	out, err := ExecRunner{}.Run(context.Background(), BuildShellCommand("pwd -P", dir))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", dir, err)
	}
	if got := strings.TrimSpace(string(out)); got != resolved {
		t.Fatalf("Run() in dir = %q, want %q", got, resolved)
	}
}

func TestExecRunnerRunNonZeroExit(t *testing.T) {
	out, err := ExecRunner{}.Run(context.Background(), BuildShellCommand("echo partial; exit 3", t.TempDir()))
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}
	if !strings.Contains(err.Error(), "proxy run sh") {
		t.Fatalf("Run() error = %v, want proxy run context", err)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %v, want *exec.ExitError in chain", err)
	}
	if exitErr.ExitCode() != 3 {
		t.Fatalf("Run() exit code = %d, want 3", exitErr.ExitCode())
	}
	if !strings.Contains(string(out), "partial") {
		t.Fatalf("Run() output = %q, want partial output preserved on failure", out)
	}
}

func TestExecRunnerRunCommandNotFound(t *testing.T) {
	cmd, err := BuildArgvCommand([]string{"af-definitely-not-a-command"}, t.TempDir())
	if err != nil {
		t.Fatalf("BuildArgvCommand() error = %v", err)
	}
	_, err = ExecRunner{}.Run(context.Background(), cmd)
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Run() error = %v, want exec.ErrNotFound in chain", err)
	}
}

func TestExecRunnerRunCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ExecRunner{}.Run(ctx, BuildShellCommand("sleep 10", t.TempDir()))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled in chain", err)
	}
}

func TestExpandStringSurvivesRealShell(t *testing.T) {
	values := []string{
		"plain",
		"two words",
		"$(touch /tmp/pwned)",
		"`id`",
		"a;b&&c|d",
		`it's "quoted"`,
		"*?[glob]~",
		"multi\nline\ttab",
	}

	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			script := ExpandString("printf %s {v}", Tokens{"v": value})
			out, err := ExecRunner{}.Run(context.Background(), BuildShellCommand(script, t.TempDir()))
			if err != nil {
				t.Fatalf("Run(%q) error = %v", script, err)
			}
			if string(out) != value {
				t.Fatalf("shell round-trip of %q = %q via script %q", value, out, script)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "''"},
		{name: "plain", value: "abc", want: "'abc'"},
		{name: "space", value: "a b", want: "'a b'"},
		{name: "single_quote", value: "a'b", want: `'a'\''b'`},
		{name: "only_quote", value: "'", want: `''\'''`},
		{name: "metacharacters", value: "$`;|&", want: "'$`;|&'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.value)
			if got != tt.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// shellMetacharacters are the POSIX shell characters that must never
// appear unquoted in an expanded substitution.
const shellMetacharacters = "|&;<>()$`\\\"' \t\n*?[]#~=%"

// decodeSingleQuoted reverses shellQuote's encoding the way a POSIX
// shell would, rejecting any byte that appears outside single quotes
// other than the backslash-escape used for embedded quotes.
func decodeSingleQuoted(s string) (string, bool) {
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			if c == '\'' {
				inQuote = false
			} else {
				b.WriteByte(c)
			}
			continue
		}
		switch c {
		case '\'':
			inQuote = true
		case '\\':
			if i+1 >= len(s) {
				return "", false
			}
			i++
			b.WriteByte(s[i])
		default:
			return "", false
		}
	}
	if inQuote {
		return "", false
	}
	return b.String(), true
}

func TestPropertyExpandStringNeverLeaksUnquotedMetacharacters(t *testing.T) {
	property := func(raw string, salt uint8) bool {
		// Salt the value with a guaranteed metacharacter so every
		// iteration exercises the dangerous path.
		value := raw + string(shellMetacharacters[int(salt)%len(shellMetacharacters)])
		expanded := ExpandString("{v}", Tokens{"v": value})
		decoded, ok := decodeSingleQuoted(expanded)
		return ok && decoded == value
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatalf("quick.Check error = %v", err)
	}
}
