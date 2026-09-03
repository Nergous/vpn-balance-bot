package sqlite

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

func TestLedgerBalanceAndRecentEntries(t *testing.T) {
	ctx := context.Background()
	store := newLedgerStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	user := createLedgerUser(t, store, ctx, 1, now)

	for index := 0; index < 11; index++ {
		amount := domain.AmountMinor(index + 1)
		if _, err := store.CreateLedgerEntry(ctx, newLedgerEntry(user.ID, domain.LedgerKindPayment, amount, now.Add(time.Duration(index)*time.Second))); err != nil {
			t.Fatal(err)
		}
	}

	balance, err := store.Balance(ctx, user.ID)
	if err != nil || balance != 66 {
		t.Fatalf("Balance() = %d, %v, want 66", balance, err)
	}
	entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 10 {
		t.Fatalf("LastLedgerEntries() count = %d, want 10", len(entries))
	}
	if entries[0].AmountMinor != 11 || entries[9].AmountMinor != 2 {
		t.Fatalf("recent entries = %#v", entries)
	}

	if _, err := store.Balance(ctx, 999); !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("Balance(unknown) error = %v", err)
	}
}

func TestReverseLedgerEntry(t *testing.T) {
	ctx := context.Background()
	store := newLedgerStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	user := createLedgerUser(t, store, ctx, 1, now)
	original, err := store.CreateLedgerEntry(ctx, newLedgerEntry(user.ID, domain.LedgerKindPayment, 1000, now))
	if err != nil {
		t.Fatal(err)
	}

	reversal, err := store.ReverseLedgerEntry(ctx, account.ReverseLedgerEntryRecord{
		UserID: user.ID, EntryID: original.ID, CreatedByTelegramID: 7, Note: "mistake", OccurredAt: now.Add(time.Second), CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if reversal.Kind != domain.LedgerKindReversal || reversal.AmountMinor != -1000 || reversal.ReversesEntryID == nil || *reversal.ReversesEntryID != original.ID {
		t.Fatalf("reversal = %#v", reversal)
	}
	balance, err := store.Balance(ctx, user.ID)
	if err != nil || balance != 0 {
		t.Fatalf("Balance() = %d, %v", balance, err)
	}

	_, err = store.ReverseLedgerEntry(ctx, account.ReverseLedgerEntryRecord{UserID: user.ID, EntryID: original.ID, CreatedByTelegramID: 7, Note: "again", OccurredAt: now.Add(2 * time.Second), CreatedAt: now.Add(2 * time.Second)})
	if !errors.Is(err, account.ErrLedgerEntryAlreadyReversed) {
		t.Fatalf("second reversal error = %v", err)
	}
	_, err = store.ReverseLedgerEntry(ctx, account.ReverseLedgerEntryRecord{UserID: user.ID, EntryID: reversal.ID, CreatedByTelegramID: 7, Note: "reverse reversal", OccurredAt: now.Add(3 * time.Second), CreatedAt: now.Add(3 * time.Second)})
	if !errors.Is(err, account.ErrCannotReverseReversal) {
		t.Fatalf("reversal-of-reversal error = %v", err)
	}
	_, err = store.ReverseLedgerEntry(ctx, account.ReverseLedgerEntryRecord{UserID: user.ID + 1, EntryID: original.ID, CreatedByTelegramID: 7, Note: "wrong user", OccurredAt: now, CreatedAt: now})
	if !errors.Is(err, account.ErrReversalUserMismatch) {
		t.Fatalf("wrong-user reversal error = %v", err)
	}
}

func TestCreateLedgerEntryRejectsCumulativeBalanceOverflow(t *testing.T) {
	ctx := context.Background()
	store := newLedgerStore(t, ctx)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user := createLedgerUser(t, store, ctx, 1, now)
	if _, err := store.CreateLedgerEntry(ctx, newLedgerEntry(user.ID, domain.LedgerKindOpeningBalance, domain.AmountMinor(math.MaxInt64), now)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateLedgerEntry(ctx, newLedgerEntry(user.ID, domain.LedgerKindAdjustment, 1, now.Add(time.Second))); !errors.Is(err, domain.ErrAmountOverflow) {
		t.Fatalf("overflow entry error = %v", err)
	}
	entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries after overflow = %d, %v", len(entries), err)
	}
}

func TestReverseLedgerEntryRollsBackOnOverflow(t *testing.T) {
	ctx := context.Background()
	store := newLedgerStore(t, ctx)
	now := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	user := createLedgerUser(t, store, ctx, 1, now)
	original, err := store.CreateLedgerEntry(ctx, newLedgerEntry(user.ID, domain.LedgerKindOpeningBalance, domain.AmountMinor(math.MinInt64), now))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.ReverseLedgerEntry(ctx, account.ReverseLedgerEntryRecord{UserID: user.ID, EntryID: original.ID, CreatedByTelegramID: 7, Note: "overflow", OccurredAt: now, CreatedAt: now})
	if !errors.Is(err, domain.ErrAmountOverflow) {
		t.Fatalf("ReverseLedgerEntry() error = %v", err)
	}
	entries, err := store.LastLedgerEntries(ctx, user.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("ledger count = %d, want 1", len(entries))
	}
}

func TestLastUnreversedPaymentIgnoresRecentLimitAndReversedPayments(t *testing.T) {
	ctx := context.Background()
	store := newLedgerStore(t, ctx)
	now := time.Date(2026, time.September, 2, 10, 0, 0, 0, time.UTC)
	user := createLedgerUser(t, store, ctx, 1, now)

	olderPayment, err := store.CreateLedgerEntry(ctx, newLedgerEntry(user.ID, domain.LedgerKindPayment, 1000, now))
	if err != nil {
		t.Fatal(err)
	}
	newerPayment, err := store.CreateLedgerEntry(ctx, newLedgerEntry(user.ID, domain.LedgerKindPayment, 2000, now.Add(time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReverseLedgerEntry(ctx, account.ReverseLedgerEntryRecord{
		UserID: user.ID, EntryID: newerPayment.ID, CreatedByTelegramID: 7, Note: "reversed",
		OccurredAt: now.Add(2 * time.Second), CreatedAt: now.Add(2 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 11; index++ {
		if _, err := store.CreateLedgerEntry(ctx, newLedgerEntry(
			user.ID,
			domain.LedgerKindAdjustment,
			domain.AmountMinor(index+1),
			now.Add(time.Duration(index+3)*time.Second),
		)); err != nil {
			t.Fatal(err)
		}
	}

	payment, found, err := store.LastUnreversedPayment(ctx, user.ID)
	if err != nil || !found || payment.ID != olderPayment.ID {
		t.Fatalf("LastUnreversedPayment() = %#v, %t, %v; want payment %d", payment, found, err, olderPayment.ID)
	}

	if _, err := store.ReverseLedgerEntry(ctx, account.ReverseLedgerEntryRecord{
		UserID: user.ID, EntryID: olderPayment.ID, CreatedByTelegramID: 7, Note: "also reversed",
		OccurredAt: now.Add(20 * time.Second), CreatedAt: now.Add(20 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	payment, found, err = store.LastUnreversedPayment(ctx, user.ID)
	if err != nil || found || payment != (domain.LedgerEntry{}) {
		t.Fatalf("LastUnreversedPayment() after reversals = %#v, %t, %v; want zero, false, nil", payment, found, err)
	}

	if _, _, err := store.LastUnreversedPayment(ctx, 999); !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("LastUnreversedPayment(unknown) error = %v, want %v", err, account.ErrNotFound)
	}
}

func newLedgerStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

func createLedgerUser(t *testing.T, store *Store, ctx context.Context, index int, now time.Time) domain.User {
	t.Helper()
	date, err := domain.NewDate(2026, time.September, index)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, domain.User{DisplayName: "Ledger user", MonthlyFeeMinor: 100000, Currency: "RUB", BillingAnchorDay: index, NextChargeOn: date, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func newLedgerEntry(userID domain.UserID, kind domain.LedgerKind, amount domain.AmountMinor, now time.Time) domain.LedgerEntry {
	adminID := int64(7)
	return domain.LedgerEntry{UserID: userID, Kind: kind, AmountMinor: amount, OccurredAt: now, CreatedByTelegramID: &adminID, CreatedAt: now}
}
