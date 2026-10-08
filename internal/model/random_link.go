package model

import "time"

// RandomLink is one album's anonymous random-image link. The token is a
// low-privilege capability that only redirects to that album's images.
type RandomLink struct {
	ID        uint64 `gorm:"primaryKey"`
	UserID    uint64
	AlbumID   uint64
	Token     string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RandomCandidate carries only what a redirect URL needs.
type RandomCandidate struct {
	StorageID   uint64
	Path        string
	Ext         string
	HasWebP     bool `gorm:"column:has_webp"`
	HasOriginal bool
}
