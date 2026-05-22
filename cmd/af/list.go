package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/session"
)

type listOptions struct {
	root *rootOptions
}

func newListCmd(opts *rootOptions) *cobra.Command {
	lOpts := &listOptions{root: opts}
	return &cobra.Command{
		Use:   "list",
		Short: "List workstreams known to af",
		Long:  "list enumerates every workstream that has a state.toml under the local sessions directory.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, lOpts)
		},
	}
}

func runList(cmd *cobra.Command, opts *listOptions) error {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	_ = opts

	states, err := readAllStates(stateDir)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	return printList(cmd, states)
}

func defaultSessionsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "af", "v1", "sessions"), nil
}

type sessionSummary struct { //nolint:govet // Field grouping prioritises readability.
	state           session.State
	statePath       string
	prRefreshFailed bool
}

func readAllStates(stateDir string) ([]sessionSummary, error) {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir %s: %w", stateDir, err)
	}
	summaries := make([]sessionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		statePath := filepath.Join(stateDir, entry.Name(), "state.toml")
		state, readErr := session.ReadState(statePath)
		if readErr != nil {
			if errors.Is(readErr, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read state %s: %w", statePath, readErr)
		}
		summaries = append(summaries, sessionSummary{state: state, statePath: statePath})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].state.Session.Name < summaries[j].state.Session.Name
	})
	return summaries, nil
}

func printList(cmd *cobra.Command, summaries []sessionSummary) error {
	w := cmd.OutOrStdout()
	if len(summaries) == 0 {
		_, err := fmt.Fprintln(w, "no workstreams")
		if err != nil {
			return fmt.Errorf("write empty list: %w", err)
		}
		return nil
	}
	for i := range summaries {
		_, err := fmt.Fprintf(w, "%-30s %-10s %s\n", summaries[i].state.Session.Name, summaries[i].state.Session.Status, summaries[i].state.Worktree.Branch)
		if err != nil {
			return fmt.Errorf("write list row: %w", err)
		}
	}
	return nil
}
