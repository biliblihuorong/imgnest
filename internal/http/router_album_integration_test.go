//go:build integration

package http_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/exif"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"github.com/biliblihuorong/imgnest/internal/thumbcache"
	"gorm.io/gorm"
)

// These are in-process route integration tests, not browser or external
// database tests. The migrated SQLite database, authentication, image services,
// libvips/EXIF processors and local object driver are all real.
type albumHTTPFixture struct {
	db          *gorm.DB
	router      http.Handler
	users       [2]service.UserView
	tokens      [2]string
	storageRoot string
}

func newAlbumHTTPFixture(t *testing.T) *albumHTTPFixture {
	t.Helper()
	ctx := t.Context()
	db := imageDatabase(t, "sqlite")
	userRepo, err := repo.NewUserRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	tokenRepo, err := repo.NewTokenRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	users, err := service.NewUserService(ctx, userRepo, settings)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := service.NewTokenService(ctx, tokenRepo, userRepo, settings, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	f := &albumHTTPFixture{db: db}
	f.setting(t, "registration_enabled", "true")
	const password = "album-route-test-password" // #nosec G101 -- invented isolated-test credential.
	for i, name := range []string{"alice", "other"} {
		f.users[i], err = users.Register(ctx, service.RegisterInput{Username: name, Email: name + "@example.test", Password: password})
		if err != nil {
			t.Fatal(err)
		}
	}
	storages, err := repo.NewStorageRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := storages.Create(ctx, model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storages.SetBaseURL(ctx, backend.ID, "http://images.test/i/"+strconv.FormatUint(backend.ID, 10)); err != nil {
		t.Fatal(err)
	}
	policies, err := repo.NewPolicyRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	policy := sitePolicyListing("album pipeline", backend.ID)
	policy.NameTpl, policy.SkipIfLarger = "{filename}", false
	if _, err := policies.CreateAndBind(ctx, policy, f.users[0].GroupID, true); err != nil {
		t.Fatal(err)
	}
	images, err := repo.NewImageRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	albums, err := repo.NewAlbumRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	albumService, err := service.NewAlbumService(ctx, albums, images, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	f.storageRoot = filepath.Join(t.TempDir(), "objects")
	local, err := storage.NewLocal(ctx, f.storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	cache, err := thumbcache.New(ctx, filepath.Join(t.TempDir(), "thumbs"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cache.Close(); err != nil {
			t.Error(err)
		}
	})
	processor, err := imaging.NewProcessor(ctx, 100000000, 1)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := exif.NewProcessor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	imageService, err := service.NewImageService(ctx, service.ImageDependencies{
		Images: images, Policies: policies, Storages: storages, Users: userRepo,
		Tokens: tokenRepo, Albums: albums, Drivers: realImageProvider{local},
		Paths:   service.PathFunctions{BuildPath: pathtpl.Build, CleanPath: pathtpl.Sanitize},
		Imaging: processor, Extractor: metadata, Scrubber: metadata, Cache: cache,
		Settings: settings, Now: time.Now, MaxFileBytes: 20 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	f.router, err = httpapi.NewRouter(ctx, httpapi.Dependencies{
		Users: users, Tokens: tokens, Images: imageService, Albums: albumService,
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Health: sqlDB.PingContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, user := range f.users {
		f.tokens[i] = loginForSite(t, f.router, user.Email, password)
	}
	return f
}

func (f *albumHTTPFixture) setting(t *testing.T, name, value string) {
	t.Helper()
	result := f.db.Model(&model.Setting{}).Where("key = ?", name).Update("value", value)
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("set test setting %s: affected=%d err=%v", name, result.RowsAffected, result.Error)
	}
}

func albumHTTPData[T any](t *testing.T, response *httptest.ResponseRecorder, status int) T {
	t.Helper()
	expectCode(t, response, status, 0)
	var value T
	if err := json.Unmarshal(envelope(t, response)["data"], &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func (f *albumHTTPFixture) upload(t *testing.T, owner int, name string, album uint64, public bool) service.ImageView {
	t.Helper()
	entries := []multipartEntry{{"file", name, string(privateJPEG(t))}, {"is_public", "", strconv.FormatBool(public)}}
	if album > 0 {
		entries = append(entries, multipartEntry{"album_id", "", strconv.FormatUint(album, 10)})
	}
	body, contentType := multipartBody(t, entries...)
	view := albumHTTPData[service.ImageView](t, httptestImageUpload(f.router, body, contentType, f.tokens[owner]), 201)
	if view.ID == 0 || view.AlbumID != album || view.IsPublic != public || view.Ext != "jpg" || !view.HasOriginal || !view.HasWebP || !view.HasThumb || view.ChargedBytes <= 0 {
		t.Fatalf("unexpected uploaded image identity/variants: %+v", view)
	}
	return view
}

func (f *albumHTTPFixture) album(t *testing.T, owner int, name string, cover uint64) service.AlbumView {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"cover_image_id":%d}`, name, cover)
	return albumHTTPData[service.AlbumView](t, request(t, f.router, "POST", "/api/albums", body, f.tokens[owner]), 201)
}

type albumHTTPBatchResult struct {
	ID     uint64            `json:"id"`
	Status int               `json:"status"`
	Code   int               `json:"code"`
	Data   service.ImageView `json:"data"`
}

func (f *albumHTTPFixture) move(t *testing.T, id, album uint64) {
	t.Helper()
	body := fmt.Sprintf(`{"action":"album","ids":[%d],"album_id":%d}`, id, album)
	results := albumHTTPData[[]albumHTTPBatchResult](t, request(t, f.router, "POST", "/api/images/batch", body, f.tokens[0]), 207)
	if len(results) != 1 || results[0].ID != id || results[0].Status != 200 || results[0].Code != 0 || results[0].Data.AlbumID != album {
		t.Fatalf("move result = %+v", results)
	}
}

func (f *albumHTTPFixture) counts(t *testing.T, want map[uint64]int64) {
	t.Helper()
	page := albumHTTPData[service.AlbumPage](t, request(t, f.router, "GET", "/api/albums", "", f.tokens[0]), 200)
	if page.Total != int64(len(want)) || len(page.Items) != len(want) {
		t.Fatalf("album page has total=%d items=%d, want %d", page.Total, len(page.Items), len(want))
	}
	for _, album := range page.Items {
		count, ok := want[album.ID]
		if !ok || album.ImageNum != count {
			t.Fatalf("album %d live count=%d, want=%d present=%v", album.ID, album.ImageNum, count, ok)
		}
		var stored model.Album
		if err := f.db.First(&stored, album.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.ImageCount != count {
			t.Fatalf("album %d stored count=%d, want=%d", album.ID, stored.ImageCount, count)
		}
	}
}

func (f *albumHTTPFixture) imageIDs(t *testing.T, path string, ids ...uint64) {
	t.Helper()
	page := albumHTTPData[service.ImagePage](t, request(t, f.router, "GET", path, "", f.tokens[0]), 200)
	got := make([]uint64, 0, len(page.Items))
	for _, image := range page.Items {
		got = append(got, image.ID)
	}
	slices.Sort(got)
	want := slices.Clone(ids)
	slices.Sort(want)
	if page.Total != int64(len(want)) || !slices.Equal(got, want) {
		t.Fatalf("%s image IDs=%v total=%d, want=%v", path, got, page.Total, want)
	}
}

func (f *albumHTTPFixture) usedBytes(t *testing.T, owner int, want int64) {
	t.Helper()
	var user model.User
	if err := f.db.First(&user, f.users[owner].ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.UsedBytes != want {
		t.Fatalf("owner %d charged bytes=%d, want=%d", owner, user.UsedBytes, want)
	}
}

func (f *albumHTTPFixture) storedObjects(t *testing.T) map[string][sha256.Size]byte {
	t.Helper()
	root, err := os.OpenRoot(f.storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	objects := map[string][sha256.Size]byte{}
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		objects[path] = sha256.Sum256(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return objects
}

func TestRealAlbumHTTPMediaLifecycle(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	first := f.upload(t, 0, "旅行.png", 0, false)
	a := f.album(t, 0, "旅行相册", first.ID)
	b := f.album(t, 0, "空相册", 0)
	if a.CoverImageID != first.ID || a.CoverThumbURL != first.LocalThumbURL || b.CoverImageID != 0 || b.CoverThumbURL != "" {
		t.Fatal("album creation did not retain the selected cover")
	}

	// A frontend sends zero to clear a cover, while the real migrated foreign
	// key must receive SQL NULL. Repeat the patch to cover saving a bare album.
	for range 2 {
		cleared := albumHTTPData[service.AlbumView](t, request(t, f.router, "PATCH", fmt.Sprintf("/api/albums/%d", a.ID), `{"cover_image_id":0}`, f.tokens[0]), 200)
		if cleared.CoverImageID != 0 || cleared.CoverThumbURL != "" {
			t.Fatal("cleared cover remains in album response")
		}
		var cover sql.NullInt64
		if err := f.db.Raw("SELECT cover_image_id FROM albums WHERE id = ?", a.ID).Row().Scan(&cover); err != nil {
			t.Fatal(err)
		}
		if cover.Valid {
			t.Fatal("cleared cover is not SQL NULL")
		}
	}

	// Repeating the same file and filename must allocate a distinct path, not
	// overwrite the first object's ownership or charge the same row twice.
	second := f.upload(t, 0, "旅行.png", a.ID, false)
	if first.ID == second.ID || first.Key == second.Key || first.Links.Original == second.Links.Original {
		t.Fatal("repeated upload reused an existing image or object path")
	}
	f.counts(t, map[uint64]int64{a.ID: 1, b.ID: 0})
	f.move(t, first.ID, a.ID)
	f.move(t, first.ID, a.ID)
	f.counts(t, map[uint64]int64{a.ID: 2, b.ID: 0})
	f.imageIDs(t, "/api/images", first.ID, second.ID)
	f.imageIDs(t, "/api/images?album_id=0")
	f.imageIDs(t, fmt.Sprintf("/api/images?album_id=%d", a.ID), first.ID, second.ID)

	// Reproduce an old deployment's persisted counter drift, then repair both
	// affected albums through the public batch-move route.
	for id, count := range map[uint64]int64{a.ID: 0, b.ID: 9} {
		if err := f.db.Model(&model.Album{}).Where("id = ?", id).Update("image_count", count).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.move(t, first.ID, b.ID)
	f.counts(t, map[uint64]int64{a.ID: 1, b.ID: 1})
	f.imageIDs(t, fmt.Sprintf("/api/images?album_id=%d", b.ID), first.ID)
	f.usedBytes(t, 0, first.ChargedBytes+second.ChargedBytes)

	objects := map[string][]byte{}
	for _, link := range []string{first.Links.Original, first.Links.WebP, first.Links.Thumbnail} {
		parsed, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		response := request(t, f.router, "GET", parsed.RequestURI(), "", "")
		if response.Code != 200 || response.Body.Len() == 0 {
			t.Fatalf("uploaded object unavailable: HTTP %d", response.Code)
		}
		objects[parsed.RequestURI()] = bytes.Clone(response.Body.Bytes())
	}
	if err := f.db.Model(&model.Album{}).Where("id = ?", b.ID).Update("image_count", 0).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		expectCode(t, request(t, f.router, "DELETE", fmt.Sprintf("/api/images/%d", first.ID), "", f.tokens[0]), 200, 0)
		f.counts(t, map[uint64]int64{a.ID: 1, b.ID: 0})
		f.usedBytes(t, 0, second.ChargedBytes)
	}
	for path := range objects {
		expectCode(t, request(t, f.router, "GET", path, "", ""), 404, 10001)
	}
	f.imageIDs(t, "/api/images", second.ID)
	f.imageIDs(t, fmt.Sprintf("/api/trash?album_id=%d", b.ID), first.ID)
	if err := f.db.Model(&model.Album{}).Where("id = ?", b.ID).Update("image_count", 9).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		expectSuccessfulImageBatch(t, request(t, f.router, "POST", "/api/trash/restore", fmt.Sprintf(`{"ids":[%d]}`, first.ID), f.tokens[0]))
		f.counts(t, map[uint64]int64{a.ID: 1, b.ID: 1})
		f.usedBytes(t, 0, first.ChargedBytes+second.ChargedBytes)
	}
	for path, before := range objects {
		response := request(t, f.router, "GET", path, "", "")
		if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), before) {
			t.Fatalf("restore changed original/derivative bytes at %s", path)
		}
	}
	f.imageIDs(t, "/api/trash")
	f.imageIDs(t, fmt.Sprintf("/api/images?album_id=%d", b.ID), first.ID)

	// Deleting an album must retain its images, objects and charged capacity.
	expectCode(t, request(t, f.router, "DELETE", fmt.Sprintf("/api/albums/%d", b.ID), "", f.tokens[0]), 200, 0)
	expectCode(t, request(t, f.router, "DELETE", fmt.Sprintf("/api/albums/%d", b.ID), "", f.tokens[0]), 404, 10001)
	f.counts(t, map[uint64]int64{a.ID: 1})
	f.imageIDs(t, "/api/images?album_id=0", first.ID)
	f.usedBytes(t, 0, first.ChargedBytes+second.ChargedBytes)
	for path, before := range objects {
		response := request(t, f.router, "GET", path, "", "")
		if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), before) {
			t.Fatal("album deletion removed or changed an image object")
		}
	}
}

func TestRealAlbumHTTPPrivacyAndRejectedMutations(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	own := f.upload(t, 0, "owner.jpg", 0, false)
	foreign := f.upload(t, 1, "foreign.jpg", 0, false)
	a := f.album(t, 0, "Owner album", own.ID)
	b := f.album(t, 1, "Other album", foreign.ID)
	f.move(t, own.ID, a.ID)
	imagePath := fmt.Sprintf("/api/images/%d", own.ID)
	albumPath := fmt.Sprintf("/api/albums/%d", a.ID)
	for _, tc := range []struct {
		name, method, path, body string
		status, code             int
	}{
		{"image details", "GET", imagePath, "", 403, 20003},
		{"private EXIF", "GET", imagePath + "/exif", "", 403, 20003},
		{"image permission", "PATCH", imagePath, `{"is_public":true}`, 403, 20003},
		{"image deletion", "DELETE", imagePath, "", 403, 20003},
		{"album update", "PATCH", albumPath, `{"name":"stolen"}`, 403, 20003},
		{"album deletion", "DELETE", albumPath, "", 403, 20003},
		{"album filter", "GET", fmt.Sprintf("/api/images?album_id=%d", a.ID), "", 403, 20003},
		{"foreign cover create", "POST", "/api/albums", fmt.Sprintf(`{"name":"invalid","cover_image_id":%d}`, own.ID), 400, 10001},
		{"foreign cover update", "PATCH", fmt.Sprintf("/api/albums/%d", b.ID), fmt.Sprintf(`{"cover_image_id":%d}`, own.ID), 400, 10001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectCode(t, request(t, f.router, tc.method, tc.path, tc.body, f.tokens[1]), tc.status, tc.code)
		})
	}
	expectCode(t, request(t, f.router, "GET", own.LocalThumbURL, "", ""), 401, 20001)
	expectCode(t, request(t, f.router, "GET", own.LocalThumbURL, "", f.tokens[1]), 403, 20003)
	ownerExif := albumHTTPData[model.ImageExif](t, request(t, f.router, "GET", imagePath+"/exif", "", f.tokens[0]), 200)
	if ownerExif.GPSLat == nil || ownerExif.GPSLng == nil || len(ownerExif.Raw) == 0 {
		t.Fatal("real EXIF archive lost owner-only metadata")
	}
	for _, action := range []string{"restore", "purge"} {
		body := fmt.Sprintf(`{"ids":[%d]}`, own.ID)
		results := albumHTTPData[[]albumHTTPBatchResult](t, request(t, f.router, "POST", "/api/trash/"+action, body, f.tokens[1]), 207)
		if len(results) != 1 || results[0].Status != 403 || results[0].Code != 20003 {
			t.Fatalf("foreign %s result = %+v", action, results)
		}
	}

	for _, tc := range []struct{ name, path, body string }{
		{"duplicate move", "/api/images/batch", fmt.Sprintf(`{"action":"album","ids":[%d,%d],"album_id":0}`, own.ID, own.ID)},
		{"mixed action fields", "/api/images/batch", fmt.Sprintf(`{"action":"album","ids":[%d],"is_public":true}`, own.ID)},
		{"zero image ID", "/api/images/batch", `{"action":"album","ids":[0],"album_id":0}`},
		{"empty selection", "/api/images/batch", `{"action":"album","ids":[],"album_id":0}`},
		{"duplicate restore", "/api/trash/restore", fmt.Sprintf(`{"ids":[%d,%d]}`, own.ID, own.ID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expectCode(t, request(t, f.router, "POST", tc.path, tc.body, f.tokens[0]), 400, 10001)
			f.counts(t, map[uint64]int64{a.ID: 1})
			f.imageIDs(t, fmt.Sprintf("/api/images?album_id=%d", a.ID), own.ID)
		})
	}

	// A foreign destination rejects before membership or quota can change.
	badMove := fmt.Sprintf(`{"action":"album","ids":[%d],"album_id":%d}`, own.ID, b.ID)
	result := albumHTTPData[[]albumHTTPBatchResult](t, request(t, f.router, "POST", "/api/images/batch", badMove, f.tokens[0]), 207)
	if len(result) != 1 || result[0].Code != 20003 || result[0].Status != 403 {
		t.Fatalf("foreign album move = %+v", result)
	}
	objectsBefore := f.storedObjects(t)
	body, contentType := multipartBody(t, multipartEntry{"file", "rejected.jpg", string(privateJPEG(t))}, multipartEntry{"album_id", "", strconv.FormatUint(b.ID, 10)})
	expectCode(t, httptestImageUpload(f.router, body, contentType, f.tokens[0]), 403, 20003)
	if !maps.Equal(objectsBefore, f.storedObjects(t)) {
		t.Fatal("rejected upload changed stored objects or left orphan files")
	}
	var imageCount int64
	if err := f.db.Model(&model.Image{}).Count(&imageCount).Error; err != nil || imageCount != 2 {
		t.Fatalf("rejected upload created image rows: count=%d err=%v", imageCount, err)
	}
	f.counts(t, map[uint64]int64{a.ID: 1})
	f.usedBytes(t, 0, own.ChargedBytes)
	f.usedBytes(t, 1, foreign.ChargedBytes)
	unchanged := albumHTTPData[service.ImageView](t, request(t, f.router, "GET", imagePath, "", f.tokens[0]), 200)
	if unchanged.IsPublic || unchanged.AlbumID != a.ID || unchanged.DeletedAt != nil {
		t.Fatal("rejected mutation changed the owner's image")
	}
	var unchangedAlbum model.Album
	if err := f.db.First(&unchangedAlbum, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchangedAlbum.Name != "Owner album" || unchangedAlbum.CoverImageID != own.ID || unchangedAlbum.IsPublic {
		t.Fatal("rejected mutation changed the owner's album")
	}

	// A mixed batch commits its permitted item and reports each rejected item
	// separately; callers must not treat outer 207/code=0 as all-success.
	bodyJSON := fmt.Sprintf(`{"action":"album","ids":[%d,%d,999999],"album_id":0}`, own.ID, foreign.ID)
	results := albumHTTPData[[]albumHTTPBatchResult](t, request(t, f.router, "POST", "/api/images/batch", bodyJSON, f.tokens[0]), 207)
	if len(results) != 3 {
		t.Fatalf("mixed batch returned %d results", len(results))
	}
	for i, want := range []struct {
		id           uint64
		status, code int
	}{{own.ID, 200, 0}, {foreign.ID, 403, 20003}, {999999, 404, 10001}} {
		if got := results[i]; got.ID != want.id || got.Status != want.status || got.Code != want.code {
			t.Fatalf("mixed batch item %d = %+v, want %+v", i, got, want)
		}
	}
	f.counts(t, map[uint64]int64{a.ID: 0})
	f.imageIDs(t, "/api/images?album_id=0", own.ID)
	f.usedBytes(t, 0, own.ChargedBytes)
	f.usedBytes(t, 1, foreign.ChargedBytes)
}

func TestRealAlbumHTTPGalleryVisibility(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	privateAlbum := f.album(t, 0, "Private album", 0)
	private := f.upload(t, 0, "private.jpg", privateAlbum.ID, false)
	public := f.upload(t, 0, "public.jpg", privateAlbum.ID, true)
	other := f.upload(t, 1, "other-public.jpg", 0, true)
	gallery := func(t *testing.T, ids ...uint64) service.GalleryPage {
		t.Helper()
		response := request(t, f.router, "GET", "/api/gallery", "", "")
		page := albumHTTPData[service.GalleryPage](t, response, 200)
		got := make([]uint64, 0, len(page.Items))
		for _, item := range page.Items {
			got = append(got, item.ID)
			if !item.IsPublic || item.Uploader == "" {
				t.Fatal("gallery contains a private or unattributed image")
			}
		}
		slices.Sort(got)
		want := slices.Clone(ids)
		slices.Sort(want)
		if page.Items == nil || page.Total != int64(len(want)) || !slices.Equal(got, want) {
			t.Fatalf("gallery IDs=%v total=%d, want=%v", got, page.Total, want)
		}
		for _, field := range []string{`"exif"`, `"gps_lat"`, `"gps_lng"`, `"raw"`, `"email"`, `"ip"`, `"password"`, `"operation"`, "private-owner-marker"} {
			if strings.Contains(response.Body.String(), field) {
				t.Fatalf("gallery exposed private field %s", field)
			}
		}
		return page
	}
	gallery(t)
	f.setting(t, "gallery_enabled", "true")
	// M5 gallery inclusion is controlled by image visibility. Album visibility
	// is independent; the optional public-albums-only mode is not implemented.
	page := gallery(t, public.ID, other.ID)
	if page.Items[0].ID != other.ID || page.Items[0].Uploader != "other" || page.Items[1].Uploader != "alice" {
		t.Fatal("gallery lost newest-first ordering or uploader attribution")
	}
	paged := albumHTTPData[service.GalleryPage](t, request(t, f.router, "GET", "/api/gallery?page=2&size=1", "", ""), 200)
	if paged.Total != 2 || paged.Page != 2 || paged.Size != 1 || len(paged.Items) != 1 || paged.Items[0].ID != public.ID {
		t.Fatalf("gallery pagination = %+v", paged)
	}
	expectCode(t, request(t, f.router, "PATCH", fmt.Sprintf("/api/albums/%d", privateAlbum.ID), `{"is_public":true}`, f.tokens[0]), 200, 0)
	gallery(t, public.ID, other.ID)
	expectCode(t, request(t, f.router, "PATCH", fmt.Sprintf("/api/images/%d", private.ID), `{"is_public":true}`, f.tokens[0]), 200, 0)
	gallery(t, private.ID, public.ID, other.ID)
	expectCode(t, request(t, f.router, "PATCH", fmt.Sprintf("/api/images/%d", public.ID), `{"is_public":false}`, f.tokens[0]), 200, 0)
	gallery(t, private.ID, other.ID)
	// Private direct objects remain available, but owner-only previews do not.
	parsed, err := url.Parse(public.Links.Original)
	if err != nil {
		t.Fatal(err)
	}
	if request(t, f.router, "GET", parsed.RequestURI(), "", "").Code != 200 {
		t.Fatal("making image private revoked its documented direct link")
	}
	expectCode(t, request(t, f.router, "GET", public.LocalThumbURL, "", ""), 401, 20001)
	expectCode(t, request(t, f.router, "DELETE", fmt.Sprintf("/api/images/%d", private.ID), "", f.tokens[0]), 200, 0)
	gallery(t, other.ID)
	expectSuccessfulImageBatch(t, request(t, f.router, "POST", "/api/trash/restore", fmt.Sprintf(`{"ids":[%d]}`, private.ID), f.tokens[0]))
	gallery(t, private.ID, other.ID)
	for _, value := range []string{"false", "null", `"invalid"`} {
		f.setting(t, "gallery_enabled", value)
		gallery(t)
	}
	f.setting(t, "gallery_enabled", "true")
	gallery(t, private.ID, other.ID)
}
