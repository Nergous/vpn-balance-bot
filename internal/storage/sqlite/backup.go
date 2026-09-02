package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrBackupExists prevents an existing verified backup from being overwritten.
var ErrBackupExists = errors.New("backup destination already exists")

// Backup creates a verified SQLite snapshot without copying a live WAL file.
func (s *Store) Backup(ctx context.Context, destinationPath string) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if _, err := os.Stat(destinationPath); err == nil {
		return ErrBackupExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat backup destination: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	temporaryPath := destinationPath + ".tmp"
	if _, err := os.Stat(temporaryPath); err == nil {
		return fmt.Errorf("backup temporary path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat backup temporary path: %w", err)
	}

	defer os.Remove(temporaryPath)

	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", temporaryPath); err != nil {
		return fmt.Errorf("create SQLite snapshot: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("secure SQLite snapshot: %w", err)
	}

	if err := integrityCheckPath(ctx, temporaryPath); err != nil {
		return fmt.Errorf("verify SQLite snapshot: %w", err)
	}

	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return fmt.Errorf("finalize SQLite snapshot: %w", err)
	}

	return nil
}

func (s *Store) IntegrityCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var result string
	if err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("run integrity check: %w", err)
	}

	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}

	return nil
}

func integrityCheckPath(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_foreign_keys=1")
	if err != nil {
		return err
	}
	defer db.Close()

	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}

	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}

	return nil
}
