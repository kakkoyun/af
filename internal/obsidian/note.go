package obsidian

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const frontmatterMarker = "---\n"

const (
	// SubfolderModeRepo nests notes under a per-repo subfolder named
	// after the last path element of the workstream's repo slug
	// (issue #34). It is the implicit default whenever a caller leaves
	// PathConfig.SubfolderMode / ComposeNotePath's mode argument empty.
	SubfolderModeRepo = "repo"
	// SubfolderModeFlat restores the pre-issue-#34 layout: every
	// workstream note lands directly in the configured notes folder,
	// regardless of repo.
	SubfolderModeFlat = "flat"
)

// autoNameTimestampRe matches the timestamp suffix workstream.AutoSessionName
// appends to auto-generated session names: "YYYYMMDD-HHMMSS".
var autoNameTimestampRe = regexp.MustCompile(`^\d{8}-\d{6}$`)

// ErrMissingFrontmatter reports a markdown note without a leading YAML block.
var ErrMissingFrontmatter = errors.New("missing obsidian frontmatter")

// ErrNoteNotFound reports a missing note in a Store.
var ErrNoteNotFound = errors.New("obsidian note not found")

// ErrVaultNotConfigured reports that a note path cannot be resolved.
var ErrVaultNotConfigured = errors.New("obsidian vault not configured")

// Agent describes one agent slot recorded in note frontmatter.
type Agent struct {
	Slot     string `yaml:"slot"`
	Provider string `yaml:"provider"`
	Status   string `yaml:"status"`
}

// Frontmatter is the versioned af metadata block in each Obsidian note.
type Frontmatter struct {
	StartedAt   time.Time  `yaml:"af_started_at"`
	CompletedAt *time.Time `yaml:"af_completed_at"`
	Session     string     `yaml:"af_session"`
	Repo        string     `yaml:"af_repo"`
	Branch      string     `yaml:"af_branch"`
	BaseBranch  string     `yaml:"af_base_branch"`
	Status      string     `yaml:"af_status"`
	PRURL       string     `yaml:"af_pr_url"`
	PRState     string     `yaml:"af_pr_state"`
	Agents      []Agent    `yaml:"af_agents"`
	Tags        []string   `yaml:"tags"`
	AFTags      []string   `yaml:"af_tags"`
	Schema      int        `yaml:"af_schema"`
	PRNumber    int        `yaml:"af_pr_number"`
}

// Note contains parsed frontmatter and the opaque markdown body.
type Note struct {
	Body        string
	Frontmatter Frontmatter
}

// ParseNote parses a markdown note with leading YAML frontmatter.
func ParseNote(content []byte) (Note, error) {
	if !bytes.HasPrefix(content, []byte(frontmatterMarker)) {
		return Note{}, ErrMissingFrontmatter
	}

	rest := content[len(frontmatterMarker):]
	separator := []byte("\n---\n")
	index := bytes.Index(rest, separator)
	if index < 0 {
		return Note{}, ErrMissingFrontmatter
	}

	var frontmatter Frontmatter
	err := yaml.Unmarshal(rest[:index], &frontmatter)
	if err != nil {
		return Note{}, fmt.Errorf("parse obsidian frontmatter: %w", err)
	}

	return Note{Frontmatter: frontmatter, Body: string(rest[index+len(separator):])}, nil
}

// EmitNote renders a note as YAML frontmatter followed by the opaque body.
func EmitNote(note Note) ([]byte, error) {
	frontmatter, err := yaml.Marshal(note.Frontmatter)
	if err != nil {
		return nil, fmt.Errorf("emit obsidian frontmatter: %w", err)
	}

	var output bytes.Buffer
	output.WriteString(frontmatterMarker)
	output.Write(frontmatter)
	output.WriteString(frontmatterMarker)
	output.WriteString(note.Body)

	return output.Bytes(), nil
}

// PathConfig resolves workstream notes into configured Obsidian vaults.
type PathConfig struct {
	Vaults      map[string]string
	NotesVault  string
	NotesFolder string
	// SubfolderMode is "repo" (default, including "") or "flat"; see
	// SubfolderModeRepo / SubfolderModeFlat.
	SubfolderMode string
}

// ResolveNotePath returns the markdown note path for session in the
// configured vault. repoSlug and gitRoot drive the issue #34
// repo-subfolder and filename rules (see ComposeNotePath); gitRoot is
// only consulted as a fallback repo identity when repoSlug is empty.
func ResolveNotePath(config PathConfig, session, repoSlug, gitRoot string) (string, error) {
	vaultName, err := selectedVault(config)
	if err != nil {
		return "", err
	}
	vaultPath := config.Vaults[vaultName]
	if vaultPath == "" {
		return "", fmt.Errorf("resolve obsidian note %s: %w", session, ErrVaultNotConfigured)
	}

	notesDir := vaultPath
	if config.NotesFolder != "" {
		notesDir = filepath.Join(vaultPath, config.NotesFolder)
	}

	return ComposeNotePath(notesDir, config.SubfolderMode, session, repoSlug, gitRoot), nil
}

// ComposeNotePath is the single function that turns an already-resolved
// notes directory (a configured vault path, optionally joined with
// notes_folder) into the final workstream note file path. It is the
// one place `af create` composes a note path: both the DirStore write
// and the printed "note: <path>" line reuse the exact string this
// returns (issue #34).
//
// mode selects the issue #34 layout: SubfolderModeFlat keeps every
// note directly under notesDir; any other value (including "", so
// callers that predate the config key keep the new default) nests the
// note under a per-repo subfolder derived from repoSlug (falling back
// to gitRoot's basename when repoSlug is empty).
func ComposeNotePath(notesDir, mode, session, repoSlug, gitRoot string) string {
	parts := []string{notesDir}
	if mode != SubfolderModeFlat {
		if repo := repoSubfolder(repoSlug, gitRoot); repo != "" {
			parts = append(parts, repo)
		}
	}
	parts = append(parts, NoteFileName(session, repoSlug))

	return filepath.Join(parts...)
}

// repoSubfolder returns the per-repo subfolder name used in
// SubfolderModeRepo: the last path element of repoSlug (repo slugs are
// always "/"-separated regardless of OS, e.g.
// "github.com/kakkoyun/af"), or the basename of gitRoot when repoSlug
// is empty (no remote configured). Returns "" when neither is available.
func repoSubfolder(repoSlug, gitRoot string) string {
	if repoSlug != "" {
		return lastSlashSegment(repoSlug)
	}
	if gitRoot != "" {
		return filepath.Base(gitRoot)
	}

	return ""
}

// lastSlashSegment returns the last "/"-separated segment of slug.
// Repo slugs are always "/"-joined regardless of host OS (e.g.
// "github.com/kakkoyun/af"), so this avoids pulling in the "path"
// package purely for path.Base, which would collide with the "path"
// parameter name used throughout this file's Store methods.
func lastSlashSegment(slug string) string {
	trimmed := strings.TrimRight(slug, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return trimmed[idx+1:]
	}

	return trimmed
}

// NoteFileName derives the markdown filename (including the ".md"
// extension) for a workstream note from its session name and repo
// slug, per issue #34:
//
//  1. When sessionName starts with "<repoSlug>-" (the
//     workstream.AutoSessionName convention), that prefix is stripped.
//  2. When the remainder matches the auto-name timestamp shape
//     "YYYYMMDD-HHMMSS", it is reformatted to "YYYY-MM-DD-HHMMSS" for
//     readability.
//  3. Every remaining "/" is replaced with "-", so a session name can
//     never create real subdirectories under the notes folder.
func NoteFileName(sessionName, repoSlug string) string {
	remainder := sessionName
	if repoSlug != "" {
		prefix := repoSlug + "-"
		if rest, ok := strings.CutPrefix(sessionName, prefix); ok {
			remainder = rest
			if autoNameTimestampRe.MatchString(remainder) {
				remainder = reformatAutoTimestamp(remainder)
			}
		}
	}

	// Both slash variants: "/" is the legal nested-name separator, and
	// "\\" would still act as a path separator on Windows via
	// filepath.Join — a note filename must never mint directories on
	// any platform.
	remainder = strings.ReplaceAll(remainder, "/", "-")
	return strings.ReplaceAll(remainder, "\\", "-") + ".md"
}

// reformatAutoTimestamp turns a workstream.AutoSessionName timestamp
// suffix "YYYYMMDD-HHMMSS" into "YYYY-MM-DD-HHMMSS". ts must already
// match autoNameTimestampRe.
func reformatAutoTimestamp(ts string) string {
	date, clock, _ := strings.Cut(ts, "-")
	return date[0:4] + "-" + date[4:6] + "-" + date[6:8] + "-" + clock
}

func selectedVault(config PathConfig) (string, error) {
	if config.NotesVault != "" {
		return config.NotesVault, nil
	}
	if len(config.Vaults) != 1 {
		return "", fmt.Errorf("select obsidian vault: %w", ErrVaultNotConfigured)
	}
	vaults := make([]string, 0, len(config.Vaults))
	for vault := range config.Vaults {
		vaults = append(vaults, vault)
	}
	sort.Strings(vaults)

	return vaults[0], nil
}

// Store reads and writes notes through a fakeable persistence seam.
type Store interface {
	Read(ctx context.Context, path string) (Note, error)
	Write(ctx context.Context, path string, note Note) error
}

// MemoryStore is an in-memory fake Store for tests.
type MemoryStore struct {
	contents map[string][]byte
	mu       sync.RWMutex
}

// NewMemoryStore returns an empty in-memory note store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{contents: map[string][]byte{}}
}

// Read returns the note at path.
func (store *MemoryStore) Read(ctx context.Context, path string) (Note, error) {
	err := ctx.Err()
	if err != nil {
		return Note{}, fmt.Errorf("read obsidian note %s: %w", path, err)
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	content, ok := store.contents[path]
	if !ok {
		return Note{}, fmt.Errorf("read obsidian note %s: %w", path, ErrNoteNotFound)
	}

	note, err := ParseNote(content)
	if err != nil {
		return Note{}, fmt.Errorf("read obsidian note %s: %w", path, err)
	}

	return note, nil
}

// Write stores note at path.
func (store *MemoryStore) Write(ctx context.Context, path string, note Note) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("write obsidian note %s: %w", path, err)
	}
	content, err := EmitNote(note)
	if err != nil {
		return fmt.Errorf("write obsidian note %s: %w", path, err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.contents[path] = []byte(strings.Clone(string(content)))

	return nil
}
