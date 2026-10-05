package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"golang.org/x/crypto/bcrypt"
)

const passwordCost = 12

// maxDisplayNameRunes bounds the self-chosen profile name in Unicode
// characters, not bytes.
const maxDisplayNameRunes = 64

// UserRepository provides account persistence and atomic credential changes.
type UserRepository interface {
	CreateUser(ctx context.Context, user model.User) (model.User, error)
	FindUserByEmail(ctx context.Context, email string) (model.User, error)
	FindUserByID(ctx context.Context, id uint64) (model.User, error)
	BootstrapAdmin(ctx context.Context, user model.User) (model.User, error)
	UpdatePasswordAndRevokeTokens(ctx context.Context, userID uint64, expectedHash, nextHash string) error
	// UpdateDisplayName replaces the optional profile name and returns the
	// refreshed account.
	UpdateDisplayName(ctx context.Context, userID uint64, displayName string) (model.User, error)
}

// RegisterInput contains the user-controlled registration fields plus the
// server-derived client address, which only the owner's profile read returns.
type RegisterInput struct {
	Username string
	Email    string
	Password string
	// IP is supplied by the HTTP layer from the client address; it is never
	// part of a decoded request body.
	IP string
}

// UserView exposes account attributes without credential hashes. Avatar
// fields are derived server-side from the site-wide provider configuration;
// the avatar address never carries the mailbox in plain text.
type UserView struct {
	ID uint64 `json:"id"`
	// DisplayName is the optional self-chosen profile name; clients fall back
	// to the username when it is empty.
	DisplayName string    `json:"display_name"`
	GroupID     uint64    `json:"group_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	UsedBytes   int64     `json:"used_bytes"`
	CreatedAt   time.Time `json:"created_at"`
	// AvatarProvider is the site-wide avatar source that produced avatar_url.
	AvatarProvider string `json:"avatar_provider"`
	// AvatarURL is the HTTPS avatar address derived from the email hash; null
	// means the account has no usable avatar and clients show the local default.
	AvatarURL *string `json:"avatar_url"`
	// AvatarConfigVersion lets clients notice avatar configuration updates.
	AvatarConfigVersion int64 `json:"avatar_config_version"`
}

// TokenSubject is opaque authorization proof produced only by successful authentication.
type TokenSubject struct {
	userID        uint64
	passwordHash  string
	sourceTokenID uint64
}

// VerifiedCredentials binds the user view to the exact password hash that was verified.
type VerifiedCredentials struct {
	User    UserView     `json:"user"`
	Subject TokenSubject `json:"-"`
}

// UserService enforces account creation and password rules.
type UserService struct {
	users    UserRepository
	settings SettingsRepository
}

// NewUserService constructs account operations with injected persistence.
func NewUserService(ctx context.Context, users UserRepository, settings SettingsRepository) (*UserService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct user service: %w", err)
	}
	if users == nil || settings == nil {
		return nil, fmt.Errorf("user service dependencies: %w", ErrInvalidInput)
	}
	return &UserService{users: users, settings: settings}, nil
}

// Register creates an enabled ordinary account when site registration is enabled.
func (s *UserService) Register(ctx context.Context, input RegisterInput) (UserView, error) {
	if err := ctx.Err(); err != nil {
		return UserView{}, fmt.Errorf("register user: %w", err)
	}
	enabled, err := s.settings.RegistrationEnabled(ctx)
	if err != nil {
		return UserView{}, fmt.Errorf("read registration setting: %w", err)
	}
	if !enabled {
		return UserView{}, ErrRegistrationDisabled
	}
	user, err := s.prepareUser(ctx, input, model.UserRoleUser)
	if err != nil {
		return UserView{}, err
	}
	created, err := s.users.CreateUser(ctx, user)
	if err != nil {
		return UserView{}, fmt.Errorf("create user: %w", err)
	}
	return s.decoratedView(ctx, created)
}

// VerifyCredentials returns an enabled user and opaque proof after password verification.
func (s *UserService) VerifyCredentials(ctx context.Context, email, password string) (VerifiedCredentials, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedCredentials{}, fmt.Errorf("verify credentials: %w", err)
	}
	if !validCredentialPassword(password) {
		return VerifiedCredentials{}, ErrInvalidCredentials
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return VerifiedCredentials{}, ErrInvalidCredentials
	}
	user, err := s.users.FindUserByEmail(ctx, normalized)
	if errors.Is(err, ErrNotFound) {
		return VerifiedCredentials{}, ErrInvalidCredentials
	}
	if err != nil {
		return VerifiedCredentials{}, fmt.Errorf("find credential user: %w", err)
	}
	if user.Status != model.UserStatusEnabled {
		return VerifiedCredentials{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return VerifiedCredentials{}, ErrInvalidCredentials
	}
	if err := ctx.Err(); err != nil {
		return VerifiedCredentials{}, fmt.Errorf("verify credentials: %w", err)
	}
	view, err := s.decoratedView(ctx, user)
	if err != nil {
		return VerifiedCredentials{}, err
	}
	return VerifiedCredentials{
		User: view, Subject: TokenSubject{userID: user.ID, passwordHash: user.PasswordHash},
	}, nil
}

// InitAdmin explicitly bootstraps the single initial administrator.
func (s *UserService) InitAdmin(ctx context.Context, input RegisterInput) (UserView, error) {
	if err := ctx.Err(); err != nil {
		return UserView{}, fmt.Errorf("initialize administrator: %w", err)
	}
	user, err := s.prepareUser(ctx, input, model.UserRoleAdmin)
	if err != nil {
		return UserView{}, err
	}
	created, err := s.users.BootstrapAdmin(ctx, user)
	if err != nil {
		return UserView{}, fmt.Errorf("bootstrap administrator: %w", err)
	}
	return s.decoratedView(ctx, created)
}

// ChangePassword checks the current password and revokes all account tokens atomically.
func (s *UserService) ChangePassword(ctx context.Context, userID uint64, current, next string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("change password: %w", err)
	}
	if userID == 0 || !validPassword(next) {
		return ErrInvalidInput
	}
	user, err := s.users.FindUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find password owner: %w", err)
	}
	if user.Status != model.UserStatusEnabled {
		return ErrForbidden
	}
	if !validCredentialPassword(current) {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
		return ErrInvalidCredentials
	}
	return s.replacePassword(
		ctx,
		user.ID,
		user.PasswordHash,
		next,
	)
}

// UpdateDisplayName saves the optional profile name from the authenticated
// identity and returns the refreshed view. Surrounding whitespace is trimmed;
// an empty value clears the custom name so clients fall back to the username.
// Only the display name is user-serviceable here: email, role, group, and
// status have no self-service request shape at all.
func (s *UserService) UpdateDisplayName(ctx context.Context, userID uint64, displayName string) (UserView, error) {
	if err := ctx.Err(); err != nil {
		return UserView{}, fmt.Errorf("update display name: %w", err)
	}
	name := strings.TrimSpace(displayName)
	if name != "" && (!utf8.ValidString(name) || containsControlRune(name) || utf8.RuneCountInString(name) > maxDisplayNameRunes) {
		return UserView{}, ErrInvalidInput
	}
	if userID == 0 {
		return UserView{}, ErrInvalidInput
	}
	user, err := s.users.UpdateDisplayName(ctx, userID, name)
	if err != nil {
		return UserView{}, fmt.Errorf("update display name: %w", err)
	}
	return s.decoratedView(ctx, user)
}

// decoratedView builds the user view and derives its avatar fields from the
// site-wide provider configuration. No external avatar service is contacted.
func (s *UserService) decoratedView(ctx context.Context, user model.User) (UserView, error) {
	config, err := s.settings.AvatarConfig(ctx)
	if err != nil {
		return UserView{}, fmt.Errorf("read avatar config: %w", err)
	}
	view := userView(user)
	applyAvatar(&view, config)
	return view, nil
}

func containsControlRune(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}

// ResetPassword replaces a password and revokes all account tokens atomically.
func (s *UserService) ResetPassword(ctx context.Context, email, next string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	normalized, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if !validPassword(next) {
		return ErrInvalidInput
	}
	user, err := s.users.FindUserByEmail(ctx, normalized)
	if err != nil {
		return fmt.Errorf("find password reset user: %w", err)
	}
	if user.Status != model.UserStatusEnabled {
		return ErrForbidden
	}
	return s.replacePassword(
		ctx,
		user.ID,
		user.PasswordHash,
		next,
	)
}

func (s *UserService) prepareUser(ctx context.Context, input RegisterInput, role string) (model.User, error) {
	username := strings.TrimSpace(input.Username)
	runes := utf8.RuneCountInString(username)
	invalidUsername := !utf8.ValidString(username) || runes < 3 || runes > 64
	if invalidUsername || !validPassword(input.Password) {
		return model.User{}, ErrInvalidInput
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return model.User{}, err
	}
	groupID, err := s.settings.DefaultGroupID(ctx)
	if err != nil {
		return model.User{}, fmt.Errorf("find default group: %w", err)
	}
	if groupID == 0 {
		return model.User{}, fmt.Errorf("invalid default group: %w", ErrInvalidInput)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), passwordCost)
	if err != nil {
		return model.User{}, fmt.Errorf("hash account password: %w", err)
	}
	return model.User{
		Username: username, Email: email, PasswordHash: string(hash),
		GroupID: groupID, Role: role, Status: model.UserStatusEnabled,
		RegisteredIP: strings.TrimSpace(input.IP),
	}, nil
}

func (s *UserService) replacePassword(ctx context.Context, userID uint64, expectedHash, next string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("replace password: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), passwordCost)
	if err != nil {
		return fmt.Errorf("hash replacement password: %w", err)
	}
	if err := s.users.UpdatePasswordAndRevokeTokens(
		ctx,
		userID,
		expectedHash,
		string(hash),
	); err != nil {
		return fmt.Errorf("replace password and revoke tokens: %w", err)
	}
	return nil
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("email: %w", ErrInvalidInput)
	}
	if address.Name != "" || address.Address != email {
		return "", fmt.Errorf("email: %w", ErrInvalidInput)
	}
	return email, nil
}

func validPassword(password string) bool {
	return len(password) >= 12 && len(password) <= 72
}

func validCredentialPassword(password string) bool {
	// Legacy bcrypt hashes may use passwords shorter than the native creation minimum.
	return len(password) > 0 && len(password) <= 72
}

func userView(user model.User) UserView {
	return UserView{
		ID: user.ID, GroupID: user.GroupID, Username: user.Username, DisplayName: user.DisplayName,
		Email: user.Email, Role: user.Role, Status: user.Status, UsedBytes: user.UsedBytes,
		CreatedAt: user.CreatedAt.UTC(),
	}
}
