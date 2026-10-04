package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func minioFixture(t *testing.T) *S3 {
	t.Helper()
	endpoint := os.Getenv("IMGNEST_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("IMGNEST_TEST_S3_ENDPOINT is not set")
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	bucket := "imgnest-storage-" + hex.EncodeToString(random[:])
	d, err := NewS3(t.Context(), S3Config{Endpoint: endpoint, Region: "us-east-1", Bucket: bucket, AccessKeyID: os.Getenv("IMGNEST_TEST_S3_ACCESS_KEY"), SecretAccessKey: os.Getenv("IMGNEST_TEST_S3_SECRET_KEY"), UsePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.client.CreateBucket(t.Context(), &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal(s3Error("create isolated test bucket", err))
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		versions, err := d.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)})
		if err != nil {
			t.Error(s3Error("list test cleanup versions", err))
			return
		}
		keys := map[string]bool{}
		for _, v := range versions.Versions {
			keys[aws.ToString(v.Key)] = true
		}
		for _, v := range versions.DeleteMarkers {
			keys[aws.ToString(v.Key)] = true
		}
		for key := range keys {
			if err := d.purge(ctx, key); err != nil {
				t.Error(err)
			}
		}
		if _, err := d.client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error(s3Error("delete isolated test bucket", err))
		}
	})
	if _, err := d.client.PutBucketVersioning(t.Context(), &s3.PutBucketVersioningInput{Bucket: aws.String(bucket), VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}}); err != nil {
		t.Fatal(s3Error("enable test versions", err))
	}
	return d
}

func TestS3MinIOVersionedLifecycle(t *testing.T) {
	d := minioFixture(t)
	if err := d.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	opts := PutOptions{OwnerID: "image-one", MIME: "image/png"}
	key := "2026/旅行 #.png"
	first, err := d.PutNew(t.Context(), key, strings.NewReader("original-bytes"), opts)
	if err != nil || first.VersionID == "" {
		t.Fatalf("put receipt %+v %v", first, err)
	}
	if _, err := d.PutNew(t.Context(), key, strings.NewReader("must-not-overwrite"), opts); !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}
	if readObject(t, d, key) != "original-bytes" {
		t.Fatal("conditional put overwrote")
	}
	for range 2 {
		if _, err := d.Copy(t.Context(), key, "_trash/"+key, opts); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Copy(t.Context(), key, "_trash/"+key, CopyOptions{OwnerID: "other"}); !errors.Is(err, ErrOwnership) {
		t.Fatal(err)
	}
	if err := d.DeleteCurrent(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Open(t.Context(), key); !errors.Is(err, ErrNotFound) {
		t.Fatal("old URL remained accessible")
	}
	if _, err := d.Copy(t.Context(), "_trash/"+key, key, opts); err != nil {
		t.Fatal(err)
	}
	if readObject(t, d, key) != "original-bytes" {
		t.Fatal("restore changed content")
	}
	versions, err := d.listVersions(t.Context(), key)
	if err != nil || len(versions) < 3 {
		t.Fatalf("expected history plus marker, versions=%d err=%v", len(versions), err)
	}
	neighbor := key + "-neighbor"
	if _, err := d.PutNew(t.Context(), neighbor, strings.NewReader("neighbor"), opts); err != nil {
		t.Fatal(err)
	}
	for _, purgeKey := range []string{key, "_trash/" + key} {
		for range 2 {
			if err := d.PurgeAllVersions(t.Context(), purgeKey); err != nil {
				t.Fatal(err)
			}
		}
		remaining, err := d.listVersions(t.Context(), purgeKey)
		if err != nil || len(remaining) != 0 {
			t.Fatalf("purge retained versions %d %v", len(remaining), err)
		}
	}
	if readObject(t, d, neighbor) != "neighbor" {
		t.Fatal("purge deleted neighbor")
	}
}

func TestS3MinIOOwnedCompensationKeepsExternalHistory(t *testing.T) {
	d := minioFixture(t)
	key := "history.jpg"
	foreign, err := d.client.PutObject(t.Context(), &s3.PutObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), Body: strings.NewReader("external"), ContentLength: aws.Int64(8)})
	if err != nil {
		t.Fatal(s3Error("create foreign test version", err))
	}
	marker, err := d.client.DeleteObject(t.Context(), &s3.DeleteObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)})
	if err != nil {
		t.Fatal(s3Error("hide foreign test version", err))
	}
	if _, err := d.PutNew(t.Context(), key, strings.NewReader("owned"), PutOptions{OwnerID: "image-one"}); err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeOwned(t.Context(), key, "image-one"); err != nil {
		t.Fatal(err)
	}
	remaining, err := d.listVersions(t.Context(), key)
	if err != nil || len(remaining) != 2 {
		t.Fatalf("foreign history changed %v %v", remaining, err)
	}
	ids := map[string]bool{}
	for _, version := range remaining {
		ids[version.id] = true
	}
	if !ids[aws.ToString(foreign.VersionId)] || !ids[aws.ToString(marker.VersionId)] {
		t.Fatal("compensation removed external history")
	}
	if _, err := d.Stat(t.Context(), key); !errors.Is(err, ErrNotFound) {
		t.Fatal("compensation revealed hidden external object")
	}
}

func TestS3MinIOPurgeImagePreservesForeignHistory(t *testing.T) {
	d := minioFixture(t)
	key := "external-history.jpg"
	if _, err := d.client.PutObject(t.Context(), &s3.PutObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), Body: strings.NewReader("external"), ContentLength: aws.Int64(8)}); err != nil {
		t.Fatal(s3Error("seed external history", err))
	}
	if _, err := d.client.DeleteObject(t.Context(), &s3.DeleteObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)}); err != nil {
		t.Fatal(s3Error("hide external history", err))
	}
	if _, err := d.PutNew(t.Context(), key, strings.NewReader("owned"), PutOptions{OwnerID: "one"}); err != nil {
		t.Fatal(err)
	}
	before, err := d.listVersions(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.PurgeImage(t.Context(), key, "one"); !errors.Is(err, ErrOwnership) {
		t.Fatalf("foreign history allowed final purge: %v", err)
	}
	after, err := d.listVersions(t.Context(), key)
	if err != nil || len(before) != len(after) {
		t.Fatal("refused final purge deleted history")
	}
	if readObject(t, d, key) != "owned" {
		t.Fatal("ownership verification was not completed before deletions")
	}
}

func TestS3MinIOPurgeImageRemovesOwnedVersionsAndMarkers(t *testing.T) {
	d := minioFixture(t)
	key := "owned-history.jpg"
	opts := PutOptions{OwnerID: "one"}
	for range 2 {
		if _, err := d.PutNew(t.Context(), key, strings.NewReader("owned"), opts); err != nil {
			t.Fatal(err)
		}
		if err := d.DeleteCurrent(t.Context(), key); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := d.listVersions(t.Context(), key)
	if err != nil || len(versions) != 4 {
		t.Fatalf("versioned fixture %v %v", versions, err)
	}
	for range 2 {
		if err := d.PurgeImage(t.Context(), key, "one"); err != nil {
			t.Fatal(err)
		}
	}
	versions, err = d.listVersions(t.Context(), key)
	if err != nil || len(versions) != 0 {
		t.Fatalf("owned final purge retained history %v %v", versions, err)
	}
}
