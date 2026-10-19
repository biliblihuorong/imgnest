package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type overviewRepoFake struct {
	data  model.Overview
	err   error
	since time.Time
	top   int
}

func (f *overviewRepoFake) Overview(_ context.Context, since time.Time, top int) (model.Overview, error) {
	f.since, f.top = since, top
	return f.data, f.err
}

func TestOverviewFillsDaysAndSplitsBytes(t *testing.T) {
	now := time.Date(2026, 10, 19, 15, 4, 0, 0, time.FixedZone("CST", 8*3600))
	repo := &overviewRepoFake{data: model.Overview{
		Users: 3, Albums: 2, GuestImages: 1, GuestBytes: 10,
		Storages: []model.StorageUsage{
			{StorageID: 1, Name: "local", Driver: "local", Enabled: true, Images: 2, Bytes: model.ObjectBytes{Total: 200, Original: 100, WebP: 40, Thumb: 20}, TrashImages: 1, TrashBytes: 50},
			{StorageID: 2, Name: "s3", Driver: "s3", Images: 1, Bytes: model.ObjectBytes{Total: 30, Original: 30}},
		},
		Policies: []model.PolicyUsage{{PolicyID: 4, Name: "default", StorageID: 1, Enabled: true, Images: 2, Bytes: 200}},
		Uploads:  []model.UploadDay{{Day: "2026-09-20", Uploads: 1, Bytes: 5}, {Day: "2026-10-19", Uploads: 2, Bytes: 9}},
		TopUsers: []model.UserUsage{{UserID: 7, Username: "alice", UsedBytes: 220, Images: 3}},

		VariantEligible: 4, VariantCovered: 1,
	}}
	overview, err := service.NewOverviewService(t.Context(), repo, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	view, err := overview.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !repo.since.Equal(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)) || repo.top != 10 {
		t.Fatalf("asked since %v top %d", repo.since, repo.top)
	}
	want := service.OverviewTotals{Users: 3, Albums: 2, Images: 3, Bytes: 230, TrashImages: 1, TrashBytes: 50, GuestImages: 1, GuestBytes: 10}
	if view.Totals != want {
		t.Fatalf("totals = %+v, want %+v", view.Totals, want)
	}
	if len(view.Storages) != 2 || view.Storages[0].VariantBytes != 40 || view.Storages[1].VariantBytes != 0 {
		t.Fatalf("storages = %+v", view.Storages)
	}
	if len(view.Uploads) != service.OverviewDays || view.Uploads[0] != (service.UploadDayView{Day: "2026-09-20", Uploads: 1, Bytes: 5}) ||
		view.Uploads[1].Uploads != 0 || view.Uploads[service.OverviewDays-1] != (service.UploadDayView{Day: "2026-10-19", Uploads: 2, Bytes: 9}) {
		t.Fatalf("uploads = %+v", view.Uploads)
	}
	if len(view.TopUsers) != 1 || view.TopUsers[0].Username != "alice" || len(view.Policies) != 1 || view.Variants.Covered != 1 {
		t.Fatalf("view = %+v", view)
	}
	repo.err = errors.New("boom")
	if _, err := overview.Overview(t.Context()); err == nil {
		t.Fatal("repository error was swallowed")
	}
	if _, err := service.NewOverviewService(t.Context(), nil, time.Now); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("nil repository err = %v", err)
	}
}
