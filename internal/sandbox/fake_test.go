package sandbox_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

func TestFake_Name(t *testing.T) {
	fake := sandbox.NewFake("fake-provider")
	if got := fake.Name(); got != "fake-provider" {
		t.Fatalf("Name() = %q, want fake-provider", got)
	}
}

func TestFake_IsAvailable(t *testing.T) {
	fake := sandbox.NewFake("fake")
	if !fake.IsAvailable(context.Background()) {
		t.Fatal("IsAvailable() = false, want true")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if fake.IsAvailable(ctx) {
		t.Fatal("IsAvailable(cancelled) = true, want false")
	}
}

func TestFake_AttachExistingAndMissing(t *testing.T) {
	ctx := context.Background()
	fake := sandbox.NewFake("fake")

	handle, err := fake.Launch(ctx, sandbox.LaunchOpts{Workstream: "session"})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	err = fake.Attach(ctx, handle)
	if err != nil {
		t.Fatalf("Attach() error = %v", err)
	}

	err = fake.Attach(ctx, &sandbox.Handle{ID: "missing"})
	if !errors.Is(err, sandbox.ErrNotFound) {
		t.Fatalf("Attach(missing) error = %v, want ErrNotFound", err)
	}
}

func TestFake_CancelledContextRejectsOperations(t *testing.T) {
	fake := sandbox.NewFake("fake")
	handle, err := fake.Launch(context.Background(), sandbox.LaunchOpts{Workstream: "session"})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		run  func() error
		name string
	}{
		{name: "launch", run: func() error {
			_, launchErr := fake.Launch(ctx, sandbox.LaunchOpts{Workstream: "other"})
			if launchErr != nil {
				return fmt.Errorf("launch: %w", launchErr)
			}
			return nil
		}},
		{name: "attach", run: func() error {
			return fake.Attach(ctx, handle)
		}},
		{name: "health", run: func() error {
			_, healthErr := fake.IsHealthy(ctx, handle)
			if healthErr != nil {
				return fmt.Errorf("is healthy: %w", healthErr)
			}
			return nil
		}},
		{name: "teardown", run: func() error {
			return fake.Teardown(ctx, handle)
		}},
		{name: "list", run: func() error {
			_, listErr := fake.List(ctx)
			if listErr != nil {
				return fmt.Errorf("list: %w", listErr)
			}
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestRecordingRunner_CancelledContextRejected(t *testing.T) {
	runner := sandbox.NewRecordingRunner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner.Run(ctx, sandbox.Command{Name: "slicer"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run(cancelled) error = %v, want context.Canceled", err)
	}
	if got := runner.Commands(); len(got) != 0 {
		t.Fatalf("Commands() = %#v, want none recorded on cancelled context", got)
	}
}

func TestRecordingRunner_QueuedOutputsReturnedInOrder(t *testing.T) {
	ctx := context.Background()
	runner := sandbox.NewRecordingRunner()
	runner.QueueOutput("first")
	runner.QueueOutput("second")

	tests := []string{"first", "second", ""}
	for _, want := range tests {
		output, err := runner.Run(ctx, sandbox.Command{Name: "slicer"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if string(output) != want {
			t.Fatalf("Run() output = %q, want %q", output, want)
		}
	}
	if got := runner.Commands(); len(got) != 3 {
		t.Fatalf("Commands() recorded %d, want 3", len(got))
	}
}
