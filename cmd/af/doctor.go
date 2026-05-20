package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/config"
	"github.com/kakkoyun/af/internal/doctor"
	"github.com/kakkoyun/af/internal/remote"
)

var errDoctorMissingTools = errors.New("missing required tool")

type doctorOptions struct {
	root    *rootOptions
	remote  string
	verbose bool
}

func newDoctorCmd(opts *rootOptions) *cobra.Command {
	docOpts := &doctorOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Probe the local (or remote) environment for required tools",
		Long:  "doctor probes the local machine (or, with --remote, an SSH host) for the tools af relies on and prints install hints. It never installs anything.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.Context(), cmd, docOpts)
		},
	}
	cmd.Flags().StringVar(&docOpts.remote, "remote", "", "probe an SSH host instead of the local machine")
	cmd.Flags().BoolVar(&docOpts.verbose, "verbose", false, "verbose probe output (full version strings)")
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

	err := doctor.Render(cmd.OutOrStdout(), report, "Local environment:")
	if err != nil {
		return fmt.Errorf("doctor: render: %w", err)
	}
	if report.HasMissingMustTools() {
		return fmt.Errorf("doctor: %w", errDoctorMissingTools)
	}
	return nil
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
