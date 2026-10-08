package service

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

func (r *uploadRepo) BeginTrash(_ context.Context, key, op string, _ model.TokenGrant, days int) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[key]
	if !ok {
		return row, ErrNotFound
	}
	if row.State != model.ImageStateActive || row.Operation != "" {
		return row, ErrImageBusy
	}
	now := time.Now().UTC()
	purge := now.Add(time.Duration(days) * 24 * time.Hour)
	row.State = model.ImageStateTrash
	row.Operation = model.ImageOperationTrash
	row.OperationID = op
	row.DeletedAt = &now
	row.PurgeAt = &purge
	r.rows[key] = row
	return row, nil
}

func TestTrashRejectsSameOwnerDifferentTargetBytes(t *testing.T) {
	svc, _, _, local, subject := uploadFixture(t, "png")
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = local.PutNew(t.Context(), ".trash/2026/10/a.png", bytes.NewReader([]byte("others")), storage.PutOptions{OwnerID: view.Key, MIME: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Trash(t.Context(), subject, view.Key); err == nil {
		t.Fatal("different trash bytes accepted as valid copy")
	}
	if _, err = local.Stat(t.Context(), "2026/10/a.png"); err != nil {
		t.Fatal("source deleted before copy integrity confirmed")
	}
}
func (r *uploadRepo) FinishTrash(_ context.Context, key, op string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if row.OperationID != op {
		return ErrImageBusy
	}
	row.Operation = ""
	r.rows[key] = row
	return nil
}
func (r *uploadRepo) BeginRestore(_ context.Context, key, op string, _ model.TokenGrant) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[key]
	if !ok {
		return row, ErrNotFound
	}
	if row.State != model.ImageStateTrash || row.Operation != "" {
		return row, ErrImageBusy
	}
	row.Operation = model.ImageOperationRestore
	row.OperationID = op
	r.rows[key] = row
	return row, nil
}
func (r *uploadRepo) FinishRestore(_ context.Context, key, op string, _ model.TokenGrant) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if r.commitErr != nil {
		return row, r.commitErr
	}
	row.State = model.ImageStateActive
	row.Operation = model.ImageOperationRestoreCleanup
	row.DeletedAt = nil
	row.PurgeAt = nil
	r.rows[key] = row
	return row, nil
}
func (r *uploadRepo) FinishRestoreCleanup(_ context.Context, key, op string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	row.Operation = ""
	r.rows[key] = row
	return nil
}
func (r *uploadRepo) CancelRestore(_ context.Context, key, op string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if row.State == model.ImageStateActive {
		return ErrImageBusy
	}
	row.Operation = ""
	r.rows[key] = row
	return nil
}
func (r *uploadRepo) BeginPurge(_ context.Context, key, op string, _ model.TokenGrant) (model.Image, error) {
	return r.BeginSystemPurge(context.Background(), key, op)
}
func (r *uploadRepo) BeginSystemPurge(_ context.Context, key, op string) (model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[key]
	if !ok {
		return row, ErrNotFound
	}
	if row.State != model.ImageStateTrash || row.Operation != "" {
		return row, ErrImageBusy
	}
	row.Operation = model.ImageOperationPurge
	row.OperationID = op
	r.rows[key] = row
	return row, nil
}
func (r *uploadRepo) FinishPurge(_ context.Context, key, op string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, key)
	return nil
}
func (r *uploadRepo) PendingOperations(context.Context) ([]model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := []model.Image{}
	for _, row := range r.rows {
		if row.Operation != "" {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
func (r *uploadRepo) DueTrash(_ context.Context, now time.Time, limit int) ([]model.Image, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := []model.Image{}
	for _, row := range r.rows {
		if len(rows) == limit {
			break
		}
		if row.State == model.ImageStateTrash && row.Operation == "" && row.PurgeAt != nil && !row.PurgeAt.After(now) {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func TestTrashRestorePurgeActualLocalObjects(t *testing.T) {
	svc, rows, _, local, subject := uploadFixture(t, "png")
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"2026/10/a.png", "2026/10/a.webp", "2026/10/a_thumbs.webp"} {
		if _, err = local.Stat(t.Context(), key); !errors.Is(err, storage.ErrNotFound) {
			t.Fatal("old URL object still accessible")
		}
	}
	if _, err = svc.deps.Cache.Open(t.Context(), 1, "2026/10/a_thumbs.webp"); err != nil {
		t.Fatal("trash preview lost")
	}
	if err = svc.Restore(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if _, err = local.Stat(t.Context(), "2026/10/a.png"); err != nil {
		t.Fatal("original not restored")
	}
	if _, err = local.Stat(t.Context(), ".trash/2026/10/a.png"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("restore kept billable trash duplicate")
	}
	if err = svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if err = svc.Purge(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	if len(rows.rows) != 0 {
		t.Fatal("purge retained metadata/path")
	}
	if _, err = local.Stat(t.Context(), ".trash/2026/10/a.png"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("purge retained object")
	}
}

func TestRestoreRevokedCommitCompensatesLiveKeepsTrash(t *testing.T) {
	svc, rows, _, local, subject := uploadFixture(t, "png")
	view, err := svc.Upload(t.Context(), subject, UploadInput{Data: []byte("source"), Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Trash(t.Context(), subject, view.Key); err != nil {
		t.Fatal(err)
	}
	rows.commitErr = ErrUnauthenticated
	if err = svc.Restore(t.Context(), subject, view.Key); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked restore: %v", err)
	}
	if _, err = local.Stat(t.Context(), "2026/10/a.png"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatal("unauthorized live copy remains")
	}
	if _, err = local.Stat(t.Context(), ".trash/2026/10/a.png"); err != nil {
		t.Fatal("failed restore lost trash")
	}
	if rows.rows[view.Key].Operation != "" {
		t.Fatal("failed restore kept reservation")
	}
}
