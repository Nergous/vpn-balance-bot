package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
)

var (
	ErrDatabaseNotInitialized = errors.New("database is not initialized")
	ErrPendingMigrations      = errors.New("database has pending migrations")
)

// InspectDatabase verifies an existing database without changing it.
func InspectDatabase(
	ctx context.Context,
	cfg *config.Config,
	databasePath string,
) (sqlite.MigrationReport, error) {
	if ctx == nil {
		return sqlite.MigrationReport{}, fmt.Errorf("inspection context is nil")
	}
	if cfg == nil {
		return sqlite.MigrationReport{}, fmt.Errorf("inspection config is nil")
	}
	databasePath = strings.TrimSpace(databasePath)
	if databasePath == "" {
		return sqlite.MigrationReport{}, fmt.Errorf("inspection database path is empty")
	}
	info, err := os.Stat(databasePath)
	if err != nil {
		return sqlite.MigrationReport{}, fmt.Errorf("stat inspection database: %w", err)
	}
	if info.IsDir() {
		return sqlite.MigrationReport{}, fmt.Errorf("inspection database is a directory")
	}

	store, err := sqlite.OpenReadOnly(ctx, databasePath, cfg.DBTimeout)
	if err != nil {
		return sqlite.MigrationReport{}, err
	}
	defer store.Close()

	if err := store.IntegrityCheck(ctx); err != nil {
		return sqlite.MigrationReport{}, fmt.Errorf("database integrity: %w", err)
	}
	report, err := store.MigrationReport(ctx)
	if err != nil {
		return sqlite.MigrationReport{}, fmt.Errorf("database migrations: %w", err)
	}
	if !report.Initialized {
		return sqlite.MigrationReport{}, ErrDatabaseNotInitialized
	}
	return report, nil
}

// Doctor verifies that the configured live database is healthy and current.
func Doctor(ctx context.Context, cfg *config.Config) (sqlite.MigrationReport, error) {
	report, err := InspectDatabase(ctx, cfg, cfg.DatabasePath)
	if err != nil {
		return sqlite.MigrationReport{}, err
	}
	if len(report.Pending) > 0 {
		return sqlite.MigrationReport{}, ErrPendingMigrations
	}
	return report, nil
}
