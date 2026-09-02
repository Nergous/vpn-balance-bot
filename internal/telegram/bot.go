package telegram

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Nergous/vpn-balance-bot/internal/domain"
	"github.com/Nergous/vpn-balance-bot/internal/localization"
	"github.com/Nergous/vpn-balance-bot/internal/service/account"
	"github.com/Nergous/vpn-balance-bot/internal/service/reminder"
	botapi "github.com/go-telegram/bot"
)

// ErrUnauthorized means that a Telegram user has no linked billing profile.
var (
	ErrUnauthorized        = errors.New("Telegram user is not linked")
	ErrPrivateChatRequired = errors.New("private Telegram chat required")
	ErrInvalidHTTPTimeout  = errors.New("Telegram HTTP timeout must be positive")
)

// AccountService contains customer-facing account operations used by Bot.
type AccountService interface {
	ConsumeInviteToken(context.Context, account.ConsumeInviteParams) (domain.User, error)
	UserByTelegramID(context.Context, int64) (domain.User, error)
	Balance(context.Context, domain.UserID) (domain.AmountMinor, error)
	LastLedgerEntries(context.Context, domain.UserID) ([]domain.LedgerEntry, error)
}

// LastUnreversedPaymentService is the optional account capability required to
// exclude reversed payments from customer status output.
type LastUnreversedPaymentService interface {
	LastUnreversedPayment(context.Context, domain.UserID) (domain.LedgerEntry, bool, error)
}

// ChatType identifies the Telegram chat surface that produced an update.
type ChatType string

const (
	ChatTypePrivate    ChatType = "private"
	ChatTypeGroup      ChatType = "group"
	ChatTypeSupergroup ChatType = "supergroup"
	ChatTypeChannel    ChatType = "channel"
)

// IncomingMessage is a transport-neutral Telegram message used by handlers.
type IncomingMessage struct {
	UpdateID       int64
	MessageID      int
	ChatID, UserID int64
	ChatType       ChatType
	Text, Username string
}

// IncomingCallback is a transport-neutral callback query used by handlers.
type IncomingCallback struct {
	UpdateID        int64
	MessageID       int
	CallbackQueryID string
	ChatID, UserID  int64
	ChatType        ChatType
	Data            string
}

// Bot adapts Telegram updates to account, admin, and reminder use cases.
type Bot struct {
	client   Client
	poller   poller
	accounts AccountService
	admin    *Admin
	language localization.Language
	logger   *slog.Logger
}

// New creates a production Telegram bot and validates its token through the adapter.
func New(
	token string,
	accounts AccountService,
	language localization.Language,
	httpTimeout time.Duration,
	logger *slog.Logger,
) (*Bot, error) {
	if _, err := localization.New(language); err != nil {
		return nil, err
	}
	if httpTimeout <= 0 {
		return nil, ErrInvalidHTTPTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}

	adapter := &Bot{
		accounts: accounts,
		language: language,
		logger:   logger,
	}

	client, err := newProductionBot(token, adapter, httpTimeout)
	if err != nil {
		return nil, err
	}

	adapter.client, adapter.poller = client, client
	return adapter, nil
}

// NewWithClient creates a bot with a supplied transport, primarily for tests.
func NewWithClient(client Client, accounts AccountService, language localization.Language) *Bot {
	localization.MustNew(language)
	return &Bot{
		client:   client,
		accounts: accounts,
		language: language,
		logger:   slog.Default(),
	}
}

func (m IncomingMessage) isPrivate() bool {
	return m.ChatType == ChatTypePrivate
}

func (c IncomingCallback) isPrivate() bool {
	return c.ChatType == ChatTypePrivate
}

// EnableAdmin binds admin-only handlers to the configured numeric Telegram ID.
func (b *Bot) EnableAdmin(adminID int64, reminders AdminReminderService) error {
	accounts, ok := b.accounts.(AdminAccountService)
	if !ok {
		return errors.New("account service does not support admin operations")
	}

	b.admin = NewAdmin(b.client, accounts, reminders, adminID, b.language)
	return nil
}

// Start begins long polling until ctx is cancelled.
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

// ClassifyReminderError maps Telegram transport failures to safe delivery codes.
func (b *Bot) ClassifyReminderError(err error) reminder.DeliveryErrorCode {
	switch {
	case errors.Is(err, botapi.ErrorForbidden):
		return reminder.DeliveryErrorOffline
	case botapi.IsTooManyRequestsError(err):
		return reminder.DeliveryErrorRetryable
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
