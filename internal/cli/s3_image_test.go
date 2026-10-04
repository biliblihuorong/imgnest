package cli

import (
	"bytes"
	"context"
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

func TestRealMinIOEncryptedImageLifecycle(t *testing.T) {
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
	bucket := fmt.Sprintf("imgnest-pipeline-%d", time.Now().UnixNano())
	if _, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatal("create isolated test bucket failed")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if _, err := client.DeleteBucket(cleanup, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error("isolated test bucket cleanup failed")
		}
	})
	if _, err = client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: aws.String(bucket), VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}}); err != nil {
		t.Fatal("enable test versioning failed")
	}
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::` + bucket + `/*"]}]}`
	if _, err = client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: aws.String(bucket), Policy: aws.String(policy)}); err != nil {
		t.Fatal("test bucket read policy failed")
	}
	t.Setenv("IMGNEST_SECURITY_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{11}, 32)))
	path := commandConfig(t)
	if _, err = command(ctx, t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	// #nosec G117 -- test-only credentials enter the encrypting CLI through stdin; plaintext persistence/output are checked below.
	cloud, _ := json.Marshal(cloudConfig{Endpoint: endpoint, Region: "us-east-1", Bucket: bucket, AccessKeyID: access, SecretAccessKey: secretKey, UsePathStyle: true})
	input, _ := json.Marshal(service.StorageInput{Name: "minio", Driver: "s3", BaseURL: endpoint + "/" + bucket, Config: cloud})
	output, err := command(ctx, t, path, string(input), "init-storage")
	if err != nil {
		t.Fatal("encrypted storage provisioning failed")
	}
	var backend service.StorageView
	if json.Unmarshal([]byte(output), &backend) != nil || backend.ID == 0 {
		t.Fatal("storage setup output invalid")
	}
	if strings.Contains(output, secretKey) {
		t.Fatal("storage setup emitted credentials")
	}
	if _, err = command(ctx, t, path, `{"skip_if_larger":false}`, "init-policy", "--storage-id", fmt.Sprint(backend.ID), "--stdin"); err != nil {
		t.Fatal(err)
	}
	err = withDatabase(ctx, path, func(db *gorm.DB, cfg config.Config) error {
		var stored model.Storage
		if err = db.First(&stored, "id = ?", backend.ID).Error; err != nil {
			return err
		}
		if strings.Contains(string(stored.Config), secretKey) {
			t.Fatal("cloud secret persisted plaintext")
		}
		users, _, err := newServices(ctx, db)
		if err != nil {
			return err
		}
		if _, err = users.InitAdmin(ctx, service.RegisterInput{Username: "minio-owner", Email: "minio@example.com", Password: "pipeline-test-password"}); err != nil {
			return err
		}
		actor, err := users.VerifyCredentials(ctx, "minio@example.com", "pipeline-test-password")
		if err != nil {
			return err
		}
		images, closeImages, err := newImageServices(ctx, db, cfg)
		if err != nil {
			return err
		}
		defer func() { _ = closeImages() }()
		var source bytes.Buffer
		if err = jpeg.Encode(&source, image.NewRGBA(image.Rect(0, 0, 64, 32)), &jpeg.Options{Quality: 90}); err != nil {
			return err
		}
		view, err := images.Upload(ctx, actor.Subject, service.UploadInput{Data: source.Bytes(), Filename: "pretend.png"})
		if err != nil {
			return err
		}
		if !view.HasOriginal || !view.HasWebP || !view.HasThumb {
			t.Fatal("three cloud objects not generated")
		}
		fetch := func(link string, want int) int64 {
			t.Helper()
			req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
			if err != nil {
				t.Fatal("cloud direct link failed")
			}
			size, readErr := io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if readErr != nil || res.StatusCode != want {
				t.Fatalf("cloud status=%d want%d", res.StatusCode, want)
			}
			return size
		}
		actual := fetch(view.Links.Original, 200) + fetch(view.Links.WebP, 200) + fetch(view.Links.Thumbnail, 200)
		if actual != view.ChargedBytes {
			t.Fatal("quota does not equal real cloud bytes")
		}
		if err = images.Trash(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		fetch(view.Links.Original, 404)
		if err = images.Restore(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		fetch(view.Links.Original, 200)
		if err = images.Trash(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		if err = images.Purge(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		versions, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)})
		if err != nil {
			return service.ErrStorage
		}
		if len(versions.Versions) != 0 || len(versions.DeleteMarkers) != 0 {
			t.Fatal("physical purge left billed versions or markers")
		}
		var user model.User
		if err = db.First(&user, "id = ?", actor.User.ID).Error; err != nil {
			return err
		}
		if user.UsedBytes != 0 {
			t.Fatal("recycled/purged image still consumes user capacity")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
