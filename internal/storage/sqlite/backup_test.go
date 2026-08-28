package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
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
