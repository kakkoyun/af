package duration_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/kakkoyun/af/internal/duration"
)

func TestParse_AcceptsDaysWeeksAndStdlibUnits(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{name: "day", input: "7d", want: 7 * 24 * time.Hour},
		{name: "week", input: "2w", want: 2 * 7 * 24 * time.Hour},
		{name: "mixed_days_weeks", input: "1w3d", want: 10 * 24 * time.Hour},
		{name: "mixed_week_hours", input: "2w12h", want: 14*24*time.Hour + 12*time.Hour},
		{name: "stdlib", input: "5h30m", want: 5*time.Hour + 30*time.Minute},
		{name: "subsecond", input: "1ms250us", want: time.Millisecond + 250*time.Microsecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := duration.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestParse_RejectsInvalidDurations(t *testing.T) {
	tests := []string{
		"",
		"30 days",
		"1 month",
		"-1d",
		"1y",
		"1.5d",
		"d",
		"1d2",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := duration.Parse(input)
			if err == nil {
				t.Fatalf("Parse(%q) error = nil, want error", input)
			}
			if !strings.Contains(err.Error(), "duration") {
				t.Fatalf("Parse(%q) error = %v, want duration context", input, err)
			}
		})
	}
}

func TestPropertyParseDaysAndWeeks(t *testing.T) {
	property := func(rawDays uint16, rawWeeks uint16) bool {
		days := rawDays % 10_000
		weeks := rawWeeks % 1_000
		dayValue, dayErr := duration.Parse(fmt.Sprintf("%dd", days))
		weekValue, weekErr := duration.Parse(fmt.Sprintf("%dw", weeks))
		if dayErr != nil || weekErr != nil {
			return false
		}

		wantDays := time.Duration(days) * 24 * time.Hour
		wantWeeks := time.Duration(weeks) * 7 * 24 * time.Hour
		return dayValue == wantDays && weekValue == wantWeeks
	}

	err := quick.Check(property, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPropertyParseMatchesStdlibForStdlibUnits(t *testing.T) {
	units := []string{"h", "m", "s", "ms", "us", "ns"}
	property := func(value uint16, unitIndex uint8) bool {
		unit := units[int(unitIndex)%len(units)]
		input := fmt.Sprintf("%d%s", value, unit)

		got, err := duration.Parse(input)
		if err != nil {
			return false
		}
		want, err := time.ParseDuration(input)
		if err != nil {
			return false
		}

		return got == want
	}

	err := quick.Check(property, nil)
	if err != nil {
		t.Fatal(err)
	}
}
