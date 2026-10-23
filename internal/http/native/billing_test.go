package native_test

import (
	"context"
	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type billingStub struct {
	extension.BillingStore
	actor  extension.BillingActor
	filter extension.BillingFilter
}

func (s *billingStub) Overview(_ context.Context, a extension.BillingActor) (extension.BillingOverview, error) {
	s.actor = a
	return extension.BillingOverview{Currency: "CNY"}, nil
}
func (s *billingStub) Purchase(_ context.Context, a extension.BillingActor, _ extension.BillingPurchase) (extension.BillingOrder, error) {
	s.actor = a
	return extension.BillingOrder{ID: 12}, nil
}
func (s *billingStub) Ledger(_ context.Context, a extension.BillingActor, f extension.BillingFilter) (extension.BillingPage[extension.BillingEntry], error) {
	s.actor = a
	s.filter = f
	return extension.BillingPage[extension.BillingEntry]{Items: []extension.BillingEntry{}}, nil
}

type billingProviderStub struct {
	store   extension.BillingStore
	enabled bool
}

func (p *billingProviderStub) BillingService(context.Context) (extension.BillingStore, bool) {
	return p.store, p.enabled
}
func billingRouter(t *testing.T) (*gin.Engine, *billingStub, *billingProviderStub) {
	t.Helper()
	s := &billingStub{}
	p := &billingProviderStub{store: s, enabled: true}
	b, err := service.NewBillingService(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	h, err := native.NewHandler(t.Context(), &siteUsersStub{}, userPluginTokens{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	if err = h.RegisterBillingRoutes(t.Context(), r, b); err != nil {
		t.Fatal(err)
	}
	return r, s, p
}
func TestBillingHTTPUsesSessionOwner(t *testing.T) {
	r, s, _ := billingRouter(t)
	for _, path := range []string{"/api/user/billing", "/api/user/billing/ledger?user_id=1"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+userBearer)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 || s.actor.UserID != 2 || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if s.filter.All || s.filter.UserID != 2 {
		t.Fatalf("cross-owner filter=%+v", s.filter)
	}
	req := httptest.NewRequest("POST", "/api/user/billing/purchases", strings.NewReader(`{"plan_id":1,"plan_version":1,"request_id":"buy","user_id":1}`))
	req.Header.Set("Authorization", "Bearer "+userBearer)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("forged owner=%d %s", w.Code, w.Body.String())
	}
}
func TestBillingHTTPRejectsAPITokenAndNonAdmin(t *testing.T) {
	r, _, _ := billingRouter(t)
	for _, tc := range []struct {
		path, bearer string
		status       int
	}{{"/api/user/billing", "api", 403}, {"/api/user/billing", "", 401}, {"/api/admin/billing/wallets", userBearer, 403}} {
		req := httptest.NewRequest("GET", tc.path, nil)
		if tc.bearer != "" {
			req.Header.Set("Authorization", "Bearer "+tc.bearer)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
	}
}
func TestBillingHTTPDisabled(t *testing.T) {
	r, _, p := billingRouter(t)
	p.enabled = false
	req := httptest.NewRequest("GET", "/api/user/billing", nil)
	req.Header.Set("Authorization", "Bearer "+userBearer)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("disabled=%d %s", w.Code, w.Body.String())
	}
}
