package http_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
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
)

func sitePolicyListing(name string, storageID uint64) model.Policy {
	return model.Policy{Name: name, StorageID: storageID, PathTpl: "{Y}/{m}", NameTpl: "{uniqid}",
		WebPMode: "both", WebPQuality: 80, WebPEffort: 4, ThumbSize: 400, ThumbEnabled: true,
		ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename",
		StripMeta: true, SkipIfLarger: true, Enabled: true}
}

func TestSiteAndPoliciesContract(t *testing.T) {
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
			tokens, err := service.NewTokenService(ctx, tokenRepo, userRepo, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("UPDATE settings SET value = 'true' WHERE key = 'registration_enabled'").Error; err != nil {
				t.Fatal(err)
			}
			alice, err := users.Register(ctx, service.RegisterInput{Username: "alice", Email: "alice@example.com", Password: "site-policy-password"})
			if err != nil {
				t.Fatal(err)
			}
			storageRepo, err := repo.NewStorageRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			enabledStorage, err := storageRepo.Create(ctx, model.Storage{Name: "on", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/1", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			disabledStorage, err := storageRepo.Create(ctx, model.Storage{Name: "off", Driver: "local", Config: json.RawMessage(`{}`), BaseURL: "http://images.test/i/2", Enabled: false})
			if err != nil {
				t.Fatal(err)
			}
			policies, err := repo.NewPolicyRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			alpha, err := policies.CreateAndBind(ctx, sitePolicyListing("alpha", enabledStorage.ID), alice.GroupID, false)
			if err != nil {
				t.Fatal(err)
			}
			beta, err := policies.CreateAndBind(ctx, sitePolicyListing("beta", enabledStorage.ID), alice.GroupID, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := policies.CreateAndBind(ctx, sitePolicyListing("off-storage", disabledStorage.ID), alice.GroupID, false); err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(ctx).Model(&model.Policy{}).Where("id = ?", beta.ID).Update("enabled", false).Error; err != nil {
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
			router, err := httpapi.NewRouter(ctx, httpapi.Dependencies{Users: users, Tokens: tokens, Images: imageService, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Health: sqlDB.PingContext})
			if err != nil {
				t.Fatal(err)
			}

			site := request(t, router, "GET", "/api/site", "", "")
			expectCode(t, site, 200, 0)
			var siteData map[string]any
			if err := json.Unmarshal(envelope(t, site)["data"], &siteData); err != nil {
				t.Fatal(err)
			}
			if len(siteData) != 3 || siteData["site_name"] != "ImgNest" || siteData["register_enabled"] != true || siteData["gallery_enabled"] != false {
				t.Fatalf("site data %s", site.Body.String())
			}
			for _, private := range []string{"trash_days", "guest_upload_enabled", "default_group_id", "registration_enabled"} {
				if strings.Contains(site.Body.String(), private) {
					t.Fatalf("site response leaked %s", private)
				}
			}

			// The gallery switch is closed by default: anonymous visitors get a
			// well-formed empty page instead of an authorization error.
			gallery := request(t, router, "GET", "/api/gallery", "", "")
			expectCode(t, gallery, 200, 0)
			var galleryData map[string]any
			if err := json.Unmarshal(envelope(t, gallery)["data"], &galleryData); err != nil {
				t.Fatal(err)
			}
			items, ok := galleryData["items"].([]any)
			if !ok || len(items) != 0 || galleryData["total"] != float64(0) {
				t.Fatalf("closed gallery page %s", gallery.Body.String())
			}

			expectCode(t, request(t, router, "GET", "/api/policies", "", ""), 401, 20001)
			expectCode(t, request(t, router, "GET", "/api/unknown", "", ""), 404, 10001)

			aliceToken := loginForSite(t, router, "alice@example.com", "site-policy-password")
			listed := request(t, router, "GET", "/api/policies", "", aliceToken)
			expectCode(t, listed, 200, 0)
			var rules []map[string]any
			if err := json.Unmarshal(envelope(t, listed)["data"], &rules); err != nil {
				t.Fatal(err)
			}
			if len(rules) != 1 || rules[0]["id"] != float64(alpha.ID) || rules[0]["name"] != "alpha" || len(rules[0]) != 2 {
				t.Fatalf("policy listing %s", listed.Body.String())
			}

			if err := db.WithContext(ctx).Exec("DELETE FROM group_policies WHERE group_id = ?", alice.GroupID).Error; err != nil {
				t.Fatal(err)
			}
			empty := request(t, router, "GET", "/api/policies", "", aliceToken)
			expectCode(t, empty, 200, 0)
			if !strings.Contains(empty.Body.String(), `"data":[]`) || strings.Contains(empty.Body.String(), `"data":null`) {
				t.Fatalf("empty policy listing %s", empty.Body.String())
			}
		})
	}
}

func loginForSite(t *testing.T, router http.Handler, email, password string) string {
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
	if session.Token == "" {
		t.Fatal("login did not issue token")
	}
	return session.Token
}
