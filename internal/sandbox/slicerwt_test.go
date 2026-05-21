package sandbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

// fakeRunner records calls for assertion.
type fakeRunner struct {
	output []byte
	err    error
	calls  []sandbox.Command
}

func (f *fakeRunner) Run(_ context.Context, cmd sandbox.Command) ([]byte, error) {
	f.calls = append(f.calls, cmd)
	return f.output, f.err
}

func TestWTPush_BuildsExpectedArgv(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{output: []byte("Launched VM sbox-abc123\n")}
	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{
		WorktreePath: "/path/to/wt",
		HostGroup:    "my-group",
		Depth:        10,
		Tags:         []string{"af-session=demo", "af-repo=owner/repo"},
	})
	if err != nil {
		t.Fatalf("WTPush: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	got := r.calls[0].Args
	want := []string{
		"wt", "push", "--launch",
		"--hostgroup", "my-group",
		"--depth", "10",
		"--tag", "af",
		"--tag", "af-session=demo",
		"--tag", "af-repo=owner/repo",
		"/path/to/wt",
	}
	if len(got) != len(want) {
		t.Fatalf("argv length: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWTPush_MinimalArgv(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{output: []byte("Launched VM sbox-abc123\n")}
	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{
		WorktreePath: "/path/to/wt",
	})
	if err != nil {
		t.Fatalf("WTPush: %v", err)
	}
	got := r.calls[0].Args
	want := []string{"wt", "push", "--launch", "--tag", "af", "/path/to/wt"}
	if len(got) != len(want) {
		t.Fatalf("argv: got %v, want %v", got, want)
	}
}

func TestWTPush_ParsesVMName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		output string
		want   string
	}{
		{"Launched VM sbox-abc123\n", "sbox-abc123"},
		{"VM: sbox-def456\n", "sbox-def456"},
		{"launched vm my-vm-789\nsome other line\n", "my-vm-789"},
	}
	for _, tc := range cases {
		r := &fakeRunner{output: []byte(tc.output)}
		res, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
		if err != nil {
			t.Errorf("output %q: unexpected error: %v", tc.output, err)
			continue
		}
		if res.VM != tc.want {
			t.Errorf("output %q: got VM %q, want %q", tc.output, res.VM, tc.want)
		}
	}
}

func TestWTPush_RejectsEmptyPath(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{}
	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !errors.Is(err, sandbox.ErrSlicerWTPushFailed) {
		t.Errorf("want ErrSlicerWTPushFailed, got %v", err)
	}
}

func TestWTPush_NameNotFound(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{output: []byte("ok\n")}
	_, err := sandbox.WTPush(context.Background(), r, sandbox.WTPushOptions{WorktreePath: "/tmp/wt"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sandbox.ErrSlicerWTNameNotFound) {
		t.Errorf("want ErrSlicerWTNameNotFound, got %v", err)
	}
}

func TestWTPull_BuildsExpectedArgv(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{output: []byte("pull complete\n")}
	_, err := sandbox.WTPull(context.Background(), r, sandbox.WTPullOptions{
		VM:           "sbox-abc123",
		WorktreePath: "/path/to/wt",
	})
	if err != nil {
		t.Fatalf("WTPull: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	got := r.calls[0].Args
	want := []string{"wt", "pull", "sbox-abc123", "/path/to/wt"}
	if len(got) != len(want) {
		t.Fatalf("argv: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWTPull_RejectsEmptyVM(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{}
	_, err := sandbox.WTPull(context.Background(), r, sandbox.WTPullOptions{WorktreePath: "/tmp"})
	if err == nil {
		t.Fatal("expected error for empty VM")
	}
	if !errors.Is(err, sandbox.ErrSlicerWTPullFailed) {
		t.Errorf("want ErrSlicerWTPullFailed, got %v", err)
	}
}

func TestWTPull_RejectsEmptyPath(t *testing.T) {
	t.Parallel()
	r := &fakeRunner{}
	_, err := sandbox.WTPull(context.Background(), r, sandbox.WTPullOptions{VM: "sbox-abc"})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !errors.Is(err, sandbox.ErrSlicerWTPullFailed) {
		t.Errorf("want ErrSlicerWTPullFailed, got %v", err)
	}
}
