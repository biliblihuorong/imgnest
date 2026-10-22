package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/cache"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// variantProcessor reports every buffer as a 1600x900 still (or animation)
// and encodes fixed renditions, so tests see exactly which bytes are stored.
type variantProcessor struct {
	uploadProcessor
	frames int
	calls  *int
}

func (p variantProcessor) Probe(ctx context.Context, data []byte) (imaging.Info, error) {
	info, err := p.uploadProcessor.Probe(ctx, data)
	info.Width, info.Height = 1600, 900
	if p.frames > 0 {
		info.LoadedFrames = p.frames
	}
	return info, err
}

func (p variantProcessor) Variants(_ context.Context, data []byte, opts imaging.VariantOptions) ([]imaging.Variant, error) {
	if p.calls != nil {
		*p.calls++
	}
	if string(data) != "webp" {
		return nil, errors.New("variants not derived from the display WebP")
	}
	if opts.Probed == nil || opts.Probed.Width != 1600 {
		return nil, errors.New("source probe not reused")
	}
	return []imaging.Variant{{Width: 400, Height: 225, Data: []byte("v400")}, {Width: 800, Height: 450, Data: []byte("v800-data")}}, nil
}

func (r *uploadRepo) FindByPath(_ context.Context, storageID uint64, path string) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.StorageID == storageID && row.Path == path {
			return row, nil
		}
	}
	return model.Image{}, ErrNotFound
}

func (r uploadStorageRepo) List(context.Context) ([]model.Storage, error) {
	return []model.Storage{r.backend}, nil
}

func variantFixture(t *testing.T) (*ImageService, *uploadRepo, *uploadPolicy, *storage.Local, TokenSubject) {
	t.Helper()
	svc, rows, policy, local, subject := uploadFixture(t, "png")
	policy.policy.ResponsiveVariants = true
	policy.group.MaxFileBytes = 4096
	svc.deps.Imaging = variantProcessor{}
	return svc, rows, policy, local, subject
}

func TestUploadStoresResponsiveVariantsAndCountsThem(t *testing.T) {
	svc, rows, _, local, subject := variantFixture(t)
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	want := []VariantView{
		{URL: "http://images.test/i/1/2026/10/a_w400_v1.webp", Width: 400, Height: 225, Bytes: 4, MIME: "image/webp"},
		{URL: "http://images.test/i/1/2026/10/a_w800_v1.webp", Width: 800, Height: 450, Bytes: 9, MIME: "image/webp"},
		// The 1600px display WebP is reused instead of being encoded twice.
		{URL: "http://images.test/i/1/2026/10/a.webp", Width: 1600, Height: 900, Bytes: 4, MIME: "image/webp"},
	}
	if len(view.Variants) != len(want) {
		t.Fatalf("variants = %+v", view.Variants)
	}
	for i := range want {
		if view.Variants[i] != want[i] {
			t.Fatalf("variant %d = %+v, want %+v", i, view.Variants[i], want[i])
		}
	}
	if view.Links.Original != "http://images.test/i/1/2026/10/a.png" || view.Links.WebP != "http://images.test/i/1/2026/10/a.webp" || view.Links.Thumbnail != "http://images.test/i/1/2026/10/a_thumbs.webp" {
		t.Fatalf("existing links changed: %+v", view.Links)
	}
	// original + webp + cloud thumbnail + two new variant files
	if wantBytes := int64(len("source") + len("webp") + len("thumb") + len("v400") + len("v800-data")); view.ChargedBytes != wantBytes {
		t.Fatalf("charged %d bytes, want %d", view.ChargedBytes, wantBytes)
	}
	row := rows.rows[view.Key]
	if !declaresCloudKey(row, "2026/10/a_w400_v1.webp") || !declaresCloudKey(row, "2026/10/a_w800_v1.webp") {
		t.Fatal("variant objects missing from the manifest")
	}
	for _, key := range []string{"2026/10/a_w400_v1.webp", "2026/10/a_w800_v1.webp"} {
		info, err := local.Stat(t.Context(), key)
		if err != nil || info.OwnerID != view.Key || info.MIME != "image/webp" {
			t.Fatalf("variant %s not stored as owned WebP: %+v %v", key, info, err)
		}
		object, err := svc.OpenPublic(t.Context(), 1, key)
		if err != nil {
			t.Fatalf("variant %s not served: %v", key, err)
		}
		_ = object.Body.Close()
	}
}

func TestUploadVariantsSkippedForAnimationAndDisabledRule(t *testing.T) {
	for _, tc := range []struct {
		name    string
		frames  int
		enabled bool
	}{{name: "animation", frames: 2, enabled: true}, {name: "rule off", frames: 1, enabled: false}} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, policy, _, subject := variantFixture(t)
			calls := 0
			svc.deps.Imaging = variantProcessor{frames: tc.frames, calls: &calls}
			policy.policy.ResponsiveVariants = tc.enabled
			view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
			if err != nil {
				t.Fatal(err)
			}
			if view.Variants == nil || len(view.Variants) != 0 || calls != 0 {
				t.Fatalf("variants = %+v after %d encodes", view.Variants, calls)
			}
			if view.Frames != tc.frames || !view.HasWebP {
				t.Fatal("primary versions changed")
			}
		})
	}
}

func TestUploadVariantWriteFailureCompensatesEveryObject(t *testing.T) {
	for _, afterWrite := range []bool{false, true} {
		svc, rows, _, local, subject := variantFixture(t)
		// Writes: original, WebP, cloud thumbnail, 400px variant, 800px variant.
		failing := &failingUploadDriver{Driver: local, failAt: 5, afterWrite: afterWrite}
		svc.deps.Drivers = uploadDriverProvider{driver: failing}
		_, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
		if !errors.Is(err, ErrStorage) || strings.Contains(err.Error(), "secret-provider-detail") {
			t.Fatalf("variant write failure not reported safely: %v", err)
		}
		for _, key := range []string{"2026/10/a.png", "2026/10/a.webp", "2026/10/a_thumbs.webp", "2026/10/a_w400_v1.webp", "2026/10/a_w800_v1.webp"} {
			if _, err := local.Stat(t.Context(), key); !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("compensation left %s (afterWrite=%v)", key, afterWrite)
			}
		}
		if len(rows.rows) != 0 {
			t.Fatal("failed upload kept its reservation")
		}
	}
}

func TestLookupVariantsResolvesOnlyLiveManagedLinks(t *testing.T) {
	svc, rows, _, _, subject := variantFixture(t)
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	links := []string{
		view.Links.Original,
		view.Links.WebP + "?v=2#top",
		view.Variants[0].URL,
		view.Links.Thumbnail,
		"https://images.test/i/1/2026/10/a.png", // scheme is ignored
		"http://elsewhere.test/i/1/2026/10/a.png",
		"http://images.test/i/1/2026/10/missing.png",
		"http://images.test/i/1/2026/10/a.gif", // not an object of the image
		"http://images.test/i/1/_trash/2026/10/a.png",
		"javascript:alert(1)",
	}
	items, err := svc.LookupVariants(t.Context(), links)
	if err != nil {
		t.Fatal(err)
	}
	for i, item := range items {
		found := i < 5
		if item.URL != links[i] || item.Found != found || (found && len(item.Variants) != 3) || (!found && len(item.Variants) != 0) {
			t.Fatalf("lookup %d (%s) = %+v", i, links[i], item)
		}
	}
	row := rows.rows[view.Key]
	row.State = model.ImageStateTrash
	rows.rows[view.Key] = row
	items, err = svc.LookupVariants(t.Context(), links[:1])
	if err != nil || items[0].Found {
		t.Fatal("recycled image still resolves")
	}
	if _, err := svc.OpenPublic(t.Context(), 1, "2026/10/a_w400_v1.webp"); !errors.Is(err, ErrNotFound) {
		t.Fatal("recycled variant still served")
	}
	for _, bad := range [][]string{nil, make([]string, MaxVariantLookup+1), {strings.Repeat("a", 2049)}} {
		if _, err := svc.LookupVariants(t.Context(), bad); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid lookup accepted: %d links", len(bad))
		}
	}
}

func TestDirectBasesPreferVariantThenLegacyPath(t *testing.T) {
	got := directBases("2026/a_w400_v1.webp")
	if len(got) != 2 || got[0] != "2026/a" || got[1] != "2026/a_w400_v1" {
		t.Fatalf("bases = %v", got)
	}
	if got := directBases("2026/a_thumbs.webp"); len(got) != 2 || got[0] != "2026/a" {
		t.Fatalf("thumb bases = %v", got)
	}
	if got := directBases("_w400_v1.webp"); len(got) != 1 || got[0] != "_w400_v1" {
		t.Fatalf("bare suffix bases = %v", got)
	}
}

// panickingPolicies stands in for a decoder or driver bug deep inside one
// image's generation.
type panickingPolicies struct{ *uploadPolicy }

func (panickingPolicies) Find(context.Context, uint64) (model.Policy, error) {
	panic("corrupt image")
}

func TestBackfillOneTurnsPanicIntoFailure(t *testing.T) {
	svc, _, policy, _, _ := variantFixture(t)
	svc.deps.Policies = panickingPolicies{policy}
	// Generation needs the variant capability; the panic fires before any
	// of its methods is called.
	svc.deps.Images = struct {
		ImageRepository
		VariantRepository
	}{ImageRepository: svc.deps.Images}
	added, err := svc.backfillOne(t.Context(), model.Image{ID: 7, Key: "k", State: model.ImageStateActive, Frames: 1})
	if added || !errors.Is(err, ErrProcessing) {
		t.Fatalf("backfillOne = %v, %v; want a processing failure instead of a crash", added, err)
	}
}

func TestNewImageServiceRequiresSettings(t *testing.T) {
	svc, _, _, _, _ := uploadFixture(t, "png")
	deps := svc.deps
	deps.Settings = nil
	if _, err := NewImageService(t.Context(), deps); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing Settings accepted: %v", err)
	}
}

func TestLookupVariantsCachesAndTrashInvalidates(t *testing.T) {
	svc, rows, _, _, subject := variantFixture(t)
	svc.deps.LookupCache = cache.NewMemory(0, nil)
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	links := []string{view.Links.Original}
	if items, err := svc.LookupVariants(t.Context(), links); err != nil || !items[0].Found || len(items[0].Variants) != 3 {
		t.Fatalf("first lookup = %+v, %v", items, err)
	}
	// A change behind the service's back is not seen while the entry lives:
	// the second lookup is answered from the cache.
	rows.mu.Lock()
	row := rows.rows[view.Key]
	row.Variants = nil
	rows.rows[view.Key] = row
	rows.mu.Unlock()
	if items, _ := svc.LookupVariants(t.Context(), links); len(items[0].Variants) != 3 {
		t.Fatalf("second lookup read the database: %+v", items)
	}
	// Moving the image to the recycle bin drops the entry at once.
	if err := svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if items, err := svc.LookupVariants(t.Context(), links); err != nil || items[0].Found {
		t.Fatalf("recycled image still resolves from the cache: %+v, %v", items, err)
	}
}

// lostCommitVariants journals generation in memory and applies FinishVariants
// to the image before reporting finishErr, like a COMMIT whose
// acknowledgement was lost. hasErr and findErr fail the confirmation reads.
type lostCommitVariants struct {
	*uploadRepo
	finishErr, hasErr, findErr error
	pending, finished          bool
	job                        model.VariantJob
}

func (r *lostCommitVariants) FindByKey(ctx context.Context, key string) (model.Image, error) {
	if r.finished && r.findErr != nil {
		return model.Image{}, r.findErr
	}
	return r.uploadRepo.FindByKey(ctx, key)
}
func (r *lostCommitVariants) VariantCandidates(context.Context, uint64, int) ([]model.Image, error) {
	return nil, nil
}
func (r *lostCommitVariants) BeginVariants(_ context.Context, job model.VariantJob) (model.Image, error) {
	r.job, r.pending = job, true
	return r.uploadRepo.rows[job.ImageKey], nil
}
func (r *lostCommitVariants) RecordVariantReceipt(context.Context, uint64, string, model.ObjectReceipt) error {
	return nil
}
func (r *lostCommitVariants) FinishVariants(_ context.Context, _ uint64, _ string) (model.Image, error) {
	r.mu.Lock()
	row := r.rows[r.job.ImageKey]
	row.Variants = r.job.Variants
	row.ObjectManifest = append(row.ObjectManifest, r.job.Objects...)
	r.rows[r.job.ImageKey] = row
	r.mu.Unlock()
	r.pending, r.finished = false, true
	return model.Image{}, r.finishErr
}
func (r *lostCommitVariants) CancelVariants(context.Context, uint64, string) error {
	r.pending = false
	return nil
}
func (r *lostCommitVariants) PendingVariantJobs(context.Context) ([]model.VariantJob, error) {
	return nil, nil
}
func (r *lostCommitVariants) HasVariantJob(context.Context, uint64) (bool, error) {
	return r.pending, r.hasErr
}
func (r *lostCommitVariants) VariantBackfill(context.Context) (model.VariantBackfill, error) {
	return model.VariantBackfill{}, nil
}
func (r *lostCommitVariants) SaveVariantBackfill(context.Context, model.VariantBackfill) error {
	return nil
}

// A FinishVariants whose COMMIT landed but whose acknowledgement was lost
// must never purge the published variants, even when the confirmation reads
// fail too.
func TestGenerateVariantsKeepsObjectsWhenCommitIsUncertain(t *testing.T) {
	for _, tc := range []struct {
		name            string
		hasErr, findErr error
		wantAdded       bool
	}{
		{name: "journal read fails", hasErr: errors.New("connection reset")},
		{name: "image read fails", findErr: errors.New("connection reset")},
		{name: "confirmed", wantAdded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, rows, policy, local, subject := variantFixture(t)
			policy.policy.ResponsiveVariants = false
			view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
			if err != nil {
				t.Fatal(err)
			}
			policy.policy.ResponsiveVariants = true
			repo := &lostCommitVariants{uploadRepo: rows, finishErr: errors.New("commit acknowledgement lost"), hasErr: tc.hasErr, findErr: tc.findErr}
			svc.deps.Images = repo
			added, err := svc.generateVariants(t.Context(), rows.rows[view.Key])
			if added != tc.wantAdded || (err == nil) != tc.wantAdded {
				t.Fatalf("generateVariants = %v, %v", added, err)
			}
			if !tc.wantAdded && !strings.Contains(err.Error(), "recovery pending") {
				t.Fatalf("uncertain commit not reported as pending recovery: %v", err)
			}
			if len(rows.rows[view.Key].Variants) == 0 {
				t.Fatal("test setup: the commit did not publish variants")
			}
			for _, key := range []string{"2026/10/a_w400_v1.webp", "2026/10/a_w800_v1.webp"} {
				if _, err := local.Stat(t.Context(), key); err != nil {
					t.Fatalf("published variant %s was purged: %v", key, err)
				}
			}
		})
	}
}

// When the journal is still there the commit did not happen, and every
// written variant is compensated.
func TestGenerateVariantsCompensatesWhenJournalRemains(t *testing.T) {
	svc, rows, policy, local, subject := variantFixture(t)
	policy.policy.ResponsiveVariants = false
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	policy.policy.ResponsiveVariants = true
	repo := &stuckVariants{lostCommitVariants{uploadRepo: rows}}
	svc.deps.Images = repo
	if added, err := svc.generateVariants(t.Context(), rows.rows[view.Key]); added || err == nil {
		t.Fatalf("generateVariants = %v, %v", added, err)
	}
	for _, key := range []string{"2026/10/a_w400_v1.webp", "2026/10/a_w800_v1.webp"} {
		if _, err := local.Stat(t.Context(), key); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("uncommitted variant %s kept: %v", key, err)
		}
	}
	if repo.pending {
		t.Fatal("journal not cancelled")
	}
}

// stuckVariants fails FinishVariants before anything is committed.
type stuckVariants struct{ lostCommitVariants }

func (r *stuckVariants) FinishVariants(context.Context, uint64, string) (model.Image, error) {
	return model.Image{}, errors.New("database unavailable")
}
