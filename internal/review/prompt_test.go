package review_test

import (
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/review"
)

func TestSystemPrompt_EmbeddedAndNonEmpty(t *testing.T) {
	t.Parallel()
	got := review.SystemPrompt()
	const sanityFloor = 100
	if len(got) < sanityFloor {
		t.Fatalf("SystemPrompt() too short: %d bytes", len(got))
	}
	// Tone constraints from ADR-073 §1 must be present verbatim.
	for _, want := range []string{
		"af review",
		"Do not post comments",
		"Do not modify any files",
		"Do not use severity tags",
		"Do not use emoji",
		"Do not produce a verdict line",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("SystemPrompt missing required directive %q", want)
		}
	}
}

func TestBuildPrompt_NoAppendsNoSkillsRendersAfPrefixAndPRContext(t *testing.T) {
	t.Parallel()
	out := review.BuildPrompt(review.PromptOpts{
		PR: review.PRContext{Number: 42, Title: "Add foo", Base: "main", Head: "feat/foo", Worktree: "/p", Diff: "diff text"},
	})
	if !strings.Contains(out, "af review") {
		t.Errorf("BuildPrompt should include af system prefix")
	}
	if strings.Contains(out, "# Repo-specific review notes") {
		t.Errorf("BuildPrompt should NOT include notes heading when all appends empty")
	}
	if strings.Contains(out, "# Suggested skills") {
		t.Errorf("BuildPrompt should NOT include skills heading when none configured")
	}
	if !strings.Contains(out, "PR #42 — Add foo") {
		t.Errorf("BuildPrompt should include PR header")
	}
	if !strings.Contains(out, "Base: main") || !strings.Contains(out, "Head: feat/foo") {
		t.Errorf("BuildPrompt should include base/head")
	}
	if !strings.Contains(out, "diff text") {
		t.Errorf("BuildPrompt should include diff body")
	}
}

func TestBuildPrompt_FourLayerAppends(t *testing.T) {
	t.Parallel()
	out := review.BuildPrompt(review.PromptOpts{
		UserAppend: "user line",
		RepoAppend: "repo line",
		FileAppend: "file line",
		CLIAppend:  "cli line",
		PR:         review.PRContext{Number: 1, Title: "x", Base: "main", Head: "x", Worktree: "/", Diff: "d"},
	})
	if !strings.Contains(out, "# Repo-specific review notes") {
		t.Errorf("notes heading should appear when appends are present")
	}
	// All four layers should appear in resolution order (user → repo → file → CLI).
	indices := []int{
		strings.Index(out, "user line"),
		strings.Index(out, "repo line"),
		strings.Index(out, "file line"),
		strings.Index(out, "cli line"),
	}
	for i, idx := range indices {
		if idx < 0 {
			t.Fatalf("layer %d (user/repo/file/cli) missing", i)
		}
	}
	for i := 1; i < len(indices); i++ {
		if indices[i] <= indices[i-1] {
			t.Errorf("layer order wrong at index %d; positions=%v", i, indices)
		}
	}
}

func TestBuildPrompt_SuggestedSkillsRenderedWhenPresent(t *testing.T) {
	t.Parallel()
	out := review.BuildPrompt(review.PromptOpts{
		SuggestedSkills: []string{"/review", "/go-review", "/simplify"},
		PR:              review.PRContext{Number: 1, Diff: "d"},
	})
	if !strings.Contains(out, "# Suggested skills") {
		t.Errorf("expected suggested skills heading")
	}
	if !strings.Contains(out, "/review, /go-review, /simplify") {
		t.Errorf("expected skills list; got:\n%s", out)
	}
}

func TestBuildPrompt_EmptySkillStringSuppressesHints(t *testing.T) {
	t.Parallel()
	out := review.BuildPrompt(review.PromptOpts{
		SuggestedSkills: []string{"", " "},
		PR:              review.PRContext{Number: 1, Diff: "d"},
	})
	if strings.Contains(out, "# Suggested skills") {
		t.Errorf("empty/whitespace skills should suppress the block")
	}
}

func TestBuildPrompt_AppendsOnlyOneNotesHeading(t *testing.T) {
	t.Parallel()
	out := review.BuildPrompt(review.PromptOpts{
		UserAppend: "only user",
		PR:         review.PRContext{Number: 1, Diff: "d"},
	})
	if c := strings.Count(out, "# Repo-specific review notes"); c != 1 {
		t.Errorf("notes heading count = %d, want 1", c)
	}
}
