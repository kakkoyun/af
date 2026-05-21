// Package sandbox provides sandbox provider implementations per ADR-060.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var (
	// ErrSlicerWTPushFailed reports that slicer wt push failed.
	ErrSlicerWTPushFailed = errors.New("slicer wt push failed")
	// ErrSlicerWTPullFailed reports that slicer wt pull failed.
	ErrSlicerWTPullFailed = errors.New("slicer wt pull failed")
	// ErrSlicerWTNameNotFound reports that the VM name could not be parsed from push output.
	ErrSlicerWTNameNotFound = errors.New("slicer wt push: could not determine VM name from output")
)

// vmNameRe matches VM name patterns in slicer wt push --launch output.
// Slicer prints lines like "Launched VM sbox-abc123" or "VM: sbox-abc123".
const (
	vmNameGroupLen = 2   // regex match has [full, group1]
	vmOutputMaxLen = 200 // chars to include in error context
)

var vmNameRe = regexp.MustCompile(`(?i)(?:launched\s+vm|vm[:\s]+)\s*(\S+)`)

// WTPushOptions configures slicer wt push --launch.
type WTPushOptions struct {
	// WorktreePath is the host worktree directory to push. Required.
	WorktreePath string
	// HostGroup is the slicer host group derived from ADR-062 resolution. Optional.
	HostGroup string
	// Tags are additional --tag entries beyond the af-standard ones.
	Tags []string
	// Depth truncates clone history. 0 means full history.
	Depth int
}

// WTPushResult captures post-push state.
type WTPushResult struct {
	PushedAt time.Time
	VM       string
}

// WTPush invokes `slicer wt push --launch [opts] <worktree-path>` and
// parses the launched VM name from the output per ADR-065.
func WTPush(ctx context.Context, runner Runner, opts WTPushOptions) (WTPushResult, error) {
	if opts.WorktreePath == "" {
		return WTPushResult{}, fmt.Errorf("%w: empty worktree path", ErrSlicerWTPushFailed)
	}
	args := wtPushArgs(opts)
	output, err := runner.Run(ctx, Command{Name: "slicer", Args: args})
	if err != nil {
		return WTPushResult{}, fmt.Errorf("%w: %w", ErrSlicerWTPushFailed, err)
	}
	vm, err := parseVMName(string(output))
	if err != nil {
		return WTPushResult{}, err
	}
	return WTPushResult{VM: vm, PushedAt: time.Now().UTC()}, nil
}

// WTPullOptions configures slicer wt pull.
type WTPullOptions struct {
	// VM is the VM name returned by WTPush. Required.
	VM string
	// WorktreePath is the host worktree directory to pull into. Required.
	WorktreePath string
}

// WTPullResult captures post-pull state.
type WTPullResult struct {
	PulledAt time.Time
}

// WTPull invokes `slicer wt pull <vm> <worktree-path>` per ADR-065.
func WTPull(ctx context.Context, runner Runner, opts WTPullOptions) (WTPullResult, error) {
	if opts.VM == "" {
		return WTPullResult{}, fmt.Errorf("%w: empty VM name", ErrSlicerWTPullFailed)
	}
	if opts.WorktreePath == "" {
		return WTPullResult{}, fmt.Errorf("%w: empty worktree path", ErrSlicerWTPullFailed)
	}
	args := []string{"wt", "pull", opts.VM, opts.WorktreePath}
	_, err := runner.Run(ctx, Command{Name: "slicer", Args: args})
	if err != nil {
		return WTPullResult{}, fmt.Errorf("%w: %w", ErrSlicerWTPullFailed, err)
	}
	return WTPullResult{PulledAt: time.Now().UTC()}, nil
}

func wtPushArgs(opts WTPushOptions) []string {
	args := []string{"wt", "push", "--launch"}
	if opts.HostGroup != "" {
		args = append(args, "--hostgroup", opts.HostGroup)
	}
	if opts.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(opts.Depth))
	}
	// Standard af tags so VMs can be identified via slicer wt list.
	args = append(args, "--tag", "af")
	for _, tag := range opts.Tags {
		args = append(args, "--tag", tag)
	}
	args = append(args, opts.WorktreePath)
	return args
}

// parseVMName extracts the VM name from slicer wt push --launch output.
// It tries the structured regex first; if that fails it looks for the last
// bare word that looks like a VM id (alphanumeric + hyphens).
func parseVMName(output string) (string, error) {
	if m := vmNameRe.FindStringSubmatch(output); len(m) == vmNameGroupLen {
		return m[1], nil
	}
	// Fallback: match any word that looks like a slicer VM name (e.g. "sbox-abc123").
	fallbackRe := regexp.MustCompile(`\b([a-z][a-z0-9-]{4,})\b`)
	matches := fallbackRe.FindAllString(output, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		candidate := matches[i]
		// Exclude common words that appear in help text.
		if candidate != "slicer" && candidate != "worktree" && candidate != "launch" {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w; output was: %s", ErrSlicerWTNameNotFound, truncate(output, vmOutputMaxLen))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
