package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// ErrImportLinkChanged rejects an imported image whose stored original would
// get a different extension than its historic link, so the old URL would
// stop resolving to it.
var ErrImportLinkChanged = errors.New("imported image would change its link")

// ErrFileTooLarge rejects an imported file over its owner group's per-file
// limit.
var ErrFileTooLarge = errors.New("imported file exceeds the group's file size limit")

// MaxImportFileBytes bounds one imported file. An import follows the owner
// group's per-file limit instead of server.max_upload_mb, which guards
// request bodies; a group without a limit is held to this ceiling.
const MaxImportFileBytes = 100 << 20

// ImportInput is one historic image re-ingested by an operator-run import.
// Path is the historic object key without its extension and Ext the
// extension the historic link ends with; both are kept unchanged.
type ImportInput struct {
	Data             []byte
	Filename, IP     string
	Path, Ext        string
	UserID, PolicyID uint64
	AlbumID          uint64
	IsPublic         bool
	CreatedAt        time.Time
	// AdoptVersion, when set, adopts the original already stored at
	// Path.Ext in the rule's bucket instead of writing it: Data must be the
	// bytes of exactly that object version. The original is never rewritten
	// or scrubbed; only the WebP, thumbnail and responsive copies are added.
	AdoptVersion string
}

// ImportImage runs one historic image through the regular upload pipeline
// (format probe, EXIF, scrub, WebP, thumbnails, responsive variants, quota
// and compensation) on behalf of its owner, keeping its historic path,
// extension and creation time. It is reachable only from the command line:
// the operator, not a bearer token, authorizes it, and the owner's account
// must still be enabled and allowed to use the rule.
func (s *ImageService) ImportImage(ctx context.Context, input ImportInput) (ImageView, error) {
	if err := ctx.Err(); err != nil {
		return ImageView{}, fmt.Errorf("import image: %w", err)
	}
	ext := model.CanonicalExt(strings.ToLower(input.Ext))
	if input.UserID == 0 || input.Path == "" || ext == "" || (input.AdoptVersion != "" && !model.AdoptableVersion(input.AdoptVersion)) {
		return ImageView{}, ErrInvalidInput
	}
	checked, err := s.deps.Paths.Sanitize(ctx, input.Path)
	if err != nil || checked != input.Path {
		return ImageView{}, ErrInvalidInput
	}
	user, err := s.deps.Users.FindUserByID(ctx, input.UserID)
	if err != nil {
		return ImageView{}, fmt.Errorf("read import owner: %w", err)
	}
	if user.Status != model.UserStatusEnabled {
		return ImageView{}, ErrForbidden
	}
	now := s.deps.Now().UTC()
	grant := model.TokenGrant{UserID: user.ID, ExpectedPasswordHash: user.PasswordHash, At: now, Clock: s.deps.Now}
	upload := UploadInput{Data: input.Data, Filename: input.Filename, IP: input.IP, PolicyID: input.PolicyID, AlbumID: input.AlbumID, IsPublic: input.IsPublic}
	return s.publish(ctx, upload, UploadLimits{MaxFileBytes: MaxImportFileBytes}, uploadPlan{
		userID: user.ID,
		policy: func(ctx context.Context, policyID uint64) (model.Policy, model.Storage, model.Group, error) {
			policy, backend, group, err := s.deps.Policies.UploadPolicy(ctx, user.ID, policyID)
			if err == nil && group.MaxFileBytes > 0 && int64(len(input.Data)) > group.MaxFileBytes {
				return policy, backend, group, ErrFileTooLarge
			}
			return policy, backend, group, err
		},
		reserve: func(ctx context.Context, reservation model.UploadReservation) (model.Image, error) {
			reservation.Grant = grant
			return s.deps.Images.ReserveUpload(ctx, reservation)
		},
		commit: func(ctx context.Context, key, op string, exif model.ImageExif) (model.Image, error) {
			return s.deps.Images.CommitUpload(ctx, key, op, exif, grant)
		},
		pinned: &pinnedPath{path: input.Path, ext: ext, createdAt: input.CreatedAt, adoptVersion: input.AdoptVersion},
	})
}
