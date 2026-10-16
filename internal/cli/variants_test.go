package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/biliblihuorong/imgnest/internal/storage"
	"gorm.io/gorm"
)

func noiseImage(w, h int, alpha bool) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			// Every channel is reduced modulo 256 first, so the conversions are exact.
			a := uint8(255)
			if alpha {
				a = uint8(40 + (x*7+y)%200) //nolint:gosec // < 240
			}
			im.SetNRGBA(x, y, color.NRGBA{R: uint8((x * 13) % 256), G: uint8((y * 7) % 256), B: uint8((x ^ y) % 256), A: a}) //nolint:gosec // reduced modulo 256
		}
	}
	return im
}

func encodedJPEG(t *testing.T, w, h int, orientation byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, noiseImage(w, h, false), &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	if orientation == 1 {
		return out.Bytes()
	}
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, orientation, 0, 0, 0, 0, 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := append([]byte{0xff, 0xe1, 0, byte(len(payload) + 2)}, payload...) //nolint:gosec // a 32-byte fixed payload
	result := append([]byte{}, out.Bytes()[:2]...)
	result = append(result, segment...)
	return append(result, out.Bytes()[2:]...)
}

func encodedPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, noiseImage(w, h, true)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func encodedGIF(t *testing.T, w, h int) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, w, h), palette)
	second := image.NewPaletted(first.Bounds(), palette)
	second.SetColorIndex(0, 0, 1)
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{5, 5}}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestResponsiveVariantsLocalLifecycle drives the real pipeline (SQLite,
// libvips, local storage): generation per shape, serving, lookup, quota,
// recycle bin, purge, backfill and journal recovery.
func TestResponsiveVariantsLocalLifecycle(t *testing.T) {
	ctx := t.Context()
	path := commandConfig(t)
	if _, err := command(ctx, t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "objects")
	out, err := command(ctx, t, path, "", "init-local", "--root", root, "--base-url", "http://img.test")
	if err != nil {
		t.Fatal(err)
	}
	var setup struct {
		Storage  service.StorageView `json:"storage"`
		PolicyID uint64              `json:"policy_id"`
	}
	if err = json.Unmarshal([]byte(out), &setup); err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("http://img.test/i/%d/", setup.Storage.ID)
	local, err := storage.NewLocal(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })
	probe, err := imaging.NewProcessor(ctx, 100_000_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = withDatabase(ctx, path, func(db *gorm.DB, cfg config.Config) error {
		users, _, err := newServices(ctx, db)
		if err != nil {
			return err
		}
		if _, err = users.InitAdmin(ctx, service.RegisterInput{Username: "variant-owner", Email: "variants@example.com", Password: "variant-test-password"}); err != nil {
			return err
		}
		actor, err := users.VerifyCredentials(ctx, "variants@example.com", "variant-test-password")
		if err != nil {
			return err
		}
		images, _, closeImages, err := newImageServices(ctx, db, cfg, nil, imageHooks{})
		if err != nil {
			return err
		}
		defer func() { _ = closeImages() }()
		used := func() int64 {
			var user model.User
			if err := db.First(&user, "id = ?", actor.User.ID).Error; err != nil {
				t.Fatal(err)
			}
			return user.UsedBytes
		}
		upload := func(data []byte, name string) service.ImageView {
			t.Helper()
			view, err := images.Upload(ctx, actor.Subject, service.UploadInput{Data: data, Filename: name})
			if err != nil {
				t.Fatalf("upload %s: %v", name, err)
			}
			return view
		}
		// Uploaded before the rule enables variants: left for the backfill.
		legacy := upload(encodedJPEG(t, 1800, 900, 1), "legacy.jpg")
		if legacy.Variants == nil || len(legacy.Variants) != 0 {
			t.Fatal("variants generated while the rule is off")
		}
		if err = db.Model(&model.Policy{}).Where("id = ?", setup.PolicyID).Update("responsive_variants", true).Error; err != nil {
			return err
		}

		check := func(view service.ImageView, want [][2]int) {
			t.Helper()
			if len(view.Variants) != len(want) {
				t.Fatalf("%s: %d variants, want %d", view.Name, len(view.Variants), len(want))
			}
			for i, variant := range view.Variants {
				if variant.Width != want[i][0] || variant.Height != want[i][1] || variant.MIME != "image/webp" || !strings.HasPrefix(variant.URL, base) {
					t.Fatalf("%s variant %d = %+v, want %v", view.Name, i, variant, want[i])
				}
				object, err := images.OpenPublic(ctx, view.StorageID, strings.TrimPrefix(variant.URL, base))
				if err != nil {
					t.Fatalf("%s variant %d not served: %v", view.Name, i, err)
				}
				data, err := io.ReadAll(object.Body)
				_ = object.Body.Close()
				if err != nil || int64(len(data)) != variant.Bytes || object.MIME != "image/webp" {
					t.Fatalf("%s variant %d served %d bytes, declared %d", view.Name, i, len(data), variant.Bytes)
				}
				info, err := probe.Probe(ctx, data)
				if err != nil || info.Width != variant.Width || info.Height != variant.Height || info.LoadedFrames != 1 {
					t.Fatalf("%s variant %d encodes %+v, declared %dx%d", view.Name, i, info, variant.Width, variant.Height)
				}
			}
		}
		landscape := upload(encodedJPEG(t, 2000, 1000, 1), "landscape.jpg")
		check(landscape, [][2]int{{400, 200}, {800, 400}, {1600, 800}})
		portrait := upload(encodedJPEG(t, 1200, 600, 6), "rotated.jpg")
		check(portrait, [][2]int{{400, 800}})
		small := upload(encodedJPEG(t, 300, 200, 1), "small.jpg")
		check(small, nil)
		transparent := upload(encodedPNG(t, 1000, 500), "alpha.png")
		check(transparent, [][2]int{{400, 200}, {800, 400}})
		animated := upload(encodedGIF(t, 900, 600), "animated.gif")
		check(animated, nil)
		if animated.Frames != 2 {
			t.Fatal("animation not preserved")
		}
		total := legacy.ChargedBytes + landscape.ChargedBytes + portrait.ChargedBytes + small.ChargedBytes + transparent.ChargedBytes + animated.ChargedBytes
		if used() != total {
			t.Fatalf("used %d bytes, charged %d", used(), total)
		}
		var variantBytes int64
		for _, variant := range landscape.Variants {
			variantBytes += variant.Bytes
		}
		if landscape.ChargedBytes <= variantBytes {
			t.Fatal("variant bytes not part of the image charge")
		}

		items, err := images.LookupVariants(ctx, []string{landscape.Links.Original, landscape.Links.WebP, landscape.Variants[0].URL, small.Links.Original, "https://img.test/i/99/x.jpg"})
		if err != nil {
			return err
		}
		if !items[0].Found || len(items[0].Variants) != 3 || !items[1].Found || !items[2].Found || !items[3].Found || len(items[3].Variants) != 0 || items[4].Found {
			t.Fatalf("lookup = %+v", items)
		}

		// Recycle bin: every variant leaves with the image and comes back.
		keys := []string{}
		for _, variant := range landscape.Variants {
			keys = append(keys, strings.TrimPrefix(variant.URL, base))
		}
		if err = images.Trash(ctx, actor.Subject, landscape.Key); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := images.OpenPublic(ctx, landscape.StorageID, key); !errors.Is(err, service.ErrNotFound) {
				t.Fatalf("recycled variant %s still served: %v", key, err)
			}
			if _, err := local.Stat(ctx, ".trash/"+key); err != nil {
				t.Fatalf("variant %s not moved to the recycle bin: %v", key, err)
			}
		}
		if items, err = images.LookupVariants(ctx, []string{landscape.Links.Original}); err != nil || items[0].Found {
			t.Fatal("recycled image still resolves")
		}
		if err = images.Restore(ctx, actor.Subject, landscape.Key); err != nil {
			return err
		}
		restored, err := images.Get(ctx, actor.Subject, landscape.ID)
		if err != nil {
			return err
		}
		check(restored, [][2]int{{400, 200}, {800, 400}, {1600, 800}})
		if err = images.Trash(ctx, actor.Subject, landscape.Key); err != nil {
			return err
		}
		if err = images.Purge(ctx, actor.Subject, landscape.Key); err != nil {
			return err
		}
		for _, key := range keys {
			for _, candidate := range []string{key, ".trash/" + key} {
				if _, err := local.Stat(ctx, candidate); !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("purge left %s: %v", candidate, err)
				}
			}
		}

		// Backfill: resumable, counted against quota, idempotent.
		before := used()
		if err = images.StartBackground(ctx); err != nil {
			return err
		}
		defer images.WaitBackground()
		if _, err = images.StartVariantBackfill(ctx, actor.Subject, false); err != nil {
			return err
		}
		deadline := time.Now().Add(30 * time.Second)
		var state service.VariantBackfillView
		for {
			state, err = images.VariantBackfillStatus(ctx, actor.Subject)
			if err != nil {
				return err
			}
			if state.Status == model.BackfillDone && !state.Active {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("backfill did not finish: %+v", state)
			}
			time.Sleep(100 * time.Millisecond)
		}
		if state.Generated != 1 || state.Failed != 0 || state.Processed != 5 {
			t.Fatalf("backfill counters = %+v", state)
		}
		filled, err := images.Get(ctx, actor.Subject, legacy.ID)
		if err != nil {
			return err
		}
		check(filled, [][2]int{{400, 200}, {800, 400}, {1600, 800}})
		var added int64
		for _, variant := range filled.Variants {
			added += variant.Bytes
		}
		if used() != before+added || filled.ChargedBytes != legacy.ChargedBytes+added {
			t.Fatalf("backfill quota: used %d -> %d, added %d", before, used(), added)
		}

		// A generation interrupted after writing is compensated by recovery.
		images2, err := repo.NewImageRepository(ctx, db)
		if err != nil {
			return err
		}
		row, err := images2.FindByKey(ctx, small.Key)
		if err != nil {
			return err
		}
		orphan := model.VariantKey(row.Path, 400)
		job := model.VariantJob{ImageKey: row.Key, OperationID: "crashed", Variants: model.VariantList{{Key: orphan, Width: 400, Height: 1, Bytes: 4, MIME: model.VariantMIME}},
			Objects: []model.ObjectReceipt{{Key: orphan, OwnerID: row.Key, Location: model.ObjectLocationCloud, MIME: model.VariantMIME, Size: 4, SHA256: strings.Repeat("ab", 32)}}}
		if _, err = images2.BeginVariants(ctx, job); err != nil {
			return err
		}
		if _, err = local.PutNew(ctx, orphan, bytes.NewReader([]byte("half")), storage.PutOptions{MIME: model.VariantMIME, OwnerID: row.Key}); err != nil {
			return err
		}
		if err = images.Trash(ctx, actor.Subject, small.Key); !errors.Is(err, service.ErrImageBusy) {
			t.Fatalf("image with an open variant journal recycled: %v", err)
		}
		if err = images.Recover(ctx); err != nil {
			return err
		}
		if _, err = local.Stat(ctx, orphan); !errors.Is(err, storage.ErrNotFound) {
			t.Fatal("recovery left the interrupted variant")
		}
		after, err := images2.FindByKey(ctx, small.Key)
		if err != nil || after.ChargedBytes != row.ChargedBytes || len(after.Variants) != 0 {
			t.Fatalf("recovery did not release the reservation: %+v", after)
		}
		if used() != before+added {
			t.Fatal("recovery changed unrelated usage")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRealMinIOResponsiveVariants checks that variants are public static
// objects on S3 and that recycle and purge remove every version of them.
func TestRealMinIOResponsiveVariants(t *testing.T) {
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
	bucket := fmt.Sprintf("imgnest-variants-%d", time.Now().UnixNano())
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
	t.Setenv("IMGNEST_SECURITY_MASTER_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{12}, 32)))
	path := commandConfig(t)
	if _, err = command(ctx, t, path, "", "migrate"); err != nil {
		t.Fatal(err)
	}
	// #nosec G117 -- test-only credentials enter the encrypting CLI through stdin.
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
	if _, err = command(ctx, t, path, `{"responsive_variants":true}`, "init-policy", "--storage-id", fmt.Sprint(backend.ID), "--stdin", "--default"); err != nil {
		t.Fatal(err)
	}
	err = withDatabase(ctx, path, func(db *gorm.DB, cfg config.Config) error {
		users, _, err := newServices(ctx, db)
		if err != nil {
			return err
		}
		if _, err = users.InitAdmin(ctx, service.RegisterInput{Username: "minio-variants", Email: "minio-variants@example.com", Password: "pipeline-test-password"}); err != nil {
			return err
		}
		actor, err := users.VerifyCredentials(ctx, "minio-variants@example.com", "pipeline-test-password")
		if err != nil {
			return err
		}
		images, _, closeImages, err := newImageServices(ctx, db, cfg, nil, imageHooks{})
		if err != nil {
			return err
		}
		defer func() { _ = closeImages() }()
		view, err := images.Upload(ctx, actor.Subject, service.UploadInput{Data: encodedJPEG(t, 1700, 850, 1), Filename: "wide.jpg"})
		if err != nil {
			return err
		}
		if len(view.Variants) != 3 || view.Variants[2].Width != 1600 || view.Variants[2].Height != 800 {
			t.Fatalf("variants = %+v", view.Variants)
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
				t.Fatalf("cloud status=%d want %d", res.StatusCode, want)
			}
			if want == 200 && !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
				t.Fatal("variant is not published as an immutable object")
			}
			return size
		}
		actual := fetch(view.Links.Original, 200) + fetch(view.Links.WebP, 200) + fetch(view.Links.Thumbnail, 200)
		for _, variant := range view.Variants {
			if size := fetch(variant.URL, 200); size != variant.Bytes {
				t.Fatalf("variant serves %d bytes, declared %d", size, variant.Bytes)
			}
			actual += variant.Bytes
		}
		if actual != view.ChargedBytes {
			t.Fatal("quota does not equal real cloud bytes")
		}
		if err = images.Trash(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		for _, variant := range view.Variants {
			fetch(variant.URL, 404)
		}
		if err = images.Restore(ctx, actor.Subject, view.Key); err != nil {
			return err
		}
		for _, variant := range view.Variants {
			fetch(variant.URL, 200)
		}
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
			t.Fatal("physical purge left variant versions or markers")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
