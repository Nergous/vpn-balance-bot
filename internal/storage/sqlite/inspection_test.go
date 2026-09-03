package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/migrations"
)

func TestOpenReadOnlyRejectsMissingDatabaseWithoutCreatingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenReadOnly(context.Background(), path, time.Second); err == nil {
		t.Fatal("OpenReadOnly() accepted a missing database")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing database was created: %v", err)
	}
}

func TestOpenReadOnlyRejectsWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "database.db")
	store, err := New(ctx, path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	readOnly, err := OpenReadOnly(ctx, path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()

	if _, err := readOnly.db.ExecContext(ctx, `
		INSERT INTO processed_telegram_updates (update_id, processed_at)
		VALUES (1, 1)
	`); err == nil {
		t.Fatal("read-only database accepted a write")
	}
}

func TestMigrationReportReturnsCurrentState(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	report, err := store.MigrationReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Initialized {
		t.Fatal("migration report is not initialized")
	}
	if len(report.Applied) != len(migrations.All()) || len(report.Pending) != 0 {
		t.Fatalf("migration report = %+v", report)
	}
}

func TestMigrationReportDoesNotInitializeEmptyDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "empty.db")
	store, err := New(ctx, path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	report, err := store.MigrationReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.Initialized || len(report.Applied) != 0 ||
		len(report.Pending) != len(migrations.All()) {
		t.Fatalf("migration report = %+v", report)
	}
}

func TestMigrationReportRejectsMissingChecksumRegistry(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE schema_migration_checksums`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.MigrationReport(ctx); !errors.Is(err, ErrMigrationChecksum) {
		t.Fatalf("MigrationReport() error = %v, want %v", err, ErrMigrationChecksum)
	}
}
