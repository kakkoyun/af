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

// transitionHint returns a short, user-facing suggestion for recovering
// from a blocked lifecycle transition, or "" when none applies. It backs
// the "cannot <verb> a <status> workstream (<hint>)" error style (issue
// #25 Part 4.3a).
func transitionHint(verb string, status State) string {
	switch {
	case verb == "suspend" && status == Suspended:
		return "it is already suspended"
	case verb == "resume" && status == Active:
		return "it is already active"
	case status == Completed || status == Abandoned:
		return "it has already finished; see 'af retro'"
	default:
		return ""
	}
}

// transitionBlockedError builds the ErrLifecycleTransition-wrapping error
// for a verb that cannot apply to a workstream currently in status.
func transitionBlockedError(verb string, status State) error {
	hint := transitionHint(verb, status)
	if hint == "" {
		return fmt.Errorf("cannot %s a %s workstream: %w", verb, status, ErrLifecycleTransition)
	}
	return fmt.Errorf("cannot %s a %s workstream (%s): %w", verb, status, hint, ErrLifecycleTransition)
}

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
		return state, fmt.Errorf("suspend: %w", transitionBlockedError("suspend", State(state.Session.Status)))
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

	err = session.WriteState(opts.StatePath, state) //nolint:forbidigo // Step of the suspend pipeline; the caller's withSessionLock closure runs autoSyncBeforeTeardown (a sandbox/network side effect) around this step, so it can't collapse into one Update.
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
		return state, fmt.Errorf("resume: %w", transitionBlockedError("resume", State(state.Session.Status)))
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
	err := session.WriteState(opts.StatePath, state) //nolint:forbidigo // maybeRespawnTmux's mux side effect already ran between ReadState and here; can't collapse into session.Update.
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
