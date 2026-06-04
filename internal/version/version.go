// Package version formats build metadata injected at link time or recorded by
// Go's VCS build stamping.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Version is the semantic version or snapshot name injected by the build.
var Version = "dev" //nolint:gochecknoglobals // Link-time -X injection requires a package variable.

// Commit is the source control revision injected by the build.
var Commit = "none" //nolint:gochecknoglobals // Link-time -X injection requires a package variable.

// Date is the build timestamp injected by the build.
var Date = "unknown" //nolint:gochecknoglobals // Link-time -X injection requires a package variable.

type buildInfo struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
	OSArch    string
	Dirty     bool
}

// String returns the user-facing af version string.
func String() string {
	return current().String()
}

func current() buildInfo {
	info := buildInfo{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		OSArch:    runtime.GOOS + "/" + runtime.GOARCH,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info = fillFromBuildInfo(info, *bi)
	}
	return info
}

func fillFromBuildInfo(info buildInfo, bi debug.BuildInfo) buildInfo {
	if isDefaultVersion(info.Version) && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	settings := map[string]string{}
	for _, setting := range bi.Settings {
		settings[setting.Key] = setting.Value
	}
	if isDefaultCommit(info.Commit) && settings["vcs.revision"] != "" {
		info.Commit = settings["vcs.revision"]
	}
	if isDefaultDate(info.Date) && settings["vcs.time"] != "" {
		info.Date = settings["vcs.time"]
	}
	info.Dirty = settings["vcs.modified"] == "true"
	return info
}

func isDefaultVersion(value string) bool {
	return value == ""
}

func isDefaultCommit(value string) bool {
	return value == "" || value == "none"
}

func isDefaultDate(value string) bool {
	return value == "" || value == "unknown"
}

func (info buildInfo) String() string {
	return fmt.Sprintf("af %s\n  commit: %s\n  date: %s\n  go: %s\n  os/arch: %s\n  dirty: %t", info.Version, info.Commit, info.Date, info.GoVersion, info.OSArch, info.Dirty)
}
