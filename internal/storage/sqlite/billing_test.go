package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/billing"
)

var _ billing.Storage = (*Store)(nil)

func TestCatchUpChargesOneDuePeriodIdempotently(t *testing.T) {
	ctx := context.Background()
	store := newBillingStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	first := billingDate(t, 2026, time.September, 1)
	user := createBillingUser(t, store, ctx, 1, 100000, 1, first, domain.UserStatusActive, now)
	service, err := billing.New(store)
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CatchUp(ctx, first)
	if err != nil || created != 1 {
		t.Fatalf("CatchUp() = %d, %v", created, err)
	}
	updated, err := store.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.NextChargeOn.String(); got != "2026-10-01" {
		t.Fatalf("next_charge_on = %s", got)
	}
	entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Kind != domain.LedgerKindSubscriptionCharge || entries[0].AmountMinor != -100000 || entries[0].BillingPeriodOn == nil || *entries[0].BillingPeriodOn != first {
		t.Fatalf("entries = %#v", entries)
	}

	created, err = service.CatchUp(ctx, first)
	if err != nil || created != 0 {
		t.Fatalf("repeat CatchUp() = %d, %v", created, err)
	}
	entries, err = store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries after repeat = %#v, %v", entries, err)
	}
}

func TestCatchUpProcessesMissedMonthsAndAnchorDays(t *testing.T) {
	ctx := context.Background()
	store := newBillingStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	first := billingDate(t, 2026, time.January, 31)
	user := createBillingUser(t, store, ctx, 1, 100000, 31, first, domain.UserStatusActive, now)
	service, err := billing.New(store)
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CatchUp(ctx, billingDate(t, 2026, time.March, 31))
	if err != nil || created != 3 {
		t.Fatalf("CatchUp() = %d, %v, want 3", created, err)
	}
	updated, err := store.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.NextChargeOn.String(); got != "2026-04-30" {
		t.Fatalf("next_charge_on = %s, want 2026-04-30", got)
	}
	entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
}

func TestCatchUpPreservesAnchorDays29To31(t *testing.T) {
	for _, anchorDay := range []int{29, 30, 31} {
		wantDay := map[int]string{29: "29", 30: "30", 31: "31"}[anchorDay]
		t.Run("anchor-"+wantDay, func(t *testing.T) {
			ctx := context.Background()
			store := newBillingStore(t, ctx)
			now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
			first := billingDate(t, 2026, time.January, anchorDay)
			user := createBillingUser(t, store, ctx, 1, 100000, anchorDay, first, domain.UserStatusActive, now)
			service, err := billing.New(store)
			if err != nil {
				t.Fatal(err)
			}
			if created, err := service.CatchUp(ctx, billingDate(t, 2026, time.February, 28)); err != nil || created != 2 {
				t.Fatalf("CatchUp() = %d, %v", created, err)
			}
			updated, err := store.UserByID(ctx, user.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "2026-03-" + wantDay
			if updated.NextChargeOn.String() != want {
				t.Fatalf("next_charge_on = %s, want %s", updated.NextChargeOn, want)
			}
		})
	}
}

func TestCatchUpUsesFeeCurrentForEachPeriod(t *testing.T) {
	ctx := context.Background()
	store := newBillingStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	first := billingDate(t, 2026, time.January, 15)
	user := createBillingUser(t, store, ctx, 1, 100000, 15, first, domain.UserStatusActive, now)
	service, err := billing.New(store)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := service.CatchUp(ctx, first); err != nil || created != 1 {
		t.Fatalf("first CatchUp() = %d, %v", created, err)
	}
	if _, err := store.SetMonthlyFee(ctx, user.ID, 150000, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if created, err := service.CatchUp(ctx, billingDate(t, 2026, time.February, 15)); err != nil || created != 1 {
		t.Fatalf("second CatchUp() = %d, %v", created, err)
	}
	entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].AmountMinor != -150000 || entries[1].AmountMinor != -100000 {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestCatchUpSkipsPausedAndDisabledUsers(t *testing.T) {
	ctx := context.Background()
	store := newBillingStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	due := billingDate(t, 2026, time.September, 1)
	paused := createBillingUser(t, store, ctx, 1, 100000, 1, due, domain.UserStatusPaused, now)
	disabled := createBillingUser(t, store, ctx, 2, 100000, 1, due, domain.UserStatusDisabled, now)
	service, err := billing.New(store)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := service.CatchUp(ctx, due); err != nil || created != 0 {
		t.Fatalf("CatchUp() = %d, %v", created, err)
	}
	for _, user := range []domain.User{paused, disabled} {
		entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
		if err != nil || len(entries) != 0 {
			t.Fatalf("entries for %d = %#v, %v", user.ID, entries, err)
		}
	}
}

func TestCatchUpIsIdempotentUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	store := newBillingStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	due := billingDate(t, 2026, time.September, 1)
	user := createBillingUser(t, store, ctx, 1, 100000, 1, due, domain.UserStatusActive, now)
	first, _ := billing.New(store)
	second, _ := billing.New(store)

	start := make(chan struct{})
	results := make(chan struct {
		created int
		err     error
	}, 2)
	var group sync.WaitGroup
	for _, service := range []*billing.Service{first, second} {
		group.Add(1)
		go func(service *billing.Service) {
			defer group.Done()
			<-start
			created, err := service.CatchUp(ctx, due)
			results <- struct {
				created int
				err     error
			}{created, err}
		}(service)
	}
	close(start)
	group.Wait()
	close(results)

	total := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("CatchUp() error = %v", result.err)
		}
		total += result.created
	}
	if total != 1 {
		t.Fatalf("created total = %d, want 1", total)
	}
	entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %#v, %v", entries, err)
	}
}

func TestSubscriptionConflictRollsBackDateAdvance(t *testing.T) {
	ctx := context.Background()
	store := newBillingStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	due := billingDate(t, 2026, time.September, 1)
	user := createBillingUser(t, store, ctx, 1, 100000, 1, due, domain.UserStatusActive, now)
	if _, err := store.CreateLedgerEntry(ctx, domain.LedgerEntry{UserID: user.ID, Kind: domain.LedgerKindSubscriptionCharge, AmountMinor: -100000, OccurredAt: now, BillingPeriodOn: &due, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	_, created, err := store.ReserveSubscriptionCharge(ctx, billing.ReserveSubscriptionChargeParams{UserID: user.ID, BillingPeriodOn: due, OccurredAt: now, UpdatedAt: now})
	if err != nil || created {
		t.Fatalf("ReserveSubscriptionCharge() = created %t, err %v", created, err)
	}
	updated, err := store.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.NextChargeOn != due {
		t.Fatalf("next_charge_on = %s, want %s", updated.NextChargeOn, due)
	}
}

func TestCatchUpStaysIdempotentAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "billing.db")
	store, err := New(ctx, path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	due := billingDate(t, 2026, time.September, 1)
	user := createBillingUser(t, store, ctx, 1, 100000, 1, due, domain.UserStatusActive, now)
	service, _ := billing.New(store)
	if created, err := service.CatchUp(ctx, due); err != nil || created != 1 {
		t.Fatalf("first CatchUp() = %d, %v", created, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(ctx, path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service, _ = billing.New(reopened)
	if created, err := service.CatchUp(ctx, due); err != nil || created != 0 {
		t.Fatalf("second CatchUp() = %d, %v", created, err)
	}
	entries, err := reopened.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries after reopen = %#v, %v", entries, err)
	}
}

func newBillingStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

func createBillingUser(t *testing.T, store *Store, ctx context.Context, index int, fee domain.AmountMinor, anchor int, next domain.Date, status domain.UserStatus, now time.Time) domain.User {
	t.Helper()
	user, err := store.CreateUser(ctx, domain.User{DisplayName: "Billing user", MonthlyFeeMinor: fee, Currency: "RUB", BillingAnchorDay: anchor, NextChargeOn: next, Status: status, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func billingDate(t *testing.T, year int, month time.Month, day int) domain.Date {
	t.Helper()
	date, err := domain.NewDate(year, month, day)
	if err != nil {
		t.Fatal(err)
	}
	return date
}

func TestUsersDueForChargeIgnoresFutureDate(t *testing.T) {
	ctx := context.Background()
	store := newBillingStore(t, ctx)
	now := time.Now().UTC()
	future := billingDate(t, 2026, time.December, 1)
	createBillingUser(t, store, ctx, 1, 100000, 1, future, domain.UserStatusActive, now)
	users, err := store.UsersDueForCharge(ctx, billingDate(t, 2026, time.November, 30))
	if err != nil || len(users) != 0 {
		t.Fatalf("UsersDueForCharge() = %#v, %v", users, err)
	}
}

func TestReserveSubscriptionChargeReturnsNoErrorForStaleUser(t *testing.T) {
	store := newBillingStore(t, context.Background())
	date := billingDate(t, 2026, time.September, 1)
	_, created, err := store.ReserveSubscriptionCharge(context.Background(), billing.ReserveSubscriptionChargeParams{UserID: 999, BillingPeriodOn: date, OccurredAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil || created {
		t.Fatalf("ReserveSubscriptionCharge() = created %t, err %v", created, err)
	}
}
