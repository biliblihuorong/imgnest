package lsky_test

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// The v1 token route cannot carry a captcha, so repeated wrong passwords for
// one account are paused even when they arrive from many addresses; the pause
// only covers addresses that failed, so it never locks the owner out.
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
	// An address that already failed stays paused, even with the password.
	paused := post(3, alicePassword)
	mustStatus(t, paused, 429)
	assertGolden(t, "tokens_throttled.json", paused.Body.Bytes())
	// A fresh address gets one guess, then it is paused too.
	mustStatus(t, post(20, "not-the-password"), 200)
	mustStatus(t, post(20, alicePassword), 429)
	// The owner, from an address that never failed, still signs in.
	ok := post(30, alicePassword)
	mustStatus(t, ok, 200)
	assertGolden(t, "tokens_success.json", ok.Body.Bytes())

	other := newV1Fixture(t, "sqlite")
	fixture = other
	for i := range 9 {
		mustStatus(t, post(i, "not-the-password"), 200)
	}
	mustStatus(t, post(5, alicePassword), 200)
}
