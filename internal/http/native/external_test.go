package native

import (
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestTicketStoreExpiresAndBounds(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	store := newTicketStore(func() time.Time { return now })
	ticket, err := store.put(service.VerifiedCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(ticketTTL)
	if _, ok := store.take(ticket); ok {
		t.Fatal("expired ticket was accepted")
	}
	for range maxTickets {
		if _, err := store.put(service.VerifiedCredentials{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.put(service.VerifiedCredentials{}); err == nil {
		t.Fatal("store grew past its bound")
	}
	// Expired entries are purged on the next put, freeing room.
	now = now.Add(ticketTTL)
	if _, err := store.put(service.VerifiedCredentials{}); err != nil {
		t.Fatalf("put after expiry: %v", err)
	}
}
