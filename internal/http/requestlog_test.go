package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type auditStub struct {
	mu      sync.Mutex
	entries []service.AuditEntry
	query   model.AuditQuery
}

func (s *auditStub) Record(_ context.Context, entry service.AuditEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (s *auditStub) AuditLogs(_ context.Context, query model.AuditQuery) (service.AuditPage, error) {
	s.query = query
	return service.AuditPage{Items: []service.AuditView{}, Page: query.Page, Size: query.Size}, nil
}

func auditFixture(t *testing.T) (*auditStub, *tokensStub, *bytes.Buffer, func(method, path, body, bearer string, header ...string) *httptest.ResponseRecorder) {
	t.Helper()
	audit, tokens, logs := &auditStub{}, &tokensStub{}, &bytes.Buffer{}
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{
		Users: &usersStub{}, Tokens: tokens, Logger: slog.New(slog.NewJSONHandler(logs, nil)), Server: config.Server{TrustedProxies: []string{}},
		Now: time.Now, Health: func(context.Context) error { return nil }, Audit: audit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return audit, tokens, logs, func(method, path, body, bearer string, header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
}

func lastLogLine(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestRequestLogCarriesClientUserAndRequestID(t *testing.T) {
	_, _, logs, do := auditFixture(t)
	w := do("GET", "/api/tokens", "", testBearer, "X-Request-ID", "proxy-req-0001")
	if got := w.Header().Get("X-Request-ID"); got != "proxy-req-0001" {
		t.Fatalf("X-Request-ID=%q, want the proxy's", got)
	}
	record := lastLogLine(t, logs)
	if record["msg"] != "http request" || record["request_id"] != "proxy-req-0001" || record["client_ip"] != "192.0.2.1" || record["user_id"] != float64(7) || record["route"] != "/api/tokens" {
		t.Fatalf("request log=%v", record)
	}
	if strings.Contains(logs.String(), testBearer) {
		t.Fatal("token logged")
	}
	// A malformed ID is never echoed into the log or the response.
	w = do("GET", "/healthz", "", "", "X-Request-ID", "bad id\"injected")
	generated := w.Header().Get("X-Request-ID")
	if generated == "" || strings.Contains(generated, "injected") || strings.Contains(logs.String(), "injected") {
		t.Fatalf("malformed request id kept: %q", generated)
	}
	if _, signedIn := lastLogLine(t, logs)["user_id"]; signedIn {
		t.Fatal("anonymous request logged a user")
	}
}

func TestAuditRecordsStateChangingRequestsOnly(t *testing.T) {
	audit, _, _, do := auditFixture(t)
	do("GET", "/api/tokens", "", testBearer)
	do("DELETE", "/api/tokens/99", "", testBearer, "X-Request-ID", "req-delete-01")
	do("POST", "/api/auth/login", `{"email":"tester@example.com","password":"valid-password-123"}`, "")
	do("POST", "/api/variants", `{}`, "")
	do("POST", "/api/nowhere", `{}`, "")
	if len(audit.entries) != 2 {
		t.Fatalf("audit entries=%+v, want the delete and the sign-in", audit.entries)
	}
	deleted := audit.entries[0]
	if deleted.Method != "DELETE" || deleted.Route != "/api/tokens/:id" || deleted.Target != "id=99" || deleted.UserID != 7 || deleted.UserName != "tester" || deleted.Status != 200 || deleted.IP != "192.0.2.1" || deleted.RequestID != "req-delete-01" || deleted.At.IsZero() {
		t.Fatalf("delete entry=%+v", deleted)
	}
	login := audit.entries[1]
	if login.Route != "/api/auth/login" || login.UserID != 7 || login.Target != "" {
		t.Fatalf("sign-in entry=%+v", login)
	}
	for _, entry := range audit.entries {
		if strings.Contains(entry.Target, "password") {
			t.Fatal("request body reached the audit trail")
		}
	}
}

func TestAuditLogsAdminOnly(t *testing.T) {
	audit, tokens, _, do := auditFixture(t)
	expectCode(t, do("GET", "/api/admin/audit-logs", "", testBearer), 403, 20003)
	tokens.role = "admin"
	expectCode(t, do("GET", "/api/admin/audit-logs?page=2&size=10&user_id=3&failed=true", "", testBearer), 200, 0)
	if audit.query != (model.AuditQuery{Page: 2, Size: 10, UserID: 3, FailedOnly: true}) {
		t.Fatalf("query=%+v", audit.query)
	}
	expectCode(t, do("GET", "/api/admin/audit-logs?user_id=0", "", testBearer), 400, 10001)
	expectCode(t, do("GET", "/api/admin/audit-logs?size=500", "", testBearer), 400, 10001)
}
