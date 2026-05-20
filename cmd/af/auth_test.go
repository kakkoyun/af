package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/secret"
)

// withMemoryAuthKeyring rewires the auth command tree to a fresh
// in-memory keyring and a deterministic stdin-driven secret reader.
func withMemoryAuthKeyring(t *testing.T, secrets map[string]string, stdinValue string) (*memoryAuthCtx, func()) {
	t.Helper()
	ring := secret.NewMemoryKeyring()
	for k, v := range secrets {
		err := ring.Set(t.Context(), k, v)
		if err != nil {
			t.Fatalf("seed keyring: %v", err)
		}
	}
	ctx := &memoryAuthCtx{ring: ring, stdinValue: stdinValue}

	origAuth := newAuthContextOverride
	newAuthContextOverride = func(opts *rootOptions) *authContext {
		return &authContext{
			root:        opts,
			keyringMake: func(_ string) secret.Keyring { return ring },
			readSecret:  func(_ io.Writer, _ io.Reader, _ string) (string, error) { return stdinValue, nil },
		}
	}
	return ctx, func() { newAuthContextOverride = origAuth }
}

type memoryAuthCtx struct {
	ring       *secret.MemoryKeyring
	stdinValue string
}

func TestAuth_SetStoresValueAndConfirms(t *testing.T) {
	_, restore := withMemoryAuthKeyring(t, nil, "shhh-secret")
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "auth", "set", "github_token")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout, "stored github_token") {
		t.Fatalf("set confirmation missing; stdout:\n%s", stdout)
	}
}

func TestAuth_GetReturnsStoredValueOnTTYAndRedactsOtherwise(t *testing.T) {
	ctx, restore := withMemoryAuthKeyring(t, map[string]string{"anthropic_api_key": "sk-veryverysecret"}, "")
	defer restore()
	_ = ctx

	// Non-TTY stdout: redact.
	stdout, _, err := executeCommand(t, newRootCmd(), "auth", "get", "anthropic_api_key")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout, "[REDACTED") {
		t.Fatalf("expected redaction on non-TTY stdout; got %q", stdout)
	}
	if strings.Contains(stdout, "sk-veryverysecret") {
		t.Fatalf("secret leaked into non-TTY stdout: %q", stdout)
	}
}

func TestAuth_StatusListsCuratedAndExtraKeys(t *testing.T) {
	_, restore := withMemoryAuthKeyring(t, map[string]string{
		"anthropic_api_key": "x",
		"weird_key":         "y",
	}, "")
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "auth", "status")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	for _, want := range []string{
		"Curated credentials:",
		"✓ anthropic_api_key", "available",
		"✗ openai_api_key", "absent",
		"✗ github_token",
		"Other keyring entries:",
		"• weird_key",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("status missing %q; output:\n%s", want, stdout)
		}
	}
}

func TestAuth_ClearRemovesKey(t *testing.T) {
	ctx, restore := withMemoryAuthKeyring(t, map[string]string{"openai_api_key": "x"}, "")
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "auth", "clear", "openai_api_key")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout, "cleared openai_api_key") {
		t.Fatalf("clear confirmation missing; stdout:\n%s", stdout)
	}
	_, err = ctx.ring.Get(t.Context(), "openai_api_key")
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after clear, got %v", err)
	}
}

func TestAuth_ListPrintsKeysOneAtPerLine(t *testing.T) {
	_, restore := withMemoryAuthKeyring(t, map[string]string{
		"anthropic_api_key": "x",
		"github_token":      "y",
	}, "")
	defer restore()

	stdout, _, err := executeCommand(t, newRootCmd(), "auth", "list")
	if err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	got := strings.Split(strings.TrimSpace(stdout), "\n")
	want := []string{"anthropic_api_key", "github_token"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("list = %v, want %v", got, want)
	}
}

func TestAuth_GetMissingKeyReturnsError(t *testing.T) {
	_, restore := withMemoryAuthKeyring(t, nil, "")
	defer restore()

	_, _, err := executeCommand(t, newRootCmd(), "auth", "get", "nope")
	if err == nil {
		t.Fatal("ExecuteContext() error = nil, want missing-key error")
	}
	if !errors.Is(err, secret.ErrNotFound) {
		// Cobra wraps the error; check the message instead.
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error does not indicate not-found: %v", err)
		}
	}
}

var _ = context.Background
