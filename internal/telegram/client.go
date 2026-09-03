package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	botapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var ErrInitialize = errors.New("failed to initialize Telegram client")

type initializeError struct{ cause error }

func (e initializeError) Error() string        { return ErrInitialize.Error() }
func (e initializeError) Unwrap() error        { return e.cause }
func (e initializeError) Is(target error) bool { return target == ErrInitialize }

type Client interface {
	SendText(context.Context, int64, string) (int, error)
	SendInviteToken(context.Context, int64, string, string, string) (int, error)
}

type poller interface{ Start(context.Context) }

type productionClient struct {
	bot            *botapi.Bot
	adapter        *Bot
	httpClient     *http.Client
	pollURL        string
	pollWait       int
	updateAttempts map[int64]int
}

const maxUpdateAttempts = 3

func (c *productionClient) SendText(ctx context.Context, chatID int64, text string) (int, error) {
	var lastMessageID int
	for _, part := range splitTelegramText(text) {
		message, err := c.bot.SendMessage(ctx, &botapi.SendMessageParams{ChatID: chatID, Text: part})
		if err != nil {
			return 0, err
		}
		lastMessageID = message.ID
	}
	return lastMessageID, nil
}

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
	var offset int64
	var backoff time.Duration
	for ctx.Err() == nil {
		if backoff > 0 {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		updates, retryAfter, err := c.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.adapter.reportClientError(err)
			if retryAfter > 0 {
				backoff = retryAfter
			} else {
				backoff = nextPollBackoff(backoff)
			}
			continue
		}
		backoff = 0
		for _, update := range updates {
			if update == nil {
				continue
			}
			if err := c.adapter.processUpdate(ctx, update, c.bot); err != nil {
				if ctx.Err() != nil {
					return
				}
				c.adapter.reportUpdateError(update, err)
				if c.acknowledgeFailedUpdate(update.ID, err) {
					offset = update.ID + 1
					continue
				}
				backoff = nextPollBackoff(backoff)
				break
			}
			delete(c.updateAttempts, update.ID)
			offset = update.ID + 1
		}
	}
}

func (c *productionClient) acknowledgeFailedUpdate(updateID int64, err error) bool {
	if !isRetryableUpdateError(err) {
		delete(c.updateAttempts, updateID)
		c.adapter.reportDroppedUpdate(updateID, err, 1)
		return true
	}
	if c.updateAttempts == nil {
		c.updateAttempts = make(map[int64]int)
	}
	c.updateAttempts[updateID]++
	if c.updateAttempts[updateID] < maxUpdateAttempts {
		return false
	}
	attempts := c.updateAttempts[updateID]
	delete(c.updateAttempts, updateID)
	c.adapter.reportDroppedUpdate(updateID, err, attempts)
	return true
}

func isRetryableUpdateError(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrPrivateChatRequired),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrAdminOnly),
		errors.Is(err, ErrTransactionalSend),
		errors.Is(err, botapi.ErrorBadRequest),
		errors.Is(err, botapi.ErrorForbidden),
		errors.Is(err, botapi.ErrorUnauthorized),
		errors.Is(err, botapi.ErrorNotFound):
		return false
	case botapi.IsTooManyRequestsError(err),
		errors.Is(err, context.DeadlineExceeded):
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) || !errors.Is(err, context.Canceled)
}

func nextPollBackoff(previous time.Duration) time.Duration {
	if previous <= 0 {
		return 100 * time.Millisecond
	}
	previous *= 2
	if previous > 5*time.Second {
		return 5 * time.Second
	}
	return previous
}

type getUpdatesResponse struct {
	OK         bool             `json:"ok"`
	Result     []*models.Update `json:"result"`
	ErrorCode  int              `json:"error_code"`
	Parameters struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (c *productionClient) getUpdates(ctx context.Context, offset int64) ([]*models.Update, time.Duration, error) {
	form := url.Values{}
	form.Set("offset", strconv.FormatInt(offset, 10))
	form.Set("limit", "25")
	form.Set("timeout", strconv.Itoa(c.pollWait))
	form.Set("allowed_updates", `["message","callback_query"]`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.pollURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, errors.New("create Telegram polling request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("telegram polling transport: %w", err)
	}
	defer response.Body.Close()
	var payload getUpdatesResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, 0, errors.New("decode Telegram polling response")
	}
	if response.StatusCode != http.StatusOK || !payload.OK {
		return nil, time.Duration(payload.Parameters.RetryAfter) * time.Second, fmt.Errorf("telegram polling API error %d", payload.ErrorCode)
	}
	return payload.Result, 0, nil
}

func newProductionBot(token string, adapter *Bot, httpTimeout time.Duration) (*productionClient, error) {
	httpClient := &http.Client{Timeout: httpTimeout}
	b, err := botapi.New(token,
		botapi.WithHTTPClient(httpTimeout, httpClient),
		botapi.WithErrorsHandler(adapter.reportClientError),
	)
	if err != nil {
		return nil, initializeError{cause: err}
	}
	return &productionClient{
		bot:            b,
		adapter:        adapter,
		httpClient:     httpClient,
		pollURL:        "https://api.telegram.org/bot" + token + "/getUpdates",
		pollWait:       max(1, int(httpTimeout.Seconds())-1),
		updateAttempts: make(map[int64]int),
	}, nil
}

func (b *Bot) processUpdate(ctx context.Context, update *models.Update, raw *botapi.Bot) error {
	if update == nil {
		return nil
	}
	handle := func(handlerCtx context.Context) error { return b.routeUpdate(handlerCtx, update, raw) }
	if isManualReminderUpdate(update) {
		err := handle(ctx)
		var acknowledgementErr adminAcknowledgementError
		if errors.As(err, &acknowledgementErr) {
			b.reportUpdateError(update, acknowledgementErr)
			return nil
		}
		return err
	}
	if b.updates != nil && isStateChangingUpdate(update) {
		handlerCtx, effects := withPostCommitQueue(ctx)
		processed, err := b.updates.ProcessTelegramUpdate(handlerCtx, update.ID, handle)
		if err != nil {
			return err
		}
		if processed {
			b.reportUpdateError(update, effects.flush(ctx))
		}
		return nil
	}
	return handle(ctx)
}

func (b *Bot) routeUpdate(ctx context.Context, update *models.Update, raw *botapi.Bot) error {
	if update.CallbackQuery != nil {
		callback, ok := incomingCallback(update)
		if _, err := raw.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID}); err != nil {
			b.reportCallbackError("acknowledge", callback, err)
		}
		if !ok {
			return nil
		}
		return b.HandleCallback(ctx, callback)
	}
	message, ok := incomingMessage(update)
	if !ok {
		return nil
	}
	switch telegramCommand(message.Text) {
	case "start":
		return b.HandleStart(ctx, message)
	case "status":
		return b.HandleStatus(ctx, message)
	case "history":
		return b.HandleHistory(ctx, message)
	case "help":
		return b.HandleHelp(ctx, message)
	case "admin":
		return b.HandleAdminCommand(ctx, message)
	default:
		return nil
	}
}

// statusUpdate remains a narrow compatibility adapter for direct handler tests.
func (b *Bot) statusUpdate(ctx context.Context, _ *botapi.Bot, update *models.Update) {
	message, ok := incomingMessage(update)
	if !ok {
		return
	}
	if err := b.HandleStatus(ctx, message); err != nil {
		b.reportMessageError("status", message, err)
	}
}

func telegramCommand(text string) string {
	parts := strings.Fields(text)
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "/") {
		return ""
	}
	command := strings.TrimPrefix(parts[0], "/")
	if index := strings.IndexByte(command, '@'); index >= 0 {
		command = command[:index]
	}
	return strings.ToLower(command)
}

func isStateChangingUpdate(update *models.Update) bool {
	if update == nil || update.Message == nil {
		return false
	}
	parts := strings.Fields(update.Message.Text)
	command := telegramCommand(update.Message.Text)
	if command == "start" {
		return len(parts) > 1
	}
	if command != "admin" || len(parts) < 2 {
		return false
	}
	switch strings.ToLower(parts[1]) {
	case "invite", "create", "pause", "disable", "resume", "fee", "opening", "adjustment", "reverse", "payment", "confirm", "cancel", "remind", "reconcile":
		return true
	default:
		return false
	}
}

func isManualReminderUpdate(update *models.Update) bool {
	if update == nil || update.Message == nil {
		return false
	}
	parts := strings.Fields(update.Message.Text)
	return telegramCommand(update.Message.Text) == "admin" && len(parts) >= 2 && strings.EqualFold(parts[1], "remind")
}

func incomingMessage(update *models.Update) (IncomingMessage, bool) {
	if update == nil || update.Message == nil {
		return IncomingMessage{}, false
	}
	message := IncomingMessage{
		UpdateID: update.ID, MessageID: update.Message.ID, ChatID: update.Message.Chat.ID,
		ChatType: ChatType(update.Message.Chat.Type), Text: update.Message.Text,
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
		UpdateID: update.ID, CallbackQueryID: update.CallbackQuery.ID,
		UserID: update.CallbackQuery.From.ID, Data: update.CallbackQuery.Data,
	}
	if message := update.CallbackQuery.Message.Message; message != nil {
		callback.MessageID, callback.ChatID, callback.ChatType = message.ID, message.Chat.ID, ChatType(message.Chat.Type)
		return callback, true
	}
	if message := update.CallbackQuery.Message.InaccessibleMessage; message != nil {
		callback.MessageID, callback.ChatID, callback.ChatType = message.MessageID, message.Chat.ID, ChatType(message.Chat.Type)
		return callback, true
	}
	return callback, false
}
