package cli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/repo"
)

type processLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (p *processLog) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buf.Write(data)
}
func (p *processLog) text() string { p.mu.Lock(); defer p.mu.Unlock(); return p.buf.String() }

func TestActualProcessSmoke(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "imgnest")
	// #nosec G204 -- literal compiler and test-owned output path; no shell or user-controlled arguments.
	build := exec.CommandContext(t.Context(), "go", "build", "-mod=readonly", "-o", binary, "../../cmd/imgnest")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s", output)
	}
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "process.db")
			if driver == "postgres" {
				dsn = os.Getenv("IMGNEST_TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("IMGNEST_TEST_POSTGRES_DSN is not set")
				}
				cfg, err := config.Load(t.Context(), "", []string{"IMGNEST_DATABASE_DRIVER=postgres", "IMGNEST_DATABASE_DSN=" + dsn})
				if err != nil {
					t.Fatal(err)
				}
				admin, err := repo.Open(t.Context(), cfg.Database)
				if err != nil {
					t.Fatal(err)
				}
				adminDB, err := admin.DB()
				if err != nil {
					t.Fatal(err)
				}
				var nameBytes [8]byte
				if _, err := rand.Read(nameBytes[:]); err != nil {
					t.Fatal(err)
				}
				schema := "process_" + hex.EncodeToString(nameBytes[:])
				if _, err := adminDB.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if _, err := adminDB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
						t.Error(err)
					}
					if err := adminDB.Close(); err != nil {
						t.Error(err)
					}
				})
				if strings.Contains(dsn, "://") {
					parsed, err := url.Parse(dsn)
					if err != nil {
						t.Fatal("invalid test DSN")
					}
					q := parsed.Query()
					q.Set("search_path", schema)
					parsed.RawQuery = q.Encode()
					dsn = parsed.String()
				} else {
					dsn += " search_path=" + schema
				}
			}
			var passwordBytes [16]byte
			if _, err := rand.Read(passwordBytes[:]); err != nil {
				t.Fatal(err)
			}
			password := hex.EncodeToString(passwordBytes[:])
			environ := append(os.Environ(), "IMGNEST_DATABASE_DRIVER="+driver, "IMGNEST_DATABASE_DSN="+dsn, "IMGNEST_SERVER_ADDR=127.0.0.1:0")
			run := func(input string, args ...string) {
				t.Helper()
				// #nosec G204 -- test-owned binary with fixed test call-site arguments; no shell execution.
				cmd := exec.CommandContext(t.Context(), binary, args...)
				cmd.Env = environ
				cmd.Stdin = strings.NewReader(input)
				var log processLog
				cmd.Stdout = &log
				cmd.Stderr = &log
				if err := cmd.Run(); err != nil {
					t.Fatalf("%s process failed", args[0])
				}
				if strings.Contains(log.text(), password) {
					t.Fatal("CLI emitted password")
				}
			}
			run("", "migrate")
			run(password+"\n", "init-admin", "--username", "smoke", "--email", "smoke@example.com")
			start := func() (string, func()) {
				t.Helper()
				cmd := exec.Command(binary, "serve")
				cmd.Env = environ
				var log processLog
				cmd.Stdout = &log
				cmd.Stderr = &log
				if err := cmd.Start(); err != nil {
					t.Fatal("serve process failed to start")
				}
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				stopped := false
				stop := func() {
					t.Helper()
					if stopped {
						return
					}
					stopped = true
					if err := cmd.Process.Signal(os.Interrupt); err != nil {
						t.Error("could not stop serve process")
					}
					select {
					case err := <-done:
						if err != nil {
							t.Error("serve process did not exit cleanly")
						}
					case <-time.After(5 * time.Second):
						if err := cmd.Process.Kill(); err != nil {
							t.Error(err)
						}
						t.Error("serve process shutdown timed out")
					}
					if strings.Contains(log.text(), password) {
						t.Error("server logged password")
					}
				}
				t.Cleanup(stop)
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					for _, line := range strings.Split(log.text(), "\n") {
						var event struct {
							Address string `json:"address"`
						}
						if json.Unmarshal([]byte(line), &event) == nil && event.Address != "" {
							return "http://" + event.Address, stop
						}
					}
					time.Sleep(5 * time.Millisecond)
				}
				stop()
				t.Fatal("server never became ready")
				return "", stop
			}
			client := &http.Client{Timeout: 3 * time.Second}
			call := func(origin, method, path, body, token string, want int) json.RawMessage {
				t.Helper()
				req, err := http.NewRequestWithContext(t.Context(), method, origin+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				response, err := client.Do(req)
				if err != nil {
					t.Fatal("HTTP process request failed")
				}
				defer func() {
					if err := response.Body.Close(); err != nil {
						t.Error(err)
					}
				}()
				var result struct {
					Code int             `json:"code"`
					Data json.RawMessage `json:"data"`
				}
				if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
					t.Fatal("invalid process response")
				}
				if response.StatusCode != want {
					t.Fatalf("%s %s HTTP=%d want%d", method, path, response.StatusCode, want)
				}
				if want < 400 && result.Code != 0 {
					t.Fatal("success response had error code")
				}
				return result.Data
			}
			loginBody, err := json.Marshal(map[string]string{"email": "smoke@example.com", "password": password})
			if err != nil {
				t.Fatal(err)
			}
			origin, stop := start()
			call(origin, "GET", "/healthz", "", "", 200)
			var login struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(call(origin, "POST", "/api/auth/login", string(loginBody), "", 200), &login); err != nil {
				t.Fatal(err)
			}
			if login.Token == "" {
				t.Fatal("process login returned no token")
			}
			call(origin, "GET", "/api/auth/me", "", login.Token, 200)
			stop()
			origin, stop = start()
			call(origin, "GET", "/api/auth/me", "", login.Token, 200)
			var issued struct {
				Token string `json:"token"`
				Info  struct {
					ID uint64 `json:"id"`
				} `json:"info"`
			}
			if err := json.Unmarshal(call(origin, "POST", "/api/tokens", `{"name":"process-smoke"}`, login.Token, 201), &issued); err != nil {
				t.Fatal(err)
			}
			if issued.Token == "" {
				t.Fatal("process token creation returned no token")
			}
			call(origin, "GET", "/api/auth/me", "", issued.Token, 200)
			call(origin, "DELETE", "/api/tokens/"+strings.SplitN(issued.Token, "|", 2)[0], "", login.Token, 200)
			call(origin, "GET", "/api/auth/me", "", issued.Token, 401)
			call(origin, "POST", "/api/auth/logout", "", login.Token, 200)
			call(origin, "GET", "/api/auth/me", "", login.Token, 401)
			stop()
			client.CloseIdleConnections()
		})
	}
}
