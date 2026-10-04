package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"strings"
	"time"
)

// Check verifies conditional puts and server-side copies using private temporary keys.
// Compatible endpoints that silently ignore preconditions are rejected before real writes.
func (d *S3) Check(ctx context.Context) error { return d.check(ctx, true) }
func (d *S3) check(ctx context.Context, copyRequired bool) (result error) {
	select {
	case d.probeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-d.probeGate }()
	if d.putVerified && (!copyRequired || d.copyVerified) {
		return ctx.Err()
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return safeError("create S3 probe identity", err)
	}
	owner := "probe-" + hex.EncodeToString(random[:])
	source := ".imgnest-probe/" + owner + "/source"
	target := ".imgnest-probe/" + owner + "/target"
	copied := ".imgnest-probe/" + owner + "/copy"
	opts := PutOptions{OwnerID: owner, MIME: "application/octet-stream", CacheControl: "no-store"}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		for _, key := range []string{source, target, copied} {
			if err := d.purgeOwned(cleanup, key, owner); err != nil {
				result = errors.Join(result, err)
			}
		}
		if result == nil {
			d.putVerified = true
			if copyRequired {
				d.copyVerified = true
			}
		}
	}()
	if _, err := d.putNew(ctx, source, strings.NewReader("source"), opts); err != nil {
		return err
	}
	if _, err := d.putNew(ctx, source, strings.NewReader("overwritten"), opts); !errors.Is(err, ErrExists) {
		return fmt.Errorf("S3 conditional put is not enforced: %w", ErrUnsupported)
	}
	if err := d.checkProbeBytes(ctx, source, "source"); err != nil {
		return err
	}
	if !copyRequired {
		return nil
	}
	if _, err := d.putNew(ctx, target, strings.NewReader("target"), opts); err != nil {
		return err
	}
	info, err := d.stat(ctx, source)
	if err != nil {
		return err
	}
	if _, err := d.copyNew(ctx, source, target, info, opts); !errors.Is(err, ErrExists) {
		return fmt.Errorf("S3 conditional copy completion is not enforced: %w", ErrUnsupported)
	}
	if err := d.checkProbeBytes(ctx, target, "target"); err != nil {
		return err
	}
	if _, err := d.copyNew(ctx, source, copied, info, opts); err != nil {
		return err
	}
	return d.checkProbeBytes(ctx, copied, "source")
}
func (d *S3) checkProbeBytes(ctx context.Context, key, want string) error {
	output, err := d.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)})
	if err != nil {
		return s3Error("read S3 capability probe", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(output.Body, 64))
	closeErr := output.Body.Close()
	if readErr != nil || closeErr != nil {
		return safeError("read S3 capability probe", errors.Join(readErr, closeErr))
	}
	if string(body) != want {
		return fmt.Errorf("S3 capability probe content changed: %w", ErrUnsupported)
	}
	return nil
}

// PurgeOwned compensates writes by deleting only versions with the given owner.
// Delete markers have no ownership metadata and are intentionally retained.
func (d *S3) PurgeOwned(ctx context.Context, key, owner string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if owner == "" {
		return ErrOwnership
	}
	return d.purgeOwned(ctx, key, owner)
}

// PurgeImage validates the entire exact-key history before removing any version.
func (d *S3) PurgeImage(ctx context.Context, key, owner string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if owner == "" {
		return ErrOwnership
	}
	versions, err := d.listVersions(ctx, key)
	if err != nil {
		return err
	}
	// Validate the complete snapshot before deleting even the first owned version.
	for _, version := range versions {
		if version.marker {
			continue
		}
		output, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), VersionId: aws.String(version.id)})
		if isS3Missing(err) {
			continue
		}
		if err != nil {
			return s3Error("verify final image owner", err)
		}
		if output.Metadata["imgnest-owner"] != owner {
			return ErrOwnership
		}
	}
	for _, version := range versions {
		if err := d.deleteVersion(ctx, key, version.id); err != nil {
			return err
		}
	}
	remaining, err := d.listVersions(ctx, key)
	if err != nil {
		return err
	}
	if len(remaining) != 0 {
		return fmt.Errorf("image history changed during purge: %w", ErrExists)
	}
	return nil
}
func (d *S3) purgeOwned(ctx context.Context, key, owner string) error {
	versions, err := d.listVersions(ctx, key)
	if err != nil {
		return err
	}
	for _, version := range versions {
		if version.marker {
			continue
		}
		output, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key), VersionId: aws.String(version.id)})
		if isS3Missing(err) {
			continue
		}
		if err != nil {
			return s3Error("inspect S3 version owner", err)
		}
		if output.Metadata["imgnest-owner"] == owner {
			if err := d.deleteVersion(ctx, key, version.id); err != nil {
				return err
			}
		}
	}
	return nil
}
