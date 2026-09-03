package sqlite

import (
	"context"
	"fmt"

	"github.com/Nergous/vpn-balance-bot/migrations"
)

// MigrationReport describes the database migration state without changing it.
type MigrationReport struct {
	Initialized bool
	Applied     []string
	Pending     []string
}

// MigrationReport reads and validates migration history without applying changes.
func (s *Store) MigrationReport(ctx context.Context) (MigrationReport, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	all := migrations.All()
	report := MigrationReport{
		Pending: migrationVersions(all),
	}

	initialized, err := tableExists(ctx, s.db, "schema_migrations")
	if err != nil {
		return MigrationReport{}, err
	}
	if !initialized {
		return report, nil
	}
	report.Initialized = true

	rows, err := s.db.QueryContext(ctx, `
		SELECT version
		FROM schema_migrations
		ORDER BY version
	`)
	if err != nil {
		return MigrationReport{}, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	known := make(map[string]struct{}, len(all))
	for _, migration := range all {
		known[migration.Version] = struct{}{}
	}
	applied := make(map[string]struct{}, len(all))
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return MigrationReport{}, fmt.Errorf("scan applied migration: %w", err)
		}
		if _, ok := known[version]; !ok {
			return MigrationReport{}, fmt.Errorf("%w: %s", ErrUnknownMigration, version)
		}
		report.Applied = append(report.Applied, version)
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return MigrationReport{}, fmt.Errorf("iterate applied migrations: %w", err)
	}

	report.Pending = report.Pending[:0]
	for _, migration := range all {
		if _, ok := applied[migration.Version]; !ok {
			report.Pending = append(report.Pending, migration.Version)
		}
	}

	checksumsExist, err := migrationChecksumTableExists(ctx, s.db)
	if err != nil {
		return MigrationReport{}, err
	}
	if !checksumsExist {
		if _, checksumMigrationApplied := applied["003_runtime_hardening.sql"]; checksumMigrationApplied {
			return MigrationReport{}, fmt.Errorf(
				"%w: checksum registry is missing",
				ErrMigrationChecksum,
			)
		}
	}
	if checksumsExist {
		if err := s.verifyMigrationChecksums(ctx, all); err != nil {
			return MigrationReport{}, err
		}
	}

	return report, nil
}

func tableExists(ctx context.Context, runner databaseRunner, name string) (bool, error) {
	var exists bool
	if err := runner.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM sqlite_schema
			WHERE type = 'table' AND name = ?
		)
	`, name).Scan(&exists); err != nil {
		return false, fmt.Errorf("check table %s: %w", name, err)
	}
	return exists, nil
}

func migrationVersions(all []migrations.Migration) []string {
	versions := make([]string, 0, len(all))
	for _, migration := range all {
		versions = append(versions, migration.Version)
	}
	return versions
}
