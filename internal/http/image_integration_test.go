package http_test

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 -- verifies the public legacy content checksum, not credentials.
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/jpeg"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/exif"
	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"github.com/biliblihuorong/imgnest/internal/thumbcache"
	"gorm.io/gorm"
)

type realImageProvider struct{ local *storage.Local }

func (p realImageProvider) DriverFor(context.Context, model.Storage) (storage.Driver, error) {
	return p.local, nil
}

func imageDatabase(t *testing.T, driver string) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "images.db")
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
		admin, err := repo.Open(t.Context(), cfg.Database)
		if err != nil {
			t.Fatal(err)
		}
		adminDB, err := admin.DB()
		if err != nil {
			t.Fatal(err)
		}
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			t.Fatal(err)
		}
		schema := "http_images_" + hex.EncodeToString(random[:])
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
		if strings.Contains(dsn, "://") {
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal("invalid isolated test DSN")
			}
			query := parsed.Query()
			query.Set("search_path", schema)
			parsed.RawQuery = query.Encode()
			cfg.Database.DSN = parsed.String()
		} else {
			cfg.Database.DSN = dsn + " search_path=" + schema
		}
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

func TestRealImageHTTPPipelineAndPrivacy(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := imageDatabase(t, driver)
			ctx := t.Context()
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
			if err := db.Model(&model.Setting{}).Where("key = ?", "registration_enabled").Update("value", "true").Error; err != nil {
				t.Fatal(err)
			}
			alice, err := users.Register(ctx, service.RegisterInput{Username: "alice", Email: "alice@example.com", Password: "real-image-test-password"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := users.Register(ctx, service.RegisterInput{Username: "other", Email: "other@example.com", Password: "other-image-test-password"}); err != nil {
				t.Fatal(err)
			}
			storageRepo, err := repo.NewStorageRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			backend, err := storageRepo.Create(ctx, model.Storage{Name: "local", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/1", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := storageRepo.SetBaseURL(ctx, backend.ID, "http://images.test/i/"+strconv.FormatUint(backend.ID, 10)); err != nil {
				t.Fatal(err)
			}
			policies, err := repo.NewPolicyRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := policies.CreateAndBind(ctx, model.Policy{Name: "default", StorageID: backend.ID, PathTpl: "{Y}/{m}", NameTpl: "{filename}", WebPMode: "both", WebPQuality: 80, WebPEffort: 4, ScrubMode: "gps", HEIFMode: "webp_only", ThumbEnabled: true, ThumbSize: 16, Enabled: true, LinkPrefer: "webp", OnConflict: "rename"}, alice.GroupID, true); err != nil {
				t.Fatal(err)
			}
			images, err := repo.NewImageRepository(ctx, db)
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
			imageService, err := service.NewImageService(ctx, service.ImageDependencies{Images: images, Policies: policies, Storages: storageRepo, Users: userRepo, Tokens: tokenRepo, Drivers: realImageProvider{local}, Paths: service.PathFunctions{BuildPath: pathtpl.Build, CleanPath: pathtpl.Sanitize}, Imaging: processor, Extractor: metadata, Scrubber: metadata, Cache: cache, Settings: settings, Now: time.Now, MaxFileBytes: 20 << 20})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			router, err := httpapi.NewRouter(ctx, httpapi.Dependencies{Users: users, Tokens: tokens, Images: imageService, Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Health: sqlDB.PingContext})
			if err != nil {
				t.Fatal(err)
			}
			login := func(email, password string) string {
				t.Helper()
				body, err := json.Marshal(map[string]string{"email": email, "password": password})
				if err != nil {
					t.Fatal(err)
				}
				response := request(t, router, "POST", "/api/auth/login", string(body), "")
				expectCode(t, response, 200, 0)
				var session struct {
					Token string `json:"token"`
				}
				if err := json.Unmarshal(envelope(t, response)["data"], &session); err != nil {
					t.Fatal(err)
				}
				return session.Token
			}
			aliceToken := login("alice@example.com", "real-image-test-password")
			otherToken := login("other@example.com", "other-image-test-password")
			source := privateJPEG(t)
			body, contentType := multipartBody(t, multipartEntry{"file", "旅行.png", string(source)})
			r := httptestImageUpload(router, body, contentType, aliceToken)
			expectCode(t, r, 201, 0)
			var imageView service.ImageView
			if err := json.Unmarshal(envelope(t, r)["data"], &imageView); err != nil {
				t.Fatal(err)
			}
			if imageView.Ext != "jpg" || !imageView.HasOriginal || !imageView.HasWebP || !imageView.HasThumb {
				t.Fatal("real detected variants missing")
			}
			if strings.Contains(r.Body.String(), "gps_") || strings.Contains(r.Body.String(), "private-owner-marker") || strings.Contains(r.Body.String(), "\"raw\"") {
				t.Fatal("upload response leaked metadata")
			}
			originalURL, err := url.Parse(imageView.Links.Original)
			if err != nil {
				t.Fatal(err)
			}
			original := request(t, router, "GET", originalURL.RequestURI(), "", "")
			if original.Code != 200 || original.Header().Get("Content-Type") != "image/jpeg" {
				t.Fatalf("private direct link unavailable: %d", original.Code)
			}
			sourcePixels := source[bytes.Index(source, []byte{0xff, 0xda}):]
			storedBytes := original.Body.Bytes()
			storedPixels := storedBytes[bytes.Index(storedBytes, []byte{0xff, 0xda}):]
			if !bytes.Equal(sourcePixels, storedPixels) {
				t.Fatal("original JPEG pixels were reencoded")
			}
			storedInfo, err := processor.Probe(ctx, storedBytes)
			if err != nil {
				t.Fatal(err)
			}
			cloudExif, err := metadata.Extract(ctx, storedBytes, storedInfo)
			if err != nil {
				t.Fatal(err)
			}
			if cloudExif.GPSLat != nil || cloudExif.GPSLng != nil || bytes.Contains(storedBytes, []byte("private-owner-marker")) {
				t.Fatal("cloud original retained private metadata")
			}
			for _, link := range []string{imageView.Links.WebP, imageView.Links.Thumbnail} {
				parsed, err := url.Parse(link)
				if err != nil {
					t.Fatal(err)
				}
				response := request(t, router, "GET", parsed.RequestURI(), "", "")
				if response.Code != 200 {
					t.Fatal("declared cloud derivative is unavailable")
				}
				info, err := processor.Probe(ctx, response.Body.Bytes())
				if err != nil || info.Format != "webp" {
					t.Fatal("cloud derivative is not real WebP")
				}
				derived, err := metadata.Extract(ctx, response.Body.Bytes(), info)
				if err != nil || derived.GPSLat != nil || derived.GPSLng != nil {
					t.Fatal("cloud derivative retained GPS")
				}
			}
			digest := md5.Sum(storedBytes) // #nosec G401 -- verifies stored-byte checksum contract.
			if imageView.MD5 != hex.EncodeToString(digest[:]) || imageView.Size != int64(len(storedBytes)) {
				t.Fatal("stored byte/hash contract differs from direct-link body")
			}
			id := strconv.FormatUint(imageView.ID, 10)
			metadataResponse := request(t, router, "GET", "/api/images/"+id+"/exif", "", aliceToken)
			expectCode(t, metadataResponse, 200, 0)
			var archived model.ImageExif
			if err := json.Unmarshal(envelope(t, metadataResponse)["data"], &archived); err != nil {
				t.Fatal(err)
			}
			if archived.GPSLat == nil || archived.GPSLng == nil || len(archived.Raw) == 0 {
				t.Fatal("full private metadata was not archived")
			}
			expectCode(t, request(t, router, "GET", "/api/images/"+id+"/exif", "", otherToken), 403, 20003)
			expectCode(t, request(t, router, "GET", imageView.LocalThumbURL, "", ""), 401, 20001)
			preview := request(t, router, "GET", imageView.LocalThumbURL, "", aliceToken)
			if preview.Code != 200 || preview.Header().Get("Content-Type") != "image/webp" {
				t.Fatal("private thumbnail unavailable")
			}
			if info, err := processor.Probe(ctx, preview.Body.Bytes()); err != nil || info.Format != "webp" {
				t.Fatal("local cache body is not raw WebP")
			}
			row, err := images.FindByKey(ctx, imageView.Key)
			if err != nil {
				t.Fatal(err)
			}
			if err := cache.Delete(ctx, row.StorageID, row.Path+"_thumbs.webp"); err != nil {
				t.Fatal(err)
			}
			if request(t, router, "GET", imageView.LocalThumbURL, "", aliceToken).Code != 200 {
				t.Fatal("missing cache did not refill")
			}
			expectCode(t, request(t, router, "PATCH", "/api/images/"+id, `{"is_public":true}`, aliceToken), 200, 0)
			if request(t, router, "GET", imageView.LocalThumbURL, "", "").Code != 200 {
				t.Fatal("public thumbnail rejected anonymous reader")
			}
			if err := db.Model(&model.Group{}).Where("id = ?", alice.GroupID).Update("capacity_bytes", imageView.ChargedBytes).Error; err != nil {
				t.Fatal(err)
			}
			expectCode(t, httptestImageUpload(router, body, contentType, aliceToken), 403, 30004)
			expectCode(t, request(t, router, "DELETE", "/api/images/"+id, "", aliceToken), 200, 0)
			expectCode(t, request(t, router, "GET", originalURL.RequestURI(), "", ""), 404, 10001)
			selection := `{"ids":[` + id + `]}`
			restored := request(t, router, "POST", "/api/trash/restore", selection, aliceToken)
			expectSuccessfulImageBatch(t, restored)
			if request(t, router, "GET", originalURL.RequestURI(), "", "").Code != 200 {
				t.Fatal("restore did not recover exact direct link")
			}
			expectCode(t, request(t, router, "DELETE", "/api/images/"+id, "", aliceToken), 200, 0)
			expectSuccessfulImageBatch(t, request(t, router, "POST", "/api/trash/purge", selection, aliceToken))
			expectCode(t, request(t, router, "GET", "/api/images/"+id, "", aliceToken), 404, 10001)
			found, err := userRepo.FindUserByID(ctx, alice.ID)
			if err != nil || found.UsedBytes != 0 {
				t.Fatal("trash/purge did not release charged capacity")
			}
			for _, secret := range []string{aliceToken, otherToken, "private-owner-marker", "real-image-test-password"} {
				if strings.Contains(logs.String(), secret) {
					t.Fatal("HTTP logs contain credentials or source metadata")
				}
			}
		})
	}
}

func httptestImageUpload(router http.Handler, body []byte, contentType, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/upload", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
func expectSuccessfulImageBatch(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	expectCode(t, response, 207, 0)
	var results []struct{ Code int }
	if err := json.Unmarshal(envelope(t, response)["data"], &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Code != 0 {
		t.Fatalf("image action failed: %s", response.Body.String())
	}
}

func privateJPEG(t *testing.T) []byte {
	t.Helper()
	var pixels bytes.Buffer
	if err := jpeg.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 64, 32)), nil); err != nil {
		t.Fatal(err)
	}
	tiff := make([]byte, 140)
	copy(tiff, []byte{'I', 'I', 42, 0, 8, 0, 0, 0})
	order := binary.LittleEndian
	order.PutUint16(tiff[8:], 2)
	tag := func(offset int, id, kind uint16, count, value uint32) {
		order.PutUint16(tiff[offset:], id)
		order.PutUint16(tiff[offset+2:], kind)
		order.PutUint32(tiff[offset+4:], count)
		order.PutUint32(tiff[offset+8:], value)
	}
	tag(10, 0x112, 3, 1, 1)
	tag(22, 0x8825, 4, 1, 38)
	order.PutUint16(tiff[38:], 4)
	tag(40, 1, 2, 2, uint32('N'))
	tag(52, 2, 5, 3, 92)
	tag(64, 3, 2, 2, uint32('E'))
	tag(76, 4, 5, 3, 116)
	for i, value := range []uint32{1, 2, 3, 4, 5, 6} {
		order.PutUint32(tiff[92+i*8:], value)
		order.PutUint32(tiff[96+i*8:], 1)
	}
	output := append([]byte{}, pixels.Bytes()[:2]...)
	for _, payload := range [][]byte{append([]byte("Exif\x00\x00"), tiff...), []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">private-owner-marker</x:xmpmeta>")} {
		length := len(payload) + 2
		if length < 0 || length > 65535 {
			t.Fatal("test metadata too large")
			return nil
		}
		prefix := []byte{0xff, 0xe1, 0, 0}
		binary.BigEndian.PutUint16(prefix[2:], uint16(length))
		output = append(output, prefix...)
		output = append(output, payload...)
	}
	return append(output, pixels.Bytes()[2:]...)
}
