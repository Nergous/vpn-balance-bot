package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

func (b *Bot) HandleStart(ctx context.Context, message IncomingMessage) error {
	parts := strings.Fields(message.Text)
	if len(parts) >= 2 {
		_, err := b.accounts.ConsumeInviteToken(ctx, account.ConsumeInviteParams{Token: parts[1], TelegramUserID: message.UserID, TelegramChatID: message.ChatID})
		if err == nil {
			return b.send(ctx, message.ChatID, "Профиль привязан. Используйте /status.")
		}
		return b.send(ctx, message.ChatID, inviteErrorText(err))
	}
	return b.send(ctx, message.ChatID, "VPN Balance Bot\n/status — мой статус\n/history — история\n/help — помощь")
}

func (b *Bot) HandleStatus(ctx context.Context, message IncomingMessage) error {
	user, err := b.authenticatedUser(ctx, message.UserID)
	if err != nil {
		return b.send(ctx, message.ChatID, "Профиль не привязан. Используйте invite link от администратора.")
	}
	balance, err := b.accounts.Balance(ctx, user.ID)
	if err != nil {
		return err
	}
	entries, err := b.accounts.LastLedgerEntries(ctx, user.ID)
	if err != nil {
		return err
	}
	return b.send(ctx, message.ChatID, formatStatus(user, balance, entries))
}

func (b *Bot) HandleHistory(ctx context.Context, message IncomingMessage) error {
	user, err := b.authenticatedUser(ctx, message.UserID)
	if err != nil {
		return b.send(ctx, message.ChatID, "Профиль не привязан.")
	}
	entries, err := b.accounts.LastLedgerEntries(ctx, user.ID)
	if err != nil {
		return err
	}
	return b.send(ctx, message.ChatID, formatHistory(entries))
}

func (b *Bot) HandleHelp(ctx context.Context, message IncomingMessage) error {
	return b.send(ctx, message.ChatID, "Команды: /status, /history, /help")
}

func inviteErrorText(err error) string {
	switch {
	case errors.Is(err, account.ErrInviteExpired):
		return "Invite link истёк. Запросите новый у администратора."
	case errors.Is(err, account.ErrInviteAlreadyUsed):
		return "Invite link уже использован."
	case errors.Is(err, account.ErrInviteNotFound), errors.Is(err, account.ErrInvalidInviteToken):
		return "Invite link недействителен."
	case errors.Is(err, account.ErrTelegramUserIDTaken), errors.Is(err, account.ErrTelegramChatIDTaken):
		return "Этот Telegram account уже привязан к другому профилю."
	default:
		return "Не удалось привязать профиль. Повторите позже."
	}
}

func formatStatus(user domain.User, balance domain.AmountMinor, entries []domain.LedgerEntry) string {
	state := "Баланс: 0"
	if balance < 0 {
		state = fmt.Sprintf("Долг: %d %s", -balance, user.Currency)
	}
	if balance > 0 {
		state = fmt.Sprintf("Предоплата: %d %s", balance, user.Currency)
	}
	text := fmt.Sprintf("%s\nТариф: %d %s\nСледующее списание: %s\n%s", user.DisplayName, user.MonthlyFeeMinor, user.Currency, user.NextChargeOn, state)
	for _, entry := range entries {
		if entry.Kind == domain.LedgerKindPayment {
			return text + fmt.Sprintf("\nПоследняя оплата: %d %s", entry.AmountMinor, user.Currency)
		}
	}
	return text
}

func formatHistory(entries []domain.LedgerEntry) string {
	if len(entries) == 0 {
		return "Операций пока нет."
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		note := ""
		if entry.Note != nil {
			note = " — " + *entry.Note
		}
		lines = append(lines, fmt.Sprintf("%s · %s · %+d%s", entry.OccurredAt.Format("2006-01-02"), entry.Kind, entry.AmountMinor, note))
	}
	return strings.Join(lines, "\n")
}
