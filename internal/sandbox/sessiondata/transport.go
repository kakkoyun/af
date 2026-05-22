package sessiondata

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kakkoyun/af/internal/sandbox"
)

// FileEntry describes one allowlisted file inside the VM as seen by
// Inventory. Path is expressed relative to the VM user's $HOME (e.g.
// ".claude/projects/foo/abc.jsonl").
type FileEntry struct {
	// ModTime is the file mtime; zero when slicer did not report it.
	ModTime time.Time
	// Path is relative to the VM user's $HOME.
	Path string
	// Size in bytes.
	Size int64
}

// Slicer is the minimal slicer subset used by the importer.
//
// Inventory enumerates allowlisted source roots inside the VM and
// returns one FileEntry per file. Roots that do not exist inside the
// VM are silently skipped (an empty agent has no files).
//
// Copy transfers vmRelPath (a directory or file under the VM user's
// $HOME) out of vm into hostPath using whichever transport the
// implementation prefers; the contract is that after Copy returns nil
// the host-side hostPath contains a byte-identical mirror of the VM
// path (including subdirectories).
//
// vmRelPath is expressed in the same home-relative form Inventory
// returns (e.g. ".claude/projects"). hostPath is an absolute host
// filesystem path.
type Slicer interface {
	Inventory(ctx context.Context, vm string, roots []string) ([]FileEntry, error)
	Copy(ctx context.Context, vm, vmRelPath, hostPath string) error
}

var (
	// ErrInventoryFailed reports a failure inside Slicer.Inventory.
	ErrInventoryFailed = errors.New("sessiondata: slicer inventory failed")
	// ErrCopyFailed reports a failure inside Slicer.Copy.
	ErrCopyFailed = errors.New("sessiondata: slicer copy failed")
	// errMalformedInventoryLine is wrapped into ErrInventoryFailed when
	// the inventory script emits a malformed record.
	errMalformedInventoryLine = errors.New("sessiondata: malformed inventory line")
)

// ExecSlicer is the real implementation of Slicer backed by sandbox.Runner
// (the same Runner that drives slicer wt push/pull per ADR-065).
type ExecSlicer struct {
	Runner sandbox.Runner
	Binary string // defaults to "slicer"
	VMHome string // absolute path to the VM user's $HOME; defaults to "/root"
}

// inventoryScript is the POSIX shell script executed inside the VM to
// enumerate allowlisted files. Records are TAB-separated:
//
//	<relative-path>\t<size-bytes>\t<mtime-epoch-seconds>
//
// Each $root is interpreted as relative to $HOME inside the VM. Missing
// roots are skipped silently so absent agents do not abort the run.
//
// %P is GNU find's relative-to-start-point format; %s is size in bytes;
// %T@ is mtime as a floating-point seconds-since-epoch. Combined output
// lines look like ".claude/projects/foo/abc.jsonl\t1234\t1716345678.123".
const inventoryScript = `set -eu
home="${HOME:-/root}"
cd "$home"
for root in ${ROOTS}; do
	[ -d "$home/$root" ] || continue
	find "$root" -type f -printf '%P\t%s\t%T@\n' 2>/dev/null | \
		sed "s|^|$root/|"
done
`

// Inventory builds the inventory script with roots, invokes it via
// slicer vm exec, and parses the TSV output into FileEntry records.
func (e ExecSlicer) Inventory(ctx context.Context, vm string, roots []string) ([]FileEntry, error) {
	if vm == "" {
		return nil, fmt.Errorf("%w: empty vm name", ErrInventoryFailed)
	}
	if len(roots) == 0 {
		return nil, nil
	}
	script := strings.ReplaceAll(inventoryScript, "${ROOTS}", strings.Join(roots, " "))
	args := []string{"vm", "exec", vm, "--", "/bin/sh", "-c", script}
	output, err := e.runner().Run(ctx, sandbox.Command{Name: e.binary(), Args: args})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInventoryFailed, err)
	}
	entries, parseErr := parseInventoryOutput(string(output))
	if parseErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrInventoryFailed, parseErr)
	}
	return entries, nil
}

// Copy runs slicer vm cp --mode=tar to mirror $VMHome/vmRelPath
// into hostPath.
func (e ExecSlicer) Copy(ctx context.Context, vm, vmRelPath, hostPath string) error {
	if vm == "" {
		return fmt.Errorf("%w: empty vm name", ErrCopyFailed)
	}
	if vmRelPath == "" || hostPath == "" {
		return fmt.Errorf("%w: empty source or destination", ErrCopyFailed)
	}
	absVM := e.vmHome() + "/" + vmRelPath
	args := []string{"vm", "cp", "--mode=tar", vm + ":" + absVM, hostPath}
	_, err := e.runner().Run(ctx, sandbox.Command{Name: e.binary(), Args: args})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCopyFailed, err)
	}
	return nil
}

func (e ExecSlicer) binary() string {
	if e.Binary == "" {
		return "slicer"
	}
	return e.Binary
}

func (e ExecSlicer) runner() sandbox.Runner { //nolint:ireturn // Factory returns the sandbox.Runner interface; matches sandbox.NewProvider's pattern.
	if e.Runner == nil {
		return sandbox.ExecRunner{}
	}
	return e.Runner
}

func (e ExecSlicer) vmHome() string {
	if e.VMHome == "" {
		return "/root"
	}
	return e.VMHome
}

func parseInventoryOutput(out string) ([]FileEntry, error) {
	entries := make([]FileEntry, 0)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		entry, err := parseInventoryLine(line)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseInventoryLine(line string) (FileEntry, error) {
	parts := strings.SplitN(line, "\t", tsvFieldCount)
	if len(parts) != tsvFieldCount {
		return FileEntry{}, fmt.Errorf("%w: %q", errMalformedInventoryLine, line)
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return FileEntry{}, fmt.Errorf("%w: size in %q: %w", errMalformedInventoryLine, line, err)
	}
	mtime, err := parseEpochSeconds(parts[2])
	if err != nil {
		return FileEntry{}, fmt.Errorf("%w: mtime in %q: %w", errMalformedInventoryLine, line, err)
	}
	return FileEntry{Path: parts[0], Size: size, ModTime: mtime}, nil
}

// parseEpochSeconds accepts "<seconds>.<fraction>" or "<seconds>" and
// returns a UTC time. Used for GNU find's %T@ format.
func parseEpochSeconds(s string) (time.Time, error) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		secs, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse epoch %q: %w", s, err)
		}
		return time.Unix(secs, 0).UTC(), nil
	}
	secs, err := strconv.ParseInt(s[:dot], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse epoch seconds %q: %w", s, err)
	}
	frac, err := strconv.ParseFloat("0."+s[dot+1:], 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse epoch fraction %q: %w", s, err)
	}
	nanos := int64(frac * nanosPerSec)
	return time.Unix(secs, nanos).UTC(), nil
}

// FakeSlicer is an in-memory Slicer used by tests. It treats Source as
// the root of the VM's $HOME and Copy performs an actual recursive
// os-level copy from Source/vmRelPath into hostPath.
//
// FakeSlicer also records every Inventory and Copy invocation in Calls
// for assertions.
type FakeSlicer struct {
	// InventoryErr forces Inventory to return this error.
	InventoryErr error
	// CopyErr forces Copy to return this error.
	CopyErr error
	// Source is the host-side directory that simulates the VM's $HOME.
	Source string
	// Calls records the slicer invocations Inventory + Copy performed.
	Calls []FakeCall
}

// FakeCall captures one Slicer method invocation for assertions.
type FakeCall struct {
	Method   string
	VM       string
	VMPath   string
	HostPath string
	Roots    []string
}

// Inventory walks Source for each root and returns matching FileEntry
// records. Roots that do not exist are silently skipped (matching the
// real script's behaviour).
func (f *FakeSlicer) Inventory(_ context.Context, vm string, roots []string) ([]FileEntry, error) {
	f.Calls = append(f.Calls, FakeCall{Method: "Inventory", VM: vm, Roots: append([]string{}, roots...)})
	if f.InventoryErr != nil {
		return nil, fmt.Errorf("fake inventory: %w", f.InventoryErr)
	}
	entries := make([]FileEntry, 0)
	for _, root := range roots {
		abs := filepath.Join(f.Source, root)
		info, statErr := os.Stat(abs)
		if statErr != nil || !info.IsDir() {
			continue
		}
		walkErr := filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(f.Source, path)
			if relErr != nil {
				return fmt.Errorf("rel %s: %w", path, relErr)
			}
			fileInfo, infoErr := d.Info()
			if infoErr != nil {
				return fmt.Errorf("info %s: %w", path, infoErr)
			}
			entries = append(entries, FileEntry{
				Path:    filepath.ToSlash(rel),
				Size:    fileInfo.Size(),
				ModTime: fileInfo.ModTime().UTC(),
			})
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("fake inventory %s: %w", root, walkErr)
		}
	}
	return entries, nil
}

// Copy mirrors Source/vmRelPath into hostPath using a recursive os-level
// copy. Existing files at hostPath are overwritten (the merge step is
// what decides whether to keep them).
func (f *FakeSlicer) Copy(_ context.Context, vm, vmRelPath, hostPath string) error {
	f.Calls = append(f.Calls, FakeCall{Method: "Copy", VM: vm, VMPath: vmRelPath, HostPath: hostPath})
	if f.CopyErr != nil {
		return fmt.Errorf("fake copy: %w", f.CopyErr)
	}
	abs := filepath.Join(f.Source, vmRelPath)
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Mirror ExecSlicer behaviour: the inventory step has
			// already filtered missing roots, so this only fires on race.
			return nil
		}
		return fmt.Errorf("fake copy stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		return copyFile(abs, hostPath)
	}
	return copyTree(abs, hostPath)
}

// copyTree recursively copies src into dst, preserving ADR-066 perms.
func copyTree(src, dst string) error {
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return fmt.Errorf("rel %s: %w", path, relErr)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			err := os.MkdirAll(target, dirPerm)
			if err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
			return nil
		}
		return copyFile(path, target)
	})
	if walkErr != nil {
		return fmt.Errorf("copy tree %s: %w", src, walkErr)
	}
	return nil
}

func copyFile(src, dst string) error {
	err := os.MkdirAll(filepath.Dir(dst), dirPerm)
	if err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", dst, err)
	}
	data, err := os.ReadFile(src) //nolint:gosec // src is bounded to FakeSlicer.Source / staging path.
	if err != nil {
		return fmt.Errorf("read source %s: %w", src, err)
	}
	err = os.WriteFile(dst, data, filePerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
