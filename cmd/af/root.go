package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	configPath  string
	sessionName string
	verbose     bool
}

func newRootCmd() *cobra.Command {
	return newRootCmdWithOptions(&rootOptions{})
}

func newRootCmdWithOptions(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "af",
		Short: "Manage isolated AI-agent workstreams",
		Long:  "af manages isolated AI-agent workstreams across git worktrees, tmux, sandboxes, and remote hosts.",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
			if err != nil {
				return fmt.Errorf("show help: %w", err)
			}
			return nil
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			err := cmd.Context().Err()
			if err != nil {
				return fmt.Errorf("prepare af command: %w", err)
			}
			return nil
		},
	}

	flags := cmd.PersistentFlags()
	flags.BoolVarP(&opts.verbose, "verbose", "v", false, "enable verbose diagnostic logging")
	flags.StringVar(&opts.configPath, "config", "", "path to an af config file")
	flags.StringVar(&opts.sessionName, "session", "", "target a specific workstream session")

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newConfigCmd(opts))
	cmd.AddCommand(newCompletionsCmd())
	cmd.AddCommand(newDoctorCmd(opts))
	cmd.AddCommand(newSetupCmd(opts))
	cmd.AddCommand(newAuthCmd(opts))
	cmd.AddCommand(newCreateCmd(opts))
	cmd.AddCommand(newListCmd(opts))
	cmd.AddCommand(newInfoCmd(opts))
	cmd.AddCommand(newAgentCmd(opts))
	cmd.AddCommand(newDoneCmd(opts))
	cmd.AddCommand(newSessionBranchCmd(opts))
	cmd.AddCommand(newSuspendCmd(opts))
	cmd.AddCommand(newResumeCmd(opts))
	cmd.AddCommand(newNoteCmd(opts))
	cmd.AddCommand(newCleanCmd(opts))
	cmd.AddCommand(newStatusCmd(opts))
	cmd.AddCommand(newStackCmd(opts))
	cmd.AddCommand(newUnstackCmd(opts))
	cmd.AddCommand(newSyncCmd(opts))

	return cmd
}
