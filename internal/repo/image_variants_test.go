package repo

import (
	"errors"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

const variantDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func withVariants(input model.UploadReservation, widths ...int) model.UploadReservation {
	image := &input.Image
	for _, width := range widths {
		key := model.VariantKey(image.Path, width)
		image.Variants = append(image.Variants, model.ImageVariant{Key: key, Width: width, Height: width / 2, Bytes: int64(width / 100), MIME: model.VariantMIME})
		input.Objects = append(input.Objects, model.ObjectReceipt{Key: key, Size: int64(width / 100), OwnerID: image.Key, Location: model.ObjectLocationCloud, MIME: model.VariantMIME})
	}
	return input
}

func TestReserveUploadValidatesVariants(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "variant-reserve")
		valid := withVariants(f.request("variant-ok", "2026/10/a"), 400, 800)
		image, err := f.images.ReserveUpload(t.Context(), valid)
		if err != nil {
			t.Fatal(err)
		}
		if image.ChargedBytes != 100+40+20+4+8 || len(image.Variants) != 2 {
			t.Fatalf("variants not reserved: charged=%d variants=%d", image.ChargedBytes, len(image.Variants))
		}
		stored, err := f.images.FindByKey(t.Context(), "variant-ok")
		if err != nil || len(stored.Variants) != 2 || stored.Variants[1].Key != "2026/10/a_w800_v1.webp" {
			t.Fatalf("variants not persisted: %+v %v", stored.Variants, err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("variant-none", "2026/10/b")); err != nil {
			t.Fatal(err)
		}
		if plain, err := f.images.FindByKey(t.Context(), "variant-none"); err != nil || plain.Variants == nil || len(plain.Variants) != 0 {
			t.Fatalf("image without variants: %+v %v", plain.Variants, err)
		}

		reused := f.request("variant-reuse", "2026/10/c")
		reused.Image.Variants = model.VariantList{{Key: "2026/10/c.webp", Width: 400, Height: 200, Bytes: 40, MIME: model.VariantMIME}}
		if _, err := f.images.ReserveUpload(t.Context(), reused); err != nil {
			t.Fatalf("reused primary WebP rejected: %v", err)
		}

		for name, mutate := range map[string]func(*model.UploadReservation){
			"undeclared object":   func(r *model.UploadReservation) { r.Objects = r.Objects[:len(r.Objects)-1] },
			"size mismatch":       func(r *model.UploadReservation) { r.Objects[len(r.Objects)-1].Size++ },
			"foreign key":         func(r *model.UploadReservation) { r.Image.Variants[0].Key = "2026/10/other_w400_v1.webp" },
			"duplicate width":     func(r *model.UploadReservation) { r.Image.Variants[1].Width = 400 },
			"reused size differs": func(r *model.UploadReservation) { r.Image.Variants[0].Key = r.Image.Path + ".webp" },
		} {
			bad := withVariants(f.request("variant-bad", "2026/10/d"), 400, 800)
			mutate(&bad)
			if _, err := f.images.ReserveUpload(t.Context(), bad); !errors.Is(err, model.ErrInvalidInput) {
				t.Fatalf("%s accepted: %v", name, err)
			}
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("variant-name", "2026/10/e_w400_v1")); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("reserved variant path accepted: %v", err)
		}
		// A row from before the reservation existed owns a variant-looking path.
		legacy := f.request("variant-legacy", "2026/10/f")
		legacy.Image.Path = "2026/10/f_w400_v1"
		if err := db.Exec("INSERT INTO images (key, user_id, policy_id, storage_id, path, ext, mime, state, operation, operation_id) VALUES (?, ?, ?, ?, ?, 'jpg', 'image/jpeg', 'active', '', 'legacy')", "legacy-row", f.user.ID, f.policy.ID, f.storage.ID, "2026/10/f_w400_v1").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), withVariants(f.request("variant-clash", "2026/10/f"), 400)); !errors.Is(err, model.ErrPathConflict) {
			t.Fatalf("variant key of a legacy path accepted: %v", err)
		}
	})
}

func variantJob(image model.Image, op string, widths ...int) model.VariantJob {
	job := model.VariantJob{ImageKey: image.Key, OperationID: op}
	for _, width := range widths {
		key := model.VariantKey(image.Path, width)
		job.Variants = append(job.Variants, model.ImageVariant{Key: key, Width: width, Height: width / 2, Bytes: int64(width / 100), MIME: model.VariantMIME})
		job.Objects = append(job.Objects, model.ObjectReceipt{Key: key, Size: int64(width / 100), OwnerID: image.Key, Location: model.ObjectLocationCloud, MIME: model.VariantMIME, SHA256: variantDigest})
	}
	return job
}

func TestVariantJournalReservesFinishesAndCancels(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "variant-job")
		if err := db.Exec("UPDATE policies SET responsive_variants = ? WHERE id = ?", true, f.policy.ID).Error; err != nil {
			t.Fatal(err)
		}
		first := reserveAndCommit(t, f, "job-a", "2026/10/a")
		second := reserveAndCommit(t, f, "job-b", "2026/10/b")
		base := usedBytes(t, f)
		candidates, err := f.images.VariantCandidates(t.Context(), first.ID, 10)
		if err != nil || len(candidates) != 1 || candidates[0].ID != second.ID {
			t.Fatalf("candidates after cursor = %v %v", candidates, err)
		}

		if _, err := f.images.BeginVariants(t.Context(), variantJob(first, "op-a", 400, 800)); err != nil {
			t.Fatal(err)
		}
		if usedBytes(t, f) != base+12 {
			t.Fatal("variant bytes not reserved against quota")
		}
		if _, err := f.images.BeginVariants(t.Context(), variantJob(first, "op-again", 400)); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("second journal for one image accepted: %v", err)
		}
		if pending, err := f.images.HasVariantJob(t.Context(), first.ID); err != nil || !pending {
			t.Fatal("journal not visible")
		}
		receipt := variantJob(first, "op-a", 400).Objects[0]
		receipt.VersionID = "v1"
		if err := f.images.RecordVariantReceipt(t.Context(), first.ID, "op-a", receipt); err != nil {
			t.Fatal(err)
		}
		if err := f.images.RecordVariantReceipt(t.Context(), first.ID, "op-other", receipt); !errors.Is(err, model.ErrImageBusy) {
			t.Fatalf("receipt of another operation accepted: %v", err)
		}
		done, err := f.images.FinishVariants(t.Context(), first.ID, "op-a")
		if err != nil {
			t.Fatal(err)
		}
		stored, err := f.images.FindByKey(t.Context(), first.Key)
		if err != nil || len(stored.Variants) != 2 || len(stored.ObjectManifest) != len(first.ObjectManifest)+2 || stored.ChargedBytes != first.ChargedBytes+12 || done.ID != first.ID {
			t.Fatalf("finished variants not published: %+v %v", stored, err)
		}
		if stored.ObjectManifest[len(stored.ObjectManifest)-2].VersionID != "v1" {
			t.Fatal("variant version not journaled")
		}
		if jobs, _ := f.images.PendingVariantJobs(t.Context()); len(jobs) != 0 {
			t.Fatal("finished journal left behind")
		}

		if _, err := f.images.BeginVariants(t.Context(), variantJob(second, "op-b", 400)); err != nil {
			t.Fatal(err)
		}
		jobs, err := f.images.PendingVariantJobs(t.Context())
		if err != nil || len(jobs) != 1 || jobs[0].ImageKey != second.Key || jobs[0].Bytes != 4 {
			t.Fatalf("pending journals = %+v %v", jobs, err)
		}
		if err := f.images.CancelVariants(t.Context(), second.ID, "op-b"); err != nil {
			t.Fatal(err)
		}
		reverted, err := f.images.FindByKey(t.Context(), second.Key)
		if err != nil || reverted.ChargedBytes != second.ChargedBytes || len(reverted.Variants) != 0 || usedBytes(t, f) != base+12 {
			t.Fatalf("cancel did not release the reservation: %+v used=%d", reverted, usedBytes(t, f))
		}
		if err := f.images.CancelVariants(t.Context(), second.ID, "op-b"); err != nil {
			t.Fatalf("repeated cancel: %v", err)
		}

		state, err := f.images.VariantBackfill(t.Context())
		if err != nil || state.Status != model.BackfillIdle {
			t.Fatalf("initial backfill = %+v %v", state, err)
		}
		state.Status, state.Cursor, state.Generated = model.BackfillPaused, second.ID, 3
		if err := f.images.SaveVariantBackfill(t.Context(), state); err != nil {
			t.Fatal(err)
		}
		if again, err := f.images.VariantBackfill(t.Context()); err != nil || again.Cursor != second.ID || again.Status != model.BackfillPaused || again.Generated != 3 {
			t.Fatalf("backfill progress not saved: %+v %v", again, err)
		}
		state.Status = "bogus"
		if err := f.images.SaveVariantBackfill(t.Context(), state); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("invalid backfill status saved: %v", err)
		}
	})
}
