package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/lifecycle"
)

// errTestGitFailed simulates a non-zero git exit status in tests.
var errTestGitFailed = errors.New("exit status 1")

const (
	testWorktree  = "/tmp/test-worktree"
	testBranch    = "feat-child"
	testParentRef = "feat-parent"
	testSession   = "child-session"

	shaBase   = "aaaa1234aaaa1234aaaa1234aaaa1234aaaa1234"
	shaParent = "bbbb5678bbbb5678bbbb5678bbbb5678bbbb5678"
	shaAfter  = "cccc9012cccc9012cccc9012cccc9012cccc9012"
	shaCommon = "dddd0000dddd0000dddd0000dddd0000dddd0000"
)

// newCleanFakeRunner returns a FakeRunner pre-configured with the
// responses needed for a clean worktree + known SHAs.
func newCleanFakeRunner(mergeBaseSHA, parentSHA, afterSHA string) *git.FakeRunner {
	r := git.NewFakeRunner()
	r.SetResponse([]string{"status", "--porcelain"}, git.FakeResponse{Output: ""})
	r.SetResponse([]string{"fetch", "origin", testParentRef}, git.FakeResponse{})
	r.SetResponse([]string{"rev-parse", "HEAD"}, git.FakeResponse{Output: shaBase + "\n"})
	r.SetResponse([]string{"merge-base", "HEAD", testParentRef}, git.FakeResponse{Output: mergeBaseSHA + "\n"})
	r.SetResponse([]string{"rev-parse", testParentRef}, git.FakeResponse{Output: parentSHA + "\n"})
	if afterSHA != "" {
		r.SetResponse([]string{"rebase", "--onto", testParentRef, mergeBaseSHA, testBranch},
			git.FakeResponse{Output: "Successfully rebased.\n"})
	}
	return r
}

// validOpts returns a fully-populated SyncOptions for the test constants.
func validOpts() lifecycle.SyncOptions {
	return lifecycle.SyncOptions{
		SessionName: testSession,
		Worktree:    testWorktree,
		Branch:      testBranch,
		ParentRef:   testParentRef,
	}
}

// TestSync_RequiresNonEmptySessionName verifies that an empty SessionName
// triggers ErrSync at validation time, before any git command runs.
func TestSync_RequiresNonEmptySessionName(t *testing.T) {
	t.Parallel()
	opts := validOpts()
	opts.SessionName = ""

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: git.NewFakeRunner()}, opts)

	if !errors.Is(err, lifecycle.ErrSync) {
		t.Fatalf("want errors.Is(err, ErrSync); got %v", err)
	}
}

// TestSync_RejectsDirtyWorktree verifies that a non-empty `git status
// --porcelain` result causes ErrSyncDirtyWorktree to be returned.
func TestSync_RejectsDirtyWorktree(t *testing.T) {
	t.Parallel()
	r := git.NewFakeRunner()
	r.SetResponse([]string{"status", "--porcelain"}, git.FakeResponse{Output: " M file.go\n"})

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())

	if !errors.Is(err, lifecycle.ErrSyncDirtyWorktree) {
		t.Fatalf("want errors.Is(err, ErrSyncDirtyWorktree); got %v", err)
	}
}

// TestSync_NoOpWhenAlreadyOnParent verifies that when merge-base HEAD
// <parent> equals rev-parse <parent> (HEAD already contains the parent),
// Sync returns Rebased=false without running a rebase.
func TestSync_NoOpWhenAlreadyOnParent(t *testing.T) {
	t.Parallel()
	// merge-base == parent SHA → already on top of parent
	r := newCleanFakeRunner(shaCommon, shaCommon, "")

	result, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Rebased {
		t.Fatalf("want Rebased=false; got true")
	}
	if result.BaseBefore != result.BaseAfter {
		t.Fatalf("want BaseBefore==BaseAfter; got %q vs %q", result.BaseBefore, result.BaseAfter)
	}
	// Ensure no rebase command was issued.
	for _, cmd := range r.CommandStrings() {
		if len(cmd) > 6 && cmd[:6] == "rebase" {
			t.Fatalf("unexpected rebase command issued: %q", cmd)
		}
	}
}

// TestSync_PerformsRebaseWhenBehind verifies that when the merge-base
// differs from the parent SHA, the rebase command is issued and
// Rebased=true with distinct Before/After SHAs is returned.
func TestSync_PerformsRebaseWhenBehind(t *testing.T) {
	t.Parallel()
	// FakeRunner returns the same response for every call with matching args;
	// rev-parse HEAD must return different results before vs after the rebase,
	// so we use an orderedFakeRunner that consumes responses in FIFO order.
	ordered := &orderedFakeRunner{
		responses: map[string][]git.FakeResponse{
			"status --porcelain":               {{Output: ""}},
			"fetch origin " + testParentRef:    {{}},
			"rev-parse HEAD":                   {{Output: shaBase + "\n"}, {Output: shaAfter + "\n"}},
			"merge-base HEAD " + testParentRef: {{Output: shaCommon + "\n"}},
			"rev-parse " + testParentRef:       {{Output: shaParent + "\n"}},
			"rebase --onto " + testParentRef + " " + shaCommon + " " + testBranch: {{Output: "Successfully rebased.\n"}},
		},
	}

	result, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: ordered}, validOpts())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Rebased {
		t.Fatal("want Rebased=true; got false")
	}
	if result.BaseBefore == result.BaseAfter {
		t.Fatalf("want BaseBefore != BaseAfter; both are %q", result.BaseBefore)
	}
	if result.BaseBefore != shaBase {
		t.Fatalf("want BaseBefore=%q; got %q", shaBase, result.BaseBefore)
	}
	if result.BaseAfter != shaAfter {
		t.Fatalf("want BaseAfter=%q; got %q", shaAfter, result.BaseAfter)
	}
}

// TestSync_ReportsConflict verifies that a rebase exit with "CONFLICT"
// in its output causes ErrSyncConflict to be returned.
func TestSync_ReportsConflict(t *testing.T) {
	t.Parallel()
	r := git.NewFakeRunner()
	r.SetResponse([]string{"status", "--porcelain"}, git.FakeResponse{Output: ""})
	r.SetResponse([]string{"fetch", "origin", testParentRef}, git.FakeResponse{})
	r.SetResponse([]string{"rev-parse", "HEAD"}, git.FakeResponse{Output: shaBase + "\n"})
	r.SetResponse([]string{"merge-base", "HEAD", testParentRef}, git.FakeResponse{Output: shaCommon + "\n"})
	r.SetResponse([]string{"rev-parse", testParentRef}, git.FakeResponse{Output: shaParent + "\n"})
	r.SetResponse(
		[]string{"rebase", "--onto", testParentRef, shaCommon, testBranch},
		git.FakeResponse{
			Output: "CONFLICT (content): Merge conflict in file.go\nerror: could not apply abc1234... some commit\n",
			Err:    errTestGitFailed,
		},
	)

	_, err := lifecycle.Sync(context.Background(), lifecycle.SyncDeps{Git: r}, validOpts())

	if !errors.Is(err, lifecycle.ErrSyncConflict) {
		t.Fatalf("want errors.Is(err, ErrSyncConflict); got %v", err)
	}
}

// orderedFakeRunner is a minimal in-test Runner that returns responses
// in FIFO order per args key — needed when the same args (rev-parse HEAD)
// must return different results on successive calls.
type orderedFakeRunner struct {
	responses map[string][]git.FakeResponse
}

func (o *orderedFakeRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	key := joinArgs(args)
	resps, ok := o.responses[key]
	if !ok || len(resps) == 0 {
		return nil, nil
	}
	resp := resps[0]
	if len(resps) > 1 {
		o.responses[key] = resps[1:]
	}
	if resp.Err != nil {
		return []byte(resp.Output), resp.Err
	}
	return []byte(resp.Output), nil
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
