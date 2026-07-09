package lifecycle_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/obsidian"
)

// notesLayoutOpts returns a bare CreateOptions rooted at home, with a
// real (DirStore-backed) notes directory so the issue #34 layout tests
// can assert actual file/directory presence rather than an opaque
// in-memory key.
func notesLayoutOpts(t *testing.T, home string) lifecycle.CreateOptions {
	t.Helper()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)
	return lifecycle.CreateOptions{
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     "github.com/kakkoyun/dotfiles",
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     filepath.Join(home, "sessions"),
		NotesDir:     filepath.Join(home, "vault", "00 - workstreams"),
		AgentName:    "pi",
		Bare:         true,
	}
}

// TestCreate_NoteLayout_RepoModeNestsUnderRepoSubfolder guards issue
// #34: the default "repo" subfolder mode writes the note at
// <NotesDir>/<repo>/<file>.md, where <repo> is the last path element
// of the repo slug.
func TestCreate_NoteLayout_RepoModeNestsUnderRepoSubfolder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := notesLayoutOpts(t, home)
	opts.Name = "fix-parser"
	opts.NotesSubfolderMode = obsidian.SubfolderModeRepo

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Notes: obsidian.NewDirStore(),
	}, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := filepath.Join(opts.NotesDir, "dotfiles", "fix-parser.md")
	if res.NotePath != want {
		t.Fatalf("NotePath = %q, want %q", res.NotePath, want)
	}
	_, statErr := os.Stat(want)
	if statErr != nil {
		t.Fatalf("note file missing at %s: %v", want, statErr)
	}
}

// TestCreate_NoteLayout_FlatModeSkipsRepoSubfolder guards the issue
// #34 opt-out: "flat" mode writes the note directly under NotesDir,
// with no per-repo subfolder.
func TestCreate_NoteLayout_FlatModeSkipsRepoSubfolder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := notesLayoutOpts(t, home)
	opts.Name = "fix-parser"
	opts.NotesSubfolderMode = obsidian.SubfolderModeFlat

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Notes: obsidian.NewDirStore(),
	}, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := filepath.Join(opts.NotesDir, "fix-parser.md")
	if res.NotePath != want {
		t.Fatalf("NotePath = %q, want %q", res.NotePath, want)
	}
	_, statErr := os.Stat(filepath.Join(opts.NotesDir, "dotfiles"))
	if !os.IsNotExist(statErr) {
		t.Fatalf("repo subfolder must not exist in flat mode (stat err = %v)", statErr)
	}
}

// TestCreate_NoteLayout_AutoNameReformatsTimestamp guards issue #34:
// an auto-generated session name ("<slug>-YYYYMMDD-HHMMSS", the
// workstream.AutoSessionName convention) produces a note file named
// "YYYY-MM-DD-HHMMSS.md", not the raw slug-and-timestamp string.
func TestCreate_NoteLayout_AutoNameReformatsTimestamp(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := notesLayoutOpts(t, home)
	opts.Now = time.Date(2026, 7, 9, 8, 50, 45, 0, time.UTC)
	// opts.Name intentionally left empty: lifecycle.Create derives the
	// auto session name "github.com/kakkoyun/dotfiles-20260709-085045".

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Notes: obsidian.NewDirStore(),
	}, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := filepath.Join(opts.NotesDir, "dotfiles", "2026-07-09-085045.md")
	if res.NotePath != want {
		t.Fatalf("NotePath = %q, want %q", res.NotePath, want)
	}
	_, statErr := os.Stat(want)
	if statErr != nil {
		t.Fatalf("reformatted note file missing at %s: %v", want, statErr)
	}
}

// TestCreate_NoteLayout_NestedSessionNameCreatesNoSubdirectory guards
// issue #34's core bug fix: a session name containing "/" must never
// create a real subdirectory under the notes folder. The sanitised
// "/"->"-" file must exist and the literal nested directory must not.
func TestCreate_NoteLayout_NestedSessionNameCreatesNoSubdirectory(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	opts := notesLayoutOpts(t, home)
	opts.Name = "team/x"

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Notes: obsidian.NewDirStore(),
	}, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	repoDir := filepath.Join(opts.NotesDir, "dotfiles")
	want := filepath.Join(repoDir, "team-x.md")
	if res.NotePath != want {
		t.Fatalf("NotePath = %q, want %q", res.NotePath, want)
	}
	_, statErr := os.Stat(want)
	if statErr != nil {
		t.Fatalf("sanitised note file missing at %s: %v", want, statErr)
	}
	_, teamStatErr := os.Stat(filepath.Join(repoDir, "team"))
	if !os.IsNotExist(teamStatErr) {
		t.Fatalf("nested \"team\" directory must not exist (stat err = %v)", teamStatErr)
	}
}

// TestCreate_NoteLayout_EmptyRepoSlugFallsBackToGitRootBasename guards
// the issue #34 fallback: when the workstream's repo slug is empty
// (no remote configured), the repo subfolder falls back to the
// basename of the git root.
func TestCreate_NoteLayout_EmptyRepoSlugFallsBackToGitRootBasename(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	gitRoot := filepath.Join(home, "scratch-repo")
	mkdirT(t, gitRoot)

	opts := lifecycle.CreateOptions{
		Name:         "demo",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     "scratch-repo",
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     filepath.Join(home, "sessions"),
		NotesDir:     filepath.Join(home, "vault", "00 - workstreams"),
		AgentName:    "pi",
		Bare:         true,
	}

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Notes: obsidian.NewDirStore(),
	}, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := filepath.Join(opts.NotesDir, "scratch-repo", "demo.md")
	if res.NotePath != want {
		t.Fatalf("NotePath = %q, want %q", res.NotePath, want)
	}
}

// TestCreate_NoteLayout_NoNotesDirSkipsNoteEntirely reaffirms that the
// note step remains fully optional (issue #17): an empty NotesDir must
// never call the note store at all, layout mode notwithstanding.
func TestCreate_NoteLayout_NoNotesDirSkipsNoteEntirely(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	gitRoot := filepath.Join(home, "repo")
	mkdirT(t, gitRoot)

	res, err := lifecycle.Create(context.Background(), lifecycle.CreateDeps{
		Git:   git.NewFakeRunner(),
		Mux:   mux.NewFakeMultiplexer(),
		Agent: agent.NewFake("pi"),
	}, lifecycle.CreateOptions{
		Name:         "demo",
		FromBranch:   "main",
		GitRoot:      gitRoot,
		RepoSlug:     "github.com/kakkoyun/dotfiles",
		WorktreeRoot: filepath.Join(home, "wt"),
		StateDir:     filepath.Join(home, "sessions"),
		AgentName:    "pi",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.NotePath != "" {
		t.Fatalf("NotePath = %q, want empty when NotesDir is unset", res.NotePath)
	}
}
