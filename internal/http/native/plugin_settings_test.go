package native_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type pluginSettingsStub struct {
	name   string
	values string
	err    error
}

func (s *pluginSettingsStub) List(context.Context) ([]service.PluginSettingsView, error) {
	return []service.PluginSettingsView{{Name: "webhook", Title: "Webhook", Fields: []service.PluginField{}, Values: map[string]any{}, Status: []service.PluginStatus{}}}, s.err
}

func (s *pluginSettingsStub) Save(_ context.Context, name string, values json.RawMessage) (service.PluginSettingsView, error) {
	s.name, s.values = name, string(values)
	return service.PluginSettingsView{Name: name}, s.err
}

func pluginSettingsRouter(t *testing.T, settings native.PluginSettings) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, adminTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := handler.RegisterPluginSettingsRoutes(t.Context(), router, settings); err != nil {
		t.Fatal(err)
	}
	return router
}

func TestPluginSettingsRoutesAreAdminOnly(t *testing.T) {
	router := pluginSettingsRouter(t, &pluginSettingsStub{})
	for _, route := range []struct{ method, path, body string }{
		{"GET", "/api/admin/plugins", ""},
		{"PUT", "/api/admin/plugins/webhook/settings", `{"values":{}}`},
	} {
		if w := adminRequest(t, router, route.method, route.path, route.body, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s anonymous: %d", route.path, w.Code)
		}
		if w := adminRequest(t, router, route.method, route.path, route.body, userBearer); w.Code != http.StatusForbidden {
			t.Fatalf("%s user: %d", route.path, w.Code)
		}
		if w := adminRequest(t, router, route.method, route.path, route.body, adminBearer); w.Code != http.StatusOK {
			t.Fatalf("%s admin: %d %s", route.path, w.Code, w.Body.String())
		}
	}
}

func TestPluginSettingsSavePassesValuesAndMapsErrors(t *testing.T) {
	stub := &pluginSettingsStub{}
	router := pluginSettingsRouter(t, stub)
	if w := adminRequest(t, router, "PUT", "/api/admin/plugins/webhook/settings", `{"values":{"enabled":true}}`, adminBearer); w.Code != 200 {
		t.Fatalf("save: %d", w.Code)
	}
	if stub.name != "webhook" || stub.values != `{"enabled":true}` {
		t.Fatalf("service got %q %q", stub.name, stub.values)
	}
	for _, body := range []string{`{}`, `{"values":{},"other":1}`, `not json`} {
		if w := adminRequest(t, router, "PUT", "/api/admin/plugins/webhook/settings", body, adminBearer); w.Code != 400 {
			t.Fatalf("%s: %d", body, w.Code)
		}
	}
	for _, tc := range []struct {
		err     error
		status  int
		code    int
		message string
	}{
		{&service.PluginSettingsError{Field: "hosts", Message: "域名格式不正确"}, 400, 10005, "域名格式不正确"},
		{service.ErrPluginSecretsUnavailable, 409, 30014, ""},
		{service.ErrNotFound, 404, 10001, ""},
	} {
		stub.err = tc.err
		w := adminRequest(t, router, "PUT", "/api/admin/plugins/webhook/settings", `{"values":{}}`, adminBearer)
		if w.Code != tc.status {
			t.Fatalf("%v: status %d", tc.err, w.Code)
		}
		expectAdminCode(t, w, tc.code)
		if tc.message != "" && !strings.Contains(w.Body.String(), tc.message) {
			t.Fatalf("message missing: %s", w.Body.String())
		}
	}
}
