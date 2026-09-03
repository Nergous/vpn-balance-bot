package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/billing"
	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// UsersDueForCharge returns active users with a billing date on or before asOf.
func (s *Store) UsersDueForCharge(ctx context.Context, asOf domain.Date, limit int) ([]domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	rows, err := s.runner(ctx).QueryContext(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE status = ? AND next_charge_on <= ?
		ORDER BY next_charge_on ASC, id ASC
		LIMIT ?
	`, domain.UserStatusActive, asOf.String(), limit)

	if err != nil {
		return nil, fmt.Errorf("select users due for charge: %w", err)
	}

	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user due for charge: %w", err)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users due for charge: %w", err)
	}

	return users, nil
}

// ReserveSubscriptionCharge atomically creates one charge and advances its next date.
// created is false when another worker already processed the same billing period.
func (s *Store) ReserveSubscriptionCharge(ctx context.Context, params billing.ReserveSubscriptionChargeParams) (domain.LedgerEntry, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var entry domain.LedgerEntry
	created := false
	err := s.withTransaction(ctx, "subscription charge", func(txCtx context.Context, tx *sql.Tx) error {
		user, err := scanUser(tx.QueryRowContext(txCtx, `SELECT `+userColumns+` FROM users WHERE id = ?`, params.UserID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read billing user: %w", err)
		}

		if user.Status != domain.UserStatusActive || user.NextChargeOn != params.BillingPeriodOn {
			return nil
		}

		nextChargeOn, err := user.NextChargeOn.NextBillingDate(user.BillingAnchorDay)
		if err != nil {
			return fmt.Errorf("calculate next charge date: %w", err)
		}
		if err := ensureBalanceRange(txCtx, tx, user.ID, -user.MonthlyFeeMinor); err != nil {
			return err
		}

		billingPeriodOn := user.NextChargeOn
		entry, err = insertLedgerEntry(txCtx, tx, domain.LedgerEntry{
			UserID:          user.ID,
			Kind:            domain.LedgerKindSubscriptionCharge,
			AmountMinor:     -user.MonthlyFeeMinor,
			OccurredAt:      params.OccurredAt,
			BillingPeriodOn: &billingPeriodOn,
			CreatedAt:       params.UpdatedAt,
		})

		if err != nil {
			if isSubscriptionChargeConflict(err) {
				return nil
			}

			return err
		}

		result, err := tx.ExecContext(txCtx, `
		UPDATE users SET next_charge_on = ?, updated_at = ? WHERE id = ?
	`, nextChargeOn.String(), params.UpdatedAt.UTC().Unix(), user.ID)
		if err != nil {
			return fmt.Errorf("advance next charge date: %w", err)
		}

		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check next charge date update: %w", err)
		}

		if updated != 1 {
			return fmt.Errorf("advance next charge date: unexpected updated rows %d", updated)
		}
		created = true
		return nil
	})
	return entry, created, err
}

func isSubscriptionChargeConflict(err error) bool {
	var sqliteErr *sqliteDriver.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE &&
		strings.Contains(err.Error(), "ledger_entries.user_id")
}
