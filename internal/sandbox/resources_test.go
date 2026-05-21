package sandbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

// fakeGroupProber is a test double for sandbox.GroupProber.
type fakeGroupProber struct {
	err     error
	exists  bool
	matches bool
	called  bool
}

func (f *fakeGroupProber) Probe(_ context.Context, _ string, _ sandbox.SlicerResources) (bool, bool, error) {
	f.called = true
	return f.exists, f.matches, f.err
}

func TestManagedGroupName_FormatsCorrectly(t *testing.T) {
	tests := []struct {
		repoSlug string
		profile  string
		want     string
	}{
		{"github.com/kakkoyun/af", "tight", "af-github-com-kakkoyun-af-tight"},
		{"github.com/kakkoyun/af", "", "af-github-com-kakkoyun-af-default"},
		{"myrepo", "fast", "af-myrepo-fast"},
		{"my_repo", "default", "af-my-repo-default"},
	}
	for _, tt := range tests {
		t.Run(tt.repoSlug+"/"+tt.profile, func(t *testing.T) {
			got := sandbox.ManagedGroupName(tt.repoSlug, tt.profile)
			if got != tt.want {
				t.Errorf("ManagedGroupName(%q, %q) = %q, want %q", tt.repoSlug, tt.profile, got, tt.want)
			}
		})
	}
}

func TestResolveLaunchGroup_FixedGroupBypassesProbe(t *testing.T) {
	prober := &fakeGroupProber{}
	r := sandbox.SlicerResources{} // empty = no resource overrides

	group, needCreate, err := sandbox.ResolveLaunchGroup(context.Background(), prober, "myrepo", "my-fixed-group", r)
	if err != nil {
		t.Fatalf("ResolveLaunchGroup() error = %v", err)
	}
	if group != "my-fixed-group" {
		t.Errorf("group = %q, want my-fixed-group", group)
	}
	if needCreate {
		t.Error("needCreate = true, want false for fixed group")
	}
	if prober.called {
		t.Error("Probe was called, want bypass for empty resources")
	}
}

func TestResolveLaunchGroup_EmptyResourcesEmptyGroupReturnsEmpty(t *testing.T) {
	prober := &fakeGroupProber{}
	r := sandbox.SlicerResources{} // empty

	group, needCreate, err := sandbox.ResolveLaunchGroup(context.Background(), prober, "myrepo", "", r)
	if err != nil {
		t.Fatalf("ResolveLaunchGroup() error = %v", err)
	}
	if group != "" {
		t.Errorf("group = %q, want empty", group)
	}
	if needCreate {
		t.Error("needCreate = true, want false")
	}
}

func TestResolveLaunchGroup_ManagedGroupExistsShapeMatch(t *testing.T) {
	prober := &fakeGroupProber{exists: true, matches: true}
	r := sandbox.SlicerResources{VCPU: 2, RAMGB: 4}

	group, needCreate, err := sandbox.ResolveLaunchGroup(context.Background(), prober, "myrepo", "", r)
	if err != nil {
		t.Fatalf("ResolveLaunchGroup() error = %v", err)
	}
	wantGroup := sandbox.ManagedGroupName("myrepo", "")
	if group != wantGroup {
		t.Errorf("group = %q, want %q", group, wantGroup)
	}
	if needCreate {
		t.Error("needCreate = true, want false when group already exists with matching shape")
	}
}

func TestResolveLaunchGroup_ManagedGroupExistsShapeMismatch(t *testing.T) {
	prober := &fakeGroupProber{exists: true, matches: false}
	r := sandbox.SlicerResources{VCPU: 8, RAMGB: 16}

	_, _, err := sandbox.ResolveLaunchGroup(context.Background(), prober, "myrepo", "", r)
	if err == nil {
		t.Fatal("ResolveLaunchGroup() error = nil, want ErrSlicerResourceMismatch")
	}
	if !errors.Is(err, sandbox.ErrSlicerResourceMismatch) {
		t.Fatalf("ResolveLaunchGroup() error = %v, want ErrSlicerResourceMismatch", err)
	}
}

func TestResolveLaunchGroup_ManagedGroupAbsentReturnsNeedCreate(t *testing.T) {
	prober := &fakeGroupProber{exists: false, matches: false}
	r := sandbox.SlicerResources{VCPU: 2, RAMGB: 4}

	group, needCreate, err := sandbox.ResolveLaunchGroup(context.Background(), prober, "myrepo", "", r)
	if err != nil {
		t.Fatalf("ResolveLaunchGroup() error = %v", err)
	}
	wantGroup := sandbox.ManagedGroupName("myrepo", "")
	if group != wantGroup {
		t.Errorf("group = %q, want %q", group, wantGroup)
	}
	if !needCreate {
		t.Error("needCreate = false, want true when group is absent")
	}
}
