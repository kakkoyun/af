package lifecycle_test

import (
	"testing"
	"testing/quick"

	"github.com/kakkoyun/af/internal/lifecycle"
)

func TestPropertyTerminalStatesRejectEveryEvent(t *testing.T) {
	property := func(rawEvent uint8) bool {
		event := lifecycle.EventFromIndex(rawEvent)

		_, completedOK := lifecycle.Apply(lifecycle.Completed, event)
		_, abandonedOK := lifecycle.Apply(lifecycle.Abandoned, event)

		return !completedOK && !abandonedOK
	}

	err := quick.Check(property, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPropertyTerminalStatesNeverBecomeActive(t *testing.T) {
	property := func(rawEvents []uint8) bool {
		state := lifecycle.Active
		terminal := false

		for _, rawEvent := range rawEvents {
			next, ok := lifecycle.Apply(state, lifecycle.EventFromIndex(rawEvent))
			if !ok {
				continue
			}
			state = next
			if lifecycle.IsTerminal(state) {
				terminal = true
			}
			if terminal && state == lifecycle.Active {
				return false
			}
		}

		return true
	}

	err := quick.Check(property, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPropertySuspendResumeRoundTrip(t *testing.T) {
	property := func(_ bool) bool {
		suspended, ok := lifecycle.Apply(lifecycle.Active, lifecycle.Suspend)
		if !ok || suspended != lifecycle.Suspended {
			return false
		}

		resumed, ok := lifecycle.Apply(suspended, lifecycle.Resume)
		return ok && resumed == lifecycle.Active
	}

	err := quick.Check(property, nil)
	if err != nil {
		t.Fatal(err)
	}
}
