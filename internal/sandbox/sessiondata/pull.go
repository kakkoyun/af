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
	// VMPath is the VM-side workspace path recorded in transcripts
	// (e.g. a Claude "cwd" field) that ContinueHost normalization
	// rewrites away. Required when ContinueHost is true; ignored
	// otherwise.
	VMPath string
	// HostPath is the host-side workspace path that VMPath references
	// are rewritten to when ContinueHost is true. Required when
	// ContinueHost is true; ignored otherwise.
	HostPath string
	// Kinds limits the import to specific agent kinds; nil means
	// AllKinds().
	Kinds []AgentKind
	// DryRun reports what would be copied + merged without touching
	// the host filesystem outside StagingRoot read-checks.
	DryRun bool
	// ContinueHost requests format-aware path normalization per
	// ADR-066 §Host continuation mode. When true, Sync runs
	// NormalizeForHost against the staging tree (using VMPath and
	// HostPath) before merging into the host agent directories. On
	// DryRun, no staging or normalization occurs; instead Sync reports
	// CandidateNormalizeCounts computed from the manifest alone.
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
	// Normalize reports the ContinueHost rewrite pass performed against
	// the staging tree before merge. Zero-value when ContinueHost was
	// false or DryRun was true.
	Normalize NormalizeResult
	// NormalizePreview reports ContinueHost candidate counts computed
	// from the manifest when both DryRun and ContinueHost were set.
	// Zero-value otherwise.
	NormalizePreview NormalizePreview
	// Merge is the per-kind aggregate of the merge step.
	Merge MergeReport
	// DryRun mirrors SyncOptions.DryRun for callers that print
	// dry-run-only diagnostics.
	DryRun bool
}

// ErrSyncFailed reports a non-recoverable error during Sync.
var ErrSyncFailed = errors.New("sessiondata: sync failed")

// errShortAppend is wrapped into the append-tail failure path when
// io.CopyN writes fewer bytes than expected.
var errShortAppend = errors.New("sessiondata: short append")

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
		return dryRunSyncResult(manifest, kinds, opts), nil
	}

	stagingPath, err := makeStaging(opts)
	if err != nil {
		return SyncResult{}, err
	}

	err = stageKinds(ctx, s, opts.VM, manifest, stagingPath)
	if err != nil {
		return SyncResult{Manifest: manifest, StagingPath: stagingPath}, err
	}

	normResult, err := normalizeIfContinueHost(stagingPath, kinds, opts)
	if err != nil {
		return SyncResult{Manifest: manifest, StagingPath: stagingPath}, fmt.Errorf("%w: %w", ErrSyncFailed, err)
	}

	report, mergeErr := mergeStagingIntoHome(stagingPath, opts.HomeDir, manifest)
	if mergeErr != nil {
		return SyncResult{Manifest: manifest, StagingPath: stagingPath, Merge: report, Normalize: normResult}, fmt.Errorf("%w: %w", ErrSyncFailed, mergeErr)
	}
	return SyncResult{Manifest: manifest, StagingPath: stagingPath, Merge: report, Normalize: normResult}, nil
}

// dryRunSyncResult builds the SyncResult returned by Sync's DryRun path.
// Per ADR-066, dry-run never copies VM content, so a requested
// ContinueHost normalization is reported as a manifest-only
// NormalizePreview (candidate counts) rather than an actual
// NormalizeResult.
func dryRunSyncResult(manifest Manifest, kinds []AgentKind, opts SyncOptions) SyncResult {
	result := SyncResult{Manifest: manifest, DryRun: true}
	if opts.ContinueHost {
		result.NormalizePreview = CandidateNormalizeCounts(manifest, kinds)
	}
	return result
}

// normalizeIfContinueHost runs NormalizeForHost against the staging
// tree when opts.ContinueHost is set, before the merge/dedup step reads
// the staged files' content hashes. Returns a zero-value NormalizeResult
// when ContinueHost is false.
func normalizeIfContinueHost(stagingPath string, kinds []AgentKind, opts SyncOptions) (NormalizeResult, error) {
	if !opts.ContinueHost {
		return NormalizeResult{}, nil
	}
	return NormalizeForHost(stagingPath, kinds, opts.VMPath, opts.HostPath)
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

func mergeOneFile(src, dest, conflictsRoot, rel string, kind AgentKind, report *MergeReport) error { //nolint:cyclop // Cleanly handling new/skipped/conflict + JSONL append in one function reads better than 4 helpers.
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
	handled, err := tryAppendJSONLMerge(src, dest, rel, kind, srcInfo, destInfo.Size(), report)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	return quarantineFile(src, conflictsRoot, rel, kind, srcInfo, srcSum, report)
}

// tryAppendJSONLMerge attempts the ADR-067 append-aware merge for
// *.jsonl files when dest is a byte-prefix of src. Returns
// (handled=true, nil) when the merge succeeded; (false, nil) when the
// file is not a candidate (non-jsonl) or dest is not a prefix; (_, err)
// on IO error. On success the SourceRecord is appended to report.
func tryAppendJSONLMerge(src, dest, rel string, kind AgentKind, srcInfo os.FileInfo, destSize int64, report *MergeReport) (bool, error) {
	if !isJSONLFile(src) {
		return false, nil
	}
	ok, offset, err := tryAppendJSONLTail(src, dest, destSize)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	sum, hashErr := fileSHA256(dest)
	if hashErr != nil {
		return false, fmt.Errorf("hash dest after append %s: %w", dest, hashErr)
	}
	report.Imported++
	rec := sourceRecord(kind, rel, dest, srcInfo, sum, "append-jsonl", SourceStatusOK)
	rec.LastOffset = offset
	report.Sources = append(report.Sources, rec)
	return true, nil
}

// isJSONLFile returns true when path ends in .jsonl. The append-aware
// merge path is gated on this extension because non-JSONL files have
// no append semantics that af understands.
func isJSONLFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".jsonl")
}

// tryAppendJSONLTail attempts the prefix-append merge per ADR-067:
// if dest's existing bytes are a byte-for-byte prefix of src, append
// only the missing tail (src[destSize:]) to dest and report (true, destSize, nil).
// If dest is not a prefix of src (divergence), (false, 0, nil) is
// returned and the caller falls back to quarantine. IO errors return
// (false, 0, err).
func tryAppendJSONLTail(src, dest string, destSize int64) (bool, int64, error) {
	srcFile, err := os.Open(src) //nolint:gosec // src is bounded to staging.
	if err != nil {
		return false, 0, fmt.Errorf("open staged %s: %w", src, err)
	}
	defer func() { _ = srcFile.Close() }() //nolint:errcheck // Best-effort close on a read-only source.

	srcInfo, statErr := srcFile.Stat()
	if statErr != nil {
		return false, 0, fmt.Errorf("stat staged %s: %w", src, statErr)
	}
	// If src is smaller than dest, dest cannot be a prefix; quarantine.
	if srcInfo.Size() < destSize {
		return false, 0, nil
	}
	prefix := make([]byte, destSize)
	_, err = io.ReadFull(srcFile, prefix)
	if err != nil {
		return false, 0, fmt.Errorf("read src prefix %s: %w", src, err)
	}

	destBytes, err := os.ReadFile(dest) //nolint:gosec // dest is rooted in host home / conflicts.
	if err != nil {
		return false, 0, fmt.Errorf("read dest %s: %w", dest, err)
	}
	if !bytesEqual(prefix, destBytes) {
		return false, 0, nil
	}

	// Atomically append the tail.
	err = appendTail(srcFile, dest, srcInfo.Size()-destSize)
	if err != nil {
		return false, 0, err
	}
	return true, destSize, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// appendTail copies remaining bytes from srcFile (already advanced
// past the verified prefix) onto dest using append+sync. The dest
// file is opened with O_APPEND|O_WRONLY; the destination's existing
// content is left untouched.
func appendTail(srcFile io.Reader, dest string, tailLen int64) error {
	out, err := os.OpenFile(dest, os.O_APPEND|os.O_WRONLY, filePerm) //nolint:gosec // dest is rooted in host home.
	if err != nil {
		return fmt.Errorf("open dest for append %s: %w", dest, err)
	}
	n, copyErr := io.CopyN(out, srcFile, tailLen)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("append tail to %s: %w", dest, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close dest after append %s: %w", dest, closeErr)
	}
	if n != tailLen {
		return fmt.Errorf("%w: %s: wrote %d of %d bytes", errShortAppend, dest, n, tailLen)
	}
	return nil
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
