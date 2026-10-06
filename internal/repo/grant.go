package repo

import (
	"context"
	"crypto/subtle"
	"errors"
	"math"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func validateGrant(ctx context.Context, tx *gorm.DB, user model.User, grant model.TokenGrant) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if grant.UserID != 0 && grant.UserID != user.ID {
		return model.ErrUnauthenticated
	}
	currentHash := subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(grant.ExpectedPasswordHash)) == 1
	if user.Status != model.UserStatusEnabled || !currentHash {
		return model.ErrUnauthenticated
	}
	if expected := grant.ExpectedAccountState; expected != nil {
		if expected.AuthVersion != user.AuthVersion || expected.Username != user.Username || expected.Email != user.Email ||
			expected.Role != user.Role || expected.Status != user.Status || expected.GroupID != user.GroupID {
			return model.ErrUnauthenticated
		}
	}
	if grant.SourceTokenID == 0 {
		return nil
	}
	if grant.SourceTokenID > math.MaxInt64 {
		return model.ErrUnauthenticated
	}
	var source model.Token
	if err := tx.WithContext(ctx).First(&source, "id = ?", grant.SourceTokenID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ErrUnauthenticated
		}
		return err
	}
	if source.UserID != user.ID {
		return model.ErrUnauthenticated
	}
	if source.ExpiresAt != nil && !grant.At.Before(*source.ExpiresAt) {
		return model.ErrUnauthenticated
	}
	return nil
}
