package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

// ErrAgentSlotExists reports a duplicate slot on AgentAdd.
var ErrAgentSlotExists = errors.New("agent slot already exists")

// ErrAgentSlotMissing reports an unknown slot on AgentStop.
var ErrAgentSlotMissing = errors.New("agent slot not found")

// AgentAddOptions configures AgentAdd.
type AgentAddOptions struct {
	Now       time.Time
	StatePath string
	Slot      string
	Provider  string
}

// AgentAddDeps wires AgentAdd to its external collaborators.
type AgentAddDeps struct {
	Git   git.Runner
	Mux   mux.Multiplexer
	Agent agent.Agent
}

// AgentAdd registers a new agent slot and, for non-primary slots,
// creates the sibling sub-worktree on the sibling branch per ADR-038.
//
// It returns the loaded state and the computed sub-worktree plan. The
// plan is computed for every slot (and its path recorded in state),
// but the worktree itself is only created for non-primary slots.
func AgentAdd(ctx context.Context, deps AgentAddDeps, opts AgentAddOptions) (session.State, git.SubWorktreePlan, error) {
	state, err := session.ReadState(opts.StatePath)
	if err != nil {
		return state, git.SubWorktreePlan{}, fmt.Errorf("agent add: read state: %w", err)
	}
	for i := range state.Agents {
		if state.Agents[i].Slot == opts.Slot {
			return state, git.SubWorktreePlan{}, fmt.Errorf("agent add: %w: %s", ErrAgentSlotExists, opts.Slot)
		}
	}

	plan := git.PlanSubWorktree(git.WorktreePlan{
		Path:   state.Worktree.Path,
		Branch: state.Worktree.Branch,
	}, opts.Slot)

	if opts.Slot != primaryAgentSlot {
		_, err = deps.Git.Run(ctx, state.Worktree.GitRoot, "worktree", "add", "-b", plan.Branch, plan.Path, state.Worktree.Branch)
		if err != nil {
			return state, plan, fmt.Errorf("agent add: git worktree add: %w", err)
		}
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state.Agents = append(state.Agents, session.AgentState{
		Slot:        opts.Slot,
		Provider:    opts.Provider,
		Status:      string(Active),
		CreatedAt:   now,
		SubWorktree: plan.Path,
		SubBranch:   plan.Branch,
		SessionIDs:  []string{},
	})

	err = session.WriteState(opts.StatePath, state) //nolint:forbidigo // git worktree add already ran between ReadState and here for non-primary slots; can't collapse into session.Update.
	if err != nil {
		return state, plan, fmt.Errorf("agent add: write state: %w", err)
	}

	ledgerPath := filepath.Join(filepath.Dir(opts.StatePath), "ledger.jsonl")
	err = session.AppendEvent(ledgerPath, session.Event{
		Timestamp: now,
		Type:      "agent_added",
		Fields: map[string]any{
			"slot":     opts.Slot,
			"provider": opts.Provider,
		},
	})
	if err != nil {
		return state, plan, fmt.Errorf("agent add: append event: %w", err)
	}
	return state, plan, nil
}

// AgentStopOptions configures AgentStop.
type AgentStopOptions struct {
	Now            time.Time
	StatePath      string
	Slot           string
	RemoveWorktree bool
}

// AgentStop marks a slot stopped. With RemoveWorktree it also removes
// the sub-worktree directory and deletes the sub-branch.
func AgentStop(ctx context.Context, runner git.Runner, opts AgentStopOptions) error {
	state, err := session.ReadState(opts.StatePath)
	if err != nil {
		return fmt.Errorf("agent stop: read state: %w", err)
	}
	idx := findAgentSlot(state, opts.Slot)
	if idx < 0 {
		return fmt.Errorf("agent stop: %w: %s", ErrAgentSlotMissing, opts.Slot)
	}
	err = maybeRemoveSubWorktree(ctx, runner, state, idx, opts.RemoveWorktree)
	if err != nil {
		return err
	}
	if opts.RemoveWorktree {
		// The worktree and branch are gone; clear them from state so a
		// later `af done` does not re-run `git worktree remove` on the
		// already-removed path (real git fatals and aborts the done).
		state.Agents[idx].SubWorktree = ""
		state.Agents[idx].SubBranch = ""
	}
	return writeAgentStopped(state, idx, opts)
}

func findAgentSlot(state session.State, slot string) int {
	for i := range state.Agents {
		if state.Agents[i].Slot == slot {
			return i
		}
	}
	return -1
}

func maybeRemoveSubWorktree(ctx context.Context, runner git.Runner, state session.State, idx int, remove bool) error {
	if !remove || state.Agents[idx].SubWorktree == "" {
		return nil
	}
	_, err := runner.Run(ctx, state.Worktree.GitRoot, "worktree", "remove", state.Agents[idx].SubWorktree, "--force")
	if err != nil {
		return fmt.Errorf("agent stop: git worktree remove: %w", err)
	}
	_, _ = runner.Run(ctx, state.Worktree.GitRoot, "branch", "-D", state.Agents[idx].SubBranch) //nolint:errcheck // Best-effort branch delete.
	return nil
}

func writeAgentStopped(state session.State, idx int, opts AgentStopOptions) error {
	state.Agents[idx].Status = "stopped"
	err := session.WriteState(opts.StatePath, state) //nolint:forbidigo // maybeRemoveSubWorktree's git worktree remove already ran between ReadState and here; can't collapse into session.Update.
	if err != nil {
		return fmt.Errorf("agent stop: write state: %w", err)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ledgerPath := filepath.Join(filepath.Dir(opts.StatePath), "ledger.jsonl")
	err = session.AppendEvent(ledgerPath, session.Event{
		Timestamp: now,
		Type:      "agent_stopped",
		Fields: map[string]any{
			"slot":            opts.Slot,
			"remove_worktree": opts.RemoveWorktree,
		},
	})
	if err != nil {
		return fmt.Errorf("agent stop: append event: %w", err)
	}
	return nil
}

// SortedAgents returns the agents from state sorted by slot for stable
// display.
func SortedAgents(state session.State) []session.AgentState {
	out := append([]session.AgentState(nil), state.Agents...)
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}
