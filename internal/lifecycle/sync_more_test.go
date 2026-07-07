package lifecycle_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
)

// TestSync_ValidatesRemainingRequiredFields covers the worktree, branch,
// and parent-ref validation arms (the session-name arm is pinned in
// sync_test.go).
func TestSync_ValidatesRemainingRequiredFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*lifecycle.SyncOptions)
	}{
		{"empty worktree", func(o *lifecycle.SyncOptions) { o.Worktree = "" }},
		{"empty branch", func(o *lifecycle.SyncOptions) { o.Branch = "" }},
		{"empty parent ref", func(o *lifecycle.SyncOptions) { o.ParentRef = "" }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validOpts()
			tt.mutate(&opts)
			_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: git.NewFakeRunner()}, opts)
			if !errors.Is(err, lifecycle.ErrSync) {
				t.Fatalf("want ErrSync, got %v", err)
			}
		})
	}
}

func TestSync_GitStatusFailure(t *testing.T) {
	t.Parallel()
	r := git.NewFakeRunner()
	r.SetResponse([]string{"status", "--porcelain"}, git.FakeResponse{Err: errTestGitFailed})

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if !errors.Is(err, lifecycle.ErrSync) {
		t.Fatalf("want ErrSync, got %v", err)
	}
	if !errors.Is(err, errTestGitFailed) {
		t.Fatalf("want wrapped git error, got %v", err)
	}
}

func TestSync_CaptureHEADFailure(t *testing.T) {
	t.Parallel()
	r := newCleanFakeRunner(shaCommon, shaParent, shaAfter)
	r.SetResponse([]string{"rev-parse", "HEAD"}, git.FakeResponse{Err: errTestGitFailed})

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if !errors.Is(err, lifecycle.ErrSync) {
		t.Fatalf("want ErrSync, got %v", err)
	}
	if !strings.Contains(err.Error(), "capture HEAD") {
		t.Fatalf("err = %v, want capture HEAD context", err)
	}
}

func TestSync_MergeBaseFailure(t *testing.T) {
	t.Parallel()
	r := newCleanFakeRunner(shaCommon, shaParent, shaAfter)
	r.SetResponse([]string{"merge-base", "HEAD", testParentRef}, git.FakeResponse{Err: errTestGitFailed})

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if !errors.Is(err, lifecycle.ErrSync) {
		t.Fatalf("want ErrSync, got %v", err)
	}
	if !strings.Contains(err.Error(), "merge-base") {
		t.Fatalf("err = %v, want merge-base context", err)
	}
}

func TestSync_RevParseParentFailure(t *testing.T) {
	t.Parallel()
	r := newCleanFakeRunner(shaCommon, shaParent, shaAfter)
	r.SetResponse([]string{"rev-parse", testParentRef}, git.FakeResponse{Err: errTestGitFailed})

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if !errors.Is(err, lifecycle.ErrSync) {
		t.Fatalf("want ErrSync, got %v", err)
	}
	if !strings.Contains(err.Error(), "rev-parse parent") {
		t.Fatalf("err = %v, want rev-parse parent context", err)
	}
}

func TestSync_RebaseNonConflictFailure(t *testing.T) {
	t.Parallel()
	r := newCleanFakeRunner(shaCommon, shaParent, "")
	r.SetResponse(
		[]string{"rebase", "--onto", testParentRef, shaCommon, testBranch},
		git.FakeResponse{Output: "fatal: invalid upstream\n", Err: errTestGitFailed},
	)

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if !errors.Is(err, lifecycle.ErrSync) {
		t.Fatalf("want ErrSync, got %v", err)
	}
	if errors.Is(err, lifecycle.ErrSyncConflict) {
		t.Fatalf("non-conflict failure must not wrap ErrSyncConflict; got %v", err)
	}
}

// TestSync_RebaseCouldNotApplyIsConflict covers the "could not apply"
// output marker (without a CONFLICT line) mapping to ErrSyncConflict.
func TestSync_RebaseCouldNotApplyIsConflict(t *testing.T) {
	t.Parallel()
	r := newCleanFakeRunner(shaCommon, shaParent, "")
	r.SetResponse(
		[]string{"rebase", "--onto", testParentRef, shaCommon, testBranch},
		git.FakeResponse{Output: "error: could not apply abc1234... commit subject\n", Err: errTestGitFailed},
	)

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if !errors.Is(err, lifecycle.ErrSyncConflict) {
		t.Fatalf("want ErrSyncConflict, got %v", err)
	}
}

// TestSync_PostRebaseHEADFailure exercises the capture-HEAD-after-rebase
// error path via the FIFO runner (rev-parse HEAD succeeds first, then fails).
func TestSync_PostRebaseHEADFailure(t *testing.T) {
	t.Parallel()
	ordered := &orderedFakeRunner{
		responses: map[string][]git.FakeResponse{
			"status --porcelain":               {{Output: ""}},
			"rev-parse HEAD":                   {{Output: shaBase + "\n"}, {Err: errTestGitFailed}},
			"merge-base HEAD " + testParentRef: {{Output: shaCommon + "\n"}},
			"rev-parse " + testParentRef:       {{Output: shaParent + "\n"}},
			"rebase --onto " + testParentRef + " " + shaCommon + " " + testBranch: {{Output: "Successfully rebased.\n"}},
		},
	}

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: ordered}, validOpts())
	if !errors.Is(err, lifecycle.ErrSync) {
		t.Fatalf("want ErrSync, got %v", err)
	}
	if !strings.Contains(err.Error(), "capture HEAD after rebase") {
		t.Fatalf("err = %v, want capture HEAD after rebase context", err)
	}
}

// TestSync_FetchWarningFallsBackToErrorString covers the tryFetchParent
// branch where the failed fetch produced no output, so the error string
// itself becomes the warning.
func TestSync_FetchWarningFallsBackToErrorString(t *testing.T) {
	t.Parallel()
	r := newCleanFakeRunner(shaCommon, shaCommon, "")
	r.SetResponse([]string{"config", "--get", "remote.origin.url"},
		git.FakeResponse{Output: "git@github.com:owner/repo.git\n"})
	r.SetResponse([]string{"fetch", "origin", testParentRef},
		git.FakeResponse{Err: errTestGitFailed})

	result, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.FetchWarning != errTestGitFailed.Error() {
		t.Fatalf("FetchWarning = %q, want %q", result.FetchWarning, errTestGitFailed.Error())
	}
}
