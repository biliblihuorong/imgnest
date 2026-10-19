package repo

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func commitReservation(t *testing.T, f imageFixture, input model.UploadReservation) model.Image {
	t.Helper()
	image, err := f.images.ReserveUpload(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range input.Objects {
		receipt.VersionID = "written-version"
		if err := f.images.RecordObjectReceipt(t.Context(), image.Key, image.OperationID, receipt); err != nil {
			t.Fatal(err)
		}
	}
	image, err = f.images.CommitUpload(t.Context(), image.Key, image.OperationID, model.ImageExif{Raw: json.RawMessage(`{}`)}, f.grant)
	if err != nil {
		t.Fatal(err)
	}
	return image
}

func TestOverviewAggregatesUsage(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "overview")
		at := func(day, hour int) time.Time { return time.Date(2026, 10, day, hour, 0, 0, 0, time.UTC) }

		// Original 100 + WebP 40 + cloud thumbnail 20; the thumb cache is free.
		plain := f.request("ov-plain", "2026/10/plain")
		plain.Image.CreatedAt = at(3, 10)
		commitReservation(t, f, plain)
		// The same plus two responsive variants of 4 and 8 bytes.
		variants := withVariants(f.request("ov-variants", "2026/10/variants"), 400, 800)
		variants.Image.CreatedAt = at(4, 23)
		commitReservation(t, f, variants)
		// A WebP source stores one shared {path}.webp object, counted once.
		webp := f.request("ov-webp", "2026/09/webp")
		webp.Image.Ext, webp.Image.MIME, webp.Image.Size, webp.Image.WebPSize = "webp", "image/webp", 50, 50
		webp.Image.CreatedAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		webp.Objects = []model.ObjectReceipt{
			{Key: "2026/09/webp.webp", Size: 50, OwnerID: "ov-webp", Location: model.ObjectLocationCloud, MIME: "image/webp"},
			{Key: "2026/09/webp_thumbs.webp", Size: 20, OwnerID: "ov-webp", Location: model.ObjectLocationCloud, MIME: "image/webp"},
			{Key: "2026/09/webp_thumbs.webp", Size: 20, OwnerID: "ov-webp", Location: model.ObjectLocationThumbCache, MIME: "image/webp"},
		}
		commitReservation(t, f, webp)
		// Trashed images count as uploads and as trash, not as active usage.
		trashed := f.request("ov-trash", "2026/10/trash")
		trashed.Image.CreatedAt = at(4, 1)
		commitReservation(t, f, trashed)
		if _, err := f.images.BeginTrash(t.Context(), "ov-trash", "ov-trash-op", f.grant, 7); err != nil {
			t.Fatal(err)
		}
		// A pending reservation is neither an upload nor usage.
		pending := f.request("ov-pending", "2026/10/pending")
		pending.Image.CreatedAt = at(4, 2)
		if _, err := f.images.ReserveUpload(t.Context(), pending); err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE policies SET responsive_variants = ? WHERE id = ?", true, f.policy.ID).Error; err != nil {
			t.Fatal(err)
		}

		overview, err := mustOverviewRepository(t, db).Overview(t.Context(), at(1, 0), 10)
		if err != nil {
			t.Fatal(err)
		}
		var users int64
		if err := db.Raw("SELECT COUNT(*) FROM users WHERE id <> 0").Scan(&users).Error; err != nil {
			t.Fatal(err)
		}
		if overview.Users != users || overview.Albums != 0 || overview.PendingImages != 1 || overview.GuestImages != 0 {
			t.Fatalf("counters = %+v, want %d users", overview, users)
		}
		var storage *model.StorageUsage
		for i := range overview.Storages {
			if overview.Storages[i].StorageID == f.storage.ID {
				storage = &overview.Storages[i]
			}
		}
		want := model.StorageUsage{
			StorageID: f.storage.ID, Name: "overview", Driver: "local", Enabled: true, Images: 3,
			Bytes:       model.ObjectBytes{Total: 160 + 172 + 70, Original: 250, WebP: 80, Thumb: 60},
			TrashImages: 1, TrashBytes: 160,
		}
		if storage == nil || *storage != want {
			t.Fatalf("storage usage = %+v, want %+v", storage, want)
		}
		if variantBytes := want.Bytes.Total - want.Bytes.Original - want.Bytes.WebP - want.Bytes.Thumb; variantBytes != 12 {
			t.Fatalf("variant remainder = %d, want 12", variantBytes)
		}
		var policy *model.PolicyUsage
		for i := range overview.Policies {
			if overview.Policies[i].PolicyID == f.policy.ID {
				policy = &overview.Policies[i]
			}
		}
		if policy == nil || policy.Images != 3 || policy.Bytes != 402 || policy.StorageID != f.storage.ID {
			t.Fatalf("policy usage = %+v", policy)
		}
		wantDays := []model.UploadDay{{Day: "2026-10-03", Uploads: 1, Bytes: 160}, {Day: "2026-10-04", Uploads: 2, Bytes: 332}}
		if len(overview.Uploads) != len(wantDays) || overview.Uploads[0] != wantDays[0] || overview.Uploads[1] != wantDays[1] {
			t.Fatalf("uploads = %+v, want %+v", overview.Uploads, wantDays)
		}
		if len(overview.TopUsers) == 0 || overview.TopUsers[0] != (model.UserUsage{UserID: f.user.ID, Username: "overview", UsedBytes: 402, Images: 3}) {
			t.Fatalf("top users = %+v", overview.TopUsers)
		}
		if overview.VariantEligible != 3 || overview.VariantCovered != 1 {
			t.Fatalf("variant coverage = %d/%d, want 1/3", overview.VariantCovered, overview.VariantEligible)
		}
	})
}

func TestOverviewEmptySiteAndLimits(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		repo := mustOverviewRepository(t, db)
		overview, err := repo.Overview(t.Context(), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), 10)
		if err != nil {
			t.Fatal(err)
		}
		if overview.Uploads == nil || overview.Storages == nil || overview.Policies == nil || overview.TopUsers == nil {
			t.Fatalf("empty lists must not be nil: %+v", overview)
		}
		if len(overview.Uploads) != 0 || overview.VariantEligible != 0 {
			t.Fatalf("empty site = %+v", overview)
		}
		if _, err := repo.Overview(t.Context(), time.Now(), 0); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("zero limit err = %v", err)
		}
	})
}

func mustOverviewRepository(t *testing.T, db *gorm.DB) *OverviewRepository {
	t.Helper()
	repo, err := NewOverviewRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}
