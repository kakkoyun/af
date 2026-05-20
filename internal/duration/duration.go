package duration

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	hoursPerDay      = 24
	hoursPerWeek     = 7 * hoursPerDay
	maxDurationHours = uint64((1<<63 - 1) / int64(time.Hour))
)

var errInvalidDuration = errors.New("invalid duration")

// Parse parses af's shared duration grammar.
func Parse(input string) (time.Duration, error) {
	converted, err := convertDaysAndWeeks(input)
	if err != nil {
		return 0, err
	}

	parsed, err := time.ParseDuration(converted)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", input, err)
	}

	return parsed, nil
}

func convertDaysAndWeeks(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("parse duration %q: empty input: %w", input, errInvalidDuration)
	}

	var converted strings.Builder
	for offset := 0; offset < len(input); {
		nextOffset, err := appendNextToken(&converted, input, offset)
		if err != nil {
			return "", err
		}
		offset = nextOffset
	}

	return converted.String(), nil
}

func appendNextToken(converted *strings.Builder, input string, offset int) (int, error) {
	start := offset
	for offset < len(input) && isASCIIDigit(rune(input[offset])) {
		offset++
	}
	if start == offset {
		return 0, fmt.Errorf("parse duration %q: expected digits at byte %d: %w", input, offset, errInvalidDuration)
	}

	unit, nextOffset, ok := scanUnit(input, offset)
	if !ok {
		return 0, fmt.Errorf("parse duration %q: expected unit at byte %d: %w", input, offset, errInvalidDuration)
	}
	err := appendToken(converted, input[start:offset], unit)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", input, err)
	}

	return nextOffset, nil
}

func scanUnit(input string, offset int) (string, int, bool) {
	for _, unit := range []string{"ms", "us", "ns", "w", "d", "h", "m", "s"} {
		if strings.HasPrefix(input[offset:], unit) {
			return unit, offset + len(unit), true
		}
	}

	return "", 0, false
}

func appendToken(converted *strings.Builder, number, unit string) error {
	switch unit {
	case "d":
		return appendHours(converted, number, hoursPerDay)
	case "w":
		return appendHours(converted, number, hoursPerWeek)
	default:
		converted.WriteString(number)
		converted.WriteString(unit)
		return nil
	}
}

func appendHours(converted *strings.Builder, number string, multiplier uint64) error {
	value, err := strconv.ParseUint(number, 10, 64)
	if err != nil {
		return fmt.Errorf("parse duration component %q: %w", number, err)
	}
	if value > maxDurationHours/multiplier {
		return fmt.Errorf("duration component %q overflows time.Duration: %w", number, errInvalidDuration)
	}

	converted.WriteString(strconv.FormatUint(value*multiplier, 10))
	converted.WriteByte('h')

	return nil
}

func isASCIIDigit(r rune) bool {
	return unicode.IsDigit(r) && r <= unicode.MaxASCII
}
