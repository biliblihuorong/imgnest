package repo

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func accountValue[T any](value T) *T { return &value }

func managedAccountFixture(t *testing.T, db *gorm.DB) (*AdminRepository, model.User, model.User, uint64) {
	t.Helper()
	admin, users, settings, _, _, _, _ := newAdminRepositories(t, db)
	groupID, err := settings.DefaultGroupID(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	owner, err := users.BootstrapAdmin(t.Context(), testUser("owner", "owner@example.com", groupID))
	if err != nil {
		t.Fatal(err)
	}
	target := seedAdminUser(t, db, "target", "target@example.com", groupID)
	return admin, owner, target, groupID
}

func accountTokens(t *testing.T, db *gorm.DB, user model.User) *TokenRepository {
	t.Helper()
	tokens, err := NewTokenRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{model.TokenKindWeb, model.TokenKindAPI} {
		_, err := tokens.CreateToken(t.Context(), model.Token{UserID: user.ID, Name: kind, Kind: kind,
			TokenHash: kind + user.Username, Abilities: []string{"*"}}, testGrant(user.PasswordHash))
		if err != nil {
			t.Fatal(err)
		}
	}
	return tokens
}

func TestAdminCreateAccountUsesExplicitFieldsAndUniqueConstraints(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, group := managedAccountFixture(t, db)
		input := testUser("created", "created@example.com", group)
		input.DisplayName = "Nickname"
		input.ID = 9999
		input.AuthVersion = 99
		input.UsedBytes = 12345
		input.RegisteredIP = "192.0.2.99"
		created, err := admin.CreateAdminUser(t.Context(), owner.ID, input)
		if err != nil || created.ID == 0 || created.ID == input.ID || created.UsedBytes != 0 || created.AuthVersion != 0 || created.RegisteredIP != "" || created.DisplayName != "Nickname" {
			t.Fatalf("create=%+v err=%v", created, err)
		}
		for _, duplicate := range []model.User{testUser("created", "other@example.com", group), testUser("other", "created@example.com", group)} {
			if _, err := admin.CreateAdminUser(t.Context(), owner.ID, duplicate); !errors.Is(err, model.ErrUserExists) {
				t.Fatalf("duplicate=%v", err)
			}
		}
		if _, err := admin.CreateAdminUser(t.Context(), target.ID, testUser("forbidden", "forbidden@example.com", group)); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("ordinary actor=%v", err)
		}
		guest, err := admin.CreateGroup(t.Context(), model.Group{Name: "Guests", IsGuest: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admin.CreateAdminUser(t.Context(), owner.ID, testUser("guestuser", "guestuser@example.com", guest.ID)); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("guest group=%v", err)
		}
	})
}

func TestAdminAccountPatchIsAtomicAndPreservesCredentials(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, _ := managedAccountFixture(t, db)
		tokens := accountTokens(t, db, target)
		_, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{
			Username: &owner.Username, DisplayName: accountValue("not saved"), Status: accountValue(model.UserStatusDisabled),
		})
		if !errors.Is(err, model.ErrUserExists) {
			t.Fatalf("duplicate patch=%v", err)
		}
		stored, err := admin.FindUserByID(t.Context(), target.ID)
		if err != nil || stored.Status != target.Status || stored.DisplayName != "" || stored.AuthVersion != target.AuthVersion {
			t.Fatalf("partial patch=%+v err=%v", stored, err)
		}
		remaining, err := tokens.ListTokens(t.Context(), target.ID)
		if err != nil || len(remaining) != 2 {
			t.Fatalf("failed patch revoked tokens: %d %v", len(remaining), err)
		}
		updated, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{DisplayName: accountValue("Renamed")})
		if err != nil || updated.DisplayName != "Renamed" || updated.PasswordHash != target.PasswordHash || updated.AuthVersion != target.AuthVersion {
			t.Fatalf("profile-only=%+v %v", updated, err)
		}
		remaining, _ = tokens.ListTokens(t.Context(), target.ID)
		if len(remaining) != 2 {
			t.Fatal("nickname-only patch revoked tokens")
		}
		updated, err = admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Username: &target.Username, Email: &target.Email, Role: &target.Role, Status: &target.Status, GroupID: &target.GroupID})
		if err != nil {
			t.Fatal(err)
		}
		remaining, _ = tokens.ListTokens(t.Context(), target.ID)
		if len(remaining) != 2 || updated.AuthVersion != target.AuthVersion {
			t.Fatal("unchanged fields revoked tokens or advanced the epoch")
		}
	})
}

func TestAdminSecurityChangesRevokeEveryToken(t *testing.T) {
	for _, field := range []string{"username", "email", "role", "group", "status"} {
		t.Run(field, func(t *testing.T) {
			forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
				admin, owner, target, _ := managedAccountFixture(t, db)
				tokens := accountTokens(t, db, target)
				patch := model.UserChanges{}
				switch field {
				case "username":
					patch.Username = accountValue("renamed")
				case "email":
					patch.Email = accountValue("renamed@example.com")
				case "role":
					patch.Role = accountValue(model.UserRoleAdmin)
				case "status":
					patch.Status = accountValue(model.UserStatusDisabled)
				case "group":
					group, err := admin.CreateGroup(t.Context(), model.Group{Name: "Team"}, nil)
					if err != nil {
						t.Fatal(err)
					}
					patch.GroupID = &group.ID
				}
				updated, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, patch)
				if err != nil {
					t.Fatal(err)
				}
				if updated.AuthVersion != target.AuthVersion+1 {
					t.Fatalf("security edit epoch=%d, want %d", updated.AuthVersion, target.AuthVersion+1)
				}
				remaining, err := tokens.ListTokens(t.Context(), target.ID)
				if err != nil || len(remaining) != 0 {
					t.Fatalf("tokens=%d err=%v", len(remaining), err)
				}
			})
		})
	}
}

func TestAdminAccountProtectsSelfAndLastEnabledAdmin(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, _ := managedAccountFixture(t, db)
		for _, patch := range []model.UserChanges{{Status: accountValue(model.UserStatusDisabled)}, {Role: accountValue(model.UserRoleUser)}} {
			if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, owner.ID, patch); !errors.Is(err, model.ErrForbidden) {
				t.Fatalf("last/self patch=%v", err)
			}
		}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Role: accountValue(model.UserRoleAdmin), Status: accountValue(model.UserStatusDisabled)}); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, owner.ID, model.UserChanges{Role: accountValue(model.UserRoleUser)}); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("disabled admin counted=%v", err)
		}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Status: accountValue(model.UserStatusEnabled)}); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, owner.ID, model.UserChanges{Role: accountValue(model.UserRoleUser)}); err != nil {
			t.Fatalf("self demotion with replacement=%v", err)
		}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Role: accountValue(model.UserRoleUser)}); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("stale admin actor=%v", err)
		}
	})
}

func TestConcurrentAdminRoleStatusEditsKeepEnabledAdmin(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
			admin, owner, target, _ := managedAccountFixture(t, db)
			if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Role: accountValue(model.UserRoleAdmin)}); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				_, err := admin.UpdateAdminUser(t.Context(), owner.ID, owner.ID, model.UserChanges{Role: accountValue(model.UserRoleUser)})
				results <- err
			}()
			go func() {
				<-start
				patch := model.UserChanges{Role: accountValue(model.UserRoleUser)}
				caller := target.ID
				if mixed {
					patch = model.UserChanges{Status: accountValue(model.UserStatusDisabled)}
					caller = owner.ID
				}
				_, err := admin.UpdateAdminUser(t.Context(), caller, target.ID, patch)
				results <- err
			}()
			close(start)
			success, forbidden := 0, 0
			for range 2 {
				err := <-results
				switch {
				case err == nil:
					success++
				case errors.Is(err, model.ErrForbidden):
					forbidden++
				default:
					t.Fatalf("concurrent patch=%v", err)
				}
			}
			var count int64
			if err := db.Model(&model.User{}).Where("id <> 0 AND role = ? AND status = ?", model.UserRoleAdmin, model.UserStatusEnabled).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 || success != 1 || forbidden != 1 {
				t.Fatalf("enabled=%d success=%d forbidden=%d mixed=%t", count, success, forbidden, mixed)
			}
		})
	}
}

func TestAdminIdentityEditRejectsStaleLoginGrant(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, _ := managedAccountFixture(t, db)
		tokens, err := NewTokenRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		grant := model.TokenGrant{ExpectedPasswordHash: target.PasswordHash, ExpectedAccountState: &model.AccountState{
			Username: target.Username, Email: target.Email, Role: target.Role, GroupID: target.GroupID, Status: target.Status,
		}, At: time.Now().UTC()}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Email: accountValue("new@example.com")}); err != nil {
			t.Fatal(err)
		}
		if _, err := tokens.CreateToken(t.Context(), testToken(target.ID), grant); !errors.Is(err, model.ErrUnauthenticated) {
			t.Fatalf("stale login issued token: %v", err)
		}
	})
}

func TestConcurrentAdminAccountUniqueness(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, groupID := managedAccountFixture(t, db)
		other := seedAdminUser(t, db, "other", "other@example.com", groupID)
		for _, operation := range []string{"create", "patch"} {
			t.Run(operation, func(t *testing.T) {
				start := make(chan struct{})
				results := make(chan error, 2)
				for _, id := range []uint64{target.ID, other.ID} {
					go func() {
						<-start
						var err error
						if operation == "create" {
							_, err = admin.CreateAdminUser(t.Context(), owner.ID, testUser("duplicate", "duplicate@example.com", groupID))
						} else {
							_, err = admin.UpdateAdminUser(t.Context(), owner.ID, id, model.UserChanges{Email: accountValue("same@example.com")})
						}
						results <- err
					}()
				}
				close(start)
				success, duplicate := 0, 0
				for range 2 {
					err := <-results
					switch {
					case err == nil:
						success++
					case errors.Is(err, model.ErrUserExists):
						duplicate++
					default:
						t.Fatalf("concurrent %s: %v", operation, err)
					}
				}
				if success != 1 || duplicate != 1 {
					t.Fatalf("success=%d duplicate=%d", success, duplicate)
				}
			})
		}
	})
}

func TestAdminAccountPatchRollsBackWhenTokenRevocationFails(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, _ := managedAccountFixture(t, db)
		tokens := accountTokens(t, db, target)
		const callback = "test:deny_account_token_delete"
		err := db.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "tokens" {
				_ = tx.AddError(errors.New("fixture token deletion failure"))
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := db.Callback().Delete().Remove(callback); err != nil {
				t.Error(err)
			}
		})
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Email: accountValue("new@example.com"), DisplayName: accountValue("not saved")}); err == nil {
			t.Fatal("failed revocation reported success")
		}
		stored, err := admin.FindUserByID(t.Context(), target.ID)
		if err != nil || stored.Email != target.Email || stored.DisplayName != "" || stored.AuthVersion != target.AuthVersion {
			t.Fatalf("partial account change: %+v %v", stored, err)
		}
		remaining, err := tokens.ListTokens(t.Context(), target.ID)
		if err != nil || len(remaining) != 2 {
			t.Fatalf("tokens=%d err=%v", len(remaining), err)
		}
	})
}

func TestAdminAccountAuthVersionExhaustionFailsClosed(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		admin, owner, target, _ := managedAccountFixture(t, db)
		tokens := accountTokens(t, db, target)
		if err := db.WithContext(t.Context()).Model(&model.User{}).Where("id = ?", target.ID).Update("auth_version", int64(math.MaxInt64)).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Email: accountValue("new@example.com")}); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("exhausted epoch accepted security edit: %v", err)
		}
		stored, err := admin.FindUserByID(t.Context(), target.ID)
		if err != nil || stored.Email != target.Email || stored.AuthVersion != math.MaxInt64 {
			t.Fatalf("epoch wrapped or partial edit persisted: %+v %v", stored, err)
		}
		remaining, err := tokens.ListTokens(t.Context(), target.ID)
		if err != nil || len(remaining) != 2 {
			t.Fatalf("failed edit revoked tokens: %d %v", len(remaining), err)
		}
		if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{DisplayName: accountValue("Still editable")}); err != nil {
			t.Fatalf("nickname unnecessarily requires a new epoch: %v", err)
		}
	})
}
