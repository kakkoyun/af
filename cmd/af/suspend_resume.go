package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kakkoyun/af/internal/lifecycle"
	"github.com/kakkoyun/af/internal/mux"
	"github.com/kakkoyun/af/internal/session"
)

func newSuspendCmd(_ *rootOptions) *cobra.Command {
	var (
		force   bool
		discard bool
	)
	cmd := &cobra.Command{
		Use:   "suspend [session]",
		Short: "Suspend a workstream (state.toml records suspension; tmux stays alive)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
			if err != nil {
				return err
			}
			var state session.State
			err = withSessionLock(statePath, func() error {
				preState, lockedErr := readStateForAutoSync(cmd.Context(), statePath)
				if lockedErr != nil {
					return fmt.Errorf("suspend: %w", lockedErr)
				}
				lockedErr = autoSyncBeforeTeardown(cmd, preState, statePath, discard)
				if lockedErr != nil {
					return fmt.Errorf("suspend: %w", lockedErr)
				}
				state, lockedErr = lifecycle.SuspendWorkstream(cmd.Context(), lifecycle.SuspendOptions{
					StatePath: statePath,
					Force:     force,
				})
				if lockedErr != nil {
					return fmt.Errorf("suspend: %w", lockedErr)
				}
				return nil
			})
			if err != nil {
				return err
			}
			if state.SlicerWT.VM != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: slicer VM %s lease is %s\n", state.SlicerWT.VM, state.SlicerWT.LeaseState) //nolint:errcheck // Informational only.
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "workstream %s -> %s\n", state.Session.Name, state.Session.Status)
			if err != nil {
				return fmt.Errorf("suspend write: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force suspend even when worktree is leased to a slicer VM (sets lease_state=discarded)")
	cmd.Flags().BoolVar(&discard, "discard", false, "discard agent session transcripts; skip ADR-067 automatic sync before VM teardown")
	return cmd
}

// newResumeMux constructs the multiplexer used by `af resume` for both
// the ADR-046 tmux respawn and the post-resume attach. Tests override
// this seam with a *mux.FakeMultiplexer so they can assert on recorded
// Attach calls without touching a real tmux server.
//
//nolint:gochecknoglobals // Test seam for `af resume`'s attach mechanism.
var newResumeMux = func() mux.Multiplexer { return mux.NewTmux() }

func newResumeCmd(_ *rootOptions) *cobra.Command {
	var bare bool
	cmd := &cobra.Command{
		Use:   "resume [session]",
		Short: "Resume a suspended workstream, attaching to its tmux session",
		Long: "resume restores a suspended workstream to active and attaches to its tmux session, " +
			"respawning it first if it died. Running resume on a workstream that is already active " +
			"just attaches instead of erroring. --bare skips both the tmux respawn and the attach.",
		Example: "  af resume demo\n" +
			"  af resume demo --bare   # skip the tmux respawn/attach; just flip state back to active",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return runResume(cmd, name, bare)
		},
	}
	cmd.Flags().BoolVar(&bare, "bare", false, "skip tmux respawn and attach")
	return cmd
}

func runResume(cmd *cobra.Command, name string, bare bool) error {
	statePath, err := resolveLifecycleStatePathForCommand(cmd, name)
	if err != nil {
		return err
	}
	multiplexer := newResumeMux()
	var (
		state         session.State
		alreadyActive bool
	)
	err = withSessionLock(statePath, func() error {
		preState, lockedErr := session.ReadState(statePath)
		if lockedErr != nil {
			return fmt.Errorf("resume: %w", lockedErr)
		}
		if preState.Session.Status == string(lifecycle.Active) {
			alreadyActive = true
			state = preState
			return nil
		}
		state, lockedErr = lifecycle.ResumeWorkstream(cmd.Context(), lifecycle.ResumeDeps{Mux: multiplexer}, lifecycle.ResumeOptions{
			StatePath: statePath,
			Bare:      bare,
		})
		if lockedErr != nil {
			return fmt.Errorf("resume: %w", lockedErr)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if alreadyActive {
		return handleResumeAlreadyActive(cmd, multiplexer, state, bare)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "workstream %s -> %s\n", state.Session.Name, state.Session.Status)
	if err != nil {
		return fmt.Errorf("resume write: %w", err)
	}
	if bare {
		return nil
	}
	return attachResumeSession(cmd, multiplexer, state.Execution.TmuxSession)
}

// handleResumeAlreadyActive implements issue #23: `af resume` on a
// workstream that is already active must not hit the lifecycle FSM's
// invalid-transition error. Instead it reports the no-op and either
// attaches (default) or, with --bare, prints the manual attach hint and
// leaves the terminal alone.
func handleResumeAlreadyActive(cmd *cobra.Command, multiplexer mux.Multiplexer, state session.State, bare bool) error {
	_, err := fmt.Fprintf(cmd.ErrOrStderr(), "session '%s' is already active\n", state.Session.Name)
	if err != nil {
		return fmt.Errorf("resume write: %w", err)
	}
	if bare {
		_, err = fmt.Fprintf(cmd.ErrOrStderr(), "to attach: tmux attach -t %s\n", state.Execution.TmuxSession)
		if err != nil {
			return fmt.Errorf("resume write: %w", err)
		}
		return nil
	}
	return attachResumeSession(cmd, multiplexer, state.Execution.TmuxSession)
}

// attachResumeSession attaches to tmuxSession using the ADR-040
// multiplexer seam. It is the one attach mechanism `af resume` and `af
// create` both call (issue #21/#23); a missing session name or nil
// multiplexer is a silent no-op rather than an error, matching bare
// workstreams that never had a tmux session in the first place.
func attachResumeSession(cmd *cobra.Command, multiplexer mux.Multiplexer, tmuxSession string) error {
	if multiplexer == nil || tmuxSession == "" {
		return nil
	}
	err := multiplexer.Attach(cmd.Context(), tmuxSession)
	if err != nil {
		return fmt.Errorf("resume: attach: %w", err)
	}
	return nil
}
