// Package lifecycle provides workstream lifecycle orchestration.
package lifecycle

import (
	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/config"
)

// ControlFlags holds CLI-level override values for control settings.
// An empty string means "no CLI override"; zero int means "no CLI override".
// Use the typed sentinel values (not raw strings) where possible.
type ControlFlags struct {
	Agent         string
	ApprovalMode  string
	Sandbox       string
	Remote        string
	RemoteControl string
	MaxAgents     int // 0 means no CLI override
}

// ControlContext holds the resolved, effective control settings for one
// workstream launch. All fields are concrete values (no empties mean
// "use default" at this layer).
type ControlContext struct {
	// Agent is the resolved provider name ("pi", "claude", or "codex").
	Agent string
	// Sandbox is the resolved sandbox provider ("" = local, "slicer" = slicer).
	Sandbox string
	// Remote is the resolved SSH host ("" = local execution).
	Remote string
	// RemoteControl is the resolved remote-control helper ("" / "off" / "superterm").
	RemoteControl string
	// ApprovalMode is the resolved agent approval mode.
	ApprovalMode agent.ApprovalMode
	// MaxAgents is the resolved per-repo agent cap (0 = no cap).
	MaxAgents int
}

// ResolveControl implements the five-rung precedence ladder from ADR-061:
//
//  1. CLI flags.
//  2. Repo config [control].
//  3. User config [control] — already merged into cfg by the config layer,
//     so repo [control] wins over user [control] at Load time.
//  4. Subsystem defaults ([general].default_agent etc.).
//  5. Compiled defaults.
//
// Because config.LoadWithOptions merges user config first and then overlays
// repo config, cfg.Control already reflects the repo-wins-over-user merge.
// ResolveControl therefore applies only rungs 1 and 4/5 on top of cfg.Control.
func ResolveControl(flags ControlFlags, cfg config.Config) ControlContext {
	ctx := ControlContext{
		Agent:         resolveString(flags.Agent, cfg.Control.Agent, cfg.General.DefaultAgent, "pi"),
		Sandbox:       resolveString(flags.Sandbox, cfg.Control.Sandbox, cfg.Sandbox.DefaultProvider, ""),
		Remote:        resolveString(flags.Remote, cfg.Control.Remote, cfg.Remote.DefaultHost, ""),
		RemoteControl: resolveString(flags.RemoteControl, cfg.Control.RemoteControl, "", ""),
		MaxAgents:     resolveInt(flags.MaxAgents, cfg.Control.MaxAgents, 0),
	}

	approvalStr := resolveString(flags.ApprovalMode, cfg.Control.ApprovalMode, "", "")
	ctx.ApprovalMode = parseApprovalMode(approvalStr)

	return ctx
}

// resolveString returns the first non-empty value from the priority chain.
func resolveString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveInt returns the first non-zero value from the priority chain.
func resolveInt(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

// approvalModeToString converts a typed agent.ApprovalMode back to the
// canonical string stored in state.toml.
func approvalModeToString(m agent.ApprovalMode) string {
	switch m {
	case agent.ApprovalAuto:
		return "auto"
	case agent.ApprovalYolo:
		return "yolo"
	case agent.ApprovalDefault:
		return ""
	}
	return ""
}

// parseApprovalMode converts the string form ("", "auto", "yolo") to the
// typed agent.ApprovalMode constant.
func parseApprovalMode(s string) agent.ApprovalMode {
	switch s {
	case "auto":
		return agent.ApprovalAuto
	case "yolo":
		return agent.ApprovalYolo
	default:
		return agent.ApprovalDefault
	}
}
