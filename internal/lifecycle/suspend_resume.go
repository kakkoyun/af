package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

var (
	// ErrLifecycleTransition reports an invalid state-machine transition.
	ErrLifecycleTransition = errors.New("invalid lifecycle transition")
	// ErrSuspendLeasedToVM reports that the host worktree is still held by a slicer VM.
	ErrSuspendLeasedToVM = errors.New("suspend: workstream is still leased to a slicer VM")
)

// SuspendOptions configures SuspendWorkstream.
type SuspendOptions struct {
	Now       time.Time
	StatePath string
	// Force allows suspending even when the worktree is leased to a slicer VM.
	// It sets lease_state=discarded.
	Force bool
}

// SuspendWorkstream transitions the workstream from active to suspended.
//
// The multiplexer is intentionally untouched here \u2014 tmux sessions persist
// across suspend by design (ADR-046). Only state and the ledger move.
func SuspendWorkstream(_ context.Context, opts SuspendOptions) (session.State, error) {
	state, err := session.ReadState(opts.StatePath)
	if err != nil {
		return state, fmt.Errorf("suspend: read state: %w", err)
	}
	next, ok := Apply(State(state.Session.Status), Suspend)
	if !ok {
		return state, fmt.Errorf("suspend: %w from %s", ErrLifecycleTransition, state.Session.Status)
	}
	state, err = checkAndClearLease(state, opts.Force, ErrSuspendLeasedToVM)
	if err != nil {
		return state, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state.Session.Status = string(next)
	state.Session.SuspendedAt = &now

	err = session.WriteState(opts.StatePath, state)
	if err != nil {
		return state, fmt.Errorf("suspend: write state: %w", err)
	}
	err = session.AppendEvent(filepath.Join(filepath.Dir(opts.StatePath), "ledger.jsonl"), session.Event{
		Timestamp: now,
		Type:      "suspended",
		Fields:    map[string]any{"session_id": state.Session.ID},
	})
	if err != nil {
		return state, fmt.Errorf("suspend: append event: %w", err)
	}
	return state, nil
}

// ResumeOptions configures ResumeWorkstream.
type ResumeOptions struct {
	Now       time.Time
	StatePath string
	Bare      bool
}

// ResumeDeps wires ResumeWorkstream to its external collaborators.
type ResumeDeps struct {
	Mux mux.Multiplexer
}

// ResumeWorkstream transitions the workstream back to active. It also
// re-creates the tmux session via the multiplexer if the prior session
// is no longer alive. Bare=true skips the multiplexer entirely.
func ResumeWorkstream(ctx context.Context, deps ResumeDeps, opts ResumeOptions) (session.State, error) {
	state, err := session.ReadState(opts.StatePath)
	if err != nil {
		return state, fmt.Errorf("resume: read state: %w", err)
	}
	next, ok := Apply(State(state.Session.Status), Resume)
	if !ok {
		return state, fmt.Errorf("resume: %w from %s", ErrLifecycleTransition, state.Session.Status)
	}

	maybeRespawnTmux(ctx, deps.Mux, state, opts.Bare)

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	state.Session.Status = string(next)
	state.Session.SuspendedAt = nil

	return persistResume(state, opts, now)
}

func maybeRespawnTmux(ctx context.Context, multiplexer mux.Multiplexer, state session.State, bare bool) {
	if bare || multiplexer == nil || state.Execution.TmuxSession == "" {
		return
	}
	exists, existsErr := multiplexer.SessionExists(ctx, state.Execution.TmuxSession)
	if existsErr != nil {
		exists = false
	}
	if exists {
		return
	}
	_ = multiplexer.CreateSession(ctx, state.Execution.TmuxSession, state.Worktree.Path) //nolint:errcheck // Best-effort tmux respawn; failure surfaces in next af info.
}

func persistResume(state session.State, opts ResumeOptions, now time.Time) (session.State, error) {
	err := session.WriteState(opts.StatePath, state)
	if err != nil {
		return state, fmt.Errorf("resume: write state: %w", err)
	}
	err = session.AppendEvent(filepath.Join(filepath.Dir(opts.StatePath), "ledger.jsonl"), session.Event{
		Timestamp: now,
		Type:      "resumed",
		Fields:    map[string]any{"session_id": state.Session.ID},
	})
	if err != nil {
		return state, fmt.Errorf("resume: append event: %w", err)
	}
	return state, nil
}
