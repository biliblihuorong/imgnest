package repo

import (
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// seedV1Images uploads three active images plus one trashed one:
//
//	alpha.jpg 100 bytes assigned to the album   (uploaded first)
//	gamma.jpg 300 bytes public, unassigned
//	beta.jpg  200 bytes private, unassigned     (uploaded last, so newest)
func seedV1Images(t *testing.T, db *gorm.DB, fixture imageFixture) (model.Album, map[string]model.Image) {
	t.Helper()
	album := model.Album{UserID: fixture.user.ID, Name: "博客"}
	if err := db.Create(&album).Error; err != nil {
		t.Fatal(err)
	}
	uploaded := map[string]model.Image{}
	for _, spec := range []struct {
		key, path, name string
		size            int64
		public          bool
		album           uint64
	}{
		{"v1-alpha", "2026/01/v1-alpha", "alpha.jpg", 100, true, album.ID},
		{"v1-gamma", "2026/01/v1-gamma", "gamma.jpg", 300, true, 0},
		{"v1-beta", "2026/01/v1-beta", "beta.jpg", 200, false, 0},
	} {
		reservation := fixture.request(spec.key, spec.path)
		reservation.Image.OriginName = spec.name
		reservation.Image.Size = spec.size
		reservation.Image.IsPublic = spec.public
		reservation.Image.AlbumID = spec.album
		image, err := fixture.images.ReserveUpload(t.Context(), reservation)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.images.CommitUpload(t.Context(), image.Key, image.OperationID, model.ImageExif{}, fixture.grant); err != nil {
			t.Fatal(err)
		}
		uploaded[spec.key] = image
	}
	trashed := fixture.request("v1-trash", "2026/01/v1-trash")
	image, err := fixture.images.ReserveUpload(t.Context(), trashed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.images.CommitUpload(t.Context(), image.Key, image.OperationID, model.ImageExif{}, fixture.grant); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE images SET state = ?, deleted_at = ? WHERE id = ?", model.ImageStateTrash, fixedRepoNow, image.ID).Error; err != nil {
		t.Fatal(err)
	}
	return album, uploaded
}

func keys(images []model.Image) []string {
	names := make([]string, 0, len(images))
	for _, image := range images {
		names = append(names, image.Key)
	}
	return names
}

func sameKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestListV1FiltersAndOrders(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		fixture := newImageFixture(t, db, "v1-list")
		album, _ := seedV1Images(t, db, fixture)

		// The Lsky quirk: a missing/zero album id selects unassigned images
		// only, newest first.
		unassigned, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "all", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 || !sameKeys(keys(unassigned), []string{"v1-beta", "v1-gamma"}) {
			t.Fatalf("unassigned page = %v total=%d, want [v1-beta v1-gamma]", keys(unassigned), total)
		}

		inAlbum, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "all", "", album.ID)
		if err != nil || total != 1 || !sameKeys(keys(inAlbum), []string{"v1-alpha"}) {
			t.Fatalf("album page = %v total=%d err=%v", keys(inAlbum), total, err)
		}

		utmost, _, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "utmost", "all", "", 0)
		if err != nil || !sameKeys(keys(utmost), []string{"v1-gamma", "v1-beta"}) {
			t.Fatalf("utmost order = %v err=%v, want the 300-byte image first", keys(utmost), err)
		}
		least, _, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "least", "all", "", 0)
		if err != nil || !sameKeys(keys(least), []string{"v1-beta", "v1-gamma"}) {
			t.Fatalf("least order = %v err=%v", keys(least), err)
		}
		earliest, _, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "earliest", "all", "", 0)
		if err != nil || !sameKeys(keys(earliest), []string{"v1-gamma", "v1-beta"}) {
			t.Fatalf("earliest order = %v err=%v", keys(earliest), err)
		}

		public, _, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "public", "", 0)
		if err != nil || !sameKeys(keys(public), []string{"v1-gamma"}) {
			t.Fatalf("public filter = %v err=%v", keys(public), err)
		}
		private, _, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "private", "", 0)
		if err != nil || !sameKeys(keys(private), []string{"v1-beta"}) {
			t.Fatalf("private filter = %v err=%v", keys(private), err)
		}

		byName, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "all", "beta", 0)
		if err != nil || total != 1 || !sameKeys(keys(byName), []string{"v1-beta"}) {
			t.Fatalf("keyword name filter = %v total=%d err=%v", keys(byName), total, err)
		}
		upper, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "all", "BETA", 0)
		if err != nil || total != 1 || !sameKeys(keys(upper), []string{"v1-beta"}) {
			t.Fatalf("keyword is case-sensitive: %v total=%d err=%v", keys(upper), total, err)
		}
		byPath, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "all", "v1-gamma", 0)
		if err != nil || total != 1 || !sameKeys(keys(byPath), []string{"v1-gamma"}) {
			t.Fatalf("keyword path filter = %v total=%d err=%v", keys(byPath), total, err)
		}
		// Keyword matching is a literal substring match; wildcards are escaped.
		if literal, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 1, 40, "newest", "all", "%", 0); err != nil || total != 0 || len(literal) != 0 {
			t.Fatalf("LIKE wildcard leaked: %v total=%d err=%v", keys(literal), total, err)
		}

		// Other owners' images never appear.
		if _, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID+1, 1, 40, "newest", "all", "", 0); err != nil || total != 0 {
			t.Fatalf("foreign owner saw images (total=%d err=%v)", total, err)
		}

		// Pagination bounds.
		page, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 2, 1, "newest", "all", "", 0)
		if err != nil || total != 2 || len(page) != 1 || page[0].Key != "v1-gamma" {
			t.Fatalf("second page = %v total=%d err=%v", keys(page), total, err)
		}
		empty, total, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 3, 1, "newest", "all", "", 0)
		if err != nil || total != 2 || len(empty) != 0 {
			t.Fatalf("past-end page = %v total=%d err=%v", keys(empty), total, err)
		}
		if _, _, err := fixture.images.ListV1(t.Context(), fixture.user.ID, 0, 40, "", "", "", 0); err == nil {
			t.Fatal("page 0 accepted")
		}
	})
}
