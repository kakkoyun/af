package lifecycle

// State is a durable workstream lifecycle state.
type State string

const (
	// Active is a running workstream that can be suspended or completed.
	Active State = "active"
	// Suspended is a parked workstream that can be resumed or completed.
	Suspended State = "suspended"
	// Completed is a terminal state for successfully finished workstreams.
	Completed State = "completed"
	// Abandoned is a terminal state for force-closed unfinished workstreams.
	Abandoned State = "abandoned"
)

// Event is a lifecycle event that may transition a workstream state.
type Event string

const (
	// Suspend parks an active workstream without ending it.
	Suspend Event = "suspend"
	// Resume restores a suspended workstream to active work.
	Resume Event = "resume"
	// Done completes an active or suspended workstream cleanly.
	Done Event = "done"
	// DoneForce abandons an active or suspended workstream.
	DoneForce Event = "done_force"
)

const (
	eventIndexSuspend = 0
	eventIndexResume  = 1
	eventIndexDone    = 2
	eventCount        = 4
)

// EventFromIndex maps arbitrary generated input onto a known lifecycle event.
func EventFromIndex(index uint8) Event {
	switch index % eventCount {
	case eventIndexSuspend:
		return Suspend
	case eventIndexResume:
		return Resume
	case eventIndexDone:
		return Done
	default:
		return DoneForce
	}
}

// Apply returns the next state for a valid transition.
func Apply(state State, event Event) (State, bool) {
	switch state {
	case Active:
		return applyActive(event)
	case Suspended:
		return applySuspended(event)
	case Completed, Abandoned:
		return state, false
	default:
		return state, false
	}
}

// IsTerminal reports whether state rejects all future lifecycle events.
func IsTerminal(state State) bool {
	return state == Completed || state == Abandoned
}

func applyActive(event Event) (State, bool) {
	switch event {
	case Suspend:
		return Suspended, true
	case Done:
		return Completed, true
	case DoneForce:
		return Abandoned, true
	case Resume:
		return Active, false
	default:
		return Active, false
	}
}

func applySuspended(event Event) (State, bool) {
	switch event {
	case Resume:
		return Active, true
	case Done:
		return Completed, true
	case DoneForce:
		return Abandoned, true
	case Suspend:
		return Suspended, false
	default:
		return Suspended, false
	}
}
