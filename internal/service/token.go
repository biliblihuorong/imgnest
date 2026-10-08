package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// Supported token kinds separate expiring web sessions from API credentials.
const (
	// TokenKindWeb identifies a web session with a default 24-hour expiry.
	TokenKindWeb = model.TokenKindWeb
	// TokenKindAPI identifies a client credential with an optional expiry.
	TokenKindAPI = model.TokenKindAPI
)

// TokenRepository persists credential hashes and scopes token mutations to owners.
type TokenRepository interface {
	CreateToken(ctx context.Context, token model.Token, grant model.TokenGrant) (model.Token, error)
	FindToken(ctx context.Context, id uint64) (model.Token, error)
	TouchToken(ctx context.Context, id uint64, at time.Time) error
	ListTokens(ctx context.Context, userID uint64) ([]model.Token, error)
	RevokeToken(ctx context.Context, userID, tokenID uint64) error
	RevokeAllTokens(ctx context.Context, userID uint64) error
}

// TokenInput configures a new credential; M1 supports only the wildcard ability.
type TokenInput struct {
	Name      string
	Kind      string
	ExpiresAt *time.Time
	Abilities []string
}

// TokenView contains credential metadata without a secret or hash.
type TokenView struct {
	ID         uint64     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Abilities  []string   `json:"abilities"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// IssuedToken returns the plaintext credential exactly once, at creation.
type IssuedToken struct {
	Token string    `json:"token"`
	Info  TokenView `json:"info"`
}

// Identity binds an authenticated credential to its enabled owner.
type Identity struct {
	User    UserView
	TokenID uint64
	Kind    string
	Subject TokenSubject `json:"-"`
}

// TokenService creates and verifies compatible bearer credentials.
type TokenService struct {
	tokens   TokenRepository
	users    UserRepository
	settings SettingsRepository
	now      func() time.Time
}

// NewTokenService constructs token operations with persistence and a clock.
func NewTokenService(
	ctx context.Context,
	tokens TokenRepository,
	users UserRepository,
	settings SettingsRepository,
	now func() time.Time,
) (*TokenService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct token service: %w", err)
	}
	missingRepository := tokens == nil || users == nil || settings == nil
	if missingRepository || now == nil {
		return nil, fmt.Errorf("token service dependencies: %w", ErrInvalidInput)
	}
	return &TokenService{tokens: tokens, users: users, settings: settings, now: now}, nil
}

// Issue creates a credential only while the authenticated subject remains valid.
func (s *TokenService) Issue(ctx context.Context, subject TokenSubject, input TokenInput) (IssuedToken, error) {
	if err := ctx.Err(); err != nil {
		return IssuedToken{}, fmt.Errorf("issue token: %w", err)
	}
	if subject.userID == 0 || subject.passwordHash == "" {
		return IssuedToken{}, ErrUnauthenticated
	}
	name := strings.TrimSpace(input.Name)
	validKind := input.Kind == TokenKindWeb || input.Kind == TokenKindAPI
	validAbilities := len(input.Abilities) == 1 && input.Abilities[0] == "*"
	invalidInput := name == "" || !validKind || !validAbilities
	if invalidInput {
		return IssuedToken{}, ErrInvalidInput
	}
	now := s.now().UTC()
	expires := utcTime(input.ExpiresAt)
	if expires == nil && input.Kind == TokenKindWeb {
		at := now.Add(24 * time.Hour)
		expires = &at
	}
	if expires != nil && !expires.After(now) {
		return IssuedToken{}, ErrInvalidInput
	}
	var random [20]byte
	if _, err := rand.Read(random[:]); err != nil {
		return IssuedToken{}, fmt.Errorf("generate token secret: %w", err)
	}
	secret := hex.EncodeToString(random[:])
	digest := sha256.Sum256([]byte(secret))
	token, err := s.tokens.CreateToken(ctx, model.Token{
		UserID: subject.userID, Name: name, Kind: input.Kind,
		TokenHash: hex.EncodeToString(digest[:]), Abilities: []string{"*"},
		ExpiresAt: expires, CreatedAt: now, UpdatedAt: now,
	}, model.TokenGrant{
		ExpectedPasswordHash: subject.passwordHash, ExpectedAccountState: subject.accountState, SourceTokenID: subject.sourceTokenID, At: now,
	})
	if err != nil {
		if errors.Is(err, ErrUnauthenticated) && subject.sourceTokenID == 0 {
			return IssuedToken{}, ErrInvalidCredentials
		}
		return IssuedToken{}, fmt.Errorf("persist token: %w", err)
	}
	return IssuedToken{
		Token: strconv.FormatUint(token.ID, 10) + "|" + secret,
		Info:  tokenView(token),
	}, nil
}

// tokenTouchInterval bounds how often a token's last-used time is rewritten.
const tokenTouchInterval = time.Minute

// Authenticate verifies a bearer secret and records its last-used UTC time.
func (s *TokenService) Authenticate(ctx context.Context, raw string) (Identity, error) {
	if err := ctx.Err(); err != nil {
		return Identity{}, fmt.Errorf("authenticate token: %w", err)
	}
	id, secret, err := parseToken(raw)
	if err != nil {
		return Identity{}, err
	}
	token, err := s.tokens.FindToken(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return Identity{}, fmt.Errorf("find bearer token: %w", err)
	}
	stored, err := hex.DecodeString(token.TokenHash)
	if err != nil || len(stored) != sha256.Size {
		return Identity{}, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(digest[:], stored) != 1 {
		return Identity{}, ErrUnauthenticated
	}
	now := s.now().UTC()
	if token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
		return Identity{}, ErrUnauthenticated
	}
	user, err := s.enabledUser(ctx, token.UserID)
	invalidOwner := errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) || errors.Is(err, ErrInvalidInput)
	if invalidOwner {
		return Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return Identity{}, fmt.Errorf("find bearer owner: %w", err)
	}
	// last_used_at is advisory; refreshing it at most once per minute keeps a
	// page of previews from turning into one database write per image.
	if token.LastUsedAt == nil || now.Sub(token.LastUsedAt.UTC()) >= tokenTouchInterval {
		if err := s.tokens.TouchToken(ctx, token.ID, now); err != nil {
			if errors.Is(err, ErrNotFound) {
				return Identity{}, ErrUnauthenticated
			}
			return Identity{}, fmt.Errorf("touch bearer token: %w", err)
		}
	}
	view := userView(user)
	config, err := s.settings.AvatarConfig(ctx)
	if err != nil {
		return Identity{}, fmt.Errorf("read avatar config: %w", err)
	}
	applyAvatar(&view, config)
	return Identity{
		User: view, TokenID: token.ID, Kind: token.Kind,
		Subject: TokenSubject{userID: user.ID, passwordHash: user.PasswordHash, sourceTokenID: token.ID, accountState: accountState(user)},
	}, nil
}

// List returns the owner's credential metadata without secrets.
func (s *TokenService) List(ctx context.Context, userID uint64) ([]TokenView, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	if _, err := s.enabledUser(ctx, userID); err != nil {
		return nil, err
	}
	tokens, err := s.tokens.ListTokens(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user tokens: %w", err)
	}
	views := make([]TokenView, 0, len(tokens))
	for _, token := range tokens {
		views = append(views, tokenView(token))
	}
	return views, nil
}

// Revoke deletes one credential owned by the enabled user.
func (s *TokenService) Revoke(ctx context.Context, userID, tokenID uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if _, err := s.enabledUser(ctx, userID); err != nil {
		return err
	}
	if tokenID == 0 {
		return ErrInvalidInput
	}
	token, err := s.tokens.FindToken(ctx, tokenID)
	if errors.Is(err, ErrNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("find token to revoke: %w", err)
	}
	if token.UserID != userID {
		return ErrForbidden
	}
	if err := s.tokens.RevokeToken(ctx, userID, tokenID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrForbidden
		}
		return fmt.Errorf("revoke user token: %w", err)
	}
	return nil
}

// RevokeAll deletes every credential owned by the enabled user.
func (s *TokenService) RevokeAll(ctx context.Context, userID uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke all tokens: %w", err)
	}
	if _, err := s.enabledUser(ctx, userID); err != nil {
		return err
	}
	if err := s.tokens.RevokeAllTokens(ctx, userID); err != nil {
		return fmt.Errorf("revoke all user tokens: %w", err)
	}
	return nil
}

func (s *TokenService) enabledUser(ctx context.Context, userID uint64) (model.User, error) {
	if userID == 0 {
		return model.User{}, ErrInvalidInput
	}
	user, err := s.users.FindUserByID(ctx, userID)
	if err != nil {
		return model.User{}, fmt.Errorf("find token user: %w", err)
	}
	if user.Status != model.UserStatusEnabled {
		return model.User{}, ErrForbidden
	}
	return user, nil
}

func parseToken(raw string) (uint64, string, error) {
	idPart, secret, found := strings.Cut(raw, "|")
	invalidFormat := !found || idPart == "" || len(secret) != 40
	if invalidFormat {
		return 0, "", ErrUnauthenticated
	}
	for _, char := range idPart {
		if char < '0' || char > '9' {
			return 0, "", ErrUnauthenticated
		}
	}
	id, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil || id == 0 {
		return 0, "", ErrUnauthenticated
	}
	for _, char := range []byte(secret) {
		isDigit := char >= '0' && char <= '9'
		isLetter := (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')
		if !isDigit && !isLetter {
			return 0, "", ErrUnauthenticated
		}
	}
	return id, secret, nil
}

func tokenView(token model.Token) TokenView {
	return TokenView{
		ID: token.ID, Name: token.Name, Kind: token.Kind,
		Abilities:  append([]string{}, token.Abilities...),
		LastUsedAt: utcTime(token.LastUsedAt), ExpiresAt: utcTime(token.ExpiresAt),
		CreatedAt: token.CreatedAt.UTC(),
	}
}

func utcTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	at := value.UTC()
	return &at
}
