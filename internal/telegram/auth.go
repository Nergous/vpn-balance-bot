package telegram

import (
	"context"
	"errors"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

func (b *Bot) authenticatedUser(ctx context.Context, telegramUserID int64) (domain.User, error) {
	user, err := b.accounts.UserByTelegramID(ctx, telegramUserID)
	if errors.Is(err, account.ErrNotFound) {
		return domain.User{}, ErrUnauthorized
	}
	return user, err
}
