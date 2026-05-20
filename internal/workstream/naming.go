package workstream

import (
	"strings"
	"unicode"
)

// Sanitize returns a stable session-safe representation of name.
func Sanitize(name string) string {
	return strings.Map(func(r rune) rune {
		if isNameSeparator(r) {
			return '-'
		}
		return r
	}, name)
}

// ApplyPrefix prepends prefix to name unless name already starts with prefix.
func ApplyPrefix(name, prefix string) string {
	if prefix == "" || strings.HasPrefix(name, prefix) {
		return name
	}
	return prefix + name
}

func isNameSeparator(r rune) bool {
	switch r {
	case '/', '\\', ':', '.':
		return true
	default:
		return unicode.IsSpace(r)
	}
}
