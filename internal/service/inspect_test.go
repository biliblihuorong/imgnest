package service

import (
	"context"
	"errors"
	"testing"
)

type recordingInspector struct {
	err    error
	calls  []UploadInspection
	images []string
}

func (r *recordingInspector) InspectUpload(_ context.Context, upload UploadInspection, image []byte) error {
	r.calls = append(r.calls, upload)
	r.images = append(r.images, string(image))
	return r.err
}

func TestUploadInspectorSeesTheDisplayCopyBeforeTransformers(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	svc.deps.Imaging = prefixProcessor{}
	inspector := &recordingInspector{}
	marker := &recordingTransformer{out: []byte("webp-marked")}
	svc.deps.Inspectors = []UploadInspector{inspector}
	svc.deps.Display = []DisplayTransformer{marker}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"}); err != nil {
		t.Fatal(err)
	}
	if len(inspector.calls) != 1 || inspector.images[0] != "webp" {
		t.Fatalf("inspector input: %v", inspector.images)
	}
	got := inspector.calls[0]
	if got.UserID != 1 || got.GroupID != 1 || got.PolicyID != 1 || got.StorageID != 1 || got.Filename != "a.png" || got.Format != "png" || got.Size != int64(len("source")) {
		t.Fatalf("inspection: %+v", got)
	}
}

func TestUploadInspectorGetsTheOriginalWithoutAnEncodedWebP(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	policy.policy.WebPMode = "none"
	inspector := &recordingInspector{}
	svc.deps.Inspectors = []UploadInspector{inspector}
	if _, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"}); err != nil {
		t.Fatal(err)
	}
	if len(inspector.images) != 1 || inspector.images[0] != "source" {
		t.Fatalf("inspector input: %v", inspector.images)
	}
}

func TestUploadInspectorRefusalsStoreNothing(t *testing.T) {
	for name, tc := range map[string]struct{ err, want error }{
		"rejected":    {ErrContentRejected, ErrContentRejected},
		"unavailable": {ErrReviewUnavailable, ErrReviewUnavailable},
		"wrapped":     {errors.Join(errors.New("provider said no"), ErrContentRejected), ErrContentRejected},
		"other":       {errors.New("boom"), ErrProcessing},
	} {
		t.Run(name, func(t *testing.T) {
			svc, rows, _, _, subject := uploadFixture(t, "png")
			second := &recordingInspector{}
			svc.deps.Inspectors = []UploadInspector{&recordingInspector{err: tc.err}, second}
			_, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("upload error: %v", err)
			}
			if len(rows.rows) != 0 {
				t.Fatal("refused upload reserved a row")
			}
			if len(second.calls) != 0 {
				t.Fatal("inspection continued after a refusal")
			}
		})
	}
}
