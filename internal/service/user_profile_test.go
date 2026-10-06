package service_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestUpdateDisplayNamePersistsAndDecorates(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "displayuser")
		if user.DisplayName != "" {
			t.Fatalf("new account display name=%q, want empty", user.DisplayName)
		}
		if user.AvatarProvider != model.DefaultAvatarProvider {
			t.Fatalf("registered view provider=%q", user.AvatarProvider)
		}
		if user.AvatarURL == nil || !strings.HasPrefix(*user.AvatarURL, "https://weavatar.com/avatar/") {
			t.Fatalf("registered view avatar url=%v", user.AvatarURL)
		}
		updated, err := fixture.service.UpdateDisplayName(t.Context(), user.ID, "  昵称 Nick ")
		if err != nil {
			t.Fatal(err)
		}
		if updated.DisplayName != "昵称 Nick" {
			t.Fatalf("display name=%q, want trimmed value", updated.DisplayName)
		}
		reread, err := fixture.users.FindUserByID(t.Context(), user.ID)
		if err != nil || reread.DisplayName != "昵称 Nick" {
			t.Fatalf("reread display name=%q error=%v", reread.DisplayName, err)
		}
		if reread.Email != user.Email || reread.Role != user.Role {
			t.Fatal("profile update touched immutable account fields")
		}
		cleared, err := fixture.service.UpdateDisplayName(t.Context(), user.ID, "   ")
		if err != nil || cleared.DisplayName != "" {
			t.Fatalf("clear display name=%q error=%v", cleared.DisplayName, err)
		}
	})
}

func TestUpdateDisplayNameRejectsInvalidValues(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "displayguard")
		tooLong := strings.Repeat("图", 65)
		for _, name := range []string{tooLong, "bad\x07name", "line\nbreak"} {
			if _, err := fixture.service.UpdateDisplayName(t.Context(), user.ID, name); !errors.Is(err, service.ErrInvalidInput) {
				t.Fatalf("display name %q accepted: %v", name, err)
			}
		}
		if _, err := fixture.service.UpdateDisplayName(t.Context(), 0, "x"); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("zero user id accepted: %v", err)
		}
		reread, err := fixture.users.FindUserByID(t.Context(), user.ID)
		if err != nil || reread.DisplayName != "" {
			t.Fatalf("rejected write persisted name=%q error=%v", reread.DisplayName, err)
		}
	})
}

func TestAuthenticateViewCarriesAvatarProviderSwitch(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "avataruser")
		if err := fixture.db.WithContext(t.Context()).Exec(
			`INSERT INTO settings (key, value) VALUES ('avatar_provider', '"gravatar"')`,
		).Error; err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
		svc := newTokenService(t, fixture, &now)
		issued, err := svc.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		identity, err := svc.Authenticate(t.Context(), issued.Token)
		if err != nil {
			t.Fatal(err)
		}
		if identity.User.AvatarProvider != model.AvatarProviderGravatar {
			t.Fatalf("authenticated provider=%q, want gravatar", identity.User.AvatarProvider)
		}
		wantPrefix := "https://gravatar.com/avatar/"
		if identity.User.AvatarURL == nil || !strings.HasPrefix(*identity.User.AvatarURL, wantPrefix) {
			t.Fatalf("authenticated avatar url=%v", identity.User.AvatarURL)
		}
		if identity.User.AvatarConfigVersion == 0 {
			t.Fatal("stored provider must expose a nonzero config version")
		}
		// The email itself must never travel in the avatar address.
		if identity.User.AvatarURL != nil && strings.Contains(*identity.User.AvatarURL, "avataruser") {
			t.Fatal("avatar URL contains the raw email")
		}
	})
}
