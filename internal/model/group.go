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

// GroupUpdate lists the fields one group PATCH sets; nil keeps the stored
// value. The repository merges it into the row it holds locked, so two
// patches of different fields never undo each other.
type GroupUpdate struct {
	Name            *string
	CapacityBytes   *int64
	MaxFileBytes    *int64
	AllowedExts     *[]string
	UploadPerMin    *int
	DefaultPolicyID *uint64
	// PolicyIDs replaces the rule bindings when set.
	PolicyIDs *[]uint64
}
