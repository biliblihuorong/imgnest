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
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	routeUID   = "Ab3dE6gH9j"
	routeToken = "0123456789abcdefghijABCD"
	routePath  = "/random/" + routeUID + "/" + routeToken
)

type randomLinksStub struct {
	view      *service.RandomLinkView
	err       error
	pickURL   string
	pickErr   error
	pickCalls [][3]any
	calls     []string
	putArg    *bool
	owner     uint64
	album     uint64
}

func (s *randomLinksStub) Get(_ context.Context, owner, album uint64) (*service.RandomLinkView, error) {
	s.calls, s.owner, s.album = append(s.calls, "get"), owner, album
	return s.view, s.err
}

func (s *randomLinksStub) Put(_ context.Context, owner, album uint64, enabled bool) (service.RandomLinkView, error) {
	s.calls, s.owner, s.album, s.putArg = append(s.calls, "put"), owner, album, &enabled
	if s.err != nil {
		return service.RandomLinkView{}, s.err
	}
	return service.RandomLinkView{Enabled: enabled, Path: routePath}, nil
}

func (s *randomLinksStub) Reset(_ context.Context, owner, album uint64) (service.RandomLinkView, error) {
	s.calls, s.owner, s.album = append(s.calls, "reset"), owner, album
	if s.err != nil {
		return service.RandomLinkView{}, s.err
	}
	return service.RandomLinkView{Enabled: true, Path: routePath}, nil
}

func (s *randomLinksStub) Delete(_ context.Context, owner, album uint64) error {
	s.calls, s.owner, s.album = append(s.calls, "delete"), owner, album
	return s.err
}

func (s *randomLinksStub) Pick(_ context.Context, uid, token string, original bool) (string, error) {
	s.pickCalls = append(s.pickCalls, [3]any{uid, token, original})
	return s.pickURL, s.pickErr
}

func randomRouter(t *testing.T, links *randomLinksStub) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, albumTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := handler.RegisterRoutes(t.Context(), router); err != nil {
		t.Fatal(err)
	}
	if err := handler.RegisterRandomLinkRoutes(t.Context(), router, links); err != nil {
		t.Fatal(err)
	}
	return router
}

func TestRegisterRandomLinkRoutesRequiresService(t *testing.T) {
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, albumTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.RegisterRandomLinkRoutes(t.Context(), gin.New(), nil); err == nil {
		t.Fatal("nil random link service accepted")
	}
}

func TestRandomRedirects(t *testing.T) {
	links := &randomLinksStub{pickURL: "https://cdn.example/a.webp"}
	router := randomRouter(t, links)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := callAlbum(t, router, method, routePath, "", "")
		if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != "https://cdn.example/a.webp" {
			t.Fatalf("%s: status=%d location=%q", method, w.Code, w.Header().Get("Location"))
		}
		if cache := w.Header().Get("Cache-Control"); cache != "no-store" {
			t.Fatalf("%s: Cache-Control=%q, want no-store", method, cache)
		}
		if strings.Contains(w.Body.String(), `"code"`) {
			t.Fatalf("%s: redirect carried a JSON envelope: %s", method, w.Body.String())
		}
	}
	if len(links.pickCalls) != 2 || links.pickCalls[0] != [3]any{routeUID, routeToken, false} {
		t.Fatalf("pick calls = %v", links.pickCalls)
	}
}

func TestRandomFormat(t *testing.T) {
	links := &randomLinksStub{pickURL: "https://cdn.example/a.jpg"}
	router := randomRouter(t, links)
	if w := callAlbum(t, router, http.MethodGet, routePath+"?format=original", "", ""); w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("format=original status=%d", w.Code)
	}
	if len(links.pickCalls) != 1 || links.pickCalls[0][2] != true {
		t.Fatalf("format=original pick = %v", links.pickCalls)
	}
	for _, bad := range []string{"png", "webp", "ORIGINAL", ""} {
		links.pickCalls = nil
		w := callAlbum(t, router, http.MethodGet, routePath+"?format="+bad, "", "")
		if bad == "" {
			if w.Code != http.StatusTemporaryRedirect || links.pickCalls[0][2] != false {
				t.Fatalf("empty format: status=%d pick=%v", w.Code, links.pickCalls)
			}
			continue
		}
		if w.Code != http.StatusBadRequest || len(links.pickCalls) != 0 {
			t.Fatalf("format=%s: status=%d pick=%v, want 400 without a lookup", bad, w.Code, links.pickCalls)
		}
	}
}

func TestRandomNotFound(t *testing.T) {
	links := &randomLinksStub{pickErr: service.ErrNotFound}
	router := randomRouter(t, links)
	w := callAlbum(t, router, http.MethodGet, routePath, "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d", w.Code)
	}
	var body native.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Code != 10001 || body.Message != "not found" || body.Data != nil {
		t.Fatalf("body=%s err=%v", w.Body.String(), err)
	}
}

func TestRandomRejectsMalformedWithoutLookup(t *testing.T) {
	links := &randomLinksStub{pickURL: "https://cdn.example/a.webp"}
	router := randomRouter(t, links)
	for _, path := range []string{
		"/random/Ab3dE6gH9/" + routeToken,
		"/random/Ab3dE6gH9jX/" + routeToken,
		"/random/Ab3dE6-H9j/" + routeToken,
		"/random/" + routeUID + "/0123456789abcdefghijABC",
		"/random/" + routeUID + "/0123456789abcdefghij%00BC",
		"/random/" + routeUID + "/" + strings.Repeat("a", 5000),
		"/random/" + routeUID + "/0123456789abcdefghij%2FBCD",
	} {
		w := callAlbum(t, router, http.MethodGet, path, "", "")
		if w.Code != http.StatusNotFound {
			t.Fatalf("%.60s: status=%d, want 404", path, w.Code)
		}
	}
	if len(links.pickCalls) != 0 {
		t.Fatalf("malformed segments reached the service: %d calls", len(links.pickCalls))
	}
}

func TestRandomRateLimited(t *testing.T) {
	links := &randomLinksStub{pickURL: "https://cdn.example/a.webp"}
	router := randomRouter(t, links)
	for i := range 600 {
		if w := callAlbum(t, router, http.MethodGet, routePath, "", ""); w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("request %d: status=%d", i+1, w.Code)
		}
	}
	w := callAlbum(t, router, http.MethodGet, routePath, "", "")
	var body native.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusTooManyRequests || body.Code != 30003 {
		t.Fatalf("request 601: status=%d body=%s", w.Code, w.Body.String())
	}
	// The limit is per client address: another visitor is still served. Table
	// isolation from the login limiter is pinned by TestLimitersAreIndependent.
	other := httptest.NewRequest(http.MethodGet, routePath, nil)
	other.RemoteAddr = "198.51.100.7:4000"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, other)
	if recorder.Code != http.StatusTemporaryRedirect {
		t.Fatalf("second client status=%d, want 307", recorder.Code)
	}
}

func TestRandomLinkManagement(t *testing.T) {
	links := &randomLinksStub{}
	router := randomRouter(t, links)
	const base = "/api/albums/5/random-link"

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, base}, {http.MethodPut, base}, {http.MethodPost, base + "/reset"}, {http.MethodDelete, base},
	} {
		if w := callAlbum(t, router, tc.method, tc.path, "", ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without bearer: status=%d", tc.method, tc.path, w.Code)
		}
	}
	if len(links.calls) != 0 {
		t.Fatalf("unauthenticated requests reached the service: %v", links.calls)
	}

	w := callAlbum(t, router, http.MethodGet, base, "", testCredential)
	if code, data := decodeEnvelope(t, w); w.Code != 200 || code != 0 || string(data) != "null" {
		t.Fatalf("get without link: status=%d code=%d data=%s", w.Code, code, data)
	}
	if links.album != 5 || links.owner == 0 {
		t.Fatalf("get reached owner=%d album=%d", links.owner, links.album)
	}

	links.view = &service.RandomLinkView{Enabled: true, Path: routePath}
	w = callAlbum(t, router, http.MethodGet, base, "", testCredential)
	var view service.RandomLinkView
	if _, data := decodeEnvelope(t, w); json.Unmarshal(data, &view) != nil || view.Path != routePath || !view.Enabled {
		t.Fatalf("get with link: %s", w.Body.String())
	}

	w = callAlbum(t, router, http.MethodPut, base, `{"enabled":false}`, testCredential)
	if _, data := decodeEnvelope(t, w); w.Code != 200 || json.Unmarshal(data, &view) != nil || view.Path != routePath || view.Enabled {
		t.Fatalf("put: status=%d body=%s", w.Code, w.Body.String())
	}
	if links.putArg == nil || *links.putArg {
		t.Fatal("put did not forward enabled=false")
	}
	for _, body := range []string{`{}`, `not json`, `{"enabled":null}`, `{"enabled":true,"token":"x"}`, ``} {
		links.calls = nil
		if w := callAlbum(t, router, http.MethodPut, base, body, testCredential); w.Code != http.StatusBadRequest || len(links.calls) != 0 {
			t.Fatalf("put %q: status=%d calls=%v, want 400 without a service call", body, w.Code, links.calls)
		}
	}

	if w := callAlbum(t, router, http.MethodPost, base+"/reset", "", testCredential); w.Code != 200 {
		t.Fatalf("reset: status=%d", w.Code)
	}
	w = callAlbum(t, router, http.MethodDelete, base, "", testCredential)
	if code, data := decodeEnvelope(t, w); w.Code != 200 || code != 0 || string(data) != "null" {
		t.Fatalf("delete: status=%d data=%s", w.Code, data)
	}

	for _, id := range []string{"0", "abc", "-1"} {
		if w := callAlbum(t, router, http.MethodGet, "/api/albums/"+id+"/random-link", "", testCredential); w.Code != http.StatusBadRequest {
			t.Fatalf("album id %q: status=%d, want 400", id, w.Code)
		}
	}

	links.err = service.ErrForbidden
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, base, ""}, {http.MethodPut, base, `{"enabled":true}`}, {http.MethodPost, base + "/reset", ""}, {http.MethodDelete, base, ""},
	} {
		w := callAlbum(t, router, tc.method, tc.path, tc.body, testCredential)
		if code, _ := decodeEnvelope(t, w); w.Code != http.StatusForbidden || code != 20003 {
			t.Fatalf("%s foreign album: status=%d code=%d", tc.method, w.Code, code)
		}
	}
	links.err = service.ErrNotFound
	if w := callAlbum(t, router, http.MethodPost, base+"/reset", "", testCredential); w.Code != http.StatusNotFound {
		t.Fatalf("reset without link: status=%d", w.Code)
	}
}
