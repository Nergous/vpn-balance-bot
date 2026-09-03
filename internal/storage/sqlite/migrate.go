package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Nergous/vpn-balance-bot/migrations"
)

const insertMigration = `
	INSERT INTO schema_migrations (version, applied_at)
	VALUES (?, ?)
`

// Migrate creates the migration registry and applies each unapplied version once.
func (s *Store) Migrate(ctx context.Context) error {
	migrateCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if _, err := s.db.ExecContext(migrateCtx, migrations.SchemaMigrationsSQL()); err != nil {
		return fmt.Errorf("%w: %w", ErrCreateMigrationRegistry, err)
	}

	all := migrations.All()
	if err := s.rejectUnknownMigrations(migrateCtx, all); err != nil {
		return err
	}
	checksumsExist, err := migrationChecksumTableExists(migrateCtx, s.db)
	if err != nil {
		return err
	}
	if checksumsExist {
		if err := s.verifyMigrationChecksums(migrateCtx, all); err != nil {
			return err
		}
	}
	if err := s.applyMigrations(migrateCtx, all, time.Now().UTC().Unix()); err != nil {
		return err
	}
	return s.verifyMigrationChecksums(migrateCtx, all)
}

func (s *Store) rejectUnknownMigrations(ctx context.Context, all []migrations.Migration) error {
	known := make(map[string]struct{}, len(all))
	for _, migration := range all {
		known[migration.Version] = struct{}{}
	}

	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("scan applied migration: %w", err)
		}
		if _, ok := known[version]; !ok {
			return fmt.Errorf("%w: %s", ErrUnknownMigration, version)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate applied migrations: %w", err)
	}
	return nil
}

func (s *Store) verifyMigrationChecksums(ctx context.Context, all []migrations.Migration) error {
	for _, migration := range all {
		checksum := migration.Checksum()
		var stored sql.NullString
		err := s.db.QueryRowContext(ctx, `
			SELECT checksums.checksum
			FROM schema_migrations AS applied
			LEFT JOIN schema_migration_checksums AS checksums
				ON checksums.version = applied.version
			WHERE applied.version = ?
		`, migration.Version).Scan(&stored)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read migration checksum %s: %w", migration.Version, err)
		}
		if !stored.Valid {
			return fmt.Errorf("%w: missing checksum for %s", ErrMigrationChecksum, migration.Version)
		}
		if stored.String != checksum {
			return fmt.Errorf("%w: %s", ErrMigrationChecksum, migration.Version)
		}
	}
	return nil
}

func migrationChecksumTableExists(ctx context.Context, runner databaseRunner) (bool, error) {
	var exists bool
	if err := runner.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sqlite_schema
			WHERE type = 'table' AND name = 'schema_migration_checksums'
		)
	`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check migration checksum table: %w", err)
	}
	return exists, nil
}

func recordAppliedMigrationChecksums(ctx context.Context, tx *sql.Tx, current migrations.Migration) error {
	known := migrations.All()
	if _, found := migrationByVersion(known, current.Version); !found {
		known = append(known, current)
	}

	for _, migration := range known {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO schema_migration_checksums (version, checksum)
			SELECT ?, ?
			WHERE EXISTS (
				SELECT 1 FROM schema_migrations WHERE version = ?
			)
			ON CONFLICT(version) DO NOTHING
		`, migration.Version, migration.Checksum(), migration.Version); err != nil {
			return fmt.Errorf("record migration checksum %s: %w", migration.Version, err)
		}
	}
	return nil
}

func migrationByVersion(all []migrations.Migration, version string) (migrations.Migration, bool) {
	for _, migration := range all {
		if migration.Version == version {
			return migration, true
		}
	}
	return migrations.Migration{}, false
}

func (s *Store) applyMigrations(ctx context.Context, all []migrations.Migration, appliedAt int64) error {
	for _, migration := range all {
		if err := s.applyMigration(ctx, migration, appliedAt); err != nil {
			return err
		}
	}

	return nil
}

// applyMigration runs migration SQL and records its version in one transaction.
func (s *Store) applyMigration(ctx context.Context, migration migrations.Migration, appliedAt int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var applied bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM schema_migrations WHERE version = ?
		)
	`, migration.Version).Scan(&applied); err != nil {
		return err
	}

	if applied {
		return nil
	}

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("%w %s: %w", ErrApplyMigration, migration.Version, err)
	}

	if _, err := tx.ExecContext(ctx, insertMigration, migration.Version, appliedAt); err != nil {
		return fmt.Errorf("%w %s: %w", ErrRecordMigration, migration.Version, err)
	}

	checksumsExist, err := migrationChecksumTableExists(ctx, tx)
	if err != nil {
		return err
	}
	if checksumsExist {
		if err := recordAppliedMigrationChecksums(ctx, tx, migration); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w %s: %w", ErrCommitMigration, migration.Version, err)
	}

	return nil
}
