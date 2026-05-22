package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/sandbox/sessiondata"
	"github.com/kakkoyun/af/internal/session"
)

// sessiondataSlicerFactory builds the Slicer used by the session-data
// CLI. Tests replace it with a FakeSlicer; production wires ExecSlicer
// over sandbox.ExecRunner.
//
//nolint:gochecknoglobals // Test seam: replaced in tests; same pattern as controlExecutorFactory in cmd/af/control.go.
var sessiondataSlicerFactory = func() sessiondata.Slicer { return sessiondata.ExecSlicer{Runner: sandbox.ExecRunner{}} }

var (
	errSessionDataNoLease     = errors.New("session-data: workstream is not slicer-backed")
	errSessionDataResolveHome = errors.New("session-data: cannot resolve $HOME")
)

func newSessionDataCmd(_ *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session-data",
		Short: "Import agent and harness session data from slicer VMs (ADR-066)",
		Long: "session-data harvests agent transcripts and harness session metadata from a slicer VM and " +
			"merges them into the host-side agent directories. Used before VM teardown so VM-only " +
			"conversation history survives suspend/done.",
	}
	cmd.AddCommand(newSessionDataPullCmd(), newSessionDataListCmd())
	return cmd
}

type sessionDataPullOptions struct {
	agents       string
	continueHost bool
	dryRun       bool
}

func newSessionDataPullCmd() *cobra.Command {
	var opts sessionDataPullOptions
	cmd := &cobra.Command{
		Use:   "pull [session]",
		Short: "Copy session data out of a slicer VM and merge it into the host",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runSessionDataPull(cmd, name, opts)
		},
	}
	cmd.Flags().StringVar(&opts.agents, "agent", "all", "comma-separated agent kinds (claude,codex,pi,harness) or 'all'")
	cmd.Flags().BoolVar(&opts.continueHost, "continue-host", false, "request host-continuation path normalization (ADR-066 §Host continuation; not yet implemented)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "print the manifest without copying")
	return cmd
}

type sessionDataListOptions struct {
	agents string
	vm     string
}

func newSessionDataListCmd() *cobra.Command {
	var opts sessionDataListOptions
	cmd := &cobra.Command{
		Use:   "list [session]",
		Short: "Inventory the allowlisted session files inside a slicer VM",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runSessionDataList(cmd, name, opts)
		},
	}
	cmd.Flags().StringVar(&opts.agents, "agent", "all", "comma-separated agent kinds or 'all'")
	cmd.Flags().StringVar(&opts.vm, "vm", "", "override the VM name (defaults to state.SlicerWT.VM)")
	return cmd
}

func runSessionDataPull(cmd *cobra.Command, name string, opts sessionDataPullOptions) error {
	state, err := loadSessionDataState(cmd.Context(), name)
	if err != nil {
		return fmt.Errorf("session-data pull: %w", err)
	}
	kinds, err := sessiondata.ParseKindFlag(opts.agents)
	if err != nil {
		return fmt.Errorf("session-data pull: %w", err)
	}
	if opts.continueHost {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "session-data: --continue-host is not yet implemented; importing for analysis only") //nolint:errcheck // Informational warning.
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("session-data pull: %w: %w", errSessionDataResolveHome, err)
	}

	slicer := sessiondataSlicerFactory()
	result, err := sessiondata.Pull(cmd.Context(), slicer, sessiondata.PullOptions{
		Session:      state.Session.Name,
		VM:           state.SlicerWT.VM,
		HomeDir:      home,
		Kinds:        kinds,
		DryRun:       opts.dryRun,
		ContinueHost: opts.continueHost,
		Now:          time.Now,
	})
	if err != nil {
		return fmt.Errorf("session-data pull: %w", err)
	}

	err = emitPullEvent(state, result)
	if err != nil {
		return fmt.Errorf("session-data pull: %w", err)
	}
	renderPullSummary(cmd, state, result)
	return nil
}

func runSessionDataList(cmd *cobra.Command, name string, opts sessionDataListOptions) error {
	state, err := loadSessionDataState(cmd.Context(), name)
	if err != nil {
		return fmt.Errorf("session-data list: %w", err)
	}
	vm := opts.vm
	if vm == "" {
		vm = state.SlicerWT.VM
	}
	if vm == "" {
		return fmt.Errorf("session-data list: %w", errSessionDataNoLease)
	}
	kinds, err := sessiondata.ParseKindFlag(opts.agents)
	if err != nil {
		return fmt.Errorf("session-data list: %w", err)
	}
	slicer := sessiondataSlicerFactory()
	manifest, err := sessiondata.List(cmd.Context(), slicer, vm, kinds)
	if err != nil {
		return fmt.Errorf("session-data list: %w", err)
	}
	renderListSummary(cmd, manifest)
	return nil
}

// loadSessionDataState resolves the workstream state and verifies the
// session is backed by a slicer VM. session-data only makes sense for
// slicer-backed sessions.
func loadSessionDataState(_ context.Context, name string) (session.State, error) {
	statePath, err := resolveLifecycleStatePath(name)
	if err != nil {
		return session.State{}, err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, fmt.Errorf("read state: %w", err)
	}
	if state.SlicerWT.VM == "" {
		return session.State{}, fmt.Errorf("%w: %s", errSessionDataNoLease, state.Session.Name)
	}
	return state, nil
}

func emitPullEvent(state session.State, result sessiondata.PullResult) error {
	if result.DryRun {
		return nil
	}
	ledgerPath := filepath.Join(filepath.Dir(stateDirOf(state)), "ledger.jsonl")
	event := session.Event{
		Timestamp: time.Now().UTC(),
		Type:      "agent_sessions_pulled",
		Fields: map[string]any{
			"session":      state.Session.Name,
			"vm":           state.SlicerWT.VM,
			"kinds":        kindNames(result.Manifest),
			"imported":     result.Merge.Imported,
			"skipped":      result.Merge.Skipped,
			"conflicts":    result.Merge.Conflicts,
			"staging":      result.StagingPath,
			"continueHost": false,
		},
	}
	err := session.AppendEvent(ledgerPath, event)
	if err != nil {
		return fmt.Errorf("append agent_sessions_pulled event: %w", err)
	}
	return nil
}

// stateDirOf returns the directory containing state.toml + ledger.jsonl
// for the given state. The state itself does not record its path, so
// we recompute from the session name.
func stateDirOf(state session.State) string {
	dir, err := defaultSessionsDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, state.Session.Name, "state.toml")
}

func kindNames(manifest sessiondata.Manifest) []string {
	kinds := manifest.NonEmptyKinds()
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, string(k))
	}
	return out
}

func renderPullSummary(cmd *cobra.Command, state session.State, result sessiondata.PullResult) {
	if result.DryRun {
		writef(cmd.OutOrStdout(),
			"session-data: %s: dry-run on VM %s — manifest: %s\n",
			state.Session.Name, state.SlicerWT.VM, sessiondata.ManifestSummary(result.Manifest))
		return
	}
	writef(cmd.OutOrStdout(),
		"session-data: %s: imported %d, skipped %d, conflicts %d (staging=%s)\n",
		state.Session.Name, result.Merge.Imported, result.Merge.Skipped, result.Merge.Conflicts, result.StagingPath)
	if result.Merge.Conflicts > 0 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "session-data: conflicts quarantined under <staging>/conflicts/:") //nolint:errcheck // Informational note.
		for _, path := range result.Merge.ConflictPaths {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", path) //nolint:errcheck // Informational note.
		}
	}
}

func renderListSummary(cmd *cobra.Command, manifest sessiondata.Manifest) {
	writef(cmd.OutOrStdout(), "session-data: vm=%s — %s\n", manifest.VM, sessiondata.ManifestSummary(manifest))
	for _, kind := range manifest.NonEmptyKinds() {
		writef(cmd.OutOrStdout(), "  [%s]\n", kind)
		for _, entry := range manifest.Items[kind] {
			writef(cmd.OutOrStdout(), "    %s (%d bytes)\n", entry.Path, entry.Size)
		}
	}
}

// writef is a fire-and-forget formatted write used for informational
// CLI output. Errors are intentionally ignored — failing to print a
// status line should not abort the command.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...) //nolint:errcheck // Informational CLI output.
}
