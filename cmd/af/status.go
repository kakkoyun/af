package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

type statusOptions struct {
	root     *rootOptions
	filter   string
	jsonMode bool
	all      bool
}

func newStatusCmd(opts *rootOptions) *cobra.Command {
	sOpts := &statusOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Workstream dashboard (active/suspended counts + per-workstream rows)",
		Long:  "status reads every state.toml under the sessions dir and prints a dashboard. With --json the dashboard is emitted as JSON; with --filter STATE only matching workstreams are shown; with --all completed/abandoned workstreams are included.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd, sOpts)
		},
	}
	cmd.Flags().BoolVar(&sOpts.jsonMode, "json", false, "emit status as JSON")
	cmd.Flags().BoolVar(&sOpts.all, "all", false, "include completed and abandoned workstreams")
	cmd.Flags().StringVar(&sOpts.filter, "filter", "", "show only workstreams in this lifecycle state (active|suspended|completed|abandoned)")
	return cmd
}

func runStatus(cmd *cobra.Command, opts *statusOptions) error {
	stateDir, err := defaultSessionsDir()
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	summaries, err := readAllStates(stateDir)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}

	filtered := filterStatusSummaries(summaries, opts)

	if opts.jsonMode {
		return statusEmitJSON(cmd, filtered)
	}
	return statusEmitText(cmd, filtered)
}

func filterStatusSummaries(summaries []sessionSummary, opts *statusOptions) []sessionSummary {
	out := make([]sessionSummary, 0, len(summaries))
	for i := range summaries {
		status := summaries[i].state.Session.Status
		if opts.filter != "" && status != opts.filter {
			continue
		}
		if !opts.all && (status == "completed" || status == "abandoned") {
			continue
		}
		out = append(out, summaries[i])
	}
	return out
}

func statusEmitJSON(cmd *cobra.Command, summaries []sessionSummary) error {
	rows := make([]map[string]any, 0, len(summaries))
	for i := range summaries {
		s := summaries[i].state
		rows = append(rows, map[string]any{
			"name":   s.Session.Name,
			"status": s.Session.Status,
			"branch": s.Worktree.Branch,
		})
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Errorf("status json: %w", err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	if err != nil {
		return fmt.Errorf("status json write: %w", err)
	}
	return nil
}

func statusEmitText(cmd *cobra.Command, summaries []sessionSummary) error {
	w := cmd.OutOrStdout()
	if len(summaries) == 0 {
		_, err := fmt.Fprintln(w, "no workstreams")
		if err != nil {
			return fmt.Errorf("status write: %w", err)
		}
		return nil
	}
	_, err := fmt.Fprintln(w, "NAME                           STATUS     BRANCH")
	if err != nil {
		return fmt.Errorf("status write header: %w", err)
	}
	for i := range summaries {
		_, err = fmt.Fprintf(w, "%-30s %-10s %s\n", summaries[i].state.Session.Name, summaries[i].state.Session.Status, summaries[i].state.Worktree.Branch)
		if err != nil {
			return fmt.Errorf("status write row: %w", err)
		}
	}
	return nil
}
