package agent

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// ErrUnknown reports an unknown agent provider name.
var ErrUnknown = errors.New("unknown agent")

// ErrUnavailable reports that no configured agent is available.
var ErrUnavailable = errors.New("no agent available")

// ApprovalMode controls provider-specific permission prompts.
type ApprovalMode int

const (
	// ApprovalDefault uses the agent's own default approval behavior.
	ApprovalDefault ApprovalMode = iota
	// ApprovalAuto asks the agent to auto-approve safe operations when supported.
	ApprovalAuto
	// ApprovalYolo asks the agent to skip all permission prompts when supported.
	ApprovalYolo
)

// LaunchOpts configures a new interactive agent session.
type LaunchOpts struct {
	SessionID    string
	Cwd          string
	ApprovalMode ApprovalMode
}

// ResumeOpts configures a resumed interactive agent session.
type ResumeOpts struct {
	SessionID    string
	Cwd          string
	ApprovalMode ApprovalMode
}

// BodyOpts configures a non-interactive agent invocation.
type BodyOpts struct {
	Cwd   string
	Model string
}

// Agent describes a supported coding-agent provider.
type Agent interface {
	Name() string
	Binary() string
	IsAvailable(ctx context.Context) bool
	LaunchCmd(opts LaunchOpts) []string
	ResumeCmd(opts ResumeOpts) []string
	PRCmd(prNumber int, opts LaunchOpts) ([]string, bool)
	BodyCmd(opts BodyOpts) ([]string, bool)
	SessionLogPaths(sessionID, projectPath string) []string
}

// KnownAgents returns provider names in fallback order.
func KnownAgents() []string {
	return []string{"pi", "claude", "codex"}
}

// ExecutableAvailable reports whether binary can be found on PATH.
func ExecutableAvailable(ctx context.Context, binary string) bool {
	if ctx.Err() != nil {
		return false
	}
	_, err := exec.LookPath(binary)

	return err == nil
}

type providerKind int

const (
	providerPi providerKind = iota
	providerClaude
	providerCodex
)

// Provider is a built-in agent provider implementation.
type Provider struct {
	name   string
	binary string
	kind   providerKind
}

// NewPi returns the pi provider.
func NewPi() Provider {
	return Provider{name: "pi", binary: "pi", kind: providerPi}
}

// NewClaude returns the claude provider.
func NewClaude() Provider {
	return Provider{name: "claude", binary: "claude", kind: providerClaude}
}

// NewCodex returns the codex provider.
func NewCodex() Provider {
	return Provider{name: "codex", binary: "codex", kind: providerCodex}
}

// DefaultRegistry returns the built-in provider registry.
func DefaultRegistry() *Registry {
	return NewRegistry(NewPi(), NewClaude(), NewCodex())
}

// Name returns the provider name.
func (provider Provider) Name() string {
	return provider.name
}

// Binary returns the provider executable name.
func (provider Provider) Binary() string {
	return provider.binary
}

// IsAvailable reports whether the provider executable is on PATH.
func (provider Provider) IsAvailable(ctx context.Context) bool {
	return ExecutableAvailable(ctx, provider.binary)
}

// LaunchCmd returns the argv for a new session.
func (provider Provider) LaunchCmd(opts LaunchOpts) []string {
	switch provider.kind {
	case providerPi:
		return []string{provider.binary}
	case providerClaude:
		argv := []string{provider.binary}
		if opts.SessionID != "" {
			argv = append(argv, "--session-id", opts.SessionID)
		}
		return appendApproval(argv, opts.ApprovalMode, provider.kind)
	case providerCodex:
		return appendApproval([]string{provider.binary}, opts.ApprovalMode, provider.kind)
	default:
		return []string{provider.binary}
	}
}

// ResumeCmd returns the argv for resuming a session.
func (provider Provider) ResumeCmd(opts ResumeOpts) []string {
	switch provider.kind {
	case providerPi:
		return []string{provider.binary, "--continue"}
	case providerClaude:
		return appendApproval([]string{provider.binary, "--continue"}, opts.ApprovalMode, provider.kind)
	case providerCodex:
		argv := appendApproval([]string{provider.binary}, opts.ApprovalMode, provider.kind)
		return append(argv, "resume", "--last")
	default:
		return []string{provider.binary}
	}
}

// PRCmd returns the argv for provider-native PR handling when supported.
func (Provider) PRCmd(_ int, _ LaunchOpts) ([]string, bool) {
	return nil, false
}

// BodyCmd returns the argv for non-interactive body generation.
func (provider Provider) BodyCmd(opts BodyOpts) ([]string, bool) {
	switch provider.kind {
	case providerPi:
		return appendModel([]string{provider.binary, "--print"}, opts.Model), true
	case providerClaude:
		return appendModel([]string{provider.binary, "-p"}, opts.Model), true
	case providerCodex:
		return appendModel([]string{provider.binary, "exec"}, opts.Model), true
	default:
		return nil, false
	}
}

// SessionLogPaths returns provider-owned session logs for analysis.
func (provider Provider) SessionLogPaths(sessionID, projectPath string) []string {
	encodedProject := url.PathEscape(filepath.Clean(projectPath))
	home := os.Getenv("HOME")
	switch provider.kind {
	case providerPi:
		return []string{filepath.Join(home, ".pi", "agent", "sessions", encodedProject, "*"+sessionID+"*.jsonl")}
	case providerClaude:
		return []string{filepath.Join(home, ".claude", "projects", encodedProject, sessionID+".jsonl")}
	case providerCodex:
		return nil
	default:
		return nil
	}
}

func appendApproval(argv []string, mode ApprovalMode, kind providerKind) []string {
	switch kind {
	case providerPi:
		return argv
	case providerClaude:
		if mode == ApprovalYolo {
			return append(argv, "--dangerously-skip-permissions")
		}
	case providerCodex:
		if mode == ApprovalYolo {
			return append(argv, "--full-auto")
		}
		if mode == ApprovalAuto {
			return append(argv, "--auto")
		}
	}

	return argv
}

func appendModel(argv []string, model string) []string {
	if model == "" {
		return argv
	}

	return append(argv, "--model", model)
}

// Registry resolves providers by name and fallback availability.
type Registry struct {
	agents map[string]Agent
}

// NewRegistry returns a registry containing agents keyed by Agent.Name.
func NewRegistry(agents ...Agent) *Registry {
	registry := &Registry{agents: map[string]Agent{}}
	for _, candidate := range agents {
		registry.agents[candidate.Name()] = candidate
	}

	return registry
}

// Resolve returns the provider named name.
//
//nolint:ireturn // Registry intentionally hides concrete provider implementations.
func (registry *Registry) Resolve(name string) (Agent, error) {
	provider, ok := registry.agents[name]
	if !ok {
		return nil, fmt.Errorf("resolve agent %s: %w", name, ErrUnknown)
	}

	return provider, nil
}

// FirstAvailable returns the first available provider in configured fallback order.
//
//nolint:ireturn // Registry intentionally hides concrete provider implementations.
func (registry *Registry) FirstAvailable(ctx context.Context) (Agent, error) {
	for _, name := range KnownAgents() {
		provider, ok := registry.agents[name]
		if ok && provider.IsAvailable(ctx) {
			return provider, nil
		}
	}
	remaining := make([]string, 0, len(registry.agents))
	for name := range registry.agents {
		remaining = append(remaining, name)
	}
	sort.Strings(remaining)
	for _, name := range remaining {
		provider := registry.agents[name]
		if provider.IsAvailable(ctx) {
			return provider, nil
		}
	}

	return nil, ErrUnavailable
}
