package workstream

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// BranchOptions describes config and repository state for branch naming.
type BranchOptions struct {
	Name              string
	Prefix            string
	PrefixOnForkOnly  bool
	HasUpstreamRemote bool
}

// Sanitize returns a stable session-safe representation of name.
func Sanitize(name string) string {
	var sanitized strings.Builder
	for _, r := range name {
		if isNameSeparator(r) {
			sanitized.WriteString("--")
			continue
		}
		sanitized.WriteRune(r)
	}

	return sanitized.String()
}

// ApplyPrefix prepends prefix to name unless name already starts with prefix.
func ApplyPrefix(name, prefix string) string {
	if prefix == "" || name == prefix || strings.HasPrefix(name, prefix+"/") {
		return name
	}
	return prefix + "/" + name
}

// BranchName returns the git branch name for a workstream.
func BranchName(opts BranchOptions) string {
	if opts.PrefixOnForkOnly && !opts.HasUpstreamRemote {
		return opts.Name
	}

	return ApplyPrefix(opts.Name, opts.Prefix)
}

// AutoSessionName returns the default session name for repo at timestamp.
func AutoSessionName(repo string, at time.Time) string {
	return fmt.Sprintf("%s-%s", repo, at.Format("20060102-150405"))
}

// SubBranchName returns the branch name for a non-primary agent slot.
func SubBranchName(primaryBranch, slot string) string {
	return primaryBranch + "--" + Sanitize(slot)
}

// SessionID derives a deterministic agent session UUID for a launch.
func SessionID(repo, branch, slot string, launch time.Time) uuid.UUID {
	name := fmt.Sprintf("%s/%s/%s/%d", repo, branch, slot, launch.UnixNano())
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(name))
}

func isNameSeparator(r rune) bool {
	switch r {
	case '/', '\\', ':', '.':
		return true
	default:
		return unicode.IsSpace(r)
	}
}
