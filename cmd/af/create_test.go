package main

import (
	"errors"
	"testing"

	"github.com/kakkoyun/af/internal/obsidian"
	sandboxpkg "github.com/kakkoyun/af/internal/sandbox"
)

func TestCreate_SandboxFlagRejectsSBX(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// af create with --sandbox sbx must fail before touching git.
	_, _, err := executeCommand(t, newRootCmd(), "create", "--sandbox", "sbx", "--bare", "demo")
	if err == nil {
		t.Fatal("create --sandbox sbx: error = nil, want errSandboxFlagUnsupported")
	}
	if !errors.Is(err, errSandboxFlagUnsupported) {
		t.Fatalf("create --sandbox sbx: error = %v, want errSandboxFlagUnsupported", err)
	}
}

func TestCreate_SandboxFlagRejectsDocker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, _, err := executeCommand(t, newRootCmd(), "create", "--sandbox", "docker", "--bare", "demo")
	if err == nil {
		t.Fatal("create --sandbox docker: error = nil, want error")
	}
	if !errors.Is(err, errSandboxFlagUnsupported) {
		t.Fatalf("create --sandbox docker: error = %v, want errSandboxFlagUnsupported", err)
	}
}

// TestCreate_SandboxProviderFactory_RejectsSBX exercises sandbox.NewProvider
// directly so the CLI plumbing has a unit-level anchor.
func TestCreate_SandboxProviderFactory_RejectsSBX(t *testing.T) {
	_, err := sandboxpkg.NewProvider("sbx")
	if err == nil {
		t.Fatal("NewProvider(sbx) error = nil, want ErrUnsupportedProvider")
	}
	if !errors.Is(err, sandboxpkg.ErrUnsupportedProvider) {
		t.Fatalf("NewProvider(sbx) error = %v, want ErrUnsupportedProvider", err)
	}
}

// TestDefaultCreateContext_WiresDiskNoteStore guards the ADR-047 wiring:
// production creates must carry a real note store, not nil, or
// note-on-create silently becomes a no-op.
func TestDefaultCreateContext_WiresDiskNoteStore(t *testing.T) {
	cc := defaultCreateContext(&rootOptions{})
	if cc.notes == nil {
		t.Fatal("defaultCreateContext().notes = nil, want obsidian.DirStore")
	}
	if _, ok := cc.notes.(obsidian.DirStore); !ok {
		t.Fatalf("defaultCreateContext().notes = %T, want obsidian.DirStore", cc.notes)
	}
}
