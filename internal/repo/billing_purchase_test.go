package repo

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func (f *billingFixture) relogin(t *testing.T) {
	t.Helper()
	var u model.User
	if err := f.db.First(&u, "id = ?", f.buyer.UserID).Error; err != nil {
		t.Fatal(err)
	}
	f.buyer = billingActor(t, f.db, u)
}
func TestBillingPurchaseAtomicityAndReplay(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		input := extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "order-1"}
		order, err := f.r.Purchase(t.Context(), f.buyer, input)
		if err != nil || !order.RequiresLogin || order.PriceCents != 3000 || !order.ExpiresAt.Equal(f.now.AddDate(0, 0, 30)) {
			t.Fatalf("order=%+v err=%v", order, err)
		}
		var u model.User
		db.First(&u, "id = ?", f.buyer.UserID)
		if u.GroupID != f.paid || u.AuthVersion != 1 {
			t.Fatalf("user=%+v", u)
		}
		var remaining int64
		db.Model(&model.Token{}).Where("user_id = ?", u.ID).Count(&remaining)
		if remaining != 0 {
			t.Fatal("old tokens survived group change")
		}
		f.relogin(t)
		replay, err := f.r.Purchase(t.Context(), f.buyer, input)
		if err != nil || replay.ID != order.ID {
			t.Fatalf("replay=%+v %v", replay, err)
		}
		view, err := f.r.Overview(t.Context(), f.buyer)
		if err != nil || view.Wallet.BalanceCents != 7000 {
			t.Fatalf("balance=%+v %v", view, err)
		}
		input.PlanVersion++
		if _, err = f.r.Purchase(t.Context(), f.buyer, input); !errors.Is(err, extension.ErrBillingConflict) {
			t.Fatalf("replay mismatch=%v", err)
		}
	})
}
func TestBillingPurchaseRollback(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		injected := errors.New("injected ledger failure")
		if err := db.Callback().Create().After("gorm:create").Register("billing-fail", func(tx *gorm.DB) {
			if tx.Statement.Table == "billing_entries" {
				_ = tx.AddError(injected)
			}
		}); err != nil {
			t.Fatal(err)
		}
		_, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "rollback"})
		if removeErr := db.Callback().Create().Remove("billing-fail"); removeErr != nil {
			t.Fatal(removeErr)
		}
		if !errors.Is(err, injected) {
			t.Fatalf("failure=%v", err)
		}
		view, err := f.r.Overview(t.Context(), f.buyer)
		if err != nil || view.Wallet.BalanceCents != 10000 || view.Membership != nil {
			t.Fatalf("partial=%+v %v", view, err)
		}
		orders, _ := f.r.Orders(t.Context(), f.buyer, extension.BillingFilter{Page: 1, Size: 20})
		if orders.Total != 0 {
			t.Fatal("failed purchase left order")
		}
		var u model.User
		db.First(&u, "id = ?", f.buyer.UserID)
		if u.GroupID != f.base || u.AuthVersion != 0 {
			t.Fatal("failed purchase changed authorization")
		}
	})
}
func TestBillingConcurrentPurchases(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		// Same-group purchases retain their session, isolating wallet concurrency.
		p, err := f.r.SavePlan(t.Context(), f.admin, extension.BillingPlanInput{ID: p.ID, Version: p.Version, Name: p.Name, PriceCents: p.PriceCents, DurationDays: 30, GroupID: f.base, Published: true})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for i := range 8 {
			wg.Go(func() {
				_, e := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: fmt.Sprintf("concurrent-%d", i)})
				results <- e
			})
		}
		wg.Wait()
		close(results)
		success := 0
		for e := range results {
			if e == nil {
				success++
			} else if !errors.Is(e, extension.ErrBillingInsufficient) {
				t.Fatal(e)
			}
		}
		view, err := f.r.Overview(t.Context(), f.buyer)
		if err != nil || success != 3 || view.Wallet.BalanceCents != 1000 || !view.Membership.ExpiresAt.Equal(f.now.AddDate(0, 0, 90)) {
			t.Fatalf("success=%d view=%+v err=%v", success, view, err)
		}
	})
}
func TestBillingRenewAndExactExpiry(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		first, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "first"})
		if err != nil {
			t.Fatal(err)
		}
		f.relogin(t)
		f.now = f.now.AddDate(0, 0, 10)
		renewed, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "renew"})
		if err != nil || renewed.RequiresLogin || !renewed.ExpiresAt.Equal(first.ExpiresAt.AddDate(0, 0, 30)) {
			t.Fatalf("renewed=%+v %v", renewed, err)
		}
		f.now = renewed.ExpiresAt.Add(-time.Nanosecond)
		if err = f.r.Expire(t.Context(), f.buyer.UserID); err != nil {
			t.Fatal(err)
		}
		view, _ := f.r.Overview(t.Context(), f.buyer)
		if view.Membership.Status != "active" {
			t.Fatal("expired early")
		}
		f.now = renewed.ExpiresAt
		if err = f.r.Expire(t.Context(), f.buyer.UserID); err != nil {
			t.Fatal(err)
		}
		var u model.User
		db.First(&u, "id = ?", f.buyer.UserID)
		if u.GroupID != f.base {
			t.Fatalf("expiry group=%d", u.GroupID)
		}
		f.relogin(t)
		view, err = f.r.Overview(t.Context(), f.buyer)
		if err != nil || view.Membership.Status != "expired" || view.Wallet.BalanceCents != 4000 {
			t.Fatalf("expired=%+v %v", view, err)
		}
		if err = f.r.Expire(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
	})
}
func TestBillingAdminOverrideAndGroupReferences(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		admin, _ := NewAdminRepository(t.Context(), db)
		if err := admin.DeleteGroup(t.Context(), p.GroupID); !errors.Is(err, model.ErrStillReferenced) {
			t.Fatalf("referenced group=%v", err)
		}
		order, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "first"})
		if err != nil {
			t.Fatal(err)
		}
		third := model.Group{Name: "Manually assigned", AllowedExts: []string{}}
		if err = db.Create(&third).Error; err != nil {
			t.Fatal(err)
		}
		if _, err = admin.UpdateAdminUser(t.Context(), f.admin.UserID, f.buyer.UserID, model.UserChanges{GroupID: &third.ID}); err != nil {
			t.Fatal(err)
		}
		f.now = order.ExpiresAt.Add(time.Second)
		if err = f.r.Expire(t.Context(), 0); err != nil {
			t.Fatal(err)
		}
		var u model.User
		db.First(&u, "id = ?", f.buyer.UserID)
		if u.GroupID != third.ID {
			t.Fatal("expiry undid admin override")
		}
		f.relogin(t)
		view, _ := f.r.Overview(t.Context(), f.buyer)
		if view.Membership.Status != "overridden" {
			t.Fatal("override not recorded")
		}
	})
}
func TestBillingExpiryRejectsRecentlyUsedToken(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		if _, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "first"}); err != nil {
			t.Fatal(err)
		}
		f.relogin(t)
		past := time.Now().Add(-time.Second)
		if err := db.Model(&billingMembership{}).Where("user_id = ?", f.buyer.UserID).Updates(map[string]any{"starts_at": past.Add(-time.Hour), "expires_at": past}).Error; err != nil {
			t.Fatal(err)
		}
		tokens, _ := NewTokenRepository(t.Context(), db)
		if _, err := tokens.FindToken(t.Context(), f.buyer.TokenID); !errors.Is(err, model.ErrNotFound) {
			t.Fatalf("expired token=%v", err)
		}
		var u model.User
		db.First(&u, "id = ?", f.buyer.UserID)
		if u.GroupID != f.base {
			t.Fatal("expiry was rolled back")
		}
	})
}
func TestBillingUploadGrantCannotCrossExpiry(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		order, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "first"})
		if err != nil {
			t.Fatal(err)
		}
		f.relogin(t)
		var u model.User
		db.First(&u, "id = ?", f.buyer.UserID)
		f.now = order.ExpiresAt
		err = db.Transaction(func(tx *gorm.DB) error {
			return validateGrant(t.Context(), tx, u, model.TokenGrant{UserID: u.ID, ExpectedPasswordHash: u.PasswordHash, SourceTokenID: f.buyer.TokenID, At: order.CreatedAt, Clock: func() time.Time { return f.now }})
		})
		if !errors.Is(err, model.ErrUnauthenticated) {
			t.Fatalf("grant crossed expiry=%v", err)
		}
	})
}
func TestBillingBackupRoundTrip(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		credit := f.topup(t, 10000)
		if _, err := f.r.Reverse(t.Context(), f.admin, extension.BillingReversal{EntryID: credit.ID, Reason: "correct", RequestID: "reverse"}); err != nil {
			t.Fatal(err)
		}
		f.topup(t, 20000)
		if _, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "purchase"}); err != nil {
			t.Fatal(err)
		}
		backups, _ := NewBackupRepository(t.Context(), db)
		var before, after bytes.Buffer
		stats, err := backups.Dump(t.Context(), &before)
		if err != nil {
			t.Fatal(err)
		}
		if err = backups.Restore(t.Context(), stats.Driver, stats.SchemaVersion, bytes.NewReader(before.Bytes())); err != nil {
			t.Fatal(err)
		}
		if _, err = backups.Dump(t.Context(), &after); err != nil {
			t.Fatal(err)
		}
		if before.String() != after.String() {
			t.Fatal("billing backup differs after restore")
		}
	})
}

func TestBillingPurchaseCannotOutliveSourceSession(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		expires := f.now.Add(time.Hour)
		if err := db.Model(&model.Token{}).Where("id = ?", f.buyer.TokenID).Update("expires_at", expires).Error; err != nil {
			t.Fatal(err)
		}
		// Model a request waiting for the target group lock until its session expires.
		if err := db.Callback().Query().After("gorm:query").Register("billing-session-deadline", func(tx *gorm.DB) {
			if tx.Statement.Table == "groups" {
				f.now = expires
			}
		}); err != nil {
			t.Fatal(err)
		}
		_, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "expired-session"})
		if removeErr := db.Callback().Query().Remove("billing-session-deadline"); removeErr != nil {
			t.Fatal(removeErr)
		}
		if !errors.Is(err, model.ErrUnauthenticated) {
			t.Fatalf("session expired before debit: %v", err)
		}
		var wallet billingWallet
		if e := db.First(&wallet, "user_id = ?", f.buyer.UserID).Error; e != nil {
			t.Fatal(e)
		}
		if wallet.BalanceCents != 10000 {
			t.Fatal("expired session was charged")
		}
	})
}

func TestBillingPurchaseCrossingMembershipExpiryKeepsBaseGroup(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		f.topup(t, 10000)
		first, err := f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "first"})
		if err != nil {
			t.Fatal(err)
		}
		f.relogin(t)
		if err = db.Callback().Query().After("gorm:query").Register("billing-lease-deadline", func(tx *gorm.DB) {
			if tx.Statement.Table == "groups" {
				f.now = first.ExpiresAt
			}
		}); err != nil {
			t.Fatal(err)
		}
		_, err = f.r.Purchase(t.Context(), f.buyer, extension.BillingPurchase{PlanID: p.ID, PlanVersion: p.Version, RequestID: "crossing"})
		if removeErr := db.Callback().Query().Remove("billing-lease-deadline"); removeErr != nil {
			t.Fatal(removeErr)
		}
		if !errors.Is(err, model.ErrUnauthenticated) {
			t.Fatalf("crossed lease expiry: %v", err)
		}
		var user model.User
		db.First(&user, "id = ?", f.buyer.UserID)
		if user.GroupID != f.base {
			t.Fatal("paid group became the new permanent base")
		}
		var wallet billingWallet
		db.First(&wallet, "user_id = ?", f.buyer.UserID)
		if wallet.BalanceCents != 7000 {
			t.Fatal("crossing expiry charged twice")
		}
	})
}
