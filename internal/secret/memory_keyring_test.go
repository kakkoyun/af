package secret_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kakkoyun/af/internal/secret"
)

func TestMemoryKeyring_Delete_NotFound(t *testing.T) {
	store := secret.NewMemoryKeyring()
	err := store.Delete(context.Background(), "missing")
	if !errors.Is(err, secret.ErrNotFound) {
		t.Fatalf("Delete(missing) error = %v, want ErrNotFound", err)
	}
}

func TestMemoryKeyring_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := secret.NewMemoryKeyring()

	tests := []struct {
		call func() error
		name string
	}{
		{name: "Set", call: func() error { return store.Set(ctx, "key", "value") }},
		{name: "Get", call: func() error {
			_, err := store.Get(ctx, "key")
			if err != nil {
				return fmt.Errorf("get: %w", err)
			}
			return nil
		}},
		{name: "Delete", call: func() error { return store.Delete(ctx, "key") }},
		{name: "List", call: func() error {
			_, err := store.List(ctx)
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s(cancelled ctx) error = %v, want context.Canceled", tt.name, err)
			}
		})
	}
}
