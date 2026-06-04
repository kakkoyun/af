package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestStringUsesDefaultBuildInfo(t *testing.T) {
	got := String()
	for _, want := range []string{
		"af dev",
		"commit:",
		"date:",
		"go:",
		"os/arch:",
		"dirty:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("String() = %q, want it to contain %q", got, want)
		}
	}
}

func TestInfoFromBuildInfoFallsBackToVCSMetadata(t *testing.T) {
	seed := buildInfo{Version: "dev", Commit: "none", Date: "unknown", GoVersion: "go1.25.0", OSArch: "darwin/arm64"}
	got := fillFromBuildInfo(seed, debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc1234"},
			{Key: "vcs.time", Value: "2026-05-09T10:11:12Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	})

	if got.Commit != "abc1234" {
		t.Fatalf("Commit = %q, want abc1234", got.Commit)
	}
	if got.Date != "2026-05-09T10:11:12Z" {
		t.Fatalf("Date = %q, want vcs.time", got.Date)
	}
	if !got.Dirty {
		t.Fatal("Dirty = false, want true")
	}
}

func TestStringFormatsInjectedBuildInfo(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	})

	Version = "1.2.3"
	Commit = "abc1234"
	Date = "2026-05-09"

	got := String()
	for _, want := range []string{
		"af 1.2.3",
		"commit: abc1234",
		"date: 2026-05-09",
		"go:",
		"os/arch:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("String() = %q, want it to contain %q", got, want)
		}
	}
}
