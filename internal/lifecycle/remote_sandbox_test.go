package lifecycle_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/remote"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/secret"
)

// ---- in-test sandbox fake that captures envelope content at launch time -----

// envelopeCaptureSandbox implements sandbox.Sandbox. Its Launch reads the
// file at envelopePath (if set) and stores the content in capturedBody so
// tests can assert on what was written before the deferred Delete fires.
type envelopeCaptureSandbox struct {
	envelopePath string
	capturedBody string
}

func (*envelopeCaptureSandbox) Name() string                                      { return "capture" }
func (*envelopeCaptureSandbox) IsAvailable(_ context.Context) bool                { return true }
func (*envelopeCaptureSandbox) Attach(_ context.Context, _ *sandbox.Handle) error { return nil }
func (*envelopeCaptureSandbox) IsHealthy(_ context.Context, _ *sandbox.Handle) (bool, error) {
	return true, nil
}
func (*envelopeCaptureSandbox) Teardown(_ context.Context, _ *sandbox.Handle) error { return nil }
func (*envelopeCaptureSandbox) List(_ context.Context) ([]sandbox.Handle, error) {
	return nil, nil
}

// Launch reads the envelope file (if envelopePath set) before returning.
func (f *envelopeCaptureSandbox) Launch(_ context.Context, _ sandbox.LaunchOpts) (*sandbox.Handle, error) {
	if f.envelopePath != "" {
		data, readErr := os.ReadFile(f.envelopePath)
		if readErr == nil {
			f.capturedBody = string(data)
		} else {
			f.capturedBody = "READ_ERROR:" + readErr.Error()
		}
	}
	return &sandbox.Handle{ID: "test-handle"}, nil
}

// ---- in-test remote executor that captures envelope content on first Run ----

// envelopeCaptureExecutor implements remote.Executor. Its first Run call
// reads the file at envelopePath and stores the content.
type envelopeCaptureExecutor struct {
	envelopePath string
	capturedBody string
	called       int
}

func (e *envelopeCaptureExecutor) Run(_ context.Context, _ remote.Command) ([]byte, error) {
	e.called++
	if e.called == 1 && e.envelopePath != "" {
		data, readErr := os.ReadFile(e.envelopePath)
		if readErr == nil {
			e.capturedBody = string(data)
		} else {
			e.capturedBody = "READ_ERROR:" + readErr.Error()
		}
	}
	return nil, nil
}

// ---- sandbox envelope tests --------------------------------------------------

func TestLaunchSandbox_WritesAndDeletesEnvelope(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "af-envelope")

	capFake := &envelopeCaptureSandbox{envelopePath: envPath}
	sc := lifecycle.SandboxContext{
		Provider: capFake,
		Envelope: secret.Envelope{
			Path:    envPath,
			Entries: map[string]string{"FOO": "bar", "API_KEY": "secret123"},
		},
	}
	opts := sandbox.LaunchOpts{Workstream: "test-ws", Worktree: tmp}

	handle, err := lifecycle.LaunchSandboxWorkstream(context.Background(), sc, opts)
	if err != nil {
		t.Fatalf("LaunchSandboxWorkstream: %v", err)
	}
	if handle == nil {
		t.Fatal("handle is nil")
	}

	// The fake captured the file content during its Launch call.
	if capFake.capturedBody == "" {
		t.Fatal("envelope was not read during Launch — file not written before Launch")
	}
	if capFake.capturedBody[:len("READ_ERROR:")] == "READ_ERROR:" {
		t.Fatalf("envelope read error during Launch: %s", capFake.capturedBody)
	}
	assertContainsLine(t, "captured body", capFake.capturedBody, "FOO=bar")
	assertContainsLine(t, "captured body", capFake.capturedBody, "API_KEY=secret123")

	// After return, the deferred Delete must have run.
	_, statErr := os.Stat(envPath)
	if !os.IsNotExist(statErr) {
		t.Fatalf("envelope file still present after LaunchSandboxWorkstream returned (stat: %v)", statErr)
	}
}

func TestLaunchSandbox_EmptyEnvelopePathSkipsWrite(t *testing.T) {
	fake := sandbox.NewFake("noop")
	sc := lifecycle.SandboxContext{
		Provider: fake,
		Envelope: secret.Envelope{}, // no path — nothing to write
	}
	opts := sandbox.LaunchOpts{Workstream: "noop-ws", Worktree: t.TempDir()}

	handle, err := lifecycle.LaunchSandboxWorkstream(context.Background(), sc, opts)
	if err != nil {
		t.Fatalf("LaunchSandboxWorkstream with empty envelope: %v", err)
	}
	if handle == nil {
		t.Fatal("expected non-nil handle")
	}
}

func TestLaunchSandbox_NilProviderErrors(t *testing.T) {
	sc := lifecycle.SandboxContext{Provider: nil}
	_, err := lifecycle.LaunchSandboxWorkstream(context.Background(), sc, sandbox.LaunchOpts{})
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
	if !errors.Is(err, lifecycle.ErrSandboxSetup) {
		t.Fatalf("err = %v, want to wrap ErrSandboxSetup", err)
	}
}

func TestLaunchSandbox_EnvelopeWriteErrorAborts(t *testing.T) {
	// Use an existing directory as the target path: os.WriteFile on a
	// directory returns "is a directory", which Write() wraps as an error.
	badPath := t.TempDir() // directory, not a file
	fake := sandbox.NewFake("noop")
	sc := lifecycle.SandboxContext{
		Provider: fake,
		Envelope: secret.Envelope{Path: badPath, Entries: map[string]string{"K": "v"}},
	}
	_, err := lifecycle.LaunchSandboxWorkstream(context.Background(), sc, sandbox.LaunchOpts{Workstream: "ws"})
	if err == nil {
		t.Fatal("expected error when envelope write fails")
	}
	// Should not wrap ErrSandboxSetup (the sandbox was never launched).
	if errors.Is(err, lifecycle.ErrSandboxSetup) {
		t.Fatalf("err should not wrap ErrSandboxSetup for an envelope write failure; got %v", err)
	}
}

// ---- remote envelope tests --------------------------------------------------

func TestPrepareRemote_WritesAndDeletesEnvelope(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, "af-remote-envelope")

	capExec := &envelopeCaptureExecutor{envelopePath: envPath}
	rc := lifecycle.RemoteContext{
		Host:       "fake-host",
		SSHOptions: []string{"-o", "BatchMode=yes"},
		Envelope: secret.Envelope{
			Path:    envPath,
			Entries: map[string]string{"TOKEN": "abc"},
		},
	}

	// Use SSH seam with the capturing executor.
	rcWithExec := remoteContextWithExecutor(rc, capExec)
	remotePath, err := lifecycle.PrepareRemoteWorkstream(
		context.Background(), rcWithExec, "owner/repo", "feat/x", "main",
	)
	if err != nil {
		t.Fatalf("PrepareRemoteWorkstream: %v", err)
	}
	if remotePath == "" {
		t.Fatal("expected non-empty remote path")
	}

	// The executor captured the file content on its first Run call.
	if capExec.capturedBody == "" {
		t.Fatal("envelope was not present during first SSH command")
	}
	assertContainsLine(t, "captured body", capExec.capturedBody, "TOKEN=abc")

	// After return, the deferred Delete must have fired.
	_, statErr := os.Stat(envPath)
	if !os.IsNotExist(statErr) {
		t.Fatalf("envelope file still present after PrepareRemoteWorkstream returned (stat: %v)", statErr)
	}
}

func TestPrepareRemote_EmptyEnvelopePathSkipsWrite(t *testing.T) {
	capExec := &envelopeCaptureExecutor{}
	rc := lifecycle.RemoteContext{
		Host:     "fake-host",
		Envelope: secret.Envelope{}, // no path
	}
	rcWithExec := remoteContextWithExecutor(rc, capExec)
	_, err := lifecycle.PrepareRemoteWorkstream(
		context.Background(), rcWithExec, "owner/repo", "feat/x", "main",
	)
	if err != nil {
		t.Fatalf("PrepareRemoteWorkstream with empty envelope: %v", err)
	}
}

func TestPrepareRemote_EmptyHostErrors(t *testing.T) {
	rc := lifecycle.RemoteContext{Host: ""}
	_, err := lifecycle.PrepareRemoteWorkstream(context.Background(), rc, "repo", "br", "main")
	if err == nil {
		t.Fatal("expected error for empty host")
	}
	if !errors.Is(err, lifecycle.ErrRemoteSetup) {
		t.Fatalf("err = %v, want to wrap ErrRemoteSetup", err)
	}
}

// ---- helpers ----------------------------------------------------------------

// remoteContextWithExecutor returns a copy of rc with SSHExecutor set to
// exec so that PrepareRemoteWorkstream routes SSH calls through exec
// instead of a real ssh binary.
func remoteContextWithExecutor(rc lifecycle.RemoteContext, exec *envelopeCaptureExecutor) lifecycle.RemoteContext {
	rc.SSHExecutor = exec
	return rc
}

func assertContainsLine(t *testing.T, label, body, line string) {
	t.Helper()
	for _, l := range splitLines(body) {
		if l == line {
			return
		}
	}
	t.Fatalf("%s does not contain line %q:\n%s", label, line, body)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
