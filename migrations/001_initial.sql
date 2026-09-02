-- UTC timestamps are stored as Unix seconds in INTEGER columns.
-- Calendar dates are stored as ISO 8601 strings (YYYY-MM-DD) in TEXT columns.
-- Foreign key enforcement must be enabled for every connection with PRAGMA foreign_keys = ON.

-- Stores each customer's billing profile and optional Telegram account binding.
CREATE TABLE IF NOT EXISTS users (
  -- Internal immutable identifier. Telegram identifiers remain nullable until invite binding.
  id INTEGER PRIMARY KEY,
  telegram_user_id INTEGER UNIQUE,
  telegram_chat_id INTEGER UNIQUE,
  username TEXT,
  display_name TEXT NOT NULL CHECK (length(trim(display_name)) > 0),
  -- Money is always stored as signed integer minor units, never floating point.
  monthly_fee_minor INTEGER NOT NULL CHECK (monthly_fee_minor > 0),
  currency TEXT NOT NULL DEFAULT 'RUB' CHECK (currency = 'RUB'),
  billing_anchor_day INTEGER NOT NULL CHECK (billing_anchor_day BETWEEN 1 AND 31),
  -- Next subscription billing date in ISO 8601 YYYY-MM-DD format.
  next_charge_on TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('active', 'paused', 'disabled')),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Stores hashed, single-use invitation tokens that bind a Telegram account to a customer.
CREATE TABLE IF NOT EXISTS invite_tokens (
  -- Only a hash is persisted; raw tokens are returned once to the administrator.
  token_hash TEXT PRIMARY KEY NOT NULL CHECK (length(token_hash) > 0),
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  expires_at INTEGER NOT NULL,
  used_at INTEGER,
  created_at INTEGER NOT NULL,
  CHECK (expires_at > created_at),
  CHECK (used_at IS NULL OR used_at BETWEEN created_at AND expires_at)
);

-- Stores the immutable financial ledger used to calculate each customer's balance.
CREATE TABLE IF NOT EXISTS ledger_entries (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  kind TEXT NOT NULL CHECK (
    kind IN ('opening_balance', 'payment', 'subscription_charge', 'adjustment', 'reversal')
  ),
  amount_minor INTEGER NOT NULL CHECK (amount_minor <> 0),
  occurred_at INTEGER NOT NULL,
  -- Billing period charged by a subscription entry; not used by manual entries.
  billing_period_on TEXT,
  -- A reversal points to exactly one original entry and is itself immutable.
  reverses_entry_id INTEGER UNIQUE REFERENCES ledger_entries(id) ON DELETE RESTRICT,
  created_by_telegram_id INTEGER,
  note TEXT,
  created_at INTEGER NOT NULL,
  CHECK (kind <> 'subscription_charge' OR billing_period_on IS NOT NULL),
  CHECK (
    (kind = 'reversal' AND reverses_entry_id IS NOT NULL)
    OR (kind <> 'reversal' AND reverses_entry_id IS NULL)
  ),
  CHECK (reverses_entry_id IS NULL OR reverses_entry_id <> id)
);

-- Stores reminder delivery state and prevents duplicate reminders for one billing period.
CREATE TABLE IF NOT EXISTS reminder_deliveries (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  billing_date TEXT NOT NULL,
  reminder_type TEXT NOT NULL CHECK (length(trim(reminder_type)) > 0),
  scheduled_date TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'skipped')),
  sent_at INTEGER,
  telegram_message_id INTEGER,
  -- Safe transport classification such as unreachable or delivery_state_unknown.
  -- Never persist raw Telegram API error text here.
  error_code TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, billing_date, reminder_type),
  CHECK (status <> 'sent' OR sent_at IS NOT NULL)
);

-- Supports scheduler queries for customers whose next charge date has arrived.
CREATE INDEX IF NOT EXISTS idx_users_status_next_charge_on
  ON users(status, next_charge_on);

-- Supports token management and cleanup by customer.
CREATE INDEX IF NOT EXISTS idx_invite_tokens_user_id
  ON invite_tokens(user_id);

-- Supports balance calculation and retrieval of recent ledger entries.
CREATE INDEX IF NOT EXISTS idx_ledger_entries_user_occurred_at
  ON ledger_entries(user_id, occurred_at DESC, id DESC);

-- Prevents duplicate monthly subscription charges while allowing other ledger entry kinds.
CREATE UNIQUE INDEX IF NOT EXISTS ux_ledger_subscription_charge_period
  ON ledger_entries(user_id, billing_period_on)
  WHERE kind = 'subscription_charge';

-- Supports scheduler queries for reminders waiting to be delivered.
CREATE INDEX IF NOT EXISTS idx_reminder_deliveries_status_scheduled_date
  ON reminder_deliveries(status, scheduled_date);
