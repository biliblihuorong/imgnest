package cli

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

type scopedUserSettings struct {
	name  string
	store service.UserPluginStore
}

func extensionUser(u model.PluginSettingsUser) extension.SettingsUser {
	return extension.SettingsUser{ID: u.ID, GroupID: u.GroupID, GroupName: u.GroupName}
}
func (s scopedUserSettings) Read(ctx context.Context, owner uint64) (extension.UserSettingsData, error) {
	data, err := s.store.Read(ctx, s.name, owner)
	return extension.UserSettingsData{User: extensionUser(data.User), Values: data.Values, Version: data.Version}, err
}
func (s scopedUserSettings) Asset(ctx context.Context, owner uint64) ([]byte, string, error) {
	return s.store.Asset(ctx, s.name, owner)
}

type userConfigurablePlugin struct {
	name   string
	plugin extension.UserConfigurable
}

func (p userConfigurablePlugin) Name() string { return p.name }
func (p userConfigurablePlugin) Schema(ctx context.Context, u model.PluginSettingsUser) (service.UserPluginSchema, bool) {
	s, visible := p.plugin.UserSettingsSchema(ctx, extensionUser(u))
	return service.UserPluginSchema{PluginSchema: service.PluginSchema{Title: s.Title, Description: s.Description, Fields: pluginFields(s.Fields)}, ReadOnly: s.ReadOnly, Notice: s.Notice, AssetLabel: s.AssetLabel}, visible
}
func userSettingsError(err error) error {
	var se *extension.SettingsError
	if errors.As(err, &se) {
		return &service.PluginSettingsError{Field: se.Field, Message: se.Message}
	}
	return err
}
func (p userConfigurablePlugin) Validate(ctx context.Context, u model.PluginSettingsUser, raw json.RawMessage) error {
	return userSettingsError(p.plugin.ValidateUserSettings(ctx, extensionUser(u), raw))
}
func (p userConfigurablePlugin) PrepareAsset(ctx context.Context, u model.PluginSettingsUser, data []byte) ([]byte, string, error) {
	processor, ok := p.plugin.(extension.UserAssetProcessor)
	if !ok {
		return nil, "", service.ErrNotFound
	}
	out, mime, err := processor.PrepareUserAsset(ctx, extensionUser(u), data)
	return out, mime, userSettingsError(err)
}

func newUserPluginSettings(ctx context.Context, db *gorm.DB, plugins []extension.Plugin) (*service.UserPluginSettingsService, error) {
	store, err := repo.NewPluginUserSettingsRepository(ctx, db, time.Now)
	if err != nil {
		return nil, err
	}
	var configurable []service.UserConfigurablePlugin
	for _, p := range plugins {
		if user, ok := p.(extension.UserSettingsUser); ok {
			user.UseUserSettings(scopedUserSettings{name: p.Name(), store: store})
		}
		if user, ok := p.(extension.UserConfigurable); ok {
			configurable = append(configurable, userConfigurablePlugin{name: p.Name(), plugin: user})
		}
	}
	return service.NewUserPluginSettingsService(ctx, store, configurable)
}
