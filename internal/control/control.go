// Package control manages the optional remote-control helper that composes
// superterm and Tailscale Serve to expose a tmux-aware browser dashboard over
// the owner's tailnet, per ADR-063.
//
// The package is intentionally side-effect-free: callers inject an Executor so
// unit tests work entirely with fakes and never require real superterm,
// tailscale, or SSH binaries.
package control

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ProviderSuperterm is the only supported remote-control provider in v1.
const ProviderSuperterm = "superterm"

var (
	// ErrProviderUnsupported is returned when provider is not "superterm".
	ErrProviderUnsupported = errors.New("unsupported remote-control provider")
	// ErrSupertermMissing is returned when the superterm binary is absent.
	ErrSupertermMissing = errors.New("superterm not found; install superterm to use remote control")
	// ErrTailscaleMissing is returned when the tailscale binary is absent.
	ErrTailscaleMissing = errors.New("tailscale not found; install tailscale to use remote control")
	// ErrSupertermStart is returned when superterm fails to start.
	ErrSupertermStart = errors.New("superterm failed to start")
	// ErrTailscaleServe is returned when tailscale serve fails.
	ErrTailscaleServe = errors.New("tailscale serve failed")
	// ErrUnresolvableEndpoint is returned when a URL cannot be parsed from output.
	ErrUnresolvableEndpoint = errors.New("could not determine endpoint URL from output")
)

// compiled once at package init.
var (
	reLocalURL   = regexp.MustCompile(`http://(?:localhost|127\.0\.0\.1):(\d+)`)
	reTailnetURL = regexp.MustCompile(`https://[a-zA-Z0-9._-]+\.ts\.net\S*`)
)

// Endpoint describes a running remote-control session.
type Endpoint struct {
	// LocalURL is the superterm URL on the local machine (e.g. http://localhost:7681).
	LocalURL string
	// TailnetURL is the public tailnet HTTPS URL (e.g. https://node.tailnet.ts.net/).
	TailnetURL string
	// Host is empty for local sessions or the SSH host for remote sessions.
	Host string
}

// Options configure a control operation.
type Options struct {
	// Provider selects the remote-control provider. Only "superterm" is supported.
	Provider string
	// Host, when non-empty, runs all commands over SSH on that host.
	Host string
	// Port is a hint for the superterm port. 0 means "let superterm choose".
	Port int
}

// Executor runs external commands and returns combined output.
// It is the single injection point; tests supply a fake.
type Executor interface {
	// Exec runs name with args (in dir if non-empty) and returns combined output.
	Exec(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// Deps wires the package to its external collaborators.
type Deps struct {
	Exec Executor
}

// Up starts remote control. It probes for required binaries, starts superterm,
// enables Tailscale Serve, and returns the resulting Endpoint.
func Up(ctx context.Context, deps Deps, opts Options) (Endpoint, error) {
	err := validateProvider(opts.Provider)
	if err != nil {
		return Endpoint{}, err
	}
	err = probeRequired(ctx, deps, opts)
	if err != nil {
		return Endpoint{}, err
	}

	localURL, err := startSuperterm(ctx, deps, opts)
	if err != nil {
		return Endpoint{}, err
	}

	tailnetURL, err := enableTailscaleServe(ctx, deps, opts, localURL)
	if err != nil {
		return Endpoint{}, err
	}

	return Endpoint{LocalURL: localURL, TailnetURL: tailnetURL, Host: opts.Host}, nil
}

// Down tears down remote control idempotently. Errors from individual teardown
// steps are reported but do not prevent other steps from running.
func Down(ctx context.Context, deps Deps, opts Options) error {
	validErr := validateProvider(opts.Provider)
	if validErr != nil {
		return validErr
	}

	var errs []string

	tsErr := runTailscaleServeOff(ctx, deps, opts)
	if tsErr != nil {
		errs = append(errs, fmt.Sprintf("tailscale serve off: %v", tsErr))
	}

	stErr := runSupertermDown(ctx, deps, opts)
	if stErr != nil {
		errs = append(errs, fmt.Sprintf("superterm down: %v", stErr))
	}

	if len(errs) > 0 {
		return fmt.Errorf("control down: %s", strings.Join(errs, "; ")) //nolint:err113 // Multi-step teardown aggregates errors as a string; no single sentinel wraps both.
	}
	return nil
}

// Status reports whether remote control is currently active.
// ok is true when both superterm and a tailscale serve mapping are running.
func Status(ctx context.Context, deps Deps, opts Options) (Endpoint, bool, error) {
	validErr := validateProvider(opts.Provider)
	if validErr != nil {
		return Endpoint{}, false, validErr
	}

	localURL, localOK := supertermStatus(ctx, deps, opts)
	tailnetURL, tsOK := tailscaleStatus(ctx, deps, opts)

	if !localOK || !tsOK {
		return Endpoint{}, false, nil
	}
	return Endpoint{LocalURL: localURL, TailnetURL: tailnetURL, Host: opts.Host}, true, nil
}

// --- internal helpers --------------------------------------------------------

func validateProvider(provider string) error {
	if provider == "" || provider == ProviderSuperterm {
		return nil
	}
	return fmt.Errorf("%w: %q (only %q is supported)", ErrProviderUnsupported, provider, ProviderSuperterm)
}

func probeRequired(ctx context.Context, deps Deps, opts Options) error {
	stErr := probeOne(ctx, deps, opts, "superterm", "--version")
	if stErr != nil {
		return ErrSupertermMissing
	}
	tsErr := probeOne(ctx, deps, opts, "tailscale", "--version")
	if tsErr != nil {
		return ErrTailscaleMissing
	}
	return nil
}

func probeOne(ctx context.Context, deps Deps, opts Options, binary string, args ...string) error {
	_, err := execCmd(ctx, deps, opts, binary, args...)
	return err
}

// execCmd runs binary on the target host (locally or via SSH).
func execCmd(ctx context.Context, deps Deps, opts Options, binary string, args ...string) ([]byte, error) {
	if opts.Host == "" {
		out, err := deps.Exec.Exec(ctx, "", binary, args...)
		if err != nil {
			return out, fmt.Errorf("exec %s: %w", binary, err)
		}
		return out, nil
	}
	// Wrap via SSH: ssh <host> <binary> <args...>
	const sshFixedArgCount = 2
	sshArgs := make([]string, 0, sshFixedArgCount+len(args))
	sshArgs = append(sshArgs, opts.Host, binary)
	sshArgs = append(sshArgs, args...)
	out, err := deps.Exec.Exec(ctx, "", "ssh", sshArgs...)
	if err != nil {
		return out, fmt.Errorf("ssh %s %s: %w", opts.Host, binary, err)
	}
	return out, nil
}

func startSuperterm(ctx context.Context, deps Deps, opts Options) (string, error) {
	out, err := execCmd(ctx, deps, opts, "superterm", "up")
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSupertermStart, err)
	}

	// Parse http://localhost:NNNN from combined output.
	loc := reLocalURL.FindString(string(out))
	if loc == "" {
		return "", fmt.Errorf("%w: superterm up output: %q", ErrUnresolvableEndpoint, string(out))
	}
	return loc, nil
}

func enableTailscaleServe(ctx context.Context, deps Deps, opts Options, localURL string) (string, error) {
	out, err := execCmd(ctx, deps, opts, "tailscale", "serve", "--bg", localURL)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrTailscaleServe, err)
	}

	// Parse https://*.ts.net URL from combined output.
	tsURL := reTailnetURL.FindString(string(out))
	if tsURL == "" {
		return "", fmt.Errorf("%w: tailscale serve output: %q", ErrUnresolvableEndpoint, string(out))
	}
	return tsURL, nil
}

func runTailscaleServeOff(ctx context.Context, deps Deps, opts Options) error {
	_, err := execCmd(ctx, deps, opts, "tailscale", "serve", "off")
	return err
}

func runSupertermDown(ctx context.Context, deps Deps, opts Options) error {
	_, err := execCmd(ctx, deps, opts, "superterm", "down")
	return err
}

func supertermStatus(ctx context.Context, deps Deps, opts Options) (string, bool) {
	out, err := execCmd(ctx, deps, opts, "superterm", "status")
	if err != nil {
		return "", false
	}
	loc := reLocalURL.FindString(string(out))
	return loc, loc != ""
}

func tailscaleStatus(ctx context.Context, deps Deps, opts Options) (string, bool) {
	out, err := execCmd(ctx, deps, opts, "tailscale", "serve", "status")
	if err != nil {
		return "", false
	}
	tsURL := reTailnetURL.FindString(string(out))
	return tsURL, tsURL != ""
}
