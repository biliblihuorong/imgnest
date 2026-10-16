package service

import (
	"errors"
	"testing"
)

func configureInput(version uint64, enabled bool, token string) CaptchaConfigureInput {
	secret := "fixture-secret"
	return CaptchaConfigureInput{ExpectedVersion: version, Enabled: enabled, SiteKey: "production-site-key",
		Hostnames: []string{"images.example.com"}, Secret: &secret, CaptchaToken: token}
}

func TestCaptchaConfigureEnablesInOneStep(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	if _, err := s.Configure(t.Context(), UserView{ID: 2, Role: "user", Status: "enabled"}, configureInput(0, true, "valid-login")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin err=%v", err)
	}
	// Enabling needs a challenge solved with the new key.
	if _, err := s.Configure(t.Context(), captchaAdmin, configureInput(0, true, "")); !errors.Is(err, ErrCaptchaFailed) {
		t.Fatalf("missing token err=%v", err)
	}
	if _, err := s.Configure(t.Context(), captchaAdmin, configureInput(0, true, "valid-register")); !errors.Is(err, ErrCaptchaFailed) {
		t.Fatalf("wrong-action token err=%v", err)
	}
	view, err := s.Configure(t.Context(), captchaAdmin, configureInput(0, true, "valid-login"))
	if err != nil || !view.Enabled || view.Active == nil || view.Draft != nil || !view.Active.SecretConfigured {
		t.Fatalf("enable view=%+v err=%v", view, err)
	}
	public, err := s.Public(t.Context())
	if err != nil || !public.Enabled || public.SiteKey != "production-site-key" {
		t.Fatalf("public=%+v err=%v", public, err)
	}
	if err := s.Verify(t.Context(), "valid-reset", "reset"); err != nil {
		t.Fatalf("reset action rejected: %v", err)
	}
	// A stale version never overwrites a newer configuration.
	if _, err := s.Configure(t.Context(), captchaAdmin, configureInput(0, false, "")); !errors.Is(err, ErrCaptchaConflict) {
		t.Fatalf("stale err=%v", err)
	}
	// Disabling with no settings keeps the stored configuration.
	disabled, err := s.Configure(t.Context(), captchaAdmin, CaptchaConfigureInput{ExpectedVersion: view.Version})
	if err != nil || disabled.Enabled || disabled.Active == nil {
		t.Fatalf("disable view=%+v err=%v", disabled, err)
	}
	if err := s.Verify(t.Context(), "", "login"); err != nil {
		t.Fatalf("disabled protection still enforced: %v", err)
	}
	// Re-enabling reuses the saved secret when none is typed.
	again := configureInput(disabled.Version, true, "valid-login")
	again.Secret = nil
	view, err = s.Configure(t.Context(), captchaAdmin, again)
	if err != nil || !view.Enabled {
		t.Fatalf("re-enable view=%+v err=%v", view, err)
	}
}

func TestCaptchaConfigureSavesWhileDisabled(t *testing.T) {
	s, _ := newCaptchaTestService(t)
	view, err := s.Configure(t.Context(), captchaAdmin, configureInput(0, false, ""))
	if err != nil || view.Enabled || view.Draft == nil || view.Active != nil {
		t.Fatalf("save view=%+v err=%v", view, err)
	}
	noSecret := configureInput(0, false, "")
	noSecret.Secret = nil
	fresh, _ := newCaptchaTestService(t)
	if _, err := fresh.Configure(t.Context(), captchaAdmin, noSecret); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("no secret anywhere err=%v", err)
	}
	bad := configureInput(view.Version, false, "")
	bad.Hostnames = []string{"https://images.example.com"}
	if _, err := s.Configure(t.Context(), captchaAdmin, bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad hostname err=%v", err)
	}
	s.deps.Secrets = nil
	if _, err := s.Configure(t.Context(), captchaAdmin, configureInput(view.Version, false, "")); !errors.Is(err, ErrCaptchaUnavailable) {
		t.Fatalf("missing master key err=%v", err)
	}
}
