package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/logx"
	"github.com/biliblihuorong/imgnest/internal/model"
)

// AuditRepository stores and reads the operation audit trail.
type AuditRepository interface {
	InsertAuditLogs(ctx context.Context, entries []model.AuditLog) error
	ListAuditLogs(ctx context.Context, query model.AuditQuery) ([]model.AuditLog, int64, error)
	PruneAuditLogs(ctx context.Context, before time.Time) (int64, error)
}

// AuditEntry is one state-changing API request as the HTTP layer saw it.
type AuditEntry struct {
	At        time.Time
	UserID    uint64
	UserName  string
	IP        string
	Method    string
	Route     string
	Target    string
	Status    int
	RequestID string
}

// AuditView is one audit entry shown to administrators.
type AuditView struct {
	ID        uint64    `json:"id"`
	At        time.Time `json:"at"`
	UserID    uint64    `json:"user_id"`
	UserName  string    `json:"user_name"`
	IP        string    `json:"ip"`
	Method    string    `json:"method"`
	Route     string    `json:"route"`
	Target    string    `json:"target"`
	Status    int       `json:"status"`
	RequestID string    `json:"request_id"`
}

// AuditPage is one page of the audit trail, newest first.
type AuditPage struct {
	Items []AuditView `json:"items"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}

// AuditDependencies wire the audit trail.
type AuditDependencies struct {
	Repo   AuditRepository
	Logger *slog.Logger
	Now    func() time.Time
	// Retention is how long entries are kept.
	Retention time.Duration
}

// Audit write pacing: entries are queued by requests and written in batches
// by one goroutine, so recording never delays a response.
const (
	auditQueue      = 1024
	auditBatch      = 100
	auditFlush      = time.Second
	auditPruneEvery = 6 * time.Hour
	auditFieldLimit = 256
)

// AuditService records operations off the request path and lists them.
type AuditService struct {
	deps    AuditDependencies
	queue   chan model.AuditLog
	dropped chan struct{}
}

// NewAuditService constructs the audit trail; Run must be started to write
// what Record queues.
func NewAuditService(ctx context.Context, deps AuditDependencies) (*AuditService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct audit service: %w", err)
	}
	if deps.Repo == nil || deps.Logger == nil || deps.Now == nil || deps.Retention <= 0 {
		return nil, fmt.Errorf("audit service dependencies: %w", ErrInvalidInput)
	}
	return &AuditService{deps: deps, queue: make(chan model.AuditLog, auditQueue), dropped: make(chan struct{}, 1)}, nil
}

// Record queues one entry without blocking. When the writer falls behind the
// entry is dropped and a warning logged once per flush.
func (s *AuditService) Record(_ context.Context, entry AuditEntry) {
	row := model.AuditLog{
		UserID: entry.UserID, UserName: clip(entry.UserName), IP: clip(entry.IP),
		Method: clip(entry.Method), Route: clip(entry.Route), Target: clip(entry.Target),
		Status: entry.Status, RequestID: clip(entry.RequestID), CreatedAt: entry.At.UTC(),
	}
	select {
	case s.queue <- row:
	default:
		select {
		case s.dropped <- struct{}{}:
		default:
		}
	}
}

// Run writes queued entries and prunes expired ones until ctx ends, then
// writes what is still queued.
func (s *AuditService) Run(ctx context.Context) {
	flush := time.NewTicker(auditFlush)
	defer flush.Stop()
	prune := time.NewTicker(auditPruneEvery)
	defer prune.Stop()
	s.prune(ctx)
	batch := make([]model.AuditLog, 0, auditBatch)
	write := func(ctx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := s.deps.Repo.InsertAuditLogs(ctx, batch); err != nil {
			s.deps.Logger.ErrorContext(ctx, "audit log write failed", "entries", len(batch), "error", logx.Redact(err.Error()))
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
		drain:
			for {
				select {
				case row := <-s.queue:
					batch = append(batch, row)
				default:
					break drain
				}
			}
			final, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			write(final)
			cancel()
			return
		case row := <-s.queue:
			batch = append(batch, row)
			if len(batch) >= auditBatch {
				write(ctx)
			}
		case <-flush.C:
			write(ctx)
			select {
			case <-s.dropped:
				s.deps.Logger.WarnContext(ctx, "audit queue full; entries dropped")
			default:
			}
		case <-prune.C:
			s.prune(ctx)
		}
	}
}

func (s *AuditService) prune(ctx context.Context) {
	if _, err := s.deps.Repo.PruneAuditLogs(ctx, s.deps.Now().Add(-s.deps.Retention)); err != nil && ctx.Err() == nil {
		s.deps.Logger.WarnContext(ctx, "audit log prune failed", "error", logx.Redact(err.Error()))
	}
}

// AuditLogs returns one page of the audit trail. Callers authorize the
// reader as an administrator.
func (s *AuditService) AuditLogs(ctx context.Context, query model.AuditQuery) (AuditPage, error) {
	if query.Page < 1 || query.Size < 1 || query.Size > 100 {
		return AuditPage{}, ErrInvalidInput
	}
	rows, total, err := s.deps.Repo.ListAuditLogs(ctx, query)
	if err != nil {
		return AuditPage{}, fmt.Errorf("list audit logs: %w", err)
	}
	items := make([]AuditView, 0, len(rows))
	for _, row := range rows {
		items = append(items, AuditView{
			ID: row.ID, At: row.CreatedAt, UserID: row.UserID, UserName: row.UserName, IP: row.IP,
			Method: row.Method, Route: row.Route, Target: row.Target, Status: row.Status, RequestID: row.RequestID,
		})
	}
	return AuditPage{Items: items, Total: total, Page: query.Page, Size: query.Size}, nil
}

// clip bounds a recorded field without splitting a UTF-8 sequence.
func clip(value string) string {
	if len(value) <= auditFieldLimit {
		return value
	}
	value = value[:auditFieldLimit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
