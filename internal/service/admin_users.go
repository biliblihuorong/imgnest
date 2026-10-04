package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// ListUsers pages every real account with an optional username/email keyword.
func (s *AdminService) ListUsers(ctx context.Context, page, size int, keyword string) (AdminUserPage, error) {
	if err := ctx.Err(); err != nil {
		return AdminUserPage{}, fmt.Errorf("list admin users: %w", err)
	}
	if page < 1 || size < 1 || size > 100 {
		return AdminUserPage{}, ErrInvalidInput
	}
	rows, total, err := s.deps.Users.ListUsers(ctx, keyword, page, size)
	if err != nil {
		return AdminUserPage{}, fmt.Errorf("list admin users: %w", err)
	}
	items := make([]UserView, 0, len(rows))
	for _, user := range rows {
		items = append(items, userView(user))
	}
	return AdminUserPage{Items: items, Total: total, Page: page, Size: size}, nil
}

// PatchUser changes an account's status or group. Disabling an account
// revokes all of its bearer credentials (the repository does this in the
// same transaction as the status flip, matching password-change semantics).
// Administrators cannot disable themselves, and there is no way to change a
// role through this API at all.
func (s *AdminService) PatchUser(ctx context.Context, callerID, targetID uint64, patch AdminUserPatch) (UserView, error) {
	if err := ctx.Err(); err != nil {
		return UserView{}, fmt.Errorf("patch admin user: %w", err)
	}
	if callerID == 0 || targetID == 0 {
		return UserView{}, ErrInvalidInput
	}
	status := ""
	if patch.Status != nil {
		status = *patch.Status
		if status != model.UserStatusEnabled && status != model.UserStatusDisabled {
			return UserView{}, ErrInvalidInput
		}
	}
	if patch.GroupID != nil {
		if *patch.GroupID == 0 {
			return UserView{}, ErrInvalidInput
		}
		group, err := s.deps.Groups.FindGroup(ctx, *patch.GroupID)
		if err != nil {
			return UserView{}, fmt.Errorf("find target group: %w", err)
		}
		// Real accounts never join the anonymous group; guest quotas are
		// governed by the guest settings instead.
		if group.IsGuest {
			return UserView{}, ErrInvalidInput
		}
	}
	if patch.Status != nil && status == model.UserStatusDisabled && callerID == targetID {
		return UserView{}, ErrForbidden
	}
	if patch.Status != nil {
		if err := s.deps.Users.SetUserStatus(ctx, targetID, status); err != nil {
			return UserView{}, fmt.Errorf("set user status: %w", err)
		}
	}
	if patch.GroupID != nil {
		if err := s.deps.Users.SetUserGroup(ctx, targetID, *patch.GroupID); err != nil {
			return UserView{}, fmt.Errorf("set user group: %w", err)
		}
	}
	user, err := s.deps.Users.FindUserByID(ctx, targetID)
	if err != nil {
		return UserView{}, fmt.Errorf("find patched user: %w", err)
	}
	return userView(user), nil
}

func (s *AdminService) groupView(group model.Group, counts map[uint64]int64, bindings map[uint64][]uint64) GroupView {
	ids := bindings[group.ID]
	if ids == nil {
		ids = []uint64{}
	}
	exts := group.AllowedExts
	if exts == nil {
		exts = []string{}
	}
	return GroupView{
		ID: group.ID, Name: group.Name, IsDefault: group.IsDefault, IsGuest: group.IsGuest,
		CapacityBytes: group.CapacityBytes, MaxFileBytes: group.MaxFileBytes, AllowedExts: exts,
		UploadPerMin: group.UploadPerMin, DefaultPolicyID: group.DefaultPolicyID,
		PolicyIDs: ids, UserCount: counts[group.ID],
	}
}

// ListGroups returns every group with member counts and rule bindings.
func (s *AdminService) ListGroups(ctx context.Context) ([]GroupView, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list admin groups: %w", err)
	}
	groups, err := s.deps.Groups.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admin groups: %w", err)
	}
	counts, err := s.deps.Groups.GroupMemberCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("count group members: %w", err)
	}
	bindings, err := s.deps.Groups.GroupPolicyIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list group policies: %w", err)
	}
	views := make([]GroupView, 0, len(groups))
	for _, group := range groups {
		views = append(views, s.groupView(group, counts, bindings))
	}
	return views, nil
}

func (s *AdminService) getGroupView(ctx context.Context, id uint64) (GroupView, error) {
	group, err := s.deps.Groups.FindGroup(ctx, id)
	if err != nil {
		return GroupView{}, fmt.Errorf("find group: %w", err)
	}
	counts, err := s.deps.Groups.GroupMemberCounts(ctx)
	if err != nil {
		return GroupView{}, fmt.Errorf("count group members: %w", err)
	}
	bindings, err := s.deps.Groups.GroupPolicyIDs(ctx)
	if err != nil {
		return GroupView{}, fmt.Errorf("list group policies: %w", err)
	}
	return s.groupView(group, counts, bindings), nil
}

// CreateGroup validates the quotas and rule bindings and stores the group
// with its bindings atomically. The default and guest roles are structural:
// there can be at most one of each, and they are fixed at creation.
func (s *AdminService) CreateGroup(ctx context.Context, input GroupInput) (GroupView, error) {
	if err := ctx.Err(); err != nil {
		return GroupView{}, fmt.Errorf("create group: %w", err)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || !utf8Valid(name, 64) || input.CapacityBytes < 0 || input.MaxFileBytes < 0 || input.UploadPerMin < 0 {
		return GroupView{}, ErrInvalidInput
	}
	exts, err := normalizeExts(input.AllowedExts)
	if err != nil {
		return GroupView{}, err
	}
	policyIDs, err := s.resolvePolicies(ctx, input.PolicyIDs)
	if err != nil {
		return GroupView{}, err
	}
	if input.DefaultPolicyID != 0 && !containsID(policyIDs, input.DefaultPolicyID) {
		return GroupView{}, ErrInvalidInput
	}
	if err := s.checkUniqueStructuralGroup(ctx, input.IsDefault, input.IsGuest); err != nil {
		return GroupView{}, err
	}
	group := model.Group{
		Name: name, CapacityBytes: input.CapacityBytes, MaxFileBytes: input.MaxFileBytes,
		AllowedExts: exts, UploadPerMin: input.UploadPerMin, DefaultPolicyID: input.DefaultPolicyID,
		IsDefault: input.IsDefault, IsGuest: input.IsGuest,
	}
	created, err := s.deps.Groups.CreateGroup(ctx, group, policyIDs)
	if err != nil {
		return GroupView{}, fmt.Errorf("create group: %w", err)
	}
	return s.getGroupView(ctx, created.ID)
}

// PatchGroup merges the supplied fields; the default/guest flags have no
// request shape and stay untouched.
func (s *AdminService) PatchGroup(ctx context.Context, id uint64, patch GroupPatch) (GroupView, error) {
	if err := ctx.Err(); err != nil {
		return GroupView{}, fmt.Errorf("patch group: %w", err)
	}
	if id == 0 {
		return GroupView{}, ErrInvalidInput
	}
	group, err := s.deps.Groups.FindGroup(ctx, id)
	if err != nil {
		return GroupView{}, fmt.Errorf("find group: %w", err)
	}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" || !utf8Valid(name, 64) {
			return GroupView{}, ErrInvalidInput
		}
		group.Name = name
	}
	if patch.CapacityBytes != nil {
		if *patch.CapacityBytes < 0 {
			return GroupView{}, ErrInvalidInput
		}
		group.CapacityBytes = *patch.CapacityBytes
	}
	if patch.MaxFileBytes != nil {
		if *patch.MaxFileBytes < 0 {
			return GroupView{}, ErrInvalidInput
		}
		group.MaxFileBytes = *patch.MaxFileBytes
	}
	if patch.UploadPerMin != nil {
		if *patch.UploadPerMin < 0 {
			return GroupView{}, ErrInvalidInput
		}
		group.UploadPerMin = *patch.UploadPerMin
	}
	if patch.AllowedExts != nil {
		exts, err := normalizeExts(*patch.AllowedExts)
		if err != nil {
			return GroupView{}, err
		}
		group.AllowedExts = exts
	}
	bindings, err := s.deps.Groups.GroupPolicyIDs(ctx)
	if err != nil {
		return GroupView{}, fmt.Errorf("list group policies: %w", err)
	}
	current := bindings[group.ID]
	replace := false
	if patch.PolicyIDs != nil {
		current, err = s.resolvePolicies(ctx, *patch.PolicyIDs)
		if err != nil {
			return GroupView{}, err
		}
		replace = true
	}
	if patch.DefaultPolicyID != nil {
		if *patch.DefaultPolicyID != 0 {
			if _, err := s.deps.Policies.Find(ctx, *patch.DefaultPolicyID); err != nil {
				return GroupView{}, fmt.Errorf("find default policy: %w", err)
			}
		}
		group.DefaultPolicyID = *patch.DefaultPolicyID
	}
	if group.DefaultPolicyID != 0 && !containsID(current, group.DefaultPolicyID) {
		return GroupView{}, ErrInvalidInput
	}
	updated, err := s.deps.Groups.UpdateGroup(ctx, group, current, replace)
	if err != nil {
		return GroupView{}, fmt.Errorf("update group: %w", err)
	}
	counts, err := s.deps.Groups.GroupMemberCounts(ctx)
	if err != nil {
		return GroupView{}, fmt.Errorf("count group members: %w", err)
	}
	return s.groupView(updated, counts, map[uint64][]uint64{updated.ID: current}), nil
}

// DeleteGroup refuses structural groups, groups with members, and groups a
// settings key still points at; deletion itself never cascades to accounts.
func (s *AdminService) DeleteGroup(ctx context.Context, id uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	group, err := s.deps.Groups.FindGroup(ctx, id)
	if err != nil {
		return fmt.Errorf("find group: %w", err)
	}
	if group.IsDefault || group.IsGuest {
		return ErrForbidden
	}
	counts, err := s.deps.Groups.GroupMemberCounts(ctx)
	if err != nil {
		return fmt.Errorf("count group members: %w", err)
	}
	if counts[group.ID] > 0 {
		return fmt.Errorf("delete group: %w", ErrGroupHasMembers)
	}
	references, err := s.deps.References.SettingGroupReferences(ctx, group.ID)
	if err != nil {
		return fmt.Errorf("read group references: %w", err)
	}
	if len(references) > 0 {
		return fmt.Errorf("delete group: %w", ErrStillReferenced)
	}
	if err := s.deps.Groups.DeleteGroup(ctx, group.ID); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	return nil
}

// resolvePolicies verifies every bound rule exists, preserving order and
// removing duplicates.
func (s *AdminService) resolvePolicies(ctx context.Context, requested []uint64) ([]uint64, error) {
	policyIDs := make([]uint64, 0, len(requested))
	for _, id := range requested {
		if id == 0 || containsID(policyIDs, id) {
			continue
		}
		if _, err := s.deps.Policies.Find(ctx, id); err != nil {
			return nil, fmt.Errorf("find bound policy: %w", err)
		}
		policyIDs = append(policyIDs, id)
	}
	return policyIDs, nil
}

func (s *AdminService) checkUniqueStructuralGroup(ctx context.Context, wantDefault, wantGuest bool) error {
	if !wantDefault && !wantGuest {
		return nil
	}
	groups, err := s.deps.Groups.ListGroups(ctx)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	for _, group := range groups {
		if (wantDefault && group.IsDefault) || (wantGuest && group.IsGuest) {
			return ErrInvalidInput
		}
	}
	return nil
}

// normalizeExts lowercases and validates the extension allowlist entries.
func normalizeExts(values []string) ([]string, error) {
	if len(values) > 64 {
		return nil, ErrInvalidInput
	}
	exts := make([]string, 0, len(values))
	for _, raw := range values {
		ext := strings.ToLower(strings.TrimSpace(raw))
		if ext == "" || len(ext) > 16 {
			return nil, ErrInvalidInput
		}
		for _, r := range ext {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				return nil, ErrInvalidInput
			}
		}
		exts = append(exts, ext)
	}
	return exts, nil
}

func utf8Valid(value string, maxRunes int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= maxRunes
}
