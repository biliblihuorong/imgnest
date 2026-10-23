package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BillingRepository keeps account money and entitlements on the host connection.
type BillingRepository struct {
	db  *gorm.DB
	now func() time.Time
}

// NewBillingRepository constructs a typed transactional store.
func NewBillingRepository(ctx context.Context, db *gorm.DB, now func() time.Time) (*BillingRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	if now == nil {
		return nil, model.ErrInvalidInput
	}
	return &BillingRepository{db: db, now: now}, nil
}
func billingLock(tx *gorm.DB) *gorm.DB {
	if tx.Name() == "postgres" {
		return tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return tx
}
func (r *BillingRepository) transact(ctx context.Context, fn func(*gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = r.db.WithContext(ctx).Transaction(fn)
		if err == nil {
			return nil
		}
		var state interface{ SQLState() string }
		var sqlite interface{ Code() int }
		retry := errors.As(err, &state) && (state.SQLState() == "40001" || state.SQLState() == "40P01")
		if errors.As(err, &sqlite) {
			retry = retry || sqlite.Code() == 5 || sqlite.Code() == 6
		}
		// go-sqlite3 exposes its numeric code as a field; retain this bounded
		// classification here rather than matching arbitrary driver messages.
		retry = retry || billingSQLiteBusy(err)
		if !retry || attempt == 3 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return repositoryError("billing transaction", err)
}
func (r *BillingRepository) actor(tx *gorm.DB, a extension.BillingActor, admin bool, target uint64) (model.User, model.User, error) {
	if a.UserID == 0 || a.TokenID == 0 || a.UserID > math.MaxInt64 || a.TokenID > math.MaxInt64 || target > math.MaxInt64 {
		return model.User{}, model.User{}, model.ErrUnauthenticated
	}
	if admin {
		if err := lockAdminAccounts(tx); err != nil {
			return model.User{}, model.User{}, err
		}
	}
	ids := []uint64{a.UserID}
	if target != 0 && target != a.UserID {
		ids = append(ids, target)
	}
	slices.Sort(ids)
	var actor, owner model.User
	for _, id := range ids {
		u, err := lockUser(tx.Statement.Context, tx, id)
		if err != nil {
			return actor, owner, err
		}
		if id == a.UserID {
			actor = u
		}
		if id == target || target == 0 {
			owner = u
		}
	}
	if actor.Status != model.UserStatusEnabled || actor.AuthVersion != a.AuthVersion {
		return actor, owner, model.ErrUnauthenticated
	}
	var token model.Token
	if err := tx.First(&token, "id = ? AND user_id = ?", a.TokenID, a.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return actor, owner, model.ErrUnauthenticated
		}
		return actor, owner, err
	}
	if token.Kind != model.TokenKindWeb {
		return actor, owner, model.ErrForbidden
	}
	if token.ExpiresAt != nil && !r.now().Before(*token.ExpiresAt) {
		return actor, owner, model.ErrUnauthenticated
	}
	if admin && actor.Role != model.UserRoleAdmin {
		return actor, owner, model.ErrForbidden
	}
	return actor, owner, nil
}
func billingText(s string, min, max int) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0) && utf8.RuneCountInString(strings.TrimSpace(s)) >= min && utf8.RuneCountInString(s) <= max
}
func billingRequestID(s string) bool {
	if len(s) < 1 || len(s) > 100 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func billingReplay(tx *gorm.DB, actor uint64, op, id string, input any, out any) (bool, error) {
	if !billingRequestID(id) {
		return false, model.ErrInvalidInput
	}
	body, err := json.Marshal(input)
	if err != nil {
		return false, model.ErrInvalidInput
	}
	digest := sha256.Sum256(body)
	var old billingRequest
	err = tx.First(&old, "actor_id = ? AND operation = ? AND request_id = ?", actor, op, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if old.Fingerprint != hex.EncodeToString(digest[:]) {
		return false, extension.ErrBillingConflict
	}
	return true, json.Unmarshal([]byte(old.Result), out)
}
func (r *BillingRepository) recordRequest(tx *gorm.DB, actor uint64, op, id string, input, out any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(body)
	result, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return tx.Create(&billingRequest{ActorID: actor, Operation: op, RequestID: id, Fingerprint: hex.EncodeToString(digest[:]), Result: string(result), CreatedAt: r.now().UTC()}).Error
}
func billingPage[T any](tx *gorm.DB, table any, f extension.BillingFilter) (extension.BillingPage[T], error) {
	page := extension.BillingPage[T]{Items: []T{}, Page: f.Page, Size: f.Size}
	if f.Page < 1 || f.Size < 1 || f.Size > 100 || f.Page > 1_000_000 {
		return page, model.ErrInvalidInput
	}
	dateColumn := "created_at"
	switch table.(type) {
	case billingWallet, billingMembership:
		dateColumn = "updated_at"
	}
	query := tx.Model(table)
	if f.RequestID != "" {
		if !billingRequestID(f.RequestID) {
			return page, model.ErrInvalidInput
		}
		if _, ok := table.(billingOrder); !ok {
			return page, model.ErrInvalidInput
		}
		query = query.Where("request_id = ?", f.RequestID)
	}
	if f.UserID != 0 {
		query = query.Where("user_id = ?", f.UserID)
	}
	if f.From != nil {
		query = query.Where(dateColumn+" >= ?", f.From.UTC())
	}
	if f.To != nil {
		query = query.Where(dateColumn+" < ?", f.To.UTC())
	}
	if err := query.Count(&page.Total).Error; err != nil {
		return page, err
	}
	order := "id DESC"
	switch table.(type) {
	case billingWallet, billingMembership:
		order = "user_id ASC"
	}
	err := query.Order(order).Offset((f.Page - 1) * f.Size).Limit(f.Size).Find(&page.Items).Error
	return page, err
}
