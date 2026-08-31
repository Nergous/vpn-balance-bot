package telegram

import "context"

func (b *Bot) HandleCallback(ctx context.Context, callback IncomingCallback) error {
	switch callback.Data {
	case "user_status":
		return b.HandleStatus(ctx, IncomingMessage{ChatID: callback.ChatID, UserID: callback.UserID})
	case "user_history":
		return b.HandleHistory(ctx, IncomingMessage{ChatID: callback.ChatID, UserID: callback.UserID})
	case "user_help":
		return b.HandleHelp(ctx, IncomingMessage{ChatID: callback.ChatID, UserID: callback.UserID})
	default:
		return b.send(ctx, callback.ChatID, localized(b.language, "callback_unknown"))
	}
}
