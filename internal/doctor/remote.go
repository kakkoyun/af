package doctor

import (
	"context"
	"fmt"
	"strings"
)

// RemoteCommander runs a remote shell command and returns its combined
// stdout/stderr output. The remote package's SSH type satisfies this
// seam; tests use a fake.
type RemoteCommander interface {
	Run(ctx context.Context, command string) ([]byte, error)
}

// RemoteLookup probes binaries on a remote host via a RemoteCommander.
//
// It uses `command -v` to detect presence (POSIX-portable) and
// `<binary> --version` for version detection. Both calls are wrapped
// with `|| true` so unknown binaries do not surface as ssh errors.
type RemoteLookup struct {
	Commander RemoteCommander
}

// LookPath returns the resolved remote path for name. It returns
// ("", false) when the binary is not on the remote PATH or when
// `command -v` fails for any other reason.
func (r RemoteLookup) LookPath(ctx context.Context, name string) (string, bool) {
	out, err := r.Commander.Run(ctx, "command -v "+shellEscape(name)+" 2>/dev/null || true")
	if err != nil {
		return "", false
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false
	}
	return path, true
}

// Version invokes `<binary> --version` on the remote and returns the
// first non-empty line, or an empty string on failure.
func (r RemoteLookup) Version(ctx context.Context, binary string) string {
	out, err := r.Commander.Run(ctx, shellEscape(binary)+" --version 2>&1 || true")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// RemoteOSRelease reads /etc/os-release from the remote host through a
// RemoteCommander.
type RemoteOSRelease struct {
	Ctx       context.Context //nolint:containedctx // OSReleaseReader signature is ctx-free; we capture it from the constructor.
	Commander RemoteCommander
}

// Read fetches /etc/os-release from the remote and parses it.
func (r RemoteOSRelease) Read() (map[string]string, error) {
	out, err := r.Commander.Run(r.Ctx, "cat /etc/os-release 2>/dev/null || true")
	if err != nil {
		return nil, fmt.Errorf("remote /etc/os-release: %w", err)
	}
	return ParseOSRelease(string(out)), nil
}

// DetectRemotePlatform fetches /etc/os-release from the remote host and
// classifies the platform. ctx is used for both the uname probe and the
// os-release fetch.
func DetectRemotePlatform(ctx context.Context, commander RemoteCommander) Platform {
	if commander == nil {
		return PlatformOther
	}
	out, err := commander.Run(ctx, "uname -s")
	if err == nil && strings.TrimSpace(string(out)) == "Darwin" {
		return PlatformMacOS
	}
	osRelease, err := RemoteOSRelease{Ctx: ctx, Commander: commander}.Read()
	if err != nil {
		return PlatformOther
	}
	return classifyLinux(osRelease)
}

// shellEscape returns arg as a POSIX-shell-safe word. Safe ASCII words
// pass through unchanged; everything else is single-quoted with any
// internal single quotes escaped via the standard '\” trick.
func shellEscape(arg string) string {
	if arg == "" {
		return "''"
	}
	if isSafeShellWord(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func isSafeShellWord(s string) bool {
	for _, r := range s {
		if !isSafeShellRune(r) {
			return false
		}
	}
	return true
}

const safeShellSymbols = "-_./"

func isSafeShellRune(r rune) bool {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
		return true
	}
	return strings.ContainsRune(safeShellSymbols, r)
}
