package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/gh"
	"github.com/kakkoyun/af/internal/review"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

const (
	reviewReportFilePerm = 0o600
	reviewReportDirPerm  = 0o750
)

var (
	errReviewNoPR             = errors.New("review: no pull request resolved; run `gh pr checkout <n>` or pass --pr <n>")
	errReviewEmptyDiff        = errors.New("review: pr diff is empty; nothing to review")
	errReviewEmptyBody        = errors.New("review: agent returned an empty body")
	errReviewAgentUnavailable = errors.New("review: configured agent provider is unavailable")
	errReviewWriteReport      = errors.New("review: write report")
)

// reviewGhFactory returns the sandbox.Runner used by the gh helper.
// Tests replace it with a fake runner returning canned output.
//
//nolint:gochecknoglobals // Test seam: replaced in tests; same pattern as sessiondataSlicerFactory.
var reviewGhFactory = func() sandbox.Runner { return sandbox.ExecRunner{} }

// reviewBodyFunc is the test seam wrapping the agent's BodyCmd path.
// Production wires through agent.NewProvider(...).BodyCmd().
type reviewBodyFn func(ctx context.Context, providerName, model, prompt string) (string, error)

//nolint:gochecknoglobals // Test seam: same pattern as prAIBodyFunc.
var reviewBodyFunc reviewBodyFn = defaultReviewBody

type reviewOptions struct { //nolint:govet // Field readability over packing for a CLI options struct.
	skills       []string
	agentName    string
	model        string
	outPath      string
	appendPrompt string
	prNumber     int
	stdout       bool
}

func newReviewCmd(_ *rootOptions) *cobra.Command {
	var opts reviewOptions
	cmd := &cobra.Command{
		Use:   "review [session]",
		Short: "Generate a draft PR review report (ADR-073). Read-only; never posts.",
		Long: "review assembles the af-owned immutable system prompt with any repo-specific " +
			"append layers, fetches the PR metadata and diff via gh, asks the configured " +
			"agent for a review, and writes the markdown to .af/reviews/. Never posts.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runReview(cmd, name, opts)
		},
	}
	cmd.Flags().IntVar(&opts.prNumber, "pr", 0, "explicit PR number (overrides auto-detect)")
	cmd.Flags().StringVar(&opts.agentName, "agent", "", "override [review].agent")
	cmd.Flags().StringVar(&opts.model, "model", "", "override [review].model")
	cmd.Flags().StringVar(&opts.outPath, "out", "", "override report path")
	cmd.Flags().StringVar(&opts.appendPrompt, "append-prompt", "", "one-shot append to the system prompt")
	cmd.Flags().StringSliceVar(&opts.skills, "skill", nil, "override [review].suggested_skills; repeatable. Use --skill \"\" to suppress.")
	cmd.Flags().BoolVar(&opts.stdout, "stdout", false, "print the report to stdout instead of writing a file")
	return cmd
}

func runReview(cmd *cobra.Command, name string, opts reviewOptions) error {
	state, statePath, cfg, err := loadReviewContext(cmd, name)
	if err != nil {
		return err
	}
	runner := reviewGhFactory()
	meta, err := gh.ViewPR(cmd.Context(), runner, opts.prNumber)
	if err != nil {
		return mapGhError(err)
	}
	diff, err := gh.DiffPR(cmd.Context(), runner, meta.Number)
	if err != nil {
		return mapGhError(err)
	}

	prompt := buildReviewPrompt(cfg.Review, opts, meta, diff, state)
	providerName, model := resolveReviewAgent(cfg.Review, state, opts)
	body, err := reviewBodyFunc(cmd.Context(), providerName, model, prompt)
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("%w", errReviewEmptyBody)
	}

	report := renderReviewReport(meta, providerName, model, body)
	if opts.stdout {
		writef(cmd.OutOrStdout(), "%s", report)
		return nil
	}

	outPath, writeErr := writeReviewReport(state, meta.Number, opts.outPath, report)
	if writeErr != nil {
		return fmt.Errorf("%w: %w", errReviewWriteReport, writeErr)
	}
	writef(cmd.OutOrStdout(), "review: wrote %s\n", outPath)
	if statePath != "" {
		err = emitReviewLedgerEvent(statePath, state, meta, outPath, providerName, model)
		if err != nil {
			return fmt.Errorf("review: %w", err)
		}
	}
	return nil
}

// loadReviewContext resolves the workstream state (if any) and the
// effective config. af review works without a workstream when --pr is
// passed; statePath is empty in that case and no ledger event is
// emitted.
func loadReviewContext(cmd *cobra.Command, name string) (session.State, string, config.Config, error) {
	ctx := cmd.Context()
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		// No state file → run anyway; the caller must pass --pr.
		cfg, cfgErr := loadConfigForRefresh(ctx)
		if cfgErr != nil {
			return session.State{}, "", config.Config{}, fmt.Errorf("review: %w", cfgErr)
		}
		return session.State{}, "", cfg, nil
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, "", config.Config{}, fmt.Errorf("review: read state: %w", err)
	}
	cfg, err := loadConfigForRefresh(ctx)
	if err != nil {
		return state, statePath, config.Config{}, fmt.Errorf("review: %w", err)
	}
	return state, statePath, cfg, nil
}

func mapGhError(err error) error {
	if errors.Is(err, gh.ErrNoPR) {
		return errReviewNoPR
	}
	if errors.Is(err, gh.ErrEmptyDiff) {
		return errReviewEmptyDiff
	}
	return fmt.Errorf("review: %w", err)
}

func buildReviewPrompt(cfg config.ReviewConfig, opts reviewOptions, meta gh.PRMeta, diff string, state session.State) string {
	fileAppend := readSystemPromptFile(cfg.SystemPromptAppendFile, state.Worktree.Path)
	skills := cfg.SuggestedSkills
	if len(opts.skills) > 0 {
		skills = opts.skills
	}
	return review.BuildPrompt(review.PromptOpts{
		UserAppend:      cfg.SystemPromptAppend,
		RepoAppend:      "", // ADR-036 layering already folds repo append into cfg.SystemPromptAppend.
		FileAppend:      fileAppend,
		CLIAppend:       opts.appendPrompt,
		SuggestedSkills: skills,
		PR: review.PRContext{
			Number:   meta.Number,
			Title:    meta.Title,
			Base:     meta.BaseRefName,
			Head:     meta.HeadRefName,
			Worktree: state.Worktree.Path,
			Diff:     diff,
		},
	})
}

// readSystemPromptFile resolves [review].system_prompt_append_file. When
// unset, falls back to <worktree>/.af/review-system-prompt.md.
func readSystemPromptFile(configured, worktree string) string {
	if configured == "" && worktree == "" {
		return ""
	}
	path := configured
	if path == "" {
		path = filepath.Join(worktree, ".af", "review-system-prompt.md")
	} else if !filepath.IsAbs(path) && worktree != "" {
		path = filepath.Join(worktree, path)
	}
	data, err := os.ReadFile(path) //nolint:gosec // Path is from repo config or worktree; bounded.
	if err != nil {
		return ""
	}
	return string(data)
}

func resolveReviewAgent(cfg config.ReviewConfig, state session.State, opts reviewOptions) (string, string) {
	provider := opts.agentName
	if provider == "" {
		provider = cfg.Agent
	}
	if provider == "" {
		for i := range state.Agents {
			if state.Agents[i].Slot == "primary" {
				provider = state.Agents[i].Provider
				break
			}
		}
	}
	if provider == "" {
		provider = "claude"
	}
	model := opts.model
	if model == "" {
		model = cfg.Model
	}
	return provider, model
}

func renderReviewReport(meta gh.PRMeta, providerName, model, body string) string {
	stamp := time.Now().UTC().Format(time.RFC3339)
	var b strings.Builder
	fmt.Fprintf(&b, "# Review draft — PR #%d %s\n\n", meta.Number, meta.Title)
	fmt.Fprintf(&b, "_Generated by af review at %s — do not post as-is._\n\n", stamp)
	fmt.Fprintf(&b, "Base: %s  Head: %s\n", meta.BaseRefName, meta.HeadRefName)
	fmt.Fprintf(&b, "Agent: %s %s\n\n", providerName, model)
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func writeReviewReport(state session.State, prNumber int, outOverride, report string) (string, error) {
	outPath := outOverride
	if outPath == "" {
		base := state.Worktree.Path
		if base == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("resolve cwd: %w", err)
			}
			base = cwd
		}
		stamp := time.Now().UTC().Format("20060102T150405Z")
		outPath = filepath.Join(base, ".af", "reviews", fmt.Sprintf("%s-pr%d.md", stamp, prNumber))
	}
	err := os.MkdirAll(filepath.Dir(outPath), reviewReportDirPerm)
	if err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
	}
	tmpPath := outPath + ".tmp"
	err = os.WriteFile(tmpPath, []byte(report), reviewReportFilePerm)
	if err != nil {
		return "", fmt.Errorf("write %s: %w", tmpPath, err)
	}
	err = os.Rename(tmpPath, outPath)
	if err != nil {
		return "", fmt.Errorf("rename %s → %s: %w", tmpPath, outPath, err)
	}
	return outPath, nil
}

func emitReviewLedgerEvent(statePath string, state session.State, meta gh.PRMeta, reportPath, providerName, model string) error {
	ledgerPath := filepath.Join(filepath.Dir(statePath), "ledger.jsonl")
	event := session.Event{
		Timestamp: time.Now().UTC(),
		Type:      "review.report.written",
		Fields: map[string]any{
			"session": state.Session.Name,
			"pr":      meta.Number,
			"path":    reportPath,
			"agent":   providerName,
			"model":   model,
		},
	}
	err := session.AppendEvent(ledgerPath, event)
	if err != nil {
		return fmt.Errorf("append review.report.written: %w", err)
	}
	return nil
}

// defaultReviewBody resolves the named agent provider, requests its
// BodyCmd argv, and pipes the prompt into the resulting process. The
// caller's working directory is "" — af review is repo-aware but the
// agent runs at the worktree root via its own logic.
//
// Mirrors the cmd/af/proxy_commands.go AI-body pattern (resolveBodyAgent
// + BodyCmd + runAgentBody) but is wired as its own seam so review tests
// do not rely on the PR-specific seam.
func defaultReviewBody(ctx context.Context, providerName, model, prompt string) (string, error) {
	provider, err := resolveBodyAgent(providerName)
	if err != nil {
		return "", fmt.Errorf("review: %w", err)
	}
	argv, ok := provider.BodyCmd(agent.BodyOpts{Cwd: "", Model: model})
	if !ok {
		return "", fmt.Errorf("review: %w", errReviewAgentUnavailable)
	}
	body, err := runAgentBody(ctx, argv, "", prompt)
	if err != nil {
		return "", fmt.Errorf("review: %w", err)
	}
	return body, nil
}
