package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type releaseStub struct {
	calls   int
	release LatestRelease
	err     error
}

func (r *releaseStub) Latest(context.Context) (LatestRelease, error) {
	r.calls++
	return r.release, r.err
}

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.0.10", "0.0.9", 1},
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0-rc.1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.2.0", "1.10.0", -1},
		{"2.0.0+build", "2.0.0", 0},
	} {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSystemVersionCachesChecks(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	source := &releaseStub{release: LatestRelease{Version: "0.1.0", URL: "https://github.com/biliblihuorong/imgnest/releases/tag/v0.1.0"}}
	svc, err := NewSystemService(context.Background(), "0.0.9\n", source, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.Version(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if view.Current != "0.0.9" || !view.UpdateAvailable || view.Latest == nil || view.Latest.Version != "0.1.0" || view.CheckedAt == nil {
		t.Fatalf("unexpected view %+v", view)
	}
	if _, err := svc.Version(context.Background(), true); err != nil || source.calls != 1 {
		t.Fatalf("forced refresh within a minute called the feed again: %d", source.calls)
	}
	now = now.Add(2 * time.Minute)
	if _, err := svc.Version(context.Background(), true); err != nil || source.calls != 2 {
		t.Fatalf("forced refresh was not honoured: %d", source.calls)
	}
	if _, err := svc.Version(context.Background(), false); err != nil || source.calls != 2 {
		t.Fatalf("cached check called the feed again: %d", source.calls)
	}
	now = now.Add(updateCheckTTL)
	source.err = errors.New("offline")
	view, err = svc.Version(context.Background(), false)
	if err != nil || source.calls != 3 {
		t.Fatalf("stale cache did not recheck: %d %v", source.calls, err)
	}
	if !view.CheckFailed || view.Latest == nil || view.Latest.Version != "0.1.0" {
		t.Fatalf("failed check should keep the last good release: %+v", view)
	}
}

func TestSystemVersionWithoutChecks(t *testing.T) {
	svc, err := NewSystemService(context.Background(), "0.0.9", nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.Version(context.Background(), true)
	if err != nil || view.CheckEnabled || view.Latest != nil || view.UpdateAvailable || view.RepositoryURL == "" {
		t.Fatalf("unexpected view %+v %v", view, err)
	}
}
