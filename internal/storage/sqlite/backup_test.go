package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

func TestBackupCreatesVerifiedSnapshot(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	date, _ := domain.NewDate(2026, time.September, 1)
	now := time.Now().UTC()
	if _, err := store.CreateUser(ctx, domain.User{DisplayName: "Backup", MonthlyFeeMinor: 100, Currency: "RUB", BillingAnchorDay: 1, NextChargeOn: date, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nested", "snapshot.db")
	if err := store.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("backup permissions = %04o, want 0600", got)
		}
		directoryInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if got := directoryInfo.Mode().Perm(); got&0o077 != 0 {
			t.Fatalf("backup directory permissions = %04o, want no group/other access", got)
		}
	}
	if err := integrityCheckPath(ctx, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUser(ctx, domain.User{DisplayName: "After backup", MonthlyFeeMinor: 100, Currency: "RUB", BillingAnchorDay: 1, NextChargeOn: date, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got := snapshotUserCount(t, ctx, path); got != 1 {
		t.Fatalf("snapshot users = %d, want 1", got)
	}
	if err := store.Backup(ctx, path); !errors.Is(err, ErrBackupExists) {
		t.Fatalf("second backup error=%v", err)
	}
	if err := store.IntegrityCheck(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBackupCancelledContextDoesNotPublishSnapshot(t *testing.T) {
	store := newTestSQLite(t, context.Background())
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Backup(ctx, path); err == nil {
		t.Fatal("Backup() returned nil for cancelled context")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot exists after cancelled backup: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary snapshot exists after cancelled backup: %v", err)
	}
}

func TestBackupExcludesUncommittedChanges(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	date, _ := domain.NewDate(2026, time.September, 1)
	now := time.Now().UTC()
	if _, err := store.CreateUser(ctx, domain.User{DisplayName: "Committed", MonthlyFeeMinor: 100, Currency: "RUB", BillingAnchorDay: 1, NextChargeOn: date, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	var sequence int
	var name, sourcePath string
	if err := store.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &sourcePath); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite", "file:"+filepath.ToSlash(sourcePath)+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tx, err := writer.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO users (display_name, monthly_fee_minor, currency, billing_anchor_day, next_charge_on, status, created_at, updated_at) VALUES ('Uncommitted', 100, 'RUB', 1, '2026-09-01', 'active', 0, 0)`); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err := store.Backup(ctx, path); err != nil {
		t.Fatal(err)
	}
	if got := snapshotUserCount(t, ctx, path); got != 1 {
		t.Fatalf("snapshot users = %d, want committed users only", got)
	}
}

func snapshotUserCount(t *testing.T, ctx context.Context, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
