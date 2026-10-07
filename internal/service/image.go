package service

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/biliblihuorong/imgnest/internal/exif"
	"github.com/biliblihuorong/imgnest/internal/imaging"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/storage"
)

// ImageRepository owns atomic image reservations and durable object transitions.
type ImageRepository interface {
	ReserveUpload(context.Context, model.UploadReservation) (model.Image, error)
	RecordObjectReceipt(context.Context, string, string, model.ObjectReceipt) error
	CommitUpload(context.Context, string, string, model.ImageExif, model.TokenGrant) (model.Image, error)
	StartCleanup(context.Context, string, string) error
	FinishCleanup(context.Context, string, string) error
	FindByKey(context.Context, string) (model.Image, error)
	FindByID(context.Context, uint64) (model.Image, error)
	FindByPath(context.Context, uint64, string) (model.Image, error)
	FindExif(context.Context, string) (model.ImageExif, error)
	List(context.Context, model.ImageListFilter, int, int) ([]model.Image, int64, error)
	SetAlbum(context.Context, string, uint64, model.TokenGrant) error
	SetPublic(context.Context, string, bool, model.TokenGrant) error
	BeginTrash(context.Context, string, string, model.TokenGrant, int) (model.Image, error)
	FinishTrash(context.Context, string, string) error
	BeginRestore(context.Context, string, string, model.TokenGrant) (model.Image, error)
	FinishRestore(context.Context, string, string, model.TokenGrant) (model.Image, error)
	FinishRestoreCleanup(context.Context, string, string) error
	CancelRestore(context.Context, string, string) error
	BeginPurge(context.Context, string, string, model.TokenGrant) (model.Image, error)
	BeginSystemPurge(context.Context, string, string) (model.Image, error)
	FinishPurge(context.Context, string, string) error
	PendingOperations(context.Context) ([]model.Image, error)
	DueTrash(context.Context, time.Time, int) ([]model.Image, error)
}

// PolicyRepository selects allowed upload rules and reads rules for existing images.
type PolicyRepository interface {
	UploadPolicy(context.Context, uint64, uint64) (model.Policy, model.Storage, model.Group, error)
	Find(context.Context, uint64) (model.Policy, error)
	CreateAndBind(context.Context, model.Policy, uint64, bool) (model.Policy, error)
	GroupPolicies(context.Context, uint64) ([]model.Policy, error)
}

// AlbumStore locates one owner's album for image assignment checks. It is an
// optional capability: the concrete album repository implements it and the
// composition root wires it into ImageDependencies.
type AlbumStore interface {
	FindOwned(ctx context.Context, ownerID, albumID uint64) (model.Album, error)
}

// StorageRepository persists backend settings without publishing credentials.
type StorageRepository interface {
	Create(context.Context, model.Storage) (model.Storage, error)
	Find(context.Context, uint64) (model.Storage, error)
	List(context.Context) ([]model.Storage, error)
}

// StorageProvider resolves a backend through injected configuration and drivers.
type StorageProvider interface {
	DriverFor(context.Context, model.Storage) (storage.Driver, error)
}

// PathBuilder renders and validates safe extension-free image paths.
type PathBuilder interface {
	Build(context.Context, string, string, pathtpl.Variables) (pathtpl.Result, error)
	Sanitize(context.Context, string) (string, error)
}

// PathFunctions adapts the independent path package at the composition root.
type PathFunctions struct {
	BuildPath func(context.Context, string, string, pathtpl.Variables) (pathtpl.Result, error)
	CleanPath func(context.Context, string) (string, error)
}

// Build invokes the injected path renderer.
func (f PathFunctions) Build(ctx context.Context, path, name string, vars pathtpl.Variables) (pathtpl.Result, error) {
	return f.BuildPath(ctx, path, name, vars)
}

// Sanitize invokes the injected path validator.
func (f PathFunctions) Sanitize(ctx context.Context, path string) (string, error) {
	return f.CleanPath(ctx, path)
}

// ThumbCache stores replaceable previews separately from charged cloud objects.
type ThumbCache interface {
	Put(context.Context, uint64, string, []byte) error
	Open(context.Context, uint64, string) (io.ReadCloser, error)
	Delete(context.Context, uint64, string) error
}

// ImageSettings provides the recycle-bin retention period.
type ImageSettings interface {
	TrashDays(context.Context) (int, error)
}

// ImageDependencies contains only the capabilities needed by image business rules.
type ImageDependencies struct {
	Images    ImageRepository
	Policies  PolicyRepository
	Storages  StorageRepository
	Users     UserRepository
	Tokens    TokenRepository
	Drivers   StorageProvider
	Paths     PathBuilder
	Imaging   imaging.Processor
	Extractor exif.Extractor
	Scrubber  exif.Scrubber
	Cache     ThumbCache
	Settings  ImageSettings
	// Albums validates album ownership for image assignment; it is optional
	// and album-related operations reject requests while it stays unset.
	Albums AlbumStore
	// RandomPool drops cached random-link candidates when an album's active
	// images change; nil disables invalidation.
	RandomPool   RandomPoolInvalidator
	Now          func() time.Time
	MaxFileBytes int64
}

// ImageService implements synchronous uploads and retryable recycle-bin operations.
type ImageService struct {
	deps       ImageDependencies
	operations chan struct{}
}

// UploadInput contains a bounded source buffer and caller-controlled upload options.
type UploadInput struct {
	Data              []byte
	Filename, IP      string
	PolicyID, AlbumID uint64
	IsPublic          bool
}

// ImageLinks are rebuilt from the backend's current base URL on every response.
type ImageLinks struct {
	URL       string `json:"url"`
	Original  string `json:"original"`
	WebP      string `json:"webp"`
	Thumbnail string `json:"thumbnail_url"`
}

// PublicObject is an authorized object stream or an original-image redirect.
type PublicObject struct {
	Body           io.ReadCloser
	Size           int64
	MIME, Redirect string
}

// ImageView deliberately excludes EXIF, IP addresses, and storage-operation internals.
type ImageView struct {
	ID            uint64     `json:"id"`
	Key           string     `json:"key"`
	UserID        uint64     `json:"user_id"`
	AlbumID       uint64     `json:"album_id"`
	PolicyID      uint64     `json:"policy_id"`
	StorageID     uint64     `json:"storage_id"`
	Name          string     `json:"name"`
	Path          string     `json:"-"`
	Ext           string     `json:"ext"`
	MIME          string     `json:"mime"`
	Size          int64      `json:"size"`
	WebPSize      int64      `json:"webp_size"`
	ChargedBytes  int64      `json:"charged_bytes"`
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	Frames        int        `json:"frames"`
	HasOriginal   bool       `json:"has_original"`
	HasWebP       bool       `json:"has_webp"`
	HasThumb      bool       `json:"has_thumb"`
	Scrubbed      bool       `json:"scrubbed"`
	IsPublic      bool       `json:"is_public"`
	MD5           string     `json:"md5"`
	SHA1          string     `json:"sha1"`
	SrcMD5        string     `json:"src_md5"`
	Links         ImageLinks `json:"links"`
	LocalThumbURL string     `json:"local_thumb_url"`
	DeletedAt     *time.Time `json:"deleted_at"`
	PurgeAt       *time.Time `json:"purge_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ImageQuery limits one owner's active images or recycle bin. AlbumID nil
// means "no album filter"; zero selects unassigned images; a positive value
// selects one owner-verified album. Keyword narrows by file name, Exif by
// make/model/lens, and the unified Q matches either (OR). Order is
// newest|oldest|largest|smallest, MinSize/MaxSize bound the original byte
// size and From/To bound the upload time.
type ImageQuery struct {
	QueryVersion  int
	Timezone      string
	LockedAlbumID *uint64
	Page, Size    int
	Trash, Admin  bool
	AlbumID       *uint64
	Keyword       string
	Q             string
	Order         string
	MinSize       int64
	MaxSize       int64
	From, To      *time.Time
	Exif          string
}

// ImagePage is the native image-list response.
type ImagePage struct {
	Search *SearchMetadata `json:"search,omitempty"`
	Items  []ImageView     `json:"items"`
	Total  int64           `json:"total"`
	Page   int             `json:"page"`
	Size   int             `json:"size"`
}

// GalleryItem is one public gallery entry: an image view plus the uploader's
// username. ImageView already excludes EXIF, addresses, and internals.
type GalleryItem struct {
	ImageView
	Uploader string `json:"uploader"`
}

// GalleryPage is the public gallery listing.
type GalleryPage struct {
	Items []GalleryItem `json:"items"`
	Total int64         `json:"total"`
	Page  int           `json:"page"`
	Size  int           `json:"size"`
}

// UploadLimits permits HTTP to check authentication and group caps before reading a body.
type UploadLimits struct {
	MaxFileBytes int64
	PerMinute    int
}

// invalidateRandom drops the random-link candidates of every named album.
// Failures are ignored: the pool's short TTL bounds how long a stale entry
// lives, and a cache outage must never fail an image operation.
func (s *ImageService) invalidateRandom(ctx context.Context, albumIDs ...uint64) {
	if s.deps.RandomPool == nil {
		return
	}
	for _, albumID := range albumIDs {
		if albumID != 0 {
			_ = s.deps.RandomPool.Invalidate(ctx, albumID)
		}
	}
}

// NewImageService constructs image business operations.
func NewImageService(ctx context.Context, deps ImageDependencies) (*ImageService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct image service: %w", err)
	}
	if deps.Images == nil || deps.Policies == nil || deps.Storages == nil || deps.Users == nil || deps.Tokens == nil || deps.Drivers == nil || deps.Paths == nil || deps.Imaging == nil || deps.Extractor == nil || deps.Scrubber == nil || deps.Cache == nil || deps.Now == nil || deps.MaxFileBytes <= 0 {
		return nil, ErrInvalidInput
	}
	return &ImageService{deps: deps, operations: make(chan struct{}, 1)}, nil
}
