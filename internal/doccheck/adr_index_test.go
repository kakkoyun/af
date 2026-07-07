// Package doccheck guards documentation invariants that CI must hold:
// the ADR INDEX table has to match each ADR's own frontmatter, because
// "Documentation Is the Spec" (constitution rule 3).
package doccheck

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	indexRowPattern   = regexp.MustCompile(`^\| \[(\d{3})\]\([^)]+\)\s*\|[^|]*\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|`)
	frontmatterField  = regexp.MustCompile(`(?m)^(status|implementation): *([^#\n]+?) *(?:#.*)?$`)
	adrFilenamePrefix = regexp.MustCompile(`^(\d{3})-.*\.md$`)
)

type adrRecord struct {
	status         string
	implementation string
}

// TestADRIndexMatchesFrontmatter fails when docs/adr/INDEX.md drifts
// from the status/implementation frontmatter of any ADR file.
func TestADRIndexMatchesFrontmatter(t *testing.T) {
	adrDir := filepath.Join("..", "..", "docs", "adr")

	fromFiles := readFrontmatterRecords(t, adrDir)
	fromIndex := readIndexRecords(t, filepath.Join(adrDir, "INDEX.md"))

	if len(fromFiles) == 0 || len(fromIndex) == 0 {
		t.Fatalf("parsed %d ADR files and %d INDEX rows; expected both non-empty", len(fromFiles), len(fromIndex))
	}

	numbers := make([]string, 0, len(fromFiles))
	for number := range fromFiles {
		numbers = append(numbers, number)
	}
	sort.Strings(numbers)

	for _, number := range numbers {
		file := fromFiles[number]
		row, ok := fromIndex[number]
		if !ok {
			t.Errorf("ADR %s has a file but no INDEX.md row", number)
			continue
		}
		if row.status != file.status {
			t.Errorf("ADR %s: INDEX status %q != frontmatter status %q", number, row.status, file.status)
		}
		if row.implementation != file.implementation {
			t.Errorf("ADR %s: INDEX implementation %q != frontmatter implementation %q", number, row.implementation, file.implementation)
		}
	}
	for number := range fromIndex {
		if _, ok := fromFiles[number]; !ok {
			t.Errorf("INDEX.md lists ADR %s but no matching file exists", number)
		}
	}
}

func readFrontmatterRecords(t *testing.T, adrDir string) map[string]adrRecord {
	t.Helper()
	entries, err := os.ReadDir(adrDir)
	if err != nil {
		t.Fatalf("read ADR dir: %v", err)
	}
	records := make(map[string]adrRecord)
	for _, entry := range entries {
		match := adrFilenamePrefix.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		content, err := os.ReadFile(filepath.Join(adrDir, entry.Name())) //nolint:gosec // Repo-relative doc path.
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		records[match[1]] = parseFrontmatter(t, entry.Name(), string(content))
	}
	return records
}

func parseFrontmatter(t *testing.T, name, content string) adrRecord {
	t.Helper()
	const marker = "---\n"
	rest, found := strings.CutPrefix(content, marker)
	if !found {
		t.Fatalf("%s: missing frontmatter opening marker", name)
	}
	end := strings.Index(rest, "\n"+marker)
	if end < 0 {
		t.Fatalf("%s: missing frontmatter closing marker", name)
	}
	record := adrRecord{}
	for _, match := range frontmatterField.FindAllStringSubmatch(rest[:end], -1) {
		value := strings.TrimSpace(strings.Trim(strings.TrimSpace(match[2]), `"`))
		switch match[1] {
		case "status":
			record.status = value
		case "implementation":
			record.implementation = value
		}
	}
	if record.status == "" || record.implementation == "" {
		t.Fatalf("%s: frontmatter missing status or implementation", name)
	}
	return record
}

func readIndexRecords(t *testing.T, indexPath string) map[string]adrRecord {
	t.Helper()
	content, err := os.ReadFile(indexPath) //nolint:gosec // Repo-relative doc path.
	if err != nil {
		t.Fatalf("read INDEX.md: %v", err)
	}
	records := make(map[string]adrRecord)
	for _, line := range strings.Split(string(content), "\n") {
		match := indexRowPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		records[match[1]] = adrRecord{
			status:         strings.TrimSpace(match[2]),
			implementation: strings.TrimSpace(match[3]),
		}
	}
	return records
}
