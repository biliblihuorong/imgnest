package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

func (s *ImageService) lockOperations(ctx context.Context) error {
	// ponytail: one lifecycle writer per monolith; use per-image gates if bulk IO throughput demands it.
	select {
	case s.operations <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *ImageService) unlockOperations() { <-s.operations }

func (s *ImageService) ownedImage(ctx context.Context, subject TokenSubject, key string) (model.Image, error) {
	return s.actedImage(ctx, subject, key, true)
}

// strictlyOwnedImage resolves an image without the admin cross-owner bypass.
// The Lsky-compatible v1 tokens are owner-scoped, unlike the native console.
func (s *ImageService) strictlyOwnedImage(ctx context.Context, subject TokenSubject, key string) (model.Image, error) {
	return s.actedImage(ctx, subject, key, false)
}

func (s *ImageService) actedImage(ctx context.Context, subject TokenSubject, key string, allowAdmin bool) (model.Image, error) {
	actor, err := s.actor(ctx, subject)
	if err != nil {
		return model.Image{}, err
	}
	image, err := s.deps.Images.FindByKey(ctx, key)
	if err != nil {
		return model.Image{}, fmt.Errorf("read image: %w", err)
	}
	if image.UserID != actor.ID && (!allowAdmin || actor.Role != model.UserRoleAdmin) {
		return model.Image{}, ErrForbidden
	}
	if image.State == model.ImageStatePending {
		return model.Image{}, ErrNotFound
	}
	return image, nil
}

func (s *ImageService) imageDriver(ctx context.Context, image model.Image) (storage.Driver, model.Storage, error) {
	backend, err := s.deps.Storages.Find(ctx, image.StorageID)
	if err != nil {
		return nil, backend, fmt.Errorf("read image backend: %w", err)
	}
	driver, err := s.deps.Drivers.DriverFor(ctx, backend)
	if err != nil {
		return nil, backend, storageError(ctx, err)
	}
	return driver, backend, nil
}

func trashKey(backend model.Storage, key string) string {
	if backend.Driver == "local" {
		return ".trash/" + key
	}
	return "_trash/" + key
}

// Cache headers for live objects and recycle-bin copies. Trash copies must not
// be cached by browsers or CDNs even when the bucket itself is publicly readable.
const (
	liveCacheControl  = "public, max-age=31536000, immutable"
	trashCacheControl = "private, no-store"
)

func ensureCopy(ctx context.Context, driver storage.Driver, source, target string, receipt model.ObjectReceipt, cacheControl string) error {
	origin, err := driver.Stat(ctx, source)
	if errors.Is(err, storage.ErrNotFound) {
		existing, err := driver.Stat(ctx, target)
		if err != nil {
			return err
		}
		if existing.OwnerID != receipt.OwnerID || existing.Size != receipt.Size {
			return storage.ErrOwnership
		}
		if receipt.SHA256 != "" {
			return verifyObjectDigest(ctx, driver, target, receipt)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if origin.OwnerID != receipt.OwnerID || origin.Size != receipt.Size {
		return storage.ErrOwnership
	}
	_, err = driver.Copy(ctx, source, target, storage.CopyOptions{OwnerID: receipt.OwnerID, MIME: receipt.MIME, CacheControl: cacheControl})
	return err
}

func verifyObjectDigest(ctx context.Context, driver storage.Driver, key string, receipt model.ObjectReceipt) error {
	body, info, err := driver.Open(ctx, key)
	if err != nil {
		return err
	}
	digest := sha256.New()
	size, copyErr := io.Copy(digest, io.LimitReader(body, receipt.Size+1))
	closeErr := body.Close()
	if copyErr != nil || closeErr != nil {
		return ErrStorage
	}
	if info.OwnerID != receipt.OwnerID || size != receipt.Size || hex.EncodeToString(digest.Sum(nil)) != receipt.SHA256 {
		return storage.ErrOwnership
	}
	return nil
}

// Trash moves each live object into the recycle bin before returning success.
func (s *ImageService) Trash(ctx context.Context, subject TokenSubject, key string) error {
	return s.trash(ctx, subject, key, true)
}

// TrashOwned moves one image into the recycle bin under strict ownership:
// administrators acting through a v1 token cannot recycle another owner's
// image, matching the owner scope of Lsky Pro tokens.
func (s *ImageService) TrashOwned(ctx context.Context, subject TokenSubject, key string) error {
	return s.trash(ctx, subject, key, false)
}

func (s *ImageService) trash(ctx context.Context, subject TokenSubject, key string, allowAdmin bool) error {
	if err := s.lockOperations(ctx); err != nil {
		return err
	}
	defer s.unlockOperations()
	var image model.Image
	var err error
	if allowAdmin {
		image, err = s.ownedImage(ctx, subject, key)
	} else {
		image, err = s.strictlyOwnedImage(ctx, subject, key)
	}
	if err != nil {
		return err
	}
	days, err := s.deps.Settings.TrashDays(ctx)
	if err != nil {
		return fmt.Errorf("read trash retention: %w", err)
	}
	if image.State == model.ImageStateTrash && image.Operation == "" {
		return nil
	}
	if image.Operation != model.ImageOperationTrash {
		op, err := operationID()
		if err != nil {
			return err
		}
		image, err = s.deps.Images.BeginTrash(ctx, key, op, s.grant(subject), days)
		if err != nil {
			return fmt.Errorf("begin recycle image: %w", err)
		}
		// The row left the active set here, whatever happens to its objects.
		s.invalidateRandom(ctx, image.AlbumID)
	}
	if err = s.moveToTrash(ctx, image); err != nil {
		return err
	}
	if days == 0 {
		op, err := operationID()
		if err != nil {
			return err
		}
		image, err = s.deps.Images.BeginSystemPurge(ctx, key, op)
		if err != nil {
			return err
		}
		return s.purgeObjects(ctx, image)
	}
	return nil
}

func (s *ImageService) moveToTrash(ctx context.Context, image model.Image) error {
	driver, backend, err := s.imageDriver(ctx, image)
	if err != nil {
		return err
	}
	for _, receipt := range image.ObjectManifest {
		if receipt.Location != model.ObjectLocationCloud {
			continue
		}
		if err = ensureCopy(ctx, driver, receipt.Key, trashKey(backend, receipt.Key), receipt, trashCacheControl); err != nil {
			return storageError(ctx, err)
		}
		live, statErr := driver.Stat(ctx, receipt.Key)
		if errors.Is(statErr, storage.ErrNotFound) {
			continue
		} else if statErr != nil {
			return storageError(ctx, statErr)
		}
		if live.OwnerID != image.Key || live.Size != receipt.Size {
			return ErrStorage
		}
		if err = driver.DeleteCurrent(ctx, receipt.Key); err != nil {
			return storageError(ctx, err)
		}
		if _, err = driver.Stat(ctx, receipt.Key); !errors.Is(err, storage.ErrNotFound) {
			return ErrStorage
		}
	}
	if err = s.deps.Images.FinishTrash(ctx, image.Key, image.OperationID); err != nil {
		return fmt.Errorf("finish recycling image: %w", err)
	}
	s.emit(ctx, EventImageTrashed, image, backend)
	return nil
}

// Restore copies live objects back and revalidates authorization before activation.
func (s *ImageService) Restore(ctx context.Context, subject TokenSubject, key string) error {
	if err := s.lockOperations(ctx); err != nil {
		return err
	}
	defer s.unlockOperations()
	image, err := s.ownedImage(ctx, subject, key)
	if err != nil {
		return err
	}
	if image.State == model.ImageStateActive && image.Operation == model.ImageOperationRestoreCleanup {
		return s.clearRestoreTrash(ctx, image)
	}
	if image.State == model.ImageStateActive && image.Operation == "" {
		return nil
	}
	op, err := operationID()
	if err != nil {
		return err
	}
	image, err = s.deps.Images.BeginRestore(ctx, key, op, s.grant(subject))
	if err != nil {
		return fmt.Errorf("begin restore: %w", err)
	}
	driver, backend, err := s.imageDriver(ctx, image)
	if err != nil {
		return err
	}
	for _, receipt := range image.ObjectManifest {
		if receipt.Location != model.ObjectLocationCloud {
			continue
		}
		if err = ensureCopy(ctx, driver, trashKey(backend, receipt.Key), receipt.Key, receipt, liveCacheControl); err != nil {
			return s.failRestore(ctx, image, driver, storageError(ctx, err))
		}
	}
	active, err := s.deps.Images.FinishRestore(ctx, key, op, s.grant(subject))
	if err != nil {
		checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		current, checkErr := s.deps.Images.FindByKey(checkCtx, key)
		if checkErr != nil {
			return fmt.Errorf("confirm restore; recovery pending: %w", err)
		}
		if current.State == model.ImageStateActive && current.OperationID == op {
			s.invalidateRandom(checkCtx, current.AlbumID)
			s.emit(checkCtx, EventImageRestored, current, backend)
			return s.clearRestoreTrash(checkCtx, current)
		}
		return s.failRestore(ctx, image, driver, fmt.Errorf("commit restore: %w", err))
	}
	// The row is active again, whether or not its trash copies clear now.
	s.invalidateRandom(ctx, active.AlbumID)
	s.emit(ctx, EventImageRestored, active, backend)
	return s.clearRestoreTrash(ctx, active)
}

func (s *ImageService) failRestore(ctx context.Context, image model.Image, driver storage.Driver, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.cancelRestoredObjects(cleanupCtx, image, driver); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (s *ImageService) cancelRestoredObjects(ctx context.Context, image model.Image, driver storage.Driver) error {
	owned, ok := driver.(storage.OwnedPurger)
	if !ok {
		return ErrStorage
	}
	var failures []error
	for _, receipt := range image.ObjectManifest {
		if receipt.Location == model.ObjectLocationCloud {
			if err := owned.PurgeOwned(ctx, receipt.Key, image.Key); err != nil && !errors.Is(err, storage.ErrOwnership) {
				failures = append(failures, storageError(ctx, err))
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	return s.deps.Images.CancelRestore(ctx, image.Key, image.OperationID)
}

func (s *ImageService) clearRestoreTrash(ctx context.Context, image model.Image) error {
	driver, backend, err := s.imageDriver(ctx, image)
	if err != nil {
		return err
	}
	purger, ok := driver.(storage.ImagePurger)
	if !ok {
		return ErrStorage
	}
	for _, receipt := range image.ObjectManifest {
		if receipt.Location == model.ObjectLocationCloud {
			if err = purger.PurgeImage(ctx, trashKey(backend, receipt.Key), image.Key); err != nil {
				return storageError(ctx, err)
			}
		}
	}
	return s.deps.Images.FinishRestoreCleanup(ctx, image.Key, image.OperationID)
}

// Purge removes every live/trash version and preview before releasing the path.
func (s *ImageService) Purge(ctx context.Context, subject TokenSubject, key string) error {
	if err := s.lockOperations(ctx); err != nil {
		return err
	}
	defer s.unlockOperations()
	image, err := s.ownedImage(ctx, subject, key)
	if err != nil {
		return err
	}
	if image.Operation != model.ImageOperationPurge {
		op, err := operationID()
		if err != nil {
			return err
		}
		image, err = s.deps.Images.BeginPurge(ctx, key, op, s.grant(subject))
		if err != nil {
			return fmt.Errorf("begin purge: %w", err)
		}
	}
	return s.purgeObjects(ctx, image)
}

func (s *ImageService) purgeObjects(ctx context.Context, image model.Image) error {
	driver, backend, err := s.imageDriver(ctx, image)
	if err != nil {
		return err
	}
	purger, ok := driver.(storage.ImagePurger)
	if !ok {
		return ErrStorage
	}
	var failures []error
	for _, receipt := range image.ObjectManifest {
		if receipt.Location == model.ObjectLocationCloud {
			for _, key := range []string{receipt.Key, trashKey(backend, receipt.Key)} {
				if err = purger.PurgeImage(ctx, key, image.Key); err != nil {
					failures = append(failures, storageError(ctx, err))
				}
			}
		} else if err = s.deps.Cache.Delete(ctx, image.StorageID, receipt.Key); err != nil {
			failures = append(failures, storageError(ctx, err))
		}
	}
	if err = errors.Join(failures...); err != nil {
		return fmt.Errorf("physical purge pending: %w", err)
	}
	if err = s.deps.Images.FinishPurge(ctx, image.Key, image.OperationID); err != nil {
		return err
	}
	s.emit(ctx, EventImagePurged, image, backend)
	return nil
}

// Recover must run before serving in the single-instance deployment; it cancels old uploads/restores.
func (s *ImageService) Recover(ctx context.Context) error { return s.recoverOperations(ctx, true) }

// Sweep retries completed-request cleanup and claims expired recycle-bin images.
func (s *ImageService) Sweep(ctx context.Context) error { return s.recoverOperations(ctx, false) }

func (s *ImageService) recoverOperations(ctx context.Context, startup bool) error {
	if err := s.lockOperations(ctx); err != nil {
		return err
	}
	defer s.unlockOperations()
	pending, err := s.deps.Images.PendingOperations(ctx)
	if err != nil {
		return fmt.Errorf("read image recovery journal: %w", err)
	}
	var failures []error
	for _, image := range pending {
		if image.Operation == model.ImageOperationUpload && !startup {
			continue
		}
		var opErr error
		switch image.Operation {
		case model.ImageOperationUpload, model.ImageOperationCleanup:
			// Journal the cleanup before touching storage, so an unreachable
			// backend leaves a row the periodic sweep keeps retrying.
			opErr = s.deps.Images.StartCleanup(ctx, image.Key, image.OperationID)
			var driver storage.Driver
			if opErr == nil {
				driver, _, opErr = s.imageDriver(ctx, image)
			}
			if opErr == nil {
				opErr = s.cleanupUpload(ctx, image, driver)
			}
		case model.ImageOperationTrash:
			opErr = s.moveToTrash(ctx, image)
		case model.ImageOperationRestore:
			driver, _, driverErr := s.imageDriver(ctx, image)
			opErr = driverErr
			if opErr == nil {
				opErr = s.cancelRestoredObjects(ctx, image, driver)
			}
		case model.ImageOperationRestoreCleanup:
			opErr = s.clearRestoreTrash(ctx, image)
		case model.ImageOperationPurge:
			opErr = s.purgeObjects(ctx, image)
		default:
			opErr = ErrImageBusy
		}
		if opErr != nil {
			failures = append(failures, opErr)
		}
	}
	if !startup {
		failures = append(failures, s.purgeDueTrash(ctx)...)
	}
	return errors.Join(failures...)
}

// dueTrashBatch bounds one journal read; purgeDueTrash keeps reading batches
// until the backlog is drained or the sweep's deadline stops it.
const dueTrashBatch = 100

func (s *ImageService) purgeDueTrash(ctx context.Context) []error {
	var failures []error
	for ctx.Err() == nil {
		due, err := s.deps.Images.DueTrash(ctx, s.deps.Now().UTC(), dueTrashBatch)
		if err != nil {
			return append(failures, err)
		}
		claimed := 0
		for _, image := range due {
			op, opErr := operationID()
			if opErr == nil {
				image, opErr = s.deps.Images.BeginSystemPurge(ctx, image.Key, op)
			}
			if opErr == nil {
				claimed++
				opErr = s.purgeObjects(ctx, image)
			}
			if opErr != nil {
				failures = append(failures, opErr)
			}
		}
		// A claimed purge leaves the due list even when storage fails, so a
		// full batch with claims makes progress; anything else ends the pass.
		if len(due) < dueTrashBatch || claimed == 0 {
			break
		}
	}
	return failures
}
