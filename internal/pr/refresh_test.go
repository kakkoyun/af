package pr_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/pr"
	"github.com/kakkoyun/af/internal/sandbox"
	"github.com/kakkoyun/af/internal/session"
)

// fakeRunner returns Output on every Run call regardless of the
// command. Err overrides Output when non-nil.
type fakeRunner struct { //nolint:govet // Test-only struct; readability over packing.
	Output []byte
	Err    error
}

// Test constants for the merged/open state labels.
const (
	testStateOpen   = "open"
	testStateMerged = "merged"
	testRepoSlug    = "kakkoyun/af"
)

func (f fakeRunner) Run(_ context.Context, _ sandbox.Command) ([]byte, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Output, nil
}

var errFakeNetwork = errors.New("network: connection refused")

func fixedNow() time.Time {
	return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
}

func TestRefresh_NoPRReturnsErrNoPR(t *testing.T) {
	t.Parallel()
	state := &session.PRState{Number: 0}
	_, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{},
		RepoSlug: testRepoSlug,
		TTL:      10 * time.Minute,
	})
	if !errors.Is(err, pr.ErrNoPR) {
		t.Errorf("want ErrNoPR, got %v", err)
	}
}

func TestRefresh_EmptyRepoSlugFails(t *testing.T) {
	t.Parallel()
	state := &session.PRState{Number: 1}
	_, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner: fakeRunner{},
		TTL:    10 * time.Minute,
	})
	if !errors.Is(err, pr.ErrEmptyRepoSlug) {
		t.Errorf("want ErrEmptyRepoSlug, got %v", err)
	}
}

func TestRefresh_FreshCacheSkips(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	last := now.Add(-1 * time.Minute) // within 10m TTL.
	state := &session.PRState{Number: 7, State: testStateOpen, LastRefreshedAt: &last}

	got, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Output: []byte(`{"state":"MERGED","isDraft":false,"mergedAt":"2026-05-22T12:00:00Z"}`)},
		RepoSlug: testRepoSlug,
		TTL:      10 * time.Minute,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !got.Skipped {
		t.Errorf("Skipped = false, want true (within TTL)")
	}
	if state.State != testStateOpen {
		t.Errorf("state mutated despite skip; got %q", state.State)
	}
}

func TestRefresh_ExpiredTTLRefreshes(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	last := now.Add(-15 * time.Minute) // older than 10m TTL.
	state := &session.PRState{Number: 7, State: testStateOpen, LastRefreshedAt: &last}

	got, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Output: []byte(`{"state":"MERGED","isDraft":false,"mergedAt":"2026-05-22T11:55:00Z"}`)},
		RepoSlug: testRepoSlug,
		TTL:      10 * time.Minute,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.Skipped {
		t.Errorf("Skipped = true, want false (TTL expired)")
	}
	if !got.Changed {
		t.Errorf("Changed = false, want true (open → merged)")
	}
	if got.Old != testStateOpen || got.New != testStateMerged {
		t.Errorf("Old=%q New=%q, want open→merged", got.Old, got.New)
	}
	if state.State != testStateMerged {
		t.Errorf("state.State = %q, want merged", state.State)
	}
	if state.LastRefreshedAt == nil || !state.LastRefreshedAt.Equal(now) {
		t.Errorf("LastRefreshedAt = %v, want %v", state.LastRefreshedAt, now)
	}
}

func TestRefresh_ForceBypassesTTL(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	last := now.Add(-1 * time.Minute) // would normally skip.
	state := &session.PRState{Number: 7, State: testStateOpen, LastRefreshedAt: &last}

	got, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Output: []byte(`{"state":"OPEN","isDraft":true}`)},
		RepoSlug: testRepoSlug,
		TTL:      10 * time.Minute,
		Force:    true,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.Skipped {
		t.Errorf("Force should bypass TTL; got Skipped=true")
	}
	if state.State != "draft" {
		t.Errorf("state.State = %q, want draft (OPEN+isDraft)", state.State)
	}
}

func TestRefresh_ZeroTTLAlwaysRefreshes(t *testing.T) {
	t.Parallel()
	state := &session.PRState{Number: 7, State: testStateOpen}
	_, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Output: []byte(`{"state":"CLOSED"}`)},
		RepoSlug: testRepoSlug,
		TTL:      0,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if state.State != "closed" {
		t.Errorf("state.State = %q, want closed", state.State)
	}
}

func TestRefresh_NeverRefreshedAlwaysFetches(t *testing.T) {
	t.Parallel()
	state := &session.PRState{Number: 7, State: ""}
	got, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Output: []byte(`{"state":"OPEN","isDraft":false}`)},
		RepoSlug: testRepoSlug,
		TTL:      10 * time.Minute,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.Old != "" || got.New != testStateOpen {
		t.Errorf("Old=%q New=%q, want ''→open", got.Old, got.New)
	}
}

func TestRefresh_GhFailureRecordsErrorAndPreservesState(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	last := now.Add(-1 * time.Hour)
	state := &session.PRState{Number: 7, State: testStateOpen, LastRefreshedAt: &last}

	_, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Err: errFakeNetwork},
		RepoSlug: testRepoSlug,
		TTL:      10 * time.Minute,
		Now:      fixedNow,
	})
	if !errors.Is(err, pr.ErrRefreshFailed) {
		t.Fatalf("want ErrRefreshFailed, got %v", err)
	}
	if state.State != testStateOpen {
		t.Errorf("state.State changed after failed refresh; got %q", state.State)
	}
	if state.LastRefreshError == "" {
		t.Errorf("LastRefreshError should be populated; got empty")
	}
	if state.LastRefreshedAt == nil || !state.LastRefreshedAt.Equal(last) {
		t.Errorf("LastRefreshedAt advanced despite failure; got %v", state.LastRefreshedAt)
	}
}

func TestRefresh_SuccessClearsPreviousError(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	last := now.Add(-1 * time.Hour)
	state := &session.PRState{
		Number:           7,
		State:            testStateOpen,
		LastRefreshedAt:  &last,
		LastRefreshError: "earlier transient failure",
	}
	_, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Output: []byte(`{"state":"OPEN","isDraft":false}`)},
		RepoSlug: testRepoSlug,
		TTL:      10 * time.Minute,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if state.LastRefreshError != "" {
		t.Errorf("LastRefreshError should be cleared; got %q", state.LastRefreshError)
	}
}

func TestRefresh_MapsClosedWithMergeStampToMerged(t *testing.T) {
	t.Parallel()
	state := &session.PRState{Number: 7, State: testStateOpen}
	_, err := pr.Refresh(context.Background(), state, pr.Options{
		Runner:   fakeRunner{Output: []byte(`{"state":"CLOSED","isDraft":false,"mergedAt":"2026-05-22T11:00:00Z"}`)},
		RepoSlug: testRepoSlug,
		TTL:      0,
		Now:      fixedNow,
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if state.State != testStateMerged {
		t.Errorf("CLOSED+mergedAt should map to merged; got %q", state.State)
	}
}
