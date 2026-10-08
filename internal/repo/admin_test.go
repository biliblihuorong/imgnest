package repo

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func newAdminRepositories(t *testing.T, db *gorm.DB) (*AdminRepository, *UserRepository, *SettingsRepository, *PolicyRepository, *StorageRepository, *ImageRepository, *TokenRepository) {
	t.Helper()
	admin, err := NewAdminRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	users, err := NewUserRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := NewSettingsRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := NewPolicyRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	storages, err := NewStorageRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	images, err := NewImageRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := NewTokenRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	return admin, users, settings, policies, storages, images, tokens
}

func seedAdminUser(t *testing.T, db *gorm.DB, username, email string, groupID uint64) model.User {
	t.Helper()
	users, err := NewUserRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(t.Context(), model.User{
		Username: username, Email: email, PasswordHash: "password-digest",
		Role: model.UserRoleUser, Status: model.UserStatusEnabled, GroupID: groupID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func TestListUsersFiltersKeywordAndPages(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, _, _, _, _, _ := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		alice := seedAdminUser(t, db, "alice", "alice@example.com", defaultGroup.ID)
		bob := seedAdminUser(t, db, "bobby", "bob@imgnest.io", defaultGroup.ID)

		users, total, err := admin.ListUsers(t.Context(), "", 1, 20)
		if err != nil {
			t.Fatal(err)
		}
		// The migration's guest anchor row (id 0) is never a manageable account.
		if total != 2 || len(users) != 2 || users[0].ID != alice.ID || users[1].ID != bob.ID {
			t.Fatalf("list users total=%d rows=%+v", total, users)
		}
		for _, user := range users {
			if user.ID == 0 {
				t.Fatal("guest anchor leaked into admin user list")
			}
		}

		users, total, err = admin.ListUsers(t.Context(), "bob", 1, 20)
		if err != nil || total != 1 || len(users) != 1 || users[0].Username != "bobby" {
			t.Fatalf("username keyword total=%d rows=%+v err=%v", total, users, err)
		}
		users, total, err = admin.ListUsers(t.Context(), "imgnest.io", 1, 20)
		if err != nil || total != 1 || len(users) != 1 || users[0].Email != "bob@imgnest.io" {
			t.Fatalf("email keyword total=%d rows=%+v err=%v", total, users, err)
		}
		_, total, err = admin.ListUsers(t.Context(), "%", 1, 20)
		if err != nil || total != 0 {
			t.Fatalf("LIKE wildcard matched literally unescaped input: total=%d err=%v", total, err)
		}
		users, total, err = admin.ListUsers(t.Context(), "", 2, 1)
		if err != nil || total != 2 || len(users) != 1 || users[0].ID != bob.ID {
			t.Fatalf("second page total=%d rows=%+v err=%v", total, users, err)
		}
	})
}

func TestSetUserStatusRevokesTokens(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, users, _, _, _, _, tokens := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		user := seedAdminUser(t, db, "alice", "alice@example.com", defaultGroup.ID)
		if _, err := tokens.CreateToken(t.Context(), model.Token{
			UserID: user.ID, Name: "web", Kind: "web", TokenHash: "digest-a", Abilities: []string{"*"},
		}, model.TokenGrant{ExpectedPasswordHash: user.PasswordHash, At: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
		if err := admin.SetUserStatus(t.Context(), user.ID, model.UserStatusDisabled); err != nil {
			t.Fatal(err)
		}
		stored, err := users.FindUserByID(t.Context(), user.ID)
		if err != nil || stored.Status != model.UserStatusDisabled {
			t.Fatalf("status=%q err=%v", stored.Status, err)
		}
		remaining, err := tokens.ListTokens(t.Context(), user.ID)
		if err != nil || len(remaining) != 0 {
			t.Fatalf("disable kept tokens: count=%d err=%v", len(remaining), err)
		}
		if err := admin.SetUserStatus(t.Context(), user.ID, model.UserStatusEnabled); err != nil {
			t.Fatal(err)
		}
		if stored, _ = users.FindUserByID(t.Context(), user.ID); stored.Status != model.UserStatusEnabled {
			t.Fatalf("re-enable failed: %+v", stored)
		}
		if err := admin.SetUserStatus(t.Context(), user.ID, "suspended"); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("invalid status accepted: %v", err)
		}
		if err := admin.SetUserStatus(t.Context(), 9999, model.UserStatusDisabled); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing user status error=%v", err)
		}
	})
}

func TestSetUserGroupValidatesTarget(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, _, _, _, _, _ := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		other, err := admin.CreateGroup(t.Context(), model.Group{Name: "Team", CapacityBytes: 10}, nil)
		if err != nil {
			t.Fatal(err)
		}
		user := seedAdminUser(t, db, "alice", "alice@example.com", defaultGroup.ID)
		if err := admin.SetUserGroup(t.Context(), user.ID, other.ID); err != nil {
			t.Fatal(err)
		}
		stored, err := admin.FindUserByID(t.Context(), user.ID)
		if err != nil || stored.GroupID != other.ID {
			t.Fatalf("group move failed: %+v err=%v", stored, err)
		}
		if err := admin.SetUserGroup(t.Context(), user.ID, 9999); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing group accepted: %v", err)
		}
		if err := admin.SetUserGroup(t.Context(), user.ID, 0); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("zero group accepted: %v", err)
		}
	})
}

func TestCreateUserPersistsRegisteredIP(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, users, _, _, _, _, _ := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		user, err := users.CreateUser(t.Context(), model.User{
			Username: "alice", Email: "alice@example.com", PasswordHash: "password-digest",
			Role: model.UserRoleUser, Status: model.UserStatusEnabled, GroupID: defaultGroup.ID,
			RegisteredIP: "192.0.2.7",
		})
		if err != nil {
			t.Fatal(err)
		}
		stored, err := users.FindUserByID(t.Context(), user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.RegisteredIP != "192.0.2.7" {
			t.Fatalf("registered_ip=%q", stored.RegisteredIP)
		}
	})
}

func TestGroupCrudWithBindingsAndCounts(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, _, policies, storages, _, _ := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		seedAdminUser(t, db, "alice", "alice@example.com", defaultGroup.ID)
		backend, err := storages.Create(t.Context(), model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/1", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		alpha, err := policies.CreateAndBind(t.Context(), model.Policy{Name: "alpha", StorageID: backend.ID, PathTpl: "{Y}", NameTpl: "{uniqid}", WebPMode: "both", ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename", WebPQuality: 80, WebPEffort: 4, ThumbSize: 400, Enabled: true}, defaultGroup.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		beta, err := policies.Create(t.Context(), model.Policy{Name: "beta", StorageID: backend.ID, PathTpl: "{Y}", NameTpl: "{uniqid}", WebPMode: "both", ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename", WebPQuality: 80, WebPEffort: 4, ThumbSize: 400, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}

		team, err := admin.CreateGroup(t.Context(), model.Group{
			Name: "Team", CapacityBytes: 1024, MaxFileBytes: 128, AllowedExts: []string{"png"},
			UploadPerMin: 5, DefaultPolicyID: alpha.ID,
		}, []uint64{alpha.ID, beta.ID})
		if err != nil {
			t.Fatal(err)
		}
		if team.ID == 0 {
			t.Fatal("created group lacks ID")
		}

		counts, err := admin.GroupMemberCounts(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if counts[defaultGroup.ID] != 1 || counts[team.ID] != 0 {
			t.Fatalf("member counts=%+v", counts)
		}
		if _, ok := counts[0]; ok {
			t.Fatal("guest anchor counted as a group member")
		}
		bindings, err := admin.GroupPolicyIDs(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(bindings[team.ID]) != 2 || bindings[team.ID][0] != alpha.ID || bindings[team.ID][1] != beta.ID {
			t.Fatalf("policy bindings=%+v", bindings[team.ID])
		}

		team.Name = "Renamed"
		team.CapacityBytes = 2048
		updated, err := admin.UpdateGroup(t.Context(), team, []uint64{beta.ID}, true)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Name != "Renamed" || updated.CapacityBytes != 2048 {
			t.Fatalf("updated group=%+v", updated)
		}
		bindings, _ = admin.GroupPolicyIDs(t.Context())
		if len(bindings[team.ID]) != 1 || bindings[team.ID][0] != beta.ID {
			t.Fatalf("replaced bindings=%+v", bindings[team.ID])
		}

		if err := admin.DeleteGroup(t.Context(), team.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.FindGroup(t.Context(), team.ID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("deleted group find error=%v", err)
		}
		bindings, _ = admin.GroupPolicyIDs(t.Context())
		if len(bindings[team.ID]) != 0 {
			t.Fatalf("deleted group kept bindings=%+v", bindings[team.ID])
		}
	})
}

func TestAdminReferenceCounts(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, _, policies, storages, _, _ := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		backend, err := storages.Create(t.Context(), model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/1", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		other, err := storages.Create(t.Context(), model.Storage{Name: "spare", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/2", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		alpha, err := policies.Create(t.Context(), model.Policy{Name: "alpha", StorageID: backend.ID, PathTpl: "{Y}", NameTpl: "{uniqid}", WebPMode: "both", ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename", WebPQuality: 80, WebPEffort: 4, ThumbSize: 400, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		alpha.ID = 0 // bind a fresh row of the same rule shape
		bound, err := policies.CreateAndBind(t.Context(), alpha, defaultGroup.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		image := model.Image{
			UserID: 0, PolicyID: bound.ID, StorageID: backend.ID, Key: "k1", Path: "2026/10/04/a", Ext: "png",
			MIME: "image/png", State: model.ImageStateActive, OperationID: "op", Frames: 1,
			ObjectManifest: []model.ObjectReceipt{},
		}
		if err := db.WithContext(t.Context()).Create(&image).Error; err != nil {
			t.Fatal(err)
		}

		// The storage holds the unbound alpha row plus its bound fresh copy.
		if count, err := admin.CountPoliciesForStorage(t.Context(), backend.ID); err != nil || count != 2 {
			t.Fatalf("policies for storage=%d err=%v", count, err)
		}
		if count, _ := admin.CountPoliciesForStorage(t.Context(), other.ID); count != 0 {
			t.Fatalf("unused storage referenced %d times", count)
		}
		if count, err := admin.CountImagesForPolicy(t.Context(), bound.ID); err != nil || count != 1 {
			t.Fatalf("images for policy=%d err=%v", count, err)
		}
		if count, err := admin.CountGroupDefaultsForPolicy(t.Context(), bound.ID); err != nil || count != 1 {
			t.Fatalf("group defaults for policy=%d err=%v", count, err)
		}
	})
}

func TestSettingGroupReferences(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, _, _, _, _, _ := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		team, err := admin.CreateGroup(t.Context(), model.Group{Name: "Team"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		referenced, err := admin.SettingGroupReferences(t.Context(), defaultGroup.ID)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, key := range referenced {
			found[key] = true
		}
		if !found["default_group_id"] {
			t.Fatalf("default group references=%+v", referenced)
		}
		if referenced, _ = admin.SettingGroupReferences(t.Context(), team.ID); len(referenced) != 0 {
			t.Fatalf("unreferenced group settings=%+v", referenced)
		}
	})
}

func TestSettingsReadAndUpdate(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, _, settings, _, _, _, _ := newAdminRepositories(t, db)
		values, err := settings.ReadSettings(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"registration_enabled", "guest_upload_enabled", "gallery_enabled", "trash_days", "api_enabled", "site_name", "guest_group_id", "default_group_id"} {
			if _, ok := values[key]; !ok {
				t.Fatalf("settings missing %s: %+v", key, values)
			}
		}
		name, err := settings.SiteName(t.Context())
		if err != nil || name != "ImgNest" {
			t.Fatalf("site name=%q err=%v", name, err)
		}
		err = settings.UpdateSettings(t.Context(), map[string]json.RawMessage{
			"site_name":  json.RawMessage(`"My Nest"`),
			"trash_days": json.RawMessage(`3`),
		})
		if err != nil {
			t.Fatal(err)
		}
		values, _ = settings.ReadSettings(t.Context())
		if string(values["site_name"]) != `"My Nest"` || string(values["trash_days"]) != "3" {
			t.Fatalf("updated settings=%+v", values)
		}
		if name, _ = settings.SiteName(t.Context()); name != "My Nest" {
			t.Fatalf("site name after update=%q", name)
		}
	})
}

func TestPolicyAdminLifecycle(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, _, _, policies, storages, _, _ := newAdminRepositories(t, db)
		backend, err := storages.Create(t.Context(), model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/1", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		created, err := policies.Create(t.Context(), model.Policy{Name: "alpha", StorageID: backend.ID, PathTpl: "{Y}/{m}"})
		if err != nil {
			t.Fatal(err)
		}
		// Repo-level defaults keep unbound rules consistent with provisioned
		// ones for the enum-valued fields; numeric fields stay caller-owned.
		if created.PathTpl != "{Y}/{m}" || created.NameTpl != "{uniqid}" || created.WebPMode != "both" ||
			created.WebPQuality != 80 || created.WebPEffort != 0 || created.ThumbSize != 400 ||
			created.ScrubMode != "gps" || created.LinkPrefer != "webp" || created.HEIFMode != "webp_only" ||
			created.OnConflict != "rename" {
			t.Fatalf("created policy defaults=%+v", created)
		}
		all, err := policies.All(t.Context())
		if err != nil || len(all) != 1 || all[0].ID != created.ID {
			t.Fatalf("all policies=%+v err=%v", all, err)
		}
		created.Enabled = false
		created.MaxWidth = 640
		if _, err := policies.Update(t.Context(), created); err != nil {
			t.Fatal(err)
		}
		stored, err := policies.Find(t.Context(), created.ID)
		if err != nil || stored.Enabled || stored.MaxWidth != 640 {
			t.Fatalf("updated policy=%+v err=%v", stored, err)
		}
		if _, err := policies.Create(t.Context(), model.Policy{Name: " ", StorageID: backend.ID}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("blank name accepted: %v", err)
		}
		if _, err := policies.Create(t.Context(), model.Policy{Name: "x", StorageID: backend.ID, WebPMode: "raw"}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("invalid webp mode accepted: %v", err)
		}
		if err := policies.Delete(t.Context(), created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := policies.Find(t.Context(), created.ID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("deleted policy find error=%v", err)
		}
	})
}

func TestStorageUpdateAndDelete(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, _, _, _, storages, _, _ := newAdminRepositories(t, db)
		backend, err := storages.Create(t.Context(), model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{"root":"data"}`), BaseURL: "http://images.test/i/1", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		backend.Name = "renamed"
		backend.BaseURL = "https://cdn.test/i/1"
		backend.Enabled = false
		backend.Config = json.RawMessage(`{"root":"elsewhere"}`)
		updated, err := storages.Update(t.Context(), backend)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Name != "renamed" || updated.BaseURL != "https://cdn.test/i/1" || updated.Enabled {
			t.Fatalf("updated storage=%+v", updated)
		}
		stored, err := storages.Find(t.Context(), backend.ID)
		if err != nil {
			t.Fatal(err)
		}
		// PostgreSQL normalizes JSONB whitespace; compare decoded values.
		var roots map[string]string
		if err := json.Unmarshal(stored.Config, &roots); err != nil || roots["root"] != "elsewhere" {
			t.Fatalf("stored config=%s err=%v", stored.Config, err)
		}
		if err := storages.Delete(t.Context(), backend.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := storages.Find(t.Context(), backend.ID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("deleted storage find error=%v", err)
		}
		if err := storages.Delete(t.Context(), 9999); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("missing storage delete error=%v", err)
		}
	})
}

func TestAdminImageListingAndTrashKeys(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		_, users, _, policies, storages, imagesRepo, _ := newAdminRepositories(t, db)
		var defaultGroup model.Group
		if err := db.WithContext(t.Context()).First(&defaultGroup, "is_default = ?", true).Error; err != nil {
			t.Fatal(err)
		}
		alice := seedAdminUser(t, db, "alice", "alice@example.com", defaultGroup.ID)
		bob := seedAdminUser(t, db, "bob", "bob@example.com", defaultGroup.ID)
		backend, err := storages.Create(t.Context(), model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/1", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		rule, err := policies.Create(t.Context(), model.Policy{Name: "rule", StorageID: backend.ID})
		if err != nil {
			t.Fatal(err)
		}
		seed := func(userID uint64, key, path, ext, origin, state string) model.Image {
			t.Helper()
			image := model.Image{
				UserID: userID, PolicyID: rule.ID, StorageID: backend.ID, Key: key, Path: path, Ext: ext,
				OriginName: origin, MIME: "image/png", State: state, OperationID: "op", Frames: 1,
				ObjectManifest: []model.ObjectReceipt{},
			}
			if state == model.ImageStateTrash {
				now := time.Now().UTC()
				image.DeletedAt = &now
			}
			if err := db.WithContext(t.Context()).Create(&image).Error; err != nil {
				t.Fatal(err)
			}
			return image
		}
		first := seed(alice.ID, "k1", "2026/10/04/sunrise", "png", "sunrise.png", model.ImageStateActive)
		second := seed(bob.ID, "k2", "2026/10/05/sunset", "webp", "holiday.png", model.ImageStateActive)
		trashed := seed(bob.ID, "k3", "2026/10/05/old", "png", "old.png", model.ImageStateTrash)

		images, total, err := imagesRepo.ListAdminPage(t.Context(), 0, "", 1, 20)
		if err != nil || total != 2 || len(images) != 2 || images[0].ID != second.ID || images[1].ID != first.ID {
			t.Fatalf("admin page total=%d rows=%+v err=%v", total, images, err)
		}
		images, total, err = imagesRepo.ListAdminPage(t.Context(), alice.ID, "", 1, 20)
		if err != nil || total != 1 || images[0].UserID != alice.ID {
			t.Fatalf("owner filter total=%d rows=%+v err=%v", total, images, err)
		}
		images, total, err = imagesRepo.ListAdminPage(t.Context(), 0, "holiday", 1, 20)
		if err != nil || total != 1 || images[0].ID != second.ID {
			t.Fatalf("origin keyword total=%d rows=%+v err=%v", total, images, err)
		}
		images, total, err = imagesRepo.ListAdminPage(t.Context(), 0, "2026/10/04/sunrise.png", 1, 20)
		if err != nil || total != 1 || images[0].ID != first.ID {
			t.Fatalf("pathname keyword total=%d rows=%+v err=%v", total, images, err)
		}
		images, total, err = imagesRepo.ListAdminPage(t.Context(), 0, "HOLIDAY", 1, 20)
		if err != nil || total != 1 || images[0].ID != second.ID {
			t.Fatalf("keyword is case-sensitive: total=%d err=%v", total, err)
		}
		_, total, err = imagesRepo.ListAdminPage(t.Context(), 0, "_", 1, 20)
		if err != nil || total != 0 {
			t.Fatalf("unescaped underscore matched: total=%d err=%v", total, err)
		}

		keys, err := imagesRepo.TrashKeys(t.Context())
		if err != nil || len(keys) != 1 || keys[0] != trashed.Key {
			t.Fatalf("trash keys=%+v err=%v", keys, err)
		}
		if _, err := users.FindUserByID(t.Context(), alice.ID); err != nil {
			t.Fatal(err)
		}
	})
}

// The database admits one guest group even when two creations race past the
// service-level check, and the loser surfaces as invalid input.
func TestCreateGroupRejectsSecondGuestGroup(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, _, _, _, _, _, _ := newAdminRepositories(t, db)
		if _, err := admin.CreateGroup(t.Context(), model.Group{Name: "guests", IsGuest: true}, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.CreateGroup(t.Context(), model.Group{Name: "guests-again", IsGuest: true}, nil); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("second guest group = %v, want invalid input", err)
		}
	})
}

func TestCountImagesForStorage(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "storage-count")
		reserveAndCommit(t, f, "counted-1", "2026/01/counted-1")
		admin, err := NewAdminRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if count, err := admin.CountImagesForStorage(t.Context(), f.storage.ID); err != nil || count != 1 {
			t.Fatalf("storage image count = %d err=%v, want 1", count, err)
		}
	})
}
