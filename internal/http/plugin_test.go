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
	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type pluginStub struct {
	name      string
	mountErr  error
	providers []extension.LoginProvider
}

func (p pluginStub) Name() string { return p.name }
func (p pluginStub) Mount(_ context.Context, router gin.IRouter, host extension.Host) error {
	if p.mountErr != nil {
		return p.mountErr
	}
	router.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
	router.GET("/callback", func(c *gin.Context) {
		host.CompleteSignIn(c, extension.ExternalIdentity{Provider: "github", Subject: c.Query("sub"), Email: "octo@example.com", EmailVerified: true})
	})
	router.GET("/denied", func(c *gin.Context) { host.FailSignIn(c) })
	// A plugin that tries to claim a core path only lands inside its group.
	router.POST("/api/auth/login", func(c *gin.Context) { c.String(200, "hijacked") })
	return nil
}
func (p pluginStub) LoginProviders(context.Context) []extension.LoginProvider { return p.providers }

func pluginRouter(t *testing.T, plugins ...extension.Plugin) (http.Handler, error) {
	t.Helper()
	return pluginRouterWith(t, nil, plugins...)
}

func pluginRouterWith(t *testing.T, signIn native.ExternalSignIn, plugins ...extension.Plugin) (http.Handler, error) {
	t.Helper()
	return httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		ExternalSignIn: signIn,
		Users:          &usersStub{}, Tokens: &tokensStub{}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Server: config.Server{TrustedProxies: []string{}},
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

type signInStub struct {
	err error
	got service.ExternalSignIn
}

func (s *signInStub) SignInExternal(_ context.Context, in service.ExternalSignIn) (service.VerifiedCredentials, error) {
	s.got = in
	if s.err != nil {
		return service.VerifiedCredentials{}, s.err
	}
	return service.VerifiedCredentials{User: testUser()}, nil
}

func ticketFrom(t *testing.T, location string) string {
	t.Helper()
	ticket, ok := strings.CutPrefix(location, "/auth/sso#ticket=")
	if !ok || ticket == "" {
		t.Fatalf("redirect = %q", location)
	}
	return ticket
}

func TestPluginSignInIssuesOneTimeTicket(t *testing.T) {
	signIn := &signInStub{}
	router, err := pluginRouterWith(t, signIn, pluginStub{name: "sso"})
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, router, "GET", "/api/ext/sso/callback?sub=1001", "", "")
	if w.Code != http.StatusSeeOther || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("callback: %d %v", w.Code, w.Header())
	}
	if signIn.got.Provider != "sso:github" || signIn.got.Subject != "1001" || signIn.got.IP != "192.0.2.1" {
		t.Fatalf("sign-in input = %+v", signIn.got)
	}
	ticket := ticketFrom(t, w.Header().Get("Location"))
	body := `{"ticket":"` + ticket + `"}`
	first := request(t, router, "POST", "/api/auth/sso/exchange", body, "")
	expectCode(t, first, 200, 0)
	var data struct {
		Token string `json:"token"`
		User  struct {
			ID uint64 `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(envelope(t, first)["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.Token != testBearer || data.User.ID != 7 {
		t.Fatalf("exchange data = %+v", data)
	}
	// A ticket works once.
	expectCode(t, request(t, router, "POST", "/api/auth/sso/exchange", body, ""), 401, 20001)
	expectCode(t, request(t, router, "POST", "/api/auth/sso/exchange", `{"ticket":"forged"}`, ""), 401, 20001)
}

func TestPluginSignInErrorsRedirectToLogin(t *testing.T) {
	cases := map[string]struct {
		signIn native.ExternalSignIn
		path   string
		want   string
	}{
		"not linked":     {&signInStub{err: service.ErrIdentityNotLinked}, "/api/ext/sso/callback?sub=1", "not_linked"},
		"email required": {&signInStub{err: service.ErrIdentityEmailRequired}, "/api/ext/sso/callback?sub=1", "email_required"},
		"disabled":       {&signInStub{err: service.ErrForbidden}, "/api/ext/sso/callback?sub=1", "disabled"},
		"other":          {&signInStub{err: errors.New("db down")}, "/api/ext/sso/callback?sub=1", "failed"},
		"no service":     {nil, "/api/ext/sso/callback?sub=1", "unavailable"},
		"plugin denied":  {&signInStub{}, "/api/ext/sso/denied", "failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router, err := pluginRouterWith(t, tc.signIn, pluginStub{name: "sso"})
			if err != nil {
				t.Fatal(err)
			}
			w := request(t, router, "GET", tc.path, "", "")
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login?sso_error="+tc.want {
				t.Fatalf("got %d %q", w.Code, w.Header().Get("Location"))
			}
		})
	}
}
