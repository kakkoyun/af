package sandbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

func TestWTPush_RunnerErrorWrapped(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("push boom")
	r := &fakeRunner{err: sentinel}

	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if !errors.Is(err, sandbox.ErrSlicerWTPushFailed) {
		t.Fatalf("WTPush() error = %v, want ErrSlicerWTPushFailed", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("WTPush() error = %v, want wrapped %v", err, sentinel)
	}
}

func TestWTPull_RunnerErrorWrapped(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("pull boom")
	r := &fakeRunner{err: sentinel}

	_, err := sandbox.WTPull(context.Background(), r, sandbox.WTPullOptions{VM: "sbox-abc", WorktreePath: "/tmp/wt"})
	if !errors.Is(err, sandbox.ErrSlicerWTPullFailed) {
		t.Fatalf("WTPull() error = %v, want ErrSlicerWTPullFailed", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("WTPull() error = %v, want wrapped %v", err, sentinel)
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
