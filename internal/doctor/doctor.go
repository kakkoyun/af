// Package doctor probes the local (or remote) environment for the tools
// that af relies on and renders an install-hint report.
//
// ADR-044 specifies the probe set and tier semantics:
//
//   - Must: missing tools fail af's core operation (exit 1).
//   - Should: missing tools degrade some commands but af still runs.
//   - Nice:   missing tools only matter for opt-in features.
//
// The package is intentionally pure: callers inject a Lookup that
// resolves binary paths and a Platform for hint selection, so unit
// tests do not require any real binaries.
package doctor

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Tier classifies how critical a probed tool is.
type Tier int

const (
	// TierMust marks tools whose absence fails af's core operation.
	TierMust Tier = iota
	// TierShould marks tools used by common but non-essential commands.
	TierShould
	// TierNice marks tools used by opt-in features only.
	TierNice
)

// String returns a single-word label for the tier.
func (t Tier) String() string {
	switch t {
	case TierMust:
		return "Must"
	case TierShould:
		return "Should"
	case TierNice:
		return "Nice"
	default:
		return "Unknown"
	}
}

// Platform names the host OS / distro family.
type Platform string

const (
	// PlatformMacOS targets Darwin hosts.
	PlatformMacOS Platform = "macos"
	// PlatformArch targets Arch / Manjaro hosts.
	PlatformArch Platform = "arch"
	// PlatformDebian targets Debian / Ubuntu hosts.
	PlatformDebian Platform = "debian"
	// PlatformOther is the fallback when no other family matches.
	PlatformOther Platform = "other"
)

// Probe describes a single tool the doctor checks for.
type Probe struct {
	// Hints maps platform to install advice; PlatformOther is the fallback.
	Hints map[Platform]string
	// Name is the binary name resolved through PATH.
	Name string
	// Group, when non-empty, defines an OR-set: a TierMust probe in a
	// non-empty group is satisfied if *any* probe in that group is found.
	Group string
	// Reason is a short human description ("core git operations").
	Reason string
	// Tier classifies how critical the tool is.
	Tier Tier
}

// Lookup resolves binary paths and version strings.
type Lookup interface {
	LookPath(ctx context.Context, name string) (string, bool)
	Version(ctx context.Context, binary string) string
}

// Result records the outcome of one Probe execution.
type Result struct { //nolint:govet // ADR-044 prioritises field readability over packing; the Probe value embeds a string-heavy struct.
	Probe   Probe
	Path    string
	Version string
	Found   bool
}

// Report aggregates results from a probe run.
type Report struct {
	Platform Platform
	Results  []Result
	// MissingMustTools lists Names of TierMust probes that failed.
	// Probes inside satisfied OR-groups are excluded.
	MissingMustTools []string
}

// HasMissingMustTools reports whether any TierMust requirement failed.
func (r Report) HasMissingMustTools() bool {
	return len(r.MissingMustTools) > 0
}

// DefaultProbes returns the probe set defined by ADR-044, with any
// user-configured [doctor].extra_tools appended at the TierNice tier.
func DefaultProbes(extraTools []string) []Probe {
	probes := mustTierProbes()
	probes = append(probes, shouldTierProbes()...)
	probes = append(probes, niceTierProbes()...)
	probes = append(probes, extraToolProbes(extraTools)...)
	return probes
}

func mustTierProbes() []Probe {
	return []Probe{
		{Name: "git", Tier: TierMust, Reason: "core git operations", Hints: map[Platform]string{
			PlatformMacOS:  "brew install git",
			PlatformArch:   "pacman -S git",
			PlatformDebian: "apt install git",
			PlatformOther:  "see https://git-scm.com/downloads",
		}},
		{Name: "tmux", Tier: TierMust, Reason: "multiplexer (ADR-040)", Hints: map[Platform]string{
			PlatformMacOS:  "brew install tmux",
			PlatformArch:   "pacman -S tmux",
			PlatformDebian: "apt install tmux",
			PlatformOther:  "see https://github.com/tmux/tmux",
		}},
		{Name: "pi", Tier: TierMust, Group: "agent", Reason: "agent provider (ADR-043)", Hints: map[Platform]string{
			PlatformOther: "see https://github.com/earendil-works/pi",
		}},
		{Name: "claude", Tier: TierMust, Group: "agent", Reason: "agent provider (ADR-043)", Hints: map[Platform]string{
			PlatformOther: "npm install -g @anthropic-ai/claude-code",
		}},
		{Name: "codex", Tier: TierMust, Group: "agent", Reason: "agent provider (ADR-043)", Hints: map[Platform]string{
			PlatformOther: "see https://github.com/openai/codex-cli",
		}},
	}
}

func shouldTierProbes() []Probe {
	return []Probe{
		{Name: "gh", Tier: TierShould, Reason: "PR detection, af pr (ADR-048)", Hints: map[Platform]string{
			PlatformMacOS:  "brew install gh",
			PlatformArch:   "pacman -S github-cli",
			PlatformDebian: "apt install gh",
			PlatformOther:  "see https://cli.github.com",
		}},
		{Name: "fzf", Tier: TierShould, Reason: "session picker in af resume", Hints: map[Platform]string{
			PlatformMacOS:  "brew install fzf",
			PlatformArch:   "pacman -S fzf",
			PlatformDebian: "apt install fzf",
			PlatformOther:  "see https://github.com/junegunn/fzf",
		}},
	}
}

func niceTierProbes() []Probe {
	return []Probe{
		{Name: "slicer", Tier: TierNice, Reason: "local sandbox (ADR-042)", Hints: map[Platform]string{
			PlatformOther: "see https://slicervm.com/install",
		}},
		{Name: "sbx", Tier: TierNice, Reason: "local sandbox (ADR-042)", Hints: map[Platform]string{
			PlatformOther: "see https://docs.docker.com/ai/sandboxes/",
		}},
		{Name: "delta", Tier: TierNice, Reason: "nicer af diff rendering", Hints: map[Platform]string{
			PlatformMacOS:  "brew install git-delta",
			PlatformArch:   "pacman -S git-delta",
			PlatformDebian: "apt install git-delta",
			PlatformOther:  "see https://github.com/dandavison/delta",
		}},
	}
}

func extraToolProbes(extraTools []string) []Probe {
	out := make([]Probe, 0, len(extraTools))
	for _, tool := range extraTools {
		name := strings.TrimSpace(tool)
		if name == "" {
			continue
		}
		out = append(out, Probe{
			Name:   name,
			Tier:   TierNice,
			Reason: "user-configured extra tool ([doctor].extra_tools)",
			Hints:  map[Platform]string{PlatformOther: "see upstream"},
		})
	}
	return out
}

// Run executes every probe through lookup and returns a Report.
//
// Group semantics: probes that share a non-empty Group are linked. A
// TierMust probe inside a group counts as missing only when the entire
// group is missing.
func Run(ctx context.Context, lookup Lookup, platform Platform, probes []Probe) Report {
	report := Report{Platform: platform, Results: make([]Result, 0, len(probes))}

	groupFound := make(map[string]bool)
	for _, probe := range probes {
		path, ok := lookup.LookPath(ctx, probe.Name)
		result := Result{Probe: probe, Found: ok, Path: path}
		if ok {
			result.Version = lookup.Version(ctx, probe.Name)
			if probe.Group != "" {
				groupFound[probe.Group] = true
			}
		}
		report.Results = append(report.Results, result)
	}

	for _, result := range report.Results {
		if result.Probe.Tier != TierMust || result.Found {
			continue
		}
		if result.Probe.Group != "" && groupFound[result.Probe.Group] {
			continue
		}
		report.MissingMustTools = append(report.MissingMustTools, result.Probe.Name)
	}

	sort.Strings(report.MissingMustTools)
	return report
}

// Render writes a human-readable report to w. The first line is a
// heading; per-probe lines follow; missing TierMust tools surface as a
// trailing summary.
func Render(w io.Writer, report Report, heading string) error {
	_, err := fmt.Fprintf(w, "%s\n\n", heading)
	if err != nil {
		return fmt.Errorf("render heading: %w", err)
	}

	groupSatisfied := computeGroupSatisfied(report)

	for _, result := range report.Results {
		err = renderResult(w, result, report.Platform, groupSatisfied)
		if err != nil {
			return err
		}
	}

	missing := report.MissingMustTools
	if len(missing) == 0 {
		_, err = fmt.Fprintln(w, "\nStatus: all required tools present.")
	} else {
		_, err = fmt.Fprintf(w, "\nStatus: %d missing required tool(s): %s.\n", len(missing), strings.Join(missing, ", "))
	}
	if err != nil {
		return fmt.Errorf("render status: %w", err)
	}

	return nil
}

func computeGroupSatisfied(report Report) map[string]bool {
	out := make(map[string]bool)
	for _, result := range report.Results {
		if result.Found && result.Probe.Group != "" {
			out[result.Probe.Group] = true
		}
	}
	return out
}

func renderResult(w io.Writer, result Result, platform Platform, groupSatisfied map[string]bool) error {
	marker := resultMarker(result, groupSatisfied)

	if result.Found {
		return renderFound(w, marker, result)
	}

	return renderMissing(w, marker, result, platform, groupSatisfied)
}

func renderFound(w io.Writer, marker string, result Result) error {
	detail := "in PATH"
	if result.Path != "" {
		detail = result.Path
	}
	if result.Version != "" {
		detail += ", version " + result.Version
	}
	_, err := fmt.Fprintf(w, "  %s %-12s (%s)\n", marker, result.Probe.Name, detail)
	if err != nil {
		return fmt.Errorf("render found: %w", err)
	}
	return nil
}

func renderMissing(w io.Writer, marker string, result Result, platform Platform, groupSatisfied map[string]bool) error {
	suffix := missingSuffix(result, groupSatisfied)
	_, err := fmt.Fprintf(w, "  %s %-12s %s\n", marker, result.Probe.Name, suffix)
	if err != nil {
		return fmt.Errorf("render missing: %w", err)
	}

	hint := result.Probe.Hints[platform]
	if hint == "" {
		hint = result.Probe.Hints[PlatformOther]
	}
	if hint != "" {
		_, err = fmt.Fprintf(w, "        → install: %s\n", hint)
		if err != nil {
			return fmt.Errorf("render hint: %w", err)
		}
	}
	return nil
}

func missingSuffix(result Result, groupSatisfied map[string]bool) string {
	if result.Probe.Tier == TierMust && result.Probe.Group != "" && groupSatisfied[result.Probe.Group] {
		return "not in PATH (group satisfied by another agent)"
	}
	if result.Probe.Tier == TierShould || result.Probe.Tier == TierNice {
		return fmt.Sprintf("not in PATH (optional; %s)", result.Probe.Reason)
	}
	return "not in PATH"
}

func resultMarker(result Result, groupSatisfied map[string]bool) string {
	if result.Found {
		return "✓"
	}
	if result.Probe.Tier == TierMust {
		if result.Probe.Group != "" && groupSatisfied[result.Probe.Group] {
			return "⚠"
		}
		return "✗"
	}
	return "⚠"
}
