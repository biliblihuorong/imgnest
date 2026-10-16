package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

// minioPipeline creates a versioned, publicly readable bucket, an encrypted
// S3 storage and a rule on it, and returns the SDK client and bucket.
func minioPipeline(t *testing.T, policyJSON, folder string) (*s3.Client, string, string, string) {
	t.Helper()
	endpoint := os.Getenv("IMGNEST_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("IMGNEST_TEST_S3_ENDPOINT is not set")
	}
	ctx := t.Context()
	access := os.Getenv("IMGNEST_TEST_S3_ACCESS_KEY")
	secretKey := os.Getenv("IMGNEST_TEST_S3_SECRET_KEY")
	sdk, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(access, secretKey, "")))
	if err != nil {
		t.Fatal("test S3 configuration failed")
	}
	client := s3.NewFromConfig(sdk, func(opts *s3.Options) {
		opts.BaseEndpoint = aws.String(endpoint)
		opts.UsePathStyle = true
		opts.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		opts.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	bucket := fmt.Sprintf("imgnest-adopt-%d", time.Now().UnixNano())
	if _, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal("create isolated test bucket failed")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		versions, _ := client.ListObjectVersions(cleanup, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)})
		if versions != nil {
			for _, v := range versions.Versions {
				_, _ = client.DeleteObject(cleanup, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: v.Key, VersionId: v.VersionId})
			}
			for _, v := range versions.DeleteMarkers {
				_, _ = client.DeleteObject(cleanup, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: v.Key, VersionId: v.VersionId})
			}
		}
		if _, err := client.DeleteBucket(cleanup, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error("isolated test bucket cleanup failed")
		}
	})
	if _, err = client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: aws.String(bucket), VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}}); err != nil {
		t.Fatal("enable test versioning failed")
	}
	readable := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` + bucket + `/*"]}]}`
	if _, err = client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: aws.String(bucket), Policy: aws.String(readable)}); err != nil {
		t.Fatal("test bucket read policy failed")
	}
	t.Setenv("IMGNEST_SECURITY_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{11}, 32)))
	path := commandConfig(t)
	if _, err = command(ctx, t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	// #nosec G117 -- test-only credentials enter the encrypting CLI through stdin.
	cloud, _ := json.Marshal(cloudConfig{Endpoint: endpoint, Region: "us-east-1", Bucket: bucket, AccessKeyID: access, SecretAccessKey: secretKey, UsePathStyle: true})
	input, _ := json.Marshal(service.StorageInput{Name: "lsky", Driver: "s3", BaseURL: strings.TrimSuffix(endpoint+"/"+bucket+"/"+folder, "/"), PathPrefix: folder, Config: cloud})
	output, err := command(ctx, t, path, string(input), "init-storage")
	if err != nil {
		t.Fatal("encrypted storage provisioning failed")
	}
	var backend service.StorageView
	if json.Unmarshal([]byte(output), &backend) != nil || backend.ID == 0 {
		t.Fatal("storage setup output invalid")
	}
	if _, err = command(ctx, t, path, policyJSON, "init-policy", "--storage-id", fmt.Sprint(backend.ID), "--stdin"); err != nil {
		t.Fatal(err)
	}
	return client, bucket, endpoint, path
}

func TestRealMinIOAdoptedImageLifecycle(t *testing.T) {
	adoptedLifecycle(t, "")
}

// The Lsky files sit under a folder of the bucket; the storage's path prefix
// keeps keys and links aligned with the old ones.
func TestRealMinIOAdoptedImageLifecycleInFolder(t *testing.T) {
	adoptedLifecycle(t, "ShellBlogImg")
}

func adoptedLifecycle(t *testing.T, folder string) {
	client, bucket, endpoint, path := minioPipeline(t, `{"skip_if_larger":false,"responsive_variants":true}`, folder)
	ctx := t.Context()
	var source bytes.Buffer
	if err := jpeg.Encode(&source, image.NewRGBA(image.Rect(0, 0, 900, 600)), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	key := strings.TrimPrefix(folder+"/1/2025/07/25/lsky.jpg", "/")
	put, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(source.Bytes()), ContentType: aws.String("image/jpeg")})
	if err != nil {
		t.Fatal("put foreign original failed")
	}
	version := aws.ToString(put.VersionId)
	link := endpoint + "/" + bucket + "/" + key
	fetch := func(link string, want int) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			t.Fatal("cloud direct link failed")
		}
		body, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil || res.StatusCode != want {
			t.Fatalf("%s status=%d want %d", link, res.StatusCode, want)
		}
		return body
	}
	err = withDatabase(ctx, path, func(db *gorm.DB, cfg config.Config) error {
		users, _, err := newServices(ctx, db)
		if err != nil {
			return err
		}
		if _, err = users.InitAdmin(ctx, service.RegisterInput{Username: "lsky-owner", Email: "lsky@example.com", Password: "pipeline-test-password"}); err != nil {
			return err
		}
		actor, err := users.VerifyCredentials(ctx, "lsky@example.com", "pipeline-test-password")
		if err != nil {
			return err
		}
		var policy model.Policy
		if err = db.First(&policy).Error; err != nil {
			return err
		}
		images, _, closeImages, err := newImageServices(ctx, db, cfg, nil, imageHooks{})
		if err != nil {
			return err
		}
		defer func() { _ = closeImages() }()
		input := service.ImportInput{Data: source.Bytes(), Filename: "lsky.jpg", Path: "1/2025/07/25/lsky", Ext: "jpg", UserID: actor.User.ID, PolicyID: policy.ID, IsPublic: true}
		stale := input
		stale.AdoptVersion = "not-the-version"
		if _, err := images.ImportImage(ctx, stale); err == nil {
			t.Fatal("adopting an unread version succeeded")
		}
		input.AdoptVersion = version
		view, err := images.ImportImage(ctx, input)
		if err != nil {
			return err
		}
		if view.Links.Original != link || !view.HasWebP || !view.HasThumb || len(view.Variants) == 0 || view.Scrubbed {
			t.Fatalf("adopted view = %+v", view)
		}
		head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		if err != nil || aws.ToString(head.VersionId) != version || len(head.Metadata) != 0 {
			t.Fatal("adoption rewrote the original")
		}
		if !bytes.Equal(fetch(link, 200), source.Bytes()) {
			t.Fatal("adopted original changed")
		}
		fetch(view.Links.WebP, 200)
		fetch(view.Links.Thumbnail, 200)
		if err = images.Trash(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		fetch(link, 404)
		kept, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), VersionId: aws.String(version)})
		if err != nil {
			t.Fatal("trash removed the adopted version")
		}
		_ = kept.Body.Close()
		if err = images.Restore(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		restored := fetch(link, 200)
		if sha256.Sum256(restored) != sha256.Sum256(source.Bytes()) {
			t.Fatal("restored original differs")
		}
		// A second round trips through the owned copy the restore wrote.
		if err = images.Trash(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		fetch(link, 404)
		if err = images.Purge(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		versions, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)})
		if err != nil {
			return service.ErrStorage
		}
		if len(versions.Versions) != 0 || len(versions.DeleteMarkers) != 0 {
			t.Fatalf("purge left %d versions and %d markers", len(versions.Versions), len(versions.DeleteMarkers))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRealMinIOAdoptFailureKeepsOriginal(t *testing.T) {
	client, bucket, _, path := minioPipeline(t, `{"skip_if_larger":false}`, "")
	ctx := t.Context()
	var source bytes.Buffer
	if err := jpeg.Encode(&source, image.NewRGBA(image.Rect(0, 0, 64, 32)), nil); err != nil {
		t.Fatal(err)
	}
	put, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String("a/b.jpg"), Body: bytes.NewReader(source.Bytes())})
	if err != nil {
		t.Fatal("put foreign original failed")
	}
	// A foreign object already at the WebP key makes the adoption fail after
	// its first write; compensation must not touch the original.
	if _, err = client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String("a/b_thumbs.webp"), Body: bytes.NewReader([]byte("x"))}); err != nil {
		t.Fatal("put foreign thumbnail failed")
	}
	err = withDatabase(ctx, path, func(db *gorm.DB, cfg config.Config) error {
		users, _, err := newServices(ctx, db)
		if err != nil {
			return err
		}
		admin, err := users.InitAdmin(ctx, service.RegisterInput{Username: "owner", Email: "o@example.com", Password: "pipeline-test-password"})
		if err != nil {
			return err
		}
		var policy model.Policy
		if err = db.First(&policy).Error; err != nil {
			return err
		}
		images, _, closeImages, err := newImageServices(ctx, db, cfg, nil, imageHooks{})
		if err != nil {
			return err
		}
		defer func() { _ = closeImages() }()
		_, err = images.ImportImage(ctx, service.ImportInput{Data: source.Bytes(), Filename: "b.jpg", Path: "a/b", Ext: "jpg", UserID: admin.ID, PolicyID: policy.ID, AdoptVersion: aws.ToString(put.VersionId)})
		if err == nil {
			t.Fatal("adoption over a foreign thumbnail succeeded")
		}
		head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("a/b.jpg")})
		if err != nil || aws.ToString(head.VersionId) != aws.ToString(put.VersionId) {
			t.Fatal("failed adoption touched the original")
		}
		if _, err = client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String("a/b.webp")}); err == nil {
			t.Fatal("failed adoption left its WebP behind")
		}
		var count int64
		db.Model(&model.Image{}).Where("state <> ?", model.ImageStatePending).Count(&count)
		if count != 0 {
			t.Fatal("failed adoption left an image row")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
