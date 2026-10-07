package service

import (
	"context"
	"errors"
	"slices"
	"testing"
)

type recordingInvalidator struct {
	albums []uint64
	err    error
}

func (r *recordingInvalidator) Invalidate(_ context.Context, albumID uint64) error {
	r.albums = append(r.albums, albumID)
	return r.err
}

func TestUploadInvalidatesAlbumPool(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	pool := &recordingInvalidator{}
	svc.deps.RandomPool = pool
	svc.deps.Albums = &albumStoreStub{}

	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "loose.png"}); err != nil {
		t.Fatal(err)
	}
	if len(pool.albums) != 0 {
		t.Fatalf("upload without an album invalidated %v", pool.albums)
	}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "in-album.png", AlbumID: 5}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pool.albums, []uint64{5}) {
		t.Fatalf("invalidated = %v, want [5]", pool.albums)
	}
}

func TestSetAlbumInvalidatesBothPools(t *testing.T) {
	svc, images, _, subject := albumImageFixture(t)
	pool := &recordingInvalidator{}
	svc.deps.RandomPool = pool
	images.image = activeImage(4)
	images.image.AlbumID = 5

	if _, err := svc.SetAlbum(t.Context(), subject, 4, 6); err != nil {
		t.Fatal(err)
	}
	if len(pool.albums) != 2 || !slices.Contains(pool.albums, 5) || !slices.Contains(pool.albums, 6) {
		t.Fatalf("move 5→6 invalidated %v, want both albums", pool.albums)
	}
	pool.albums = nil
	if _, err := svc.SetAlbum(t.Context(), subject, 4, 0); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pool.albums, []uint64{5}) {
		t.Fatalf("move out of 5 invalidated %v, want [5]", pool.albums)
	}

	pool.albums = nil
	images.setErr = errors.New("write failed")
	if _, err := svc.SetAlbum(t.Context(), subject, 4, 6); err == nil {
		t.Fatal("failed move succeeded")
	}
	if len(pool.albums) != 0 {
		t.Fatalf("failed move invalidated %v", pool.albums)
	}
}

func TestTrashAndRestoreInvalidate(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	pool := &recordingInvalidator{}
	svc.deps.RandomPool = pool
	svc.deps.Albums = &albumStoreStub{}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png", AlbumID: 5})
	if err != nil {
		t.Fatal(err)
	}
	pool.albums = nil
	if err := svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pool.albums, []uint64{5}) {
		t.Fatalf("trash invalidated %v, want [5]", pool.albums)
	}
	pool.albums = nil
	if err := svc.Restore(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pool.albums, []uint64{5}) {
		t.Fatalf("restore invalidated %v, want [5]", pool.albums)
	}
}

func TestAdminTrashInvalidates(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	pool := &recordingInvalidator{}
	svc.deps.RandomPool = pool
	svc.deps.Albums = &albumStoreStub{}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png", AlbumID: 5})
	if err != nil {
		t.Fatal(err)
	}
	pool.albums = nil
	if err := svc.AdminTrash(t.Context(), subject, view.ID); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pool.albums, []uint64{5}) {
		t.Fatalf("admin trash invalidated %v, want [5]", pool.albums)
	}
}

func TestInvalidateFailureDoesNotFailOperation(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	svc.deps.RandomPool = &recordingInvalidator{err: errors.New("pool unavailable")}
	svc.deps.Albums = &albumStoreStub{}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png", AlbumID: 5})
	if err != nil {
		t.Fatalf("upload failed on a pool outage: %v", err)
	}
	if err := svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatalf("trash failed on a pool outage: %v", err)
	}
}

func TestNilRandomPoolIsNoop(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	svc.deps.Albums = &albumStoreStub{}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png", AlbumID: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
}
