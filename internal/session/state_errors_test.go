package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/session"
)

func TestReadState_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := session.ReadState(filepath.Join(t.TempDir(), "absent", "state.toml"))
	if err == nil {
		t.Fatal("ReadState() error = nil, want open error")
	}
	if !strings.Contains(err.Error(), "read state") {
		t.Fatalf("ReadState() error = %v, want read state context", err)
	}
}

func TestReadState_InvalidTOML(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.toml")
	writeFile(t, path, "not = [valid\n")

	_, err := session.ReadState(path)
	if err == nil {
		t.Fatal("ReadState() error = nil, want decode error")
	}
}

func TestReadState_SchemaTooNewSentinel(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.toml")
	writeFile(t, path, "schema_version = 99\n")

	_, err := session.ReadState(path)
	if !errors.Is(err, session.ErrSchemaTooNew) {
		t.Fatalf("ReadState() error = %v, want ErrSchemaTooNew", err)
	}
}

func TestWriteState_Failures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{
			name: "parent is a regular file",
			path: func(t *testing.T) string {
				t.Helper()
				blocker := filepath.Join(t.TempDir(), "blocker")
				writeFile(t, blocker, "not a directory\n")
				return filepath.Join(blocker, "sub", "state.toml")
			},
			want: "create state directory",
		},
		{
			name: "target path is a directory",
			path: func(t *testing.T) string {
				t.Helper()
				path := filepath.Join(t.TempDir(), "state.toml")
				err := os.MkdirAll(path, testDirPerm)
				if err != nil {
					t.Fatalf("create blocking target directory: %v", err)
				}
				return path
			},
			// "replace" (not "replace state") because WriteState now
			// delegates the write tail to the shared WriteFileAtomic
			// primitive, which also backs obsidian.DirStore.Write and
			// uses domain-neutral wording.
			want: "replace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := session.WriteState(tt.path(t), session.State{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("WriteState() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestWriteState_SurvivesHostileFixedTmpName pins the behavior change
// from moving WriteState onto session.WriteFileAtomic: WriteState used
// to fail if something occupied the fixed "<path>.tmp" name (see the
// now-removed "temporary path is a directory" case above); it now uses
// a unique os.CreateTemp name per write, so a squatted fixed name no
// longer breaks it.
func TestWriteState_SurvivesHostileFixedTmpName(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.toml")
	err := os.MkdirAll(path+".tmp", testDirPerm)
	if err != nil {
		t.Fatalf("seed squatted tmp directory: %v", err)
	}

	err = session.WriteState(path, session.State{})
	if err != nil {
		t.Fatalf("WriteState() error = %v, want success despite squatted .tmp name", err)
	}
}

func TestAppendEvent_Failures(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, time.July, 3, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		path  func(t *testing.T) string
		event session.Event
		want  string
	}{
		{
			name:  "missing timestamp",
			path:  func(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "ledger.jsonl") },
			event: session.Event{Type: "created"},
			want:  "timestamp is required",
		},
		{
			name:  "missing type",
			path:  func(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "ledger.jsonl") },
			event: session.Event{Timestamp: ts},
			want:  "type is required",
		},
		{
			name:  "unencodable field",
			path:  func(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "ledger.jsonl") },
			event: session.Event{Timestamp: ts, Type: "created", Fields: map[string]any{"bad": make(chan int)}},
			want:  "encode ledger event",
		},
		{
			name:  "ledger path is a directory",
			path:  func(t *testing.T) string { t.Helper(); return t.TempDir() },
			event: session.Event{Timestamp: ts, Type: "created"},
			want:  "open ledger",
		},
		{
			name: "parent is a regular file",
			path: func(t *testing.T) string {
				t.Helper()
				blocker := filepath.Join(t.TempDir(), "blocker")
				writeFile(t, blocker, "not a directory\n")
				return filepath.Join(blocker, "ledger.jsonl")
			},
			event: session.Event{Timestamp: ts, Type: "created"},
			want:  "create ledger directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := session.AppendEvent(tt.path(t), tt.event)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("AppendEvent() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestLastTouchedAt_Failures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{
			name: "missing file",
			path: func(t *testing.T) string { t.Helper(); return filepath.Join(t.TempDir(), "absent.jsonl") },
			want: "open ledger",
		},
		{
			name: "whitespace-only ledger",
			path: func(t *testing.T) string {
				t.Helper()
				path := filepath.Join(t.TempDir(), "ledger.jsonl")
				writeFile(t, path, "\n   \n\t\n")
				return path
			},
			want: "empty ledger",
		},
		{
			name: "corrupt last record",
			path: func(t *testing.T) string {
				t.Helper()
				path := filepath.Join(t.TempDir(), "ledger.jsonl")
				writeFile(t, path, `{"ts":"2026-07-03T10:00:00Z","event":"created"}`+"\nnot json\n")
				return path
			},
			want: "decode last ledger record",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := session.LastTouchedAt(tt.path(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LastTouchedAt() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRepoSlugFromRemote_MalformedPaths(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"https://github.com/kakkoyun":     "",
		"https://github.com/a/b/c":        "",
		"https://github.com/":             "",
		"git@github.com:kakkoyun":         "",
		"https://github.com/kakkoyun/af/": "kakkoyun/af",
	}
	for remote, want := range tests {
		got := session.RepoSlugFromRemote(remote)
		if got != want {
			t.Errorf("RepoSlugFromRemote(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestDiscoverStatePath_TmuxSessionFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	state := filepath.Join(sessionsDir, "tmuxy", "state.toml")
	writeFile(t, state, "schema_version = 1\n")
	cwd := filepath.Join(root, "plain")
	err := os.MkdirAll(cwd, testDirPerm)
	if err != nil {
		t.Fatalf("create cwd: %v", err)
	}

	got, err := session.DiscoverStatePath(session.DiscoverOptions{Cwd: cwd, SessionsDir: sessionsDir, TmuxSession: "tmuxy"})
	if err != nil {
		t.Fatalf("DiscoverStatePath(tmux fallback) error = %v", err)
	}
	if got != state {
		t.Fatalf("DiscoverStatePath(tmux fallback) = %q, want %q", got, state)
	}
}

func TestDiscoverStatePath_NoMatches(t *testing.T) {
	t.Parallel()
	_, err := session.DiscoverStatePath(session.DiscoverOptions{SessionsDir: t.TempDir(), TmuxSession: "ghost"})
	if !errors.Is(err, session.ErrNoCurrentWorkstream) {
		t.Fatalf("DiscoverStatePath(missing tmux state) error = %v, want ErrNoCurrentWorkstream", err)
	}

	_, err = session.DiscoverStatePath(session.DiscoverOptions{})
	if !errors.Is(err, session.ErrNoCurrentWorkstream) {
		t.Fatalf("DiscoverStatePath(empty options) error = %v, want ErrNoCurrentWorkstream", err)
	}
}

func TestDiscoverStatePath_TmuxStatErrorPropagates(t *testing.T) {
	t.Parallel()
	sessionsDir := filepath.Join(t.TempDir(), "sessions")
	writeFile(t, sessionsDir, "not a directory\n")

	_, err := session.DiscoverStatePath(session.DiscoverOptions{SessionsDir: sessionsDir, TmuxSession: "x"})
	if err == nil || !strings.Contains(err.Error(), "stat tmux session state") {
		t.Fatalf("DiscoverStatePath() error = %v, want stat tmux session state context", err)
	}
	if errors.Is(err, session.ErrNoCurrentWorkstream) {
		t.Fatalf("DiscoverStatePath() error = %v, must not be ErrNoCurrentWorkstream", err)
	}
}

func TestDiscoverStatePath_BrokenSymlinkErrors(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	afDir := filepath.Join(cwd, ".af")
	err := os.MkdirAll(afDir, testDirPerm)
	if err != nil {
		t.Fatalf("create .af dir: %v", err)
	}
	err = os.Symlink(filepath.Join(cwd, "missing-target"), filepath.Join(afDir, "state.toml"))
	if err != nil {
		t.Fatalf("create broken symlink: %v", err)
	}

	_, err = session.DiscoverStatePath(session.DiscoverOptions{Cwd: cwd})
	if err == nil || !strings.Contains(err.Error(), "resolve discovery symlink") {
		t.Fatalf("DiscoverStatePath() error = %v, want resolve discovery symlink context", err)
	}
}

func TestDiscoverStatePath_DotAFRegularFileErrors(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, ".af"), "not a directory\n")

	_, err := session.DiscoverStatePath(session.DiscoverOptions{Cwd: cwd})
	if err == nil || !strings.Contains(err.Error(), "stat discovery symlink") {
		t.Fatalf("DiscoverStatePath() error = %v, want stat discovery symlink context", err)
	}
}

func TestDiscoverStatePath_RegularStateFileIsIgnored(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	writeFile(t, filepath.Join(cwd, ".af", "state.toml"), "schema_version = 1\n")

	_, err := session.DiscoverStatePath(session.DiscoverOptions{Cwd: cwd})
	if !errors.Is(err, session.ErrNoCurrentWorkstream) {
		t.Fatalf("DiscoverStatePath() error = %v, want ErrNoCurrentWorkstream for non-symlink state.toml", err)
	}
}

func TestLockFile_SharedMode(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.toml.lock")
	lock, err := session.LockFile(path, session.LockShared)
	if err != nil {
		t.Fatalf("LockFile(shared) error = %v", err)
	}
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
}

func TestLockFile_Failures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path func(t *testing.T) string
		want string
	}{
		{
			name: "path is a directory",
			path: func(t *testing.T) string { t.Helper(); return t.TempDir() },
			want: "open lock",
		},
		{
			name: "parent is a regular file",
			path: func(t *testing.T) string {
				t.Helper()
				blocker := filepath.Join(t.TempDir(), "blocker")
				writeFile(t, blocker, "not a directory\n")
				return filepath.Join(blocker, "state.toml.lock")
			},
			want: "create lock directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := session.LockFile(tt.path(t), session.LockExclusive)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LockFile() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestUnlock_NilAndDoubleUnlockAreNoops(t *testing.T) {
	t.Parallel()
	var nilLock *session.Lock
	err := nilLock.Unlock()
	if err != nil {
		t.Fatalf("nil Lock Unlock() error = %v, want nil", err)
	}

	lock, err := session.LockFile(filepath.Join(t.TempDir(), "l.lock"), session.LockExclusive)
	if err != nil {
		t.Fatalf("LockFile() error = %v", err)
	}
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("first Unlock() error = %v", err)
	}
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("second Unlock() error = %v, want nil", err)
	}
}

func TestWithLock_LockAcquisitionFailure(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFile(t, blocker, "not a directory\n")
	statePath := filepath.Join(blocker, "state.toml")

	called := false
	err := session.WithLock(statePath, func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "session lock") {
		t.Fatalf("WithLock() error = %v, want session lock context", err)
	}
	if called {
		t.Fatal("WithLock() ran fn despite lock acquisition failure")
	}
}
