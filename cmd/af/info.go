package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/session"
)

type infoOptions struct {
	root     *rootOptions
	ledgerN  int
	jsonMode bool
	refresh  bool
}

func newInfoCmd(opts *rootOptions) *cobra.Command {
	iOpts := &infoOptions{root: opts}
	cmd := &cobra.Command{
		Use:   "info [session]",
		Short: "Print detailed state for one workstream",
		Long:  "info reads state.toml for the named workstream (or the workstream detected from cwd) and prints a summary. With --json the summary is emitted as JSON; with --ledger N the last N ledger events are appended.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runInfo(cmd, iOpts, name)
		},
	}
	cmd.Flags().BoolVar(&iOpts.jsonMode, "json", false, "emit the info payload as JSON")
	cmd.Flags().IntVar(&iOpts.ledgerN, "ledger", 0, "include the last N ledger events")
	cmd.Flags().BoolVar(&iOpts.refresh, "refresh", false, "force-refresh cached PR state before rendering")
	return cmd
}

func runInfo(cmd *cobra.Command, opts *infoOptions, name string) error {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return err
	}

	state, err := session.ReadState(statePath)
	if err != nil {
		return fmt.Errorf("info: read state %s: %w", statePath, err)
	}
	prRefreshFailed := false
	if state.PR.Number != 0 {
		refreshErr := withSessionLock(statePath, func() error {
			return refreshPRCacheForState(cmd.Context(), statePath, &state, prCacheRefreshOptions{
				Command: "info",
				Force:   opts.refresh,
			})
		})
		if refreshErr != nil {
			prRefreshFailed = true
			warned := false
			warnPRRefreshOnce(cmd.Context(), &warned, "info", refreshErr)
		}
	}

	ledgerPath := filepath.Join(filepath.Dir(statePath), "ledger.jsonl")
	var events []session.Event
	if opts.ledgerN > 0 {
		events, err = session.ReadLedgerTail(cmd.Context(), ledgerPath, opts.ledgerN)
		if err != nil {
			return fmt.Errorf("info: read ledger: %w", err)
		}
	}

	if opts.jsonMode {
		return writeInfoJSON(cmd, state, events, prRefreshFailed)
	}
	return writeInfoText(cmd, state, events, prRefreshFailed)
}

func writeInfoJSON(cmd *cobra.Command, state session.State, events []session.Event, prRefreshFailed bool) error {
	payload := map[string]any{
		"session":   state.Session,
		"worktree":  state.Worktree,
		"slicer_wt": state.SlicerWT,
		"agents":    state.Agents,
		"pr":        infoPRPayload(state, prRefreshFailed),
		"events":    events,
	}
	return writeJSONEnvelope(cmd, 1, payload)
}

func writeInfoText(cmd *cobra.Command, state session.State, events []session.Event, prRefreshFailed bool) error {
	w := cmd.OutOrStdout()
	err := writeInfoCore(w, state, prRefreshFailed)
	if err != nil {
		return err
	}
	err = writeInfoAgents(w, state.Agents)
	if err != nil {
		return err
	}
	return writeInfoEvents(w, events)
}

func writeInfoCore(w io.Writer, state session.State, prRefreshFailed bool) error {
	lines := []string{
		"Session:   " + state.Session.Name,
		"Status:    " + state.Session.Status,
		"Branch:    " + state.Worktree.Branch,
		"Base:      " + state.Worktree.BaseBranch,
		"Worktree:  " + state.Worktree.Path,
		"Repo:      " + state.Worktree.RepoSlug,
		"Created:   " + state.Session.CreatedAt.Format("2006-01-02 15:04:05 MST"),
	}
	if state.PR.Number != 0 {
		stateLabel := state.PR.State
		if prRefreshFailed || stateLabel == "" {
			stateLabel = "?"
		}
		prLines := []string{
			"",
			"PR:",
			fmt.Sprintf("  Number:    %d", state.PR.Number),
			"  URL:       " + state.PR.URL,
			"  State:     " + stateLabel,
		}
		if state.PR.LastRefreshError != "" {
			prLines = append(prLines, "  Refresh error: "+state.PR.LastRefreshError)
		}
		lines = append(lines, prLines...)
	}
	if state.SlicerWT.VM != "" {
		wtLines := []string{
			"",
			"Slicer worktree:",
			"  VM:        " + state.SlicerWT.VM,
			"  Path:      " + state.SlicerWT.Path,
			"  Pushed:    " + state.SlicerWT.PushedAt.Format("2006-01-02 15:04:05 MST"),
		}
		if state.SlicerWT.PulledAt != nil {
			wtLines = append(wtLines, "  Pulled:    "+state.SlicerWT.PulledAt.Format("2006-01-02 15:04:05 MST"))
		}
		wtLines = append(wtLines, "  Lease:     "+string(state.SlicerWT.LeaseState))
		lines = append(lines, wtLines...)
	}
	for _, line := range lines {
		_, err := fmt.Fprintln(w, line)
		if err != nil {
			return fmt.Errorf("info write: %w", err)
		}
	}
	return nil
}

func writeInfoAgents(w io.Writer, agents []session.AgentState) error {
	if len(agents) == 0 {
		return nil
	}
	_, err := fmt.Fprintln(w, "Agents:")
	if err != nil {
		return fmt.Errorf("info write agents header: %w", err)
	}
	for i := range agents {
		_, err = fmt.Fprintf(w, "  - slot=%s provider=%s status=%s\n", agents[i].Slot, agents[i].Provider, agents[i].Status)
		if err != nil {
			return fmt.Errorf("info write agent: %w", err)
		}
	}
	return nil
}

func writeInfoEvents(w io.Writer, events []session.Event) error {
	if len(events) == 0 {
		return nil
	}
	_, err := fmt.Fprintln(w, "\nRecent events:")
	if err != nil {
		return fmt.Errorf("info write events header: %w", err)
	}
	for i := range events {
		_, err = fmt.Fprintf(w, "  %s %s\n", events[i].Timestamp.Format("2006-01-02 15:04:05"), events[i].Type)
		if err != nil {
			return fmt.Errorf("info write event: %w", err)
		}
	}
	return nil
}

func infoPRPayload(state session.State, refreshFailed bool) map[string]any {
	if state.PR.Number == 0 {
		return nil
	}
	stateLabel := state.PR.State
	if refreshFailed || stateLabel == "" {
		stateLabel = "?"
	}
	return map[string]any{
		"number":             state.PR.Number,
		"url":                state.PR.URL,
		"state":              stateLabel,
		"last_refreshed_at":  state.PR.LastRefreshedAt,
		"last_refresh_error": state.PR.LastRefreshError,
	}
}
