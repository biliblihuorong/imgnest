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

// UploadGroup resolves the group bounding a user's uploads without requiring
// any of its rules to be usable, for the pre-read of a multipart body whose
// selected rule is not known yet.
func (r *PolicyRepository) UploadGroup(ctx context.Context, userID uint64) (model.Group, error) {
	var group model.Group
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		if user.Status != model.UserStatusEnabled {
			return model.ErrForbidden
		}
		return tx.First(&group, "id = ?", user.GroupID).Error
	})
	if err != nil {
		return model.Group{}, imageError("resolve upload group", err)
	}
	return group, nil
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

// GroupPolicies lists the enabled rules bound to a group and ordered by ID.
// A rule whose storage backend is disabled is excluded, matching the
// upload-time grant semantics of UploadPolicy; a group without bindings
// yields an empty, non-nil list.
func (r *PolicyRepository) GroupPolicies(ctx context.Context, groupID uint64) ([]model.Policy, error) {
	if err := checkRecordID(ctx, groupID); err != nil {
		return nil, fmt.Errorf("list group policies: %w", err)
	}
	policies := []model.Policy{}
	err := r.db.WithContext(ctx).Table("policies").
		Select("policies.*").
		Joins("JOIN group_policies ON group_policies.policy_id = policies.id").
		Joins("JOIN storages ON storages.id = policies.storage_id").
		Where("group_policies.group_id = ? AND policies.enabled = ? AND storages.enabled = ?", groupID, true, true).
		Order("policies.id ASC").
		Find(&policies).Error
	if err != nil {
		return nil, repositoryError("list group policies", err)
	}
	return policies, nil
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

// All lists every rule in database ID order, enabled or not, for the
// management console.
func (r *PolicyRepository) All(ctx context.Context) ([]model.Policy, error) {
	policies := []model.Policy{}
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&policies).Error; err != nil {
		return nil, repositoryError("list policies", err)
	}
	return policies, nil
}

// Create stores an unbound rule after applying provisioning defaults; group
// bindings are managed separately through the group endpoints.
func (r *PolicyRepository) Create(ctx context.Context, value model.Policy) (model.Policy, error) {
	value = policyDefaults(value)
	if err := ctx.Err(); err != nil {
		return model.Policy{}, fmt.Errorf("create policy: %w", err)
	}
	if !validPolicy(value) {
		return model.Policy{}, fmt.Errorf("create policy: %w", model.ErrInvalidInput)
	}
	if err := r.db.WithContext(ctx).Create(&value).Error; err != nil {
		return model.Policy{}, repositoryError("create policy", err)
	}
	return value, nil
}

// Update stores a full rule replacement after validation, preserving the
// creation timestamp.
func (r *PolicyRepository) Update(ctx context.Context, value model.Policy) (model.Policy, error) {
	value = policyDefaults(value)
	if err := ctx.Err(); err != nil {
		return model.Policy{}, fmt.Errorf("update policy: %w", err)
	}
	if err := checkRecordID(ctx, value.ID); err != nil {
		return model.Policy{}, fmt.Errorf("update policy: %w", err)
	}
	if !validPolicy(value) {
		return model.Policy{}, fmt.Errorf("update policy: %w", model.ErrInvalidInput)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Policy
		if err := tx.First(&existing, "id = ?", value.ID).Error; err != nil {
			return err
		}
		value.CreatedAt = existing.CreatedAt
		return tx.Save(&value).Error
	})
	if err != nil {
		return model.Policy{}, repositoryError("update policy", err)
	}
	return value, nil
}

// Delete removes a rule; image and group-default references are rejected by
// the caller and by the database foreign keys.
func (r *PolicyRepository) Delete(ctx context.Context, id uint64) error {
	if err := checkRecordID(ctx, id); err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	deleted := r.db.WithContext(ctx).Delete(&model.Policy{}, "id = ?", id)
	if deleted.Error != nil {
		return repositoryError("delete policy", deleted.Error)
	}
	if deleted.RowsAffected == 0 {
		return fmt.Errorf("delete policy: %w", model.ErrNotFound)
	}
	return nil
}
