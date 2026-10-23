package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/exif"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"github.com/biliblihuorong/imgnest/internal/thumbcache"
	"gorm.io/gorm"
)

// These adapters retain real processing, repositories and owned filesystem IO;
// only the deterministic instant at which the lease expires is controlled.
type billingExpiryProcessor struct {
	imaging.Processor
	after func()
}

func (p billingExpiryProcessor) Process(ctx context.Context, data []byte, opts imaging.Options) (imaging.Result, error) {
	out, err := p.Processor.Process(ctx, data, opts)
	if err == nil && p.after != nil {
		p.after()
	}
	return out, err
}

type billingExpiryStorage struct {
	*storage.Local
	after  func()
	writes []string
}

func (s *billingExpiryStorage) PutNew(ctx context.Context, key string, data io.ReadSeeker, opts storage.PutOptions) (storage.Receipt, error) {
	receipt, err := s.Local.PutNew(ctx, key, data, opts)
	if err == nil {
		s.writes = append(s.writes, key)
		if s.after != nil {
			s.after()
		}
	}
	return receipt, err
}

func (s *billingExpiryStorage) DriverFor(context.Context, model.Storage) (storage.Driver, error) {
	return s, nil
}

func TestBillingUploadPipelineExpiryCompensatesObjects(t *testing.T) {
	for _, boundary := range []string{"reserve", "commit"} {
		t.Run(boundary, func(t *testing.T) {
			forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
				f := billingSetup(t, db)
				images := newImageFixture(t, db, "billing-upload")
				f.buyer = billingActor(t, db, images.user)
				f.base = images.user.GroupID
				plan := f.plan(t)
				f.topup(t, 10000)
				order, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: plan.ID, PlanVersion: plan.Version, RequestID: "upload-plan"})
				if err != nil {
					t.Fatal(err)
				}
				// Re-login after purchase changes the group, using a real bearer.
				var owner model.User
				if err = db.First(&owner, images.user.ID).Error; err != nil {
					t.Fatal(err)
				}
				actor := billingActor(t, db, owner)
				secret := strings.Repeat("b", 40)
				digest := sha256.Sum256([]byte(secret))
				if err = db.Model(&model.Token{}).Where("id = ?", actor.TokenID).Update("token_hash", hex.EncodeToString(digest[:])).Error; err != nil {
					t.Fatal(err)
				}
				if err = db.Table("group_policies").Create(map[string]any{"group_id": f.paid, "policy_id": images.policy.ID}).Error; err != nil {
					t.Fatal(err)
				}
				_, settings := repositories(t, db)
				tokens, err := service.NewTokenService(t.Context(), images.tokens, images.users, settings, f.r.now)
				if err != nil {
					t.Fatal(err)
				}
				identity, err := tokens.Authenticate(t.Context(), fmt.Sprintf("%d|%s", actor.TokenID, secret))
				if err != nil {
					t.Fatal(err)
				}
				local, err := storage.NewLocal(t.Context(), filepath.Join(t.TempDir(), "objects"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = local.Close() })
				cache, err := thumbcache.New(t.Context(), filepath.Join(t.TempDir(), "thumbs"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cache.Close() })
				processor, err := imaging.NewProcessor(t.Context(), 1_000_000, 1)
				if err != nil {
					t.Fatal(err)
				}
				metadata, err := exif.NewProcessor(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				policies, err := NewPolicyRepository(t.Context(), db)
				if err != nil {
					t.Fatal(err)
				}
				backends, err := NewStorageRepository(t.Context(), db)
				if err != nil {
					t.Fatal(err)
				}
				expire := func() { f.now = order.ExpiresAt }
				driver := &billingExpiryStorage{Local: local}
				processing := billingExpiryProcessor{Processor: processor}
				if boundary == "reserve" {
					processing.after = expire
				} else {
					driver.after = expire
				}
				svc, err := service.NewImageService(t.Context(), service.ImageDependencies{
					Images: images.images, Users: images.users, Tokens: images.tokens, Policies: policies, Storages: backends,
					Drivers: driver, Paths: service.PathFunctions{BuildPath: pathtpl.Build, CleanPath: pathtpl.Sanitize},
					Imaging: processing, Extractor: metadata, Scrubber: metadata, Cache: cache, Settings: settings,
					Now: f.r.now, MaxFileBytes: 1 << 20,
				})
				if err != nil {
					t.Fatal(err)
				}
				source := image.NewNRGBA(image.Rect(0, 0, 64, 48))
				for y := range 48 {
					for x := range 64 {
						source.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 3), G: uint8(y * 4), B: 123, A: 255})
					}
				}
				var encoded bytes.Buffer
				if err = png.Encode(&encoded, source); err != nil {
					t.Fatal(err)
				}
				_, err = svc.Upload(t.Context(), identity.Subject, service.UploadInput{Data: encoded.Bytes(), Filename: "expired.png", PolicyID: images.policy.ID})
				if !errors.Is(err, model.ErrUnauthenticated) {
					t.Fatalf("%s expiry accepted or wrong failure: %v", boundary, err)
				}
				if boundary == "reserve" && len(driver.writes) != 0 || boundary == "commit" && len(driver.writes) < 2 {
					t.Fatalf("wrong storage boundary: %v", driver.writes)
				}
				for _, key := range driver.writes {
					if _, err = local.Stat(t.Context(), key); !errors.Is(err, storage.ErrNotFound) {
						t.Fatalf("object survived compensation: %s %v", key, err)
					}
					if strings.HasSuffix(key, "_thumbs.webp") {
						if reader, cacheErr := cache.Open(t.Context(), images.storage.ID, key); cacheErr == nil {
							_ = reader.Close()
							t.Fatal("thumbnail cache survived compensation")
						}
					}
				}
				var count int64
				if err = db.Model(&model.Image{}).Where("user_id = ?", owner.ID).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("upload reservation survived: %d %v", count, err)
				}
				if err = db.First(&owner, owner.ID).Error; err != nil || owner.UsedBytes != 0 {
					t.Fatalf("quota survived: %+v %v", owner, err)
				}
			})
		})
	}
}
