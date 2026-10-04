package model

import "time"

// Group stores account quota and upload policy attributes.
type Group struct {
	ID              uint64 `gorm:"primaryKey"`
	Name            string
	IsDefault       bool
	IsGuest         bool
	CapacityBytes   int64
	MaxFileBytes    int64
	AllowedExts     []string `gorm:"serializer:json"`
	UploadPerMin    int
	DefaultPolicyID uint64 `gorm:"default:null"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
