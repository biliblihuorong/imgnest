package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/logx"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// VariantRepository journals background variant generation. The concrete
// image repository implements it; image services without it never backfill.
type VariantRepository interface {
	VariantCandidates(context.Context, uint64, int) ([]model.Image, error)
	BeginVariants(context.Context, model.VariantJob) (model.Image, error)
	RecordVariantReceipt(context.Context, uint64, string, model.ObjectReceipt) error
	FinishVariants(context.Context, uint64, string) (model.Image, error)
	CancelVariants(context.Context, uint64, string) error
	PendingVariantJobs(context.Context) ([]model.VariantJob, error)
	HasVariantJob(context.Context, uint64) (bool, error)
	VariantBackfill(context.Context) (model.VariantBackfill, error)
	SaveVariantBackfill(context.Context, model.VariantBackfill) error
}

// VariantView is one public responsive rendition. Width and Height are the
// encoded file's pixel size; Bytes is its exact byte size.
type VariantView struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int64  `json:"bytes"`
	MIME   string `json:"mime"`
}

func variantViews(variants []model.ImageVariant, baseURL string) []VariantView {
	views := make([]VariantView, 0, len(variants))
	for _, variant := range variants {
		views = append(views, VariantView{URL: objectURL(baseURL, variant.Key), Width: variant.Width, Height: variant.Height, Bytes: variant.Bytes, MIME: variant.MIME})
	}
	slices.SortFunc(views, func(a, b VariantView) int { return a.Width - b.Width })
	return views
}

// variantPlan holds the renditions of one image before their keys are known.
// reuse, when set, is the primary WebP itself at exactly a target width.
type variantPlan struct {
	generated []imaging.Variant
	reuse     *imaging.Variant
}

// planVariants encodes the responsive renditions of the display copy: the
// stored WebP when there is one (so display transformers such as watermarks
// carry over), otherwise the stored original. Animations get none.
func (s *ImageService) planVariants(ctx context.Context, policy model.Policy, source []byte, sourceIsWebP bool) (variantPlan, error) {
	encoder, ok := s.deps.Imaging.(imaging.VariantEncoder)
	if !policy.ResponsiveVariants || !ok || len(source) == 0 {
		return variantPlan{}, nil
	}
	info, err := s.deps.Imaging.Probe(ctx, source)
	if err != nil {
		return variantPlan{}, processingError(ctx, err)
	}
	if info.LoadedFrames != 1 {
		return variantPlan{}, nil
	}
	generated, err := encoder.Variants(ctx, source, imaging.VariantOptions{Widths: model.VariantWidths, Quality: policy.WebPQuality, Effort: policy.WebPEffort, Lossless: policy.WebPLossless, Probed: &info})
	if err != nil {
		return variantPlan{}, processingError(ctx, err)
	}
	plan := variantPlan{generated: generated}
	if sourceIsWebP && info.Format == "webp" && info.Orientation == 1 && slices.Contains(model.VariantWidths, info.Width) {
		plan.reuse = &imaging.Variant{Width: info.Width, Height: info.Height, Data: source}
	}
	return plan, nil
}

func (p variantPlan) empty() bool { return len(p.generated) == 0 && p.reuse == nil }

// variants names the plan's renditions under an image path; objects are the
// new files to write (a reused primary WebP is already written).
func (p variantPlan) variants(image model.Image) ([]model.ImageVariant, []uploadObject) {
	variants := []model.ImageVariant{}
	objects := []uploadObject{}
	for _, generated := range p.generated {
		key := model.VariantKey(image.Path, generated.Width)
		digest := sha256.Sum256(generated.Data)
		variants = append(variants, model.ImageVariant{Key: key, Width: generated.Width, Height: generated.Height, Bytes: int64(len(generated.Data)), MIME: model.VariantMIME})
		objects = append(objects, uploadObject{receipt: model.ObjectReceipt{Key: key, OwnerID: image.Key, Location: model.ObjectLocationCloud, MIME: model.VariantMIME, Size: int64(len(generated.Data)), SHA256: hex.EncodeToString(digest[:])}, data: generated.Data})
	}
	if p.reuse != nil && image.HasWebP {
		variants = append(variants, model.ImageVariant{Key: image.Path + ".webp", Width: p.reuse.Width, Height: p.reuse.Height, Bytes: int64(len(p.reuse.Data)), MIME: model.VariantMIME})
	}
	slices.SortFunc(variants, func(a, b model.ImageVariant) int { return a.Width - b.Width })
	return variants, objects
}

// directBases lists the image paths an object key may belong to: a variant,
// a cloud thumbnail or a primary file. Rows from before the reserved variant
// suffix existed can still own a path that looks like a variant, so every
// candidate is tried and the image's manifest decides.
func directBases(key string) []string {
	bases := []string{}
	if base, ok := model.VariantBase(key); ok {
		bases = append(bases, base)
	}
	if base, ok := strings.CutSuffix(key, "_thumbs.webp"); ok && base != "" {
		bases = append(bases, base)
	}
	if base := strings.TrimSuffix(key, path.Ext(key)); base != "" && !slices.Contains(bases, base) {
		bases = append(bases, base)
	}
	return bases
}

func declaresCloudKey(image model.Image, key string) bool {
	for _, receipt := range image.ObjectManifest {
		if receipt.Location == model.ObjectLocationCloud && receipt.Key == key {
			return true
		}
	}
	return false
}

// MaxVariantLookup bounds one public variant lookup.
const MaxVariantLookup = 50

// maxLookupURLBytes bounds each looked-up link.
const maxLookupURLBytes = 2048

// VariantLookupItem answers one looked-up link. Found is false for links
// ImgNest does not serve as a live image; Variants is then empty.
type VariantLookupItem struct {
	URL      string        `json:"url"`
	Found    bool          `json:"found"`
	Variants []VariantView `json:"variants"`
}

// LookupVariants maps existing direct links of live images to their stored
// responsive variants. It only parses the links and reads the database: no
// URL is ever fetched, and nothing beyond the variants' public links and
// sizes is returned. Recycled, pending and unknown images are not found.
func (s *ImageService) LookupVariants(ctx context.Context, links []string) ([]VariantLookupItem, error) {
	if len(links) == 0 || len(links) > MaxVariantLookup {
		return nil, ErrInvalidInput
	}
	for _, link := range links {
		if link == "" || len(link) > maxLookupURLBytes {
			return nil, ErrInvalidInput
		}
	}
	backends, err := s.deps.Storages.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list storages: %w", err)
	}
	type location struct {
		storage uint64
		path    string
	}
	found := map[location]lookupRecord{}
	items := make([]VariantLookupItem, 0, len(links))
	for _, link := range links {
		item := VariantLookupItem{URL: link, Variants: []VariantView{}}
		backend, key, ok := matchStorage(backends, link)
		if ok {
			for _, base := range directBases(key) {
				at := location{storage: backend.ID, path: base}
				record, cached := found[at]
				if !cached {
					if record, err = s.lookupRecord(ctx, backend.ID, base); err != nil {
						return nil, err
					}
					found[at] = record
				}
				if !record.Live || !slices.Contains(record.Keys, key) {
					continue
				}
				item.Found = true
				item.Variants = variantViews(record.Variants, backend.BaseURL)
				break
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// lookupCacheTTL matches the public max-age of a lookup response, so the
// server-side cache adds no staleness the HTTP cache did not already allow;
// trash, restore, uploads and new variants also drop the entry at once.
const lookupCacheTTL = 5 * time.Minute

// lookupRecord is what a lookup needs to know about the image stored at one
// path: whether it is live, its cloud object keys and its variants. Live is
// false when nothing is stored there or the image is not active.
type lookupRecord struct {
	Live     bool                 `json:"live"`
	Keys     []string             `json:"keys,omitempty"`
	Variants []model.ImageVariant `json:"variants,omitempty"`
}

func lookupCacheKey(storageID uint64, path string) string {
	return "variants:" + strconv.FormatUint(storageID, 10) + ":" + path
}

// lookupRecord reads through the lookup cache. A failing or undecodable
// cache only costs the database read.
func (s *ImageService) lookupRecord(ctx context.Context, storageID uint64, path string) (lookupRecord, error) {
	cacheKey := lookupCacheKey(storageID, path)
	if s.deps.LookupCache != nil {
		if raw, ok, err := s.deps.LookupCache.Get(ctx, cacheKey); err == nil && ok {
			var record lookupRecord
			if json.Unmarshal(raw, &record) == nil {
				return record, nil
			}
		}
	}
	record := lookupRecord{}
	row, err := s.deps.Images.FindByPath(ctx, storageID, path)
	switch {
	case err == nil:
		if row.State == model.ImageStateActive {
			record.Live, record.Variants = true, row.Variants
			for _, receipt := range row.ObjectManifest {
				if receipt.Location == model.ObjectLocationCloud {
					record.Keys = append(record.Keys, receipt.Key)
				}
			}
		}
	case errors.Is(err, ErrNotFound):
	default:
		return lookupRecord{}, fmt.Errorf("find linked image: %w", err)
	}
	if s.deps.LookupCache != nil {
		if raw, encodeErr := json.Marshal(record); encodeErr == nil {
			_ = s.deps.LookupCache.Set(ctx, cacheKey, raw, lookupCacheTTL)
		}
	}
	return record, nil
}

// invalidateLookup drops the cached lookup of an image whose live state or
// variants changed. Failures are ignored: the entry expires on its own.
func (s *ImageService) invalidateLookup(ctx context.Context, image model.Image) {
	if s.deps.LookupCache == nil || image.Path == "" {
		return
	}
	_ = s.deps.LookupCache.Delete(ctx, lookupCacheKey(image.StorageID, image.Path))
}

// matchStorage resolves a link to the storage whose base URL prefixes it,
// preferring the longest base, and returns the unescaped object key.
// Scheme, query and fragment are ignored; the host must match when the base
// names one.
func matchStorage(backends []model.Storage, link string) (model.Storage, string, bool) {
	target, err := url.Parse(link)
	if err != nil || (target.Scheme != "" && target.Scheme != "http" && target.Scheme != "https") {
		return model.Storage{}, "", false
	}
	var (
		best    model.Storage
		bestKey string
		bestLen = -1
	)
	for _, backend := range backends {
		base, err := url.Parse(strings.TrimRight(backend.BaseURL, "/"))
		if err != nil || backend.BaseURL == "" {
			continue
		}
		if base.Host != "" && !strings.EqualFold(base.Host, target.Host) {
			continue
		}
		if base.Host == "" && target.Host != "" && target.Scheme == "" {
			continue
		}
		prefix := strings.TrimRight(base.Path, "/") + "/"
		key, ok := strings.CutPrefix(target.Path, prefix)
		if !ok || key == "" || len(prefix) <= bestLen {
			continue
		}
		if path.Clean(key) != key || strings.HasPrefix(key, "_trash/") || strings.HasPrefix(key, ".trash/") {
			continue
		}
		best, bestKey, bestLen = backend, key, len(prefix)
	}
	return best, bestKey, bestLen >= 0
}

// variantRepo returns the optional journal capability of the image repository.
func (s *ImageService) variantRepo() (VariantRepository, bool) {
	repo, ok := s.deps.Images.(VariantRepository)
	return repo, ok
}

// generateVariants creates the missing responsive variants of one idle active
// image from its stored display copy. It reports whether variants were added.
// Writes are journaled first and compensated on failure, so a crash leaves a
// job that recovery removes, never an unlisted object.
func (s *ImageService) generateVariants(ctx context.Context, image model.Image) (bool, error) {
	repo, ok := s.variantRepo()
	if !ok {
		return false, ErrStorage
	}
	if image.State != model.ImageStateActive || image.Operation != "" || len(image.Variants) > 0 || image.Frames != 1 {
		return false, nil
	}
	policy, err := s.deps.Policies.Find(ctx, image.PolicyID)
	if err != nil {
		return false, fmt.Errorf("read image rule: %w", err)
	}
	if !policy.ResponsiveVariants {
		return false, nil
	}
	driver, _, err := s.imageDriver(ctx, image)
	if err != nil {
		return false, err
	}
	sourceKey, sourceIsWebP := image.Path+".webp", true
	if !image.HasWebP {
		sourceKey, sourceIsWebP = image.Path+"."+image.Ext, false
	}
	source, err := s.readOwnedObject(ctx, driver, image, sourceKey)
	if err != nil {
		return false, err
	}
	plan, err := s.planVariants(ctx, policy, source, sourceIsWebP)
	if err != nil || plan.empty() {
		return false, err
	}
	if err := s.lockOperations(ctx); err != nil {
		return false, err
	}
	defer s.unlockOperations()
	current, err := s.deps.Images.FindByKey(ctx, image.Key)
	if err != nil {
		return false, fmt.Errorf("re-read image: %w", err)
	}
	if current.ID != image.ID || current.Path != image.Path || current.StorageID != image.StorageID || current.State != model.ImageStateActive || current.Operation != "" || len(current.Variants) > 0 {
		return false, nil
	}
	op, err := operationID()
	if err != nil {
		return false, err
	}
	variants, objects := plan.variants(current)
	receipts := make([]model.ObjectReceipt, 0, len(objects))
	for _, object := range objects {
		receipts = append(receipts, object.receipt)
	}
	if _, err := repo.BeginVariants(ctx, model.VariantJob{ImageKey: current.Key, OperationID: op, Objects: receipts, Variants: variants, CreatedAt: s.deps.Now().UTC()}); err != nil {
		return false, fmt.Errorf("begin variants: %w", err)
	}
	for _, object := range objects {
		receipt := object.receipt
		written, err := driver.PutNew(ctx, receipt.Key, bytes.NewReader(object.data), storage.PutOptions{MIME: receipt.MIME, OwnerID: current.Key, CacheControl: liveCacheControl})
		if err == nil {
			receipt.VersionID = written.VersionID
			err = repo.RecordVariantReceipt(ctx, current.ID, op, receipt)
		}
		if err != nil {
			return false, s.failVariants(ctx, repo, current, op, receipts, driver, storageError(ctx, err))
		}
	}
	if _, err := repo.FinishVariants(ctx, current.ID, op); err != nil {
		// A lost COMMIT acknowledgement may already have published the
		// variants; only a journal still present proves they were not.
		checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		pending, checkErr := repo.HasVariantJob(checkCtx, current.ID)
		if checkErr != nil {
			return false, fmt.Errorf("confirm variants commit; recovery pending: %w", err)
		}
		if !pending {
			done, findErr := s.deps.Images.FindByKey(checkCtx, current.Key)
			if findErr != nil {
				return false, fmt.Errorf("confirm variants commit; recovery pending: %w", err)
			}
			if done.ID == current.ID && len(done.Variants) > 0 {
				s.invalidateLookup(checkCtx, current)
				return true, nil
			}
		}
		return false, s.failVariants(ctx, repo, current, op, receipts, driver, fmt.Errorf("finish variants: %w", err))
	}
	s.invalidateLookup(ctx, current)
	return true, nil
}

// readOwnedObject reads one stored object of an image, checking its owner
// and, when recorded, its SHA256.
func (s *ImageService) readOwnedObject(ctx context.Context, driver storage.Driver, image model.Image, key string) ([]byte, error) {
	body, info, err := driver.Open(ctx, key)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	limit := max(4*s.deps.MaxFileBytes, objectReadLimit(0, image))
	data, readErr := io.ReadAll(io.LimitReader(body, limit+1))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || int64(len(data)) > limit {
		return nil, ErrStorage
	}
	if !ownsObject(image, key, info) {
		return nil, ErrStorage
	}
	for _, receipt := range image.ObjectManifest {
		if receipt.Location != model.ObjectLocationCloud || receipt.Key != key || receipt.SHA256 == "" {
			continue
		}
		digest := sha256.Sum256(data)
		if int64(len(data)) != receipt.Size || hex.EncodeToString(digest[:]) != receipt.SHA256 {
			return nil, ErrStorage
		}
	}
	return data, nil
}

// ownsObject reports whether a stored object belongs to image: written by
// it, or the adopted original still at the version that was adopted.
func ownsObject(image model.Image, key string, info storage.ObjectInfo) bool {
	if info.OwnerID == image.Key {
		return true
	}
	if info.OwnerID != "" {
		return false
	}
	for _, receipt := range image.ObjectManifest {
		if receipt.Adopted && receipt.Location == model.ObjectLocationCloud && receipt.Key == key {
			return info.VersionID == receipt.VersionID && info.Size == receipt.Size
		}
	}
	return false
}

// failVariants removes every object the journal may have written, then
// releases the reservation. A failed removal leaves the journal for recovery.
func (s *ImageService) failVariants(ctx context.Context, repo VariantRepository, image model.Image, op string, receipts []model.ObjectReceipt, driver storage.Driver, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := purgeVariantObjects(cleanupCtx, driver, image.Key, receipts); err != nil {
		return errors.Join(cause, err)
	}
	if err := repo.CancelVariants(cleanupCtx, image.ID, op); err != nil {
		return errors.Join(cause, fmt.Errorf("cancel variants: %w", err))
	}
	return cause
}

func purgeVariantObjects(ctx context.Context, driver storage.Driver, owner string, receipts []model.ObjectReceipt) error {
	owned, ok := driver.(storage.OwnedPurger)
	if !ok {
		return ErrStorage
	}
	var failures []error
	for _, receipt := range receipts {
		if err := owned.PurgeOwned(ctx, receipt.Key, owner); err != nil && !errors.Is(err, storage.ErrOwnership) {
			failures = append(failures, storageError(ctx, err))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return fmt.Errorf("variant cleanup pending: %w", err)
	}
	return nil
}

// recoverVariantJobs compensates journals left by an interrupted generation.
// It runs under the lifecycle fence, which every generation holds while its
// journal exists, so each job found here is abandoned.
func (s *ImageService) recoverVariantJobs(ctx context.Context) []error {
	repo, ok := s.variantRepo()
	if !ok {
		return nil
	}
	jobs, err := repo.PendingVariantJobs(ctx)
	if err != nil {
		return []error{fmt.Errorf("read variant journal: %w", err)}
	}
	var failures []error
	for _, job := range jobs {
		image, err := s.deps.Images.FindByKey(ctx, job.ImageKey)
		if err == nil {
			var driver storage.Driver
			driver, _, err = s.imageDriver(ctx, image)
			if err == nil {
				err = purgeVariantObjects(ctx, driver, image.Key, job.Objects)
			}
		}
		if err == nil {
			err = repo.CancelVariants(ctx, job.ImageID, job.OperationID)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return failures
}

// variantJobPending blocks recycling an image whose variant journal still
// awaits compensation, so its objects are never orphaned by a purge.
func (s *ImageService) variantJobPending(ctx context.Context, image model.Image) error {
	repo, ok := s.variantRepo()
	if !ok {
		return nil
	}
	pending, err := repo.HasVariantJob(ctx, image.ID)
	if err != nil {
		return fmt.Errorf("read variant journal: %w", err)
	}
	if pending {
		return ErrImageBusy
	}
	return nil
}

// Backfill pacing: one image at a time with a pause between images, so the
// historic task never competes with uploads for more than one imaging slot.
const (
	backfillBatch = 20
	backfillPause = 500 * time.Millisecond
)

// VariantBackfillView reports the historic backfill progress.
type VariantBackfillView struct {
	model.VariantBackfill
	// Active is true while this process is generating.
	Active bool `json:"active"`
}

type backfillRunner struct {
	mu     sync.Mutex
	base   context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// StartBackground binds background work to the server's lifetime and resumes
// a backfill that was running when the server stopped.
func (s *ImageService) StartBackground(ctx context.Context) error {
	s.backfill.mu.Lock()
	s.backfill.base = ctx
	s.backfill.mu.Unlock()
	repo, ok := s.variantRepo()
	if !ok {
		return nil
	}
	state, err := repo.VariantBackfill(ctx)
	if err != nil {
		return fmt.Errorf("read variant backfill: %w", err)
	}
	if state.Status == model.BackfillRunning {
		s.launchBackfill()
	}
	return nil
}

// WaitBackground stops the backfill without changing its saved status, so
// the next start resumes it.
func (s *ImageService) WaitBackground() {
	s.backfill.mu.Lock()
	cancel, done := s.backfill.cancel, s.backfill.done
	s.backfill.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *ImageService) adminActor(ctx context.Context, subject TokenSubject) error {
	actor, err := s.actor(ctx, subject)
	if err != nil {
		return err
	}
	if actor.Role != model.UserRoleAdmin {
		return ErrForbidden
	}
	return nil
}

// VariantBackfillStatus returns the backfill progress to administrators.
func (s *ImageService) VariantBackfillStatus(ctx context.Context, subject TokenSubject) (VariantBackfillView, error) {
	if err := s.adminActor(ctx, subject); err != nil {
		return VariantBackfillView{}, err
	}
	return s.backfillView(ctx)
}

func (s *ImageService) backfillView(ctx context.Context) (VariantBackfillView, error) {
	repo, ok := s.variantRepo()
	if !ok {
		return VariantBackfillView{}, ErrStorage
	}
	state, err := repo.VariantBackfill(ctx)
	if err != nil {
		return VariantBackfillView{}, fmt.Errorf("read variant backfill: %w", err)
	}
	s.backfill.mu.Lock()
	active := s.backfill.cancel != nil
	s.backfill.mu.Unlock()
	return VariantBackfillView{VariantBackfill: state, Active: active}, nil
}

// StartVariantBackfill starts or resumes the historic backfill; restart
// begins again from the first image and resets the counters.
func (s *ImageService) StartVariantBackfill(ctx context.Context, subject TokenSubject, restart bool) (VariantBackfillView, error) {
	if err := s.adminActor(ctx, subject); err != nil {
		return VariantBackfillView{}, err
	}
	repo, ok := s.variantRepo()
	if !ok {
		return VariantBackfillView{}, ErrStorage
	}
	s.stopBackfill()
	state, err := repo.VariantBackfill(ctx)
	if err != nil {
		return VariantBackfillView{}, fmt.Errorf("read variant backfill: %w", err)
	}
	if restart || state.Status == model.BackfillDone {
		state = model.VariantBackfill{}
	}
	state.Status = model.BackfillRunning
	state.UpdatedAt = s.deps.Now().UTC()
	if err := repo.SaveVariantBackfill(ctx, state); err != nil {
		return VariantBackfillView{}, fmt.Errorf("save variant backfill: %w", err)
	}
	s.launchBackfill()
	return s.backfillView(ctx)
}

// PauseVariantBackfill stops the backfill after the current image; a later
// start continues from the saved cursor.
func (s *ImageService) PauseVariantBackfill(ctx context.Context, subject TokenSubject) (VariantBackfillView, error) {
	if err := s.adminActor(ctx, subject); err != nil {
		return VariantBackfillView{}, err
	}
	repo, ok := s.variantRepo()
	if !ok {
		return VariantBackfillView{}, ErrStorage
	}
	s.stopBackfill()
	state, err := repo.VariantBackfill(ctx)
	if err != nil {
		return VariantBackfillView{}, fmt.Errorf("read variant backfill: %w", err)
	}
	if state.Status == model.BackfillRunning {
		state.Status = model.BackfillPaused
		state.UpdatedAt = s.deps.Now().UTC()
		if err := repo.SaveVariantBackfill(ctx, state); err != nil {
			return VariantBackfillView{}, fmt.Errorf("save variant backfill: %w", err)
		}
	}
	return s.backfillView(ctx)
}

func (s *ImageService) stopBackfill() {
	s.backfill.mu.Lock()
	cancel, done := s.backfill.cancel, s.backfill.done
	s.backfill.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *ImageService) launchBackfill() {
	s.backfill.mu.Lock()
	defer s.backfill.mu.Unlock()
	if s.backfill.cancel != nil {
		return
	}
	base := s.backfill.base
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	done := make(chan struct{})
	s.backfill.cancel, s.backfill.done = cancel, done
	go func() {
		defer func() {
			s.backfill.mu.Lock()
			s.backfill.cancel, s.backfill.done = nil, nil
			s.backfill.mu.Unlock()
			cancel()
			close(done)
		}()
		_ = s.runBackfill(ctx)
	}()
}

// backfillOne generates one image's variants and turns a panic into a
// failure, so a decoder bug on one corrupt file cannot take the process down
// from this background goroutine (the HTTP recovery never sees it). A job log
// left half-written is cleaned by the next startup recovery.
func (s *ImageService) backfillOne(ctx context.Context, image model.Image) (added bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			added, err = false, fmt.Errorf("generate variants for image %d: %w", image.ID, ErrProcessing)
			s.deps.Logger.ErrorContext(ctx, "variant backfill panicked", append([]any{"image_id", image.ID}, logx.Panic(recovered)...)...)
			return
		}
		if err != nil && ctx.Err() == nil {
			s.deps.Logger.WarnContext(ctx, "variant backfill image failed", "image_id", image.ID, "error", logx.Redact(err.Error()))
		}
	}()
	return s.generateVariants(ctx, image)
}

// runBackfill walks idle active images in ID order and saves the cursor
// after each one, so a pause, crash or restart resumes where it stopped.
// Images that fail are counted and left without variants.
func (s *ImageService) runBackfill(ctx context.Context) error {
	repo, ok := s.variantRepo()
	if !ok {
		return ErrStorage
	}
	for ctx.Err() == nil {
		state, err := repo.VariantBackfill(ctx)
		if err != nil {
			return err
		}
		if state.Status != model.BackfillRunning {
			return nil
		}
		batch, err := repo.VariantCandidates(ctx, state.Cursor, backfillBatch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			state.Status = model.BackfillDone
			state.UpdatedAt = s.deps.Now().UTC()
			return repo.SaveVariantBackfill(ctx, state)
		}
		for _, image := range batch {
			added, genErr := s.backfillOne(ctx, image)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			state.Processed++
			switch {
			case genErr != nil:
				state.Failed++
			case added:
				state.Generated++
			default:
				state.Skipped++
			}
			state.Cursor = image.ID
			state.UpdatedAt = s.deps.Now().UTC()
			if err := repo.SaveVariantBackfill(ctx, state); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backfillPause):
			}
		}
	}
	return ctx.Err()
}
