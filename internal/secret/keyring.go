package secret

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrNotFound reports a missing keyring entry.
var ErrNotFound = errors.New("secret not found")

// Keyring stores credentials by key name.
type Keyring interface {
	Set(ctx context.Context, key, value string) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context) ([]string, error)
}

// MemoryKeyring is an in-memory fake Keyring for tests.
type MemoryKeyring struct {
	entries map[string]string
	mu      sync.RWMutex
}

// NewMemoryKeyring returns an empty fake keyring.
func NewMemoryKeyring() *MemoryKeyring {
	return &MemoryKeyring{entries: map[string]string{}}
}

// Set stores value under key.
func (keyring *MemoryKeyring) Set(ctx context.Context, key, value string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("set secret %s: %w", key, err)
	}
	keyring.mu.Lock()
	defer keyring.mu.Unlock()
	keyring.entries[key] = value

	return nil
}

// Get returns the value stored under key.
func (keyring *MemoryKeyring) Get(ctx context.Context, key string) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", fmt.Errorf("get secret %s: %w", key, err)
	}
	keyring.mu.RLock()
	defer keyring.mu.RUnlock()
	value, ok := keyring.entries[key]
	if !ok {
		return "", fmt.Errorf("get secret %s: %w", key, ErrNotFound)
	}

	return value, nil
}

// Delete removes key from the keyring.
func (keyring *MemoryKeyring) Delete(ctx context.Context, key string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("delete secret %s: %w", key, err)
	}
	keyring.mu.Lock()
	defer keyring.mu.Unlock()
	if _, ok := keyring.entries[key]; !ok {
		return fmt.Errorf("delete secret %s: %w", key, ErrNotFound)
	}
	delete(keyring.entries, key)

	return nil
}

// List returns stored key names in lexical order.
func (keyring *MemoryKeyring) List(ctx context.Context) ([]string, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	keyring.mu.RLock()
	defer keyring.mu.RUnlock()
	keys := make([]string, 0, len(keyring.entries))
	for key := range keyring.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys, nil
}
