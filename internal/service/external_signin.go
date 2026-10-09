package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// IdentityRepository persists links between external subjects and accounts.
type IdentityRepository interface {
	FindUserByIdentity(ctx context.Context, provider, subject string) (model.User, error)
	LinkIdentity(ctx context.Context, identity model.UserIdentity) error
	CreateUserWithIdentity(ctx context.Context, user model.User, identity model.UserIdentity) (model.User, error)
}

// ExternalSignIn is an identity a trusted plugin has already verified.
type ExternalSignIn struct {
	// Provider is namespaced by the plugin ("sso:github"); Subject is the
	// provider's stable user ID, never an email address.
	Provider string
	Subject  string
	Email    string
	// EmailVerified must be true only when the provider asserts it.
	EmailVerified bool
	// UsernameHint seeds the username of an account created on first sign-in.
	UsernameHint string
	// LinkByEmail allows attaching a first-time subject to an existing
	// non-administrator account with the same verified email.
	LinkByEmail bool
	// IP is the server-derived client address recorded on account creation.
	IP string
}

// usernameAttempts bounds retries when a generated username is taken.
const usernameAttempts = 5

// UseIdentities enables SignInExternal; without it every external sign-in is
// rejected as not linked.
func (s *UserService) UseIdentities(identities IdentityRepository) { s.identities = identities }

// SignInExternal resolves a verified external identity to an account:
// a linked subject signs in; otherwise it links by verified email when the
// provider allows it, or creates an ordinary account when registration is
// open. Administrators are never linked automatically.
func (s *UserService) SignInExternal(ctx context.Context, in ExternalSignIn) (VerifiedCredentials, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedCredentials{}, fmt.Errorf("external sign-in: %w", err)
	}
	if s.identities == nil {
		return VerifiedCredentials{}, ErrIdentityNotLinked
	}
	if !validIdentityPart(in.Provider, 80) || !validIdentityPart(in.Subject, 255) {
		return VerifiedCredentials{}, ErrInvalidInput
	}
	user, err := s.identities.FindUserByIdentity(ctx, in.Provider, in.Subject)
	if err == nil {
		return s.externalCredentials(ctx, user)
	}
	if !errors.Is(err, ErrNotFound) {
		return VerifiedCredentials{}, fmt.Errorf("find linked account: %w", err)
	}
	email := ""
	if in.EmailVerified {
		if normalized, err := normalizeEmail(in.Email); err == nil {
			email = normalized
		}
	}
	identity := model.UserIdentity{Provider: in.Provider, Subject: in.Subject}
	if email != "" {
		existing, err := s.users.FindUserByEmail(ctx, email)
		switch {
		case err == nil:
			if !in.LinkByEmail || existing.Role == model.UserRoleAdmin {
				return VerifiedCredentials{}, ErrIdentityNotLinked
			}
			identity.UserID = existing.ID
			if err := s.identities.LinkIdentity(ctx, identity); err != nil {
				return s.afterLinkConflict(ctx, in, err)
			}
			return s.externalCredentials(ctx, existing)
		case !errors.Is(err, ErrNotFound):
			return VerifiedCredentials{}, fmt.Errorf("find account by email: %w", err)
		}
	}
	return s.createExternalUser(ctx, in, email, identity)
}

func (s *UserService) createExternalUser(ctx context.Context, in ExternalSignIn, email string, identity model.UserIdentity) (VerifiedCredentials, error) {
	enabled, err := s.settings.RegistrationEnabled(ctx)
	if err != nil {
		return VerifiedCredentials{}, fmt.Errorf("read registration setting: %w", err)
	}
	if !enabled {
		return VerifiedCredentials{}, ErrIdentityNotLinked
	}
	if email == "" {
		return VerifiedCredentials{}, ErrIdentityEmailRequired
	}
	groupID, err := s.settings.DefaultGroupID(ctx)
	if err != nil {
		return VerifiedCredentials{}, fmt.Errorf("find default group: %w", err)
	}
	if groupID == 0 {
		return VerifiedCredentials{}, fmt.Errorf("invalid default group: %w", ErrInvalidInput)
	}
	// The account gets a random password nobody knows; the owner signs in
	// through the provider until they set one through a reset.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return VerifiedCredentials{}, fmt.Errorf("generate account secret: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(secret)), passwordCost)
	if err != nil {
		return VerifiedCredentials{}, fmt.Errorf("hash account secret: %w", err)
	}
	base := usernameBase(in.UsernameHint, email)
	for attempt := range usernameAttempts {
		username := base
		if attempt > 0 {
			username = base + "-" + randomSuffix()
		}
		created, err := s.identities.CreateUserWithIdentity(ctx, model.User{
			Username: username, Email: email, PasswordHash: string(hash),
			GroupID: groupID, Role: model.UserRoleUser, Status: model.UserStatusEnabled,
			RegisteredIP: strings.TrimSpace(in.IP),
		}, identity)
		if err == nil {
			return s.externalCredentials(ctx, created)
		}
		if !errors.Is(err, ErrUserExists) {
			return VerifiedCredentials{}, fmt.Errorf("create external account: %w", err)
		}
		// A concurrent sign-in may have linked this subject, or registered
		// this email, between the checks above and the insert.
		if linked, err := s.identities.FindUserByIdentity(ctx, in.Provider, in.Subject); err == nil {
			return s.externalCredentials(ctx, linked)
		}
		if _, err := s.users.FindUserByEmail(ctx, email); err == nil {
			return VerifiedCredentials{}, ErrIdentityNotLinked
		}
	}
	return VerifiedCredentials{}, fmt.Errorf("pick a free username: %w", ErrUserExists)
}

// afterLinkConflict resolves a link that lost a race to a concurrent one.
func (s *UserService) afterLinkConflict(ctx context.Context, in ExternalSignIn, err error) (VerifiedCredentials, error) {
	if !errors.Is(err, ErrUserExists) {
		return VerifiedCredentials{}, fmt.Errorf("link identity: %w", err)
	}
	user, findErr := s.identities.FindUserByIdentity(ctx, in.Provider, in.Subject)
	if findErr != nil {
		return VerifiedCredentials{}, fmt.Errorf("link identity: %w", err)
	}
	return s.externalCredentials(ctx, user)
}

// externalCredentials issues the same opaque proof as a password login, bound
// to the account's current password hash and authorization state.
func (s *UserService) externalCredentials(ctx context.Context, user model.User) (VerifiedCredentials, error) {
	if user.Status != model.UserStatusEnabled {
		return VerifiedCredentials{}, ErrForbidden
	}
	view, err := s.decoratedView(ctx, user)
	if err != nil {
		return VerifiedCredentials{}, err
	}
	return VerifiedCredentials{
		User: view, Subject: TokenSubject{userID: user.ID, passwordHash: user.PasswordHash, accountState: accountState(user)},
	}, nil
}

func validIdentityPart(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	return !containsControlRune(value)
}

// usernameBase keeps letters, digits, '-', '_' and '.' from the hint (or the
// email's local part) and pads short results so they pass username rules.
func usernameBase(hint, email string) string {
	clean := func(raw string) string {
		var b strings.Builder
		for _, r := range raw {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
				b.WriteRune(r)
			}
			if utf8.RuneCountInString(b.String()) >= 48 {
				break
			}
		}
		return b.String()
	}
	base := clean(hint)
	if utf8.RuneCountInString(base) < 3 {
		local, _, _ := strings.Cut(email, "@")
		base = clean(local)
	}
	if utf8.RuneCountInString(base) < 3 {
		base = "user-" + randomSuffix()
	}
	return base
}

func randomSuffix() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "000000"
	}
	return hex.EncodeToString(b)
}
