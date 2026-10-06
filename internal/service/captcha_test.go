package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/captcha"
	"github.com/biliblihuorong/imgnest/internal/secret"
)

type captchaMemory struct {
	mu  sync.Mutex
	raw json.RawMessage
	err error
}

func (r *captchaMemory) ReadCaptcha(context.Context) (json.RawMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append(json.RawMessage(nil), r.raw...), r.err
}
func (r *captchaMemory) CompareAndSwapCaptcha(_ context.Context, old, next json.RawMessage) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return false, r.err
	}
	if !bytes.Equal(old, r.raw) {
		return false, nil
	}
	r.raw = append(json.RawMessage(nil), next...)
	return true, nil
}

type captchaVerifyFunc func(context.Context, captcha.Request) error

func (f captchaVerifyFunc) Verify(ctx context.Context, in captcha.Request) error { return f(ctx, in) }

var captchaAdmin = UserView{ID: 1, Role: "admin", Status: "enabled"}

func newCaptchaTestService(t *testing.T) (*CaptchaService, *captchaMemory) {
	t.Helper()
	codec, err := secret.NewCodec(t.Context(), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	store := &captchaMemory{}
	svc, err := NewCaptchaService(t.Context(), CaptchaDependencies{Repository: store, Secrets: codec, Verifier: captchaVerifyFunc(func(_ context.Context, in captcha.Request) error {
		if in.Secret != "fixture-secret" && in.Secret != "rotated-secret" {
			return captcha.ErrRejected
		}
		if in.Token != "valid-"+in.Action {
			return captcha.ErrRejected
		}
		return nil
	}), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store
}
func captchaDraftInput(version uint64) CaptchaDraftInput {
	value := "fixture-secret"
	return CaptchaDraftInput{ExpectedVersion: version, Provider: "turnstile", SiteKey: "production-site-key", Hostnames: []string{"images.example.com"}, Secret: &value}
}
func saveCaptchaDraft(t *testing.T, s *CaptchaService, version uint64) CaptchaAdminView {
	t.Helper()
	view, err := s.SaveDraft(t.Context(), captchaAdmin, captchaDraftInput(version))
	if err != nil {
		t.Fatal(err)
	}
	return view
}
func testCaptchaBoth(t *testing.T, s *CaptchaService, version uint64) CaptchaAdminView {
	t.Helper()
	var view CaptchaAdminView
	for _, action := range []string{"login", "register"} {
		var err error
		view, err = s.TestDraft(t.Context(), captchaAdmin, CaptchaTestInput{ExpectedVersion: version, Action: action, CaptchaToken: "valid-" + action})
		if err != nil {
			t.Fatal(err)
		}
		version = view.Version
	}
	return view
}
func activateCaptcha(t *testing.T, s *CaptchaService) CaptchaAdminView {
	t.Helper()
	view := saveCaptchaDraft(t, s, 0)
	view = testCaptchaBoth(t, s, view.Version)
	view, err := s.Activate(t.Context(), captchaAdmin, CaptchaActivationInput{ExpectedVersion: view.Version, Enabled: true, AcknowledgeLegacyIncompatibility: true, AcknowledgeV1Unprotected: true})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestCaptchaDefaultsAndMissingMasterKey(t *testing.T) {
	s, store := newCaptchaTestService(t)
	s.deps.Secrets = nil
	public, err := s.Public(t.Context())
	if err != nil || public.Enabled || public.SiteKey != "" {
		t.Fatalf("default=%+v err=%v", public, err)
	}
	if err := s.Verify(t.Context(), "", "login"); err != nil {
		t.Fatal("disabled auth broken", err)
	}
	view, err := s.Admin(t.Context(), captchaAdmin)
	if err != nil || view.ConfigurationAvailable {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, captchaDraftInput(0)); !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("missing key err=%v", err)
	}
	if store.raw != nil {
		t.Fatal("missing key wrote config")
	}
}
func TestCaptchaDraftEncryptionNoEchoAndAtomicActivation(t *testing.T) {
	s, store := newCaptchaTestService(t)
	draft := saveCaptchaDraft(t, s, 0)
	if draft.Enabled || draft.Draft == nil || !draft.Draft.SecretConfigured || draft.Version != 1 {
		t.Fatalf("draft=%+v", draft)
	}
	if bytes.Contains(store.raw, []byte("fixture-secret")) {
		t.Fatal("plaintext persisted")
	}
	body, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("sealed")) || bytes.Contains(body, []byte("fixture-secret")) {
		t.Fatal("encrypted or plaintext secret echoed")
	}
	public, err := s.Public(t.Context())
	if err != nil || public.Enabled || public.SiteKey != "" {
		t.Fatal("draft leaked to public")
	}
	if _, err := s.Activate(t.Context(), captchaAdmin, CaptchaActivationInput{ExpectedVersion: 1, Enabled: true, AcknowledgeLegacyIncompatibility: true, AcknowledgeV1Unprotected: true}); !errors.Is(err, ErrCaptchaNotReady) {
		t.Fatalf("untested activation=%v", err)
	}
	tested := testCaptchaBoth(t, s, 1)
	if !tested.Draft.Tested || len(tested.Draft.TestedActions) != 2 {
		t.Fatalf("test not recorded=%+v", tested)
	}
	if _, err := s.Activate(t.Context(), captchaAdmin, CaptchaActivationInput{ExpectedVersion: tested.Version, Enabled: true}); !errors.Is(err, ErrCaptchaNotReady) {
		t.Fatalf("unacknowledged activation=%v", err)
	}
	active, err := s.Activate(t.Context(), captchaAdmin, CaptchaActivationInput{ExpectedVersion: tested.Version, Enabled: true, AcknowledgeLegacyIncompatibility: true, AcknowledgeV1Unprotected: true})
	if err != nil || !active.Enabled {
		t.Fatalf("activate=%+v err=%v", active, err)
	}
	activeBody, err := json.Marshal(active.Active)
	if err != nil {
		t.Fatal(err)
	}
	var activeFields map[string]json.RawMessage
	if err := json.Unmarshal(activeBody, &activeFields); err != nil {
		t.Fatal(err)
	}
	if len(activeFields) != 5 || activeFields["tested"] != nil || activeFields["tested_actions"] != nil {
		t.Fatalf("active configuration must omit draft test state: %s", activeBody)
	}
	public, err = s.Public(t.Context())
	if err != nil || !public.Enabled || public.Provider != "turnstile" || public.SiteKey != "production-site-key" {
		t.Fatalf("public=%+v err=%v", public, err)
	}
	if err := s.Verify(t.Context(), "valid-login", "login"); err != nil {
		t.Fatal(err)
	}
}
func TestCaptchaSecretOmissionClearRotationAndInvalidation(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	view := activateCaptcha(t, s)
	patch := captchaDraftInput(view.Version)
	patch.Secret = nil
	view, err := s.SaveDraft(t.Context(), captchaAdmin, patch)
	if err != nil || !view.Draft.SecretConfigured || view.Draft.Tested {
		t.Fatalf("reuse=%+v %v", view, err)
	}
	patch.ExpectedVersion = view.Version
	patch.ClearSecret = true
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, patch); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("active clear=%v", err)
	}
	view, err = s.Activate(t.Context(), captchaAdmin, CaptchaActivationInput{ExpectedVersion: view.Version, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	patch.ExpectedVersion = view.Version
	view, err = s.SaveDraft(t.Context(), captchaAdmin, patch)
	if err != nil || view.Draft.SecretConfigured || (view.Active != nil && view.Active.SecretConfigured) {
		t.Fatalf("disabled clear=%+v %v", view, err)
	}
	view, err = s.Admin(t.Context(), captchaAdmin)
	if err != nil || view.Draft.SecretConfigured {
		t.Fatalf("cleared secret reappeared after reload: %+v %v", view.Draft, err)
	}
	patch.ExpectedVersion = view.Version
	patch.ClearSecret = false
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, patch); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing secret=%v", err)
	}
	rotated := "rotated-secret"
	patch.Secret = &rotated
	view, err = s.SaveDraft(t.Context(), captchaAdmin, patch)
	if err != nil || !view.Draft.SecretConfigured {
		t.Fatalf("rotation=%+v %v", view, err)
	}
	patch.ExpectedVersion = view.Version
	patch.Provider = "hcaptcha"
	patch.Secret = nil
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, patch); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsupported provider=%v", err)
	}
}

func TestCaptchaActivationRequiresBothActionsAndEachAcknowledgement(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	view := saveCaptchaDraft(t, s, 0)
	view, err := s.TestDraft(t.Context(), captchaAdmin, CaptchaTestInput{
		ExpectedVersion: view.Version, CaptchaToken: "valid-login", Action: "login",
	})
	if err != nil {
		t.Fatal(err)
	}
	activation := CaptchaActivationInput{
		ExpectedVersion: view.Version, Enabled: true,
		AcknowledgeLegacyIncompatibility: true, AcknowledgeV1Unprotected: true,
	}
	if _, err := s.Activate(t.Context(), captchaAdmin, activation); !errors.Is(err, ErrCaptchaNotReady) {
		t.Fatalf("one action allowed activation: %v", err)
	}
	view, err = s.TestDraft(t.Context(), captchaAdmin, CaptchaTestInput{
		ExpectedVersion: view.Version, CaptchaToken: "valid-register", Action: "register",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		legacy, v1 bool
	}{
		{name: "missing legacy acknowledgement", legacy: false, v1: true},
		{name: "missing v1 acknowledgement", legacy: true, v1: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			activation.ExpectedVersion = view.Version
			activation.AcknowledgeLegacyIncompatibility = tc.legacy
			activation.AcknowledgeV1Unprotected = tc.v1
			if _, err := s.Activate(t.Context(), captchaAdmin, activation); !errors.Is(err, ErrCaptchaNotReady) {
				t.Fatalf("one acknowledgement allowed activation: %v", err)
			}
		})
	}
	public, err := s.Public(t.Context())
	if err != nil || public.Enabled || public.Version != view.Version {
		t.Fatalf("rejected activation changed protection: %+v %v", public, err)
	}
}

func TestCaptchaAuthenticatedDisableSurvivesUnavailableMasterKey(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	active := activateCaptcha(t, s)
	s.deps.Secrets = nil
	view, err := s.Activate(t.Context(), captchaAdmin, CaptchaActivationInput{
		ExpectedVersion: active.Version, Enabled: false,
	})
	if err != nil || view.Enabled || view.Active == nil || !view.Active.SecretConfigured {
		t.Fatalf("authenticated recovery failed or discarded credentials: %+v %v", view, err)
	}
	if err := s.Verify(t.Context(), "", "login"); err != nil {
		t.Fatalf("explicitly disabled protection did not take effect: %v", err)
	}
}

func TestCaptchaProductionRejectsKnownTestKeysAndBadHosts(t *testing.T) {
	for _, key := range []string{"1x00000000000000000000AA", "2x00000000000000000000AB", "1x00000000000000000000BB", "2x00000000000000000000BB", "3x00000000000000000000FF", "1x0000000000000000000000000000000AA", "2x0000000000000000000000000000000AA", "3x0000000000000000000000000000000AA"} {
		for _, field := range []string{"site", "secret"} {
			t.Run(field+key, func(t *testing.T) {
				s, _ := newCaptchaTestService(t)
				in := captchaDraftInput(0)
				if field == "site" {
					in.SiteKey = key
				} else {
					in.Secret = &key
				}
				if _, err := s.SaveDraft(t.Context(), captchaAdmin, in); !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("test key accepted: %v", err)
				}
			})
		}
	}
	for _, host := range []string{"https://images.example.com", "*.example.com", "example.com:80", "evil/example.com", "", "127.0.0.1", "localhost", "EXAMPLE.COM", "example.com.", "-bad.example"} {
		t.Run(host, func(t *testing.T) {
			s, _ := newCaptchaTestService(t)
			in := captchaDraftInput(0)
			in.Hostnames = []string{host}
			if _, err := s.SaveDraft(t.Context(), captchaAdmin, in); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("bad host accepted: %v", err)
			}
		})
	}
}
func TestCaptchaAllPrivateOperationsEnforceRole(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	for _, user := range []UserView{{}, {ID: 2, Role: "user", Status: "enabled"}, {ID: 1, Role: "admin", Status: "disabled"}} {
		if _, err := s.Admin(t.Context(), user); !errors.Is(err, ErrForbidden) {
			t.Fatalf("read=%v", err)
		}
		if _, err := s.SaveDraft(t.Context(), user, captchaDraftInput(0)); !errors.Is(err, ErrForbidden) {
			t.Fatalf("save=%v", err)
		}
		if _, err := s.TestDraft(t.Context(), user, CaptchaTestInput{}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("test=%v", err)
		}
		if _, err := s.Activate(t.Context(), user, CaptchaActivationInput{}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("activate=%v", err)
		}
	}
}
func TestCaptchaEnabledFailsClosedAndErrorsAreSanitized(t *testing.T) {
	s, store := newCaptchaTestService(t)
	activateCaptcha(t, s)
	for _, token := range []string{"", strings.Repeat("x", 2049), "expired", "replayed", "valid-register"} {
		if err := s.Verify(t.Context(), token, "login"); !errors.Is(err, ErrCaptchaFailed) {
			t.Fatalf("token accepted error=%v", err)
		}
	}
	s.deps.Verifier = captchaVerifyFunc(func(context.Context, captcha.Request) error { return errors.New("private provider error") })
	if err := s.Verify(t.Context(), "valid-login", "login"); !errors.Is(err, ErrCaptchaUnavailable) || strings.Contains(err.Error(), "private") {
		t.Fatalf("provider err=%v", err)
	}
	s.deps.Secrets = nil
	if err := s.Verify(t.Context(), "valid-login", "login"); !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("missing key enabled=%v", err)
	}
	store.raw = json.RawMessage(`{"enabled":true,"version":9,"active":null}`)
	if err := s.Verify(t.Context(), "valid-login", "login"); !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("corrupt active=%v", err)
	}
	store.raw = json.RawMessage(`invalid`)
	if err := s.Verify(t.Context(), "valid-login", "login"); !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("corrupt row=%v", err)
	}
}

func TestCaptchaMissingOrNullStoredEnabledFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{name: "missing", raw: `{"version":1,"active":null,"draft":null}`},
		{name: "null", raw: `{"version":1,"enabled":null,"active":null,"draft":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, store := newCaptchaTestService(t)
			store.raw = json.RawMessage(tc.raw)
			if err := s.Verify(t.Context(), "", "login"); !errors.Is(err, ErrCaptchaUnavailable) {
				t.Fatalf("malformed persisted policy permitted password-only login: %v", err)
			}
			if _, err := s.Public(t.Context()); !errors.Is(err, ErrCaptchaUnavailable) {
				t.Fatalf("malformed persisted policy published disabled state: %v", err)
			}
		})
	}
}

func TestCaptchaVersionConflictAndConcurrentTesting(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	view := saveCaptchaDraft(t, s, 0)
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, captchaDraftInput(0)); !errors.Is(err, ErrCaptchaConflict) {
		t.Fatalf("stale write=%v", err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	s.deps.Verifier = captchaVerifyFunc(func(context.Context, captcha.Request) error { close(started); <-release; return nil })
	done := make(chan error, 1)
	go func() {
		_, err := s.TestDraft(t.Context(), captchaAdmin, CaptchaTestInput{ExpectedVersion: view.Version, CaptchaToken: "valid-login", Action: "login"})
		done <- err
	}()
	<-started
	changed := captchaDraftInput(view.Version)
	changed.SiteKey = "rotated-site-key"
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, changed); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, ErrCaptchaConflict) {
		t.Fatalf("old test attested new draft=%v", err)
	}
}
func TestCaptchaVerificationUsesOneSnapshotDuringRotation(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	active := activateCaptcha(t, s)
	s.deps.Verifier = captchaVerifyFunc(func(_ context.Context, in captcha.Request) error {
		if in.Secret != "fixture-secret" || in.Hostnames[0] != "images.example.com" {
			t.Fatal("mixed configuration version")
		}
		return nil
	})
	next := captchaDraftInput(active.Version)
	next.SiteKey = "second-site"
	secret := "rotated-secret"
	next.Secret = &secret
	next.Hostnames = []string{"second.example.com"}
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, next); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(t.Context(), "valid-login", "login"); err != nil {
		t.Fatal(err)
	}
}
func TestCaptchaAttestationsExpireAndEncryptFailuresDoNotWrite(t *testing.T) {
	s, store := newCaptchaTestService(t)
	now := time.Now()
	s.deps.Now = func() time.Time { return now }
	view := saveCaptchaDraft(t, s, 0)
	view = testCaptchaBoth(t, s, view.Version)
	now = now.Add(16 * time.Minute)
	if _, err := s.Activate(t.Context(), captchaAdmin, CaptchaActivationInput{ExpectedVersion: view.Version, Enabled: true, AcknowledgeLegacyIncompatibility: true, AcknowledgeV1Unprotected: true}); !errors.Is(err, ErrCaptchaNotReady) {
		t.Fatalf("expired tests activated=%v", err)
	}
	previous := string(store.raw)
	codec, err := secret.NewCodec(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	s.deps.Secrets = codec
	if _, err := s.SaveDraft(t.Context(), captchaAdmin, captchaDraftInput(view.Version)); !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("encrypt failure=%v", err)
	}
	if string(store.raw) != previous {
		t.Fatal("failed encryption changed snapshot")
	}
}
