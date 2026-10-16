package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// EmailCodeRepository stores pending registration and password-reset codes.
type EmailCodeRepository struct{ db *gorm.DB }

// NewEmailCodeRepository creates the email code store.
func NewEmailCodeRepository(ctx context.Context, db *gorm.DB) (*EmailCodeRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &EmailCodeRepository{db: db}, nil
}

type emailCodeRow struct {
	ID        uint64
	Purpose   string
	Email     string
	CodeHash  string
	Attempts  int
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (emailCodeRow) TableName() string { return "email_codes" }

// FindEmailCode returns the pending code for purpose and email.
func (r *EmailCodeRepository) FindEmailCode(ctx context.Context, purpose, email string) (model.EmailCode, error) {
	var row emailCodeRow
	err := r.db.WithContext(ctx).Where("purpose = ? AND email = ?", purpose, email).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.EmailCode{}, fmt.Errorf("find email code: %w", model.ErrNotFound)
	}
	if err != nil {
		return model.EmailCode{}, databaseError("find email code", err)
	}
	return model.EmailCode(row), nil
}

// ReplaceEmailCode stores code as the only pending code for its purpose and
// email, resetting the attempt counter.
func (r *EmailCodeRepository) ReplaceEmailCode(ctx context.Context, code model.EmailCode) (model.EmailCode, error) {
	var row emailCodeRow
	err := r.db.WithContext(ctx).Raw(`INSERT INTO email_codes (purpose, email, code_hash, attempts, expires_at, created_at)
		VALUES (?, ?, ?, 0, ?, ?)
		ON CONFLICT (purpose, email) DO UPDATE
		SET code_hash = excluded.code_hash, attempts = 0, expires_at = excluded.expires_at, created_at = excluded.created_at
		RETURNING id, purpose, email, code_hash, attempts, expires_at, created_at`,
		code.Purpose, code.Email, code.CodeHash, code.ExpiresAt.UTC(), code.CreatedAt.UTC()).Scan(&row).Error
	if err != nil {
		return model.EmailCode{}, databaseError("store email code", err)
	}
	return model.EmailCode(row), nil
}

// AddEmailCodeAttempt counts one wrong guess against the code with id.
func (r *EmailCodeRepository) AddEmailCodeAttempt(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Exec("UPDATE email_codes SET attempts = attempts + 1 WHERE id = ?", id).Error; err != nil {
		return databaseError("count email code attempt", err)
	}
	return nil
}

// DeleteEmailCode removes the code with id and the exact hash it was checked
// against; false means another request already used or replaced it.
func (r *EmailCodeRepository) DeleteEmailCode(ctx context.Context, id uint64, codeHash string) (bool, error) {
	result := r.db.WithContext(ctx).Exec("DELETE FROM email_codes WHERE id = ? AND code_hash = ?", id, codeHash)
	if result.Error != nil {
		return false, databaseError("delete email code", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// PruneEmailCodes deletes codes that expired before cutoff.
func (r *EmailCodeRepository) PruneEmailCodes(ctx context.Context, cutoff time.Time) error {
	if err := r.db.WithContext(ctx).Exec("DELETE FROM email_codes WHERE expires_at < ?", cutoff.UTC()).Error; err != nil {
		return databaseError("prune email codes", err)
	}
	return nil
}
