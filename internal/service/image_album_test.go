package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"github.com/biliblihuorong/imgnest/internal/thumbcache"
)

type albumImagesRepo struct {
	ImageRepository
	image      model.Image
	findErr    error
	setAlbumID uint64
	setKey     string
	setErr     error
	listAlbum  *uint64
	listResult []model.Image
	listTotal  int64
	gallery    []model.GalleryImage
	galleryAll int64
	galleryErr error
}

func (r *albumImagesRepo) FindByID(_ context.Context, id uint64) (model.Image, error) {
	if r.findErr != nil {
		return model.Image{}, r.findErr
	}
	if r.image.ID != id {
		return model.Image{}, ErrNotFound
	}
	return r.image, nil
}

func (r *albumImagesRepo) SetAlbum(_ context.Context, key string, albumID uint64, _ model.TokenGrant) error {
	r.setKey, r.setAlbumID = key, albumID
	return r.setErr
}

func (r *albumImagesRepo) List(_ context.Context, _ uint64, _, _ bool, _, _ int, albumID *uint64) ([]model.Image, int64, error) {
	r.listAlbum = albumID
	return r.listResult, r.listTotal, nil
}

func (r *albumImagesRepo) ListGallery(_ context.Context, _, _ int) ([]model.GalleryImage, int64, error) {
	return r.gallery, r.galleryAll, r.galleryErr
}

type albumStoreStub struct {
	AlbumStore
	albumID   uint64
	calls     []uint64
	findErr   error
	albumName string
}

func (s *albumStoreStub) FindOwned(_ context.Context, ownerID, albumID uint64) (model.Album, error) {
	s.calls = append(s.calls, ownerID, albumID)
	if s.findErr != nil {
		return model.Album{}, s.findErr
	}
	return model.Album{ID: albumID, UserID: ownerID, Name: s.albumName}, nil
}

type gallerySettings struct {
	ImageSettings
	enabled bool
	err     error
}

func (s gallerySettings) TrashDays(context.Context) (int, error) { return 7, nil }
func (s gallerySettings) GalleryEnabled(context.Context) (bool, error) {
	return s.enabled, s.err
}

func albumImageFixture(t *testing.T) (*ImageService, *albumImagesRepo, *albumStoreStub, TokenSubject) {
	t.Helper()
	ctx := t.Context()
	dir := t.TempDir()
	local, err := storage.NewLocal(ctx, filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })
	cache, err := thumbcache.New(ctx, filepath.Join(dir, "thumbs"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	images := &albumImagesRepo{}
	albums := &albumStoreStub{albumName: "博客"}
	policy := uploadPolicy{policy: model.Policy{ID: 1, StorageID: 1, PathTpl: "{Y}/{m}", NameTpl: "{filename}", WebPMode: "both", ScrubMode: "gps", HEIFMode: "webp_only", LinkPrefer: "webp", OnConflict: "rename", WebPQuality: 80, WebPEffort: 4, ThumbEnabled: true, ThumbSize: 400, Enabled: true}, backend: model.Storage{ID: 1, Driver: "local", BaseURL: "http://images.test/i/1", Enabled: true}}
	subject := TokenSubject{userID: 1, passwordHash: "verified"}
	svc, err := NewImageService(ctx, ImageDependencies{Images: images, Policies: &policy, Storages: uploadStorageRepo{backend: policy.backend}, Users: uploadUserRepo{user: model.User{ID: 1, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusEnabled, Role: model.UserRoleUser}}, Tokens: &uploadTokenRepo{}, Drivers: uploadDriverProvider{driver: local}, Paths: PathFunctions{BuildPath: pathtpl.Build, CleanPath: pathtpl.Sanitize}, Imaging: uploadProcessor{format: "png"}, Extractor: uploadMetadata{}, Scrubber: uploadMetadata{}, Cache: cache, Settings: gallerySettings{enabled: true}, Albums: albums, Now: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) }, MaxFileBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	return svc, images, albums, subject
}

func activeImage(id uint64) model.Image {
	return model.Image{ID: id, UserID: 1, Key: "image-key", State: model.ImageStateActive, HasThumb: true}
}

func TestImageListAlbumFilterSemantics(t *testing.T) {
	svc, images, albums, subject := albumImageFixture(t)
	filter := uint64(5)

	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20}); err != nil {
		t.Fatal(err)
	}
	if images.listAlbum != nil {
		t.Fatal("missing album_id still filtered")
	}
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, AlbumID: &filter}); err != nil {
		t.Fatal(err)
	}
	if len(albums.calls) != 2 || albums.calls[0] != uint64(1) || albums.calls[1] != uint64(5) {
		t.Fatalf("album ownership not checked: %v", albums.calls)
	}
	if images.listAlbum == nil || *images.listAlbum != 5 {
		t.Fatalf("album filter lost: %v", images.listAlbum)
	}

	unassigned := uint64(0)
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, AlbumID: &unassigned}); err != nil {
		t.Fatal(err)
	}
	if images.listAlbum == nil || *images.listAlbum != 0 {
		t.Fatalf("unassigned filter lost: %v", images.listAlbum)
	}
	if len(albums.calls) != 2 {
		t.Fatal("explicit zero album ownership should not be checked")
	}

	albums.findErr = ErrNotFound
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, AlbumID: &filter}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing album filter = %v", err)
	}
	albums.findErr = ErrForbidden
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, AlbumID: &filter}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign album filter = %v", err)
	}
}

func TestImageSetAlbumMovesOwnedActiveImage(t *testing.T) {
	svc, images, albums, subject := albumImageFixture(t)
	images.image = activeImage(4)

	view, err := svc.SetAlbum(t.Context(), subject, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if images.setKey != "image-key" || images.setAlbumID != 3 {
		t.Fatalf("repository move = key %q album %d", images.setKey, images.setAlbumID)
	}
	if view.AlbumID != 3 || view.ID != 4 {
		t.Fatalf("moved view = %+v", view)
	}

	if _, err := svc.SetAlbum(t.Context(), subject, 4, 0); err != nil {
		t.Fatal(err)
	}
	if images.setAlbumID != 0 {
		t.Fatalf("move out wrote %d", images.setAlbumID)
	}
	if len(albums.calls) != 2 {
		t.Fatal("move out should not check album ownership")
	}

	cases := []struct {
		name  string
		image model.Image
		find  error
		album uint64
		want  error
	}{
		{"missing image", model.Image{}, ErrNotFound, 3, ErrNotFound},
		{"foreign image", model.Image{ID: 4, UserID: 2, State: model.ImageStateActive}, nil, 3, ErrForbidden},
		{"trashed image", model.Image{ID: 4, UserID: 1, State: model.ImageStateTrash}, nil, 3, ErrInvalidInput},
		{"missing album", activeImage(4), ErrNotFound, 3, ErrNotFound},
		{"foreign album", activeImage(4), ErrForbidden, 3, ErrForbidden},
	}
	for _, tc := range cases {
		images.image, images.findErr, albums.findErr = tc.image, tc.find, tc.find
		_, err := svc.SetAlbum(t.Context(), subject, 4, tc.album)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s = %v, want %v", tc.name, err, tc.want)
		}
	}
	images.image, images.findErr, albums.findErr = activeImage(4), nil, nil
	svc.deps.Albums = nil
	if _, err := svc.SetAlbum(t.Context(), subject, 4, 3); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing album capability = %v", err)
	}
	svc.deps.Albums = albums
	images.setErr = ErrStorage
	if _, err := svc.SetAlbum(t.Context(), subject, 4, 3); !errors.Is(err, ErrStorage) {
		t.Fatalf("repository error swallowed: %v", err)
	}
	if _, err := svc.SetAlbum(canceledContext(), subject, 4, 3); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled move accepted")
	}
}

func TestGallerySwitchControlsVisibility(t *testing.T) {
	svc, images, _, _ := albumImageFixture(t)
	switcher := gallerySettings{enabled: false}
	svc.deps.Settings = switcher

	page, err := svc.Gallery(t.Context(), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("closed gallery page = %+v", page)
	}

	switcher.err = ErrStorage
	svc.deps.Settings = switcher
	page, err = svc.Gallery(t.Context(), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatal("failing switch exposed gallery state")
	}

	switcher.enabled, switcher.err = true, nil
	svc.deps.Settings = switcher
	images.gallery = []model.GalleryImage{
		{Image: activeImage(1), Uploader: "alice"},
		{Image: model.Image{ID: 2, UserID: 0, Key: "guest-key", State: model.ImageStateActive}, Uploader: ""},
	}
	images.galleryAll = 2
	page, err = svc.Gallery(t.Context(), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Page != 1 || page.Size != 20 {
		t.Fatalf("gallery page = %+v", page)
	}
	if page.Items[0].Uploader != "alice" || page.Items[0].ID != 1 {
		t.Fatalf("first gallery item = %+v", page.Items[0])
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"email", "exif", "gps", "ip\"", "password", "registered_ip"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("gallery leaked %s: %s", leak, raw)
		}
	}

	images.galleryErr = ErrStorage
	if _, err = svc.Gallery(t.Context(), 1, 20); !errors.Is(err, ErrStorage) {
		t.Fatalf("repository error swallowed: %v", err)
	}
	images.galleryErr = nil
	if _, err := svc.Gallery(t.Context(), 0, 20); err != nil || page.Page != 1 {
		t.Fatalf("default page = %+v err=%v", page, err)
	}
	if _, err := svc.Gallery(t.Context(), 1, 101); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized page accepted: %v", err)
	}
	if _, err := svc.Gallery(canceledContext(), 1, 20); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled gallery accepted")
	}
}

func TestGalleryWithoutCapabilitiesStaysClosed(t *testing.T) {
	svc, _, _, _ := albumImageFixture(t)
	svc.deps.Settings = uploadSettings{}
	page, err := svc.Gallery(t.Context(), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Items == nil {
		t.Fatalf("capability-less gallery = %+v", page)
	}
}
