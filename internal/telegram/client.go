package telegram

import (
	"context"
	"errors"

	botapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var ErrInitialize = errors.New("failed to initialize Telegram client")

// Client is the narrow Telegram output boundary used by handlers.
type Client interface {
	SendText(ctx context.Context, chatID int64, text string) (int, error)
}

type poller interface{ Start(context.Context) }

type productionClient struct{ bot *botapi.Bot }

func (c *productionClient) SendText(ctx context.Context, chatID int64, text string) (int, error) {
	message, err := c.bot.SendMessage(ctx, &botapi.SendMessageParams{ChatID: chatID, Text: text})
	if err != nil {
		return 0, err
	}
	return message.ID, nil
}

func (c *productionClient) Start(ctx context.Context) { c.bot.Start(ctx) }

func newProductionBot(token string, adapter *Bot) (*productionClient, error) {
	b, err := botapi.New(token,
		botapi.WithMessageTextHandler("start", botapi.MatchTypeCommand, adapter.startUpdate),
		botapi.WithMessageTextHandler("status", botapi.MatchTypeCommand, adapter.statusUpdate),
		botapi.WithMessageTextHandler("history", botapi.MatchTypeCommand, adapter.historyUpdate),
		botapi.WithMessageTextHandler("help", botapi.MatchTypeCommand, adapter.helpUpdate),
		botapi.WithMessageTextHandler("admin", botapi.MatchTypeCommand, adapter.adminUpdate),
		botapi.WithCallbackQueryDataHandler("user_", botapi.MatchTypePrefix, adapter.callbackUpdate),
	)
	if err != nil {
		return nil, ErrInitialize
	}
	return &productionClient{bot: b}, nil
}
func (b *Bot) adminUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	if update.Message != nil {
		_ = b.HandleAdminCommand(ctx, IncomingMessage{ChatID: update.Message.Chat.ID, UserID: update.Message.From.ID, Text: update.Message.Text})
	}
}

func (b *Bot) startUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	_ = b.HandleStart(ctx, IncomingMessage{ChatID: update.Message.Chat.ID, UserID: update.Message.From.ID, Text: update.Message.Text, Username: update.Message.From.Username})
}
func (b *Bot) statusUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	if update.Message != nil {
		_ = b.HandleStatus(ctx, IncomingMessage{ChatID: update.Message.Chat.ID, UserID: update.Message.From.ID, Username: update.Message.From.Username})
	}
}
func (b *Bot) historyUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	if update.Message != nil {
		_ = b.HandleHistory(ctx, IncomingMessage{ChatID: update.Message.Chat.ID, UserID: update.Message.From.ID, Username: update.Message.From.Username})
	}
}
func (b *Bot) helpUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	if update.Message != nil {
		_ = b.HandleHelp(ctx, IncomingMessage{ChatID: update.Message.Chat.ID, UserID: update.Message.From.ID})
	}
}
func (b *Bot) callbackUpdate(ctx context.Context, raw *botapi.Bot, update *models.Update) {
	if update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return
	}
	_, _ = raw.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
	_ = b.HandleCallback(ctx, IncomingCallback{ChatID: update.CallbackQuery.Message.Message.Chat.ID, UserID: update.CallbackQuery.From.ID, Data: update.CallbackQuery.Data})
}
