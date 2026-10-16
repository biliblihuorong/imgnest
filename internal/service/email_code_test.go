package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type codeMemory struct {
	mu    sync.Mutex
	codes map[string]model.EmailCode
	next  uint64
}

func (m *codeMemory) key(purpose, email string) string { return purpose + "|" + email }
func (m *codeMemory) FindEmailCode(_ context.Context, purpose, email string) (model.EmailCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	code, ok := m.codes[m.key(purpose, email)]
	if !ok {
		return model.EmailCode{}, ErrNotFound
	}
	return code, nil
}
func (m *codeMemory) ReplaceEmailCode(_ context.Context, code model.EmailCode) (model.EmailCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	code.ID = m.next
	m.codes[m.key(code.Purpose, code.Email)] = code
	return code, nil
}
func (m *codeMemory) AddEmailCodeAttempt(_ context.Context, id uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, code := range m.codes {
		if code.ID == id {
			code.Attempts++
			m.codes[key] = code
		}
	}
	return nil
}
func (m *codeMemory) DeleteEmailCode(_ context.Context, id uint64, hash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, code := range m.codes {
		if code.ID == id && code.CodeHash == hash {
			delete(m.codes, key)
			return true, nil
		}
	}
	return false, nil
}
func (m *codeMemory) PruneEmailCodes(context.Context, time.Time) error { return nil }

type mailerFake struct {
	ready bool
	err   error
	sent  []string
}

func (m *mailerFake) Ready(context.Context) (bool, error) { return m.ready, nil }
func (m *mailerFake) Send(_ context.Context, to, subject, text string) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, to+"\n"+subject+"\n"+text)
	return nil
}

var sixDigits = regexp.MustCompile(`\b\d{6}\b`)

func (m *mailerFake) lastCode(t *testing.T) string {
	t.Helper()
	if len(m.sent) == 0 {
		t.Fatal("no email sent")
	}
	code := sixDigits.FindString(m.sent[len(m.sent)-1])
	if code == "" {
		t.Fatalf("email has no code: %s", m.sent[len(m.sent)-1])
	}
	return code
}

type verifySettings struct{ enabled bool }

func (s verifySettings) EmailVerificationEnabled(context.Context) (bool, error) {
	return s.enabled, nil
}

type accountMemory struct {
	UserRepository
	mu     sync.Mutex
	users  map[string]model.User
	hashes map[uint64]string
}

func (r *accountMemory) FindUserByEmail(_ context.Context, email string) (model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[email]
	if !ok {
		return model.User{}, ErrNotFound
	}
	return user, nil
}
func (r *accountMemory) CreateUser(_ context.Context, user model.User) (model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[user.Email]; ok {
		return model.User{}, ErrUserExists
	}
	user.ID = uint64(len(r.users) + 1)
	r.users[user.Email] = user
	return user, nil
}
func (r *accountMemory) UpdatePasswordAndRevokeTokens(_ context.Context, id uint64, expected, next string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for email, user := range r.users {
		if user.ID == id {
			if user.PasswordHash != expected {
				return ErrInvalidCredentials
			}
			user.PasswordHash = next
			r.users[email] = user
			return nil
		}
	}
	return ErrNotFound
}

type registrationSettings struct {
	SettingsRepository
	open bool
}

func (s registrationSettings) RegistrationEnabled(context.Context) (bool, error) { return s.open, nil }
func (s registrationSettings) DefaultGroupID(context.Context) (uint64, error)    { return 1, nil }
func (s registrationSettings) SiteName(context.Context) (string, error)          { return "Nest", nil }
func (s registrationSettings) GalleryEnabled(context.Context) (bool, error)      { return false, nil }
func (s registrationSettings) AvatarConfig(context.Context) (model.AvatarConfig, error) {
	return model.AvatarConfig{Provider: model.DefaultAvatarProvider}, nil
}

type codeFixture struct {
	users  *UserService
	codes  *codeMemory
	mailer *mailerFake
	repo   *accountMemory
	now    time.Time
}

func newCodeFixture(t *testing.T, verification bool) *codeFixture {
	t.Helper()
	f := &codeFixture{codes: &codeMemory{codes: map[string]model.EmailCode{}}, mailer: &mailerFake{ready: true},
		repo: &accountMemory{users: map[string]model.User{}}, now: time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC)}
	users, err := NewUserService(t.Context(), f.repo, registrationSettings{open: true})
	if err != nil {
		t.Fatal(err)
	}
	users.UseEmailCodes(f.codes, f.mailer, verifySettings{enabled: verification}, func() time.Time { return f.now })
	f.users = users
	return f
}

func TestRegisterRequiresMailedCode(t *testing.T) {
	f := newCodeFixture(t, true)
	site, err := f.users.Site(t.Context())
	if err != nil || !site.EmailVerification || !site.PasswordReset {
		t.Fatalf("site=%+v err=%v", site, err)
	}
	input := RegisterInput{Username: "alice", Email: "Alice@Example.com", Password: "correct-password-123"}
	if _, err := f.users.Register(t.Context(), input); !errors.Is(err, ErrEmailCodeInvalid) {
		t.Fatalf("register without code: %v", err)
	}
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "Alice@Example.com"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.mailer.sent[0], "alice@example.com\n") {
		t.Fatalf("code sent to %q", f.mailer.sent[0])
	}
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "alice@example.com"); !errors.Is(err, ErrEmailCodeCooldown) {
		t.Fatalf("second request within a minute: %v", err)
	}
	code := f.mailer.lastCode(t)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	input.EmailCode = wrong
	if _, err := f.users.Register(t.Context(), input); !errors.Is(err, ErrEmailCodeInvalid) {
		t.Fatalf("wrong code: %v", err)
	}
	input.EmailCode = code
	if _, err := f.users.Register(t.Context(), input); err != nil {
		t.Fatalf("register with code: %v", err)
	}
	if _, err := f.codes.FindEmailCode(t.Context(), model.EmailCodeRegister, "alice@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatal("code survived registration")
	}
	f.now = f.now.Add(2 * time.Minute)
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "alice@example.com"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("code for a registered address: %v", err)
	}
}

func TestEmailCodeExpiresAndLocksAfterGuesses(t *testing.T) {
	f := newCodeFixture(t, true)
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	code := f.mailer.lastCode(t)
	input := RegisterInput{Username: "bob", Email: "bob@example.com", Password: "correct-password-123", EmailCode: "12345"}
	for range emailCodeMaxAttempts {
		if _, err := f.users.Register(t.Context(), input); !errors.Is(err, ErrEmailCodeInvalid) {
			t.Fatalf("guess: %v", err)
		}
	}
	input.EmailCode = code
	if _, err := f.users.Register(t.Context(), input); !errors.Is(err, ErrEmailCodeInvalid) {
		t.Fatalf("correct code after too many guesses: %v", err)
	}
	f.now = f.now.Add(2 * time.Minute)
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "bob@example.com"); err != nil {
		t.Fatal(err)
	}
	input.EmailCode = f.mailer.lastCode(t)
	f.now = f.now.Add(emailCodeTTL)
	if _, err := f.users.Register(t.Context(), input); !errors.Is(err, ErrEmailCodeInvalid) {
		t.Fatalf("expired code: %v", err)
	}
}

func TestRegisterWithoutVerificationIgnoresCodes(t *testing.T) {
	f := newCodeFixture(t, false)
	if _, err := f.users.Register(t.Context(), RegisterInput{Username: "carol", Email: "carol@example.com", Password: "correct-password-123"}); err != nil {
		t.Fatal(err)
	}
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "dave@example.com"); !errors.Is(err, ErrMailNotConfigured) {
		t.Fatalf("register code while verification is off: %v", err)
	}
	// Turning SMTP off also turns verification off.
	g := newCodeFixture(t, true)
	g.mailer.ready = false
	site, _ := g.users.Site(t.Context())
	if site.EmailVerification || site.PasswordReset {
		t.Fatalf("site=%+v", site)
	}
	if _, err := g.users.Register(t.Context(), RegisterInput{Username: "erin", Email: "erin@example.com", Password: "correct-password-123"}); err != nil {
		t.Fatal(err)
	}
}

func TestResetPasswordWithCode(t *testing.T) {
	f := newCodeFixture(t, false)
	f.repo.users["frank@example.com"] = model.User{ID: 9, Email: "frank@example.com", PasswordHash: "old", Status: model.UserStatusEnabled}
	// Unknown addresses look the same as known ones and get no email.
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeResetPassword, "nobody@example.com"); err != nil || len(f.mailer.sent) != 0 {
		t.Fatalf("unknown address: %v, sent %d", err, len(f.mailer.sent))
	}
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeResetPassword, "frank@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeResetPassword, "frank@example.com"); err != nil || len(f.mailer.sent) != 1 {
		t.Fatalf("cooldown must be silent for resets: %v, sent %d", err, len(f.mailer.sent))
	}
	code := f.mailer.lastCode(t)
	if err := f.users.ResetPasswordWithCode(t.Context(), "frank@example.com", code, "short"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("weak password: %v", err)
	}
	if err := f.users.ResetPasswordWithCode(t.Context(), "frank@example.com", code, "a-brand-new-password"); err != nil {
		t.Fatal(err)
	}
	if f.repo.users["frank@example.com"].PasswordHash == "old" {
		t.Fatal("password unchanged")
	}
	if err := f.users.ResetPasswordWithCode(t.Context(), "frank@example.com", code, "another-new-password"); !errors.Is(err, ErrEmailCodeInvalid) {
		t.Fatalf("code reused: %v", err)
	}
	f.mailer.ready = false
	f.now = f.now.Add(time.Hour)
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeResetPassword, "frank@example.com"); !errors.Is(err, ErrMailNotConfigured) {
		t.Fatalf("reset without SMTP: %v", err)
	}
}

func TestFailedDeliveryDropsTheCode(t *testing.T) {
	f := newCodeFixture(t, true)
	f.mailer.err = ErrMailUnavailable
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "gina@example.com"); !errors.Is(err, ErrMailUnavailable) {
		t.Fatalf("send: %v", err)
	}
	f.mailer.err = nil
	// The failed attempt does not start a cooldown.
	if err := f.users.SendEmailCode(t.Context(), model.EmailCodeRegister, "gina@example.com"); err != nil {
		t.Fatalf("retry: %v", err)
	}
}
