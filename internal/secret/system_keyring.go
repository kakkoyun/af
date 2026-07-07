package secret

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/zalando/go-keyring"
)

// SystemKeyring wraps github.com/zalando/go-keyring with the project's
// service-name convention and exposes a Keyring-compatible surface.
//
// Account names are the credential keys (e.g. anthropic_api_key); the
// service name is configured via [secret].keyring_service in ADR-036
// (default "af"). The package keeps a per-service index of known
// accounts under a dedicated index key so List can enumerate them.
type SystemKeyring struct {
	service string
}

// NewSystemKeyring returns a SystemKeyring bound to service.
func NewSystemKeyring(service string) *SystemKeyring {
	if service == "" {
		service = "af"
	}
	return &SystemKeyring{service: service}
}

// indexAccount is the keyring entry that records the set of stored
// account names for the service. The OS keyring API has no native
// enumeration, so we maintain this index manually.
const indexAccount = "__af_index__"

// Seams over the go-keyring package functions, replaced in internal
// tests by an in-memory fake so tests never touch the OS keychain.
var (
	keyringSet    = keyring.Set    //nolint:gochecknoglobals // Test seam: replaced in internal tests to avoid the real OS keychain.
	keyringGet    = keyring.Get    //nolint:gochecknoglobals // Test seam: replaced in internal tests to avoid the real OS keychain.
	keyringDelete = keyring.Delete //nolint:gochecknoglobals // Test seam: replaced in internal tests to avoid the real OS keychain.
)

// Set stores value under key and updates the per-service index.
func (k *SystemKeyring) Set(_ context.Context, key, value string) error {
	if key == "" || key == indexAccount {
		return fmt.Errorf("set secret %q: %w", key, ErrInvalidKey)
	}
	err := keyringSet(k.service, key, value)
	if err != nil {
		return fmt.Errorf("keyring set %s/%s: %w", k.service, key, err)
	}
	return k.addToIndex(key)
}

// Get returns the value stored under key.
func (k *SystemKeyring) Get(_ context.Context, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("get secret: %w", ErrInvalidKey)
	}
	value, err := keyringGet(k.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("get secret %s: %w", key, ErrNotFound)
		}
		return "", fmt.Errorf("keyring get %s/%s: %w", k.service, key, err)
	}
	return value, nil
}

// Delete removes key from the keyring and the per-service index.
func (k *SystemKeyring) Delete(_ context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("delete secret: %w", ErrInvalidKey)
	}
	err := keyringDelete(k.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("delete secret %s: %w", key, ErrNotFound)
		}
		return fmt.Errorf("keyring delete %s/%s: %w", k.service, key, err)
	}
	return k.removeFromIndex(key)
}

// List returns the keys recorded in the per-service index, sorted.
func (k *SystemKeyring) List(_ context.Context) ([]string, error) {
	keys, err := k.readIndex()
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}

func (k *SystemKeyring) readIndex() ([]string, error) {
	raw, err := keyringGet(k.service, indexAccount)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("read keyring index: %w", err)
	}
	return splitIndex(raw), nil
}

func (k *SystemKeyring) writeIndex(keys []string) error {
	err := keyringSet(k.service, indexAccount, joinIndex(keys))
	if err != nil {
		return fmt.Errorf("write keyring index: %w", err)
	}
	return nil
}

func (k *SystemKeyring) addToIndex(key string) error {
	existing, err := k.readIndex()
	if err != nil {
		return err
	}
	for _, k := range existing {
		if k == key {
			return nil
		}
	}
	return k.writeIndex(append(existing, key))
}

func (k *SystemKeyring) removeFromIndex(key string) error {
	existing, err := k.readIndex()
	if err != nil {
		return err
	}
	out := make([]string, 0, len(existing))
	for _, k := range existing {
		if k != key {
			out = append(out, k)
		}
	}
	return k.writeIndex(out)
}

// ErrInvalidKey reports an empty or reserved key passed to the keyring.
var ErrInvalidKey = errors.New("invalid secret key")

func splitIndex(raw string) []string {
	if raw == "" {
		return nil
	}
	out := make([]string, 0)
	start := 0
	for i := range len(raw) {
		if raw[i] == '\n' {
			if i > start {
				out = append(out, raw[start:i])
			}
			start = i + 1
		}
	}
	if start < len(raw) {
		out = append(out, raw[start:])
	}
	return out
}

func joinIndex(keys []string) string {
	out := ""
	for i, key := range keys {
		if i > 0 {
			out += "\n"
		}
		out += key
	}
	return out
}
