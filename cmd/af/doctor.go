package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/doctor"
	"github.com/kakkoyun/af/internal/remote"
)

var errDoctorMissingTools = errors.New("missing required tool")

type doctorOptions struct {
	root           *rootOptions
	remote         string
	smokeReportDir string
	verbose        bool
	smokeAll       bool
	smokeReport    bool
	smokeIssue     bool
}

func newDoctorCmd(opts *rootOptions) *cobra.Command {
	docOpts := &doctorOptions{root: opts}
	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Probe the local (or remote) environment for required tools",
		Long:    "doctor probes the local machine (or, with --remote, an SSH host) for the tools af relies on and prints install hints. It never installs anything.",
		Example: "  af doctor --all --report",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := validateDoctorFlags(docOpts)
			if err != nil {
				return err
			}
			if docOpts.smokeAll {
				return runDoctorSmoke(cmd, docOpts)
			}
			return runDoctor(cmd.Context(), cmd, docOpts)
		},
	}
	cmd.Flags().StringVar(&docOpts.remote, "remote", "", "probe an SSH host instead of the local machine")
	cmd.Flags().BoolVar(&docOpts.verbose, "verbose", false, "verbose probe output (full version strings)")
	cmd.Flags().BoolVar(&docOpts.smokeAll, "all", false, "run the host self-smoke: real af commands in an isolated scratch HOME, cleaned up afterwards (ADR-074)")
	cmd.Flags().BoolVar(&docOpts.smokeReport, "report", false, "with --all: write markdown + json report files")
	cmd.Flags().StringVar(&docOpts.smokeReportDir, "report-dir", "", "with --report or --issue: directory for report files (default: current directory)")
	cmd.Flags().BoolVar(&docOpts.smokeIssue, "issue", false, "with --all: file failing steps as a GitHub issue on "+afRepoSlug+" via gh")
	return cmd
}

func runDoctor(ctx context.Context, cmd *cobra.Command, opts *doctorOptions) error {
	cfg, err := loadDoctorConfig(ctx, opts.root)
	if err != nil {
		return err
	}

	if opts.remote != "" {
		return runDoctorRemote(ctx, cmd, opts, cfg)
	}

	return runDoctorLocal(ctx, cmd, cfg)
}

func runDoctorLocal(ctx context.Context, cmd *cobra.Command, cfg config.Config) error {
	probes := doctor.DefaultProbes(cfg.Doctor.ExtraTools)
	platform := doctor.DetectPlatform(localOSRelease{})
	report := doctor.Run(ctx, doctor.SystemLookup{}, platform, probes)
	appendObsidianVaultResults(&report, cfg)

	err := doctor.Render(cmd.OutOrStdout(), report, "Local environment:")
	if err != nil {
		return fmt.Errorf("doctor: render: %w", err)
	}
	if report.HasMissingMustTools() {
		return fmt.Errorf("doctor: %w", errDoctorMissingTools)
	}
	return nil
}

func appendObsidianVaultResults(report *doctor.Report, cfg config.Config) {
	if len(cfg.Obsidian.Vaults) == 0 {
		return
	}
	names := make([]string, 0, len(cfg.Obsidian.Vaults))
	for name := range cfg.Obsidian.Vaults {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := cfg.Obsidian.Vaults[name]
		found, reason := obsidianVaultAccessible(path)
		probe := doctor.Probe{
			Name:   "obsidian:" + name,
			Tier:   doctor.TierNice,
			Reason: reason,
			Hints: map[doctor.Platform]string{
				doctor.PlatformOther: "create vault directory or update [obsidian.vaults]." + name,
			},
		}
		report.Results = append(report.Results, doctor.Result{
			Probe: probe,
			Path:  path,
			Found: found,
		})
	}
}

func obsidianVaultAccessible(path string) (bool, string) {
	if path == "" {
		return false, "configured Obsidian vault has an empty path"
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, "configured Obsidian vault is not accessible"
	}
	if !info.IsDir() {
		return false, "configured Obsidian vault path is not a directory"
	}
	_, err = os.ReadDir(path)
	if err != nil {
		return false, "configured Obsidian vault is not readable"
	}
	return true, "configured Obsidian vault is accessible"
}

func loadDoctorConfig(ctx context.Context, opts *rootOptions) (config.Config, error) {
	repoDir, err := os.Getwd()
	if err != nil {
		repoDir = ""
	}
	cfg, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: opts.configPath,
		RepoDir:        repoDir,
	})
	if err != nil {
		return config.Config{}, fmt.Errorf("doctor: load config: %w", err)
	}
	return cfg, nil
}

func runDoctorRemote(ctx context.Context, cmd *cobra.Command, opts *doctorOptions, cfg config.Config) error {
	ssh := remote.NewSSH(opts.remote, cfg.Remote.SSHOptions)
	probes := doctor.DefaultProbes(cfg.Doctor.ExtraTools)
	platform := doctor.DetectRemotePlatform(ctx, ssh)
	report := doctor.Run(ctx, doctor.RemoteLookup{Commander: ssh}, platform, probes)

	heading := fmt.Sprintf("Remote environment (%s):", opts.remote)
	err := doctor.Render(cmd.OutOrStdout(), report, heading)
	if err != nil {
		return fmt.Errorf("doctor --remote: render: %w", err)
	}
	if report.HasMissingMustTools() {
		return fmt.Errorf("doctor --remote %s: %w", opts.remote, errDoctorMissingTools)
	}
	return nil
}

// localOSRelease reads /etc/os-release for Linux platform detection.
type localOSRelease struct{}

func (localOSRelease) Read() (map[string]string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("read /etc/os-release: %w", err)
	}
	return doctor.ParseOSRelease(string(data)), nil
}

// errDoctorFlagUsage rejects ambiguous doctor flag combinations.
var errDoctorFlagUsage = errors.New("--report, --report-dir, and --issue require --all; --remote cannot be combined with --all")

// validateDoctorFlags rejects smoke flags without --all and --remote
// with --all instead of silently ignoring either.
func validateDoctorFlags(opts *doctorOptions) error {
	smokeExtras := opts.smokeReport || opts.smokeIssue || opts.smokeReportDir != ""
	if !opts.smokeAll && smokeExtras {
		return fmt.Errorf("doctor: %w", errDoctorFlagUsage)
	}
	if opts.smokeAll && opts.remote != "" {
		return fmt.Errorf("doctor: %w", errDoctorFlagUsage)
	}
	return nil
}
