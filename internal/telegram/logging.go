package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

func (b *Bot) reportMessageError(operation string, message IncomingMessage, err error) {
	if err == nil {
		return
	}

	b.logger.Error("Telegram message handler failed",
		slog.String("operation", operation),
		slog.Int64("update_id", message.UpdateID),
		slog.Int("message_id", message.MessageID),
		slog.Int64("chat_id", message.ChatID),
		slog.String("chat_type", string(message.ChatType)),
		slog.Int64("telegram_user_id", message.UserID),
		slog.String("error_type", fmt.Sprintf("%T", err)),
		slog.String("error_kind", safeErrorKind(err)),
	)
}

func (b *Bot) reportCallbackError(operation string, callback IncomingCallback, err error) {
	if err == nil {
		return
	}

	b.logger.Error("Telegram callback failed",
		slog.String("operation", operation),
		slog.Int64("update_id", callback.UpdateID),
		slog.Int("message_id", callback.MessageID),
		slog.String("callback_query_id", callback.CallbackQueryID),
		slog.Int64("chat_id", callback.ChatID),
		slog.String("chat_type", string(callback.ChatType)),
		slog.Int64("telegram_user_id", callback.UserID),
		slog.String("error_type", fmt.Sprintf("%T", err)),
		slog.String("error_kind", safeErrorKind(err)),
	)
}

func (b *Bot) reportClientError(err error) {
	if err == nil {
		return
	}

	b.logger.Error("Telegram client failed",
		slog.String("operation", "long_polling"),
		slog.String("error_type", fmt.Sprintf("%T", err)),
		slog.String("error_kind", safeErrorKind(err)),
	)
}

func safeErrorKind(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrPrivateChatRequired):
		return "private_chat_required"
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, ErrAdminOnly):
		return "admin_only"
	default:
		return "failed"
	}
}
