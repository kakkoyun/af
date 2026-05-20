package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes git commands. Real implementations spawn `git`; the
// fake records invocations for tests.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// ErrRunnerCall reports any git invocation failure.
var ErrRunnerCall = errors.New("git runner call failed")

// ExecRunner runs `git` via os/exec.
type ExecRunner struct {
	Binary string
}

// NewExecRunner returns a Runner that shells out to `git`.
func NewExecRunner() ExecRunner {
	return ExecRunner{Binary: "git"}
}

// Run executes `git <args>` in dir and returns combined stdout+stderr.
func (r ExecRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	binary := r.Binary
	if binary == "" {
		binary = "git"
	}
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec // args are constructed by typed helpers in this package, not from shell input.
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), dir, ErrRunnerCall, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// FakeRunner records each Run invocation for tests and returns canned
// responses. If no canned response is set, Run returns an empty []byte
// and nil error.
type FakeRunner struct { //nolint:govet // Map+slice field order prioritises readability.
	Calls     []FakeCall
	Responses map[string]FakeResponse
}

// FakeCall captures one Runner.Run invocation.
type FakeCall struct {
	Dir  string
	Args []string
}

// FakeResponse is the canned reply for a matching args signature.
type FakeResponse struct {
	Err    error
	Output string
}

// NewFakeRunner returns an empty FakeRunner.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{Responses: make(map[string]FakeResponse)}
}

// SetResponse arranges that subsequent Run calls matching args return
// resp. The args signature is `strings.Join(args, " ")`.
func (r *FakeRunner) SetResponse(args []string, resp FakeResponse) {
	r.Responses[strings.Join(args, " ")] = resp
}

// Run records the call and returns the canned response if one matches.
func (r *FakeRunner) Run(_ context.Context, dir string, args ...string) ([]byte, error) {
	r.Calls = append(r.Calls, FakeCall{Dir: dir, Args: append([]string(nil), args...)})
	resp, ok := r.Responses[strings.Join(args, " ")]
	if !ok {
		return nil, nil
	}
	if resp.Err != nil {
		return []byte(resp.Output), resp.Err
	}
	return []byte(resp.Output), nil
}

// CommandStrings returns recorded calls as "args" lines for assertions.
func (r *FakeRunner) CommandStrings() []string {
	out := make([]string, 0, len(r.Calls))
	for _, c := range r.Calls {
		out = append(out, strings.Join(c.Args, " "))
	}
	return out
}
