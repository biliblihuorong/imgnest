package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/imaging"
)

// prefixProcessor probes any "webp..." payload as a 10x8 WebP, except
// "webp-small", so display transformers can return marked WebP bytes.
type prefixProcessor struct{ uploadProcessor }

func (p prefixProcessor) Probe(ctx context.Context, data []byte) (imaging.Info, error) {
	if strings.HasPrefix(string(data), "webp") {
		info := imaging.Info{Format: "webp", Ext: "webp", MIME: "image/webp", Width: 10, Height: 8, LoadedFrames: 1, Orientation: 1}
		if string(data) == "webp-small" {
			info.Width, info.Height = 4, 3
		}
		return info, nil
	}
	return p.uploadProcessor.Probe(ctx, data)
}

type recordingTransformer struct {
	out   []byte
	err   error
	calls []DisplayImage
}

func (r *recordingTransformer) TransformDisplay(_ context.Context, image DisplayImage, webp []byte) ([]byte, error) {
	r.calls = append(r.calls, image)
	if r.out == nil {
		return webp, r.err
	}
	return r.out, r.err
}

type recordingSink struct {
	mu     sync.Mutex
	events []Event
}

func (r *recordingSink) Publish(_ context.Context, event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingSink) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	types := make([]string, 0, len(r.events))
	for _, event := range r.events {
		types = append(types, event.Type)
	}
	return types
}

func TestDisplayTransformerRewritesOnlyTheEncodedWebP(t *testing.T) {
	svc, _, _, local, subject := uploadFixture(t, "png")
	svc.deps.Imaging = prefixProcessor{}
	marker := &recordingTransformer{out: []byte("webp-marked")}
	svc.deps.Display = []DisplayTransformer{marker}
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if len(marker.calls) != 1 {
		t.Fatalf("transformer calls: %d", len(marker.calls))
	}
	got := marker.calls[0]
	if got.UserID != 1 || got.GroupID != 1 || got.PolicyID != 1 || got.StorageID != 1 || got.Format != "png" || got.Width != 10 || got.Height != 8 || got.Frames != 1 || got.Quality != 80 || got.Effort != 4 || got.Lossless {
		t.Fatalf("display input: %+v", got)
	}
	if view.WebPSize != int64(len("webp-marked")) {
		t.Fatalf("stored WebP was not the transformed one: %d", view.WebPSize)
	}
	body, _, err := local.Open(t.Context(), "2026/10/a.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	original := make([]byte, 64)
	n, _ := body.Read(original)
	if string(original[:n]) != "source" {
		t.Fatalf("original changed: %q", original[:n])
	}
}

func TestDisplayTransformerSkipsWebPSourcesAndNoWebPRules(t *testing.T) {
	for _, tc := range []struct{ format, mode string }{{"webp", "both"}, {"png", "none"}} {
		svc, _, policy, _, subject := uploadFixture(t, tc.format)
		policy.policy.WebPMode = tc.mode
		marker := &recordingTransformer{}
		svc.deps.Display = []DisplayTransformer{marker}
		data := "source"
		if tc.format == "webp" {
			data = "webp"
		}
		if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte(data), Filename: "a." + tc.format}); err != nil {
			t.Fatal(err)
		}
		if len(marker.calls) != 0 {
			t.Fatalf("%s/%s: original WebP passed to transformer", tc.format, tc.mode)
		}
	}
}

func TestDisplayTransformerFailuresRejectTheUpload(t *testing.T) {
	for name, transformer := range map[string]*recordingTransformer{
		"error":     {err: errors.New("watermark failed")},
		"not webp":  {out: []byte("png-bytes")},
		"resized":   {out: []byte("webp-small")},
		"too large": {out: []byte("webp" + strings.Repeat("x", 2048))},
		"empty":     {out: []byte{}},
	} {
		t.Run(name, func(t *testing.T) {
			svc, rows, _, _, subject := uploadFixture(t, "png")
			svc.deps.Imaging = prefixProcessor{}
			svc.deps.Display = []DisplayTransformer{transformer}
			if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"}); !errors.Is(err, ErrProcessing) {
				t.Fatalf("upload accepted: %v", err)
			}
			if len(rows.rows) != 0 {
				t.Fatal("rejected upload reserved a row")
			}
		})
	}
}

func TestImageLifecycleEventsAfterCommit(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	sink := &recordingSink{}
	svc.deps.Events = sink
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if err = svc.Restore(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if err = svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if err = svc.Purge(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	want := []string{EventImageUploaded, EventImageTrashed, EventImageRestored, EventImageTrashed, EventImagePurged}
	if got := sink.types(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events: %v", got)
	}
	uploaded := sink.events[0].Image
	if uploaded == nil || uploaded.ID != view.ID || uploaded.Key != view.Key || uploaded.Original != "http://images.test/i/1/2026/10/a.png" || uploaded.WebP != "http://images.test/i/1/2026/10/a.webp" || uploaded.Thumbnail != "http://images.test/i/1/2026/10/a_thumbs.webp" {
		t.Fatalf("uploaded event: %+v", uploaded)
	}
}

func TestFailedUploadPublishesNoEvent(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	sink := &recordingSink{}
	svc.deps.Events = sink
	svc.deps.Display = []DisplayTransformer{&recordingTransformer{err: errors.New("boom")}}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"}); err == nil {
		t.Fatal("upload accepted")
	}
	if len(sink.types()) != 0 {
		t.Fatalf("events for a failed upload: %v", sink.types())
	}
}
