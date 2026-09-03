package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

func (s *Store) SavePaymentDraft(ctx context.Context, draft account.PaymentDraftRecord) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	_, err := s.runner(ctx).ExecContext(ctx, `
		INSERT INTO payment_drafts (admin_telegram_id, user_id, amount_minor, note, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(admin_telegram_id) DO UPDATE SET
			user_id = excluded.user_id,
			amount_minor = excluded.amount_minor,
			note = excluded.note,
			updated_at = excluded.updated_at
	`, draft.AdminTelegramID, draft.UserID, draft.AmountMinor, draft.Note, draft.UpdatedAt.UTC().Unix())
	if err != nil {
		return fmt.Errorf("save payment draft: %w", err)
	}
	return nil
}

func (s *Store) PaymentDraft(ctx context.Context, adminID int64) (account.PaymentDraftRecord, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	var draft account.PaymentDraftRecord
	var note sql.NullString
	var updatedAt int64
	err := s.runner(ctx).QueryRowContext(ctx, `
		SELECT user_id, amount_minor, note, updated_at
		FROM payment_drafts WHERE admin_telegram_id = ?
	`, adminID).Scan(&draft.UserID, &draft.AmountMinor, &note, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return account.PaymentDraftRecord{}, false, nil
		}
		return account.PaymentDraftRecord{}, false, fmt.Errorf("load payment draft: %w", err)
	}
	draft.AdminTelegramID = adminID
	draft.Note = stringPointer(note)
	draft.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return draft, true, nil
}

func (s *Store) DeletePaymentDraft(ctx context.Context, adminID int64) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	if _, err := s.runner(ctx).ExecContext(ctx, "DELETE FROM payment_drafts WHERE admin_telegram_id = ?", adminID); err != nil {
		return fmt.Errorf("delete payment draft: %w", err)
	}
	return nil
}
