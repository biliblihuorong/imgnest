package service

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// AlbumRepository pages and mutates one owner's albums.
type AlbumRepository interface {
	ListByOwner(ctx context.Context, ownerID uint64, page, size int, order, keyword string) ([]model.Album, int64, error)
	FindOwned(ctx context.Context, ownerID, albumID uint64) (model.Album, error)
	CountByOwner(ctx context.Context, ownerID uint64) (int64, error)
	DeleteOwned(ctx context.Context, ownerID, albumID uint64) error
}

// AlbumQuery bounds one owner's album listing.
type AlbumQuery struct {
	Page, Size int
	Order      string
	Keyword    string
}

// AlbumView is one album row for the v1 album list.
type AlbumView struct {
	ID       uint64
	Name     string
	Intro    string
	ImageNum int64
}

// AlbumPage is a paginated album listing.
type AlbumPage struct {
	Items []AlbumView
	Total int64
	Page  int
	Size  int
}

// AlbumService implements the minimal album surface the v1 API needs; full
// album management stays a later milestone.
type AlbumService struct {
	albums AlbumRepository
	now    func() time.Time
}

// NewAlbumService constructs album operations with injected persistence.
func NewAlbumService(ctx context.Context, albums AlbumRepository, now func() time.Time) (*AlbumService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct album service: %w", err)
	}
	if albums == nil || now == nil {
		return nil, fmt.Errorf("album service dependencies: %w", ErrInvalidInput)
	}
	return &AlbumService{albums: albums, now: now}, nil
}

// List pages one owner's albums; unknown orders fall back to newest.
func (s *AlbumService) List(ctx context.Context, ownerID uint64, query AlbumQuery) (AlbumPage, error) {
	if err := ctx.Err(); err != nil {
		return AlbumPage{}, fmt.Errorf("list albums: %w", err)
	}
	if ownerID == 0 {
		return AlbumPage{}, ErrInvalidInput
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Size < 1 {
		query.Size = 40
	}
	albums, total, err := s.albums.ListByOwner(ctx, ownerID, query.Page, query.Size, query.Order, query.Keyword)
	if err != nil {
		return AlbumPage{}, fmt.Errorf("list albums: %w", err)
	}
	items := make([]AlbumView, 0, len(albums))
	for _, album := range albums {
		items = append(items, AlbumView{ID: album.ID, Name: album.Name, Intro: album.Intro, ImageNum: album.ImageCount})
	}
	return AlbumPage{Items: items, Total: total, Page: query.Page, Size: query.Size}, nil
}

// Count returns how many albums one owner has.
func (s *AlbumService) Count(ctx context.Context, ownerID uint64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("count albums: %w", err)
	}
	count, err := s.albums.CountByOwner(ctx, ownerID)
	if err != nil {
		return 0, fmt.Errorf("count albums: %w", err)
	}
	return count, nil
}

// FindOwned resolves one owned album name for image listings.
func (s *AlbumService) FindOwned(ctx context.Context, ownerID, albumID uint64) (AlbumView, error) {
	if err := ctx.Err(); err != nil {
		return AlbumView{}, fmt.Errorf("find album: %w", err)
	}
	album, err := s.albums.FindOwned(ctx, ownerID, albumID)
	if err != nil {
		return AlbumView{}, fmt.Errorf("find album: %w", err)
	}
	return AlbumView{ID: album.ID, Name: album.Name, Intro: album.Intro, ImageNum: album.ImageCount}, nil
}

// Delete removes one owned album; its images stay and become unassigned.
func (s *AlbumService) Delete(ctx context.Context, ownerID, albumID uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete album: %w", err)
	}
	if ownerID == 0 || albumID == 0 {
		return ErrInvalidInput
	}
	if err := s.albums.DeleteOwned(ctx, ownerID, albumID); err != nil {
		return fmt.Errorf("delete album: %w", err)
	}
	return nil
}
