package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/captcha"
	"github.com/biliblihuorong/imgnest/internal/model"
)

// CAPTCHA errors are safe to return: they contain no provider response or secrets.
var (
	ErrCaptchaFailed      = errors.New("captcha challenge required or rejected")
	ErrCaptchaConflict    = errors.New("captcha configuration changed")
	ErrCaptchaUnavailable = errors.New("captcha temporarily unavailable")
	ErrCaptchaNotReady    = errors.New("captcha activation prerequisites not satisfied")
)

// CaptchaRepository persists a whole JSON snapshot with atomic compare-and-swap.
type CaptchaRepository interface {
	ReadCaptcha(context.Context) (json.RawMessage, error)
	CompareAndSwapCaptcha(context.Context, json.RawMessage, json.RawMessage) (bool, error)
}

// CaptchaVerifier never receives account identifiers or passwords.
type CaptchaVerifier interface {
	Verify(context.Context, captcha.Request) error
}

// CaptchaDependencies allows a missing codec only for installations with no master key.
type CaptchaDependencies struct {
	Repository CaptchaRepository
	Secrets    SecretCodec
	Verifier   CaptchaVerifier
	Now        func() time.Time
}

// CaptchaService uses fresh immutable database snapshots; it keeps no stale enabled cache.
type CaptchaService struct{ deps CaptchaDependencies }

// NewCaptchaService constructs the native-web CAPTCHA policy, disabled by default.
func NewCaptchaService(ctx context.Context, deps CaptchaDependencies) (*CaptchaService, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if deps.Repository == nil || deps.Verifier == nil || deps.Now == nil {
		return nil, ErrInvalidInput
	}
	return &CaptchaService{deps: deps}, nil
}

// CaptchaPublicView deliberately exposes no draft, hosts, secret or ciphertext.
type CaptchaPublicView struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	SiteKey  string `json:"site_key"`
	Version  uint64 `json:"version"`
}

// CaptchaConfigView is the safe administrator view of one immutable configuration.
type CaptchaConfigView struct {
	Provider         string   `json:"provider"`
	SiteKey          string   `json:"site_key"`
	Hostnames        []string `json:"hostnames"`
	SecretConfigured bool     `json:"secret_configured"`
	Version          uint64   `json:"version"`
}

// CaptchaDraftView includes only the candidate's recent activation attestations.
type CaptchaDraftView struct {
	CaptchaConfigView
	Tested        bool     `json:"tested"`
	TestedActions []string `json:"tested_actions"`
}

// CaptchaAdminView reports current and candidate settings without any secret material.
type CaptchaAdminView struct {
	Version                uint64             `json:"version"`
	Enabled                bool               `json:"enabled"`
	Active                 *CaptchaConfigView `json:"active"`
	Draft                  *CaptchaDraftView  `json:"draft"`
	ConfigurationAvailable bool               `json:"configuration_available"`
}

// CaptchaDraftInput edits only a candidate; it never silently changes active protection.
type CaptchaDraftInput struct {
	ExpectedVersion uint64   `json:"expected_version"`
	Provider        string   `json:"provider"`
	SiteKey         string   `json:"site_key"`
	Hostnames       []string `json:"hostnames"`
	Secret          *string  `json:"secret,omitempty"`
	ClearSecret     bool     `json:"clear_secret,omitempty"`
}

// CaptchaTestInput consumes a real candidate token for one native route action.
type CaptchaTestInput struct {
	ExpectedVersion uint64 `json:"expected_version"`
	CaptchaToken    string `json:"captcha_token"`
	Action          string `json:"action"`
}

// CaptchaActivationInput requires both explicit compatibility acknowledgements to enable.
type CaptchaActivationInput struct {
	ExpectedVersion                  uint64 `json:"expected_version"`
	Enabled                          bool   `json:"enabled"`
	AcknowledgeLegacyIncompatibility bool   `json:"acknowledge_legacy_incompatibility"`
	AcknowledgeV1Unprotected         bool   `json:"acknowledge_v1_unprotected"`
}

type captchaConfig struct {
	Provider      string               `json:"provider"`
	SiteKey       string               `json:"site_key"`
	Hostnames     []string             `json:"hostnames"`
	Secret        json.RawMessage      `json:"secret"`
	Version       uint64               `json:"version"`
	TestedActions map[string]time.Time `json:"tested_actions"`
}
type captchaState struct {
	Version uint64         `json:"version"`
	Enabled bool           `json:"enabled"`
	Active  *captchaConfig `json:"active"`
	Draft   *captchaConfig `json:"draft"`
}

// Public reads the active public values only; absent configuration means disabled.
func (s *CaptchaService) Public(ctx context.Context) (CaptchaPublicView, error) {
	state, _, err := s.read(ctx)
	if err != nil {
		return CaptchaPublicView{}, err
	}
	view := CaptchaPublicView{Enabled: state.Enabled, Version: state.Version}
	if state.Enabled {
		view.Provider = state.Active.Provider
		view.SiteKey = state.Active.SiteKey
	}
	return view, nil
}

// Admin enforces the role even when invoked without an HTTP handler.
func (s *CaptchaService) Admin(ctx context.Context, actor UserView) (CaptchaAdminView, error) {
	if !captchaIsAdmin(actor) {
		return CaptchaAdminView{}, ErrForbidden
	}
	state, _, err := s.read(ctx)
	if err != nil {
		return CaptchaAdminView{}, err
	}
	return s.view(state), nil
}

// SaveDraft seals a write-only secret and invalidates all prior candidate tests.
// Omitting secret reuses this provider's draft/active secret; an empty secret is invalid.
// Explicit clear is possible only while disabled and also discards the old active copy.
func (s *CaptchaService) SaveDraft(ctx context.Context, actor UserView, in CaptchaDraftInput) (CaptchaAdminView, error) {
	if !captchaIsAdmin(actor) {
		return CaptchaAdminView{}, ErrForbidden
	}
	if s.deps.Secrets == nil {
		return CaptchaAdminView{}, ErrCaptchaUnavailable
	}
	state, old, err := s.read(ctx)
	if err != nil {
		return CaptchaAdminView{}, err
	}
	if state.Version != in.ExpectedVersion {
		return CaptchaAdminView{}, ErrCaptchaConflict
	}
	if !validCaptchaConfig(in.Provider, in.SiteKey, in.Hostnames) || in.ClearSecret && (in.Secret != nil || state.Enabled) {
		return CaptchaAdminView{}, ErrInvalidInput
	}
	candidate := &captchaConfig{Provider: in.Provider, SiteKey: in.SiteKey,
		Hostnames: slices.Clone(in.Hostnames), Version: state.Version + 1, TestedActions: map[string]time.Time{}}
	if in.ClearSecret {
		state.Active = nil
	} else if in.Secret != nil {
		if !validCaptchaSecret(*in.Secret) {
			return CaptchaAdminView{}, ErrInvalidInput
		}
		plain, err := json.Marshal(*in.Secret)
		if err != nil {
			return CaptchaAdminView{}, ErrInvalidInput
		}
		candidate.Secret, err = s.deps.Secrets.Seal(ctx, "captcha:"+candidate.Provider, plain)
		clear(plain)
		if err != nil || len(candidate.Secret) == 0 {
			return CaptchaAdminView{}, ErrCaptchaUnavailable
		}
	} else {
		prior := state.Draft
		if prior == nil {
			prior = state.Active
		}
		if prior == nil || prior.Provider != candidate.Provider || len(prior.Secret) == 0 {
			return CaptchaAdminView{}, ErrInvalidInput
		}
		// Re-open before reusing: a missing/rotated master key must not save an unusable draft.
		if _, err := s.openSecret(ctx, prior); err != nil {
			return CaptchaAdminView{}, err
		}
		candidate.Secret = slices.Clone(prior.Secret)
	}
	state.Draft = candidate
	return s.write(ctx, old, state)
}

// TestDraft binds each successful provider verification to the exact unchanged draft.
func (s *CaptchaService) TestDraft(ctx context.Context, actor UserView, in CaptchaTestInput) (CaptchaAdminView, error) {
	if !captchaIsAdmin(actor) {
		return CaptchaAdminView{}, ErrForbidden
	}
	state, old, err := s.read(ctx)
	if err != nil {
		return CaptchaAdminView{}, err
	}
	if state.Version != in.ExpectedVersion {
		return CaptchaAdminView{}, ErrCaptchaConflict
	}
	if state.Draft == nil {
		return CaptchaAdminView{}, ErrCaptchaNotReady
	}
	if err := s.verifyConfig(ctx, state.Draft, in.CaptchaToken, in.Action); err != nil {
		return CaptchaAdminView{}, err
	}
	if state.Draft.TestedActions == nil {
		state.Draft.TestedActions = map[string]time.Time{}
	}
	state.Draft.TestedActions[in.Action] = s.deps.Now().UTC()
	return s.write(ctx, old, state)
}

// Activate atomically promotes a recently tested candidate. Disabling retains the
// administrator session and secrets; no frontend selector or anonymous bypass exists.
func (s *CaptchaService) Activate(ctx context.Context, actor UserView, in CaptchaActivationInput) (CaptchaAdminView, error) {
	if !captchaIsAdmin(actor) {
		return CaptchaAdminView{}, ErrForbidden
	}
	state, old, err := s.read(ctx)
	if err != nil {
		return CaptchaAdminView{}, err
	}
	if state.Version != in.ExpectedVersion {
		return CaptchaAdminView{}, ErrCaptchaConflict
	}
	if in.Enabled {
		if s.deps.Secrets == nil {
			return CaptchaAdminView{}, ErrCaptchaUnavailable
		}
		if !in.AcknowledgeLegacyIncompatibility || !in.AcknowledgeV1Unprotected || state.Draft == nil ||
			len(s.testedActions(state.Draft)) != 2 {
			return CaptchaAdminView{}, ErrCaptchaNotReady
		}
		if _, err := s.openSecret(ctx, state.Draft); err != nil {
			return CaptchaAdminView{}, err
		}
		state.Active = state.Draft
	}
	state.Enabled = in.Enabled
	return s.write(ctx, old, state)
}

// Verify is used only by native web login/register. Existing /api/v1 token issuance
// and token-authenticated uploads intentionally retain their original contracts.
func (s *CaptchaService) Verify(ctx context.Context, token, action string) error {
	state, _, err := s.read(ctx)
	if err != nil {
		return err
	}
	if !state.Enabled {
		return nil
	}
	return s.verifyConfig(ctx, state.Active, token, action)
}

func (s *CaptchaService) verifyConfig(ctx context.Context, cfg *captchaConfig, token, action string) error {
	if token == "" || len(token) > 2048 || (action != "login" && action != "register") {
		return ErrCaptchaFailed
	}
	value, err := s.openSecret(ctx, cfg)
	if err != nil {
		return err
	}
	err = s.deps.Verifier.Verify(ctx, captcha.Request{Provider: cfg.Provider, Secret: value,
		Token: token, Hostnames: slices.Clone(cfg.Hostnames), Action: action})
	if errors.Is(err, captcha.ErrRejected) {
		return ErrCaptchaFailed
	}
	if err != nil {
		return ErrCaptchaUnavailable
	}
	return nil
}
func (s *CaptchaService) openSecret(ctx context.Context, cfg *captchaConfig) (string, error) {
	if cfg == nil || s.deps.Secrets == nil || len(cfg.Secret) == 0 {
		return "", ErrCaptchaUnavailable
	}
	plain, err := s.deps.Secrets.Open(ctx, "captcha:"+cfg.Provider, cfg.Secret)
	if err != nil {
		return "", ErrCaptchaUnavailable
	}
	defer clear(plain)
	var value string
	if json.Unmarshal(plain, &value) != nil || !validCaptchaSecret(value) {
		return "", ErrCaptchaUnavailable
	}
	return value, nil
}
func (s *CaptchaService) read(ctx context.Context) (captchaState, json.RawMessage, error) {
	raw, err := s.deps.Repository.ReadCaptcha(ctx)
	if err != nil {
		return captchaState{}, nil, ErrCaptchaUnavailable
	}
	if raw == nil {
		return captchaState{}, nil, nil
	}
	var state captchaState
	// Require an explicit stored boolean; missing/null must not become a disabled policy.
	stored := struct {
		*captchaState
		Enabled *bool `json:"enabled"`
	}{captchaState: &state}
	if len(raw) > 64<<10 || json.Unmarshal(raw, &stored) != nil || stored.Enabled == nil || state.Version == 0 || state.Version >= 1<<53 {
		return captchaState{}, nil, ErrCaptchaUnavailable
	}
	state.Enabled = *stored.Enabled
	for _, cfg := range []*captchaConfig{state.Active, state.Draft} {
		if cfg != nil && (!validCaptchaConfig(cfg.Provider, cfg.SiteKey, cfg.Hostnames) || cfg.Version == 0 || cfg.Version > state.Version) {
			return captchaState{}, nil, ErrCaptchaUnavailable
		}
		if cfg != nil && bytes.Equal(bytes.TrimSpace(cfg.Secret), []byte("null")) {
			// RawMessage preserves JSON null as bytes; a cleared secret stays absent after reload.
			cfg.Secret = nil
		}
	}
	if state.Enabled && (state.Active == nil || len(state.Active.Secret) == 0) {
		return captchaState{}, nil, ErrCaptchaUnavailable
	}
	return state, raw, nil
}
func (s *CaptchaService) write(ctx context.Context, old json.RawMessage, state captchaState) (CaptchaAdminView, error) {
	if state.Version+1 >= 1<<53 {
		return CaptchaAdminView{}, ErrCaptchaUnavailable
	}
	state.Version++
	next, err := json.Marshal(state)
	if err != nil {
		return CaptchaAdminView{}, ErrCaptchaUnavailable
	}
	written, err := s.deps.Repository.CompareAndSwapCaptcha(ctx, old, next)
	if err != nil {
		return CaptchaAdminView{}, ErrCaptchaUnavailable
	}
	if !written {
		return CaptchaAdminView{}, ErrCaptchaConflict
	}
	return s.view(state), nil
}
func (s *CaptchaService) view(state captchaState) CaptchaAdminView {
	return CaptchaAdminView{Version: state.Version, Enabled: state.Enabled, Active: s.configView(state.Active),
		Draft: s.draftView(state.Draft), ConfigurationAvailable: s.deps.Secrets != nil}
}
func (s *CaptchaService) configView(cfg *captchaConfig) *CaptchaConfigView {
	if cfg == nil {
		return nil
	}
	return &CaptchaConfigView{Provider: cfg.Provider, SiteKey: cfg.SiteKey, Hostnames: slices.Clone(cfg.Hostnames),
		SecretConfigured: len(cfg.Secret) > 0, Version: cfg.Version}
}
func (s *CaptchaService) draftView(cfg *captchaConfig) *CaptchaDraftView {
	if cfg == nil {
		return nil
	}
	actions := s.testedActions(cfg)
	return &CaptchaDraftView{CaptchaConfigView: *s.configView(cfg), Tested: len(actions) == 2, TestedActions: actions}
}
func (s *CaptchaService) testedActions(cfg *captchaConfig) []string {
	actions := []string{}
	for _, action := range []string{"login", "register"} {
		tested, ok := cfg.TestedActions[action]
		age := s.deps.Now().Sub(tested)
		if ok && age >= 0 && age <= 15*time.Minute {
			actions = append(actions, action)
		}
	}
	return actions
}
func captchaIsAdmin(actor UserView) bool {
	return actor.ID > 0 && actor.Role == model.UserRoleAdmin && actor.Status == model.UserStatusEnabled
}

var captchaKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var captchaHostLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func validCaptchaConfig(provider, key string, hosts []string) bool {
	if provider != "turnstile" || len(key) == 0 || len(key) > 256 || !captchaKeyPattern.MatchString(key) || captchaTestKey(key) || len(hosts) == 0 || len(hosts) > 10 {
		return false
	}
	seen := map[string]bool{}
	for _, host := range hosts {
		if len(host) > 253 || net.ParseIP(host) != nil || seen[host] {
			return false
		}
		labels := strings.Split(host, ".")
		if len(labels) < 2 {
			return false
		}
		for _, label := range labels {
			if !captchaHostLabel.MatchString(label) {
				return false
			}
		}
		seen[host] = true
	}
	return true
}
func validCaptchaSecret(value string) bool {
	return len(value) > 0 && len(value) <= 2048 && captchaKeyPattern.MatchString(value) && !captchaTestKey(value)
}
func captchaTestKey(value string) bool {
	switch value {
	case "1x00000000000000000000AA", "2x00000000000000000000AB", "1x00000000000000000000BB",
		"2x00000000000000000000BB", "3x00000000000000000000FF", "1x0000000000000000000000000000000AA",
		"2x0000000000000000000000000000000AA", "3x0000000000000000000000000000000AA":
		return true
	}
	return false
}
