package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type albumRepoStub struct {
	AlbumRepository
	listArgs    []any
	listResult  []model.Album
	listTotal   int64
	findResult  model.Album
	findErr     error
	deleteCalls []uint64
	deleteErr   error
	count       int64
	created     model.Album
	createErr   error
	updated     model.Album
	updateCalls []any
	updateErr   error
}

func (s *albumRepoStub) ListByOwner(_ context.Context, ownerID uint64, page, size int, order, keyword string) ([]model.Album, int64, error) {
	s.listArgs = []any{ownerID, page, size, order, keyword}
	return s.listResult, s.listTotal, nil
}
func (s *albumRepoStub) FindOwned(_ context.Context, _, _ uint64) (model.Album, error) {
	return s.findResult, s.findErr
}
func (s *albumRepoStub) CountByOwner(context.Context, uint64) (int64, error) { return s.count, nil }
func (s *albumRepoStub) DeleteOwned(_ context.Context, ownerID, albumID uint64) error {
	s.deleteCalls = append(s.deleteCalls, ownerID, albumID)
	return s.deleteErr
}
func (s *albumRepoStub) Create(_ context.Context, album model.Album) (model.Album, error) {
	if s.createErr != nil {
		return model.Album{}, s.createErr
	}
	s.created = album
	persisted := album
	persisted.ID = 12
	return persisted, nil
}
func (s *albumRepoStub) Update(_ context.Context, ownerID, albumID uint64, values map[string]any) (model.Album, error) {
	if s.updateErr != nil {
		return model.Album{}, s.updateErr
	}
	s.updateCalls = []any{ownerID, albumID, values}
	persisted := s.updated
	if persisted.ID == 0 {
		persisted = model.Album{ID: albumID, UserID: ownerID, Name: "博客", ImageCount: 3}
	}
	return persisted, nil
}

type albumImageStub struct {
	ImageOwnership
	image model.Image
	err   error
}

func (s *albumImageStub) OwnedActiveImage(context.Context, uint64, uint64) (model.Image, error) {
	return s.image, s.err
}

func newAlbumFixture(t *testing.T, repo AlbumRepository) *AlbumService {
	t.Helper()
	if _, err := NewAlbumService(t.Context(), repo, nil, nil); err == nil {
		t.Fatal("nil clock accepted")
	}
	if _, err := NewAlbumService(t.Context(), repo, &albumImageStub{}, nil); err == nil {
		t.Fatal("nil image ownership accepted")
	}
	svc, err := NewAlbumService(t.Context(), repo, &albumImageStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestAlbumListMappingAndDefaults(t *testing.T) {
	repo := &albumRepoStub{listResult: []model.Album{
		{ID: 3, UserID: 7, Name: "博客", Intro: "配图", ImageCount: 4},
		{ID: 2, UserID: 7, Name: "笔记"},
	}, listTotal: 9}
	svc := newAlbumFixture(t, repo)

	page, err := svc.List(t.Context(), 7, AlbumQuery{})
	if err != nil {
		t.Fatal(err)
	}
	// Normalized paging defaults and the owner pass through to the repository.
	if got := repo.listArgs; got[0] != uint64(7) || got[1] != 1 || got[2] != 40 || got[3] != "" || got[4] != "" {
		t.Fatalf("repository query args = %v", got)
	}
	if page.Total != 9 || page.Page != 1 || page.Size != 40 {
		t.Fatalf("page envelope = %+v", page)
	}
	if len(page.Items) != 2 || page.Items[0].ID != 3 || page.Items[0].Name != "博客" || page.Items[0].Intro != "配图" || page.Items[0].ImageNum != 4 || page.Items[1].ImageNum != 0 {
		t.Fatalf("album views = %+v", page.Items)
	}

	if _, err := svc.List(t.Context(), 0, AlbumQuery{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("guest owner accepted: %v", err)
	}
	if _, err := svc.List(canceledContext(), 7, AlbumQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled list accepted: %v", err)
	}
}

func TestAlbumCreateValidatesAndBuildsCover(t *testing.T) {
	repo := &albumRepoStub{}
	cover := &albumImageStub{image: model.Image{ID: 5, Key: "coverkey", HasThumb: true}}
	svc, err := NewAlbumService(t.Context(), repo, cover, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.Create(t.Context(), 7, AlbumInput{Name: "  博客  ", Intro: "配图", IsPublic: true, CoverImageID: 5})
	if err != nil {
		t.Fatal(err)
	}
	if repo.created.UserID != 7 || repo.created.Name != "博客" || repo.created.Intro != "配图" || !repo.created.IsPublic || repo.created.CoverImageID != 5 {
		t.Fatalf("created album = %+v", repo.created)
	}
	if view.ID != 12 || view.Name != "博客" || !view.IsPublic || view.ImageNum != 0 || view.CoverThumbURL != "/t/coverkey.webp" {
		t.Fatalf("created view = %+v", view)
	}
	raw := mustJSON(t, view)
	for _, field := range []string{"id", "name", "intro", "is_public", "cover_image_id", "image_count", "cover_thumb_url", "created_at", "updated_at"} {
		if !strings.Contains(raw, `"`+field+`"`) {
			t.Fatalf("album view lacks %s: %s", field, raw)
		}
	}

	cases := []AlbumInput{
		{Name: "   "},
		{Name: strings.Repeat("名", 101)},
		{Name: "ok", Intro: strings.Repeat("介", 501)},
		{Name: "ok", CoverImageID: 5},
	}
	cover.err = ErrNotFound
	for index, input := range cases {
		if _, err := svc.Create(t.Context(), 7, input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("case %d accepted invalid input: %v", index, err)
		}
	}
	if _, err := svc.Create(t.Context(), 0, AlbumInput{Name: "ok"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("guest owner create accepted")
	}
	if _, err := svc.Create(canceledContext(), 7, AlbumInput{Name: "ok"}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled create accepted")
	}
	repo.createErr = ErrStorage
	if _, err := svc.Create(t.Context(), 7, AlbumInput{Name: "ok"}); !errors.Is(err, ErrStorage) {
		t.Fatalf("repository error swallowed: %v", err)
	}
}

func TestAlbumCoverWithoutThumbStaysEmpty(t *testing.T) {
	repo := &albumRepoStub{}
	cover := &albumImageStub{image: model.Image{ID: 5, Key: "coverkey"}}
	svc, err := NewAlbumService(t.Context(), repo, cover, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.Create(t.Context(), 7, AlbumInput{Name: "博客", CoverImageID: 5})
	if err != nil {
		t.Fatal(err)
	}
	if view.CoverThumbURL != "" {
		t.Fatalf("cover without thumb = %q", view.CoverThumbURL)
	}
}

func TestAlbumUpdatePartialAndOwnership(t *testing.T) {
	public := true
	zero := uint64(0)
	repo := &albumRepoStub{updated: model.Album{ID: 5, UserID: 7, Name: "新名", ImageCount: 3, CoverImageID: 9, CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}}
	cover := &albumImageStub{image: model.Image{ID: 9, Key: "coverkey", HasThumb: true}}
	svc, err := NewAlbumService(t.Context(), repo, cover, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	view, err := svc.Update(t.Context(), 7, 5, AlbumPatch{Name: strPtr("新名"), IsPublic: &public, CoverImageID: uintPtr(9)})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.updateCalls) != 3 || repo.updateCalls[0] != uint64(7) || repo.updateCalls[1] != uint64(5) {
		t.Fatalf("update args = %v", repo.updateCalls)
	}
	changes := repo.updateCalls[2].(map[string]any)
	if changes["name"] != "新名" || changes["is_public"] != true || changes["cover_image_id"] != uint64(9) {
		t.Fatalf("update values = %v", changes)
	}
	if view.ImageNum != 3 || view.CoverThumbURL != "/t/coverkey.webp" || view.UpdatedAt.IsZero() {
		t.Fatalf("updated view = %+v", view)
	}

	// Cover cleared by an explicit zero.
	if _, err := svc.Update(t.Context(), 7, 5, AlbumPatch{CoverImageID: &zero}); err != nil {
		t.Fatal(err)
	}
	if cleared := repo.updateCalls[2].(map[string]any); cleared["cover_image_id"] != uint64(0) {
		t.Fatalf("cover clear values = %v", cleared)
	}

	for _, patch := range []AlbumPatch{
		{},
		{Name: strPtr("  ")},
		{Name: strPtr(strings.Repeat("名", 101))},
		{Intro: strPtr(strings.Repeat("介", 501))},
		{CoverImageID: uintPtr(9)},
	} {
		if patch.CoverImageID != nil && *patch.CoverImageID == 9 {
			cover.err = ErrForbidden
		} else {
			cover.err = nil
		}
		if _, err := svc.Update(t.Context(), 7, 5, patch); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid patch accepted: %+v err=%v", patch, err)
		}
	}

	repo.findErr = ErrNotFound
	if _, err := svc.Update(t.Context(), 7, 5, AlbumPatch{Name: strPtr("新名")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing album update = %v", err)
	}
	repo.findErr = ErrForbidden
	if _, err := svc.Update(t.Context(), 7, 5, AlbumPatch{Name: strPtr("新名")}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign album update = %v", err)
	}
	repo.findErr = nil
	repo.updateErr = ErrStorage
	if _, err := svc.Update(t.Context(), 7, 5, AlbumPatch{Name: strPtr("新名")}); !errors.Is(err, ErrStorage) {
		t.Fatalf("repository error swallowed: %v", err)
	}
	if _, err := svc.Update(t.Context(), 0, 5, AlbumPatch{Name: strPtr("新名")}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("guest owner update accepted")
	}
}

func TestAlbumDeleteAndFind(t *testing.T) {
	repo := &albumRepoStub{findResult: model.Album{ID: 5, UserID: 7, Name: "博客", ImageCount: 2}, count: 4}
	svc := newAlbumFixture(t, repo)

	found, err := svc.FindOwned(t.Context(), 7, 5)
	if err != nil || found.ID != 5 || found.Name != "博客" || found.ImageNum != 2 {
		t.Fatalf("FindOwned = %+v err=%v", found, err)
	}
	if count, err := svc.Count(t.Context(), 7); err != nil || count != 4 {
		t.Fatalf("Count = %d err=%v", count, err)
	}

	if err := svc.Delete(t.Context(), 7, 5); err != nil {
		t.Fatal(err)
	}
	if got := repo.deleteCalls; got[0] != uint64(7) || got[1] != uint64(5) {
		t.Fatalf("delete args = %v", got)
	}
	if err := svc.Delete(t.Context(), 0, 5); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("guest owner delete accepted: %v", err)
	}
	if err := svc.Delete(t.Context(), 7, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero album delete accepted: %v", err)
	}
	repo.deleteErr = ErrNotFound
	if err := svc.Delete(t.Context(), 7, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repository error swallowed: %v", err)
	}
}

func TestAlbumServiceRejectsMissingDependencies(t *testing.T) {
	if _, err := NewAlbumService(t.Context(), nil, nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil repository accepted: %v", err)
	}
	if _, err := NewAlbumService(t.Context(), &albumRepoStub{}, nil, time.Now); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil image ownership accepted: %v", err)
	}
	if _, err := NewAlbumService(canceledContext(), &albumRepoStub{}, &albumImageStub{}, time.Now); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled construction accepted: %v", err)
	}
}

func strPtr(value string) *string  { return &value }
func uintPtr(value uint64) *uint64 { return &value }

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
