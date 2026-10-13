package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// ListStorages returns every backend without any configuration values.
func (s *AdminService) ListStorages(ctx context.Context) ([]StorageView, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list admin storages: %w", err)
	}
	rows, err := s.deps.Storages.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list admin storages: %w", err)
	}
	views := make([]StorageView, 0, len(rows))
	for _, row := range rows {
		views = append(views, storageView(row))
	}
	return views, nil
}

// CreateStorage reuses the host provisioning path so cloud credentials are
// sealed with the deployment master key and connectivity is probed exactly
// like init-storage; responses never contain the configuration.
func (s *AdminService) CreateStorage(ctx context.Context, input StorageInput) (StorageView, error) {
	if err := ctx.Err(); err != nil {
		return StorageView{}, fmt.Errorf("create storage: %w", err)
	}
	view, err := s.deps.Provision.CreateStorage(ctx, input)
	if err != nil {
		return StorageView{}, err
	}
	return view, nil
}

// PatchStorage merges the supplied fields. A supplied config is validated and,
// for S3, re-sealed; the candidate driver then passes a full put/copy/delete
// probe before the change is stored. No field of the stored configuration is
// ever part of the response.
func (s *AdminService) PatchStorage(ctx context.Context, id uint64, patch StoragePatch) (StorageView, error) {
	if err := ctx.Err(); err != nil {
		return StorageView{}, fmt.Errorf("patch storage: %w", err)
	}
	if id == 0 {
		return StorageView{}, ErrInvalidInput
	}
	backend, err := s.deps.Storages.Find(ctx, id)
	if err != nil {
		return StorageView{}, fmt.Errorf("find storage: %w", err)
	}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64 {
			return StorageView{}, ErrInvalidInput
		}
		backend.Name = name
	}
	if patch.BaseURL != nil {
		if !validStorageBaseURL(*patch.BaseURL) {
			return StorageView{}, ErrInvalidInput
		}
		backend.BaseURL = strings.TrimRight(*patch.BaseURL, "/")
	}
	if patch.Enabled != nil {
		backend.Enabled = *patch.Enabled
	}
	if len(patch.Config) > 0 {
		if !json.Valid(patch.Config) || len(patch.Config) > 64<<10 {
			return StorageView{}, ErrInvalidInput
		}
		if err := s.checkStorageLocation(ctx, backend, patch.Config); err != nil {
			return StorageView{}, err
		}
		config := patch.Config
		if backend.Driver == "s3" {
			config, err = s.deps.Secrets.Seal(ctx, backend.Driver, patch.Config)
			if err != nil {
				return StorageView{}, fmt.Errorf("encrypt storage configuration: %w", errors.Join(ErrInvalidInput, err))
			}
		}
		candidate := backend
		candidate.Config = config
		driver, err := s.deps.Drivers.DriverFor(ctx, candidate)
		if err != nil {
			return StorageView{}, storageError(ctx, err)
		}
		put, copyOK, deleted := probeStorage(ctx, driver)
		if !put || !copyOK || !deleted {
			return StorageView{}, ErrStorage
		}
		backend.Config = config
	}
	updated, err := s.deps.Storages.Update(ctx, backend)
	if err != nil {
		return StorageView{}, fmt.Errorf("update storage: %w", err)
	}
	return storageView(updated), nil
}

// storageLocation names where a backend's objects live: the local root, or
// the S3 endpoint, region and bucket. Credentials, path style and the public
// base URL can change without moving any object.
type storageLocation struct {
	Root     string `json:"root"`
	Endpoint string `json:"endpoint"`
	Region   string `json:"region"`
	Bucket   string `json:"bucket"`
}

// checkStorageLocation refuses a configuration that would point a backend
// holding images at another directory or bucket: existing links would break
// and the old objects would be orphaned. Moving data needs an explicit
// migration, which ImgNest does not offer yet.
func (s *AdminService) checkStorageLocation(ctx context.Context, backend model.Storage, next json.RawMessage) error {
	images, err := s.deps.References.CountImagesForStorage(ctx, backend.ID)
	if err != nil {
		return fmt.Errorf("count storage images: %w", err)
	}
	if images == 0 {
		return nil
	}
	current := []byte(backend.Config)
	if backend.Driver == "s3" {
		current, err = s.deps.Secrets.Open(ctx, backend.Driver, backend.Config)
		if err != nil {
			return fmt.Errorf("open storage configuration: %w", ErrStillReferenced)
		}
	}
	var before, after storageLocation
	if json.Unmarshal(current, &before) != nil || json.Unmarshal(next, &after) != nil {
		return ErrInvalidInput
	}
	if backend.Driver == "local" {
		before.Root, after.Root = filepath.Clean(before.Root), filepath.Clean(after.Root)
	}
	if before != after {
		return fmt.Errorf("move storage with images: %w", ErrStillReferenced)
	}
	return nil
}

// DeleteStorage refuses backends that rules still reference; the policies
// foreign key guards against concurrent creation races.
func (s *AdminService) DeleteStorage(ctx context.Context, id uint64) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete storage: %w", err)
	}
	if id == 0 {
		return ErrInvalidInput
	}
	references, err := s.deps.References.CountPoliciesForStorage(ctx, id)
	if err != nil {
		return fmt.Errorf("count storage references: %w", err)
	}
	if references > 0 {
		return fmt.Errorf("delete storage: %w", ErrStillReferenced)
	}
	if err := s.deps.Storages.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete storage: %w", err)
	}
	return nil
}

// TestStorage runs the put/copy/delete probe against the stored backend and
// reports each capability separately.
func (s *AdminService) TestStorage(ctx context.Context, id uint64) (StorageTestResult, error) {
	if err := ctx.Err(); err != nil {
		return StorageTestResult{}, fmt.Errorf("test storage: %w", err)
	}
	if id == 0 {
		return StorageTestResult{}, ErrInvalidInput
	}
	backend, err := s.deps.Storages.Find(ctx, id)
	if err != nil {
		return StorageTestResult{}, fmt.Errorf("find storage: %w", err)
	}
	driver, err := s.deps.Drivers.DriverFor(ctx, backend)
	if err != nil {
		return StorageTestResult{}, storageError(ctx, err)
	}
	put, copyOK, deleted := probeStorage(ctx, driver)
	return StorageTestResult{OK: put && copyOK && deleted, Checks: StorageChecks{Put: put, Copy: copyOK, Delete: deleted}}, nil
}

// probeStorage writes, copies, and deletes private probe objects, reporting
// each capability separately. Probe keys live under _checks and are always
// cleaned up; no user data is read or written.
func probeStorage(ctx context.Context, driver storage.Driver) (put, copyOK, deleted bool) {
	owner, err := operationID()
	if err != nil {
		return false, false, false
	}
	key := "_checks/" + owner
	copyKey := key + "-copy"
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		// DeleteCurrent leaves a delete marker on versioned buckets (B2), and
		// PurgeOwned keeps markers, so prefer the full-history purge the
		// creation check uses; probe keys are random and only ever ours.
		for _, probeKey := range []string{key, copyKey} {
			if purger, ok := driver.(storage.ImagePurger); ok {
				_ = purger.PurgeImage(cleanup, probeKey, owner)
			} else if purger, ok := driver.(storage.OwnedPurger); ok {
				_ = purger.PurgeOwned(cleanup, probeKey, owner)
			}
		}
	}()
	options := storage.PutOptions{OwnerID: owner, MIME: "application/octet-stream", CacheControl: "no-store"}
	if _, err := driver.PutNew(ctx, key, strings.NewReader("imgnest storage probe"), options); err != nil {
		return false, false, false
	}
	put = true
	if _, err := driver.Copy(ctx, key, copyKey, options); err == nil {
		copyOK = true
	}
	if err := driver.DeleteCurrent(ctx, key); err == nil {
		if _, statErr := driver.Stat(ctx, key); errors.Is(statErr, storage.ErrNotFound) {
			deleted = true
		}
	}
	return put, copyOK, deleted
}

// validStorageBaseURL mirrors the provisioning URL rules.
func validStorageBaseURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Host != "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}
