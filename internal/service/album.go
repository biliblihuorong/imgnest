package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// Album limits keep names and intros inside the frozen native contract.
const (
	AlbumNameMaxRunes  = 100
	AlbumIntroMaxRunes = 500
)

// AlbumRepository pages and mutates one owner's albums.
type AlbumRepository interface {
	ListByOwner(ctx context.Context, ownerID uint64, page, size int, order, keyword string) ([]model.Album, int64, error)
	FindOwned(ctx context.Context, ownerID, albumID uint64) (model.Album, error)
	CountByOwner(ctx context.Context, ownerID uint64) (int64, error)
	DeleteOwned(ctx context.Context, ownerID, albumID uint64) error
	Create(ctx context.Context, album model.Album) (model.Album, error)
	Update(ctx context.Context, ownerID, albumID uint64, values map[string]any) (model.Album, error)
}

// ImageOwnership resolves images an album cover may reference. A cover must
// exist, belong to the owner, and still be active.
type ImageOwnership interface {
	OwnedActiveImage(ctx context.Context, ownerID, imageID uint64) (model.Image, error)
}

// AlbumQuery bounds one owner's album listing.
type AlbumQuery struct {
	Page, Size int
	Order      string
	Keyword    string
}

// AlbumInput is the payload for creating one album.
type AlbumInput struct {
	Name         string
	Intro        string
	IsPublic     bool
	CoverImageID uint64
}

// AlbumPatch carries partial updates; nil pointers leave fields unchanged.
type AlbumPatch struct {
	Name         *string
	Intro        *string
	IsPublic     *bool
	CoverImageID *uint64
}

// AlbumView is one album row. The v1 album list maps a subset of these fields
// into its own wire type, so the JSON tags serve the native API only.
type AlbumView struct {
	ID            uint64    `json:"id"`
	Name          string    `json:"name"`
	Intro         string    `json:"intro"`
	IsPublic      bool      `json:"is_public"`
	CoverImageID  uint64    `json:"cover_image_id"`
	ImageNum      int64     `json:"image_count"`
	CoverThumbURL string    `json:"cover_thumb_url"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// AlbumPage is a paginated album listing.
type AlbumPage struct {
	Items []AlbumView
	Total int64
	Page  int
	Size  int
}

// AlbumService implements album ownership, covers and detachment rules shared
// by the native and v1 APIs.
type AlbumService struct {
	albums AlbumRepository
	images ImageOwnership
	now    func() time.Time
}

// NewAlbumService constructs album operations with injected persistence.
func NewAlbumService(ctx context.Context, albums AlbumRepository, images ImageOwnership, now func() time.Time) (*AlbumService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct album service: %w", err)
	}
	if albums == nil || images == nil || now == nil {
		return nil, fmt.Errorf("album service dependencies: %w", ErrInvalidInput)
	}
	return &AlbumService{albums: albums, images: images, now: now}, nil
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
		view, err := s.view(ctx, ownerID, album)
		if err != nil {
			return AlbumPage{}, err
		}
		items = append(items, view)
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

// FindOwned resolves one owned album for image listings.
func (s *AlbumService) FindOwned(ctx context.Context, ownerID, albumID uint64) (AlbumView, error) {
	if err := ctx.Err(); err != nil {
		return AlbumView{}, fmt.Errorf("find album: %w", err)
	}
	album, err := s.albums.FindOwned(ctx, ownerID, albumID)
	if err != nil {
		return AlbumView{}, fmt.Errorf("find album: %w", err)
	}
	return s.view(ctx, ownerID, album)
}

// Create validates and persists one owned album; a nonzero cover must point
// at one of the owner's active images.
func (s *AlbumService) Create(ctx context.Context, ownerID uint64, input AlbumInput) (AlbumView, error) {
	if err := ctx.Err(); err != nil {
		return AlbumView{}, fmt.Errorf("create album: %w", err)
	}
	if ownerID == 0 {
		return AlbumView{}, ErrInvalidInput
	}
	name, intro, cover, err := s.validatedCover(ctx, ownerID, input.Name, input.Intro, input.CoverImageID)
	if err != nil {
		return AlbumView{}, err
	}
	album, err := s.albums.Create(ctx, model.Album{
		UserID: ownerID, Name: name, Intro: intro, IsPublic: input.IsPublic, CoverImageID: cover,
	})
	if err != nil {
		return AlbumView{}, fmt.Errorf("create album: %w", err)
	}
	return s.view(ctx, ownerID, album)
}

// Update applies a partial patch to one owned album and returns the fresh
// live-counted view. Existing-but-foreign albums reject with ErrForbidden.
func (s *AlbumService) Update(ctx context.Context, ownerID, albumID uint64, patch AlbumPatch) (AlbumView, error) {
	if err := ctx.Err(); err != nil {
		return AlbumView{}, fmt.Errorf("update album: %w", err)
	}
	if ownerID == 0 || albumID == 0 {
		return AlbumView{}, ErrInvalidInput
	}
	// The ownership check doubles as the missing/foreign gate for the patch.
	if _, err := s.albums.FindOwned(ctx, ownerID, albumID); err != nil {
		return AlbumView{}, fmt.Errorf("update album: %w", err)
	}
	values := map[string]any{}
	if patch.Name != nil {
		name, err := validAlbumName(*patch.Name)
		if err != nil {
			return AlbumView{}, err
		}
		values["name"] = name
	}
	if patch.Intro != nil {
		intro, err := validAlbumIntro(*patch.Intro)
		if err != nil {
			return AlbumView{}, err
		}
		values["intro"] = intro
	}
	if patch.IsPublic != nil {
		values["is_public"] = *patch.IsPublic
	}
	if patch.CoverImageID != nil {
		resolved, err := s.resolveCover(ctx, ownerID, *patch.CoverImageID)
		if err != nil {
			return AlbumView{}, err
		}
		values["cover_image_id"] = resolved
	}
	if len(values) == 0 {
		return AlbumView{}, ErrInvalidInput
	}
	album, err := s.albums.Update(ctx, ownerID, albumID, values)
	if err != nil {
		return AlbumView{}, fmt.Errorf("update album: %w", err)
	}
	return s.view(ctx, ownerID, album)
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

func (s *AlbumService) validatedCover(ctx context.Context, ownerID uint64, name, intro string, cover uint64) (string, string, uint64, error) {
	validName, err := validAlbumName(name)
	if err != nil {
		return "", "", 0, err
	}
	validIntro, err := validAlbumIntro(intro)
	if err != nil {
		return "", "", 0, err
	}
	resolved, err := s.resolveCover(ctx, ownerID, cover)
	if err != nil {
		return "", "", 0, err
	}
	return validName, validIntro, resolved, nil
}

// resolveCover maps cover_image_id 0 to "no cover" and otherwise requires one
// of the owner's active images; every mismatch reports invalid input so the
// cover selector reveals nothing about other accounts' images.
func (s *AlbumService) resolveCover(ctx context.Context, ownerID, cover uint64) (uint64, error) {
	if cover == 0 {
		return 0, nil
	}
	if _, err := s.images.OwnedActiveImage(ctx, ownerID, cover); err != nil {
		return 0, fmt.Errorf("resolve album cover: %w", ErrInvalidInput)
	}
	return cover, nil
}

func (s *AlbumService) view(ctx context.Context, ownerID uint64, album model.Album) (AlbumView, error) {
	view := AlbumView{
		ID: album.ID, Name: album.Name, Intro: album.Intro, IsPublic: album.IsPublic,
		CoverImageID: album.CoverImageID, ImageNum: album.ImageCount,
		CreatedAt: album.CreatedAt.UTC(), UpdatedAt: album.UpdatedAt.UTC(),
	}
	if album.CoverImageID != 0 {
		if cover, err := s.images.OwnedActiveImage(ctx, ownerID, album.CoverImageID); err == nil && cover.HasThumb {
			view.CoverThumbURL = "/t/" + cover.Key + ".webp"
		}
	}
	return view, nil
}

func validAlbumName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > AlbumNameMaxRunes {
		return "", ErrInvalidInput
	}
	return trimmed, nil
}

func validAlbumIntro(intro string) (string, error) {
	if utf8.RuneCountInString(intro) > AlbumIntroMaxRunes {
		return "", ErrInvalidInput
	}
	return intro, nil
}
