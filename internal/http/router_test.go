package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

const testBearer = "7|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type usersStub struct {
	err    error
	panics bool
}

func (s *usersStub) Register(_ context.Context, _ service.RegisterInput) (service.UserView, error) {
	if s.panics {
		panic("private-panic-token")
	}
	return testUser(), s.err
}
func (s *usersStub) VerifyCredentials(_ context.Context, _, _ string) (service.UserView, error) {
	return testUser(), s.err
}
func (s *usersStub) ChangePassword(_ context.Context, _ uint64, _, _ string) error { return s.err }
func testUser() service.UserView {
	return service.UserView{ID: 7, Username: "tester", Email: "tester@example.com", Role: "user", Status: "enabled", GroupID: 1, CreatedAt: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)}
}

type tokensStub struct {
	revoked   bool
	revokeErr error
}

func (s *tokensStub) Issue(_ context.Context, _ uint64, in service.TokenInput) (service.IssuedToken, error) {
	return service.IssuedToken{Token: testBearer, Info: service.TokenView{ID: 7, Name: in.Name, Kind: in.Kind, ExpiresAt: in.ExpiresAt, Abilities: []string{"*"}}}, nil
}
func (s *tokensStub) Authenticate(_ context.Context, raw string) (service.Identity, error) {
	if raw != testBearer || s.revoked {
		return service.Identity{}, service.ErrUnauthenticated
	}
	return service.Identity{User: testUser(), TokenID: 7, Kind: "web"}, nil
}
func (s *tokensStub) List(_ context.Context, _ uint64) ([]service.TokenView, error) {
	return []service.TokenView{}, nil
}
func (s *tokensStub) Revoke(_ context.Context, _, _ uint64) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	s.revoked = true
	return nil
}

func routerFixture(t *testing.T) (http.Handler, *usersStub, *tokensStub, *bytes.Buffer) {
	t.Helper()
	u, tokens, logs := &usersStub{}, &tokensStub{}, &bytes.Buffer{}
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		Users: u, Tokens: tokens, Logger: slog.New(slog.NewJSONHandler(logs, nil)), Server: config.Server{TrustedProxies: []string{}},
		Now: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) }, Health: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return router, u, tokens, logs
}
func request(t *testing.T, router http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
func envelope(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	for _, key := range []string{"code", "message", "data"} {
		if _, exists := body[key]; !exists {
			t.Fatalf("missing %s", key)
		}
	}
	return body
}
func expectCode(t *testing.T, w *httptest.ResponseRecorder, status, code int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP=%d want %d body=%s", w.Code, status, w.Body.String())
	}
	var actual int
	if err := json.Unmarshal(envelope(t, w)["code"], &actual); err != nil {
		t.Fatal(err)
	}
	if actual != code {
		t.Fatalf("code=%d want%d", actual, code)
	}
}

func TestLoginMeLogout(t *testing.T) {
	router, _, _, _ := routerFixture(t)
	login := request(t, router, http.MethodPost, "/api/auth/login", `{"email":"tester@example.com","password":"valid-password-123"}`, "")
	expectCode(t, login, 200, 0)
	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(envelope(t, login)["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.Token == "" {
		t.Fatal("login did not issue token")
	}
	expectCode(t, request(t, router, http.MethodGet, "/api/auth/me", "", data.Token), 200, 0)
	expectCode(t, request(t, router, http.MethodPost, "/api/auth/logout", "", data.Token), 200, 0)
	expectCode(t, request(t, router, http.MethodGet, "/api/auth/me", "", data.Token), 401, 20001)
}
func TestRegisterCannotEscalateRole(t *testing.T) {
	router, _, _, _ := routerFixture(t)
	for _, field := range []string{`"role":"admin"`, `"group_id":99`} {
		expectCode(t, request(t, router, "POST", "/api/auth/register", `{"username":"tester","email":"tester@example.com","password":"valid-password-123",`+field+`}`, ""), 400, 10001)
	}
}
func TestNativeEnvelopeAndErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		err          error
		status, code int
	}{
		{service.ErrInvalidInput, 400, 10001}, {service.ErrRegistrationDisabled, 403, 30001}, {service.ErrUserExists, 409, 30002}, {errors.New("private-password-and-dsn"), 500, 50001},
	} {
		router, users, _, logs := routerFixture(t)
		users.err = tc.err
		w := request(t, router, "POST", "/api/auth/register", `{"username":"tester","email":"tester@example.com","password":"valid-password-123"}`, "")
		expectCode(t, w, tc.status, tc.code)
		if string(envelope(t, w)["data"]) != "null" {
			t.Fatal("error data is not null")
		}
		if strings.Contains(w.Body.String(), "private-password-and-dsn") || strings.Contains(logs.String(), "private-password-and-dsn") {
			t.Fatal("technical secret leaked")
		}
	}
}
func TestTokenListNoSecret(t *testing.T) {
	router, _, _, logs := routerFixture(t)
	w := request(t, router, "GET", "/api/tokens", "", testBearer)
	expectCode(t, w, 200, 0)
	if string(envelope(t, w)["data"]) != "[]" {
		t.Fatal("empty token list is not array")
	}
	if strings.Contains(w.Body.String(), testBearer) || strings.Contains(logs.String(), testBearer) {
		t.Fatal("token leaked")
	}
}
func TestCrossUserTokenRevoke(t *testing.T) {
	router, _, tokens, _ := routerFixture(t)
	tokens.revokeErr = service.ErrForbidden
	expectCode(t, request(t, router, "DELETE", "/api/tokens/99", "", testBearer), 403, 20003)
}
func TestPasswordChangeReturnsNoCredentials(t *testing.T) {
	router, _, _, _ := routerFixture(t)
	// The service performs the atomic revocation; this verifies successful response and no new token issuance.
	w := request(t, router, "PATCH", "/api/auth/password", `{"current_password":"valid-password-123","new_password":"next-password-123"}`, testBearer)
	expectCode(t, w, 200, 0)
	if string(envelope(t, w)["data"]) != "null" {
		t.Fatal("password change returned credentials")
	}
}
func TestRateLimitRejectsFourthAttempt(t *testing.T) {
	router, users, _, _ := routerFixture(t)
	users.err = service.ErrInvalidCredentials
	for i := range 4 {
		status, code := 401, 20002
		if i == 3 {
			status, code = 429, 30003
		}
		expectCode(t, request(t, router, "POST", "/api/auth/login", `{"email":"tester@example.com","password":"wrong-password"}`, ""), status, code)
	}
}
func TestForwardedHeaderCannotBypassLimit(t *testing.T) {
	router, users, _, _ := routerFixture(t)
	users.err = service.ErrInvalidCredentials
	for i := range 4 {
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"email":"tester@example.com","password":"wrong-password"}`))
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Forwarded-For", []string{"198.51.100.1", "198.51.100.2", "198.51.100.3", "198.51.100.4"}[i])
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if i == 3 {
			expectCode(t, w, 429, 30003)
		}
	}
}
func TestMalformedJSONAndExtraBodyRejected(t *testing.T) {
	router, _, _, _ := routerFixture(t)
	for _, body := range []string{`{broken`, `{} {}`, strings.Repeat("x", 65537)} {
		expectCode(t, request(t, router, "POST", "/api/tokens", body, testBearer), 400, 10001)
	}
}
func TestUnauthorizedAndUnknownAPI(t *testing.T) {
	router, _, _, _ := routerFixture(t)
	expectCode(t, request(t, router, "GET", "/api/auth/me", "", "invalid"), 401, 20001)
	w := request(t, router, "GET", "/api/unknown?token=sensitive-query", "", "")
	if w.Code != 404 {
		t.Fatalf("unknown API=%d", w.Code)
	}
	envelope(t, w)
}
func TestHealthAndContextCancellation(t *testing.T) {
	router, _, _, _ := routerFixture(t)
	expectCode(t, request(t, router, "GET", "/healthz", "", ""), 200, 0)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := httpapi.NewRouter(ctx, httpapi.Dependencies{Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled router=%v", err)
	}
}

func TestPanicRecoveryAvoidsCredentialLogging(t *testing.T) {
	var panicLog bytes.Buffer
	previous := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &panicLog
	t.Cleanup(func() { gin.DefaultErrorWriter = previous })
	router, users, _, logs := routerFixture(t)
	users.panics = true
	w := request(t, router, "POST", "/api/auth/register", `{"username":"tester","email":"tester@example.com","password":"valid-password-123"}`, "")
	expectCode(t, w, 500, 50001)
	for _, secret := range []string{"private-panic-token", "valid-password-123"} {
		if strings.Contains(panicLog.String(), secret) || strings.Contains(logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
			t.Fatal("recovery logged a credential or panic payload")
		}
	}
}
