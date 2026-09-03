package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/localization"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
)

// HandleStart binds an invite token or sends the command overview.
func (b *Bot) HandleStart(ctx context.Context, message IncomingMessage) error {
	parts := strings.Fields(message.Text)

	if len(parts) >= 2 {
		if !message.isPrivate() {
			return ErrPrivateChatRequired
		}

		_, err := b.accounts.ConsumeInviteToken(ctx, account.ConsumeInviteParams{
			Token:          parts[1],
			TelegramUserID: message.UserID,
			TelegramChatID: message.ChatID,
		})

		if err == nil {
			return b.send(ctx, message.ChatID, localized(b.language, "StartBound"))
		}

		if text, known := inviteErrorText(b.language, err); known {
			return b.send(ctx, message.ChatID, text)
		}
		return err
	}

	return b.send(ctx, message.ChatID, localized(b.language, "StartWelcome"))
}

// HandleStatus sends the linked user's own balance and next charge details.
func (b *Bot) HandleStatus(ctx context.Context, message IncomingMessage) error {
	if !message.isPrivate() {
		return ErrPrivateChatRequired
	}

	user, err := b.authenticatedUser(ctx, message.UserID, message.ChatID)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return b.send(ctx, message.ChatID, localized(b.language, "UnlinkedStatus"))
		}
		return err
	}

	balance, err := b.accounts.Balance(ctx, user.ID)
	if err != nil {
		return err
	}

	lastPayment, found, err := b.lastUnreversedPayment(ctx, user.ID)
	if err != nil {
		return err
	}

	return b.send(ctx, message.ChatID, formatStatus(b.language, user, balance, lastPayment, found))
}

// HandleHistory sends up to ten recent ledger entries for the linked user.
func (b *Bot) HandleHistory(ctx context.Context, message IncomingMessage) error {
	if !message.isPrivate() {
		return ErrPrivateChatRequired
	}

	user, err := b.authenticatedUser(ctx, message.UserID, message.ChatID)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return b.send(ctx, message.ChatID, localized(b.language, "UnlinkedHistory"))
		}
		return err
	}

	entries, err := b.accounts.LastLedgerEntries(ctx, user.ID)
	if err != nil {
		return err
	}

	return b.send(ctx, message.ChatID, formatHistory(b.language, entries))
}

// HandleHelp sends the supported customer commands.
func (b *Bot) HandleHelp(ctx context.Context, message IncomingMessage) error {
	return b.send(ctx, message.ChatID, localized(b.language, "Help"))
}

func inviteErrorText(language localization.Language, err error) (string, bool) {
	switch {
	case errors.Is(err, account.ErrInviteExpired):
		return localized(language, "InviteExpired"), true

	case errors.Is(err, account.ErrInviteAlreadyUsed):
		return localized(language, "InviteUsed"), true

	case errors.Is(err, account.ErrInviteNotFound), errors.Is(err, account.ErrInvalidInviteToken):
		return localized(language, "InviteInvalid"), true

	case errors.Is(err, account.ErrTelegramUserIDTaken), errors.Is(err, account.ErrTelegramChatIDTaken):
		return localized(language, "InviteTaken"), true

	default:
		return "", false
	}
}

func (b *Bot) lastUnreversedPayment(ctx context.Context, userID domain.UserID) (domain.LedgerEntry, bool, error) {
	if accounts, ok := b.accounts.(LastUnreversedPaymentService); ok {
		return accounts.LastUnreversedPayment(ctx, userID)
	}

	entries, err := b.accounts.LastLedgerEntries(ctx, userID)
	if err != nil {
		return domain.LedgerEntry{}, false, err
	}
	for _, entry := range entries {
		if entry.Kind == domain.LedgerKindPayment {
			return entry, true, nil
		}
	}

	return domain.LedgerEntry{}, false, nil
}

func formatStatus(
	language localization.Language,
	user domain.User,
	balance domain.AmountMinor,
	lastPayment domain.LedgerEntry,
	hasLastPayment bool,
) string {
	state := localized(language, "StatusZero")

	if balance < 0 {
		state = localized(language, "StatusDebt", amountMessageData{Amount: formatAmountMinor(-balance), Currency: user.Currency})
	}

	if balance > 0 {
		state = localized(language, "StatusPrepayment", amountMessageData{
			Amount:         formatAmountMinor(balance),
			Currency:       user.Currency,
			CoveredPeriods: balance.Int64() / user.MonthlyFeeMinor.Int64(),
		})
	}

	text := localized(language, "Status", statusMessageData{
		Name: user.DisplayName, Fee: formatAmountMinor(user.MonthlyFeeMinor), Currency: user.Currency,
		NextCharge: user.NextChargeOn, Balance: state,
	})

	if hasLastPayment {
		return text + localized(language, "LastPayment", amountMessageData{
			Amount:   formatAmountMinor(lastPayment.AmountMinor),
			Currency: user.Currency,
		})
	}

	return text
}

func formatHistory(language localization.Language, entries []domain.LedgerEntry) string {
	if len(entries) == 0 {
		return localized(language, "HistoryEmpty")
	}

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		note := ""
		if entry.Note != nil {
			note = " — " + *entry.Note
		}

		lines = append(lines, fmt.Sprintf("%s · %s · %s %s%s", entry.OccurredAt.Format("2006-01-02"), localizedLedgerKind(language, entry.Kind), formatSignedAmountMinor(entry.AmountMinor), "RUB", note))
	}

	return strings.Join(lines, "\n")
}

func localizedLedgerKind(language localization.Language, kind domain.LedgerKind) string {
	var id string
	switch kind {
	case domain.LedgerKindOpeningBalance:
		id = "LedgerKindOpeningBalance"
	case domain.LedgerKindPayment:
		id = "LedgerKindPayment"
	case domain.LedgerKindSubscriptionCharge:
		id = "LedgerKindSubscriptionCharge"
	case domain.LedgerKindAdjustment:
		id = "LedgerKindAdjustment"
	case domain.LedgerKindReversal:
		id = "LedgerKindReversal"
	default:
		return string(kind)
	}
	return localized(language, id)
}
