package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

func TestAdminSettingsAvatarProviderRoundTrip(t *testing.T) {
	svc, _, _, _, _, _, settingsFake := adminServiceFixture(t, AdminDependencies{})
	gravatar := "gravatar"
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{AvatarProvider: &gravatar}); err != nil {
		t.Fatal(err)
	}
	// The fixture does not replay writes into reads; the written payload is
	// the contract under test.
	if len(settingsFake.written) != 1 || string(settingsFake.written[0]["avatar_provider"]) != `"gravatar"` {
		t.Fatalf("written=%+v", settingsFake.written)
	}
	if len(settingsFake.written[0]) != 1 {
		t.Fatalf("partial write touched %d keys", len(settingsFake.written[0]))
	}
	for _, bad := range []string{"", "qq", "WeAvatar", "gravatar ", "https://gravatar.com"} {
		invalid := bad
		if _, err := svc.PutSettings(t.Context(), SettingsPatch{AvatarProvider: &invalid}); err == nil {
			t.Fatalf("provider %q accepted", invalid)
		}
	}
	settingsFake.values = map[string]json.RawMessage{"avatar_provider": json.RawMessage(`"weavatar"`)}
	view, err := svc.GetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view.AvatarProvider != model.AvatarProviderWeavatar {
		t.Fatalf("stored provider=%q, want weavatar", view.AvatarProvider)
	}
	// A stored value that parses as a string but is not a supported enum
	// degrades to the documented default instead of failing the read.
	for _, corrupt := range []string{`"qq"`, `"  "`, `null`} {
		settingsFake.values = map[string]json.RawMessage{"avatar_provider": json.RawMessage(corrupt)}
		view, err = svc.GetSettings(t.Context())
		if err != nil {
			t.Fatalf("corrupt value %s failed the read: %v", corrupt, err)
		}
		if view.AvatarProvider != model.DefaultAvatarProvider {
			t.Fatalf("corrupt value %s yielded provider=%q", corrupt, view.AvatarProvider)
		}
	}
	// Unparseable JSON or a wrong JSON type fails the read, matching how
	// every other settings key treats undecodable values; the database CHECK
	// makes unreachable JSON impossible in the first place.
	for _, broken := range []string{`weavatar`, `42`, `[]`} {
		settingsFake.values = map[string]json.RawMessage{"avatar_provider": json.RawMessage(broken)}
		if _, err = svc.GetSettings(t.Context()); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("unparseable provider %s error=%v, want ErrInvalidInput", broken, err)
		}
	}
}

func TestAdminSettingsAvatarProviderIgnoresOtherWrites(t *testing.T) {
	svc, _, _, _, _, _, settingsFake := adminServiceFixture(t, AdminDependencies{})
	name := "Kept"
	if _, err := svc.PutSettings(t.Context(), SettingsPatch{SiteName: &name}); err != nil {
		t.Fatal(err)
	}
	if _, contains := settingsFake.written[0]["avatar_provider"]; contains {
		t.Fatal("a settings write without avatar_provider touched the avatar key")
	}
}
