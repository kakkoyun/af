package sessiondata_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
)

// fakeRunner is a scripted sandbox.Runner that records every command it
// receives and returns a canned output or error.
type fakeRunner struct {
	err    error
	output string
	cmds   []sandbox.Command
}

func (r *fakeRunner) Run(_ context.Context, cmd sandbox.Command) ([]byte, error) {
	r.cmds = append(r.cmds, cmd)
	if r.err != nil {
		return nil, r.err
	}
	return []byte(r.output), nil
}

func TestExecSlicer_Inventory_ParsesTSVOutput(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{output: ".claude/projects/a.jsonl\t12\t1716345678\r\n" +
		"\n" +
		".codex/sessions/b.jsonl\t34\t1716345678.5\n"}
	slicer := sessiondata.ExecSlicer{Runner: runner}

	entries, err := slicer.Inventory(context.Background(), "vm1", []string{".claude/projects", ".codex/sessions"})
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Path != ".claude/projects/a.jsonl" || entries[0].Size != 12 {
		t.Errorf("entries[0] = %+v, want .claude/projects/a.jsonl size 12", entries[0])
	}
	if want := time.Unix(1716345678, 0).UTC(); !entries[0].ModTime.Equal(want) {
		t.Errorf("entries[0].ModTime = %v, want %v", entries[0].ModTime, want)
	}
	if want := time.Unix(1716345678, 500_000_000).UTC(); !entries[1].ModTime.Equal(want) {
		t.Errorf("entries[1].ModTime = %v, want %v", entries[1].ModTime, want)
	}

	if len(runner.cmds) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.cmds))
	}
	assertInventoryCommand(t, runner.cmds[0])
}

// assertInventoryCommand verifies the recorded inventory invocation is a
// `slicer vm exec` shell script interpolating the requested roots.
func assertInventoryCommand(t *testing.T, cmd sandbox.Command) {
	t.Helper()
	if cmd.Name != "slicer" {
		t.Errorf("command name = %q, want slicer (default binary)", cmd.Name)
	}
	wantPrefix := []string{"vm", "exec", "vm1", "--", "/bin/sh", "-c"}
	if len(cmd.Args) != len(wantPrefix)+1 {
		t.Fatalf("args = %v, want %v plus script", cmd.Args, wantPrefix)
	}
	for i, arg := range wantPrefix {
		if cmd.Args[i] != arg {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], arg)
		}
	}
	script := cmd.Args[len(cmd.Args)-1]
	if !strings.Contains(script, ".claude/projects .codex/sessions") {
		t.Errorf("script does not interpolate roots:\n%s", script)
	}
}

func TestExecSlicer_Inventory_EmptyVM(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	slicer := sessiondata.ExecSlicer{Runner: runner}

	_, err := slicer.Inventory(context.Background(), "", []string{".codex/sessions"})
	if !errors.Is(err, sessiondata.ErrInventoryFailed) {
		t.Fatalf("Inventory(empty vm) error = %v, want ErrInventoryFailed", err)
	}
	if len(runner.cmds) != 0 {
		t.Errorf("runner invoked %d times, want 0", len(runner.cmds))
	}
}

func TestExecSlicer_Inventory_NoRootsIsNoop(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	slicer := sessiondata.ExecSlicer{Runner: runner}

	entries, err := slicer.Inventory(context.Background(), "vm1", nil)
	if err != nil {
		t.Fatalf("Inventory(no roots) error = %v, want nil", err)
	}
	if entries != nil {
		t.Errorf("entries = %v, want nil", entries)
	}
	if len(runner.cmds) != 0 {
		t.Errorf("runner invoked %d times, want 0", len(runner.cmds))
	}
}

func TestExecSlicer_Inventory_RunnerError(t *testing.T) {
	t.Parallel()
	slicer := sessiondata.ExecSlicer{Runner: &fakeRunner{err: errTestBoom}}

	_, err := slicer.Inventory(context.Background(), "vm1", []string{".codex/sessions"})
	if !errors.Is(err, sessiondata.ErrInventoryFailed) {
		t.Fatalf("Inventory error = %v, want ErrInventoryFailed", err)
	}
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("Inventory error = %v, want wrapped runner error", err)
	}
}

func TestExecSlicer_Inventory_MalformedOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		output string
	}{
		{"missing tabs", "no-tabs-here"},
		{"bad size", "p\tNaN\t123"},
		{"bad integer mtime", "p\t1\tabc"},
		{"bad mtime fraction", "p\t1\t1.zz"},
		{"bad mtime seconds before dot", "p\t1\tzz.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			slicer := sessiondata.ExecSlicer{Runner: &fakeRunner{output: tt.output + "\n"}}
			_, err := slicer.Inventory(context.Background(), "vm1", []string{".codex/sessions"})
			if !errors.Is(err, sessiondata.ErrInventoryFailed) {
				t.Fatalf("Inventory(%q) error = %v, want ErrInventoryFailed", tt.output, err)
			}
		})
	}
}

func TestExecSlicer_Copy_BuildsCommand(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	slicer := sessiondata.ExecSlicer{Runner: runner}

	err := slicer.Copy(context.Background(), "vm1", ".claude/projects", "/host/path")
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if len(runner.cmds) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.cmds))
	}
	cmd := runner.cmds[0]
	if cmd.Name != "slicer" {
		t.Errorf("command name = %q, want slicer (default binary)", cmd.Name)
	}
	want := []string{"vm", "cp", "--mode=tar", "vm1:/root/.claude/projects", "/host/path"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("args = %v, want %v", cmd.Args, want)
	}
	for i, arg := range want {
		if cmd.Args[i] != arg {
			t.Errorf("args[%d] = %q, want %q", i, cmd.Args[i], arg)
		}
	}
}

func TestExecSlicer_Copy_CustomBinaryAndVMHome(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	slicer := sessiondata.ExecSlicer{Runner: runner, Binary: "myslicer", VMHome: "/home/dev"}

	err := slicer.Copy(context.Background(), "vm2", ".pi/agent/sessions", "/tmp/out")
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	cmd := runner.cmds[0]
	if cmd.Name != "myslicer" {
		t.Errorf("command name = %q, want myslicer", cmd.Name)
	}
	if got := cmd.Args[3]; got != "vm2:/home/dev/.pi/agent/sessions" {
		t.Errorf("source arg = %q, want vm2:/home/dev/.pi/agent/sessions", got)
	}
}

func TestExecSlicer_Copy_Failures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		runnerErr error
		name      string
		vm        string
		vmRelPath string
		hostPath  string
	}{
		{name: "empty vm", vm: "", vmRelPath: "p", hostPath: "/h"},
		{name: "empty source", vm: "vm1", vmRelPath: "", hostPath: "/h"},
		{name: "empty destination", vm: "vm1", vmRelPath: "p", hostPath: ""},
		{name: "runner error", vm: "vm1", vmRelPath: "p", hostPath: "/h", runnerErr: errTestBoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			slicer := sessiondata.ExecSlicer{Runner: &fakeRunner{err: tt.runnerErr}}
			err := slicer.Copy(context.Background(), tt.vm, tt.vmRelPath, tt.hostPath)
			if !errors.Is(err, sessiondata.ErrCopyFailed) {
				t.Fatalf("Copy error = %v, want ErrCopyFailed", err)
			}
			if tt.runnerErr != nil && !errors.Is(err, tt.runnerErr) {
				t.Fatalf("Copy error = %v, want wrapped runner error", err)
			}
		})
	}
}

// TestExecSlicer_DefaultRunnerFailsFast exercises the nil-Runner default
// (sandbox.ExecRunner). The binary name is guaranteed not to exist, so
// exec fails immediately without spawning anything.
func TestExecSlicer_DefaultRunnerFailsFast(t *testing.T) {
	t.Parallel()
	slicer := sessiondata.ExecSlicer{Binary: "af-test-nonexistent-slicer-binary"}

	_, err := slicer.Inventory(context.Background(), "vm1", []string{".codex/sessions"})
	if !errors.Is(err, sessiondata.ErrInventoryFailed) {
		t.Fatalf("Inventory error = %v, want ErrInventoryFailed", err)
	}
}
