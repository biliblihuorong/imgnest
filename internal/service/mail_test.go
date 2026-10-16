package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/mail"
	"github.com/biliblihuorong/imgnest/internal/secret"
)

type mailMemory struct{ raw json.RawMessage }

func (m *mailMemory) ReadMailConfig(context.Context) (json.RawMessage, error) { return m.raw, nil }
func (m *mailMemory) WriteMailConfig(_ context.Context, raw json.RawMessage) error {
	m.raw = append(json.RawMessage(nil), raw...)
	return nil
}

type transportFake struct {
	err    error
	server mail.Server
	msg    mail.Message
}

func (f *transportFake) Send(_ context.Context, server mail.Server, msg mail.Message) error {
	f.server, f.msg = server, msg
	return f.err
}

type siteNameStub struct{}

func (siteNameStub) SiteName(context.Context) (string, error) { return "Nest", nil }

var mailAdmin = UserView{ID: 1, Role: "admin", Status: "enabled", Email: "root@example.com"}

func newMailFixture(t *testing.T, withKey bool) (*MailService, *mailMemory, *transportFake) {
	t.Helper()
	var codec SecretCodec
	if withKey {
		c, err := secret.NewCodec(t.Context(), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)))
		if err != nil {
			t.Fatal(err)
		}
		codec = c
	}
	store, transport := &mailMemory{}, &transportFake{}
	svc, err := NewMailService(t.Context(), store, codec, transport, siteNameStub{})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, transport
}

func smtpInput(password *string) MailConfigInput {
	return MailConfigInput{Enabled: true, Host: "smtp.example.com", Port: 465, Encryption: mail.EncryptionTLS,
		Username: "noreply@example.com", Password: password, FromAddress: "noreply@example.com", FromName: "Nest"}
}

func TestMailConfigKeepsPasswordWriteOnly(t *testing.T) {
	svc, store, transport := newMailFixture(t, true)
	if ready, _ := svc.Ready(t.Context()); ready {
		t.Fatal("unconfigured mail reported ready")
	}
	if _, err := svc.SaveConfig(t.Context(), UserView{ID: 2, Role: "user", Status: "enabled"}, smtpInput(nil)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin: %v", err)
	}
	if _, err := svc.SaveConfig(t.Context(), mailAdmin, smtpInput(nil)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("account without password: %v", err)
	}
	password := "app-password"
	view, err := svc.SaveConfig(t.Context(), mailAdmin, smtpInput(&password))
	if err != nil || !view.PasswordConfigured || !view.ConfigurationAvailable {
		t.Fatalf("save view=%+v err=%v", view, err)
	}
	if strings.Contains(string(store.raw), password) {
		t.Fatal("password stored in plain text")
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), password) {
		t.Fatal("password returned to the client")
	}
	// Saving without a password keeps the stored one.
	view, err = svc.SaveConfig(t.Context(), mailAdmin, smtpInput(nil))
	if err != nil || !view.PasswordConfigured {
		t.Fatalf("resave view=%+v err=%v", view, err)
	}
	if ready, _ := svc.Ready(t.Context()); !ready {
		t.Fatal("configured mail not ready")
	}
	if err := svc.Send(t.Context(), "user@example.com", "subject", "text"); err != nil {
		t.Fatal(err)
	}
	if transport.server.Password != password || transport.msg.FromName != "Nest" {
		t.Fatalf("transport got %+v %+v", transport.server, transport.msg)
	}
}

func TestMailConfigValidation(t *testing.T) {
	svc, _, _ := newMailFixture(t, false)
	password := "app-password"
	if _, err := svc.SaveConfig(t.Context(), mailAdmin, smtpInput(&password)); !errors.Is(err, ErrMailSecretsUnavailable) {
		t.Fatalf("password without master key: %v", err)
	}
	relay := MailConfigInput{Enabled: true, Host: "10.0.0.5", Port: 25, Encryption: mail.EncryptionNone, FromAddress: "a@example.com"}
	if _, err := svc.SaveConfig(t.Context(), mailAdmin, relay); err != nil {
		t.Fatalf("open relay without key: %v", err)
	}
	for _, mutate := range []func(*MailConfigInput){
		func(in *MailConfigInput) { in.Host = "smtp://example.com" },
		func(in *MailConfigInput) { in.Port = 0 },
		func(in *MailConfigInput) { in.Encryption = "ssl" },
		func(in *MailConfigInput) { in.FromAddress = "Name <a@example.com>" },
		func(in *MailConfigInput) { in.Username = "user" },
		func(in *MailConfigInput) { in.FromName = "a\nBcc: x" },
	} {
		in := relay
		mutate(&in)
		if _, err := svc.SaveConfig(t.Context(), mailAdmin, in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("SaveConfig(%+v) = %v", in, err)
		}
	}
	if _, err := svc.SaveConfig(t.Context(), mailAdmin, MailConfigInput{Port: 465, Encryption: mail.EncryptionTLS}); err != nil {
		t.Fatalf("empty disabled config: %v", err)
	}
}

func TestMailTestReportsTheFailedStep(t *testing.T) {
	svc, _, transport := newMailFixture(t, true)
	if err := svc.Test(t.Context(), mailAdmin, ""); !errors.Is(err, ErrMailNotConfigured) {
		t.Fatalf("unconfigured: %v", err)
	}
	password := "app-password"
	if _, err := svc.SaveConfig(t.Context(), mailAdmin, smtpInput(&password)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Test(t.Context(), mailAdmin, ""); err != nil || transport.msg.To != "root@example.com" {
		t.Fatalf("test to self: %v, to %q", err, transport.msg.To)
	}
	transport.err = &mail.Error{Kind: mail.ErrAuth, Code: 535}
	var delivery *MailDeliveryError
	if err := svc.Test(t.Context(), mailAdmin, "other@example.com"); !errors.As(err, &delivery) || delivery.Reason != "auth" || delivery.SMTPCode != 535 {
		t.Fatalf("auth failure: %v", err)
	}
	// Visitors only learn that email is unavailable.
	if err := svc.Send(t.Context(), "user@example.com", "s", "t"); !errors.Is(err, ErrMailUnavailable) || errors.As(err, &delivery) {
		t.Fatalf("visitor error: %v", err)
	}
}
