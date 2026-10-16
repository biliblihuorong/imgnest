package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Responsive variants are static WebP renditions at fixed target widths,
// generated once per image and never rewritten in place.
const (
	// VariantVersion is the processing version embedded in every variant key.
	// Changing how variants are encoded requires a new version, so already
	// published (immutable) files are never overwritten.
	VariantVersion = 1
	VariantMIME    = "image/webp"
)

// VariantWidths are the target widths of the first variant generation.
var VariantWidths = []int{400, 800, 1600}

// variantSuffix matches the reserved `_w{width}_v{version}` name suffix.
var variantSuffix = regexp.MustCompile(`(?i)_w[0-9]+_v[0-9]+$`)

// ImageVariant records one stored rendition: its object key and the actual
// encoded dimensions. URLs are never stored; they are built from the
// storage's current base URL.
type ImageVariant struct {
	Key    string `json:"key"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int64  `json:"bytes"`
	MIME   string `json:"mime"`
}

// VariantList is stored as a JSON array; a nil list is stored as [] so the
// column's array constraint holds for every image row.
type VariantList []ImageVariant

// Value encodes the list for the database.
func (l VariantList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	encoded, err := json.Marshal([]ImageVariant(l))
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

// Scan decodes a stored JSON array.
func (l *VariantList) Scan(value any) error {
	var raw []byte
	switch typed := value.(type) {
	case nil:
		*l = VariantList{}
		return nil
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return errors.New("unsupported variant list encoding")
	}
	list := []ImageVariant{}
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	if list == nil {
		list = []ImageVariant{}
	}
	*l = list
	return nil
}

// VariantKey is the object key of the width rendition of an image path.
func VariantKey(imagePath string, width int) string {
	return imagePath + "_w" + strconv.Itoa(width) + "_v" + strconv.Itoa(VariantVersion) + ".webp"
}

// VariantBase returns the image path a variant key belongs to.
func VariantBase(key string) (string, bool) {
	stem, ok := strings.CutSuffix(key, ".webp")
	if !ok {
		return "", false
	}
	location := variantSuffix.FindStringIndex(stem)
	if location == nil || location[0] == 0 || stem[location[0]-1] == '/' {
		return "", false
	}
	return stem[:location[0]], true
}

// ReservedVariantName reports whether an extension-free file name ends with
// the variant suffix, which user path templates may not produce.
func ReservedVariantName(name string) bool {
	return variantSuffix.MatchString(name)
}

// VariantJob journals a background variant generation for one active image:
// the objects it intends to write and has written, with their reserved quota.
type VariantJob struct {
	ImageID     uint64          `gorm:"primaryKey" json:"image_id"`
	ImageKey    string          `json:"image_key"`
	OperationID string          `json:"-"`
	Objects     []ObjectReceipt `gorm:"serializer:json" json:"objects"`
	Variants    VariantList     `json:"variants"`
	Bytes       int64           `json:"bytes"`
	CreatedAt   time.Time       `json:"created_at"`
}

// TableName keeps the journal table name explicit.
func (VariantJob) TableName() string { return "image_variant_jobs" }

// Variant backfill states.
const (
	BackfillIdle    = "idle"
	BackfillRunning = "running"
	BackfillPaused  = "paused"
	BackfillDone    = "done"
)

// VariantBackfill is the single resumable progress row of the historic
// variant generation task.
type VariantBackfill struct {
	ID        uint64    `gorm:"primaryKey" json:"-"`
	Status    string    `json:"status"`
	Cursor    uint64    `gorm:"column:last_id" json:"cursor"`
	Processed int64     `json:"processed"`
	Generated int64     `json:"generated"`
	Skipped   int64     `json:"skipped"`
	Failed    int64     `json:"failed"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName keeps the progress table name explicit.
func (VariantBackfill) TableName() string { return "variant_backfill" }
