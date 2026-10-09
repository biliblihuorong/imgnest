package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Config configures one S3-compatible bucket without logging its credentials.
type S3Config struct {
	Endpoint, Region, Bucket, AccessKeyID, SecretAccessKey, SessionToken string
	UsePathStyle                                                         bool
}

// Network bounds for one S3 request. The SDK default has no response-header
// timeout, so an endpoint that accepts the connection but never answers would
// hold an upload, a preview or a connection test until the caller gives up.
var (
	s3DialTimeout           = 10 * time.Second
	s3ResponseHeaderTimeout = 60 * time.Second
)

// S3 provides conditional owned objects in an S3-compatible bucket.
type S3 struct {
	client                    *s3.Client
	bucket                    string
	probeGate                 chan struct{}
	putVerified, copyVerified bool
}

// NewS3 constructs the pinned SDK adapter. Capabilities are checked before first writes.
func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Bucket == "" || strings.ContainsAny(cfg.Bucket, "/\\\r\n") || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("invalid S3 configuration")
	}
	if cfg.Endpoint != "" {
		endpoint, err := url.Parse(cfg.Endpoint)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return nil, fmt.Errorf("invalid S3 endpoint")
		}
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	sdk, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)), awsconfig.WithRetryMaxAttempts(1), awsconfig.WithHTTPClient(s3HTTPClient()))
	if err != nil {
		return nil, safeError("configure S3 client", err)
	}
	client := s3.NewFromConfig(sdk, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	return &S3{client: client, bucket: cfg.Bucket, probeGate: make(chan struct{}, 1)}, nil
}

func s3HTTPClient() *awshttp.BuildableClient {
	return awshttp.NewBuildableClient().
		WithDialerOptions(func(d *net.Dialer) { d.Timeout = s3DialTimeout }).
		WithTransportOptions(func(t *http.Transport) { t.ResponseHeaderTimeout = s3ResponseHeaderTimeout })
}

// PutNew installs a new key atomically; a failed request may still have committed.
func (d *S3) PutNew(ctx context.Context, key string, body io.ReadSeeker, opts PutOptions) (Receipt, error) {
	if err := validateKey(key); err != nil {
		return Receipt{}, err
	}
	if err := validateOptions(opts); err != nil {
		return Receipt{}, err
	}
	if err := d.check(ctx, false); err != nil {
		return Receipt{}, err
	}
	return d.putNew(ctx, key, body, opts)
}
func (d *S3) putNew(ctx context.Context, key string, body io.ReadSeeker, opts PutOptions) (Receipt, error) {
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	if body == nil {
		return Receipt{}, fmt.Errorf("object body is required")
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return Receipt{}, safeError("rewind S3 content", err)
	}
	digest := sha256.New()
	size, err := io.Copy(digest, contextReader{ctx, body})
	if err != nil {
		return Receipt{}, safeError("hash S3 content", err)
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return Receipt{}, safeError("rewind S3 content", err)
	}
	receipt := Receipt{Key: key, Size: size, OwnerID: opts.OwnerID}
	output, err := d.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), Body: body, ContentLength: aws.Int64(size), ContentType: aws.String(opts.MIME), CacheControl: aws.String(cacheControl(opts.CacheControl)), IfNoneMatch: aws.String("*"), Metadata: map[string]string{"imgnest-owner": opts.OwnerID, "imgnest-sha256": hex.EncodeToString(digest.Sum(nil))}})
	if err != nil {
		return receipt, s3Error("put S3 object", err)
	}
	receipt.VersionID = aws.ToString(output.VersionId)
	return receipt, nil
}

// Open returns original bytes and persisted ownership metadata.
func (d *S3) Open(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	if err := validateKey(key); err != nil {
		return nil, ObjectInfo{}, err
	}
	output, err := d.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, ObjectInfo{}, s3Error("read S3 object", err)
	}
	return &s3Reader{contextReader: contextReader{ctx, output.Body}, body: output.Body}, objectInfo(output.ContentLength, output.ContentType, output.VersionId, output.ETag, output.Metadata), nil
}

// Stat returns current object metadata, including ownership for ambiguous-write recovery.
func (d *S3) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	if err := validateKey(key); err != nil {
		return ObjectInfo{}, err
	}
	return d.stat(ctx, key)
}
func (d *S3) stat(ctx context.Context, key string) (ObjectInfo, error) {
	output, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)})
	if err != nil {
		return ObjectInfo{}, s3Error("inspect S3 object", err)
	}
	return objectInfo(output.ContentLength, output.ContentType, output.VersionId, output.ETag, output.Metadata), nil
}
func objectInfo(size *int64, mime, version, etag *string, metadata map[string]string) ObjectInfo {
	return ObjectInfo{Size: aws.ToInt64(size), MIME: aws.ToString(mime), VersionID: aws.ToString(version), OwnerID: metadata["imgnest-owner"], digest: metadata["imgnest-sha256"], etag: aws.ToString(etag)}
}

// Copy performs server-side copying with atomic destination non-existence checks.
// A retry can reuse an existing target only when owner, size and SHA-256 match.
func (d *S3) Copy(ctx context.Context, source, target string, opts CopyOptions) (Receipt, error) {
	if err := validateKey(source); err != nil {
		return Receipt{}, err
	}
	if err := validateKey(target); err != nil {
		return Receipt{}, err
	}
	if err := validateOptions(opts); err != nil {
		return Receipt{}, err
	}
	info, err := d.stat(ctx, source)
	if err != nil {
		return Receipt{}, err
	}
	if info.OwnerID != opts.OwnerID || len(info.digest) != 64 {
		return Receipt{}, ErrOwnership
	}
	existing, err := d.stat(ctx, target)
	if err == nil {
		return matchingCopy(target, info, existing)
	}
	if !errors.Is(err, ErrNotFound) {
		return Receipt{}, err
	}
	if err := d.check(ctx, true); err != nil {
		return Receipt{}, err
	}
	if opts.MIME == "" {
		opts.MIME = info.MIME
	}
	receipt, err := d.copyNew(ctx, source, target, info, opts)
	if !errors.Is(err, ErrExists) {
		return receipt, err
	}
	existing, err = d.stat(ctx, target)
	if err != nil {
		return receipt, err
	}
	return matchingCopy(target, info, existing)
}
func matchingCopy(key string, source, target ObjectInfo) (Receipt, error) {
	if source.OwnerID != target.OwnerID {
		return Receipt{}, ErrOwnership
	}
	if source.Size != target.Size || source.digest == "" || source.digest != target.digest {
		return Receipt{}, ErrExists
	}
	return Receipt{Key: key, Size: target.Size, OwnerID: target.OwnerID, VersionID: target.VersionID}, nil
}

func (d *S3) copyNew(ctx context.Context, source, target string, info ObjectInfo, opts CopyOptions) (receipt Receipt, result error) {
	if info.Size == 0 {
		return d.putNew(ctx, target, strings.NewReader(""), opts)
	}
	created, err := d.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(d.bucket), Key: aws.String(target), ContentType: aws.String(opts.MIME), CacheControl: aws.String(cacheControl(opts.CacheControl)), Metadata: map[string]string{"imgnest-owner": opts.OwnerID, "imgnest-sha256": info.digest}})
	if err != nil {
		return Receipt{}, s3Error("begin S3 copy", err)
	}
	if aws.ToString(created.UploadId) == "" {
		return Receipt{}, fmt.Errorf("S3 copy omitted upload identifier: %w", ErrUnsupported)
	}
	completed := false
	defer func() {
		if !completed {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			_, err := d.client.AbortMultipartUpload(cleanup, &s3.AbortMultipartUploadInput{Bucket: aws.String(d.bucket), Key: aws.String(target), UploadId: created.UploadId})
			if err != nil && !isS3Missing(err) {
				result = errors.Join(result, s3Error("abort S3 copy", err))
			}
		}
	}()
	escaped := escapeCopySource(d.bucket, source, info.VersionID)
	parts := make([]types.CompletedPart, 0)
	const partSize int64 = 512 << 20
	if info.Size > partSize*10000 {
		return Receipt{}, fmt.Errorf("object exceeds multipart copy limit: %w", ErrUnsupported)
	}
	number := int32(1)
	for offset := int64(0); offset < info.Size; offset, number = offset+partSize, number+1 {
		end := min(offset+partSize, info.Size) - 1
		input := &s3.UploadPartCopyInput{Bucket: aws.String(d.bucket), Key: aws.String(target), UploadId: created.UploadId, PartNumber: aws.Int32(number), CopySource: aws.String(escaped), CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", offset, end))}
		if info.etag != "" {
			input.CopySourceIfMatch = aws.String(info.etag)
		}
		copied, err := d.client.UploadPartCopy(ctx, input)
		if err != nil {
			return Receipt{}, s3Error("copy S3 part", err)
		}
		if copied.CopyPartResult == nil || aws.ToString(copied.CopyPartResult.ETag) == "" {
			return Receipt{}, fmt.Errorf("S3 copy omitted part identity: %w", ErrUnsupported)
		}
		parts = append(parts, types.CompletedPart{ETag: copied.CopyPartResult.ETag, PartNumber: aws.Int32(number)})
	}
	receipt = Receipt{Key: target, Size: info.Size, OwnerID: opts.OwnerID}
	output, err := d.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(d.bucket), Key: aws.String(target), UploadId: created.UploadId, IfNoneMatch: aws.String("*"), MultipartUpload: &types.CompletedMultipartUpload{Parts: parts}})
	if err != nil {
		return receipt, s3Error("complete S3 copy", err)
	}
	completed = true
	receipt.VersionID = aws.ToString(output.VersionId)
	return receipt, nil
}

// DeleteCurrent makes the current key unavailable while retaining historical versions.
func (d *S3) DeleteCurrent(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	info, err := d.stat(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.OwnerID == "" {
		return ErrOwnership
	}
	_, err = d.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)})
	if err != nil {
		return s3Error("delete current S3 object", err)
	}
	return nil
}

// PurgeAllVersions removes all versions and deletion markers of precisely one key.
func (d *S3) PurgeAllVersions(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	return d.purge(ctx, key)
}
func (d *S3) purge(ctx context.Context, key string) error {
	versions, err := d.listVersions(ctx, key)
	if err != nil {
		return err
	}
	for _, version := range versions {
		if err := d.deleteVersion(ctx, key, version.id); err != nil {
			return err
		}
	}
	return nil
}

type objectVersion struct {
	id     string
	marker bool
}

func (d *S3) listVersions(ctx context.Context, key string) ([]objectVersion, error) {
	var keyMarker, versionMarker *string
	var versions []objectVersion
	for {
		output, err := d.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(d.bucket), Prefix: aws.String(key), KeyMarker: keyMarker, VersionIdMarker: versionMarker})
		if err != nil {
			return nil, s3Error("list S3 versions", err)
		}
		for _, version := range output.Versions {
			if aws.ToString(version.Key) == key {
				if aws.ToString(version.VersionId) == "" {
					return nil, fmt.Errorf("S3 version identifier absent: %w", ErrUnsupported)
				}
				versions = append(versions, objectVersion{id: aws.ToString(version.VersionId)})
			}
		}
		for _, marker := range output.DeleteMarkers {
			if aws.ToString(marker.Key) == key {
				if aws.ToString(marker.VersionId) == "" {
					return nil, fmt.Errorf("S3 marker identifier absent: %w", ErrUnsupported)
				}
				versions = append(versions, objectVersion{id: aws.ToString(marker.VersionId), marker: true})
			}
		}
		if !aws.ToBool(output.IsTruncated) {
			break
		}
		if aws.ToString(output.NextKeyMarker) == "" || (aws.ToString(keyMarker) == aws.ToString(output.NextKeyMarker) && aws.ToString(versionMarker) == aws.ToString(output.NextVersionIdMarker)) {
			return nil, fmt.Errorf("S3 pagination did not advance: %w", ErrUnsupported)
		}
		keyMarker, versionMarker = output.NextKeyMarker, output.NextVersionIdMarker
	}
	return versions, nil
}
func (d *S3) deleteVersion(ctx context.Context, key, version string) error {
	_, err := d.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), VersionId: aws.String(version)})
	if err != nil && !isS3Missing(err) {
		return s3Error("purge S3 version", err)
	}
	return nil
}

func escapeCopySource(bucket, key, version string) string {
	parts := strings.Split(bucket+"/"+key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	source := strings.Join(parts, "/")
	if version != "" {
		source += "?versionId=" + url.QueryEscape(version)
	}
	return source
}
func cacheControl(value string) string {
	if value == "" {
		return "public, max-age=31536000, immutable"
	}
	return value
}
func isS3Missing(err error) bool {
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchVersion", "NoSuchUpload":
			return true
		}
	}
	return false
}
func s3Error(operation string, err error) error {
	var api smithy.APIError
	if isS3Missing(err) {
		return fmt.Errorf("%s: %w", operation, ErrNotFound)
	}
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "PreconditionFailed", "ConditionalRequestConflict":
			return fmt.Errorf("%s: %w", operation, ErrExists)
		case "NotImplemented", "UnsupportedOperation", "InvalidRequest":
			return fmt.Errorf("%s: %w", operation, ErrUnsupported)
		}
	}
	return safeError(operation, err)
}

type s3Reader struct {
	contextReader
	body io.ReadCloser
}

func (r *s3Reader) Close() error { return r.body.Close() }
