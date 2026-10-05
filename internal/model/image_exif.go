package model

import (
	"encoding/json"
	"time"
)

// ImageExif contains owner/admin metadata and must never be embedded in a public image DTO.
type ImageExif struct {
	CameraSearch *string         `json:"-"`
	LensSearch   *string         `json:"-"`
	ImageID      uint64          `gorm:"primaryKey" json:"image_id"`
	Make         string          `json:"make"`
	Model        string          `json:"model"`
	Lens         string          `json:"lens"`
	Exposure     string          `json:"exposure"`
	FNumber      string          `json:"f_number"`
	FocalLength  string          `json:"focal_length"`
	ISO          int             `gorm:"column:iso" json:"iso"`
	Orientation  int             `json:"orientation"`
	TakenAt      *time.Time      `json:"taken_at"`
	GPSLat       *float64        `gorm:"column:gps_lat" json:"gps_lat"`
	GPSLng       *float64        `gorm:"column:gps_lng" json:"gps_lng"`
	GPSAlt       *float64        `gorm:"column:gps_alt" json:"gps_alt"`
	Raw          json.RawMessage `gorm:"serializer:json" json:"raw"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// TableName preserves the singular table name specified by the image metadata contract.
func (ImageExif) TableName() string { return "image_exif" }
