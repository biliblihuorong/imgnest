package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/gin-gonic/gin"
)

type pluginStub struct {
	name      string
	mountErr  error
	providers []extension.LoginProvider
}

func (p pluginStub) Name() string { return p.name }
func (p pluginStub) Mount(_ context.Context, router gin.IRouter) error {
	if p.mountErr != nil {
		return p.mountErr
	}
	router.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
	// A plugin that tries to claim a core path only lands inside its group.
	router.POST("/api/auth/login", func(c *gin.Context) { c.String(200, "hijacked") })
	return nil
}
func (p pluginStub) LoginProviders(context.Context) []extension.LoginProvider { return p.providers }

func pluginRouter(t *testing.T, plugins ...extension.Plugin) (http.Handler, error) {
	t.Helper()
	return httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		Users: &usersStub{}, Tokens: &tokensStub{}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Server: config.Server{TrustedProxies: []string{}},
		Now: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) }, Health: func(context.Context) error { return nil },
		Plugins: plugins,
	})
}

func TestPluginRoutesAreMountedUnderTheirOwnGroup(t *testing.T) {
	router, err := pluginRouter(t, pluginStub{name: "sso"})
	if err != nil {
		t.Fatal(err)
	}
	if w := request(t, router, "GET", "/api/ext/sso/ping", "", ""); w.Code != 200 || w.Body.String() != "pong" {
		t.Fatalf("plugin route: %d %q", w.Code, w.Body.String())
	}
	w := request(t, router, "POST", "/api/auth/login", `{"email":"tester@example.com","password":"secret-password"}`, "")
	if strings.Contains(w.Body.String(), "hijacked") {
		t.Fatal("plugin shadowed the core login route")
	}
}

func TestPluginLoginProvidersAppearInSiteView(t *testing.T) {
	provider := extension.LoginProvider{ID: "github", Name: "GitHub", StartURL: "/api/ext/sso/github/start"}
	router, err := pluginRouter(t, pluginStub{name: "sso", providers: []extension.LoginProvider{provider}})
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		SiteName       string                    `json:"site_name"`
		LoginProviders []extension.LoginProvider `json:"login_providers"`
	}
	if err := json.Unmarshal(envelope(t, request(t, router, "GET", "/api/site", "", ""))["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.SiteName != "ImgNest" || len(data.LoginProviders) != 1 || data.LoginProviders[0] != provider {
		t.Fatalf("site view = %+v", data)
	}
}

func TestSiteViewListsNoProvidersWithoutPlugins(t *testing.T) {
	router, err := pluginRouter(t)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(envelope(t, request(t, router, "GET", "/api/site", "", ""))["data"])
	if !strings.Contains(raw, `"login_providers":[]`) {
		t.Fatalf("login_providers must be an empty array: %s", raw)
	}
}

func TestInvalidPluginsAreRejected(t *testing.T) {
	cases := map[string][]extension.Plugin{
		"nil":       {nil},
		"bad name":  {pluginStub{name: "../admin"}},
		"empty":     {pluginStub{name: ""}},
		"duplicate": {pluginStub{name: "sso"}, pluginStub{name: "sso"}},
		"mount err": {pluginStub{name: "sso", mountErr: errors.New("boom")}},
	}
	for name, plugins := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := pluginRouter(t, plugins...); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
