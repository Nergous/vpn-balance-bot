package sqlite

import "errors"

var (
	// ErrOpenConnection wraps SQLite open and ping failures.
	ErrOpenConnection = errors.New("failed to open SQLite connection")
	// ErrCreateMigrationRegistry wraps schema_migrations creation failures.
	ErrCreateMigrationRegistry = errors.New("failed to create migration registry")
	// ErrApplyMigration wraps SQL execution failures for one migration version.
	ErrApplyMigration = errors.New("failed to apply migration")
	// ErrRecordMigration wraps failures to persist an applied migration version.
	ErrRecordMigration = errors.New("failed to record migration")
	// ErrCommitMigration wraps failures that commit a migration transaction.
	ErrCommitMigration = errors.New("failed to commit migration")
)
