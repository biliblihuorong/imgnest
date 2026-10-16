package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/mail"
)

// Mail errors carry no server response, address or credential.
var (
	// ErrMailNotConfigured rejects a feature that needs outgoing email while
	// SMTP is off or incomplete.
	ErrMailNotConfigured = errors.New("outgoing email is not configured")
	// ErrMailUnavailable reports a failed delivery to a visitor.
	ErrMailUnavailable = errors.New("outgoing email temporarily unavailable")
	// ErrMailSecretsUnavailable rejects saving an SMTP password without a master key.
	ErrMailSecretsUnavailable = errors.New("saving the SMTP password requires security.master_key")
)

// MailDeliveryError tells an administrator testing SMTP which step failed.
type MailDeliveryError struct {
	// Reason is one of connect, tls, auth, rejected.
	Reason string
}

func (e *MailDeliveryError) Error() string { return "mail delivery failed: " + e.Reason }

// Unwrap lets callers treat a failed test like any unavailable delivery.
func (e *MailDeliveryError) Unwrap() error { return ErrMailUnavailable }

// MailRepository stores the single SMTP configuration document.
type MailRepository interface {
	ReadMailConfig(context.Context) (json.RawMessage, error)
	WriteMailConfig(context.Context, json.RawMessage) error
}

// MailTransport delivers one message through one relay.
type MailTransport interface {
	Send(context.Context, mail.Server, mail.Message) error
}

// MailService owns the SMTP settings and sends the site's email.
type MailService struct {
	repo      MailRepository
	secrets   SecretCodec
	transport MailTransport
	site      interface {
		SiteName(context.Context) (string, error)
	}
}

// NewMailService builds the mailer; a nil codec allows only relays without a password.
func NewMailService(ctx context.Context, repo MailRepository, secrets SecretCodec, transport MailTransport, site interface {
	SiteName(context.Context) (string, error)
}) (*MailService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create mail service: %w", err)
	}
	if repo == nil || transport == nil || site == nil {
		return nil, fmt.Errorf("create mail service: %w", ErrInvalidInput)
	}
	return &MailService{repo: repo, secrets: secrets, transport: transport, site: site}, nil
}

// MailConfigView is the administrator view; the password is write-only.
type MailConfigView struct {
	Enabled                bool   `json:"enabled"`
	Host                   string `json:"host"`
	Port                   int    `json:"port"`
	Encryption             string `json:"encryption"`
	Username               string `json:"username"`
	PasswordConfigured     bool   `json:"password_configured"`
	FromAddress            string `json:"from_address"`
	FromName               string `json:"from_name"`
	ConfigurationAvailable bool   `json:"configuration_available"`
}

// MailConfigInput replaces the configuration. A nil Password keeps the saved one.
type MailConfigInput struct {
	Enabled       bool    `json:"enabled"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Encryption    string  `json:"encryption"`
	Username      string  `json:"username"`
	Password      *string `json:"password,omitempty"`
	ClearPassword bool    `json:"clear_password,omitempty"`
	FromAddress   string  `json:"from_address"`
	FromName      string  `json:"from_name"`
}

type mailConfig struct {
	Enabled     bool            `json:"enabled"`
	Host        string          `json:"host"`
	Port        int             `json:"port"`
	Encryption  string          `json:"encryption"`
	Username    string          `json:"username"`
	Password    json.RawMessage `json:"password,omitempty"`
	FromAddress string          `json:"from_address"`
	FromName    string          `json:"from_name"`
}

const mailSecretContext = "mail:smtp"

// Config returns the administrator view of the SMTP settings.
func (s *MailService) Config(ctx context.Context, actor UserView) (MailConfigView, error) {
	if !captchaIsAdmin(actor) {
		return MailConfigView{}, ErrForbidden
	}
	cfg, err := s.read(ctx)
	if err != nil {
		return MailConfigView{}, err
	}
	return s.view(cfg), nil
}

// SaveConfig validates and stores in, sealing a new password.
func (s *MailService) SaveConfig(ctx context.Context, actor UserView, in MailConfigInput) (MailConfigView, error) {
	if !captchaIsAdmin(actor) {
		return MailConfigView{}, ErrForbidden
	}
	previous, err := s.read(ctx)
	if err != nil {
		return MailConfigView{}, err
	}
	next := mailConfig{Enabled: in.Enabled, Host: strings.TrimSpace(in.Host), Port: in.Port,
		Encryption: in.Encryption, Username: strings.TrimSpace(in.Username),
		FromAddress: strings.TrimSpace(in.FromAddress), FromName: strings.TrimSpace(in.FromName)}
	if !validMailConfig(next) || (in.ClearPassword && in.Password != nil) {
		return MailConfigView{}, ErrInvalidInput
	}
	switch {
	case in.ClearPassword || next.Username == "":
		next.Password = nil
	case in.Password != nil:
		if *in.Password == "" || len(*in.Password) > 1024 || strings.ContainsAny(*in.Password, "\r\n\x00") {
			return MailConfigView{}, ErrInvalidInput
		}
		if s.secrets == nil {
			return MailConfigView{}, ErrMailSecretsUnavailable
		}
		// The codec seals JSON documents, so the password travels as a JSON string.
		plain, err := json.Marshal(*in.Password)
		if err != nil {
			return MailConfigView{}, ErrInvalidInput
		}
		next.Password, err = s.secrets.Seal(ctx, mailSecretContext, plain)
		clear(plain)
		if err != nil || len(next.Password) == 0 {
			return MailConfigView{}, fmt.Errorf("seal smtp password: %w", ErrMailSecretsUnavailable)
		}
	default:
		next.Password = previous.Password
	}
	if next.Enabled && next.Username != "" && len(next.Password) == 0 {
		// An account without its password cannot sign in to the relay.
		return MailConfigView{}, ErrInvalidInput
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return MailConfigView{}, ErrInvalidInput
	}
	if err := s.repo.WriteMailConfig(ctx, raw); err != nil {
		return MailConfigView{}, fmt.Errorf("save mail settings: %w", err)
	}
	return s.view(next), nil
}

// Test sends a test message to "to", or to the administrator's own address
// when it is empty, through the saved configuration.
func (s *MailService) Test(ctx context.Context, actor UserView, to string) error {
	if !captchaIsAdmin(actor) {
		return ErrForbidden
	}
	if strings.TrimSpace(to) == "" {
		to = actor.Email
	}
	to, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return ErrInvalidInput
	}
	name := s.siteName(ctx)
	err = s.deliver(ctx, to, name+" 测试邮件 / test email",
		"这是一封来自 "+name+" 的测试邮件，收到说明 SMTP 配置可用。\n\n"+
			"This is a test email from "+name+". Receiving it means the SMTP settings work.\n")
	var delivery *MailDeliveryError
	if errors.As(err, &delivery) {
		return delivery
	}
	return err
}

// Ready reports whether email can be sent right now.
func (s *MailService) Ready(ctx context.Context) (bool, error) {
	cfg, err := s.read(ctx)
	if err != nil {
		return false, err
	}
	if !cfg.Enabled || !validMailConfig(cfg) {
		return false, nil
	}
	if cfg.Username != "" {
		if _, err := s.password(ctx, cfg); err != nil {
			return false, nil
		}
	}
	return true, nil
}

// Send delivers one plain-text message. Failures are reported as
// ErrMailNotConfigured or ErrMailUnavailable without delivery details.
func (s *MailService) Send(ctx context.Context, to, subject, text string) error {
	err := s.deliver(ctx, to, subject, text)
	var delivery *MailDeliveryError
	if errors.As(err, &delivery) {
		return ErrMailUnavailable
	}
	return err
}

// siteName returns the configured site name for message subjects.
func (s *MailService) siteName(ctx context.Context) string {
	name, err := s.site.SiteName(ctx)
	if err != nil || name == "" {
		return DefaultSiteName
	}
	return name
}

func (s *MailService) deliver(ctx context.Context, to, subject, text string) error {
	cfg, err := s.read(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled || !validMailConfig(cfg) {
		return ErrMailNotConfigured
	}
	server := mail.Server{Host: cfg.Host, Port: cfg.Port, Encryption: cfg.Encryption, Username: cfg.Username}
	if cfg.Username != "" {
		server.Password, err = s.password(ctx, cfg)
		if err != nil {
			return ErrMailNotConfigured
		}
	}
	err = s.transport.Send(ctx, server, mail.Message{FromAddress: cfg.FromAddress, FromName: cfg.FromName,
		To: to, Subject: subject, Text: text})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mail.ErrInvalid):
		return ErrInvalidInput
	case errors.Is(err, mail.ErrTLS):
		return &MailDeliveryError{Reason: "tls"}
	case errors.Is(err, mail.ErrAuth):
		return &MailDeliveryError{Reason: "auth"}
	case errors.Is(err, mail.ErrRejected):
		return &MailDeliveryError{Reason: "rejected"}
	default:
		return &MailDeliveryError{Reason: "connect"}
	}
}

func (s *MailService) password(ctx context.Context, cfg mailConfig) (string, error) {
	if s.secrets == nil || len(cfg.Password) == 0 {
		return "", ErrMailNotConfigured
	}
	plain, err := s.secrets.Open(ctx, mailSecretContext, cfg.Password)
	if err != nil {
		return "", ErrMailNotConfigured
	}
	defer clear(plain)
	var password string
	if json.Unmarshal(plain, &password) != nil || password == "" {
		return "", ErrMailNotConfigured
	}
	return password, nil
}

func (s *MailService) read(ctx context.Context) (mailConfig, error) {
	raw, err := s.repo.ReadMailConfig(ctx)
	if err != nil {
		return mailConfig{}, fmt.Errorf("read mail settings: %w", err)
	}
	cfg := mailConfig{Port: 465, Encryption: mail.EncryptionTLS}
	if raw == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return mailConfig{}, fmt.Errorf("decode mail settings: %w", ErrInvalidInput)
	}
	if string(cfg.Password) == "null" {
		cfg.Password = nil
	}
	return cfg, nil
}

func (s *MailService) view(cfg mailConfig) MailConfigView {
	return MailConfigView{Enabled: cfg.Enabled, Host: cfg.Host, Port: cfg.Port, Encryption: cfg.Encryption,
		Username: cfg.Username, PasswordConfigured: len(cfg.Password) > 0, FromAddress: cfg.FromAddress,
		FromName: cfg.FromName, ConfigurationAvailable: s.secrets != nil}
}

// validMailConfig accepts an empty disabled configuration and otherwise
// requires a relay host and sender address.
func validMailConfig(cfg mailConfig) bool {
	if cfg.Port < 1 || cfg.Port > 65535 {
		return false
	}
	switch cfg.Encryption {
	case mail.EncryptionTLS, mail.EncryptionSTARTTLS:
	case mail.EncryptionNone:
		if cfg.Username != "" {
			return false
		}
	default:
		return false
	}
	if len(cfg.Username) > 256 || containsControlRune(cfg.Username) ||
		utf8.RuneCountInString(cfg.FromName) > 100 || !utf8.ValidString(cfg.FromName) || containsControlRune(cfg.FromName) {
		return false
	}
	if cfg.Host == "" && cfg.FromAddress == "" {
		return !cfg.Enabled
	}
	if !validMailHost(cfg.Host) {
		return false
	}
	_, err := mail.ParseAddress(cfg.FromAddress)
	return err == nil
}

func validMailHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	return host != "" && len(host) <= 253 && utf8.ValidString(host) &&
		!strings.ContainsAny(host, " /:@\\[]") && !containsControlRune(host)
}
