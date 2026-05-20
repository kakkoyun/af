package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/duration"
	"github.com/kakkoyun/af/internal/obsidian"
)

const defaultRetroLimit = 50

type retroOptions struct {
	root    *rootOptions
	since   string
	search  string
	aiModel string
	tags    []string
	limit   int
	ai      bool
}

var (
	errRetroNoArchive = errors.New("archive directory does not exist")
	errRetroAINoNotes = errors.New("retro: --ai requires at least one matching note")
	errRetroAIEmpty   = errors.New("retro: agent returned an empty narrative")
	errRetroAINoCmd   = errors.New("retro: agent does not support body generation")
)

// retroAIBodyFunc is the injectable seam for tests. Production code uses
// defaultRetroAIBody which shells out to the configured agent binary.
var retroAIBodyFunc = defaultRetroAIBody //nolint:gochecknoglobals // test seam: overridden in tests to avoid exec'ing real agent binary.

// defaultRetroAIBody invokes provider's non-interactive body generation,
// passing prompt to the agent's stdin and returning the trimmed stdout.
func defaultRetroAIBody(ctx context.Context, provider agent.Agent, opts agent.BodyOpts, prompt string) (string, error) {
	argv, ok := provider.BodyCmd(opts)
	if !ok {
		return "", fmt.Errorf("%w: %s", errRetroAINoCmd, provider.Name())
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv from agent.BodyCmd; binary is a known provider name, not user input.
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("retro: agent body: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func newRetroCmd(opts *rootOptions) *cobra.Command {
	rOpts := &retroOptions{root: opts, limit: defaultRetroLimit}
	cmd := &cobra.Command{
		Use:   "retro",
		Short: "Mine archived workstream notes for a retrospective",
		Long: "retro enumerates Obsidian notes for archived workstreams and prints a summary." +
			" --since restricts the time window, --tag filters by note tags, --search filters by" +
			" substring, and --ai asks the primary agent to synthesise a narrative.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRetro(cmd, rOpts)
		},
	}
	cmd.Flags().StringVar(&rOpts.since, "since", "", "include archives newer than this duration (e.g. 7d)")
	cmd.Flags().StringSliceVar(&rOpts.tags, "tag", nil, "filter by note tag (may be repeated)")
	cmd.Flags().StringVar(&rOpts.search, "search", "", "substring match in note body")
	cmd.Flags().IntVar(&rOpts.limit, "limit", defaultRetroLimit, "maximum notes to consider")
	cmd.Flags().BoolVar(&rOpts.ai, "ai", false, "ask the primary agent to synthesise a narrative (ADR-058)")
	cmd.Flags().StringVar(&rOpts.aiModel, "ai-model", "", "override the agent model used by --ai")
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
	if len(notes) == 0 && !opts.ai {
		_, err := fmt.Fprintln(w, "no archived workstreams matched")
		if err != nil {
			return fmt.Errorf("retro write: %w", err)
		}
		return nil
	}
	for i := range notes {
		_, err := fmt.Fprintf(w, "- %s [%s] %s\n",
			notes[i].Frontmatter.StartedAt.Format("2006-01-02"),
			notes[i].Frontmatter.Session,
			notes[i].Frontmatter.Branch,
		)
		if err != nil {
			return fmt.Errorf("retro write: %w", err)
		}
	}
	if !opts.ai {
		return nil
	}
	narrative, err := runRetroAI(cmd.Context(), notes, opts)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "\n## Narrative\n\n%s\n", narrative)
	if err != nil {
		return fmt.Errorf("retro write narrative: %w", err)
	}
	return nil
}

// runRetroAI resolves the configured agent, builds the retrospective prompt,
// invokes retroAIBodyFunc, and returns the trimmed narrative.
func runRetroAI(ctx context.Context, notes []obsidian.Note, opts *retroOptions) (string, error) {
	if len(notes) == 0 {
		return "", errRetroAINoNotes
	}
	cfg, err := config.Load(ctx, "")
	if err != nil {
		return "", fmt.Errorf("retro: load config: %w", err)
	}
	agentName := cfg.General.DefaultAgent
	if agentName == "" {
		agentName = "pi"
	}
	provider, err := agent.DefaultRegistry().Resolve(agentName)
	if err != nil {
		return "", fmt.Errorf("retro: resolve agent: %w", err)
	}
	prompt := buildRetroPrompt(notes)
	bodyOpts := agent.BodyOpts{Cwd: "", Model: opts.aiModel}
	narrative, err := retroAIBodyFunc(ctx, provider, bodyOpts, prompt)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(narrative) == "" {
		return "", errRetroAIEmpty
	}
	return narrative, nil
}

// buildRetroPrompt concatenates the collection of notes into the
// retrospective prompt expected by ADR-058.
func buildRetroPrompt(notes []obsidian.Note) string {
	var b strings.Builder
	b.WriteString("Review the workstream notes below and identify:\n")
	b.WriteString("- 3 reusable patterns\n")
	b.WriteString("- 3 recurring problems\n")
	b.WriteString("- 1 process improvement worth proposing\n\n")
	for i := range notes {
		_, _ = fmt.Fprintf(&b, "--- [%s] ---\n%s\n", notes[i].Frontmatter.Session, notes[i].Body)
	}
	return b.String()
}
