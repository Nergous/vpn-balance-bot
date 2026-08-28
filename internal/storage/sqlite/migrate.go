package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/Nergous/vpn-balance-bot/migrations"
)

const insertMigration = `
	INSERT INTO schema_migrations (version, applied_at)
	VALUES (?, ?)
`

func (s *Store) Migrate(ctx context.Context) error {
	migrateCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if _, err := s.db.ExecContext(migrateCtx, migrations.SchemaMigrationsSQL()); err != nil {
		return fmt.Errorf("%w: %w", ErrCreateMigrationRegistry, err)
	}

	return s.applyMigrations(migrateCtx, migrations.All(), time.Now().UTC().Unix())
}

func (s *Store) applyMigrations(ctx context.Context, all []migrations.Migration, appliedAt int64) error {
	for _, migration := range all {
		if err := s.applyMigration(ctx, migration, appliedAt); err != nil {
			return err
		}
	}

	return nil
}

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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w %s: %w", ErrCommitMigration, migration.Version, err)
	}

	return nil
}
