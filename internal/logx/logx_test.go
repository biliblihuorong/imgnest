package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactMasksCredentialShapes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"dial postgres://imgnest:hunter2@db:5432/imgnest", "dial postgres://[redacted]@db:5432/imgnest"},
		{"redis://:hunter2@cache:6379/0 refused", "redis://[redacted]@cache:6379/0 refused"},
		{"host=db user=imgnest password=hunter2 dbname=x", "host=db user=imgnest password=[redacted] dbname=x"},
		{`secret: "hunter2" rejected`, `secret: [redacted] rejected`},
		{"Authorization: Bearer abc.def", "Authorization: [redacted]"},
		{"bearer 12|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bearer [redacted]"},
		{"token 12|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa expired", "token [redacted] expired"},
		{"put images/a.webp: storage operation failed (AccessDenied)", "put images/a.webp: storage operation failed (AccessDenied)"},
	} {
		if got := Redact(tc.in); got != tc.want || strings.Contains(got, "hunter2") {
			t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPanicDescribesRuntimeFaultsOnly(t *testing.T) {
	var runtimeFault any
	func() {
		defer func() { runtimeFault = recover() }()
		var counter *struct{ n int }
		_ = counter.n
	}()
	attrs := Panic(runtimeFault)
	if attrs[1] != "runtime error: invalid memory address or nil pointer dereference" || !strings.HasPrefix(attrs[3].(string), "goroutine ") {
		t.Fatalf("runtime fault attrs=%v", attrs[:2])
	}
	if got := Describe("password=hunter2"); got != "panic value of type string" {
		t.Fatalf("string panic described as %q", got)
	}
}

func TestLoggerAddsRequestIDAndLevel(t *testing.T) {
	var out bytes.Buffer
	log := New(&out, slog.LevelWarn)
	ctx := WithRequestID(context.Background(), "req-0001")
	log.InfoContext(ctx, "hidden")
	log.WarnContext(ctx, "shown")
	log.WarnContext(ctx, "explicit", "request_id", "req-0001")
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%q, want warn records only", lines)
	}
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["request_id"] != "req-0001" || strings.Count(line, "request_id") != 1 {
			t.Fatalf("record=%s", line)
		}
	}
}
