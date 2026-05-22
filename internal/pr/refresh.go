package pr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

// FetchTimeout caps a single gh pr view invocation.
const FetchTimeout = 5 * time.Second

// MaxRefreshErrorLength truncates last_refresh_error so state.toml
// stays readable. ADR-071 §"Refresh implementation".
const MaxRefreshErrorLength = 120

// State labels for the cached PR.state field. These mirror what
// af status / af info render in tables; the ADR-071 mapping table
// is the authoritative source.
const (
	StateOpen   = "open"
	StateDraft  = "draft"
	StateClosed = "closed"
	StateMerged = "merged"
)

// ErrNoPR reports that the cached PRState has no number; refresh is
// a no-op. Callers map this to EX_DATAERR (65) when invoked via
// af pr --refresh per ADR-068 §2.
var ErrNoPR = errors.New("pr: workstream has no pull request")

// ErrRefreshFailed wraps any error that prevented a successful refresh.
// Callers can errors.Is against it; the underlying message is also
// captured in PRState.LastRefreshError.
var ErrRefreshFailed = errors.New("pr: refresh failed")

// ErrEmptyRepoSlug reports that Refresh was called with no repo slug.
// Refresh cannot derive `gh pr view --repo` without one.
var ErrEmptyRepoSlug = errors.New("pr: empty repo slug")

// Result captures the outcome of one Refresh call.
type Result struct {
	// RefreshedAt is the time stamped into PRState.LastRefreshedAt
	// when the fetch succeeded. Zero on failure.
	RefreshedAt time.Time
	// Old is the pre-refresh State value (may be empty).
	Old string
	// New is the post-refresh State value. Equal to Old when nothing
	// changed.
	New string
	// Skipped reports that the TTL is still valid; no network call
	// was made.
	Skipped bool
	// Changed reports that Old != New (a flip happened).
	Changed bool
}

// Options configures Refresh.
type Options struct {
	// Runner executes the gh CLI. Tests substitute sandbox.Runner
	// fakes; production wires sandbox.ExecRunner.
	Runner sandbox.Runner
	// Now overrides the clock for deterministic tests. Defaults to
	// time.Now when nil.
	Now func() time.Time
	// RepoSlug, e.g. "kakkoyun/af". Required.
	RepoSlug string
	// TTL is the cache validity window. When the existing
	// LastRefreshedAt is within now-TTL, Refresh returns Skipped.
	// Zero TTL forces an unconditional refresh (Skipped=false).
	TTL time.Duration
	// Force overrides the TTL check; the fetch is unconditional.
	Force bool
}

// Refresh consults gh pr view to update the cached PR state on prState
// per ADR-071. The PRState argument is mutated in place; callers are
// expected to write the updated session.State back to disk via
// session.WriteState. PRState.LastRefreshError is cleared on success
// and populated on failure.
func Refresh(ctx context.Context, prState *session.PRState, opts Options) (Result, error) {
	if prState == nil {
		return Result{}, fmt.Errorf("%w: nil PRState", ErrRefreshFailed)
	}
	if prState.Number == 0 {
		return Result{}, ErrNoPR
	}
	if opts.RepoSlug == "" {
		return Result{}, fmt.Errorf("%w: %w", ErrRefreshFailed, ErrEmptyRepoSlug)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	if !opts.Force && !needsRefresh(prState, opts.TTL, now()) {
		return Result{Old: prState.State, New: prState.State, Skipped: true}, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, FetchTimeout)
	defer cancel()
	newState, fetchErr := fetchState(fetchCtx, opts.Runner, opts.RepoSlug, prState.Number)
	if fetchErr != nil {
		prState.LastRefreshError = truncate(fetchErr.Error(), MaxRefreshErrorLength)
		return Result{Old: prState.State, New: prState.State}, fmt.Errorf("%w: %w", ErrRefreshFailed, fetchErr)
	}

	stamp := now().UTC()
	old := prState.State
	prState.State = newState
	prState.LastRefreshedAt = &stamp
	prState.LastRefreshError = ""

	return Result{
		Old:         old,
		New:         newState,
		RefreshedAt: stamp,
		Changed:     old != newState,
	}, nil
}

// needsRefresh reports whether the TTL has expired (or never refreshed).
func needsRefresh(prState *session.PRState, ttl time.Duration, now time.Time) bool {
	if ttl <= 0 {
		return true
	}
	if prState.LastRefreshedAt == nil || prState.LastRefreshedAt.IsZero() {
		return true
	}
	return now.Sub(*prState.LastRefreshedAt) > ttl
}

// ghResponse is the JSON subset af consumes from `gh pr view`.
type ghResponse struct {
	MergedAt *time.Time `json:"mergedAt"`
	ClosedAt *time.Time `json:"closedAt"`
	State    string     `json:"state"`
	IsDraft  bool       `json:"isDraft"`
}

func fetchState(ctx context.Context, runner sandbox.Runner, repoSlug string, number int) (string, error) {
	if runner == nil {
		runner = sandbox.ExecRunner{}
	}
	args := []string{
		"pr", "view", strconv.Itoa(number),
		"--repo", repoSlug,
		"--json", "state,isDraft,mergedAt,closedAt",
	}
	output, err := runner.Run(ctx, sandbox.Command{Name: "gh", Args: args})
	if err != nil {
		return "", fmt.Errorf("gh pr view: %w", err)
	}
	var resp ghResponse
	err = json.Unmarshal(output, &resp)
	if err != nil {
		return "", fmt.Errorf("parse gh response: %w", err)
	}
	return mapState(resp), nil
}

func mapState(resp ghResponse) string {
	switch strings.ToUpper(resp.State) {
	case "OPEN":
		if resp.IsDraft {
			return StateDraft
		}
		return StateOpen
	case "MERGED":
		return StateMerged
	case "CLOSED":
		if resp.MergedAt != nil {
			return StateMerged
		}
		return StateClosed
	default:
		// Unknown state — preserve the upstream label verbatim for
		// diagnosis.
		return strings.ToLower(resp.State)
	}
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if limit <= 1 {
		return s[:limit]
	}
	return s[:limit-1] + "…"
}
