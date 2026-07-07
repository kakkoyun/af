package setup

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionsSummaryLine_UnsupportedShellFallback(t *testing.T) {
	// Unreachable through Run (installCompletions aborts on unsupported
	// shells before a summary is printed), so exercised directly.
	line := completionsSummaryLine(Result{Shell: "tcsh"})
	if !strings.Contains(line, `shell "tcsh" not supported`) {
		t.Fatalf("line = %q, want unsupported-shell fallback", line)
	}
}

func TestExpandTilde(t *testing.T) {
	home := "/home/owner"

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: ""},
		{name: "bare tilde", path: "~", want: home},
		{name: "tilde slash", path: "~/ignore", want: filepath.Join(home, "ignore")},
		{name: "absolute untouched", path: "/etc/gitignore", want: "/etc/gitignore"},
		{name: "tilde mid-path untouched", path: "dir/~file", want: "dir/~file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTilde(tt.path, home)
			if got != tt.want {
				t.Fatalf("expandTilde(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
