package lsky

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestForwardedProtoOnlyFromTrustedProxies(t *testing.T) {
	proxies, err := parseProxies([]string{"10.0.0.0/8", "192.0.2.10"})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{proxies: proxies}
	for _, tc := range []struct {
		peer, want string
	}{{"10.1.2.3:443", "https"}, {"192.0.2.10:443", "https"}, {"198.51.100.7:443", ""}, {"[::ffff:10.0.0.1]:443", "https"}} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/api/v1/images", nil)
		c.Request.RemoteAddr = tc.peer
		c.Request.Header.Set("X-Forwarded-Proto", "https")
		if got := h.forwardedProto(c); got != tc.want {
			t.Fatalf("peer %s: got %q want %q", tc.peer, got, tc.want)
		}
	}
	if _, err := parseProxies([]string{"not-an-ip"}); err == nil {
		t.Fatal("invalid proxy accepted")
	}
}
