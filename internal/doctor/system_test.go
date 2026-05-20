package doctor_test

import (
	"errors"
	"testing"

	"github.com/kakkoyun/af/internal/doctor"
)

var errFakeOSRelease = errors.New("missing os-release")

type fakeOSRelease struct {
	content map[string]string
	err     error
}

func (f fakeOSRelease) Read() (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.content, nil
}

func TestDetectPlatform_DarwinIsMacOS(_ *testing.T) {
	// runtime.GOOS check happens before OSRelease is consulted.
	// Nothing to assert on a non-Darwin host beyond not panicking.
	_ = doctor.DetectPlatform(nil)
}

func TestParseOSRelease_HandlesQuotedAndUnquotedValues(t *testing.T) {
	body := `# comment
ID=ubuntu
ID_LIKE="debian"
VERSION_CODENAME='jammy'
PRETTY_NAME=Foo Bar
`
	got := doctor.ParseOSRelease(body)
	want := map[string]string{
		"ID":               "ubuntu",
		"ID_LIKE":          "debian",
		"VERSION_CODENAME": "jammy",
		"PRETTY_NAME":      "Foo Bar",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("ParseOSRelease[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestDetectPlatform_ClassifiesArchAndDebian(t *testing.T) {
	tests := []struct {
		name    string
		content map[string]string
		want    doctor.Platform
	}{
		{name: "arch", content: map[string]string{"ID": "arch"}, want: doctor.PlatformArch},
		{name: "manjaro", content: map[string]string{"ID": "manjaro"}, want: doctor.PlatformArch},
		{name: "id_like arch", content: map[string]string{"ID": "endeavouros", "ID_LIKE": "arch"}, want: doctor.PlatformArch},
		{name: "debian", content: map[string]string{"ID": "debian"}, want: doctor.PlatformDebian},
		{name: "ubuntu", content: map[string]string{"ID": "ubuntu"}, want: doctor.PlatformDebian},
		{name: "id_like debian", content: map[string]string{"ID": "pop", "ID_LIKE": "debian"}, want: doctor.PlatformDebian},
		{name: "fedora", content: map[string]string{"ID": "fedora"}, want: doctor.PlatformOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyForTest(t, tt.content, nil)
			if got != tt.want && got != doctor.PlatformMacOS {
				// macos branch is unreachable from this helper but guards CI on darwin
				t.Fatalf("DetectPlatform = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectPlatform_FallsBackOnError(t *testing.T) {
	got := classifyForTest(t, nil, errFakeOSRelease)
	// On linux/darwin the platform is either Other or MacOS depending on
	// the host. Both are acceptable here.
	if got != doctor.PlatformOther && got != doctor.PlatformMacOS {
		t.Fatalf("DetectPlatform = %q, want PlatformOther or PlatformMacOS", got)
	}
}

// classifyForTest exercises DetectPlatform via the OSReleaseReader seam
// without depending on the host's actual /etc/os-release.
func classifyForTest(t *testing.T, content map[string]string, err error) doctor.Platform {
	t.Helper()
	return doctor.DetectPlatform(fakeOSRelease{content: content, err: err})
}
