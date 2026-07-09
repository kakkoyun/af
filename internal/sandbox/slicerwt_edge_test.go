package sandbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

var (
	// errPushBoom is a sentinel runner failure for WTPush tests.
	errPushBoom = errors.New("push boom")
	// errPullBoom is a sentinel runner failure for WTPull tests.
	errPullBoom = errors.New("pull boom")
)

func TestWTPush_RunnerErrorWrapped(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{err: errPushBoom}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if !errors.Is(err, sandbox.ErrSlicerWTPushFailed) {
		t.Fatalf("WTPush() error = %v, want ErrSlicerWTPushFailed", err)
	}
	if !errors.Is(err, errPushBoom) {
		t.Fatalf("WTPush() error = %v, want wrapped %v", err, errPushBoom)
	}
}

func TestWTPull_RunnerErrorWrapped(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{err: errPullBoom}

	_, err := sandbox.WTPull(context.Background(), r, sandbox.WTPullOptions{VM: "sbox-abc", WorktreePath: "/tmp/wt"})
	if !errors.Is(err, sandbox.ErrSlicerWTPullFailed) {
		t.Fatalf("WTPull() error = %v, want ErrSlicerWTPullFailed", err)
	}
	if !errors.Is(err, errPullBoom) {
		t.Fatalf("WTPull() error = %v, want wrapped %v", err, errPullBoom)
	}
}

func TestWTPush_FallbackVMNameParsing(t *testing.T) {
	t.Parallel()
	// No "Launched VM"/"VM:" marker: the last plausible word wins,
	// skipping common help-text words like "worktree".
	r := &fakeRunner{output: []byte("pushed worktree\nsbox-zz111\n")}

	res, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if err != nil {
		t.Fatalf("WTPush() error = %v", err)
	}
	if res.VM != "sbox-zz111" {
		t.Fatalf("VM = %q, want sbox-zz111", res.VM)
	}
	if res.PushedAt.IsZero() {
		t.Fatal("PushedAt is zero, want timestamp")
	}
}

func TestWTPush_FallbackSkipsExcludedWordsOnly(t *testing.T) {
	t.Parallel()
	// All candidate words are excluded help-text words → name not found.
	r := &fakeRunner{output: []byte("slicer worktree launch\n")}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if !errors.Is(err, sandbox.ErrSlicerWTNameNotFound) {
		t.Fatalf("WTPush() error = %v, want ErrSlicerWTNameNotFound", err)
	}
}

func TestWTPush_NameNotFoundTruncatesLongOutput(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{output: []byte(strings.Repeat("slicer ", 40))}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if !errors.Is(err, sandbox.ErrSlicerWTNameNotFound) {
		t.Fatalf("WTPush() error = %v, want ErrSlicerWTNameNotFound", err)
	}
	if !strings.Contains(err.Error(), "…") {
		t.Fatalf("WTPush() error = %v, want truncated output marker …", err)
	}
}

// errExitStatus1 stands in for the *exec.ExitError a real failed slicer
// invocation would return; only its message ("exit status 1") matters here.
var errExitStatus1 = errors.New("exit status 1")

// TestWTPush_MultipleHostGroupsGuidance covers issue #19: when slicer has
// multiple host groups and af's configured group is empty, slicer wt push
// fails with "Multiple host groups present (N), specify --hostgroup" on its
// stderr. af must surface that stderr AND add guidance pointing at
// [sandbox.slicer] group.
func TestWTPush_MultipleHostGroupsGuidance(t *testing.T) {
	t.Parallel()
	stderr := "Multiple host groups present (2), specify --hostgroup"
	r := &fakeRunner{err: sandbox.WrapCommandError("slicer", []string{"wt", "push", "--launch", "/tmp/wt"}, errExitStatus1, []byte(stderr))}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if err == nil {
		t.Fatal("WTPush() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "Multiple host groups present") {
		t.Fatalf("WTPush() error = %v, want propagated stderr", err)
	}
	if !strings.Contains(err.Error(), "set [sandbox.slicer] group") {
		t.Fatalf("WTPush() error = %v, want guidance to set [sandbox.slicer] group", err)
	}
	if !errors.Is(err, sandbox.ErrSlicerMultipleHostGroups) {
		t.Fatalf("WTPush() error = %v, want ErrSlicerMultipleHostGroups", err)
	}
}

// TestWTPush_PropagatesArbitraryStderr covers the generic case: any stderr
// from a failing slicer invocation should show up in af's error, not just
// the multiple-host-groups case.
func TestWTPush_PropagatesArbitraryStderr(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{err: sandbox.WrapCommandError("slicer", []string{"wt", "push", "--launch", "/tmp/wt"}, errExitStatus1, []byte("boom disk full"))}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if err == nil {
		t.Fatal("WTPush() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "boom disk full") {
		t.Fatalf("WTPush() error = %v, want propagated stderr", err)
	}
}

// TestWTPush_MultipleHostGroupsGuidanceSuppressedWhenGroupConfigured covers
// the case where [sandbox.slicer] group is already set: af's guidance line
// should not appear (the config already resolves the ambiguity), even
// though slicer's stderr happens to still mention multiple host groups
// (e.g. a stale/misconfigured group name).
func TestWTPush_MultipleHostGroupsGuidanceSuppressedWhenGroupConfigured(t *testing.T) {
	t.Parallel()
	stderr := "Multiple host groups present (2), specify --hostgroup"
	r := &fakeRunner{err: sandbox.WrapCommandError("slicer", []string{"wt", "push", "--launch", "--hostgroup", "sbox", "/tmp/wt"}, errExitStatus1, []byte(stderr))}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt", HostGroup: "sbox"})
	if err == nil {
		t.Fatal("WTPush() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "Multiple host groups present") {
		t.Fatalf("WTPush() error = %v, want propagated stderr", err)
	}
	if strings.Contains(err.Error(), "set [sandbox.slicer] group") {
		t.Fatalf("WTPush() error = %v, want no guidance line when group is already configured", err)
	}
	if errors.Is(err, sandbox.ErrSlicerMultipleHostGroups) {
		t.Fatalf("WTPush() error = %v, want no ErrSlicerMultipleHostGroups when group is already configured", err)
	}
}

// TestWTPush_StderrTruncatedAt512Bytes covers the truncation bound end to
// end through WTPush: a 2000-byte stderr snippet must not blow up af's
// error message.
func TestWTPush_StderrTruncatedAt512Bytes(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("z", 2000)
	r := &fakeRunner{err: sandbox.WrapCommandError("slicer", []string{"wt", "push", "--launch", "/tmp/wt"}, errExitStatus1, []byte(long))}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if err == nil {
		t.Fatal("WTPush() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "…") {
		t.Fatalf("WTPush() error = %v, want truncation marker …", err)
	}
	if strings.Contains(err.Error(), strings.Repeat("z", 600)) {
		t.Fatalf("WTPush() error = %v, want stderr truncated to <= 512 bytes", err)
	}
}
