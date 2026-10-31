package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type auditRepoStub struct {
	mu       sync.Mutex
	inserted []model.AuditLog
	cutoff   time.Time
	query    model.AuditQuery
}

func (r *auditRepoStub) InsertAuditLogs(_ context.Context, rows []model.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inserted = append(r.inserted, rows...)
	return nil
}

func (r *auditRepoStub) ListAuditLogs(_ context.Context, query model.AuditQuery) ([]model.AuditLog, int64, error) {
	r.query = query
	return []model.AuditLog{{ID: 3, UserID: 1, Method: "POST", Route: "/api/upload", Status: 201}}, 1, nil
}

func (r *auditRepoStub) PruneAuditLogs(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cutoff = before
	return 0, nil
}

func TestAuditServiceWritesQueuedEntriesOnShutdown(t *testing.T) {
	repo := &auditRepoStub{}
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	svc, err := NewAuditService(t.Context(), AuditDependencies{Repo: repo, Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return now }, Retention: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("é", 300)
	svc.Record(t.Context(), AuditEntry{At: now, UserID: 1, UserName: long, Method: "POST", Route: "/api/upload", Status: 201})
	svc.Record(t.Context(), AuditEntry{At: now, Method: "POST", Route: "/api/auth/login", Status: 401})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); svc.Run(ctx) }()
	cancel()
	<-done
	if len(repo.inserted) != 2 || repo.inserted[1].Route != "/api/auth/login" {
		t.Fatalf("inserted=%+v", repo.inserted)
	}
	if name := repo.inserted[0].UserName; len(name) > auditFieldLimit || !strings.HasPrefix(long, name) {
		t.Fatalf("user name not clipped on a rune boundary: %d bytes", len(name))
	}
	if !repo.cutoff.Equal(now.Add(-90 * 24 * time.Hour)) {
		t.Fatalf("prune cutoff=%v", repo.cutoff)
	}
}

func TestAuditServiceDropsWhenQueueFull(t *testing.T) {
	svc, err := NewAuditService(t.Context(), AuditDependencies{Repo: &auditRepoStub{}, Logger: slog.New(slog.DiscardHandler), Now: time.Now, Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	for range auditQueue + 10 {
		svc.Record(t.Context(), AuditEntry{Method: "POST", Route: "/api/upload"})
	}
	if len(svc.queue) != auditQueue || len(svc.dropped) != 1 {
		t.Fatalf("queue=%d dropped=%d", len(svc.queue), len(svc.dropped))
	}
}

func TestAuditServiceListsPages(t *testing.T) {
	repo := &auditRepoStub{}
	svc, err := NewAuditService(t.Context(), AuditDependencies{Repo: repo, Logger: slog.New(slog.DiscardHandler), Now: time.Now, Retention: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuditLogs(t.Context(), model.AuditQuery{Page: 1, Size: 101}); err != ErrInvalidInput {
		t.Fatalf("oversized page err=%v", err)
	}
	page, err := svc.AuditLogs(t.Context(), model.AuditQuery{Page: 2, Size: 20, UserID: 1})
	if err != nil || page.Total != 1 || page.Page != 2 || len(page.Items) != 1 || page.Items[0].ID != 3 || repo.query.UserID != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if _, err := NewAuditService(t.Context(), AuditDependencies{Repo: repo, Logger: slog.New(slog.DiscardHandler), Now: time.Now}); err == nil {
		t.Fatal("zero retention accepted")
	}
}
