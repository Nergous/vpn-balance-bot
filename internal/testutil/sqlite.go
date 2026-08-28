package testutil

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
)

const sqliteTimeout = 5 * time.Second

// NewSQLite creates a migrated SQLite database isolated in t.TempDir.
func NewSQLite(t *testing.T) *sqlite.Store {
	t.Helper()

	store, err := sqlite.New(
		context.Background(),
		filepath.Join(t.TempDir(), "vpn-balance-bot.db"),
		sqliteTimeout,
	)
	if err != nil {
		t.Fatalf("open SQLite test database: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close SQLite test database: %v", err)
		}
	})

	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate SQLite test database: %v", err)
	}

	return store
}
