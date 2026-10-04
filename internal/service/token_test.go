package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

func newTokenService(t *testing.T, fixture *authFixture, now *time.Time) *service.TokenService {
	t.Helper()
	svc, err := service.NewTokenService(
		t.Context(),
		fixture.tokens,
		fixture.users,
		func() time.Time { return *now },
	)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func tokenInput() service.TokenInput {
	return service.TokenInput{Name: "client", Kind: "api", Abilities: []string{"*"}}
}

func passwordSubject(t *testing.T, fixture *authFixture, user service.UserView) service.TokenSubject {
	t.Helper()
	verified, err := fixture.service.VerifyCredentials(t.Context(), user.Email, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return verified.Subject
}

func TestTokenFormatAndStoredHash(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.FixedZone("SGT", 8*60*60))
		svc := newTokenService(t, fixture, &now)
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		idPart, secret, found := strings.Cut(issued.Token, "|")
		if !found || len(secret) != 40 {
			t.Fatal("issued token does not have ID and 40-character secret")
		}
		if _, err := hex.DecodeString(secret); err != nil {
			t.Fatal("issued random secret is not hex")
		}
		id, err := strconv.ParseUint(idPart, 10, 64)
		if err != nil || id != issued.Info.ID || id == 0 {
			t.Fatal("issued token ID does not match persisted token ID")
		}
		stored, err := fixture.tokens.FindToken(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(secret))
		if stored.TokenHash != hex.EncodeToString(digest[:]) {
			t.Fatal("stored token hash did not hash only the secret after the pipe")
		}
		if stored.TokenHash == secret || strings.Contains(stored.TokenHash, issued.Token) {
			t.Fatal("repository stored a plaintext token")
		}
		if issued.Info.ExpiresAt != nil || issued.Info.Kind != "api" {
			t.Fatal("API token default expiry or kind is incorrect")
		}
		identity, err := svc.Authenticate(t.Context(), issued.Token)
		if err != nil || identity.User.ID != user.ID || identity.TokenID != id || identity.Kind != "api" {
			t.Fatalf("issued token did not authenticate its owner: %v", err)
		}
		other, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		if other.Token == issued.Token {
			t.Fatal("two issuances returned the same credential")
		}
	})
}

func TestWebTokenDefaultExpiry(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.FixedZone("SGT", 8*60*60))
		svc := newTokenService(t, fixture, &now)
		input := tokenInput()
		input.Kind = "web"
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), input)
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
		if issued.Info.ExpiresAt == nil || !issued.Info.ExpiresAt.Equal(want) {
			t.Fatalf("web expiry=%v, want %v", issued.Info.ExpiresAt, want)
		}
		if issued.Info.ExpiresAt.Location() != time.UTC {
			t.Fatal("expiry DTO is not UTC")
		}
	})
}

func TestTokenInputValidation(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	user := registerUser(t, fixture, "alice")
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := newTokenService(t, fixture, &now)
	subject := passwordSubject(t, fixture, user)
	past := now.Add(-time.Second)
	cases := []struct {
		name  string
		input service.TokenInput
	}{
		{name: "empty name", input: service.TokenInput{Kind: "api", Abilities: []string{"*"}}},
		{name: "blank name", input: service.TokenInput{Name: "  ", Kind: "api", Abilities: []string{"*"}}},
		{name: "unknown kind", input: service.TokenInput{Name: "client", Kind: "other", Abilities: []string{"*"}}},
		{name: "empty abilities", input: service.TokenInput{Name: "client", Kind: "api", Abilities: []string{}}},
		{name: "restricted abilities", input: service.TokenInput{Name: "client", Kind: "api", Abilities: []string{"read"}}},
		{name: "extra abilities", input: service.TokenInput{Name: "client", Kind: "api", Abilities: []string{"*", "admin"}}},
		{name: "expired", input: service.TokenInput{Name: "client", Kind: "api", Abilities: []string{"*"}, ExpiresAt: &past}},
		{
			name:  "expires now",
			input: service.TokenInput{Name: "client", Kind: "api", Abilities: []string{"*"}, ExpiresAt: &now},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Issue(t.Context(), subject, tc.input)
			if !errors.Is(err, service.ErrInvalidInput) {
				t.Fatalf("invalid token input error=%v", err)
			}
		})
	}
	tokens, err := svc.List(t.Context(), user.ID)
	if err != nil || len(tokens) != 0 {
		t.Fatalf("invalid issuance created %d tokens, err=%v", len(tokens), err)
	}
}

func TestAuthenticateMalformed(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := newTokenService(t, fixture, &now)
	secret := strings.Repeat("a", 40)
	cases := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "no pipe", raw: secret},
		{name: "two pipes", raw: "1|" + secret + "|suffix"},
		{name: "missing ID", raw: "|" + secret},
		{name: "zero ID", raw: "0|" + secret},
		{name: "negative ID", raw: "-1|" + secret},
		{name: "nondigit ID", raw: "one|" + secret},
		{name: "signed ID", raw: "+1|" + secret},
		{name: "overflow ID", raw: "18446744073709551616|" + secret},
		{name: "short secret", raw: "1|" + strings.Repeat("a", 39)},
		{name: "long secret", raw: "1|" + strings.Repeat("a", 41)},
		{name: "punctuation secret", raw: "1|" + strings.Repeat("!", 40)},
		{name: "non-ASCII secret", raw: "1|" + strings.Repeat("图", 40)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Authenticate(t.Context(), tc.raw); !errors.Is(err, service.ErrUnauthenticated) {
				t.Fatalf("malformed credential error=%v", err)
			}
		})
	}
}

func TestAuthenticateCompatibleAlphanumericSecret(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		secret := strings.Repeat("Gz8X", 10)
		digest := sha256.Sum256([]byte(secret))
		storedUser, err := fixture.users.FindUserByID(t.Context(), user.ID)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		stored, err := fixture.tokens.CreateToken(t.Context(), model.Token{
			UserID: user.ID, Name: "compatible", Kind: "api", Abilities: []string{"*"},
			TokenHash: hex.EncodeToString(digest[:]),
		}, model.TokenGrant{ExpectedPasswordHash: storedUser.PasswordHash, At: now})
		if err != nil {
			t.Fatal(err)
		}
		svc := newTokenService(t, fixture, &now)
		raw := strconv.FormatUint(stored.ID, 10) + "|" + secret
		if _, err := svc.Authenticate(t.Context(), raw); err != nil {
			t.Fatalf("valid Sanctum-format alphanumeric secret rejected: %v", err)
		}
	})
}

func TestTokenExpiryAtBoundary(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		expires := now.Add(time.Hour)
		input := tokenInput()
		input.ExpiresAt = &expires
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), input)
		if err != nil {
			t.Fatal(err)
		}
		now = expires.Add(-time.Nanosecond)
		if _, err := svc.Authenticate(t.Context(), issued.Token); err != nil {
			t.Fatalf("token rejected before expiry: %v", err)
		}
		now = expires
		if _, err := svc.Authenticate(t.Context(), issued.Token); !errors.Is(err, service.ErrUnauthenticated) {
			t.Fatalf("token authenticated at expiry: %v", err)
		}
	})
}

func TestDisabledUser(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		subject := passwordSubject(t, fixture, user)
		issued, err := svc.Issue(t.Context(), subject, tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		if err := fixture.db.WithContext(t.Context()).Model(&model.User{}).
			Where("id = ?", user.ID).Update("status", "disabled").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Authenticate(t.Context(), issued.Token); !errors.Is(err, service.ErrUnauthenticated) {
			t.Fatalf("disabled user authenticated: %v", err)
		}
		if _, err := svc.Issue(t.Context(), subject, tokenInput()); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("disabled user issued token: %v", err)
		}
		if _, err := svc.List(t.Context(), user.ID); !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("disabled user listed tokens: %v", err)
		}
	})
}

func TestRevokedToken(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Authenticate(t.Context(), issued.Token); err != nil {
			t.Fatal("fresh token failed authentication")
		}
		if err := svc.Revoke(t.Context(), user.ID, issued.Info.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Authenticate(t.Context(), issued.Token); !errors.Is(err, service.ErrUnauthenticated) {
			t.Fatalf("revoked token authenticated: %v", err)
		}
	})
}

func TestVerifiedPasswordProofCannotIssueAfterReset(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		verified, err := fixture.service.VerifyCredentials(t.Context(), user.Email, testPassword)
		if err != nil {
			t.Fatal(err)
		}
		if err := fixture.service.ResetPassword(t.Context(), user.Email, "reset-password-123"); err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		if _, err := svc.Issue(t.Context(), verified.Subject, tokenInput()); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("stale password proof issued a token after reset: %v", err)
		}
		tokens, err := fixture.tokens.ListTokens(t.Context(), user.ID)
		if err != nil || len(tokens) != 0 {
			t.Fatalf("rejected issuance left tokens: count=%d, err=%v", len(tokens), err)
		}
	})
}

func TestIssueRejectsEmptySubject(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := newTokenService(t, fixture, &now)
	_, err := svc.Issue(t.Context(), service.TokenSubject{}, tokenInput())
	if !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("empty authorization subject error=%v, want unauthenticated", err)
	}
}

func TestAuthenticatedTokenCannotIssueAfterRevoke(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		identity, err := svc.Authenticate(t.Context(), issued.Token)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Revoke(t.Context(), user.ID, issued.Info.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Issue(t.Context(), identity.Subject, tokenInput()); !errors.Is(err, service.ErrUnauthenticated) {
			t.Fatalf("revoked bearer proof issued a token: %v", err)
		}
	})
}

func TestAuthenticatedTokenCannotIssueAtExpiry(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		expires := now.Add(time.Hour)
		input := tokenInput()
		input.ExpiresAt = &expires
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), input)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := svc.Authenticate(t.Context(), issued.Token)
		if err != nil {
			t.Fatal(err)
		}
		now = expires
		if _, err := svc.Issue(t.Context(), identity.Subject, tokenInput()); !errors.Is(err, service.ErrUnauthenticated) {
			t.Fatalf("bearer proof issued a token at source expiry: %v", err)
		}
	})
}

func TestTokenOwnerIsolation(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		alice := registerUser(t, fixture, "alice")
		bob := registerUser(t, fixture, "bob")
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		aliceToken, err := svc.Issue(t.Context(), passwordSubject(t, fixture, alice), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		bobToken, err := svc.Issue(t.Context(), passwordSubject(t, fixture, bob), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		aliceList, err := svc.List(t.Context(), alice.ID)
		if err != nil || len(aliceList) != 1 || aliceList[0].ID != aliceToken.Info.ID {
			t.Fatalf("token list crossed owner boundary: count=%d, err=%v", len(aliceList), err)
		}
		if err := svc.Revoke(t.Context(), alice.ID, bobToken.Info.ID); !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("other owner's token revocation error=%v", err)
		}
		if err := svc.Revoke(t.Context(), alice.ID, bobToken.Info.ID+1000); !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("missing token revocation error=%v, want same owner isolation error", err)
		}
		if _, err := svc.Authenticate(t.Context(), bobToken.Token); err != nil {
			t.Fatal("unauthorized revocation removed another user's token")
		}
		if err := svc.RevokeAll(t.Context(), alice.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Authenticate(t.Context(), aliceToken.Token); !errors.Is(err, service.ErrUnauthenticated) {
			t.Fatal("revoke all retained owner's token")
		}
		if _, err := svc.Authenticate(t.Context(), bobToken.Token); err != nil {
			t.Fatal("revoke all removed another user's token")
		}
	})
}

func TestListOmitsSecrets(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		empty, err := svc.List(t.Context(), user.ID)
		if err != nil {
			t.Fatal(err)
		}
		emptyJSON, err := json.Marshal(empty)
		if err != nil || string(emptyJSON) != "[]" {
			t.Fatalf("empty token list is %s, err=%v; want []", emptyJSON, err)
		}
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		stored, err := fixture.tokens.FindToken(t.Context(), issued.Info.ID)
		if err != nil {
			t.Fatal(err)
		}
		list, err := svc.List(t.Context(), user.ID)
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(list)
		if err != nil {
			t.Fatal(err)
		}
		_, secret, found := strings.Cut(issued.Token, "|")
		if !found {
			t.Fatal("issued token has no secret")
		}
		for _, forbidden := range []string{"hash", secret, issued.Token, stored.TokenHash} {
			if strings.Contains(string(body), forbidden) {
				t.Fatal("token list exposed a secret or hash")
			}
		}
		if !strings.Contains(string(body), "\"last_used_at\"") || !strings.Contains(string(body), "\"expires_at\"") {
			t.Fatal("token DTO is missing snake_case nullable times")
		}
	})
}

func TestTouchUpdatesLastUsedAt(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "alice")
		now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.FixedZone("SGT", 8*60*60))
		svc := newTokenService(t, fixture, &now)
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		if issued.Info.LastUsedAt != nil {
			t.Fatal("unused token has last-used timestamp")
		}
		now = now.Add(time.Minute)
		if _, err := svc.Authenticate(t.Context(), issued.Token); err != nil {
			t.Fatal(err)
		}
		stored, err := fixture.tokens.FindToken(t.Context(), issued.Info.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.LastUsedAt == nil || !stored.LastUsedAt.Equal(now.UTC()) {
			t.Fatalf("last_used_at=%v, want UTC authentication time", stored.LastUsedAt)
		}
		list, err := svc.List(t.Context(), user.ID)
		if err != nil || len(list) != 1 {
			t.Fatalf("list after authentication count=%d, err=%v", len(list), err)
		}
		if list[0].LastUsedAt == nil || list[0].LastUsedAt.Location() != time.UTC {
			t.Fatal("last-used DTO is not UTC")
		}
	})
}

type touchFailure struct {
	service.TokenRepository
	err error
}

func (failure touchFailure) TouchToken(context.Context, uint64, time.Time) error {
	return failure.err
}

func TestAuthenticatePropagatesTouchFailure(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	user := registerUser(t, fixture, "alice")
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := newTokenService(t, fixture, &now)
	issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("touch unavailable")
	failing, err := service.NewTokenService(
		t.Context(),
		touchFailure{TokenRepository: fixture.tokens, err: want},
		fixture.users,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	if identity, err := failing.Authenticate(t.Context(), issued.Token); !errors.Is(err, want) || identity.User.ID != 0 {
		t.Fatalf("touch failure returned identity=%d, err=%v", identity.User.ID, err)
	}
}

func TestAuthenticateWrongSecret(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	user := registerUser(t, fixture, "alice")
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	svc := newTokenService(t, fixture, &now)
	issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
	if err != nil {
		t.Fatal(err)
	}
	idPart, secret, found := strings.Cut(issued.Token, "|")
	if !found {
		t.Fatal("issued token has no secret")
	}
	changed := "a" + secret[1:]
	if secret[0] == 'a' {
		changed = "b" + secret[1:]
	}
	if _, err := svc.Authenticate(t.Context(), idPart+"|"+changed); !errors.Is(err, service.ErrUnauthenticated) {
		t.Fatalf("incorrect secret authenticated: %v", err)
	}
	stored, err := fixture.tokens.FindToken(t.Context(), issued.Info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastUsedAt != nil {
		t.Fatal("invalid secret changed last-used timestamp")
	}
}
