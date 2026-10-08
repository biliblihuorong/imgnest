package cli

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

type recovererFunc func(context.Context) error

func (f recovererFunc) Recover(ctx context.Context) error { return f(ctx) }

func TestRecoverAtStartupKeepsServingWhenStorageFails(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	var deadline time.Time
	recoverAtStartup(t.Context(), recovererFunc(func(ctx context.Context) error {
		deadline, _ = ctx.Deadline()
		return errors.New("storage unreachable: secret-provider-detail")
	}), logger)
	if deadline.IsZero() {
		t.Fatal("startup recovery has no deadline")
	}
	if !strings.Contains(logs.String(), "50002") || strings.Contains(logs.String(), "secret-provider-detail") {
		t.Fatalf("recovery failure not logged safely: %s", logs.String())
	}
}
