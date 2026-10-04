package repo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// TokenRepository persists hashed bearer tokens.
type TokenRepository struct{ db *gorm.DB }

// NewTokenRepository creates a token persistence adapter.
func NewTokenRepository(ctx context.Context, db *gorm.DB) (*TokenRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &TokenRepository{db: db}, nil
}

// CreateToken verifies the credential grant under the user's lock and inserts
// the secret digest in the same transaction.
func (r *TokenRepository) CreateToken(ctx context.Context, token model.Token, grant model.TokenGrant) (model.Token, error) {
	if token.Abilities == nil {
		token.Abilities = []string{}
	}
	if token.ExpiresAt != nil {
		at := token.ExpiresAt.UTC()
		token.ExpiresAt = &at
	}
	if token.LastUsedAt != nil {
		at := token.LastUsedAt.UTC()
		token.LastUsedAt = &at
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockUser(ctx, tx, token.UserID)
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound) {
			return model.ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if err := validateGrant(ctx, tx, user, grant); err != nil {
			return err
		}
		return tx.Create(&token).Error
	})
	if errors.Is(err, model.ErrUnauthenticated) {
		return model.Token{}, fmt.Errorf("create token: %w", model.ErrUnauthenticated)
	}
	if err != nil {
		return model.Token{}, repositoryError("create token", err)
	}
	return token, nil
}

// FindToken fetches one persisted bearer token by ID.
func (r *TokenRepository) FindToken(ctx context.Context, id uint64) (model.Token, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.Token{}, fmt.Errorf("find token: %w", err)
	}
	var token model.Token
	if err := r.db.WithContext(ctx).First(&token, "id = ?", id).Error; err != nil {
		return model.Token{}, repositoryError("find token", err)
	}
	return token, nil
}

// TouchToken records a successful use and reports a token revoked during authentication.
func (r *TokenRepository) TouchToken(ctx context.Context, id uint64, at time.Time) error {
	if err := checkRecordID(ctx, id); err != nil {
		return fmt.Errorf("touch token: %w", err)
	}
	updated := r.db.WithContext(ctx).Model(&model.Token{}).Where("id = ?", id).Update("last_used_at", at.UTC())
	if updated.Error != nil {
		return repositoryError("touch token", updated.Error)
	}
	if updated.RowsAffected == 0 {
		return fmt.Errorf("touch token: %w", model.ErrNotFound)
	}
	return nil
}

// ListTokens returns only the named user's token records in stable creation order.
func (r *TokenRepository) ListTokens(ctx context.Context, userID uint64) ([]model.Token, error) {
	tokens := []model.Token{}
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id ASC").Find(&tokens).Error; err != nil {
		return nil, repositoryError("list tokens", err)
	}
	return tokens, nil
}

// RevokeToken deletes an owned token; foreign and missing IDs are indistinguishable.
func (r *TokenRepository) RevokeToken(ctx context.Context, userID, tokenID uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if userID > math.MaxInt64 || tokenID > math.MaxInt64 {
		return fmt.Errorf("revoke token: %w", model.ErrForbidden)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound) {
				return model.ErrForbidden
			}
			return err
		}
		deleted := tx.Where("user_id = ? AND id = ?", userID, tokenID).Delete(&model.Token{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return model.ErrForbidden
		}
		return nil
	})
	if errors.Is(err, model.ErrForbidden) {
		return fmt.Errorf("revoke token: %w", model.ErrForbidden)
	}
	if err != nil {
		return repositoryError("revoke token", err)
	}
	return nil
}

// RevokeAllTokens removes all bearer credentials belonging to the user.
func (r *TokenRepository) RevokeAllTokens(ctx context.Context, userID uint64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound) {
				return nil
			}
			return err
		}
		return tx.Where("user_id = ?", userID).Delete(&model.Token{}).Error
	})
	if err != nil {
		return repositoryError("revoke all tokens", err)
	}
	return nil
}
