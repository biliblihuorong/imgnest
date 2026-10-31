// Package logx builds the process logger and the attributes shared by every
// layer: the request ID carried in a context and panic descriptions with a
// goroutine stack.
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"runtime"
	"runtime/debug"
)

type requestIDKey struct{}

// New returns a JSON logger writing records at level or above to w. Records
// logged with a context that carries a request ID include it as request_id.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(contextHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

// WithRequestID returns ctx carrying id for the records logged under it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID carried by ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	if id := RequestID(ctx); id != "" && !hasRequestID(record) {
		record.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, record)
}

func hasRequestID(record slog.Record) bool {
	found := false
	record.Attrs(func(attr slog.Attr) bool {
		found = attr.Key == "request_id"
		return !found
	})
	return found
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

// Panic returns the attributes describing a recovered panic: what panicked
// and the stack of the panicking goroutine. Call it from the deferred
// function that recovered. Runtime faults (nil dereference, index out of
// range) are described by their message; any other value only by its type,
// since a payload passed to panic may hold request data or secrets.
func Panic(recovered any) []any {
	return []any{"panic", describe(recovered), "stack", Stack()}
}

func describe(recovered any) string {
	switch v := recovered.(type) {
	case runtime.Error:
		return v.Error()
	case nil:
		return "nil"
	default:
		return fmt.Sprintf("panic value of type %T", v)
	}
}

// Stack returns the calling goroutine's stack.
func Stack() string { return string(debug.Stack()) }

var secretShapes = []struct {
	pattern *regexp.Regexp
	replace string
}{
	// user:password@ in URLs and DSNs.
	{regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s]*@`), "${1}[redacted]@"},
	// key=value and key: value pairs whose key names a credential.
	{regexp.MustCompile(`(?i)((?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|authorization|dsn)\s*[=:]\s*)((?:bearer\s+|basic\s+)?(?:"[^"]*"|'[^']*'|\S+))`), "${1}[redacted]"},
	// Bearer credentials and ImgNest tokens (<id>|<secret>).
	{regexp.MustCompile(`(?i)(bearer\s+)\S+`), "${1}[redacted]"},
	{regexp.MustCompile(`\b\d+\|[A-Za-z0-9]{20,}`), "[redacted]"},
}

// Redact masks the credential shapes an error message may quote (URL user
// info, password=…, bearer tokens) before it is written to the log.
func Redact(message string) string {
	for _, shape := range secretShapes {
		message = shape.pattern.ReplaceAllString(message, shape.replace)
	}
	return message
}

// Describe names a recovered panic value as Panic does.
func Describe(recovered any) string { return describe(recovered) }
