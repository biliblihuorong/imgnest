package ratelimit

import (
	"strconv"
	"testing"
	"time"
)

func TestFullTableEvictsInsteadOfRejecting(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	limiter := New(func() time.Time { return now }, time.Minute, 4)
	for i := range 10 {
		now = now.Add(time.Second)
		if !limiter.Allow("flood:"+strconv.Itoa(i), 3) {
			t.Fatalf("new key %d rejected by a full table", i)
		}
	}
	if !limiter.Allow("victim", 3) {
		t.Fatal("legitimate client locked out after a flood")
	}
	if len(limiter.entries) > 4 {
		t.Fatalf("table grew past its capacity: %d", len(limiter.entries))
	}
}

func TestWindowLimitAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	limiter := New(func() time.Time { return now }, time.Minute, 0)
	for range 3 {
		if !limiter.Allow("k", 3) {
			t.Fatal("attempt within limit rejected")
		}
	}
	if limiter.Allow("k", 3) || !limiter.Exceeded("k", 3) {
		t.Fatal("fourth attempt allowed")
	}
	now = now.Add(time.Minute)
	if limiter.Exceeded("k", 3) || !limiter.Allow("k", 3) {
		t.Fatal("window did not expire")
	}
	limiter.Reset("k")
	if limiter.Exceeded("k", 1) {
		t.Fatal("reset kept the counter")
	}
	if !limiter.Allow("unlimited", 0) {
		t.Fatal("zero limit must mean unlimited")
	}
}

func TestClientKeyGroupsIPv6Subnets(t *testing.T) {
	if ClientKey("2001:db8:1:2:aaaa::1") != ClientKey("2001:db8:1:2:bbbb::9") {
		t.Fatal("addresses in one /64 counted separately")
	}
	if ClientKey("2001:db8:1:2::1") == ClientKey("2001:db8:1:3::1") {
		t.Fatal("different /64 subnets merged")
	}
	if ClientKey("203.0.113.7") != "203.0.113.7" || ClientKey("::ffff:203.0.113.7") != "::ffff:203.0.113.7" {
		t.Fatal("IPv4 must stay per address")
	}
}
