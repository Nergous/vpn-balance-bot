package telegram

import (
	"context"
	"errors"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

// authenticatedUser resolves a profile by Telegram user ID and verifies that
// the request came from the chat stored during invite consumption.
func (b *Bot) authenticatedUser(ctx context.Context, telegramUserID, telegramChatID int64) (domain.User, error) {
	user, err := b.accounts.UserByTelegramID(ctx, telegramUserID)

	if errors.Is(err, account.ErrNotFound) {
		return domain.User{}, ErrUnauthorized
	}

	if err != nil {
		return domain.User{}, err
	}
	if user.TelegramChatID == nil || *user.TelegramChatID != telegramChatID {
		return domain.User{}, ErrUnauthorized
	}

	return user, nil
}
