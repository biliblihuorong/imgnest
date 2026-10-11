package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"github.com/biliblihuorong/imgnest/internal/thumbcache"
)

type uploadRepo struct {
	ImageRepository
	mu                                    sync.Mutex
	rows                                  map[string]model.Image
	commitErr                             error
	committedDespiteError, cleanupFailure bool
}

func (r *uploadRepo) ReserveUpload(_ context.Context, in model.UploadReservation) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.Path == in.Image.Path && row.StorageID == in.Image.StorageID {
			return model.Image{}, ErrPathConflict
		}
	}
	image := in.Image
	image.ID = uint64(len(r.rows) + 1)
	image.State = model.ImageStatePending
	image.Operation = model.ImageOperationUpload
	image.ObjectManifest = in.Objects
	r.rows[image.Key] = image
	return image, nil
}
func (r *uploadRepo) RecordObjectReceipt(_ context.Context, key, op string, receipt model.ObjectReceipt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if row.OperationID != op {
		return ErrImageBusy
	}
	for i, v := range row.ObjectManifest {
		if v.Key == receipt.Key && v.Location == receipt.Location {
			row.ObjectManifest[i] = receipt
		}
	}
	r.rows[key] = row
	return nil
}
func (r *uploadRepo) CommitUpload(_ context.Context, key, op string, _ model.ImageExif, _ model.TokenGrant) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if r.commitErr == nil || r.committedDespiteError {
		row.State = model.ImageStateActive
		row.Operation = ""
		r.rows[key] = row
	}
	return row, r.commitErr
}
func (r *uploadRepo) FindByKey(_ context.Context, key string) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[key]
	if !ok {
		return row, ErrNotFound
	}
	return row, nil
}
func (r *uploadRepo) StartCleanup(_ context.Context, key, op string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if row.State == model.ImageStateActive {
		return ErrImageBusy
	}
	row.Operation = model.ImageOperationCleanup
	r.rows[key] = row
	return nil
}
func (r *uploadRepo) FinishCleanup(_ context.Context, key, op string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cleanupFailure {
		return ErrStorage
	}
	delete(r.rows, key)
	return nil
}

type uploadPolicy struct {
	PolicyRepository
	policy  model.Policy
	backend model.Storage
	group   model.Group
}

func (p *uploadPolicy) UploadPolicy(context.Context, uint64, uint64) (model.Policy, model.Storage, model.Group, error) {
	return p.policy, p.backend, p.group, nil
}
func (p *uploadPolicy) UploadGroup(context.Context, uint64) (model.Group, error) {
	return p.group, nil
}
func (p *uploadPolicy) Find(context.Context, uint64) (model.Policy, error) { return p.policy, nil }

type uploadStorageRepo struct {
	StorageRepository
	backend model.Storage
}

func (r uploadStorageRepo) Find(context.Context, uint64) (model.Storage, error) {
	return r.backend, nil
}

type uploadUserRepo struct {
	UserRepository
	user model.User
}

func (r uploadUserRepo) FindUserByID(context.Context, uint64) (model.User, error) { return r.user, nil }

type uploadSettings struct{}

func (uploadSettings) TrashDays(context.Context) (int, error) { return 7, nil }

type uploadTokenRepo struct {
	TokenRepository
	token model.Token
	err   error
}

func (r uploadTokenRepo) FindToken(context.Context, uint64) (model.Token, error) {
	return r.token, r.err
}

type uploadDriverProvider struct{ driver storage.Driver }

func (p uploadDriverProvider) DriverFor(context.Context, model.Storage) (storage.Driver, error) {
	return p.driver, nil
}

func (r *uploadRepo) FindByID(_ context.Context, id uint64) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return model.Image{}, ErrNotFound
}

func TestUploadAuthorizationProofAndLiveLinks(t *testing.T) {
	svc, rows, _, _, subject := uploadFixture(t, "png")
	subject.sourceTokenID = 5
	svc.deps.Tokens = uploadTokenRepo{token: model.Token{ID: 5, UserID: 1}}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	backend := svc.deps.Storages.(uploadStorageRepo)
	backend.backend.BaseURL = "https://new-host.test/prefix"
	svc.deps.Storages = backend
	current, err := svc.Get(t.Context(), subject, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(current.Links.URL, "https://new-host.test/prefix/") {
		t.Fatal("image URL was cached in persistence")
	}
	for _, row := range rows.rows {
		data, _ := json.Marshal(row)
		if strings.Contains(string(data), "https://") {
			t.Fatal("object URL persisted")
		}
	}
	expired := svc.deps.Now().Add(-time.Second)
	for _, proof := range []uploadTokenRepo{{err: ErrNotFound}, {token: model.Token{ID: 5, UserID: 2}}, {token: model.Token{ID: 5, UserID: 1, ExpiresAt: &expired}}} {
		svc.deps.Tokens = proof
		if _, err = svc.Preflight(t.Context(), subject, 0); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("revoked/wrong-owner/expired proof accepted: %v", err)
		}
	}
	svc.deps.Tokens = uploadTokenRepo{token: model.Token{ID: 5, UserID: 1}}
	stale := subject
	stale.passwordHash = "old-hash"
	if _, err = svc.Preflight(t.Context(), stale, 0); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("stale password proof accepted")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = svc.Upload(canceled, subject, UploadInput{Data: []byte("source"), Filename: "a.png"}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled upload accepted")
	}
}

type uploadProcessor struct {
	format  string
	failure bool
}

func (p uploadProcessor) Probe(_ context.Context, data []byte) (imaging.Info, error) {
	if p.failure {
		return imaging.Info{}, imaging.ErrInvalidImage
	}
	format := p.format
	if format == "" {
		format = "png"
	}
	if string(data) == "webp" {
		format = "webp"
	}
	return imaging.Info{Format: format, Ext: format, MIME: "image/" + format, Width: 10, Height: 8, LoadedFrames: 1, Orientation: 1}, nil
}

type resizedUploadProcessor struct{ uploadProcessor }

func (p resizedUploadProcessor) Probe(ctx context.Context, data []byte) (imaging.Info, error) {
	info, err := p.uploadProcessor.Probe(ctx, data)
	if string(data) == "webp" {
		info.Width = 4
		info.Height = 3
	}
	return info, err
}

// The service-layer allowlist check canonicalizes aliases the same way the
// reservation check does, so "jpeg"/"tif" entries govern jpg/tiff uploads.
func TestUploadAllowedExtAliases(t *testing.T) {
	svc, rows, policy, _, subject := uploadFixture(t, "jpg")
	policy.group.AllowedExts = []string{"jpeg"}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.jpg"})
	if err != nil {
		t.Fatalf(`allowlist ["jpeg"] refused a JPG upload: %v`, err)
	}
	if len(rows.rows) != 1 || view.Key == "" {
		t.Fatal("upload not committed")
	}
	policy.group.AllowedExts = []string{"tif"}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "b.jpg"}); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf(`allowlist ["tif"] accepted a JPG upload: %v`, err)
	}
}

// The stored original filename is bounded: multipart part headers have no
// independent limit, so one request must not park megabytes in origin_name.
func TestUploadOriginNameClamped(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	policy.policy.NameTpl = "{uniqid}"
	long := strings.Repeat("\u56fe", 400) + ".png"
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: long})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Name) > 255 || !utf8.ValidString(view.Name) || view.Name == "" {
		t.Fatalf("origin name not clamped to a valid prefix: %d bytes", len(view.Name))
	}
}

func TestUploadPrimaryDimensionsAndCollisionNames(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	policy.policy.WebPMode = "webp_only"
	svc.deps.Imaging = resizedUploadProcessor{}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Width != 4 || view.Height != 3 || view.Ext != "webp" || view.MIME != "image/webp" {
		t.Fatal("dimensions do not describe the stored primary")
	}
	view2, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view2.Links.WebP, "/a-1.webp") || view2.UserID != view.UserID || view2.StorageID != view.StorageID {
		t.Fatal("collision lost image fields or rename")
	}
	long := strings.Repeat("a", 100) + ".png"
	if _, err = svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: long}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: long}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized rename accepted: %v", err)
	}
}

type failingThumbCache struct{ ThumbCache }

func (f failingThumbCache) Put(context.Context, uint64, string, []byte) error {
	return errors.New("private-provider-marker")
}

func TestUploadLocalCacheFailureAndRecoveryRetainsPath(t *testing.T) {
	svc, rows, _, local, subject := uploadFixture(t, "png")
	svc.deps.Cache = failingThumbCache{ThumbCache: svc.deps.Cache}
	rows.cleanupFailure = true
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"}); err == nil || strings.Contains(err.Error(), "private-provider-marker") {
		t.Fatal("cache failure accepted or exposed")
	}
	if len(rows.rows) != 1 {
		t.Fatal("unfinished cleanup released path")
	}
	for _, row := range rows.rows {
		if row.Operation != model.ImageOperationCleanup {
			t.Fatal("cleanup not journaled")
		}
	}
	if _, err := local.Stat(t.Context(), "2026/10/a.png"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("cleanup retained cloud object")
	}
	rows.cleanupFailure = false
	if err := svc.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(rows.rows) != 0 {
		t.Fatal("startup recovery retained failed reservation")
	}
}

func TestUploadHEIFModesAndThumbDisabled(t *testing.T) {
	for _, mode := range []string{"webp_only", "keep", "reject"} {
		t.Run(mode, func(t *testing.T) {
			svc, _, policy, _, subject := uploadFixture(t, "heic")
			policy.policy.HEIFMode = mode
			policy.policy.ThumbEnabled = false
			if mode == "keep" {
				policy.policy.ScrubMode = "none"
			}
			view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.fake"})
			if mode == "reject" {
				if !errors.Is(err, ErrUnsupportedFormat) {
					t.Fatal("HEIF reject ignored")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if view.HasThumb || view.Links.Thumbnail != "" || view.LocalThumbURL != "" {
				t.Fatal("disabled thumbnail emitted")
			}
			if mode == "webp_only" && (view.HasOriginal || view.ChargedBytes != 4) {
				t.Fatal("HEIF original stored in only mode")
			}
		})
	}
}
func (p uploadProcessor) Process(ctx context.Context, data []byte, opts imaging.Options) (imaging.Result, error) {
	info, err := p.Probe(ctx, data)
	return imaging.Result{Info: info, WebP: []byte("webp"), Thumbnail: []byte("thumb")}, err
}

type uploadMetadata struct{ fail bool }

func (p uploadMetadata) Extract(context.Context, []byte, imaging.Info) (model.ImageExif, error) {
	gps := 1.25
	return model.ImageExif{GPSLat: &gps, Raw: json.RawMessage(`{"private":"gps-marker"}`)}, nil
}
func (p uploadMetadata) Scrub(_ context.Context, data []byte, _, _ string) ([]byte, error) {
	if p.fail {
		return nil, ErrProcessing
	}
	return append([]byte(nil), data...), nil
}

type failingUploadDriver struct {
	storage.Driver
	calls, failAt int
	afterWrite    bool
}

func (d *failingUploadDriver) PutNew(ctx context.Context, key string, body io.ReadSeeker, opts storage.PutOptions) (storage.Receipt, error) {
	d.calls++
	if d.calls == d.failAt && !d.afterWrite {
		return storage.Receipt{}, errors.New("injected secret-provider-detail")
	}
	receipt, err := d.Driver.PutNew(ctx, key, body, opts)
	if d.calls == d.failAt && d.afterWrite {
		return storage.Receipt{}, errors.New("write confirmation lost")
	}
	return receipt, err
}
func (d *failingUploadDriver) PurgeOwned(ctx context.Context, key, owner string) error {
	return d.Driver.(storage.OwnedPurger).PurgeOwned(ctx, key, owner)
}

func uploadFixture(t *testing.T, format string) (*ImageService, *uploadRepo, *uploadPolicy, *storage.Local, TokenSubject) {
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
	images := &uploadRepo{rows: map[string]model.Image{}}
	policy := &uploadPolicy{policy: model.Policy{ID: 1, StorageID: 1, PathTpl: "{Y}/{m}", NameTpl: "{filename}", WebPMode: "both", ScrubMode: "gps", HEIFMode: "webp_only", LinkPrefer: "webp", OnConflict: "rename", WebPQuality: 80, WebPEffort: 4, ThumbEnabled: true, ThumbSize: 400, Enabled: true}, backend: model.Storage{ID: 1, Driver: "local", BaseURL: "http://images.test/i/1", Enabled: true}, group: model.Group{ID: 1, AllowedExts: []string{"png", "webp", "heic"}, MaxFileBytes: 1024}}
	subject := TokenSubject{userID: 1, passwordHash: "verified"}
	svc, err := NewImageService(ctx, ImageDependencies{Images: images, Policies: policy, Storages: uploadStorageRepo{backend: policy.backend}, Users: uploadUserRepo{user: model.User{ID: 1, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusEnabled, Role: model.UserRoleUser}}, Tokens: &uploadTokenRepo{}, Drivers: uploadDriverProvider{driver: local}, Paths: PathFunctions{BuildPath: pathtpl.Build, CleanPath: pathtpl.Sanitize}, Imaging: uploadProcessor{format: format}, Extractor: uploadMetadata{}, Scrubber: uploadMetadata{}, Cache: cache, Settings: uploadSettings{}, Now: func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) }, MaxFileBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	return svc, images, policy, local, subject
}

func TestUploadPhysicalBytesAndMetadataBoundary(t *testing.T) {
	for _, tc := range []struct {
		mode, format   string
		original, webp bool
		bytes          int64
	}{{"both", "png", true, true, 15}, {"webp_only", "png", false, true, 9}, {"none", "png", true, false, 11}, {"both", "webp", true, true, 11}, {"webp_only", "webp", false, true, 11}} {
		t.Run(tc.mode+tc.format, func(t *testing.T) {
			svc, rows, policy, _, subject := uploadFixture(t, tc.format)
			policy.policy.WebPMode = tc.mode
			view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "旅行.untrusted", IsPublic: false})
			if err != nil {
				t.Fatal(err)
			}
			if view.ChargedBytes != tc.bytes || view.HasOriginal != tc.original || view.HasWebP != tc.webp {
				t.Fatalf("wrong version/charge: %+v", view)
			}
			raw, _ := json.Marshal(view)
			if strings.Contains(string(raw), "gps") || strings.Contains(string(raw), "raw") || strings.Contains(string(raw), "private") {
				t.Fatal("private metadata escaped DTO")
			}
			if len(rows.rows) != 1 || view.Links.URL == "" || view.LocalThumbURL == "" {
				t.Fatal("missing committed image or links")
			}
		})
	}
}

func TestUploadFailureCompensatesEveryAttemptedObject(t *testing.T) {
	for _, tc := range []struct {
		name               string
		failAt             int
		afterWrite, commit bool
	}{{"second", 2, false, false}, {"third", 3, false, false}, {"uncertainwrite", 2, true, false}, {"database", 0, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			svc, rows, _, local, subject := uploadFixture(t, "png")
			svc.deps.Drivers = uploadDriverProvider{driver: &failingUploadDriver{Driver: local, failAt: tc.failAt, afterWrite: tc.afterWrite}}
			if tc.commit {
				rows.commitErr = ErrStorage
			}
			_, uploadErr := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
			if uploadErr == nil {
				t.Fatal("injected failure accepted")
			}
			for _, key := range []string{"2026/10/a.png", "2026/10/a.webp", "2026/10/a_thumbs.webp"} {
				if _, statErr := local.Stat(t.Context(), key); !errors.Is(statErr, storage.ErrNotFound) {
					t.Fatalf("orphan %s: %v", key, statErr)
				}
			}
			if len(rows.rows) != 0 {
				t.Fatal("reservation released before cleanup or still present")
			}
			if strings.Contains(errText(uploadErr), "secret-provider-detail") {
				t.Fatal("provider error exposed")
			}
		})
	}
}
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestUploadCommitConfirmationLossKeepsActiveObjects(t *testing.T) {
	svc, rows, _, local, subject := uploadFixture(t, "png")
	rows.commitErr = ErrStorage
	rows.committedDespiteError = true
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil || view.Key == "" {
		t.Fatalf("committed upload lost: %v", err)
	}
	if _, err = local.Stat(t.Context(), "2026/10/a.png"); err != nil {
		t.Fatal("committed object deleted")
	}
}

func TestUploadScrubAndPreflightRejectBeforeStorage(t *testing.T) {
	svc, rows, _, local, subject := uploadFixture(t, "png")
	svc.deps.Scrubber = uploadMetadata{fail: true}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"}); !errors.Is(err, ErrProcessing) {
		t.Fatalf("scrub failure: %v", err)
	}
	if len(rows.rows) != 0 {
		t.Fatal("scrub failed after reservation")
	}
	if _, err := local.Stat(t.Context(), "2026/10/a.png"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("unsanitized original entered storage")
	}
	if _, err := svc.Preflight(t.Context(), TokenSubject{}, 0); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("anonymous preflight accepted")
	}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: make([]byte, 1025), Filename: "a"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("file cap ignored")
	}
}
