package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/service"
)

const adminBearer = "8|bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// roleTokens authenticates testBearer as a user and adminBearer as an
// administrator, and records the client details the router attached.
type roleTokens struct {
	tokensStub
	client service.ClientInfo
}

func (s *roleTokens) Authenticate(ctx context.Context, raw string) (service.Identity, error) {
	s.client = service.ClientFrom(ctx)
	if raw == adminBearer {
		admin := testUser()
		admin.ID, admin.Role = 8, "admin"
		return service.Identity{User: admin, TokenID: 8, Kind: "web"}, nil
	}
	return s.tokensStub.Authenticate(ctx, raw)
}

type activityStub struct{ asked []uint64 }

func (s *activityStub) Activity(_ context.Context, id uint64) (service.ActivityView, error) {
	s.asked = append(s.asked, id)
	if id == 404 {
		return service.ActivityView{}, service.ErrNotFound
	}
	return service.ActivityView{Logins: []service.LoginView{}, Tokens: []service.TokenView{}, ActiveSessions: int(id % 100)}, nil
}

type systemStub struct{ refreshed bool }

func (s *systemStub) Version(_ context.Context, refresh bool) (service.SystemVersion, error) {
	s.refreshed = refresh
	return service.SystemVersion{Current: "0.0.9", UpdateAvailable: true}, nil
}

func activityRouter(t *testing.T) (http.Handler, *roleTokens, *activityStub, *systemStub) {
	t.Helper()
	tokens, activity, system := &roleTokens{}, &activityStub{}, &systemStub{}
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		Users: &usersStub{}, Tokens: tokens, Activity: activity, System: system,
		Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), Server: config.Server{TrustedProxies: []string{}},
		Now: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) }, Health: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return router, tokens, activity, system
}

func TestAccountActivityReadsOwnAccount(t *testing.T) {
	router, tokens, activity, _ := activityRouter(t)
	resp := request(t, router, http.MethodGet, "/api/account/activity", "", testBearer)
	expectCode(t, resp, 200, 0)
	if len(activity.asked) != 1 || activity.asked[0] != 7 {
		t.Fatalf("activity asked for %v, want the caller 7", activity.asked)
	}
	if tokens.client.IP != "192.0.2.1" {
		t.Fatalf("client IP %q was not attached to the request", tokens.client.IP)
	}
	expectCode(t, request(t, router, http.MethodGet, "/api/account/activity", "", ""), 401, 20001)
}

func TestAdminUserActivityRequiresAdmin(t *testing.T) {
	router, _, activity, _ := activityRouter(t)
	expectCode(t, request(t, router, http.MethodGet, "/api/admin/users/9/activity", "", testBearer), 403, 20003)
	if len(activity.asked) != 0 {
		t.Fatal("non-administrator reached the activity service")
	}
	resp := request(t, router, http.MethodGet, "/api/admin/users/9/activity", "", adminBearer)
	expectCode(t, resp, 200, 0)
	var data service.ActivityView
	if err := json.Unmarshal(envelope(t, resp)["data"], &data); err != nil || data.ActiveSessions != 9 {
		t.Fatalf("activity for user 9 = %+v, %v", data, err)
	}
	expectCode(t, request(t, router, http.MethodGet, "/api/admin/users/404/activity", "", adminBearer), 404, 10001)
	expectCode(t, request(t, router, http.MethodGet, "/api/admin/users/x/activity", "", adminBearer), 400, 10001)
}

func TestAdminSystemVersion(t *testing.T) {
	router, _, _, system := activityRouter(t)
	expectCode(t, request(t, router, http.MethodGet, "/api/admin/system/version", "", testBearer), 403, 20003)
	resp := request(t, router, http.MethodGet, "/api/admin/system/version?refresh=1", "", adminBearer)
	expectCode(t, resp, 200, 0)
	if !system.refreshed {
		t.Fatal("refresh=1 was not passed on")
	}
	var data service.SystemVersion
	if err := json.Unmarshal(envelope(t, resp)["data"], &data); err != nil || data.Current != "0.0.9" || !data.UpdateAvailable {
		t.Fatalf("version = %+v, %v", data, err)
	}
}
