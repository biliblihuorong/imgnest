package repo

import (
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// TestImageListNativeFilters exercises the native listing extensions beyond
// the album filter: keyword, explicit order, byte-size bounds, upload-time
// window and the EXIF make/model/lens match.
func TestImageListNativeFilters(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "imgfilter")
		// alpha.jpg 100B uploaded first, gamma.jpg 300B, beta.jpg 200B newest.
		_, seeded := seedV1Images(t, db, fixture)
		alpha, gamma, beta := seeded["v1-alpha"], seeded["v1-gamma"], seeded["v1-beta"]
		// Aging alpha anchors the time-window filter; the other rows keep the
		// real upload timestamp, which is strictly after fixedRepoNow.
		if err := db.Exec("UPDATE images SET created_at = ? WHERE id = ?", fixedRepoNow, alpha.ID).Error; err != nil {
			t.Fatal(err)
		}
		// CommitUpload already seeds an empty metadata row per image; fill
		// two of them instead of inserting duplicates.
		if err := db.Model(&model.ImageExif{}).Where("image_id = ?", alpha.ID).Updates(map[string]any{"make": "Canon", "model": "EOS R5", "lens": "RF 50mm"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&model.ImageExif{}).Where("image_id = ?", gamma.ID).Updates(map[string]any{"make": "Sony", "model": "A7 IV"}).Error; err != nil {
			t.Fatal(err)
		}
		list := func(filter model.ImageListFilter) ([]model.Image, int64, error) {
			filter.UserID = fixture.user.ID
			return fixture.images.List(t.Context(), filter, 1, 40)
		}
		names := func(rows []model.Image) []string {
			out := []string{}
			for _, row := range rows {
				out = append(out, row.OriginName)
			}
			return out
		}

		rows, total, err := list(model.ImageListFilter{Keyword: "alpha"})
		if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != alpha.ID {
			t.Fatalf("keyword name filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		// The stored path also matches even when the display name does not.
		rows, total, err = list(model.ImageListFilter{Keyword: "v1-gamma"})
		if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != gamma.ID {
			t.Fatalf("keyword path filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, _, err = list(model.ImageListFilter{Order: "largest"})
		if err != nil || len(rows) != 3 {
			t.Fatalf("largest order rows=%v err=%v", names(rows), err)
		}
		if rows[0].ID != gamma.ID || rows[1].ID != beta.ID || rows[2].ID != alpha.ID {
			t.Fatalf("largest order wrong sequence: %v", names(rows))
		}
		rows, _, err = list(model.ImageListFilter{Order: "smallest"})
		if err != nil || rows[0].ID != alpha.ID || rows[2].ID != gamma.ID {
			t.Fatalf("smallest order wrong sequence: %v err=%v", names(rows), err)
		}
		rows, _, err = list(model.ImageListFilter{Order: "oldest"})
		if err != nil || rows[0].ID != alpha.ID || rows[2].ID != beta.ID {
			t.Fatalf("oldest order wrong sequence: %v err=%v", names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{MinSize: 250})
		if err != nil || total != 1 || rows[0].ID != gamma.ID {
			t.Fatalf("min_size filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{MinSize: 150, MaxSize: 250})
		if err != nil || total != 1 || rows[0].ID != beta.ID {
			t.Fatalf("size range filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		// fixedRepoNow anchors alpha strictly in the past; a window ending
		// right after it keeps alpha, one starting after it drops alpha.
		rows, total, err = list(model.ImageListFilter{To: ptrTime(fixedRepoNow.Add(time.Minute))})
		if err != nil || total != 1 || rows[0].ID != alpha.ID {
			t.Fatalf("to-window filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{From: ptrTime(fixedRepoNow.Add(time.Minute))})
		if err != nil || total != 2 || len(rows) != 2 {
			t.Fatalf("from-window filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{Exif: "canon"})
		if err != nil || total != 1 || rows[0].ID != alpha.ID {
			t.Fatalf("exif make filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{Exif: "A7"})
		if err != nil || total != 1 || rows[0].ID != gamma.ID {
			t.Fatalf("exif model filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{Exif: "RF 50"})
		if err != nil || total != 1 || rows[0].ID != alpha.ID {
			t.Fatalf("exif lens filter total=%d rows=%v err=%v", total, names(rows), err)
		}
		if _, total, err = list(model.ImageListFilter{Exif: "Nikon"}); err != nil || total != 0 {
			t.Fatalf("exif miss total=%d err=%v", total, err)
		}
		// Keyword and EXIF narrow independently of each other.
		rows, total, err = list(model.ImageListFilter{Keyword: "v1", Exif: "sony"})
		if err != nil || total != 1 || rows[0].ID != gamma.ID {
			t.Fatalf("combined keyword+exif total=%d rows=%v err=%v", total, names(rows), err)
		}
		// The unified needle matches either surface: a display name, a stored
		// path, and camera metadata.
		rows, total, err = list(model.ImageListFilter{Q: "alpha"})
		if err != nil || total != 1 || rows[0].ID != alpha.ID {
			t.Fatalf("unified name hit total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{Q: "v1-beta"})
		if err != nil || total != 1 || rows[0].ID != beta.ID {
			t.Fatalf("unified path hit total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{Q: "eos"})
		if err != nil || total != 1 || rows[0].ID != alpha.ID {
			t.Fatalf("unified exif hit total=%d rows=%v err=%v", total, names(rows), err)
		}
		rows, total, err = list(model.ImageListFilter{Q: "v1"})
		if err != nil || total != 3 {
			t.Fatalf("unified prefix total=%d rows=%v err=%v", total, names(rows), err)
		}

		if _, _, err := list(model.ImageListFilter{MinSize: 300, MaxSize: 100}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("inverted size bounds = %v", err)
		}
		if _, _, err := list(model.ImageListFilter{From: ptrTime(fixedRepoNow), To: ptrTime(fixedRepoNow.Add(-time.Minute))}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("inverted time window = %v", err)
		}
	})
}

func ptrTime(value time.Time) *time.Time { return &value }
