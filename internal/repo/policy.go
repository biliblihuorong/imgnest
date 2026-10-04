package repo

import (
	"context"
	"fmt"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PolicyRepository resolves processing rules permitted by a user's group.
type PolicyRepository struct{ db *gorm.DB }

// NewPolicyRepository constructs the processing-rule persistence adapter.
func NewPolicyRepository(ctx context.Context, db *gorm.DB) (*PolicyRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &PolicyRepository{db: db}, nil
}

// CreateAndBind stores a rule and its group binding in one transaction.
func (r *PolicyRepository) CreateAndBind(ctx context.Context, value model.Policy, groupID uint64, makeDefault bool) (model.Policy, error) {
	value = policyDefaults(value)
	if err := ctx.Err(); err != nil {
		return model.Policy{}, fmt.Errorf("create policy: %w", err)
	}
	if !validPolicy(value) {
		return model.Policy{}, fmt.Errorf("create policy: %w", model.ErrInvalidInput)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkRecordID(ctx, groupID); err != nil {
			return err
		}
		query := tx
		if tx.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var group model.Group
		if err := query.First(&group, "id = ?", groupID).Error; err != nil {
			return err
		}
		var backend model.Storage
		if err := tx.First(&backend, "id = ?", value.StorageID).Error; err != nil {
			return err
		}
		if err := tx.Create(&value).Error; err != nil {
			return err
		}
		if err := tx.Table("group_policies").Create(map[string]any{"group_id": groupID, "policy_id": value.ID}).Error; err != nil {
			return err
		}
		if makeDefault {
			return tx.Model(&model.Group{}).Where("id = ?", groupID).Update("default_policy_id", value.ID).Error
		}
		return nil
	})
	if err != nil {
		return model.Policy{}, repositoryError("create and bind policy", err)
	}
	return value, nil
}

// UploadPolicy resolves an enabled rule and backend, using the group default for policyID zero.
func (r *PolicyRepository) UploadPolicy(ctx context.Context, userID, policyID uint64) (model.Policy, model.Storage, model.Group, error) {
	var policy model.Policy
	var storage model.Storage
	var group model.Group
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if user.Status != model.UserStatusEnabled {
			return model.ErrForbidden
		}
		policy, storage, group, err = permittedPolicy(tx, user, policyID)
		return err
	})
	if err != nil {
		return model.Policy{}, model.Storage{}, model.Group{}, imageError("resolve upload policy", err)
	}
	return policy, storage, group, nil
}

// Find resolves an existing rule even when disabled, for previously uploaded image links.
func (r *PolicyRepository) Find(ctx context.Context, id uint64) (model.Policy, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.Policy{}, fmt.Errorf("find policy: %w", err)
	}
	var value model.Policy
	if err := r.db.WithContext(ctx).First(&value, "id = ?", id).Error; err != nil {
		return model.Policy{}, repositoryError("find policy", err)
	}
	return value, nil
}

func permittedPolicy(tx *gorm.DB, user model.User, policyID uint64) (model.Policy, model.Storage, model.Group, error) {
	var group model.Group
	if err := tx.First(&group, "id = ?", user.GroupID).Error; err != nil {
		return model.Policy{}, model.Storage{}, model.Group{}, err
	}
	if policyID == 0 {
		policyID = group.DefaultPolicyID
	}
	if policyID == 0 {
		return model.Policy{}, model.Storage{}, model.Group{}, model.ErrNotFound
	}
	var allowed int64
	if err := tx.Table("group_policies").Where("group_id = ? AND policy_id = ?", group.ID, policyID).Count(&allowed).Error; err != nil {
		return model.Policy{}, model.Storage{}, model.Group{}, err
	}
	if allowed != 1 {
		return model.Policy{}, model.Storage{}, model.Group{}, model.ErrForbidden
	}
	var policy model.Policy
	if err := tx.First(&policy, "id = ?", policyID).Error; err != nil {
		return model.Policy{}, model.Storage{}, model.Group{}, err
	}
	var storage model.Storage
	if err := tx.First(&storage, "id = ?", policy.StorageID).Error; err != nil {
		return model.Policy{}, model.Storage{}, model.Group{}, err
	}
	if !policy.Enabled || !storage.Enabled {
		return model.Policy{}, model.Storage{}, model.Group{}, model.ErrForbidden
	}
	return policy, storage, group, nil
}

func policyDefaults(value model.Policy) model.Policy {
	if value.NameTpl == "" {
		value.NameTpl = "{uniqid}"
	}
	if value.WebPMode == "" {
		value.WebPMode = "both"
	}
	if value.WebPQuality == 0 {
		value.WebPQuality = 80
	}
	if value.ThumbSize == 0 {
		value.ThumbSize = 400
	}
	if value.ScrubMode == "" {
		value.ScrubMode = "gps"
	}
	if value.LinkPrefer == "" {
		value.LinkPrefer = "webp"
	}
	if value.HEIFMode == "" {
		value.HEIFMode = "webp_only"
	}
	if value.OnConflict == "" {
		value.OnConflict = "rename"
	}
	return value
}

func validPolicy(value model.Policy) bool {
	if strings.TrimSpace(value.Name) == "" || value.StorageID == 0 {
		return false
	}
	validQuality := value.WebPQuality >= 1 && value.WebPQuality <= 100
	validEffort := value.WebPEffort >= 0 && value.WebPEffort <= 6
	validDimensions := value.MaxWidth >= 0 && value.MaxHeight >= 0 && value.ThumbSize > 0
	if !validQuality || !validEffort || !validDimensions {
		return false
	}
	checks := []struct {
		value   string
		allowed []string
	}{
		{value: value.WebPMode, allowed: []string{"both", "webp_only", "none"}},
		{value: value.ScrubMode, allowed: []string{"none", "gps", "all"}},
		{value: value.LinkPrefer, allowed: []string{"origin", "original", "webp"}},
		{value: value.HEIFMode, allowed: []string{"webp_only", "keep", "reject"}},
		{value: value.OnConflict, allowed: []string{"rename", "reject"}},
	}
	for _, check := range checks {
		found := false
		for _, candidate := range check.allowed {
			if check.value == candidate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
