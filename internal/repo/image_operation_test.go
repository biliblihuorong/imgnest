package repo

import (
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestImageTrashRestorePurgeLifecycle(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "lifecycle")
		image := reserveAndCommit(t, f, "lifecycle-image", "files/lifecycle")
		trash, err := f.images.BeginTrash(t.Context(), image.Key, "trash-op", f.grant, 7)
		if err != nil {
			t.Fatal(err)
		}
		if trash.State != model.ImageStateTrash || trash.DeletedAt == nil || trash.PurgeAt == nil || !trash.PurgeAt.Equal(f.grant.At.Add(7*24*time.Hour)) || usedBytes(t, f) != 0 {
			t.Fatal("trash transition did not release charge and retain deadlines")
		}
		if _, err := f.images.BeginTrash(t.Context(), image.Key, "trash-op", f.grant, 7); err != nil {
			t.Fatal(err)
		}
		if usedBytes(t, f) != 0 {
			t.Fatal("trash retry deducted twice")
		}
		if _, err := f.images.BeginRestore(t.Context(), image.Key, "restore-too-early", f.grant); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("restore bypassed unfinished move: %v", err)
		}
		if err := f.images.FinishTrash(t.Context(), image.Key, "trash-op"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("trash-collision", image.Path)); !errors.Is(err, model.ErrPathConflict) {
			t.Fatalf("trash lost unique path: %v", err)
		}
		if _, err := f.images.BeginRestore(t.Context(), image.Key, "restore-op", f.grant); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.FinishRestore(t.Context(), image.Key, "restore-op", f.grant); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.FinishRestore(t.Context(), image.Key, "restore-op", f.grant); err != nil {
			t.Fatal(err)
		}
		active, err := f.images.FindByKey(t.Context(), image.Key)
		if err != nil || active.State != model.ImageStateActive || active.Operation != model.ImageOperationRestoreCleanup || usedBytes(t, f) != 160 {
			t.Fatalf("restore finalization lost cleanup or duplicated charge: %v", err)
		}
		operations, err := f.images.PendingOperations(t.Context())
		if err != nil || len(operations) != 1 {
			t.Fatalf("restore cleanup lost from recovery: %v", err)
		}
		if err := f.images.CancelRestore(t.Context(), image.Key, "restore-op"); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("cancel removed activated restore: %v", err)
		}
		if err := f.images.FinishRestoreCleanup(t.Context(), image.Key, "restore-op"); err != nil {
			t.Fatal(err)
		}
		if err := f.images.SetPublic(t.Context(), image.Key, true, f.grant); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.BeginTrash(t.Context(), image.Key, "trash-again", f.grant, 0); err != nil {
			t.Fatal(err)
		}
		if err := f.images.FinishTrash(t.Context(), image.Key, "trash-again"); err != nil {
			t.Fatal(err)
		}
		due, err := f.images.DueTrash(t.Context(), f.grant.At, 10)
		if err != nil || len(due) != 1 {
			t.Fatalf("trash0 not due: %v", err)
		}
		if _, err := f.images.BeginSystemPurge(t.Context(), image.Key, "purge-op"); err != nil {
			t.Fatal(err)
		}
		if err := f.images.FinishPurge(t.Context(), image.Key, "wrong-purge"); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("stale purge finished current op: %v", err)
		}
		if err := f.images.FinishPurge(t.Context(), image.Key, "purge-op"); err != nil {
			t.Fatal(err)
		}
		if err := f.images.FinishPurge(t.Context(), image.Key, "purge-op"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.FindByKey(t.Context(), image.Key); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("purged row still present: %v", err)
		}
		var exifs int64
		if err := db.WithContext(t.Context()).Model(&model.ImageExif{}).Count(&exifs).Error; err != nil {
			t.Fatal(err)
		}
		if exifs != 0 {
			t.Fatal("purge left metadata behind")
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("after-purge", image.Path)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestImageRestoreQuotaAndActorScope(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "restorescope")
		image := reserveAndCommit(t, f, "scope-image", "files/scope")
		other, _, _ := grantFixture(t, db, "scope-other")
		foreign := testGrant(other.PasswordHash)
		foreign.UserID = other.ID
		if _, err := f.images.BeginTrash(t.Context(), image.Key, "foreign-op", foreign, 7); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign image trashed: %v", err)
		}
		if err := f.images.SetPublic(t.Context(), image.Key, true, foreign); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign visibility changed: %v", err)
		}
		if err := db.WithContext(t.Context()).Model(&model.User{}).Where("id = ?", other.ID).Update("role", model.UserRoleAdmin).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.BeginTrash(t.Context(), image.Key, "admin-trash", foreign, 7); err != nil {
			t.Fatal(err)
		}
		if err := f.images.FinishTrash(t.Context(), image.Key, "admin-trash"); err != nil {
			t.Fatal(err)
		}
		if usedBytes(t, f) != 0 {
			t.Fatal("admin trash changed the wrong account charge")
		}
		if err := db.WithContext(t.Context()).Model(&model.Group{}).Where("id = ?", f.user.GroupID).Update("capacity_bytes", 150).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.BeginRestore(t.Context(), image.Key, "no-quota", f.grant); !errors.Is(err, model.ErrQuotaExceeded) {
			t.Fatalf("restore exceeded capacity: %v", err)
		}
		if err := db.WithContext(t.Context()).Model(&model.Group{}).Where("id = ?", f.user.GroupID).Update("capacity_bytes", 1000).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.BeginRestore(t.Context(), image.Key, "reserved-restore", f.grant); err != nil {
			t.Fatal(err)
		}
		input := f.request("large-image", "files/large")
		setAmount(&input, 900)
		if _, err := f.images.ReserveUpload(t.Context(), input); !errors.Is(err, model.ErrQuotaExceeded) {
			t.Fatalf("restore reservation not counted: %v", err)
		}
		if err := f.images.CancelRestore(t.Context(), image.Key, "reserved-restore"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	})
}
