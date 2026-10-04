package model

import "time"

// Policy stores the processing and path rules for one storage backend.
type Policy struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	StorageID    uint64    `json:"storage_id"`
	Name         string    `json:"name"`
	PathTpl      string    `json:"path_tpl"`
	NameTpl      string    `json:"name_tpl"`
	WebPMode     string    `gorm:"column:webp_mode" json:"webp_mode"`
	ScrubMode    string    `json:"scrub_mode"`
	LinkPrefer   string    `json:"link_prefer"`
	HEIFMode     string    `gorm:"column:heif_mode" json:"heif_mode"`
	OnConflict   string    `json:"on_conflict"`
	WebPQuality  int       `gorm:"column:webp_quality" json:"webp_quality"`
	WebPEffort   int       `gorm:"column:webp_effort" json:"webp_effort"`
	MaxWidth     int       `json:"max_width"`
	MaxHeight    int       `json:"max_height"`
	ThumbSize    int       `json:"thumb_size"`
	WebPLossless bool      `gorm:"column:webp_lossless" json:"webp_lossless"`
	StripMeta    bool      `json:"strip_meta"`
	SkipIfLarger bool      `json:"skip_if_larger"`
	ThumbEnabled bool      `json:"thumb_enabled"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
