package repo

import (
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestImageSetNameUpdatesSearchAndKeepsKey(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "rename")
		image := reserveAndCommit(t, f, "rename-image", "files/rename")
		if err := f.images.SetName(t.Context(), image.Key, "Holiday Beach.JPG", f.grant); err != nil {
			t.Fatal(err)
		}
		renamed, err := f.images.FindByKey(t.Context(), image.Key)
		if err != nil {
			t.Fatal(err)
		}
		if renamed.OriginName != "Holiday Beach.JPG" || renamed.Path != image.Path || renamed.Ext != image.Ext {
			t.Fatalf("rename changed more than the name: %+v", renamed)
		}
		var search string
		if err := db.WithContext(t.Context()).Table("images").Select("filename_search").Where("id = ?", image.ID).Scan(&search).Error; err != nil {
			t.Fatal(err)
		}
		if search != "holiday beach.jpg" {
			t.Fatalf("search key = %q", search)
		}
		other, _, _ := grantFixture(t, db, "rename-other")
		foreign := testGrant(other.PasswordHash)
		foreign.UserID = other.ID
		if err := f.images.SetName(t.Context(), image.Key, "stolen", foreign); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign rename = %v", err)
		}
		if _, err := f.images.BeginTrash(t.Context(), image.Key, "trash-op", f.grant, 7); err != nil {
			t.Fatal(err)
		}
		if err := f.images.SetName(t.Context(), image.Key, "late", f.grant); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("trashed rename = %v", err)
		}
	})
}
