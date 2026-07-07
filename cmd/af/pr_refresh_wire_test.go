package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/pr"
	"github.com/kakkoyun/af/internal/session"
)

var (
	errInfoRefreshRateLimited  = errors.New("rate limited")
	errCleanRefreshUnavailable = errors.New("refresh unavailable")
	errSyncRefreshUnavailable  = errors.New("parent refresh unavailable")
	errDoneRefreshUnavailable  = errors.New("done refresh unavailable")
)

func setTestPRState(t *testing.T, home, name string, prState session.PRState) string {
	t.Helper()
	statePath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", name, "state.toml")
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	state.PR = prState
	err = session.WriteState(statePath, state)
	if err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	return statePath
}

func installPRRefreshStub(t *testing.T, fn func(context.Context, *session.PRState, pr.Options) (pr.Result, error)) {
	t.Helper()
	orig := prRefreshFunc
	t.Cleanup(func() { prRefreshFunc = orig })
	prRefreshFunc = fn
}

func TestStatus_RefreshFlagRefreshesPRState(t *testing.T) {
	installPRRefreshStub(t, func(_ context.Context, prState *session.PRState, opts pr.Options) (pr.Result, error) {
		if !opts.Force {
			t.Fatalf("status --refresh must force the PR cache refresh")
		}
		old := prState.State
		stamp := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)
		prState.State = pr.StateMerged
		prState.LastRefreshedAt = &stamp
		prState.LastRefreshError = ""
		return pr.Result{Old: old, New: pr.StateMerged, RefreshedAt: stamp, Changed: old != pr.StateMerged}, nil
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "status-pr", "feat/status", "active")
	statePath := setTestPRState(t, home, "status-pr", session.PRState{Number: 42, URL: "https://github.com/owner/repo/pull/42", State: pr.StateOpen})

	stdout, _, err := executeCommand(t, newRootCmd(), "status", "--refresh")
	if err != nil {
		t.Fatalf("status --refresh: %v", err)
	}
	if !strings.Contains(stdout, pr.StateMerged) {
		t.Fatalf("status output should show refreshed PR state %q; got:\n%s", pr.StateMerged, stdout)
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.PR.State != pr.StateMerged {
		t.Fatalf("state.PR.State = %q, want %q", state.PR.State, pr.StateMerged)
	}
}

func TestInfo_RefreshFailureRendersQuestionMarkAndPersistsError(t *testing.T) {
	installPRRefreshStub(t, func(_ context.Context, prState *session.PRState, opts pr.Options) (pr.Result, error) {
		if !opts.Force {
			t.Fatalf("info --refresh must force the PR cache refresh")
		}
		prState.LastRefreshError = "rate limited"
		return pr.Result{Old: prState.State, New: prState.State}, errInfoRefreshRateLimited
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "info-pr", "feat/info", "active")
	statePath := setTestPRState(t, home, "info-pr", session.PRState{Number: 7, URL: "https://github.com/owner/repo/pull/7", State: pr.StateOpen})

	stdout, _, err := executeCommand(t, newRootCmd(), "info", "--refresh", "info-pr")
	if err != nil {
		t.Fatalf("info --refresh should render stale state with ? rather than fail: %v", err)
	}
	if !strings.Contains(stdout, "PR:") || !strings.Contains(stdout, "State:     ?") {
		t.Fatalf("info output should render failed refresh state as ?; got:\n%s", stdout)
	}
	state, err := session.ReadState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.PR.LastRefreshError != "rate limited" {
		t.Fatalf("LastRefreshError = %q, want rate limited", state.PR.LastRefreshError)
	}
}

func TestClean_ForceRefreshFailureIsHardError(t *testing.T) {
	refreshErr := errCleanRefreshUnavailable
	installPRRefreshStub(t, func(_ context.Context, prState *session.PRState, opts pr.Options) (pr.Result, error) {
		if !opts.Force {
			t.Fatalf("clean must force-refresh correctness-critical PR cache")
		}
		prState.LastRefreshError = refreshErr.Error()
		return pr.Result{Old: prState.State, New: prState.State}, refreshErr
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "clean-pr", "feat/clean", "completed")
	setTestPRState(t, home, "clean-pr", session.PRState{Number: 9, State: pr.StateOpen})

	_, _, err := executeCommand(t, newRootCmd(), "clean", "--dry-run")
	if !errors.Is(err, refreshErr) {
		t.Fatalf("clean should return refresh failure, got %v", err)
	}
}

func TestSync_ForceRefreshesParentPRBeforeRebase(t *testing.T) {
	refreshErr := errSyncRefreshUnavailable
	installPRRefreshStub(t, func(_ context.Context, prState *session.PRState, opts pr.Options) (pr.Result, error) {
		if prState.Number != 17 {
			t.Fatalf("sync refreshed PR number %d, want parent PR 17", prState.Number)
		}
		if !opts.Force {
			t.Fatalf("sync must force-refresh parent PR cache")
		}
		prState.LastRefreshError = refreshErr.Error()
		return pr.Result{Old: prState.State, New: prState.State}, refreshErr
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, staleTestParent, "feat/parent", "active")
	writeTestSessionState(t, home, "child", "feat/child", "active")
	setTestPRState(t, home, staleTestParent, session.PRState{Number: 17, State: pr.StateOpen})
	childPath := filepath.Join(home, ".local", "share", "af", "v1", "sessions", "child", "state.toml")
	child, err := session.ReadState(childPath)
	if err != nil {
		t.Fatal(err)
	}
	child.Stack.ParentSession = staleTestParent
	err = session.WriteState(childPath, child)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = executeCommand(t, newRootCmd(), "sync", "child")
	if !errors.Is(err, refreshErr) {
		t.Fatalf("sync should stop on parent refresh failure, got %v", err)
	}
}

func TestDone_ForceRefreshFailureStopsBeforeTeardown(t *testing.T) {
	refreshErr := errDoneRefreshUnavailable
	installPRRefreshStub(t, func(_ context.Context, prState *session.PRState, opts pr.Options) (pr.Result, error) {
		if !opts.Force {
			t.Fatalf("done must force-refresh PR cache before completion decision")
		}
		prState.LastRefreshError = refreshErr.Error()
		return pr.Result{Old: prState.State, New: prState.State}, refreshErr
	})

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestSessionState(t, home, "done-pr", "feat/done-pr", "active")
	setTestPRState(t, home, "done-pr", session.PRState{Number: 5, State: pr.StateOpen})
	installNoopSlicerFactory(t)

	_, _, err := executeCommand(t, newRootCmd(), "done", "done-pr")
	if !errors.Is(err, refreshErr) {
		t.Fatalf("done should stop on refresh failure, got %v", err)
	}
}
