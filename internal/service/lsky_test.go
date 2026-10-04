package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type lskySettingsStub struct {
	apiEnabled   bool
	guestEnabled bool
	group        model.Group
	groupFound   bool
	apiErr       error
	guestErr     error
	ip           string
	capacity     int64
	imageCount   int64
}

func (s *lskySettingsStub) APIEnabled(context.Context) (bool, error) {
	return s.apiEnabled, s.apiErr
}
func (s *lskySettingsStub) GuestUploadEnabled(context.Context) (bool, error) {
	return s.guestEnabled, s.guestErr
}
func (s *lskySettingsStub) GuestGroup(context.Context) (model.Group, bool, error) {
	return s.group, s.groupFound, nil
}
func (s *lskySettingsStub) RegisteredIP(context.Context, uint64) (string, error) {
	return s.ip, nil
}
func (s *lskySettingsStub) GroupCapacityBytes(context.Context, uint64) (int64, error) {
	return s.capacity, nil
}
func (s *lskySettingsStub) CountActiveImages(context.Context, uint64) (int64, error) {
	return s.imageCount, nil
}

type lskyAlbumsStub struct {
	AlbumRepository
	count    int64
	findItem model.Album
	findErr  error
}

func (s *lskyAlbumsStub) CountByOwner(context.Context, uint64) (int64, error) { return s.count, nil }
func (s *lskyAlbumsStub) FindOwned(context.Context, uint64, uint64) (model.Album, error) {
	return s.findItem, s.findErr
}

type lskyPoliciesStub struct {
	PolicyRepository
	rules []model.Policy
}

func (s *lskyPoliciesStub) GroupPolicies(context.Context, uint64) ([]model.Policy, error) {
	return s.rules, nil
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func newLskyServiceFixture(t *testing.T, lsky *lskySettingsStub) *LskyService {
	t.Helper()
	svc, err := NewLskyService(t.Context(), LskyDependencies{
		Lsky: lsky, Albums: &lskyAlbumsStub{count: 2}, Policies: &lskyPoliciesStub{rules: []model.Policy{{ID: 5, Name: "B2 默认"}, {ID: 6, Name: "本机", Enabled: false}}},
		Now: func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestAPIEnabledAndGuestState(t *testing.T) {
	enabled := &lskySettingsStub{apiEnabled: true}
	svc := newLskyServiceFixture(t, enabled)
	if got, err := svc.APIEnabled(t.Context()); err != nil || !got {
		t.Fatalf("APIEnabled = %v err=%v", got, err)
	}
	enabled.apiErr = context.Canceled
	if _, err := svc.APIEnabled(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatalf("APIEnabled swallowed the error: %v", err)
	}

	// Guests need both the switch and a usable group; ok is false otherwise.
	enabled.apiErr = nil
	if state, _, ok, err := svc.GuestUploadState(t.Context()); err != nil || state || ok {
		t.Fatalf("closed guest state = %v/%v err=%v, want closed", state, ok, err)
	}
	enabled.guestEnabled = true
	enabled.guestErr = context.DeadlineExceeded
	if _, _, _, err := svc.GuestUploadState(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("guest state swallowed the error: %v", err)
	}
	enabled.guestErr = nil
	if _, _, ok, err := svc.GuestUploadState(t.Context()); err != nil || ok {
		t.Fatalf("enabled switch without a group = ok %v err=%v, want false", ok, err)
	}
	enabled.groupFound = true
	enabled.group = model.Group{ID: 9, Name: "Guests"}
	state, group, ok, err := svc.GuestUploadState(t.Context())
	if err != nil || !state || !ok || group.ID != 9 {
		t.Fatalf("guest state = %v/%+v/%v err=%v", state, group, ok, err)
	}

	// A canceled context never reaches the repository.
	if _, err := svc.APIEnabled(canceledContext()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context accepted: %v", err)
	}
}

func TestLskyStrategiesMapping(t *testing.T) {
	svc := newLskyServiceFixture(t, &lskySettingsStub{apiEnabled: true})
	rules, err := svc.Strategies(t.Context(), 3)
	if err != nil {
		t.Fatal(err)
	}
	// Only id and name reach the v1 layer, and disabled rules stay filtered by
	// the repository grant semantics.
	if len(rules) != 2 || rules[0].ID != 5 || rules[0].Name != "B2 默认" || rules[1].ID != 6 {
		t.Fatalf("strategies = %+v", rules)
	}
	empty := &lskyPoliciesStub{rules: []model.Policy{}}
	svc2, err := NewLskyService(t.Context(), LskyDependencies{Lsky: &lskySettingsStub{apiEnabled: true}, Albums: &lskyAlbumsStub{}, Policies: empty, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	rules, err = svc2.Strategies(t.Context(), 3)
	if err != nil || len(rules) != 0 {
		t.Fatalf("empty strategies = %+v err=%v", rules, err)
	}
}

func TestLskyProfileAggregation(t *testing.T) {
	stub := &lskySettingsStub{apiEnabled: true, ip: "203.0.113.5", capacity: 1048576, imageCount: 7}
	svc := newLskyServiceFixture(t, stub)
	view, err := svc.Profile(t.Context(), UserView{ID: 4, Username: "alice", Email: "alice@example.com", GroupID: 3, UsedBytes: 2048})
	if err != nil {
		t.Fatal(err)
	}
	if view.Name != "alice" || view.Email != "alice@example.com" || view.RegisteredIP != "203.0.113.5" {
		t.Fatalf("profile identity = %+v", view)
	}
	// Capacities stay in bytes here; the HTTP layer converts to KB.
	if view.Capacity != 1048576 || view.UsedCapacity != 2048 {
		t.Fatalf("profile capacity = %d/%d", view.Capacity, view.UsedCapacity)
	}
	if view.ImageNum != 7 || view.AlbumNum != 2 {
		t.Fatalf("profile counters = %d/%d", view.ImageNum, view.AlbumNum)
	}
	if _, err := svc.Profile(t.Context(), UserView{}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("guest profile accepted: %v", err)
	}
	if _, err := svc.Profile(canceledContext(), UserView{ID: 4}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled profile accepted: %v", err)
	}
}
