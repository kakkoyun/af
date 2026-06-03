package sessiondata

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// AgentKind identifies one of the supported agent or harness session
// sources defined by ADR-066. Each kind maps to a fixed allowlisted
// source root inside the VM and a host-side destination root.
type AgentKind string

const (
	// KindClaude harvests ~/.claude/projects and ~/.claude/sessions.
	KindClaude AgentKind = "claude"
	// KindCodex harvests ~/.codex/sessions/**/rollout-*.jsonl.
	KindCodex AgentKind = "codex"
	// KindPi harvests pi's default sessionDir (~/.pi/agent/sessions).
	KindPi AgentKind = "pi"
	// KindHarness harvests ~/.pi/agent/teams and other harness roots.
	KindHarness AgentKind = "harness"
)

// ErrUnknownAgent reports an unsupported --agent value.
var ErrUnknownAgent = errors.New("sessiondata: unknown agent kind")

// AllKinds returns every supported agent kind in stable order. Used
// by --agent all and by inventory enumeration.
func AllKinds() []AgentKind {
	return []AgentKind{KindClaude, KindCodex, KindPi, KindHarness}
}

// ParseKindFlag parses the value of the --agent flag.
//
//   - "" or "all" → AllKinds()
//   - a single name (e.g. "pi") → []AgentKind{KindPi}
//   - a comma-separated list (e.g. "claude,codex") → its parsed kinds
//
// Unknown names return ErrUnknownAgent wrapped with the offending name.
// Duplicates are removed; the returned slice preserves the AllKinds
// ordering.
func ParseKindFlag(value string) ([]AgentKind, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "all" {
		return AllKinds(), nil
	}
	seen := make(map[AgentKind]struct{})
	for _, raw := range strings.Split(trimmed, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		kind, err := parseSingleKind(name)
		if err != nil {
			return nil, err
		}
		seen[kind] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("%w: empty list", ErrUnknownAgent)
	}
	out := make([]AgentKind, 0, len(seen))
	for _, kind := range AllKinds() {
		if _, ok := seen[kind]; ok {
			out = append(out, kind)
		}
	}
	return out, nil
}

func parseSingleKind(s string) (AgentKind, error) {
	switch AgentKind(strings.ToLower(s)) {
	case KindClaude:
		return KindClaude, nil
	case KindCodex:
		return KindCodex, nil
	case KindPi:
		return KindPi, nil
	case KindHarness:
		return KindHarness, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownAgent, s)
	}
}

// SourceRoots returns the allowlisted source roots inside the VM for
// kind, expressed as paths relative to the VM user's $HOME.
//
// Multiple roots per kind are supported (Claude Code has both
// projects/ and sessions/). The slicer transport copies each root
// independently into the staging directory.
func SourceRoots(kind AgentKind) []string {
	switch kind {
	case KindClaude:
		return []string{".claude/projects", ".claude/sessions"}
	case KindCodex:
		return []string{".codex/sessions"}
	case KindPi:
		return []string{".pi/agent/sessions"}
	case KindHarness:
		return []string{".pi/agent/teams"}
	default:
		return nil
	}
}

// HostDestination returns the host-relative destination directory for
// kind. The merge step joins this against the host user's $HOME.
//
// The destination layout intentionally mirrors the VM source layout so
// host-side tools (recall indexes, agent resume commands) find the
// imported transcripts in the same place as native host sessions.
func HostDestination(kind AgentKind) string {
	switch kind {
	case KindClaude:
		// Claude has two roots; the merge step preserves both as
		// sub-trees underneath ~/.claude/.
		return ".claude"
	case KindCodex:
		return ".codex"
	case KindPi:
		return ".pi/agent"
	case KindHarness:
		return ".pi/agent"
	default:
		return ""
	}
}

// SourceRootRelToHostHome returns the host-relative path that one VM
// source root should land at. For kinds with multiple roots (Claude),
// each VM root maps to the matching subdirectory under HostDestination.
//
// Example: vmRoot ".claude/projects" → ".claude/projects".
//
// Returns the empty string when vmRoot is not part of kind's allowlist.
func SourceRootRelToHostHome(kind AgentKind, vmRoot string) string {
	for _, root := range SourceRoots(kind) {
		if root == vmRoot {
			return root
		}
	}
	return ""
}

// SortedRoots returns every allowlisted VM root across all kinds in a
// deterministic order. Useful for diagnostics.
func SortedRoots() []string {
	seen := make(map[string]struct{})
	for _, kind := range AllKinds() {
		for _, root := range SourceRoots(kind) {
			seen[root] = struct{}{}
		}
	}
	roots := make([]string, 0, len(seen))
	for root := range seen {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}
