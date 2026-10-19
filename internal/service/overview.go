package service

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// OverviewRepository reads the site-wide aggregates behind the console
// overview.
type OverviewRepository interface {
	Overview(ctx context.Context, since time.Time, topUsers int) (model.Overview, error)
}

// OverviewDays is the length of the daily upload series.
const OverviewDays = 30

// overviewTopUsers is how many accounts the usage ranking lists.
const overviewTopUsers = 10

// OverviewTotals are site-wide counters. Bytes are charged cloud bytes
// (original, WebP, cloud thumbnail and responsive variants); the local
// preview cache is never counted.
type OverviewTotals struct {
	Users         int64 `json:"users"`
	Albums        int64 `json:"albums"`
	Images        int64 `json:"images"`
	Bytes         int64 `json:"bytes"`
	TrashImages   int64 `json:"trash_images"`
	TrashBytes    int64 `json:"trash_bytes"`
	PendingImages int64 `json:"pending_images"`
	GuestImages   int64 `json:"guest_images"`
	GuestBytes    int64 `json:"guest_bytes"`
}

// StorageUsageView is one backend's active usage split by object, plus what
// waits in its recycle bin. No configuration or credential is included.
type StorageUsageView struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	Driver        string `json:"driver"`
	Enabled       bool   `json:"enabled"`
	Images        int64  `json:"images"`
	Bytes         int64  `json:"bytes"`
	OriginalBytes int64  `json:"original_bytes"`
	WebPBytes     int64  `json:"webp_bytes"`
	ThumbBytes    int64  `json:"thumb_bytes"`
	VariantBytes  int64  `json:"variant_bytes"`
	TrashImages   int64  `json:"trash_images"`
	TrashBytes    int64  `json:"trash_bytes"`
}

// PolicyUsageView is one upload rule's active usage.
type PolicyUsageView struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	StorageID uint64 `json:"storage_id"`
	Enabled   bool   `json:"enabled"`
	Images    int64  `json:"images"`
	Bytes     int64  `json:"bytes"`
}

// UploadDayView is one UTC day of uploads, trashed images included.
type UploadDayView struct {
	Day     string `json:"day"`
	Uploads int64  `json:"uploads"`
	Bytes   int64  `json:"bytes"`
}

// UserUsageView is one account in the usage ranking.
type UserUsageView struct {
	ID        uint64 `json:"id"`
	Username  string `json:"username"`
	UsedBytes int64  `json:"used_bytes"`
	Images    int64  `json:"images"`
}

// VariantCoverageView counts active still images under rules with
// responsive variants, and those that already have them.
type VariantCoverageView struct {
	Eligible int64 `json:"eligible"`
	Covered  int64 `json:"covered"`
}

// AdminOverview is the console's site-wide dashboard.
type AdminOverview struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Totals      OverviewTotals      `json:"totals"`
	Storages    []StorageUsageView  `json:"storages"`
	Policies    []PolicyUsageView   `json:"policies"`
	Uploads     []UploadDayView     `json:"uploads"`
	TopUsers    []UserUsageView     `json:"top_users"`
	Variants    VariantCoverageView `json:"variants"`
}

// OverviewService assembles the administrator dashboard.
type OverviewService struct {
	repo OverviewRepository
	now  func() time.Time
}

// NewOverviewService constructs dashboard reads.
func NewOverviewService(ctx context.Context, repo OverviewRepository, now func() time.Time) (*OverviewService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct overview service: %w", err)
	}
	if repo == nil || now == nil {
		return nil, fmt.Errorf("overview service dependencies: %w", ErrInvalidInput)
	}
	return &OverviewService{repo: repo, now: now}, nil
}

// Overview returns the site-wide snapshot. Callers authorize the reader as
// an administrator.
func (s *OverviewService) Overview(ctx context.Context) (AdminOverview, error) {
	if err := ctx.Err(); err != nil {
		return AdminOverview{}, fmt.Errorf("read overview: %w", err)
	}
	now := s.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	since := today.AddDate(0, 0, -(OverviewDays - 1))
	data, err := s.repo.Overview(ctx, since, overviewTopUsers)
	if err != nil {
		return AdminOverview{}, fmt.Errorf("read overview: %w", err)
	}
	view := AdminOverview{
		GeneratedAt: now,
		Totals: OverviewTotals{
			Users: data.Users, Albums: data.Albums, PendingImages: data.PendingImages,
			GuestImages: data.GuestImages, GuestBytes: data.GuestBytes,
		},
		Storages: make([]StorageUsageView, 0, len(data.Storages)),
		Policies: make([]PolicyUsageView, 0, len(data.Policies)),
		Uploads:  make([]UploadDayView, 0, OverviewDays),
		TopUsers: make([]UserUsageView, 0, len(data.TopUsers)),
		Variants: VariantCoverageView{Eligible: data.VariantEligible, Covered: data.VariantCovered},
	}
	// Every image belongs to exactly one storage, so the storage rows add up
	// to the site totals.
	for _, usage := range data.Storages {
		bytes := usage.Bytes
		view.Storages = append(view.Storages, StorageUsageView{
			ID: usage.StorageID, Name: usage.Name, Driver: usage.Driver, Enabled: usage.Enabled,
			Images: usage.Images, Bytes: bytes.Total, OriginalBytes: bytes.Original, WebPBytes: bytes.WebP,
			ThumbBytes: bytes.Thumb, VariantBytes: max(bytes.Total-bytes.Original-bytes.WebP-bytes.Thumb, 0),
			TrashImages: usage.TrashImages, TrashBytes: usage.TrashBytes,
		})
		view.Totals.Images += usage.Images
		view.Totals.Bytes += bytes.Total
		view.Totals.TrashImages += usage.TrashImages
		view.Totals.TrashBytes += usage.TrashBytes
	}
	for _, usage := range data.Policies {
		view.Policies = append(view.Policies, PolicyUsageView{
			ID: usage.PolicyID, Name: usage.Name, StorageID: usage.StorageID, Enabled: usage.Enabled,
			Images: usage.Images, Bytes: usage.Bytes,
		})
	}
	days := make(map[string]model.UploadDay, len(data.Uploads))
	for _, day := range data.Uploads {
		days[day.Day] = day
	}
	for offset := range OverviewDays {
		day := since.AddDate(0, 0, offset).Format(time.DateOnly)
		recorded := days[day]
		view.Uploads = append(view.Uploads, UploadDayView{Day: day, Uploads: recorded.Uploads, Bytes: recorded.Bytes})
	}
	for _, user := range data.TopUsers {
		view.TopUsers = append(view.TopUsers, UserUsageView{ID: user.UserID, Username: user.Username, UsedBytes: user.UsedBytes, Images: user.Images})
	}
	return view, nil
}
