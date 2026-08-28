package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
)

const insertReminderDelivery = "INSERT INTO reminder_deliveries (user_id, billing_date, reminder_type, scheduled_date, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(user_id, billing_date, reminder_type) DO NOTHING"

func (s *Store) CreateReminderDelivery(ctx context.Context, delivery domain.ReminderDelivery) (domain.ReminderDelivery, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.db.ExecContext(ctx, insertReminderDelivery,
		delivery.UserID,
		delivery.BillingDate.String(),
		delivery.ReminderType,
		delivery.ScheduledDate.String(),
		delivery.Status,
		delivery.CreatedAt.UTC().Unix(),
		delivery.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return domain.ReminderDelivery{}, false, fmt.Errorf("create reminder delivery: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.ReminderDelivery{}, false, fmt.Errorf("check reminder reservation: %w", err)
	}
	if rows == 1 {
		return delivery, true, nil
	}

	existing, err := s.reminderDelivery(ctx, delivery.UserID, delivery.BillingDate, delivery.ReminderType)
	return existing, false, err
}

const updateReminderDelivery = "UPDATE reminder_deliveries SET status = ?, sent_at = ?, telegram_message_id = ?, error_code = ?, updated_at = ? WHERE user_id = ? AND billing_date = ? AND reminder_type = ?"

func (s *Store) UpdateReminderDelivery(ctx context.Context, delivery domain.ReminderDelivery) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	_, err := s.db.ExecContext(ctx, updateReminderDelivery,
		delivery.Status,
		unixOrNil(delivery.SentAt),
		int64OrNil(delivery.TelegramMessageID),
		delivery.ErrorCode,
		delivery.UpdatedAt.UTC().Unix(),
		delivery.UserID,
		delivery.BillingDate.String(),
		delivery.ReminderType,
	)
	if err != nil {
		return fmt.Errorf("update reminder delivery: %w", err)
	}
	return nil
}

const markPendingUnknown = "UPDATE reminder_deliveries SET status = 'failed', error_code = 'delivery_state_unknown', updated_at = ? WHERE status = 'pending'"

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

const selectReminderDelivery = "SELECT billing_date, scheduled_date, status, sent_at, telegram_message_id, error_code, created_at, updated_at FROM reminder_deliveries WHERE user_id = ? AND billing_date = ? AND reminder_type = ?"

func (s *Store) reminderDelivery(ctx context.Context, userID domain.UserID, billingDate domain.Date, kind domain.ReminderType) (domain.ReminderDelivery, error) {
	var (
		delivery             domain.ReminderDelivery
		billing, scheduled   string
		sentAt               sql.NullInt64
		messageID            sql.NullInt64
		errorCode            sql.NullString
		createdAt, updatedAt int64
	)
	err := s.db.QueryRowContext(ctx, selectReminderDelivery, userID, billingDate.String(), kind).Scan(
		&billing, &scheduled, &delivery.Status, &sentAt, &messageID, &errorCode, &createdAt, &updatedAt,
	)
	if err != nil {
		return delivery, err
	}
	delivery.UserID = userID
	delivery.ReminderType = kind
	delivery.BillingDate, err = domain.ParseDate(billing)
	if err != nil {
		return domain.ReminderDelivery{}, err
	}
	delivery.ScheduledDate, err = domain.ParseDate(scheduled)
	if err != nil {
		return domain.ReminderDelivery{}, err
	}
	delivery.SentAt = reminderTimePointer(sentAt)
	delivery.TelegramMessageID = int64Pointer(messageID)
	delivery.ErrorCode = stringPointer(errorCode)
	delivery.CreatedAt = time.Unix(createdAt, 0).UTC()
	delivery.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return delivery, nil
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
