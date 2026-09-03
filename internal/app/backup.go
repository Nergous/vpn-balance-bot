package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/config"
	"github.com/Nergous/vpn-balance-bot/internal/storage/sqlite"
)

// Backup creates a verified snapshot of an existing configured database.
func Backup(ctx context.Context, cfg *config.Config, destinationPath string) error {
	if ctx == nil {
		return fmt.Errorf("backup context is nil")
	}
	if cfg == nil {
		return fmt.Errorf("backup config is nil")
	}
	if strings.TrimSpace(destinationPath) == "" {
		return fmt.Errorf("backup destination is empty")
	}
	info, err := os.Stat(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("stat source database: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("source database is a directory")
	}

	store, err := sqlite.New(ctx, cfg.DatabasePath, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Backup(ctx, destinationPath); err != nil {
		return err
	}
	return nil
}
