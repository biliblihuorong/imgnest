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

func TestPolicyGroupListingVisibility(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		user, _, _ := grantFixture(t, db, "policylist")
		ctx := t.Context()
		storages, err := NewStorageRepository(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		enabledStorage, err := storages.Create(ctx, model.Storage{
			Name: "list-on", Driver: "local", BaseURL: "http://localhost/files", Enabled: true, Config: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		disabledStorage, err := storages.Create(ctx, model.Storage{
			Name: "list-off", Driver: "local", BaseURL: "http://localhost/files", Enabled: false, Config: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		r, err := NewPolicyRepository(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		alpha := testPolicy(enabledStorage.ID)
		alpha.Name = "alpha"
		first, err := r.CreateAndBind(ctx, alpha, user.GroupID, false)
		if err != nil {
			t.Fatal(err)
		}
		beta := testPolicy(enabledStorage.ID)
		beta.Name = "beta"
		second, err := r.CreateAndBind(ctx, beta, user.GroupID, false)
		if err != nil {
			t.Fatal(err)
		}
		onDisabled := testPolicy(disabledStorage.ID)
		onDisabled.Name = "on-disabled"
		if _, err := r.CreateAndBind(ctx, onDisabled, user.GroupID, false); err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(ctx).Model(&model.Policy{}).Where("id = ?", second.ID).Update("enabled", false).Error; err != nil {
			t.Fatal(err)
		}
		unbound := testPolicy(enabledStorage.ID)
		unbound.Name = "unbound"
		if err := db.WithContext(ctx).Create(&unbound).Error; err != nil {
			t.Fatal(err)
		}
		other := model.Group{Name: "OtherGroup", AllowedExts: []string{}}
		if err := db.WithContext(ctx).Create(&other).Error; err != nil {
			t.Fatal(err)
		}
		foreign := testPolicy(enabledStorage.ID)
		foreign.Name = "foreign"
		if _, err := r.CreateAndBind(ctx, foreign, other.ID, false); err != nil {
			t.Fatal(err)
		}
		listed, err := r.GroupPolicies(ctx, user.GroupID)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || listed[0].ID != first.ID || listed[0].Name != "alpha" {
			t.Fatalf("listing exposed disabled, unbound, disabled-storage or foreign rules: %+v", listed)
		}
		empty := model.Group{Name: "EmptyGroup", AllowedExts: []string{}}
		if err := db.WithContext(ctx).Create(&empty).Error; err != nil {
			t.Fatal(err)
		}
		none, err := r.GroupPolicies(ctx, empty.ID)
		if err != nil {
			t.Fatal(err)
		}
		if none == nil || len(none) != 0 {
			t.Fatalf("empty group listing = %#v", none)
		}
		gamma := testPolicy(enabledStorage.ID)
		gamma.Name = "gamma"
		third, err := r.CreateAndBind(ctx, gamma, user.GroupID, false)
		if err != nil {
			t.Fatal(err)
		}
		listed, err = r.GroupPolicies(ctx, user.GroupID)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 2 || listed[0].ID != first.ID || listed[1].ID != third.ID || listed[0].ID > listed[1].ID {
			t.Fatalf("listing is not ascending by id: %+v", listed)
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
