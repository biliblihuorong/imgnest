package http_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/gin-gonic/gin"
)

type apiSwitchStub struct {
	enabled bool
	err     error
}

func (s *apiSwitchStub) APIEnabled(context.Context) (bool, error) { return s.enabled, s.err }

func TestV2ReusesNativeHandlersBehindTheAPISwitch(t *testing.T) {
	images := &imagesStub{}
	router := imageRouter(t, images, native.ImageOptions{})
	// Same handler, same envelope: the v2 path behaves like the console route.
	expectCode(t, request(t, router, "PATCH", "/api/v2/images/1", `{"name":"v2.png"}`, testBearer), 200, 0)
	expectCode(t, request(t, router, "GET", "/api/v2/images/9/exif", "", testBearer), 404, 10001)
	expectCode(t, request(t, router, "GET", "/api/v2/profile", "", testBearer), 200, 0)
	expectCode(t, request(t, router, "GET", "/api/v2/profile", "", ""), 401, 20001)
	if strings.Join(images.actions, ",") != "rename:v2.png" {
		t.Fatalf("v2 rename did not reach the shared service: %v", images.actions)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler, err := native.NewHandler(t.Context(), &usersStub{}, &tokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.RegisterImageRoutes(t.Context(), engine, images, native.ImageOptions{}); err != nil {
		t.Fatal(err)
	}
	api := &apiSwitchStub{enabled: false}
	if err := handler.RegisterV2Routes(t.Context(), engine, api); err != nil {
		t.Fatal(err)
	}
	response := request(t, engine, "GET", "/api/v2/images/1", "", testBearer)
	expectCode(t, response, 403, 20003)
	if !strings.Contains(response.Body.String(), `"reason":"api_disabled"`) {
		t.Fatalf("disabled API body = %s", response.Body.String())
	}
	// The console route is not an API client and ignores the switch.
	expectCode(t, request(t, engine, "GET", "/api/images/1", "", testBearer), 200, 0)
	api.enabled = true
	expectCode(t, request(t, engine, "GET", "/api/v2/images/1", "", testBearer), 200, 0)
	api.err = errors.New("settings unavailable")
	expectCode(t, request(t, engine, "GET", "/api/v2/images/1", "", testBearer), 500, 50001)
}

// API tokens live in third-party upload clients: they reach the image API
// only while the switch is on, and never the console's management routes.
func TestAPITokensStayOutOfConsoleRoutesAndObeyTheSwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	tokens := &tokensStub{kind: "api", role: "admin"}
	handler, err := native.NewHandler(t.Context(), &usersStub{}, tokens, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	images := &imagesStub{}
	if err := handler.RegisterRoutes(t.Context(), engine); err != nil {
		t.Fatal(err)
	}
	if err := handler.RegisterImageRoutes(t.Context(), engine, images, native.ImageOptions{}); err != nil {
		t.Fatal(err)
	}
	overview := &overviewStub{}
	if err := handler.RegisterAdminOverviewRoutes(t.Context(), engine, overview); err != nil {
		t.Fatal(err)
	}
	api := &apiSwitchStub{enabled: true}
	if err := handler.RegisterV2Routes(t.Context(), engine, api); err != nil {
		t.Fatal(err)
	}
	forbidden := []struct{ method, path, body string }{
		{"GET", "/api/admin/overview", ""},
		{"GET", "/api/tokens", ""},
		{"POST", "/api/tokens", `{"name":"more"}`},
		{"DELETE", "/api/tokens/7", ""},
		{"PATCH", "/api/auth/password", `{"current_password":"a","new_password":"b"}`},
	}
	for _, route := range forbidden {
		response := request(t, engine, route.method, route.path, route.body, testBearer)
		expectCode(t, response, 403, 20003)
		if !strings.Contains(response.Body.String(), `"reason":"api_token_forbidden"`) {
			t.Fatalf("%s %s body = %s", route.method, route.path, response.Body.String())
		}
	}
	if overview.calls != 0 {
		t.Fatal("an API token reached the admin overview")
	}
	expectCode(t, request(t, engine, "GET", "/api/images/1", "", testBearer), 200, 0)
	api.enabled = false
	for _, path := range []string{"/api/images/1", "/api/v2/images/1"} {
		response := request(t, engine, "GET", path, "", testBearer)
		expectCode(t, response, 403, 20003)
		if !strings.Contains(response.Body.String(), `"reason":"api_disabled"`) {
			t.Fatalf("%s body = %s", path, response.Body.String())
		}
	}
	// A web session keeps the console whatever the switch says.
	tokens.kind = "web"
	expectCode(t, request(t, engine, "GET", "/api/images/1", "", testBearer), 200, 0)
	expectCode(t, request(t, engine, "GET", "/api/admin/overview", "", testBearer), 200, 0)
}
