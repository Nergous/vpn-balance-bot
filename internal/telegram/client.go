package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"

	botapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// ErrInitialize means that the production Telegram client could not be created.
var ErrInitialize = errors.New("failed to initialize Telegram client")

// Client is the narrow Telegram output boundary used by handlers.
type Client interface {
	SendText(ctx context.Context, chatID int64, text string) (int, error)
	SendInviteToken(ctx context.Context, chatID int64, title, copyLabel, token string) (int, error)
}

// poller is the lifecycle boundary implemented by the production client.
type poller interface {
	Start(context.Context)
}

type productionClient struct {
	bot *botapi.Bot
}

func (c *productionClient) SendText(ctx context.Context, chatID int64, text string) (int, error) {
	message, err := c.bot.SendMessage(ctx, &botapi.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})

	if err != nil {
		return 0, err
	}

	return message.ID, nil
}

// SendInviteToken hides the token until revealed and provides native Telegram copying.
func (c *productionClient) SendInviteToken(ctx context.Context, chatID int64, title, copyLabel, token string) (int, error) {
	message, err := c.bot.SendMessage(ctx, &botapi.SendMessageParams{
		ChatID:    chatID,
		ParseMode: models.ParseModeHTML,
		Text:      fmt.Sprintf("%s\n<tg-spoiler><code>%s</code></tg-spoiler>", html.EscapeString(title), html.EscapeString(token)),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
			Text: copyLabel, CopyText: &models.CopyTextButton{Text: token},
		}}}},
	})

	if err != nil {
		return 0, err
	}

	return message.ID, nil
}

func (c *productionClient) Start(ctx context.Context) {
	c.bot.Start(ctx)
}

// newProductionBot registers all production command and callback handlers.
func newProductionBot(token string, adapter *Bot, httpTimeout time.Duration) (*productionClient, error) {
	b, err := botapi.New(token,
		botapi.WithHTTPClient(httpTimeout, &http.Client{Timeout: httpTimeout}),
		botapi.WithErrorsHandler(adapter.reportClientError),
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

	return &productionClient{
		bot: b,
	}, nil
}

// adminUpdate converts an /admin update into the transport-neutral command handler.
func (b *Bot) adminUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	message, ok := incomingMessage(update)
	if !ok {
		return
	}
	if err := b.HandleAdminCommand(ctx, message); err != nil {
		b.reportMessageError("admin", message, err)
	}
}

func (b *Bot) startUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	message, ok := incomingMessage(update)
	if !ok {
		return
	}
	if err := b.HandleStart(ctx, message); err != nil {
		b.reportMessageError("start", message, err)
	}
}

func (b *Bot) statusUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	message, ok := incomingMessage(update)
	if !ok {
		return
	}
	if err := b.HandleStatus(ctx, message); err != nil {
		b.reportMessageError("status", message, err)
	}
}

func (b *Bot) historyUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	message, ok := incomingMessage(update)
	if !ok {
		return
	}
	if err := b.HandleHistory(ctx, message); err != nil {
		b.reportMessageError("history", message, err)
	}
}

func (b *Bot) helpUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	message, ok := incomingMessage(update)
	if !ok {
		return
	}
	if err := b.HandleHelp(ctx, message); err != nil {
		b.reportMessageError("help", message, err)
	}
}
func (b *Bot) callbackUpdate(ctx context.Context, raw *botapi.Bot, update *models.Update) {
	callback, ok := incomingCallback(update)
	if update.CallbackQuery == nil {
		return
	}

	_, err := raw.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
	if err != nil {
		b.reportCallbackError("acknowledge", callback, err)
	}

	if !ok {
		return
	}
	if err := b.HandleCallback(ctx, callback); err != nil {
		b.reportCallbackError("handle", callback, err)
	}
}

func incomingMessage(update *models.Update) (IncomingMessage, bool) {
	if update == nil || update.Message == nil {
		return IncomingMessage{}, false
	}

	message := IncomingMessage{
		UpdateID:  update.ID,
		MessageID: update.Message.ID,
		ChatID:    update.Message.Chat.ID,
		ChatType:  ChatType(update.Message.Chat.Type),
		Text:      update.Message.Text,
	}
	if update.Message.From != nil {
		message.UserID = update.Message.From.ID
		message.Username = update.Message.From.Username
	}
	return message, true
}

func incomingCallback(update *models.Update) (IncomingCallback, bool) {
	if update == nil || update.CallbackQuery == nil {
		return IncomingCallback{}, false
	}

	callback := IncomingCallback{
		UpdateID:        update.ID,
		CallbackQueryID: update.CallbackQuery.ID,
		UserID:          update.CallbackQuery.From.ID,
		Data:            update.CallbackQuery.Data,
	}
	if message := update.CallbackQuery.Message.Message; message != nil {
		callback.MessageID = message.ID
		callback.ChatID = message.Chat.ID
		callback.ChatType = ChatType(message.Chat.Type)
		return callback, true
	}
	if message := update.CallbackQuery.Message.InaccessibleMessage; message != nil {
		callback.MessageID = message.MessageID
		callback.ChatID = message.Chat.ID
		callback.ChatType = ChatType(message.Chat.Type)
		return callback, true
	}
	return callback, false
}
