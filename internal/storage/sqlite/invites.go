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

func (s *Store) CreateInviteToken(ctx context.Context, record account.CreateInviteTokenRecord) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	result, err := s.db.ExecContext(
		ctx,
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

func (s *Store) ConsumeInviteToken(ctx context.Context, record account.ConsumeInviteTokenRecord) (domain.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.User{}, fmt.Errorf("start invite consume transaction: %w", err)
	}
	defer tx.Rollback()

	var (
		userID         domain.UserID
		expiresAt      int64
		usedAt         sql.NullInt64
		telegramUserID sql.NullInt64
		telegramChatID sql.NullInt64
	)
	err = tx.QueryRowContext(ctx, inviteForConsumeQuery, record.TokenHash).Scan(
		&userID,
		&expiresAt,
		&usedAt,
		&telegramUserID,
		&telegramChatID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, account.ErrInviteNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("read invite token: %w", err)
	}

	consumedAt := record.ConsumedAt.UTC().Unix()
	if usedAt.Valid {
		return domain.User{}, account.ErrInviteAlreadyUsed
	}
	if expiresAt <= consumedAt {
		return domain.User{}, account.ErrInviteExpired
	}
	if telegramUserID.Valid || telegramChatID.Valid {
		return domain.User{}, account.ErrInviteUserLinked
	}

	claimed, err := tx.ExecContext(ctx, claimInviteTokenQuery, consumedAt, record.TokenHash, consumedAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("claim invite token: %w", err)
	}
	claimedRows, err := claimed.RowsAffected()
	if err != nil {
		return domain.User{}, fmt.Errorf("check invite token claim: %w", err)
	}
	if claimedRows == 0 {
		return domain.User{}, account.ErrInviteAlreadyUsed
	}

	if _, err := tx.ExecContext(
		ctx,
		bindInviteTelegramQuery,
		record.TelegramUserID,
		record.TelegramChatID,
		consumedAt,
		userID,
	); err != nil {
		return domain.User{}, mapTelegramConstraintError("bind Telegram account", err)
	}

	query := `SELECT ` + userColumns + ` FROM users WHERE id = ?`
	user, err := scanUser(tx.QueryRowContext(ctx, query, userID))
	if err != nil {
		return domain.User{}, wrapUserReadError("read bound user", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.User{}, fmt.Errorf("commit invite consume transaction: %w", err)
	}

	return user, nil
}
