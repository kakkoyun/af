package sessiondata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNormalizeFailed reports a non-recoverable error while rewriting
// staged transcripts for ADR-066 §Host continuation mode.
var ErrNormalizeFailed = errors.New("sessiondata: continue-host normalize failed")

// NormalizeResult summarises one NormalizeForHost pass.
type NormalizeResult struct {
	// RewrittenFiles counts, per agent kind, how many staged files had
	// at least one JSON field rewritten from a vmPath-derived value to
	// a hostPath-derived value.
	RewrittenFiles map[AgentKind]int
	// RewrittenPaths lists the staging-relative paths (slash-separated,
	// post-rename for Claude) of every file that was rewritten, sorted.
	RewrittenPaths []string
	// RenamedDirs lists "<old> -> <new>" staging-relative directory
	// renames performed for agents whose host destination is keyed by a
	// workspace-path slug (Claude Code project directories), sorted.
	RenamedDirs []string
}

// NormalizeForHost rewrites the STAGED copy of a sync (the tree under
// stagingRoot, before the merge-into-host step) so host-side agent
// commands can resume the imported session against hostPath instead of
// the VM's vmPath, per ADR-066 §Host continuation mode.
//
// Per-kind behaviour:
//
//   - KindClaude: renames the staged
//     ".claude/projects/<vm-slug>" directory to
//     ".claude/projects/<host-slug>" (the Claude Code project-directory
//     slug is the workspace path with "/" and "." replaced by "-"), then
//     rewrites vmPath-derived string fields inside every *.jsonl file
//     under the renamed directory.
//   - KindCodex: rewrites vmPath-derived string fields inside every
//     *.jsonl file under ".codex/sessions" in place (no rename — Codex
//     keys sessions by date + rollout ID, not by workspace path).
//   - KindPi: pi's on-disk session-index format is not reverse-engineered
//     in this codebase. As a conservative fallback, NormalizeForHost
//     rewrites exact vmPath string occurrences inside every *.json /
//     *.jsonl file under ".pi/agent/sessions" in place.
//   - KindHarness: ADR-066 does not define a host-continuation rewrite
//     for harness roots; harness files are left untouched.
//
// The rewrite itself parses JSON generically (map[string]any / []any /
// string / number / bool / null) and recursively replaces any string
// value that equals vmPath, or has vmPath+"/" as a prefix, with the
// hostPath equivalent. Multi-line files whose whole content is one JSON
// value (pretty-printed *.json) are rewritten as a single value;
// everything else is treated as JSONL, line by line, where a line is
// rewritten only when it is one complete JSON value on its own — blank
// lines, unparsable lines, and lines with trailing bytes after a valid
// JSON prefix are passed through byte-for-byte unchanged.
// NormalizeForHost never regexes raw bytes. Unknown JSON fields
// round-trip because values are decoded generically rather than through
// a fixed struct.
//
// Normalization runs against the staging tree, before the merge/dedup
// step computes content hashes, so re-running a sync against unchanged
// VM content reproduces byte-identical normalized output and the merge
// step correctly reports the file as already imported (skipped).
//
// When vmPath or hostPath is empty, or the two are equal, NormalizeForHost
// is a no-op (nothing to rewrite) and returns a zero-value NormalizeResult.
func NormalizeForHost(stagingRoot string, kinds []AgentKind, vmPath, hostPath string) (NormalizeResult, error) {
	result := NormalizeResult{RewrittenFiles: make(map[AgentKind]int)}
	if vmPath == "" || hostPath == "" || vmPath == hostPath {
		return result, nil
	}

	for _, kind := range kinds {
		var err error
		switch kind {
		case KindClaude:
			err = normalizeClaude(stagingRoot, vmPath, hostPath, &result)
		case KindCodex:
			err = normalizeTree(stagingRoot, filepath.Join(".codex", "sessions"), KindCodex, vmPath, hostPath, &result)
		case KindPi:
			err = normalizeTree(stagingRoot, filepath.Join(".pi", "agent", "sessions"), KindPi, vmPath, hostPath, &result)
		case KindHarness:
			// No host-continuation rewrite defined for harness roots.
		}
		if err != nil {
			return result, fmt.Errorf("%w: %s: %w", ErrNormalizeFailed, kind, err)
		}
	}
	sort.Strings(result.RewrittenPaths)
	sort.Strings(result.RenamedDirs)
	return result, nil
}

// claudeProjectSlug derives the Claude Code project-directory slug from
// a workspace path: "/" and "." become "-". This mirrors the on-disk
// naming Claude Code uses under ~/.claude/projects/.
func claudeProjectSlug(path string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(path)
}

// normalizeClaude renames the staged Claude project directory from the
// VM slug to the host slug (when they differ and the VM-slug directory
// exists in staging) and rewrites *.jsonl content under it.
func normalizeClaude(stagingRoot, vmPath, hostPath string, result *NormalizeResult) error {
	projectsRoot := filepath.Join(stagingRoot, ".claude", "projects")
	info, err := os.Stat(projectsRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", projectsRoot, err)
	}
	if !info.IsDir() {
		return nil
	}

	targetDir, err := renameClaudeProjectDir(stagingRoot, projectsRoot, vmPath, hostPath, result)
	if err != nil {
		return err
	}
	return normalizeJSONLDir(stagingRoot, targetDir, KindClaude, vmPath, hostPath, result)
}

// renameClaudeProjectDir performs the vm-slug -> host-slug directory
// rename and returns the directory that should now be walked for
// content rewriting (the renamed directory, or the host-slug directory
// if no VM-slug directory was staged).
func renameClaudeProjectDir(stagingRoot, projectsRoot, vmPath, hostPath string, result *NormalizeResult) (string, error) {
	vmSlug := claudeProjectSlug(vmPath)
	hostSlug := claudeProjectSlug(hostPath)
	hostDir := filepath.Join(projectsRoot, hostSlug)
	if vmSlug == hostSlug {
		return hostDir, nil
	}

	vmDir := filepath.Join(projectsRoot, vmSlug)
	_, statErr := os.Stat(vmDir)
	switch {
	case statErr == nil:
		err := moveClaudeDir(vmDir, hostDir)
		if err != nil {
			return "", err
		}
		oldRel, err := filepath.Rel(stagingRoot, vmDir)
		if err != nil {
			return "", fmt.Errorf("rel %s: %w", vmDir, err)
		}
		newRel, err := filepath.Rel(stagingRoot, hostDir)
		if err != nil {
			return "", fmt.Errorf("rel %s: %w", hostDir, err)
		}
		result.RenamedDirs = append(result.RenamedDirs, filepath.ToSlash(oldRel)+" -> "+filepath.ToSlash(newRel))
		return hostDir, nil
	case errors.Is(statErr, fs.ErrNotExist):
		return hostDir, nil
	default:
		return "", fmt.Errorf("stat %s: %w", vmDir, statErr)
	}
}

// moveClaudeDir renames vmDir to hostDir. If hostDir already exists as
// a directory (e.g. a same-second staging timestamp re-runs Sync
// against the same VM content, leaving a prior pass's host-slug
// directory in place), a bare os.Rename fails with ENOTEMPTY/EEXIST; in
// that case the VM-slug directory's files are merged into hostDir
// file-by-file instead of aborting the whole sync.
func moveClaudeDir(vmDir, hostDir string) error {
	err := os.Rename(vmDir, hostDir)
	if err == nil {
		return nil
	}
	hostInfo, hostStatErr := os.Stat(hostDir)
	if hostStatErr != nil || !hostInfo.IsDir() {
		return fmt.Errorf("rename %s to %s: %w", vmDir, hostDir, err)
	}
	mergeErr := mergeDirContents(vmDir, hostDir)
	if mergeErr != nil {
		return fmt.Errorf("merge %s into existing %s: %w", vmDir, hostDir, mergeErr)
	}
	return nil
}

// mergeDirContents moves every file under src into the matching
// relative path under dst (creating parent directories as needed),
// overwriting any same-path file already in dst, then removes src.
// Used only as the moveClaudeDir fallback, where src and dst are known
// to be re-staged copies of the same VM content.
func mergeDirContents(src, dst string) error {
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return fmt.Errorf("rel %s: %w", path, relErr)
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		return os.Rename(path, target)
	})
	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", src, walkErr)
	}
	err := os.RemoveAll(src)
	if err != nil {
		return fmt.Errorf("remove %s: %w", src, err)
	}
	return nil
}

// normalizeTree walks stagingRoot/relRoot and rewrites every *.json /
// *.jsonl file found (case-insensitive) using the generic vmPath ->
// hostPath string rewrite. Used for Codex and pi, neither of which key
// their staged directory layout by workspace-path slug.
func normalizeTree(stagingRoot, relRoot string, kind AgentKind, vmPath, hostPath string, result *NormalizeResult) error {
	root := filepath.Join(stagingRoot, relRoot)
	return normalizeJSONLDir(stagingRoot, root, kind, vmPath, hostPath, result)
}

// normalizeJSONLDir walks dir (skipping entirely when absent) and
// rewrites every *.json / *.jsonl file, recording per-kind counts and
// staging-relative paths in result for files that actually changed.
func normalizeJSONLDir(stagingRoot, dir string, kind AgentKind, vmPath, hostPath string, result *NormalizeResult) error {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil
	}

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, entryErr error) error {
		if entryErr != nil {
			return entryErr
		}
		if d.IsDir() || !isNormalizableFile(path) {
			return nil
		}
		return normalizeCandidateFile(stagingRoot, path, kind, vmPath, hostPath, result)
	})
	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", dir, walkErr)
	}
	return nil
}

// normalizeCandidateFile rewrites one candidate file found by
// normalizeJSONLDir's walk and, when it changed, records its
// staging-relative path and bumps the per-kind counter in result.
func normalizeCandidateFile(stagingRoot, path string, kind AgentKind, vmPath, hostPath string, result *NormalizeResult) error {
	changed, err := rewriteJSONLFile(path, vmPath, hostPath)
	if err != nil {
		return fmt.Errorf("rewrite %s: %w", path, err)
	}
	if !changed {
		return nil
	}
	rel, err := filepath.Rel(stagingRoot, path)
	if err != nil {
		return fmt.Errorf("rel %s: %w", path, err)
	}
	result.RewrittenFiles[kind]++
	result.RewrittenPaths = append(result.RewrittenPaths, filepath.ToSlash(rel))
	return nil
}

// isNormalizableFile reports whether path is a candidate for JSON-line
// rewriting: files ending in .json or .jsonl (case-insensitive).
func isNormalizableFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".jsonl") || strings.HasSuffix(lower, ".json")
}

// rewriteJSONLFile rewrites path in place, applying the generic vmPath
// -> hostPath string rewrite. Multi-line content that parses as a
// single JSON value (pretty-printed *.json, as pi writes) is rewritten
// as one value; everything else goes through the JSONL line-by-line
// path, where lines that are blank or fail to parse are passed through
// byte-for-byte. Returns (true, nil) when content changed and the file
// was rewritten; (false, nil) when nothing changed (the file is left
// untouched, including its mtime).
func rewriteJSONLFile(path, vmPath, hostPath string) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is bounded to the staging tree.
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	out, changed := rewriteJSONContent(data, vmPath, hostPath)
	if !changed {
		return false, nil
	}
	err = os.WriteFile(path, out, filePerm)
	if err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// rewriteJSONContent applies the vmPath -> hostPath rewrite to one
// file's bytes and reports whether anything changed. The whole-file
// branch must run before the line branch: a pretty-printed JSON value
// contains lines (bare strings, numbers) that parse as JSON on their
// own, and rewriting those individually would corrupt the file — part
// rewritten, part stale, indentation lost.
func rewriteJSONContent(data []byte, vmPath, hostPath string) ([]byte, bool) {
	trailingNewline := len(data) > 0 && data[len(data)-1] == '\n'
	body := data
	if trailingNewline {
		body = data[:len(data)-1]
	}

	out, changed, handled := rewriteWholeFileJSON(body, vmPath, hostPath)
	if !handled {
		out, changed = rewriteJSONLines(body, vmPath, hostPath)
	}
	if !changed {
		return data, false
	}
	if trailingNewline {
		out = append(out, '\n')
	}
	return out, true
}

// rewriteJSONLines is the JSONL branch of rewriteJSONContent: body is
// split on newlines and each line rewritten independently via
// rewriteJSONLine. Returns (nil, false) when no line changed.
func rewriteJSONLines(body []byte, vmPath, hostPath string) ([]byte, bool) {
	var lines [][]byte
	if len(body) > 0 {
		lines = bytes.Split(body, []byte("\n"))
	}
	changedAny := false
	for i, line := range lines {
		newLine, changed := rewriteJSONLine(line, vmPath, hostPath)
		if changed {
			lines[i] = newLine
			changedAny = true
		}
	}
	if !changedAny {
		return nil, false
	}
	return bytes.Join(lines, []byte("\n")), true
}

// rewriteWholeFileJSON handles files whose whole content is a single
// JSON value spanning multiple lines (pi writes *.json pretty-printed).
// Results are (out, changed, handled): handled=false hands the content
// to the line-by-line JSONL path — single-line files always go there
// (same rewrite, and the line's compact encoding is preserved), as does
// anything that is not one valid JSON value. A changed value is
// re-encoded with two-space indentation, which stays resumable JSON
// even when it does not match the writer's original style.
func rewriteWholeFileJSON(body []byte, vmPath, hostPath string) ([]byte, bool, bool) {
	trimmed := bytes.TrimSpace(body)
	if !bytes.ContainsRune(trimmed, '\n') || !json.Valid(trimmed) {
		return nil, false, false
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var value any
	err := dec.Decode(&value)
	if err != nil {
		return nil, false, false
	}

	newValue, valueChanged := rewriteJSONValue(value, vmPath, hostPath)
	if !valueChanged {
		return nil, false, true
	}
	marshaled, marshalErr := json.MarshalIndent(newValue, "", "  ")
	if marshalErr != nil {
		// Defensive: a value decoded by encoding/json always re-marshals.
		return nil, false, false
	}
	return marshaled, true, true
}

// rewriteJSONLine decodes one JSONL line generically and applies the
// vmPath -> hostPath rewrite. The second return mirrors "this line's
// content changed"; non-JSON lines return the original bytes and
// changed=false so the caller passes them through untouched (the
// documented fallback for lines NormalizeForHost does not understand).
func rewriteJSONLine(line []byte, vmPath, hostPath string) ([]byte, bool) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return line, false
	}
	// The whole line must be one valid JSON value: Decoder.Decode alone
	// would accept `{"a":1} trailing` and re-marshaling would then drop
	// the trailing bytes, breaking the byte-for-byte round-trip
	// guarantee for unknown content.
	if !json.Valid(trimmed) {
		return line, false
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var value any
	err := dec.Decode(&value)
	if err != nil {
		return line, false
	}

	newValue, valueChanged := rewriteJSONValue(value, vmPath, hostPath)
	if !valueChanged {
		return line, false
	}

	marshaled, marshalErr := json.Marshal(newValue)
	if marshalErr != nil {
		// Defensive: a value decoded by encoding/json always re-marshals.
		return line, false
	}
	return marshaled, true
}

// rewriteJSONValue recursively rewrites string values equal to, or
// prefixed by, vmPath (with a "/" boundary) into their hostPath
// equivalent. Maps and slices are rewritten in place and returned;
// every other JSON type (number, bool, null) round-trips unchanged.
func rewriteJSONValue(v any, vmPath, hostPath string) (any, bool) {
	switch val := v.(type) {
	case string:
		return rewritePathString(val, vmPath, hostPath)
	case map[string]any:
		changedAny := false
		for k, child := range val {
			newChild, changed := rewriteJSONValue(child, vmPath, hostPath)
			if changed {
				val[k] = newChild
				changedAny = true
			}
		}
		return val, changedAny
	case []any:
		changedAny := false
		for i, child := range val {
			newChild, changed := rewriteJSONValue(child, vmPath, hostPath)
			if changed {
				val[i] = newChild
				changedAny = true
			}
		}
		return val, changedAny
	default:
		return v, false
	}
}

// rewritePathString rewrites s when it equals vmPath or starts with
// vmPath followed by "/" (a child path beneath the VM workspace root),
// replacing the vmPath prefix with hostPath.
func rewritePathString(s, vmPath, hostPath string) (string, bool) {
	if s == vmPath {
		return hostPath, true
	}
	if strings.HasPrefix(s, vmPath+"/") {
		return hostPath + s[len(vmPath):], true
	}
	return s, false
}

// NormalizePreview summarises, for `--dry-run --continue-host`, the
// per-kind candidate files a real (non-dry-run) ContinueHost sync would
// inspect for path rewriting. Per ADR-066's CLI surface, --dry-run never
// copies VM content, so NormalizeForHost cannot actually be run — the
// counts below are upper bounds computed from the manifest alone
// (allowlisted-root + extension match), not confirmed rewrites. A live
// `--continue-host` sync may report fewer RewrittenFiles per kind in its
// NormalizeResult than CandidateFiles reports here, when a candidate
// file's paths never referenced vmPath.
type NormalizePreview struct {
	// CandidateFiles counts, per agent kind, manifest entries that a
	// live sync's NormalizeForHost pass would attempt to parse.
	CandidateFiles map[AgentKind]int
	// CandidatePaths lists the VM-relative paths of every candidate
	// file, sorted.
	CandidatePaths []string
}

// CandidateNormalizeCounts computes NormalizePreview from manifest
// without touching the filesystem or the VM, for `--dry-run
// --continue-host` reporting. Only kinds with a defined
// NormalizeForHost rewriter (claude, codex, pi) ever contribute
// candidates; harness never does.
func CandidateNormalizeCounts(manifest Manifest, kinds []AgentKind) NormalizePreview {
	preview := NormalizePreview{CandidateFiles: make(map[AgentKind]int)}
	for _, kind := range kinds {
		if kind == KindHarness {
			continue
		}
		for _, entry := range manifest.Items[kind] {
			if !isNormalizableFile(entry.Path) {
				continue
			}
			preview.CandidateFiles[kind]++
			preview.CandidatePaths = append(preview.CandidatePaths, entry.Path)
		}
	}
	sort.Strings(preview.CandidatePaths)
	return preview
}
