package workstream

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// ErrInvalidSessionName reports a session name that is unusable as a
// state-directory path or git branch component.
var ErrInvalidSessionName = errors.New("invalid session name")

// maxSessionNameBytes bounds session names so derived paths and refs stay
// well under filesystem and git limits.
const maxSessionNameBytes = 200

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

// ValidateSessionName rejects names that could escape the state root or
// produce a malformed git ref. The name becomes an on-disk directory, a
// branch component, and the collision key, so containment is enforced
// rather than sanitized. Empty names are valid: they select an
// auto-generated name. Slash-nested names stay legal because
// auto-generated names embed the repo slug.
func ValidateSessionName(name string) error {
	if name == "" {
		return nil
	}
	if len(name) > maxSessionNameBytes {
		return fmt.Errorf("%w: longer than %d bytes", ErrInvalidSessionName, maxSessionNameBytes)
	}
	err := validateSessionNameChars(name)
	if err != nil {
		return err
	}
	for _, element := range strings.Split(name, "/") {
		err = validateSessionNameElement(element)
		if err != nil {
			return err
		}
	}
	if !filepath.IsLocal(filepath.FromSlash(name)) {
		return fmt.Errorf("%w: escapes the state root: %q", ErrInvalidSessionName, name)
	}
	return nil
}

func validateSessionNameChars(name string) error {
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("%w: absolute path: %q", ErrInvalidSessionName, name)
	}
	for _, r := range name {
		switch {
		case r == '\\':
			return fmt.Errorf("%w: contains backslash: %q", ErrInvalidSessionName, name)
		case r < ' ' || r == 0x7f:
			return fmt.Errorf("%w: contains control character: %q", ErrInvalidSessionName, name)
		case unicode.IsSpace(r):
			return fmt.Errorf("%w: contains whitespace: %q", ErrInvalidSessionName, name)
		case strings.ContainsRune("~^:?*[", r):
			return fmt.Errorf("%w: contains git-ref-illegal character %q: %q", ErrInvalidSessionName, r, name)
		}
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: contains %q: %q", ErrInvalidSessionName, "..", name)
	}
	if strings.Contains(name, "@{") {
		return fmt.Errorf("%w: contains %q: %q", ErrInvalidSessionName, "@{", name)
	}
	return nil
}

func validateSessionNameElement(element string) error {
	switch {
	case element == "":
		return fmt.Errorf("%w: empty path element", ErrInvalidSessionName)
	case element == ".":
		return fmt.Errorf("%w: %q path element", ErrInvalidSessionName, ".")
	case strings.HasPrefix(element, "-"):
		return fmt.Errorf("%w: path element starts with %q: %q", ErrInvalidSessionName, "-", element)
	case strings.HasSuffix(element, "."):
		return fmt.Errorf("%w: path element ends with %q: %q", ErrInvalidSessionName, ".", element)
	case strings.HasSuffix(element, ".lock"):
		return fmt.Errorf("%w: path element ends with %q: %q", ErrInvalidSessionName, ".lock", element)
	}
	return nil
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
