package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewConfiguresSQLiteConnection(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)

	if err := store.db.PingContext(ctx); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}

	stats := store.db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}

	var foreignKeys int
	if err := store.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := store.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal_mode = %q, want WAL", journalMode)
	}

	var busyTimeout int
	if err := store.db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5_000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	if runtime.GOOS != "windows" {
		var sequence int
		var name, path string
		if err := store.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("database permissions = %04o, want 0600", got)
		}
	}
}

func TestNewRejectsUnavailablePath(t *testing.T) {
	ctx := context.Background()
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("block database path"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := New(ctx, filepath.Join(parent, "vpn-balance-bot.db"), time.Second)
	if !errors.Is(err, ErrOpenConnection) {
		t.Fatalf("New() error = %v, want %v", err, ErrOpenConnection)
	}
}

func TestNewCreatesMissingDatabaseDirectory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "data", "nested", "vpn-balance-bot.db")
	store, err := New(ctx, path, time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("database directory: %v", err)
	}
}

func TestNewRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New(ctx, filepath.Join(t.TempDir(), "vpn-balance-bot.db"), time.Second)
	if !errors.Is(err, ErrOpenConnection) {
		t.Fatalf("New() error = %v, want %v", err, ErrOpenConnection)
	}
}

func TestClosePreventsFurtherQueries(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLite(t, ctx)

	if err := store.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if err := store.db.PingContext(ctx); err == nil {
		t.Fatal("PingContext() after Close() error = nil, want non-nil")
	}
}

func newTestSQLite(t *testing.T, ctx context.Context) *Store {
	t.Helper()

	path := filepath.Join(t.TempDir(), "vpn-balance-bot.db")
	store, err := New(ctx, path, 5*time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	return store
}
