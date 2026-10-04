package repo

import (
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
		created, err := tokens.CreateToken(t.Context(), input)
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

func testToken(userID uint64) model.Token {
	return model.Token{
		UserID: userID, Name: "integration", Kind: model.TokenKindAPI,
		TokenHash: "aabbccddee", Abilities: []string{"*"},
	}
}
