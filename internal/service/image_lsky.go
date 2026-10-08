package service

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// AlbumBrief names the album an image belongs to in the v1 list.
type AlbumBrief struct {
	ID   uint64
	Name string
}

// V1Image pairs an image view with its album reference for the v1 list.
type V1Image struct {
	Image ImageView
	Album *AlbumBrief
}

// V1ImageLister is the optional repository capability backing the v1 image
// list; the concrete image repository implements it.
type V1ImageLister interface {
	ListV1(ctx context.Context, ownerID uint64, page, size int, order, permission, keyword string, albumID uint64) ([]model.Image, int64, error)
}

// ListV1 pages one owner's active images for the Lsky v1 list with its order,
// visibility, keyword and album filters. albumID zero selects only images that
// are not assigned to any album, matching the Lsky quirk.
func (s *ImageService) ListV1(ctx context.Context, ownerID uint64, page, size int, order, permission, keyword string, albumID uint64) ([]V1Image, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, fmt.Errorf("list v1 images: %w", err)
	}
	if ownerID == 0 {
		return nil, 0, ErrInvalidInput
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 40
	}
	lister, ok := s.deps.Images.(V1ImageLister)
	if !ok {
		return nil, 0, fmt.Errorf("v1 image list capability: %w", ErrInvalidInput)
	}
	images, total, err := lister.ListV1(ctx, ownerID, page, size, order, permission, keyword, albumID)
	if err != nil {
		return nil, 0, fmt.Errorf("list v1 images: %w", err)
	}
	items := make([]V1Image, 0, len(images))
	build := s.viewer()
	for _, image := range images {
		view, err := build(ctx, image)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, V1Image{Image: view})
	}
	return items, total, nil
}
