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

func (s *Store) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.db.ExecContext(
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

func (s *Store) UserByID(ctx context.Context, userID domain.UserID) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	query := `SELECT ` + userColumns + ` FROM users WHERE id = ?`
	user, err := scanUser(s.db.QueryRowContext(ctx, query, userID))
	if err != nil {
		return domain.User{}, wrapUserReadError("get user by ID", err)
	}

	return user, nil
}

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

	rows, err := s.db.QueryContext(ctx, query, args...)
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

func (s *Store) UserByTelegramID(ctx context.Context, telegramID int64) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	query := `SELECT ` + userColumns + ` FROM users WHERE telegram_user_id = ?`
	user, err := scanUser(s.db.QueryRowContext(ctx, query, telegramID))
	if err != nil {
		return domain.User{}, wrapUserReadError("get user by Telegram ID", err)
	}

	return user, nil
}

func (s *Store) SetMonthlyFee(
	ctx context.Context,
	userID domain.UserID,
	fee domain.AmountMinor,
	updatedAt time.Time,
) (domain.User, error) {
	query := `
		UPDATE users
		SET monthly_fee_minor = ?, updated_at = ?
		WHERE id = ?
		RETURNING ` + userColumns

	return s.updateUser(
		ctx,
		"set user monthly fee",
		query,
		fee.Int64(),
		updatedAt.UTC().Unix(),
		userID,
	)
}

func (s *Store) PauseUser(
	ctx context.Context,
	userID domain.UserID,
	updatedAt time.Time,
) (domain.User, error) {
	return s.setUserStatus(ctx, userID, domain.UserStatusPaused, nil, updatedAt)
}

func (s *Store) ResumeUser(
	ctx context.Context,
	userID domain.UserID,
	nextChargeOn domain.Date,
	updatedAt time.Time,
) (domain.User, error) {
	if !nextChargeOn.IsValid() {
		return domain.User{}, domain.ErrInvalidDate
	}

	return s.setUserStatus(ctx, userID, domain.UserStatusActive, &nextChargeOn, updatedAt)
}

func (s *Store) DisableUser(
	ctx context.Context,
	userID domain.UserID,
	updatedAt time.Time,
) (domain.User, error) {
	return s.setUserStatus(ctx, userID, domain.UserStatusDisabled, nil, updatedAt)
}

func (s *Store) setUserStatus(
	ctx context.Context,
	userID domain.UserID,
	status domain.UserStatus,
	nextChargeOn *domain.Date,
	updatedAt time.Time,
) (domain.User, error) {
	query := `UPDATE users SET status = ?, updated_at = ?`
	args := []any{status, updatedAt.UTC().Unix()}

	if nextChargeOn != nil {
		query += `, next_charge_on = ?`
		args = append(args, nextChargeOn.String())
	}

	query += ` WHERE id = ? RETURNING ` + userColumns
	args = append(args, userID)

	return s.updateUser(ctx, "set user status", query, args...)
}

func (s *Store) updateUser(
	ctx context.Context,
	operation string,
	query string,
	args ...any,
) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	user, err := scanUser(s.db.QueryRowContext(ctx, query, args...))
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
	return mapTelegramConstraintError("create user", err)
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
