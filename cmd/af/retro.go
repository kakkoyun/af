package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/duration"
	"github.com/kakkoyun/af/internal/obsidian"
)

const defaultRetroLimit = 50

type retroOptions struct {
	root   *rootOptions
	since  string
	search string
	tags   []string
	limit  int
	ai     bool
}

var errRetroNoArchive = errors.New("archive directory does not exist")

func newRetroCmd(opts *rootOptions) *cobra.Command {
	rOpts := &retroOptions{root: opts, limit: defaultRetroLimit}
	cmd := &cobra.Command{
		Use:   "retro",
		Short: "Mine archived workstream notes for a retrospective",
		Long:  "retro enumerates Obsidian notes for archived workstreams and prints a summary. --since restricts the time window, --tag filters by note tags, --search filters by substring, and --ai asks the primary agent to synthesise a narrative (placeholder).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRetro(cmd, rOpts)
		},
	}
	cmd.Flags().StringVar(&rOpts.since, "since", "", "include archives newer than this duration (e.g. 7d)")
	cmd.Flags().StringSliceVar(&rOpts.tags, "tag", nil, "filter by note tag (may be repeated)")
	cmd.Flags().StringVar(&rOpts.search, "search", "", "substring match in note body")
	cmd.Flags().IntVar(&rOpts.limit, "limit", defaultRetroLimit, "maximum notes to consider")
	cmd.Flags().BoolVar(&rOpts.ai, "ai", false, "ask the primary agent to synthesise a narrative")
	return cmd
}

func runRetro(cmd *cobra.Command, opts *retroOptions) error {
	archiveDir, err := defaultArchiveDir()
	if err != nil {
		return fmt.Errorf("retro: %w", err)
	}
	notes, err := loadArchivedNotes(archiveDir, opts)
	if err != nil {
		return err
	}
	return writeRetro(cmd, notes, opts)
}

func defaultArchiveDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "af", "v1", "archive"), nil
}

func loadArchivedNotes(archiveDir string, opts *retroOptions) ([]obsidian.Note, error) {
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("retro: %w: %s", errRetroNoArchive, archiveDir)
		}
		return nil, fmt.Errorf("retro: read archive: %w", err)
	}
	cutoff, err := retroCutoff(opts.since)
	if err != nil {
		return nil, err
	}

	notes := make([]obsidian.Note, 0)
	for _, entry := range entries {
		note, ok := readArchiveEntry(archiveDir, entry, opts, cutoff)
		if !ok {
			continue
		}
		notes = append(notes, note)
		if len(notes) >= opts.limit {
			break
		}
	}
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Frontmatter.StartedAt.After(notes[j].Frontmatter.StartedAt)
	})
	return notes, nil
}

func readArchiveEntry(archiveDir string, entry os.DirEntry, opts *retroOptions, cutoff time.Time) (obsidian.Note, bool) {
	if !entry.IsDir() {
		return obsidian.Note{}, false
	}
	notePath := filepath.Join(archiveDir, entry.Name(), "note.md")
	data, readErr := os.ReadFile(notePath) //nolint:gosec // path under archive dir
	if readErr != nil {
		return obsidian.Note{}, false
	}
	note, parseErr := obsidian.ParseNote(data)
	if parseErr != nil {
		return obsidian.Note{}, false
	}
	if !cutoff.IsZero() && note.Frontmatter.StartedAt.Before(cutoff) {
		return obsidian.Note{}, false
	}
	if !noteMatchesTags(note, opts.tags) {
		return obsidian.Note{}, false
	}
	if opts.search != "" && !strings.Contains(note.Body, opts.search) {
		return obsidian.Note{}, false
	}
	return note, true
}

func retroCutoff(since string) (time.Time, error) {
	if since == "" {
		return time.Time{}, nil
	}
	d, err := duration.Parse(since)
	if err != nil {
		return time.Time{}, fmt.Errorf("retro: parse --since: %w", err)
	}
	return time.Now().UTC().Add(-d), nil
}

func noteMatchesTags(note obsidian.Note, requested []string) bool {
	if len(requested) == 0 {
		return true
	}
	for _, want := range requested {
		if !hasTag(note.Frontmatter.AFTags, want) && !hasTag(note.Frontmatter.Tags, want) {
			return false
		}
	}
	return true
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func writeRetro(cmd *cobra.Command, notes []obsidian.Note, opts *retroOptions) error {
	w := cmd.OutOrStdout()
	if len(notes) == 0 {
		_, err := fmt.Fprintln(w, "no archived workstreams matched")
		if err != nil {
			return fmt.Errorf("retro write: %w", err)
		}
		return nil
	}
	for i := range notes {
		_, err := fmt.Fprintf(w, "- %s [%s] %s\n", notes[i].Frontmatter.StartedAt.Format("2006-01-02"), notes[i].Frontmatter.Session, notes[i].Frontmatter.Branch)
		if err != nil {
			return fmt.Errorf("retro write: %w", err)
		}
	}
	if opts.ai {
		_, _ = fmt.Fprintln(w, "\n[--ai narrative synthesis is a placeholder; ADR-058 implementation pending]") //nolint:errcheck // Informational tail.
	}
	return nil
}
