// Package mail sends plain-text notification email through an SMTP relay
// using only the standard library.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	netmail "net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Encryption modes for the connection to the relay.
const (
	EncryptionNone     = "none"
	EncryptionSTARTTLS = "starttls"
	EncryptionTLS      = "tls"
)

// Failure classes. They carry no server response, address or credential, so
// they are safe to show an administrator testing the configuration.
var (
	ErrConnect  = errors.New("mail server unreachable")
	ErrTLS      = errors.New("mail server TLS negotiation failed")
	ErrAuth     = errors.New("mail server rejected the credentials")
	ErrRejected = errors.New("mail server rejected the message")
	ErrInvalid  = errors.New("invalid mail message")
)

// Server is one relay and the account used to sign in to it.
type Server struct {
	Host       string
	Port       int
	Encryption string
	// Username empty means the relay accepts mail without authentication.
	Username string
	Password string
}

// Message is a single plain-text email to one recipient.
type Message struct {
	FromAddress string
	FromName    string
	To          string
	Subject     string
	Text        string
}

// SMTP delivers messages; the zero value is not usable, call NewSMTP.
type SMTP struct {
	timeout time.Duration
	now     func() time.Time
	// rootCAs overrides the system pool in tests.
	rootCAs *x509.CertPool
}

// NewSMTP builds a sender that gives each delivery 20 seconds in total.
func NewSMTP() *SMTP { return &SMTP{timeout: 20 * time.Second, now: time.Now} }

// Send delivers msg through srv. It never retries: a failure after DATA may
// still have delivered the message.
func (s *SMTP) Send(ctx context.Context, srv Server, msg Message) error {
	body, err := Build(msg, s.now())
	if err != nil {
		return err
	}
	if srv.Host == "" || srv.Port < 1 || srv.Port > 65535 {
		return ErrInvalid
	}
	if srv.Encryption == EncryptionNone && srv.Username != "" {
		// A password never crosses an unencrypted link, even to loopback.
		return ErrAuth
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.Port))
	tlsConfig := &tls.Config{ServerName: srv.Host, MinVersion: tls.VersionTLS12, RootCAs: s.rootCAs}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	switch srv.Encryption {
	case EncryptionTLS:
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", addr)
		if err != nil {
			var certErr *tls.CertificateVerificationError
			if errors.As(err, &certErr) {
				return ErrTLS
			}
			return ErrConnect
		}
	case EncryptionSTARTTLS, EncryptionNone:
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return ErrConnect
		}
	default:
		return ErrInvalid
	}
	// The deadline bounds every read and write; cancellation unblocks them.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()
	client, err := smtp.NewClient(conn, srv.Host)
	if err != nil {
		_ = conn.Close()
		return ErrConnect
	}
	defer func() { _ = client.Close() }()
	if err := client.Hello(heloName(msg.FromAddress)); err != nil {
		return ErrConnect
	}
	if srv.Encryption == EncryptionSTARTTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return ErrTLS
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return ErrTLS
		}
	}
	if srv.Username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return ErrAuth
		}
		// PlainAuth refuses to send the password over an unencrypted link.
		if err := client.Auth(smtp.PlainAuth("", srv.Username, srv.Password, srv.Host)); err != nil {
			return ErrAuth
		}
	}
	if err := client.Mail(msg.FromAddress); err != nil {
		return ErrRejected
	}
	if err := client.Rcpt(msg.To); err != nil {
		return ErrRejected
	}
	writer, err := client.Data()
	if err != nil {
		return ErrRejected
	}
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return ErrRejected
	}
	if err := writer.Close(); err != nil {
		return ErrRejected
	}
	_ = client.Quit()
	return nil
}

// Build renders msg as an RFC 5322 message with a base64 UTF-8 body.
func Build(msg Message, now time.Time) ([]byte, error) {
	from, err := ParseAddress(msg.FromAddress)
	if err != nil {
		return nil, ErrInvalid
	}
	to, err := ParseAddress(msg.To)
	if err != nil {
		return nil, ErrInvalid
	}
	if hasLineBreak(msg.FromName) || hasLineBreak(msg.Subject) {
		return nil, ErrInvalid
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("generate message id: %w", err)
	}
	var b bytes.Buffer
	header := func(name, value string) { b.WriteString(name + ": " + value + "\r\n") }
	header("From", (&netmail.Address{Name: msg.FromName, Address: from}).String())
	header("To", (&netmail.Address{Address: to}).String())
	header("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domainOf(from)+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "base64")
	b.WriteString("\r\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(msg.Text))
	for len(encoded) > 76 {
		b.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	b.WriteString(encoded + "\r\n")
	return b.Bytes(), nil
}

// ParseAddress accepts one bare address ("user@example.com") and returns it.
func ParseAddress(raw string) (string, error) {
	if raw == "" || len(raw) > 254 || hasLineBreak(raw) {
		return "", ErrInvalid
	}
	parsed, err := netmail.ParseAddress(raw)
	if err != nil || parsed.Name != "" || parsed.Address != raw || !strings.Contains(raw, "@") {
		return "", ErrInvalid
	}
	return parsed.Address, nil
}

func hasLineBreak(value string) bool { return strings.ContainsAny(value, "\r\n\x00") }

func domainOf(address string) string { return address[strings.LastIndexByte(address, '@')+1:] }

// heloName introduces the client by the sender's domain, which relays accept
// more readily than the default "localhost".
func heloName(from string) string {
	if i := strings.LastIndexByte(from, '@'); i >= 0 && i < len(from)-1 {
		return from[i+1:]
	}
	return "localhost"
}
