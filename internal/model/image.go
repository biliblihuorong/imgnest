package model

import "time"

// Image states describe visibility; operations describe unfinished object IO.
const (
	ImageStatePending            = "pending"
	ImageStateActive             = "active"
	ImageStateTrash              = "trash"
	ImageOperationUpload         = "upload"
	ImageOperationCleanup        = "cleanup"
	ImageOperationTrash          = "trash"
	ImageOperationRestore        = "restore"
	ImageOperationRestoreCleanup = "restore_cleanup"
	ImageOperationPurge          = "purge"
	ObjectLocationCloud          = "cloud"
	ObjectLocationThumbCache     = "thumb-cache"
)

// ObjectReceipt records an intended or completed object owned by one image key.
type ObjectReceipt struct {
	Key       string `json:"key"`
	VersionID string `json:"version_id"`
	OwnerID   string `json:"owner_id"`
	Location  string `json:"location"`
	MIME      string `json:"mime"`
	SHA256    string `json:"sha256,omitempty"`
	Size      int64  `json:"size"`
}

// CanonicalExt maps extension aliases onto the form the imaging pipeline
// detects, so an allowlist entry of "jpeg" or "tif" governs the same upload
// as "jpg" or "tiff".
func CanonicalExt(ext string) string {
	switch ext {
	case "jpeg":
		return "jpg"
	case "tif":
		return "tiff"
	}
	return ext
}

// Image persists identity, version sizes and a durable storage-operation journal.
type Image struct {
	ID             uint64          `gorm:"primaryKey" json:"id"`
	UserID         uint64          `json:"user_id"`
	AlbumID        uint64          `gorm:"default:null" json:"album_id"`
	PolicyID       uint64          `json:"policy_id"`
	StorageID      uint64          `json:"storage_id"`
	Key            string          `json:"key"`
	Path           string          `json:"path"`
	Ext            string          `json:"ext"`
	FilenameSearch *string         `json:"-"`
	OriginName     string          `json:"origin_name"`
	MIME           string          `gorm:"column:mime" json:"mime"`
	SrcMD5         string          `gorm:"column:src_md5" json:"src_md5"`
	MD5            string          `gorm:"column:md5" json:"md5"`
	SHA1           string          `gorm:"column:sha1" json:"sha1"`
	IP             string          `json:"ip"`
	State          string          `json:"state"`
	Operation      string          `json:"operation"`
	OperationID    string          `json:"-"`
	HasOriginal    bool            `json:"has_original"`
	HasWebP        bool            `gorm:"column:has_webp" json:"has_webp"`
	HasThumb       bool            `json:"has_thumb"`
	Scrubbed       bool            `json:"scrubbed"`
	IsPublic       bool            `json:"is_public"`
	Size           int64           `json:"size"`
	WebPSize       int64           `gorm:"column:webp_size" json:"webp_size"`
	ThumbBytes     int64           `json:"thumb_bytes"`
	ChargedBytes   int64           `json:"charged_bytes"`
	Width          int             `json:"width"`
	Height         int             `json:"height"`
	Frames         int             `json:"frames"`
	ObjectManifest []ObjectReceipt `gorm:"serializer:json" json:"-"`
	DeletedAt      *time.Time      `json:"deleted_at"`
	PurgeAt        *time.Time      `json:"purge_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// UploadReservation binds prepared image bytes and object intent to an authentication proof.
type UploadReservation struct {
	Image   Image
	Objects []ObjectReceipt
	Grant   TokenGrant
	// SourceExt is the probed format of the uploaded bytes. It differs from
	// Image.Ext when only the WebP conversion is stored, and it is what the
	// group's allowed formats apply to.
	SourceExt string
}

// GalleryImage pairs one public gallery row with its uploader's username.
// Guest uploads (user_id zero) have no account row and carry an empty name.
type GalleryImage struct {
	Image    `gorm:"embedded"`
	Uploader string
}
