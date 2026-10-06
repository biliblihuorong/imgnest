package model

import "time"

// ImageListFilter carries the native image-list predicates. AlbumID keeps the
// native semantics: nil means "no album filter", zero selects unassigned
// images and a positive value selects one album. Order is one of
// "newest" (default), "oldest", "largest", "smallest". MinSize/MaxSize bound
// the stored original size in bytes (zero = unbounded). From/To bound
// created_at in UTC. Keyword matches file names only, Exif matches
// make/model/lens only, and the unified Q matches either (OR).
type ImageListFilter struct {
	Search  *ImageSearchFilter
	UserID  uint64
	Admin   bool
	Trash   bool
	AlbumID *uint64
	Keyword string
	Q       string
	Order   string
	MinSize int64
	MaxSize int64
	From    *time.Time
	To      *time.Time
	Exif    string
}
