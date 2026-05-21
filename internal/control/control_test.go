package control_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/control"
)

// sentinel errors for test fakes (avoids err113 dynamic-error lint).
var (
	errFakeNotFound   = errors.New("exec: not found")
	errFakeNotRunning = errors.New("not running")
)

// fakeExec implements Executor for tests. responses maps
// "binary arg0 arg1..." to a (output, error) pair.
type fakeExec struct {
	responses map[string]fakeResponse
}

type fakeResponse struct {
	err error
	out []byte
}

func (f *fakeExec) Exec(_ context.Context, _, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), " ")
	if r, ok := f.responses[key]; ok {
		return r.out, r.err
	}
	// Default: success with empty output.
	return nil, nil
}

// happyExec returns a fakeExec with responses that satisfy a full Up flow.
func happyExec() *fakeExec {
	return &fakeExec{
		responses: map[string]fakeResponse{
			"superterm --version":                        {out: []byte("superterm 0.5.0")},
			"tailscale --version":                        {out: []byte("1.98.2\n  tailscale commit: abc")},
			"superterm up":                               {out: []byte("Superterm running at http://localhost:7681\n")},
			"tailscale serve --bg http://localhost:7681": {out: []byte("Available on your tailnet:\nhttps://mynode.my-tailnet.ts.net/\n")},
			"tailscale serve off":                        {},
			"superterm down":                             {},
			"superterm status":                           {out: []byte("running at http://localhost:7681\n")},
			"tailscale serve status":                     {out: []byte("https://mynode.my-tailnet.ts.net/\n")},
		},
	}
}

func deps(exec control.Executor) control.Deps {
	return control.Deps{Exec: exec}
}

func defaultOpts() control.Options {
	return control.Options{Provider: control.ProviderSuperterm}
}

// --- Up tests ----------------------------------------------------------------

func TestUp_HappyPath(t *testing.T) {
	ep, err := control.Up(t.Context(), deps(happyExec()), defaultOpts())
	if err != nil {
		t.Fatalf("Up() error = %v, want nil", err)
	}
	if ep.LocalURL != "http://localhost:7681" {
		t.Errorf("LocalURL = %q, want http://localhost:7681", ep.LocalURL)
	}
	if !strings.HasPrefix(ep.TailnetURL, "https://") || !strings.Contains(ep.TailnetURL, ".ts.net") {
		t.Errorf("TailnetURL = %q, want https://*.ts.net URL", ep.TailnetURL)
	}
	if ep.Host != "" {
		t.Errorf("Host = %q, want empty for local session", ep.Host)
	}
}

func TestUp_SupertermMissing(t *testing.T) {
	exec := &fakeExec{
		responses: map[string]fakeResponse{
			"superterm --version": {err: errFakeNotFound},
		},
	}
	_, err := control.Up(t.Context(), deps(exec), defaultOpts())
	if !errors.Is(err, control.ErrSupertermMissing) {
		t.Fatalf("Up() error = %v, want ErrSupertermMissing", err)
	}
}

func TestUp_TailscaleMissing(t *testing.T) {
	exec := &fakeExec{
		responses: map[string]fakeResponse{
			"superterm --version": {out: []byte("superterm 0.5.0")},
			"tailscale --version": {err: errFakeNotFound},
		},
	}
	_, err := control.Up(t.Context(), deps(exec), defaultOpts())
	if !errors.Is(err, control.ErrTailscaleMissing) {
		t.Fatalf("Up() error = %v, want ErrTailscaleMissing", err)
	}
}

func TestUp_RejectsUnsupportedProvider(t *testing.T) {
	opts := control.Options{Provider: "ngrok"}
	_, err := control.Up(t.Context(), deps(happyExec()), opts)
	if !errors.Is(err, control.ErrProviderUnsupported) {
		t.Fatalf("Up() error = %v, want ErrProviderUnsupported", err)
	}
}

func TestUp_RemoteWrapsCallsViaSSH(t *testing.T) {
	exec := &fakeExec{
		responses: map[string]fakeResponse{
			// Remote mode wraps all calls as: ssh <host> <binary> <args...>
			"ssh work-mini superterm --version": {out: []byte("superterm 0.5.0")},
			"ssh work-mini tailscale --version": {out: []byte("1.98.2")},
			"ssh work-mini superterm up":        {out: []byte("http://localhost:7681")},
			"ssh work-mini tailscale serve --bg http://localhost:7681": {
				out: []byte("https://work-mini.tailnet.ts.net/"),
			},
		},
	}
	opts := control.Options{Provider: control.ProviderSuperterm, Host: "work-mini"}
	ep, err := control.Up(t.Context(), deps(exec), opts)
	if err != nil {
		t.Fatalf("Up() error = %v, want nil", err)
	}
	if ep.Host != "work-mini" {
		t.Errorf("Host = %q, want work-mini", ep.Host)
	}
	if !strings.Contains(ep.TailnetURL, "work-mini") {
		t.Errorf("TailnetURL = %q, want URL containing work-mini", ep.TailnetURL)
	}
}

// --- Down tests --------------------------------------------------------------

func TestDown_Idempotent(t *testing.T) {
	// First call: both succeed.
	firstErr := control.Down(t.Context(), deps(happyExec()), defaultOpts())
	if firstErr != nil {
		t.Fatalf("Down() first call error = %v, want nil", firstErr)
	}

	// Second call: both binaries return errors (nothing is running); Down still
	// succeeds — teardown errors are surfaced but do not prevent other steps.
	exec := &fakeExec{
		responses: map[string]fakeResponse{
			"tailscale serve off": {err: errFakeNotRunning},
			"superterm down":      {err: errFakeNotRunning},
		},
	}
	// Down with all-fail exec should return an error (both steps failed), not panic.
	err := control.Down(t.Context(), deps(exec), defaultOpts())
	if err == nil {
		t.Fatal("Down() with all-failing exec expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "tailscale serve off") || !strings.Contains(err.Error(), "superterm down") {
		t.Errorf("Down() error = %v, want both step names in message", err)
	}
}

// --- Status tests ------------------------------------------------------------

func TestStatus_ReportsLiveEndpoint(t *testing.T) {
	ep, ok, err := control.Status(t.Context(), deps(happyExec()), defaultOpts())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if !ok {
		t.Fatal("Status() ok = false, want true when both services are running")
	}
	if ep.LocalURL == "" {
		t.Error("Status() LocalURL is empty")
	}
	if ep.TailnetURL == "" {
		t.Error("Status() TailnetURL is empty")
	}
}

func TestStatus_ReportsNoEndpoint(t *testing.T) {
	exec := &fakeExec{
		responses: map[string]fakeResponse{
			"superterm status":       {out: []byte("not running")}, // no URL in output
			"tailscale serve status": {out: []byte("no mappings")}, // no URL in output
		},
	}
	_, ok, err := control.Status(t.Context(), deps(exec), defaultOpts())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if ok {
		t.Fatal("Status() ok = true, want false when neither service is running")
	}
}
