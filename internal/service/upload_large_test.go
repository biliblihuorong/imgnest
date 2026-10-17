package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
)

// countingMetadata records whether the pipeline handed a file to the
// metadata parser or scrubber at all.
type countingMetadata struct {
	uploadMetadata
	extracts, scrubs *atomic.Int32
}

func (m countingMetadata) Extract(ctx context.Context, data []byte, info imaging.Info) (model.ImageExif, error) {
	m.extracts.Add(1)
	return m.uploadMetadata.Extract(ctx, data, info)
}
func (m countingMetadata) Scrub(ctx context.Context, data []byte, format, mode string) ([]byte, error) {
	m.scrubs.Add(1)
	return m.uploadMetadata.Scrub(ctx, data, format, mode)
}

func TestGroupFileLimitIsNotCappedByServerDefault(t *testing.T) {
	svc, rows, policy, _, subject := uploadFixture(t, "png")
	policy.group.MaxFileBytes = 4096 // the server default in the fixture is 1024
	for name, read := range map[string]func() (UploadLimits, error){
		"preflight":       func() (UploadLimits, error) { return svc.Preflight(t.Context(), subject, 1) },
		"preflightLimits": func() (UploadLimits, error) { return svc.PreflightLimits(t.Context(), subject) },
		"limitsFor":       func() (UploadLimits, error) { return svc.UploadLimitsFor(t.Context(), subject) },
	} {
		if limits, err := read(); err != nil || limits.MaxFileBytes != 4096 {
			t.Fatalf("%s = %+v, %v; want the group's 4096", name, limits, err)
		}
	}
	data := make([]byte, 3000)
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: data, Filename: "big.png"}); err != nil {
		t.Fatalf("file under the group limit but over the server default refused: %v", err)
	}
	if len(rows.rows) != 1 {
		t.Fatal("upload not committed")
	}
	policy.group.MaxFileBytes = 0
	if limits, err := svc.UploadLimitsFor(t.Context(), subject); err != nil || limits.MaxFileBytes != 1024 {
		t.Fatalf("group without a limit = %+v, %v; want the server default", limits, err)
	}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: data, Filename: "big.png"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("file over the server default accepted for a group without a limit: %v", err)
	}
}

func TestUploadLimitsForAnswersWhenAccountIsFull(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	policy.group.CapacityBytes = 1
	svc.deps.Users = uploadUserRepo{user: model.User{ID: 1, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusEnabled, Role: model.UserRoleUser, UsedBytes: 5}}
	if _, err := svc.PreflightLimits(t.Context(), subject); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("full account preflight = %v", err)
	}
	if limits, err := svc.UploadLimitsFor(t.Context(), subject); err != nil || limits.MaxFileBytes != 1024 {
		t.Fatalf("limits for a full account = %+v, %v", limits, err)
	}
	if _, err := svc.UploadLimitsFor(t.Context(), TokenSubject{userID: 1, passwordHash: "stale"}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("stale credential = %v", err)
	}
}

// lowMetadataLimit stands in for the real 2 GiB metadata limit.
func lowMetadataLimit(t *testing.T) int {
	t.Helper()
	saved := metadataSourceLimit
	metadataSourceLimit = 4096
	t.Cleanup(func() { metadataSourceLimit = saved })
	return metadataSourceLimit
}

func TestOversizedUploadSkipsMetadataSteps(t *testing.T) {
	big := make([]byte, lowMetadataLimit(t)+1)
	for _, tc := range []struct {
		name, format, scrub, mode string
		original, webp, scrubbed  bool
		wantErr                   bool
	}{
		// The original cannot be scrubbed, so only the metadata-free WebP is kept.
		{name: "scrubbed png keeps only webp", format: "png", scrub: "gps", mode: "both", webp: true},
		{name: "no webp rule still drops the original", format: "png", scrub: "all", mode: "none", webp: true},
		// Nothing to clean: the original is stored exactly as uploaded.
		{name: "unscrubbed png keeps original", format: "png", scrub: "none", mode: "both", original: true, webp: true},
		// A WebP source is its own display copy and cannot be dropped.
		{name: "scrubbed webp is refused", format: "webp", scrub: "gps", mode: "both", wantErr: true},
		{name: "unscrubbed webp is stored", format: "webp", scrub: "none", mode: "both", original: true, webp: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, rows, policy, _, subject := uploadFixture(t, tc.format)
			policy.group.MaxFileBytes = int64(len(big)) + 1
			policy.group.AllowedExts = nil
			policy.policy.ScrubMode, policy.policy.WebPMode = tc.scrub, tc.mode
			var extracts, scrubs atomic.Int32
			counting := countingMetadata{extracts: &extracts, scrubs: &scrubs}
			svc.deps.Extractor, svc.deps.Scrubber = counting, counting
			view, err := svc.Upload(t.Context(), subject, UploadInput{Data: big, Filename: "big." + tc.format})
			if extracts.Load() != 0 || scrubs.Load() != 0 {
				t.Fatalf("oversized file reached the metadata parser: extract=%d scrub=%d", extracts.Load(), scrubs.Load())
			}
			if tc.wantErr {
				if !errors.Is(err, ErrProcessing) || ErrorReason(err) != "scrub_too_large" || len(rows.rows) != 0 {
					t.Fatalf("err = %v reason=%q rows=%d", err, ErrorReason(err), len(rows.rows))
				}
				var step *ProcessingError
				if !errors.As(err, &step) || step.Step != ProcessingScrub {
					t.Fatalf("import reports would not name the scrub step: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if view.HasOriginal != tc.original || view.HasWebP != tc.webp || view.Scrubbed {
				t.Fatalf("view = %+v", view)
			}
			if rows.exif.GPSLat != nil || len(rows.exif.Raw) != 0 || rows.exif.Orientation != 1 {
				t.Fatalf("oversized file recorded metadata: %+v", rows.exif)
			}
		})
	}
}

func TestOversizedImportWithScrubIsRefusedClearly(t *testing.T) {
	svc, rows, policy, _, _ := uploadFixture(t, "png")
	policy.group.MaxFileBytes = 0
	policy.group.AllowedExts = nil
	big := make([]byte, lowMetadataLimit(t)+1)
	_, err := svc.ImportImage(t.Context(), ImportInput{Data: big, Filename: "old.png", Path: "2020/01/old", Ext: "png", UserID: 1, PolicyID: 1})
	if ErrorReason(err) != "scrub_too_large" || len(rows.rows) != 0 {
		t.Fatalf("copy import of an oversized scrubbed original = %v", err)
	}
	policy.policy.ScrubMode = "none"
	if _, err := svc.ImportImage(t.Context(), ImportInput{Data: big, Filename: "old.png", Path: "2020/01/old", Ext: "png", UserID: 1, PolicyID: 1}); err != nil {
		t.Fatalf("copy import without scrubbing = %v", err)
	}
	over := make([]byte, MaxImportFileBytes+1)
	if _, err := svc.ImportImage(t.Context(), ImportInput{Data: over, Filename: "huge.png", Path: "2020/01/huge", Ext: "png", UserID: 1, PolicyID: 1}); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("group without a limit imported past the default: %v", err)
	}
	policy.group.MaxFileBytes = int64(len(over)) + 1
	if _, err := svc.ImportImage(t.Context(), ImportInput{Data: over, Filename: "huge.png", Path: "2020/01/huge", Ext: "png", UserID: 1, PolicyID: 1}); err != nil {
		t.Fatalf("group limit above the default refused: %v", err)
	}
}

// A file past the old 20 MiB cap is an ordinary upload now: its EXIF is
// archived and its original scrubbed and kept, like any smaller file.
func TestUploadOver20MiBStillGetsMetadataSteps(t *testing.T) {
	svc, rows, policy, _, subject := uploadFixture(t, "png")
	policy.group.MaxFileBytes = 64 << 20
	policy.group.AllowedExts = nil
	var extracts, scrubs atomic.Int32
	counting := countingMetadata{extracts: &extracts, scrubs: &scrubs}
	svc.deps.Extractor, svc.deps.Scrubber = counting, counting
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: make([]byte, 21<<20), Filename: "big.png"})
	if err != nil {
		t.Fatal(err)
	}
	if extracts.Load() != 1 || scrubs.Load() != 1 || !view.HasOriginal || !view.HasWebP || !view.Scrubbed || rows.exif.GPSLat == nil {
		t.Fatalf("extract=%d scrub=%d view=%+v exif=%+v", extracts.Load(), scrubs.Load(), view, rows.exif)
	}
}
