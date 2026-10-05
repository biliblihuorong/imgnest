package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserRepository persists user accounts and their credential lifecycle.
type UserRepository struct{ db *gorm.DB }

// NewUserRepository creates a user persistence adapter.
func NewUserRepository(ctx context.Context, db *gorm.DB) (*UserRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &UserRepository{db: db}, nil
}

// CreateUser inserts an account; database uniqueness decides conflicting requests.
func (r *UserRepository) CreateUser(ctx context.Context, user model.User) (model.User, error) {
	if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
		return model.User{}, userWriteError("create user", err)
	}
	return user, nil
}

// FindUserByEmail fetches an account by its normalized email address.
func (r *UserRepository) FindUserByEmail(ctx context.Context, email string) (model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return model.User{}, repositoryError("find user by email", err)
	}
	return user, nil
}

// FindUserByID fetches an account by its database ID.
func (r *UserRepository) FindUserByID(ctx context.Context, id uint64) (model.User, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.User{}, fmt.Errorf("find user by ID: %w", err)
	}
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return model.User{}, repositoryError("find user by ID", err)
	}
	return user, nil
}

// BootstrapAdmin creates exactly one initial administrator without promoting an existing account.
func (r *UserRepository) BootstrapAdmin(ctx context.Context, user model.User) (model.User, error) {
	user.Role = model.UserRoleAdmin
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(current_schema()), 1229801282)").Error; err != nil {
				return err
			}
		}
		// SQLite transactions are BEGIN IMMEDIATE through the connection DSN.
		var admins int64
		if err := tx.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&admins).Error; err != nil {
			return err
		}
		if admins != 0 {
			return model.ErrForbidden
		}
		return tx.Create(&user).Error
	})
	if errors.Is(err, model.ErrForbidden) {
		return model.User{}, fmt.Errorf("bootstrap admin: %w", model.ErrForbidden)
	}
	if err != nil {
		return model.User{}, userWriteError("bootstrap admin", err)
	}
	return user, nil
}

// UpdateDisplayName replaces the optional profile name and returns the
// refreshed account; an empty value clears the custom display name.
func (r *UserRepository) UpdateDisplayName(
	ctx context.Context,
	userID uint64,
	displayName string,
) (model.User, error) {
	if err := checkRecordID(ctx, userID); err != nil {
		return model.User{}, fmt.Errorf("update display name: %w", err)
	}
	var user model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).
			Where("id = ?", userID).
			Update("display_name", displayName).Error; err != nil {
			return err
		}
		return tx.First(&user, "id = ?", userID).Error
	})
	if err != nil {
		return model.User{}, repositoryError("update display name", err)
	}
	return user, nil
}

// UpdatePasswordAndRevokeTokens replaces an enabled user's credential only when
// expectedHash still matches, revoking all tokens in the same transaction.
func (r *UserRepository) UpdatePasswordAndRevokeTokens(
	ctx context.Context,
	userID uint64,
	expectedHash, nextHash string,
) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		updated := tx.Model(&model.User{}).
			Where(
				"id = ? AND password_hash = ? AND status = ?",
				userID,
				expectedHash,
				model.UserStatusEnabled,
			).
			Update("password_hash", nextHash)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return model.ErrInvalidCredentials
		}
		return tx.Where("user_id = ?", userID).Delete(&model.Token{}).Error
	})
	if errors.Is(err, model.ErrInvalidCredentials) {
		return fmt.Errorf("update password: %w", model.ErrInvalidCredentials)
	}
	if errors.Is(err, model.ErrNotFound) {
		return fmt.Errorf("update password: %w", model.ErrNotFound)
	}
	if err != nil {
		return repositoryError("update password and revoke tokens", err)
	}
	return nil
}

func lockUser(ctx context.Context, tx *gorm.DB, userID uint64) (model.User, error) {
	if err := checkRecordID(ctx, userID); err != nil {
		return model.User{}, err
	}
	query := tx.WithContext(ctx)
	if tx.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user model.User
	err := query.First(&user, "id = ?", userID).Error
	return user, err
}

func userWriteError(operation string, err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%s: %w", operation, model.ErrUserExists)
	}
	return repositoryError(operation, err)
}
