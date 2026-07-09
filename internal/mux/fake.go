package mux

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// FakeMultiplexer is an in-memory Multiplexer for tests.
type FakeMultiplexer struct {
	sessions map[string]*fakeSession
	counter  int
	mu       sync.RWMutex
}

type fakeSession struct {
	env      map[string]string
	options  map[string]string
	panes    []Pane
	sentKeys []string
	attached bool
}

// NewFakeMultiplexer returns an empty fake multiplexer.
func NewFakeMultiplexer() *FakeMultiplexer {
	return &FakeMultiplexer{sessions: map[string]*fakeSession{}}
}

// IsAvailable always reports true for the fake multiplexer.
func (*FakeMultiplexer) IsAvailable(ctx context.Context) bool {
	return ctx.Err() == nil
}

// InsideSession reports that the fake is outside a multiplexer session.
func (*FakeMultiplexer) InsideSession(ctx context.Context) (string, bool, error) {
	err := ctx.Err()
	if err != nil {
		return "", false, fmt.Errorf("check fake mux session: %w", err)
	}

	return "", false, nil
}

// CreateSession creates a fake session with a primary pane.
func (fake *FakeMultiplexer) CreateSession(ctx context.Context, name, cwd string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("create fake mux session %s: %w", name, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.sessions[name] = &fakeSession{
		env:     map[string]string{},
		options: map[string]string{"@AF_SESSION": "1"},
		panes:   []Pane{{ID: "%0", CWD: cwd}},
	}

	return nil
}

// KillSession removes a fake session.
func (fake *FakeMultiplexer) KillSession(ctx context.Context, name string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("kill fake mux session %s: %w", name, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	delete(fake.sessions, name)

	return nil
}

// SessionExists reports whether a fake session exists.
func (fake *FakeMultiplexer) SessionExists(ctx context.Context, name string) (bool, error) {
	err := ctx.Err()
	if err != nil {
		return false, fmt.Errorf("check fake mux session %s: %w", name, err)
	}
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	_, ok := fake.sessions[name]

	return ok, nil
}

// Attach marks a fake session attached.
func (fake *FakeMultiplexer) Attach(ctx context.Context, name string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("attach fake mux session %s: %w", name, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	session, err := fake.sessionLocked(name)
	if err != nil {
		return err
	}
	session.attached = true

	return nil
}

// SendKeys accepts keys for a fake session, recording them so tests can
// inspect what was sent via SentKeys (issue #33 Fix 3).
func (fake *FakeMultiplexer) SendKeys(ctx context.Context, session, _, keys string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("send fake mux keys to %s: %w", session, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	entry, err := fake.sessionLocked(session)
	if err != nil {
		return err
	}
	entry.sentKeys = append(entry.sentKeys, keys)

	return nil
}

// SentKeys returns the keys recorded by SendKeys for session, in call
// order. It returns nil for a session that never received SendKeys (or
// never existed), so callers don't need a separate existence check.
func (fake *FakeMultiplexer) SentKeys(session string) []string {
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	entry, ok := fake.sessions[session]
	if !ok || len(entry.sentKeys) == 0 {
		return nil
	}
	keys := make([]string, len(entry.sentKeys))
	copy(keys, entry.sentKeys)

	return keys
}

// SetEnv sets a fake session environment variable.
func (fake *FakeMultiplexer) SetEnv(ctx context.Context, session, key, value string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("set fake mux env %s: %w", key, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	entry, err := fake.sessionLocked(session)
	if err != nil {
		return err
	}
	entry.env[key] = value

	return nil
}

// GetEnv returns a fake session environment variable.
func (fake *FakeMultiplexer) GetEnv(ctx context.Context, session, key string) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", fmt.Errorf("get fake mux env %s: %w", key, err)
	}
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	entry, err := fake.sessionLocked(session)
	if err != nil {
		return "", err
	}
	value, ok := entry.env[key]
	if !ok {
		return "", fmt.Errorf("get fake mux env %s: %w", key, ErrSessionNotFound)
	}

	return value, nil
}

// SetOption sets a fake session option.
func (fake *FakeMultiplexer) SetOption(ctx context.Context, session, key, value string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("set fake mux option %s: %w", key, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	entry, err := fake.sessionLocked(session)
	if err != nil {
		return err
	}
	entry.options[key] = value

	return nil
}

// ListSessions returns fake sessions sorted by name.
func (fake *FakeMultiplexer) ListSessions(ctx context.Context) ([]Session, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("list fake mux sessions: %w", err)
	}
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	names := make([]string, 0, len(fake.sessions))
	for name := range fake.sessions {
		names = append(names, name)
	}
	sort.Strings(names)
	sessions := make([]Session, 0, len(names))
	for _, name := range names {
		sessions = append(sessions, Session{Name: name, Attached: fake.sessions[name].attached})
	}

	return sessions, nil
}

// SplitVertical adds a fake pane and returns its id.
func (fake *FakeMultiplexer) SplitVertical(ctx context.Context, session, cwd string) (string, error) {
	err := ctx.Err()
	if err != nil {
		return "", fmt.Errorf("split fake mux session %s: %w", session, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	entry, err := fake.sessionLocked(session)
	if err != nil {
		return "", err
	}
	fake.counter++
	pane := Pane{ID: fmt.Sprintf("%%%d", fake.counter), CWD: cwd}
	entry.panes = append(entry.panes, pane)

	return pane.ID, nil
}

// KillPane removes a fake pane.
func (fake *FakeMultiplexer) KillPane(ctx context.Context, session, pane string) error {
	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("kill fake mux pane %s: %w", pane, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	entry, err := fake.sessionLocked(session)
	if err != nil {
		return err
	}
	for index, candidate := range entry.panes {
		if candidate.ID == pane {
			entry.panes = append(entry.panes[:index], entry.panes[index+1:]...)
			return nil
		}
	}

	return fmt.Errorf("kill fake mux pane %s: %w", pane, ErrPaneNotFound)
}

// ListPanes returns fake panes for a session.
func (fake *FakeMultiplexer) ListPanes(ctx context.Context, session string) ([]Pane, error) {
	err := ctx.Err()
	if err != nil {
		return nil, fmt.Errorf("list fake mux panes for %s: %w", session, err)
	}
	fake.mu.RLock()
	defer fake.mu.RUnlock()
	entry, err := fake.sessionLocked(session)
	if err != nil {
		return nil, err
	}
	panes := make([]Pane, 0, len(entry.panes))
	panes = append(panes, entry.panes...)

	return panes, nil
}

func (fake *FakeMultiplexer) sessionLocked(name string) (*fakeSession, error) {
	session, ok := fake.sessions[name]
	if !ok {
		return nil, fmt.Errorf("find fake mux session %s: %w", name, ErrSessionNotFound)
	}

	return session, nil
}
