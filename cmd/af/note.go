package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/session"
)

var errNoteAppendRequired = errors.New("--append <text> required")

func newNoteCmd(_ *rootOptions) *cobra.Command {
	var appendText string
	cmd := &cobra.Command{
		Use:   "note [session]",
		Short: "Append a free-form note event to the workstream ledger",
		Long:  "note records an entry in the workstream's ledger.jsonl. Pass --append \"text\" to add a structured note event used by the Obsidian integration (ADR-047).",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			if appendText == "" {
				return fmt.Errorf("note: %w", errNoteAppendRequired)
			}
			statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
			if err != nil {
				return err
			}
			ledgerPath := filepath.Join(filepath.Dir(statePath), "ledger.jsonl")
			err = withSessionLock(statePath, func() error {
				return session.AppendEvent(ledgerPath, session.Event{
					Timestamp: time.Now().UTC(),
					Type:      "note",
					Fields:    map[string]any{"text": appendText},
				})
			})
			if err != nil {
				return fmt.Errorf("note: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "appended note event")
			if err != nil {
				return fmt.Errorf("note write: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&appendText, "append", "", "text to append as a note event")
	return cmd
}
