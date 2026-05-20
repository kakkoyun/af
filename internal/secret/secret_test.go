package secret_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/secret"
)

func TestRedactingHandler_RedactsBuiltInAndConfiguredKeys(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: nil})
	logger := slog.New(secret.NewRedactingHandler(base, []string{"session_id"}))

	logger.InfoContext(context.Background(), "launch",
		slog.String("token", "ghp_secret"),
		slog.String("user", "kemal"),
		slog.String("session_id", "session-secret"),
		slog.Group("nested", "api_key", "sk-secret", "safe", "ok"),
	)

	out := buf.String()
	for _, leaked := range []string{"ghp_secret", "session-secret", "sk-secret"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("log output leaked %q: %s", leaked, out)
		}
	}
	for _, want := range []string{"\"token\":\"<redacted>\"", "\"session_id\":\"<redacted>\"", "\"api_key\":\"<redacted>\"", "\"user\":\"kemal\"", "\"safe\":\"ok\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output %s missing %s", out, want)
		}
	}
}

func TestMemoryKeyring_SetGetDeleteAndList(t *testing.T) {
	ctx := context.Background()
	store := secret.NewMemoryKeyring()

	err := store.Set(ctx, "github_token", "ghp_secret")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	err = store.Set(ctx, "openai_api_key", "sk-secret")
	if err != nil {
		t.Fatalf("Set(second) error = %v", err)
	}

	got, err := store.Get(ctx, "github_token")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "ghp_secret" {
		t.Fatalf("Get() = %q, want stored value", got)
	}

	keys, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if strings.Join(keys, ",") != "github_token,openai_api_key" {
		t.Fatalf("List() = %#v, want sorted keys", keys)
	}

	err = store.Delete(ctx, "github_token")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err = store.Get(ctx, "github_token")
	if err == nil {
		t.Fatal("Get(deleted) error = nil, want not found")
	}
}
