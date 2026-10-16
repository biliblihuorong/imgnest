package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRelay speaks just enough SMTP to accept one message per connection.
type fakeRelay struct {
	t        *testing.T
	listener net.Listener
	tls      *tls.Config
	implicit bool
	password string
	// mechanisms is the AUTH line advertised; empty means "PLAIN".
	mechanisms string
	// ownDomain makes the relay refuse a HELO that claims its domain, as
	// Postfix's common "Don't use my own domain" rule does.
	ownDomain string
	mu        sync.Mutex
	data      string
	authed    bool
}

func testCertificate(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, pool
}

func startRelay(t *testing.T, implicit bool, password string) (*fakeRelay, *x509.CertPool) {
	t.Helper()
	config, pool := testCertificate(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relay := &fakeRelay{t: t, listener: listener, tls: config, implicit: implicit, password: password}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go relay.serve(conn)
		}
	}()
	return relay, pool
}

func (r *fakeRelay) port() int { return r.listener.Addr().(*net.TCPAddr).Port }

func (r *fakeRelay) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	secure := false
	if r.implicit {
		conn = tls.Server(conn, r.tls)
		secure = true
	}
	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 fake ESMTP")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			if r.ownDomain != "" && strings.EqualFold(strings.TrimSpace(cmd[4:]), r.ownDomain) {
				write("550 5.7.1 Helo command rejected: Don't use my own domain")
				continue
			}
			write("250-fake")
			if !secure {
				write("250-STARTTLS")
			}
			mechanisms := r.mechanisms
			if mechanisms == "" {
				mechanisms = "PLAIN"
			}
			write("250 AUTH " + mechanisms)
		case upper == "STARTTLS":
			write("220 go ahead")
			conn = tls.Server(conn, r.tls)
			reader = bufio.NewReader(conn)
			secure = true
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(cmd[len("AUTH PLAIN"):]))
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 && parts[2] == r.password {
				r.mu.Lock()
				r.authed = true
				r.mu.Unlock()
				write("235 ok")
			} else {
				write("535 bad credentials")
			}
		case upper == "AUTH LOGIN":
			write("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
			if _, err := reader.ReadString('\n'); err != nil {
				return
			}
			write("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
			answer, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(answer))
			if string(raw) == r.password {
				r.mu.Lock()
				r.authed = true
				r.mu.Unlock()
				write("235 ok")
			} else {
				write("535 bad credentials")
			}
		case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
			if strings.Contains(cmd, "blocked@") {
				write("550 no")
			} else {
				write("250 ok")
			}
		case upper == "DATA":
			write("354 send")
			var data strings.Builder
			for {
				part, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if part == ".\r\n" {
					break
				}
				data.WriteString(part)
			}
			r.mu.Lock()
			r.data = data.String()
			r.mu.Unlock()
			write("250 queued")
		case upper == "QUIT":
			write("221 bye")
			return
		default:
			write("250 ok")
		}
	}
}

func testMessage() Message {
	return Message{FromAddress: "noreply@example.com", FromName: "图床", To: "user@example.com",
		Subject: "验证码", Text: "你的验证码是 123456"}
}

func TestSendOverImplicitTLSWithAuth(t *testing.T) {
	relay, pool := startRelay(t, true, "s3cret")
	sender := NewSMTP()
	sender.rootCAs = pool
	err := sender.Send(context.Background(), Server{Host: "localhost", Port: relay.port(), Encryption: EncryptionTLS,
		Username: "noreply@example.com", Password: "s3cret"}, testMessage())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	relay.mu.Lock()
	defer relay.mu.Unlock()
	if !relay.authed || !strings.Contains(relay.data, "Content-Transfer-Encoding: base64") {
		t.Fatalf("relay did not receive an authenticated message: %q", relay.data)
	}
	body := relay.data[strings.Index(relay.data, "\r\n\r\n")+4:]
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", ""))
	if err != nil || string(decoded) != "你的验证码是 123456" {
		t.Fatalf("body = %q, %v", decoded, err)
	}
}

func TestSendOverSTARTTLS(t *testing.T) {
	relay, pool := startRelay(t, false, "s3cret")
	sender := NewSMTP()
	sender.rootCAs = pool
	err := sender.Send(context.Background(), Server{Host: "localhost", Port: relay.port(), Encryption: EncryptionSTARTTLS,
		Username: "u", Password: "s3cret"}, testMessage())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestSendClassifiesFailures(t *testing.T) {
	relay, pool := startRelay(t, true, "s3cret")
	sender := NewSMTP()
	sender.rootCAs = pool
	srv := Server{Host: "localhost", Port: relay.port(), Encryption: EncryptionTLS, Username: "u", Password: "wrong"}
	if err := sender.Send(context.Background(), srv, testMessage()); !errors.Is(err, ErrAuth) {
		t.Fatalf("wrong password: %v", err)
	}
	srv.Password = "s3cret"
	msg := testMessage()
	msg.To = "blocked@example.com"
	if err := sender.Send(context.Background(), srv, msg); !errors.Is(err, ErrRejected) {
		t.Fatalf("rejected recipient: %v", err)
	}
	// The system pool does not trust the test certificate.
	if err := NewSMTP().Send(context.Background(), srv, testMessage()); !errors.Is(err, ErrTLS) {
		t.Fatalf("untrusted certificate: %v", err)
	}
	// Plain links never carry a password.
	plain, _ := startRelay(t, false, "s3cret")
	err := sender.Send(context.Background(), Server{Host: "localhost", Port: plain.port(), Encryption: EncryptionNone,
		Username: "u", Password: "s3cret"}, testMessage())
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("password over plain link: %v", err)
	}
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	port, _ := strconv.Atoi(strings.TrimPrefix(closed.Addr().String(), "127.0.0.1:"))
	_ = closed.Close()
	if err := sender.Send(context.Background(), Server{Host: "127.0.0.1", Port: port, Encryption: EncryptionNone}, testMessage()); !errors.Is(err, ErrConnect) {
		t.Fatalf("closed port: %v", err)
	}
}

func TestSendUsesLoginWhenPlainIsNotOffered(t *testing.T) {
	relay, pool := startRelay(t, true, "s3cret")
	relay.mechanisms = "LOGIN XOAUTH2"
	sender := NewSMTP()
	sender.rootCAs = pool
	err := sender.Send(context.Background(), Server{Host: "localhost", Port: relay.port(), Encryption: EncryptionTLS,
		Username: "u", Password: "s3cret"}, testMessage())
	if err != nil {
		t.Fatalf("send with AUTH LOGIN: %v", err)
	}
	relay.mu.Lock()
	defer relay.mu.Unlock()
	if !relay.authed {
		t.Fatal("relay did not authenticate")
	}
}

func TestSendDoesNotClaimTheSenderDomainInHelo(t *testing.T) {
	relay, pool := startRelay(t, true, "s3cret")
	relay.ownDomain = "example.com"
	sender := NewSMTP()
	sender.rootCAs = pool
	err := sender.Send(context.Background(), Server{Host: "localhost", Port: relay.port(), Encryption: EncryptionTLS,
		Username: "u", Password: "s3cret"}, testMessage())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestSendReportsTLSAndStatusCodes(t *testing.T) {
	// Implicit TLS against a plain listener is a TLS failure, not "unreachable".
	plain, _ := startRelay(t, false, "s3cret")
	err := NewSMTP().Send(context.Background(), Server{Host: "localhost", Port: plain.port(), Encryption: EncryptionTLS,
		Username: "u", Password: "s3cret"}, testMessage())
	if !errors.Is(err, ErrTLS) {
		t.Fatalf("TLS to a plain port: %v", err)
	}
	relay, pool := startRelay(t, true, "s3cret")
	sender := NewSMTP()
	sender.rootCAs = pool
	err = sender.Send(context.Background(), Server{Host: "localhost", Port: relay.port(), Encryption: EncryptionTLS,
		Username: "u", Password: "wrong"}, testMessage())
	if !errors.Is(err, ErrAuth) || StatusCode(err) != 535 {
		t.Fatalf("wrong password: %v (code %d)", err, StatusCode(err))
	}
}

func TestBuildRejectsHeaderInjection(t *testing.T) {
	for _, mutate := range []func(*Message){
		func(m *Message) { m.Subject = "hi\r\nBcc: x@example.com" },
		func(m *Message) { m.FromName = "a\nb" },
		func(m *Message) { m.To = "a@example.com\r\nBcc: x@example.com" },
		func(m *Message) { m.To = "Name <a@example.com>" },
		func(m *Message) { m.FromAddress = "not-an-address" },
	} {
		msg := testMessage()
		mutate(&msg)
		if _, err := Build(msg, time.Now()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Build(%+v) = %v, want ErrInvalid", msg, err)
		}
	}
	raw, err := Build(testMessage(), time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"Subject: =?utf-8?q?", "From: =?utf-8?q?", "<noreply@example.com>", "@example.com>\r\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("message lacks %q:\n%s", want, text)
		}
	}
}
