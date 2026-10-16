package storage

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// CopyLegacy copies the adopted version of an object another program wrote
// into a new ImgNest-owned target. The source version must still carry no
// owner; digest is the SHA-256 recorded when the image was adopted.
func (d *S3) CopyLegacy(ctx context.Context, source, version, target, digest string, opts CopyOptions) (Receipt, error) {
	if err := validateKey(source); err != nil {
		return Receipt{}, err
	}
	if err := validateKey(target); err != nil {
		return Receipt{}, err
	}
	if err := validateOptions(opts); err != nil {
		return Receipt{}, err
	}
	if !adoptableVersion(version) || !validDigest(digest) {
		return Receipt{}, ErrOwnership
	}
	output, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(source), VersionId: aws.String(version)})
	if err != nil {
		return Receipt{}, s3Error("inspect adopted S3 object", err)
	}
	info := objectInfo(output.ContentLength, output.ContentType, output.VersionId, output.ETag, output.Metadata)
	if info.OwnerID != "" {
		return Receipt{}, ErrOwnership
	}
	// From here the pinned version stands in for an owned source.
	info.OwnerID, info.VersionID, info.digest = opts.OwnerID, version, digest
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

// DeleteLegacy hides an adopted object while it is still the adopted
// version. The bucket keeps that version behind a delete marker, so the
// bytes another program wrote are never destroyed here.
func (d *S3) DeleteLegacy(ctx context.Context, key, version string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if !adoptableVersion(version) {
		return ErrOwnership
	}
	info, err := d.stat(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.OwnerID != "" || info.VersionID != version {
		return ErrOwnership
	}
	if _, err := d.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)}); err != nil {
		return s3Error("delete current S3 object", err)
	}
	return nil
}

// PurgeLegacy removes the whole history of an adopted key: the adopted
// version and any versions owner wrote after it (a restored copy).
func (d *S3) PurgeLegacy(ctx context.Context, key, owner, version string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if owner == "" || !adoptableVersion(version) {
		return ErrOwnership
	}
	return d.purgeImage(ctx, key, owner, version)
}

// adoptableVersion rejects the empty and "null" ids of unversioned buckets,
// where a delete could not keep the adopted bytes.
func adoptableVersion(version string) bool {
	return version != "" && version != "null" && len(version) <= 1024
}

func validDigest(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, r := range digest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

var _ LegacyObjects = (*S3)(nil)
