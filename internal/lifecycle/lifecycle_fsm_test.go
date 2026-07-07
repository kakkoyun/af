package lifecycle_test

import (
	"testing"

	"github.com/kakkoyun/af/internal/lifecycle"
)

// TestApply_TransitionTable pins the full lifecycle state machine,
// including rejected transitions, unknown events, and unknown states.
func TestApply_TransitionTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		state lifecycle.State
		event lifecycle.Event
		want  lifecycle.State
		ok    bool
	}{
		{"active suspend", lifecycle.Active, lifecycle.Suspend, lifecycle.Suspended, true},
		{"active done", lifecycle.Active, lifecycle.Done, lifecycle.Completed, true},
		{"active done_force", lifecycle.Active, lifecycle.DoneForce, lifecycle.Abandoned, true},
		{"active resume rejected", lifecycle.Active, lifecycle.Resume, lifecycle.Active, false},
		{"active unknown event rejected", lifecycle.Active, lifecycle.Event("bogus"), lifecycle.Active, false},
		{"suspended resume", lifecycle.Suspended, lifecycle.Resume, lifecycle.Active, true},
		{"suspended done", lifecycle.Suspended, lifecycle.Done, lifecycle.Completed, true},
		{"suspended done_force", lifecycle.Suspended, lifecycle.DoneForce, lifecycle.Abandoned, true},
		{"suspended suspend rejected", lifecycle.Suspended, lifecycle.Suspend, lifecycle.Suspended, false},
		{"suspended unknown event rejected", lifecycle.Suspended, lifecycle.Event("bogus"), lifecycle.Suspended, false},
		{"completed rejects everything", lifecycle.Completed, lifecycle.Resume, lifecycle.Completed, false},
		{"abandoned rejects everything", lifecycle.Abandoned, lifecycle.Suspend, lifecycle.Abandoned, false},
		{"unknown state rejected", lifecycle.State("weird"), lifecycle.Done, lifecycle.State("weird"), false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := lifecycle.Apply(tt.state, tt.event)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Apply(%q, %q) = (%q, %v), want (%q, %v)",
					tt.state, tt.event, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestIsTerminal_Table pins terminality per state, including unknown states.
func TestIsTerminal_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state lifecycle.State
		want  bool
	}{
		{lifecycle.Active, false},
		{lifecycle.Suspended, false},
		{lifecycle.Completed, true},
		{lifecycle.Abandoned, true},
		{lifecycle.State("weird"), false},
	}
	for _, tt := range cases {
		if got := lifecycle.IsTerminal(tt.state); got != tt.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

// TestEventFromIndex_Mapping pins the exact index-to-event mapping and
// its modulo wrap-around.
func TestEventFromIndex_Mapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		want  lifecycle.Event
		index uint8
	}{
		{lifecycle.Suspend, 0},
		{lifecycle.Resume, 1},
		{lifecycle.Done, 2},
		{lifecycle.DoneForce, 3},
		{lifecycle.Suspend, 4},
		{lifecycle.DoneForce, 7},
		{lifecycle.DoneForce, 255},
	}
	for _, tt := range cases {
		if got := lifecycle.EventFromIndex(tt.index); got != tt.want {
			t.Errorf("EventFromIndex(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}
