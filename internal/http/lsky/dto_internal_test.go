package lsky

import (
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
)

// The v1 date follows the server time zone, like Lsky's app timezone.
func TestImageItemDateUsesServerTimeZone(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("CST", 8*3600)
	t.Cleanup(func() { time.Local = previous })
	created := time.Date(2026, 10, 11, 1, 2, 3, 0, time.UTC)
	item := buildImageItem(service.ImageView{CreatedAt: created}, nil, created)
	if item.Date != "2026-10-11 09:02:03" {
		t.Fatalf("date = %q, want the server-local time", item.Date)
	}
}
