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
			return b.send(ctx, message.ChatID, localized(b.language, "start_bound"))
		}
		return b.send(ctx, message.ChatID, inviteErrorText(b.language, err))
	}
	return b.send(ctx, message.ChatID, localized(b.language, "start_welcome"))
}

func (b *Bot) HandleStatus(ctx context.Context, message IncomingMessage) error {
	user, err := b.authenticatedUser(ctx, message.UserID)
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "unlinked_status"))
	}
	balance, err := b.accounts.Balance(ctx, user.ID)
	if err != nil {
		return err
	}
	entries, err := b.accounts.LastLedgerEntries(ctx, user.ID)
	if err != nil {
		return err
	}
	return b.send(ctx, message.ChatID, formatStatus(b.language, user, balance, entries))
}

func (b *Bot) HandleHistory(ctx context.Context, message IncomingMessage) error {
	user, err := b.authenticatedUser(ctx, message.UserID)
	if err != nil {
		return b.send(ctx, message.ChatID, localized(b.language, "unlinked_history"))
	}
	entries, err := b.accounts.LastLedgerEntries(ctx, user.ID)
	if err != nil {
		return err
	}
	return b.send(ctx, message.ChatID, formatHistory(b.language, entries))
}

func (b *Bot) HandleHelp(ctx context.Context, message IncomingMessage) error {
	return b.send(ctx, message.ChatID, localized(b.language, "help"))
}

func inviteErrorText(language string, err error) string {
	switch {
	case errors.Is(err, account.ErrInviteExpired):
		return localized(language, "invite_expired")
	case errors.Is(err, account.ErrInviteAlreadyUsed):
		return localized(language, "invite_used")
	case errors.Is(err, account.ErrInviteNotFound), errors.Is(err, account.ErrInvalidInviteToken):
		return localized(language, "invite_invalid")
	case errors.Is(err, account.ErrTelegramUserIDTaken), errors.Is(err, account.ErrTelegramChatIDTaken):
		return localized(language, "invite_taken")
	default:
		return localized(language, "invite_error")
	}
}

func formatStatus(language string, user domain.User, balance domain.AmountMinor, entries []domain.LedgerEntry) string {
	if normalizeLanguage(language) == LanguageEnglish {
		state := "Balance: 0"
		if balance < 0 {
			state = fmt.Sprintf("Debt: %d %s", -balance, user.Currency)
		}
		if balance > 0 {
			state = fmt.Sprintf("Prepayment: %d %s", balance, user.Currency)
		}
		text := fmt.Sprintf("%s\nFee: %d %s\nNext charge: %s\n%s", user.DisplayName, user.MonthlyFeeMinor, user.Currency, user.NextChargeOn, state)
		for _, entry := range entries {
			if entry.Kind == domain.LedgerKindPayment {
				return text + fmt.Sprintf("\nLast payment: %d %s", entry.AmountMinor, user.Currency)
			}
		}
		return text
	}
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

func formatHistory(language string, entries []domain.LedgerEntry) string {
	if len(entries) == 0 {
		return localized(language, "history_empty")
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
