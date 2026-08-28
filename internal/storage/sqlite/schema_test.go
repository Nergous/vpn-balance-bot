package sqlite

import (
	"context"
	"testing"
)

func TestSchemaRejectsInvalidUserValues(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t, ctx)

	tests := []struct {
		name  string
		query string
	}{
		{
			name: "zero monthly fee",
			query: `
				INSERT INTO users (
					id, display_name, monthly_fee_minor, currency, billing_anchor_day,
					next_charge_on, status, created_at, updated_at
				) VALUES (1, 'Alice', 0, 'RUB', 1, '2026-08-28', 'active', 1, 1)
			`,
		},
		{
			name: "unsupported currency",
			query: `
				INSERT INTO users (
					id, display_name, monthly_fee_minor, currency, billing_anchor_day,
					next_charge_on, status, created_at, updated_at
				) VALUES (2, 'Alice', 1000, 'USD', 1, '2026-08-28', 'active', 1, 1)
			`,
		},
		{
			name: "invalid billing anchor day",
			query: `
				INSERT INTO users (
					id, display_name, monthly_fee_minor, currency, billing_anchor_day,
					next_charge_on, status, created_at, updated_at
				) VALUES (3, 'Alice', 1000, 'RUB', 32, '2026-08-28', 'active', 1, 1)
			`,
		},
		{
			name: "invalid status",
			query: `
				INSERT INTO users (
					id, display_name, monthly_fee_minor, currency, billing_anchor_day,
					next_charge_on, status, created_at, updated_at
				) VALUES (4, 'Alice', 1000, 'RUB', 1, '2026-08-28', 'unknown', 1, 1)
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.db.ExecContext(ctx, test.query); err == nil {
				t.Fatal("invalid user was accepted")
			}
		})
	}
}

func TestSchemaEnforcesUniqueTelegramIDs(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t, ctx)
	insertUser(t, store, 1, 101, 201)

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO users (
			id, telegram_user_id, telegram_chat_id, display_name, monthly_fee_minor,
			currency, billing_anchor_day, next_charge_on, status, created_at, updated_at
		) VALUES (2, 101, 202, 'Bob', 1000, 'RUB', 1, '2026-08-28', 'active', 1, 1)
	`); err == nil {
		t.Fatal("duplicate telegram_user_id was accepted")
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO users (
			id, telegram_user_id, telegram_chat_id, display_name, monthly_fee_minor,
			currency, billing_anchor_day, next_charge_on, status, created_at, updated_at
		) VALUES (3, 102, 201, 'Carol', 1000, 'RUB', 1, '2026-08-28', 'active', 1, 1)
	`); err == nil {
		t.Fatal("duplicate telegram_chat_id was accepted")
	}
}

func TestSchemaEnforcesForeignKeys(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t, ctx)

	queries := []string{
		`INSERT INTO invite_tokens (token_hash, user_id, expires_at, created_at) VALUES ('token', 99, 2, 1)`,
		`INSERT INTO ledger_entries (id, user_id, kind, amount_minor, occurred_at, created_at) VALUES (1, 99, 'payment', 1, 1, 1)`,
		`INSERT INTO reminder_deliveries (user_id, billing_date, reminder_type, scheduled_date, status, created_at, updated_at) VALUES (99, '2026-08-28', 'manual', '2026-08-28', 'pending', 1, 1)`,
	}

	for _, query := range queries {
		if _, err := store.db.ExecContext(ctx, query); err == nil {
			t.Fatal("foreign key violation was accepted")
		}
	}
}

func TestSchemaRejectsInvalidLedgerEntries(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t, ctx)
	insertUser(t, store, 1, 101, 201)

	queries := []string{
		`INSERT INTO ledger_entries (id, user_id, kind, amount_minor, occurred_at, created_at) VALUES (1, 1, 'payment', 0, 1, 1)`,
		`INSERT INTO ledger_entries (id, user_id, kind, amount_minor, occurred_at, created_at) VALUES (2, 1, 'unknown', 1, 1, 1)`,
		`INSERT INTO ledger_entries (id, user_id, kind, amount_minor, occurred_at, created_at) VALUES (3, 1, 'subscription_charge', -1000, 1, 1)`,
		`INSERT INTO ledger_entries (id, user_id, kind, amount_minor, occurred_at, created_at) VALUES (4, 1, 'reversal', -1000, 1, 1)`,
	}

	for _, query := range queries {
		if _, err := store.db.ExecContext(ctx, query); err == nil {
			t.Fatal("invalid ledger entry was accepted")
		}
	}
}

func TestSchemaPreventsDuplicateSubscriptionChargesAndReversals(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t, ctx)
	insertUser(t, store, 1, 101, 201)

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, user_id, kind, amount_minor, occurred_at, billing_period_on, created_at
		) VALUES (1, 1, 'subscription_charge', -1000, 1, '2026-08-28', 1)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, user_id, kind, amount_minor, occurred_at, billing_period_on, created_at
		) VALUES (2, 1, 'subscription_charge', -1000, 2, '2026-08-28', 2)
	`); err == nil {
		t.Fatal("duplicate subscription charge was accepted")
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, user_id, kind, amount_minor, occurred_at, created_at)
		VALUES (3, 1, 'payment', 1000, 3, 3)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, user_id, kind, amount_minor, occurred_at, reverses_entry_id, created_at
		) VALUES (4, 1, 'reversal', -1000, 4, 3, 4)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, user_id, kind, amount_minor, occurred_at, reverses_entry_id, created_at
		) VALUES (5, 1, 'reversal', -1000, 5, 3, 5)
	`); err == nil {
		t.Fatal("second reversal was accepted")
	}
}

func TestSchemaPreventsDuplicateReminderDeliveries(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t, ctx)
	insertUser(t, store, 1, 101, 201)

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO reminder_deliveries (
			user_id, billing_date, reminder_type, scheduled_date, status, created_at, updated_at
		) VALUES (1, '2026-08-28', 'manual', '2026-08-28', 'pending', 1, 1)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO reminder_deliveries (
			user_id, billing_date, reminder_type, scheduled_date, status, created_at, updated_at
		) VALUES (1, '2026-08-28', 'manual', '2026-08-29', 'pending', 2, 2)
	`); err == nil {
		t.Fatal("duplicate reminder delivery was accepted")
	}
}

func TestSchemaRejectsInvalidReminderDeliveries(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t, ctx)
	insertUser(t, store, 1, 101, 201)

	queries := []string{
		`INSERT INTO reminder_deliveries (user_id, billing_date, reminder_type, scheduled_date, status, created_at, updated_at) VALUES (1, '2026-08-28', 'manual', '2026-08-28', 'unknown', 1, 1)`,
		`INSERT INTO reminder_deliveries (user_id, billing_date, reminder_type, scheduled_date, status, created_at, updated_at) VALUES (1, '2026-08-28', 'manual', '2026-08-28', 'sent', 1, 1)`,
	}

	for _, query := range queries {
		if _, err := store.db.ExecContext(ctx, query); err == nil {
			t.Fatal("invalid reminder delivery was accepted")
		}
	}
}

func newMigratedStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()

	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	return store
}

func insertUser(t *testing.T, store *Store, id, telegramUserID, telegramChatID int64) {
	t.Helper()

	if _, err := store.db.ExecContext(context.Background(), `
		INSERT INTO users (
			id, telegram_user_id, telegram_chat_id, display_name, monthly_fee_minor,
			currency, billing_anchor_day, next_charge_on, status, created_at, updated_at
		) VALUES (?, ?, ?, 'Alice', 1000, 'RUB', 1, '2026-08-28', 'active', 1, 1)
	`, id, telegramUserID, telegramChatID); err != nil {
		t.Fatal(err)
	}
}
