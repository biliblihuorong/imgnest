package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// Email code errors are safe to show visitors.
var (
	// ErrEmailCodeInvalid rejects a wrong, expired or used code.
	ErrEmailCodeInvalid = errors.New("email code invalid or expired")
	// ErrEmailCodeCooldown rejects a new code requested too soon after the last one.
	ErrEmailCodeCooldown = errors.New("email code requested too recently")
)

const (
	emailCodeTTL         = 10 * time.Minute
	emailCodeCooldown    = time.Minute
	emailCodeMaxAttempts = 5
)

// EmailCodeRepository stores one pending code per purpose and address.
type EmailCodeRepository interface {
	FindEmailCode(ctx context.Context, purpose, email string) (model.EmailCode, error)
	ReplaceEmailCode(ctx context.Context, code model.EmailCode) (model.EmailCode, error)
	AddEmailCodeAttempt(ctx context.Context, id uint64) error
	DeleteEmailCode(ctx context.Context, id uint64, codeHash string) (bool, error)
	PruneEmailCodes(ctx context.Context, cutoff time.Time) error
}

// Mailer sends the site's email; Ready is false while SMTP is off.
type Mailer interface {
	Ready(context.Context) (bool, error)
	Send(ctx context.Context, to, subject, text string) error
}

// EmailCodeSettings supplies the registration verification switch.
type EmailCodeSettings interface {
	EmailVerificationEnabled(context.Context) (bool, error)
}

type emailCodes struct {
	repo     EmailCodeRepository
	mailer   Mailer
	settings EmailCodeSettings
	now      func() time.Time
}

// UseEmailCodes enables registration email verification and password reset
// by email. Without it both stay off.
func (s *UserService) UseEmailCodes(repo EmailCodeRepository, mailer Mailer, settings EmailCodeSettings, now func() time.Time) {
	s.codes = &emailCodes{repo: repo, mailer: mailer, settings: settings, now: now}
}

// emailFeatures reports which email flows work right now: verification needs
// both the switch and working SMTP, reset needs working SMTP.
func (s *UserService) emailFeatures(ctx context.Context) (verification, reset bool, err error) {
	if s.codes == nil {
		return false, false, nil
	}
	ready, err := s.codes.mailer.Ready(ctx)
	if err != nil || !ready {
		return false, false, err
	}
	enabled, err := s.codes.settings.EmailVerificationEnabled(ctx)
	if err != nil {
		return false, false, fmt.Errorf("read email verification setting: %w", err)
	}
	return enabled, true, nil
}

// EmailCodeRequired reports whether registration currently needs an email
// code; the handler then lets the code stand in for the CAPTCHA.
func (s *UserService) EmailCodeRequired(ctx context.Context) (bool, error) {
	verification, _, err := s.emailFeatures(ctx)
	return verification, err
}

// SendEmailCode mails a fresh code for purpose to email. A reset request for
// an unknown or disabled account succeeds without sending anything, so the
// response never reveals which addresses are registered.
func (s *UserService) SendEmailCode(ctx context.Context, purpose, email string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("send email code: %w", err)
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	verification, reset, err := s.emailFeatures(ctx)
	if err != nil {
		return err
	}
	switch purpose {
	case model.EmailCodeRegister:
		if !verification {
			return ErrMailNotConfigured
		}
		enabled, err := s.settings.RegistrationEnabled(ctx)
		if err != nil {
			return fmt.Errorf("read registration setting: %w", err)
		}
		if !enabled {
			return ErrRegistrationDisabled
		}
		if _, err := s.users.FindUserByEmail(ctx, normalized); err == nil {
			return ErrUserExists
		} else if !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("find account by email: %w", err)
		}
	case model.EmailCodeResetPassword:
		if !reset {
			return ErrMailNotConfigured
		}
		user, err := s.users.FindUserByEmail(ctx, normalized)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find account by email: %w", err)
		}
		if user.Status != model.UserStatusEnabled {
			return nil
		}
	default:
		return ErrInvalidInput
	}
	now := s.codes.now().UTC()
	_ = s.codes.repo.PruneEmailCodes(ctx, now)
	previous, err := s.codes.repo.FindEmailCode(ctx, purpose, normalized)
	if err == nil && now.Sub(previous.CreatedAt) < emailCodeCooldown {
		if purpose == model.EmailCodeResetPassword {
			// Reporting the cooldown would confirm the account exists.
			return nil
		}
		return ErrEmailCodeCooldown
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("find email code: %w", err)
	}
	code, err := randomEmailCode()
	if err != nil {
		return err
	}
	stored, err := s.codes.repo.ReplaceEmailCode(ctx, model.EmailCode{Purpose: purpose, Email: normalized,
		CodeHash: emailCodeHash(purpose, normalized, code), ExpiresAt: now.Add(emailCodeTTL), CreatedAt: now})
	if err != nil {
		return fmt.Errorf("store email code: %w", err)
	}
	subject, text := s.emailCodeMessage(ctx, purpose, code)
	if err := s.codes.mailer.Send(ctx, normalized, subject, text); err != nil {
		// The visitor never saw this code; drop it so a retry is not blocked.
		_, _ = s.codes.repo.DeleteEmailCode(context.WithoutCancel(ctx), stored.ID, stored.CodeHash)
		return err
	}
	return nil
}

// checkEmailCode verifies code without using it up; each wrong guess counts
// against the code, which stops working after a few.
func (s *UserService) checkEmailCode(ctx context.Context, purpose, email, code string) (model.EmailCode, error) {
	stored, err := s.codes.repo.FindEmailCode(ctx, purpose, email)
	if errors.Is(err, ErrNotFound) {
		return model.EmailCode{}, ErrEmailCodeInvalid
	}
	if err != nil {
		return model.EmailCode{}, fmt.Errorf("find email code: %w", err)
	}
	if !s.codes.now().Before(stored.ExpiresAt) || stored.Attempts >= emailCodeMaxAttempts {
		return model.EmailCode{}, ErrEmailCodeInvalid
	}
	want := []byte(stored.CodeHash)
	got := []byte(emailCodeHash(purpose, email, code))
	if len(code) != 6 || subtle.ConstantTimeCompare(want, got) != 1 {
		if err := s.codes.repo.AddEmailCodeAttempt(ctx, stored.ID); err != nil {
			return model.EmailCode{}, fmt.Errorf("count email code attempt: %w", err)
		}
		return model.EmailCode{}, ErrEmailCodeInvalid
	}
	return stored, nil
}

// useEmailCode spends a checked code; false means a concurrent request won.
func (s *UserService) useEmailCode(ctx context.Context, code model.EmailCode) error {
	used, err := s.codes.repo.DeleteEmailCode(ctx, code.ID, code.CodeHash)
	if err != nil {
		return fmt.Errorf("use email code: %w", err)
	}
	if !used {
		return ErrEmailCodeInvalid
	}
	return nil
}

// ResetPasswordWithCode sets a new password for the account that received
// code and signs out every session and API token of that account.
func (s *UserService) ResetPasswordWithCode(ctx context.Context, email, code, next string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	if s.codes == nil {
		return ErrMailNotConfigured
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return ErrEmailCodeInvalid
	}
	if !validPassword(next) {
		return ErrInvalidInput
	}
	stored, err := s.checkEmailCode(ctx, model.EmailCodeResetPassword, normalized, code)
	if err != nil {
		return err
	}
	user, err := s.users.FindUserByEmail(ctx, normalized)
	if errors.Is(err, ErrNotFound) {
		return ErrEmailCodeInvalid
	}
	if err != nil {
		return fmt.Errorf("find password reset user: %w", err)
	}
	if user.Status != model.UserStatusEnabled {
		return ErrEmailCodeInvalid
	}
	if err := s.useEmailCode(ctx, stored); err != nil {
		return err
	}
	return s.replacePassword(ctx, user.ID, user.PasswordHash, next)
}

func (s *UserService) emailCodeMessage(ctx context.Context, purpose, code string) (string, string) {
	name, err := s.settings.SiteName(ctx)
	if err != nil || name == "" {
		name = DefaultSiteName
	}
	action, actionEn := "注册账号", "sign up"
	if purpose == model.EmailCodeResetPassword {
		action, actionEn = "重置密码", "reset your password"
	}
	subject := name + " 验证码 " + code
	text := "你正在 " + name + " " + action + "，验证码是：\n\n    " + code + "\n\n" +
		"验证码 10 分钟内有效，只能使用一次。如果不是你本人操作，请忽略这封邮件。\n\n" +
		"Your " + name + " code to " + actionEn + " is " + code + ". It expires in 10 minutes. " +
		"If you did not request it, you can ignore this email.\n"
	return subject, text
}

func randomEmailCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate email code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func emailCodeHash(purpose, email, code string) string {
	sum := sha256.Sum256([]byte(purpose + "\x00" + email + "\x00" + code))
	return hex.EncodeToString(sum[:])
}
