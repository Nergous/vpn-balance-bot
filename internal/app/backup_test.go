package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
)

func TestBackupCreatesSnapshotFromConfiguredExistingDatabase(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "source.db")
	store, err := sqlite.New(ctx, source, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "backups", "snapshot.db")
	cfg := &config.Config{DatabasePath: source, DBTimeout: 5 * time.Second}
	if err := Backup(ctx, cfg, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
}

func TestBackupRejectsMissingSourceDatabase(t *testing.T) {
	cfg := &config.Config{
		DatabasePath: filepath.Join(t.TempDir(), "missing.db"),
		DBTimeout:    5 * time.Second,
	}
	if err := Backup(context.Background(), cfg, filepath.Join(t.TempDir(), "snapshot.db")); err == nil {
		t.Fatal("Backup() accepted a missing source database")
	}
}
