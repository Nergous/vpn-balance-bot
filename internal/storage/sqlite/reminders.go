package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/reminder"
)

const reserveReminderDelivery = `
	INSERT INTO reminder_deliveries (
		user_id, billing_date, reminder_type, scheduled_date, status,
		created_at, updated_at, attempt_count, next_attempt_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 1, NULL)
	ON CONFLICT(user_id, billing_date, reminder_type) DO UPDATE SET
		scheduled_date = excluded.scheduled_date,
		status = excluded.status,
		sent_at = NULL,
		telegram_message_id = NULL,
		error_code = NULL,
		updated_at = excluded.updated_at,
		attempt_count = reminder_deliveries.attempt_count + 1,
		next_attempt_at = NULL
	WHERE reminder_deliveries.status = 'failed'
		AND reminder_deliveries.error_code = 'delivery_retryable'
		AND reminder_deliveries.attempt_count < ?
		AND reminder_deliveries.next_attempt_at IS NOT NULL
		AND reminder_deliveries.next_attempt_at <= excluded.updated_at
`

// ReserveReminderDelivery creates a first attempt or atomically reclaims a due
// definite retryable failure. Sent, pending, ambiguous, and exhausted rows stay closed.
func (s *Store) ReserveReminderDelivery(ctx context.Context, delivery domain.ReminderDelivery, maxAttempts int) (domain.ReminderDelivery, int, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.db.ExecContext(ctx, reserveReminderDelivery,
		delivery.UserID,
		delivery.BillingDate.String(),
		delivery.ReminderType,
		delivery.ScheduledDate.String(),
		delivery.Status,
		delivery.CreatedAt.UTC().Unix(),
		delivery.UpdatedAt.UTC().Unix(),
		maxAttempts,
	)
	if err != nil {
		return domain.ReminderDelivery{}, 0, false, fmt.Errorf("reserve reminder delivery: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return domain.ReminderDelivery{}, 0, false, fmt.Errorf("check reminder reservation: %w", err)
	}

	existing, attempt, err := s.reminderDeliveryAttempt(ctx, delivery.UserID, delivery.BillingDate, delivery.ReminderType)
	if err != nil {
		return domain.ReminderDelivery{}, 0, false, err
	}

	return existing, attempt, rows == 1, nil
}

// CreateReminderDelivery reserves one unique delivery before a message is sent.
// created is false when the same user, billing date, and reminder type already exist.
func (s *Store) CreateReminderDelivery(ctx context.Context, delivery domain.ReminderDelivery) (domain.ReminderDelivery, bool, error) {
	existing, _, created, err := s.ReserveReminderDelivery(ctx, delivery, 1)
	return existing, created, err
}

const updateReminderDelivery = "UPDATE reminder_deliveries SET status = ?, sent_at = ?, telegram_message_id = ?, error_code = ?, updated_at = ?, next_attempt_at = ? WHERE user_id = ? AND billing_date = ? AND reminder_type = ?"

// UpdateReminderDeliveryAttempt records one transport result and optional retry time.
func (s *Store) UpdateReminderDeliveryAttempt(ctx context.Context, delivery domain.ReminderDelivery, retryAt *time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	_, err := s.db.ExecContext(ctx, updateReminderDelivery,
		delivery.Status,
		unixOrNil(delivery.SentAt),
		int64OrNil(delivery.TelegramMessageID),
		delivery.ErrorCode,
		delivery.UpdatedAt.UTC().Unix(),
		unixOrNil(retryAt),
		delivery.UserID,
		delivery.BillingDate.String(),
		delivery.ReminderType,
	)

	if err != nil {
		return fmt.Errorf("update reminder delivery: %w", err)
	}
	return nil
}

// UpdateReminderDelivery records the final transport outcome of a reserved delivery.
func (s *Store) UpdateReminderDelivery(ctx context.Context, delivery domain.ReminderDelivery) error {
	return s.UpdateReminderDeliveryAttempt(ctx, delivery, nil)
}

const markPendingUnknown = "UPDATE reminder_deliveries SET status = 'failed', error_code = 'delivery_state_unknown', next_attempt_at = NULL, updated_at = ? WHERE status = 'pending'"

// MarkPendingUnknown marks interrupted sends as ambiguous after a process restart.
func (s *Store) MarkPendingUnknown(ctx context.Context, updatedAt time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.db.ExecContext(ctx, markPendingUnknown, updatedAt.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("mark pending reminder deliveries unknown: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count recovered pending reminder deliveries: %w", err)
	}

	return int(rows), nil
}

const markUnknownRetryable = `
	UPDATE reminder_deliveries
	SET error_code = 'delivery_retryable', next_attempt_at = ?, updated_at = ?
	WHERE user_id = ? AND billing_date = ? AND reminder_type = ?
		AND status = 'failed' AND error_code = 'delivery_state_unknown'
		AND attempt_count < ?
`

// MarkUnknownRetryable reopens an ambiguous row only after explicit confirmation
// by a reconciliation caller that the original message was not sent.
func (s *Store) MarkUnknownRetryable(ctx context.Context, userID domain.UserID, billingDate domain.Date, kind domain.ReminderType, updatedAt time.Time, maxAttempts int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.db.ExecContext(
		ctx,
		markUnknownRetryable,
		updatedAt.UTC().Unix(),
		updatedAt.UTC().Unix(),
		userID,
		billingDate.String(),
		kind,
		maxAttempts,
	)
	if err != nil {
		return false, fmt.Errorf("mark unknown reminder retryable: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check unknown reminder reconciliation: %w", err)
	}

	return rows == 1, nil
}

const selectReminderDelivery = "SELECT billing_date, scheduled_date, status, sent_at, telegram_message_id, error_code, created_at, updated_at, attempt_count FROM reminder_deliveries WHERE user_id = ? AND billing_date = ? AND reminder_type = ?"

func (s *Store) reminderDelivery(ctx context.Context, userID domain.UserID, billingDate domain.Date, kind domain.ReminderType) (domain.ReminderDelivery, error) {
	delivery, _, err := s.reminderDeliveryAttempt(ctx, userID, billingDate, kind)
	return delivery, err
}

func (s *Store) reminderDeliveryAttempt(ctx context.Context, userID domain.UserID, billingDate domain.Date, kind domain.ReminderType) (domain.ReminderDelivery, int, error) {
	var (
		delivery             domain.ReminderDelivery
		billing, scheduled   string
		sentAt               sql.NullInt64
		messageID            sql.NullInt64
		errorCode            sql.NullString
		createdAt, updatedAt int64
		attempt              int
	)

	err := s.db.QueryRowContext(ctx, selectReminderDelivery, userID, billingDate.String(), kind).Scan(
		&billing, &scheduled, &delivery.Status, &sentAt, &messageID, &errorCode, &createdAt, &updatedAt, &attempt,
	)

	if err != nil {
		return delivery, 0, err
	}

	delivery.UserID = userID
	delivery.ReminderType = kind
	delivery.BillingDate, err = domain.ParseDate(billing)
	if err != nil {
		return domain.ReminderDelivery{}, 0, err
	}

	delivery.ScheduledDate, err = domain.ParseDate(scheduled)
	if err != nil {
		return domain.ReminderDelivery{}, 0, err
	}

	delivery.SentAt = reminderTimePointer(sentAt)
	delivery.TelegramMessageID = int64Pointer(messageID)
	delivery.ErrorCode = stringPointer(errorCode)
	delivery.CreatedAt = time.Unix(createdAt, 0).UTC()
	delivery.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	return delivery, attempt, nil
}

const reminderCandidateUserColumns = `
	u.id,
	u.telegram_user_id,
	u.telegram_chat_id,
	u.username,
	u.display_name,
	u.monthly_fee_minor,
	u.currency,
	u.billing_anchor_day,
	u.next_charge_on,
	u.status,
	u.created_at,
	u.updated_at
`

// ReminderCandidates returns active users with one aggregated balance and their
// latest non-reversed subscription charge period on or before asOf.
func (s *Store) ReminderCandidates(ctx context.Context, asOf domain.Date) ([]reminder.Candidate, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		WITH balances AS (
			SELECT user_id, SUM(amount_minor) AS balance_minor
			FROM ledger_entries
			GROUP BY user_id
		), latest_charges AS (
			SELECT charges.user_id, MAX(charges.billing_period_on) AS billing_period_on
			FROM ledger_entries AS charges
			LEFT JOIN ledger_entries AS reversals
				ON reversals.reverses_entry_id = charges.id
				AND reversals.kind = 'reversal'
			WHERE charges.kind = 'subscription_charge'
				AND charges.billing_period_on <= ?
				AND reversals.id IS NULL
			GROUP BY charges.user_id
		)
		SELECT `+reminderCandidateUserColumns+`,
			COALESCE(balances.balance_minor, 0),
			latest_charges.billing_period_on
		FROM users AS u
		LEFT JOIN balances ON balances.user_id = u.id
		LEFT JOIN latest_charges ON latest_charges.user_id = u.id
		WHERE u.status = ?
		ORDER BY u.id ASC
	`, asOf.String(), domain.UserStatusActive)
	if err != nil {
		return nil, fmt.Errorf("select reminder candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]reminder.Candidate, 0)
	for rows.Next() {
		candidate, err := scanReminderCandidate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reminder candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reminder candidates: %w", err)
	}

	return candidates, nil
}

func scanReminderCandidate(scanner rowScanner) (reminder.Candidate, error) {
	var (
		candidate          reminder.Candidate
		telegramUserID     sql.NullInt64
		telegramChatID     sql.NullInt64
		username           sql.NullString
		nextChargeOn       string
		latestChargePeriod sql.NullString
		createdAt          int64
		updatedAt          int64
	)

	err := scanner.Scan(
		&candidate.User.ID,
		&telegramUserID,
		&telegramChatID,
		&username,
		&candidate.User.DisplayName,
		&candidate.User.MonthlyFeeMinor,
		&candidate.User.Currency,
		&candidate.User.BillingAnchorDay,
		&nextChargeOn,
		&candidate.User.Status,
		&createdAt,
		&updatedAt,
		&candidate.Balance,
		&latestChargePeriod,
	)
	if err != nil {
		return reminder.Candidate{}, err
	}

	candidate.User.NextChargeOn, err = domain.ParseDate(nextChargeOn)
	if err != nil {
		return reminder.Candidate{}, fmt.Errorf("parse reminder next charge date: %w", err)
	}

	if latestChargePeriod.Valid {
		parsed, err := domain.ParseDate(latestChargePeriod.String)
		if err != nil {
			return reminder.Candidate{}, fmt.Errorf("parse reminder billing period: %w", err)
		}
		candidate.LatestChargePeriodOn = &parsed
	}

	candidate.User.TelegramUserID = int64Pointer(telegramUserID)
	candidate.User.TelegramChatID = int64Pointer(telegramChatID)
	candidate.User.Username = stringPointer(username)
	candidate.User.CreatedAt = time.Unix(createdAt, 0).UTC()
	candidate.User.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	return candidate, nil
}

func unixOrNil(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC().Unix()
}

func int64OrNil(value *int64) any {
	if value == nil {
		return nil
	}

	return *value
}

func reminderTimePointer(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}

	parsed := time.Unix(value.Int64, 0).UTC()
	return &parsed
}
