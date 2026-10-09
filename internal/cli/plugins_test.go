package cli

import (
	"bytes"
	"context"
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
