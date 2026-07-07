package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/smoke"
)

// afRepoSlug is where `af doctor --all --issue` files failure reports.
const afRepoSlug = "kakkoyun/af"

// Report artifact permissions: owner-only files in a user-chosen dir.
const (
	reportDirPerm  = 0o750
	reportFilePerm = 0o600
)

// errDoctorSmokeFailed marks a self-smoke run with at least one failing
// step; main maps it to a non-zero exit per ADR-068.
var errDoctorSmokeFailed = errors.New("doctor self-smoke reported failures")

type smokeRunFn func(ctx context.Context, opts smoke.Options) (smoke.Report, error)

// smokeRunFunc is the test seam wrapping smoke.Run.
//
//nolint:gochecknoglobals // Test seam: same pattern as prAIBodyFunc.
var smokeRunFunc smokeRunFn = smoke.Run

type smokeIssueFn func(ctx context.Context, mdPath, title string) (string, error)

// smokeIssueFunc is the test seam wrapping the gh issue creation.
//
//nolint:gochecknoglobals // Test seam: same pattern as smokeRunFunc above.
var smokeIssueFunc smokeIssueFn = defaultSmokeIssue

// runDoctorSmoke drives the ADR-074 self-smoke: isolated temp root,
// real af re-execution, terminal summary, optional report files and
// GitHub issue.
func runDoctorSmoke(cmd *cobra.Command, opts *doctorOptions) error {
	root, err := os.MkdirTemp("", "af-doctor-smoke-*")
	if err != nil {
		return fmt.Errorf("doctor --all: create scratch root: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }() //nolint:errcheck // Best-effort cleanup of the scratch root.

	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("doctor --all: resolve own binary: %w", err)
	}

	report, err := smokeRunFunc(cmd.Context(), smoke.Options{
		Binary:   binary,
		Root:     root,
		Exec:     smoke.ExecCommand,
		LookPath: exec.LookPath,
		Now:      time.Now,
	})
	if err != nil {
		return fmt.Errorf("doctor --all: %w", err)
	}

	renderSmokeSummary(cmd, report)
	err = writeSmokeArtifacts(cmd, opts, report)
	if err != nil {
		return err
	}
	_, fail, _ := report.Counts()
	if fail > 0 {
		return fmt.Errorf("%w: %d failing step(s)", errDoctorSmokeFailed, fail)
	}
	return nil
}

func renderSmokeSummary(cmd *cobra.Command, report smoke.Report) {
	w := cmd.OutOrStdout()
	pass, fail, skip := report.Counts()
	writef(w, "doctor self-smoke: %d pass, %d fail, %d skip (af %s on %s/%s)\n",
		pass, fail, skip, report.Env.AFVersion, report.Env.GOOS, report.Env.GOARCH)
	for i := range report.Steps {
		s := report.Steps[i]
		switch s.Status {
		case smoke.StatusFail:
			writef(w, "  FAIL %-28s %s\n", s.Name, s.Reason)
		case smoke.StatusSkip:
			writef(w, "  skip %-28s %s\n", s.Name, s.Reason)
		case smoke.StatusPass:
			// Passes stay compact; the report file carries details.
		}
	}
}

// writeSmokeArtifacts writes the report files and files the GitHub
// issue when requested. Environment-only problems (skips) never file.
func writeSmokeArtifacts(cmd *cobra.Command, opts *doctorOptions, report smoke.Report) error {
	if !opts.smokeReport && !opts.smokeIssue {
		return nil
	}
	dir := opts.smokeReportDir
	if dir == "" {
		dir = "."
	}
	stamp := report.GeneratedAt.UTC().Format("20060102-150405")
	mdPath := filepath.Join(dir, "af-doctor-smoke-"+stamp+".md")
	jsonPath := filepath.Join(dir, "af-doctor-smoke-"+stamp+".json")

	err := os.MkdirAll(dir, reportDirPerm)
	if err != nil {
		return fmt.Errorf("doctor --all: create report dir: %w", err)
	}
	err = os.WriteFile(mdPath, []byte(smoke.RenderMarkdown(report)), reportFilePerm)
	if err != nil {
		return fmt.Errorf("doctor --all: write report: %w", err)
	}
	data, err := smoke.RenderJSON(report)
	if err != nil {
		return fmt.Errorf("doctor --all: %w", err)
	}
	err = os.WriteFile(jsonPath, data, reportFilePerm)
	if err != nil {
		return fmt.Errorf("doctor --all: write json report: %w", err)
	}
	writef(cmd.OutOrStdout(), "report: %s (json: %s)\n", mdPath, jsonPath)
	return fileSmokeIssue(cmd, opts, report, mdPath)
}

// fileSmokeIssue files failing steps as a GitHub issue when --issue is
// set; environment problems (skips) never file, and a missing gh
// degrades to a terminal hint.
func fileSmokeIssue(cmd *cobra.Command, opts *doctorOptions, report smoke.Report, mdPath string) error {
	_, fail, _ := report.Counts()
	if !opts.smokeIssue || fail == 0 {
		return nil
	}
	_, lookErr := exec.LookPath("gh")
	if lookErr != nil {
		// Missing gh is an environment condition, not a failure: the
		// report file is the fallback delivery path.
		writef(cmd.ErrOrStderr(), "doctor --all: --issue requested but gh is not installed; paste %s manually\n", mdPath)
		return nil //nolint:nilerr // Degrade to terminal hint by design.
	}
	title := fmt.Sprintf("doctor smoke: %d failing step(s) on %s/%s (%s)",
		fail, report.Env.GOOS, report.Env.GOARCH, strings.TrimSpace(report.Env.AFVersion))
	url, issueErr := smokeIssueFunc(cmd.Context(), mdPath, title)
	if issueErr != nil {
		return fmt.Errorf("doctor --all: file issue: %w", issueErr)
	}
	writef(cmd.OutOrStdout(), "issue filed: %s\n", url)
	return nil
}

func defaultSmokeIssue(ctx context.Context, mdPath, title string) (string, error) {
	gh := exec.CommandContext(ctx, "gh", "issue", "create", "-R", afRepoSlug, "--title", title, "--body-file", mdPath)
	out, err := gh.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh issue create: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
