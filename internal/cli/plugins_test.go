package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type subscriberPlugin struct {
	name    string
	mu      sync.Mutex
	got     []extension.Event
	block   chan struct{}
	panicky bool
}

func (p *subscriberPlugin) Name() string { return p.name }
func (p *subscriberPlugin) Mount(context.Context, gin.IRouter, extension.Host) error {
	return nil
}
func (p *subscriberPlugin) LoginProviders(context.Context) []extension.LoginProvider { return nil }
func (p *subscriberPlugin) HandleEvent(_ context.Context, event extension.Event) {
	if p.block != nil {
		<-p.block
	}
	if p.panicky {
		panic("subscriber bug")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got = append(p.got, event)
}
func (p *subscriberPlugin) events() []extension.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]extension.Event(nil), p.got...)
}

type plainPlugin struct{}

func (plainPlugin) Name() string                                             { return "plain" }
func (plainPlugin) Mount(context.Context, gin.IRouter, extension.Host) error { return nil }
func (plainPlugin) LoginProviders(context.Context) []extension.LoginProvider { return nil }

func TestPluginEventsDeliverConvertedEvents(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if newPluginEvents(t.Context(), []extension.Plugin{plainPlugin{}}, logger) != nil {
		t.Fatal("events enabled without a subscriber")
	}
	subscriber := &subscriberPlugin{name: "hooks"}
	events := newPluginEvents(t.Context(), []extension.Plugin{plainPlugin{}, subscriber}, logger)
	at := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	events.Publish(t.Context(), service.Event{Type: service.EventImageUploaded, At: at, Image: &service.EventImage{ID: 7, Key: "k", WebP: "https://cdn/a.webp"}})
	events.Publish(t.Context(), service.Event{Type: service.EventUserRegistered, At: at, User: &service.EventUser{ID: 3, Username: "neo"}})
	events.Close()
	got := subscriber.events()
	if len(got) != 2 || got[0].Image == nil || got[0].Image.ID != 7 || got[0].Image.WebP != "https://cdn/a.webp" || !got[0].At.Equal(at) || got[1].User == nil || got[1].User.Username != "neo" {
		t.Fatalf("delivered: %+v", got)
	}
	// Publishing after Close is a no-op rather than a panic on a closed queue.
	events.Publish(t.Context(), service.Event{Type: service.EventImagePurged})
}

func TestPluginEventsDropWhenFullAndSurvivePanics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	slow := &subscriberPlugin{name: "slow", block: make(chan struct{})}
	broken := &subscriberPlugin{name: "broken", panicky: true}
	events := newPluginEvents(t.Context(), []extension.Plugin{slow, broken}, logger)
	for range pluginEventQueue + 5 {
		events.Publish(t.Context(), service.Event{Type: service.EventImageTrashed})
	}
	close(slow.block)
	events.Close()
	if !strings.Contains(logs.String(), "plugin event queue full") || !strings.Contains(logs.String(), "plugin event handler panicked") {
		t.Fatalf("logs: %s", logs.String())
	}
	if n := len(slow.events()); n < pluginEventQueue || n > pluginEventQueue+1 {
		t.Fatalf("slow subscriber got %d events", n)
	}
}

type reviewPlugin struct {
	plainPlugin
	err error
	got extension.UploadImage
}

func (p *reviewPlugin) InspectUpload(_ context.Context, upload extension.UploadImage, _ []byte) error {
	p.got = upload
	return p.err
}

func TestUploadInspectorAdapterMapsRefusals(t *testing.T) {
	for _, tc := range []struct{ err, want error }{
		{nil, nil},
		{fmt.Errorf("porn: %w", extension.ErrUploadRejected), service.ErrContentRejected},
		{extension.ErrReviewUnavailable, service.ErrReviewUnavailable},
		{extension.ErrUploadLimitReached, service.ErrUploadLimitReached},
	} {
		plugin := &reviewPlugin{err: tc.err}
		hooks := pluginImageHooks([]extension.Plugin{plainPlugin{}, plugin}, nil)
		if len(hooks.Inspectors) != 1 {
			t.Fatalf("inspectors: %d", len(hooks.Inspectors))
		}
		err := hooks.Inspectors[0].InspectUpload(t.Context(), service.UploadInspection{UserID: 3, Filename: "a.png", Format: "png", Width: 4, Height: 5, Frames: 1, Size: 9}, []byte("x"))
		if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Fatalf("%v: got %v", tc.err, err)
		}
		if plugin.got.UserID != 3 || plugin.got.Filename != "a.png" || plugin.got.Width != 4 || plugin.got.Size != 9 {
			t.Fatalf("converted upload: %+v", plugin.got)
		}
	}
}

type settingsPlugin struct {
	plainPlugin
	applied string
}

func (p *settingsPlugin) SettingsSchema(context.Context) (extension.SettingsSchema, bool) {
	return extension.SettingsSchema{Title: "T", Fields: []extension.SettingField{
		{Key: "mode", Label: "模式", Type: extension.SettingSelect, Default: "a", Options: []extension.SettingOption{{Label: "A", Value: "a"}}},
		{Key: "targets", Label: "目标", Type: extension.SettingList, Fields: []extension.SettingField{{Key: "url", Label: "地址", Type: extension.SettingText}}},
	}}, true
}

func (p *settingsPlugin) ApplySettings(_ context.Context, values json.RawMessage) error {
	if strings.Contains(string(values), "bad") {
		return &extension.SettingsError{Field: "targets", Message: "地址无效"}
	}
	p.applied = string(values)
	return nil
}

func (p *settingsPlugin) SettingsStatus(context.Context) []extension.SettingStatus {
	return []extension.SettingStatus{{Label: "授权", Value: "有效", Level: extension.StatusSuccess}}
}

func TestConfigurablePluginAdapter(t *testing.T) {
	plugin := &settingsPlugin{}
	configurable := configurablePlugins([]extension.Plugin{plainPlugin{}, plugin})
	if len(configurable) != 1 || configurable[0].Name() != "plain" {
		t.Fatalf("configurable: %v", configurable)
	}
	schema, ok := configurable[0].Schema(t.Context())
	if !ok || schema.Title != "T" || len(schema.Fields) != 2 || schema.Fields[0].Options[0].Value != "a" || schema.Fields[1].Fields[0].Key != "url" {
		t.Fatalf("schema: %+v", schema)
	}
	if status := configurable[0].Status(t.Context()); len(status) != 1 || status[0].Level != "success" {
		t.Fatalf("status: %v", status)
	}
	err := configurable[0].Apply(t.Context(), json.RawMessage(`{"targets":[{"url":"bad"}]}`))
	var settingsErr *service.PluginSettingsError
	if !errors.As(err, &settingsErr) || settingsErr.Message != "地址无效" || settingsErr.Field != "targets" {
		t.Fatalf("refusal: %v", err)
	}
	if err := configurable[0].Apply(t.Context(), json.RawMessage(`{"mode":"a"}`)); err != nil || plugin.applied != `{"mode":"a"}` {
		t.Fatalf("apply: %v %q", err, plugin.applied)
	}
}
