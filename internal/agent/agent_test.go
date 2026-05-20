package agent_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/kakkoyun/af/internal/agent"
	"github.com/kakkoyun/af/internal/testutil"
)

func TestKnownAgents_ArePiClaudeCodexInFallbackOrder(t *testing.T) {
	got := agent.KnownAgents()
	want := []string{"pi", "claude", "codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KnownAgents() = %#v, want %#v", got, want)
	}
}

func TestRegistry_ResolveAndFirstAvailable(t *testing.T) {
	ctx := context.Background()
	pi := agent.NewFake("pi")
	pi.SetAvailable(false)
	claude := agent.NewFake("claude")
	registry := agent.NewRegistry(pi, claude)

	got, err := registry.Resolve("claude")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.Name() != "claude" {
		t.Fatalf("Resolve() = %q, want claude", got.Name())
	}

	got, err = registry.FirstAvailable(ctx)
	if err != nil {
		t.Fatalf("FirstAvailable() error = %v", err)
	}
	if got.Name() != "claude" {
		t.Fatalf("FirstAvailable() = %q, want claude", got.Name())
	}
}

func TestProviders_RenderLaunchResumeAndBodyCommands(t *testing.T) {
	launch := agent.LaunchOpts{SessionID: "session-uuid", ApprovalMode: agent.ApprovalYolo, Cwd: "/worktree"}
	resume := agent.ResumeOpts{SessionID: "session-uuid", ApprovalMode: agent.ApprovalAuto, Cwd: "/worktree"}
	body := agent.BodyOpts{Cwd: "/worktree", Model: "sonnet"}

	tests := []struct {
		name       string
		provider   agent.Agent
		launchWant []string
		resumeWant []string
		bodyWant   []string
	}{
		{name: "pi", provider: agent.NewPi(), launchWant: []string{"pi"}, resumeWant: []string{"pi", "--continue"}, bodyWant: []string{"pi", "--print", "--model", "sonnet"}},
		{name: "claude", provider: agent.NewClaude(), launchWant: []string{"claude", "--session-id", "session-uuid", "--dangerously-skip-permissions"}, resumeWant: []string{"claude", "--continue"}, bodyWant: []string{"claude", "-p", "--model", "sonnet"}},
		{name: "codex", provider: agent.NewCodex(), launchWant: []string{"codex", "--full-auto"}, resumeWant: []string{"codex", "--auto", "resume", "--last"}, bodyWant: []string{"codex", "exec", "--model", "sonnet"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.LaunchCmd(launch); !reflect.DeepEqual(got, tt.launchWant) {
				t.Fatalf("LaunchCmd() = %#v, want %#v", got, tt.launchWant)
			}
			if got := tt.provider.ResumeCmd(resume); !reflect.DeepEqual(got, tt.resumeWant) {
				t.Fatalf("ResumeCmd() = %#v, want %#v", got, tt.resumeWant)
			}
			got, ok := tt.provider.BodyCmd(body)
			if !ok {
				t.Fatal("BodyCmd() ok = false, want true")
			}
			if !reflect.DeepEqual(got, tt.bodyWant) {
				t.Fatalf("BodyCmd() = %#v, want %#v", got, tt.bodyWant)
			}
		})
	}
}

func TestExecutableAvailable_UsesPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	testutil.WriteExecutable(t, dir, "af-fake-agent", "exit 0")
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", testutil.PrependPath(dir, oldPath))

	if !agent.ExecutableAvailable(ctx, "af-fake-agent") {
		t.Fatal("ExecutableAvailable(fake) = false, want true")
	}
	if agent.ExecutableAvailable(ctx, "af-missing-agent") {
		t.Fatal("ExecutableAvailable(missing) = true, want false")
	}
}
