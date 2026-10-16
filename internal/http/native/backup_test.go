package native_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type backupArchiveStub struct{ *bytes.Reader }

func (backupArchiveStub) Close() error { return nil }

type backupsStub struct {
	schedule service.BackupSchedule
	deleted  string
	err      error
}

func (s *backupsStub) Overview(context.Context) (service.BackupOverview, error) {
	return service.BackupOverview{Items: []model.BackupFile{{Name: "imgnest-20261016-080000.zip", Size: 3}}, Location: "data/backup"}, s.err
}

func (s *backupsStub) Create(context.Context) (model.BackupFile, error) {
	return model.BackupFile{Name: "imgnest-20261016-080000.zip", Size: 3}, s.err
}

func (s *backupsStub) Open(_ context.Context, name string) (service.BackupArchive, model.BackupFile, error) {
	if name != "imgnest-20261016-080000.zip" {
		return nil, model.BackupFile{}, service.ErrNotFound
	}
	return backupArchiveStub{bytes.NewReader([]byte("zip"))}, model.BackupFile{Name: name, Size: 3}, nil
}

func (s *backupsStub) Delete(_ context.Context, name string) error {
	s.deleted = name
	return s.err
}

func (s *backupsStub) PutSchedule(_ context.Context, schedule service.BackupSchedule) (service.BackupSchedule, error) {
	s.schedule = schedule
	return schedule, s.err
}

func backupRouter(t *testing.T, stub *backupsStub) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, variantTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := handler.RegisterBackupRoutes(t.Context(), router, stub); err != nil {
		t.Fatal(err)
	}
	return router
}

func backupRequest(router *gin.Engine, method, path, bearer string, body io.Reader) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, body)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestBackupRoutesRequireAdmin(t *testing.T) {
	router := backupRouter(t, &backupsStub{})
	for _, bearer := range []string{"", "user-token"} {
		for _, route := range [][2]string{{http.MethodGet, "/api/admin/backups"}, {http.MethodPost, "/api/admin/backups"}, {http.MethodGet, "/api/admin/backups/imgnest-20261016-080000.zip"}} {
			w := backupRequest(router, route[0], route[1], bearer, nil)
			if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
				t.Fatalf("%s %s bearer %q: status %d", route[0], route[1], bearer, w.Code)
			}
		}
	}
}

func TestBackupRoutesListCreateDownloadDeleteAndSchedule(t *testing.T) {
	stub := &backupsStub{}
	router := backupRouter(t, stub)
	if w := backupRequest(router, http.MethodGet, "/api/admin/backups", "admin-token", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"location":"data/backup"`) {
		t.Fatalf("overview: %d %s", w.Code, w.Body.String())
	}
	if w := backupRequest(router, http.MethodPost, "/api/admin/backups", "admin-token", nil); w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	w := backupRequest(router, http.MethodGet, "/api/admin/backups/imgnest-20261016-080000.zip", "admin-token", nil)
	if w.Code != 200 || w.Body.String() != "zip" || w.Header().Get("Content-Type") != "application/zip" ||
		w.Header().Get("Content-Disposition") != `attachment; filename="imgnest-20261016-080000.zip"` {
		t.Fatalf("download: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
	if w := backupRequest(router, http.MethodGet, "/api/admin/backups/other.zip", "admin-token", nil); w.Code != 404 {
		t.Fatalf("missing download: %d", w.Code)
	}
	if w := backupRequest(router, http.MethodDelete, "/api/admin/backups/imgnest-20261016-080000.zip", "admin-token", nil); w.Code != 200 || stub.deleted != "imgnest-20261016-080000.zip" {
		t.Fatalf("delete: %d %q", w.Code, stub.deleted)
	}
	w = backupRequest(router, http.MethodPut, "/api/admin/backups/schedule", "admin-token", strings.NewReader(`{"enabled":true,"interval_hours":12,"keep":5}`))
	if w.Code != 200 || stub.schedule != (service.BackupSchedule{Enabled: true, IntervalHours: 12, Keep: 5}) {
		t.Fatalf("schedule: %d %+v", w.Code, stub.schedule)
	}
	stub.err = service.ErrBackupBusy
	w = backupRequest(router, http.MethodPost, "/api/admin/backups", "admin-token", nil)
	var envelope struct{ Code int }
	_ = json.Unmarshal(w.Body.Bytes(), &envelope)
	if w.Code != 409 || envelope.Code != 30020 {
		t.Fatalf("busy: %d code %d", w.Code, envelope.Code)
	}
}
