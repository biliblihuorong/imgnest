package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// AdminImageQuery bounds a site-wide image listing.
type AdminImageQuery struct {
	Page, Size int
	// UserID filters by owner; zero means every owner.
	UserID  uint64
	Keyword string
}

// adminImageStore is the optional image capability behind the console's
// site-wide listing and recycle-bin purge.
type adminImageStore interface {
	ListAdminPage(context.Context, uint64, string, int, int) ([]model.Image, int64, error)
	TrashKeys(context.Context) ([]string, error)
}

func (s *ImageService) adminCapabilities() (adminImageStore, error) {
	store, ok := s.deps.Images.(adminImageStore)
	if !ok {
		return nil, fmt.Errorf("admin image capability: %w", ErrInvalidInput)
	}
	return store, nil
}

// AdminList pages every active image site-wide with optional owner and
// keyword filters; the same ImageView DTO as the owner listing is returned.
func (s *ImageService) AdminList(ctx context.Context, subject TokenSubject, query AdminImageQuery) (ImagePage, error) {
	actor, err := s.actor(ctx, subject)
	if err != nil {
		return ImagePage{}, err
	}
	if actor.Role != model.UserRoleAdmin {
		return ImagePage{}, ErrForbidden
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Size == 0 {
		query.Size = 20
	}
	if query.Page < 1 || query.Page > 1000000 || query.Size < 1 || query.Size > 100 {
		return ImagePage{}, ErrInvalidInput
	}
	store, err := s.adminCapabilities()
	if err != nil {
		return ImagePage{}, err
	}
	images, total, err := store.ListAdminPage(ctx, query.UserID, query.Keyword, query.Page, query.Size)
	if err != nil {
		return ImagePage{}, fmt.Errorf("list admin images: %w", err)
	}
	views := make([]ImageView, 0, len(images))
	build := s.viewer()
	for _, image := range images {
		view, err := build(ctx, image)
		if err != nil {
			return ImagePage{}, err
		}
		views = append(views, view)
	}
	return ImagePage{Items: views, Total: total, Page: query.Page, Size: query.Size}, nil
}

// AdminTrash moves any owner's image into the recycle bin; the underlying
// trash pipeline is exactly the owner path with the administrator override
// of ownedImage.
func (s *ImageService) AdminTrash(ctx context.Context, subject TokenSubject, id uint64) error {
	image, err := s.imageByID(ctx, subject, id)
	if err != nil {
		return err
	}
	return s.Trash(ctx, subject, image.Key)
}

// AdminPurgeAll physically purges the entire site-wide recycle bin, one image
// through the regular purge path at a time. It reports how many images were
// purged even when some fail, so callers can show partial progress.
func (s *ImageService) AdminPurgeAll(ctx context.Context, subject TokenSubject) (int64, error) {
	actor, err := s.actor(ctx, subject)
	if err != nil {
		return 0, err
	}
	if actor.Role != model.UserRoleAdmin {
		return 0, ErrForbidden
	}
	store, err := s.adminCapabilities()
	if err != nil {
		return 0, err
	}
	keys, err := store.TrashKeys(ctx)
	if err != nil {
		return 0, fmt.Errorf("list trash keys: %w", err)
	}
	var purged int64
	var failures []error
	for _, key := range keys {
		if err := s.Purge(ctx, subject, key); err != nil {
			failures = append(failures, err)
			continue
		}
		purged++
	}
	return purged, errors.Join(failures...)
}
