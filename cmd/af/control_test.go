package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/control"
)

// errFakeControlNotFound is a sentinel for the fakeControlExec "not found" response.
var errFakeControlNotFound = errors.New("exec: not found")

// fakeControlExec implements control.Executor for cmd-level tests.
type fakeControlExec struct {
	responses map[string]fakeControlResponse
}

type fakeControlResponse struct {
	err error
	out []byte
}

func (f *fakeControlExec) Exec(_ context.Context, _, name string, args ...string) ([]byte, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	if r, ok := f.responses[key]; ok {
		return r.out, r.err
	}
	return nil, nil
}

// happyControlExec supplies all responses needed for a successful Up flow.
func happyControlExec() *fakeControlExec {
	return &fakeControlExec{
		responses: map[string]fakeControlResponse{
			"superterm --version": {out: []byte("superterm 0.5.0")},
			"tailscale --version": {out: []byte("1.98.2")},
			"superterm up":        {out: []byte("running at http://localhost:7681\n")},
			"tailscale serve --bg http://localhost:7681": {
				out: []byte("Available on your tailnet:\nhttps://mynode.my-tailnet.ts.net/\n"),
			},
			"tailscale serve off": {},
			"superterm down":      {},
			"superterm status":    {out: []byte("running at http://localhost:7681\n")},
			"tailscale serve status": {
				out: []byte("https://mynode.my-tailnet.ts.net/\n"),
			},
		},
	}
}

func withFakeControlExec(exec *fakeControlExec) func() {
	orig := controlExecutorFactory
	controlExecutorFactory = func() control.Executor { return exec }
	return func() { controlExecutorFactory = orig }
}

func TestControlUp_PrintsEndpoint(t *testing.T) {
	restore := withFakeControlExec(happyControlExec())
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "control", "up")
	if err != nil {
		t.Fatalf("control up error = %v, want nil", err)
	}
	if !strings.Contains(stdout, "http://localhost:7681") {
		t.Errorf("stdout missing LocalURL; got: %s", stdout)
	}
	if !strings.Contains(stdout, ".ts.net") {
		t.Errorf("stdout missing TailnetURL; got: %s", stdout)
	}
}

func TestControlUp_JSONOutput(t *testing.T) {
	restore := withFakeControlExec(happyControlExec())
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "control", "up", "--json")
	if err != nil {
		t.Fatalf("control up --json error = %v, want nil", err)
	}
	var ep struct {
		LocalURL   string `json:"local_url"`
		TailnetURL string `json:"tailnet_url"`
	}
	jsonErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &ep)
	if jsonErr != nil {
		t.Fatalf("unmarshal JSON error = %v; got: %s", jsonErr, stdout)
	}
	if ep.LocalURL == "" {
		t.Error("JSON local_url is empty")
	}
	if ep.TailnetURL == "" {
		t.Error("JSON tailnet_url is empty")
	}
}

func TestControlDown_Idempotent(t *testing.T) {
	restore := withFakeControlExec(happyControlExec())
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "control", "down")
	if err != nil {
		t.Fatalf("control down error = %v, want nil", err)
	}
	if !strings.Contains(stdout, "stopped") {
		t.Errorf("stdout missing 'stopped'; got: %s", stdout)
	}

	// Second call with a fresh root so cobra flag state is reset.
	stdout2, _, err2 := executeCommand(t, newRootCmd(), "control", "down")
	if err2 != nil {
		t.Fatalf("control down (2nd) error = %v, want nil", err2)
	}
	if !strings.Contains(stdout2, "stopped") {
		t.Errorf("stdout2 missing 'stopped'; got: %s", stdout2)
	}
}

func TestControlStatus_ReportsNotRunning(t *testing.T) {
	exec := &fakeControlExec{
		responses: map[string]fakeControlResponse{
			"superterm status":       {out: []byte("not running")},
			"tailscale serve status": {out: []byte("no mappings")},
		},
	}
	restore := withFakeControlExec(exec)
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "control", "status")
	if err != nil {
		t.Fatalf("control status error = %v, want nil", err)
	}
	if !strings.Contains(stdout, "not running") {
		t.Errorf("stdout missing 'not running'; got: %s", stdout)
	}
}

func TestControlUp_FailsWhenSupertermMissing(t *testing.T) {
	exec := &fakeControlExec{
		responses: map[string]fakeControlResponse{
			"superterm --version": {err: errFakeControlNotFound},
		},
	}
	restore := withFakeControlExec(exec)
	defer restore()

	_, _, err := executeCommand(t, newRootCmd(), "control", "up")
	if err == nil {
		t.Fatal("control up with missing superterm: want error, got nil")
	}
	if !errors.Is(err, control.ErrSupertermMissing) {
		t.Errorf("error = %v, want wrapping ErrSupertermMissing", err)
	}
}
