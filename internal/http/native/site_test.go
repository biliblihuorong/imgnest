package native_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type siteUsersStub struct {
	native.UserService
	view service.SiteView
	err  error
}

func (s *siteUsersStub) Site(context.Context) (service.SiteView, error) { return s.view, s.err }

type rejectingTokensStub struct{ native.TokenService }

func (rejectingTokensStub) Authenticate(context.Context, string) (service.Identity, error) {
	return service.Identity{}, service.ErrUnauthenticated
}

func siteRouter(t *testing.T, users *siteUsersStub) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), users, rejectingTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := handler.RegisterRoutes(t.Context(), router); err != nil {
		t.Fatal(err)
	}
	return router
}

func requestSite(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestSiteViewIsPublicAndExact(t *testing.T) {
	for _, tc := range []struct {
		name     string
		enabled  bool
		gallery  bool
		expected string
	}{
		{"registration open", true, false, `{"site_name":"ImgNest","register_enabled":true,"gallery_enabled":false}`},
		{"registration closed", false, false, `{"site_name":"ImgNest","register_enabled":false,"gallery_enabled":false}`},
		{"gallery open", true, true, `{"site_name":"ImgNest","register_enabled":true,"gallery_enabled":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := siteRouter(t, &siteUsersStub{view: service.SiteView{SiteName: "ImgNest", RegisterEnabled: tc.enabled, GalleryEnabled: tc.gallery}})
			response := requestSite(t, router, "/api/site")
			if response.Code != http.StatusOK {
				t.Fatalf("anonymous site status=%d", response.Code)
			}
			var envelope struct {
				Code int             `json:"code"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != 0 || string(envelope.Data) != tc.expected {
				t.Fatalf("site envelope code=%d data=%s", envelope.Code, envelope.Data)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(envelope.Data, &fields); err != nil {
				t.Fatal(err)
			}
			if len(fields) != 3 {
				t.Fatalf("site data exposes %d fields: %s", len(fields), envelope.Data)
			}
			// The public descriptor must not change unmatched API routing.
			if unmatched := requestSite(t, router, "/api/unknown"); unmatched.Code != http.StatusNotFound {
				t.Fatalf("unmatched path status=%d", unmatched.Code)
			}
		})
	}
}

func TestSiteViewFailureMapsToInternalCode(t *testing.T) {
	router := siteRouter(t, &siteUsersStub{err: errors.New("private-setting-diagnostics")})
	response := requestSite(t, router, "/api/site")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failing site status=%d", response.Code)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var code int
	if err := json.Unmarshal(envelope["code"], &code); err != nil || code != 50001 {
		t.Fatalf("failing site code=%d err=%v", code, err)
	}
	if raw, exists := envelope["data"]; !exists || string(raw) != "null" {
		t.Fatalf("failing site data=%s exists=%t", raw, exists)
	}
	var message string
	if err := json.Unmarshal(envelope["message"], &message); err != nil || strings.TrimSpace(message) == "" {
		t.Fatalf("failing site message=%q err=%v", message, err)
	}
	if strings.Contains(response.Body.String(), "private-setting-diagnostics") {
		t.Fatal("site failure leaked diagnostics")
	}
}
