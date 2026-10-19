package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// OverviewRepository reads the site-wide aggregates of the console overview.
// Every query is a plain aggregate shared by SQLite and PostgreSQL; nothing
// here reads EXIF, credentials or storage configuration.
type OverviewRepository struct{ db *gorm.DB }

// NewOverviewRepository creates the overview persistence adapter.
func NewOverviewRepository(ctx context.Context, db *gorm.DB) (*OverviewRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &OverviewRepository{db: db}, nil
}

// objectBytesColumns splits charged cloud bytes of active images by object.
// The original is `{path}.{ext}`; a separate WebP exists only when the
// original is not itself the `{path}.webp` key (a WebP source or a
// WebP-only image stores one shared object, counted once); the cloud
// thumbnail always mirrors thumb_bytes. Charged bytes beyond these are the
// responsive variants.
const objectBytesColumns = `
	COALESCE(SUM(CASE WHEN i.state = 'active' THEN i.charged_bytes ELSE 0 END), 0) AS total,
	COALESCE(SUM(CASE WHEN i.state = 'active' AND i.has_original THEN i.size ELSE 0 END), 0) AS original,
	COALESCE(SUM(CASE WHEN i.state = 'active' AND i.has_webp AND (NOT i.has_original OR i.ext <> 'webp') THEN i.webp_size ELSE 0 END), 0) AS web_p,
	COALESCE(SUM(CASE WHEN i.state = 'active' AND i.has_thumb THEN i.thumb_bytes ELSE 0 END), 0) AS thumb`

// Overview gathers the console snapshot: account and album counts, usage
// per storage and per rule, daily uploads from since (a UTC day start),
// the top accounts by quota usage and responsive-variant coverage.
func (r *OverviewRepository) Overview(ctx context.Context, since time.Time, topUsers int) (model.Overview, error) {
	if err := ctx.Err(); err != nil {
		return model.Overview{}, fmt.Errorf("read overview: %w", err)
	}
	if topUsers < 1 || topUsers > 100 {
		return model.Overview{}, fmt.Errorf("read overview: %w", model.ErrInvalidInput)
	}
	var overview model.Overview
	// The aggregates run as separate reads, not one transaction: SQLite
	// transactions here begin IMMEDIATE and would block uploads for a
	// dashboard that tolerates a moment of skew between its sections.
	tx := r.db.WithContext(ctx)
	err := func() error {
		if err := tx.Raw("SELECT COUNT(*) FROM users WHERE id <> 0").Scan(&overview.Users).Error; err != nil {
			return err
		}
		if err := tx.Raw("SELECT COUNT(*) FROM albums").Scan(&overview.Albums).Error; err != nil {
			return err
		}
		var guest struct{ Images, Bytes int64 }
		if err := tx.Raw(
			"SELECT COUNT(*) AS images, COALESCE(SUM(charged_bytes), 0) AS bytes FROM images WHERE user_id = 0 AND state = ?",
			model.ImageStateActive,
		).Scan(&guest).Error; err != nil {
			return err
		}
		overview.GuestImages, overview.GuestBytes = guest.Images, guest.Bytes
		if err := tx.Raw("SELECT COUNT(*) FROM images WHERE state = ?", model.ImageStatePending).Scan(&overview.PendingImages).Error; err != nil {
			return err
		}
		overview.Storages = []model.StorageUsage{}
		if err := tx.Raw(`SELECT s.id AS storage_id, s.name, s.driver, s.enabled,
			COUNT(CASE WHEN i.state = 'active' THEN 1 END) AS images,` + objectBytesColumns + `,
			COUNT(CASE WHEN i.state = 'trash' THEN 1 END) AS trash_images,
			COALESCE(SUM(CASE WHEN i.state = 'trash' THEN i.charged_bytes ELSE 0 END), 0) AS trash_bytes
			FROM storages s LEFT JOIN images i ON i.storage_id = s.id
			GROUP BY s.id, s.name, s.driver, s.enabled ORDER BY s.id ASC`).Scan(&overview.Storages).Error; err != nil {
			return err
		}
		overview.Policies = []model.PolicyUsage{}
		if err := tx.Raw(`SELECT p.id AS policy_id, p.name, p.storage_id, p.enabled,
			COUNT(i.id) AS images, COALESCE(SUM(i.charged_bytes), 0) AS bytes
			FROM policies p LEFT JOIN images i ON i.policy_id = p.id AND i.state = ?
			GROUP BY p.id, p.name, p.storage_id, p.enabled ORDER BY p.id ASC`, model.ImageStateActive).Scan(&overview.Policies).Error; err != nil {
			return err
		}
		day := "date(created_at)"
		if tx.Name() == "postgres" {
			day = "to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')"
		}
		overview.Uploads = []model.UploadDay{}
		if err := tx.Raw(`SELECT `+day+` AS day, COUNT(*) AS uploads, COALESCE(SUM(charged_bytes), 0) AS bytes
			FROM images WHERE state IN (?, ?) AND created_at >= ?
			GROUP BY `+day+` ORDER BY day ASC`,
			model.ImageStateActive, model.ImageStateTrash, since.UTC()).Scan(&overview.Uploads).Error; err != nil {
			return err
		}
		overview.TopUsers = []model.UserUsage{}
		if err := tx.Raw(`SELECT u.id AS user_id, u.username, u.used_bytes, COUNT(i.id) AS images
			FROM users u LEFT JOIN images i ON i.user_id = u.id AND i.state = ?
			WHERE u.id <> 0
			GROUP BY u.id, u.username, u.used_bytes
			ORDER BY u.used_bytes DESC, images DESC, u.id ASC LIMIT ?`, model.ImageStateActive, topUsers).Scan(&overview.TopUsers).Error; err != nil {
			return err
		}
		var coverage struct{ Eligible, Covered int64 }
		if err := tx.Raw(`SELECT COUNT(*) AS eligible,
			COUNT(CASE WHEN i.variants <> '[]' THEN 1 END) AS covered
			FROM images i JOIN policies p ON p.id = i.policy_id
			WHERE i.state = ? AND i.frames = 1 AND p.responsive_variants = ?`,
			model.ImageStateActive, true).Scan(&coverage).Error; err != nil {
			return err
		}
		overview.VariantEligible, overview.VariantCovered = coverage.Eligible, coverage.Covered
		return nil
	}()
	if err != nil {
		return model.Overview{}, databaseError("read overview", err)
	}
	return overview, nil
}
