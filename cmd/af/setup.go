package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/setup"
)

type setupOptions struct {
	root            *rootOptions
	shell           string
	force           bool
	skipCompletions bool
	skipGitignore   bool
}

func newSetupCmd(opts *rootOptions) *cobra.Command {
	setupOpts := &setupOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Initialise the user-scope af environment (no sudo, no installs)",
		Long:  "setup writes the user-scope files described in ADR-045: state directory tree, default config, global gitignore entry, shell completions, and an Obsidian vault hint.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd.Context(), cmd, setupOpts)
		},
	}
	cmd.Flags().BoolVar(&setupOpts.force, "force", false, "overwrite existing ~/.config/af/config.toml")
	cmd.Flags().StringVar(&setupOpts.shell, "shell", "", "shell to install completions for (bash, zsh, fish, powershell)")
	cmd.Flags().BoolVar(&setupOpts.skipCompletions, "skip-completions", false, "do not install shell completions")
	cmd.Flags().BoolVar(&setupOpts.skipGitignore, "skip-gitignore", false, "do not modify the global gitignore")
	return cmd
}

func runSetup(ctx context.Context, cmd *cobra.Command, opts *setupOptions) error {
	root := cmd.Root()
	_, err := setup.Run(ctx, cmd.OutOrStdout(), setup.Options{
		Force:           opts.force,
		Shell:           opts.shell,
		SkipCompletions: opts.skipCompletions,
		SkipGitignore:   opts.skipGitignore,
		GenerateBash:    func(w io.Writer) error { return root.GenBashCompletion(w) },
		GenerateZsh:     func(w io.Writer) error { return root.GenZshCompletion(w) },
		GenerateFish:    func(w io.Writer) error { return root.GenFishCompletion(w, true) },
		Git:             gitConfigCLI{},
	})
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	return nil
}

// gitConfigCLI invokes the real git binary to read and write user-scope
// configuration. Failures are surfaced unchanged to the caller.
type gitConfigCLI struct{}

func (gitConfigCLI) GetGlobal(ctx context.Context, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--get", key)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		// git config returns exit 1 when the key is not set; treat that
		// as "value is empty" rather than an error.
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("git config get %s: %w", key, err)
	}
	return trimTrailingNewline(string(out)), nil
}

func (gitConfigCLI) SetGlobal(ctx context.Context, key, value string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", key, value)
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("git config set %s=%s: %w", key, value, err)
	}
	return nil
}

func trimTrailingNewline(s string) string {
	for s != "" && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
