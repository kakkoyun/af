package lifecycle

import (
	"testing"

	"github.com/kakkoyun/af/internal/agent"
)

// TestApprovalModeToString_RoundTrip pins the typed-to-string mapping,
// including the defensive fallback for out-of-range values.
func TestApprovalModeToString_RoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mode agent.ApprovalMode
		want string
	}{
		{"default", agent.ApprovalDefault, ""},
		{"auto", agent.ApprovalAuto, "auto"},
		{"yolo", agent.ApprovalYolo, "yolo"},
		{"out of range", agent.ApprovalMode(99), ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := approvalModeToString(tt.mode); got != tt.want {
				t.Fatalf("approvalModeToString(%v) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// TestParseApprovalMode_UnknownFallsBackToDefault pins the parser fallback.
func TestParseApprovalMode_UnknownFallsBackToDefault(t *testing.T) {
	t.Parallel()
	if got := parseApprovalMode("nonsense"); got != agent.ApprovalDefault {
		t.Fatalf("parseApprovalMode(nonsense) = %v, want ApprovalDefault", got)
	}
}
