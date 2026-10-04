package service

import (
	"context"
	"errors"
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
	deleteCalls []uint64
	deleteErr   error
	count       int64
}

func (s *albumRepoStub) ListByOwner(_ context.Context, ownerID uint64, page, size int, order, keyword string) ([]model.Album, int64, error) {
	s.listArgs = []any{ownerID, page, size, order, keyword}
	return s.listResult, s.listTotal, nil
}
func (s *albumRepoStub) FindOwned(_ context.Context, _, _ uint64) (model.Album, error) {
	return s.findResult, nil
}
func (s *albumRepoStub) CountByOwner(context.Context, uint64) (int64, error) { return s.count, nil }
func (s *albumRepoStub) DeleteOwned(_ context.Context, ownerID, albumID uint64) error {
	s.deleteCalls = append(s.deleteCalls, ownerID, albumID)
	return s.deleteErr
}

func newAlbumFixture(t *testing.T, repo AlbumRepository) *AlbumService {
	t.Helper()
	if _, err := NewAlbumService(t.Context(), repo, nil); err == nil {
		t.Fatal("nil clock accepted")
	}
	svc, err := NewAlbumService(t.Context(), repo, time.Now)
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
	if _, err := NewAlbumService(t.Context(), nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil repository accepted: %v", err)
	}
	if _, err := NewAlbumService(canceledContext(), &albumRepoStub{}, time.Now); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled construction accepted: %v", err)
	}
}
