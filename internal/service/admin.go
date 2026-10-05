package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// AdminUserRepository provides the account reads and lifecycle mutations the
// management console needs.
type AdminUserRepository interface {
	FindUserByID(context.Context, uint64) (model.User, error)
	ListUsers(context.Context, string, int, int) ([]model.User, int64, error)
	SetUserStatus(context.Context, uint64, string) error
	SetUserGroup(context.Context, uint64, uint64) error
}

// AdminGroupRepository manages account groups, their rule bindings, and the
// per-group counters shown in the console.
type AdminGroupRepository interface {
	FindGroup(context.Context, uint64) (model.Group, error)
	ListGroups(context.Context) ([]model.Group, error)
	GroupMemberCounts(context.Context) (map[uint64]int64, error)
	GroupPolicyIDs(context.Context) (map[uint64][]uint64, error)
	CreateGroup(context.Context, model.Group, []uint64) (model.Group, error)
	UpdateGroup(context.Context, model.Group, []uint64, bool) (model.Group, error)
	DeleteGroup(context.Context, uint64) error
}

// AdminReferenceRepository counts cross-record references so deletions can
// refuse to leave dangling relations behind.
type AdminReferenceRepository interface {
	CountPoliciesForStorage(context.Context, uint64) (int64, error)
	CountImagesForPolicy(context.Context, uint64) (int64, error)
	CountGroupDefaultsForPolicy(context.Context, uint64) (int64, error)
	SettingGroupReferences(context.Context, uint64) ([]string, error)
}

// AdminStorageRepository manages backend records without ever returning
// their configuration to callers.
type AdminStorageRepository interface {
	Create(context.Context, model.Storage) (model.Storage, error)
	Find(context.Context, uint64) (model.Storage, error)
	List(context.Context) ([]model.Storage, error)
	Update(context.Context, model.Storage) (model.Storage, error)
	Delete(context.Context, uint64) error
}

// AdminPolicyRepository manages upload rules independently of group bindings.
type AdminPolicyRepository interface {
	Find(context.Context, uint64) (model.Policy, error)
	All(context.Context) ([]model.Policy, error)
	Create(context.Context, model.Policy) (model.Policy, error)
	Update(context.Context, model.Policy) (model.Policy, error)
	Delete(context.Context, uint64) error
}

// AdminSettingsRepository reads and upserts the raw JSON settings values.
type AdminSettingsRepository interface {
	ReadSettings(context.Context) (map[string]json.RawMessage, error)
	UpdateSettings(context.Context, map[string]json.RawMessage) error
	// AvatarConfig returns the site-wide avatar provider selection used to
	// decorate the user views the console returns.
	AvatarConfig(context.Context) (model.AvatarConfig, error)
}

// AdminDependencies contains only the capabilities the console needs.
type AdminDependencies struct {
	Users      AdminUserRepository
	Groups     AdminGroupRepository
	References AdminReferenceRepository
	Storages   AdminStorageRepository
	Policies   AdminPolicyRepository
	Settings   AdminSettingsRepository
	// Provision reuses the host setup path for storage creation so cloud
	// credentials are sealed and connectivity is probed exactly once.
	Provision *ProvisionService
	Drivers   StorageProvider
	Secrets   SecretCodec
	Templates TemplateValidator
	Now       func() time.Time
}

// AdminService implements the management console's business rules on top of
// the same persistence adapters the account and image services use.
type AdminService struct{ deps AdminDependencies }

// NewAdminService constructs the console operations with injected capabilities.
func NewAdminService(ctx context.Context, deps AdminDependencies) (*AdminService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct admin service: %w", err)
	}
	if deps.Users == nil || deps.Groups == nil || deps.References == nil || deps.Storages == nil ||
		deps.Policies == nil || deps.Settings == nil || deps.Provision == nil || deps.Drivers == nil ||
		deps.Secrets == nil || deps.Templates == nil || deps.Now == nil {
		return nil, fmt.Errorf("admin service dependencies: %w", ErrInvalidInput)
	}
	return &AdminService{deps: deps}, nil
}

// AdminUserPage is the console's user-list response.
type AdminUserPage struct {
	Items []UserView `json:"items"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

// AdminUserPatch carries the optional account changes; there is deliberately
// no role field, so role escalation has no request shape at all.
type AdminUserPatch struct {
	Status  *string `json:"status,omitempty"`
	GroupID *uint64 `json:"group_id,omitempty"`
}

// GroupView is the console's group entry with live counters.
type GroupView struct {
	ID              uint64   `json:"id"`
	Name            string   `json:"name"`
	IsDefault       bool     `json:"is_default"`
	IsGuest         bool     `json:"is_guest"`
	CapacityBytes   int64    `json:"capacity_bytes"`
	MaxFileBytes    int64    `json:"max_file_bytes"`
	AllowedExts     []string `json:"allowed_exts"`
	UploadPerMin    int      `json:"upload_per_min"`
	DefaultPolicyID uint64   `json:"default_policy_id"`
	PolicyIDs       []uint64 `json:"policy_ids"`
	UserCount       int64    `json:"user_count"`
}

// GroupInput creates a group; the structural flags are fixed at creation.
type GroupInput struct {
	Name            string   `json:"name"`
	CapacityBytes   int64    `json:"capacity_bytes"`
	MaxFileBytes    int64    `json:"max_file_bytes"`
	AllowedExts     []string `json:"allowed_exts"`
	UploadPerMin    int      `json:"upload_per_min"`
	DefaultPolicyID uint64   `json:"default_policy_id"`
	PolicyIDs       []uint64 `json:"policy_ids"`
	IsDefault       bool     `json:"is_default,omitempty"`
	IsGuest         bool     `json:"is_guest,omitempty"`
}

// GroupPatch carries the optional group changes; the default/guest flags are
// structural and cannot be changed after creation.
type GroupPatch struct {
	Name            *string   `json:"name,omitempty"`
	CapacityBytes   *int64    `json:"capacity_bytes,omitempty"`
	MaxFileBytes    *int64    `json:"max_file_bytes,omitempty"`
	AllowedExts     *[]string `json:"allowed_exts,omitempty"`
	UploadPerMin    *int      `json:"upload_per_min,omitempty"`
	DefaultPolicyID *uint64   `json:"default_policy_id,omitempty"`
	PolicyIDs       *[]uint64 `json:"policy_ids,omitempty"`
}

// StoragePatch changes a backend's display values, switch state, or
// configuration; a supplied config is re-encrypted before persistence and is
// never returned to any caller.
type StoragePatch struct {
	Name    *string         `json:"name,omitempty"`
	BaseURL *string         `json:"base_url,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
	Config  json.RawMessage `json:"config,omitempty"`
}

// StorageChecks reports each connectivity probe separately.
type StorageChecks struct {
	Put    bool `json:"put"`
	Copy   bool `json:"copy"`
	Delete bool `json:"delete"`
}

// StorageTestResult is the storage connection-test response.
type StorageTestResult struct {
	OK     bool          `json:"ok"`
	Checks StorageChecks `json:"checks"`
}

// PolicyPatch carries optional rule changes; pointer fields distinguish an
// unset field from a zero value.
type PolicyPatch struct {
	Name         *string `json:"name,omitempty"`
	StorageID    *uint64 `json:"storage_id,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
	PathTpl      *string `json:"path_tpl,omitempty"`
	NameTpl      *string `json:"name_tpl,omitempty"`
	WebPMode     *string `json:"webp_mode,omitempty"`
	WebPQuality  *int    `json:"webp_quality,omitempty"`
	WebPEffort   *int    `json:"webp_effort,omitempty"`
	WebPLossless *bool   `json:"webp_lossless,omitempty"`
	MaxWidth     *int    `json:"max_width,omitempty"`
	MaxHeight    *int    `json:"max_height,omitempty"`
	ThumbEnabled *bool   `json:"thumb_enabled,omitempty"`
	ThumbSize    *int    `json:"thumb_size,omitempty"`
	ScrubMode    *string `json:"scrub_mode,omitempty"`
	HEIFMode     *string `json:"heif_mode,omitempty"`
	LinkPrefer   *string `json:"link_prefer,omitempty"`
	OnConflict   *string `json:"on_conflict,omitempty"`
	StripMeta    *bool   `json:"strip_meta,omitempty"`
	SkipIfLarger *bool   `json:"skip_if_larger,omitempty"`
}

// AdminSettingsView is the console's complete site configuration.
type AdminSettingsView struct {
	SiteName            string `json:"site_name"`
	RegistrationEnabled bool   `json:"registration_enabled"`
	GuestUploadEnabled  bool   `json:"guest_upload_enabled"`
	GalleryEnabled      bool   `json:"gallery_enabled"`
	TrashDays           int    `json:"trash_days"`
	APIEnabled          bool   `json:"api_enabled"`
	GuestGroupID        uint64 `json:"guest_group_id"`
	DefaultGroupID      uint64 `json:"default_group_id"`
	// AvatarProvider is the site-wide external avatar source; users cannot
	// change it and only the two supported enums are ever stored or returned.
	AvatarProvider string `json:"avatar_provider"`
}

// SettingsPatch changes only the settings keys present in the request.
type SettingsPatch struct {
	SiteName            *string `json:"site_name,omitempty"`
	RegistrationEnabled *bool   `json:"registration_enabled,omitempty"`
	GuestUploadEnabled  *bool   `json:"guest_upload_enabled,omitempty"`
	GalleryEnabled      *bool   `json:"gallery_enabled,omitempty"`
	TrashDays           *int    `json:"trash_days,omitempty"`
	APIEnabled          *bool   `json:"api_enabled,omitempty"`
	GuestGroupID        *uint64 `json:"guest_group_id,omitempty"`
	DefaultGroupID      *uint64 `json:"default_group_id,omitempty"`
	AvatarProvider      *string `json:"avatar_provider,omitempty"`
}

func containsID(values []uint64, id uint64) bool {
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}
