package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// refererGuard refuses requests whose Referer mentions "evil" and records
// which kinds of route it saw.
type refererGuard struct {
	pluginStub
	kinds *[]string
}

func (g refererGuard) GuardAccess(c *gin.Context, kind string) bool {
	*g.kinds = append(*g.kinds, kind)
	if strings.Contains(c.GetHeader("Referer"), "evil") {
		c.String(http.StatusForbidden, "hotlink")
		return false
	}
	return true
}

type randomStub struct{ picks int }

func (r *randomStub) Get(context.Context, uint64, uint64) (*service.RandomLinkView, error) {
	return nil, service.ErrNotFound
}
func (r *randomStub) Put(context.Context, uint64, uint64, bool) (service.RandomLinkView, error) {
	return service.RandomLinkView{}, service.ErrNotFound
}
func (r *randomStub) Reset(context.Context, uint64, uint64) (service.RandomLinkView, error) {
	return service.RandomLinkView{}, service.ErrNotFound
}
func (r *randomStub) Delete(context.Context, uint64, uint64) error { return service.ErrNotFound }
func (r *randomStub) Pick(context.Context, string, string, bool) (string, error) {
	r.picks++
	return "https://cdn.example.com/a.webp", nil
}

func guardedRouter(t *testing.T, images *imagesStub, random *randomStub, plugins ...extension.Plugin) http.Handler {
	t.Helper()
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{Users: &usersStub{}, Tokens: &tokensStub{}, Images: images, RandomLinks: random, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Now: time.Now, Health: func(context.Context) error { return nil }, Plugins: plugins})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func get(router http.Handler, path, referer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	r.RemoteAddr = "192.0.2.1:1234"
	if referer != "" {
		r.Header.Set("Referer", referer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestAccessGuardRunsBeforePublicImageRoutes(t *testing.T) {
	var kinds []string
	images, random := &imagesStub{}, &randomStub{}
	router := guardedRouter(t, images, random, refererGuard{pluginStub: pluginStub{name: "hotlink"}, kinds: &kinds})
	random10, random24 := strings.Repeat("a", 10), strings.Repeat("b", 24)
	for _, path := range []string{"/i/1/a.png", "/t/key.webp", "/random/" + random10 + "/" + random24} {
		if w := get(router, path, "https://evil.example/post"); w.Code != http.StatusForbidden || w.Body.String() != "hotlink" {
			t.Fatalf("%s from a refused referer: %d %q", path, w.Code, w.Body.String())
		}
		if w := get(router, path, "https://blog.example/post"); w.Code >= 400 {
			t.Fatalf("%s from an allowed referer: %d", path, w.Code)
		}
	}
	if random.picks != 1 {
		t.Fatalf("refused random request still drew an image: %d picks", random.picks)
	}
	want := "object,object,thumbnail,thumbnail,random,random"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("guard kinds: %s", got)
	}
}

func TestPluginsWithoutAccessGuardLeaveImagesAlone(t *testing.T) {
	router := guardedRouter(t, &imagesStub{}, &randomStub{}, pluginStub{name: "sso"})
	if w := get(router, "/i/1/a.png", "https://evil.example/post"); w.Code != http.StatusOK {
		t.Fatalf("image refused without a guard: %d", w.Code)
	}
}
