package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

type statusOptions struct {
	root     *rootOptions
	filter   string
	jsonMode bool
	all      bool
	refresh  bool
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
	cmd.Flags().BoolVar(&sOpts.refresh, "refresh", false, "force-refresh cached PR state before rendering")
	cmd.Flags().StringVar(&sOpts.filter, "filter", "", "show only workstreams in this lifecycle state (active|suspended|completed|abandoned)")
	registerFlagCompletion(cmd, "filter", completeLifecycleStates)
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
	refreshStatusSummaries(cmd, filtered, opts.refresh)

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
		row := map[string]any{
			"name":   s.Session.Name,
			"status": s.Session.Status,
			"branch": s.Worktree.Branch,
		}
		if s.PR.Number != 0 {
			row["pr_number"] = s.PR.Number
			row["pr_state"] = statusPRState(summaries[i])
			if s.PR.LastRefreshError != "" {
				row["pr_last_refresh_error"] = s.PR.LastRefreshError
			}
		}
		if s.SlicerWT.VM != "" {
			row["slicer_wt_vm"] = s.SlicerWT.VM
			row["slicer_wt_lease"] = string(s.SlicerWT.LeaseState)
		}
		rows = append(rows, row)
	}
	return writeJSONEnvelope(cmd, 1, rows)
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
	_, err := fmt.Fprintln(w, "NAME                           STATUS     BRANCH                         PR")
	if err != nil {
		return fmt.Errorf("status write header: %w", err)
	}
	for i := range summaries {
		s := summaries[i].state
		suffix := ""
		if s.SlicerWT.VM != "" {
			suffix = " [vm=" + s.SlicerWT.VM + " lease=" + string(s.SlicerWT.LeaseState) + "]"
		}
		_, err = fmt.Fprintf(w, "%-30s %-10s %-30s %s%s\n", s.Session.Name, s.Session.Status, s.Worktree.Branch, statusPRState(summaries[i]), suffix)
		if err != nil {
			return fmt.Errorf("status write row: %w", err)
		}
	}
	return nil
}

func refreshStatusSummaries(cmd *cobra.Command, summaries []sessionSummary, force bool) {
	warned := false
	for i := range summaries {
		if summaries[i].state.PR.Number == 0 {
			continue
		}
		err := withSessionLock(summaries[i].statePath, func() error {
			return refreshPRCacheForState(cmd.Context(), summaries[i].statePath, &summaries[i].state, prCacheRefreshOptions{
				Command: "status",
				Force:   force,
			})
		})
		if err != nil {
			summaries[i].prRefreshFailed = true
			warnPRRefreshOnce(cmd.Context(), &warned, "status", err)
		}
	}
}

func statusPRState(summary sessionSummary) string {
	if summary.state.PR.Number == 0 {
		return "-"
	}
	if summary.prRefreshFailed {
		return "?"
	}
	if summary.state.PR.State == "" {
		return "?"
	}
	return summary.state.PR.State
}
