package main

import (
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
	cmd.AddCommand(newSessionDataSyncCmd(), newSessionDataListCmd())
	return cmd
}

type sessionDataSyncOptions struct {
	agents       string
	continueHost bool
	dryRun       bool
}

func newSessionDataSyncCmd() *cobra.Command {
	var opts sessionDataSyncOptions
	cmd := &cobra.Command{
		Use:   "sync [session]",
		Short: "Sync session data out of a slicer VM and merge it into the host (ADR-067)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runSessionDataSync(cmd, name, opts)
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

func runSessionDataSync(cmd *cobra.Command, name string, opts sessionDataSyncOptions) error {
	state, statePath, err := loadSessionDataState(cmd, name)
	if err != nil {
		return fmt.Errorf("session-data sync: %w", err)
	}
	kinds, err := sessiondata.ParseKindFlag(opts.agents)
	if err != nil {
		return fmt.Errorf("session-data sync: %w", err)
	}
	if opts.continueHost {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "session-data: --continue-host is not yet implemented; importing for analysis only") //nolint:errcheck // Informational warning.
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("session-data sync: %w: %w", errSessionDataResolveHome, err)
	}

	slicer := sessiondataSlicerFactory()
	result, err := sessiondata.Sync(cmd.Context(), slicer, sessiondata.SyncOptions{
		Session:      state.Session.Name,
		VM:           state.SlicerWT.VM,
		HomeDir:      home,
		Kinds:        kinds,
		DryRun:       opts.dryRun,
		ContinueHost: opts.continueHost,
		Now:          time.Now,
	})
	if err != nil {
		return fmt.Errorf("session-data sync: %w", err)
	}

	err = withSessionLock(statePath, func() error {
		lockedErr := emitSyncEvent(state, result)
		if lockedErr != nil {
			return lockedErr
		}
		return writebackSessionExport(statePath, state, result)
	})
	if err != nil {
		return fmt.Errorf("session-data sync: %w", err)
	}
	renderSyncSummary(cmd, state, result)
	return nil
}

func runSessionDataList(cmd *cobra.Command, name string, opts sessionDataListOptions) error {
	state, _, err := loadSessionDataState(cmd, name)
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
// slicer-backed sessions. Returns the state, its on-disk path (for
// post-sync writeback), and any error.
func loadSessionDataState(cmd *cobra.Command, name string) (session.State, string, error) {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return session.State{}, "", err
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		return session.State{}, "", fmt.Errorf("read state: %w", err)
	}
	if state.SlicerWT.VM == "" {
		return session.State{}, "", fmt.Errorf("%w: %s", errSessionDataNoLease, state.Session.Name)
	}
	return state, statePath, nil
}

func emitSyncEvent(state session.State, result sessiondata.SyncResult) error {
	if result.DryRun {
		return nil
	}
	ledgerPath := filepath.Join(filepath.Dir(stateDirOf(state)), "ledger.jsonl")
	event := session.Event{
		Timestamp: time.Now().UTC(),
		Type:      "agent_sessions_synced",
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

func renderSyncSummary(cmd *cobra.Command, state session.State, result sessiondata.SyncResult) {
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

// writebackSessionExport records the result of a successful sync in
// state.toml per ADR-067 §State schema. Dry-run results are not
// written back. The state's existing SessionExport.Sources is replaced
// with the latest cursors (sync is authoritative for what is currently
// present in the staging tree; older runs are recoverable via the
// ledger if needed).
func writebackSessionExport(statePath string, state session.State, result sessiondata.SyncResult) error {
	if result.DryRun {
		return nil
	}
	// The sync itself can run for minutes; re-read under the caller's
	// lock so the writeback cannot revert fields committed by a
	// concurrent command since the pre-sync snapshot was taken.
	fresh, err := session.ReadState(statePath)
	if err != nil {
		return fmt.Errorf("reread state before writeback: %w", err)
	}
	now := time.Now().UTC()
	status := session.ExportSyncOK
	if result.Merge.Conflicts > 0 {
		status = session.ExportSyncBlocked
	}
	fresh.SessionExport = session.ExportState{
		LastSyncAt:     &now,
		LastSyncStatus: status,
		LastManifest:   result.StagingPath,
		Sources:        mapSourceRecords(state.SlicerWT.VM, result.Merge.Sources),
	}
	err = session.WriteState(statePath, fresh)
	if err != nil {
		return fmt.Errorf("writeback session_export state: %w", err)
	}
	return nil
}

// mapSourceRecords translates the package-local SourceRecord into the
// state-schema ExportSource. VM is carried over from each
// record's parent state (caller already holds state.SlicerWT.VM).
func mapSourceRecords(vm string, records []sessiondata.SourceRecord) []session.ExportSource {
	if len(records) == 0 {
		return nil
	}
	out := make([]session.ExportSource, 0, len(records))
	for i := range records {
		r := &records[i]
		out = append(out, session.ExportSource{
			Agent:      string(r.Agent),
			VM:         vm,
			SourcePath: r.VMRelPath,
			DestPath:   r.DestPath,
			Mode:       r.Mode,
			Hash:       r.Hash,
			Status:     session.ExportSourceStatus(r.Status),
			Size:       r.Size,
			LastOffset: r.LastOffset,
			MTime:      r.MTime,
		})
	}
	return out
}

// writef is a fire-and-forget formatted write used for informational
// CLI output. Errors are intentionally ignored — failing to print a
// status line should not abort the command.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...) //nolint:errcheck // Informational CLI output.
}
