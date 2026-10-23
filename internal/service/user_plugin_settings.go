package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// UserPluginSchema extends an account settings card with group policy and a
// label for its optional private asset. No credentials belong in this schema.
type UserPluginSchema struct {
	PluginSchema
	ReadOnly   bool
	Notice     string
	AssetLabel string
}

// UserConfigurablePlugin validates personal settings without changing shared
// plugin state. The service alone persists authenticated-owner values.
type UserConfigurablePlugin interface {
	Name() string
	Schema(context.Context, model.PluginSettingsUser) (UserPluginSchema, bool)
	Validate(context.Context, model.PluginSettingsUser, json.RawMessage) error
}

// UserPluginAssetProcessor normalizes one bounded private asset.
type UserPluginAssetProcessor interface {
	PrepareAsset(context.Context, model.PluginSettingsUser, []byte) ([]byte, string, error)
}

// UserPluginStore is durable data keyed by plugin and owner. Writes verify
// the expected current group under the user lock before changing anything.
type UserPluginStore interface {
	Read(context.Context, string, uint64) (model.PluginUserData, error)
	Asset(context.Context, string, uint64) ([]byte, string, error)
	SaveValues(context.Context, string, uint64, uint64, json.RawMessage) error
	SaveAsset(context.Context, string, uint64, uint64, []byte, string) error
}

// UserPluginSettingsView is a visible personal card. Asset bytes are served
// only by a separate authenticated endpoint, never included in the JSON.
type UserPluginSettingsView struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Fields      []PluginField  `json:"fields"`
	Values      map[string]any `json:"values"`
	ReadOnly    bool           `json:"read_only"`
	Notice      string         `json:"notice,omitempty"`
	AssetLabel  string         `json:"asset_label,omitempty"`
	HasAsset    bool           `json:"has_asset"`
}

// UserPluginSettingsService exposes only the signed-in account's extensions.
type UserPluginSettingsService struct {
	store   UserPluginStore
	plugins []UserConfigurablePlugin
}

var personalPluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// NewUserPluginSettingsService wires generic account settings capabilities.
func NewUserPluginSettingsService(ctx context.Context, store UserPluginStore, plugins []UserConfigurablePlugin) (*UserPluginSettingsService, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, errors.New("create user plugin settings: missing store")
	}
	seen := map[string]bool{}
	for _, p := range plugins {
		if p == nil || !personalPluginName.MatchString(p.Name()) || seen[p.Name()] {
			return nil, errors.New("create user plugin settings: invalid plugin")
		}
		seen[p.Name()] = true
	}
	return &UserPluginSettingsService{store: store, plugins: plugins}, nil
}

// List returns visible cards, or an empty list in the ordinary edition.
func (s *UserPluginSettingsService) List(ctx context.Context, userID uint64) ([]UserPluginSettingsView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	views := []UserPluginSettingsView{}
	for _, p := range s.plugins {
		data, err := s.store.Read(ctx, p.Name(), userID)
		if err != nil {
			return nil, err
		}
		schema, visible := p.Schema(ctx, data.User)
		if !visible {
			continue
		}
		view, err := s.view(ctx, p.Name(), data, schema)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *UserPluginSettingsService) lookup(ctx context.Context, userID uint64, name string, write bool) (UserConfigurablePlugin, model.PluginUserData, UserPluginSchema, error) {
	if err := ctx.Err(); err != nil {
		return nil, model.PluginUserData{}, UserPluginSchema{}, err
	}
	for _, p := range s.plugins {
		if p.Name() != name {
			continue
		}
		data, err := s.store.Read(ctx, name, userID)
		if err != nil {
			return nil, data, UserPluginSchema{}, err
		}
		schema, visible := p.Schema(ctx, data.User)
		if !visible {
			return nil, data, schema, ErrNotFound
		}
		if write && schema.ReadOnly {
			return nil, data, schema, &ReasonError{Reason: "plugin_user_read_only", Err: ErrForbidden}
		}
		return p, data, schema, nil
	}
	return nil, model.PluginUserData{}, UserPluginSchema{}, ErrNotFound
}

func personalFields(fields []PluginField) error {
	if err := checkPluginFields(fields, true); err != nil {
		return err
	}
	for _, field := range fields {
		if field.Type == PluginFieldSecret || field.Type == PluginFieldList || field.OptionsFrom != "" {
			return ErrInvalidInput
		}
		for _, child := range field.Fields {
			if child.Type == PluginFieldSecret || child.OptionsFrom != "" {
				return ErrInvalidInput
			}
		}
	}
	return nil
}

func (s *UserPluginSettingsService) view(ctx context.Context, name string, data model.PluginUserData, schema UserPluginSchema) (UserPluginSettingsView, error) {
	if err := personalFields(schema.Fields); err != nil {
		return UserPluginSettingsView{}, fmt.Errorf("invalid personal plugin schema: %w", ErrProcessing)
	}
	values, err := decodePluginObject(data.Values)
	if err != nil {
		return UserPluginSettingsView{}, fmt.Errorf("invalid personal plugin data: %w", ErrProcessing)
	}
	if schema.ReadOnly {
		values = nil
	}
	values, err = normalizePluginValues(schema.Fields, values, nil, false)
	if err != nil {
		return UserPluginSettingsView{}, err
	}
	fields := schema.Fields
	if fields == nil {
		fields = []PluginField{}
	}
	view := UserPluginSettingsView{Name: name, Title: schema.Title, Description: schema.Description, Fields: fields, Values: values, ReadOnly: schema.ReadOnly, Notice: schema.Notice, AssetLabel: schema.AssetLabel}
	if schema.AssetLabel != "" && !schema.ReadOnly {
		asset, _, err := s.store.Asset(ctx, name, data.User.ID)
		if err != nil {
			return view, err
		}
		view.HasAsset = len(asset) > 0
	}
	return view, nil
}

// Save validates settings and persists them without applying another user's
// personal state to the shared plugin instance.
func (s *UserPluginSettingsService) Save(ctx context.Context, userID uint64, name string, raw json.RawMessage) (UserPluginSettingsView, error) {
	p, data, schema, err := s.lookup(ctx, userID, name, true)
	if err != nil {
		return UserPluginSettingsView{}, err
	}
	if len(raw) > maxPluginDocument || personalFields(schema.Fields) != nil {
		return UserPluginSettingsView{}, ErrInvalidInput
	}
	input, err := decodePluginObject(raw)
	if err != nil {
		return UserPluginSettingsView{}, ErrInvalidInput
	}
	values, err := normalizePluginValues(schema.Fields, input, nil, true)
	if err != nil {
		return UserPluginSettingsView{}, err
	}
	document := pluginApplyDocument(schema.Fields, values)
	if err := p.Validate(ctx, data.User, document); err != nil {
		return UserPluginSettingsView{}, err
	}
	if _, _, _, err := s.lookup(ctx, userID, name, true); err != nil {
		return UserPluginSettingsView{}, err
	}
	if err := s.store.SaveValues(ctx, name, userID, data.User.GroupID, document); err != nil {
		return UserPluginSettingsView{}, err
	}
	data.Values = document
	return s.view(ctx, name, data, schema)
}

// Asset reads a private asset only while its plugin is visible.
func (s *UserPluginSettingsService) Asset(ctx context.Context, userID uint64, name string) ([]byte, string, error) {
	_, _, schema, err := s.lookup(ctx, userID, name, false)
	if err != nil {
		return nil, "", err
	}
	if schema.AssetLabel == "" {
		return nil, "", ErrNotFound
	}
	data, mime, err := s.store.Asset(ctx, name, userID)
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", ErrNotFound
	}
	return data, mime, nil
}

// PutAsset prepares the Logo before atomically replacing it. Permission is
// checked again after image processing and in the repository transaction.
func (s *UserPluginSettingsService) PutAsset(ctx context.Context, userID uint64, name string, data []byte) (UserPluginSettingsView, error) {
	p, owner, schema, err := s.lookup(ctx, userID, name, true)
	if err != nil {
		return UserPluginSettingsView{}, err
	}
	processor, ok := p.(UserPluginAssetProcessor)
	if !ok || schema.AssetLabel == "" {
		return UserPluginSettingsView{}, ErrNotFound
	}
	if len(data) == 0 {
		return UserPluginSettingsView{}, ErrInvalidInput
	}
	if len(data) > model.MaxPluginUserAssetBytes {
		return UserPluginSettingsView{}, ErrFileTooLarge
	}
	prepared, mime, err := processor.PrepareAsset(ctx, owner.User, data)
	if err != nil {
		return UserPluginSettingsView{}, err
	}
	if len(prepared) == 0 || len(prepared) > model.MaxPluginUserAssetBytes || (mime != "image/png" && mime != "image/webp" && mime != "image/jpeg") {
		return UserPluginSettingsView{}, ErrProcessing
	}
	_, current, schema, err := s.lookup(ctx, userID, name, true)
	if err != nil {
		return UserPluginSettingsView{}, err
	}
	if current.User.GroupID != owner.User.GroupID {
		return UserPluginSettingsView{}, ErrForbidden
	}
	if err := s.store.SaveAsset(ctx, name, userID, owner.User.GroupID, prepared, mime); err != nil {
		return UserPluginSettingsView{}, err
	}
	return s.view(ctx, name, current, schema)
}

// DeleteAsset removes the private asset while retaining the user's settings.
func (s *UserPluginSettingsService) DeleteAsset(ctx context.Context, userID uint64, name string) (UserPluginSettingsView, error) {
	_, data, schema, err := s.lookup(ctx, userID, name, true)
	if err != nil {
		return UserPluginSettingsView{}, err
	}
	if schema.AssetLabel == "" {
		return UserPluginSettingsView{}, ErrNotFound
	}
	if err := s.store.SaveAsset(ctx, name, userID, data.User.GroupID, nil, ""); err != nil {
		return UserPluginSettingsView{}, err
	}
	return s.view(ctx, name, data, schema)
}
