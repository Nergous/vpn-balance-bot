package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
	"github.com/Nergous/vpn-balance-bot/migrations"
)

func TestDoctorInspectsMigratedDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "database.db")
	store, err := sqlite.New(ctx, path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := Doctor(ctx, &config.Config{
		DatabasePath: path,
		DBTimeout:    time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Applied) != len(migrations.All()) || len(report.Pending) != 0 {
		t.Fatalf("doctor report = %+v", report)
	}
}

func TestInspectDatabaseRejectsUninitializedDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "database.db")
	store, err := sqlite.New(ctx, path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = InspectDatabase(ctx, &config.Config{DBTimeout: time.Second}, path)
	if !errors.Is(err, ErrDatabaseNotInitialized) {
		t.Fatalf("InspectDatabase() error = %v, want %v", err, ErrDatabaseNotInitialized)
	}
}
