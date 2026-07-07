package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/smoke"
)

func cannedSmokeReport(fail bool) smoke.Report {
	report := smoke.Report{
		GeneratedAt: time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
		Env:         smoke.Env{AFVersion: "af dev", GOOS: "linux", GOARCH: "amd64", Tools: map[string]string{"git": "git version 2.x"}},
		Steps: []smoke.StepResult{
			{Name: "version", Status: smoke.StatusPass, Expect: "exit 0"},
			{Name: "pull", Status: smoke.StatusSkip, Reason: "slicer not installed"},
		},
	}
	if fail {
		report.Steps = append(report.Steps, smoke.StepResult{
			Name: "create", Status: smoke.StatusFail, Reason: "exit code 1, want 0",
			Argv: []string{"af", "create", "smoke-ws", "--bare"}, ExitCode: 1, Stderr: "boom",
			Expect: "exit 0, workstream created",
		})
	}
	return report
}

func withSmokeSeams(t *testing.T, report smoke.Report, issueURL string) *[]string {
	t.Helper()
	restoreRun := smokeRunFunc
	restoreIssue := smokeIssueFunc
	var issueCalls []string
	smokeRunFunc = func(context.Context, smoke.Options) (smoke.Report, error) { return report, nil }
	smokeIssueFunc = func(_ context.Context, mdPath, title string) (string, error) {
		issueCalls = append(issueCalls, title+" | "+mdPath)
		return issueURL, nil
	}
	t.Cleanup(func() { smokeRunFunc = restoreRun; smokeIssueFunc = restoreIssue })
	return &issueCalls
}

func TestDoctorAll_SummaryAndCleanExitOnPass(t *testing.T) {
	withSmokeSeams(t, cannedSmokeReport(false), "")

	stdout, _, err := executeCommand(t, newRootCmd(), "doctor", "--all")
	if err != nil {
		t.Fatalf("doctor --all: %v", err)
	}
	if !strings.Contains(stdout, "1 pass, 0 fail, 1 skip") {
		t.Fatalf("summary missing counts:\n%s", stdout)
	}
	if !strings.Contains(stdout, "slicer not installed") {
		t.Fatalf("summary missing skip reason:\n%s", stdout)
	}
}

func TestDoctorAll_FailuresProduceNonZeroAndReportFiles(t *testing.T) {
	withSmokeSeams(t, cannedSmokeReport(true), "")
	dir := t.TempDir()

	stdout, _, err := executeCommand(t, newRootCmd(), "doctor", "--all", "--report", "--report-dir", dir)
	if !errors.Is(err, errDoctorSmokeFailed) {
		t.Fatalf("err = %v, want errDoctorSmokeFailed", err)
	}
	if !strings.Contains(stdout, "FAIL create") {
		t.Fatalf("summary missing failure line:\n%s", stdout)
	}
	assertSmokeReportFiles(t, dir)
}

// assertSmokeReportFiles verifies both artifacts exist and the markdown
// carries the failures section.
func assertSmokeReportFiles(t *testing.T, dir string) {
	t.Helper()
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir: %v", readErr)
	}
	var md, js bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			md = true
			content, fileErr := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // Test path under t.TempDir.
			if fileErr != nil {
				t.Fatalf("read report: %v", fileErr)
			}
			if !strings.Contains(string(content), "## Failures") {
				t.Fatalf("markdown report missing failures section:\n%s", content)
			}
		}
		if strings.HasSuffix(e.Name(), ".json") {
			js = true
		}
	}
	if !md || !js {
		t.Fatalf("report files missing: md=%v json=%v (%v)", md, js, entries)
	}
}

func TestDoctorAll_IssueFiledOnlyOnFailures(t *testing.T) {
	t.Run("failures file an issue", func(t *testing.T) {
		calls := withSmokeSeams(t, cannedSmokeReport(true), "https://github.com/kakkoyun/af/issues/99")
		installFakeGH(t)
		dir := t.TempDir()

		stdout, _, err := executeCommand(t, newRootCmd(), "doctor", "--all", "--issue", "--report-dir", dir)
		if !errors.Is(err, errDoctorSmokeFailed) {
			t.Fatalf("err = %v, want errDoctorSmokeFailed", err)
		}
		if len(*calls) != 1 {
			t.Fatalf("issue calls = %v, want exactly one", *calls)
		}
		if !strings.Contains((*calls)[0], "1 failing step(s)") {
			t.Fatalf("issue title missing failure count: %v", *calls)
		}
		if !strings.Contains(stdout, "issues/99") {
			t.Fatalf("stdout missing filed issue URL:\n%s", stdout)
		}
	})
	t.Run("no failures, no issue", func(t *testing.T) {
		calls := withSmokeSeams(t, cannedSmokeReport(false), "unused")
		installFakeGH(t)
		dir := t.TempDir()

		_, _, err := executeCommand(t, newRootCmd(), "doctor", "--all", "--issue", "--report-dir", dir)
		if err != nil {
			t.Fatalf("doctor --all --issue with all-pass: %v", err)
		}
		if len(*calls) != 0 {
			t.Fatalf("issue filed despite zero failures: %v", *calls)
		}
	})
}

// installFakeGH puts a stub gh on PATH so the LookPath gate passes;
// the seam intercepts before it would run.
func installFakeGH(t *testing.T) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	err := os.MkdirAll(bin, 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(bin, "gh")
	err = os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700) //nolint:gosec // Executable test stub requires the exec bit.
	if err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

// TestDoctorFlagValidation rejects smoke flags without --all and
// --remote combined with --all (Copilot review, PR #8).
func TestDoctorFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"doctor", "--report"},
		{"doctor", "--issue"},
		{"doctor", "--report-dir", "/tmp/x"},
		{"doctor", "--all", "--remote", "host"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := executeCommand(t, newRootCmd(), args...)
			if !errors.Is(err, errDoctorFlagUsage) {
				t.Fatalf("%v: err = %v, want errDoctorFlagUsage", args, err)
			}
		})
	}
}
