package service

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// LskyRepository supplies the v1 compatibility layer's settings gates and
// owner counters without exposing setting values.
type LskyRepository interface {
	APIEnabled(ctx context.Context) (bool, error)
	GuestUploadEnabled(ctx context.Context) (bool, error)
	GuestGroup(ctx context.Context) (model.Group, bool, error)
	RegisteredIP(ctx context.Context, userID uint64) (string, error)
	GroupCapacityBytes(ctx context.Context, groupID uint64) (int64, error)
	CountActiveImages(ctx context.Context, userID uint64) (int64, error)
}

// ProfileView carries one account's v1 profile in raw bytes; the HTTP layer
// converts capacities to kilobytes.
type ProfileView struct {
	Name         string
	Email        string
	Capacity     int64
	UsedCapacity int64
	ImageNum     int64
	AlbumNum     int64
	RegisteredIP string
}

// LskyDependencies wire the small reads the v1 layer needs beyond the shared
// upload, image, token and album services.
type LskyDependencies struct {
	Lsky     LskyRepository
	Albums   AlbumRepository
	Policies PolicyRepository
	Now      func() time.Time
}

// LskyService aggregates the settings gates, strategy lists and profile reads
// consumed by the v1 compatibility handlers.
type LskyService struct {
	lsky     LskyRepository
	albums   AlbumRepository
	policies PolicyRepository
	now      func() time.Time
}

// NewLskyService constructs the v1 support service.
func NewLskyService(ctx context.Context, deps LskyDependencies) (*LskyService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct lsky service: %w", err)
	}
	if deps.Lsky == nil || deps.Albums == nil || deps.Policies == nil || deps.Now == nil {
		return nil, fmt.Errorf("lsky service dependencies: %w", ErrInvalidInput)
	}
	return &LskyService{lsky: deps.Lsky, albums: deps.Albums, policies: deps.Policies, now: deps.Now}, nil
}

// APIEnabled reports whether the administrator allows the v1 API.
func (s *LskyService) APIEnabled(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("read api switch: %w", err)
	}
	enabled, err := s.lsky.APIEnabled(ctx)
	if err != nil {
		return false, fmt.Errorf("read api switch: %w", err)
	}
	return enabled, nil
}

// GuestUploadState reports whether anonymous uploads are allowed and which
// group governs them. ok is false when either check fails.
func (s *LskyService) GuestUploadState(ctx context.Context) (enabled bool, group model.Group, ok bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, model.Group{}, false, fmt.Errorf("read guest state: %w", err)
	}
	enabled, err = s.lsky.GuestUploadEnabled(ctx)
	if err != nil {
		return false, model.Group{}, false, fmt.Errorf("read guest state: %w", err)
	}
	if !enabled {
		return false, model.Group{}, false, nil
	}
	group, ok, err = s.lsky.GuestGroup(ctx)
	if err != nil {
		return false, model.Group{}, false, fmt.Errorf("read guest state: %w", err)
	}
	if !ok {
		return false, model.Group{}, false, nil
	}
	return true, group, true, nil
}

// Strategies lists the enabled rules one group may use, ordered by ID.
func (s *LskyService) Strategies(ctx context.Context, groupID uint64) ([]PolicySummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list strategies: %w", err)
	}
	rules, err := s.policies.GroupPolicies(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("list strategies: %w", err)
	}
	summaries := make([]PolicySummary, 0, len(rules))
	for _, rule := range rules {
		summaries = append(summaries, PolicySummary{ID: rule.ID, Name: rule.Name})
	}
	return summaries, nil
}

// Profile assembles the v1 profile of one authenticated account.
func (s *LskyService) Profile(ctx context.Context, user UserView) (ProfileView, error) {
	if err := ctx.Err(); err != nil {
		return ProfileView{}, fmt.Errorf("read profile: %w", err)
	}
	if user.ID == 0 {
		return ProfileView{}, ErrUnauthenticated
	}
	registered, err := s.lsky.RegisteredIP(ctx, user.ID)
	if err != nil {
		return ProfileView{}, fmt.Errorf("read profile: %w", err)
	}
	capacity, err := s.lsky.GroupCapacityBytes(ctx, user.GroupID)
	if err != nil {
		return ProfileView{}, fmt.Errorf("read profile: %w", err)
	}
	imageNum, err := s.lsky.CountActiveImages(ctx, user.ID)
	if err != nil {
		return ProfileView{}, fmt.Errorf("read profile: %w", err)
	}
	albumNum, err := s.albums.CountByOwner(ctx, user.ID)
	if err != nil {
		return ProfileView{}, fmt.Errorf("read profile: %w", err)
	}
	return ProfileView{
		Name: user.Username, Email: user.Email,
		Capacity: capacity, UsedCapacity: user.UsedBytes,
		ImageNum: imageNum, AlbumNum: albumNum, RegisteredIP: registered,
	}, nil
}
