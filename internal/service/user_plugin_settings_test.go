package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type personalStoreFake struct {
	data   model.PluginUserData
	asset  []byte
	mime   string
	fail   error
	writes int
}

func (f *personalStoreFake) Read(_ context.Context, _ string, id uint64) (model.PluginUserData, error) {
	if id != f.data.User.ID {
		return model.PluginUserData{}, ErrNotFound
	}
	return f.data, nil
}
func (f *personalStoreFake) Asset(context.Context, string, uint64) ([]byte, string, error) {
	return append([]byte(nil), f.asset...), f.mime, nil
}
func (f *personalStoreFake) SaveValues(_ context.Context, _ string, _, group uint64, values json.RawMessage) error {
	if f.fail != nil {
		return f.fail
	}
	if group != f.data.User.GroupID {
		return ErrForbidden
	}
	f.data.Values = append(json.RawMessage(nil), values...)
	f.writes++
	return nil
}
func (f *personalStoreFake) SaveAsset(_ context.Context, _ string, _, group uint64, data []byte, mime string) error {
	if f.fail != nil {
		return f.fail
	}
	if group != f.data.User.GroupID {
		return ErrForbidden
	}
	f.asset = append([]byte(nil), data...)
	f.mime = mime
	f.writes++
	return nil
}

type personalPluginFake struct {
	hidden        bool
	beforePrepare func()
}

func (p *personalPluginFake) Name() string { return "example" }
func (p *personalPluginFake) Schema(_ context.Context, user model.PluginSettingsUser) (UserPluginSchema, bool) {
	return UserPluginSchema{PluginSchema: PluginSchema{Title: "Personal example", Fields: []PluginField{{Key: "text", Label: "Text", Type: PluginFieldText, Default: "group default"}}}, ReadOnly: user.GroupName == "Free", Notice: "Group policy", AssetLabel: "Logo"}, !p.hidden
}
func (p *personalPluginFake) Validate(context.Context, model.PluginSettingsUser, json.RawMessage) error {
	return nil
}
func (p *personalPluginFake) PrepareAsset(_ context.Context, _ model.PluginSettingsUser, data []byte) ([]byte, string, error) {
	if p.beforePrepare != nil {
		p.beforePrepare()
	}
	if string(data) == "bad" {
		return nil, "", ErrUnsupportedFormat
	}
	return append([]byte("normalized:"), data...), "image/png", nil
}

func personalService(t *testing.T) (*UserPluginSettingsService, *personalStoreFake, *personalPluginFake) {
	t.Helper()
	store := &personalStoreFake{data: model.PluginUserData{User: model.PluginSettingsUser{ID: 7, GroupID: 2, GroupName: "Paid"}, Values: json.RawMessage(`{"text":"Alice"}`)}}
	plugin := &personalPluginFake{}
	svc, err := NewUserPluginSettingsService(t.Context(), store, []UserConfigurablePlugin{plugin})
	if err != nil {
		t.Fatal(err)
	}
	return svc, store, plugin
}

func TestUserPluginSettingsOwnershipAndGroupControl(t *testing.T) {
	svc, store, plugin := personalService(t)
	views, err := svc.List(t.Context(), 7)
	if err != nil || len(views) != 1 || views[0].Values["text"] != "Alice" {
		t.Fatalf("list=%+v err=%v", views, err)
	}
	if _, err := svc.List(t.Context(), 8); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other owner=%v", err)
	}
	store.data.User.GroupName = "Free"
	views, err = svc.List(t.Context(), 7)
	if err != nil || !views[0].ReadOnly || views[0].Values["text"] != "group default" {
		t.Fatalf("controlled view=%+v err=%v", views, err)
	}
	if _, err := svc.Save(t.Context(), 7, "example", json.RawMessage(`{"text":"bypass"}`)); !errors.Is(err, ErrForbidden) {
		t.Fatalf("controlled save=%v", err)
	}
	if _, err := svc.PutAsset(t.Context(), 7, "example", []byte("new")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("controlled upload=%v", err)
	}
	if _, err := svc.DeleteAsset(t.Context(), 7, "example"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("controlled delete=%v", err)
	}
	if store.writes != 0 {
		t.Fatal("controlled requests persisted changes")
	}
	plugin.hidden = true
	views, err = svc.List(t.Context(), 7)
	if err != nil || len(views) != 0 {
		t.Fatalf("hidden list=%v %v", views, err)
	}
	if _, _, err := svc.Asset(t.Context(), 7, "example"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("hidden asset=%v", err)
	}
}

func TestUserPluginAssetFailureAndGroupRacePreservePrevious(t *testing.T) {
	svc, store, plugin := personalService(t)
	store.asset = []byte("old")
	store.mime = "image/png"
	if _, err := svc.PutAsset(t.Context(), 7, "example", []byte("bad")); !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("bad format=%v", err)
	}
	if string(store.asset) != "old" {
		t.Fatal("invalid upload replaced old asset")
	}
	store.fail = errors.New("database unavailable")
	if _, err := svc.PutAsset(t.Context(), 7, "example", []byte("new")); err == nil {
		t.Fatal("failed write succeeded")
	}
	if string(store.asset) != "old" {
		t.Fatal("failed write replaced old asset")
	}
	store.fail = nil
	plugin.beforePrepare = func() { store.data.User.GroupID = 3; store.data.User.GroupName = "Free" }
	if _, err := svc.PutAsset(t.Context(), 7, "example", []byte("new")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("demoted upload=%v", err)
	}
	if string(store.asset) != "old" || store.writes != 0 {
		t.Fatal("demotion race wrote an asset")
	}
}
