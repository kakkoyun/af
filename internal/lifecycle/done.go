package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/kakkoyun/af/internal/git"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

var (
	// ErrDoneAlreadyTerminal reports a Done on a workstream already in a terminal state.
	ErrDoneAlreadyTerminal = errors.New("workstream already terminal")
	// ErrDoneLeasedToVM reports that the host worktree is still held by a slicer VM.
	ErrDoneLeasedToVM = errors.New("done: workstream is still leased to a slicer VM")
)

// DoneOptions configures Done.
type DoneOptions struct {
	Now        time.Time
	StatePath  string
	ArchiveDir string
	// Force treats the workstream as Abandoned and skips safety checks.
	// When the worktree is leased to a VM, Force sets lease_state=discarded.
	Force bool
}

// DoneDeps wires Done to its external collaborators.
type DoneDeps struct {
	Git git.Runner
	Mux mux.Multiplexer
}

// checkAndClearLease guards against starting a destructive operation while the
// host worktree is leased to a slicer VM. If force is true it sets the lease
// state to discarded; otherwise it returns baseErr with a hint.
func checkAndClearLease(state session.State, force bool, baseErr error) (session.State, error) {
	if !state.IsLeasedToVM() {
		return state, nil
	}
	if !force {
		return state, fmt.Errorf("%w (vm=%s); run `af pull` first or pass --force", baseErr, state.SlicerWT.VM)
	}
	state.SlicerWT.LeaseState = session.SlicerWTLeaseDiscarded
	return state, nil
}

// FinishWorkstream cleans up the workstream: kills the tmux session,
// removes the git worktree (and sub-worktrees), moves the session dir
// into the archive, and appends a terminal lifecycle event.
//
// Force=true treats the workstream as Abandoned rather than Completed
// and skips the "merged into base" gate on sub-worktree removal.
func FinishWorkstream(ctx context.Context, deps DoneDeps, opts DoneOptions) (session.State, error) {
	state, err := session.ReadState(opts.StatePath)
	if err != nil {
		return state, fmt.Errorf("done: read state: %w", err)
	}
	if IsTerminal(State(state.Session.Status)) {
		return state, fmt.Errorf("done: cannot finish a %s workstream (it has already finished; see 'af retro'): %w", state.Session.Status, ErrDoneAlreadyTerminal)
	}
	state, err = checkAndClearLease(state, opts.Force, ErrDoneLeasedToVM)
	if err != nil {
		return state, err
	}

	killMuxSession(ctx, deps.Mux, state.Execution.TmuxSession)

	err = removeSubWorktrees(ctx, deps.Git, state, opts.Force)
	if err != nil {
		return state, err
	}

	_, err = deps.Git.Run(ctx, state.Worktree.GitRoot, "worktree", "remove", state.Worktree.Path, "--force")
	if err != nil && !opts.Force {
		return state, fmt.Errorf("done: remove primary worktree: %w", err)
	}

	finalState, eventType := terminalLabels(opts.Force)
	state.Session.Status = string(finalState)
	err = persistDone(state, opts, eventType)
	if err != nil {
		return state, err
	}

	if opts.ArchiveDir != "" {
		err = archiveSessionDir(filepath.Dir(opts.StatePath), opts.ArchiveDir, state.Session.Name)
		if err != nil {
			return state, fmt.Errorf("done: archive: %w", err)
		}
	}

	return state, nil
}

func killMuxSession(ctx context.Context, multiplexer mux.Multiplexer, name string) {
	if multiplexer == nil || name == "" {
		return
	}
	_ = multiplexer.KillSession(ctx, name) //nolint:errcheck // best-effort tmux teardown
}

func removeSubWorktrees(ctx context.Context, runner git.Runner, state session.State, force bool) error {
	for i := range state.Agents {
		subWorktree := state.Agents[i].SubWorktree
		if subWorktree == "" {
			continue
		}
		_, err := runner.Run(ctx, state.Worktree.GitRoot, "worktree", "remove", subWorktree, "--force")
		if err != nil && !force {
			return fmt.Errorf("done: remove sub-worktree %s: %w", subWorktree, err)
		}
	}
	return nil
}

func terminalLabels(force bool) (State, string) {
	if force {
		return Abandoned, "abandoned"
	}
	return Completed, "completed"
}

func persistDone(state session.State, opts DoneOptions, eventType string) error {
	err := session.WriteState(opts.StatePath, state) //nolint:forbidigo // Done pipeline: tmux kill + git worktree remove(s) already ran between ReadState and here; can't collapse into session.Update.
	if err != nil {
		return fmt.Errorf("done: write state: %w", err)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ledgerPath := filepath.Join(filepath.Dir(opts.StatePath), "ledger.jsonl")
	err = session.AppendEvent(ledgerPath, session.Event{
		Timestamp: now,
		Type:      eventType,
		Fields: map[string]any{
			"session_id": state.Session.ID,
		},
	})
	if err != nil {
		return fmt.Errorf("done: append event: %w", err)
	}
	return nil
}

func archiveSessionDir(sessionDir, archiveRoot, name string) error {
	err := os.MkdirAll(archiveRoot, stateDirPerm)
	if err != nil {
		return fmt.Errorf("create archive root: %w", err)
	}
	target := filepath.Join(archiveRoot, name)
	err = os.Rename(sessionDir, target)
	if err != nil {
		return fmt.Errorf("move %s -> %s: %w", sessionDir, target, err)
	}
	// ADR-068 §4 (issue #16): the lazily-created .af.lock must not
	// survive into the archive — nothing will ever hold it again.
	// Removal happens after the rename so the caller's flock (done runs
	// under withSessionLock) stays valid on the open descriptor; a
	// concurrent locker retrying against the old sessions/<name> path is
	// stopped by LockFile's ghost-dir guard once the rename lands.
	err = os.Remove(filepath.Join(target, session.LockFileName))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove archived lock file: %w", err)
	}
	return nil
}
