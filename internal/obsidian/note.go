package obsidian

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const frontmatterMarker = "---\n"

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
}

// ResolveNotePath returns the markdown path for session in the configured vault.
func ResolveNotePath(config PathConfig, session string) (string, error) {
	vaultName, err := selectedVault(config)
	if err != nil {
		return "", err
	}
	vaultPath := config.Vaults[vaultName]
	if vaultPath == "" {
		return "", fmt.Errorf("resolve obsidian note %s: %w", session, ErrVaultNotConfigured)
	}

	parts := []string{vaultPath}
	if config.NotesFolder != "" {
		parts = append(parts, config.NotesFolder)
	}
	parts = append(parts, session+".md")

	return filepath.Join(parts...), nil
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
