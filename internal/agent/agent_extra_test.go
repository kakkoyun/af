package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/testutil"
)

func TestDefaultRegistry_ContainsBuiltinProviders(t *testing.T) {
	registry := agent.DefaultRegistry()

	for _, name := range agent.KnownAgents() {
		provider, err := registry.Resolve(name)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", name, err)
		}
		if provider.Name() != name {
			t.Fatalf("Resolve(%q).Name() = %q, want %q", name, provider.Name(), name)
		}
		if provider.Binary() != name {
			t.Fatalf("Resolve(%q).Binary() = %q, want %q", name, provider.Binary(), name)
		}
	}
}

func TestRegistry_ResolveUnknownReturnsErrUnknown(t *testing.T) {
	registry := agent.NewRegistry(agent.NewFake("pi"))

	provider, err := registry.Resolve("nonexistent")
	if !errors.Is(err, agent.ErrUnknown) {
		t.Fatalf("Resolve() error = %v, want ErrUnknown", err)
	}
	if provider != nil {
		t.Fatalf("Resolve() = %v, want nil", provider)
	}
}

func TestRegistry_FirstAvailablePrefersFallbackOrder(t *testing.T) {
	ctx := context.Background()
	pi := agent.NewFake("pi")
	pi.SetAvailable(false)
	claude := agent.NewFake("claude")
	claude.SetAvailable(false)
	codex := agent.NewFake("codex")

	registry := agent.NewRegistry(pi, claude, codex)

	got, err := registry.FirstAvailable(ctx)
	if err != nil {
		t.Fatalf("FirstAvailable() error = %v", err)
	}
	if got.Name() != "codex" {
		t.Fatalf("FirstAvailable() = %q, want codex", got.Name())
	}
}

func TestRegistry_FirstAvailableFallsBackToSortedUnknownNames(t *testing.T) {
	ctx := context.Background()
	pi := agent.NewFake("pi")
	pi.SetAvailable(false)
	alpha := agent.NewFake("custom-alpha")
	alpha.SetAvailable(false)
	beta := agent.NewFake("custom-beta")
	zeta := agent.NewFake("custom-zeta")

	registry := agent.NewRegistry(pi, zeta, beta, alpha)

	got, err := registry.FirstAvailable(ctx)
	if err != nil {
		t.Fatalf("FirstAvailable() error = %v", err)
	}
	if got.Name() != "custom-beta" {
		t.Fatalf("FirstAvailable() = %q, want custom-beta (first available in sorted order)", got.Name())
	}
}

func TestRegistry_FirstAvailableNoneAvailableReturnsErrUnavailable(t *testing.T) {
	ctx := context.Background()
	pi := agent.NewFake("pi")
	pi.SetAvailable(false)
	other := agent.NewFake("other")
	other.SetAvailable(false)

	registry := agent.NewRegistry(pi, other)

	provider, err := registry.FirstAvailable(ctx)
	if !errors.Is(err, agent.ErrUnavailable) {
		t.Fatalf("FirstAvailable() error = %v, want ErrUnavailable", err)
	}
	if provider != nil {
		t.Fatalf("FirstAvailable() = %v, want nil", provider)
	}
}

func TestExecutableAvailable_CanceledContextReturnsFalse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if agent.ExecutableAvailable(ctx, "sh") {
		t.Fatal("ExecutableAvailable(canceled ctx) = true, want false")
	}
}

func TestProvider_IsAvailableChecksPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	testutil.WriteExecutable(t, dir, "claude", "exit 0")
	t.Setenv("PATH", dir)

	if !agent.NewClaude().IsAvailable(ctx) {
		t.Fatal("NewClaude().IsAvailable() = false, want true")
	}
	if agent.NewCodex().IsAvailable(ctx) {
		t.Fatal("NewCodex().IsAvailable() = true, want false")
	}
}

func TestProviders_LaunchCmdApprovalMatrix(t *testing.T) {
	tests := []struct {
		name     string
		provider agent.Agent
		opts     agent.LaunchOpts
		want     []string
	}{
		{name: "pi ignores session and yolo", provider: agent.NewPi(), opts: agent.LaunchOpts{SessionID: "sid", ApprovalMode: agent.ApprovalYolo}, want: []string{"pi"}},
		{name: "claude default without session", provider: agent.NewClaude(), opts: agent.LaunchOpts{}, want: []string{"claude"}},
		{name: "claude auto adds no flag", provider: agent.NewClaude(), opts: agent.LaunchOpts{ApprovalMode: agent.ApprovalAuto}, want: []string{"claude"}},
		{name: "claude yolo without session", provider: agent.NewClaude(), opts: agent.LaunchOpts{ApprovalMode: agent.ApprovalYolo}, want: []string{"claude", "--dangerously-skip-permissions"}},
		{name: "claude session with default approval", provider: agent.NewClaude(), opts: agent.LaunchOpts{SessionID: "sid"}, want: []string{"claude", "--session-id", "sid"}},
		{name: "codex default", provider: agent.NewCodex(), opts: agent.LaunchOpts{}, want: []string{"codex"}},
		{name: "codex auto", provider: agent.NewCodex(), opts: agent.LaunchOpts{ApprovalMode: agent.ApprovalAuto}, want: []string{"codex", "--auto"}},
		{name: "codex yolo", provider: agent.NewCodex(), opts: agent.LaunchOpts{ApprovalMode: agent.ApprovalYolo}, want: []string{"codex", "--full-auto"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.LaunchCmd(tt.opts); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("LaunchCmd(%+v) = %#v, want %#v", tt.opts, got, tt.want)
			}
		})
	}
}

func TestProviders_ResumeCmdApprovalMatrix(t *testing.T) {
	tests := []struct {
		name     string
		provider agent.Agent
		opts     agent.ResumeOpts
		want     []string
	}{
		{name: "pi ignores approval", provider: agent.NewPi(), opts: agent.ResumeOpts{ApprovalMode: agent.ApprovalYolo}, want: []string{"pi", "--continue"}},
		{name: "claude default", provider: agent.NewClaude(), opts: agent.ResumeOpts{}, want: []string{"claude", "--continue"}},
		{name: "claude yolo", provider: agent.NewClaude(), opts: agent.ResumeOpts{ApprovalMode: agent.ApprovalYolo}, want: []string{"claude", "--continue", "--dangerously-skip-permissions"}},
		{name: "codex default", provider: agent.NewCodex(), opts: agent.ResumeOpts{}, want: []string{"codex", "resume", "--last"}},
		{name: "codex yolo", provider: agent.NewCodex(), opts: agent.ResumeOpts{ApprovalMode: agent.ApprovalYolo}, want: []string{"codex", "--full-auto", "resume", "--last"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.ResumeCmd(tt.opts); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResumeCmd(%+v) = %#v, want %#v", tt.opts, got, tt.want)
			}
		})
	}
}

func TestProviders_BodyCmdWithoutModelOmitsFlag(t *testing.T) {
	tests := []struct {
		name     string
		provider agent.Agent
		want     []string
	}{
		{name: "pi", provider: agent.NewPi(), want: []string{"pi", "--print"}},
		{name: "claude", provider: agent.NewClaude(), want: []string{"claude", "-p"}},
		{name: "codex", provider: agent.NewCodex(), want: []string{"codex", "exec"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.provider.BodyCmd(agent.BodyOpts{Cwd: "/worktree"})
			if !ok {
				t.Fatal("BodyCmd() ok = false, want true")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("BodyCmd() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestProviders_PRCmdIsUnsupported(t *testing.T) {
	providers := []agent.Agent{agent.NewPi(), agent.NewClaude(), agent.NewCodex()}
	for _, provider := range providers {
		t.Run(provider.Name(), func(t *testing.T) {
			argv, ok := provider.PRCmd(42, agent.LaunchOpts{SessionID: "sid"})
			if ok {
				t.Fatal("PRCmd() ok = true, want false")
			}
			if argv != nil {
				t.Fatalf("PRCmd() = %#v, want nil", argv)
			}
		})
	}
}

func TestProviders_SessionLogPaths(t *testing.T) {
	t.Setenv("HOME", "/fakehome")

	tests := []struct {
		name     string
		provider agent.Agent
		want     []string
	}{
		{name: "pi globs sessions dir", provider: agent.NewPi(), want: []string{"/fakehome/.pi/agent/sessions/%2Fproj%2Fdir/*sess-1*.jsonl"}},
		{name: "claude uses projects dir", provider: agent.NewClaude(), want: []string{"/fakehome/.claude/projects/%2Fproj%2Fdir/sess-1.jsonl"}},
		{name: "codex has no logs", provider: agent.NewCodex(), want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.SessionLogPaths("sess-1", "/proj/dir/")
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SessionLogPaths() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFake_Behaviors(t *testing.T) {
	ctx := context.Background()
	fake := agent.NewFake("fakey")

	if fake.Name() != "fakey" {
		t.Fatalf("Name() = %q, want fakey", fake.Name())
	}
	if fake.Binary() != "fakey" {
		t.Fatalf("Binary() = %q, want fakey", fake.Binary())
	}
	if !fake.IsAvailable(ctx) {
		t.Fatal("IsAvailable() = false, want true after NewFake")
	}
	fake.SetAvailable(false)
	if fake.IsAvailable(ctx) {
		t.Fatal("IsAvailable() = true, want false after SetAvailable(false)")
	}
}

func TestFake_Commands(t *testing.T) {
	fake := agent.NewFake("fakey")

	if got, want := fake.LaunchCmd(agent.LaunchOpts{}), []string{"fakey", "launch"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LaunchCmd() = %#v, want %#v", got, want)
	}
	if got, want := fake.ResumeCmd(agent.ResumeOpts{}), []string{"fakey", "resume"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeCmd() = %#v, want %#v", got, want)
	}

	prArgv, ok := fake.PRCmd(7, agent.LaunchOpts{})
	if !ok {
		t.Fatal("PRCmd() ok = false, want true")
	}
	if want := []string{"fakey", "pr", "7"}; !reflect.DeepEqual(prArgv, want) {
		t.Fatalf("PRCmd() = %#v, want %#v", prArgv, want)
	}

	bodyArgv, ok := fake.BodyCmd(agent.BodyOpts{})
	if !ok {
		t.Fatal("BodyCmd() ok = false, want true")
	}
	if want := []string{"fakey", "body"}; !reflect.DeepEqual(bodyArgv, want) {
		t.Fatalf("BodyCmd() = %#v, want %#v", bodyArgv, want)
	}

	if got := fake.SessionLogPaths("sid", "/proj"); got != nil {
		t.Fatalf("SessionLogPaths() = %#v, want nil", got)
	}
}
