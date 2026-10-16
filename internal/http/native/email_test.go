package native_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type emailAuthStub struct {
	required       bool
	err            error
	purpose, email string
	sends, resets  int
}

func (s *emailAuthStub) EmailCodeRequired(context.Context) (bool, error) { return s.required, nil }
func (s *emailAuthStub) SendEmailCode(_ context.Context, purpose, email string) error {
	s.sends++
	s.purpose, s.email = purpose, email
	return s.err
}
func (s *emailAuthStub) ResetPasswordWithCode(context.Context, string, string, string) error {
	s.resets++
	return s.err
}

type mailSettingsStub struct {
	native.MailSettings
	err error
	to  string
}

func (s *mailSettingsStub) Config(context.Context, service.UserView) (service.MailConfigView, error) {
	return service.MailConfigView{Host: "smtp.example.com"}, s.err
}
func (s *mailSettingsStub) Test(_ context.Context, _ service.UserView, to string) error {
	s.to = to
	return s.err
}

type codeUsers struct {
	captchaUsers
	input service.RegisterInput
}

func (s *codeUsers) Register(_ context.Context, in service.RegisterInput) (service.UserView, error) {
	s.registrations++
	s.input = in
	return service.UserView{ID: 8}, nil
}

func emailRouter(t *testing.T, cap *captchaStub, users native.UserService, email *emailAuthStub, mail *mailSettingsStub) http.Handler {
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
	if err := h.RegisterCaptchaRoutes(t.Context(), r, cap); err != nil {
		t.Fatal(err)
	}
	if err := h.RegisterEmailRoutes(t.Context(), r, email, mail); err != nil {
		t.Fatal(err)
	}
	return r
}

func mailEnvelope(t *testing.T, body []byte) (int, json.RawMessage) {
	t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Code, envelope.Data
}

func TestEmailCodeRequestPassesTheMatchingCaptcha(t *testing.T) {
	for purpose, action := range map[string]string{"register": "register", "reset_password": "reset"} {
		cap := &captchaStub{enabled: true}
		email := &emailAuthStub{}
		h := emailRouter(t, cap, &captchaUsers{registrationEnabled: true}, email, &mailSettingsStub{})
		w := captchaRequest(h, "POST", "/api/auth/email-code", `{"email":"a@example.com","purpose":"`+purpose+`","captcha_token":"tok"}`, "")
		if w.Code != 200 || cap.lastAction != action || email.purpose != purpose || email.email != "a@example.com" {
			t.Fatalf("%s: status=%d action=%q purpose=%q body=%s", purpose, w.Code, cap.lastAction, email.purpose, w.Body)
		}
	}
	cap := &captchaStub{enabled: true, err: service.ErrCaptchaFailed}
	email := &emailAuthStub{}
	h := emailRouter(t, cap, &captchaUsers{}, email, &mailSettingsStub{})
	if w := captchaRequest(h, "POST", "/api/auth/email-code", `{"email":"a@example.com","purpose":"register"}`, ""); w.Code != 422 || email.sends != 0 {
		t.Fatalf("failed captcha still sent: status=%d sends=%d", w.Code, email.sends)
	}
	if w := captchaRequest(h, "POST", "/api/auth/email-code", `{"email":"a@example.com","purpose":"login"}`, ""); w.Code != 400 {
		t.Fatalf("unknown purpose status=%d", w.Code)
	}
}

func TestRegisterWithEmailCodeSkipsCaptcha(t *testing.T) {
	cap := &captchaStub{enabled: true, err: service.ErrCaptchaFailed}
	users := &codeUsers{captchaUsers: captchaUsers{registrationEnabled: true}}
	h := emailRouter(t, cap, users, &emailAuthStub{required: true}, &mailSettingsStub{})
	w := captchaRequest(h, "POST", "/api/auth/register", `{"username":"alice","email":"a@example.com","password":"correct-password-123","email_code":"123456"}`, "")
	if w.Code != 201 || cap.calls != 0 || users.input.EmailCode != "123456" {
		t.Fatalf("status=%d captcha calls=%d code=%q", w.Code, cap.calls, users.input.EmailCode)
	}
	// Without verification the CAPTCHA still guards registration.
	users = &codeUsers{captchaUsers: captchaUsers{registrationEnabled: true}}
	h = emailRouter(t, cap, users, &emailAuthStub{}, &mailSettingsStub{})
	w = captchaRequest(h, "POST", "/api/auth/register", `{"username":"alice","email":"a@example.com","password":"correct-password-123","email_code":"123456"}`, "")
	if w.Code != 422 || users.registrations != 0 {
		t.Fatalf("status=%d registrations=%d", w.Code, users.registrations)
	}
}

func TestEmailErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   int
	}{
		{service.ErrEmailCodeInvalid, 422, 30018},
		{service.ErrEmailCodeCooldown, 429, 30019},
		{service.ErrMailNotConfigured, 409, 30017},
		{service.ErrMailUnavailable, 503, 50006},
	} {
		h := emailRouter(t, &captchaStub{}, &captchaUsers{}, &emailAuthStub{err: tc.err}, &mailSettingsStub{})
		w := captchaRequest(h, "POST", "/api/auth/password-reset", `{"email":"a@example.com","code":"123456","new_password":"correct-password-123"}`, "")
		code, data := mailEnvelope(t, w.Body.Bytes())
		if w.Code != tc.status || code != tc.code || string(data) != "null" {
			t.Fatalf("%v: status=%d code=%d data=%s", tc.err, w.Code, code, data)
		}
	}
}

func TestMailSettingsAreAdminOnlyAndReportTheFailedStep(t *testing.T) {
	mail := &mailSettingsStub{}
	h := emailRouter(t, &captchaStub{}, &captchaUsers{}, &emailAuthStub{}, mail)
	if w := captchaRequest(h, "GET", "/api/admin/mail", "", userBearer); w.Code != 403 {
		t.Fatalf("user read mail settings: %d", w.Code)
	}
	if w := captchaRequest(h, "GET", "/api/admin/mail", "", ""); w.Code != 401 {
		t.Fatalf("anonymous read mail settings: %d", w.Code)
	}
	if w := captchaRequest(h, "GET", "/api/admin/mail", "", adminBearer); w.Code != 200 {
		t.Fatalf("admin read mail settings: %d", w.Code)
	}
	mail.err = &service.MailDeliveryError{Reason: "auth"}
	w := captchaRequest(h, "POST", "/api/admin/mail/test", `{"to":"x@example.com"}`, adminBearer)
	code, data := mailEnvelope(t, w.Body.Bytes())
	if w.Code != 502 || code != 50006 || string(data) != `{"reason":"auth"}` || mail.to != "x@example.com" {
		t.Fatalf("status=%d code=%d data=%s to=%q", w.Code, code, data, mail.to)
	}
}
