package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type overviewStub struct {
	calls int
	err   error
}

func (s *overviewStub) Overview(context.Context) (service.AdminOverview, error) {
	s.calls++
	if s.err != nil {
		return service.AdminOverview{}, s.err
	}
	return service.AdminOverview{
		Totals:   service.OverviewTotals{Users: 2, Images: 5, Bytes: 4096},
		Storages: []service.StorageUsageView{{ID: 1, Name: "local", Driver: "local", Images: 5, Bytes: 4096}},
		Policies: []service.PolicyUsageView{}, Uploads: []service.UploadDayView{}, TopUsers: []service.UserUsageView{},
	}, nil
}

func overviewRouter(t *testing.T, overview *overviewStub) http.Handler {
	t.Helper()
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		Users: &usersStub{}, Tokens: &roleTokens{}, Overview: overview,
		Logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)), Server: config.Server{TrustedProxies: []string{}},
		Now: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) }, Health: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestAdminOverviewRequiresAdmin(t *testing.T) {
	overview := &overviewStub{}
	router := overviewRouter(t, overview)
	expectCode(t, request(t, router, http.MethodGet, "/api/admin/overview", "", ""), 401, 20001)
	expectCode(t, request(t, router, http.MethodGet, "/api/admin/overview", "", testBearer), 403, 20003)
	if overview.calls != 0 {
		t.Fatal("non-administrator reached the overview service")
	}
	resp := request(t, router, http.MethodGet, "/api/admin/overview", "", adminBearer)
	expectCode(t, resp, 200, 0)
	raw := envelope(t, resp)["data"]
	var data service.AdminOverview
	if err := json.Unmarshal(raw, &data); err != nil || data.Totals.Bytes != 4096 || len(data.Storages) != 1 {
		t.Fatalf("overview = %+v, %v", data, err)
	}
	for _, secret := range []string{"config", "exif", "gps", "password", "token", "email"} {
		if strings.Contains(strings.ToLower(string(raw)), secret) {
			t.Fatalf("overview leaks %q: %s", secret, raw)
		}
	}
}

func TestAdminOverviewMapsFailure(t *testing.T) {
	router := overviewRouter(t, &overviewStub{err: service.ErrInvalidInput})
	expectCode(t, request(t, router, http.MethodGet, "/api/admin/overview", "", adminBearer), 400, 10001)
}
