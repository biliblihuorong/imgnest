package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// putForeign writes an object the way Lsky Pro does: no ImgNest metadata.
func putForeign(t *testing.T, d *S3, key, content string) string {
	t.Helper()
	output, err := d.client.PutObject(t.Context(), &s3.PutObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), Body: strings.NewReader(content), ContentType: aws.String("image/png")})
	if err != nil {
		t.Fatal(s3Error("put foreign object", err))
	}
	if aws.ToString(output.VersionId) == "" {
		t.Fatal("test bucket is not versioned")
	}
	return aws.ToString(output.VersionId)
}

func TestS3MinIOLegacyLifecycle(t *testing.T) {
	legacyLifecycle(t, minioFixture(t))
}

func TestS3B2LikeLegacyLifecycle(t *testing.T) {
	legacyLifecycle(t, b2Fixture(t, true))
}

func legacyLifecycle(t *testing.T, d *S3) {
	ctx := t.Context()
	const key, trash, owner, content = "1/2025/07/abc.png", "_trash/1/2025/07/abc.png", "image-1", "lsky original"
	sum := sha256.Sum256([]byte(content))
	digest := hex.EncodeToString(sum[:])
	version := putForeign(t, d, key, content)
	opts := CopyOptions{OwnerID: owner, MIME: "image/png", CacheControl: "private, no-store"}

	if _, err := d.CopyLegacy(ctx, key, "", trash, digest, opts); !errors.Is(err, ErrOwnership) {
		t.Fatalf("unpinned copy = %v, want ownership error", err)
	}
	if _, err := d.CopyLegacy(ctx, key, version, trash, digest, opts); err != nil {
		t.Fatal(err)
	}
	// A retry reuses the identical trash copy.
	if _, err := d.CopyLegacy(ctx, key, version, trash, digest, opts); err != nil {
		t.Fatalf("retried legacy copy: %v", err)
	}
	info, err := d.Stat(ctx, trash)
	if err != nil || info.OwnerID != owner {
		t.Fatalf("trash copy = %+v, %v", info, err)
	}
	if err := d.DeleteLegacy(ctx, key, "other"); !errors.Is(err, ErrOwnership) {
		t.Fatalf("delete with wrong version = %v", err)
	}
	if err := d.DeleteLegacy(ctx, key, version); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Stat(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("live key after legacy delete = %v", err)
	}
	// The adopted version is still in the bucket behind the delete marker.
	kept, err := d.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), VersionId: aws.String(version)})
	if err != nil {
		t.Fatal(s3Error("read adopted version", err))
	}
	body, _ := io.ReadAll(kept.Body)
	_ = kept.Body.Close()
	if string(body) != content {
		t.Fatalf("adopted version = %q", body)
	}
	// Restoring writes an owned copy back to the live key.
	if _, err := d.Copy(ctx, trash, key, CopyOptions{OwnerID: owner, MIME: "image/png"}); err != nil {
		t.Fatalf("restore owned copy: %v", err)
	}
	if err := d.PurgeImage(ctx, key, owner); !errors.Is(err, ErrOwnership) {
		t.Fatalf("plain purge of adopted history = %v, want ownership error", err)
	}
	if err := d.PurgeLegacy(ctx, key, owner, "other"); !errors.Is(err, ErrOwnership) {
		t.Fatalf("legacy purge with wrong version = %v", err)
	}
	if err := d.PurgeLegacy(ctx, key, owner, version); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeImage(ctx, trash, owner); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{key, trash} {
		versions, err := d.listVersions(ctx, k)
		if err != nil || len(versions) != 0 {
			t.Fatalf("%s history after purge = %v, %v", k, versions, err)
		}
	}
}

func TestS3MinIOLegacyRefusesOwnedOrForeignVersions(t *testing.T) {
	d := minioFixture(t)
	ctx := t.Context()
	const key = "2/x.png"
	sum := sha256.Sum256([]byte("a"))
	digest := hex.EncodeToString(sum[:])
	if _, err := d.PutNew(ctx, key, strings.NewReader("a"), PutOptions{OwnerID: "someone", MIME: "image/png"}); err != nil {
		t.Fatal(err)
	}
	owned, err := d.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CopyLegacy(ctx, key, owned.VersionID, "_trash/"+key, digest, CopyOptions{OwnerID: "image-2"}); !errors.Is(err, ErrOwnership) {
		t.Fatalf("legacy copy of an owned object = %v", err)
	}
	if err := d.DeleteLegacy(ctx, key, owned.VersionID); !errors.Is(err, ErrOwnership) {
		t.Fatalf("legacy delete of an owned object = %v", err)
	}
	// A second foreign version written after adoption is never purged.
	adopted := putForeign(t, d, "3/y.png", "first")
	putForeign(t, d, "3/y.png", "second")
	if err := d.PurgeLegacy(ctx, "3/y.png", "image-3", adopted); !errors.Is(err, ErrOwnership) {
		t.Fatalf("legacy purge over a newer foreign version = %v", err)
	}
	if err := d.DeleteLegacy(ctx, "3/y.png", adopted); !errors.Is(err, ErrOwnership) {
		t.Fatalf("legacy delete of a replaced object = %v", err)
	}
}

func TestS3MinIOPrefixedLegacyLifecycle(t *testing.T) {
	d := minioFixture(t)
	ctx := t.Context()
	const content = "lsky original under a folder"
	sum := sha256.Sum256([]byte(content))
	version := putForeign(t, d, "blog/1/a.png", content)
	wrapped, err := WithPrefix(d, "blog")
	if err != nil {
		t.Fatal(err)
	}
	legacy, ok := wrapped.(LegacyObjects)
	if !ok {
		t.Fatal("prefixed S3 lost legacy support")
	}
	body, info, err := wrapped.Open(ctx, "1/a.png")
	if err != nil || info.VersionID != version || info.OwnerID != "" {
		t.Fatalf("open = %+v, %v", info, err)
	}
	_ = body.Close()
	opts := CopyOptions{OwnerID: "image-1", MIME: "image/png", CacheControl: "private, no-store"}
	if _, err := legacy.CopyLegacy(ctx, "1/a.png", version, "_trash/1/a.png", hex.EncodeToString(sum[:]), opts); err != nil {
		t.Fatal(err)
	}
	if info, err := d.Stat(ctx, "blog/_trash/1/a.png"); err != nil || info.OwnerID != "image-1" {
		t.Fatalf("trash copy outside folder: %+v, %v", info, err)
	}
	if err := legacy.DeleteLegacy(ctx, "1/a.png", version); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Stat(ctx, "blog/1/a.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("live key after delete: %v", err)
	}
	if err := legacy.PurgeLegacy(ctx, "1/a.png", "image-1", version); err != nil {
		t.Fatal(err)
	}
	if err := wrapped.(ImagePurger).PurgeImage(ctx, "_trash/1/a.png", "image-1"); err != nil {
		t.Fatal(err)
	}
}
