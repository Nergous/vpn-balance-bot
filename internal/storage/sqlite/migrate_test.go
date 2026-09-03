package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/Nergous/vpn-balance-bot/migrations"
)

func TestMigrateCreatesInitialSchema(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	for _, name := range []string{
		"schema_migrations",
		"schema_migration_checksums",
		"processed_telegram_updates",
		"payment_drafts",
		"users",
		"invite_tokens",
		"ledger_entries",
		"reminder_deliveries",
	} {
		if !sqliteObjectExists(t, store, ctx, "table", name) {
			t.Errorf("table %q was not created", name)
		}
	}

	for _, name := range []string{
		"idx_users_status_next_charge_on",
		"idx_invite_tokens_user_id",
		"idx_ledger_entries_user_occurred_at",
		"ux_ledger_subscription_charge_period",
		"idx_reminder_deliveries_status_scheduled_date",
		"idx_reminder_deliveries_retry",
		"idx_reminder_deliveries_pending_lease",
	} {
		if !sqliteObjectExists(t, store, ctx, "index", name) {
			t.Errorf("index %q was not created", name)
		}
	}

	var appliedAt int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT applied_at FROM schema_migrations WHERE version = ?
	`, "001_initial.sql").Scan(&appliedAt); err != nil {
		t.Fatal(err)
	}
	if appliedAt <= 0 {
		t.Fatalf("applied_at = %d, want positive Unix timestamp", appliedAt)
	}

	var integrity string
	if err := store.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}
}

func TestMigrateRejectsUnknownAppliedVersion(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, applied_at) VALUES ('999_future.sql', 1)
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); !errors.Is(err, ErrUnknownMigration) {
		t.Fatalf("Migrate() error = %v, want %v", err, ErrUnknownMigration)
	}
}

func TestMigrateRejectsChangedAppliedMigration(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE schema_migration_checksums SET checksum = ? WHERE version = ?
	`, "0000000000000000000000000000000000000000000000000000000000000000", "001_initial.sql"); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); !errors.Is(err, ErrMigrationChecksum) {
		t.Fatalf("Migrate() error = %v, want %v", err, ErrMigrationChecksum)
	}
}

func TestMigrateRejectsMissingAppliedMigrationChecksum(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		DELETE FROM schema_migration_checksums WHERE version = '001_initial.sql'
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); !errors.Is(err, ErrMigrationChecksum) {
		t.Fatalf("Migrate() error = %v, want %v", err, ErrMigrationChecksum)
	}
}

func TestApplyMigrationRollsBackWhenChecksumCannotBeRecorded(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER reject_atomic_checksum
		BEFORE INSERT ON schema_migration_checksums
		WHEN NEW.version = '999_atomic.sql'
		BEGIN
			SELECT RAISE(ABORT, 'checksum denied');
		END
	`); err != nil {
		t.Fatal(err)
	}

	err := store.applyMigration(ctx, migrations.Migration{
		Version: "999_atomic.sql",
		SQL:     `CREATE TABLE atomic_migration_probe (id INTEGER PRIMARY KEY);`,
	}, 1)
	if err == nil {
		t.Fatal("applyMigration() error = nil")
	}
	if sqliteObjectExists(t, store, ctx, "table", "atomic_migration_probe") {
		t.Fatal("atomic_migration_probe exists after checksum failure")
	}
	var applied int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM schema_migrations WHERE version = '999_atomic.sql'
	`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("applied migration count = %d, want 0", applied)
	}
}

func TestRuntimeHardeningMigrationPreservesReminderRows(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	all := migrations.All()
	if len(all) < 3 {
		t.Fatal("expected runtime hardening migration")
	}
	if _, err := store.db.ExecContext(ctx, migrations.SchemaMigrationsSQL()); err != nil {
		t.Fatal(err)
	}
	if err := store.applyMigrations(ctx, all[:2], 1); err != nil {
		t.Fatal(err)
	}
	insertUser(t, store, 1, 101, 201)
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO reminder_deliveries (
			user_id, billing_date, reminder_type, scheduled_date, status,
			created_at, updated_at, attempt_count
		) VALUES (1, '2026-09-02', 'manual', '2026-09-02', 'pending', 1, 1, 1)
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.applyMigrations(ctx, all[2:], 2); err != nil {
		t.Fatal(err)
	}
	var status, code, key string
	if err := store.db.QueryRowContext(ctx, `
		SELECT status, error_code, delivery_key FROM reminder_deliveries WHERE user_id = 1
	`).Scan(&status, &code, &key); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || code != "delivery_state_unknown" || key != "" {
		t.Fatalf("migrated reminder = status %q code %q key %q", status, code, key)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO users (
			id, display_name, monthly_fee_minor, currency, billing_anchor_day,
			next_charge_on, status, created_at, updated_at
		) VALUES (1, 'Alice', 1000, 'RUB', 1, '2026-08-28', 'active', 1, 1)
	`); err != nil {
		t.Fatal(err)
	}

	var firstAppliedAt int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT applied_at FROM schema_migrations WHERE version = ?
	`, "001_initial.sql").Scan(&firstAppliedAt); err != nil {
		t.Fatal(err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var usersCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&usersCount); err != nil {
		t.Fatal(err)
	}
	if usersCount != 1 {
		t.Fatalf("users count = %d, want 1", usersCount)
	}

	var versionsCount int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&versionsCount); err != nil {
		t.Fatal(err)
	}
	if versionsCount != len(migrations.All()) {
		t.Fatalf("migration versions count = %d, want %d", versionsCount, len(migrations.All()))
	}

	var secondAppliedAt int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT applied_at FROM schema_migrations WHERE version = ?
	`, "001_initial.sql").Scan(&secondAppliedAt); err != nil {
		t.Fatal(err)
	}
	if secondAppliedAt != firstAppliedAt {
		t.Fatalf("applied_at changed from %d to %d", firstAppliedAt, secondAppliedAt)
	}
}

func TestApplyMigrationsRollsBackFailedMigration(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)

	if _, err := store.db.ExecContext(ctx, migrations.SchemaMigrationsSQL()); err != nil {
		t.Fatal(err)
	}

	err := store.applyMigrations(ctx, []migrations.Migration{
		{
			Version: "999_broken.sql",
			SQL: `
				CREATE TABLE rollback_probe (id INTEGER PRIMARY KEY);
				THIS IS NOT VALID SQL;
			`,
		},
	}, 1)
	if !errors.Is(err, ErrApplyMigration) {
		t.Fatalf("applyMigrations() error = %v, want %v", err, ErrApplyMigration)
	}

	if sqliteObjectExists(t, store, ctx, "table", "rollback_probe") {
		t.Error("rollback_probe exists after failed migration")
	}

	var versionsCount int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM schema_migrations WHERE version = ?
	`, "999_broken.sql").Scan(&versionsCount); err != nil {
		t.Fatal(err)
	}
	if versionsCount != 0 {
		t.Fatalf("failed migration version count = %d, want 0", versionsCount)
	}
}

func sqliteObjectExists(t *testing.T, store *Store, ctx context.Context, objectType, name string) bool {
	t.Helper()

	var exists bool
	if err := store.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sqlite_schema WHERE type = ? AND name = ?
		)
	`, objectType, name).Scan(&exists); err != nil {
		t.Fatal(err)
	}

	return exists
}
