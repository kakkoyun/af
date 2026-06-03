package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

var (
	errFakeGhNoResponse  = errors.New("fake gh: no response for key")
	errFakeGhCouldNotRes = errors.New("could not resolve to a PullRequest")
)

// fakeGhRunner is a minimal sandbox.Runner for review tests. It returns
// the next response from Responses in order, keyed by argv (so the
// runner can serve both `pr view` and `pr diff` in one test).
type fakeGhRunner struct {
	Responses map[string][]byte
	Errs      map[string]error
}

func (f fakeGhRunner) Run(_ context.Context, cmd sandbox.Command) ([]byte, error) {
	key := strings.Join(cmd.Args, " ")
	if err, ok := f.Errs[key]; ok {
		return nil, err
	}
	if out, ok := f.Responses[key]; ok {
		return out, nil
	}
	// Match by prefix (e.g. all "pr view ..." calls).
	for k, v := range f.Responses {
		if strings.HasPrefix(key, k) {
			return v, nil
		}
	}
	return nil, errFakeGhNoResponse
}

// installReviewFakes replaces both seams (gh runner + agent body) and
// restores them on cleanup.
func installReviewFakes(t *testing.T, runner sandbox.Runner, body func(context.Context, string, string, string) (string, error)) {
	t.Helper()
	origGh := reviewGhFactory
	origBody := reviewBodyFunc
	t.Cleanup(func() {
		reviewGhFactory = origGh
		reviewBodyFunc = origBody
	})
	reviewGhFactory = func() sandbox.Runner { return runner }
	reviewBodyFunc = body
}

func TestReview_GoldenPathWritesReportAndLedgerEvent(t *testing.T) { //nolint:cyclop,funlen // Test asserts full file + ledger invariants in one place.
	home := t.TempDir()
	t.Setenv("HOME", home)
	worktreeDir := t.TempDir()
	writeTestSessionStateWithWorktree(t, home, "rev-golden", worktreeDir, "feat/r", "main")

	runner := fakeGhRunner{
		Responses: map[string][]byte{
			"pr view --json": []byte(`{"number":42,"title":"Add foo","headRefName":"feat/r","baseRefName":"main"}`),
			"pr diff 42":     []byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -0,0 +1 @@\n+hello\n"),
		},
	}
	installReviewFakes(t, runner, func(_ context.Context, _, _, _ string) (string, error) {
		return "Looks good overall. A few minor notes follow.", nil
	})

	stdout, _, err := executeCommand(t, newRootCmd(), "review", "rev-golden")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !strings.Contains(stdout, "review: wrote") {
		t.Errorf("stdout should announce written path; got: %s", stdout)
	}
	// The report should be in <worktree>/.af/reviews/<ts>-pr42.md.
	reviewDir := filepath.Join(worktreeDir, ".af", "reviews")
	entries, err := os.ReadDir(reviewDir)
	if err != nil {
		t.Fatalf("read review dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("review dir has %d files, want 1", len(entries))
	}
	reportPath := filepath.Join(reviewDir, entries[0].Name())
	data, err := os.ReadFile(reportPath) //nolint:gosec // test path.
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	report := string(data)
	if !strings.Contains(report, "Review draft — PR #42 Add foo") {
		t.Errorf("report missing header; got:\n%s", report)
	}
	if !strings.Contains(report, "do not post as-is") {
		t.Errorf("report missing draft warning")
	}
	if !strings.Contains(report, "Looks good overall") {
		t.Errorf("report missing agent body")
	}
	// Ledger event must be present.
	ledgerPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "rev-golden", "ledger.jsonl")
	events, err := session.ReadLedgerTail(ledgerPath, 10)
	if err != nil {
		t.Fatalf("ReadLedgerTail: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Type == "review.report.written" {
			found = true
			pr, ok := e.Fields["pr"].(float64)
			if !ok || int(pr) != 42 {
				t.Errorf("ledger pr = %v, want 42", e.Fields["pr"])
			}
		}
	}
	if !found {
		t.Errorf("ledger missing review.report.written event; got: %+v", events)
	}
}

func TestReview_NoPRReturnsErrReviewNoPR(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	worktreeDir := t.TempDir()
	writeTestSessionStateWithWorktree(t, home, "rev-nopr", worktreeDir, "feat/r", "main")

	runner := fakeGhRunner{
		Errs: map[string]error{
			"pr view --json number,title,headRefName,baseRefName": errFakeGhCouldNotRes,
		},
	}
	installReviewFakes(t, runner, func(_ context.Context, _, _, _ string) (string, error) {
		t.Errorf("agent should not be invoked when no PR resolves")
		return "", nil
	})

	_, _, err := executeCommand(t, newRootCmd(), "review", "rev-nopr")
	if !errors.Is(err, errReviewNoPR) {
		t.Errorf("want errReviewNoPR, got %v", err)
	}
}

func TestReview_EmptyDiffReturnsErrReviewEmptyDiff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	worktreeDir := t.TempDir()
	writeTestSessionStateWithWorktree(t, home, "rev-empty", worktreeDir, "feat/r", "main")

	runner := fakeGhRunner{
		Responses: map[string][]byte{
			"pr view --json": []byte(`{"number":7,"title":"x","headRefName":"feat/r","baseRefName":"main"}`),
			"pr diff 7":      []byte(""),
		},
	}
	installReviewFakes(t, runner, func(_ context.Context, _, _, _ string) (string, error) {
		t.Errorf("agent should not be invoked when diff is empty")
		return "", nil
	})

	_, _, err := executeCommand(t, newRootCmd(), "review", "rev-empty")
	if !errors.Is(err, errReviewEmptyDiff) {
		t.Errorf("want errReviewEmptyDiff, got %v", err)
	}
}

func TestReview_EmptyBodyReturnsErrReviewEmptyBody(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	worktreeDir := t.TempDir()
	writeTestSessionStateWithWorktree(t, home, "rev-blank", worktreeDir, "feat/r", "main")

	runner := fakeGhRunner{
		Responses: map[string][]byte{
			"pr view --json": []byte(`{"number":1,"title":"x","headRefName":"feat/r","baseRefName":"main"}`),
			"pr diff 1":      []byte("diff --git a/x b/x\n+x\n"),
		},
	}
	installReviewFakes(t, runner, func(_ context.Context, _, _, _ string) (string, error) {
		return "   \n\n", nil
	})

	_, _, err := executeCommand(t, newRootCmd(), "review", "rev-blank")
	if !errors.Is(err, errReviewEmptyBody) {
		t.Errorf("want errReviewEmptyBody, got %v", err)
	}
}

func TestReview_StdoutFlagSkipsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	worktreeDir := t.TempDir()
	writeTestSessionStateWithWorktree(t, home, "rev-stdout", worktreeDir, "feat/r", "main")

	runner := fakeGhRunner{
		Responses: map[string][]byte{
			"pr view --json": []byte(`{"number":99,"title":"X","headRefName":"feat/r","baseRefName":"main"}`),
			"pr diff 99":     []byte("diff text"),
		},
	}
	installReviewFakes(t, runner, func(_ context.Context, _, _, _ string) (string, error) {
		return "review body", nil
	})

	stdout, _, err := executeCommand(t, newRootCmd(), "review", "--stdout", "rev-stdout")
	if err != nil {
		t.Fatalf("review --stdout: %v", err)
	}
	if !strings.Contains(stdout, "Review draft — PR #99 X") {
		t.Errorf("stdout should contain report; got: %s", stdout)
	}
	// No file should have been written.
	reviewDir := filepath.Join(worktreeDir, ".af", "reviews")
	_, statErr := os.Stat(reviewDir)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf(".af/reviews/ should NOT exist with --stdout; statErr=%v", statErr)
	}
}

// TestReview_AppendPromptFlagThreadsThroughToAgent verifies that
// --append-prompt content appears in the prompt sent to the agent.
func TestReview_AppendPromptFlagThreadsThroughToAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	worktreeDir := t.TempDir()
	writeTestSessionStateWithWorktree(t, home, "rev-append", worktreeDir, "feat/r", "main")

	runner := fakeGhRunner{
		Responses: map[string][]byte{
			"pr view --json": []byte(`{"number":1,"title":"x","headRefName":"feat/r","baseRefName":"main"}`),
			"pr diff 1":      []byte("diff"),
		},
	}
	var capturedPrompt string
	installReviewFakes(t, runner, func(_ context.Context, _, _, prompt string) (string, error) {
		capturedPrompt = prompt
		return "body", nil
	})

	_, _, err := executeCommand(t, newRootCmd(), "review", "--append-prompt", "FOCUS ON SECURITY", "rev-append")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !strings.Contains(capturedPrompt, "FOCUS ON SECURITY") {
		t.Errorf("prompt should contain CLI append; got:\n%s", capturedPrompt)
	}
}
