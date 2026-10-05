package repo

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

type imageFixture struct {
	user    model.User
	grant   model.TokenGrant
	storage model.Storage
	policy  model.Policy
	images  *ImageRepository
	tokens  *TokenRepository
	users   *UserRepository
}

func newImageFixture(t *testing.T, db *gorm.DB, name string) imageFixture {
	t.Helper()
	user, users, tokens := grantFixture(t, db, name)
	backends, err := NewStorageRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	backend, err := backends.Create(t.Context(), model.Storage{Name: name, Driver: "local", Enabled: true, BaseURL: "http://localhost/files", Config: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := NewPolicyRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.CreateAndBind(t.Context(), testPolicy(backend.ID), user.GroupID, true)
	if err != nil {
		t.Fatal(err)
	}
	images, err := NewImageRepository(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	grant := testGrant(user.PasswordHash)
	grant.UserID = user.ID
	return imageFixture{user: user, users: users, tokens: tokens, grant: grant, storage: backend, policy: policy, images: images}
}

func (f imageFixture) request(key, path string) model.UploadReservation {
	return model.UploadReservation{
		Grant: f.grant,
		Image: model.Image{
			UserID: f.user.ID, PolicyID: f.policy.ID, StorageID: f.storage.ID, Key: key, Path: path,
			Ext: "jpg", MIME: "image/jpeg", OriginName: "photo.jpg", SrcMD5: "source", MD5: "stored", SHA1: "stored-sha1",
			OperationID: "upload-" + key, HasOriginal: true, HasWebP: true, HasThumb: true, Scrubbed: true,
			Size: 100, WebPSize: 40, ThumbBytes: 20, Width: 10, Height: 10, Frames: 1,
		},
		Objects: []model.ObjectReceipt{
			{Key: path + ".jpg", Size: 100, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/jpeg"},
			{Key: path + ".webp", Size: 40, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/webp"},
			{Key: path + "_thumbs.webp", Size: 20, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/webp"},
			{Key: path + "_thumbs.webp", Size: 20, OwnerID: key, Location: model.ObjectLocationThumbCache, MIME: "image/webp"},
		},
	}
}

func reserveAndCommit(t *testing.T, f imageFixture, key, path string) model.Image {
	t.Helper()
	input := f.request(key, path)
	image, err := f.images.ReserveUpload(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range input.Objects {
		receipt.VersionID = "written-version"
		if err := f.images.RecordObjectReceipt(t.Context(), image.Key, image.OperationID, receipt); err != nil {
			t.Fatal(err)
		}
	}
	image, err = f.images.CommitUpload(t.Context(), image.Key, image.OperationID, model.ImageExif{Raw: json.RawMessage(`{"camera":"test"}`)}, f.grant)
	if err != nil {
		t.Fatal(err)
	}
	return image
}

func usedBytes(t *testing.T, f imageFixture) int64 {
	t.Helper()
	user, err := f.users.FindUserByID(t.Context(), f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	return user.UsedBytes
}

func TestImageReservationCommitAndCleanup(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "imagebasic")
		input := f.request("first-image", "files/first")
		image, err := f.images.ReserveUpload(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if image.State != model.ImageStatePending || image.ChargedBytes != 160 || image.AlbumID != 0 {
			t.Fatalf("reservation state=%s charged=%d album=%d", image.State, image.ChargedBytes, image.AlbumID)
		}
		repeated, err := f.images.ReserveUpload(t.Context(), input)
		if err != nil || repeated.ID != image.ID {
			t.Fatalf("reservation retry changed row: %v", err)
		}
		if usedBytes(t, f) != 0 {
			t.Fatal("pending upload charged active capacity")
		}
		for _, receipt := range input.Objects {
			receipt.VersionID = "v1"
			if err := f.images.RecordObjectReceipt(t.Context(), image.Key, image.OperationID, receipt); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.images.RecordObjectReceipt(t.Context(), image.Key, "wrong-operation", input.Objects[0]); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("stale receipt error=%v", err)
		}
		gps := 1.25
		exif := model.ImageExif{GPSLat: &gps, Raw: json.RawMessage(`{"blocks":["archived"]}`)}
		image, err = f.images.CommitUpload(t.Context(), image.Key, image.OperationID, exif, f.grant)
		if err != nil {
			t.Fatal(err)
		}
		if image.State != model.ImageStateActive || image.Operation != "" || image.OperationID != input.Image.OperationID {
			t.Fatal("commit lost the operation confirmation")
		}
		if _, err := f.images.CommitUpload(t.Context(), image.Key, image.OperationID, exif, f.grant); err != nil {
			t.Fatal(err)
		}
		if usedBytes(t, f) != 160 {
			t.Fatal("duplicate commit charged twice")
		}
		metadata, err := f.images.FindExif(t.Context(), image.Key)
		if err != nil || metadata.ImageID != image.ID || metadata.GPSLat == nil || *metadata.GPSLat != gps {
			t.Fatalf("owner metadata did not persist: %v", err)
		}
		if err := f.images.StartCleanup(t.Context(), image.Key, image.OperationID); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("active cleanup allowed: %v", err)
		}
		if err := f.images.FinishCleanup(t.Context(), image.Key, image.OperationID); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("active cleanup removed committed row: %v", err)
		}
		byID, err := f.images.FindByID(t.Context(), image.ID)
		if err != nil || byID.Key != image.Key {
			t.Fatalf("numeric lookup failed: %v", err)
		}
		listed, total, err := f.images.List(t.Context(), model.ImageListFilter{UserID: f.user.ID}, 1, 10)
		if err != nil || total != 1 || len(listed) != 1 {
			t.Fatalf("active page count=%d total=%d error=%v", len(listed), total, err)
		}
		pending, err := f.images.ReserveUpload(t.Context(), f.request("pending-image", "files/pending"))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.images.StartCleanup(t.Context(), pending.Key, pending.OperationID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("blocked-image", pending.Path)); !errors.Is(err, model.ErrPathConflict) {
			t.Fatalf("cleanup released the path early: %v", err)
		}
		if err := f.images.FinishCleanup(t.Context(), pending.Key, pending.OperationID); err != nil {
			t.Fatal(err)
		}
		if err := f.images.FinishCleanup(t.Context(), pending.Key, pending.OperationID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("replacement-image", pending.Path)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestImageConcurrentQuotaAndPath(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		t.Run("quota", func(t *testing.T) {
			f := newImageFixture(t, db, "quotarace")
			if err := db.WithContext(t.Context()).Model(&model.Group{}).Where("id = ?", f.user.GroupID).Update("capacity_bytes", 1000).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(t.Context()).Model(&model.User{}).Where("id = ?", f.user.ID).Update("used_bytes", 600).Error; err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			for _, name := range []string{"quota-one", "quota-two"} {
				go func() {
					input := f.request(name, "files/"+name)
					setAmount(&input, 300)
					<-start
					_, err := f.images.ReserveUpload(t.Context(), input)
					results <- err
				}()
			}
			close(start)
			assertCompetition(t, results, model.ErrQuotaExceeded)
		})
		t.Run("cross-user-path", func(t *testing.T) {
			f := newImageFixture(t, db, "pathrace")
			other, _, _ := grantFixture(t, db, "otherpathuser")
			start := make(chan struct{})
			results := make(chan error, 2)
			for index, name := range []string{"path-one", "path-two"} {
				go func() {
					input := f.request(name, "files/same-path")
					if index == 1 {
						input.Image.UserID = other.ID
						input.Grant.UserID = other.ID
						input.Grant.ExpectedPasswordHash = other.PasswordHash
					}
					<-start
					_, err := f.images.ReserveUpload(t.Context(), input)
					results <- err
				}()
			}
			close(start)
			assertCompetition(t, results, model.ErrPathConflict)
		})
	})
}

func TestImageChargeDeduplicatesWebPAndRejectsOverflow(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "dedupe")
		input := f.request("webp-image", "files/webp")
		input.Image.Ext = "webp"
		input.Image.MIME = "image/webp"
		input.Image.WebPSize = 100
		input.Objects[0].Key = "files/webp.webp"
		input.Objects[0].MIME = "image/webp"
		input.Objects[1] = input.Objects[0]
		image, err := f.images.ReserveUpload(t.Context(), input)
		if err != nil || image.ChargedBytes != 120 {
			t.Fatalf("unique WebP charge=%d error=%v", image.ChargedBytes, err)
		}
		if err := f.images.StartCleanup(t.Context(), image.Key, image.OperationID); err != nil {
			t.Fatal(err)
		}
		if err := f.images.FinishCleanup(t.Context(), image.Key, image.OperationID); err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(t.Context()).Model(&model.User{}).Where("id = ?", f.user.ID).Update("used_bytes", math.MaxInt64).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("overflow", "files/overflow")); !errors.Is(err, model.ErrQuotaExceeded) {
			t.Fatalf("overflow error=%v, want ErrQuotaExceeded", err)
		}
	})
}

func TestImageCommitRollbackAndRevokedProof(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "commitguard")
		input := f.request("rollback-image", "files/rollback")
		image, err := f.images.ReserveUpload(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(t.Context()).Exec("ALTER TABLE image_exif RENAME TO image_exif_saved").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.CommitUpload(t.Context(), image.Key, image.OperationID, model.ImageExif{}, f.grant); err == nil {
			t.Fatal("metadata failure still committed image")
		}
		found, err := f.images.FindByKey(t.Context(), image.Key)
		if err != nil || found.State != model.ImageStatePending || usedBytes(t, f) != 0 {
			t.Fatalf("commit failure did not roll back: %v", err)
		}
		if err := db.WithContext(t.Context()).Exec("ALTER TABLE image_exif_saved RENAME TO image_exif").Error; err != nil {
			t.Fatal(err)
		}
		source, err := f.tokens.CreateToken(t.Context(), testToken(f.user.ID), f.grant)
		if err != nil {
			t.Fatal(err)
		}
		proof := f.grant
		proof.SourceTokenID = source.ID
		if err := f.tokens.RevokeToken(t.Context(), f.user.ID, source.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.CommitUpload(t.Context(), image.Key, image.OperationID, model.ImageExif{}, proof); !errors.Is(err, model.ErrUnauthenticated) {
			t.Fatalf("revoked proof activated image: %v", err)
		}
		if usedBytes(t, f) != 0 {
			t.Fatal("revoked grant charged capacity")
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := f.images.ReserveUpload(ctx, f.request("cancelled", "files/cancelled")); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled reservation error=%v", err)
		}
	})
}

func TestImageObjectDigestCannotChange(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "objectdigest")
		input := f.request("digest-image", "files/digest")
		for index := range input.Objects {
			input.Objects[index].SHA256 = strings.Repeat("a", 64)
		}
		image, err := f.images.ReserveUpload(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		receipt := input.Objects[0]
		receipt.SHA256 = strings.Repeat("b", 64)
		if err := f.images.RecordObjectReceipt(t.Context(), image.Key, image.OperationID, receipt); !errors.Is(err, model.ErrInvalidInput) {
			t.Errorf("receipt changed planned digest: %v", err)
		}
		receipt.SHA256 = ""
		if err := f.images.RecordObjectReceipt(t.Context(), image.Key, image.OperationID, receipt); err != nil {
			t.Fatal(err)
		}
		found, err := f.images.FindByKey(t.Context(), image.Key)
		if err != nil || found.ObjectManifest[0].SHA256 != strings.Repeat("a", 64) {
			t.Errorf("receipt dropped digest: %v", err)
		}
		invalid := f.request("bad-digest", "files/bad-digest")
		invalid.Objects[0].SHA256 = "not-a-sha256"
		if _, err := f.images.ReserveUpload(t.Context(), invalid); !errors.Is(err, model.ErrInvalidInput) {
			t.Errorf("invalid object digest accepted: %v", err)
		}
	})
}

func setAmount(input *model.UploadReservation, total int64) {
	part := total / 3
	input.Image.Size = total - 2*part
	input.Image.WebPSize = part
	input.Image.ThumbBytes = part
	input.Objects[0].Size = input.Image.Size
	input.Objects[1].Size = part
	input.Objects[2].Size = part
	input.Objects[3].Size = part
}

func assertCompetition(t *testing.T, results <-chan error, want error) {
	t.Helper()
	var successes, failures int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, want):
			failures++
		default:
			t.Fatalf("unexpected competition error: %v", err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("successes=%d failures=%d, want 1 each", successes, failures)
	}
}
