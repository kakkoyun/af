package sessiondata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SyncOptions configures Pull.
//
// Session is the af workstream name; it appears in staging paths and
// in any emitted ledger event. VM identifies the slicer VM. HomeDir is
// the host user's $HOME (state-import staging and merge destinations
// are rooted under it). StagingRoot overrides the default staging
// directory under HomeDir/.local/share/af/v1/session-import/<session>/
// — tests use this to avoid touching the real home directory.
//
// Now is the clock used for staging timestamps; defaults to time.Now
// when nil.
type SyncOptions struct {
	// Now overrides the clock for staging timestamps. Defaults to time.Now.
	Now func() time.Time
	// Session is the af workstream name. Required.
	Session string
	// VM is the slicer VM name returned by ADR-065's WTPush. Required.
	VM string
	// HomeDir is the host user's $HOME. Required (caller resolves it).
	HomeDir string
	// StagingRoot overrides the default staging directory; when empty
	// the staging tree lives under HomeDir/.local/share/af/v1/session-import.
	StagingRoot string
	// Kinds limits the import to specific agent kinds; nil means
	// AllKinds().
	Kinds []AgentKind
	// DryRun reports what would be copied + merged without touching
	// the host filesystem outside StagingRoot read-checks.
	DryRun bool
	// ContinueHost requests format-aware path normalization. ADR-066
	// §Host continuation. Not yet implemented; setting this prints a
	// stderr hint via the caller but does not rewrite transcripts.
	ContinueHost bool
}

// SourceStatus mirrors session.SessionExportSourceStatus values. The
// sessiondata package emits free-form strings; the CLI layer maps them
// onto the typed state constants.
type SourceStatus string

const (
	// SourceStatusOK reports a successful import.
	SourceStatusOK SourceStatus = "ok"
	// SourceStatusSkipped reports an unchanged file (hash matched).
	SourceStatusSkipped SourceStatus = "skipped"
	// SourceStatusConflict reports a quarantined divergent file.
	SourceStatusConflict SourceStatus = "conflict"
)

// SourceRecord captures one per-file outcome of a Sync. The CLI layer
// maps this onto session.SessionExportSource for state writeback per
// ADR-067 §State schema.
type SourceRecord struct {
	MTime      time.Time
	Agent      AgentKind
	VMRelPath  string
	DestPath   string
	Mode       string
	Hash       string
	Status     SourceStatus
	Size       int64
	LastOffset int64
}

// MergeReport summarises a Sync's merge step.
type MergeReport struct {
	// ConflictPaths lists host-relative paths quarantined under
	// <staging>/conflicts/; useful for surfacing in CLI output.
	ConflictPaths []string
	// Sources is the per-file cursor list, one entry per merged file
	// across every kind. Used by the CLI to populate
	// state.toml.[session_export.sources] per ADR-067.
	Sources []SourceRecord
	// Imported counts files newly written into the host destination.
	Imported int
	// Skipped counts files already present with byte-identical content.
	Skipped int
	// Conflicts counts files quarantined under the conflicts/ tree
	// because the host destination existed with different content.
	Conflicts int
}

// SyncResult is what Pull returns to callers.
type SyncResult struct {
	// Manifest is the inventory captured at the start of the pull.
	Manifest Manifest
	// StagingPath is the absolute path to the per-pull staging dir.
	StagingPath string
	// Merge is the per-kind aggregate of the merge step.
	Merge MergeReport
	// DryRun mirrors SyncOptions.DryRun for callers that print
	// dry-run-only diagnostics.
	DryRun bool
}

// ErrSyncFailed reports a non-recoverable error during Sync.
var ErrSyncFailed = errors.New("sessiondata: sync failed")

// Sync copies allowlisted session data out of the VM, stages it under
// HomeDir/.local/share/af/v1/session-import/<session>/<vm>/<ts>/, then
// merges from staging into the host agent directories per ADR-066.
//
// On DryRun, Pull stops after the manifest step and returns. No
// staging directory is created; the caller is expected to print the
// manifest summary for the user.
func Sync(ctx context.Context, s Slicer, opts SyncOptions) (SyncResult, error) {
	err := validateSyncOpts(opts)
	if err != nil {
		return SyncResult{}, err
	}
	kinds := opts.Kinds
	if len(kinds) == 0 {
		kinds = AllKinds()
	}

	manifest, err := FetchManifest(ctx, s, opts.VM, kinds)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %w", ErrSyncFailed, err)
	}
	if opts.DryRun {
		return SyncResult{Manifest: manifest, DryRun: true}, nil
	}

	stagingPath, err := makeStaging(opts)
	if err != nil {
		return SyncResult{}, err
	}

	err = stageKinds(ctx, s, opts.VM, manifest, stagingPath)
	if err != nil {
		return SyncResult{Manifest: manifest, StagingPath: stagingPath}, err
	}

	report, mergeErr := mergeStagingIntoHome(stagingPath, opts.HomeDir, manifest)
	if mergeErr != nil {
		return SyncResult{Manifest: manifest, StagingPath: stagingPath, Merge: report}, fmt.Errorf("%w: %w", ErrSyncFailed, mergeErr)
	}
	return SyncResult{Manifest: manifest, StagingPath: stagingPath, Merge: report}, nil
}

func validateSyncOpts(opts SyncOptions) error {
	switch {
	case opts.Session == "":
		return fmt.Errorf("%w: empty session name", ErrSyncFailed)
	case opts.VM == "":
		return fmt.Errorf("%w: empty vm name", ErrSyncFailed)
	case opts.HomeDir == "":
		return fmt.Errorf("%w: empty home directory", ErrSyncFailed)
	}
	return nil
}

func makeStaging(opts SyncOptions) (string, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	stamp := now().UTC().Format("20060102T150405Z")
	stagingPath := filepath.Join(opts.stagingRoot(), opts.Session, opts.VM, stamp)
	err := os.MkdirAll(stagingPath, dirPerm)
	if err != nil {
		return "", fmt.Errorf("%w: mkdir staging: %w", ErrSyncFailed, err)
	}
	return stagingPath, nil
}

func stageKinds(ctx context.Context, s Slicer, vm string, manifest Manifest, stagingPath string) error {
	for _, kind := range manifest.NonEmptyKinds() {
		err := copyKindToStaging(ctx, s, vm, kind, stagingPath)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrSyncFailed, err)
		}
	}
	return nil
}

// stagingRoot returns the resolved staging directory.
func (opts SyncOptions) stagingRoot() string {
	if opts.StagingRoot != "" {
		return opts.StagingRoot
	}
	return filepath.Join(opts.HomeDir, ".local", "share", "af", "v1", "session-import")
}

func copyKindToStaging(ctx context.Context, s Slicer, vm string, kind AgentKind, stagingPath string) error {
	for _, root := range SourceRoots(kind) {
		hostTarget := filepath.Join(stagingPath, root)
		err := os.MkdirAll(filepath.Dir(hostTarget), dirPerm)
		if err != nil {
			return fmt.Errorf("mkdir staging parent for %s: %w", root, err)
		}
		err = s.Copy(ctx, vm, root, hostTarget)
		if err != nil {
			return fmt.Errorf("copy %s: %w", root, err)
		}
	}
	return nil
}

// mergeStagingIntoHome walks stagingPath and, for each file under one
// of manifest's kinds, decides whether to import, skip, or quarantine.
//
// import: host destination does not exist → write 0o600.
// skip:   host destination exists and sha256 matches.
// conflict: host destination exists with different content → write
// under conflictsRoot(stagingPath) preserving the relative path.
func mergeStagingIntoHome(stagingPath, homeDir string, manifest Manifest) (MergeReport, error) {
	report := MergeReport{}
	conflictsRoot := filepath.Join(stagingPath, "conflicts")

	for _, kind := range manifest.NonEmptyKinds() {
		for _, root := range SourceRoots(kind) {
			err := mergeStagedRoot(stagingPath, homeDir, conflictsRoot, root, kind, &report)
			if err != nil {
				return report, err
			}
		}
	}
	if len(report.ConflictPaths) > 1 {
		sort.Strings(report.ConflictPaths)
	}
	return report, nil
}

func mergeStagedRoot(stagingPath, homeDir, conflictsRoot, root string, kind AgentKind, report *MergeReport) error {
	stagedRoot := filepath.Join(stagingPath, root)
	info, err := os.Stat(stagedRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat staged %s: %w", stagedRoot, err)
	}
	if !info.IsDir() {
		return nil
	}
	walkErr := filepath.WalkDir(stagedRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(stagingPath, path)
		if relErr != nil {
			return fmt.Errorf("rel %s: %w", path, relErr)
		}
		dest := filepath.Join(homeDir, rel)
		return mergeOneFile(path, dest, conflictsRoot, rel, kind, report)
	})
	if walkErr != nil {
		return fmt.Errorf("merge %s: %w", root, walkErr)
	}
	return nil
}

func mergeOneFile(src, dest, conflictsRoot, rel string, kind AgentKind, report *MergeReport) error {
	srcSum, err := fileSHA256(src)
	if err != nil {
		return fmt.Errorf("hash source %s: %w", src, err)
	}
	srcInfo, srcStatErr := os.Stat(src)
	if srcStatErr != nil {
		return fmt.Errorf("stat staged %s: %w", src, srcStatErr)
	}
	destInfo, destStatErr := os.Stat(dest)
	switch {
	case destStatErr != nil && errors.Is(destStatErr, fs.ErrNotExist):
		err = installFile(src, dest)
		if err != nil {
			return err
		}
		report.Imported++
		report.Sources = append(report.Sources, sourceRecord(kind, rel, dest, srcInfo, srcSum, "copy", SourceStatusOK))
		return nil
	case destStatErr != nil:
		return fmt.Errorf("stat dest %s: %w", dest, destStatErr)
	case destInfo.IsDir():
		return quarantineFile(src, conflictsRoot, rel, kind, srcInfo, srcSum, report)
	}
	destSum, err := fileSHA256(dest)
	if err != nil {
		return fmt.Errorf("hash dest %s: %w", dest, err)
	}
	if srcSum == destSum {
		report.Skipped++
		report.Sources = append(report.Sources, sourceRecord(kind, rel, dest, srcInfo, srcSum, "copy", SourceStatusSkipped))
		return nil
	}
	return quarantineFile(src, conflictsRoot, rel, kind, srcInfo, srcSum, report)
}

func sourceRecord(kind AgentKind, rel, dest string, info os.FileInfo, hash, mode string, status SourceStatus) SourceRecord {
	return SourceRecord{
		Agent:     kind,
		VMRelPath: filepath.ToSlash(rel),
		DestPath:  dest,
		Mode:      mode,
		Hash:      "sha256:" + hash,
		Status:    status,
		Size:      info.Size(),
		MTime:     info.ModTime().UTC(),
	}
}

func quarantineFile(src, conflictsRoot, rel string, kind AgentKind, info os.FileInfo, srcSum string, report *MergeReport) error {
	target := filepath.Join(conflictsRoot, rel)
	err := installFile(src, target)
	if err != nil {
		return err
	}
	report.Conflicts++
	report.ConflictPaths = append(report.ConflictPaths, rel)
	report.Sources = append(report.Sources, sourceRecord(kind, rel, target, info, srcSum, "copy", SourceStatusConflict))
	return nil
}

// installFile copies src into dest with 0o600 perms and 0o700 parent
// directories per ADR-066 §Privacy and safety. Existing dest is
// overwritten — callers check first.
func installFile(src, dest string) error {
	err := os.MkdirAll(filepath.Dir(dest), dirPerm)
	if err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", dest, err)
	}
	in, err := os.Open(src) //nolint:gosec // src is bounded to the staging tree.
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer func() { _ = in.Close() }() //nolint:errcheck // Best-effort close on a read-only source.

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm) //nolint:gosec // dest is rooted in homeDir or conflicts/.
	if err != nil {
		return fmt.Errorf("create dest %s: %w", dest, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dest, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close dest %s: %w", dest, closeErr)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is bounded to staging/home merge inputs.
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // Best-effort close on a read-only handle.
	h := sha256.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// renderManifestSummary produces a human-readable line per kind for
// callers (CLI dry-run output, ledger summaries).
func renderManifestSummary(manifest Manifest) string {
	parts := make([]string, 0, len(AllKinds()))
	for _, kind := range AllKinds() {
		count := len(manifest.Items[kind])
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", kind, count))
		}
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	return strings.Join(parts, " ")
}

// ManifestSummary is exported for the CLI to render a one-line digest.
func ManifestSummary(manifest Manifest) string { return renderManifestSummary(manifest) }
