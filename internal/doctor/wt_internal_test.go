package doctor

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// slicerProbeName is the name of the slicer probe registered in
// DefaultProbes; used by tests in this file to locate its Result.
const slicerProbeName = "slicer"

// fakeLookupInternal mirrors the test-package fakeLookup but lives in
// the doctor package so this file can access the unexported
// slicerWTChecker seam.
type fakeLookupInternal struct {
	paths    map[string]string
	versions map[string]string
}

func (f fakeLookupInternal) LookPath(_ context.Context, name string) (string, bool) {
	path, ok := f.paths[name]
	return path, ok
}

func (f fakeLookupInternal) Version(_ context.Context, binary string) string {
	return f.versions[binary]
}

// withSlicerWTChecker swaps the package-level seam for the duration of a
// test. It restores the original on cleanup.
func withSlicerWTChecker(t *testing.T, fn func(context.Context) (bool, string)) {
	t.Helper()
	orig := slicerWTChecker
	t.Cleanup(func() { slicerWTChecker = orig })
	slicerWTChecker = fn
}

// TestRun_SlicerWTNoteOnMissingAPI asserts that when slicer is found
// but the wt API is missing, Run attaches the hint as a Note on the
// slicer result. This is the I12.1 (ADR-065) wiring.
func TestRun_SlicerWTNoteOnMissingAPI(t *testing.T) {
	withSlicerWTChecker(t, func(_ context.Context) (bool, string) {
		return false, "wt API missing"
	})

	lookup := fakeLookupInternal{paths: map[string]string{
		"git":    "/g",
		"tmux":   "/t",
		"pi":     "/p",
		"slicer": "/usr/local/bin/slicer",
	}}
	report := Run(context.Background(), lookup, PlatformMacOS, DefaultProbes(nil))

	var slicerResult Result
	for _, r := range report.Results {
		if r.Probe.Name == slicerProbeName {
			slicerResult = r
			break
		}
	}
	if slicerResult.Probe.Name != slicerProbeName {
		t.Fatalf("slicer result missing from report")
	}
	if !slicerResult.Found {
		t.Fatalf("slicer probe should be Found in this fixture")
	}
	if got, want := slicerResult.Note, "wt API missing"; got != want {
		t.Fatalf("slicer Note = %q, want %q", got, want)
	}
}

// TestRun_SlicerWTNoNoteWhenAvailable asserts that when the wt API is
// available the slicer result carries no note.
func TestRun_SlicerWTNoNoteWhenAvailable(t *testing.T) {
	withSlicerWTChecker(t, func(_ context.Context) (bool, string) {
		return true, ""
	})

	lookup := fakeLookupInternal{paths: map[string]string{
		"git":    "/g",
		"tmux":   "/t",
		"pi":     "/p",
		"slicer": "/usr/local/bin/slicer",
	}}
	report := Run(context.Background(), lookup, PlatformMacOS, DefaultProbes(nil))

	for _, r := range report.Results {
		if r.Probe.Name == slicerProbeName && r.Note != "" {
			t.Fatalf("slicer Note = %q, want empty when wt API is available", r.Note)
		}
	}
}

// TestRun_SlicerWTNoNoteWhenSlicerAbsent asserts that when slicer is
// not on PATH the checker is not consulted (no note appears).
func TestRun_SlicerWTNoNoteWhenSlicerAbsent(t *testing.T) {
	// If the checker ran it would set this flag; the test fails on call.
	withSlicerWTChecker(t, func(_ context.Context) (bool, string) {
		t.Errorf("slicerWTChecker should not be called when slicer is absent")
		return false, "should not appear"
	})

	lookup := fakeLookupInternal{paths: map[string]string{
		"git":  "/g",
		"tmux": "/t",
		"pi":   "/p",
	}}
	_ = Run(context.Background(), lookup, PlatformMacOS, DefaultProbes(nil))
}

// TestRender_EmitsSlicerWTWarning asserts that the wt-API note is
// rendered as an indented warning sub-line after the slicer probe row.
func TestRender_EmitsSlicerWTWarning(t *testing.T) {
	withSlicerWTChecker(t, func(_ context.Context) (bool, string) {
		return false, "wt API missing — upgrade slicer"
	})

	lookup := fakeLookupInternal{paths: map[string]string{
		"git":    "/g",
		"tmux":   "/t",
		"pi":     "/p",
		"slicer": "/usr/local/bin/slicer",
	}}
	report := Run(context.Background(), lookup, PlatformMacOS, DefaultProbes(nil))

	var buf bytes.Buffer
	err := Render(&buf, report, "Local environment:")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "wt API missing — upgrade slicer") {
		t.Fatalf("Render output missing wt warning; got:\n%s", out)
	}
	// Make sure the warning marker is the indented ⚠ sub-line, not a row marker.
	if !strings.Contains(out, "        ⚠ wt API missing") {
		t.Fatalf("Render output missing indented warning sub-line; got:\n%s", out)
	}
}
