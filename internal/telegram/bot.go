package telegram

import (
	"context"
	"errors"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
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
}

func New(token string, accounts AccountService) (*Bot, error) {
	adapter := &Bot{accounts: accounts}
	client, err := newProductionBot(token, adapter)
	if err != nil {
		return nil, err
	}
	adapter.client, adapter.poller = client, client
	return adapter, nil
}

func NewWithClient(client Client, accounts AccountService) *Bot {
	return &Bot{client: client, accounts: accounts}
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

func (b *Bot) send(ctx context.Context, chatID int64, text string) error {
	_, err := b.client.SendText(ctx, chatID, text)
	return err
}
