package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const ledgerColumns = `
	id,
	user_id,
	kind,
	amount_minor,
	occurred_at,
	billing_period_on,
	reverses_entry_id,
	created_by_telegram_id,
	note,
	created_at
`

const insertLedgerEntryQuery = `
	INSERT INTO ledger_entries (
		user_id,
		kind,
		amount_minor,
		occurred_at,
		billing_period_on,
		reverses_entry_id,
		created_by_telegram_id,
		note,
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	RETURNING ` + ledgerColumns

// CreateLedgerEntry appends one immutable financial entry for an existing user.
func (s *Store) CreateLedgerEntry(ctx context.Context, entry domain.LedgerEntry) (domain.LedgerEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("start ledger transaction: %w", err)
	}
	defer tx.Rollback()

	if err := ensureLedgerUser(ctx, tx, entry.UserID); err != nil {
		return domain.LedgerEntry{}, err
	}

	created, err := insertLedgerEntry(ctx, tx, entry)
	if err != nil {
		return domain.LedgerEntry{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("commit ledger transaction: %w", err)
	}

	return created, nil
}

// Balance returns the signed sum of every ledger entry for one user.
func (s *Store) Balance(ctx context.Context, userID domain.UserID) (domain.AmountMinor, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if err := ensureLedgerUser(ctx, s.db, userID); err != nil {
		return 0, err
	}

	var balance domain.AmountMinor
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_minor), 0) FROM ledger_entries WHERE user_id = ?
	`, userID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("calculate balance: %w", err)
	}

	return balance, nil
}

// LastLedgerEntries returns newest immutable entries in deterministic order.
func (s *Store) LastLedgerEntries(ctx context.Context, userID domain.UserID, limit int) ([]domain.LedgerEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if err := ensureLedgerUser(ctx, s.db, userID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+ledgerColumns+`
		FROM ledger_entries
		WHERE user_id = ?
		ORDER BY occurred_at DESC, id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.LedgerEntry, 0)
	for rows.Next() {
		entry, err := scanLedgerEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger entries: %w", err)
	}

	return entries, nil
}

// LastUnreversedPayment returns the newest payment not targeted by any reversal.
func (s *Store) LastUnreversedPayment(ctx context.Context, userID domain.UserID) (domain.LedgerEntry, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	if err := ensureLedgerUser(ctx, s.db, userID); err != nil {
		return domain.LedgerEntry{}, false, err
	}

	entry, err := scanLedgerEntry(s.db.QueryRowContext(ctx, `
		SELECT `+ledgerColumns+`
		FROM ledger_entries AS payment
		WHERE payment.user_id = ?
			AND payment.kind = ?
			AND NOT EXISTS (
				SELECT 1
				FROM ledger_entries AS reversal
				WHERE reversal.reverses_entry_id = payment.id
			)
		ORDER BY payment.occurred_at DESC, payment.id DESC
		LIMIT 1
	`, userID, domain.LedgerKindPayment))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LedgerEntry{}, false, nil
	}
	if err != nil {
		return domain.LedgerEntry{}, false, fmt.Errorf("find last unreversed payment: %w", err)
	}

	return entry, true, nil
}

// ReverseLedgerEntry appends one opposite-signed entry without editing history.
func (s *Store) ReverseLedgerEntry(ctx context.Context, record account.ReverseLedgerEntryRecord) (domain.LedgerEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("start reversal transaction: %w", err)
	}

	defer tx.Rollback()

	original, err := ledgerEntryByID(ctx, tx, record.EntryID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LedgerEntry{}, account.ErrLedgerEntryNotFound
	}

	if err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("read reversal source: %w", err)
	}

	if original.UserID != record.UserID {
		return domain.LedgerEntry{}, account.ErrReversalUserMismatch
	}

	var alreadyReversed bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM ledger_entries WHERE reverses_entry_id = ?)
	`, original.ID).Scan(&alreadyReversed); err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("check existing reversal: %w", err)
	}

	if alreadyReversed {
		return domain.LedgerEntry{}, account.ErrLedgerEntryAlreadyReversed
	}

	if original.AmountMinor.Int64() == math.MinInt64 {
		return domain.LedgerEntry{}, domain.ErrAmountOverflow
	}

	reversesEntryID := original.ID
	createdByTelegramID := record.CreatedByTelegramID
	note := record.Note
	reversal, err := insertLedgerEntry(ctx, tx, domain.LedgerEntry{
		UserID:              original.UserID,
		Kind:                domain.LedgerKindReversal,
		AmountMinor:         -original.AmountMinor,
		OccurredAt:          record.OccurredAt,
		ReversesEntryID:     &reversesEntryID,
		CreatedByTelegramID: &createdByTelegramID,
		Note:                &note,
		CreatedAt:           record.CreatedAt,
	})

	if err != nil {
		return domain.LedgerEntry{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.LedgerEntry{}, fmt.Errorf("commit reversal transaction: %w", err)
	}

	return reversal, nil
}

type ledgerRowScanner interface {
	Scan(dest ...any) error
}

func insertLedgerEntry(ctx context.Context, tx *sql.Tx, entry domain.LedgerEntry) (domain.LedgerEntry, error) {
	var billingPeriodOn any
	if entry.BillingPeriodOn != nil {
		billingPeriodOn = entry.BillingPeriodOn.String()
	}

	created, err := scanLedgerEntry(tx.QueryRowContext(ctx, insertLedgerEntryQuery,
		entry.UserID,
		entry.Kind,
		entry.AmountMinor.Int64(),
		entry.OccurredAt.UTC().Unix(),
		billingPeriodOn,
		entry.ReversesEntryID,
		entry.CreatedByTelegramID,
		entry.Note,
		entry.CreatedAt.UTC().Unix(),
	))

	if err != nil {
		return domain.LedgerEntry{}, mapLedgerInsertError(err)
	}

	return created, nil
}

func ledgerEntryByID(ctx context.Context, tx *sql.Tx, entryID int64) (domain.LedgerEntry, error) {
	return scanLedgerEntry(tx.QueryRowContext(ctx, `SELECT `+ledgerColumns+` FROM ledger_entries WHERE id = ?`, entryID))
}

func scanLedgerEntry(scanner ledgerRowScanner) (domain.LedgerEntry, error) {
	var (
		entry               domain.LedgerEntry
		billingPeriodOn     sql.NullString
		reversesEntryID     sql.NullInt64
		createdByTelegramID sql.NullInt64
		note                sql.NullString
		occurredAtSeconds   int64
		createdAtSeconds    int64
	)

	if err := scanner.Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Kind,
		&entry.AmountMinor,
		&occurredAtSeconds,
		&billingPeriodOn,
		&reversesEntryID,
		&createdByTelegramID,
		&note,
		&createdAtSeconds,
	); err != nil {
		return domain.LedgerEntry{}, err
	}

	if billingPeriodOn.Valid {
		date, err := domain.ParseDate(billingPeriodOn.String)
		if err != nil {
			return domain.LedgerEntry{}, fmt.Errorf("parse billing period: %w", err)
		}

		entry.BillingPeriodOn = &date
	}

	entry.ReversesEntryID = int64Pointer(reversesEntryID)
	entry.CreatedByTelegramID = int64Pointer(createdByTelegramID)
	entry.Note = stringPointer(note)
	entry.OccurredAt = time.Unix(occurredAtSeconds, 0).UTC()
	entry.CreatedAt = time.Unix(createdAtSeconds, 0).UTC()

	return entry, nil
}

type ledgerUserQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ensureLedgerUser(ctx context.Context, queryer ledgerUserQuerier, userID domain.UserID) error {
	var exists bool
	if err := queryer.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`, userID).Scan(&exists); err != nil {
		return fmt.Errorf("check ledger user: %w", err)
	}

	if !exists {
		return account.ErrNotFound
	}

	return nil
}

func mapLedgerInsertError(err error) error {
	var sqliteErr *sqliteDriver.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE &&
		strings.Contains(err.Error(), "ledger_entries.reverses_entry_id") {
		return account.ErrLedgerEntryAlreadyReversed
	}

	return fmt.Errorf("create ledger entry: %w", err)
}
