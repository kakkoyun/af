package secret_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kakkoyun/af/internal/secret"
)

func TestRedactingHandler_WithAttrsRedactsEagerly(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(secret.NewRedactingHandler(base, nil)).With(
		slog.String("api_key", "sk-secret"),
		slog.String("host", "example.test"),
	)

	logger.InfoContext(context.Background(), "launch")

	out := buf.String()
	if strings.Contains(out, "sk-secret") {
		t.Fatalf("log output leaked pre-bound secret: %s", out)
	}
	for _, want := range []string{"\"api_key\":\"<redacted>\"", "\"host\":\"example.test\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output %s missing %s", out, want)
		}
	}
}

func TestRedactingHandler_WithGroupRedactsInsideGroup(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(secret.NewRedactingHandler(base, nil)).WithGroup("agent")

	logger.InfoContext(context.Background(), "launch",
		slog.String("token", "ghp_secret"),
		slog.String("name", "claude"),
	)

	out := buf.String()
	if strings.Contains(out, "ghp_secret") {
		t.Fatalf("log output leaked grouped secret: %s", out)
	}
	for _, want := range []string{"\"token\":\"<redacted>\"", "\"name\":\"claude\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output %s missing %s", out, want)
		}
	}
}

var errFakeSink = errors.New("sink closed")

// failingHandler always errors from Handle so the wrapping path is testable.
type failingHandler struct {
	err error
}

func (failingHandler) Enabled(context.Context, slog.Level) bool    { return true }
func (h failingHandler) Handle(context.Context, slog.Record) error { return h.err }
func (h failingHandler) WithAttrs([]slog.Attr) slog.Handler        { return h }
func (h failingHandler) WithGroup(string) slog.Handler             { return h }

func TestRedactingHandler_HandleWrapsNextError(t *testing.T) {
	handler := secret.NewRedactingHandler(failingHandler{err: errFakeSink}, nil)

	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "launch", 0)
	err := handler.Handle(context.Background(), record)
	if !errors.Is(err, errFakeSink) {
		t.Fatalf("Handle() error = %v, want wrapped sink error", err)
	}
}
