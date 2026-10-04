package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
)

// ListPolicies returns every rule in database ID order, enabled or not.
func (s *AdminService) ListPolicies(ctx context.Context) ([]model.Policy, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list admin policies: %w", err)
	}
	rows, err := s.deps.Policies.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admin policies: %w", err)
	}
	if rows == nil {
		rows = []model.Policy{}
	}
	return rows, nil
}

// CreatePolicy builds a rule from the documented conservative defaults and
// the supplied overrides; rules are created unbound and become usable once a
// group binds them.
func (s *AdminService) CreatePolicy(ctx context.Context, patch PolicyPatch) (model.Policy, error) {
	if err := ctx.Err(); err != nil {
		return model.Policy{}, fmt.Errorf("create policy: %w", err)
	}
	if patch.Name == nil || patch.StorageID == nil || *patch.StorageID == 0 {
		return model.Policy{}, ErrInvalidInput
	}
	policy, err := DefaultPolicy(ctx, *patch.StorageID, *patch.Name)
	if err != nil {
		return model.Policy{}, err
	}
	if err := s.applyPolicyPatch(ctx, &policy, patch); err != nil {
		return model.Policy{}, err
	}
	if err := s.validatePolicy(ctx, &policy); err != nil {
		return model.Policy{}, err
	}
	created, err := s.deps.Policies.Create(ctx, policy)
	if err != nil {
		return model.Policy{}, fmt.Errorf("create policy: %w", err)
	}
	return created, nil
}

// PatchPolicy merges the supplied fields over the stored rule and validates
// the final state before persisting it.
func (s *AdminService) PatchPolicy(ctx context.Context, id uint64, patch PolicyPatch) (model.Policy, error) {
	if err := ctx.Err(); err != nil {
		return model.Policy{}, fmt.Errorf("patch policy: %w", err)
	}
	if id == 0 {
		return model.Policy{}, ErrInvalidInput
	}
	policy, err := s.deps.Policies.Find(ctx, id)
	if err != nil {
		return model.Policy{}, fmt.Errorf("find policy: %w", err)
	}
	if err := s.applyPolicyPatch(ctx, &policy, patch); err != nil {
		return model.Policy{}, err
	}
	if err := s.validatePolicy(ctx, &policy); err != nil {
		return model.Policy{}, err
	}
	updated, err := s.deps.Policies.Update(ctx, policy)
	if err != nil {
		return model.Policy{}, fmt.Errorf("update policy: %w", err)
	}
	return updated, nil
}

// DeletePolicy refuses rules that images or group defaults still reference;
// the database foreign keys guard the remaining races.
func (s *AdminService) DeletePolicy(ctx context.Context, id uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	if id == 0 {
		return ErrInvalidInput
	}
	images, err := s.deps.References.CountImagesForPolicy(ctx, id)
	if err != nil {
		return fmt.Errorf("count policy images: %w", err)
	}
	if images > 0 {
		return fmt.Errorf("delete policy: %w", ErrStillReferenced)
	}
	defaults, err := s.deps.References.CountGroupDefaultsForPolicy(ctx, id)
	if err != nil {
		return fmt.Errorf("count policy defaults: %w", err)
	}
	if defaults > 0 {
		return fmt.Errorf("delete policy: %w", ErrStillReferenced)
	}
	if err := s.deps.Policies.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}
	return nil
}

// PreviewPolicy renders one sample path with fixed example inputs: the
// current time, the caller's ID, "example.jpg", and fixed content digests.
// Template failures return an error whose message is a static template
// diagnostic, safe to show to administrators.
func (s *AdminService) PreviewPolicy(ctx context.Context, userID uint64, pathTpl, nameTpl string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("preview policy: %w", err)
	}
	result, err := pathtpl.Build(ctx, pathTpl, nameTpl, pathtpl.Variables{
		Time:     s.deps.Now().UTC(),
		UserID:   userID,
		Filename: "example.jpg",
		MD5:      "0123456789abcdef0123456789abcdef",
		SHA1:     "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		return "", err
	}
	return result.Path, nil
}

func (s *AdminService) applyPolicyPatch(ctx context.Context, policy *model.Policy, patch PolicyPatch) error {
	if patch.Name != nil {
		policy.Name = *patch.Name
	}
	if patch.StorageID != nil {
		policy.StorageID = *patch.StorageID
	}
	if patch.Enabled != nil {
		policy.Enabled = *patch.Enabled
	}
	if patch.PathTpl != nil {
		policy.PathTpl = *patch.PathTpl
	}
	if patch.NameTpl != nil {
		policy.NameTpl = *patch.NameTpl
	}
	if patch.WebPMode != nil {
		policy.WebPMode = *patch.WebPMode
	}
	if patch.WebPQuality != nil {
		policy.WebPQuality = *patch.WebPQuality
	}
	if patch.WebPEffort != nil {
		policy.WebPEffort = *patch.WebPEffort
	}
	if patch.WebPLossless != nil {
		policy.WebPLossless = *patch.WebPLossless
	}
	if patch.MaxWidth != nil {
		policy.MaxWidth = *patch.MaxWidth
	}
	if patch.MaxHeight != nil {
		policy.MaxHeight = *patch.MaxHeight
	}
	if patch.ThumbEnabled != nil {
		policy.ThumbEnabled = *patch.ThumbEnabled
	}
	if patch.ThumbSize != nil {
		policy.ThumbSize = *patch.ThumbSize
	}
	if patch.ScrubMode != nil {
		policy.ScrubMode = *patch.ScrubMode
	}
	if patch.HEIFMode != nil {
		policy.HEIFMode = *patch.HEIFMode
	}
	if patch.LinkPrefer != nil {
		policy.LinkPrefer = *patch.LinkPrefer
	}
	if patch.OnConflict != nil {
		policy.OnConflict = *patch.OnConflict
	}
	if patch.StripMeta != nil {
		policy.StripMeta = *patch.StripMeta
	}
	if patch.SkipIfLarger != nil {
		policy.SkipIfLarger = *patch.SkipIfLarger
	}
	return ctx.Err()
}

// validatePolicy enforces the same field rules as host provisioning: template
// placeholders, processing enums, numeric ranges, and an existing enabled
// storage backend.
func (s *AdminService) validatePolicy(ctx context.Context, policy *model.Policy) error {
	name := strings.TrimSpace(policy.Name)
	if name == "" || !utf8.ValidString(policy.Name) || utf8.RuneCountInString(policy.Name) > 64 {
		return ErrInvalidInput
	}
	if err := s.deps.Templates.Validate(ctx, policy.PathTpl, policy.NameTpl); err != nil {
		return ErrInvalidInput
	}
	switch policy.WebPMode {
	case "both", "webp_only", "none":
	default:
		return ErrInvalidInput
	}
	switch policy.ScrubMode {
	case "gps", "all", "none":
	default:
		return ErrInvalidInput
	}
	switch policy.LinkPrefer {
	case "webp", "original":
	default:
		return ErrInvalidInput
	}
	switch policy.HEIFMode {
	case "webp_only", "keep", "reject":
	default:
		return ErrInvalidInput
	}
	if policy.HEIFMode == "keep" && policy.ScrubMode != "none" {
		return ErrInvalidInput
	}
	switch policy.OnConflict {
	case "rename", "reject":
	default:
		return ErrInvalidInput
	}
	if policy.WebPQuality < 1 || policy.WebPQuality > 100 || policy.WebPEffort < 0 || policy.WebPEffort > 6 ||
		policy.MaxWidth < 0 || policy.MaxHeight < 0 || policy.MaxWidth > 100000000 || policy.MaxHeight > 100000000 ||
		policy.ThumbSize < 1 || policy.ThumbSize > 100000 {
		return ErrInvalidInput
	}
	backend, err := s.deps.Storages.Find(ctx, policy.StorageID)
	if err != nil {
		return fmt.Errorf("read policy storage: %w", err)
	}
	if !backend.Enabled {
		return ErrForbidden
	}
	return nil
}
