package version

import "testing"

func TestStringUsesDefaultBuildInfo(t *testing.T) {
	got := String()
	want := "af dev (none, unknown)"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
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
	want := "af 1.2.3 (abc1234, 2026-05-09)"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
