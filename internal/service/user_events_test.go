package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/service"
)

type userEventSink struct {
	mu     sync.Mutex
	events []service.Event
}

func (s *userEventSink) Publish(_ context.Context, event service.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func TestRegistrationPublishesUserEvent(t *testing.T) {
	fixture := newAuthFixture(t, "sqlite")
	sink := &userEventSink{}
	fixture.service.UseEvents(sink)
	user := registerUser(t, fixture, "eventuser")
	if len(sink.events) != 1 {
		t.Fatalf("events: %d", len(sink.events))
	}
	event := sink.events[0]
	if event.Type != service.EventUserRegistered || event.User == nil || event.User.ID != user.ID || event.User.Username != "eventuser" || event.Image != nil {
		t.Fatalf("event: %+v", event)
	}
	enableRegistration(t, fixture)
	if _, err := fixture.service.Register(t.Context(), service.RegisterInput{Username: "eventuser", Email: "eventuser@example.com", Password: testPassword}); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	if len(sink.events) != 1 {
		t.Fatal("failed registration published an event")
	}
}
