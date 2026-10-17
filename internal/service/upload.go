package service

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 -- content-addressing compatibility, never credential authentication.
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- persisted content checksum required by the public contract.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/exif"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

func (s *ImageService) actor(ctx context.Context, subject TokenSubject) (model.User, error) {
	if err := ctx.Err(); err != nil {
		return model.User{}, fmt.Errorf("authorize image operation: %w", err)
	}
	if subject.userID == 0 || subject.passwordHash == "" {
		return model.User{}, ErrUnauthenticated
	}
	user, err := s.deps.Users.FindUserByID(ctx, subject.userID)
	if errors.Is(err, ErrNotFound) {
		return model.User{}, ErrUnauthenticated
	}
	if err != nil {
		return model.User{}, fmt.Errorf("read image actor: %w", err)
	}
	if user.Status != model.UserStatusEnabled || user.PasswordHash != subject.passwordHash {
		return model.User{}, ErrUnauthenticated
	}
	if subject.sourceTokenID != 0 {
		token, err := s.deps.Tokens.FindToken(ctx, subject.sourceTokenID)
		if errors.Is(err, ErrNotFound) {
			return model.User{}, ErrUnauthenticated
		}
		if err != nil {
			return model.User{}, fmt.Errorf("read image authorization: %w", err)
		}
		if token.UserID != user.ID || (token.ExpiresAt != nil && !token.ExpiresAt.After(s.deps.Now().UTC())) {
			return model.User{}, ErrUnauthenticated
		}
	}
	return user, nil
}

func (s *ImageService) grant(subject TokenSubject) model.TokenGrant {
	return model.TokenGrant{UserID: subject.userID, ExpectedPasswordHash: subject.passwordHash, SourceTokenID: subject.sourceTokenID, At: s.deps.Now().UTC(), Clock: s.deps.Now}
}

// Preflight validates the actor and selected rule before a multipart body is read.
func (s *ImageService) Preflight(ctx context.Context, subject TokenSubject, policyID uint64) (UploadLimits, error) {
	user, err := s.actor(ctx, subject)
	if err != nil {
		return UploadLimits{}, err
	}
	policy, backend, group, err := s.deps.Policies.UploadPolicy(ctx, user.ID, policyID)
	if err != nil {
		return UploadLimits{}, fmt.Errorf("select upload policy: %w", err)
	}
	if !policy.Enabled || !backend.Enabled {
		return UploadLimits{}, ErrForbidden
	}
	if group.CapacityBytes > 0 && user.UsedBytes >= group.CapacityBytes {
		return UploadLimits{}, ErrQuotaExceeded
	}
	return s.groupLimits(group), nil
}

// PreflightLimits resolves the actor and the group-level bounds that gate a
// multipart body before the form fields have named a rule. Unlike Preflight
// it does not require the group's default rule to be usable: a request that
// names an explicit, usable rule in its fields is judged by Preflight once
// those fields have been read.
func (s *ImageService) PreflightLimits(ctx context.Context, subject TokenSubject) (UploadLimits, error) {
	user, err := s.actor(ctx, subject)
	if err != nil {
		return UploadLimits{}, err
	}
	group, err := s.deps.Policies.UploadGroup(ctx, user.ID)
	if err != nil {
		return UploadLimits{}, fmt.Errorf("resolve upload group: %w", err)
	}
	if group.CapacityBytes > 0 && user.UsedBytes >= group.CapacityBytes {
		return UploadLimits{}, ErrQuotaExceeded
	}
	return s.groupLimits(group), nil
}

// metadataSourceLimit is the largest upload the metadata extractor and
// scrubber read (a variable so tests need not allocate gigabytes).
var metadataSourceLimit = exif.MaxSourceBytes

// groupLimits derives the request-level read bound and per-minute rate from
// the group caps; they never depend on the rule a request selects. A group's
// own per-file limit wins over server.max_upload_mb in both directions: the
// administrator may allow files of any size and owns the memory cost.
func (s *ImageService) groupLimits(group model.Group) UploadLimits {
	cap := s.deps.MaxFileBytes
	if group.MaxFileBytes > 0 {
		cap = group.MaxFileBytes
	}
	return UploadLimits{MaxFileBytes: cap, PerMinute: group.UploadPerMin}
}

// UploadLimitsFor returns the authenticated user's per-file limit and
// upload rate so a client can check files before sending them. Unlike
// PreflightLimits it answers even when the account is full.
func (s *ImageService) UploadLimitsFor(ctx context.Context, subject TokenSubject) (UploadLimits, error) {
	user, err := s.actor(ctx, subject)
	if err != nil {
		return UploadLimits{}, err
	}
	group, err := s.deps.Policies.UploadGroup(ctx, user.ID)
	if err != nil {
		return UploadLimits{}, fmt.Errorf("resolve upload group: %w", err)
	}
	return s.groupLimits(group), nil
}

type uploadObject struct {
	receipt model.ObjectReceipt
	data    []byte
}

// uploadPlan binds one upload to its authorization mode: account uploads
// re-verify the user row and credential grant inside the reservation, while
// guest uploads resolve the guest group without touching any user row.
type uploadPlan struct {
	userID  uint64
	policy  func(context.Context, uint64) (model.Policy, model.Storage, model.Group, error)
	reserve func(context.Context, model.UploadReservation) (model.Image, error)
	commit  func(context.Context, string, string, model.ImageExif) (model.Image, error)
	// pinned, when set, replaces the rule's path templates for an operator
	// import that must keep a historic link: the path is used as is, a
	// collision is never renamed and the stored extension must stay pinned.ext.
	pinned *pinnedPath
}

// pinnedPath is the historic path (without extension), stored extension and
// creation time an imported image must keep.
type pinnedPath struct {
	path, ext string
	createdAt time.Time
	// adoptVersion, when set, is the version of the original already in
	// the bucket that the image takes over instead of writing its own.
	adoptVersion string
}

// Upload prepares all versions and publishes them with compensating cleanup.
func (s *ImageService) Upload(ctx context.Context, subject TokenSubject, input UploadInput) (ImageView, error) {
	limits, err := s.Preflight(ctx, subject, input.PolicyID)
	if err != nil {
		return ImageView{}, err
	}
	grant := s.grant(subject)
	return s.publish(ctx, input, limits, uploadPlan{
		userID: subject.userID,
		policy: func(ctx context.Context, policyID uint64) (model.Policy, model.Storage, model.Group, error) {
			return s.deps.Policies.UploadPolicy(ctx, subject.userID, policyID)
		},
		reserve: func(ctx context.Context, reservation model.UploadReservation) (model.Image, error) {
			reservation.Grant = grant
			return s.deps.Images.ReserveUpload(ctx, reservation)
		},
		commit: func(ctx context.Context, key, op string, exif model.ImageExif) (model.Image, error) {
			return s.deps.Images.CommitUpload(ctx, key, op, exif, grant)
		},
	})
}

// publish runs the shared upload pipeline: probe, scrub, process, reserve,
// write objects with compensating cleanup, then commit metadata and quota.
func (s *ImageService) publish(ctx context.Context, input UploadInput, limits UploadLimits, plan uploadPlan) (ImageView, error) {
	if len(input.Data) == 0 || int64(len(input.Data)) > limits.MaxFileBytes || input.Filename == "" {
		return ImageView{}, ErrInvalidInput
	}
	policy, backend, group, err := plan.policy(ctx, input.PolicyID)
	if err != nil {
		return ImageView{}, fmt.Errorf("select upload rule: %w", err)
	}
	info, err := s.deps.Imaging.Probe(ctx, input.Data)
	if err != nil {
		return ImageView{}, processingFailure(ctx, ProcessingDecode, err)
	}
	if !allowedFormat(group.AllowedExts, info.Ext) {
		return ImageView{}, ErrUnsupportedFormat
	}
	// An adopted original stays in the bucket exactly as it is, so it is
	// neither scrubbed nor replaced by its WebP.
	adopt := plan.pinned != nil && plan.pinned.adoptVersion != ""
	// Metadata parsing and lossless scrubbing stop at exif.MaxSourceBytes,
	// while a group may allow larger files. Such a file is still stored: it
	// gets no EXIF record, and under an enabled scrub mode its original,
	// which cannot be cleaned, is replaced by the metadata-free WebP below.
	oversized := len(input.Data) > metadataSourceLimit
	metadata := model.ImageExif{Orientation: max(info.Orientation, 1)}
	if !oversized {
		metadata, err = s.deps.Extractor.Extract(ctx, input.Data, info)
	}
	if err != nil {
		// Nothing rewrites an adopted original, so metadata the parser
		// cannot read is left in it and simply not recorded.
		if !adopt || ctx.Err() != nil {
			return ImageView{}, processingFailure(ctx, ProcessingMetadata, err)
		}
		metadata = model.ImageExif{Orientation: max(info.Orientation, 1)}
	}
	// Lsky could name a PNG or JPEG ".webp" without converting it. The old
	// link must stay a WebP link: a copy keeps only the real WebP, and an
	// adopted original keeps its key and gets no separate WebP, whose key
	// would be its own.
	mislabeled := plan.pinned != nil && plan.pinned.ext == "webp" && info.Format != "webp"
	mode := policy.WebPMode
	heif := info.Format == "heic" || info.Format == "heif" || info.Format == "avif"
	if heif {
		switch policy.HEIFMode {
		case "webp_only":
			mode = "webp_only"
		case "keep":
			if policy.ScrubMode != "none" && !adopt {
				return ImageView{}, ErrProcessing
			}
		case "reject":
			return ImageView{}, ErrUnsupportedFormat
		default:
			return ImageView{}, ErrInvalidInput
		}
	}
	if adopt && mode == "webp_only" {
		mode = "both"
	}
	if mislabeled && !adopt {
		mode = "webp_only"
	}
	keepsOriginal := func() bool { return !adopt && (mode != "webp_only" || info.Format == "webp") }
	if oversized && keepsOriginal() && policy.ScrubMode != "none" {
		// A WebP source is its own display copy, and an import must keep
		// its historic extension, so neither can fall back to a WebP alone.
		if info.Format == "webp" || plan.pinned != nil {
			return ImageView{}, errScrubTooLarge
		}
		mode = "webp_only"
	}
	original := input.Data
	scrubbed := false
	if keepsOriginal() && !oversized {
		original, err = s.deps.Scrubber.Scrub(ctx, input.Data, info.Format, policy.ScrubMode)
		if err != nil || len(original) == 0 {
			return ImageView{}, processingFailure(ctx, ProcessingScrub, err)
		}
		check, probeErr := s.deps.Imaging.Probe(ctx, original)
		if probeErr != nil || check.Format != info.Format || check.Width != info.Width || check.Height != info.Height || check.LoadedFrames != info.LoadedFrames {
			return ImageView{}, processingFailure(ctx, ProcessingScrub, probeErr)
		}
		scrubbed = policy.ScrubMode != "none"
	}
	// input.Data is the buffer Probe validated above; reuse that decode.
	result, err := s.deps.Imaging.Process(ctx, input.Data, imaging.Options{Probed: &info, WebPMode: mode, Quality: policy.WebPQuality, Effort: policy.WebPEffort, Lossless: policy.WebPLossless, MaxWidth: policy.MaxWidth, MaxHeight: policy.MaxHeight, SkipIfLarger: policy.SkipIfLarger, ThumbEnabled: policy.ThumbEnabled, ThumbSize: policy.ThumbSize})
	if err != nil {
		return ImageView{}, processingFailure(ctx, ProcessingConvert, err)
	}
	if len(s.deps.Inspectors) > 0 {
		display := original
		if info.Format != "webp" && mode != "none" && len(result.WebP) > 0 {
			display = result.WebP
		}
		if err := s.inspect(ctx, UploadInspection{UserID: plan.userID, GroupID: group.ID, PolicyID: policy.ID, StorageID: backend.ID, Filename: input.Filename, Format: info.Format, Width: info.Width, Height: info.Height, Frames: info.LoadedFrames, Size: int64(len(input.Data))}, display); err != nil {
			return ImageView{}, err
		}
	}
	if info.Format != "webp" && mode != "none" && len(result.WebP) > 0 && len(s.deps.Display) > 0 {
		result.WebP, err = s.transformDisplay(ctx, max(s.deps.MaxFileBytes, int64(len(input.Data))), DisplayImage{UserID: plan.userID, GroupID: group.ID, PolicyID: policy.ID, StorageID: backend.ID, Format: info.Format, Quality: policy.WebPQuality, Effort: policy.WebPEffort, Lossless: policy.WebPLossless}, result.WebP)
		if err != nil {
			return ImageView{}, err
		}
	}
	sourceFrames := info.LoadedFrames
	hasOriginal := mode != "webp_only"
	webp := result.WebP
	if info.Format == "webp" {
		webp = original
	}
	if mode == "none" && info.Format != "webp" {
		webp = nil
	}
	if mode == "both" && info.Format != "webp" && policy.SkipIfLarger && len(webp) > len(original) {
		webp = nil
	}
	if mislabeled && adopt {
		webp = nil
	}
	if mode == "webp_only" && len(webp) == 0 {
		return ImageView{}, ErrProcessing
	}
	if policy.ThumbEnabled && len(result.Thumbnail) == 0 {
		return ImageView{}, ErrProcessing
	}
	if !policy.ThumbEnabled {
		result.Thumbnail = nil
	}
	primary := original
	sourceExt, storedExt, storedMIME := info.Ext, info.Ext, info.MIME
	if adopt {
		// The adopted object keeps its key; its MIME stays the real format.
		storedExt = plan.pinned.ext
	}
	if !hasOriginal {
		primary = webp
		storedExt = "webp"
		storedMIME = "image/webp"
		info, err = s.deps.Imaging.Probe(ctx, primary)
		if err != nil || info.Format != "webp" {
			return ImageView{}, processingError(ctx, err)
		}
	}
	// Responsive renditions come from the final display copy, after any
	// display transformer, so they show exactly what the primary link shows.
	variantSource, variantFromWebP := webp, true
	if len(webp) == 0 {
		variantSource, variantFromWebP = primary, false
	}
	var variants variantPlan
	if sourceFrames == 1 {
		variants, err = s.planVariants(ctx, policy, variantSource, variantFromWebP)
		if err != nil {
			return ImageView{}, err
		}
	}
	srcMD5, srcSHA1 := contentHashes(input.Data)
	storedMD5, storedSHA1 := contentHashes(primary)
	imageKey, err := operationID()
	if err != nil {
		return ImageView{}, err
	}
	op, err := operationID()
	if err != nil {
		return ImageView{}, err
	}
	image := model.Image{UserID: plan.userID, AlbumID: input.AlbumID, PolicyID: policy.ID, StorageID: backend.ID, Key: imageKey, Ext: storedExt, OriginName: clampOriginName(path.Base(strings.ReplaceAll(input.Filename, "\\", "/"))), MIME: storedMIME, SrcMD5: srcMD5, MD5: storedMD5, SHA1: storedSHA1, IP: input.IP, State: model.ImageStatePending, Operation: model.ImageOperationUpload, OperationID: op, HasOriginal: hasOriginal, HasWebP: len(webp) > 0, HasThumb: len(result.Thumbnail) > 0, Scrubbed: scrubbed, IsPublic: input.IsPublic, Size: int64(len(primary)), WebPSize: int64(len(webp)), ThumbBytes: int64(len(result.Thumbnail)), Width: info.Width, Height: info.Height, Frames: info.LoadedFrames, CreatedAt: s.deps.Now().UTC()}
	if plan.pinned != nil {
		if storedExt != plan.pinned.ext {
			return ImageView{}, ErrImportLinkChanged
		}
		if !plan.pinned.createdAt.IsZero() {
			image.CreatedAt = plan.pinned.createdAt.UTC()
		}
	}
	driver, err := s.deps.Drivers.DriverFor(ctx, backend)
	if err != nil {
		return ImageView{}, storageError(ctx, err)
	}
	if adopt {
		if err := checkAdoptable(ctx, driver, plan.pinned, int64(len(input.Data))); err != nil {
			return ImageView{}, err
		}
	}
	vars := pathtpl.Variables{Time: image.CreatedAt, UserID: image.UserID, Filename: input.Filename, MD5: srcMD5, SHA1: srcSHA1}
	var rendered pathtpl.Result
	if plan.pinned != nil {
		rendered.Path = plan.pinned.path
	} else {
		rendered, err = s.deps.Paths.Build(ctx, policy.PathTpl, policy.NameTpl, vars)
		if err != nil {
			return ImageView{}, fmt.Errorf("render image path: %w", ErrInvalidInput)
		}
	}
	var objects []uploadObject
	reserved := false
	basePath := rendered.Path
	for attempt := 0; attempt < 1000; attempt++ {
		if attempt > 0 {
			if plan.pinned != nil {
				return ImageView{}, ErrPathConflict
			}
			if rendered.HasRandom {
				if attempt >= 5 {
					return ImageView{}, ErrPathConflict
				}
				rendered, err = s.deps.Paths.Build(ctx, policy.PathTpl, policy.NameTpl, vars)
				if err != nil {
					return ImageView{}, ErrInvalidInput
				}
			} else {
				if policy.OnConflict != "rename" {
					return ImageView{}, ErrPathConflict
				}
				rendered.Path = basePath + "-" + strconv.Itoa(attempt)
			}
		}
		image.Path = rendered.Path
		checked, pathErr := s.deps.Paths.Sanitize(ctx, image.Path)
		if pathErr != nil || checked != image.Path {
			return ImageView{}, ErrInvalidInput
		}
		objects = makeUploadObjects(image, original, webp, result.Thumbnail)
		if adopt {
			objects = adoptOriginal(image, objects, plan.pinned.adoptVersion)
		}
		var variantObjects []uploadObject
		image.Variants, variantObjects = variants.variants(image)
		objects = append(objects, variantObjects...)
		manifests := make([]model.ObjectReceipt, 0, len(objects))
		image.ChargedBytes = 0
		for _, object := range objects {
			manifests = append(manifests, object.receipt)
			if object.receipt.Location == model.ObjectLocationCloud {
				if object.receipt.Size > math.MaxInt64-image.ChargedBytes {
					return ImageView{}, ErrQuotaExceeded
				}
				image.ChargedBytes += object.receipt.Size
			}
		}
		var reservation model.Image
		reservation, err = plan.reserve(ctx, model.UploadReservation{Image: image, Objects: manifests, SourceExt: sourceExt, Storage: backend})
		if errors.Is(err, ErrPathConflict) {
			continue
		}
		if err != nil {
			return ImageView{}, fmt.Errorf("reserve upload: %w", err)
		}
		image = reservation
		reserved = true
		break
	}
	if !reserved {
		return ImageView{}, ErrPathConflict
	}
	for _, object := range objects {
		receipt := object.receipt
		if receipt.Adopted {
			// Already in the bucket; the reservation recorded its version.
			continue
		}
		if receipt.Location == model.ObjectLocationThumbCache {
			err = s.deps.Cache.Put(ctx, image.StorageID, receipt.Key, object.data)
		} else {
			var written storage.Receipt
			written, err = driver.PutNew(ctx, receipt.Key, bytes.NewReader(object.data), storage.PutOptions{MIME: receipt.MIME, OwnerID: image.Key, CacheControl: liveCacheControl})
			if err == nil {
				receipt.VersionID = written.VersionID
				receipt.Size = written.Size
				receipt.OwnerID = written.OwnerID
			}
		}
		if err == nil {
			err = s.deps.Images.RecordObjectReceipt(ctx, image.Key, op, receipt)
		}
		if err != nil {
			return ImageView{}, s.failUpload(ctx, image, driver, storageError(ctx, err))
		}
	}
	committed, err := plan.commit(ctx, image.Key, op, metadata)
	if err != nil {
		// A lost COMMIT acknowledgement must never compensate an already active image.
		checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		current, checkErr := s.deps.Images.FindByKey(checkCtx, image.Key)
		cancel()
		if checkErr == nil && current.State == model.ImageStateActive && current.OperationID == op {
			s.invalidateRandom(ctx, current.AlbumID)
			s.invalidateLookup(ctx, current)
			s.emit(ctx, EventImageUploaded, current, backend)
			return imageView(current, backend, policy), nil
		}
		if checkErr != nil {
			return ImageView{}, fmt.Errorf("confirm upload commit; recovery pending: %w", err)
		}
		return ImageView{}, s.failUpload(ctx, image, driver, fmt.Errorf("commit upload: %w", err))
	}
	s.invalidateRandom(ctx, committed.AlbumID)
	s.invalidateLookup(ctx, committed)
	s.emit(ctx, EventImageUploaded, committed, backend)
	return imageView(committed, backend, policy), nil
}

// inspect runs every upload inspector in order; the first refusal wins.
func (s *ImageService) inspect(ctx context.Context, upload UploadInspection, image []byte) error {
	for _, inspector := range s.deps.Inspectors {
		err := inspector.InspectUpload(ctx, upload, image)
		switch {
		case err == nil:
		case errors.Is(err, ErrContentRejected):
			return ErrContentRejected
		case errors.Is(err, ErrReviewUnavailable):
			return ErrReviewUnavailable
		case errors.Is(err, ErrUploadLimitReached):
			return ErrUploadLimitReached
		default:
			return processingError(ctx, err)
		}
	}
	return nil
}

// transformDisplay runs each display transformer and accepts only a WebP with
// the same pixel size and frame count, so a plugin cannot swap the image.
// limit bounds each result; it is never below the upload itself.
func (s *ImageService) transformDisplay(ctx context.Context, limit int64, image DisplayImage, webp []byte) ([]byte, error) {
	before, err := s.deps.Imaging.Probe(ctx, webp)
	if err != nil {
		return nil, processingError(ctx, err)
	}
	image.Width, image.Height, image.Frames = before.Width, before.Height, before.LoadedFrames
	for _, transformer := range s.deps.Display {
		out, err := transformer.TransformDisplay(ctx, image, webp)
		if err != nil || len(out) == 0 || int64(len(out)) > limit {
			return nil, processingError(ctx, err)
		}
		after, err := s.deps.Imaging.Probe(ctx, out)
		if err != nil || after.Format != "webp" || after.Width != before.Width || after.Height != before.Height || after.LoadedFrames != before.LoadedFrames {
			return nil, processingError(ctx, err)
		}
		webp = out
	}
	return webp, nil
}

func makeUploadObjects(image model.Image, original, webp, thumbnail []byte) []uploadObject {
	objects := []uploadObject{}
	seen := map[string]bool{}
	add := func(key, mime string, data []byte, location string) {
		if len(data) == 0 || seen[location+":"+key] {
			return
		}
		seen[location+":"+key] = true
		digest := sha256.Sum256(data)
		objects = append(objects, uploadObject{receipt: model.ObjectReceipt{Key: key, OwnerID: image.Key, Location: location, MIME: mime, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:])}, data: data})
	}
	if image.HasOriginal {
		add(image.Path+"."+image.Ext, image.MIME, original, model.ObjectLocationCloud)
	}
	if image.HasWebP {
		add(image.Path+".webp", "image/webp", webp, model.ObjectLocationCloud)
	}
	if image.HasThumb {
		add(image.Path+"_thumbs.webp", "image/webp", thumbnail, model.ObjectLocationCloud)
		add(image.Path+"_thumbs.webp", "image/webp", thumbnail, model.ObjectLocationThumbCache)
	}
	return objects
}

// checkAdoptable requires the original to still be the foreign object
// version the import read, in a bucket that can keep it on delete.
func checkAdoptable(ctx context.Context, driver storage.Driver, pinned *pinnedPath, size int64) error {
	if _, legacy := driver.(storage.LegacyObjects); !legacy {
		return ErrInvalidInput
	}
	live, err := driver.Stat(ctx, pinned.path+"."+pinned.ext)
	if errors.Is(err, storage.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return storageError(ctx, err)
	}
	if live.OwnerID != "" || live.VersionID != pinned.adoptVersion || live.Size != size {
		return ErrPathConflict
	}
	return nil
}

// adoptOriginal marks the original as the object already in the bucket,
// pinned to the version that was read, so nothing writes over it.
func adoptOriginal(image model.Image, objects []uploadObject, version string) []uploadObject {
	for i := range objects {
		receipt := &objects[i].receipt
		if receipt.Location == model.ObjectLocationCloud && receipt.Key == image.Path+"."+image.Ext {
			receipt.Adopted, receipt.VersionID = true, version
		}
	}
	return objects
}

func (s *ImageService) failUpload(ctx context.Context, image model.Image, driver storage.Driver, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.deps.Images.StartCleanup(cleanupCtx, image.Key, image.OperationID); err != nil {
		return errors.Join(cause, fmt.Errorf("mark failed upload for recovery: %w", err))
	}
	if err := s.lockOperations(cleanupCtx); err != nil {
		return errors.Join(cause, fmt.Errorf("wait for upload cleanup: %w", err))
	}
	defer s.unlockOperations()
	current, err := s.deps.Images.FindByKey(cleanupCtx, image.Key)
	if errors.Is(err, ErrNotFound) {
		return cause
	}
	if err != nil {
		return errors.Join(cause, fmt.Errorf("read failed upload journal: %w", err))
	}
	if current.State != model.ImageStatePending || current.OperationID != image.OperationID || current.Operation != model.ImageOperationCleanup {
		return errors.Join(cause, ErrImageBusy)
	}
	if err := s.cleanupUpload(cleanupCtx, current, driver); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (s *ImageService) cleanupUpload(ctx context.Context, image model.Image, driver storage.Driver) error {
	var failures []error
	owned, ok := driver.(storage.OwnedPurger)
	if !ok {
		return ErrStorage
	}
	for _, receipt := range image.ObjectManifest {
		var err error
		if receipt.Adopted {
			// The adopted original was there before the upload and stays.
			continue
		}
		if receipt.Location == model.ObjectLocationThumbCache {
			err = s.deps.Cache.Delete(ctx, image.StorageID, receipt.Key)
		} else {
			err = owned.PurgeOwned(ctx, receipt.Key, image.Key)
			if errors.Is(err, storage.ErrOwnership) {
				err = nil
			}
		}
		if err != nil {
			failures = append(failures, storageError(ctx, err))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return fmt.Errorf("upload cleanup pending: %w", err)
	}
	if err := s.deps.Images.FinishCleanup(ctx, image.Key, image.OperationID); err != nil {
		return fmt.Errorf("finish upload cleanup: %w", err)
	}
	return nil
}

// maxOriginNameBytes bounds the stored original filename: multipart part
// headers carry no independent limit, so a single request could otherwise
// park megabytes in origin_name and its search-copy column.
const maxOriginNameBytes = 255

// clampOriginName truncates on a UTF-8 rune boundary so list responses and
// the search normalization never see a torn character.
func clampOriginName(name string) string {
	if len(name) <= maxOriginNameBytes {
		return name
	}
	name = name[:maxOriginNameBytes]
	for len(name) > 0 && !utf8.ValidString(name) {
		name = name[:len(name)-1]
	}
	return name
}

func allowedFormat(allowed []string, ext string) bool {
	ext = model.CanonicalExt(ext)
	for _, candidate := range allowed {
		candidate = model.CanonicalExt(strings.ToLower(strings.TrimPrefix(candidate, ".")))
		if candidate == ext {
			return true
		}
	}
	return len(allowed) == 0
}
func contentHashes(data []byte) (string, string) {
	md := md5.Sum(data)  // #nosec G401 -- public legacy content checksum, not a security primitive.
	sh := sha1.Sum(data) // #nosec G401 -- content checksum only.
	return hex.EncodeToString(md[:]), hex.EncodeToString(sh[:])
}
func operationID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate image operation: %w", err)
	}
	return hex.EncodeToString(random[:]), nil
}
func processingError(ctx context.Context, _ error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("process upload: %w", err)
	}
	return ErrProcessing
}

// processingFailure is processingError naming the step that failed; a pixel
// limit hit while decoding is reported as such.
func processingFailure(ctx context.Context, step ProcessingStep, err error) error {
	if canceled := ctx.Err(); canceled != nil {
		return fmt.Errorf("process upload: %w", canceled)
	}
	if errors.Is(err, imaging.ErrPixelLimit) {
		step = ProcessingPixels
	}
	return &ProcessingError{Step: step}
}
func storageError(ctx context.Context, err error) error {
	if canceled := ctx.Err(); canceled != nil {
		return fmt.Errorf("store image: %w", canceled)
	}
	if errors.Is(err, storage.ErrExists) {
		return ErrPathConflict
	}
	if err == nil || errors.Is(err, ErrStorage) {
		return ErrStorage
	}
	return storageFailure{cause: err}
}

func (s *ImageService) view(ctx context.Context, image model.Image) (ImageView, error) {
	backend, err := s.deps.Storages.Find(ctx, image.StorageID)
	if err != nil {
		return ImageView{}, fmt.Errorf("read image storage: %w", err)
	}
	policy, err := s.deps.Policies.Find(ctx, image.PolicyID)
	if err != nil {
		return ImageView{}, fmt.Errorf("read image policy: %w", err)
	}
	return imageView(image, backend, policy), nil
}

// viewer returns a view builder that reads each storage and rule once, so a
// listing page costs two lookups per distinct backend instead of per image.
func (s *ImageService) viewer() func(context.Context, model.Image) (ImageView, error) {
	backends := map[uint64]model.Storage{}
	policies := map[uint64]model.Policy{}
	return func(ctx context.Context, image model.Image) (ImageView, error) {
		backend, ok := backends[image.StorageID]
		if !ok {
			found, err := s.deps.Storages.Find(ctx, image.StorageID)
			if err != nil {
				return ImageView{}, fmt.Errorf("read image storage: %w", err)
			}
			backend, backends[image.StorageID] = found, found
		}
		policy, ok := policies[image.PolicyID]
		if !ok {
			found, err := s.deps.Policies.Find(ctx, image.PolicyID)
			if err != nil {
				return ImageView{}, fmt.Errorf("read image policy: %w", err)
			}
			policy, policies[image.PolicyID] = found, found
		}
		return imageView(image, backend, policy), nil
	}
}

func imageView(image model.Image, backend model.Storage, policy model.Policy) ImageView {
	links := ImageLinks{}
	if image.HasOriginal {
		links.Original = objectURL(backend.BaseURL, image.Path+"."+image.Ext)
	}
	if image.HasWebP {
		links.WebP = objectURL(backend.BaseURL, image.Path+".webp")
	}
	if image.HasThumb {
		links.Thumbnail = objectURL(backend.BaseURL, image.Path+"_thumbs.webp")
	}
	links.URL = links.Original
	if policy.LinkPrefer == "webp" && links.WebP != "" {
		links.URL = links.WebP
	}
	if links.URL == "" {
		links.URL = links.WebP
	}
	localThumb := ""
	if image.HasThumb {
		localThumb = "/t/" + image.Key + ".webp"
	}
	return ImageView{ID: image.ID, Key: image.Key, UserID: image.UserID, AlbumID: image.AlbumID, PolicyID: image.PolicyID, StorageID: image.StorageID, Name: image.OriginName, Path: image.Path, Ext: image.Ext, MIME: image.MIME, Size: image.Size, WebPSize: image.WebPSize, ChargedBytes: image.ChargedBytes, Width: image.Width, Height: image.Height, Frames: image.Frames, HasOriginal: image.HasOriginal, HasWebP: image.HasWebP, HasThumb: image.HasThumb, Scrubbed: image.Scrubbed, IsPublic: image.IsPublic, MD5: image.MD5, SHA1: image.SHA1, SrcMD5: image.SrcMD5, Links: links, Variants: variantViews(image.Variants, backend.BaseURL), LocalThumbURL: localThumb, DeletedAt: image.DeletedAt, PurgeAt: image.PurgeAt, CreatedAt: image.CreatedAt.UTC()}
}

func objectURL(base, key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.TrimRight(base, "/") + "/" + strings.Join(segments, "/")
}
