package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/config"
)

func newConfigCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and initialise the af configuration",
		Long: "config groups the read-only and write-once configuration helpers.\n\n" +
			"Use `af config init` to scaffold ~/.config/af/config.toml, and\n" +
			"`af config show` to print the effective merged configuration.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
			if err != nil {
				return fmt.Errorf("show config help: %w", err)
			}
			return nil
		},
	}

	cmd.AddCommand(newConfigInitCmd(opts))
	cmd.AddCommand(newConfigShowCmd(opts))

	return cmd
}

func newConfigInitCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write the annotated user-config template",
		Long: "init writes the annotated user-config template to either the path given\n" +
			"by --config or, when --config is unset, to $HOME/.config/af/config.toml.\n" +
			"It refuses to overwrite an existing file.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigInit(cmd, opts)
		},
	}
}

func newConfigShowCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the effective merged configuration",
		Long: "show loads the layered configuration (compiled defaults, then the user\n" +
			"config under --config or $HOME/.config/af/config.toml, then any repo\n" +
			"config under <cwd>/.af/config.toml) and prints the merged result as TOML.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigShow(cmd.Context(), cmd, opts)
		},
	}
}

func runConfigInit(cmd *cobra.Command, opts *rootOptions) error {
	path, err := config.ResolveUserConfigPath(opts.configPath)
	if err != nil {
		return fmt.Errorf("config init: resolve path: %w", err)
	}

	err = config.WriteUserConfig(path)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("config init: %w; remove %s first to reinitialise", fs.ErrExist, path)
		}
		return fmt.Errorf("config init: %w", err)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
	if err != nil {
		return fmt.Errorf("config init: write success message: %w", err)
	}

	return nil
}

func runConfigShow(ctx context.Context, cmd *cobra.Command, opts *rootOptions) error {
	repoDir, err := os.Getwd()
	if err != nil {
		repoDir = ""
	}

	cfg, err := config.LoadWithOptions(ctx, config.LoadOptions{
		UserConfigPath: opts.configPath,
		RepoDir:        repoDir,
	})
	if err != nil {
		return fmt.Errorf("config show: load: %w", err)
	}

	_, err = fmt.Fprint(cmd.OutOrStdout(), config.Render(cfg))
	if err != nil {
		return fmt.Errorf("config show: write output: %w", err)
	}

	return nil
}
