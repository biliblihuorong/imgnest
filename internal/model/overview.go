package model

// ObjectBytes splits charged cloud bytes by the object they belong to.
// Original, WebP and Thumb come from the per-version size columns; Variants
// is what remains of charged_bytes, i.e. the stored responsive renditions.
type ObjectBytes struct {
	Total    int64
	Original int64
	WebP     int64
	Thumb    int64
}

// StorageUsage aggregates the images stored on one backend.
type StorageUsage struct {
	StorageID   uint64
	Name        string
	Driver      string
	Enabled     bool
	Images      int64
	Bytes       ObjectBytes `gorm:"embedded"`
	TrashImages int64
	TrashBytes  int64
}

// PolicyUsage aggregates the active images uploaded through one rule.
type PolicyUsage struct {
	PolicyID  uint64
	Name      string
	StorageID uint64
	Enabled   bool
	Images    int64
	Bytes     int64
}

// UploadDay counts the images created on one UTC day, including those
// already in the recycle bin.
type UploadDay struct {
	Day     string
	Uploads int64
	Bytes   int64
}

// UserUsage is one account's quota counter and active image count.
type UserUsage struct {
	UserID    uint64
	Username  string
	UsedBytes int64
	Images    int64
}

// Overview is the site-wide snapshot behind the console's overview page.
type Overview struct {
	Users         int64
	Albums        int64
	PendingImages int64
	GuestImages   int64
	GuestBytes    int64
	Storages      []StorageUsage
	Policies      []PolicyUsage
	Uploads       []UploadDay
	TopUsers      []UserUsage
	// VariantEligible counts active single-frame images under rules with
	// responsive variants; VariantCovered those that already have them.
	VariantEligible int64
	VariantCovered  int64
}
