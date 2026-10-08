package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

func TestSweepDrainsDueTrashBeyondOneBatch(t *testing.T) {
	svc, rows, _, _, _ := uploadFixture(t, "png")
	due := svc.deps.Now().Add(-time.Hour)
	for i := range 2*dueTrashBatch + 5 {
		key := fmt.Sprintf("due-%03d", i)
		rows.rows[key] = model.Image{ID: uint64(i + 1), Key: key, StorageID: 1, Path: "old/" + key, Ext: "png", State: model.ImageStateTrash, PurgeAt: &due}
	}
	if err := svc.Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(rows.rows) != 0 {
		t.Fatalf("sweep left %d expired images in the recycle bin", len(rows.rows))
	}
}

func TestStartupRecoveryLeavesDueTrashToSweep(t *testing.T) {
	svc, rows, _, _, _ := uploadFixture(t, "png")
	due := svc.deps.Now().Add(-time.Hour)
	rows.rows["due"] = model.Image{ID: 1, Key: "due", StorageID: 1, Path: "old/due", Ext: "png", State: model.ImageStateTrash, PurgeAt: &due}
	if err := svc.Recover(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(rows.rows) != 1 {
		t.Fatal("startup recovery purged expired trash instead of leaving it to the sweep")
	}
}

func TestThumbnailDoesNotWaitForLifecycleFence(t *testing.T) {
	svc, _, _, _, subject := uploadFixture(t, "png")
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.lockOperations(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer svc.unlockOperations()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	object, err := svc.Thumbnail(ctx, subject, view.Key)
	if err != nil {
		t.Fatalf("cached preview blocked by a running lifecycle operation: %v", err)
	}
	_ = object.Body.Close()
	if err = svc.deps.Cache.Delete(t.Context(), 1, "2026/10/a_thumbs.webp"); err != nil {
		t.Fatal(err)
	}
	object, err = svc.Thumbnail(ctx, subject, view.Key)
	if err != nil {
		t.Fatalf("preview rebuild blocked by a running lifecycle operation: %v", err)
	}
	_ = object.Body.Close()
	if body, openErr := svc.deps.Cache.Open(t.Context(), 1, "2026/10/a_thumbs.webp"); openErr == nil {
		_ = body.Close()
		t.Fatal("preview cached without holding the lifecycle fence")
	}
}
