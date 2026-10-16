package native_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type variantsStub struct {
	links   []string
	actions []string
}

func (s *variantsStub) LookupVariants(_ context.Context, links []string) ([]service.VariantLookupItem, error) {
	s.links = links
	items := []service.VariantLookupItem{}
	for _, link := range links {
		items = append(items, service.VariantLookupItem{URL: link, Found: true, Variants: []service.VariantView{{URL: "https://cdn.test/a_w400_v1.webp", Width: 400, Height: 300, Bytes: 1234, MIME: "image/webp"}}})
	}
	return items, nil
}
func (s *variantsStub) VariantBackfillStatus(context.Context, service.TokenSubject) (service.VariantBackfillView, error) {
	s.actions = append(s.actions, "status")
	return service.VariantBackfillView{VariantBackfill: model.VariantBackfill{Status: model.BackfillIdle}}, nil
}
func (s *variantsStub) StartVariantBackfill(_ context.Context, _ service.TokenSubject, restart bool) (service.VariantBackfillView, error) {
	s.actions = append(s.actions, "start:"+map[bool]string{true: "restart", false: "resume"}[restart])
	return service.VariantBackfillView{VariantBackfill: model.VariantBackfill{Status: model.BackfillRunning}, Active: true}, nil
}
func (s *variantsStub) PauseVariantBackfill(context.Context, service.TokenSubject) (service.VariantBackfillView, error) {
	s.actions = append(s.actions, "pause")
	return service.VariantBackfillView{VariantBackfill: model.VariantBackfill{Status: model.BackfillPaused}}, nil
}

type variantTokensStub struct{ native.TokenService }

func (variantTokensStub) Authenticate(_ context.Context, raw string) (service.Identity, error) {
	switch raw {
	case "admin-token":
		return service.Identity{User: service.UserView{ID: 1, Role: model.UserRoleAdmin}, TokenID: 7, Kind: "web"}, nil
	case "user-token":
		return service.Identity{User: service.UserView{ID: 2, Role: model.UserRoleUser}, TokenID: 8, Kind: "web"}, nil
	}
	return service.Identity{}, service.ErrUnauthenticated
}

func variantRouter(t *testing.T, stub *variantsStub, opts native.VariantOptions) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, variantTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store") })
	if err := handler.RegisterRoutes(t.Context(), router); err != nil {
		t.Fatal(err)
	}
	if err := handler.RegisterVariantRoutes(t.Context(), router, stub, opts); err != nil {
		t.Fatal(err)
	}
	return router
}

func callVariants(router http.Handler, method, target, body, origin, bearer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestVariantLookupRoutesCORSCacheAndLimits(t *testing.T) {
	stub := &variantsStub{}
	router := variantRouter(t, stub, native.VariantOptions{CORSOrigins: []string{"https://blog.example"}, PerMinute: 3})
	w := callVariants(router, http.MethodGet, "/api/variants?url=https%3A%2F%2Fcdn.test%2Fa.jpg&url=https%3A%2F%2Fcdn.test%2Fb.jpg", "", "https://blog.example", "")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "https://blog.example" || w.Header().Get("Cache-Control") != "public, max-age=300" || !strings.Contains(w.Header().Get("Vary"), "Origin") {
		t.Fatalf("GET lookup: %d %v", w.Code, w.Header())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Items []service.VariantLookupItem `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code != 0 || len(body.Data.Items) != 2 || body.Data.Items[0].Variants[0].Bytes != 1234 {
		t.Fatalf("lookup body: %s", w.Body.String())
	}
	if len(stub.links) != 2 || stub.links[1] != "https://cdn.test/b.jpg" {
		t.Fatalf("links passed = %v", stub.links)
	}
	w = callVariants(router, http.MethodPost, "/api/variants", `{"urls":["https://cdn.test/a.jpg"]}`, "https://evil.example", "")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("POST lookup from another origin: %d %v", w.Code, w.Header())
	}
	w = callVariants(router, http.MethodOptions, "/api/variants", "", "https://blog.example", "")
	if w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "POST") {
		t.Fatalf("preflight: %d %v", w.Code, w.Header())
	}
	tooMany := make([]string, service.MaxVariantLookup+1)
	for i := range tooMany {
		tooMany[i] = `"https://cdn.test/x.jpg"`
	}
	w = callVariants(router, http.MethodPost, "/api/variants", `{"urls":[`+strings.Join(tooMany, ",")+`]}`, "", "")
	if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("oversized batch: %d", w.Code)
	}
	w = callVariants(router, http.MethodGet, "/api/variants?url=x", "", "", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit not applied: %d", w.Code)
	}
}

func TestVariantLookupWildcardOrigin(t *testing.T) {
	router := variantRouter(t, &variantsStub{}, native.VariantOptions{CORSOrigins: []string{"*"}})
	w := callVariants(router, http.MethodGet, "/api/variants?url=x", "", "https://any.example", "")
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("wildcard origin not applied")
	}
	router = variantRouter(t, &variantsStub{}, native.VariantOptions{})
	w = callVariants(router, http.MethodGet, "/api/variants?url=x", "", "https://any.example", "")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS granted without configuration")
	}
}

func TestVariantBackfillRoutesRequireAdmin(t *testing.T) {
	stub := &variantsStub{}
	router := variantRouter(t, stub, native.VariantOptions{})
	if w := callVariants(router, http.MethodGet, "/api/admin/variants/backfill", "", "", ""); w.Code != 401 {
		t.Fatalf("anonymous status: %d", w.Code)
	}
	if w := callVariants(router, http.MethodPost, "/api/admin/variants/backfill", `{"action":"start"}`, "", "user-token"); w.Code != 403 {
		t.Fatalf("member start: %d", w.Code)
	}
	for _, body := range []string{`{"action":"start"}`, `{"action":"start","restart":true}`, `{"action":"pause"}`} {
		if w := callVariants(router, http.MethodPost, "/api/admin/variants/backfill", body, "", "admin-token"); w.Code != 200 {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	if w := callVariants(router, http.MethodPost, "/api/admin/variants/backfill", `{"action":"explode"}`, "", "admin-token"); w.Code != 400 {
		t.Fatalf("unknown action: %d", w.Code)
	}
	if w := callVariants(router, http.MethodGet, "/api/admin/variants/backfill", "", "", "admin-token"); w.Code != 200 {
		t.Fatalf("admin status: %d", w.Code)
	}
	if strings.Join(stub.actions, ",") != "start:resume,start:restart,pause,status" {
		t.Fatalf("actions = %v", stub.actions)
	}
}
