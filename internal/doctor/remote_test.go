package doctor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kakkoyun/af/internal/doctor"
)

type fakeRemoteCommander struct {
	responses map[string]string
	errors    map[string]error
}

func (f fakeRemoteCommander) Run(_ context.Context, command string) ([]byte, error) {
	if err, ok := f.errors[command]; ok {
		return nil, err
	}
	return []byte(f.responses[command]), nil
}

func TestRemoteLookup_FindsBinaryViaCommandV(t *testing.T) {
	commander := fakeRemoteCommander{responses: map[string]string{
		"command -v git 2>/dev/null || true": "/usr/bin/git\n",
	}}
	lookup := doctor.RemoteLookup{Commander: commander}

	path, ok := lookup.LookPath(context.Background(), "git")
	if !ok {
		t.Fatal("LookPath returned false, want true")
	}
	if path != "/usr/bin/git" {
		t.Fatalf("path = %q, want /usr/bin/git", path)
	}
}

func TestRemoteLookup_ReturnsNotFoundWhenCommandVEmpty(t *testing.T) {
	commander := fakeRemoteCommander{responses: map[string]string{
		"command -v missing 2>/dev/null || true": "",
	}}
	lookup := doctor.RemoteLookup{Commander: commander}

	if _, ok := lookup.LookPath(context.Background(), "missing"); ok {
		t.Fatal("LookPath = true, want false on empty output")
	}
}

func TestRemoteLookup_HandlesRunError(t *testing.T) {
	commander := fakeRemoteCommander{errors: map[string]error{
		"command -v boom 2>/dev/null || true": errFakeRemote,
	}}
	lookup := doctor.RemoteLookup{Commander: commander}

	if _, ok := lookup.LookPath(context.Background(), "boom"); ok {
		t.Fatal("LookPath = true, want false on commander error")
	}
}

func TestRemoteLookup_VersionTrimsAndReturnsFirstNonEmptyLine(t *testing.T) {
	commander := fakeRemoteCommander{responses: map[string]string{
		"git --version 2>&1 || true": "\n  git version 2.43.0  \nextra\n",
	}}
	lookup := doctor.RemoteLookup{Commander: commander}

	got := lookup.Version(context.Background(), "git")
	if got != "git version 2.43.0" {
		t.Fatalf("Version = %q", got)
	}
}

func TestDetectRemotePlatform_PrefersUnameDarwin(t *testing.T) {
	commander := fakeRemoteCommander{responses: map[string]string{
		"uname -s": "Darwin\n",
	}}
	if got := doctor.DetectRemotePlatform(context.Background(), commander); got != doctor.PlatformMacOS {
		t.Fatalf("DetectRemotePlatform = %q, want macos", got)
	}
}

func TestDetectRemotePlatform_FallsThroughToOSRelease(t *testing.T) {
	commander := fakeRemoteCommander{responses: map[string]string{
		"uname -s": "Linux\n",
		"cat /etc/os-release 2>/dev/null || true": "ID=arch\nID_LIKE=\"\"\n",
	}}
	if got := doctor.DetectRemotePlatform(context.Background(), commander); got != doctor.PlatformArch {
		t.Fatalf("DetectRemotePlatform = %q, want arch", got)
	}
}

func TestRemoteLookup_ShellEscapesNamesWithSpecialCharacters(t *testing.T) {
	var captured string
	commander := captureCommander{record: &captured}
	lookup := doctor.RemoteLookup{Commander: commander}
	_, _ = lookup.LookPath(context.Background(), "weird name")

	if !strings.Contains(captured, `'weird name'`) {
		t.Fatalf("command did not single-quote-escape the name; got %q", captured)
	}
}

type captureCommander struct {
	record *string
}

func (c captureCommander) Run(_ context.Context, command string) ([]byte, error) {
	*c.record = command
	return []byte(""), nil
}
