package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// ErrUnsupportedProvider reports an unknown or removed sandbox provider name.
var ErrUnsupportedProvider = errors.New("unsupported sandbox provider")

// LaunchOpts configures a sandbox launch.
type LaunchOpts struct {
	Workstream string
	Worktree   string
	AgentArgv  []string
	// Tags are additional --tag entries appended to the standard af tags (ADR-065).
	Tags []string
}

// Handle identifies a running sandbox.
type Handle struct {
	ID        string
	VMName    string
	AttachCmd []string
}

// Sandbox manages one sandbox provider.
type Sandbox interface {
	Name() string
	IsAvailable(ctx context.Context) bool
	Launch(ctx context.Context, opts LaunchOpts) (*Handle, error)
	Attach(ctx context.Context, handle *Handle) error
	IsHealthy(ctx context.Context, handle *Handle) (bool, error)
	Teardown(ctx context.Context, handle *Handle) error
	List(ctx context.Context) ([]Handle, error)
}

// Command is one external command invocation.
type Command struct {
	Name string
	Dir  string
	Args []string
}

// Runner executes sandbox CLI commands.
type Runner interface {
	Run(ctx context.Context, command Command) ([]byte, error)
}

// ExecRunner runs commands through os/exec.
type ExecRunner struct{}

// Run executes command and returns combined stdout/stderr.
func (ExecRunner) Run(ctx context.Context, command Command) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...) //nolint:gosec // Provider argv is constructed by typed methods, not shell input.
	cmd.Dir = command.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run %s %s: %w", command.Name, strings.Join(command.Args, " "), err)
	}

	return output, nil
}

// KnownProviders returns sandbox provider names in fallback order.
// Only "slicer" is supported per ADR-060.
func KnownProviders() []string {
	return []string{"slicer"}
}

// NewProvider returns the named sandbox provider backed by os/exec.
// Only "slicer" is accepted; all other names return ErrUnsupportedProvider.
//
//nolint:ireturn // Factory intentionally returns the Sandbox interface.
func NewProvider(name string) (Sandbox, error) {
	switch name {
	case "slicer":
		return NewSlicer(), nil
	default:
		return nil, fmt.Errorf("%w: %q (only \"slicer\" is supported per ADR-060)", ErrUnsupportedProvider, name)
	}
}

// Provider is a CLI-backed sandbox provider.
type Provider struct {
	runner    Runner
	resources SlicerResources
	name      string
	binary    string
	group     string
	kind      providerKind
}

type providerKind int

const (
	providerSlicer providerKind = iota
)

// SlicerOptions configures a slicer provider with an optional resource
// profile and group name, resolved by sandbox.ResolveLaunchGroup.
type SlicerOptions struct {
	// Group is the host group name to use for launches. Empty means use
	// the slicer daemon default.
	Group string
	// Resources is the resolved resource profile. When non-empty, slicer
	// receives the profile via the --group flag pointing to a managed group
	// already created (or flagged needCreate) by ResolveLaunchGroup.
	Resources SlicerResources
}

// NewSlicer returns the slicer sandbox provider with no group or resource overrides.
func NewSlicer() Provider {
	return NewSlicerWithRunner(ExecRunner{})
}

// NewSlicerWithRunner returns the slicer sandbox provider using runner.
func NewSlicerWithRunner(runner Runner) Provider {
	return newProvider("slicer", providerSlicer, runner)
}

// NewSlicerProvider returns a slicer provider configured with SlicerOptions.
// Use this when ResolveLaunchGroup has already determined the group name and
// resource profile.
func NewSlicerProvider(opts SlicerOptions, runner Runner) Provider {
	p := newProvider("slicer", providerSlicer, runner)
	p.group = opts.Group
	p.resources = opts.Resources
	return p
}

func newProvider(binary string, kind providerKind, runner Runner) Provider {
	if runner == nil {
		runner = ExecRunner{}
	}

	return Provider{name: binary, binary: binary, kind: kind, runner: runner}
}

// Name returns the provider name.
func (provider Provider) Name() string {
	return provider.name
}

// IsAvailable reports whether the provider binary is on PATH.
func (provider Provider) IsAvailable(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	_, err := exec.LookPath(provider.binary)

	return err == nil
}

// Launch starts a sandbox and returns its handle.
func (provider Provider) Launch(ctx context.Context, opts LaunchOpts) (*Handle, error) {
	switch provider.kind {
	case providerSlicer:
		return provider.slicerWTLaunch(ctx, opts)
	default:
		return nil, fmt.Errorf("launch %s sandbox: %w", provider.name, ErrUnsupportedProvider)
	}
}

// Attach attaches to a running sandbox.
func (provider Provider) Attach(ctx context.Context, handle *Handle) error {
	_, err := provider.run(ctx, provider.attachArgs(handle.ID)...)
	if err != nil {
		return fmt.Errorf("attach %s sandbox %s: %w", provider.name, handle.ID, err)
	}

	return nil
}

// IsHealthy reports whether a sandbox appears alive.
func (provider Provider) IsHealthy(ctx context.Context, handle *Handle) (bool, error) {
	_, err := provider.run(ctx, provider.healthArgs(handle.ID)...)
	if err != nil {
		return false, fmt.Errorf("check %s sandbox %s: %w", provider.name, handle.ID, err)
	}

	return true, nil
}

// Teardown removes a sandbox.
func (provider Provider) Teardown(ctx context.Context, handle *Handle) error {
	_, err := provider.run(ctx, provider.teardownArgs(handle.ID)...)
	if err != nil {
		return fmt.Errorf("teardown %s sandbox %s: %w", provider.name, handle.ID, err)
	}

	return nil
}

// List returns running sandboxes known by the provider.
func (provider Provider) List(ctx context.Context) ([]Handle, error) {
	output, err := provider.run(ctx, provider.listArgs()...)
	if err != nil {
		return nil, fmt.Errorf("list %s sandboxes: %w", provider.name, err)
	}
	ids := strings.Fields(string(output))
	handles := make([]Handle, 0, len(ids))
	for _, id := range ids {
		handles = append(handles, Handle{ID: id, AttachCmd: provider.attachCommand(id)})
	}
	sort.Slice(handles, func(i, j int) bool { return handles[i].ID < handles[j].ID })

	return handles, nil
}

// slicerWTLaunch performs `slicer wt push --launch` and returns a Handle with the VM name.
func (provider Provider) slicerWTLaunch(ctx context.Context, opts LaunchOpts) (*Handle, error) {
	pushOpts := WTPushOptions{
		WorktreePath: opts.Worktree,
		HostGroup:    provider.group,
		Tags:         wtTags(opts),
	}
	result, err := WTPush(ctx, provider.runner, pushOpts)
	if err != nil {
		return nil, fmt.Errorf("slicer wt push: %w", err)
	}
	return &Handle{
		ID:        result.VM,
		VMName:    result.VM,
		AttachCmd: []string{provider.binary, "vm", "shell", result.VM},
	}, nil
}

const wtBaseTagCount = 2 // "af" + "af-session=..."

func wtTags(opts LaunchOpts) []string {
	tags := make([]string, 0, len(opts.Tags)+wtBaseTagCount)
	if opts.Workstream != "" {
		tags = append(tags, "af-session="+opts.Workstream)
	}
	tags = append(tags, opts.Tags...)
	return tags
}

func (provider Provider) attachCommand(id string) []string {
	return append([]string{provider.binary}, provider.attachArgs(id)...)
}

func (provider Provider) attachArgs(id string) []string {
	switch provider.kind {
	case providerSlicer:
		return []string{"vm", "shell", id}
	default:
		return nil
	}
}

func (provider Provider) healthArgs(id string) []string {
	switch provider.kind {
	case providerSlicer:
		return []string{"vm", "status", id}
	default:
		return nil
	}
}

func (provider Provider) teardownArgs(id string) []string {
	switch provider.kind {
	case providerSlicer:
		return []string{"vm", "delete", id}
	default:
		return nil
	}
}

func (provider Provider) listArgs() []string {
	switch provider.kind {
	case providerSlicer:
		return []string{"vm", "list"}
	default:
		return nil
	}
}

func (provider Provider) run(ctx context.Context, args ...string) ([]byte, error) {
	output, err := provider.runner.Run(ctx, Command{Name: provider.binary, Args: args})
	if err != nil {
		return output, fmt.Errorf("%s %s: %w", provider.binary, strings.Join(args, " "), err)
	}

	return output, nil
}
