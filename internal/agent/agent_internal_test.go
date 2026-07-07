package agent

import (
	"reflect"
	"testing"
)

func TestProvider_UnknownKindFallsBackToBinaryOnly(t *testing.T) {
	provider := Provider{name: "mystery", binary: "mystery", kind: providerKind(99)}

	if got, want := provider.LaunchCmd(LaunchOpts{SessionID: "sid", ApprovalMode: ApprovalYolo}), []string{"mystery"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LaunchCmd() = %#v, want %#v", got, want)
	}
	if got, want := provider.ResumeCmd(ResumeOpts{ApprovalMode: ApprovalYolo}), []string{"mystery"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeCmd() = %#v, want %#v", got, want)
	}

	argv, ok := provider.BodyCmd(BodyOpts{Model: "sonnet"})
	if ok {
		t.Fatal("BodyCmd() ok = true, want false for unknown kind")
	}
	if argv != nil {
		t.Fatalf("BodyCmd() = %#v, want nil", argv)
	}

	if got := provider.SessionLogPaths("sid", "/proj"); got != nil {
		t.Fatalf("SessionLogPaths() = %#v, want nil", got)
	}
}

func TestAppendApproval_PiIgnoresAllModes(t *testing.T) {
	for _, mode := range []ApprovalMode{ApprovalDefault, ApprovalAuto, ApprovalYolo} {
		got := appendApproval([]string{"pi"}, mode, providerPi)
		if want := []string{"pi"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("appendApproval(pi, %d) = %#v, want %#v", mode, got, want)
		}
	}
}
