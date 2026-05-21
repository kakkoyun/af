package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/control"
)

// controlExecutorFactory is the package-level seam that allows tests to inject
// a fake Executor without touching the real os/exec path.
var controlExecutorFactory = func() control.Executor { //nolint:gochecknoglobals // Test seam: replaced by tests to avoid spawning real superterm/tailscale; same pattern as prAIBodyFunc.
	return execExecutor{}
}

// execExecutor implements control.Executor via os/exec.
type execExecutor struct{}

// Exec runs name with args (in dir if non-empty) and returns combined output.
func (execExecutor) Exec(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("exec %s: %w", name, err)
	}
	return out, nil
}

// controlOptions collects the flags shared across af control sub-commands.
type controlOptions struct {
	root     *rootOptions
	remote   string
	provider string
	port     int
	jsonOut  bool
}

// endpointJSON is the JSON output schema for af control up/status.
type endpointJSON struct {
	LocalURL   string `json:"local_url"`
	TailnetURL string `json:"tailnet_url"`
	Host       string `json:"host,omitempty"`
}

// newControlCmd returns the `af control` parent command with up/down/status sub-commands.
func newControlCmd(opts *rootOptions) *cobra.Command {
	cOpts := &controlOptions{root: opts, provider: control.ProviderSuperterm}

	cmd := &cobra.Command{
		Use:   "control",
		Short: "Manage the remote-control helper (superterm + Tailscale Serve)",
		Long: "control starts, stops, or reports the status of the optional remote-control " +
			"helper that exposes a browser-accessible tmux dashboard over the owner's tailnet. " +
			"The helper composes superterm (tmux web UI) with Tailscale Serve (HTTPS exposure). " +
			"Both tools must be installed to use this command.",
	}

	// Persistent flags inherited by all sub-commands.
	pf := cmd.PersistentFlags()
	pf.StringVar(&cOpts.remote, "remote", "", "run control on this SSH host instead of locally")
	pf.StringVar(&cOpts.provider, "provider", control.ProviderSuperterm, "remote-control provider (only \"superterm\" in v1)")
	pf.BoolVar(&cOpts.jsonOut, "json", false, "emit JSON instead of human-readable output")

	cmd.AddCommand(newControlUpCmd(cOpts))
	cmd.AddCommand(newControlDownCmd(cOpts))
	cmd.AddCommand(newControlStatusCmd(cOpts))

	return cmd
}

// newControlUpCmd returns the `af control up [session]` command.
func newControlUpCmd(cOpts *controlOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "up [session]",
		Short: "Start remote control (superterm + Tailscale Serve)",
		Long: "up ensures the superterm UI is running on the target host and exposes it " +
			"through Tailscale Serve. Prints the HTTPS tailnet URL when successful.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runControlUp(cmd, cOpts)
		},
	}
	cmd.Flags().IntVar(&cOpts.port, "port", 0, "hint for the superterm listen port (0 = let superterm choose)")
	return cmd
}

// newControlDownCmd returns the `af control down` command.
func newControlDownCmd(cOpts *controlOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop remote control (remove Tailscale Serve mapping, stop superterm)",
		Long: "down removes the Tailscale Serve mapping and stops the superterm helper. " +
			"It never kills agent tmux sessions. Teardown is idempotent.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runControlDown(cmd, cOpts)
		},
	}
}

// newControlStatusCmd returns the `af control status` command.
func newControlStatusCmd(cOpts *controlOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report remote-control status",
		Long:  "status probes superterm and Tailscale Serve and prints the active endpoint URL if found.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runControlStatus(cmd, cOpts)
		},
	}
}

// --- runners -----------------------------------------------------------------

func runControlUp(cmd *cobra.Command, cOpts *controlOptions) error {
	ep, err := control.Up(cmd.Context(), control.Deps{Exec: controlExecutorFactory()}, resolveControlOpts(cOpts))
	if err != nil {
		return fmt.Errorf("control up: %w", err)
	}
	return writeEndpoint(cmd, cOpts, ep)
}

func runControlDown(cmd *cobra.Command, cOpts *controlOptions) error {
	err := control.Down(cmd.Context(), control.Deps{Exec: controlExecutorFactory()}, resolveControlOpts(cOpts))
	if err != nil {
		return fmt.Errorf("control down: %w", err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "remote control stopped")
	if err != nil {
		return fmt.Errorf("control down write: %w", err)
	}
	return nil
}

func runControlStatus(cmd *cobra.Command, cOpts *controlOptions) error {
	ep, active, err := control.Status(cmd.Context(), control.Deps{Exec: controlExecutorFactory()}, resolveControlOpts(cOpts))
	if err != nil {
		return fmt.Errorf("control status: %w", err)
	}
	if !active {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "remote control is not running")
		if err != nil {
			return fmt.Errorf("control status write: %w", err)
		}
		return nil
	}
	return writeEndpoint(cmd, cOpts, ep)
}

// --- helpers -----------------------------------------------------------------

func resolveControlOpts(cOpts *controlOptions) control.Options {
	return control.Options{
		Provider: cOpts.provider,
		Host:     cOpts.remote,
		Port:     cOpts.port,
	}
}

func writeEndpoint(cmd *cobra.Command, cOpts *controlOptions, ep control.Endpoint) error {
	if cOpts.jsonOut {
		return writeEndpointJSON(cmd, ep)
	}
	return writeEndpointText(cmd, ep)
}

func writeEndpointText(cmd *cobra.Command, ep control.Endpoint) error {
	host := ep.Host
	if host == "" {
		host = "localhost"
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(),
		"Remote control active on %s\n  Local:   %s\n  Tailnet: %s\n",
		host, ep.LocalURL, ep.TailnetURL)
	if err != nil {
		return fmt.Errorf("write endpoint: %w", err)
	}
	return nil
}

func writeEndpointJSON(cmd *cobra.Command, ep control.Endpoint) error {
	out, err := json.Marshal(endpointJSON{
		LocalURL:   ep.LocalURL,
		TailnetURL: ep.TailnetURL,
		Host:       ep.Host,
	})
	if err != nil {
		return fmt.Errorf("marshal endpoint: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", out)
	if err != nil {
		return fmt.Errorf("write endpoint json: %w", err)
	}
	return nil
}
