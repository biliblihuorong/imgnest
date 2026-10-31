package repo

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestSlowQueriesLoggedWithoutValues(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		var logs bytes.Buffer
		slow := db.Session(&gorm.Session{Logger: newSlowQueryLogger(db, slog.New(slog.NewJSONHandler(&logs, nil)), time.Nanosecond)})
		var users []model.User
		if err := slow.Where("email = ?", "private-address@example.com").Find(&users).Error; err != nil {
			t.Fatal(err)
		}
		logged := logs.String()
		if !strings.Contains(logged, `"msg":"slow database query"`) || !strings.Contains(logged, "email = '?'") || !strings.Contains(logged, `"source":`) {
			t.Fatalf("slow query log=%s", logged)
		}
		if strings.Contains(logged, "private-address") {
			t.Fatal("query value logged")
		}
		logs.Reset()
		fast := db.Session(&gorm.Session{Logger: newSlowQueryLogger(db, slog.New(slog.NewJSONHandler(&logs, nil)), time.Hour)})
		if err := fast.Find(&users).Error; err != nil || logs.Len() != 0 {
			t.Fatalf("fast query logged: %s %v", logs.String(), err)
		}
	})
}
