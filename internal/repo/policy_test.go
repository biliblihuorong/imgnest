package repo

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestPolicyGroupDefaultAndPermissions(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		user, _, _ := grantFixture(t, db, "policyuser")
		storages, err := NewStorageRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		storage, err := storages.Create(t.Context(), model.Storage{
			Name: "rules", Driver: "local", BaseURL: "http://localhost/files", Enabled: true, Config: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		r, err := NewPolicyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		policy, err := r.CreateAndBind(t.Context(), testPolicy(storage.ID), user.GroupID, true)
		if err != nil {
			t.Fatal(err)
		}
		selected, backend, group, err := r.UploadPolicy(t.Context(), user.ID, 0)
		if err != nil || selected.ID != policy.ID || backend.ID != storage.ID || group.DefaultPolicyID != policy.ID {
			t.Fatalf("default policy unresolved: policy=%d default=%d error=%v", selected.ID, group.DefaultPolicyID, err)
		}
		unbound := testPolicy(storage.ID)
		unbound.Name = "unbound"
		if err := db.WithContext(t.Context()).Create(&unbound).Error; err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := r.UploadPolicy(t.Context(), user.ID, unbound.ID); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("unbound rule allowed: %v", err)
		}
		if err := db.WithContext(t.Context()).Model(&model.Policy{}).Where("id = ?", policy.ID).Update("enabled", false).Error; err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := r.UploadPolicy(t.Context(), user.ID, policy.ID); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("disabled rule allowed: %v", err)
		}
		found, err := r.Find(t.Context(), policy.ID)
		if err != nil || found.Enabled {
			t.Fatalf("existing disabled rule cannot resolve old links: %v", err)
		}
	})
}

func TestCreateAndBindRollsBackUnknownGroup(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		storages, err := NewStorageRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		storage, err := storages.Create(t.Context(), model.Storage{Name: "rollback", Driver: "local", Config: json.RawMessage(`{}`), Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		r, err := NewPolicyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.CreateAndBind(t.Context(), testPolicy(storage.ID), 99999, true); err == nil {
			t.Fatal("unknown group accepted")
		}
		var count int64
		if err := db.WithContext(t.Context()).Model(&model.Policy{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("failed binding left %d rules", count)
		}
	})
}

func testPolicy(storageID uint64) model.Policy {
	return model.Policy{
		StorageID: storageID, Name: "default", PathTpl: "{Y}/{m}/{d}", NameTpl: "{uniqid}",
		WebPMode: "both", WebPQuality: 80, WebPEffort: 4, ThumbSize: 400, ThumbEnabled: true,
		ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename",
		StripMeta: true, SkipIfLarger: true, Enabled: true,
	}
}

func TestPolicyInvalidModeCannotBorrowOtherFieldValidation(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "policymode")
		r, err := NewPolicyRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		policy := testPolicy(f.storage.ID)
		policy.WebPMode = "rename"
		if _, err := r.CreateAndBind(t.Context(), policy, f.user.GroupID, false); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("invalid mode error=%v", err)
		}
	})
}
