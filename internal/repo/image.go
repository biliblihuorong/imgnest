package repo

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/searchquery"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ImageRepository owns image reservations and transitions around storage IO.
type ImageRepository struct{ db *gorm.DB }

// NewImageRepository constructs the image persistence adapter.
func NewImageRepository(ctx context.Context, db *gorm.DB) (*ImageRepository, error) {
	if err := checkDatabase(ctx, db); err != nil {
		return nil, err
	}
	return &ImageRepository{db: db}, nil
}

// ReserveUpload claims a path and quota for an authenticated user's prepared objects.
func (r *ImageRepository) ReserveUpload(ctx context.Context, req model.UploadReservation) (model.Image, error) {
	if err := ctx.Err(); err != nil {
		return model.Image{}, fmt.Errorf("reserve upload: %w", err)
	}
	if req.Grant.UserID == 0 {
		return model.Image{}, fmt.Errorf("reserve upload: %w", model.ErrUnauthenticated)
	}
	if req.Image.UserID != req.Grant.UserID {
		return model.Image{}, fmt.Errorf("reserve upload: %w", model.ErrForbidden)
	}
	image, err := preparedImage(req)
	if err != nil {
		return model.Image{}, imageError("prepare reservation", err)
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockUser(ctx, tx, image.UserID)
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound) {
			return model.ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if err := validateGrant(ctx, tx, user, req.Grant); err != nil {
			return err
		}
		policy, backend, group, err := permittedPolicy(tx, user, image.PolicyID)
		if err != nil {
			return err
		}
		if backend.ID != image.StorageID {
			return model.ErrForbidden
		}
		image.PolicyID = policy.ID
		if err := checkUploadGroup(image, req.SourceExt, group); err != nil {
			return err
		}
		if image.AlbumID != 0 {
			if err := checkRecordID(ctx, image.AlbumID); err != nil {
				return err
			}
			var album model.Album
			if err := tx.First(&album, "id = ?", image.AlbumID).Error; err != nil {
				return err
			}
			if album.UserID != user.ID {
				return model.ErrForbidden
			}
		}
		var existing model.Image
		lookup := tx.First(&existing, "key = ?", image.Key).Error
		if lookup == nil {
			same := existing.UserID == image.UserID && existing.OperationID == image.OperationID &&
				existing.StorageID == image.StorageID && existing.PolicyID == image.PolicyID && existing.Path == image.Path &&
				existing.SrcMD5 == image.SrcMD5 && existing.ChargedBytes == image.ChargedBytes
			resumable := existing.State == model.ImageStateActive ||
				(existing.State == model.ImageStatePending && existing.Operation == model.ImageOperationUpload)
			if !same || !resumable {
				return model.ErrImageBusy
			}
			image = existing
			return nil
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		if err := checkQuota(tx, user, image.ChargedBytes); err != nil {
			return err
		}
		filenameSearch := searchquery.Normalize(image.OriginName)
		image.FilenameSearch = &filenameSearch
		if err := tx.Create(&image).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return model.ErrPathConflict
			}
			return err
		}
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("reserve upload", err)
	}
	return image, nil
}

// RecordObjectReceipt records a storage result within the matching upload operation.
func (r *ImageRepository) RecordObjectReceipt(ctx context.Context, key, op string, receipt model.ObjectReceipt) error {
	err := r.withImage(ctx, key, nil, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if op == "" || image.OperationID != op {
			return model.ErrImageBusy
		}
		if receipt.OwnerID != image.Key {
			return model.ErrForbidden
		}
		for index, current := range image.ObjectManifest {
			if receipt.Key != current.Key || receipt.Location != current.Location {
				continue
			}
			if receipt.Size != current.Size {
				return model.ErrInvalidInput
			}
			if receipt.MIME == "" {
				receipt.MIME = current.MIME
			}
			if receipt.SHA256 == "" {
				receipt.SHA256 = current.SHA256
			}
			digest, err := objectDigest(receipt.SHA256)
			if err != nil {
				return err
			}
			receipt.SHA256 = digest
			if current.SHA256 != "" && current.SHA256 != receipt.SHA256 {
				return model.ErrInvalidInput
			}
			if image.State == model.ImageStateActive {
				if image.Operation == "" && receipt == current {
					return nil
				}
				return model.ErrImageBusy
			}
			allowed := image.Operation == model.ImageOperationUpload || image.Operation == model.ImageOperationCleanup
			if image.State != model.ImageStatePending || !allowed {
				return model.ErrImageBusy
			}
			image.ObjectManifest[index] = receipt
			return saveManifest(tx, *image)
		}
		return model.ErrInvalidInput
	})
	return finishImageError("record object receipt", err)
}

// CommitUpload activates the reserved image and commits metadata and quota together.
func (r *ImageRepository) CommitUpload(ctx context.Context, key, op string, exif model.ImageExif, grant model.TokenGrant) (model.Image, error) {
	var result model.Image
	err := r.withImage(ctx, key, &grant, func(tx *gorm.DB, image *model.Image, owner model.User) error {
		if image.UserID != grant.UserID {
			return model.ErrForbidden
		}
		if op == "" || image.OperationID != op {
			return model.ErrImageBusy
		}
		if image.State == model.ImageStateActive && image.Operation == "" {
			result = *image
			return nil
		}
		if image.State != model.ImageStatePending || image.Operation != model.ImageOperationUpload {
			return model.ErrImageBusy
		}
		_, backend, group, err := permittedPolicy(tx, owner, image.PolicyID)
		if err != nil {
			return err
		}
		if backend.ID != image.StorageID {
			return model.ErrForbidden
		}
		if err := checkUploadGroup(*image, "", group); err != nil {
			return err
		}
		if err := checkQuota(tx, owner, 0); err != nil {
			return err
		}
		if len(exif.Raw) == 0 {
			exif.Raw = json.RawMessage("{}")
		}
		if !json.Valid(exif.Raw) {
			return model.ErrInvalidInput
		}
		exif.ImageID = image.ID
		camera := searchquery.Normalize(strings.TrimSpace(exif.Make + " " + exif.Model))
		lens := searchquery.Normalize(exif.Lens)
		exif.CameraSearch, exif.LensSearch = &camera, &lens
		if err := tx.Create(&exif).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", owner.ID).
			Update("used_bytes", gorm.Expr("used_bytes + ?", image.ChargedBytes)).Error; err != nil {
			return err
		}
		if err := adjustAlbum(tx, *image, 1); err != nil {
			return err
		}
		if err := updateImage(tx, image.ID, map[string]any{"state": model.ImageStateActive, "operation": ""}); err != nil {
			return err
		}
		image.State = model.ImageStateActive
		image.Operation = ""
		result = *image
		return nil
	})
	if err != nil {
		return model.Image{}, imageError("commit upload", err)
	}
	return result, nil
}

// StartCleanup marks an uncommitted upload for durable compensation.
func (r *ImageRepository) StartCleanup(ctx context.Context, key, op string) error {
	err := r.withImage(ctx, key, nil, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if op == "" || image.OperationID != op || image.State != model.ImageStatePending {
			return model.ErrImageBusy
		}
		if image.Operation == model.ImageOperationCleanup {
			return nil
		}
		if image.Operation != model.ImageOperationUpload {
			return model.ErrImageBusy
		}
		return updateImage(tx, image.ID, map[string]any{"operation": model.ImageOperationCleanup})
	})
	return finishImageError("start cleanup", err)
}

// FinishCleanup removes a pending row only after its owned objects have been removed.
func (r *ImageRepository) FinishCleanup(ctx context.Context, key, op string) error {
	err := r.withImage(ctx, key, nil, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if op == "" || image.OperationID != op || image.State != model.ImageStatePending ||
			image.Operation != model.ImageOperationCleanup {
			return model.ErrImageBusy
		}
		return deleteImage(tx, *image)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound) {
		return nil
	}
	return finishImageError("finish cleanup", err)
}

// FindByKey includes reserved and trashed images required by trusted recovery.
func (r *ImageRepository) FindByKey(ctx context.Context, key string) (model.Image, error) {
	var image model.Image
	if err := r.db.WithContext(ctx).First(&image, "key = ?", key).Error; err != nil {
		return model.Image{}, repositoryError("find image", err)
	}
	return image, nil
}

// FindByID resolves an image for the native numeric-ID routes.
func (r *ImageRepository) FindByID(ctx context.Context, id uint64) (model.Image, error) {
	if err := checkRecordID(ctx, id); err != nil {
		return model.Image{}, fmt.Errorf("find image by ID: %w", err)
	}
	var image model.Image
	if err := r.db.WithContext(ctx).First(&image, "id = ?", id).Error; err != nil {
		return model.Image{}, repositoryError("find image by ID", err)
	}
	return image, nil
}

// SetPublic changes display visibility after an owner/admin proof check.
func (r *ImageRepository) SetPublic(ctx context.Context, key string, public bool, grant model.TokenGrant) error {
	err := r.withImage(ctx, key, &grant, func(tx *gorm.DB, image *model.Image, _ model.User) error {
		if image.State == model.ImageStatePending {
			return model.ErrImageBusy
		}
		return updateImage(tx, image.ID, map[string]any{"is_public": public})
	})
	return finishImageError("set image visibility", err)
}

// FindByPath returns the image holding a backend path across all states.
func (r *ImageRepository) FindByPath(ctx context.Context, storageID uint64, imagePath string) (model.Image, error) {
	if err := checkRecordID(ctx, storageID); err != nil {
		return model.Image{}, fmt.Errorf("find image path: %w", err)
	}
	var image model.Image
	if err := r.db.WithContext(ctx).First(&image, "storage_id = ? AND path = ?", storageID, imagePath).Error; err != nil {
		return model.Image{}, repositoryError("find image path", err)
	}
	return image, nil
}

// FindExif reads protected metadata after the service authorizes the image.
func (r *ImageRepository) FindExif(ctx context.Context, key string) (model.ImageExif, error) {
	image, err := r.FindByKey(ctx, key)
	if err != nil {
		return model.ImageExif{}, err
	}
	var exif model.ImageExif
	if err := r.db.WithContext(ctx).First(&exif, "image_id = ?", image.ID).Error; err != nil {
		return model.ImageExif{}, repositoryError("find image metadata", err)
	}
	return exif, nil
}

// List pages active or trashed records and excludes pending uploads. The
// album filter distinguishes absence (no filter), zero (unassigned, meaning
// a NULL or zero album reference) and a positive album ID. The remaining
// filter fields mirror the native query surface: keyword on file name,
// explicit ordering, byte-size bounds, an upload-time window and an EXIF
// make/model/lens match.
func (r *ImageRepository) List(ctx context.Context, filter model.ImageListFilter, page, size int) ([]model.Image, int64, error) {
	if filter.Search != nil {
		return r.listSearch(ctx, filter, page, size)
	}
	if page < 1 || size < 1 || size > 200 {
		return nil, 0, fmt.Errorf("list images: %w", model.ErrInvalidInput)
	}
	if page-1 > math.MaxInt/size {
		return nil, 0, fmt.Errorf("list images: %w", model.ErrInvalidInput)
	}
	if filter.MinSize < 0 || filter.MaxSize < 0 || (filter.MinSize > 0 && filter.MaxSize > 0 && filter.MinSize > filter.MaxSize) {
		return nil, 0, fmt.Errorf("list images: %w", model.ErrInvalidInput)
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return nil, 0, fmt.Errorf("list images: %w", model.ErrInvalidInput)
	}
	state := model.ImageStateActive
	if filter.Trash {
		state = model.ImageStateTrash
	}
	query := r.db.WithContext(ctx).Model(&model.Image{}).Where("state = ?", state)
	if !filter.Admin {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.AlbumID != nil {
		if *filter.AlbumID == 0 {
			query = query.Where("album_id IS NULL OR album_id = 0")
		} else {
			query = query.Where("album_id = ?", *filter.AlbumID)
		}
	}
	if filter.Keyword != "" {
		pattern := "%" + escapeLike(filter.Keyword) + "%"
		// LOWER on both sides keeps the match case-insensitive on PostgreSQL,
		// where LIKE is case-sensitive unlike SQLite.
		query = query.Where(
			"(LOWER(origin_name) LIKE LOWER(?) ESCAPE '\\' OR LOWER(path || '.' || ext) LIKE LOWER(?) ESCAPE '\\')",
			pattern, pattern,
		)
	}
	if filter.Q != "" {
		// Unified search: one needle across file names and camera metadata (OR).
		pattern := "%" + escapeLike(filter.Q) + "%"
		query = query.Where(
			"(LOWER(origin_name) LIKE LOWER(?) ESCAPE '\\' OR LOWER(path || '.' || ext) LIKE LOWER(?) ESCAPE '\\'"+
				" OR EXISTS (SELECT 1 FROM image_exif WHERE image_exif.image_id = images.id"+
				" AND (LOWER(image_exif.make) LIKE LOWER(?) ESCAPE '\\'"+
				" OR LOWER(image_exif.model) LIKE LOWER(?) ESCAPE '\\'"+
				" OR LOWER(image_exif.lens) LIKE LOWER(?) ESCAPE '\\')))",
			pattern, pattern, pattern, pattern, pattern,
		)
	}
	if filter.MinSize > 0 {
		query = query.Where("size >= ?", filter.MinSize)
	}
	if filter.MaxSize > 0 {
		query = query.Where("size <= ?", filter.MaxSize)
	}
	if filter.From != nil {
		query = query.Where("created_at >= ?", filter.From.UTC())
	}
	if filter.To != nil {
		query = query.Where("created_at <= ?", filter.To.UTC())
	}
	if filter.Exif != "" {
		pattern := "%" + escapeLike(filter.Exif) + "%"
		query = query.Where(
			"EXISTS (SELECT 1 FROM image_exif WHERE image_exif.image_id = images.id"+
				" AND (LOWER(image_exif.make) LIKE LOWER(?) ESCAPE '\\'"+
				" OR LOWER(image_exif.model) LIKE LOWER(?) ESCAPE '\\'"+
				" OR LOWER(image_exif.lens) LIKE LOWER(?) ESCAPE '\\'))",
			pattern, pattern, pattern,
		)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, repositoryError("count images", err)
	}
	images := []model.Image{}
	if err := query.Order(imageListOrder(filter.Order)).Limit(size).Offset((page - 1) * size).Find(&images).Error; err != nil {
		return nil, 0, repositoryError("list images", err)
	}
	return images, total, nil
}

// imageListOrder maps the native listing order names; the default is newest.
func imageListOrder(order string) string {
	switch order {
	case "oldest":
		return "created_at ASC, id ASC"
	case "largest":
		return "size DESC, id DESC"
	case "smallest":
		return "size ASC, id ASC"
	default:
		return "created_at DESC, id DESC"
	}
}

// PendingOperations lists unfinished work for the trusted single-instance recovery loop.
func (r *ImageRepository) PendingOperations(ctx context.Context) ([]model.Image, error) {
	images := []model.Image{}
	if err := r.db.WithContext(ctx).Where("operation <> ?", "").Order("id ASC").Find(&images).Error; err != nil {
		return nil, repositoryError("find pending image operations", err)
	}
	return images, nil
}

// DueTrash lists idle trash at or past its physical cleanup deadline.
func (r *ImageRepository) DueTrash(ctx context.Context, now time.Time, limit int) ([]model.Image, error) {
	if limit < 1 {
		return nil, fmt.Errorf("find due trash: %w", model.ErrInvalidInput)
	}
	images := []model.Image{}
	err := r.db.WithContext(ctx).Where("state = ? AND operation = ? AND purge_at <= ?", model.ImageStateTrash, "", now.UTC()).
		Order("purge_at ASC").Order("id ASC").Limit(limit).Find(&images).Error
	if err != nil {
		return nil, repositoryError("find due trash", err)
	}
	return images, nil
}

func (r *ImageRepository) withImage(ctx context.Context, key string, grant *model.TokenGrant, apply func(*gorm.DB, *model.Image, model.User) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if grant != nil && grant.UserID == 0 {
		return model.ErrUnauthenticated
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var snapshot model.Image
		if err := tx.First(&snapshot, "key = ?", key).Error; err != nil {
			return err
		}
		actorID := snapshot.UserID
		if grant != nil {
			actorID = grant.UserID
		}
		ids := []uint64{snapshot.UserID}
		if actorID != snapshot.UserID {
			ids = append(ids, actorID)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var actor, owner model.User
		for _, id := range ids {
			user, err := lockUser(ctx, tx, id)
			if err != nil {
				if grant != nil && id == actorID && (errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, model.ErrNotFound)) {
					return model.ErrUnauthenticated
				}
				return err
			}
			if id == actorID {
				actor = user
			}
			if id == snapshot.UserID {
				owner = user
			}
		}
		if grant != nil {
			if err := validateGrant(ctx, tx, actor, *grant); err != nil {
				return err
			}
			if actor.ID != owner.ID && actor.Role != model.UserRoleAdmin {
				return model.ErrForbidden
			}
		}
		query := tx
		if tx.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var image model.Image
		if err := query.First(&image, "id = ?", snapshot.ID).Error; err != nil {
			return err
		}
		if image.UserID != owner.ID {
			return model.ErrImageBusy
		}
		return apply(tx, &image, owner)
	})
}

func preparedImage(req model.UploadReservation) (model.Image, error) {
	image := req.Image
	// A WebP-only image must name its source format so the group's allowed
	// formats can be checked against what was actually uploaded.
	invalidIdentity := image.Key == "" || image.OperationID == "" || image.StorageID == 0 ||
		(!image.HasOriginal && req.SourceExt == "")
	invalidSize := image.Size < 0 || image.WebPSize < 0 || image.ThumbBytes < 0
	invalidDimensions := image.Width < 1 || image.Height < 1 || image.Frames < 1
	if invalidIdentity || invalidSize || invalidDimensions {
		return model.Image{}, model.ErrInvalidInput
	}
	if !validImagePath(image.Path) {
		return model.Image{}, model.ErrInvalidInput
	}
	allowedCloud := map[string]bool{}
	if image.HasOriginal {
		allowedCloud[image.Path+"."+image.Ext] = true
	}
	if image.HasWebP {
		allowedCloud[image.Path+".webp"] = true
	}
	if image.HasThumb {
		allowedCloud[image.Path+"_thumbs.webp"] = true
	}
	if len(allowedCloud) == 0 {
		return model.Image{}, model.ErrInvalidInput
	}
	cacheKey := image.Path + "_thumbs.webp"
	objects := make([]model.ObjectReceipt, 0, len(req.Objects))
	seen := map[string]model.ObjectReceipt{}
	var charged int64
	for _, object := range req.Objects {
		digest, err := objectDigest(object.SHA256)
		if err != nil {
			return model.Image{}, err
		}
		object.SHA256 = digest
		if object.OwnerID == "" {
			object.OwnerID = image.Key
		}
		if object.OwnerID != image.Key {
			return model.Image{}, model.ErrForbidden
		}
		if object.Size < 0 {
			return model.Image{}, model.ErrInvalidInput
		}
		switch object.Location {
		case model.ObjectLocationCloud:
			if !allowedCloud[object.Key] {
				return model.Image{}, model.ErrInvalidInput
			}
		case model.ObjectLocationThumbCache:
			if !image.HasThumb || object.Key != cacheKey {
				return model.Image{}, model.ErrInvalidInput
			}
		default:
			return model.Image{}, model.ErrInvalidInput
		}
		identity := object.Location + "\x00" + object.Key
		if prior, exists := seen[identity]; exists {
			if prior.Size != object.Size || prior.OwnerID != object.OwnerID || prior.MIME != object.MIME || prior.SHA256 != object.SHA256 {
				return model.Image{}, model.ErrInvalidInput
			}
			continue
		}
		if object.Location == model.ObjectLocationCloud {
			if object.Size > math.MaxInt64-charged {
				return model.Image{}, model.ErrQuotaExceeded
			}
			charged += object.Size
		}
		object.VersionID = ""
		seen[identity] = object
		objects = append(objects, object)
	}
	for key := range allowedCloud {
		if _, exists := seen[model.ObjectLocationCloud+"\x00"+key]; !exists {
			return model.Image{}, model.ErrInvalidInput
		}
	}
	if image.HasThumb {
		if _, exists := seen[model.ObjectLocationThumbCache+"\x00"+cacheKey]; !exists {
			return model.Image{}, model.ErrInvalidInput
		}
	}
	image.ID = 0
	image.State = model.ImageStatePending
	image.Operation = model.ImageOperationUpload
	image.ChargedBytes = charged
	image.ObjectManifest = objects
	image.DeletedAt = nil
	image.PurgeAt = nil
	return image, nil
}

func validImagePath(value string) bool {
	if value == "" || len(value) > 255 || !utf8.ValidString(value) {
		return false
	}
	segments := strings.Split(value, "/")
	if segments[0] == "_trash" || segments[0] == ".trash" || strings.HasSuffix(path.Base(value), "_thumbs") {
		return false
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || utf8.RuneCountInString(segment) > 100 {
			return false
		}
		for _, char := range segment {
			if unicode.IsControl(char) || unicode.IsSpace(char) || strings.ContainsRune("?#%&\\:*\"<>|", char) {
				return false
			}
		}
	}
	return true
}

func objectDigest(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) != 64 {
		return "", model.ErrInvalidInput
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", model.ErrInvalidInput
	}
	return strings.ToLower(value), nil
}

// checkUploadGroup applies the group's format and size limits. Allowed formats
// describe what users may upload, so they are checked against the source
// format: the stored extension when the original is kept, otherwise sourceExt.
// A WebP-only image re-checked at commit has no source format on record; the
// reservation already checked it.
func checkUploadGroup(image model.Image, sourceExt string, group model.Group) error {
	supported := []string{"jpg", "png", "gif", "webp", "bmp", "tif", "tiff", "heic", "avif"}
	valid := false
	for _, ext := range supported {
		if image.Ext == ext {
			valid = true
			break
		}
	}
	if !valid {
		return model.ErrUnsupportedFormat
	}
	uploaded := image.Ext
	if !image.HasOriginal {
		uploaded = sourceExt
	}
	if len(group.AllowedExts) > 0 && uploaded != "" {
		allowed := false
		canonical := model.CanonicalExt(uploaded)
		for _, ext := range group.AllowedExts {
			if model.CanonicalExt(strings.ToLower(ext)) == canonical {
				allowed = true
				break
			}
		}
		if !allowed {
			return model.ErrUnsupportedFormat
		}
	}
	if group.MaxFileBytes > 0 && image.Size > group.MaxFileBytes {
		return model.ErrInvalidInput
	}
	return nil
}

func checkQuota(tx *gorm.DB, user model.User, additional int64) error {
	var reserved int64
	err := tx.Model(&model.Image{}).
		Where("user_id = ? AND (state = ? OR (state = ? AND operation = ?))", user.ID, model.ImageStatePending, model.ImageStateTrash, model.ImageOperationRestore).
		Select("COALESCE(SUM(charged_bytes),0)").Scan(&reserved).Error
	if err != nil {
		return err
	}
	if additional < 0 || reserved < 0 || user.UsedBytes < 0 {
		return model.ErrInvalidInput
	}
	if reserved > math.MaxInt64-user.UsedBytes {
		return model.ErrQuotaExceeded
	}
	total := user.UsedBytes + reserved
	if additional > math.MaxInt64-total {
		return model.ErrQuotaExceeded
	}
	var group model.Group
	if err := tx.First(&group, "id = ?", user.GroupID).Error; err != nil {
		return err
	}
	if group.CapacityBytes > 0 && (total > group.CapacityBytes || additional > group.CapacityBytes-total) {
		return model.ErrQuotaExceeded
	}
	return nil
}

func updateImage(tx *gorm.DB, id uint64, values map[string]any) error {
	values["updated_at"] = time.Now().UTC()
	return tx.Table("images").Where("id = ?", id).Updates(values).Error
}

func saveManifest(tx *gorm.DB, image model.Image) error {
	encoded, err := json.Marshal(image.ObjectManifest)
	if err != nil {
		return err
	}
	return updateImage(tx, image.ID, map[string]any{"object_manifest": string(encoded)})
}

func adjustAlbum(tx *gorm.DB, image model.Image, delta int64) error {
	if image.AlbumID == 0 {
		return nil
	}
	if err := lockImageAlbums(tx, image.UserID, []uint64{image.AlbumID}); err != nil {
		return err
	}
	return refreshAlbumCount(tx, image.AlbumID, delta)
}

func deleteImage(tx *gorm.DB, image model.Image) error {
	if err := tx.Where("image_id = ?", image.ID).Delete(&model.ImageExif{}).Error; err != nil {
		return err
	}
	return tx.Where("id = ?", image.ID).Delete(&model.Image{}).Error
}

func finishImageError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return imageError(operation, err)
}

func imageError(operation string, err error) error {
	for _, sentinel := range []error{
		model.ErrInvalidInput, model.ErrNotFound, model.ErrForbidden, model.ErrUnauthenticated,
		model.ErrQuotaExceeded, model.ErrPathConflict, model.ErrImageBusy, model.ErrUnsupportedFormat,
	} {
		if errors.Is(err, sentinel) {
			return fmt.Errorf("%s: %w", operation, sentinel)
		}
	}
	return repositoryError(operation, err)
}

// ListAdminPage pages every active image site-wide with optional owner and
// keyword filters; the keyword matches the original filename or the stored
// pathname exactly like the owner listing.
func (r *ImageRepository) ListAdminPage(ctx context.Context, userID uint64, keyword string, page, size int) ([]model.Image, int64, error) {
	if page < 1 || size < 1 || size > 200 || page-1 > math.MaxInt/size {
		return nil, 0, fmt.Errorf("list admin images: %w", model.ErrInvalidInput)
	}
	query := r.db.WithContext(ctx).Model(&model.Image{}).Where("state = ?", model.ImageStateActive)
	if userID != 0 {
		query = query.Where("user_id = ?", userID)
	}
	if strings.TrimSpace(keyword) != "" {
		pattern := "%" + escapeLike(strings.TrimSpace(keyword)) + "%"
		query = query.Where(
			"(LOWER(origin_name) LIKE LOWER(?) ESCAPE '\\' OR LOWER(path || '.' || ext) LIKE LOWER(?) ESCAPE '\\')",
			pattern, pattern,
		)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, repositoryError("count admin images", err)
	}
	images := []model.Image{}
	if err := query.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&images).Error; err != nil {
		return nil, 0, repositoryError("list admin images", err)
	}
	return images, total, nil
}

// TrashKeys lists every recycled image key in stable order for site-wide
// physical purges.
func (r *ImageRepository) TrashKeys(ctx context.Context) ([]string, error) {
	keys := []string{}
	err := r.db.WithContext(ctx).Model(&model.Image{}).
		Where("state = ?", model.ImageStateTrash).
		Order("id ASC").
		Pluck("key", &keys).Error
	if err != nil {
		return nil, repositoryError("list trash keys", err)
	}
	return keys, nil
}
