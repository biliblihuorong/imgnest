package service

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// GallerySwitch reports whether the public gallery accepts visitors. It is an
// optional settings capability; deployments without it keep the gallery shut.
type GallerySwitch interface {
	GalleryEnabled(ctx context.Context) (bool, error)
}

// GalleryAlbumScope reports whether the gallery is limited to public albums.
// It is optional; without it the gallery lists every public image.
type GalleryAlbumScope interface {
	GalleryPublicAlbumsOnly(ctx context.Context) (bool, error)
}

// GallerySource pages public active images together with uploader names. It
// is an optional repository capability wired by the composition root.
type GallerySource interface {
	ListGallery(ctx context.Context, page, size int, publicAlbumsOnly bool) ([]model.GalleryImage, int64, error)
}

// gallery bounds keep the public listing cheap and cache-friendly.
const (
	galleryMaxPage     = 1000000
	galleryMaxSize     = 100
	galleryDefaultSize = 20
)

func (s *ImageService) albumStore() (AlbumStore, error) {
	if s.deps.Albums == nil {
		return nil, fmt.Errorf("album capability: %w", ErrInvalidInput)
	}
	return s.deps.Albums, nil
}

// SetAlbum moves one owned active image into an owned album, or out of any
// album when albumID is zero. The album must belong to the image's owner so
// the owner-scoped album filter stays consistent.
func (s *ImageService) SetAlbum(ctx context.Context, subject TokenSubject, id, albumID uint64) (ImageView, error) {
	if err := ctx.Err(); err != nil {
		return ImageView{}, fmt.Errorf("move image: %w", err)
	}
	image, err := s.imageByID(ctx, subject, id)
	if err != nil {
		return ImageView{}, err
	}
	if image.State != model.ImageStateActive {
		return ImageView{}, ErrInvalidInput
	}
	albums, err := s.albumStore()
	if err != nil {
		return ImageView{}, err
	}
	if albumID > 0 {
		if _, err := albums.FindOwned(ctx, image.UserID, albumID); err != nil {
			return ImageView{}, fmt.Errorf("move image: %w", err)
		}
	}
	if err := s.deps.Images.SetAlbum(ctx, image.Key, albumID, s.grant(subject)); err != nil {
		return ImageView{}, fmt.Errorf("move image: %w", err)
	}
	image.AlbumID = albumID
	return s.view(ctx, image)
}

// Gallery pages the public gallery. A closed switch, a missing capability, or
// an unreadable setting all serve the same well-formed empty page so callers
// cannot probe switch state; only a repository failure becomes an error.
func (s *ImageService) Gallery(ctx context.Context, page, size int) (GalleryPage, error) {
	if err := ctx.Err(); err != nil {
		return GalleryPage{}, fmt.Errorf("list gallery: %w", err)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = galleryDefaultSize
	}
	if page > galleryMaxPage || size > galleryMaxSize {
		return GalleryPage{}, ErrInvalidInput
	}
	empty := GalleryPage{Items: []GalleryItem{}, Total: 0, Page: page, Size: size}
	switcher, ok := s.deps.Settings.(GallerySwitch)
	if !ok {
		return empty, nil
	}
	enabled, err := switcher.GalleryEnabled(ctx)
	if err != nil || !enabled {
		return empty, nil
	}
	source, ok := s.deps.Images.(GallerySource)
	if !ok {
		return empty, nil
	}
	albumsOnly := false
	if scope, ok := s.deps.Settings.(GalleryAlbumScope); ok {
		if albumsOnly, err = scope.GalleryPublicAlbumsOnly(ctx); err != nil {
			return empty, nil
		}
	}
	images, total, err := source.ListGallery(ctx, page, size, albumsOnly)
	if err != nil {
		return GalleryPage{}, fmt.Errorf("list gallery: %w", err)
	}
	items := make([]GalleryItem, 0, len(images))
	views := s.viewer()
	for _, image := range images {
		view, err := views(ctx, image.Image)
		if err != nil {
			return GalleryPage{}, err
		}
		items = append(items, GalleryItem{ID: view.ID, Name: view.Name, Ext: view.Ext, MIME: view.MIME, Size: view.Size, Width: view.Width, Height: view.Height, Frames: view.Frames, Links: view.Links, CreatedAt: view.CreatedAt, Uploader: image.Uploader})
	}
	return GalleryPage{Items: items, Total: total, Page: page, Size: size}, nil
}
