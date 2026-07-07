//go:build integration

package secret_test

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/secret"
)

// TestIntegrationSystemKeyring_RoundTrip exercises the real OS keychain
// (macOS Keychain in CI; Secret Service on a Linux desktop). It runs
// only under -tags integration on hosts with a usable keychain and
// covers the path the hermetic suite fakes: Set/Get/List/Delete against
// the actual backend, including the sidecar index entry.
func TestIntegrationSystemKeyring_RoundTrip(t *testing.T) {
	if os.Getenv("AF_INTEGRATION_KEYRING") == "" {
		t.Skip("AF_INTEGRATION_KEYRING not set; requires a real, unlocked keychain")
	}

	// A unique service name isolates this run from real af credentials
	// and from concurrent CI runs on the same runner image.
	service := fmt.Sprintf("af-integration-%d-%d", os.Getpid(), time.Now().UnixNano())
	k := secret.NewSystemKeyring(service)
	ctx := t.Context()

	t.Cleanup(func() {
		_ = k.Delete(ctx, "alpha_key") //nolint:errcheck // Best-effort cleanup of CI keychain entries.
		_ = k.Delete(ctx, "beta_key")  //nolint:errcheck // Best-effort cleanup of CI keychain entries.
	})

	err := k.Set(ctx, "alpha_key", "alpha-value-123")
	if err != nil {
		t.Fatalf("Set(alpha_key): %v", err)
	}
	err = k.Set(ctx, "beta_key", "beta-value-456")
	if err != nil {
		t.Fatalf("Set(beta_key): %v", err)
	}

	got, err := k.Get(ctx, "alpha_key")
	if err != nil {
		t.Fatalf("Get(alpha_key): %v", err)
	}
	if got != "alpha-value-123" {
		t.Fatalf("Get(alpha_key) = %q, want alpha-value-123", got)
	}

	assertKeyringListThenDelete(t, k)
}

func assertKeyringListThenDelete(t *testing.T, k *secret.SystemKeyring) {
	t.Helper()
	ctx := t.Context()
	keys, err := k.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("List = %v, want exactly [alpha_key beta_key]", keys)
	}

	err = k.Delete(ctx, "alpha_key")
	if err != nil {
		t.Fatalf("Delete(alpha_key): %v", err)
	}
	_, err = k.Get(ctx, "alpha_key")
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}

	keys, err = k.List(ctx)
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	if len(keys) != 1 || keys[0] != "beta_key" {
		t.Fatalf("List after Delete = %v, want [beta_key]", keys)
	}
}
