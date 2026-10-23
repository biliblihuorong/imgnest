package repo

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

type billingFixture struct {
	r            *BillingRepository
	db           *gorm.DB
	now          time.Time
	admin, buyer extension.BillingActor
	base, paid   uint64
}

func billingActor(t *testing.T, db *gorm.DB, u model.User) extension.BillingActor {
	t.Helper()
	token := model.Token{UserID: u.ID, Name: "billing test", TokenHash: fmt.Sprintf("billing-%d-%d", u.ID, time.Now().UnixNano()), Kind: model.TokenKindWeb, Abilities: []string{"*"}}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	return extension.BillingActor{UserID: u.ID, TokenID: token.ID, AuthVersion: u.AuthVersion}
}
func billingSetup(t *testing.T, db *gorm.DB) *billingFixture {
	t.Helper()
	f := &billingFixture{db: db, now: time.Now().UTC().Truncate(time.Second)}
	users, settings := repositories(t, db)
	f.base, _ = settings.DefaultGroupID(t.Context())
	admin, err := users.CreateUser(t.Context(), testUser("billing-admin", "ba@example.com", f.base))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&admin).Update("role", model.UserRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	admin.Role = model.UserRoleAdmin
	buyer, err := users.CreateUser(t.Context(), testUser("billing-buyer", "bb@example.com", f.base))
	if err != nil {
		t.Fatal(err)
	}
	f.admin = billingActor(t, db, admin)
	f.buyer = billingActor(t, db, buyer)
	group := model.Group{Name: "Paid", AllowedExts: []string{"png"}, CapacityBytes: 1 << 30}
	if err = db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	f.paid = group.ID
	f.r, err = NewBillingRepository(t.Context(), db, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *billingFixture) plan(t *testing.T) extension.BillingPlan {
	t.Helper()
	p, err := f.r.SavePlan(t.Context(), f.admin, extension.BillingPlanInput{Name: "Standard", PriceCents: 3000, DurationDays: 30, GroupID: f.paid, Published: true})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f *billingFixture) topup(t *testing.T, amount int64) extension.BillingEntry {
	t.Helper()
	e, err := f.r.Adjust(t.Context(), f.admin, extension.BillingAdjustment{UserID: f.buyer.UserID, AmountCents: amount, Reason: "manual deposit", RequestID: fmt.Sprintf("topup-%d", amount)})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestBillingWalletLedgerAndReversal(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		initial, err := f.r.Overview(t.Context(), f.buyer)
		if err != nil || initial.Wallet.BalanceCents != 0 {
			t.Fatalf("initial=%+v %v", initial, err)
		}
		credit := f.topup(t, 10000)
		debit, err := f.r.Adjust(t.Context(), f.admin, extension.BillingAdjustment{UserID: f.buyer.UserID, AmountCents: -1200, Reason: "correction", RequestID: "debit-1"})
		if err != nil || debit.BeforeCents != 10000 || debit.AfterCents != 8800 {
			t.Fatalf("debit=%+v %v", debit, err)
		}
		reversal, err := f.r.Reverse(t.Context(), f.admin, extension.BillingReversal{EntryID: debit.ID, Reason: "undo mistaken debit", RequestID: "reverse-1"})
		if err != nil || reversal.AmountCents != 1200 || reversal.AfterCents != 10000 {
			t.Fatalf("reverse=%+v %v", reversal, err)
		}
		if _, err = f.r.Reverse(t.Context(), f.admin, extension.BillingReversal{EntryID: debit.ID, Reason: "again", RequestID: "reverse-2"}); !errors.Is(err, extension.ErrBillingConflict) {
			t.Fatalf("double reverse=%v", err)
		}
		history, err := f.r.Ledger(t.Context(), f.buyer, extension.BillingFilter{Page: 1, Size: 20})
		if err != nil || history.Total != 3 || history.Items[2].ID != credit.ID {
			t.Fatalf("history=%+v %v", history, err)
		}
		if err = db.Delete(&model.User{}, f.buyer.UserID).Error; err == nil {
			t.Fatal("deleting owner erased funds")
		}
	})
}
func TestBillingRejectsInvalidAmountsAndActors(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		for i, n := range []int64{0, -1, extension.MaxBillingCents + 1} {
			_, err := f.r.Adjust(t.Context(), f.admin, extension.BillingAdjustment{UserID: f.buyer.UserID, AmountCents: n, Reason: "test", RequestID: fmt.Sprint("bad-", i)})
			if err == nil {
				t.Fatalf("accepted %d", n)
			}
		}
		if _, err := f.r.Adjust(t.Context(), f.buyer, extension.BillingAdjustment{UserID: f.buyer.UserID, AmountCents: 10, Reason: "fraud", RequestID: "fraud"}); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("non-admin=%v", err)
		}
		f.topup(t, 100)
		if _, err := f.r.Adjust(t.Context(), f.admin, extension.BillingAdjustment{UserID: f.buyer.UserID, AmountCents: 1, Reason: " ", RequestID: "empty-reason"}); err == nil {
			t.Fatal("accepted blank reason")
		}
		if err := db.Model(&model.Token{}).Where("id = ?", f.admin.TokenID).Update("kind", model.TokenKindAPI).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.r.Overview(t.Context(), f.admin); !errors.Is(err, model.ErrForbidden) {
			t.Fatalf("API token=%v", err)
		}
	})
}
func TestBillingIdempotencyAndConcurrentAdjustments(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		f.topup(t, 10000)
		entry := extension.BillingAdjustment{UserID: f.buyer.UserID, AmountCents: 100, Reason: "same deposit", RequestID: "repeat"}
		var wg sync.WaitGroup
		results := make(chan error, 12)
		for range 12 {
			wg.Go(func() { _, err := f.r.Adjust(t.Context(), f.admin, entry); results <- err })
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		overview, err := f.r.Overview(t.Context(), f.buyer)
		if err != nil || overview.Wallet.BalanceCents != 10100 {
			t.Fatalf("overview=%+v %v", overview, err)
		}
		entry.AmountCents = 200
		if _, err = f.r.Adjust(t.Context(), f.admin, entry); !errors.Is(err, extension.ErrBillingConflict) {
			t.Fatalf("mismatch=%v", err)
		}
		ledger, _ := f.r.Ledger(t.Context(), f.buyer, extension.BillingFilter{Page: 1, Size: 100})
		if ledger.Total != 2 {
			t.Fatalf("ledger=%+v", ledger)
		}
	})
}
func TestBillingPlanVersionAndSoldTerms(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		f := billingSetup(t, db)
		p := f.plan(t)
		input := extension.BillingPlanInput{ID: p.ID, Version: p.Version, Name: p.Name, PriceCents: 4000, DurationDays: p.DurationDays, GroupID: p.GroupID, Published: true}
		next, err := f.r.SavePlan(t.Context(), f.admin, input)
		if err != nil || next.Version != p.Version+1 {
			t.Fatalf("updated=%+v %v", next, err)
		}
		if _, err = f.r.SavePlan(t.Context(), f.admin, input); !errors.Is(err, extension.ErrBillingConflict) {
			t.Fatalf("stale update=%v", err)
		}
		p = next
		f.topup(t, 10000)
		order := billingOrder{UserID: f.buyer.UserID, Username: "billing-buyer", PlanID: p.ID, PlanName: p.Name, PlanVersion: p.Version, PriceCents: p.PriceCents, DurationDays: p.DurationDays, GroupID: p.GroupID, ExpiresAt: f.now.AddDate(0, 0, 30), RequestID: "fixture-order", CreatedAt: f.now}
		if err = db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		input.Version = p.Version
		input.DurationDays = 60
		if _, err = f.r.SavePlan(t.Context(), f.admin, input); !errors.Is(err, extension.ErrBillingConflict) {
			t.Fatalf("sold terms=%v", err)
		}
	})
}
