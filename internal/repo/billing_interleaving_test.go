package repo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

type billingBarrierKey struct{}

// Pause a real transaction after acquiring its row lock. The second operation
// must remain blocked until release, on PostgreSQL and SQLite's immediate tx.
func billingInterleave(t *testing.T, db *gorm.DB, table string, buyer uint64, beforeSecond func(), first, second func(context.Context) error) (error, error) {
	t.Helper()
	held, release := make(chan struct{}), make(chan struct{})
	var entered, released sync.Once
	unblock := func() { released.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if err := db.Callback().Query().After("gorm:query").Register("billing-interleave", func(tx *gorm.DB) {
		if tx.Statement.Table != table || tx.Statement.Context.Value(billingBarrierKey{}) != true {
			return
		}
		if table == "users" {
			user, ok := tx.Statement.Dest.(*model.User)
			if !ok || user.ID != buyer {
				return
			}
		}
		entered.Do(func() {
			close(held)
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Callback().Query().Remove("billing-interleave"); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	a, b := make(chan error, 1), make(chan error, 1)
	go func() { a <- first(context.WithValue(ctx, billingBarrierKey{}, true)) }()
	select {
	case <-held:
	case err := <-a:
		t.Fatalf("first transaction never reached lock: %v", err)
	case <-ctx.Done():
		t.Fatal("first transaction did not reach barrier")
	}
	if beforeSecond != nil {
		beforeSecond()
	}
	started := make(chan struct{})
	go func() {
		close(started)
		b <- second(ctx)
	}()
	<-started
	select {
	case err := <-b:
		t.Fatalf("second operation escaped locked transaction: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	var firstErr, secondErr error
	select {
	case firstErr = <-a:
	case <-ctx.Done():
		t.Fatal("first operation did not finish")
	}
	select {
	case secondErr = <-b:
	case <-ctx.Done():
		t.Fatal("second operation did not finish")
	}
	return firstErr, secondErr
}

func billingAssertAccounts(t *testing.T, f *billingFixture, balance int64, orders int) {
	t.Helper()
	var wallet billingWallet
	if err := f.db.First(&wallet, "user_id = ?", f.buyer.UserID).Error; err != nil || wallet.BalanceCents != balance {
		t.Fatalf("wallet=%+v %v", wallet, err)
	}
	var entries []billingEntry
	if err := f.db.Where("user_id = ?", f.buyer.UserID).Order("sequence").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	var net int64
	consumed := 0
	for i, entry := range entries {
		if entry.Sequence != uint64(i+1) || entry.BeforeCents != net || entry.AfterCents != net+entry.AmountCents {
			t.Fatalf("discontinuous ledger: %+v", entry)
		}
		net = entry.AfterCents
		if entry.Kind == "purchase" {
			consumed++
			var order billingOrder
			if entry.OrderID == nil {
				t.Fatal("purchase entry lacks order")
			}
			if err := f.db.First(&order, *entry.OrderID).Error; err != nil || order.UserID != entry.UserID || -entry.AmountCents != order.PriceCents {
				t.Fatalf("order/debit mismatch: %+v %v", order, err)
			}
		}
	}
	var count int64
	if err := f.db.Model(&billingOrder{}).Where("user_id = ?", f.buyer.UserID).Count(&count).Error; err != nil || count != int64(orders) || consumed != orders || net != balance {
		t.Fatalf("net=%d consumed=%d orders=%d err=%v", net, consumed, count, err)
	}
}

func TestBillingPurchaseInterleavesPlanEdits(t *testing.T) {
	for _, change := range []string{"price", "archive"} {
		for _, winner := range []string{"purchase", "edit"} {
			t.Run(change+"/"+winner, func(t *testing.T) {
				forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
					f := billingSetup(t, db)
					p, err := f.r.SavePlan(t.Context(), f.admin, extension.BillingPlanInput{Name: "Standard", PriceCents: 3000, DurationDays: 30, GroupID: f.base, Published: true})
					if err != nil {
						t.Fatal(err)
					}
					f.topup(t, 10000)
					var order extension.BillingOrder
					purchase := func(ctx context.Context) (e error) {
						order, e = f.r.Purchase(ctx, f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "raced-purchase"})
						return e
					}
					edit := func(ctx context.Context) error {
						_, e := f.r.SavePlan(ctx, f.admin, extension.BillingPlanInput{ID: p.ID, Version: p.Version, Name: p.Name, PriceCents: 5000, DurationDays: 30, GroupID: f.base, Published: change != "archive", Archived: change == "archive"})
						return e
					}
					if winner == "purchase" {
						a, b := billingInterleave(t, db, "billing_plans", 0, nil, purchase, edit)
						if a != nil || b != nil || order.PriceCents != 3000 || order.PlanVersion != p.Version {
							t.Fatalf("purchase first: %+v %v %v", order, a, b)
						}
						billingAssertAccounts(t, f, 7000, 1)
					} else {
						a, b := billingInterleave(t, db, "billing_plans", 0, nil, edit, purchase)
						if a != nil || !errors.Is(b, extension.ErrBillingConflict) {
							t.Fatalf("edit first: %v %v", a, b)
						}
						billingAssertAccounts(t, f, 10000, 0)
					}
					if _, err = f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "stale-after-edit"}); !errors.Is(err, extension.ErrBillingConflict) {
						t.Fatalf("edited terms accepted: %v", err)
					}
				})
			})
		}
	}
}

func TestBillingPurchaseInterleavesManualDebit(t *testing.T) {
	for _, winner := range []string{"purchase", "adjustment"} {
		t.Run(winner, func(t *testing.T) {
			forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
				f := billingSetup(t, db)
				p, err := f.r.SavePlan(t.Context(), f.admin, extension.BillingPlanInput{Name: "Standard", PriceCents: 3000, DurationDays: 30, GroupID: f.base, Published: true})
				if err != nil {
					t.Fatal(err)
				}
				f.topup(t, 10000)
				purchase := func(ctx context.Context) error {
					_, e := f.r.Purchase(ctx, f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "raced-purchase"})
					return e
				}
				debit := func(ctx context.Context) error {
					_, e := f.r.Adjust(ctx, f.admin, extension.BillingAdjustment{UserID: f.buyer.UserID, AmountCents: -8000, Reason: "manual correction", RequestID: "raced-debit"})
					return e
				}
				first, second := purchase, debit
				if winner == "adjustment" {
					first, second = debit, purchase
				}
				a, b := billingInterleave(t, db, "users", f.buyer.UserID, nil, first, second)
				if a != nil || !errors.Is(b, extension.ErrBillingInsufficient) {
					t.Fatalf("%s first: %v %v", winner, a, b)
				}
				if winner == "purchase" {
					billingAssertAccounts(t, f, 7000, 1)
				} else {
					billingAssertAccounts(t, f, 2000, 0)
				}
			})
		})
	}
}

func TestBillingPurchaseInterleavesExpiry(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		original, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "first"})
		if err != nil {
			t.Fatal(err)
		}
		f.relogin(t)
		f.now = original.ExpiresAt.Add(-time.Nanosecond)
		renew := func(ctx context.Context) error {
			_, e := f.r.Purchase(ctx, f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "crossed-renewal"})
			return e
		}
		expire := func(ctx context.Context) error { return f.r.Expire(ctx, f.buyer.UserID) }
		a, b := billingInterleave(t, db, "billing_plans", 0, func() { f.now = original.ExpiresAt }, renew, expire)
		if !errors.Is(a, model.ErrUnauthenticated) || b != nil {
			t.Fatalf("renewal/expiry race: %v %v", a, b)
		}
		billingAssertAccounts(t, f, 7000, 1)
		var owner model.User
		var lease billingMembership
		if err = db.First(&owner, f.buyer.UserID).Error; err != nil || owner.GroupID != f.base || owner.AuthVersion != 2 {
			t.Fatalf("base group restoration: %+v %v", owner, err)
		}
		if err = db.First(&lease, "user_id = ?", f.buyer.UserID).Error; err != nil || lease.Status != "expired" || lease.BaseGroupID != f.base || !lease.ExpiresAt.Equal(original.ExpiresAt) {
			t.Fatalf("lease extended or rebased: %+v %v", lease, err)
		}
		var count int64
		if err = db.Model(&model.Token{}).Where("user_id = ?", f.buyer.UserID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("old tokens survived: %d %v", count, err)
		}
	})
}
