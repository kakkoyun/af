package workstream_test

import (
	"strings"
	"testing"
	"testing/quick"

	"github.com/kakkoyun/af/internal/workstream"
)

func TestPropertySanitizeIsIdempotent(t *testing.T) {
	property := func(input string) bool {
		once := workstream.Sanitize(input)
		twice := workstream.Sanitize(once)
		return once == twice
	}

	err := quick.Check(property, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPropertyApplyPrefixDoesNotDoubleApply(t *testing.T) {
	property := func(prefix, name string) bool {
		if prefix == "" {
			return workstream.ApplyPrefix(name, prefix) == name
		}

		once := workstream.ApplyPrefix(name, prefix)
		twice := workstream.ApplyPrefix(once, prefix)
		return once == twice && strings.HasPrefix(once, prefix)
	}

	err := quick.Check(property, nil)
	if err != nil {
		t.Fatal(err)
	}
}
