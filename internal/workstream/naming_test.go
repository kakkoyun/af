package workstream_test

import (
	"errors"
	"strings"
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

func TestValidateSessionName_RejectsTraversalAndBadRefs(t *testing.T) {
	tests := []struct {
		name    string
		session string
	}{
		{name: "parent traversal", session: "../evil"},
		{name: "embedded traversal", session: "a/../../b"},
		{name: "dot dot element", session: ".."},
		{name: "single dot element", session: "."},
		{name: "dot element inside", session: "a/./b"},
		{name: "absolute path", session: "/abs"},
		{name: "backslash", session: `a\b`},
		{name: "nul byte", session: "a\x00b"},
		{name: "control char", session: "a\x01b"},
		{name: "leading dash flag injection", session: "-rf"},
		{name: "leading dash in element", session: "a/-rf"},
		{name: "trailing slash", session: "a/"},
		{name: "empty element", session: "a//b"},
		{name: "double dot sequence", session: "a..b"},
		{name: "tilde", session: "a~b"},
		{name: "caret", session: "a^b"},
		{name: "colon", session: "a:b"},
		{name: "question mark", session: "a?b"},
		{name: "asterisk", session: "a*b"},
		{name: "open bracket", session: "a[b"},
		{name: "at brace", session: "a@{b"},
		{name: "trailing lock", session: "foo.lock"},
		{name: "whitespace", session: "a b"},
		{name: "trailing dot", session: "foo."},
		{name: "too long", session: strings.Repeat("a", 201)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := workstream.ValidateSessionName(tt.session)
			if !errors.Is(err, workstream.ErrInvalidSessionName) {
				t.Fatalf("ValidateSessionName(%q) = %v, want ErrInvalidSessionName", tt.session, err)
			}
		})
	}
}

func TestValidateSessionName_AcceptsSlugStyleAndAutoNames(t *testing.T) {
	tests := []struct {
		name    string
		session string
	}{
		{name: "empty is auto-generated", session: ""},
		{name: "simple slug", session: "feature-x"},
		{name: "nested slug", session: "kakkoyun/issue-42"},
		{name: "auto name with host slug", session: "github.com/kakkoyun/af-20260703-120000"},
		{name: "underscores and digits", session: "fix_thing2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := workstream.ValidateSessionName(tt.session); err != nil {
				t.Fatalf("ValidateSessionName(%q) = %v, want nil", tt.session, err)
			}
		})
	}
}
