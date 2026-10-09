package repo

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// FindUserByIdentity fetches the account linked to an external subject.
func (r *UserRepository) FindUserByIdentity(ctx context.Context, provider, subject string) (model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Joins("JOIN user_identities ON user_identities.user_id = users.id").
		Where("user_identities.provider = ? AND user_identities.subject = ?", provider, subject).
		First(&user).Error
	if err != nil {
		return model.User{}, repositoryError("find user by identity", err)
	}
	return user, nil
}

// LinkIdentity attaches an external subject to an existing account. A subject
// already linked anywhere reports model.ErrUserExists.
func (r *UserRepository) LinkIdentity(ctx context.Context, identity model.UserIdentity) error {
	if err := checkRecordID(ctx, identity.UserID); err != nil {
		return fmt.Errorf("link identity: %w", err)
	}
	identity.ID = 0
	if err := r.db.WithContext(ctx).Create(&identity).Error; err != nil {
		return userWriteError("link identity", err)
	}
	return nil
}

// CreateUserWithIdentity creates an account and its first identity in one
// transaction, so a failed link never leaves an orphan account behind.
func (r *UserRepository) CreateUserWithIdentity(ctx context.Context, user model.User, identity model.UserIdentity) (model.User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return userWriteError("create user", err)
		}
		identity.ID = 0
		identity.UserID = user.ID
		if err := tx.Create(&identity).Error; err != nil {
			return userWriteError("link identity", err)
		}
		return nil
	})
	if err != nil {
		return model.User{}, fmt.Errorf("create user with identity: %w", err)
	}
	return user, nil
}
