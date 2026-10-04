package service

import (
	"context"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// guestPolicyStub adds the optional guest capability to the shared upload
// policy stub; the returned triple is exactly what a real repository resolves.
type guestPolicyStub struct {
	uploadPolicy
	calls int
	err   error
}

func (g *guestPolicyStub) GuestUploadPolicy(context.Context, uint64) (model.Policy, model.Storage, model.Group, error) {
	g.calls++
	return g.policy, g.backend, g.group, g.err
}

// guestStoreStub adds the guest image capability on top of the shared upload
// repository stub, mirroring the user_id = 0 rows of the real adapter.
type guestStoreStub struct {
	*uploadRepo
	used      int64
	usedErr   error
	reserved  []model.UploadReservation
	reserveRe func(context.Context, model.UploadReservation) (model.Image, error)
	committed int
}

func (g *guestStoreStub) GuestUsedBytes(context.Context) (int64, error) { return g.used, g.usedErr }
func (g *guestStoreStub) ReserveGuestUpload(ctx context.Context, in model.UploadReservation) (model.Image, error) {
	g.reserved = append(g.reserved, in)
	if g.reserveRe != nil {
		return g.reserveRe(ctx, in)
	}
	return g.ReserveUpload(ctx, in)
}
func (g *guestStoreStub) CommitGuestUpload(ctx context.Context, key, op string, exif model.ImageExif) (model.Image, error) {
	g.committed++
	return g.CommitUpload(ctx, key, op, exif, model.TokenGrant{})
}

func guestFixture(t *testing.T) (*ImageService, *guestPolicyStub, *guestStoreStub, *uploadRepo) {
	t.Helper()
	svc, rows, policy, _, subject := uploadFixture(t, "png")
	guests := &guestPolicyStub{uploadPolicy: *policy}
	store := &guestStoreStub{uploadRepo: rows}
	svc.deps.Policies = guests
	svc.deps.Images = store
	_, _ = subject, rows
	return svc, guests, store, rows
}

func TestGuestPreflightAppliesGroupLimits(t *testing.T) {
	svc, guests, store, _ := guestFixture(t)
	guests.group.CapacityBytes = 0
	guests.group.MaxFileBytes = 0

	limits, err := svc.GuestPreflight(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if limits.MaxFileBytes != 1024 || limits.PerMinute != 0 {
		t.Fatalf("limits = %+v, want the deployment cap without group overrides", limits)
	}
	guests.group.CapacityBytes = 512
	guests.group.MaxFileBytes = 256
	store.used = 512
	if _, err := svc.GuestPreflight(t.Context(), 0); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("full guest bucket accepted: %v", err)
	}
	store.used = 100
	limits, err = svc.GuestPreflight(t.Context(), 0)
	if err != nil || limits.MaxFileBytes != 256 || limits.PerMinute != 0 {
		t.Fatalf("capped limits = %+v err=%v", limits, err)
	}

	guests.err = ErrForbidden
	guests.calls = 0
	if _, err := svc.GuestPreflight(t.Context(), 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("closed rule accepted: %v", err)
	}
	if guests.calls != 1 {
		t.Fatalf("guest policy consulted %d times", guests.calls)
	}
}

func TestGuestUploadSharesThePublishPipeline(t *testing.T) {
	svc, guests, store, _ := guestFixture(t)

	// Albums stay an account feature; the service refuses them directly.
	if _, err := svc.GuestUpload(t.Context(), UploadInput{Data: []byte("source"), Filename: "a.png", AlbumID: 3}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("guest album upload accepted: %v", err)
	}

	view, err := svc.GuestUpload(t.Context(), UploadInput{Data: []byte("source"), Filename: "a.png", IsPublic: true})
	if err != nil {
		t.Fatal(err)
	}
	// The reservation went through the guest entry point with user_id = 0.
	if len(store.reserved) != 1 || store.reserved[0].Image.UserID != 0 {
		t.Fatalf("guest reservations = %+v", store.reserved)
	}
	if store.committed != 1 {
		t.Fatalf("guest commit count = %d", store.committed)
	}
	if view.UserID != 0 || !view.IsPublic {
		t.Fatalf("published guest view = %+v", view)
	}
	if !view.HasOriginal || !view.HasWebP || !view.HasThumb {
		t.Fatal("guest upload skipped the shared processing pipeline")
	}

	// A disabled rule surfaces as forbidden before anything is reserved.
	guests.err = ErrForbidden
	store.reserved = nil
	if _, err := svc.GuestUpload(t.Context(), UploadInput{Data: []byte("source"), Filename: "a.png"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("closed guest upload accepted: %v", err)
	}
	if len(store.reserved) != 0 {
		t.Fatal("refused guest upload still reserved quota")
	}
}

func TestGuestCapabilitiesAreRequired(t *testing.T) {
	// The shared fixture's repositories do not implement the guest
	// capabilities, so the service must refuse to serve guests at all.
	svc, _, _, _, _ := uploadFixture(t, "png")
	if _, err := svc.GuestPreflight(t.Context(), 0); err == nil {
		t.Fatal("guest preflight without the guest policy capability accepted")
	}
	if _, err := svc.GuestUpload(t.Context(), UploadInput{Data: []byte("source"), Filename: "a.png"}); err == nil {
		t.Fatal("guest upload without the guest capabilities accepted")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	svc2, _, _, _ := guestFixture(t)
	if _, err := svc2.GuestUpload(canceled, UploadInput{Data: []byte("source"), Filename: "a.png"}); err == nil {
		t.Fatal("canceled guest upload accepted")
	}
}
