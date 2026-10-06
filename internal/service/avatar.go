package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// avatarURLSize is the fixed image edge length requested from the avatar
// provider. Clients never append query parameters of their own.
const avatarURLSize = 160

// avatarEmailHash normalizes a mailbox for avatar matching: surrounding
// whitespace is trimmed, the address is lowercased, and the SHA-256 of the
// normalized UTF-8 bytes is returned as lowercase hex. The normalization only
// applies to this matching copy; the stored account email is never modified.
func avatarEmailHash(email string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(digest[:])
}

// avatarURL builds the HTTPS avatar address for an email under the configured
// provider, or nil when the account has no usable mailbox. Only the two
// supported provider hosts can appear; d=404 lets clients fall back to the
// local default when the provider has no image for the hash.
func avatarURL(config model.AvatarConfig, email string) *string {
	if strings.TrimSpace(email) == "" {
		return nil
	}
	provider := config.Provider
	if !model.ValidAvatarProvider(provider) {
		provider = model.DefaultAvatarProvider
	}
	host := "weavatar.com"
	if provider == model.AvatarProviderGravatar {
		host = "gravatar.com"
	}
	url := fmt.Sprintf(
		"https://%s/avatar/%s?s=%d&d=404",
		host, avatarEmailHash(email), avatarURLSize,
	)
	return &url
}

// applyAvatar fills the avatar fields of a user view from the site-wide
// configuration. The reported provider is always a supported enum value.
func applyAvatar(view *UserView, config model.AvatarConfig) {
	provider := config.Provider
	if !model.ValidAvatarProvider(provider) {
		provider = model.DefaultAvatarProvider
	}
	view.AvatarProvider = provider
	view.AvatarURL = avatarURL(config, view.Email)
	view.AvatarConfigVersion = config.Version
}
