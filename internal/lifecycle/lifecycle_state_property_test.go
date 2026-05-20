package lifecycle_test

import (
	"testing"
	"testing/quick"

	"github.com/kakkoyun/af/internal/lifecycle"
)

// nonTerminalState maps a uint8 index to one of the two non-terminal
// states so property tests can exercise both Active and Suspended with
// generated inputs.
func nonTerminalState(idx uint8) lifecycle.State {
	states := [2]lifecycle.State{lifecycle.Active, lifecycle.Suspended}
	return states[idx%2]
}

// TestProperty_TerminalStatesRejectAllEvents verifies that for every
// terminal state (Completed, Abandoned) and every possible event, Apply
// returns (state, false) — i.e. no transition is accepted.
func TestProperty_TerminalStatesRejectAllEvents(t *testing.T) {
	property := func(rawEvent uint8) bool {
		event := lifecycle.EventFromIndex(rawEvent)
		_, completedOK := lifecycle.Apply(lifecycle.Completed, event)
		_, abandonedOK := lifecycle.Apply(lifecycle.Abandoned, event)
		return !completedOK && !abandonedOK
	}
	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProperty_ActiveSuspendThenResumeReturnsActive verifies that
// Active → Suspend → Resume round-trips back to Active.
func TestProperty_ActiveSuspendThenResumeReturnsActive(t *testing.T) {
	property := func(_ bool) bool {
		suspended, ok := lifecycle.Apply(lifecycle.Active, lifecycle.Suspend)
		if !ok {
			return false
		}
		resumed, ok := lifecycle.Apply(suspended, lifecycle.Resume)
		return ok && resumed == lifecycle.Active
	}
	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProperty_SuspendIsIdempotentOnSuspended verifies that applying
// Suspend to an already-Suspended workstream returns (Suspended, false)
// — the transition is not valid from that state.
func TestProperty_SuspendIsIdempotentOnSuspended(t *testing.T) {
	property := func(_ bool) bool {
		next, ok := lifecycle.Apply(lifecycle.Suspended, lifecycle.Suspend)
		return !ok && next == lifecycle.Suspended
	}
	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProperty_ResumeIsIdempotentOnActive verifies that applying Resume
// to an already-Active workstream returns (Active, false) — the
// transition is not valid from that state.
func TestProperty_ResumeIsIdempotentOnActive(t *testing.T) {
	property := func(_ bool) bool {
		next, ok := lifecycle.Apply(lifecycle.Active, lifecycle.Resume)
		return !ok && next == lifecycle.Active
	}
	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProperty_DoneFromAnyNonTerminalIsTerminal verifies that applying
// Done from any non-terminal state (Active or Suspended) yields a state
// where IsTerminal returns true.
func TestProperty_DoneFromAnyNonTerminalIsTerminal(t *testing.T) {
	property := func(stateIdx uint8) bool {
		state := nonTerminalState(stateIdx)
		next, ok := lifecycle.Apply(state, lifecycle.Done)
		return ok && lifecycle.IsTerminal(next)
	}
	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProperty_DoneForceFromAnyNonTerminalIsAbandoned verifies that
// applying DoneForce from any non-terminal state yields Abandoned.
func TestProperty_DoneForceFromAnyNonTerminalIsAbandoned(t *testing.T) {
	property := func(stateIdx uint8) bool {
		state := nonTerminalState(stateIdx)
		next, ok := lifecycle.Apply(state, lifecycle.DoneForce)
		return ok && next == lifecycle.Abandoned
	}
	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatal(err)
	}
}

// TestProperty_EventFromIndexIsTotal verifies that EventFromIndex is a
// total function: every possible uint8 input maps to one of the four
// known Event constants.
func TestProperty_EventFromIndexIsTotal(t *testing.T) {
	knownEvents := [4]lifecycle.Event{
		lifecycle.Suspend,
		lifecycle.Resume,
		lifecycle.Done,
		lifecycle.DoneForce,
	}
	property := func(idx uint8) bool {
		event := lifecycle.EventFromIndex(idx)
		for _, known := range knownEvents {
			if event == known {
				return true
			}
		}
		return false
	}
	err := quick.Check(property, &quick.Config{MaxCount: 100})
	if err != nil {
		t.Fatal(err)
	}
}
