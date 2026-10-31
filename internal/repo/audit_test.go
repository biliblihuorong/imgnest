package repo

import (
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestAuditLogsInsertFilterAndPrune(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		audits, err := NewAuditRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		day := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		rows := []model.AuditLog{
			{UserID: 1, UserName: "alice", IP: "203.0.113.1", Method: "POST", Route: "/api/upload", Status: 201, RequestID: "r1", CreatedAt: day},
			{UserID: 2, UserName: "bob", Method: "DELETE", Route: "/api/images/:id", Target: "id=9", Status: 404, CreatedAt: day.Add(time.Hour)},
			{Method: "POST", Route: "/api/auth/login", Status: 401, CreatedAt: day.Add(2 * time.Hour)},
		}
		if err := audits.InsertAuditLogs(t.Context(), rows); err != nil {
			t.Fatal(err)
		}
		all, total, err := audits.ListAuditLogs(t.Context(), model.AuditQuery{Page: 1, Size: 2})
		if err != nil || total != 3 || len(all) != 2 || all[0].Route != "/api/auth/login" || all[1].Target != "id=9" || all[1].CreatedAt.Location() != time.UTC {
			t.Fatalf("page=%+v total=%d err=%v", all, total, err)
		}
		failed, total, err := audits.ListAuditLogs(t.Context(), model.AuditQuery{Page: 1, Size: 10, FailedOnly: true})
		if err != nil || total != 2 || len(failed) != 2 {
			t.Fatalf("failed=%+v total=%d err=%v", failed, total, err)
		}
		mine, total, err := audits.ListAuditLogs(t.Context(), model.AuditQuery{Page: 1, Size: 10, UserID: 1})
		if err != nil || total != 1 || mine[0].UserName != "alice" || mine[0].IP != "203.0.113.1" || mine[0].RequestID != "r1" || !mine[0].CreatedAt.Equal(day) {
			t.Fatalf("mine=%+v total=%d err=%v", mine, total, err)
		}
		pruned, err := audits.PruneAuditLogs(t.Context(), day.Add(90*time.Minute))
		if err != nil || pruned != 2 {
			t.Fatalf("pruned=%d err=%v", pruned, err)
		}
		if _, total, _ := audits.ListAuditLogs(t.Context(), model.AuditQuery{Page: 1, Size: 10}); total != 1 {
			t.Fatalf("after prune total=%d", total)
		}
	})
}
