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

// lockAdminAccounts serializes admin membership changes across processes and
// shares BootstrapAdmin's lock. Locking only the target row would allow two
// administrators to remove each other concurrently. SQLite uses BEGIN IMMEDIATE.
func lockAdminAccounts(tx *gorm.DB) error {
	if tx.Name() == "postgres" {
		return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(current_schema()), 1229801282)").Error
	}
	return nil
}

func requireEnabledAdmin(tx *gorm.DB, callerID uint64) error {
	if callerID == 0 {
		return model.ErrForbidden
	}
	var caller model.User
	if err := tx.First(&caller, "id = ?", callerID).Error; err != nil {
		return err
	}
	if caller.Role != model.UserRoleAdmin || caller.Status != model.UserStatusEnabled {
		return model.ErrForbidden
	}
	return nil
}

func requireAccountGroup(tx *gorm.DB, groupID uint64) error {
	if groupID == 0 {
		return model.ErrInvalidInput
	}
	var group model.Group
	if err := tx.First(&group, "id = ?", groupID).Error; err != nil {
		return err
	}
	if group.IsGuest {
		return model.ErrInvalidInput
	}
	return nil
}

// CreateAdminUser creates an account independently of public registration.
// The actor and group are rechecked inside the transaction; uniqueness is
// enforced by the database rather than a racy preflight existence check.
func (r *AdminRepository) CreateAdminUser(ctx context.Context, callerID uint64, user model.User) (model.User, error) {
	user = model.User{
		Username: user.Username, Email: user.Email, PasswordHash: user.PasswordHash,
		DisplayName: user.DisplayName, Role: user.Role, Status: user.Status, GroupID: user.GroupID,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdminAccounts(tx); err != nil {
			return err
		}
		if err := requireEnabledAdmin(tx, callerID); err != nil {
			return err
		}
		if err := requireAccountGroup(tx, user.GroupID); err != nil {
			return err
		}
		return tx.Create(&user).Error
	})
	if err != nil {
		return model.User{}, userWriteError("create admin user", err)
	}
	return user, nil
}

// UpdateAdminUser applies all supplied fields and revokes affected credentials
// atomically. Actor checks, self-disable and last-admin protection run under
// the same membership lock, including concurrent role/status requests.
func (r *AdminRepository) UpdateAdminUser(ctx context.Context, callerID, userID uint64, patch model.UserChanges) (model.User, error) {
	if callerID == 0 {
		return model.User{}, fmt.Errorf("update admin user: %w", model.ErrForbidden)
	}
	return r.updateAdminUser(ctx, callerID, userID, patch)
}

func (r *AdminRepository) updateAdminUser(ctx context.Context, callerID, userID uint64, patch model.UserChanges) (model.User, error) {
	if userID == 0 {
		return model.User{}, fmt.Errorf("update admin user: %w", model.ErrInvalidInput)
	}
	var user model.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockAdminAccounts(tx); err != nil {
			return err
		}
		if callerID != 0 {
			if err := requireEnabledAdmin(tx, callerID); err != nil {
				return err
			}
		}
		var err error
		user, err = lockUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		next := user
		updates := map[string]any{}
		for _, field := range []struct {
			name   string
			value  *string
			target *string
		}{
			{"username", patch.Username, &next.Username}, {"email", patch.Email, &next.Email},
			{"display_name", patch.DisplayName, &next.DisplayName}, {"role", patch.Role, &next.Role},
			{"status", patch.Status, &next.Status},
		} {
			if field.value != nil {
				*field.target = *field.value
				updates[field.name] = *field.value
			}
		}
		if next.Role != model.UserRoleAdmin && next.Role != model.UserRoleUser {
			return model.ErrInvalidInput
		}
		if next.Status != model.UserStatusEnabled && next.Status != model.UserStatusDisabled {
			return model.ErrInvalidInput
		}
		if callerID == userID && next.Status == model.UserStatusDisabled {
			return model.ErrForbidden
		}
		if patch.GroupID != nil {
			if err := requireAccountGroup(tx, *patch.GroupID); err != nil {
				return err
			}
			next.GroupID = *patch.GroupID
			updates["group_id"] = *patch.GroupID
		}
		if user.Role == model.UserRoleAdmin && user.Status == model.UserStatusEnabled &&
			(next.Role != model.UserRoleAdmin || next.Status != model.UserStatusEnabled) {
			var remaining int64
			if err := tx.Model(&model.User{}).Where("id <> ? AND id <> 0 AND role = ? AND status = ?", userID, model.UserRoleAdmin, model.UserStatusEnabled).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				return model.ErrForbidden
			}
		}
		if len(updates) == 0 {
			return nil
		}
		securityChanged := next.Username != user.Username || next.Email != user.Email ||
			next.Role != user.Role || next.Status != user.Status || next.GroupID != user.GroupID
		if securityChanged {
			// A→B→A edits must not resurrect a proof verified before revocation.
			// Both databases store signed BIGINTs; fail closed at exhaustion.
			if user.AuthVersion >= math.MaxInt64 {
				return model.ErrInvalidInput
			}
			updates["auth_version"] = gorm.Expr("auth_version + 1")
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return err
		}
		if securityChanged {
			if err := tx.Where("user_id = ?", userID).Delete(&model.Token{}).Error; err != nil {
				return err
			}
		}
		return tx.First(&user, "id = ?", userID).Error
	})
	if err != nil {
		return model.User{}, userWriteError("update admin user", err)
	}
	return user, nil
}

// SetUserStatus is the internal lifecycle operation; it shares the atomic
// account mutation and last-enabled-administrator protections.
func (r *AdminRepository) SetUserStatus(ctx context.Context, userID uint64, status string) error {
	_, err := r.updateAdminUser(ctx, 0, userID, model.UserChanges{Status: &status})
	return err
}

// SetUserGroup moves an account and invalidates its previous credentials.
func (r *AdminRepository) SetUserGroup(ctx context.Context, userID, groupID uint64) error {
	_, err := r.updateAdminUser(ctx, 0, userID, model.UserChanges{GroupID: &groupID})
	return err
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
