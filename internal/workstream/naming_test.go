package workstream_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kakkoyun/af/internal/workstream"
)

func TestSanitize_ReplacesTmuxSeparatorsWithDoubleDash(t *testing.T) {
	got := workstream.Sanitize("kakkoyun/issue.42:fix")
	want := "kakkoyun--issue--42--fix"
	if got != want {
		t.Fatalf("Sanitize() = %q, want %q", got, want)
	}
}

func TestBranchName_AppliesPrefixOnlyWhenRulesAllow(t *testing.T) {
	tests := []struct {
		name string
		want string
		opts workstream.BranchOptions
	}{
		{
			name: "no prefix",
			opts: workstream.BranchOptions{Name: "issue-42"},
			want: "issue-42",
		},
		{
			name: "fork only with upstream",
			opts: workstream.BranchOptions{Name: "issue-42", Prefix: "kakkoyun", PrefixOnForkOnly: true, HasUpstreamRemote: true},
			want: "kakkoyun/issue-42",
		},
		{
			name: "fork only without upstream",
			opts: workstream.BranchOptions{Name: "issue-42", Prefix: "kakkoyun", PrefixOnForkOnly: true, HasUpstreamRemote: false},
			want: "issue-42",
		},
		{
			name: "always prefix without upstream",
			opts: workstream.BranchOptions{Name: "issue-42", Prefix: "kakkoyun", PrefixOnForkOnly: false, HasUpstreamRemote: false},
			want: "kakkoyun/issue-42",
		},
		{
			name: "already prefixed",
			opts: workstream.BranchOptions{Name: "kakkoyun/issue-42", Prefix: "kakkoyun", PrefixOnForkOnly: true, HasUpstreamRemote: true},
			want: "kakkoyun/issue-42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workstream.BranchName(tt.opts)
			if got != tt.want {
				t.Fatalf("BranchName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoSessionName_UsesRepoAndWallClockTimestamp(t *testing.T) {
	at := time.Date(2026, time.May, 20, 13, 14, 15, 999, time.UTC)
	got := workstream.AutoSessionName("af", at)
	want := "af-20260520-131415"
	if got != want {
		t.Fatalf("AutoSessionName() = %q, want %q", got, want)
	}
}

func TestSubBranchName_AppendsSanitizedSlot(t *testing.T) {
	got := workstream.SubBranchName("kakkoyun/issue-42", "review.bot")
	want := "kakkoyun/issue-42--review--bot"
	if got != want {
		t.Fatalf("SubBranchName() = %q, want %q", got, want)
	}
}

func TestSessionID_DerivesUUID5FromStableInputs(t *testing.T) {
	launch := time.Unix(0, 123456789)
	got := workstream.SessionID("af", "kakkoyun/issue-42", "primary", launch)
	want := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("af/kakkoyun/issue-42/primary/123456789"))
	if got != want {
		t.Fatalf("SessionID() = %s, want %s", got, want)
	}

	again := workstream.SessionID("af", "kakkoyun/issue-42", "primary", launch)
	if got != again {
		t.Fatalf("SessionID() = %s on first call, %s on second", got, again)
	}
}
