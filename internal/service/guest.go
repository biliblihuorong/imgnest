package service

import (
	"context"
	"fmt"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// GuestPolicySource is the optional policy capability behind guest uploads.
// The concrete repository implements it; test doubles that never serve guests
// stay untouched.
type GuestPolicySource interface {
	GuestUploadPolicy(ctx context.Context, policyID uint64) (model.Policy, model.Storage, model.Group, error)
	GuestUploadGroup(ctx context.Context) (model.Group, error)
}

// GuestImageStore is the optional image capability behind guest uploads.
type GuestImageStore interface {
	GuestUsedBytes(ctx context.Context) (int64, error)
	ReserveGuestUpload(ctx context.Context, reservation model.UploadReservation) (model.Image, error)
	CommitGuestUpload(ctx context.Context, key, op string, exif model.ImageExif) (model.Image, error)
}

// GuestPreflight validates the guest group's rule and capacity before a
// multipart body is read. No user row is read or locked.
func (s *ImageService) GuestPreflight(ctx context.Context, policyID uint64) (UploadLimits, error) {
	policies, store, err := s.guestCapabilities()
	if err != nil {
		return UploadLimits{}, err
	}
	policy, backend, group, err := policies.GuestUploadPolicy(ctx, policyID)
	if err != nil {
		return UploadLimits{}, fmt.Errorf("select guest upload policy: %w", err)
	}
	if !policy.Enabled || !backend.Enabled {
		return UploadLimits{}, ErrForbidden
	}
	limits, err := s.guestLimits(ctx, store, group)
	if err != nil {
		return UploadLimits{}, err
	}
	return limits, nil
}

// GuestPreflightLimits resolves the guest group's capacity-derived bounds
// without requiring its default rule to be usable; the rule named in the form
// fields is judged by GuestPreflight once they have been read.
func (s *ImageService) GuestPreflightLimits(ctx context.Context) (UploadLimits, error) {
	policies, store, err := s.guestCapabilities()
	if err != nil {
		return UploadLimits{}, err
	}
	group, err := policies.GuestUploadGroup(ctx)
	if err != nil {
		return UploadLimits{}, fmt.Errorf("resolve guest upload group: %w", err)
	}
	return s.guestLimits(ctx, store, group)
}

func (s *ImageService) guestLimits(ctx context.Context, store GuestImageStore, group model.Group) (UploadLimits, error) {
	used, err := store.GuestUsedBytes(ctx)
	if err != nil {
		return UploadLimits{}, fmt.Errorf("read guest usage: %w", err)
	}
	if group.CapacityBytes > 0 && used >= group.CapacityBytes {
		return UploadLimits{}, ErrQuotaExceeded
	}
	return s.groupLimits(group), nil
}

// GuestUpload publishes anonymous content under user_id = 0. Reservation,
// object writes, compensation and charging follow exactly the same pipeline as
// account uploads; only the authorization mode differs.
func (s *ImageService) GuestUpload(ctx context.Context, input UploadInput) (ImageView, error) {
	if input.AlbumID != 0 {
		return ImageView{}, ErrForbidden
	}
	limits, err := s.GuestPreflight(ctx, input.PolicyID)
	if err != nil {
		return ImageView{}, err
	}
	policies, store, err := s.guestCapabilities()
	if err != nil {
		return ImageView{}, err
	}
	return s.publish(ctx, input, limits, uploadPlan{
		policy:  policies.GuestUploadPolicy,
		reserve: store.ReserveGuestUpload,
		commit:  store.CommitGuestUpload,
	})
}

func (s *ImageService) guestCapabilities() (GuestPolicySource, GuestImageStore, error) {
	policies, ok := s.deps.Policies.(GuestPolicySource)
	if !ok {
		return nil, nil, fmt.Errorf("guest policy capability: %w", ErrInvalidInput)
	}
	store, ok := s.deps.Images.(GuestImageStore)
	if !ok {
		return nil, nil, fmt.Errorf("guest image capability: %w", ErrInvalidInput)
	}
	return policies, store, nil
}
