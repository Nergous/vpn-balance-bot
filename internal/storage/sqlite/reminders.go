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
		user_id, billing_date, reminder_type, delivery_key, message_text, scheduled_date, status,
		created_at, updated_at, attempt_count, next_attempt_at, lease_expires_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, NULL, ?)
	ON CONFLICT(user_id, billing_date, reminder_type, delivery_key) DO UPDATE SET
		scheduled_date = excluded.scheduled_date,
		status = excluded.status,
		sent_at = NULL,
		telegram_message_id = NULL,
		error_code = NULL,
		message_text = excluded.message_text,
		updated_at = excluded.updated_at,
		attempt_count = reminder_deliveries.attempt_count + 1,
		next_attempt_at = NULL,
		lease_expires_at = excluded.lease_expires_at
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

	if delivery.LeaseExpiresAt == nil {
		lease := delivery.UpdatedAt.Add(2 * time.Minute)
		delivery.LeaseExpiresAt = &lease
	}
	runner := s.runner(ctx)
	result, err := runner.ExecContext(ctx, reserveReminderDelivery,
		delivery.UserID,
		delivery.BillingDate.String(),
		delivery.ReminderType,
		delivery.DeliveryKey,
		delivery.MessageText,
		delivery.ScheduledDate.String(),
		delivery.Status,
		delivery.CreatedAt.UTC().Unix(),
		delivery.UpdatedAt.UTC().Unix(),
		delivery.LeaseExpiresAt.UTC().Unix(),
		maxAttempts,
	)
	if err != nil {
		return domain.ReminderDelivery{}, 0, false, fmt.Errorf("reserve reminder delivery: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return domain.ReminderDelivery{}, 0, false, fmt.Errorf("check reminder reservation: %w", err)
	}

	existing, attempt, err := s.reminderDeliveryAttempt(ctx, delivery.UserID, delivery.BillingDate, delivery.ReminderType, delivery.DeliveryKey)
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

const updateReminderDelivery = "UPDATE reminder_deliveries SET status = ?, sent_at = ?, telegram_message_id = ?, error_code = ?, updated_at = ?, next_attempt_at = ?, lease_expires_at = NULL WHERE user_id = ? AND billing_date = ? AND reminder_type = ? AND delivery_key = ? AND status = 'pending' AND attempt_count = ?"

// UpdateReminderDeliveryAttempt records one transport result and optional retry time.
func (s *Store) UpdateReminderDeliveryAttempt(ctx context.Context, delivery domain.ReminderDelivery, attempt int, retryAt *time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.runner(ctx).ExecContext(ctx, updateReminderDelivery,
		delivery.Status,
		unixOrNil(delivery.SentAt),
		int64OrNil(delivery.TelegramMessageID),
		delivery.ErrorCode,
		delivery.UpdatedAt.UTC().Unix(),
		unixOrNil(retryAt),
		delivery.UserID,
		delivery.BillingDate.String(),
		delivery.ReminderType,
		delivery.DeliveryKey,
		attempt,
	)

	if err != nil {
		return false, fmt.Errorf("update reminder delivery: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check reminder delivery update: %w", err)
	}
	return rows == 1, nil
}

// UpdateReminderDelivery records the final transport outcome of a reserved delivery.
func (s *Store) UpdateReminderDelivery(ctx context.Context, delivery domain.ReminderDelivery) error {
	attempt := delivery.AttemptCount
	if attempt == 0 {
		_, current, err := s.reminderDeliveryAttempt(ctx, delivery.UserID, delivery.BillingDate, delivery.ReminderType, delivery.DeliveryKey)
		if err != nil {
			return err
		}
		attempt = current
	}
	updated, err := s.UpdateReminderDeliveryAttempt(ctx, delivery, attempt, nil)
	if err != nil {
		return err
	}
	if !updated {
		return fmt.Errorf("update reminder delivery: stale attempt")
	}
	return nil
}

const markPendingUnknown = "UPDATE reminder_deliveries SET status = 'failed', error_code = 'delivery_state_unknown', next_attempt_at = NULL, lease_expires_at = NULL, updated_at = ? WHERE status = 'pending' AND lease_expires_at <= ?"
const markAllPendingUnknown = "UPDATE reminder_deliveries SET status = 'failed', error_code = 'delivery_state_unknown', next_attempt_at = NULL, lease_expires_at = NULL, updated_at = ? WHERE status = 'pending'"

// MarkAllPendingUnknown recovers every send interrupted by the previous
// single application process, regardless of its former lease deadline.
func (s *Store) MarkAllPendingUnknown(ctx context.Context, updatedAt time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.runner(ctx).ExecContext(ctx, markAllPendingUnknown, updatedAt.UTC().Unix())
	if err != nil {
		return 0, fmt.Errorf("mark all pending reminder deliveries unknown: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count all recovered pending reminder deliveries: %w", err)
	}
	return int(rows), nil
}

// MarkPendingUnknown marks runtime attempts whose leases have expired.
func (s *Store) MarkPendingUnknown(ctx context.Context, updatedAt time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	unix := updatedAt.UTC().Unix()
	result, err := s.runner(ctx).ExecContext(ctx, markPendingUnknown, unix, unix)
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
	WHERE user_id = ? AND billing_date = ? AND reminder_type = ? AND delivery_key = ?
		AND status = 'failed' AND error_code = 'delivery_state_unknown'
		AND attempt_count < ?
`

// MarkUnknownRetryable reopens an ambiguous row only after explicit confirmation
// by a reconciliation caller that the original message was not sent.
func (s *Store) MarkUnknownRetryable(ctx context.Context, userID domain.UserID, billingDate domain.Date, kind domain.ReminderType, deliveryKey string, updatedAt time.Time, maxAttempts int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.runner(ctx).ExecContext(
		ctx,
		markUnknownRetryable,
		updatedAt.UTC().Unix(),
		updatedAt.UTC().Unix(),
		userID,
		billingDate.String(),
		kind,
		deliveryKey,
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

const selectReminderDelivery = "SELECT billing_date, scheduled_date, status, sent_at, telegram_message_id, error_code, message_text, created_at, updated_at, attempt_count, lease_expires_at FROM reminder_deliveries WHERE user_id = ? AND billing_date = ? AND reminder_type = ? AND delivery_key = ?"

func (s *Store) reminderDeliveryAttempt(ctx context.Context, userID domain.UserID, billingDate domain.Date, kind domain.ReminderType, deliveryKey string) (domain.ReminderDelivery, int, error) {
	var (
		delivery             domain.ReminderDelivery
		billing, scheduled   string
		sentAt               sql.NullInt64
		messageID            sql.NullInt64
		errorCode            sql.NullString
		createdAt, updatedAt int64
		attempt              int
		leaseExpiresAt       sql.NullInt64
	)

	err := s.runner(ctx).QueryRowContext(ctx, selectReminderDelivery, userID, billingDate.String(), kind, deliveryKey).Scan(
		&billing, &scheduled, &delivery.Status, &sentAt, &messageID, &errorCode, &delivery.MessageText, &createdAt, &updatedAt, &attempt, &leaseExpiresAt,
	)

	if err != nil {
		return delivery, 0, err
	}

	delivery.UserID = userID
	delivery.ReminderType = kind
	delivery.DeliveryKey = deliveryKey
	delivery.AttemptCount = attempt
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
	delivery.LeaseExpiresAt = reminderTimePointer(leaseExpiresAt)

	return delivery, attempt, nil
}

// RetryableReminderDeliveries returns due retries independently from today's
// calendar-based automatic reminder selection.
func (s *Store) RetryableReminderDeliveries(ctx context.Context, dueAt time.Time, maxAttempts, limit int) ([]reminder.RetryCandidate, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if maxAttempts <= 0 || limit <= 0 {
		return nil, fmt.Errorf("retryable reminder limits must be positive")
	}

	rows, err := s.runner(ctx).QueryContext(ctx, `
		SELECT `+reminderCandidateUserColumns+`,
			d.billing_date, d.reminder_type, d.delivery_key, d.message_text,
			d.scheduled_date, d.status, d.sent_at, d.telegram_message_id,
			d.error_code, d.created_at, d.updated_at, d.attempt_count, d.lease_expires_at
		FROM reminder_deliveries AS d
		JOIN users AS u ON u.id = d.user_id
		WHERE d.status = 'failed'
			AND d.error_code = 'delivery_retryable'
			AND d.next_attempt_at IS NOT NULL
			AND d.next_attempt_at <= ?
			AND d.attempt_count < ?
			AND (d.reminder_type = 'manual' OR u.status = 'active')
		ORDER BY d.next_attempt_at ASC, d.user_id ASC, d.delivery_key ASC
		LIMIT ?
	`, dueAt.UTC().Unix(), maxAttempts, limit)
	if err != nil {
		return nil, fmt.Errorf("select retryable reminder deliveries: %w", err)
	}
	defer rows.Close()

	candidates := make([]reminder.RetryCandidate, 0)
	for rows.Next() {
		candidate, err := scanRetryCandidate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan retryable reminder delivery: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retryable reminder deliveries: %w", err)
	}
	return candidates, nil
}

func scanRetryCandidate(scanner rowScanner) (reminder.RetryCandidate, error) {
	var (
		candidate                      reminder.RetryCandidate
		telegramUserID, telegramChatID sql.NullInt64
		username                       sql.NullString
		nextChargeOn, billingDate      string
		scheduledDate                  string
		sentAt, messageID, lease       sql.NullInt64
		errorCode                      sql.NullString
		userCreatedAt, userUpdatedAt   int64
		deliveryCreatedAt              int64
		deliveryUpdatedAt              int64
	)
	err := scanner.Scan(
		&candidate.User.ID, &telegramUserID, &telegramChatID, &username,
		&candidate.User.DisplayName, &candidate.User.MonthlyFeeMinor,
		&candidate.User.Currency, &candidate.User.BillingAnchorDay,
		&nextChargeOn, &candidate.User.Status, &userCreatedAt, &userUpdatedAt,
		&billingDate, &candidate.Delivery.ReminderType, &candidate.Delivery.DeliveryKey,
		&candidate.Delivery.MessageText, &scheduledDate, &candidate.Delivery.Status,
		&sentAt, &messageID, &errorCode, &deliveryCreatedAt, &deliveryUpdatedAt,
		&candidate.Delivery.AttemptCount, &lease,
	)
	if err != nil {
		return reminder.RetryCandidate{}, err
	}

	candidate.User.NextChargeOn, err = domain.ParseDate(nextChargeOn)
	if err != nil {
		return reminder.RetryCandidate{}, err
	}
	candidate.Delivery.BillingDate, err = domain.ParseDate(billingDate)
	if err != nil {
		return reminder.RetryCandidate{}, err
	}
	candidate.Delivery.ScheduledDate, err = domain.ParseDate(scheduledDate)
	if err != nil {
		return reminder.RetryCandidate{}, err
	}

	candidate.User.TelegramUserID = int64Pointer(telegramUserID)
	candidate.User.TelegramChatID = int64Pointer(telegramChatID)
	candidate.User.Username = stringPointer(username)
	candidate.User.CreatedAt = time.Unix(userCreatedAt, 0).UTC()
	candidate.User.UpdatedAt = time.Unix(userUpdatedAt, 0).UTC()
	candidate.Delivery.UserID = candidate.User.ID
	candidate.Delivery.SentAt = reminderTimePointer(sentAt)
	candidate.Delivery.TelegramMessageID = int64Pointer(messageID)
	candidate.Delivery.ErrorCode = stringPointer(errorCode)
	candidate.Delivery.CreatedAt = time.Unix(deliveryCreatedAt, 0).UTC()
	candidate.Delivery.UpdatedAt = time.Unix(deliveryUpdatedAt, 0).UTC()
	candidate.Delivery.LeaseExpiresAt = reminderTimePointer(lease)
	return candidate, nil
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
	var candidates []reminder.Candidate
	var afterID domain.UserID
	for {
		page, hasMore, err := s.ReminderCandidatesPage(ctx, asOf, afterID, 1000)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, page...)
		if !hasMore || len(page) == 0 {
			return candidates, nil
		}
		afterID = page[len(page)-1].User.ID
	}
}

// ReminderCandidatesPage returns a bounded active-user page and aggregates
// ledger data only for users in that page.
func (s *Store) ReminderCandidatesPage(ctx context.Context, asOf domain.Date, afterID domain.UserID, limit int) ([]reminder.Candidate, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if limit <= 0 {
		return nil, false, fmt.Errorf("reminder candidate page limit must be positive")
	}

	rows, err := s.runner(ctx).QueryContext(ctx, `
		WITH target_users AS (
			SELECT * FROM users
			WHERE status = ? AND id > ?
			ORDER BY id ASC
			LIMIT ?
		), balances AS (
			SELECT entries.user_id, SUM(entries.amount_minor) AS balance_minor
			FROM ledger_entries AS entries
			JOIN target_users ON target_users.id = entries.user_id
			GROUP BY entries.user_id
		), latest_charges AS (
			SELECT charges.user_id, MAX(charges.billing_period_on) AS billing_period_on
			FROM ledger_entries AS charges
			JOIN target_users ON target_users.id = charges.user_id
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
		FROM target_users AS u
		LEFT JOIN balances ON balances.user_id = u.id
		LEFT JOIN latest_charges ON latest_charges.user_id = u.id
		ORDER BY u.id ASC
	`, domain.UserStatusActive, afterID, limit+1, asOf.String())
	if err != nil {
		return nil, false, fmt.Errorf("select reminder candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]reminder.Candidate, 0)
	for rows.Next() {
		candidate, err := scanReminderCandidate(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan reminder candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate reminder candidates: %w", err)
	}
	hasMore := len(candidates) > limit
	if hasMore {
		candidates = candidates[:limit]
	}
	return candidates, hasMore, nil
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
