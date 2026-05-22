package gh_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kakkoyun/af/internal/gh"
	"github.com/kakkoyun/af/internal/sandbox"
)

type fakeRunner struct { //nolint:govet // Test-only struct.
	Output []byte
	Err    error
}

func (f fakeRunner) Run(_ context.Context, _ sandbox.Command) ([]byte, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Output, nil
}

var (
	errFakeNoPR = errors.New("could not resolve to a PullRequest")
	errFakeBoom = errors.New("network failure")
)

func TestViewPR_Success(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{Output: []byte(`{"number":7,"title":"Add foo","headRefName":"feat/foo","baseRefName":"main"}`)}
	meta, err := gh.ViewPR(context.Background(), runner, 0)
	if err != nil {
		t.Fatalf("ViewPR: %v", err)
	}
	if meta.Number != 7 || meta.Title != "Add foo" {
		t.Errorf("meta = %+v", meta)
	}
	if meta.BaseRefName != "main" || meta.HeadRefName != "feat/foo" {
		t.Errorf("meta refs wrong: %+v", meta)
	}
}

func TestViewPR_NoPRMappedFromGhError(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{Err: errFakeNoPR}
	_, err := gh.ViewPR(context.Background(), runner, 0)
	if !errors.Is(err, gh.ErrNoPR) {
		t.Errorf("want ErrNoPR, got %v", err)
	}
}

func TestViewPR_NumberZeroAfterParseAlsoNoPR(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{Output: []byte(`{"number":0}`)}
	_, err := gh.ViewPR(context.Background(), runner, 0)
	if !errors.Is(err, gh.ErrNoPR) {
		t.Errorf("want ErrNoPR on number=0, got %v", err)
	}
}

func TestViewPR_OtherErrorWrapped(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{Err: errFakeBoom}
	_, err := gh.ViewPR(context.Background(), runner, 0)
	if errors.Is(err, gh.ErrNoPR) {
		t.Errorf("non-no-PR error should not map to ErrNoPR; got %v", err)
	}
	if !errors.Is(err, gh.ErrCommandFailed) {
		t.Errorf("want ErrCommandFailed wrap, got %v", err)
	}
}

func TestDiffPR_Success(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{Output: []byte("diff --git a/a b/a\n--- a\n+++ a\n@@\n+x\n")}
	diff, err := gh.DiffPR(context.Background(), runner, 7)
	if err != nil {
		t.Fatalf("DiffPR: %v", err)
	}
	if diff == "" {
		t.Error("DiffPR returned empty result")
	}
}

func TestDiffPR_EmptyDiff(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{Output: []byte("")}
	_, err := gh.DiffPR(context.Background(), runner, 7)
	if !errors.Is(err, gh.ErrEmptyDiff) {
		t.Errorf("want ErrEmptyDiff, got %v", err)
	}
}

func TestDiffPR_WhitespaceOnlyDiff(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{Output: []byte("   \n  \n")}
	_, err := gh.DiffPR(context.Background(), runner, 7)
	if !errors.Is(err, gh.ErrEmptyDiff) {
		t.Errorf("whitespace-only diff should map to ErrEmptyDiff; got %v", err)
	}
}

func TestDiffPR_ZeroNumberFails(t *testing.T) {
	t.Parallel()
	runner := fakeRunner{}
	_, err := gh.DiffPR(context.Background(), runner, 0)
	if !errors.Is(err, gh.ErrNoPR) {
		t.Errorf("zero number should map to ErrNoPR; got %v", err)
	}
}
