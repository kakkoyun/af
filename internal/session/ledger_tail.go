package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"
)

// ReadLedgerTail returns the last n Events from ledger at path. If n is
// zero or negative, all events are returned. A missing file returns an
// empty slice and nil error. Blank and unparseable lines are skipped
// (with an slog warning) rather than failing the read.
func ReadLedgerTail(ctx context.Context, path string, n int) ([]Event, error) {
	file, err := os.Open(path) //nolint:gosec // Caller controls the ledger path.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open ledger %s: %w", path, err)
	}
	defer func() {
		_ = file.Close() //nolint:errcheck // Best-effort close on a read-only ledger handle.
	}()

	const (
		initialBufBytes = 64 * 1024
		maxBufBytes     = 16 * 1024 * 1024
	)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, initialBufBytes), maxBufBytes)
	events := make([]Event, 0)
	line := 0
	for scanner.Scan() {
		line++
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		event, parseErr := parseLedgerLine(scanner.Bytes())
		if parseErr != nil {
			// One corrupt line must not poison the whole ledger; the
			// remaining events still carry the session's history.
			slog.WarnContext(ctx, "skipping corrupt ledger line", "path", path, "line", line, "error", parseErr)
			continue
		}
		events = append(events, event)
	}
	err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("scan ledger %s: %w", path, err)
	}

	if n > 0 && len(events) > n {
		events = events[len(events)-n:]
	}
	return events, nil
}

func parseLedgerLine(line []byte) (Event, error) {
	var raw map[string]any
	err := json.Unmarshal(line, &raw)
	if err != nil {
		return Event{}, fmt.Errorf("parse ledger line: %w", err)
	}
	event := Event{Fields: make(map[string]any)}
	for key, value := range raw {
		switch key {
		// "event" is the canonical key written by marshalEvent; "type" is
		// accepted for forward compatibility with future writers and for
		// hand-written ledger fixtures.
		case "event", "type":
			if s, ok := value.(string); ok {
				event.Type = s
			}
		case "ts":
			if s, ok := value.(string); ok {
				ts, parseErr := time.Parse(time.RFC3339Nano, s)
				if parseErr == nil {
					event.Timestamp = ts
				}
			}
		default:
			event.Fields[key] = value
		}
	}
	return event, nil
}
