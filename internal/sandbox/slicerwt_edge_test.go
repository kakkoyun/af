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
