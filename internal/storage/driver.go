// Package storage provides owned, non-overwriting image objects.
package storage

import (
	"context"
	"errors"
	"io"
)

// Driver is the object lifecycle used by upload and recycle-bin services.
type Driver interface {
	PutNew(context.Context, string, io.ReadSeeker, PutOptions) (Receipt, error)
	Open(context.Context, string) (io.ReadCloser, ObjectInfo, error)
	Stat(context.Context, string) (ObjectInfo, error)
	Copy(context.Context, string, string, CopyOptions) (Receipt, error)
	DeleteCurrent(context.Context, string) error
	PurgeAllVersions(context.Context, string) error
}

// OwnedPurger compensates only versions written by a specific image owner.
type OwnedPurger interface {
	PurgeOwned(context.Context, string, string) error
}

// ImagePurger removes full image history only after validating every object owner.
type ImagePurger interface {
	PurgeImage(context.Context, string, string) error
}

// Storage errors have stable classifications independent of filesystem or vendor.
var (
	ErrNotFound    = errors.New("object not found")
	ErrExists      = errors.New("object already exists")
	ErrOwnership   = errors.New("object ownership mismatch")
	ErrUnsupported = errors.New("storage capability unsupported")
)

// Receipt identifies a completed or possibly completed object write.
type Receipt struct {
	Key, VersionID, OwnerID string
	Size                    int64
}

// ObjectInfo contains persisted object metadata, never credentials.
type ObjectInfo struct {
	Size                     int64
	MIME, OwnerID, VersionID string
	digest, etag             string
}

// PutOptions supplies ownership and public object headers.
type PutOptions struct{ MIME, OwnerID, CacheControl string }

// CopyOptions supplies ownership and public object headers for a new target.
type CopyOptions = PutOptions
