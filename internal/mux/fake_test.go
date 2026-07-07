package mux_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/kakkoyun/af/internal/mux"
)

func TestFakeMultiplexer_IsAvailable(t *testing.T) {
	fake := mux.NewFakeMultiplexer()
	if !fake.IsAvailable(context.Background()) {
		t.Fatal("IsAvailable() = false, want true")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if fake.IsAvailable(ctx) {
		t.Fatal("IsAvailable() = true, want false with canceled context")
	}
}

func TestFakeMultiplexer_InsideSession(t *testing.T) {
	fake := mux.NewFakeMultiplexer()

	pane, inside, err := fake.InsideSession(context.Background())
	requireNoError(t, err)
	if inside {
		t.Fatal("InsideSession() inside = true, want false")
	}
	if pane != "" {
		t.Fatalf("InsideSession() pane = %q, want empty", pane)
	}
}

func TestFakeMultiplexer_AttachMarksSessionAttached(t *testing.T) {
	ctx := context.Background()
	fake := mux.NewFakeMultiplexer()
	requireNoError(t, fake.CreateSession(ctx, "beta", "/repo"))
	requireNoError(t, fake.CreateSession(ctx, "alpha", "/repo"))

	requireNoError(t, fake.Attach(ctx, "beta"))

	sessions, err := fake.ListSessions(ctx)
	requireNoError(t, err)
	want := []mux.Session{
		{Name: "alpha", Attached: false},
		{Name: "beta", Attached: true},
	}
	if !reflect.DeepEqual(sessions, want) {
		t.Fatalf("ListSessions() = %#v, want %#v", sessions, want)
	}
}

func TestFakeMultiplexer_SendKeysAndSetOption(t *testing.T) {
	ctx := context.Background()
	fake := mux.NewFakeMultiplexer()
	requireNoError(t, fake.CreateSession(ctx, "work", "/repo"))

	requireNoError(t, fake.SendKeys(ctx, "work", "%0", "claude"))
	requireNoError(t, fake.SetOption(ctx, "work", "@AF_WORKSTREAM", "issue-42"))
}

func TestFakeMultiplexer_GetEnvMissingKey(t *testing.T) {
	ctx := context.Background()
	fake := mux.NewFakeMultiplexer()
	requireNoError(t, fake.CreateSession(ctx, "work", "/repo"))

	_, err := fake.GetEnv(ctx, "work", "AF_MISSING")
	requireErrorIs(t, err, mux.ErrSessionNotFound)
}

func TestFakeMultiplexer_KillPaneMissingPane(t *testing.T) {
	ctx := context.Background()
	fake := mux.NewFakeMultiplexer()
	requireNoError(t, fake.CreateSession(ctx, "work", "/repo"))

	err := fake.KillPane(ctx, "work", "%99")
	requireErrorIs(t, err, mux.ErrPaneNotFound)
}

func TestFakeMultiplexer_MissingSessionErrors(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, fake *mux.FakeMultiplexer) error
	}{
		{"attach", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.Attach(ctx, "gone") }},
		{"send keys", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.SendKeys(ctx, "gone", "", "k") }},
		{"set env", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.SetEnv(ctx, "gone", "K", "v") }},
		{"get env", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.GetEnv(ctx, "gone", "K")
			return err
		}},
		{"set option", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			return fake.SetOption(ctx, "gone", "@k", "v")
		}},
		{"split vertical", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.SplitVertical(ctx, "gone", "/repo")
			return err
		}},
		{"kill pane", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.KillPane(ctx, "gone", "%0") }},
		{"list panes", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.ListPanes(ctx, "gone")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := mux.NewFakeMultiplexer()
			err := test.call(context.Background(), fake)
			requireErrorIs(t, err, mux.ErrSessionNotFound)
		})
	}
}

func TestFakeMultiplexer_CanceledContextErrors(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, fake *mux.FakeMultiplexer) error
	}{
		{"inside session", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, _, err := fake.InsideSession(ctx)
			return err
		}},
		{"create session", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			return fake.CreateSession(ctx, "s", "/repo")
		}},
		{"kill session", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.KillSession(ctx, "s") }},
		{"session exists", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.SessionExists(ctx, "s")
			return err
		}},
		{"attach", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.Attach(ctx, "s") }},
		{"send keys", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.SendKeys(ctx, "s", "", "k") }},
		{"set env", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.SetEnv(ctx, "s", "K", "v") }},
		{"get env", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.GetEnv(ctx, "s", "K")
			return err
		}},
		{"set option", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.SetOption(ctx, "s", "@k", "v") }},
		{"list sessions", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.ListSessions(ctx)
			return err
		}},
		{"split vertical", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.SplitVertical(ctx, "s", "/repo")
			return err
		}},
		{"kill pane", func(ctx context.Context, fake *mux.FakeMultiplexer) error { return fake.KillPane(ctx, "s", "%0") }},
		{"list panes", func(ctx context.Context, fake *mux.FakeMultiplexer) error {
			_, err := fake.ListPanes(ctx, "s")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := mux.NewFakeMultiplexer()
			requireNoError(t, fake.CreateSession(context.Background(), "s", "/repo"))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := test.call(ctx, fake)
			requireErrorIs(t, err, context.Canceled)
		})
	}
}
