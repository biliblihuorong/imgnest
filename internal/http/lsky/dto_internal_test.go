package lsky

import (
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
)

// The v1 date follows the injected zone, like Lsky's app timezone: the
// handler passes the server's local zone in production and UTC in tests.
func TestImageItemDateUsesInjectedZone(t *testing.T) {
	created := time.Date(2026, 10, 11, 1, 2, 3, 0, time.UTC)
	cst := time.FixedZone("CST", 8*3600)
	if item := buildImageItem(service.ImageView{CreatedAt: created}, nil, created, cst); item.Date != "2026-10-11 09:02:03" {
		t.Fatalf("date = %q, want the zone-shifted time", item.Date)
	}
	if item := buildImageItem(service.ImageView{CreatedAt: created}, nil, created, time.UTC); item.Date != "2026-10-11 01:02:03" {
		t.Fatalf("date = %q, want the UTC time", item.Date)
	}
	if got := humanDate(created, created.Add(-45*24*time.Hour), cst); got != "2026-08-27" {
		t.Fatalf("humanDate fallback = %q, want the zone-shifted date", got)
	}
}
