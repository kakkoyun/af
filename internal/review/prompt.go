package review

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed system_prompt.md
var systemPromptBytes []byte

// SystemPrompt returns the immutable af-owned prefix loaded at
// build time. ADR-073 §1: configuration cannot replace this text;
// it can only append after it (see BuildPrompt below).
func SystemPrompt() string {
	return string(systemPromptBytes)
}

// PromptOpts assembles the four ADR-073 append layers plus the PR
// context block.
type PromptOpts struct {
	// PR holds the rendered PR header and diff body.
	PR PRContext
	// User-level append from ~/.config/af/config.toml [review].system_prompt_append.
	UserAppend string
	// Repo-level append from <repo>/.af/config.toml [review].system_prompt_append.
	RepoAppend string
	// Contents of [review].system_prompt_append_file, resolved to the
	// repo (defaults to .af/review-system-prompt.md when unset).
	FileAppend string
	// CLI --append-prompt one-shot override; highest priority.
	CLIAppend string
	// SuggestedSkills are advisory slash-command names rendered into
	// a "Suggested skills" block. Pass nil/empty to suppress.
	SuggestedSkills []string
}

// PRContext is the human-readable PR header af writes before the diff.
type PRContext struct {
	Title    string
	Base     string
	Head     string
	Worktree string
	Diff     string
	Number   int
}

// BuildPrompt assembles the full stdin payload sent to the agent:
// af prefix → repo-specific notes → suggested skills → PR header → diff.
//
// Empty append layers are silently skipped; the "Repo-specific review
// notes" heading only appears when at least one of the four append
// fields is non-empty.
func BuildPrompt(opts PromptOpts) string {
	var b strings.Builder
	b.WriteString(SystemPrompt())
	b.WriteString("\n")

	notes := joinNonEmpty([]string{
		strings.TrimSpace(opts.UserAppend),
		strings.TrimSpace(opts.RepoAppend),
		strings.TrimSpace(opts.FileAppend),
		strings.TrimSpace(opts.CLIAppend),
	}, "\n\n")
	if notes != "" {
		b.WriteString("\n# Repo-specific review notes\n\n")
		b.WriteString(notes)
		b.WriteString("\n")
	}

	skills := renderSuggestedSkills(opts.SuggestedSkills)
	if skills != "" {
		b.WriteString("\n")
		b.WriteString(skills)
	}

	b.WriteString("\n")
	b.WriteString(renderPRContext(opts.PR))
	return b.String()
}

func renderSuggestedSkills(skills []string) string {
	cleaned := make([]string, 0, len(skills))
	for _, s := range skills {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		cleaned = append(cleaned, s)
	}
	if len(cleaned) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"# Suggested skills\nIf any of the following skills are defined in this repo's\n"+
			".claude/commands/ directory, prefer using them where appropriate:\n%s\n",
		strings.Join(cleaned, ", "),
	)
}

func renderPRContext(pr PRContext) string {
	var b strings.Builder
	b.WriteString("# PR\n")
	if pr.Number > 0 {
		fmt.Fprintf(&b, "PR #%d — %s\n", pr.Number, pr.Title)
	} else {
		b.WriteString(pr.Title)
		b.WriteString("\n")
	}
	if pr.Base != "" {
		fmt.Fprintf(&b, "Base: %s\n", pr.Base)
	}
	if pr.Head != "" {
		fmt.Fprintf(&b, "Head: %s\n", pr.Head)
	}
	if pr.Worktree != "" {
		fmt.Fprintf(&b, "Worktree: %s\n", pr.Worktree)
	}
	b.WriteString("\n# Diff\n")
	b.WriteString(pr.Diff)
	if !strings.HasSuffix(pr.Diff, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func joinNonEmpty(parts []string, sep string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
