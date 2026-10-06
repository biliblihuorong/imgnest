// Package captcha implements fixed, bounded CAPTCHA provider protocols.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Provider errors deliberately contain no upstream response or credential data.
var (
	ErrRejected    = errors.New("captcha challenge rejected")
	ErrUnavailable = errors.New("captcha provider unavailable")
)

// Request is a single immutable configuration/token pair; account fields are absent.
type Request struct {
	Provider  string
	Secret    string
	Token     string
	Hostnames []string
	Action    string
}

// Turnstile validates tokens only against Cloudflare's fixed HTTPS endpoint.
type Turnstile struct {
	client *http.Client
	now    func() time.Time
}

// NewTurnstile builds a bounded client without redirects or automatic retries.
func NewTurnstile() *Turnstile {
	return &Turnstile{now: time.Now, client: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second,
			IdleConnTimeout: 30 * time.Second, MaxIdleConns: 10, MaxConnsPerHost: 20,
		},
	}}
}

// Verify rejects expired/replayed challenges, unexpected actions and untrusted hosts.
// Cloudflare enforces single use. No retry is made after an ambiguous network result.
func (v *Turnstile) Verify(ctx context.Context, in Request) error {
	if in.Provider != "turnstile" || len(in.Token) == 0 || len(in.Token) > 2048 || in.Secret == "" ||
		(in.Action != "login" && in.Action != "register") || len(in.Hostnames) == 0 {
		return ErrRejected
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	form := url.Values{"secret": {in.Secret}, "response": {in.Token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	// Response validity depends on the bounded read; closing only releases transport resources.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<10)+1))
	if err != nil || len(body) > 16<<10 {
		return ErrUnavailable
	}
	var result struct {
		Success     bool      `json:"success"`
		Hostname    string    `json:"hostname"`
		Action      string    `json:"action"`
		ChallengeTS time.Time `json:"challenge_ts"`
		ErrorCodes  []string  `json:"error-codes"`
	}
	if json.Unmarshal(body, &result) != nil {
		return ErrUnavailable
	}
	if len(result.ErrorCodes) > 0 {
		if result.Success {
			return ErrUnavailable
		}
		for _, code := range result.ErrorCodes {
			switch code {
			case "missing-input-response", "invalid-input-response", "timeout-or-duplicate":
				// Only failures attributed to the submitted challenge are retryable challenges.
			default:
				return ErrUnavailable
			}
		}
		return ErrRejected
	}
	if !result.Success {
		return ErrUnavailable
	}
	age := v.now().Sub(result.ChallengeTS)
	if result.Action != in.Action || !slices.Contains(in.Hostnames, result.Hostname) ||
		result.ChallengeTS.IsZero() || age < 0 || age > 300*time.Second {
		return ErrRejected
	}
	return nil
}
