package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

func TestAvatarEmailHashNormalizesTrimAndCase(t *testing.T) {
	digest := sha256.Sum256([]byte("user@example.com"))
	want := hex.EncodeToString(digest[:])
	cases := map[string]string{
		"plain":            "user@example.com",
		"upper":            "USER@EXAMPLE.COM",
		"mixed":            "User@Example.Com",
		"padded":           "  user@example.com\t",
		"padded and upper": " User@Example.COM ",
	}
	for name, raw := range cases {
		if got := avatarEmailHash(raw); got != want {
			t.Fatalf("%s hash=%q, want %q", name, got, want)
		}
	}
	// The plus tag and the dot are part of the mailbox identity for avatar
	// matching and must produce a different digest, never be stripped.
	tagged := avatarEmailHash("user+tag@example.com")
	plain := avatarEmailHash("user@example.com")
	if tagged == plain {
		t.Fatal("plus tag was stripped from the avatar hash")
	}
	dotted := avatarEmailHash("u.s.e.r@example.com")
	if dotted == plain {
		t.Fatal("dots were stripped from the avatar hash")
	}
}

func TestAvatarURLBuildsOnlyControlledHTTPSAddresses(t *testing.T) {
	hash := avatarEmailHash("User@Example.com")
	weavatar := avatarURL(model.AvatarConfig{Provider: model.AvatarProviderWeavatar}, "user@example.com")
	if weavatar == nil || *weavatar != "https://weavatar.com/avatar/"+hash+"?s=160&d=404" {
		t.Fatalf("weavatar url=%v", weavatar)
	}
	gravatar := avatarURL(model.AvatarConfig{Provider: model.AvatarProviderGravatar}, "user@example.com")
	if gravatar == nil || *gravatar != "https://gravatar.com/avatar/"+hash+"?s=160&d=404" {
		t.Fatalf("gravatar url=%v", gravatar)
	}
	// An unsupported stored provider never selects a different external host.
	fallback := avatarURL(model.AvatarConfig{Provider: "evil.example"}, "user@example.com")
	if fallback == nil || !strings.HasPrefix(*fallback, "https://weavatar.com/avatar/") {
		t.Fatalf("unknown provider url=%v", fallback)
	}
	if avatarURL(model.AvatarConfig{Provider: model.AvatarProviderWeavatar}, "   ") != nil {
		t.Fatal("blank email must not produce an avatar URL")
	}
	if avatarURL(model.AvatarConfig{Provider: model.AvatarProviderWeavatar}, "") != nil {
		t.Fatal("empty email must not produce an avatar URL")
	}
}

func TestApplyAvatarAlwaysReportsSupportedProvider(t *testing.T) {
	view := UserView{Email: "someone@example.com"}
	applyAvatar(&view, model.AvatarConfig{Provider: "garbage", Version: 7})
	if view.AvatarProvider != model.DefaultAvatarProvider {
		t.Fatalf("provider=%q leaked an unsupported value", view.AvatarProvider)
	}
	if view.AvatarURL == nil || !strings.HasPrefix(*view.AvatarURL, "https://weavatar.com/avatar/") {
		t.Fatalf("avatar url=%v", view.AvatarURL)
	}
	if view.AvatarConfigVersion != 7 {
		t.Fatalf("config version=%d, want 7", view.AvatarConfigVersion)
	}
	anonymous := UserView{}
	applyAvatar(&anonymous, model.AvatarConfig{Provider: model.AvatarProviderGravatar, Version: 3})
	if anonymous.AvatarProvider != model.AvatarProviderGravatar {
		t.Fatalf("provider=%q", anonymous.AvatarProvider)
	}
	if anonymous.AvatarURL != nil {
		t.Fatalf("missing email avatar url=%v", anonymous.AvatarURL)
	}
	if anonymous.AvatarConfigVersion != 3 {
		t.Fatalf("config version=%d, want 3", anonymous.AvatarConfigVersion)
	}
}

func TestDisplayNameValidationRules(t *testing.T) {
	// Mirrors the bounds UpdateDisplayName enforces; kept here so the rules
	// stay pinned next to the avatar contract tests.
	if maxDisplayNameRunes != 64 {
		t.Fatalf("display name bound=%d, want 64", maxDisplayNameRunes)
	}
	if !model.ValidAvatarProvider(model.DefaultAvatarProvider) {
		t.Fatal("default avatar provider must be a supported enum value")
	}
}
