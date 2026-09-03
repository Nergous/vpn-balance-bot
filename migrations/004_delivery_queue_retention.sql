-- Makes reminder retries independent from calendar selection and supports exact reconciliation.

ALTER TABLE reminder_deliveries
ADD COLUMN message_text TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_reminder_deliveries_due_retry
  ON reminder_deliveries(next_attempt_at, user_id)
  WHERE status = 'failed' AND error_code = 'delivery_retryable';
