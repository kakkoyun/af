package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var errUnsupportedShell = errors.New("unsupported completion shell")

const (
	shellBash       = "bash"
	shellZsh        = "zsh"
	shellFish       = "fish"
	shellPowerShell = "powershell"
)

func supportedCompletionShells() []string {
	return []string{shellBash, shellZsh, shellFish, shellPowerShell}
}

func newCompletionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completions <bash|zsh|fish|powershell>",
		Short:     "Emit a shell completion script for af",
		Long:      "completions writes a shell-specific completion script to stdout. Pipe it into your shell's completion loader (e.g. `af completions zsh > \"${fpath[1]}/_af\"`).",
		Args:      cobra.ExactArgs(1),
		ValidArgs: supportedCompletionShells(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletions(cmd, args[0])
		},
	}
}

func runCompletions(cmd *cobra.Command, shell string) error {
	root := cmd.Root()
	out := cmd.OutOrStdout()

	switch shell {
	case shellBash:
		err := root.GenBashCompletion(out)
		if err != nil {
			return fmt.Errorf("completions bash: %w", err)
		}
	case shellZsh:
		err := root.GenZshCompletion(out)
		if err != nil {
			return fmt.Errorf("completions zsh: %w", err)
		}
	case shellFish:
		err := root.GenFishCompletion(out, true)
		if err != nil {
			return fmt.Errorf("completions fish: %w", err)
		}
	case shellPowerShell:
		err := root.GenPowerShellCompletionWithDesc(out)
		if err != nil {
			return fmt.Errorf("completions powershell: %w", err)
		}
	default:
		return fmt.Errorf("completions: %w %q; want one of %v", errUnsupportedShell, shell, supportedCompletionShells())
	}

	return nil
}
