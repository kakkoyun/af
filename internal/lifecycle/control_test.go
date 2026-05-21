package lifecycle_test

import (
	"testing"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/lifecycle"
)

const (
	testAgentClaude = "claude"
	testAgentCodex  = "codex"
	testAgentPi     = "pi"
)

// TestResolveControl_CLIFlagWins verifies that CLI flags are the highest
// precedence rung per ADR-061.
func TestResolveControl_CLIFlagWins(t *testing.T) {
	cfg := config.Defaults()
	cfg.Control.Agent = testAgentPi
	cfg.Control.ApprovalMode = "auto"
	cfg.General.DefaultAgent = testAgentCodex

	flags := lifecycle.ControlFlags{
		Agent:        testAgentClaude,
		ApprovalMode: "yolo",
	}

	ctx := lifecycle.ResolveControl(flags, cfg)

	if ctx.Agent != testAgentClaude {
		t.Errorf("Agent = %q, want claude (CLI flag wins)", ctx.Agent)
	}
	if ctx.ApprovalMode != agent.ApprovalYolo {
		t.Errorf("ApprovalMode = %v, want ApprovalYolo (CLI flag wins)", ctx.ApprovalMode)
	}
}

// TestResolveControl_RepoOverridesUser verifies that repo config [control]
// values (already merged into cfg.Control by config.Load) win over user
// config defaults when no CLI flag is set.
func TestResolveControl_RepoOverridesUser(t *testing.T) {
	cfg := config.Defaults()
	// Simulate repo config overriding user config (both merged into cfg.Control).
	cfg.Control.Agent = testAgentClaude
	cfg.Control.MaxAgents = 4
	cfg.General.DefaultAgent = testAgentPi // subsystem default — should lose

	ctx := lifecycle.ResolveControl(lifecycle.ControlFlags{}, cfg)

	if ctx.Agent != testAgentClaude {
		t.Errorf("Agent = %q, want claude (repo control wins over subsystem default)", ctx.Agent)
	}
	if ctx.MaxAgents != 4 {
		t.Errorf("MaxAgents = %d, want 4 (repo control wins)", ctx.MaxAgents)
	}
}

// TestResolveControl_FallbackToSubsystemDefault verifies that when control
// fields are empty, subsystem defaults are used per ADR-061 rung 4.
func TestResolveControl_FallbackToSubsystemDefault(t *testing.T) {
	cfg := config.Defaults()
	cfg.General.DefaultAgent = testAgentCodex
	cfg.Remote.DefaultHost = "hetzner-af"
	cfg.Sandbox.DefaultProvider = "slicer"
	// cfg.Control is zero (empty).

	ctx := lifecycle.ResolveControl(lifecycle.ControlFlags{}, cfg)

	if ctx.Agent != testAgentCodex {
		t.Errorf("Agent = %q, want codex (subsystem default fallback)", ctx.Agent)
	}
	if ctx.Remote != "hetzner-af" {
		t.Errorf("Remote = %q, want hetzner-af (subsystem default fallback)", ctx.Remote)
	}
	if ctx.Sandbox != "slicer" {
		t.Errorf("Sandbox = %q, want slicer (subsystem default fallback)", ctx.Sandbox)
	}
}

// TestResolveControl_MaxAgentsRespected verifies that MaxAgents is resolved
// correctly from all three priority rungs.
func TestResolveControl_MaxAgentsRespected(t *testing.T) {
	tests := []struct {
		name    string
		flagMax int
		repoMax int
		wantMax int
	}{
		{"cli_flag_wins", 5, 3, 5},
		{"repo_when_no_flag", 0, 3, 3},
		{"zero_when_none", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.Control.MaxAgents = tt.repoMax

			ctx := lifecycle.ResolveControl(lifecycle.ControlFlags{MaxAgents: tt.flagMax}, cfg)
			if ctx.MaxAgents != tt.wantMax {
				t.Errorf("MaxAgents = %d, want %d", ctx.MaxAgents, tt.wantMax)
			}
		})
	}
}

// TestResolveControl_ApprovalModeParsed verifies all three approval mode
// string values round-trip through ResolveControl correctly.
func TestResolveControl_ApprovalModeParsed(t *testing.T) {
	cases := []struct {
		input string
		want  agent.ApprovalMode
	}{
		{"", agent.ApprovalDefault},
		{"auto", agent.ApprovalAuto},
		{"yolo", agent.ApprovalYolo},
	}
	for _, tt := range cases {
		t.Run("mode="+tt.input, func(t *testing.T) {
			cfg := config.Defaults()
			got := lifecycle.ResolveControl(lifecycle.ControlFlags{ApprovalMode: tt.input}, cfg)
			if got.ApprovalMode != tt.want {
				t.Errorf("ApprovalMode = %v, want %v", got.ApprovalMode, tt.want)
			}
		})
	}
}
