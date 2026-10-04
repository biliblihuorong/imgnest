package repo

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestTokenPersistenceOwnershipAndTouch(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, settings := repositories(t, db)
		groupID, err := settings.DefaultGroupID(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		owner, err := users.CreateUser(t.Context(), testUser("owner", "owner@example.com", groupID))
		if err != nil {
			t.Fatal(err)
		}
		other, err := users.CreateUser(t.Context(), testUser("other", "other@example.com", groupID))
		if err != nil {
			t.Fatal(err)
		}
		tokens, err := NewTokenRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		input := testToken(owner.ID)
		expiry := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
		input.ExpiresAt = &expiry
		created, err := tokens.CreateToken(t.Context(), input, testGrant(owner.PasswordHash))
		if err != nil {
			t.Fatal(err)
		}
		found, err := tokens.FindToken(t.Context(), created.ID)
		if err != nil || found.TokenHash != input.TokenHash || !reflect.DeepEqual(found.Abilities, []string{"*"}) {
			t.Fatalf("token did not roundtrip: %+v, error=%v", found, err)
		}
		if found.LastUsedAt != nil || found.ExpiresAt == nil || !found.ExpiresAt.Equal(expiry) {
			t.Fatal("token nullable dates changed")
		}
		at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
		if err := tokens.TouchToken(t.Context(), created.ID, at); err != nil {
			t.Fatal(err)
		}
		found, err = tokens.FindToken(t.Context(), created.ID)
		if err != nil || found.LastUsedAt == nil || !found.LastUsedAt.Equal(at) {
			t.Fatalf("last usage not persisted: %v", err)
		}
		foreign, err := tokens.ListTokens(t.Context(), other.ID)
		if err != nil || foreign == nil || len(foreign) != 0 {
			t.Fatalf("foreign list=%+v, error=%v", foreign, err)
		}
		if err := tokens.RevokeToken(t.Context(), other.ID, created.ID); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("foreign revoke error=%v, want ErrForbidden", err)
		}
		if err := tokens.RevokeToken(t.Context(), owner.ID, created.ID+99); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("missing revoke error=%v, want ErrForbidden", err)
		}
		if err := tokens.RevokeToken(t.Context(), owner.ID, created.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := tokens.FindToken(t.Context(), created.ID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("revoked token error=%v, want ErrNotFound", err)
		}
	})
}

func TestOutOfRangeIDsAreAbsent(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		users, _ := repositories(t, db)
		tokens, err := NewTokenRepository(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := users.FindUserByID(t.Context(), math.MaxUint64); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("out-of-range user lookup error=%v, want ErrNotFound", err)
		}
		if _, err := tokens.FindToken(t.Context(), math.MaxUint64); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("out-of-range token lookup error=%v, want ErrNotFound", err)
		}
		if err := tokens.TouchToken(t.Context(), math.MaxUint64, time.Now()); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("out-of-range token touch error=%v, want ErrNotFound", err)
		}
		if err := tokens.RevokeToken(t.Context(), 1, math.MaxUint64); !errors.Is(err, model.ErrForbidden) {
			t.Errorf("out-of-range token revoke error=%v, want ErrForbidden", err)
		}
	})
}

func TestTokenGrantRejectsExplicitOtherActor(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		first, _, tokens := grantFixture(t, db, "grantactor")
		other, _, _ := grantFixture(t, db, "granttarget")
		grant := testGrant(first.PasswordHash)
		grant.UserID = first.ID
		if _, err := tokens.CreateToken(t.Context(), testToken(other.ID), grant); !errors.Is(err, model.ErrUnauthenticated) {
			t.Fatalf("explicit grant actor was ignored: %v", err)
		}
	})
}

func TestTokenGrantRejectsChangedCredentials(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, scenario := range []string{"password", "disabled", "revoked", "expired", "foreign"} {
			t.Run(scenario, func(t *testing.T) {
				user, users, tokens := grantFixture(t, db, scenario)
				grant := testGrant(user.PasswordHash)
				if scenario == "revoked" || scenario == "expired" || scenario == "foreign" {
					source := testToken(user.ID)
					if scenario == "expired" {
						source.ExpiresAt = &grant.At
					}
					if scenario == "foreign" {
						other, _, _ := grantFixture(t, db, "foreignowner")
						source.UserID = other.ID
					}
					created, err := tokens.CreateToken(t.Context(), source, grant)
					if err != nil {
						t.Fatal(err)
					}
					grant.SourceTokenID = created.ID
					if scenario == "revoked" {
						if err := tokens.RevokeToken(t.Context(), user.ID, created.ID); err != nil {
							t.Fatal(err)
						}
					}
				}
				if scenario == "password" {
					if err := users.UpdatePasswordAndRevokeTokens(
						t.Context(), user.ID, user.PasswordHash, "replacement-digest",
					); err != nil {
						t.Fatal(err)
					}
				}
				if scenario == "disabled" {
					if err := db.WithContext(t.Context()).Model(&model.User{}).
						Where("id = ?", user.ID).Update("status", model.UserStatusDisabled).Error; err != nil {
						t.Fatal(err)
					}
				}
				candidate := testToken(user.ID)
				candidate.Name = "must-not-exist"
				if _, err := tokens.CreateToken(t.Context(), candidate, grant); !errors.Is(err, model.ErrUnauthenticated) {
					t.Errorf("stale %s grant error=%v, want ErrUnauthenticated", scenario, err)
				}
				var minted int64
				if err := db.WithContext(t.Context()).Model(&model.Token{}).
					Where("user_id = ? AND name = ?", user.ID, candidate.Name).Count(&minted).Error; err != nil {
					t.Fatal(err)
				}
				if minted != 0 {
					t.Errorf("stale grant minted %d token(s)", minted)
				}
			})
		}
	})
}

func TestTokenGrantSerializesCredentialChanges(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		for _, operation := range []string{"password", "revoke", "revoke_all"} {
			t.Run(operation, func(t *testing.T) {
				user, users, tokens := grantFixture(t, db, "locked"+operation)
				grant := testGrant(user.PasswordHash)
				source, err := tokens.CreateToken(t.Context(), testToken(user.ID), grant)
				if err != nil {
					t.Fatal(err)
				}
				grant.SourceTokenID = source.ID
				entered := make(chan struct{})
				release := make(chan struct{})
				callback := "test:hold-token-" + operation
				if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					candidate, ok := tx.Statement.Dest.(*model.Token)
					if !ok || candidate.Name != "held-token" {
						return
					}
					close(entered)
					<-release
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := db.Callback().Create().Remove(callback); err != nil {
						t.Error(err)
					}
				})
				minted := make(chan error, 1)
				go func() {
					candidate := testToken(user.ID)
					candidate.Name = "held-token"
					_, err := tokens.CreateToken(t.Context(), candidate, grant)
					minted <- err
				}()
				select {
				case <-entered:
				case <-time.After(5 * time.Second):
					close(release)
					t.Fatal("token creation never reached the insert barrier")
				}
				mutate := func(ctx context.Context) error {
					switch operation {
					case "password":
						return users.UpdatePasswordAndRevokeTokens(
							ctx,
							user.ID,
							user.PasswordHash,
							"replacement-digest",
						)
					case "revoke":
						return tokens.RevokeToken(ctx, user.ID, source.ID)
					default:
						return tokens.RevokeAllTokens(ctx, user.ID)
					}
				}
				blocked, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
				mutationErr := mutate(blocked)
				cancel()
				close(release)
				if err := <-minted; err != nil {
					t.Fatal(err)
				}
				if !errors.Is(mutationErr, context.DeadlineExceeded) {
					t.Errorf("%s passed an in-flight grant: error=%v, want DeadlineExceeded", operation, mutationErr)
				}
				if mutationErr == nil {
					return
				}
				if err := mutate(t.Context()); err != nil {
					t.Fatal(err)
				}
				if operation != "revoke" {
					remaining, err := tokens.ListTokens(t.Context(), user.ID)
					if err != nil || len(remaining) != 0 {
						t.Errorf("%s did not remove minted tokens: count=%d error=%v", operation, len(remaining), err)
					}
				}
			})
		}
	})
}

func grantFixture(t *testing.T, db *gorm.DB, name string) (model.User, *UserRepository, *TokenRepository) {
	t.Helper()
	users, settings := repositories(t, db)
	groupID, err := settings.DefaultGroupID(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(t.Context(), testUser(name, name+"@example.com", groupID))
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := NewTokenRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	return user, users, tokens
}

func testToken(userID uint64) model.Token {
	return model.Token{
		UserID: userID, Name: "integration", Kind: model.TokenKindAPI,
		TokenHash: "aabbccddee", Abilities: []string{"*"},
	}
}

func testGrant(hash string) model.TokenGrant {
	return model.TokenGrant{
		ExpectedPasswordHash: hash,
		At:                   time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}
}
