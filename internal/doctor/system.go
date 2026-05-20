package doctor

import (
	"bufio"
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// SystemLookup resolves binaries via os/exec.LookPath and runs
// `<binary> --version` to capture a version line.
type SystemLookup struct{}

// LookPath wraps exec.LookPath, returning the resolved path and true
// when binary is present in PATH. ctx is accepted to match the Lookup
// interface but exec.LookPath has no cancellation surface.
func (SystemLookup) LookPath(_ context.Context, name string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

// Version invokes `binary --version` and returns the trimmed first line
// of output. It returns an empty string on any failure (best-effort).
func (SystemLookup) Version(ctx context.Context, binary string) string {
	cmd := exec.CommandContext(ctx, binary, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

// DetectPlatform returns the local Platform by inspecting GOOS and, on
// Linux, /etc/os-release via OSReleaseReader.
func DetectPlatform(read OSReleaseReader) Platform {
	if runtime.GOOS == "darwin" {
		return PlatformMacOS
	}
	if runtime.GOOS != "linux" {
		return PlatformOther
	}
	if read == nil {
		return PlatformOther
	}
	osRelease, err := read.Read()
	if err != nil {
		return PlatformOther
	}
	return classifyLinux(osRelease)
}

// OSReleaseReader reads /etc/os-release (or an equivalent) and returns
// the parsed key=value map.
type OSReleaseReader interface {
	Read() (map[string]string, error)
}

func classifyLinux(osRelease map[string]string) Platform {
	id := strings.ToLower(osRelease["ID"])
	idLike := strings.ToLower(osRelease["ID_LIKE"])
	switch {
	case id == "arch" || id == "manjaro" || strings.Contains(idLike, "arch"):
		return PlatformArch
	case id == "debian" || id == "ubuntu" || strings.Contains(idLike, "debian"):
		return PlatformDebian
	default:
		return PlatformOther
	}
}

// ParseOSRelease parses /etc/os-release content (KEY="value" lines) into
// a map. Unquoted values are accepted; comments and blank lines are
// skipped.
func ParseOSRelease(content string) map[string]string {
	out := make(map[string]string)
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		value = strings.Trim(value, "\"'")
		out[key] = value
	}
	return out
}
