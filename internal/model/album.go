package model

import "time"

// Album stores image ownership and grouping; its full management UI is a later milestone.
type Album struct {
	NameNFC      *string   `gorm:"column:name_nfc" json:"-"`
	NameSearch   *string   `json:"-"`
	ID           uint64    `gorm:"primaryKey" json:"id"`
	UserID       uint64    `json:"user_id"`
	Name         string    `json:"name"`
	Intro        string    `json:"intro"`
	IsPublic     bool      `json:"is_public"`
	CoverImageID uint64    `gorm:"default:null" json:"cover_image_id"`
	ImageCount   int64     `json:"image_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
