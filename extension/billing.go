package extension

import (
	"context"
	"errors"
	"time"
)

// MaxBillingCents bounds amounts to exact integers in both databases and JavaScript.
const MaxBillingCents int64 = 1_000_000_000_000

var (
	ErrBillingConflict     = errors.New("billing state changed")
	ErrBillingInsufficient = errors.New("insufficient balance")
	ErrBillingUnavailable  = errors.New("billing unavailable")
)

// BillingActor is derived by the host from a web session, never from JSON.
type BillingActor struct{ UserID, TokenID, AuthVersion uint64 }
type BillingWallet struct {
	UserID       uint64    `json:"user_id"`
	Username     string    `json:"username"`
	BalanceCents int64     `json:"balance_cents"`
	Version      uint64    `json:"version"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type BillingEntry struct {
	ID          uint64    `json:"id"`
	UserID      uint64    `json:"user_id"`
	Username    string    `json:"username"`
	Sequence    uint64    `json:"sequence"`
	AmountCents int64     `json:"amount_cents"`
	BeforeCents int64     `json:"before_cents"`
	AfterCents  int64     `json:"after_cents"`
	Kind        string    `json:"kind"`
	Reason      string    `json:"reason"`
	ActorID     uint64    `json:"actor_id"`
	ActorName   string    `json:"actor_name"`
	OrderID     *uint64   `json:"order_id"`
	ReversesID  *uint64   `json:"reverses_id"`
	CreatedAt   time.Time `json:"created_at"`
}
type BillingGroup struct {
	Name          string `json:"name"`
	CapacityBytes int64  `json:"capacity_bytes"`
	MaxFileBytes  int64  `json:"max_file_bytes"`
	UploadPerMin  int    `json:"upload_per_min"`
}
type BillingPlan struct {
	ID           uint64        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	PriceCents   int64         `json:"price_cents"`
	DurationDays int           `json:"duration_days"`
	GroupID      uint64        `json:"group_id"`
	Published    bool          `json:"published"`
	Archived     bool          `json:"archived"`
	Version      uint64        `json:"version"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Group        *BillingGroup `json:"group,omitempty" gorm:"-"`
}
type BillingPlanInput struct {
	ID           uint64 `json:"-"`
	Version      uint64 `json:"version"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	PriceCents   int64  `json:"price_cents"`
	DurationDays int    `json:"duration_days"`
	GroupID      uint64 `json:"group_id"`
	Published    bool   `json:"published"`
	Archived     bool   `json:"archived"`
}
type BillingOrder struct {
	Currency          string     `json:"currency"`
	ID                uint64     `json:"id"`
	UserID            uint64     `json:"user_id"`
	Username          string     `json:"username"`
	PlanID            uint64     `json:"plan_id"`
	PlanName          string     `json:"plan_name"`
	PlanVersion       uint64     `json:"plan_version"`
	PriceCents        int64      `json:"price_cents"`
	DurationDays      int        `json:"duration_days"`
	GroupID           uint64     `json:"group_id"`
	PreviousExpiresAt *time.Time `json:"previous_expires_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	RequiresLogin     bool       `json:"requires_login"`
	RequestID         string     `json:"request_id"`
	CreatedAt         time.Time  `json:"created_at"`
}
type BillingMembership struct {
	UserID      uint64    `json:"user_id" gorm:"primaryKey"`
	PlanID      uint64    `json:"plan_id"`
	GroupID     uint64    `json:"group_id"`
	BaseGroupID uint64    `json:"base_group_id"`
	StartsAt    time.Time `json:"starts_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type BillingOverview struct {
	Currency   string             `json:"currency"`
	Wallet     BillingWallet      `json:"wallet"`
	Membership *BillingMembership `json:"membership"`
}
type BillingPage[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}
type BillingFilter struct {
	RequestID  string
	UserID     uint64
	All        bool
	Page, Size int
	From, To   *time.Time
}
type BillingAdjustment struct {
	UserID      uint64 `json:"user_id"`
	AmountCents int64  `json:"amount_cents"`
	Reason      string `json:"reason"`
	RequestID   string `json:"request_id"`
}
type BillingReversal struct {
	EntryID   uint64 `json:"entry_id"`
	Reason    string `json:"reason"`
	RequestID string `json:"request_id"`
}
type BillingPurchase struct {
	PlanID      uint64 `json:"plan_id"`
	PlanVersion uint64 `json:"plan_version"`
	RequestID   string `json:"request_id"`
}

// BillingStore exposes only typed, authenticated operations. Each write is atomic.
type BillingStore interface {
	Overview(context.Context, BillingActor) (BillingOverview, error)
	Plans(context.Context, BillingActor, bool) ([]BillingPlan, error)
	SavePlan(context.Context, BillingActor, BillingPlanInput) (BillingPlan, error)
	Wallets(context.Context, BillingActor, BillingFilter) (BillingPage[BillingWallet], error)
	Ledger(context.Context, BillingActor, BillingFilter) (BillingPage[BillingEntry], error)
	Orders(context.Context, BillingActor, BillingFilter) (BillingPage[BillingOrder], error)
	Memberships(context.Context, BillingActor, BillingFilter) (BillingPage[BillingMembership], error)
	Adjust(context.Context, BillingActor, BillingAdjustment) (BillingEntry, error)
	Reverse(context.Context, BillingActor, BillingReversal) (BillingEntry, error)
	Purchase(context.Context, BillingActor, BillingPurchase) (BillingOrder, error)
}

// BillingStoreUser receives the host's durable store before requests are served.
type BillingStoreUser interface{ UseBillingStore(BillingStore) }

// BillingProvider contributes an optional licensed commerce service.
type BillingProvider interface {
	BillingService(context.Context) (BillingStore, bool)
}
