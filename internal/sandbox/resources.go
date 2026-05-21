package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrSlicerResourceMismatch reports that a named managed group exists but its
// resource shape does not match the requested profile. The caller should
// choose a different profile name or update the group manually.
var ErrSlicerResourceMismatch = errors.New(
	"requested resource shape does not match existing slicer group; " +
		"choose a new profile name or update the slicer group manually",
)

// SlicerResources is the runtime view of the resolved slicer resource profile.
// Zero values mean "use slicer/group default".
//
//nolint:govet // field order prioritises readability over pointer-size packing
type SlicerResources struct {
	VCPU        int
	RAMGB       int
	GPUCount    int
	Name        string // optional profile name; empty = derived from repo slug
	StorageSize string
	Image       string
	Hypervisor  string
}

// IsEmpty reports whether every resource field is at its zero value.
// An empty SlicerResources means "no profile override; use whatever the
// target group or slicer daemon defaults to".
func (r SlicerResources) IsEmpty() bool {
	return r.VCPU == 0 &&
		r.RAMGB == 0 &&
		r.StorageSize == "" &&
		r.GPUCount == 0 &&
		r.Image == "" &&
		r.Hypervisor == ""
}

// ManagedGroupName returns the deterministic af-managed group name for a repo
// and profile. Profile "" maps to "default". The name follows the pattern
// "af-<repo-slug>-<profile>" where non-alphanumeric chars in the slug are
// replaced with "-" and sequences are collapsed.
//
// Example: repo "github.com/kakkoyun/af", profile "tight" → "af-github-com-kakkoyun-af-tight".
func ManagedGroupName(repoSlug, profile string) string {
	slug := sanitizeSlug(repoSlug)
	if profile == "" {
		profile = "default"
	}
	profileSafe := sanitizeSlug(profile)
	return "af-" + slug + "-" + profileSafe
}

// sanitizeSlug replaces non-alphanumeric runes with "-" and collapses runs.
func sanitizeSlug(s string) string {
	var b strings.Builder
	prev := '-'
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prev = r
		} else if prev != '-' {
			b.WriteByte('-')
			prev = '-'
		}
	}
	return strings.Trim(b.String(), "-")
}

// GroupProber probes the slicer CLI for an existing host group.
// Returns (exists, shapeMatches, err). shapeMatches is meaningful only
// when exists is true.
type GroupProber interface {
	Probe(ctx context.Context, name string, want SlicerResources) (exists bool, shapeMatches bool, err error)
}

// ResolveLaunchGroup decides which group name to use when launching a
// slicer sandbox.
//
//   - If r.IsEmpty() and cfgGroup != "" → return cfgGroup, needCreate=false.
//   - If r.IsEmpty() and cfgGroup == "" → return "", needCreate=false (use slicer default).
//   - If !r.IsEmpty() → derive managed name, probe; return managed name + needCreate flag.
func ResolveLaunchGroup(ctx context.Context, prober GroupProber, repoSlug, cfgGroup string, r SlicerResources) (string, bool, error) {
	if r.IsEmpty() {
		return cfgGroup, false, nil
	}
	return resolveResourceGroup(ctx, prober, repoSlug, r)
}

func resolveResourceGroup(ctx context.Context, prober GroupProber, repoSlug string, r SlicerResources) (string, bool, error) {
	managedName := ManagedGroupName(repoSlug, r.Name)
	exists, matches, err := prober.Probe(ctx, managedName, r)
	if err != nil {
		return "", false, fmt.Errorf("probe slicer group %q: %w", managedName, err)
	}
	if !exists {
		return managedName, true, nil
	}
	if !matches {
		return "", false, fmt.Errorf("%w: group=%q", ErrSlicerResourceMismatch, managedName)
	}
	return managedName, false, nil
}

// groupNameRe matches a group name line from `slicer vm group` output.
// The real slicer output format at time of writing is a table with headers;
// we match lines that start with optional whitespace followed by a
// non-empty token that looks like a group name.
//
// NOTE (ADR-062): slicer's machine-readable output format is not fully
// stable. The regex is intentionally permissive: it looks for the target
// name as a word anywhere in the output. If slicer adds a --json flag,
// prefer that in a future revision.
var groupNameRe = regexp.MustCompile(`(?m)\b([A-Za-z0-9_-]+)\b`)

// ExecGroupProber probes the slicer CLI via `slicer vm group`.
// It uses a Runner so tests can inject fakes without a real slicer daemon.
type ExecGroupProber struct {
	Runner Runner
}

// Probe calls `slicer vm group` and checks whether name appears in the output.
// Shape matching is not yet implemented at the CLI level because slicer does
// not expose a stable machine-readable format for individual group fields.
// When the group is found, shapeMatches is always returned as true (optimistic).
// A future revision should add `slicer vm group --json` parsing once that flag
// is documented. See ADR-062 §Resolution step 6.
func (p ExecGroupProber) Probe(ctx context.Context, name string, _ SlicerResources) (bool, bool, error) {
	output, err := p.Runner.Run(ctx, Command{Name: "slicer", Args: []string{"vm", "group"}})
	if err != nil {
		// Connection-refused from a non-running daemon is not a fatal error —
		// treat as "group does not exist yet" so callers emit the needCreate hint.
		if isSlicerDaemonUnavailable(err) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("slicer vm group: %w", err)
	}
	found := groupNamePresentInOutput(output, name)
	return found, found, nil
}

// isSlicerDaemonUnavailable returns true for connection-refused errors from
// the slicer daemon API. These are not probe failures; they mean the daemon
// is not running and no groups can be validated.
func isSlicerDaemonUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connect: connection refused")
}

// groupNamePresentInOutput scans CLI output for a word that exactly equals name.
func groupNamePresentInOutput(output []byte, name string) bool {
	for _, line := range bytes.Split(output, []byte("\n")) {
		for _, match := range groupNameRe.FindAllSubmatch(line, -1) {
			if string(match[1]) == name {
				return true
			}
		}
	}
	return false
}
