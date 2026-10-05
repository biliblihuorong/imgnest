package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestImageListQueryValidation pins the native list guard rails on top of the
// album semantics covered by TestImageListAlbumFilterSemantics.
func TestImageListQueryValidation(t *testing.T) {
	svc, images, _, subject := albumImageFixture(t)
	valid := ImageQuery{Page: 1, Size: 20}
	if _, err := svc.List(t.Context(), subject, valid); err != nil {
		t.Fatal(err)
	}
	if got := images.listOrder(); got != "" {
		t.Fatalf("default order leaked: %q", got)
	}

	for _, order := range []string{"newest", "oldest", "largest", "smallest"} {
		if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, Order: order}); err != nil {
			t.Fatalf("order %q rejected: %v", order, err)
		}
	}
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, Order: "bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown order = %v", err)
	}
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, MinSize: -1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative min size = %v", err)
	}
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, MinSize: 300, MaxSize: 100}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("inverted size bounds = %v", err)
	}
	from := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Minute)
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, From: &from, To: &to}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("inverted time window = %v", err)
	}
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, Keyword: strings.Repeat("字", 201)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized keyword = %v", err)
	}
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, Exif: strings.Repeat("x", 201)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized exif = %v", err)
	}
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, Q: strings.Repeat("x", 201)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversized unified search = %v", err)
	}

	keyword := "vacation"
	exifNeedle := "Canon"
	toBound := to.Add(time.Hour)
	if _, err := svc.List(t.Context(), subject, ImageQuery{Page: 1, Size: 20, Keyword: keyword, Order: "largest", MinSize: 1, MaxSize: 2, Exif: exifNeedle, From: &from, To: &toBound}); err != nil {
		t.Fatal(err)
	}
	filter := images.capturedFilter()
	if filter.Keyword != keyword || filter.Order != "largest" || filter.MinSize != 1 || filter.MaxSize != 2 || filter.Exif != exifNeedle || filter.From == nil || filter.To == nil {
		t.Fatalf("filter mapping lost fields: %+v", filter)
	}
	if filter.Admin || filter.Trash || filter.UserID != 1 {
		t.Fatalf("filter scope wrong: %+v", filter)
	}
}
