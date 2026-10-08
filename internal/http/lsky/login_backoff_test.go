package lsky_test

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// The v1 token route cannot carry a captcha, so repeated wrong passwords for
// one account are paused even when they arrive from many addresses.
func TestV1TokensPauseAccountAfterRepeatedFailures(t *testing.T) {
	fixture := newV1Fixture(t, "sqlite")
	post := func(i int, password string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader("email=Alice@example.com&password="+password))
		r.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", i)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		fixture.router.ServeHTTP(w, r)
		return w
	}
	for i := range 10 {
		mustStatus(t, post(i, "not-the-password"), 200)
	}
	paused := post(20, alicePassword)
	mustStatus(t, paused, 429)
	assertGolden(t, "tokens_throttled.json", paused.Body.Bytes())

	other := newV1Fixture(t, "sqlite")
	fixture = other
	for i := range 9 {
		mustStatus(t, post(i, "not-the-password"), 200)
	}
	ok := post(30, alicePassword)
	mustStatus(t, ok, 200)
	assertGolden(t, "tokens_success.json", ok.Body.Bytes())
}
