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
