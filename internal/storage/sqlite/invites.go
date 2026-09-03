package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

const createInviteTokenQuery = `
	INSERT INTO invite_tokens (token_hash, user_id, expires_at, created_at)
	SELECT ?, ?, ?, ?
	WHERE EXISTS (SELECT 1 FROM users WHERE id = ?)
`

const deleteUnusedInviteTokensQuery = `
	DELETE FROM invite_tokens
	WHERE user_id = ? AND used_at IS NULL
`

// CreateInviteToken atomically replaces every unused token for an existing user.
func (s *Store) CreateInviteToken(ctx context.Context, record account.CreateInviteTokenRecord) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	return s.withTransaction(ctx, "invite create", func(txCtx context.Context, tx *sql.Tx) error {
		var linked bool
		err := tx.QueryRowContext(txCtx, `
			SELECT telegram_user_id IS NOT NULL OR telegram_chat_id IS NOT NULL
			FROM users WHERE id = ?
		`, record.UserID).Scan(&linked)
		if errors.Is(err, sql.ErrNoRows) {
			return account.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("check invite user: %w", err)
		}
		if linked {
			return account.ErrInviteUserLinked
		}

		if _, err := tx.ExecContext(txCtx, deleteUnusedInviteTokensQuery, record.UserID); err != nil {
			return fmt.Errorf("delete prior unused invite tokens: %w", err)
		}

		result, err := tx.ExecContext(
			txCtx,
			createInviteTokenQuery,
			record.TokenHash,
			record.UserID,
			record.ExpiresAt.UTC().Unix(),
			record.CreatedAt.UTC().Unix(),
			record.UserID,
		)
		if err != nil {
			return fmt.Errorf("create invite token: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check created invite token: %w", err)
		}
		if rows == 0 {
			return account.ErrNotFound
		}
		return nil
	})
}

const inviteForConsumeQuery = `
	SELECT
		i.user_id,
		i.expires_at,
		i.used_at,
		u.telegram_user_id,
		u.telegram_chat_id
	FROM invite_tokens AS i
	JOIN users AS u ON u.id = i.user_id
	WHERE i.token_hash = ?
`

const claimInviteTokenQuery = `
	UPDATE invite_tokens
	SET used_at = ?
	WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?
`

const bindInviteTelegramQuery = `
	UPDATE users
	SET telegram_user_id = ?, telegram_chat_id = ?, updated_at = ?
	WHERE id = ?
`

// ConsumeInviteToken atomically claims one token and binds its Telegram account.
func (s *Store) ConsumeInviteToken(ctx context.Context, record account.ConsumeInviteTokenRecord) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var user domain.User
	err := s.withTransaction(ctx, "invite consume", func(txCtx context.Context, tx *sql.Tx) error {
		var (
			userID         domain.UserID
			expiresAt      int64
			usedAt         sql.NullInt64
			telegramUserID sql.NullInt64
			telegramChatID sql.NullInt64
		)
		err := tx.QueryRowContext(txCtx, inviteForConsumeQuery, record.TokenHash).Scan(
			&userID,
			&expiresAt,
			&usedAt,
			&telegramUserID,
			&telegramChatID,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return account.ErrInviteNotFound
		}
		if err != nil {
			return fmt.Errorf("read invite token: %w", err)
		}

		consumedAt := record.ConsumedAt.UTC().Unix()
		if usedAt.Valid {
			return account.ErrInviteAlreadyUsed
		}
		if expiresAt <= consumedAt {
			return account.ErrInviteExpired
		}
		if telegramUserID.Valid || telegramChatID.Valid {
			return account.ErrInviteUserLinked
		}

		claimed, err := tx.ExecContext(txCtx, claimInviteTokenQuery, consumedAt, record.TokenHash, consumedAt)
		if err != nil {
			return fmt.Errorf("claim invite token: %w", err)
		}
		claimedRows, err := claimed.RowsAffected()
		if err != nil {
			return fmt.Errorf("check invite token claim: %w", err)
		}
		if claimedRows == 0 {
			return account.ErrInviteAlreadyUsed
		}

		if _, err := tx.ExecContext(
			txCtx,
			bindInviteTelegramQuery,
			record.TelegramUserID,
			record.TelegramChatID,
			consumedAt,
			userID,
		); err != nil {
			return mapTelegramConstraintError("bind Telegram account", err)
		}

		query := `SELECT ` + userColumns + ` FROM users WHERE id = ?`
		user, err = scanUser(tx.QueryRowContext(txCtx, query, userID))
		if err != nil {
			return wrapUserReadError("read bound user", err)
		}
		return nil
	})
	return user, err
}
