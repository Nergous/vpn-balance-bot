package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	sqliteDriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const userColumns = `
	id,
	telegram_user_id,
	telegram_chat_id,
	username,
	display_name,
	monthly_fee_minor,
	currency,
	billing_anchor_day,
	next_charge_on,
	status,
	created_at,
	updated_at
`

const createUserQuery = `
	INSERT INTO users (
		telegram_user_id,
		telegram_chat_id,
		username,
		display_name,
		monthly_fee_minor,
		currency,
		billing_anchor_day,
		next_charge_on,
		status,
		created_at,
		updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

// CreateUser persists a new billing profile and returns its generated ID.
func (s *Store) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.runner(ctx).ExecContext(
		ctx,
		createUserQuery,
		user.TelegramUserID,
		user.TelegramChatID,
		user.Username,
		user.DisplayName,
		user.MonthlyFeeMinor.Int64(),
		user.Currency,
		user.BillingAnchorDay,
		user.NextChargeOn.String(),
		user.Status,
		user.CreatedAt.UTC().Unix(),
		user.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return domain.User{}, mapCreateUserError(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return domain.User{}, fmt.Errorf("get created user ID: %w", err)
	}

	user.ID = domain.UserID(id)
	user.CreatedAt = user.CreatedAt.UTC().Truncate(time.Second)
	user.UpdatedAt = user.UpdatedAt.UTC().Truncate(time.Second)

	return user, nil
}

const selectUserByIDQuery = `SELECT ` + userColumns + ` FROM users WHERE id = ?`

// UserByID returns one profile by its internal identifier.
func (s *Store) UserByID(ctx context.Context, userID domain.UserID) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	user, err := scanUser(s.runner(ctx).QueryRowContext(ctx, selectUserByIDQuery, userID))
	if err != nil {
		return domain.User{}, wrapUserReadError("UserByID", err)
	}

	return user, nil
}

// ListUsers returns profiles, optionally restricted to one status.
func (s *Store) ListUsers(ctx context.Context, status *domain.UserStatus) ([]domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	query := `SELECT ` + userColumns + ` FROM users`

	var args []any
	if status != nil {
		query += ` WHERE status = ?`
		args = append(args, *status)
	}
	query += ` ORDER BY id ASC`

	rows, err := s.runner(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan listed user: %w", err)
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

// ListUsersPage returns a bounded page and whether another page exists.
func (s *Store) ListUsersPage(ctx context.Context, status *domain.UserStatus, afterID domain.UserID, limit int) ([]domain.User, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	query := `SELECT ` + userColumns + ` FROM users WHERE id > ?`
	args := []any{afterID}
	if status != nil {
		query += ` AND status = ?`
		args = append(args, *status)
	}
	query += ` ORDER BY id ASC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.runner(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list user page: %w", err)
	}
	defer rows.Close()

	users := make([]domain.User, 0, limit+1)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan user page: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate user page: %w", err)
	}

	hasMore := len(users) > limit
	if hasMore {
		users = users[:limit]
	}
	return users, hasMore, nil
}

// UserStatusCounts returns aggregate profile counts for the admin dashboard.
func (s *Store) UserStatusCounts(ctx context.Context) (account.UserStatusCounts, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var counts account.UserStatusCounts
	err := s.runner(ctx).QueryRowContext(ctx, `
		WITH balances AS (
			SELECT user_id, SUM(amount_minor) AS balance_minor
			FROM ledger_entries
			GROUP BY user_id
		), latest_deliveries AS (
			SELECT user_id,
			       error_code,
			       ROW_NUMBER() OVER (
				   PARTITION BY user_id
				   ORDER BY updated_at DESC, rowid DESC
			       ) AS sequence
			FROM reminder_deliveries
		)
		SELECT COUNT(*),
		       COALESCE(SUM(u.status = 'active'), 0),
		       COALESCE(SUM(u.status = 'paused'), 0),
		       COALESCE(SUM(u.status = 'disabled'), 0),
		       COALESCE(SUM(COALESCE(b.balance_minor, 0) < 0), 0),
		       COALESCE(SUM(u.status = 'active' AND COALESCE(b.balance_minor, 0) < u.monthly_fee_minor), 0),
		       COALESCE(SUM(u.telegram_user_id IS NULL OR u.telegram_chat_id IS NULL), 0),
		       COALESCE(SUM(COALESCE(d.error_code, '') = 'unreachable'), 0)
		FROM users AS u
		LEFT JOIN balances AS b ON b.user_id = u.id
		LEFT JOIN latest_deliveries AS d ON d.user_id = u.id AND d.sequence = 1
	`).Scan(
		&counts.Total,
		&counts.Active,
		&counts.Paused,
		&counts.Disabled,
		&counts.Debtors,
		&counts.Insufficient,
		&counts.Unlinked,
		&counts.Unreachable,
	)
	if err != nil {
		return account.UserStatusCounts{}, fmt.Errorf("count users by status: %w", err)
	}
	return counts, nil
}

const selectUserByTelegramIDQuery = `SELECT ` + userColumns + ` FROM users WHERE telegram_user_id = ?`

// UserByTelegramID resolves a profile using its numeric Telegram user ID.
func (s *Store) UserByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	user, err := scanUser(s.runner(ctx).QueryRowContext(ctx, selectUserByTelegramIDQuery, telegramID))
	if err != nil {
		return domain.User{}, wrapUserReadError("UserByTelegramID", err)
	}

	return user, nil
}

const updateMonthlyFeeQuery = `UPDATE users SET monthly_fee_minor = ?, updated_at = ? WHERE id = ? RETURNING ` + userColumns

// SetMonthlyFee changes only the fee used by future subscription charges.
func (s *Store) SetMonthlyFee(
	ctx context.Context,
	userID domain.UserID,
	fee domain.AmountMinor,
	updatedAt time.Time,
) (domain.User, error) {

	return s.updateUser(
		ctx,
		"SetMonthlyFee",
		updateMonthlyFeeQuery,
		fee.Int64(),
		updatedAt.UTC().Unix(),
		userID,
	)
}

// PauseUser stops future automatic billing without changing ledger history.
func (s *Store) PauseUser(
	ctx context.Context,
	userID domain.UserID,
	updatedAt time.Time,
) (domain.User, error) {
	return s.setUserStatus(ctx, userID, domain.UserStatusPaused, nil, updatedAt, domain.UserStatusActive)
}

// ResumeUser activates a profile with an explicitly chosen next charge date.
func (s *Store) ResumeUser(
	ctx context.Context,
	userID domain.UserID,
	nextChargeOn domain.Date,
	updatedAt time.Time,
) (domain.User, error) {
	if !nextChargeOn.IsValid() {
		return domain.User{}, domain.ErrInvalidDate
	}

	return s.setUserStatus(ctx, userID, domain.UserStatusActive, &nextChargeOn, updatedAt, domain.UserStatusPaused)
}

// DisableUser permanently excludes a profile from automatic billing and reminders.
func (s *Store) DisableUser(
	ctx context.Context,
	userID domain.UserID,
	updatedAt time.Time,
) (domain.User, error) {
	return s.setUserStatus(
		ctx,
		userID,
		domain.UserStatusDisabled,
		nil,
		updatedAt,
		domain.UserStatusActive,
		domain.UserStatusPaused,
	)
}

func (s *Store) setUserStatus(
	ctx context.Context,
	userID domain.UserID,
	status domain.UserStatus,
	nextChargeOn *domain.Date,
	updatedAt time.Time,
	fromStatuses ...domain.UserStatus,
) (domain.User, error) {
	query := `UPDATE users SET status = ?, updated_at = ?`
	args := []any{status, updatedAt.UTC().Unix()}

	if nextChargeOn != nil {
		query += `, next_charge_on = ?`
		args = append(args, nextChargeOn.String())
	}

	query += ` WHERE id = ? AND status IN (`
	args = append(args, userID)
	placeholders := make([]string, len(fromStatuses))
	for index, fromStatus := range fromStatuses {
		placeholders[index] = "?"
		args = append(args, fromStatus)
	}
	query += strings.Join(placeholders, ", ") + `) RETURNING ` + userColumns

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	runner := s.runner(ctx)
	user, err := scanUser(runner.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if existsErr := runner.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`, userID).Scan(&exists); existsErr != nil {
			return domain.User{}, fmt.Errorf("check user status transition: %w", existsErr)
		}
		if !exists {
			return domain.User{}, account.ErrNotFound
		}
		return domain.User{}, account.ErrInvalidUserStatusTransition
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("set user status: %w", err)
	}

	return user, nil
}

func (s *Store) updateUser(
	ctx context.Context,
	operation string,
	query string,
	args ...any,
) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	user, err := scanUser(s.runner(ctx).QueryRowContext(ctx, query, args...))
	if err != nil {
		return domain.User{}, wrapUserReadError(operation, err)
	}

	return user, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(scanner rowScanner) (domain.User, error) {
	var (
		user             domain.User
		telegramUserID   sql.NullInt64
		telegramChatID   sql.NullInt64
		username         sql.NullString
		nextChargeOn     string
		createdAtSeconds int64
		updatedAtSeconds int64
	)

	err := scanner.Scan(
		&user.ID,
		&telegramUserID,
		&telegramChatID,
		&username,
		&user.DisplayName,
		&user.MonthlyFeeMinor,
		&user.Currency,
		&user.BillingAnchorDay,
		&nextChargeOn,
		&user.Status,
		&createdAtSeconds,
		&updatedAtSeconds,
	)
	if err != nil {
		return domain.User{}, err
	}

	parsedNextChargeOn, err := domain.ParseDate(nextChargeOn)
	if err != nil {
		return domain.User{}, fmt.Errorf("parse next charge date: %w", err)
	}

	user.TelegramUserID = int64Pointer(telegramUserID)
	user.TelegramChatID = int64Pointer(telegramChatID)
	user.Username = stringPointer(username)
	user.NextChargeOn = parsedNextChargeOn
	user.CreatedAt = time.Unix(createdAtSeconds, 0).UTC()
	user.UpdatedAt = time.Unix(updatedAtSeconds, 0).UTC()

	return user, nil
}

func int64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func wrapUserReadError(operation string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, account.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func mapCreateUserError(err error) error {
	return mapTelegramConstraintError("CreateUser", err)
}

func mapTelegramConstraintError(operation string, err error) error {
	var sqliteErr *sqliteDriver.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code() != sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return fmt.Errorf("%s: %w", operation, err)
	}

	switch {
	case strings.Contains(err.Error(), "users.telegram_user_id"):
		return fmt.Errorf("%s: %w", operation, account.ErrTelegramUserIDTaken)
	case strings.Contains(err.Error(), "users.telegram_chat_id"):
		return fmt.Errorf("%s: %w", operation, account.ErrTelegramChatIDTaken)
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}
