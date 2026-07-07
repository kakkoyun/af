package git_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/testutil"
)

func TestNewExecRunner_DefaultsToGitBinary(t *testing.T) {
	runner := git.NewExecRunner()
	if runner.Binary != "git" {
		t.Fatalf("NewExecRunner().Binary = %q, want %q", runner.Binary, "git")
	}
}

func TestExecRunner_Run_ReturnsCombinedOutputFromWorkingDirectory(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	testutil.WriteExecutable(t, binDir, "git", `printf 'args=%s\n' "$*"
printf 'dir=%s\n' "$PWD"
printf 'stderr line\n' >&2`)
	t.Setenv("PATH", testutil.PrependPath(binDir, os.Getenv("PATH")))

	workDir := t.TempDir()
	runner := git.ExecRunner{Binary: ""}
	out, err := runner.Run(context.Background(), workDir, "status", "--porcelain")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := string(out)
	if !strings.Contains(got, "args=status --porcelain\n") {
		t.Fatalf("Run() output = %q, want args line", got)
	}
	resolvedDir, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatalf("resolve work dir: %v", err)
	}
	if !strings.Contains(got, "dir="+resolvedDir+"\n") && !strings.Contains(got, "dir="+workDir+"\n") {
		t.Fatalf("Run() output = %q, want dir line for %q", got, workDir)
	}
	if !strings.Contains(got, "stderr line\n") {
		t.Fatalf("Run() output = %q, want combined stderr", got)
	}
}

func TestExecRunner_Run_UsesExplicitBinaryPath(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	fake := testutil.WriteExecutable(t, binDir, "not-git", `printf 'explicit binary\n'`)

	runner := git.ExecRunner{Binary: fake}
	out, err := runner.Run(context.Background(), t.TempDir(), "version")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(out) != "explicit binary\n" {
		t.Fatalf("Run() output = %q, want %q", out, "explicit binary\n")
	}
}

func TestExecRunner_Run_WrapsFailureWithErrRunnerCall(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	testutil.WriteExecutable(t, binDir, "git", `printf 'fatal: not a git repository\n' >&2
exit 128`)
	t.Setenv("PATH", testutil.PrependPath(binDir, os.Getenv("PATH")))

	workDir := t.TempDir()
	runner := git.NewExecRunner()
	out, err := runner.Run(context.Background(), workDir, "rev-parse", "HEAD")
	if !errors.Is(err, git.ErrRunnerCall) {
		t.Fatalf("Run() error = %v, want ErrRunnerCall", err)
	}
	if !strings.Contains(string(out), "fatal: not a git repository") {
		t.Fatalf("Run() output = %q, want captured stderr", out)
	}

	msg := err.Error()
	for _, want := range []string{"git rev-parse HEAD", workDir, "fatal: not a git repository"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Run() error = %q, want it to contain %q", msg, want)
		}
	}
}

func TestExecRunner_Run_WrapsMissingBinaryWithErrRunnerCall(t *testing.T) {
	runner := git.ExecRunner{Binary: filepath.Join(t.TempDir(), "no-such-git")}
	_, err := runner.Run(context.Background(), t.TempDir(), "status")
	if !errors.Is(err, git.ErrRunnerCall) {
		t.Fatalf("Run() error = %v, want ErrRunnerCall", err)
	}
}

func TestFakeRunner_Run_RecordsCallsAndReturnsCannedResponses(t *testing.T) {
	sentinel := errors.New("canned failure")
	runner := git.NewFakeRunner()
	runner.SetResponse([]string{"branch", "--merged"}, git.FakeResponse{Output: "main\n"})
	runner.SetResponse([]string{"worktree", "remove", "/tmp/w"}, git.FakeResponse{Output: "busy", Err: sentinel})

	out, err := runner.Run(context.Background(), "/repo", "branch", "--merged")
	if err != nil || string(out) != "main\n" {
		t.Fatalf("Run(matched) = (%q, %v), want (%q, nil)", out, err, "main\n")
	}

	out, err = runner.Run(context.Background(), "/repo", "worktree", "remove", "/tmp/w")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Run(error response) error = %v, want %v", err, sentinel)
	}
	if string(out) != "busy" {
		t.Fatalf("Run(error response) output = %q, want %q", out, "busy")
	}

	out, err = runner.Run(context.Background(), "/elsewhere", "status")
	if err != nil || out != nil {
		t.Fatalf("Run(unmatched) = (%v, %v), want (nil, nil)", out, err)
	}

	wantCalls := []git.FakeCall{
		{Dir: "/repo", Args: []string{"branch", "--merged"}},
		{Dir: "/repo", Args: []string{"worktree", "remove", "/tmp/w"}},
		{Dir: "/elsewhere", Args: []string{"status"}},
	}
	if len(runner.Calls) != len(wantCalls) {
		t.Fatalf("Calls = %#v, want %#v", runner.Calls, wantCalls)
	}
	for index, call := range runner.Calls {
		if call.Dir != wantCalls[index].Dir || !equalStringSlices(call.Args, wantCalls[index].Args) {
			t.Fatalf("Calls[%d] = %#v, want %#v", index, call, wantCalls[index])
		}
	}
}

func TestFakeRunner_Run_CopiesArgsSoCallerMutationIsInvisible(t *testing.T) {
	runner := git.NewFakeRunner()
	args := []string{"fetch", "origin"}
	_, err := runner.Run(context.Background(), "/repo", args...)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	args[1] = "mutated"
	if got := runner.Calls[0].Args[1]; got != "origin" {
		t.Fatalf("Calls[0].Args[1] = %q, want %q", got, "origin")
	}
}

func TestFakeRunner_CommandStrings_JoinsRecordedArgs(t *testing.T) {
	runner := git.NewFakeRunner()
	if got := runner.CommandStrings(); len(got) != 0 {
		t.Fatalf("CommandStrings() = %#v, want empty", got)
	}

	for _, call := range [][]string{{"status"}, {"worktree", "add", "/tmp/w"}} {
		_, err := runner.Run(context.Background(), "/repo", call...)
		if err != nil {
			t.Fatalf("Run(%v) error = %v", call, err)
		}
	}

	want := []string{"status", "worktree add /tmp/w"}
	if !equalStringSlices(runner.CommandStrings(), want) {
		t.Fatalf("CommandStrings() = %#v, want %#v", runner.CommandStrings(), want)
	}
}
