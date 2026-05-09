// Package version formats build metadata injected at link time.
package version

import "fmt"

// Version is the semantic version or snapshot name injected by the build.
var Version = "dev"

// Commit is the source control revision injected by the build.
var Commit = "none"

// Date is the build timestamp injected by the build.
var Date = "unknown"

// String returns the user-facing af version string.
func String() string {
	return fmt.Sprintf("af %s (%s, %s)", Version, Commit, Date)
}
