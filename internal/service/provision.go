package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// SecretCodec authenticates encrypted storage configuration without exposing its key.
type SecretCodec interface {
	Seal(context.Context, string, []byte) (json.RawMessage, error)
	Open(context.Context, string, json.RawMessage) ([]byte, error)
}

// TemplateValidator checks supported path variables before a rule is saved.
type TemplateValidator interface {
	Validate(context.Context, ...string) error
}

// TemplateValidatorFunc adapts the independent template validator.
type TemplateValidatorFunc func(context.Context, ...string) error

// Validate invokes the supplied validator.
func (f TemplateValidatorFunc) Validate(ctx context.Context, templates ...string) error {
	return f(ctx, templates...)
}

// StorageInput is accepted only by the host-administrator setup commands in M2.
type StorageInput struct {
	Name    string          `json:"name"`
	Driver  string          `json:"driver"`
	BaseURL string          `json:"base_url"`
	Config  json.RawMessage `json:"config"`
}

// StorageView contains no backend credentials or encrypted configuration.
type StorageView struct {
	ID      uint64 `json:"id"`
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`
}

// ProvisionService validates storage and processing configuration before persistence.
type ProvisionService struct {
	storages  StorageRepository
	policies  PolicyRepository
	drivers   StorageProvider
	secrets   SecretCodec
	templates TemplateValidator
}

// NewProvisionService creates host setup operations through injected capabilities.
func NewProvisionService(ctx context.Context, stores StorageRepository, policies PolicyRepository, drivers StorageProvider, secrets SecretCodec, templates TemplateValidator) (*ProvisionService, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if stores == nil || policies == nil || drivers == nil || secrets == nil || templates == nil {
		return nil, ErrInvalidInput
	}
	return &ProvisionService{storages: stores, policies: policies, drivers: drivers, secrets: secrets, templates: templates}, nil
}

// CreateStorage validates connectivity and encrypts cloud configuration at rest.
func (s *ProvisionService) CreateStorage(ctx context.Context, input StorageInput) (StorageView, error) {
	if err := ctx.Err(); err != nil {
		return StorageView{}, err
	}
	name := strings.TrimSpace(input.Name)
	// The OpenAPI contract declares config optional; an omitted object is an
	// empty one, and the driver decides whether its keys are required.
	if len(input.Config) == 0 {
		input.Config = json.RawMessage("{}")
	}
	base, err := url.Parse(input.BaseURL)
	if name == "" || utf8.RuneCountInString(name) > 64 || !utf8.ValidString(name) || err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" || !json.Valid(input.Config) || len(input.Config) > 64<<10 {
		return StorageView{}, ErrInvalidInput
	}
	if input.Driver != "local" && input.Driver != "s3" {
		return StorageView{}, ErrInvalidInput
	}
	config := input.Config
	if input.Driver == "s3" {
		config, err = s.secrets.Seal(ctx, input.Driver, input.Config)
		if err != nil {
			return StorageView{}, fmt.Errorf("encrypt storage configuration: %w", errors.Join(ErrInvalidInput, err))
		}
	}
	candidate := model.Storage{Name: name, Driver: input.Driver, BaseURL: strings.TrimRight(input.BaseURL, "/"), Config: config, Enabled: true}
	driver, err := s.drivers.DriverFor(ctx, candidate)
	if err != nil {
		return StorageView{}, storageError(ctx, err)
	}
	if err = checkStorage(ctx, driver); err != nil {
		return StorageView{}, err
	}
	created, err := s.storages.Create(ctx, candidate)
	if err != nil {
		return StorageView{}, fmt.Errorf("create storage: %w", err)
	}
	return storageView(created), nil
}

func storageView(value model.Storage) StorageView {
	return StorageView{ID: value.ID, Name: value.Name, Driver: value.Driver, BaseURL: value.BaseURL, Enabled: value.Enabled}
}

// DefaultPolicy returns the documented conservative processing defaults.
func DefaultPolicy(ctx context.Context, storageID uint64, name string) (model.Policy, error) {
	if err := ctx.Err(); err != nil {
		return model.Policy{}, err
	}
	if storageID == 0 {
		return model.Policy{}, ErrInvalidInput
	}
	return model.Policy{StorageID: storageID, Name: name, PathTpl: "{Y}/{m}/{d}", NameTpl: "{uniqid}", WebPMode: "both", ScrubMode: "gps", LinkPrefer: "webp", HEIFMode: "webp_only", OnConflict: "rename", WebPQuality: 80, WebPEffort: 4, ThumbSize: 400, ThumbEnabled: true, StripMeta: true, SkipIfLarger: true, Enabled: true}, nil
}

// CreatePolicy validates each processing field and atomically binds the rule to a group.
func (s *ProvisionService) CreatePolicy(ctx context.Context, policy model.Policy, groupID uint64, makeDefault bool) (model.Policy, error) {
	if err := ctx.Err(); err != nil {
		return model.Policy{}, err
	}
	if strings.TrimSpace(policy.Name) == "" || utf8.RuneCountInString(policy.Name) > 64 || !utf8.ValidString(policy.Name) || policy.StorageID == 0 || groupID == 0 {
		return model.Policy{}, ErrInvalidInput
	}
	if err := s.templates.Validate(ctx, policy.PathTpl, policy.NameTpl); err != nil {
		return model.Policy{}, ErrInvalidInput
	}
	if policy.WebPMode != "both" && policy.WebPMode != "webp_only" && policy.WebPMode != "none" {
		return model.Policy{}, ErrInvalidInput
	}
	if policy.ScrubMode != "gps" && policy.ScrubMode != "all" && policy.ScrubMode != "none" {
		return model.Policy{}, ErrInvalidInput
	}
	if policy.LinkPrefer != "webp" && policy.LinkPrefer != "original" {
		return model.Policy{}, ErrInvalidInput
	}
	if policy.HEIFMode != "webp_only" && policy.HEIFMode != "keep" && policy.HEIFMode != "reject" {
		return model.Policy{}, ErrInvalidInput
	}
	if policy.HEIFMode == "keep" && policy.ScrubMode != "none" {
		return model.Policy{}, ErrInvalidInput
	}
	if policy.OnConflict != "rename" && policy.OnConflict != "reject" {
		return model.Policy{}, ErrInvalidInput
	}
	if policy.WebPQuality < 1 || policy.WebPQuality > 100 || policy.WebPEffort < 0 || policy.WebPEffort > 6 || policy.MaxWidth < 0 || policy.MaxHeight < 0 || policy.MaxWidth > 100000000 || policy.MaxHeight > 100000000 || policy.ThumbSize < 1 || policy.ThumbSize > 100000 {
		return model.Policy{}, ErrInvalidInput
	}
	backend, err := s.storages.Find(ctx, policy.StorageID)
	if err != nil {
		return model.Policy{}, fmt.Errorf("read policy storage: %w", err)
	}
	if !backend.Enabled {
		return model.Policy{}, ErrForbidden
	}
	created, err := s.policies.CreateAndBind(ctx, policy, groupID, makeDefault)
	if err != nil {
		return model.Policy{}, fmt.Errorf("create upload policy: %w", err)
	}
	return created, nil
}

func checkStorage(ctx context.Context, driver storage.Driver) (result error) {
	owner, err := operationID()
	if err != nil {
		return err
	}
	key := "_checks/" + owner
	copyKey := key + "-copy"
	content := []byte("imgnest storage check")
	owned, ok := driver.(storage.OwnedPurger)
	if !ok {
		return ErrStorage
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		for _, probeKey := range []string{key, copyKey} {
			if err := owned.PurgeOwned(cleanup, probeKey, owner); err != nil && !errors.Is(err, storage.ErrOwnership) {
				result = errors.Join(result, ErrStorage)
			}
		}
	}()
	options := storage.PutOptions{OwnerID: owner, MIME: "application/octet-stream", CacheControl: "no-store"}
	if _, err = driver.PutNew(ctx, key, bytes.NewReader(content), options); err != nil {
		return storageError(ctx, err)
	}
	if _, err = driver.PutNew(ctx, key, bytes.NewReader([]byte("must not replace")), options); !errors.Is(err, storage.ErrExists) {
		return ErrStorage
	}
	if _, err = driver.Copy(ctx, key, copyKey, options); err != nil {
		return storageError(ctx, err)
	}
	body, info, err := driver.Open(ctx, copyKey)
	if err != nil {
		return storageError(ctx, err)
	}
	got, readErr := io.ReadAll(io.LimitReader(body, 1024))
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || info.OwnerID != owner || !bytes.Equal(got, content) {
		return ErrStorage
	}
	if err = driver.DeleteCurrent(ctx, key); err != nil {
		return storageError(ctx, err)
	}
	if _, err = driver.Stat(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		return ErrStorage
	}
	purger, ok := driver.(storage.ImagePurger)
	if !ok {
		return ErrStorage
	}
	for _, probeKey := range []string{key, copyKey} {
		if err = purger.PurgeImage(ctx, probeKey, owner); err != nil {
			return storageError(ctx, err)
		}
	}
	return nil
}
