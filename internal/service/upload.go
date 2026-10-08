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
	return model.TokenGrant{UserID: subject.userID, ExpectedPasswordHash: subject.passwordHash, SourceTokenID: subject.sourceTokenID, At: s.deps.Now().UTC()}
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
	cap := s.deps.MaxFileBytes
	if group.MaxFileBytes > 0 && group.MaxFileBytes < cap {
		cap = group.MaxFileBytes
	}
	return UploadLimits{MaxFileBytes: cap, PerMinute: group.UploadPerMin}, nil
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
		return ImageView{}, processingError(ctx, err)
	}
	if !allowedFormat(group.AllowedExts, info.Ext) {
		return ImageView{}, ErrUnsupportedFormat
	}
	metadata, err := s.deps.Extractor.Extract(ctx, input.Data, info)
	if err != nil {
		return ImageView{}, processingError(ctx, err)
	}
	mode := policy.WebPMode
	heif := info.Format == "heic" || info.Format == "heif" || info.Format == "avif"
	if heif {
		switch policy.HEIFMode {
		case "webp_only":
			mode = "webp_only"
		case "keep":
			if policy.ScrubMode != "none" {
				return ImageView{}, ErrProcessing
			}
		case "reject":
			return ImageView{}, ErrUnsupportedFormat
		default:
			return ImageView{}, ErrInvalidInput
		}
	}
	original := input.Data
	scrubbed := false
	if mode != "webp_only" || info.Format == "webp" {
		original, err = s.deps.Scrubber.Scrub(ctx, input.Data, info.Format, policy.ScrubMode)
		if err != nil || len(original) == 0 {
			return ImageView{}, processingError(ctx, err)
		}
		check, probeErr := s.deps.Imaging.Probe(ctx, original)
		if probeErr != nil || check.Format != info.Format || check.Width != info.Width || check.Height != info.Height || check.LoadedFrames != info.LoadedFrames {
			return ImageView{}, processingError(ctx, probeErr)
		}
		scrubbed = policy.ScrubMode != "none"
	}
	result, err := s.deps.Imaging.Process(ctx, input.Data, imaging.Options{WebPMode: mode, Quality: policy.WebPQuality, Effort: policy.WebPEffort, Lossless: policy.WebPLossless, MaxWidth: policy.MaxWidth, MaxHeight: policy.MaxHeight, SkipIfLarger: policy.SkipIfLarger, ThumbEnabled: policy.ThumbEnabled, ThumbSize: policy.ThumbSize})
	if err != nil {
		return ImageView{}, processingError(ctx, err)
	}
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
	storedExt, storedMIME := info.Ext, info.MIME
	if !hasOriginal {
		primary = webp
		storedExt = "webp"
		storedMIME = "image/webp"
		info, err = s.deps.Imaging.Probe(ctx, primary)
		if err != nil || info.Format != "webp" {
			return ImageView{}, processingError(ctx, err)
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
	image := model.Image{UserID: plan.userID, AlbumID: input.AlbumID, PolicyID: policy.ID, StorageID: backend.ID, Key: imageKey, Ext: storedExt, OriginName: path.Base(strings.ReplaceAll(input.Filename, "\\", "/")), MIME: storedMIME, SrcMD5: srcMD5, MD5: storedMD5, SHA1: storedSHA1, IP: input.IP, State: model.ImageStatePending, Operation: model.ImageOperationUpload, OperationID: op, HasOriginal: hasOriginal, HasWebP: len(webp) > 0, HasThumb: len(result.Thumbnail) > 0, Scrubbed: scrubbed, IsPublic: input.IsPublic, Size: int64(len(primary)), WebPSize: int64(len(webp)), ThumbBytes: int64(len(result.Thumbnail)), Width: info.Width, Height: info.Height, Frames: info.LoadedFrames, CreatedAt: s.deps.Now().UTC()}
	driver, err := s.deps.Drivers.DriverFor(ctx, backend)
	if err != nil {
		return ImageView{}, storageError(ctx, err)
	}
	vars := pathtpl.Variables{Time: image.CreatedAt, UserID: image.UserID, Filename: input.Filename, MD5: srcMD5, SHA1: srcSHA1}
	rendered, err := s.deps.Paths.Build(ctx, policy.PathTpl, policy.NameTpl, vars)
	if err != nil {
		return ImageView{}, fmt.Errorf("render image path: %w", ErrInvalidInput)
	}
	var objects []uploadObject
	reserved := false
	basePath := rendered.Path
	for attempt := 0; attempt < 1000; attempt++ {
		if attempt > 0 {
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
		reservation, err = plan.reserve(ctx, model.UploadReservation{Image: image, Objects: manifests})
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
			return imageView(current, backend, policy), nil
		}
		if checkErr != nil {
			return ImageView{}, fmt.Errorf("confirm upload commit; recovery pending: %w", err)
		}
		return ImageView{}, s.failUpload(ctx, image, driver, fmt.Errorf("commit upload: %w", err))
	}
	s.invalidateRandom(ctx, committed.AlbumID)
	return imageView(committed, backend, policy), nil
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

func allowedFormat(allowed []string, ext string) bool {
	for _, candidate := range allowed {
		candidate = strings.ToLower(strings.TrimPrefix(candidate, "."))
		if candidate == ext || (ext == "jpg" && candidate == "jpeg") || (ext == "tiff" && candidate == "tif") {
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
func storageError(ctx context.Context, err error) error {
	if canceled := ctx.Err(); canceled != nil {
		return fmt.Errorf("store image: %w", canceled)
	}
	if errors.Is(err, storage.ErrExists) {
		return ErrPathConflict
	}
	return ErrStorage
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
	return ImageView{ID: image.ID, Key: image.Key, UserID: image.UserID, AlbumID: image.AlbumID, PolicyID: image.PolicyID, StorageID: image.StorageID, Name: image.OriginName, Path: image.Path, Ext: image.Ext, MIME: image.MIME, Size: image.Size, WebPSize: image.WebPSize, ChargedBytes: image.ChargedBytes, Width: image.Width, Height: image.Height, Frames: image.Frames, HasOriginal: image.HasOriginal, HasWebP: image.HasWebP, HasThumb: image.HasThumb, Scrubbed: image.Scrubbed, IsPublic: image.IsPublic, MD5: image.MD5, SHA1: image.SHA1, SrcMD5: image.SrcMD5, Links: links, LocalThumbURL: localThumb, DeletedAt: image.DeletedAt, PurgeAt: image.PurgeAt, CreatedAt: image.CreatedAt.UTC()}
}

func objectURL(base, key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.TrimRight(base, "/") + "/" + strings.Join(segments, "/")
}
