-- Adds durable bounded-retry state without changing existing delivery keys.
ALTER TABLE reminder_deliveries
  ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 1 CHECK (attempt_count >= 1);

ALTER TABLE reminder_deliveries
  ADD COLUMN next_attempt_at INTEGER;

CREATE INDEX IF NOT EXISTS idx_reminder_deliveries_retry
  ON reminder_deliveries(status, error_code, next_attempt_at);
