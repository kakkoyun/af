package doctor_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/doctor"
)

var errFakeRemote = errors.New("ssh failed")

type fakeLookup struct {
	paths    map[string]string
	versions map[string]string
}

func (f fakeLookup) LookPath(_ context.Context, name string) (string, bool) {
	path, ok := f.paths[name]
	return path, ok
}

func (f fakeLookup) Version(_ context.Context, binary string) string {
	return f.versions[binary]
}

func TestRun_FlagsMissingMustTools(t *testing.T) {
	lookup := fakeLookup{
		paths: map[string]string{
			"tmux": "/opt/homebrew/bin/tmux",
			"pi":   "/opt/homebrew/bin/pi",
		},
		versions: map[string]string{
			"tmux": "3.4",
			"pi":   "0.73.0",
		},
	}

	report := doctor.Run(context.Background(), lookup, doctor.PlatformMacOS, doctor.DefaultProbes(nil))

	if !report.HasMissingMustTools() {
		t.Fatalf("HasMissingMustTools = false, want true (git missing)")
	}
	if got, want := report.MissingMustTools, []string{"git"}; !equalStrings(got, want) {
		t.Fatalf("MissingMustTools = %v, want %v", got, want)
	}
}

func TestRun_SatisfiesAgentGroupWhenOneAgentPresent(t *testing.T) {
	lookup := fakeLookup{
		paths: map[string]string{
			"git":    "/usr/bin/git",
			"tmux":   "/usr/bin/tmux",
			"claude": "/usr/local/bin/claude",
		},
	}

	report := doctor.Run(context.Background(), lookup, doctor.PlatformMacOS, doctor.DefaultProbes(nil))

	if report.HasMissingMustTools() {
		t.Fatalf("MissingMustTools = %v, want empty when one agent is present", report.MissingMustTools)
	}
}

func TestRun_FailsWhenNoAgentInGroupPresent(t *testing.T) {
	lookup := fakeLookup{
		paths: map[string]string{
			"git":  "/usr/bin/git",
			"tmux": "/usr/bin/tmux",
		},
	}

	report := doctor.Run(context.Background(), lookup, doctor.PlatformMacOS, doctor.DefaultProbes(nil))

	if !report.HasMissingMustTools() {
		t.Fatalf("HasMissingMustTools = false, want true")
	}
	for _, want := range []string{"pi", "claude", "codex"} {
		if !contains(report.MissingMustTools, want) {
			t.Fatalf("MissingMustTools = %v, want %s included", report.MissingMustTools, want)
		}
	}
}

func TestRun_IncludesExtraToolsAtNiceTier(t *testing.T) {
	lookup := fakeLookup{paths: map[string]string{"git": "/g", "tmux": "/t", "pi": "/p"}}

	report := doctor.Run(context.Background(), lookup, doctor.PlatformMacOS, doctor.DefaultProbes([]string{"jq", "fzy"}))

	for _, want := range []string{"jq", "fzy"} {
		var found bool
		for _, r := range report.Results {
			if r.Probe.Name == want {
				found = true
				if r.Probe.Tier != doctor.TierNice {
					t.Fatalf("extra tool %q tier = %v, want TierNice", want, r.Probe.Tier)
				}
			}
		}
		if !found {
			t.Fatalf("extra tool %q missing from report", want)
		}
	}
}

func TestRender_EmitsHintsForMissingTools(t *testing.T) {
	lookup := fakeLookup{paths: map[string]string{"tmux": "/t", "pi": "/p"}}
	report := doctor.Run(context.Background(), lookup, doctor.PlatformDebian, doctor.DefaultProbes(nil))

	var buf bytes.Buffer
	err := doctor.Render(&buf, report, "Local environment:")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"Local environment:",
		"✗ git",
		"apt install git",
		"Status: 1 missing required tool(s): git.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Render missing %q; output:\n%s", want, out)
		}
	}
}

func TestRender_DowngradesUnsatisfiedAgentToWarning(t *testing.T) {
	lookup := fakeLookup{paths: map[string]string{
		"git": "/g", "tmux": "/t", "claude": "/c",
	}}
	report := doctor.Run(context.Background(), lookup, doctor.PlatformMacOS, doctor.DefaultProbes(nil))

	var buf bytes.Buffer
	err := doctor.Render(&buf, report, "Local environment:")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "⚠ pi") {
		t.Fatalf("pi should be ⚠ when claude satisfies agent group; output:\n%s", out)
	}
	if !strings.Contains(out, "✓ claude") {
		t.Fatalf("claude should be ✓; output:\n%s", out)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, x := range haystack {
		if x == needle {
			return true
		}
	}
	return false
}
