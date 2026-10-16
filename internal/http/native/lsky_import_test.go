package native_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type lskyImportStub struct {
	dump, zone string
	started    service.LskyImportOptions
	err        error
}

func (s *lskyImportStub) LoadDump(_ context.Context, body io.Reader, zone string) (service.LskyImportStatus, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return service.LskyImportStatus{}, err
	}
	s.dump, s.zone = string(data), zone
	return service.LskyImportStatus{Loaded: true, Phase: service.LskyPhaseIdle}, s.err
}
func (s *lskyImportStub) Discard(context.Context) (service.LskyImportStatus, error) {
	return service.LskyImportStatus{Phase: service.LskyPhaseIdle}, nil
}
func (s *lskyImportStub) Status(context.Context) (service.LskyImportStatus, error) {
	return service.LskyImportStatus{Phase: service.LskyPhaseIdle}, nil
}
func (s *lskyImportStub) Start(_ context.Context, opts service.LskyImportOptions) (service.LskyImportStatus, error) {
	s.started = opts
	return service.LskyImportStatus{Running: true, Phase: service.LskyPhaseAccounts}, s.err
}
func (s *lskyImportStub) Cancel(context.Context) (service.LskyImportStatus, error) {
	return service.LskyImportStatus{Phase: service.LskyPhaseCanceled}, nil
}

func lskyImportRouter(t *testing.T, stub *lskyImportStub) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, variantTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := handler.RegisterLskyImportRoutes(t.Context(), router, stub); err != nil {
		t.Fatal(err)
	}
	return router
}

func multipartDump(t *testing.T, content string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("note", "ignored"); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("file", "LsyPro.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(content))
	_ = form.Close()
	return &body, form.FormDataContentType()
}

func TestLskyImportRoutesRequireAdmin(t *testing.T) {
	router := lskyImportRouter(t, &lskyImportStub{})
	for _, bearer := range []string{"", "user-token"} {
		r := httptest.NewRequest(http.MethodGet, "/api/admin/migrations/lsky", nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
			t.Fatalf("bearer %q: status %d", bearer, w.Code)
		}
	}
}

func TestLskyImportUploadStreamsFilePart(t *testing.T) {
	stub := &lskyImportStub{}
	router := lskyImportRouter(t, stub)
	body, contentType := multipartDump(t, "INSERT INTO users ...")
	r := httptest.NewRequest(http.MethodPost, "/api/admin/migrations/lsky/dump?timezone=UTC", body)
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 || stub.dump != "INSERT INTO users ..." || stub.zone != "UTC" {
		t.Fatalf("status %d dump %q zone %q", w.Code, stub.dump, stub.zone)
	}
	stub.err = service.ErrNotLskyDump
	body, contentType = multipartDump(t, "garbage")
	r = httptest.NewRequest(http.MethodPost, "/api/admin/migrations/lsky/dump", body)
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Authorization", "Bearer admin-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var envelope struct{ Code int }
	_ = json.Unmarshal(w.Body.Bytes(), &envelope)
	if w.Code != 400 || envelope.Code != 10006 || stub.zone != "Asia/Shanghai" {
		t.Fatalf("not a dump: status %d code %d zone %q", w.Code, envelope.Code, stub.zone)
	}
}

func TestLskyImportStartDecodesOptions(t *testing.T) {
	stub := &lskyImportStub{}
	router := lskyImportRouter(t, stub)
	r := httptest.NewRequest(http.MethodPost, "/api/admin/migrations/lsky/start", strings.NewReader(`{"policy_id":1,"mode":"adopt","strategy_id":9}`))
	r.Header.Set("Authorization", "Bearer admin-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 || stub.started.PolicyID != 1 || stub.started.Mode != "adopt" || stub.started.StrategyID != 9 {
		t.Fatalf("status %d started %+v", w.Code, stub.started)
	}
	stub.err = service.ErrImportBusy
	r = httptest.NewRequest(http.MethodPost, "/api/admin/migrations/lsky/start", strings.NewReader(`{"policy_id":1,"mode":"adopt"}`))
	r.Header.Set("Authorization", "Bearer admin-token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatalf("busy start: status %d", w.Code)
	}
}
