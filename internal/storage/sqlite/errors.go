package sqlite

import "errors"

var (
	ErrOpenConnection          = errors.New("failed to open SQLite connection")
	ErrCreateMigrationRegistry = errors.New("failed to create migration registry")
	ErrApplyMigration          = errors.New("failed to apply migration")
	ErrRecordMigration         = errors.New("failed to record migration")
	ErrCommitMigration         = errors.New("failed to commit migration")
)
