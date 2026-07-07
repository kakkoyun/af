package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

var (
	// ErrPullNoLease reports a pull on a workstream that has no slicer wt lease.
	ErrPullNoLease = errors.New("pull: workstream has no active slicer worktree lease")
	// ErrPullAlreadyPulled reports a pull on a workstream whose lease is already pulled.
	ErrPullAlreadyPulled = errors.New("pull: lease already pulled")
	// ErrPullDiscarded reports a pull on a workstream whose lease was forcibly discarded.
	ErrPullDiscarded = errors.New("pull: lease was discarded; pull is not meaningful")
	// ErrPullFailed wraps a slicer wt pull execution failure.
	ErrPullFailed = errors.New("pull: slicer wt pull failed")
)

// PullOptions configures Pull.
type PullOptions struct { //nolint:govet // Field grouping prioritises readability over packing.
	// StatePath is the absolute path to state.toml for the target workstream.
	StatePath string
	// Now overrides the pull timestamp. Zero means time.Now().
	Now time.Time
}

// PullDeps wires Pull to its external collaborators.
type PullDeps struct {
	// Runner executes the slicer CLI commands.
	Runner sandbox.Runner
}

// PullResult describes the outcome of a successful pull.
type PullResult struct { //nolint:govet // Field grouping prioritises readability over packing.
	SessionName string
	VM          string
	PulledAt    time.Time
}

// Pull runs `slicer wt pull` for the named workstream, updates the
// lease state to `pulled`, and returns the pull result per ADR-065.
func Pull(ctx context.Context, deps PullDeps, opts PullOptions) (PullResult, error) {
	state, err := session.ReadState(opts.StatePath)
	if err != nil {
		return PullResult{}, fmt.Errorf("pull: read state: %w", err)
	}

	switch state.SlicerWT.LeaseState {
	case "":
		return PullResult{}, ErrPullNoLease
	case session.SlicerWTLeasePulled:
		return PullResult{}, ErrPullAlreadyPulled
	case session.SlicerWTLeaseDiscarded:
		return PullResult{}, ErrPullDiscarded
	case session.SlicerWTLeaseHeldByVM:
		// proceed
	}

	res, err := sandbox.WTPull(ctx, deps.Runner, sandbox.WTPullOptions{
		VM:           state.SlicerWT.VM,
		WorktreePath: state.SlicerWT.Path,
	})
	if err != nil {
		return PullResult{}, fmt.Errorf("%w: %w", ErrPullFailed, err)
	}

	now := opts.Now
	if now.IsZero() {
		now = res.PulledAt
	}
	state.SlicerWT.LeaseState = session.SlicerWTLeasePulled
	state.SlicerWT.PulledAt = &now

	err = session.WriteState(opts.StatePath, state) //nolint:forbidigo // sandbox.WTPull's slicer subprocess call already ran between ReadState and here; can't collapse into session.Update.
	if err != nil {
		return PullResult{}, fmt.Errorf("pull: write state: %w", err)
	}

	return PullResult{
		SessionName: state.Session.Name,
		VM:          state.SlicerWT.VM,
		PulledAt:    now,
	}, nil
}
