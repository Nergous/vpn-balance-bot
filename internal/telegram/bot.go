package telegram

import (
	"context"
	"errors"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	"github.com/Nergous/vpn-balance-bot/internal/service/reminder"
	botapi "github.com/go-telegram/bot"
)

var ErrUnauthorized = errors.New("Telegram user is not linked")

type AccountService interface {
	ConsumeInviteToken(context.Context, account.ConsumeInviteParams) (domain.User, error)
	UserByTelegramID(context.Context, int64) (domain.User, error)
	Balance(context.Context, domain.UserID) (domain.AmountMinor, error)
	LastLedgerEntries(context.Context, domain.UserID) ([]domain.LedgerEntry, error)
}

type IncomingMessage struct {
	ChatID, UserID int64
	Text, Username string
}
type IncomingCallback struct {
	ChatID, UserID int64
	Data           string
}

type Bot struct {
	client   Client
	poller   poller
	accounts AccountService
	admin    *Admin
	language string
}

func New(token string, accounts AccountService, language string) (*Bot, error) {
	adapter := &Bot{accounts: accounts, language: normalizeLanguage(language)}
	client, err := newProductionBot(token, adapter)
	if err != nil {
		return nil, err
	}
	adapter.client, adapter.poller = client, client
	return adapter, nil
}

func NewWithClient(client Client, accounts AccountService, language ...string) *Bot {
	selected := LanguageRussian
	if len(language) > 0 {
		selected = language[0]
	}
	return &Bot{client: client, accounts: accounts, language: normalizeLanguage(selected)}
}

// EnableAdmin binds admin-only handlers to the configured numeric Telegram ID.
func (b *Bot) EnableAdmin(adminID int64, reminders ...AdminReminderService) error {
	accounts, ok := b.accounts.(AdminAccountService)
	if !ok {
		return errors.New("account service does not support admin operations")
	}
	b.admin = NewAdmin(b.client, accounts, adminID, reminders...)
	b.admin.language = b.language
	return nil
}
func (b *Bot) Start(ctx context.Context) {
	if b.poller != nil {
		b.poller.Start(ctx)
	}
}

// SendReminder implements reminder.Sender using the same Telegram transport
// that serves user commands.
func (b *Bot) SendReminder(ctx context.Context, chatID int64, text string) (int, error) {
	return b.client.SendText(ctx, chatID, text)
}

func (b *Bot) ClassifyReminderError(err error) reminder.DeliveryErrorCode {
	switch {
	case errors.Is(err, botapi.ErrorForbidden):
		return reminder.DeliveryErrorOffline
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return reminder.DeliveryErrorUnknown
	default:
		return reminder.DeliveryErrorFailed
	}
}

func (b *Bot) send(ctx context.Context, chatID int64, text string) error {
	_, err := b.client.SendText(ctx, chatID, text)
	return err
}
