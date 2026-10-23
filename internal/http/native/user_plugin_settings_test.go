package native_test

import (
	"bytes"
	"context"
	"encoding/json"
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

type userPluginTokens struct{ adminTokensStub }

func (s userPluginTokens) Authenticate(ctx context.Context, raw string) (service.Identity, error) {
	if raw == "api" {
		id, _ := s.adminTokensStub.Authenticate(ctx, userBearer)
		id.Kind = service.TokenKindAPI
		return id, nil
	}
	return s.adminTokensStub.Authenticate(ctx, raw)
}

type userPluginsStub struct {
	native.UserPluginSettings
	owner uint64
	logo  []byte
}

func (s *userPluginsStub) List(_ context.Context, owner uint64) ([]service.UserPluginSettingsView, error) {
	s.owner = owner
	return []service.UserPluginSettingsView{}, nil
}
func (s *userPluginsStub) Save(_ context.Context, owner uint64, _ string, _ json.RawMessage) (service.UserPluginSettingsView, error) {
	s.owner = owner
	return service.UserPluginSettingsView{Name: "example"}, nil
}
func (s *userPluginsStub) Asset(_ context.Context, owner uint64, _ string) ([]byte, string, error) {
	s.owner = owner
	return []byte("private-logo"), "image/png", nil
}
func (s *userPluginsStub) PutAsset(_ context.Context, owner uint64, _ string, data []byte) (service.UserPluginSettingsView, error) {
	s.owner, s.logo = owner, data
	return service.UserPluginSettingsView{Name: "example"}, nil
}

func TestUserPluginRoutesUseWebSessionOwner(t *testing.T) {
	h, err := native.NewHandler(t.Context(), &siteUsersStub{}, userPluginTokens{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	s := &userPluginsStub{}
	if err := h.RegisterUserPluginSettingsRoutes(t.Context(), r, s); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		bearer string
		status int
	}{{"", 401}, {"api", 403}, {userBearer, 200}} {
		request := httptest.NewRequest(http.MethodGet, "/api/user/plugins", nil)
		if tc.bearer != "" {
			request.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		out := httptest.NewRecorder()
		r.ServeHTTP(out, request)
		if out.Code != tc.status {
			t.Fatalf("%q: %d %s", tc.bearer, out.Code, out.Body.String())
		}
	}
	if s.owner != 2 {
		t.Fatalf("owner = %d", s.owner)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/user/plugins/example/settings", strings.NewReader(`{"values":{},"user_id":1}`))
	request.Header.Set("Authorization", "Bearer "+userBearer)
	request.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	r.ServeHTTP(out, request)
	if out.Code != 400 {
		t.Fatalf("forged owner: %d", out.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/user/plugins/example/asset", nil)
	request.Header.Set("Authorization", "Bearer "+userBearer)
	out = httptest.NewRecorder()
	r.ServeHTTP(out, request)
	if out.Code != 200 || out.Header().Get("Cache-Control") != "private, no-store" || out.Header().Get("Content-Type") != "image/png" || out.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("asset response: %d %v", out.Code, out.Header())
	}
	for _, size := range []int{16, (4 << 20) + 1, (5 << 20) + 1} {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, _ := form.CreateFormFile("file", "logo.png")
		_, _ = file.Write(bytes.Repeat([]byte{1}, size))
		_ = form.Close()
		request = httptest.NewRequest(http.MethodPut, "/api/user/plugins/example/asset", &body)
		request.Header.Set("Authorization", "Bearer "+userBearer)
		request.Header.Set("Content-Type", form.FormDataContentType())
		out = httptest.NewRecorder()
		r.ServeHTTP(out, request)
		want := 200
		if size > 4<<20 {
			want = 413
		}
		if out.Code != want {
			t.Fatalf("size %d: %d %s", size, out.Code, out.Body.String())
		}
	}
	if len(s.logo) != 16 || s.owner != 2 {
		t.Fatalf("stored asset: %d bytes, owner %d", len(s.logo), s.owner)
	}
}
