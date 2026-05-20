package sandbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Fake is an in-memory Sandbox implementation for tests.
type Fake struct {
	handles map[string]Handle
	name    string
	counter int
	mu      sync.RWMutex
}

// NewFake returns an empty fake sandbox provider named name.
func NewFake(name string) *Fake {
	return &Fake{name: name, handles: map[string]Handle{}}
}

// Name returns the fake provider name.
func (fake *Fake) Name() string {
	return fake.name
}

// IsAvailable always reports true for the fake sandbox.
func (*Fake) IsAvailable(ctx context.Context) bool {
	return ctx.Err() == nil
}

// Launch records a fake sandbox and returns its handle.
func (fake *Fake) Launch(ctx context.Context, opts LaunchOpts) (*Handle, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("launch fake sandbox %s: %w", opts.Workstream, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.counter++
	handle := Handle{ID: fmt.Sprintf("%s-%d", opts.Workstream, fake.counter), AttachCmd: []string{fake.name, "attach", opts.Workstream}}
	fake.handles[handle.ID] = handle

	return &handle, nil
}

// Attach checks that a fake handle exists.
func (fake *Fake) Attach(ctx context.Context, handle *Handle) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("attach fake sandbox %s: %w", handle.ID, err)
	}
	_, err = fake.lookup(handle.ID)
	if err != nil {
		return err
	}

	return nil
}

// IsHealthy reports whether the fake handle exists.
func (fake *Fake) IsHealthy(ctx context.Context, handle *Handle) (bool, error) {
	err := ctx.Err()
	if err != nil {
		return false, fmt.Errorf("check fake sandbox %s: %w", handle.ID, err)
	}
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	_, ok := fake.handles[handle.ID]

	return ok, nil
}

// Teardown removes a fake sandbox.
func (fake *Fake) Teardown(ctx context.Context, handle *Handle) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("teardown fake sandbox %s: %w", handle.ID, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	delete(fake.handles, handle.ID)

	return nil
}

// List returns fake handles sorted by id.
func (fake *Fake) List(ctx context.Context) ([]Handle, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("list fake sandboxes: %w", err)
	}
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	ids := make([]string, 0, len(fake.handles))
	for id := range fake.handles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	handles := make([]Handle, 0, len(ids))
	for _, id := range ids {
		handles = append(handles, fake.handles[id])
	}

	return handles, nil
}

func (fake *Fake) lookup(id string) (Handle, error) {
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	handle, ok := fake.handles[id]
	if !ok {
		return Handle{}, fmt.Errorf("find fake sandbox %s: %w", id, ErrNotFound)
	}

	return handle, nil
}

// ErrNotFound reports a missing sandbox handle.
var ErrNotFound = errors.New("sandbox handle not found")
