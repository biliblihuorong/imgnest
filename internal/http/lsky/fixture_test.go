package lsky_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/exif"
	"github.com/biliblihuorong/imgnest/internal/http/lsky"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"github.com/biliblihuorong/imgnest/internal/thumbcache"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// fixedNow pins the injectable clock so human_date, date and throttling
// windows stay deterministic.
var fixedNow = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

const (
	alicePassword = "v1-contract-password"
	imageBaseURL  = "http://images.test/i/1"
	groupCapacity = int64(1048576) // profile capacity renders as 1024 KB
)

// v1Fixture wires the real repositories and services behind the v1 handler,
// mirroring the production wiring in cmd serve.
type v1Fixture struct {
	db      *gorm.DB
	router  http.Handler
	users   *service.UserService
	tokens  *service.TokenService
	images  *service.ImageService
	albums  *service.AlbumService
	handler *lsky.Handler
	alice   service.UserView
	policy  model.Policy
	backend model.Storage
	local   *storage.Local
}

// Row aliases keep the scenario tests free of extra imports.
type (
	modelImage = model.Image
	albumRow   = model.Album
	imageRow   = model.Image
)

func newV1Fixture(t *testing.T, driver string) *v1Fixture {
	t.Helper()
	db := v1Database(t, driver)
	ctx := t.Context()
	userRepo, err := repo.NewUserRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	settingsRepo, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	tokenRepo, err := repo.NewTokenRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	users, err := service.NewUserService(ctx, userRepo, settingsRepo)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := service.NewTokenService(ctx, tokenRepo, userRepo, settingsRepo, func() time.Time { return fixedNow })
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Exec("UPDATE settings SET value = 'true' WHERE key = 'registration_enabled'").Error; err != nil {
		t.Fatal(err)
	}
	alice, err := users.Register(ctx, service.RegisterInput{Username: "alice", Email: "alice@example.com", Password: alicePassword})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Exec("UPDATE groups SET capacity_bytes = ?, allowed_exts = ? WHERE id = ?", groupCapacity, `["png"]`, alice.GroupID).Error; err != nil {
		t.Fatal(err)
	}
	storageRepo, err := repo.NewStorageRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := storageRepo.Create(ctx, model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: imageBaseURL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	policyRepo, err := repo.NewPolicyRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policyRepo.CreateAndBind(ctx, model.Policy{Name: "alpha", StorageID: backend.ID, PathTpl: "{Y}/{m}", NameTpl: "{uniqid}",
		WebPMode: "both", WebPQuality: 80, WebPEffort: 4, ThumbSize: 16, ThumbEnabled: true,
		ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename",
		StripMeta: true, SkipIfLarger: false, Enabled: true}, alice.GroupID, true)
	if err != nil {
		t.Fatal(err)
	}
	imageRepo, err := repo.NewImageRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	lskyRepo, err := repo.NewLskyRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	albumRepo, err := repo.NewAlbumRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	local, err := storage.NewLocal(ctx, filepath.Join(t.TempDir(), "objects"))
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
	processor, err := imaging.NewProcessor(ctx, 100000000, 2)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := exif.NewProcessor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	images, err := service.NewImageService(ctx, service.ImageDependencies{Images: imageRepo, Policies: policyRepo, Storages: storageRepo, Users: userRepo, Tokens: tokenRepo, Drivers: v1DriverProvider{local}, Paths: service.PathFunctions{BuildPath: pathtpl.Build, CleanPath: pathtpl.Sanitize}, Imaging: processor, Extractor: metadata, Scrubber: metadata, Cache: cache, Settings: settingsRepo, Now: func() time.Time { return fixedNow }, MaxFileBytes: 20 << 20})
	if err != nil {
		t.Fatal(err)
	}
	albums, err := service.NewAlbumService(ctx, albumRepo, imageRepo, func() time.Time { return fixedNow })
	if err != nil {
		t.Fatal(err)
	}
	lskyService, err := service.NewLskyService(ctx, service.LskyDependencies{Lsky: lskyRepo, Albums: albumRepo, Policies: policyRepo, Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := lsky.NewHandler(ctx, lsky.Dependencies{Users: users, Tokens: tokens, Images: images, Albums: albums, Lsky: lskyService, Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	router := ginRouter(t, handler)
	return &v1Fixture{db: db, router: router, users: users, tokens: tokens, images: images, albums: albums, handler: handler, alice: alice, policy: policy, backend: backend, local: local}
}

// imagesFixture adds the seeded albums an images/albums/profile scenario needs.
type imagesFixture struct {
	*v1Fixture
	albumID        uint64
	foreignAlbumID uint64
}

// seedImages runs three real uploads (one assigned to a album) and creates
// the albums the list scenarios page through:
//
//	alpha.png 32x16 public    -> album 博客
//	gamma.png 64x32 public    -> unassigned (uploaded before beta)
//	beta.png   8x8  private   -> unassigned (uploaded last, so newest first)
func seedImages(t *testing.T, driver string) *imagesFixture {
	t.Helper()
	fixture := newV1Fixture(t, driver)
	ctx := t.Context()
	subject := fixture.subject(t)

	gamma, err := fixture.images.Upload(ctx, subject, service.UploadInput{Data: pngBytes(t, 64, 32), Filename: "gamma.png", IsPublic: true})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := fixture.images.Upload(ctx, subject, service.UploadInput{Data: pngBytes(t, 8, 8), Filename: "beta.png"})
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := fixture.images.Upload(ctx, subject, service.UploadInput{Data: pngBytes(t, 32, 16), Filename: "alpha.png", IsPublic: true})
	if err != nil {
		t.Fatal(err)
	}

	mallory, err := fixture.users.Register(ctx, service.RegisterInput{Username: "mallory", Email: "mallory@example.com", Password: "v1-contract-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Exec("UPDATE users SET registered_ip = '203.0.113.5' WHERE id = ?", fixture.alice.ID).Error; err != nil {
		t.Fatal(err)
	}
	blog := albumRow{UserID: fixture.alice.ID, Name: "博客", Intro: "文章配图"}
	if err := fixture.db.Create(&blog).Error; err != nil {
		t.Fatal(err)
	}
	note := albumRow{UserID: fixture.alice.ID, Name: "笔记", Intro: "随手记"}
	if err := fixture.db.Create(&note).Error; err != nil {
		t.Fatal(err)
	}
	foreign := albumRow{UserID: mallory.ID, Name: "别人的"}
	if err := fixture.db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Exec("UPDATE images SET album_id = ? WHERE id = ?", blog.ID, alpha.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Exec("UPDATE albums SET image_count = 1 WHERE id = ?", blog.ID).Error; err != nil {
		t.Fatal(err)
	}
	_ = gamma
	_ = beta
	return &imagesFixture{v1Fixture: fixture, albumID: blog.ID, foreignAlbumID: foreign.ID}
}

type v1DriverProvider struct{ local *storage.Local }

func (p v1DriverProvider) DriverFor(context.Context, model.Storage) (storage.Driver, error) {
	return p.local, nil
}

func v1Database(t *testing.T, driver string) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "v1.db")
	if driver == "postgres" {
		dsn = os.Getenv("IMGNEST_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("IMGNEST_TEST_POSTGRES_DSN is not set")
		}
	}
	cfg, err := config.Load(t.Context(), "", []string{"IMGNEST_DATABASE_DRIVER=" + driver, "IMGNEST_DATABASE_DSN=" + dsn})
	if err != nil {
		t.Fatal(err)
	}
	if driver == "postgres" {
		cfg.Database.DSN = isolatedV1Postgres(t, cfg.Database.DSN)
	}
	db, err := repo.Open(t.Context(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := migrate.Up(t.Context(), sqlDB, driver); err != nil {
		t.Fatal(err)
	}
	return db
}

func isolatedV1Postgres(t *testing.T, dsn string) string {
	t.Helper()
	admin, err := repo.Open(t.Context(), config.Database{Driver: "postgres", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	adminDB, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	schema := "v1_" + hex.EncodeToString(raw[:])
	if _, err := adminDB.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := adminDB.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
		if err := adminDB.Close(); err != nil {
			t.Error(err)
		}
	})
	return dsn + " search_path=" + schema
}

func ginRouter(t *testing.T, handler *lsky.Handler) http.Handler {
	t.Helper()
	router := gin.New()
	if err := handler.RegisterRoutes(t.Context(), router); err != nil {
		t.Fatal(err)
	}
	return router
}

// request issues one JSON or form request against the v1 router.
func (f *v1Fixture) request(t *testing.T, method, target, body, contentType, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	} else {
		r.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

// upload posts one multipart v1 upload with the optional v1 fields.
func (f *v1Fixture) upload(t *testing.T, filename string, data []byte, bearer string, fields ...string) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for i := 0; i+1 < len(fields); i += 2 {
		if err := writer.WriteField(fields[i], fields[i+1]); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/v1/upload", body)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Content-Type", writer.FormDataContentType())
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

// login exchanges alice's credentials for a fresh v1 API token.
func (f *v1Fixture) login(t *testing.T) string {
	t.Helper()
	response := f.request(t, "POST", "/api/v1/tokens", "email=alice@example.com&password="+alicePassword, "application/x-www-form-urlencoded", "")
	var body struct {
		Status bool `json:"status"`
		Data   struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || !body.Status || body.Data.Token == "" {
		t.Fatalf("v1 login failed: %s", response.Body.String())
	}
	return body.Data.Token
}

// enableGuestUpload creates the is_guest group, grants the policy to it as
// the group default (the same configuration an administrator would make) and
// switches guest uploads on. guest_group_id keeps its migration default of 0,
// so the fallback-to-is_guest path stays under test.
func (f *v1Fixture) enableGuestUpload(t *testing.T) uint64 {
	t.Helper()
	guests := model.Group{Name: "Guests", IsGuest: true, CapacityBytes: groupCapacity, AllowedExts: []string{"png"}}
	if err := f.db.Create(&guests).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("INSERT INTO group_policies (group_id, policy_id) VALUES (?, ?)", guests.ID, f.policy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE groups SET default_policy_id = ? WHERE id = ?", f.policy.ID, guests.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec("UPDATE settings SET value = 'true' WHERE key = 'guest_upload_enabled'").Error; err != nil {
		t.Fatal(err)
	}
	return guests.ID
}

// subject resolves alice's opaque upload proof through the real credential path.
func (f *v1Fixture) subject(t *testing.T) service.TokenSubject {
	t.Helper()
	verified, err := f.users.VerifyCredentials(t.Context(), "alice@example.com", alicePassword)
	if err != nil {
		t.Fatal(err)
	}
	return verified.Subject
}

// png renders a real PNG of the given size so libvips probes it for real.
func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// jpegBytes renders a real JPEG for format-rejection scenarios.
func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func mustStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("HTTP=%d want %d body=%s", response.Code, want, response.Body.String())
	}
}
