package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AdminRepository provides the management-console persistence queries for
// users, groups, cross-record reference checks, and site-wide image views.
type AdminRepository struct{ db *gorm.DB }

// NewAdminRepository creates the admin-console persistence adapter.
func NewAdminRepository(ctx context.Context, db *gorm.DB) (*AdminRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &AdminRepository{db: db}, nil
}

// FindUserByID fetches one manageable account by ID.
func (r *AdminRepository) FindUserByID(ctx context.Context, id uint64) (model.User, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.User{}, fmt.Errorf("find admin user: %w", err)
	}
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return model.User{}, repositoryError("find admin user", err)
	}
	return user, nil
}

// ListUsers pages every real account, optionally filtered by a keyword over
// username and email. The guest anchor row (id 0) is never a manageable
// account and stays out of every result.
func (r *AdminRepository) ListUsers(ctx context.Context, keyword string, page, size int) ([]model.User, int64, error) {
	if page < 1 || size < 1 || size > 100 || page-1 > math.MaxInt/size {
		return nil, 0, fmt.Errorf("list admin users: %w", model.ErrInvalidInput)
	}
	query := r.db.WithContext(ctx).Model(&model.User{}).Where("id <> 0")
	if strings.TrimSpace(keyword) != "" {
		pattern := "%" + escapeLike(strings.TrimSpace(keyword)) + "%"
		query = query.Where(
			"(username LIKE ? ESCAPE '\\' OR email LIKE ? ESCAPE '\\')",
			pattern, pattern,
		)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, repositoryError("count admin users", err)
	}
	users := []model.User{}
	if err := query.Order("id ASC").Limit(size).Offset((page - 1) * size).Find(&users).Error; err != nil {
		return nil, 0, repositoryError("list admin users", err)
	}
	return users, total, nil
}

// SetUserStatus changes an account's availability. Disabling an account
// revokes every bearer credential in the same transaction, matching the
// password-change semantics.
func (r *AdminRepository) SetUserStatus(ctx context.Context, userID uint64, status string) error {
	if status != model.UserStatusEnabled && status != model.UserStatusDisabled {
		return fmt.Errorf("set user status: %w", model.ErrInvalidInput)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Update("status", status).Error; err != nil {
			return err
		}
		if status != model.UserStatusDisabled {
			return nil
		}
		return tx.Where("user_id = ?", userID).Delete(&model.Token{}).Error
	})
	if err != nil {
		return repositoryError("set user status", err)
	}
	return nil
}

// SetUserGroup moves an account to an existing group.
func (r *AdminRepository) SetUserGroup(ctx context.Context, userID, groupID uint64) error {
	if groupID == 0 {
		return fmt.Errorf("set user group: %w", model.ErrInvalidInput)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockUser(ctx, tx, userID); err != nil {
			return err
		}
		var group model.Group
		if err := tx.First(&group, "id = ?", groupID).Error; err != nil {
			return err
		}
		return tx.Model(&model.User{}).Where("id = ?", userID).Update("group_id", groupID).Error
	})
	if err != nil {
		return repositoryError("set user group", err)
	}
	return nil
}

// FindGroup fetches one group by ID.
func (r *AdminRepository) FindGroup(ctx context.Context, id uint64) (model.Group, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.Group{}, fmt.Errorf("find group: %w", err)
	}
	var group model.Group
	if err := r.db.WithContext(ctx).First(&group, "id = ?", id).Error; err != nil {
		return model.Group{}, repositoryError("find group", err)
	}
	return group, nil
}

// ListGroups returns every group in database ID order.
func (r *AdminRepository) ListGroups(ctx context.Context) ([]model.Group, error) {
	groups := []model.Group{}
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, repositoryError("list groups", err)
	}
	return groups, nil
}

// GroupMemberCounts counts real accounts per group; the guest anchor row is
// not a member of anything.
func (r *AdminRepository) GroupMemberCounts(ctx context.Context) (map[uint64]int64, error) {
	var rows []struct {
		GroupID uint64
		Count   int64
	}
	err := r.db.WithContext(ctx).Table("users").
		Select("group_id, COUNT(*) AS count").
		Where("id <> 0").
		Group("group_id").
		Scan(&rows).Error
	if err != nil {
		return nil, databaseError("count group members", err)
	}
	counts := make(map[uint64]int64, len(rows))
	for _, row := range rows {
		counts[row.GroupID] = row.Count
	}
	return counts, nil
}

// GroupPolicyIDs maps every group to its bound rule IDs in rule-ID order.
func (r *AdminRepository) GroupPolicyIDs(ctx context.Context) (map[uint64][]uint64, error) {
	var rows []struct {
		GroupID  uint64
		PolicyID uint64
	}
	err := r.db.WithContext(ctx).Table("group_policies").
		Select("group_id, policy_id").
		Order("policy_id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, databaseError("list group policies", err)
	}
	bindings := make(map[uint64][]uint64, len(rows))
	for _, row := range rows {
		bindings[row.GroupID] = append(bindings[row.GroupID], row.PolicyID)
	}
	return bindings, nil
}

// CreateGroup stores a group and its rule bindings in one transaction. An
// unset default rule stays SQL NULL so the policies foreign key holds.
func (r *AdminRepository) CreateGroup(ctx context.Context, group model.Group, policyIDs []uint64) (model.Group, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The JSON serializer writes NULL for a nil slice; the column is NOT NULL.
		if group.AllowedExts == nil {
			group.AllowedExts = []string{}
		}
		create := tx
		if group.DefaultPolicyID == 0 {
			create = create.Omit("DefaultPolicyID")
		}
		if err := create.Create(&group).Error; err != nil {
			return err
		}
		return replaceGroupPolicies(tx, group.ID, policyIDs)
	})
	if err != nil {
		return model.Group{}, repositoryError("create group", err)
	}
	return group, nil
}

// UpdateGroup stores scalar changes and optionally replaces the rule
// bindings under a row lock.
func (r *AdminRepository) UpdateGroup(ctx context.Context, group model.Group, policyIDs []uint64, replace bool) (model.Group, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx
		if tx.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var existing model.Group
		if err := query.First(&existing, "id = ?", group.ID).Error; err != nil {
			return err
		}
		existing.Name = group.Name
		existing.CapacityBytes = group.CapacityBytes
		existing.MaxFileBytes = group.MaxFileBytes
		existing.AllowedExts = group.AllowedExts
		if existing.AllowedExts == nil {
			existing.AllowedExts = []string{}
		}
		existing.UploadPerMin = group.UploadPerMin
		save := tx
		if group.DefaultPolicyID == 0 {
			if err := tx.Model(&model.Group{}).Where("id = ?", group.ID).Update("default_policy_id", nil).Error; err != nil {
				return err
			}
			save = save.Omit("DefaultPolicyID")
		}
		existing.DefaultPolicyID = group.DefaultPolicyID
		if err := save.Save(&existing).Error; err != nil {
			return err
		}
		if !replace {
			return nil
		}
		return replaceGroupPolicies(tx, group.ID, policyIDs)
	})
	if err != nil {
		return model.Group{}, repositoryError("update group", err)
	}
	return group, nil
}

// DeleteGroup removes a group and its rule bindings. Member accounts and
// in-use settings references are rejected before this runs, and the users
// foreign key still guards against races.
func (r *AdminRepository) DeleteGroup(ctx context.Context, id uint64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkRecordID(ctx, id); err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Delete(&model.Group{}).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM group_policies WHERE group_id = ?", id).Error
	})
	if err != nil {
		return repositoryError("delete group", err)
	}
	return nil
}

func replaceGroupPolicies(tx *gorm.DB, groupID uint64, policyIDs []uint64) error {
	if err := tx.Exec("DELETE FROM group_policies WHERE group_id = ?", groupID).Error; err != nil {
		return err
	}
	for _, policyID := range policyIDs {
		if err := tx.Table("group_policies").Create(map[string]any{"group_id": groupID, "policy_id": policyID}).Error; err != nil {
			return err
		}
	}
	return nil
}

// CountPoliciesForStorage counts rules pointing at one backend.
func (r *AdminRepository) CountPoliciesForStorage(ctx context.Context, storageID uint64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Policy{}).Where("storage_id = ?", storageID).Count(&count).Error; err != nil {
		return 0, databaseError("count storage policies", err)
	}
	return count, nil
}

// CountImagesForPolicy counts images, active or recycled, bound to one rule.
func (r *AdminRepository) CountImagesForPolicy(ctx context.Context, policyID uint64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Image{}).Where("policy_id = ?", policyID).Count(&count).Error; err != nil {
		return 0, databaseError("count policy images", err)
	}
	return count, nil
}

// CountGroupDefaultsForPolicy counts groups whose default rule points at one rule.
func (r *AdminRepository) CountGroupDefaultsForPolicy(ctx context.Context, policyID uint64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Group{}).Where("default_policy_id = ?", policyID).Count(&count).Error; err != nil {
		return 0, databaseError("count policy defaults", err)
	}
	return count, nil
}

// SettingGroupReferences reports which settings keys still point at a group,
// so deletions cannot leave registration or guest routing dangling.
func (r *AdminRepository) SettingGroupReferences(ctx context.Context, groupID uint64) ([]string, error) {
	var settings []model.Setting
	if err := r.db.WithContext(ctx).Where("key IN ?", []string{"default_group_id", "guest_group_id"}).Find(&settings).Error; err != nil {
		return nil, databaseError("read group settings", err)
	}
	references := []string{}
	for _, setting := range settings {
		var value uint64
		if err := json.Unmarshal(setting.Value, &value); err != nil {
			continue
		}
		if value == groupID {
			references = append(references, setting.Key)
		}
	}
	return references, nil
}

// ListAdminPage and TrashKeys live on ImageRepository beside the other image
// queries; the admin service consumes them as an optional image capability.
