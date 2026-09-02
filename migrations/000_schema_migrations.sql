-- Records successfully applied migration versions.
-- A migration version and its applied_at timestamp are inserted in the same transaction
-- as the migration SQL, so a failed migration is never marked as applied.
CREATE TABLE IF NOT EXISTS schema_migrations (
  -- Lexicographically sortable migration filename, for example 001_initial.sql.
	version TEXT PRIMARY KEY,
  -- UTC Unix timestamp of the successful migration transaction.
	applied_at INTEGER NOT NULL
);
