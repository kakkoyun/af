package secret

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	envelopeFilePerm = 0o600
	envelopeDirPerm  = 0o700
)

var (
	// ErrEnvelopeMissing reports a Read on a path that does not exist.
	ErrEnvelopeMissing = errors.New("envelope missing")
	// ErrEmptyPath reports an empty path passed to Envelope.Write.
	ErrEmptyPath = errors.New("envelope path is empty")
)

// Envelope describes one ephemeral env-file used to ship secrets to a
// remote/sandboxed agent process per ADR-049.
//
// Values are stored in the keyring; the envelope is only ever written
// once at launch and immediately deleted after the agent process sources
// it.
type Envelope struct { //nolint:govet // Field grouping prioritises readability.
	Path    string
	Entries map[string]string
}

// Write serialises Entries as KEY=value lines into Path, creating
// parent directories at 0700 and the file at 0600. Keys are written
// in sorted order so the envelope content is reproducible.
func (e Envelope) Write() error {
	if e.Path == "" {
		return fmt.Errorf("write envelope: %w", ErrEmptyPath)
	}
	err := os.MkdirAll(filepath.Dir(e.Path), envelopeDirPerm)
	if err != nil {
		return fmt.Errorf("create envelope dir: %w", err)
	}
	body := renderEnvelopeBody(e.Entries)
	err = os.WriteFile(e.Path, []byte(body), envelopeFilePerm)
	if err != nil {
		return fmt.Errorf("write envelope %s: %w", e.Path, err)
	}
	return nil
}

// Delete removes the envelope file. Missing files are not errors so the
// teardown path is idempotent.
func (e Envelope) Delete() error {
	if e.Path == "" {
		return nil
	}
	err := os.Remove(e.Path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete envelope %s: %w", e.Path, err)
	}
	return nil
}

func renderEnvelopeBody(entries map[string]string) string {
	if len(entries) == 0 {
		return ""
	}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(escapeEnvValue(entries[k]))
		b.WriteString("\n")
	}
	return b.String()
}

// escapeEnvValue prepares value for KEY=value env-file syntax. Newlines
// are forbidden; double quotes are escaped; the result is wrapped in
// double quotes if it contains whitespace, '$', or '"' characters.
func escapeEnvValue(value string) string {
	if strings.ContainsAny(value, "\n\r") {
		return "\"\""
	}
	if !strings.ContainsAny(value, " \t\"$") {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return "\"" + escaped + "\""
}
