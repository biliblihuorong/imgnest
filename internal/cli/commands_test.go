package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"gorm.io/gorm"
)

func commandConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	dsn := filepath.Join(t.TempDir(), "cli.db")
	contents := "database:\n  driver: sqlite\n  dsn: " + dsn + "\nserver:\n  addr: 127.0.0.1:8080\nimages:\n  thumb_cache: " + filepath.Join(t.TempDir(), "thumbs") + "\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func command(ctx context.Context, t *testing.T, path, password string, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	full := append([]string{"--config", path}, args...)
	err := executeWithInput(ctx, full, strings.NewReader(password), &out, &errOut)
	if strings.Contains(out.String(), password) && password != "" {
		t.Fatal("command echoed password")
	}
	return out.String(), err
}
func TestMigrateCLIIsIdempotent(t *testing.T) {
	path := commandConfig(t)
	for range 2 {
		if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
			t.Fatal(err)
		}
	}
}
func TestServeRefusesPendingMigrations(t *testing.T) {
	path := commandConfig(t)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err := command(ctx, t, path, "", "serve")
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("serve did not reject pending migration: %v", err)
	}
}
func TestInitAdminCLIReadsPasswordFromStdin(t *testing.T) {
	path := commandConfig(t)
	if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := command(t.Context(), t, path, "valid-password-123\n", "init-admin", "--username", "tester", "--email", "tester@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := command(t.Context(), t, path, "next-password-123\n", "init-admin", "--username", "other", "--email", "other@example.com"); err == nil {
		t.Fatal("second administrator bootstrap succeeded")
	}
}
func TestResetPasswordRevokesTokens(t *testing.T) {
	path := commandConfig(t)
	if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	if _, err := command(t.Context(), t, path, "valid-password-123\n", "init-admin", "--username", "tester", "--email", "tester@example.com"); err != nil {
		t.Fatal(err)
	}
	var userID uint64
	err := withDatabase(t.Context(), path, func(db *gorm.DB, _ config.Config) error {
		if err := db.Raw("SELECT id FROM users WHERE email = ?", "tester@example.com").Scan(&userID).Error; err != nil {
			return err
		}
		return db.Exec("INSERT INTO tokens (user_id,name,token_hash,kind,abilities,created_at,updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", userID, "test", strings.Repeat("a", 64), "api", `["*"]`).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := command(t.Context(), t, path, "next-password-123\n", "reset-password", "--email", "tester@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := withDatabase(t.Context(), path, func(db *gorm.DB, _ config.Config) error {
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM tokens WHERE user_id = ?", userID).Scan(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("password reset retained tokens")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestShutdownClosesResources(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }), ReadHeaderTimeout: time.Second}
	finished := make(chan error, 1)
	go func() { finished <- runServer(ctx, server, listener, time.Second) }()
	client := &http.Client{Timeout: time.Second}
	url := "http://" + listener.Addr().String()
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, requestErr := client.Get(url)
		if requestErr == nil {
			if err := response.Body.Close(); err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not listen before deadline")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 50*time.Millisecond)
	if err == nil {
		if closeErr := conn.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal("listener remained open")
	}
	path := commandConfig(t)
	var sqlDB *sql.DB
	if err := withDatabase(t.Context(), path, func(db *gorm.DB, _ config.Config) error { var err error; sqlDB, err = db.DB(); return err }); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.PingContext(t.Context()); err == nil {
		t.Fatal("database was not closed")
	}
}
func TestAdminPasswordInputIsBounded(t *testing.T) {
	path := commandConfig(t)
	if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	err := executeWithInput(t.Context(), []string{"--config", path, "init-admin", "--username", "tester", "--email", "tester@example.com"}, strings.NewReader(strings.Repeat("p", 10000)), io.Discard, io.Discard)
	if err == nil {
		t.Fatal("oversized stdin password accepted")
	}
}

type blockingPasswordReader struct {
	reader  *io.PipeReader
	started chan struct{}
}

func (r *blockingPasswordReader) Read(p []byte) (int, error) {
	select {
	case <-r.started:
	default:
		close(r.started)
	}
	return r.reader.Read(p)
}

func TestAdminCancelledStdinReturns(t *testing.T) {
	path := commandConfig(t)
	if _, err := command(t.Context(), t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	})
	input := &blockingPasswordReader{reader: reader, started: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- executeWithInput(ctx, []string{"--config", path, "init-admin", "--username", "tester", "--email", "tester@example.com"}, input, io.Discard, io.Discard)
	}()
	select {
	case <-input.started:
	case <-time.After(time.Second):
		t.Fatal("command did not begin stdin read")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled stdin=%v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("cancelled command remained blocked on stdin")
	}
}
