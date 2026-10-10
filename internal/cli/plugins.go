package cli

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/service"
)

// pluginEventQueue bounds the events waiting for one subscriber.
const pluginEventQueue = 256

// pluginEventDrain bounds how long shutdown waits for queued events.
const pluginEventDrain = 10 * time.Second

// imageHooks are the plugin capabilities the image service runs.
type imageHooks struct {
	Events     service.EventSink
	Inspectors []service.UploadInspector
	Trash      []service.TrashPolicy
	Display    []service.DisplayTransformer
}

// pluginEvents fans committed events out to every EventSubscriber plugin,
// each with its own queue and goroutine so one slow plugin delays only itself.
type pluginEvents struct {
	logger  *slog.Logger
	mu      sync.RWMutex
	closed  bool
	queues  []chan extension.Event
	names   []string
	workers sync.WaitGroup
}

// newPluginEvents starts one delivery goroutine per subscriber; it returns nil
// when no plugin subscribes, which disables publishing entirely.
func newPluginEvents(ctx context.Context, plugins []extension.Plugin, logger *slog.Logger) *pluginEvents {
	events := &pluginEvents{logger: logger}
	for _, plugin := range plugins {
		subscriber, ok := plugin.(extension.EventSubscriber)
		if !ok {
			continue
		}
		queue := make(chan extension.Event, pluginEventQueue)
		events.queues = append(events.queues, queue)
		events.names = append(events.names, plugin.Name())
		events.workers.Add(1)
		go events.deliver(context.WithoutCancel(ctx), plugin.Name(), subscriber, queue)
	}
	if len(events.queues) == 0 {
		return nil
	}
	return events
}

func (p *pluginEvents) deliver(ctx context.Context, name string, subscriber extension.EventSubscriber, queue <-chan extension.Event) {
	defer p.workers.Done()
	for event := range queue {
		func() {
			defer func() {
				if recover() != nil {
					p.logger.ErrorContext(ctx, "plugin event handler panicked", "plugin", name, "event", event.Type)
				}
			}()
			subscriber.HandleEvent(ctx, event)
		}()
	}
}

// Publish implements service.EventSink without ever blocking the caller.
func (p *pluginEvents) Publish(ctx context.Context, event service.Event) {
	value := extension.Event{Type: event.Type, At: event.At}
	if image := event.Image; image != nil {
		value.Image = &extension.EventImage{ID: image.ID, Key: image.Key, UserID: image.UserID, AlbumID: image.AlbumID, StorageID: image.StorageID, Path: image.Path, Name: image.Name, MIME: image.MIME, Size: image.Size, Width: image.Width, Height: image.Height, IsPublic: image.IsPublic, Original: image.Original, WebP: image.WebP, Thumbnail: image.Thumbnail}
	}
	if user := event.User; user != nil {
		value.User = &extension.EventUser{ID: user.ID, Username: user.Username}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return
	}
	for i, queue := range p.queues {
		select {
		case queue <- value:
		default:
			p.logger.WarnContext(ctx, "plugin event queue full; event dropped", "plugin", p.names[i], "event", value.Type)
		}
	}
}

// Close stops accepting events and waits a bounded time for queued ones.
func (p *pluginEvents) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		for _, queue := range p.queues {
			close(queue)
		}
	}
	p.mu.Unlock()
	done := make(chan struct{})
	go func() { p.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(pluginEventDrain):
		p.logger.Warn("plugin events still pending at shutdown")
	}
}

// displayTransformer adapts a plugin's DisplayTransformer to the service.
type displayTransformer struct{ plugin extension.DisplayTransformer }

func (d displayTransformer) TransformDisplay(ctx context.Context, image service.DisplayImage, webp []byte) ([]byte, error) {
	return d.plugin.TransformDisplay(ctx, extension.DisplayImage{UserID: image.UserID, GroupID: image.GroupID, PolicyID: image.PolicyID, StorageID: image.StorageID, Format: image.Format, Width: image.Width, Height: image.Height, Frames: image.Frames, Quality: image.Quality, Effort: image.Effort, Lossless: image.Lossless}, webp)
}

// pluginImageHooks collects the plugins' image capabilities; events may be nil.
func pluginImageHooks(plugins []extension.Plugin, events *pluginEvents) imageHooks {
	hooks := imageHooks{}
	if events != nil {
		hooks.Events = events
	}
	for _, plugin := range plugins {
		if inspector, ok := plugin.(extension.UploadInspector); ok {
			hooks.Inspectors = append(hooks.Inspectors, uploadInspector{plugin: inspector})
		}
		if transformer, ok := plugin.(extension.DisplayTransformer); ok {
			hooks.Display = append(hooks.Display, displayTransformer{plugin: transformer})
		}
		if policy, ok := plugin.(extension.TrashPolicy); ok {
			hooks.Trash = append(hooks.Trash, trashPolicy{plugin: policy})
		}
	}
	return hooks
}

// uploadInspector adapts a plugin's UploadInspector to the service, mapping
// the extension's refusal errors to the service's.
type uploadInspector struct{ plugin extension.UploadInspector }

func (u uploadInspector) InspectUpload(ctx context.Context, upload service.UploadInspection, image []byte) error {
	err := u.plugin.InspectUpload(ctx, extension.UploadImage{UserID: upload.UserID, GroupID: upload.GroupID, PolicyID: upload.PolicyID, StorageID: upload.StorageID, Filename: upload.Filename, Format: upload.Format, Width: upload.Width, Height: upload.Height, Frames: upload.Frames, Size: upload.Size}, image)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, extension.ErrUploadRejected):
		return service.ErrContentRejected
	case errors.Is(err, extension.ErrReviewUnavailable):
		return service.ErrReviewUnavailable
	case errors.Is(err, extension.ErrUploadLimitReached):
		return service.ErrUploadLimitReached
	}
	return err
}

// trashPolicy adapts a plugin's TrashPolicy to the service.
type trashPolicy struct{ plugin extension.TrashPolicy }

func (t trashPolicy) TrashDays(ctx context.Context, userID, groupID uint64) (int, bool) {
	return t.plugin.TrashDays(ctx, extension.TrashOwner{UserID: userID, GroupID: groupID})
}

// configurablePlugin adapts a plugin's Configurable (and optional status
// reporter) to the service.
type configurablePlugin struct {
	name   string
	plugin extension.Configurable
	status extension.SettingsStatusReporter
}

func (p configurablePlugin) Name() string { return p.name }

func (p configurablePlugin) Schema(ctx context.Context) (service.PluginSchema, bool) {
	schema, ok := p.plugin.SettingsSchema(ctx)
	return service.PluginSchema{Title: schema.Title, Description: schema.Description, Fields: pluginFields(schema.Fields)}, ok
}

func (p configurablePlugin) Apply(ctx context.Context, values json.RawMessage) error {
	err := p.plugin.ApplySettings(ctx, values)
	var settingsErr *extension.SettingsError
	if errors.As(err, &settingsErr) {
		return &service.PluginSettingsError{Field: settingsErr.Field, Message: settingsErr.Message}
	}
	return err
}

func (p configurablePlugin) Status(ctx context.Context) []service.PluginStatus {
	if p.status == nil {
		return nil
	}
	lines := p.status.SettingsStatus(ctx)
	out := make([]service.PluginStatus, 0, len(lines))
	for _, line := range lines {
		out = append(out, service.PluginStatus{Label: line.Label, Value: line.Value, Level: line.Level})
	}
	return out
}

func pluginFields(fields []extension.SettingField) []service.PluginField {
	if len(fields) == 0 {
		return nil
	}
	out := make([]service.PluginField, 0, len(fields))
	for _, field := range fields {
		options := make([]service.PluginOption, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, service.PluginOption{Label: option.Label, Value: option.Value})
		}
		out = append(out, service.PluginField{Key: field.Key, Label: field.Label, Help: field.Help, Type: field.Type, Placeholder: field.Placeholder, Required: field.Required, Default: field.Default, Min: field.Min, Max: field.Max, Options: options, OptionsFrom: field.OptionsFrom, Multiple: field.Multiple, Fields: pluginFields(field.Fields), ItemLabel: field.ItemLabel})
	}
	return out
}

// configurablePlugins lists the plugins with settings cards.
func configurablePlugins(plugins []extension.Plugin) []service.ConfigurablePlugin {
	var out []service.ConfigurablePlugin
	for _, plugin := range plugins {
		configurable, ok := plugin.(extension.Configurable)
		if !ok {
			continue
		}
		status, _ := plugin.(extension.SettingsStatusReporter)
		out = append(out, configurablePlugin{name: plugin.Name(), plugin: configurable, status: status})
	}
	return out
}

// pluginCounterStore is the durable counter table plugins share.
type pluginCounterStore interface {
	Add(ctx context.Context, plugin, name, subject string, delta int64) (int64, error)
	Get(ctx context.Context, plugin, name, subject string) (int64, error)
}

// pluginCounters scopes the counter table to one plugin.
type pluginCounters struct {
	plugin string
	store  pluginCounterStore
}

func (p pluginCounters) Add(ctx context.Context, name, subject string, delta int64) (int64, error) {
	return p.store.Add(ctx, p.plugin, name, subject, delta)
}

func (p pluginCounters) Get(ctx context.Context, name, subject string) (int64, error) {
	return p.store.Get(ctx, p.plugin, name, subject)
}
