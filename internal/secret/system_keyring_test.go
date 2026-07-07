package secret

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// fakeBackend is an in-memory stand-in for the OS keychain. Error hooks
// let tests fail individual operations, including index-only failures
// that the real go-keyring mock cannot express.
type fakeBackend struct {
	store  map[string]map[string]string
	setErr func(service, user string) error
	getErr func(service, user string) error
	delErr func(service, user string) error
}

func (f *fakeBackend) set(service, user, pass string) error {
	if f.setErr != nil {
		err := f.setErr(service, user)
		if err != nil {
			return err
		}
	}
	if f.store[service] == nil {
		f.store[service] = map[string]string{}
	}
	f.store[service][user] = pass
	return nil
}

func (f *fakeBackend) get(service, user string) (string, error) {
	if f.getErr != nil {
		err := f.getErr(service, user)
		if err != nil {
			return "", err
		}
	}
	value, ok := f.store[service][user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (f *fakeBackend) delete(service, user string) error {
	if f.delErr != nil {
		err := f.delErr(service, user)
		if err != nil {
			return err
		}
	}
	if _, ok := f.store[service][user]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.store[service], user)
	return nil
}

// installFakeBackend swaps the go-keyring seams for an in-memory fake
// and restores the real functions when the test finishes.
func installFakeBackend(t *testing.T) *fakeBackend {
	t.Helper()
	fake := &fakeBackend{store: map[string]map[string]string{}}
	origSet, origGet, origDelete := keyringSet, keyringGet, keyringDelete
	keyringSet, keyringGet, keyringDelete = fake.set, fake.get, fake.delete
	t.Cleanup(func() {
		keyringSet, keyringGet, keyringDelete = origSet, origGet, origDelete
	})
	return fake
}

var (
	errFakeKeychain   = errors.New("keychain locked")
	errFakeIndexRead  = errors.New("index read failed")
	errFakeIndexWrite = errors.New("index write failed")
)

// failOn returns an error hook that fails only for the given account name.
func failOn(user string, err error) func(string, string) error {
	return func(_, gotUser string) error {
		if gotUser == user {
			return err
		}
		return nil
	}
}

func TestNewSystemKeyring_DefaultsEmptyServiceToAf(t *testing.T) {
	tests := []struct {
		name    string
		service string
		want    string
	}{
		{name: "empty defaults to af", service: "", want: "af"},
		{name: "explicit service preserved", service: "af-test", want: "af-test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := installFakeBackend(t)
			k := NewSystemKeyring(tt.service)
			if k.service != tt.want {
				t.Fatalf("NewSystemKeyring(%q).service = %q, want %q", tt.service, k.service, tt.want)
			}
			err := k.Set(context.Background(), "github_token", "ghp_secret")
			if err != nil {
				t.Fatalf("Set() error = %v", err)
			}
			if got := fake.store[tt.want]["github_token"]; got != "ghp_secret" {
				t.Fatalf("backend store under service %q = %q, want stored value", tt.want, got)
			}
		})
	}
}

// assertListedKeys verifies List returns exactly the given comma-joined keys.
func assertListedKeys(t *testing.T, ctx context.Context, k *SystemKeyring, want string) {
	t.Helper()
	keys, err := k.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if strings.Join(keys, ",") != want {
		t.Fatalf("List() = %#v, want keys %q", keys, want)
	}
}

func TestSystemKeyring_RoundTrip(t *testing.T) {
	installFakeBackend(t)
	ctx := context.Background()
	k := NewSystemKeyring("af-test")

	for _, key := range []string{"openai_api_key", "github_token"} {
		setErr := k.Set(ctx, key, "value-"+key)
		if setErr != nil {
			t.Fatalf("Set(%s) error = %v", key, setErr)
		}
	}

	got, err := k.Get(ctx, "github_token")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "value-github_token" {
		t.Fatalf("Get() = %q, want stored value", got)
	}
	assertListedKeys(t, ctx, k, "github_token,openai_api_key")

	err = k.Delete(ctx, "github_token")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err = k.Get(ctx, "github_token")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(deleted) error = %v, want ErrNotFound", err)
	}
	assertListedKeys(t, ctx, k, "openai_api_key")
}

func TestSystemKeyring_Set_RejectsInvalidKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "empty key", key: ""},
		{name: "reserved index key", key: indexAccount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installFakeBackend(t)
			k := NewSystemKeyring("af-test")
			err := k.Set(context.Background(), tt.key, "value")
			if !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("Set(%q) error = %v, want ErrInvalidKey", tt.key, err)
			}
		})
	}
}

func TestSystemKeyring_Set_BackendError(t *testing.T) {
	fake := installFakeBackend(t)
	fake.setErr = failOn("github_token", errFakeKeychain)

	k := NewSystemKeyring("af-test")
	err := k.Set(context.Background(), "github_token", "ghp_secret")
	if !errors.Is(err, errFakeKeychain) {
		t.Fatalf("Set() error = %v, want wrapped backend error", err)
	}
	if !strings.Contains(err.Error(), "keyring set af-test/github_token") {
		t.Fatalf("Set() error = %v, want service/key context", err)
	}
}

func TestSystemKeyring_Set_DuplicateKeyKeepsIndexUnique(t *testing.T) {
	installFakeBackend(t)
	ctx := context.Background()
	k := NewSystemKeyring("af-test")

	for range 2 {
		err := k.Set(ctx, "github_token", "ghp_secret")
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	keys, err := k.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if strings.Join(keys, ",") != "github_token" {
		t.Fatalf("List() = %#v, want single index entry", keys)
	}
}

func TestSystemKeyring_Set_IndexReadError(t *testing.T) {
	fake := installFakeBackend(t)
	fake.getErr = failOn(indexAccount, errFakeIndexRead)

	k := NewSystemKeyring("af-test")
	err := k.Set(context.Background(), "github_token", "ghp_secret")
	if !errors.Is(err, errFakeIndexRead) {
		t.Fatalf("Set() error = %v, want wrapped index read error", err)
	}
}

func TestSystemKeyring_Set_IndexWriteError(t *testing.T) {
	fake := installFakeBackend(t)
	fake.setErr = failOn(indexAccount, errFakeIndexWrite)

	k := NewSystemKeyring("af-test")
	err := k.Set(context.Background(), "github_token", "ghp_secret")
	if !errors.Is(err, errFakeIndexWrite) {
		t.Fatalf("Set() error = %v, want wrapped index write error", err)
	}
}

func TestSystemKeyring_Get_EmptyKey(t *testing.T) {
	installFakeBackend(t)
	k := NewSystemKeyring("af-test")
	_, err := k.Get(context.Background(), "")
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Get(\"\") error = %v, want ErrInvalidKey", err)
	}
}

func TestSystemKeyring_Get_NotFound(t *testing.T) {
	installFakeBackend(t)
	k := NewSystemKeyring("af-test")
	_, err := k.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrNotFound", err)
	}
}

func TestSystemKeyring_Get_BackendError(t *testing.T) {
	fake := installFakeBackend(t)
	fake.getErr = failOn("github_token", errFakeKeychain)

	k := NewSystemKeyring("af-test")
	_, err := k.Get(context.Background(), "github_token")
	if !errors.Is(err, errFakeKeychain) {
		t.Fatalf("Get() error = %v, want wrapped backend error", err)
	}
}

func TestSystemKeyring_Delete_EmptyKey(t *testing.T) {
	installFakeBackend(t)
	k := NewSystemKeyring("af-test")
	err := k.Delete(context.Background(), "")
	if !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("Delete(\"\") error = %v, want ErrInvalidKey", err)
	}
}

func TestSystemKeyring_Delete_NotFound(t *testing.T) {
	installFakeBackend(t)
	k := NewSystemKeyring("af-test")
	err := k.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) error = %v, want ErrNotFound", err)
	}
}

func TestSystemKeyring_Delete_BackendError(t *testing.T) {
	fake := installFakeBackend(t)
	fake.delErr = failOn("github_token", errFakeKeychain)

	k := NewSystemKeyring("af-test")
	err := k.Delete(context.Background(), "github_token")
	if !errors.Is(err, errFakeKeychain) {
		t.Fatalf("Delete() error = %v, want wrapped backend error", err)
	}
}

func TestSystemKeyring_Delete_IndexReadError(t *testing.T) {
	fake := installFakeBackend(t)
	ctx := context.Background()
	k := NewSystemKeyring("af-test")
	err := k.Set(ctx, "github_token", "ghp_secret")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	fake.getErr = failOn(indexAccount, errFakeIndexRead)
	err = k.Delete(ctx, "github_token")
	if !errors.Is(err, errFakeIndexRead) {
		t.Fatalf("Delete() error = %v, want wrapped index read error", err)
	}
}

func TestSystemKeyring_Delete_IndexWriteError(t *testing.T) {
	fake := installFakeBackend(t)
	ctx := context.Background()
	k := NewSystemKeyring("af-test")
	err := k.Set(ctx, "github_token", "ghp_secret")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	fake.setErr = failOn(indexAccount, errFakeIndexWrite)
	err = k.Delete(ctx, "github_token")
	if !errors.Is(err, errFakeIndexWrite) {
		t.Fatalf("Delete() error = %v, want wrapped index write error", err)
	}
}

func TestSystemKeyring_List_EmptyIndex(t *testing.T) {
	installFakeBackend(t)
	k := NewSystemKeyring("af-test")
	keys, err := k.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("List() = %#v, want empty", keys)
	}
}

func TestSystemKeyring_List_IndexReadError(t *testing.T) {
	fake := installFakeBackend(t)
	fake.getErr = failOn(indexAccount, errFakeIndexRead)

	k := NewSystemKeyring("af-test")
	_, err := k.List(context.Background())
	if !errors.Is(err, errFakeIndexRead) {
		t.Fatalf("List() error = %v, want wrapped index read error", err)
	}
}

func TestSplitIndex(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty string", raw: "", want: nil},
		{name: "single key", raw: "a", want: []string{"a"}},
		{name: "two keys", raw: "a\nb", want: []string{"a", "b"}},
		{name: "trailing newline", raw: "a\nb\n", want: []string{"a", "b"}},
		{name: "blank segments skipped", raw: "\n\na\n\nb", want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitIndex(tt.raw)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("splitIndex(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestJoinIndex(t *testing.T) {
	tests := []struct {
		name string
		want string
		keys []string
	}{
		{name: "nil keys", keys: nil, want: ""},
		{name: "single key", keys: []string{"a"}, want: "a"},
		{name: "multiple keys", keys: []string{"a", "b", "c"}, want: "a\nb\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinIndex(tt.keys); got != tt.want {
				t.Fatalf("joinIndex(%#v) = %q, want %q", tt.keys, got, tt.want)
			}
		})
	}
}

func TestSystemKeyring_ListReflectsAllSetKeys(t *testing.T) {
	installFakeBackend(t)
	ctx := context.Background()
	k := NewSystemKeyring("af-test")
	keys := []string{"alpha", "beta", "gamma", "delta"}
	for _, key := range keys {
		err := k.Set(ctx, key, "v")
		if err != nil {
			t.Fatalf("Set(%s) error = %v", key, err)
		}
	}
	got, err := k.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if strings.Join(got, ",") != "alpha,beta,delta,gamma" {
		t.Fatalf("List() = %#v, want all keys sorted", got)
	}
}
