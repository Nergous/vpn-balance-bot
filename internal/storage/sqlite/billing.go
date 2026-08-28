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

func (s *Store) UsersDueForCharge(ctx context.Context, asOf domain.Date) ([]domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE status = ? AND next_charge_on <= ?
		ORDER BY next_charge_on ASC, id ASC
	`, domain.UserStatusActive, asOf.String())
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

func (s *Store) ReserveSubscriptionCharge(ctx context.Context, params billing.ReserveSubscriptionChargeParams) (domain.LedgerEntry, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LedgerEntry{}, false, fmt.Errorf("start subscription charge transaction: %w", err)
	}
	defer tx.Rollback()

	user, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, params.UserID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LedgerEntry{}, false, nil
	}
	if err != nil {
		return domain.LedgerEntry{}, false, fmt.Errorf("read billing user: %w", err)
	}
	if user.Status != domain.UserStatusActive || user.NextChargeOn != params.BillingPeriodOn {
		return domain.LedgerEntry{}, false, nil
	}

	nextChargeOn, err := user.NextChargeOn.NextBillingDate(user.BillingAnchorDay)
	if err != nil {
		return domain.LedgerEntry{}, false, fmt.Errorf("calculate next charge date: %w", err)
	}
	billingPeriodOn := user.NextChargeOn
	entry, err := insertLedgerEntry(ctx, tx, domain.LedgerEntry{
		UserID:          user.ID,
		Kind:            domain.LedgerKindSubscriptionCharge,
		AmountMinor:     -user.MonthlyFeeMinor,
		OccurredAt:      params.OccurredAt,
		BillingPeriodOn: &billingPeriodOn,
		CreatedAt:       params.UpdatedAt,
	})
	if err != nil {
		if isSubscriptionChargeConflict(err) {
			return domain.LedgerEntry{}, false, nil
		}
		return domain.LedgerEntry{}, false, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE users SET next_charge_on = ?, updated_at = ? WHERE id = ?
	`, nextChargeOn.String(), params.UpdatedAt.UTC().Unix(), user.ID)
	if err != nil {
		return domain.LedgerEntry{}, false, fmt.Errorf("advance next charge date: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return domain.LedgerEntry{}, false, fmt.Errorf("check next charge date update: %w", err)
	}
	if updated != 1 {
		return domain.LedgerEntry{}, false, fmt.Errorf("advance next charge date: unexpected updated rows %d", updated)
	}

	if err := tx.Commit(); err != nil {
		return domain.LedgerEntry{}, false, fmt.Errorf("commit subscription charge transaction: %w", err)
	}

	return entry, true, nil
}

func isSubscriptionChargeConflict(err error) bool {
	var sqliteErr *sqliteDriver.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE &&
		strings.Contains(err.Error(), "ledger_entries.user_id")
}
