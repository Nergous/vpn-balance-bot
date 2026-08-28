package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestSetMonthlyFeeDoesNotModifyLedgerHistory(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	nextChargeOn, err := domain.NewDate(2026, time.September, 1)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	user, err := store.CreateUser(ctx, domain.User{
		DisplayName: "Alice", MonthlyFeeMinor: 100000, Currency: "RUB",
		BillingAnchorDay: 1, NextChargeOn: nextChargeOn,
		Status: domain.UserStatusActive, CreatedAt: createdAt, UpdatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO ledger_entries (user_id, kind, amount_minor, occurred_at, created_at)
		VALUES (?, 'payment', 500000, ?, ?)
	`, user.ID, createdAt.Unix(), createdAt.Unix()); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SetMonthlyFee(ctx, user.ID, 150000, createdAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	var amountMinor int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT amount_minor FROM ledger_entries WHERE user_id = ?
	`, user.ID).Scan(&amountMinor); err != nil {
		t.Fatal(err)
	}
	if amountMinor != 500000 {
		t.Fatalf("ledger amount = %d, want 500000", amountMinor)
	}
}
