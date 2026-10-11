package repo

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

// webpOnlyRequest stores only the WebP conversion of an upload.
func (f imageFixture) webpOnlyRequest(key, path, sourceExt string) model.UploadReservation {
	req := f.request(key, path)
	req.SourceExt = sourceExt
	req.Image.Ext, req.Image.MIME, req.Image.HasOriginal = "webp", "image/webp", false
	req.Image.Size = 40
	req.Objects = []model.ObjectReceipt{
		{Key: path + ".webp", Size: 40, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/webp"},
		{Key: path + "_thumbs.webp", Size: 20, OwnerID: key, Location: model.ObjectLocationCloud, MIME: "image/webp"},
		{Key: path + "_thumbs.webp", Size: 20, OwnerID: key, Location: model.ObjectLocationThumbCache, MIME: "image/webp"},
	}
	return req
}

// A group's allowed formats apply to what the user uploaded, not to the WebP
// that a webp_only rule stores in its place.
func TestWebPOnlyUploadChecksSourceFormat(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "webp-only")
		if err := db.Exec("UPDATE groups SET allowed_exts = ? WHERE id = ?", `["png"]`, f.user.GroupID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.webpOnlyRequest("no-src", "2026/01/no-src", "")); !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("WebP-only reservation without a source format accepted: %v", err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.webpOnlyRequest("gif-src", "2026/01/gif-src", "gif")); !errors.Is(err, model.ErrUnsupportedFormat) {
			t.Fatalf("disallowed source accepted: %v", err)
		}
		input := f.webpOnlyRequest("png-src", "2026/01/png-src", "png")
		image, err := f.images.ReserveUpload(t.Context(), input)
		if err != nil {
			t.Fatalf("allowed PNG converted to WebP refused: %v", err)
		}
		for _, receipt := range input.Objects {
			receipt.VersionID = "written-version"
			if err := f.images.RecordObjectReceipt(t.Context(), image.Key, image.OperationID, receipt); err != nil {
				t.Fatal(err)
			}
		}
		committed, err := f.images.CommitUpload(t.Context(), image.Key, image.OperationID, model.ImageExif{Raw: json.RawMessage(`{}`)}, f.grant)
		if err != nil || committed.Ext != "webp" || committed.HasOriginal {
			t.Fatalf("commit = %+v err=%v", committed, err)
		}
	})
}

// Allowlist entries of "jpeg" or "tif" govern the same uploads as the
// canonical "jpg" and "tiff" the imaging pipeline reports: the reservation
// check must agree with the service-layer allowlist check.
func TestAllowedExtAliasesAccepted(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := newImageFixture(t, db, "ext-alias")
		if err := db.Exec("UPDATE groups SET allowed_exts = ? WHERE id = ?", `["jpeg"]`, f.user.GroupID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.request("alias-jpg", "2026/01/alias-jpg")); err != nil {
			t.Fatalf(`allowlist ["jpeg"] refused a JPG upload: %v`, err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.webpOnlyRequest("alias-tif", "2026/01/alias-tif", "tiff")); !errors.Is(err, model.ErrUnsupportedFormat) {
			t.Fatalf(`allowlist ["jpeg"] accepted a TIFF upload: %v`, err)
		}
		if err := db.Exec("UPDATE groups SET allowed_exts = ? WHERE id = ?", `["tif"]`, f.user.GroupID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.images.ReserveUpload(t.Context(), f.webpOnlyRequest("alias-tiff", "2026/01/alias-tiff", "tiff")); err != nil {
			t.Fatalf(`allowlist ["tif"] refused a TIFF upload: %v`, err)
		}
	})
}
