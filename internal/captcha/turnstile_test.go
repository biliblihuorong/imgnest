package captcha

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTurnstileValidationAndBoundedProtocol(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	valid := fmt.Sprintf(`{"success":true,"hostname":"images.example.com","action":"login","challenge_ts":%q}`, now.Add(-time.Second).Format(time.RFC3339))
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"valid", valid, 200, nil},
		{"replayed", `{"success":false,"error-codes":["timeout-or-duplicate"]}`, 200, ErrRejected},
		{"invalid response token", `{"success":false,"error-codes":["invalid-input-response"]}`, 200, ErrRejected},
		{"provider internal error", `{"success":false,"error-codes":["internal-error"]}`, 200, ErrUnavailable},
		{"invalid provider credentials", `{"success":false,"error-codes":["invalid-input-secret"]}`, 200, ErrUnavailable},
		{"provider request error", `{"success":false,"error-codes":["bad-request"]}`, 200, ErrUnavailable},
		{"unknown provider failure", `{"success":false,"error-codes":["new-provider-error"]}`, 200, ErrUnavailable},
		{"missing result", `{}`, 200, ErrUnavailable},
		{"contradictory result", `{"success":true,"error-codes":["invalid-input-response"]}`, 200, ErrUnavailable},
		{"wrong hostname", strings.Replace(valid, "images.example.com", "evil.example.com", 1), 200, ErrRejected},
		{"wrong action", strings.Replace(valid, "login", "register", 1), 200, ErrRejected},
		{"expired", strings.Replace(valid, now.Add(-time.Second).Format(time.RFC3339), now.Add(-301*time.Second).Format(time.RFC3339), 1), 200, ErrRejected},
		{"future", strings.Replace(valid, now.Add(-time.Second).Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), 1), 200, ErrRejected},
		{"missing timestamp", `{"success":true,"hostname":"images.example.com","action":"login"}`, 200, ErrRejected},
		{"invalid JSON", "secret-token-private-response", 200, ErrUnavailable},
		{"trailing JSON", valid + `{}`, 200, ErrUnavailable},
		{"body budget", strings.Repeat("x", 16385), 200, ErrUnavailable},
		{"upstream error", "secret-token-private-response", 503, ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			adapter := NewTurnstile()
			adapter.now = func() time.Time { return now }
			adapter.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "POST" || r.URL.String() != "https://challenges.cloudflare.com/turnstile/v0/siteverify" {
					t.Fatalf("unexpected target %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Fatal("wrong content type")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				form, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatal(err)
				}
				if len(form) != 2 || form.Get("secret") != "private-secret" || form.Get("response") != "private-token" {
					t.Fatal("provider payload violates minimal contract")
				}
				if _, ok := r.Context().Deadline(); !ok {
					t.Fatal("missing request deadline")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			err := adapter.Verify(t.Context(), Request{Provider: "turnstile", Secret: "private-secret", Token: "private-token", Hostnames: []string{"images.example.com"}, Action: "login"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want %v", err, tc.want)
			}
			if calls != 1 {
				t.Fatalf("calls=%d; automatic retry not allowed", calls)
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Fatal("sensitive error response escaped")
			}
		})
	}
}
func TestTurnstileRejectsInvalidInputsBeforeNetwork(t *testing.T) {
	for _, tc := range []struct {
		name, provider, token, action string
		hosts                         []string
	}{
		{"missing", "turnstile", "", "login", []string{"images.example.com"}},
		{"overlong", "turnstile", strings.Repeat("a", 2049), "login", []string{"images.example.com"}},
		{"action", "turnstile", "token", "test", []string{"images.example.com"}},
		{"provider", "hcaptcha", "token", "login", []string{"images.example.com"}},
		{"hostname", "turnstile", "token", "login", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewTurnstile()
			a.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid input sent to provider")
				return nil, nil
			})
			if err := a.Verify(t.Context(), Request{Provider: tc.provider, Secret: "secret", Token: tc.token, Action: tc.action, Hostnames: tc.hosts}); !errors.Is(err, ErrRejected) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestTurnstileNetworkFailureIsSanitized(t *testing.T) {
	a := NewTurnstile()
	a.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private token and secret") })
	if err := a.Verify(t.Context(), Request{Provider: "turnstile", Secret: "secret", Token: "token", Action: "login", Hostnames: []string{"images.example.com"}}); err != ErrUnavailable {
		t.Fatalf("unsanitized error=%v", err)
	}
	if a.client.Timeout <= 0 || a.client.Timeout > 6*time.Second {
		t.Fatal("unbounded timeout")
	}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://evil.example", nil)
	if err := a.client.CheckRedirect(req, nil); err == nil {
		t.Fatal("redirect may transmit credentials")
	}
}
