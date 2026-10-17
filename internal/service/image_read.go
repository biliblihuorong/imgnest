package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// Get returns an owner/admin image without EXIF or operation internals.
func (s *ImageService) Get(ctx context.Context, subject TokenSubject, id uint64) (ImageView, error) {
	image, err := s.imageByID(ctx, subject, id)
	if err != nil {
		return ImageView{}, err
	}
	return s.view(ctx, image)
}

func (s *ImageService) imageByID(ctx context.Context, subject TokenSubject, id uint64) (model.Image, error) {
	if id == 0 {
		return model.Image{}, ErrInvalidInput
	}
	actor, err := s.actor(ctx, subject)
	if err != nil {
		return model.Image{}, err
	}
	image, err := s.deps.Images.FindByID(ctx, id)
	if err != nil {
		return model.Image{}, fmt.Errorf("read image: %w", err)
	}
	if image.UserID != actor.ID && actor.Role != model.UserRoleAdmin {
		return model.Image{}, ErrForbidden
	}
	if image.State == model.ImageStatePending {
		return model.Image{}, ErrNotFound
	}
	return image, nil
}

// List paginates the actor's active images or trash, with explicit administrator scope.
// A positive album filter is verified to belong to the actor first; a foreign
// album rejects with ErrForbidden exactly like direct image access. The
// keyword/order/size/time/EXIF fields are optional narrowings mapped onto the
// repository filter after validation.
func (s *ImageService) List(ctx context.Context, subject TokenSubject, query ImageQuery) (ImagePage, error) {
	if query.QueryVersion != 0 {
		return s.listSearch(ctx, subject, query)
	}
	actor, err := s.actor(ctx, subject)
	if err != nil {
		return ImagePage{}, err
	}
	if query.Admin && actor.Role != model.UserRoleAdmin {
		return ImagePage{}, ErrForbidden
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Size == 0 {
		query.Size = 40
	}
	if query.Page < 1 || query.Page > 1000000 || query.Size < 1 || query.Size > 100 {
		return ImagePage{}, ErrInvalidInput
	}
	switch query.Order {
	case "", "newest", "oldest", "largest", "smallest":
	default:
		return ImagePage{}, ErrInvalidInput
	}
	if query.MinSize < 0 || query.MaxSize < 0 || (query.MinSize > 0 && query.MaxSize > 0 && query.MinSize > query.MaxSize) {
		return ImagePage{}, ErrInvalidInput
	}
	if query.From != nil && query.To != nil && query.From.After(*query.To) {
		return ImagePage{}, ErrInvalidInput
	}
	if len(query.Keyword) > 200 || len(query.Exif) > 200 || len(query.Q) > 200 {
		return ImagePage{}, ErrInvalidInput
	}
	if query.AlbumID != nil && *query.AlbumID > 0 && !query.Admin {
		albums, err := s.albumStore()
		if err != nil {
			return ImagePage{}, err
		}
		if _, err := albums.FindOwned(ctx, actor.ID, *query.AlbumID); err != nil {
			return ImagePage{}, fmt.Errorf("list album images: %w", err)
		}
	}
	filter := model.ImageListFilter{
		UserID:  actor.ID,
		Admin:   query.Admin,
		Trash:   query.Trash,
		AlbumID: query.AlbumID,
		Keyword: query.Keyword,
		Q:       query.Q,
		Order:   query.Order,
		MinSize: query.MinSize,
		MaxSize: query.MaxSize,
		From:    query.From,
		To:      query.To,
		Exif:    query.Exif,
	}
	images, total, err := s.deps.Images.List(ctx, filter, query.Page, query.Size)
	if err != nil {
		return ImagePage{}, fmt.Errorf("list images: %w", err)
	}
	views := make([]ImageView, 0, len(images))
	build := s.viewer()
	for _, image := range images {
		view, err := build(ctx, image)
		if err != nil {
			return ImagePage{}, err
		}
		views = append(views, view)
	}
	return ImagePage{Items: views, Total: total, Page: query.Page, Size: query.Size}, nil
}

// Exif returns complete original metadata only after owner/admin authorization.
func (s *ImageService) Exif(ctx context.Context, subject TokenSubject, id uint64) (model.ImageExif, error) {
	image, err := s.imageByID(ctx, subject, id)
	if err != nil {
		return model.ImageExif{}, err
	}
	value, err := s.deps.Images.FindExif(ctx, image.Key)
	if err != nil {
		return model.ImageExif{}, fmt.Errorf("read private image metadata: %w", err)
	}
	return value, nil
}

// SetPublic changes gallery/list visibility while direct URLs remain available.
func (s *ImageService) SetPublic(ctx context.Context, subject TokenSubject, id uint64, public bool) (ImageView, error) {
	image, err := s.imageByID(ctx, subject, id)
	if err != nil {
		return ImageView{}, err
	}
	if err = s.deps.Images.SetPublic(ctx, image.Key, public, s.grant(subject)); err != nil {
		return ImageView{}, fmt.Errorf("change image visibility: %w", err)
	}
	image.IsPublic = public
	return s.view(ctx, image)
}

// Rename changes an image's display name. Only the name shown in lists and
// searches changes; the object key, file extension and every URL stay as
// they are, so links already shared keep working.
func (s *ImageService) Rename(ctx context.Context, subject TokenSubject, id uint64, name string) (ImageView, error) {
	name, err := normalizeImageName(name)
	if err != nil {
		return ImageView{}, err
	}
	image, err := s.imageByID(ctx, subject, id)
	if err != nil {
		return ImageView{}, err
	}
	if image.State != model.ImageStateActive {
		return ImageView{}, ErrInvalidInput
	}
	if err = s.deps.Images.SetName(ctx, image.Key, name, s.grant(subject)); err != nil {
		return ImageView{}, fmt.Errorf("rename image: %w", err)
	}
	image.OriginName = name
	return s.view(ctx, image)
}

// normalizeImageName trims a requested display name and rejects empty names,
// control characters, path separators and names over the upload limit.
func normalizeImageName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > maxOriginNameBytes || !utf8.ValidString(name) ||
		containsControlRune(name) || strings.ContainsAny(name, "/\\") {
		return "", errImageNameInvalid
	}
	return name, nil
}

// OpenPublic serves only declared active local objects, including private direct URLs.
func (s *ImageService) OpenPublic(ctx context.Context, storageID uint64, key string) (PublicObject, error) {
	if storageID == 0 || key == "" || path.Clean(key) != key || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || strings.HasPrefix(key, "../") || strings.HasPrefix(key, "_trash/") || strings.HasPrefix(key, ".trash/") {
		return PublicObject{}, ErrNotFound
	}
	var (
		image   model.Image
		found   bool
		lastErr = ErrNotFound
	)
	for _, base := range directBases(key) {
		candidate, err := s.deps.Images.FindByPath(ctx, storageID, base)
		if err != nil {
			lastErr = err
			continue
		}
		if candidate.State != model.ImageStateActive {
			continue
		}
		if !found || declaresCloudKey(candidate, key) {
			image, found = candidate, true
		}
		if declaresCloudKey(candidate, key) {
			break
		}
	}
	if !found {
		if errors.Is(lastErr, ErrNotFound) {
			return PublicObject{}, ErrNotFound
		}
		return PublicObject{}, fmt.Errorf("find direct image: %w", lastErr)
	}
	driver, backend, err := s.imageDriver(ctx, image)
	if err != nil {
		return PublicObject{}, err
	}
	if backend.Driver != "local" {
		return PublicObject{}, ErrNotFound
	}
	if !declaresCloudKey(image, key) {
		if key == image.Path+".webp" && !image.HasWebP && image.HasOriginal {
			return PublicObject{Redirect: objectURL(backend.BaseURL, image.Path+"."+image.Ext)}, nil
		}
		return PublicObject{}, ErrNotFound
	}
	body, info, err := driver.Open(ctx, key)
	if err != nil {
		return PublicObject{}, ErrNotFound
	}
	if info.OwnerID != image.Key {
		_ = body.Close()
		return PublicObject{}, ErrNotFound
	}
	return PublicObject{Body: body, Size: info.Size, MIME: info.MIME}, nil
}

// Thumbnail returns a local preview; missing previews are lazily regenerated from owned cloud objects.
// Reads and regeneration run outside the lifecycle fence so one slow preview or
// a long sweep cannot stall every other preview; only the cache write re-checks
// the image under the fence, so purge can never be outlived by a stale write.
func (s *ImageService) Thumbnail(ctx context.Context, subject TokenSubject, key string) (PublicObject, error) {
	image, err := s.deps.Images.FindByKey(ctx, key)
	if err != nil {
		return PublicObject{}, fmt.Errorf("find preview: %w", err)
	}
	if image.State == model.ImageStatePending || !image.HasThumb || image.Operation == model.ImageOperationPurge {
		return PublicObject{}, ErrNotFound
	}
	if subject.userID == 0 {
		if !image.IsPublic || image.State != model.ImageStateActive {
			return PublicObject{}, ErrUnauthenticated
		}
	} else if _, err = s.ownedImage(ctx, subject, key); err != nil {
		return PublicObject{}, err
	}
	name := image.Path + "_thumbs.webp"
	body, err := s.deps.Cache.Open(ctx, image.StorageID, name)
	if err == nil {
		return boundedPreview(body, s.deps.MaxFileBytes)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return PublicObject{}, ErrStorage
	}
	driver, backend, err := s.imageDriver(ctx, image)
	if err != nil {
		return PublicObject{}, err
	}
	candidates := []string{name}
	if image.HasWebP {
		candidates = append(candidates, image.Path+".webp")
	}
	if image.HasOriginal {
		candidates = append(candidates, image.Path+"."+image.Ext)
	}
	for _, candidate := range candidates {
		cloudKey := candidate
		if image.State == model.ImageStateTrash {
			cloudKey = trashKey(backend, candidate)
		}
		cloud, info, openErr := driver.Open(ctx, cloudKey)
		if openErr != nil {
			// A missing candidate falls through to the next one, but a broken
			// backend must not masquerade as a missing image.
			if !errors.Is(openErr, storage.ErrNotFound) {
				return PublicObject{}, storageError(ctx, openErr)
			}
			continue
		}
		if !ownsObject(image, cloudKey, info) {
			_ = cloud.Close()
			continue
		}
		limit := objectReadLimit(s.deps.MaxFileBytes, image)
		data, readErr := io.ReadAll(io.LimitReader(cloud, limit+1))
		closeErr := cloud.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) > limit {
			return PublicObject{}, ErrStorage
		}
		if candidate != name {
			rule, ruleErr := s.deps.Policies.Find(ctx, image.PolicyID)
			if ruleErr != nil {
				return PublicObject{}, ruleErr
			}
			result, processErr := s.deps.Imaging.Process(ctx, data, imaging.Options{WebPMode: "none", Quality: 80, Effort: 4, ThumbEnabled: true, ThumbSize: rule.ThumbSize})
			if processErr != nil {
				return PublicObject{}, ErrProcessing
			}
			data = result.Thumbnail
		}
		s.cachePreview(ctx, image, name, data)
		return PublicObject{Body: io.NopCloser(bytes.NewReader(data)), Size: int64(len(data)), MIME: "image/webp"}, nil
	}
	return PublicObject{}, ErrNotFound
}

// cachePreview stores a regenerated preview only while the same image still
// owns the path. It never waits for the fence: when a lifecycle operation is
// running the preview is served uncached and rebuilt on a later request.
func (s *ImageService) cachePreview(ctx context.Context, image model.Image, name string, data []byte) {
	select {
	case s.operations <- struct{}{}:
	default:
		return
	}
	defer s.unlockOperations()
	current, err := s.deps.Images.FindByKey(ctx, image.Key)
	if err != nil || current.ID != image.ID || current.Path != image.Path || current.StorageID != image.StorageID ||
		current.State == model.ImageStatePending || current.Operation == model.ImageOperationPurge {
		return
	}
	_ = s.deps.Cache.Put(ctx, image.StorageID, name, data)
}

// objectReadLimit bounds reading one stored object of image back into
// memory: the default per-file limit, or the image's own recorded sizes when
// its group allowed a larger file.
func objectReadLimit(base int64, image model.Image) int64 {
	return max(base, image.Size, image.WebPSize, image.ThumbBytes)
}

func boundedPreview(body io.ReadCloser, limit int64) (PublicObject, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	closeErr := body.Close()
	if err != nil || closeErr != nil || int64(len(data)) > limit {
		return PublicObject{}, ErrStorage
	}
	return PublicObject{Body: io.NopCloser(bytes.NewReader(data)), Size: int64(len(data)), MIME: "image/webp"}, nil
}
