package secret_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kakkoyun/af/internal/secret"
)

func TestEnvelope_Write_SortedEscapedEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{
			name:    "nil entries produce empty body",
			entries: nil,
			want:    "",
		},
		{
			name: "keys sorted and plain values unquoted",
			entries: map[string]string{
				"B_TOKEN": "beta",
				"A_TOKEN": "alpha",
			},
			want: "A_TOKEN=alpha\nB_TOKEN=beta\n",
		},
		{
			name: "whitespace value is quoted",
			entries: map[string]string{
				"KEY": "two words",
			},
			want: "KEY=\"two words\"\n",
		},
		{
			name: "dollar value is quoted",
			entries: map[string]string{
				"KEY": "pre$var",
			},
			want: "KEY=\"pre$var\"\n",
		},
		{
			name: "quotes and backslashes escaped",
			entries: map[string]string{
				"KEY": `say "hi" c:\dir`,
			},
			want: "KEY=\"say \\\"hi\\\" c:\\\\dir\"\n",
		},
		{
			name: "newline value dropped to empty quotes",
			entries: map[string]string{
				"KEY": "line1\nline2",
			},
			want: "KEY=\"\"\n",
		},
		{
			name: "carriage return value dropped to empty quotes",
			entries: map[string]string{
				"KEY": "line1\rline2",
			},
			want: "KEY=\"\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertEnvelopeWrite(t, tt.entries, tt.want)
		})
	}
}

// assertEnvelopeWrite writes the entries to a fresh envelope file and
// verifies the resulting body byte-for-byte.
func assertEnvelopeWrite(t *testing.T, entries map[string]string, want string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "agent.env")
	env := secret.Envelope{Path: path, Entries: entries}
	err := env.Write()
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	body, err := os.ReadFile(path) //nolint:gosec // test path under t.TempDir.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != want {
		t.Fatalf("Write() body = %q, want %q", body, want)
	}
}

func TestEnvelope_Write_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits are not enforced on windows")
	}
	dir := filepath.Join(t.TempDir(), "envelopes")
	path := filepath.Join(dir, "agent.env")
	env := secret.Envelope{Path: path, Entries: map[string]string{"KEY": "value"}}
	err := env.Write()
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(dir) error = %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("envelope dir perm = %o, want 0700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(file) error = %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("envelope file perm = %o, want 0600", got)
	}
}

func TestEnvelope_Write_EmptyPath(t *testing.T) {
	env := secret.Envelope{Path: "", Entries: map[string]string{"KEY": "value"}}
	err := env.Write()
	if !errors.Is(err, secret.ErrEmptyPath) {
		t.Fatalf("Write() error = %v, want ErrEmptyPath", err)
	}
}

func TestEnvelope_Write_MkdirError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	err := os.WriteFile(blocker, []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("WriteFile(blocker) error = %v", err)
	}

	env := secret.Envelope{Path: filepath.Join(blocker, "agent.env")}
	err = env.Write()
	if err == nil {
		t.Fatal("Write() error = nil, want create-dir failure")
	}
}

func TestEnvelope_Write_FileError(t *testing.T) {
	dir := t.TempDir()
	env := secret.Envelope{Path: dir, Entries: map[string]string{"KEY": "value"}}
	err := env.Write()
	if err == nil {
		t.Fatal("Write() error = nil, want write failure for directory path")
	}
}

func TestEnvelope_Delete(t *testing.T) {
	t.Run("removes existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "agent.env")
		env := secret.Envelope{Path: path, Entries: map[string]string{"KEY": "value"}}
		err := env.Write()
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		err = env.Delete()
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		_, err = os.Stat(path)
		if !os.IsNotExist(err) {
			t.Fatalf("Stat(deleted) error = %v, want not-exist", err)
		}
	})
	t.Run("missing file is not an error", func(t *testing.T) {
		env := secret.Envelope{Path: filepath.Join(t.TempDir(), "missing.env")}
		err := env.Delete()
		if err != nil {
			t.Fatalf("Delete(missing) error = %v", err)
		}
	})
	t.Run("empty path is a no-op", func(t *testing.T) {
		env := secret.Envelope{}
		err := env.Delete()
		if err != nil {
			t.Fatalf("Delete(empty path) error = %v", err)
		}
	})
	t.Run("non-empty directory fails", func(t *testing.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "child"), []byte("x"), 0o600)
		if err != nil {
			t.Fatalf("WriteFile(child) error = %v", err)
		}
		env := secret.Envelope{Path: dir}
		err = env.Delete()
		if err == nil {
			t.Fatal("Delete(non-empty dir) error = nil, want failure")
		}
	})
}
