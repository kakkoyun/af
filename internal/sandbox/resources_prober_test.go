package sandbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/sandbox"
)

func TestSlicerResources_IsEmpty(t *testing.T) {
	tests := []struct {
		name      string
		resources sandbox.SlicerResources
		want      bool
	}{
		{name: "zero value", resources: sandbox.SlicerResources{}, want: true},
		{name: "name only is still empty", resources: sandbox.SlicerResources{Name: "tight"}, want: true},
		{name: "vcpu set", resources: sandbox.SlicerResources{VCPU: 2}, want: false},
		{name: "ram set", resources: sandbox.SlicerResources{RAMGB: 4}, want: false},
		{name: "storage set", resources: sandbox.SlicerResources{StorageSize: "20G"}, want: false},
		{name: "gpu set", resources: sandbox.SlicerResources{GPUCount: 1}, want: false},
		{name: "image set", resources: sandbox.SlicerResources{Image: "ubuntu"}, want: false},
		{name: "hypervisor set", resources: sandbox.SlicerResources{Hypervisor: "qemu"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.resources.IsEmpty(); got != tt.want {
				t.Fatalf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveLaunchGroup_ProbeErrorWrapped(t *testing.T) {
	sentinel := errors.New("probe boom")
	prober := &fakeGroupProber{err: sentinel}
	r := sandbox.SlicerResources{VCPU: 2}

	_, _, err := sandbox.ResolveLaunchGroup(context.Background(), prober, "myrepo", "", r)
	if !errors.Is(err, sentinel) {
		t.Fatalf("ResolveLaunchGroup() error = %v, want wrapped %v", err, sentinel)
	}
}

func TestExecGroupProber_Probe(t *testing.T) {
	tests := []struct {
		runnerErr   error
		wantErrIs   error
		name        string
		output      string
		group       string
		wantExists  bool
		wantMatches bool
		wantErr     bool
	}{
		{
			name:        "group present in output",
			output:      "NAME        HOSTS\naf-myrepo-default  2\nother-group 1\n",
			group:       "af-myrepo-default",
			wantExists:  true,
			wantMatches: true,
		},
		{
			name:   "group absent",
			output: "NAME        HOSTS\nother-group 1\n",
			group:  "af-myrepo-default",
		},
		{
			name:   "substring does not count as match",
			output: "af-myrepo-default-extra 1\n",
			group:  "af-myrepo-default",
		},
		{
			name:      "daemon unavailable treated as absent",
			runnerErr: errors.New("dial unix /run/slicer.sock: connect: connection refused"),
			group:     "af-myrepo-default",
		},
		{
			name:      "other runner error is fatal",
			runnerErr: errors.New("slicer exploded"),
			group:     "af-myrepo-default",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{output: []byte(tt.output), err: tt.runnerErr}
			prober := sandbox.ExecGroupProber{Runner: runner}

			exists, matches, err := prober.Probe(context.Background(), tt.group, sandbox.SlicerResources{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("Probe() error = nil, want error")
				}
				if !strings.Contains(err.Error(), "slicer vm group") {
					t.Fatalf("Probe() error = %v, want slicer vm group context", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Probe() error = %v", err)
			}
			if exists != tt.wantExists || matches != tt.wantMatches {
				t.Fatalf("Probe() = (%v, %v), want (%v, %v)", exists, matches, tt.wantExists, tt.wantMatches)
			}
			wantCalls := 1
			if len(runner.calls) != wantCalls {
				t.Fatalf("runner calls = %d, want %d", len(runner.calls), wantCalls)
			}
			args := runner.calls[0].Args
			if runner.calls[0].Name != "slicer" || len(args) != 2 || args[0] != "vm" || args[1] != "group" {
				t.Fatalf("command = %s %v, want slicer vm group", runner.calls[0].Name, args)
			}
		})
	}
}
