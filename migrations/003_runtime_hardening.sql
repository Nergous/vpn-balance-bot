-- Adds durable update idempotency, migration checksums, and fenced reminder attempts.

CREATE TABLE schema_migration_checksums (
  version TEXT PRIMARY KEY NOT NULL
    REFERENCES schema_migrations(version) ON DELETE CASCADE,
  checksum TEXT NOT NULL CHECK (length(checksum) = 64)
);

CREATE TABLE processed_telegram_updates (
	update_id INTEGER PRIMARY KEY,
	processed_at INTEGER NOT NULL
);

CREATE TABLE payment_drafts (
	admin_telegram_id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
	amount_minor INTEGER NOT NULL CHECK (amount_minor > 0),
	note TEXT,
	updated_at INTEGER NOT NULL
);

ALTER TABLE reminder_deliveries RENAME TO reminder_deliveries_legacy;

CREATE TABLE reminder_deliveries (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  billing_date TEXT NOT NULL,
  reminder_type TEXT NOT NULL CHECK (length(trim(reminder_type)) > 0),
  delivery_key TEXT NOT NULL DEFAULT '',
  scheduled_date TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'skipped')),
  sent_at INTEGER,
  telegram_message_id INTEGER,
  error_code TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 1 CHECK (attempt_count >= 1),
  next_attempt_at INTEGER,
  lease_expires_at INTEGER,
  PRIMARY KEY (user_id, billing_date, reminder_type, delivery_key),
  CHECK (status <> 'sent' OR sent_at IS NOT NULL),
  CHECK (status <> 'pending' OR lease_expires_at IS NOT NULL)
);

INSERT INTO reminder_deliveries (
  user_id, billing_date, reminder_type, delivery_key, scheduled_date, status,
  sent_at, telegram_message_id, error_code, created_at, updated_at,
  attempt_count, next_attempt_at, lease_expires_at
)
SELECT
  user_id, billing_date, reminder_type, '', scheduled_date,
  CASE WHEN status = 'pending' THEN 'failed' ELSE status END,
  sent_at, telegram_message_id,
  CASE WHEN status = 'pending' THEN 'delivery_state_unknown' ELSE error_code END,
  created_at, updated_at, attempt_count, next_attempt_at, NULL
FROM reminder_deliveries_legacy;

DROP TABLE reminder_deliveries_legacy;

CREATE INDEX idx_reminder_deliveries_status_scheduled_date
  ON reminder_deliveries(status, scheduled_date);

CREATE INDEX idx_reminder_deliveries_retry
  ON reminder_deliveries(status, error_code, next_attempt_at);

CREATE INDEX idx_reminder_deliveries_pending_lease
  ON reminder_deliveries(status, lease_expires_at)
  WHERE status = 'pending';
