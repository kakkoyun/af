package secret

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

const redactedValue = "<redacted>"

// NewRedactingHandler wraps next and redacts sensitive attribute keys.
func NewRedactingHandler(next slog.Handler, extraKeys []string) slog.Handler {
	return &redactingHandler{next: next, keys: redactKeySet(extraKeys)}
}

type redactingHandler struct {
	next slog.Handler
	keys map[string]struct{}
}

func (handler *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

func (handler *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(handler.redactAttr(attr))
		return true
	})

	err := handler.next.Handle(ctx, redacted)
	if err != nil {
		return fmt.Errorf("handle redacted log record: %w", err)
	}

	return nil
}

func (handler *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, handler.redactAttr(attr))
	}

	return &redactingHandler{next: handler.next.WithAttrs(redacted), keys: handler.keys}
}

func (handler *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: handler.next.WithGroup(name), keys: handler.keys}
}

func (handler *redactingHandler) redactAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if handler.isSensitive(attr.Key) {
		return slog.String(attr.Key, redactedValue)
	}
	if attr.Value.Kind() != slog.KindGroup {
		return attr
	}

	children := attr.Value.Group()
	redacted := make([]slog.Attr, 0, len(children))
	for _, child := range children {
		redacted = append(redacted, handler.redactAttr(child))
	}

	return slog.Group(attr.Key, attrsToAny(redacted)...)
}

func (handler *redactingHandler) isSensitive(key string) bool {
	_, ok := handler.keys[strings.ToLower(key)]
	return ok
}

func redactKeySet(extraKeys []string) map[string]struct{} {
	builtIns := []string{"api_key", "token", "password", "bearer", "secret", "auth"}
	keys := make(map[string]struct{}, len(builtIns)+len(extraKeys))
	for _, key := range builtIns {
		keys[key] = struct{}{}
	}
	for _, key := range extraKeys {
		keys[strings.ToLower(key)] = struct{}{}
	}

	return keys
}

func attrsToAny(attrs []slog.Attr) []any {
	values := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		values = append(values, attr)
	}

	return values
}
