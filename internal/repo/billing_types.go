package repo

import (
	"github.com/biliblihuorong/imgnest/extension"
	"time"
)

type billingWallet extension.BillingWallet

func (billingWallet) TableName() string { return "billing_wallets" }

type billingEntry extension.BillingEntry

func (billingEntry) TableName() string { return "billing_entries" }

type billingPlan extension.BillingPlan

func (billingPlan) TableName() string { return "billing_plans" }

type billingOrder extension.BillingOrder

func (billingOrder) TableName() string { return "billing_orders" }

type billingMembership extension.BillingMembership

func (billingMembership) TableName() string { return "billing_memberships" }

type billingRequest struct {
	ActorID     uint64
	Operation   string
	RequestID   string
	Fingerprint string
	Result      string
	CreatedAt   time.Time
}

func (billingRequest) TableName() string { return "billing_requests" }
