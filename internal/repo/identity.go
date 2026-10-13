package repo

import (
	"context"
	"errors"
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

// LinkIdentity attaches an external subject only while the ordinary account
// still matches the verified email and authorization proof. Account edits and
// the insert share the user lock, so a rejected proof never leaves a link.
// A subject already linked anywhere reports model.ErrUserExists.
func (r *UserRepository) LinkIdentity(ctx context.Context, identity model.UserIdentity, grant model.TokenGrant) (model.User, error) {
	if err := checkRecordID(ctx, identity.UserID); err != nil {
		return model.User{}, fmt.Errorf("link identity: %w", err)
	}
	if grant.UserID != identity.UserID || grant.ExpectedAccountState == nil || grant.ExpectedPasswordHash == "" {
		return model.User{}, fmt.Errorf("link identity: %w", model.ErrInvalidInput)
	}
	var user model.User
	identity.ID = 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, err = lockUser(ctx, tx, identity.UserID)
		if err != nil {
			return err
		}
		if user.Role != model.UserRoleUser {
			return model.ErrIdentityNotLinked
		}
		if err := validateGrant(ctx, tx, user, grant); err != nil {
			if errors.Is(err, model.ErrUnauthenticated) {
				return model.ErrIdentityNotLinked
			}
			return err
		}
		return tx.Create(&identity).Error
	})
	if err != nil {
		return model.User{}, userWriteError("link identity", err)
	}
	return user, nil
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
