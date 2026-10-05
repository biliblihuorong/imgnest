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

type captchaStub struct {
	native.CaptchaService
	enabled               bool
	calls                 int
	privateCalls          int
	err                   error
	lastAction, lastToken string
}

func (s *captchaStub) Public(context.Context) (service.CaptchaPublicView, error) {
	return service.CaptchaPublicView{Enabled: s.enabled, Provider: "turnstile", SiteKey: "public-key", Version: 1}, nil
}
func (s *captchaStub) Verify(_ context.Context, token, action string) error {
	s.calls++
	s.lastAction = action
	s.lastToken = token
	return s.err
}
func (s *captchaStub) Admin(context.Context, service.UserView) (service.CaptchaAdminView, error) {
	s.privateCalls++
	return service.CaptchaAdminView{}, s.err
}
func (s *captchaStub) SaveDraft(context.Context, service.UserView, service.CaptchaDraftInput) (service.CaptchaAdminView, error) {
	s.privateCalls++
	return service.CaptchaAdminView{}, s.err
}
func (s *captchaStub) TestDraft(context.Context, service.UserView, service.CaptchaTestInput) (service.CaptchaAdminView, error) {
	s.privateCalls++
	return service.CaptchaAdminView{}, s.err
}
func (s *captchaStub) Activate(_ context.Context, _ service.UserView, in service.CaptchaActivationInput) (service.CaptchaAdminView, error) {
	s.privateCalls++
	if s.err == nil {
		s.enabled = in.Enabled
	}
	return service.CaptchaAdminView{}, s.err
}

type captchaUsers struct {
	native.UserService
	logins, registrations int
	registrationEnabled   bool
}

func (s *captchaUsers) Site(context.Context) (service.SiteView, error) {
	return service.SiteView{RegisterEnabled: s.registrationEnabled}, nil
}
func (s *captchaUsers) VerifyCredentials(context.Context, string, string) (service.VerifiedCredentials, error) {
	s.logins++
	return service.VerifiedCredentials{}, service.ErrInvalidCredentials
}
func (s *captchaUsers) Register(context.Context, service.RegisterInput) (service.UserView, error) {
	s.registrations++
	return service.UserView{ID: 8}, nil
}
func captchaRouter(t *testing.T, cap *captchaStub, users *captchaUsers) http.Handler {
	t.Helper()
	h, err := native.NewHandler(t.Context(), users, adminTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterRoutes(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if cap != nil {
		if err := h.RegisterCaptchaRoutes(t.Context(), r, cap); err != nil {
			t.Fatal(err)
		}
	}
	return r
}
func captchaRequest(h http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestCaptchaNativeRateLimitRunsBeforeVerifierAndCredentials(t *testing.T) {
	for _, route := range []string{"login", "register"} {
		t.Run(route, func(t *testing.T) {
			cap := &captchaStub{enabled: true, err: service.ErrCaptchaFailed}
			users := &captchaUsers{registrationEnabled: true}
			h := captchaRouter(t, cap, users)
			for i := 1; i <= 4; i++ {
				w := captchaRequest(h, "POST", "/api/auth/"+route, `{"email":"a@example.com","password":"password","captcha_token":"token"}`, "")
				want := 422
				if i == 4 {
					want = 429
				}
				if w.Code != want {
					t.Fatalf("attempt %d status=%d body=%s", i, w.Code, w.Body)
				}
			}
			if cap.calls != 3 || users.logins != 0 || users.registrations != 0 {
				t.Fatalf("calls=%d login=%d register=%d", cap.calls, users.logins, users.registrations)
			}
			if cap.lastAction != route || cap.lastToken != "token" {
				t.Fatal("incorrect route proof")
			}
		})
	}
}
func TestCaptchaNativeStrictDTOAndClosedRegistration(t *testing.T) {
	cap := &captchaStub{err: service.ErrCaptchaFailed}
	users := &captchaUsers{}
	h := captchaRouter(t, cap, users)
	w := captchaRequest(h, "POST", "/api/auth/register", `{"username":"one","email":"a@example.com","password":"password"}`, "")
	if w.Code != 403 || !strings.Contains(w.Body.String(), `"code":30001`) || cap.calls != 0 {
		t.Fatalf("closed registration=%d %s calls=%d", w.Code, w.Body, cap.calls)
	}
	for _, body := range []string{`{"email":"a@example.com","password":"password","captcha_ok":true}`, `{"email":"a@example.com","password":"password","captcha_token":true}`} {
		w := captchaRequest(h, "POST", "/api/auth/login", body, "")
		if w.Code != 400 || cap.calls != 0 {
			t.Fatalf("strict DTO=%d %s", w.Code, w.Body)
		}
	}
}
func TestCaptchaNativeDisabledCompatibilityAndPublicMinimization(t *testing.T) {
	users := &captchaUsers{}
	h := captchaRouter(t, nil, users)
	w := captchaRequest(h, "POST", "/api/auth/login", `{"email":"a@example.com","password":"password"}`, "")
	if w.Code != 401 || users.logins != 1 {
		t.Fatalf("legacy payload broken=%d %s", w.Code, w.Body)
	}
	w = captchaRequest(h, "GET", "/api/auth/captcha", "", "")
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(envelope.Data) != 4 || string(envelope.Data["enabled"]) != "false" {
		t.Fatalf("public shape=%d %s", w.Code, w.Body)
	}
}

func TestCaptchaNativeLegacyHintsNeverBypassProtection(t *testing.T) {
	for _, route := range []string{"login", "register"} {
		t.Run(route, func(t *testing.T) {
			cap := &captchaStub{enabled: true, err: service.ErrCaptchaFailed}
			users := &captchaUsers{registrationEnabled: true}
			h := captchaRouter(t, cap, users)
			r := httptest.NewRequest("POST", "/api/auth/"+route+"?frontend=legacy", strings.NewReader(`{"username":"tester","email":"user@example.com","password":"password"}`))
			if route == "login" {
				r = httptest.NewRequest("POST", "/api/auth/login?frontend=legacy", strings.NewReader(`{"email":"user@example.com","password":"password"}`))
			}
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Frontend", "legacy")
			r.Header.Set("Frontend", "legacy")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 422 || cap.calls != 1 || users.logins != 0 || users.registrations != 0 {
				t.Fatalf("legacy hint bypassed CAPTCHA: %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestCaptchaNativePrivateEndpointsRequireAdmin(t *testing.T) {
	for _, route := range []struct{ method, path, body string }{
		{"GET", "/api/admin/captcha", ""},
		{"PUT", "/api/admin/captcha/draft", `{"expected_version":0,"provider":"turnstile","site_key":"site-key","hostnames":["images.example.com"],"secret":"new-secret"}`},
		{"POST", "/api/admin/captcha/test", `{"expected_version":1,"captcha_token":"new-token","action":"login"}`},
		{"POST", "/api/admin/captcha/activation", `{"expected_version":2,"enabled":false,"acknowledge_legacy_incompatibility":false,"acknowledge_v1_unprotected":false}`},
	} {
		for _, bearer := range []string{"", userBearer, adminBearer} {
			t.Run(route.path+bearer, func(t *testing.T) {
				cap := &captchaStub{}
				h := captchaRouter(t, cap, &captchaUsers{})
				w := captchaRequest(h, route.method, route.path, route.body, bearer)
				want := 200
				switch bearer {
				case "":
					want = 401
				case userBearer:
					want = 403
				}
				if w.Code != want {
					t.Fatalf("status=%d %s", w.Code, w.Body)
				}
				if want != 200 && cap.privateCalls != 0 {
					t.Fatal("private endpoint reached service")
				}
			})
		}
	}
}

func TestCaptchaNativeMutationsRequireExplicitVersionAndActivationDecision(t *testing.T) {
	for _, route := range []struct {
		name, method, path, body string
		fields                   []string
	}{
		{
			name: "draft", method: "PUT", path: "/api/admin/captcha/draft",
			body:   `{"expected_version":0,"provider":"turnstile","site_key":"site-key","hostnames":["images.example.com"],"secret":"new-secret"}`,
			fields: []string{"expected_version"},
		},
		{
			name: "test", method: "POST", path: "/api/admin/captcha/test",
			body:   `{"expected_version":1,"captcha_token":"new-token","action":"login"}`,
			fields: []string{"expected_version"},
		},
		{
			name: "activation", method: "POST", path: "/api/admin/captcha/activation",
			body:   `{"expected_version":2,"enabled":false,"acknowledge_legacy_incompatibility":false,"acknowledge_v1_unprotected":false}`,
			fields: []string{"expected_version", "enabled", "acknowledge_legacy_incompatibility", "acknowledge_v1_unprotected"},
		},
	} {
		for _, field := range route.fields {
			for _, value := range []string{"missing", "null"} {
				t.Run(route.name+"/"+field+"/"+value, func(t *testing.T) {
					var body map[string]json.RawMessage
					if err := json.Unmarshal([]byte(route.body), &body); err != nil {
						t.Fatal(err)
					}
					delete(body, field)
					if value == "null" {
						body[field] = json.RawMessage("null")
					}
					payload, err := json.Marshal(body)
					if err != nil {
						t.Fatal(err)
					}
					cap := &captchaStub{enabled: true}
					h := captchaRouter(t, cap, &captchaUsers{})
					w := captchaRequest(h, route.method, route.path, string(payload), adminBearer)
					if w.Code != 400 || cap.privateCalls != 0 || !cap.enabled {
						t.Fatalf("incomplete decision reached mutation: status=%d calls=%d enabled=%v body=%s", w.Code, cap.privateCalls, cap.enabled, w.Body)
					}
				})
			}
		}
	}
}

func TestCaptchaNativeErrorsAreDistinctAndSafe(t *testing.T) {
	for _, tc := range []struct {
		err          error
		status, code int
	}{{service.ErrCaptchaFailed, 422, 30010}, {service.ErrCaptchaConflict, 409, 30011}, {service.ErrCaptchaNotReady, 409, 30012}, {service.ErrCaptchaUnavailable, 503, 50004}} {
		cap := &captchaStub{err: tc.err}
		h := captchaRouter(t, cap, &captchaUsers{})
		w := captchaRequest(h, "POST", "/api/auth/login", `{"email":"private@example.com","password":"private-password","captcha_token":"private-token"}`, "")
		var envelope struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if w.Code != tc.status || envelope.Code != tc.code || strings.Contains(w.Body.String(), "private") {
			t.Fatalf("unsafe error %d %s", w.Code, w.Body)
		}
	}
}
